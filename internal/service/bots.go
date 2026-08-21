package service

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	modelnew "github.com/webitel/chat-migration-cli/internal/model/new"
	"github.com/webitel/chat-migration-cli/internal/model/old"
)

const BotIssuerID = "schema"

// loadBotMapping returns a nil map, no error, when c.botMappingTable is unset -- the
// mapped-bot lookup downstream then always misses, so the feature is a no-op. Mapped
// new_bot_id values are trusted as-is, never validated against existing contacts. The
// schema.table split is already validated at config load (main.go's mustLoadConfig);
// repeating it here is defensive, not load-bearing.
func (c *Converter) loadBotMapping(ctx context.Context, tx pgx.Tx) (map[string]uuid.UUID, error) {
	if c.botMappingTable == "" {
		return nil, nil
	}
	schema, table, ok := strings.Cut(c.botMappingTable, ".")
	if !ok || schema == "" || table == "" {
		return nil, fmt.Errorf("invalid MIGRATION_BOT_MAPPING_TABLE %q: expected \"schema.table\"", c.botMappingTable)
	}
	rows, err := c.newDB.BotMappingStore().GetAll(ctx, tx, schema, table)
	if err != nil {
		return nil, err
	}
	mapping := make(map[string]uuid.UUID, len(rows))
	for _, r := range rows {
		oldID := strconv.Itoa(r.OldBotID)
		if _, exists := mapping[oldID]; exists {
			c.log.Warn("duplicate old_bot_id in bot mapping table, last row wins",
				"old_bot_id", r.OldBotID, "table", c.botMappingTable)
		}
		mapping[oldID] = r.NewBotID
	}
	return mapping, nil
}

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
	botMapping, err := c.loadBotMapping(ctx, tx)
	if err != nil {
		tx.Rollback(ctx)
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
			oldID := strconv.Itoa(bot.FlowID)
			if newBotID, mapped := botMapping[oldID]; mapped {
				migrationRows = append(migrationRows, &modelnew.MigrationRow{
					ID:         uuid.New(),
					EntityType: modelnew.EntityTypeBotContact,
					OldID:      oldID,
					NewID:      newBotID,
					DomainID:   bot.DC,
				})
				continue
			}
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
		c.addRecordsMigrated(len(contacts))
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
	botMapping, err := c.loadBotMapping(ctx, tx)
	if err != nil {
		tx.Rollback(ctx)
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
			oldID := strconv.Itoa(bot.FlowID)
			if newBotID, mapped := botMapping[oldID]; mapped {
				migrationRows = append(migrationRows, &modelnew.MigrationRow{
					ID:         uuid.New(),
					EntityType: modelnew.EntityTypeBotContact,
					OldID:      oldID,
					NewID:      newBotID,
					DomainID:   bot.DC,
				})
				continue
			}
			converted, migrationRow := convertBotToContact(bot)
			contacts = append(contacts, converted)
			migrationRows = append(migrationRows, migrationRow)
		}
		rowsAffected, err := c.newDB.ContactStore().InsertContactsIgnoreConflicts(ctx, tx, contacts)
		if err != nil {
			return false, err
		}

		if err := c.newDB.MigrationStore().InsertMigrations(ctx, tx, migrationRows); err != nil {
			return false, err
		}
		c.addRecordsMigrated(int(rowsAffected))
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
