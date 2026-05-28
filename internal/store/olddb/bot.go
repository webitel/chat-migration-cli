package olddb

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/webitel/chat-migration-cli/internal/model/old"
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

func (s *BotStore) GetMetaGateways(ctx context.Context, offset int, limit int) ([]*old.Gateway, error) {
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

	res, err := pgx.CollectRows(rows, pgx.RowToAddrOfStructByName[old.Gateway])
	if err != nil {
		return nil, err
	}

	return res, nil
}
