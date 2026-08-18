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

	rowsAffected, err := c.newDB.ContactStore().SyncContactVias(ctx, tx)
	if err != nil {
		return err
	}
	c.addRecordsMigrated(int(rowsAffected))

	return tx.Commit(ctx)
}
