package newdb

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	modelnew "github.com/webitel/chat-migration-cli/internal/model/new"
)

type BotMappingStore struct {
	db *DB
}

func NewBotMappingStore(db *DB) *BotMappingStore {
	return &BotMappingStore{db: db}
}

// GetAll assumes schema/table are already split and validated by the caller. The
// identifier is quoted via pgx.Identifier.Sanitize() -- never naive concatenation --
// since schema/table ultimately come from client-supplied config.
func (s *BotMappingStore) GetAll(ctx context.Context, schema, table string) ([]*modelnew.BotMapping, error) {
	ident := pgx.Identifier{schema, table}.Sanitize()
	query := fmt.Sprintf("SELECT old_bot_id, new_bot_id FROM %s", ident)
	rows, err := s.db.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to read bot mapping table %s.%s: %w", schema, table, err)
	}
	defer rows.Close()

	result, err := pgx.CollectRows(rows, pgx.RowToAddrOfStructByName[modelnew.BotMapping])
	if err != nil {
		return nil, fmt.Errorf("failed to scan bot mapping table %s.%s (check for NULL old_bot_id/new_bot_id): %w", schema, table, err)
	}
	return result, nil
}
