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

// Get returns the next page of clients ordered by id, keyset-paginated after afterID
// (i.e. c.id > afterID) rather than OFFSET-paginated, to avoid O(N) skip cost on large tables.
func (s *ClientStore) Get(ctx context.Context, afterID int, limit int) ([]*old.Client, error) {
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
AND c.id > $1
ORDER BY c.id LIMIT $2`
	)
	if afterID < 0 {
		afterID = 0
	}
	if limit < 1 {
		limit = 1
	}
	rows, err := s.db.Pool().Query(ctx, query, afterID, limit)
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

// GetFromDate is Get filtered to clients created at or after from, for sync mode.
func (s *ClientStore) GetFromDate(ctx context.Context, afterID int, limit int, from *time.Time) ([]*old.Client, error) {
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
AND ($3::timestamp IS NULL OR c.created_at >= $3::timestamp)
AND c.id > $1`
	)
	if afterID < 0 {
		afterID = 0
	}
	if limit < 1 {
		limit = 1
	}
	query += ` ORDER BY c.id LIMIT $2`
	rows, err := s.db.Pool().Query(ctx, query, afterID, limit, from)
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
