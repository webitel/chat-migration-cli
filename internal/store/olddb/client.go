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
AND ($3::timestamp IS NULL OR c.created_at >= $3::timestamp)`
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

func (s *ClientStore) GetPortalClients(ctx context.Context, offset int, limit int) ([]*old.PortalClient, error) {
	var (
		query = `SELECT c.id,
					merged_identity."name" AS name,
					merged_identity.phone_number AS number,
					acc.created_at AS created_at,
					acc.updated_at AS updated_at,
					acc.profile_id AS profile_id,
					merged_identity.given_name AS first_name,
					merged_identity.family_name AS last_name,
					'portal' AS type,
					acc.dc AS dc,
					credentials_identity.sub AS sub,
					credentials_identity.iss AS iss
				FROM chat.client c
				INNER JOIN portal.user_account acc ON acc.id = c.external_id::uuid
				JOIN LATERAL (SELECT *
            FROM portal.identity i
            WHERE i.top = acc.profile_id
            ORDER BY i.updated_at DESC
            LIMIT 1) credentials_identity ON TRUE
				JOIN LATERAL (SELECT *
            FROM portal.identity i
            WHERE i.id = acc.profile_id
            ORDER BY i.updated_at DESC
            LIMIT 1) merged_identity ON TRUE
				WHERE c.type = 'portal'
				ORDER BY c.id`
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

	res, err := pgx.CollectRows(rows, pgx.RowToAddrOfStructByName[old.PortalClient])
	if err != nil {
		return nil, err
	}

	return res, nil
}

func (s *ClientStore) GetPortalClientsFromDate(ctx context.Context, offset int, limit int, from *time.Time) ([]*old.PortalClient, error) {
	var (
		query = `SELECT c.id,
			i."name" AS name,
			i.phone_number AS number,
			acc.created_at AS created_at,
			acc.updated_at AS updated_at,
			acc.profile_id AS profile_id,
			i.given_name AS first_name,
			i.family_name AS last_name,
			'portal' AS type,
			acc.dc AS dc,
			i.sub AS sub,
			i.iss AS iss
		FROM chat.client c
		INNER JOIN portal.user_account acc ON acc.id = c.external_id::uuid
		INNER JOIN portal.identity i ON i.top = acc.profile_id
		WHERE c.type = 'portal' AND c.external_id ~ '^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$'
		AND ($3::timestamp IS NULL OR c.created_at >= $3::timestamp)
		ORDER BY c.id`
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

	res, err := pgx.CollectRows(rows, pgx.RowToAddrOfStructByName[old.PortalClient])
	if err != nil {
		return nil, err
	}

	return res, nil
}
