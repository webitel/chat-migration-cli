package service

import (
	"context"
	"strconv"

	"github.com/google/uuid"
	modelnew "github.com/webitel/chat-migration-cli/internal/model/new"
	"github.com/webitel/chat-migration-cli/internal/model/old"
)

func (c *Converter) MigrateClientsToContacts(ctx context.Context) error {
	var (
		perPage = 1000
	)
	tx, err := c.newDB.Pool().Begin(ctx)
	if err != nil {
		return err
	}
	err = PagerFunc(ctx, perPage, func(ctx context.Context, offset, limit int) (bool, error) {
		iterate := true
		clients, err := c.oldDB.ClientStore().Get(ctx, offset, limit)
		if err != nil {
			return false, err
		}
		if len(clients) < limit {
			iterate = false
		}
		var (
			contacts      []*modelnew.Contact
			migrationRows []*modelnew.MigrationRow
		)
		for _, client := range clients {
			converted := convertClientToContact(client)
			contacts = append(contacts, converted...)
			for _, contact := range converted {
				migrationRows = append(migrationRows, &modelnew.MigrationRow{
					ID:         uuid.New(),
					EntityType: modelnew.EntityTypeClientContact,
					OldID:      strconv.Itoa(int(client.ID)),
					NewID:      contact.ID,
					DomainID:   contact.DomainID,
				})
			}
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

func convertClientToContact(client *old.Client) []*modelnew.Contact {
	var contacts []*modelnew.Contact
	for _, domain := range client.DomainIDs {
		contacts = append(contacts, &modelnew.Contact{
			BaseModel: modelnew.BaseModel{
				ID:        uuid.New(),
				DomainID:  domain,
				CreatedAt: client.CreatedAt,
				UpdatedAt: client.CreatedAt,
			},
			IssuerID:  client.Type,
			SubjectID: client.ExternalID,
			Type:      client.Type,
			Name:      client.Name,
			IsBot:     false,
		})
	}
	return contacts
}
