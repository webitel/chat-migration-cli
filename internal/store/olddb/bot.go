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
		query = `SELECT id, dc, uri, name, flow_id, enabled, provider, metadata, created_at, created_by, updated_at, updated_by, updates, storage_id FROM chat.bot`
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
