package service

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/google/uuid"
	modelnew "github.com/webitel/chat-migration-cli/internal/model/new"
	modelold "github.com/webitel/chat-migration-cli/internal/model/old"
	"github.com/webitel/chat-migration-cli/internal/model/old/proto"
)

func (c *Converter) MigrateFacebookProviders(ctx context.Context) error {
	var (
		perPage = 1000
	)
	c.log.Debug("starting facebook/whatsapp providers migration")
	tx, err := c.newDB.Pool().Begin(ctx)
	if err != nil {
		return err
	}
	err = PagerFunc(ctx, perPage, func(ctx context.Context, offset, limit int) (bool, error) {
		iterate := true
		providers, err := c.oldDB.BotStore().GetMetaGateways(ctx, offset, limit)
		if err != nil {
			return false, err
		}
		if len(providers) < limit {
			iterate = false
		}
		c.log.Debug("providers page fetched", "offset", offset, "count", len(providers))
		appsOldNewMap, gatesOldNewMap, err := c.BuildMetaGates(providers)
		if err != nil {
			return false, err
		}
		var (
			gates []*modelnew.Gate
			apps  []*modelnew.MetaApp
			wabas []*modelnew.GateWABA
			pages []*modelnew.Facebook
			bots  []*modelnew.Bot
		)
		migrationRows := []*modelnew.MigrationRow{}
		for oldID, providerGates := range gatesOldNewMap {
			for _, gate := range providerGates {
				gates = append(gates, gate)
				if gate.FacebookPage != nil {
					pages = append(pages, gate.FacebookPage)
				} else if gate.WhatsAppAccount != nil {
					wabas = append(wabas, gate.WhatsAppAccount)
				} else {
					continue
				}
				migrationRows = append(migrationRows,
					&modelnew.MigrationRow{
						ID:         uuid.New(),
						OldID:      strconv.Itoa(oldID),
						NewID:      gate.ID,
						DomainID:   int(gate.DC),
						EntityType: modelnew.EntityTypeProviderToGateway,
					})

				bots = append(bots, gate.Bot)

			}
		}
		for oldID, app := range appsOldNewMap {
			apps = append(apps, app)
			migrationRows = append(migrationRows,
				&modelnew.MigrationRow{
					ID:         uuid.New(),
					OldID:      strconv.Itoa(oldID),
					NewID:      app.ID,
					DomainID:   app.DomainID,
					EntityType: modelnew.EntityTypeProviderToMetaApp,
				})
		}
		err = c.newDB.ProviderStore().InsertMetaApps(ctx, tx, apps)
		if err != nil {
			return false, err
		}
		err = c.newDB.ProviderStore().InsertGates(ctx, tx, gates)
		if err != nil {
			return false, err
		}
		err = c.newDB.ProviderStore().InsertFacebooks(ctx, tx, pages)
		if err != nil {
			return false, err
		}
		err = c.newDB.ProviderStore().InsertGateWABAs(ctx, tx, wabas)
		if err != nil {
			return false, err
		}
		err = c.newDB.ProviderStore().InsertBots(ctx, tx, bots)
		if err != nil {
			return false, err
		}
		err = c.newDB.MigrationStore().InsertMigrations(ctx, tx, migrationRows)
		if err != nil {
			return false, err
		}
		return iterate, nil
	})

	if err != nil {
		tx.Rollback(ctx)
		return err
	}
	return tx.Commit(ctx)
}

func (c *Converter) BuildMetaGates(providers []*modelold.Provider[modelold.FBProviderMetadata]) (map[int]*modelnew.MetaApp, map[int][]*modelnew.Gate, error) {
	resultGates := map[int][]*modelnew.Gate{}
	resultApps := map[int]*modelnew.MetaApp{}
	for _, provider := range providers {
		gates := []*modelnew.Gate{}
		metadata := provider.Metadata
		if metadata == nil {
			c.log.Warn("metadata is nil", slog.Int("provider_id", provider.ID))
			continue
		}
		metaApp := &modelnew.MetaApp{
			ID:          uuid.New(),
			Name:        provider.Name,
			AppID:       metadata.ClientID,
			AppSecret:   metadata.ClientSecret,
			URI:         provider.URI,
			CreatedAt:   provider.CreatedAt,
			UpdatedAt:   provider.UpdatedAt,
			VerifyToken: RandomBase64String(64),
			DomainID:    provider.DC,
		}

		if metadata.FB != nil {
			result, err := c.BuildFBGates(metaApp, provider)
			if err != nil {
				return nil, nil, err
			}
			gates = append(gates, result...)
		}

		if metadata.WA != "" {
			result, err := c.BuildWAGates(metaApp, provider)
			if err != nil {
				return nil, nil, err
			}
			gates = append(gates, result...)
		}

		if metadata.IG != nil {
			metaApp.Scopes = append(metaApp.Scopes,
				"instagram_basic",
				"instagram_manage_messages",
			)
		}
		if len(metaApp.Scopes) == 0 {
			slog.Warn("provider %d has no scopes, skipping", slog.Int("provider_id", provider.ID))
			continue
		}

		resultGates[provider.ID] = gates
		resultApps[provider.ID] = metaApp

	}
	return resultApps, resultGates, nil
}

