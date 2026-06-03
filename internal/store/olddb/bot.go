package olddb

import (
	"context"
	"encoding/base64"

	"github.com/jackc/pgx/v5"
	"github.com/webitel/chat-migration-cli/internal/model/old"
	protomodel "github.com/webitel/chat-migration-cli/internal/model/old/proto"
	"google.golang.org/protobuf/proto"
)

type BotStore struct {
	db *DB
}

func NewBotStore(db *DB) *BotStore {
	return &BotStore{db: db}
}

func (s *BotStore) Get(ctx context.Context, offset int, limit int) ([]*old.Bot, error) {
	var (
		query = `SELECT ARRAY_AGG(id) ids,
       dc,
       STRING_AGG(name, ',') name,
       flow_id,
       MIN(created_at) created_at,
       MAX(updated_at) updated_at
FROM chat.bot
WHERE flow_id IS NOT NULL
GROUP BY
    flow_id, dc`
	)
	if offset < 0 {
		offset = 0
	}
	if limit < 1 {
		limit = 1
	}
	query += ` OFFSET $1 LIMIT $2`

	rows, err := s.db.Pool().Query(ctx, query, offset, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	res, err := pgx.CollectRows(rows, pgx.RowToAddrOfStructByName[old.Bot])
	if err != nil {
		return nil, err
	}

	return res, nil
}

type facebookGatewayMetadata struct {
	FB           string `json:"fb"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`

	IG                     string `json:"ig"`
	InstagramComments      string `json:"instagram_comments"`
	InstagramMentions      string `json:"instagram_mentions"`
	InstagramStoryMentions string `json:"instagram_story_mentions"`

	WA            string `json:"wa"`
	WhatsAppToken string `json:"whatsapp_token"`

	Version string `json:"version"`
}

func (s *BotStore) GetMetaGateways(ctx context.Context, offset int, limit int) ([]*old.Provider[old.FBProviderMetadata], error) {
	var (
		query = `SELECT id, dc, uri, name, flow_id, enabled,
       metadata, created_at, updated_at, updates
FROM chat.bot
WHERE provider = 'messenger'`
	)
	if offset < 0 {
		offset = 0
	}
	if limit < 1 {
		limit = 1
	}
	query += ` OFFSET $1 LIMIT $2`

	rows, err := s.db.Pool().Query(ctx, query, offset, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	internalResult, err := pgx.CollectRows(rows, pgx.RowToAddrOfStructByName[old.Provider[facebookGatewayMetadata]])
	if err != nil {
		return nil, err
	}

	var res []*old.Provider[old.FBProviderMetadata]
	for _, gateway := range internalResult {
		metaGateway := &old.Provider[old.FBProviderMetadata]{
			ID:        gateway.ID,
			DC:        gateway.DC,
			URI:       gateway.URI,
			Name:      gateway.Name,
			FlowID:    gateway.FlowID,
			Enabled:   gateway.Enabled,
			CreatedAt: gateway.CreatedAt,
			UpdatedAt: gateway.UpdatedAt,
			Updates:   gateway.Updates,
		}
		if gateway.Metadata != nil {
			metaGateway.Metadata = &old.FBProviderMetadata{
				ClientID:     gateway.Metadata.ClientID,
				ClientSecret: gateway.Metadata.ClientSecret,
				// InstagramComments:      gateway.Metadata.InstagramComments,
				// InstagramMentions:      gateway.Metadata.InstagramMentions,
				// InstagramStoryMentions: gateway.Metadata.InstagramStoryMentions,
				WA:            gateway.Metadata.WA,
				WhatsAppToken: gateway.Metadata.WhatsAppToken,
				Version:       gateway.Metadata.Version,
			}
			if gateway.Metadata.FB != "" {
				metaGateway.Metadata.FB, err = decodeFB(gateway.Metadata.FB)
				if err != nil {
					return nil, err
				}
			}
			if gateway.Metadata.IG != "" {
				metaGateway.Metadata.IG, err = decodeFB(gateway.Metadata.IG)
				if err != nil {
					return nil, err
				}
			}
		}
		res = append(res, metaGateway)
	}

	return res, nil
}

func decodeFB(fbEncoded string) (*protomodel.Messenger, error) {
	data, err := base64.RawURLEncoding.DecodeString(fbEncoded)
	if err != nil {
		return nil, err
	}
	var msg protomodel.Messenger
	if err := proto.Unmarshal(data, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}
