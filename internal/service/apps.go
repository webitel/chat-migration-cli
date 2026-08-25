package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	modelnew "github.com/webitel/chat-migration-cli/internal/model/new"
	"github.com/webitel/chat-migration-cli/internal/model/old"
)

func (c *Converter) MigratePortalAppsToAccounts(ctx context.Context) error {
	var (
		perPage = 1000
	)
	c.log.Debug("starting portal-apps-to-accounts migration")
	tx, err := c.newDB.Pool().Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	err = PagerFunc(ctx, perPage, func(ctx context.Context, offset, limit int) (bool, error) {
		iterate := true
		apps, err := c.oldDB.AppStore().Get(ctx, offset, limit)
		if err != nil {
			return false, err
		}
		if len(apps) < limit {
			iterate = false
		}
		c.log.Debug("portal apps page fetched", "offset", offset, "count", len(apps))
		var (
			convertedApps []*modelnew.App
			migrationRows []*modelnew.MigrationRow
		)
		for _, app := range apps {
			account, err := convertPortalAppToAccount(c.log, app)
			if err != nil {
				return false, fmt.Errorf("convert portal app %s: %w", app.ID, err)
			}
			convertedApps = append(convertedApps, account)
			migrationRows = append(migrationRows, &modelnew.MigrationRow{
				ID:         uuid.New(),
				EntityType: modelnew.EntityTypePortalAppAccount,
				OldID:      app.ID.String(),
				NewID:      account.ID,
				DomainID:   account.DomainID,
			})
		}
		if err := c.newDB.AppStore().InsertApps(ctx, tx, convertedApps); err != nil {
			return false, fmt.Errorf("insert accounts: %w", err)
		}
		if err := c.newDB.MigrationStore().InsertMigrations(ctx, tx, migrationRows); err != nil {
			return false, fmt.Errorf("insert migration rows: %w", err)
		}
		c.addRecordsMigrated(len(convertedApps))
		return iterate, nil
	})
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func convertPortalAppToAccount(log *slog.Logger, app *old.PortalApp) (*modelnew.App, error) {
	config := &modelnew.AppConfig{}

	hasClientConfig := len(app.UA) > 0 || len(app.Net) > 0 || len(app.Web) > 0 || len(app.Issuers) > 0 ||
		app.JwksURI != nil || len(app.Jwks) > 0 || len(app.JwtIdentity) > 0
	if hasClientConfig {
		clients := &modelnew.AppClients{Ua: app.UA}
		if len(app.Net) > 0 {
			clients.Net = &modelnew.ClientNet{Cidr: app.Net}
		}
		if len(app.Web) > 0 {
			clients.Web = &modelnew.ClientWeb{Origin: app.Web}
		}
		if len(app.Issuers) > 0 {
			idp := make(map[string]string, len(app.Issuers))
			for _, issuer := range app.Issuers {
				idp[issuer] = ""
			}
			clients.Idp = idp
		}

		var jwtClaims map[string]string
		if len(app.JwtIdentity) > 0 {
			if err := json.Unmarshal(app.JwtIdentity, &jwtClaims); err != nil {
				log.Warn("portal app jwt_identity is not a flat string map, dropping claims", "app_id", app.ID, "error", err)
				jwtClaims = nil
			}
		}
		jwksURI := ""
		if app.JwksURI != nil {
			jwksURI = *app.JwksURI
		}
		if jwksURI != "" || len(app.Jwks) > 0 || len(jwtClaims) > 0 {
			clients.Jwt = &modelnew.JwtIdentity{
				Enabled: true,
				JwksUri: jwksURI,
				Jwks:    app.Jwks,
				Claims:  jwtClaims,
			}
		}
		config.Clients = clients
	}

	// old service_app.token is intentionally NOT carried over: its runtime
	// compatibility with the new AppService.Secret field is unconfirmed.
	service := &modelnew.AppServiceConfig{}
	service.PushService = validJSONOrNil(log, app.ID, "push", app.Push)
	service.RateLimits = validJSONOrNil(log, app.ID, "limit", app.Limit)
	if service.PushService != nil || service.RateLimits != nil || service.SendUpdate != nil || service.Secret != "" {
		config.Service = service
	}

	configBytes, err := json.Marshal(config)
	if err != nil {
		return nil, err
	}

	return &modelnew.App{
		ID:        uuid.New(),
		DomainID:  app.DomainID,
		Name:      app.Name,
		About:     nil,
		Config:    configBytes,
		CreatedAt: app.CreatedAt,
		UpdatedAt: &app.UpdatedAt,
		// old service_app.expires_at has no equivalent on im_account.app and is
		// intentionally dropped; revoked_at is never populated by this migration.
		RevokedAt: nil,
	}, nil
}

// validJSONOrNil drops malformed input instead of erroring, so one app's bad
// jsonb column can't abort the whole migration step.
func validJSONOrNil(log *slog.Logger, appID uuid.UUID, field string, raw []byte) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	if !json.Valid(raw) {
		log.Warn("portal app field is not valid JSON, dropping from migrated config", "app_id", appID, "field", field)
		return nil
	}
	return json.RawMessage(raw)
}
