package olddb

import (
	"context"
	"time"

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
		query = `SELECT
    id,
       name,
       number,
       created_at,
       external_id,
       first_name,
       last_name,
       COALESCE(type, 'webchat') type,
       channels.domains           domain_ids,
       channels.gateways              gateways
FROM chat.client c
         LEFT JOIN LATERAL (
    SELECT ARRAY_AGG(DISTINCT ch.domain_id) domains, ARRAY_AGG(DISTINCT ch.connection::bigint) gateways
    FROM chat.channel ch
    WHERE ch.user_id = c.id AND NOT ch.internal AND ch.connection IS NOT NULL
             ) channels ON true
WHERE channels.domains IS NOT NULL
AND type != 'portal'`
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

func (s *ClientStore) GetFromDate(ctx context.Context, offset int, limit int, from *time.Time) ([]*old.Client, error) {
	var (
		query = `SELECT
    id,
       name,
       number,
       created_at,
       external_id,
       first_name,
       last_name,
       COALESCE(type, 'webchat') type,
       channels.domains           domain_ids,
       channels.gateways              gateways
FROM chat.client c
         LEFT JOIN LATERAL (
    SELECT ARRAY_AGG(DISTINCT ch.domain_id) domains, ARRAY_AGG(DISTINCT ch.connection::bigint) gateways
    FROM chat.channel ch
    WHERE ch.user_id = c.id AND NOT ch.internal AND ch.connection IS NOT NULL
             ) channels ON true
WHERE channels.domains IS NOT NULL
AND type != 'portal'
AND ($3 IS NULL OR c.created_at >= $3)`
	)
	if offset < 0 {
		offset = 0
	}
	if limit < 1 {
		limit = 1
	}
	query += ` OFFSET $1 LIMIT $2`
	rows, err := s.db.Pool().Query(ctx, query, offset, limit, from)
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
