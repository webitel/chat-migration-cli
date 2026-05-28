package service

import (
	"context"

	modelnew "github.com/webitel/chat-migration-cli/internal/model/new"
	modelold "github.com/webitel/chat-migration-cli/internal/model/old"
)

func (c *Converter) MigrateFacebookProviders(ctx context.Context) error {
	var (
		perPage = 1000
	)
	tx, err := c.newDB.Pool().Begin(ctx)
	if err != nil {
		return err
	}
	err = PagerFunc(ctx, perPage, func(ctx context.Context, offset, limit int) (bool, error) {
		iterate := true
		providers, err := c.oldDB.BotStore().GetMetaGateways(ctx, offset, limit)
		if err != nil {
			return false, err
		}
		if len(providers) < limit {
			iterate = false
		}
		var (
			contacts      []*modelnew.Contact
			migrationRows []*modelnew.MigrationRow
		)
		// for _, provider := range providers {
		// 	// converted, migrations := convertBotToContact(provider)
		// 	// contacts = append(contacts, converted)
		// 	// migrationRows = append(migrationRows, migrations...)
		// }
		if err := c.newDB.ContactStore().InsertContacts(ctx, tx, contacts); err != nil {
			return false, err
		}
		if err := c.newDB.MigrationStore().InsertMigrations(ctx, tx, migrationRows); err != nil {
			return false, err
		}
		return iterate, nil
	})
	if err != nil {
		tx.Rollback(ctx)
		return err
	}
	return tx.Commit(ctx)
}

func (c *Converter) insertGateways(providers []*modelold.Gateway) error {
	// for _, provider := range providers {

	// 	// gate := &modelnew.Gate{
	// 	// 	ID:        strconv.Itoa(provider.ID),
	// 	// 	DC:        int64(provider.DC),
	// 	// 	Name:      provider.Name,
	// 	// 	Type:      "meta",
	// 	// 	Enabled:   provider.Enabled,
	// 	// 	CreatedAt: provider.CreatedAt,
	// 	// 	UpdatedAt: provider.UpdatedAt,
	// 	// }
	// }
	return nil
}
