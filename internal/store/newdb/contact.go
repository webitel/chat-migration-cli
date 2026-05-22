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
	var (
		query = squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar).Insert("im_contact.contact").Columns(
			"id",
			"domain_id",
			"created_at",
			"updated_at",
			"created_by",
			"updated_by",
			"issuer_id",
			"application_id",
			"subject_id",
			"type",
			"name",
			"username",
			"metadata",
			"is_bot",
		)
	)

	for _, contact := range contacts {
		query = query.Values(
			contact.ID,
			contact.DomainID,
			contact.CreatedAt,
			contact.UpdatedAt,
			contact.CreatedBy,
			contact.UpdatedBy,
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

func (s *ContactStore) GetWebitelUsersByWebitelUserIDs(ctx context.Context, tx pgx.Tx, webitelUserIDs []int) ([]*new.Contact, error) {
	if len(webitelUserIDs) == 0 {
		return nil, nil
	}
	var (
		query = `
		SELECT id, domain_id, created_at, updated_at, issuer_id, application_id, subject_id, type, name, username, metadata, is_bot
		FROM im_contact.contact
		WHERE subject_id = ANY($1) AND issuer_id = 'webitel' AND is_bot = false`
	)
	rows, err := s.db.Pool().Query(ctx, query, webitelUserIDs)
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
