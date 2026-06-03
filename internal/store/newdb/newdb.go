package newdb

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgxpool"
)

// DB is a connection to the new micro-services database.
type DB struct {
	pool *pgxpool.Pool

	contactStore      *ContactStore
	threadStore       *ThreadStore
	threadDialogStore *ThreadDialogStore
	migrationStore    *MigrationStore
	messageStore      *MessageStore
	providerStore     *ProviderStore
}

func New(pool *pgxpool.Pool) (*DB, error) {
	db := &DB{pool: pool}
	if err := db.initializeMigrationTable(context.Background()); err != nil {
		return nil, errors.Join(errors.New("failed to init migration table"), err)
	}
	return db, nil
}

func (db *DB) Pool() *pgxpool.Pool { return db.pool }

func (db *DB) Close() { db.pool.Close() }

func (db *DB) ContactStore() *ContactStore {
	if db.contactStore == nil {
		db.contactStore = NewContactStore(db)
	}
	return db.contactStore
}

func (db *DB) ThreadStore() *ThreadStore {
	if db.threadStore == nil {
		db.threadStore = NewThreadStore(db)
	}
	return db.threadStore
}

func (db *DB) MigrationStore() *MigrationStore {
	if db.migrationStore == nil {
		db.migrationStore = NewMigrationStore(db)
	}
	return db.migrationStore
}

func (db *DB) MessageStore() *MessageStore {
	if db.messageStore == nil {
		db.messageStore = NewMessageStore(db)
	}
	return db.messageStore
}

func (db *DB) initializeMigrationTable(ctx context.Context) error {
	_, err := db.pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS public.chat_migration(
	id UUID PRIMARY KEY NOT NULL DEFAULT gen_random_uuid(),
	entity_type TEXT NOT NULL,
	old_id TEXT NOT NULL,
	new_id UUID NOT NULL,
	domain_id INT,
	extra_key TEXT
);

	CREATE UNIQUE INDEX IF NOT EXISTS chat_migration_entity_old_id_uindex ON public.chat_migration (entity_type, old_id, domain_id, new_id, extra_key);
	CREATE INDEX IF NOT EXISTS chat_migration_new_id_index ON public.chat_migration (new_id);

	CREATE TABLE IF NOT EXISTS public.chat_migration_step(
	id UUID PRIMARY KEY NOT NULL DEFAULT gen_random_uuid(),
	step TEXT NOT NULL,
	status TEXT NOT NULL
);

	CREATE UNIQUE INDEX IF NOT EXISTS chat_migration_step_step_uindex ON public.chat_migration_step (step);

	ALTER TABLE public.chat_migration_step ADD COLUMN IF NOT EXISTS page_offset INT NOT NULL DEFAULT 0;
	ALTER TABLE public.chat_migration_step ADD COLUMN IF NOT EXISTS page_cursor TEXT;
	ALTER TABLE public.chat_migration_step ADD COLUMN IF NOT EXISTS error TEXT;
	`)
	return err
}

func (db *DB) ThreadDialogStore() *ThreadDialogStore {
	if db.threadDialogStore == nil {
		db.threadDialogStore = &ThreadDialogStore{store: db}
	}
	return db.threadDialogStore
}

func (db *DB) ProviderStore() *ProviderStore {
	if db.providerStore == nil {
		db.providerStore = NewProviderStore(db)
	}
	return db.providerStore
}
