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
	const perPage = 1000
	c.log.Debug("starting bots-to-contacts migration")

	startOffset, err := c.newDB.MigrationStore().GetStepProgress(ctx, StepBotsToContacts)
	if err != nil {
		return err
	}
	if startOffset > 0 {
		c.log.Info("resuming bots-to-contacts migration", "startOffset", startOffset)
	}

	lastCommittedOffset := startOffset
	fail := func(cause error) error {
		_ = c.newDB.MigrationStore().MarkStepFailed(ctx, StepBotsToContacts, lastCommittedOffset, cause.Error())
		return cause
	}

	err = PagerFunc(ctx, perPage, func(ctx context.Context, offset, limit int) (bool, error) {
		absOffset := offset + startOffset

		tx, err := c.newDB.Pool().Begin(ctx)
		if err != nil {
			return false, err
		}

		iterate := true
		bots, err := c.oldDB.BotStore().Get(ctx, absOffset, limit)
		if err != nil {
			tx.Rollback(ctx)
			return false, err
		}
		if len(bots) < limit {
			iterate = false
		}
		c.log.Debug("bots page fetched", "offset", absOffset, "count", len(bots))
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
			tx.Rollback(ctx)
			return false, err
		}
		if err := c.newDB.MigrationStore().InsertMigrations(ctx, tx, migrationRows); err != nil {
			tx.Rollback(ctx)
			return false, err
		}

		if err := c.newDB.MigrationStore().SaveStepProgressInTx(ctx, tx, StepBotsToContacts, absOffset+limit); err != nil {
			tx.Rollback(ctx)
			return false, err
		}

		if err := tx.Commit(ctx); err != nil {
			return false, err
		}
		lastCommittedOffset = absOffset + limit

		return iterate, nil
	})
	if err != nil {
		return fail(err)
	}
	return nil
}

func (c *Converter) MigrateBotsToContactsSyncMode(ctx context.Context) error {
	const perPage = 1000
	c.log.Debug("starting bots-to-contacts migration in sync mode")

	startOffset, err := c.newDB.MigrationStore().GetStepProgress(ctx, SyncStepBotsToContacts)
	if err != nil {
		return err
	}
	if startOffset > 0 {
		c.log.Info("resuming bots-to-contacts migration", "startOffset", startOffset)
	}

	completedAt, err := c.GetStepCompletedAt(ctx, SyncStepBotsToContacts)
	if err != nil {
		return err
	}

	lastCommittedOffset := startOffset
	fail := func(cause error) error {
		_ = c.newDB.MigrationStore().MarkStepFailed(ctx, SyncStepBotsToContacts, lastCommittedOffset, cause.Error())
		return cause
	}

	err = PagerFunc(ctx, perPage, func(ctx context.Context, offset, limit int) (bool, error) {
		absOffset := offset + startOffset

		tx, err := c.newDB.Pool().Begin(ctx)
		if err != nil {
			return false, err
		}

		iterate := true
		bots, err := c.oldDB.BotStore().GetFromDate(ctx, absOffset, limit, &completedAt)
		if err != nil {
			tx.Rollback(ctx)
			return false, err
		}
		if len(bots) < limit {
			iterate = false
		}
		c.log.Debug("bots page fetched", "offset", absOffset, "count", len(bots))
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
			tx.Rollback(ctx)
			return false, err
		}

		if err := c.newDB.MigrationStore().InsertMigrations(ctx, tx, migrationRows); err != nil {
			tx.Rollback(ctx)
			return false, err
		}

		if err := c.newDB.MigrationStore().SaveStepProgressInTx(ctx, tx, SyncStepBotsToContacts, absOffset+limit); err != nil {
			tx.Rollback(ctx)
			return false, err
		}

		if err := tx.Commit(ctx); err != nil {
			return false, err
		}
		lastCommittedOffset = absOffset + limit

		return iterate, nil
	})
	if err != nil {
		return fail(err)
	}
	return nil
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
