package olddb

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/webitel/chat-migration-cli/internal/model/old"
)

type ClientStore struct {
	db *DB
}

func NewClientStore(db *DB) *ClientStore {
	return &ClientStore{db: db}
}

func (s *ClientStore) Get(ctx context.Context, offset int, limit int) ([]*old.Client, error) {
	var (
		query = `SELECT id, name, number, created_at, external_id, first_name, last_name, COALESCE(type, 'webchat') type, domains.domains domain_ids FROM chat.client c LEFT JOIN LATERAL (
    SELECT ARRAY_AGG(DISTINCT domain_id) domains FROM chat.channel ch where ch.user_id = c.id ) domains ON true`
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

	res, err := pgx.CollectRows(rows, pgx.RowToAddrOfStructByName[old.Client])
	if err != nil {
		return nil, err
	}

	return res, nil
}