func (c *Converter) BuildFBGates(metaApp *modelnew.MetaApp, provider *modelold.Provider[modelold.FBProviderMetadata]) ([]*modelnew.Gate, error) {
	var gates []*modelnew.Gate
	metaApp.Scopes = append(metaApp.Scopes,
		"pages_show_list",
		"pages_messaging",
		"pages_manage_metadata",
	)
	fbPages, err := c.convertToFacebookPages(provider.Metadata.FB)
	if err != nil {
		return nil, err
	}
	for _, page := range fbPages {
		gate, err := c.buildGate(provider, "facebook")
		if err != nil {
			return nil, err
		}
		page.GateID = gate.ID
		page.MetaAppID = metaApp.ID
		gate.FacebookPage = page
		gates = append(gates, gate)
	}
	return gates, nil
}

func (c *Converter) BuildWAGates(metaApp *modelnew.MetaApp, provider *modelold.Provider[modelold.FBProviderMetadata]) ([]*modelnew.Gate, error) {
	var gates []*modelnew.Gate
	metaApp.Scopes = append(metaApp.Scopes,
		"whatsapp_bussiness_management",
		"whatsapp_bussiness_messaging",
	)
	waAccounts, err := c.convertToWABAAccounts(provider.Metadata.WhatsAppToken, provider.Metadata.WA)
	if err != nil {
		return nil, err
	}
	for _, account := range waAccounts {
		gate, err := c.buildGate(provider, "whatsapp")
		if err != nil {
			return nil, err
		}
		account.MetaAppID = metaApp.ID
		account.ID = gate.ID
		gate.WhatsAppAccount = account
		gates = append(gates, gate)
	}
	return gates, nil
}

func (c *Converter) buildGate(provider *modelold.Provider[modelold.FBProviderMetadata], providerType string) (*modelnew.Gate, error) {
	metadata := provider.Metadata
	if metadata == nil {
		return nil, fmt.Errorf("metadata is nil")
	}
	gateID := uuid.New()
	gate := &modelnew.Gate{
		ID:        gateID,
		DC:        int64(provider.DC),
		Name:      fmt.Sprintf("%s (%s)", provider.Name, providerType),
		Enabled:   provider.Enabled,
		CreatedAt: provider.CreatedAt,
		UpdatedAt: provider.UpdatedAt,

		Bot: &modelnew.Bot{
			ID:        uuid.New(),
			Sub:       "schema",
			Iss:       strconv.Itoa(provider.FlowID),
			GateID:    gateID,
			CreatedAt: provider.CreatedAt,
		},
	}
	return gate, nil
}

func RandomBase64String(n int) string {
	encoding := base64.RawURLEncoding
	buf := make([]byte, encoding.DecodedLen(n))
	if _, err := io.ReadFull(rand.Reader, buf); err != nil {
		panic(err)
	}
	text := encoding.EncodeToString(buf)
	return text[:n]
}

func (c *Converter) convertToFacebookPages(fb *proto.Messenger) ([]*modelnew.Facebook, error) {
	pages := make([]*modelnew.Facebook, 0, len(fb.Pages))
	for _, page := range fb.Pages {
		if page == nil {
			continue
		}
		if len(page.Accounts) == 0 {
			continue
		}
		pages = append(pages, &modelnew.Facebook{
			PageID:    page.Id,
			PageToken: page.Accounts[0].AccessToken,
		})

	}
	return pages, nil
}

func (c *Converter) convertToWABAAccounts(token, encoded string) ([]*modelnew.GateWABA, error) {
	accounts, err := c.fetchAccounts(token, encoded)
	if err != nil {
		return nil, err
	}
	var result []*modelnew.GateWABA
	for _, account := range accounts {
		for _, number := range account.PhoneNumbers.Data {
			result = append(result, &modelnew.GateWABA{
				ID:                   uuid.New(),
				PhoneNumber:          number.PhoneNumber,
				PhoneNumberID:        number.ID,
				AccessToken:          []byte(token),
				AccessTokenExpiresAt: nil,
				BusinessID:           account.ID,
			})
		}
	}
	return result, nil
}

type PhoneNumber struct {
	ID           string `json:"id"`
	PhoneNumber  string `json:"display_phone_number"`
	VerifiedName string `json:"verified_name"`
}

type BusinessAccount struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	PhoneNumbers struct {
		Data []*PhoneNumber `json:"data"`
	} `json:"phone_numbers"`
}

func (c *Converter) fetchAccounts(waToken, waEncoded string) ([]*BusinessAccount, error) {
	wabaIDs, err := decodeWABAIDs(waEncoded)
	if err != nil {
		return nil, err
	}

	accounts, err := fetchAccounts(waToken, wabaIDs)
	if err != nil {
		return nil, err
	}

	return accounts, nil
}

// --- Decode "wa" metadata value into WABA IDs ---

func decodeWABAIDs(encoded string) ([]string, error) {
	data, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, err
	}
	const (
		offset = '0'       // 0x30
		delim  = ':' - '0' // 0x0A
	)
	var ids []string
	for _, part := range bytes.Split(data, []byte{delim}) {
		if len(part) == 0 {
			continue
		}
		ascii := make([]byte, len(part))
		for i, b := range part {
			ascii[i] = b + offset
		}
		ids = append(ids, string(ascii))
	}
	return ids, nil
}

// --- Fetch accounts from Meta Graph API ---

func fetchAccounts(token string, wabaIDs []string) ([]*BusinessAccount, error) {
	params := url.Values{
		"ids":          {strings.Join(wabaIDs, ",")},
		"fields":       {"id,name,phone_numbers{id,display_phone_number,verified_name}"},
		"access_token": {token},
	}
	resp, err := http.Get("https://graph.facebook.com/v19.0/?" + params.Encode())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var result map[string]*BusinessAccount
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	accounts := make([]*BusinessAccount, 0, len(result))
	for _, a := range result {
		accounts = append(accounts, a)
	}
	return accounts, nil
}
