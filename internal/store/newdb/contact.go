package newdb

import (
	"context"

	"github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"
	"github.com/webitel/chat-migration-cli/internal/model/new"
)

type ContactStore struct {
	db *DB
}

func NewContactStore(db *DB) *ContactStore {
	return &ContactStore{db: db}
}

func (s *ContactStore) InsertContacts(ctx context.Context, tx pgx.Tx, contacts []*new.Contact) error {
	if len(contacts) == 0 {
		return nil
	}
	var (
		query = squirrel.Insert("im_contact.contact").Columns(
			"id",
			"domain_id",
			"created_at",
			"updated_at",
			"issuer_id",
			"application_id",
			"subject_id",
			"type",
			"name",
			"username",
			"metadata",
			"is_bot",
		).PlaceholderFormat(squirrel.Dollar)
	)

	for _, contact := range contacts {
		query = query.Values(
			contact.ID,
			contact.DomainID,
			contact.CreatedAt,
			contact.UpdatedAt,
			contact.IssuerID,
			contact.ApplicationID,
			contact.SubjectID,
			contact.Type,
			contact.Name,
			contact.Username,
			contact.Metadata,
			contact.IsBot,
		)
	}

	sql, args, err := query.ToSql()
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, sql, args...)
	if err != nil {
		return err
	}

	return nil

}
func (s *ContactStore) InsertContactsIgnoreConflicts(ctx context.Context, tx pgx.Tx, contacts []*new.Contact) error {
	if len(contacts) == 0 {
		return nil
	}
	var (
		query = squirrel.Insert("im_contact.contact").Columns(
			"id",
			"domain_id",
			"created_at",
			"updated_at",
			"issuer_id",
			"application_id",
			"subject_id",
			"type",
			"name",
			"username",
			"metadata",
			"is_bot",
		).PlaceholderFormat(squirrel.Dollar).Suffix("ON CONFLICT DO NOTHING")
	)

	for _, contact := range contacts {
		query = query.Values(
			contact.ID,
			contact.DomainID,
			contact.CreatedAt,
			contact.UpdatedAt,
			contact.IssuerID,
			contact.ApplicationID,
			contact.SubjectID,
			contact.Type,
			contact.Name,
			contact.Username,
			contact.Metadata,
			contact.IsBot,
		)
	}

	sql, args, err := query.ToSql()
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, sql, args...)
	if err != nil {
		return err
	}

	return nil

}

func (s *ContactStore) SyncContactVias(ctx context.Context, tx pgx.Tx) error {
	query := `WITH chain AS (SELECT ct.new_id contact_id, gt.new_id gate_id
               FROM public.chat_migration ct
                        LEFT JOIN public.chat_migration gt
                                  ON gt.old_id = ct.old_id AND gt.entity_type = 'provider_to_gateway' AND
                                     ct.domain_id = gt.domain_id
               WHERE ct.entity_type = 'gateway_to_contact')

INSERT
INTO im_contact.via(contact_id, via)
SELECT contact_id, gate_id
FROM chain
ON CONFLICT (contact_id, via) DO NOTHING;
`
	_, err := tx.Exec(ctx, query)
	if err != nil {
		return err
	}
	return nil
}

func (s *ContactStore) GetByWebitelUserIDs(ctx context.Context, tx pgx.Tx, webitelUserIDs []string) ([]*new.Contact, error) {
	if len(webitelUserIDs) == 0 {
		return nil, nil
	}
	var (
		query = `
		SELECT id, domain_id, created_at, updated_at, issuer_id, application_id, subject_id, type, name, username, metadata, is_bot
		FROM im_contact.contact
		WHERE subject_id = ANY($1::text[]) AND issuer_id = 'webitel' AND is_bot = false`
	)
	rows, err := tx.Query(ctx, query, webitelUserIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result, err := pgx.CollectRows(rows, pgx.RowToAddrOfStructByName[new.Contact])
	if err != nil {
		return nil, err
	}
	return result, nil
}
func (s *ContactStore) GetByFlowIDs(ctx context.Context, tx pgx.Tx, flowIDs []string) ([]*new.Contact, error) {
	if len(flowIDs) == 0 {
		return nil, nil
	}
	var (
		query = `
		SELECT id, domain_id, created_at, updated_at, issuer_id, application_id, subject_id, type, name, username, metadata, is_bot
		FROM im_contact.contact
		WHERE subject_id = ANY($1::text[]) AND issuer_id = 'schema' AND is_bot = false`
	)
	rows, err := tx.Query(ctx, query, flowIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result, err := pgx.CollectRows(rows, pgx.RowToAddrOfStructByName[new.Contact])
	if err != nil {
		return nil, err
	}
	return result, nil
}
