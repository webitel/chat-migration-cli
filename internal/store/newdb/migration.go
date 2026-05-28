package newdb

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"
	modelnew "github.com/webitel/chat-migration-cli/internal/model/new"
)

type MigrationStore struct {
	store *DB
}

func NewMigrationStore(store *DB) *MigrationStore {
	return &MigrationStore{store: store}
}

func (s *MigrationStore) GetMigrationRow(ctx context.Context, tx pgx.Tx, filters *modelnew.MigrationRowFilters) (*modelnew.MigrationRow, error) {
	if tx == nil {
		return nil, errors.New("transaction required")
	}
	var (
		query = squirrel.StatementBuilder.
			PlaceholderFormat(squirrel.Dollar).
			Select("*").
			From("public.chat_migration")
	)
	query = s.applyFilters(query, filters)
	sql, args, err := query.ToSql()
	if err != nil {
		return nil, err
	}

	rows, err := tx.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}

	result, err := pgx.CollectExactlyOneRow(rows, pgx.RowToAddrOfStructByName[modelnew.MigrationRow])
	if err != nil {
		return nil, err
	}

	return result, nil
}

func (s *MigrationStore) applyFilters(query squirrel.SelectBuilder, filters *modelnew.MigrationRowFilters) squirrel.SelectBuilder {
	if filters == nil {
		return query
	}

	if len(filters.Type) != 0 {
		query = query.Where("entity_type = ANY(?)", filters.Type)
	}
	if len(filters.OldIDs) != 0 {
		query = query.Where("old_id = ANY(?)", filters.OldIDs)
	}
	if len(filters.ExtraKeys) != 0 {
		query = query.Where("extra_key = ANY(?)", filters.ExtraKeys)
	}
	if filters.DomainID != 0 {
		query = query.Where("domain_id = ?", filters.DomainID)
	}

	return query
}

func (s *MigrationStore) GetMigrationRows(ctx context.Context, tx pgx.Tx, filters *modelnew.MigrationRowFilters) ([]*modelnew.MigrationRow, error) {
	if tx == nil {
		return nil, errors.New("transaction required")
	}
	var (
		query = squirrel.StatementBuilder.
			PlaceholderFormat(squirrel.Dollar).
			Select("*").
			From("public.chat_migration")
	)

	query = s.applyFilters(query, filters)

	sql, args, err := query.ToSql()
	if err != nil {
		return nil, err
	}

	rows, err := tx.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result, err := pgx.CollectRows(rows, pgx.RowToAddrOfStructByName[modelnew.MigrationRow])
	if err != nil {
		return nil, err
	}

	return result, nil
}

func (s *MigrationStore) InsertMigrations(ctx context.Context, tx pgx.Tx, migrations []*modelnew.MigrationRow) error {
	if len(migrations) == 0 {
		return nil
	}

	// 65535 parameters max for a single query
	// for each row there is 6 params
	// 8000 rows * 6 parameters per row = 48000
	const chunkSize = 8000

	for i := 0; i < len(migrations); i += chunkSize {
		end := i + chunkSize
		if end > len(migrations) {
			end = len(migrations)
		}

		chunk := migrations[i:end]

		var (
			query = squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar).Insert("public.chat_migration").Columns(
				"id",
				"entity_type",
				"old_id",
				"new_id",
				"domain_id",
				"extra_key",
			)
		)
		for _, migration := range chunk {
			query = query.Values(
				migration.ID,
				migration.EntityType,
				migration.OldID,
				migration.NewID,
				migration.DomainID,
				migration.ExtraKey,
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
	}

	return nil
}

func (s *MigrationStore) GetCompletedSteps(ctx context.Context) (map[string]struct{}, error) {
	rows, err := s.store.Pool().Query(ctx, `SELECT step FROM public.chat_migration_step WHERE status = 'completed'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	completed := make(map[string]struct{})
	for rows.Next() {
		var step string
		if err := rows.Scan(&step); err != nil {
			return nil, err
		}
		completed[step] = struct{}{}
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return completed, nil
}

func (s *MigrationStore) MarkStepCompleted(ctx context.Context, step string) error {
	_, err := s.store.Pool().Exec(ctx, `
		INSERT INTO public.chat_migration_step (id, step, status, page_offset)
		VALUES (gen_random_uuid(), $1, 'completed', 0)
		ON CONFLICT (step) DO UPDATE SET status = 'completed', page_offset = 0, error = NULL
	`, step)
	return err
}

// GetStepProgress returns the last successfully committed page offset for a step.
// Returns 0 if the step has no recorded progress (first run).
func (s *MigrationStore) GetStepProgress(ctx context.Context, step string) (int, error) {
	var offset int
	err := s.store.Pool().QueryRow(ctx, `
		SELECT page_offset FROM public.chat_migration_step
		WHERE step = $1 AND status != 'completed'
	`, step).Scan(&offset)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	return offset, err
}

// SaveStepProgress records that the page at the given offset completed successfully.
// offset should be the next offset to process (current offset + page size).
func (s *MigrationStore) SaveStepProgress(ctx context.Context, step string, offset int) error {
	_, err := s.store.Pool().Exec(ctx, `
		INSERT INTO public.chat_migration_step (id, step, status, page_offset)
		VALUES (gen_random_uuid(), $1, 'in_progress', $2)
		ON CONFLICT (step) DO UPDATE SET status = 'in_progress', page_offset = EXCLUDED.page_offset, error = NULL
	`, step, offset)
	return err
}

// GetCursorProgress returns the last successfully committed keyset cursor for a step.
// Returns (0, 0, nil) if the step has no recorded cursor progress (first run).
func (s *MigrationStore) GetCursorProgress(ctx context.Context, step string) (initiator int, flowID int, err error) {
	var cursor *string
	err = s.store.Pool().QueryRow(ctx, `
		SELECT page_cursor FROM public.chat_migration_step
		WHERE step = $1 AND status != 'completed'
	`, step).Scan(&cursor)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, 0, nil
	}
	if err != nil || cursor == nil {
		return 0, 0, err
	}
	parts := strings.SplitN(*cursor, ":", 2)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid cursor %q", *cursor)
	}
	initiator, err = strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, fmt.Errorf("invalid cursor initiator: %w", err)
	}
	flowID, err = strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, fmt.Errorf("invalid cursor flowID: %w", err)
	}
	return initiator, flowID, nil
}

// SaveCursorProgress records the last successfully committed keyset cursor for a step.
func (s *MigrationStore) SaveCursorProgress(ctx context.Context, step string, initiator int, flowID int) error {
	cursor := fmt.Sprintf("%d:%d", initiator, flowID)
	_, err := s.store.Pool().Exec(ctx, `
		INSERT INTO public.chat_migration_step (id, step, status, page_cursor)
		VALUES (gen_random_uuid(), $1, 'in_progress', $2)
		ON CONFLICT (step) DO UPDATE SET status = 'in_progress', page_cursor = EXCLUDED.page_cursor, error = NULL
	`, step, cursor)
	return err
}

// MarkStepFailed records the offset and error message at the point of failure.
func (s *MigrationStore) MarkStepFailed(ctx context.Context, step string, offset int, errMsg string) error {
	_, err := s.store.Pool().Exec(ctx, `
		INSERT INTO public.chat_migration_step (id, step, status, page_offset, error)
		VALUES (gen_random_uuid(), $1, 'failed', $2, $3)
		ON CONFLICT (step) DO UPDATE SET status = 'failed', page_offset = EXCLUDED.page_offset, error = EXCLUDED.error
	`, step, offset, errMsg)
	return err
}
