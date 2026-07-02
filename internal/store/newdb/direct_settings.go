package newdb

import (
	"context"

	"github.com/jackc/pgx/v5"
	modelnew "github.com/webitel/chat-migration-cli/internal/model/new"
)

type DirectSettingsStore struct {
	store *DB
}

func NewDirectSettingsStore(store *DB) *DirectSettingsStore {
	return &DirectSettingsStore{store: store}
}

func (s *DirectSettingsStore) InsertDirectSettings(ctx context.Context, tx pgx.Tx, settings []*modelnew.DirectSettings) error {
	_, err := tx.Exec(
		ctx,
		`INSERT INTO im_thread.direct_settings (thread_dialog_id, domain_id, title) VALUES ($1, $2, $3)`,
		settings[0].ThreadDialogID,
		settings[0].DomainID,
		settings[0].Title,
	)

	return err
}
