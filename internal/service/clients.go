package service

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"
	modelnew "github.com/webitel/chat-migration-cli/internal/model/new"
	"github.com/webitel/chat-migration-cli/internal/model/old"
)

func (c *Converter) MigrateClientsToContacts(ctx context.Context) error {
	var (
		perPage = 1000
	)
	c.log.Debug("starting clients-to-contacts migration")
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
		c.log.Debug("clients page fetched", "offset", offset, "count", len(clients))
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
				for _, gateway := range client.Gateways {
					migrationRows = append(migrationRows, &modelnew.MigrationRow{
						ID:         uuid.New(),
						EntityType: modelnew.EntityTypeGatewayToContact,
						OldID:      strconv.Itoa(int(gateway)),
						NewID:      contact.ID,
						DomainID:   contact.DomainID,
					})
				}

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

func (c *Converter) MigrateClientsToContactsSyncMode(ctx context.Context) error {
	var (
		perPage = 1000
	)
	c.log.Debug("starting clients-to-contacts migration")
	tx, err := c.newDB.Pool().Begin(ctx)
	if err != nil {
		return err
	}

	completedAt, err := c.GetStepCompletedAtInTx(ctx, tx, SyncStepClientsToContacts)
	if err != nil {
		return err
	}

	err = PagerFunc(ctx, perPage, func(ctx context.Context, offset, limit int) (bool, error) {
		iterate := true
		clients, err := c.oldDB.ClientStore().GetFromDate(ctx, offset, limit, &completedAt)
		if err != nil {
			return false, err
		}
		if len(clients) < limit {
			iterate = false
		}
		c.log.Debug("clients page fetched", "offset", offset, "count", len(clients))
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
				for _, gateway := range client.Gateways {
					migrationRows = append(migrationRows, &modelnew.MigrationRow{
						ID:         uuid.New(),
						EntityType: modelnew.EntityTypeGatewayToContact,
						OldID:      strconv.Itoa(int(gateway)),
						NewID:      contact.ID,
						DomainID:   contact.DomainID,
					})
				}

			}
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
			Username:  buildUsernameForClient(client),
			IsBot:     false,
		})
	}
	return contacts
}

func buildUsernameForClient(cli *old.Client) string {
	return buildUsername(cli.Name, cli.Type, cli.ExternalID)
}

func buildUsername(name, userType, userID string) string {
	replacedName := replaceCharactersForUsername(name)
	replacedType := replaceCharactersForUsername(userType)
	replacedID := replaceCharactersForUsername(userID)
	return fmt.Sprintf("%s_%s_%s", replacedName, replacedType, replacedID)
}

func replaceCharactersForUsername(in string) string {
	lowered := strings.ToLower(in)
	replacedBlank := strings.ReplaceAll(lowered, " ", "_")
	replacedDash := strings.ReplaceAll(replacedBlank, "-", "_")
	return replacedDash
}
