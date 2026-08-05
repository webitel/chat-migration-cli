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
	const perPage = 1000
	c.log.Debug("starting clients-to-contacts migration")

	lastID, err := c.newDB.MigrationStore().GetIDCursorProgress(ctx, StepClientsToContacts)
	if err != nil {
		return err
	}
	if lastID > 0 {
		c.log.Info("resuming clients-to-contacts migration", "lastID", lastID)
	}

	fail := func(cause error) error {
		_ = c.newDB.MigrationStore().MarkStepFailed(ctx, StepClientsToContacts, 0, cause.Error())
		return cause
	}

	for {
		tx, err := c.newDB.Pool().Begin(ctx)
		if err != nil {
			return fail(err)
		}

		clients, err := c.oldDB.ClientStore().Get(ctx, lastID, perPage)
		if err != nil {
			tx.Rollback(ctx)
			return fail(err)
		}
		if len(clients) == 0 {
			tx.Rollback(ctx)
			break
		}
		c.log.Debug("clients page fetched", "lastID", lastID, "count", len(clients))
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
			tx.Rollback(ctx)
			return fail(err)
		}
		if err := c.newDB.MigrationStore().InsertMigrations(ctx, tx, migrationRows); err != nil {
			tx.Rollback(ctx)
			return fail(err)
		}

		// query is ORDER BY c.id on the outer SELECT, so the last row is the max id fetched
		lastID = clients[len(clients)-1].ID

		if err := c.newDB.MigrationStore().SaveIDCursorProgressInTx(ctx, tx, StepClientsToContacts, lastID); err != nil {
			tx.Rollback(ctx)
			return fail(err)
		}

		if err := tx.Commit(ctx); err != nil {
			return fail(err)
		}

		if len(clients) < perPage {
			break
		}
	}

	return nil
}

func (c *Converter) MigrateClientsToContactsSyncMode(ctx context.Context) error {
	const perPage = 1000
	c.log.Debug("starting clients-to-contacts migration")

	lastID, err := c.newDB.MigrationStore().GetIDCursorProgress(ctx, SyncStepClientsToContacts)
	if err != nil {
		return err
	}
	if lastID > 0 {
		c.log.Info("resuming clients-to-contacts migration", "lastID", lastID)
	}

	completedAt, err := c.GetStepCompletedAt(ctx, SyncStepClientsToContacts)
	if err != nil {
		return err
	}

	fail := func(cause error) error {
		_ = c.newDB.MigrationStore().MarkStepFailed(ctx, SyncStepClientsToContacts, 0, cause.Error())
		return cause
	}

	for {
		tx, err := c.newDB.Pool().Begin(ctx)
		if err != nil {
			return fail(err)
		}

		clients, err := c.oldDB.ClientStore().GetFromDate(ctx, lastID, perPage, &completedAt)
		if err != nil {
			tx.Rollback(ctx)
			return fail(err)
		}
		if len(clients) == 0 {
			tx.Rollback(ctx)
			break
		}
		c.log.Debug("clients page fetched", "lastID", lastID, "count", len(clients))
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
			tx.Rollback(ctx)
			return fail(err)
		}
		if err := c.newDB.MigrationStore().InsertMigrations(ctx, tx, migrationRows); err != nil {
			tx.Rollback(ctx)
			return fail(err)
		}

		// query is ORDER BY c.id on the outer SELECT, so the last row is the max id fetched
		lastID = clients[len(clients)-1].ID

		if err := c.newDB.MigrationStore().SaveIDCursorProgressInTx(ctx, tx, SyncStepClientsToContacts, lastID); err != nil {
			tx.Rollback(ctx)
			return fail(err)
		}

		if err := tx.Commit(ctx); err != nil {
			return fail(err)
		}

		if len(clients) < perPage {
			break
		}
	}

	return nil
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
