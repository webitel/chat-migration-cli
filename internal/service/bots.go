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
			converted, migrations := convertBotToContact(bot)
			contacts = append(contacts, converted)
			migrationRows = append(migrationRows, migrations...)
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

func convertBotToContact(bot *old.Bot) (*modelnew.Contact, []*modelnew.MigrationRow) {
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
	var migrationRows = make([]*modelnew.MigrationRow, 0, len(bot.IDs))
	for _, id := range bot.IDs {
		migrationRows = append(migrationRows, &modelnew.MigrationRow{
			ID:         uuid.New(),
			EntityType: modelnew.EntityTypeBotContact,
			OldID:      strconv.Itoa(id),
			NewID:      res.ID,
			DomainID:   bot.DC,
		})
	}

	return res, migrationRows
}
