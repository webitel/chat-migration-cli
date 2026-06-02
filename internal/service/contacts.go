package service

import (
	"context"

	"github.com/jackc/pgx/v5"
)

func (c *Converter) SyncContactsVias(ctx context.Context) error {
	tx, err := c.newDB.Pool().BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if err := c.newDB.ContactStore().SyncContactVias(ctx, tx); err != nil {
		return err
	}

	return tx.Commit(ctx)
}
