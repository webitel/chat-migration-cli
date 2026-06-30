package service

import (
	"context"
	"strconv"
	"strings"

	"github.com/google/uuid"
	modelnew "github.com/webitel/chat-migration-cli/internal/model/new"
	"github.com/webitel/chat-migration-cli/internal/model/old"
)

const BotIssuerID = "schema"

// NOTE: MigrationRow old_id = flow_id, new_id = contact_id
func (c *Converter) MigrateBotsToContacts(ctx context.Context) error {
	var (
		perPage = 1000
	)
	c.log.Debug("starting bots-to-contacts migration")
	tx, err := c.newDB.Pool().Begin(ctx)
	if err != nil {
		return err

	}
	err = PagerFunc(ctx, perPage, func(ctx context.Context, offset, limit int) (bool, error) {
		iterate := true
		bots, err := c.oldDB.BotStore().Get(ctx, offset, limit)
		if err != nil {
			return false, err
		}
		if len(bots) < limit {
			iterate = false
		}
		c.log.Debug("bots page fetched", "offset", offset, "count", len(bots))
		var (
			contacts      []*modelnew.Contact
			migrationRows []*modelnew.MigrationRow
		)
		for _, bot := range bots {
			converted, migrationRow := convertBotToContact(bot)
			contacts = append(contacts, converted)
			migrationRows = append(migrationRows, migrationRow)
		}
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

func (c *Converter) MigrateBotsToContactsSyncMode(ctx context.Context) error {
	var (
		perPage = 1000
	)
	c.log.Debug("starting bots-to-contacts migration in sync mode")
	tx, err := c.newDB.Pool().Begin(ctx)
	if err != nil {
		return err

	}
	completedAt, err := c.GetStepCompletedAtInTx(ctx, tx, SyncStepBotsToContacts)
	if err != nil {
		return err
	}

	err = PagerFunc(ctx, perPage, func(ctx context.Context, offset, limit int) (bool, error) {
		iterate := true
		bots, err := c.oldDB.BotStore().GetFromDate(ctx, offset, limit, &completedAt)
		if err != nil {
			return false, err
		}
		if len(bots) < limit {
			iterate = false
		}
		c.log.Debug("bots page fetched", "offset", offset, "count", len(bots))
		var (
			contacts      []*modelnew.Contact
			migrationRows []*modelnew.MigrationRow
		)
		for _, bot := range bots {
			converted, migrationRow := convertBotToContact(bot)
			contacts = append(contacts, converted)
			migrationRows = append(migrationRows, migrationRow)
		}
		if err := c.newDB.ContactStore().InsertContactsIgnoreConflicts(ctx, tx, contacts); err != nil {
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

func convertBotToContact(bot *old.Bot) (*modelnew.Contact, *modelnew.MigrationRow) {
	res := &modelnew.Contact{
		BaseModel: modelnew.BaseModel{
			ID:        uuid.New(),
			DomainID:  bot.DC,
			CreatedAt: bot.CreatedAt,
			UpdatedAt: bot.UpdatedAt,
		},
		IssuerID:  BotIssuerID,
		SubjectID: strconv.Itoa(bot.FlowID),
		Type:      "bot",
		Name:      bot.Name,
		Username:  strings.ToLower(strings.Replace(bot.Name, " ", "_", -1)),
		IsBot:     true,
	}
	migrationRow := &modelnew.MigrationRow{
		ID:         uuid.New(),
		EntityType: modelnew.EntityTypeBotContact,
		OldID:      strconv.Itoa(bot.FlowID),
		NewID:      res.ID,
		DomainID:   bot.DC,
	}

	return res, migrationRow
}
