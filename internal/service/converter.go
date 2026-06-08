package service

import (
	"context"
	"errors"
	"log/slog"

	"github.com/jackc/pgx/v5"
	modelnew "github.com/webitel/chat-migration-cli/internal/model/new"
	"github.com/webitel/chat-migration-cli/internal/store/newdb"
	"github.com/webitel/chat-migration-cli/internal/store/olddb"
)

const (
	StepClientsToContacts = "clients_to_contacts"
	StepBotsToContacts    = "bots_to_contacts"
	StepConversations     = "conversations"
	StepMembers           = "members"
	StepMessages          = "messages"
	StepGateways          = "gateways"

	StepFacebookAndWhatsApp = "facebook_and_whatsapp"
	StepSyncContactVias     = "sync_contact_vias"
)

type Resolver struct {
	db *newdb.DB
}

func NewResolver(db *newdb.DB) *Resolver {
	return &Resolver{db: db}
}

func (r *Resolver) ResolveMigrationRow(ctx context.Context, tx pgx.Tx, entityType modelnew.EntityType, oldID string, extraKey string, domainID int) (*modelnew.MigrationRow, error) {
	return r.db.MigrationStore().GetMigrationRow(ctx, tx, &modelnew.MigrationRowFilters{
		Type:      []modelnew.EntityType{entityType},
		OldIDs:    []string{oldID},
		ExtraKeys: []string{extraKey},
		DomainID:  domainID,
	})
}

func (r *Resolver) ResolveMigrationRows(ctx context.Context, tx pgx.Tx, filters *modelnew.MigrationRowFilters) ([]*modelnew.MigrationRow, error) {
	return r.db.MigrationStore().GetMigrationRows(ctx, tx, filters)
}

type Converter struct {
	log       *slog.Logger
	oldDB     *olddb.DB
	newDB     *newdb.DB
	resolver  *Resolver
	encryptor *Encryptor
}

type MigrationStep struct {
	Name string
	Run  func(ctx context.Context) error
}

func NewConverter(oldDB *olddb.DB, modelnewDB *newdb.DB, encryptor *Encryptor) *Converter {
	return &Converter{
		log:       slog.Default(),
		oldDB:     oldDB,
		newDB:     modelnewDB,
		resolver:  NewResolver(modelnewDB),
		encryptor: encryptor,
	}
}

func (c *Converter) Migrate(ctx context.Context) error {
	return c.runSteps(ctx)
}

func (c *Converter) MigrateFromStep(ctx context.Context, stepName string) error {
	return c.runStepsFrom(ctx, stepName)
}

func (c *Converter) runStepsFrom(ctx context.Context, startFrom string) error {
	if startFrom == "" {
		return errors.New("step requires to start from it")
	}
	steps := c.getMigrationSteps()

	completed, err := c.newDB.MigrationStore().GetCompletedSteps(ctx)
	if err != nil {
		return err
	}

	var (
		firstStepIndex int
	)
	for i, step := range steps {
		if step.Name == startFrom {
			if _, alreadyCompleted := completed[step.Name]; alreadyCompleted {
				for s, nextUncompletedStep := range steps[i-1:] {
					if _, alreadyCompleted := completed[nextUncompletedStep.Name]; !alreadyCompleted {
						firstStepIndex = s
						break
					}
				}
			} else {
				firstStepIndex = i
			}
			break
		}
	}

	for _, step := range steps[firstStepIndex:] {
		if _, ok := completed[step.Name]; ok {
			c.log.Info("migration step already completed, skipping", "step", step.Name)
			continue
		}

		c.log.Info("migration step started", "step", step.Name)
		if err := step.Run(ctx); err != nil {
			return err
		}

		if err := c.newDB.MigrationStore().MarkStepCompleted(ctx, step.Name); err != nil {
			return err
		}
		c.log.Info("migration step completed", "step", step.Name)
	}
	return nil
}

func (c *Converter) runSteps(ctx context.Context) error {
	steps := c.getMigrationSteps()

	completed, err := c.newDB.MigrationStore().GetCompletedSteps(ctx)
	if err != nil {
		return err
	}

	for _, step := range steps {
		if _, ok := completed[step.Name]; ok {
			c.log.Info("migration step already completed, skipping", "step", step.Name)
			continue
		}

		c.log.Info("migration step started", "step", step.Name)
		if err := step.Run(ctx); err != nil {
			return err
		}

		if err := c.newDB.MigrationStore().MarkStepCompleted(ctx, step.Name); err != nil {
			return err
		}
		c.log.Info("migration step completed", "step", step.Name)
	}
	return nil
}

func (c *Converter) getMigrationSteps() []MigrationStep {
	return []MigrationStep{
		{Name: StepClientsToContacts, Run: c.MigrateClientsToContacts},
		{Name: StepBotsToContacts, Run: c.MigrateBotsToContacts},
		{Name: StepConversations, Run: c.MigrateConversations},
		{Name: StepMembers, Run: c.MigrateMembers},
		{Name: StepMessages, Run: c.MigrateMessages},
		{Name: StepFacebookAndWhatsApp, Run: c.MigrateFacebookProviders},
		{Name: StepSyncContactVias, Run: c.SyncContactsVias},
	}
}

func PagerFunc(ctx context.Context, perPage int, do func(ctx context.Context, offset, limit int) (bool, error)) error {
	var (
		limit   = perPage
		iterate = true
		err     error
	)
	for offset := 0; iterate; offset += perPage {
		iterate, err = do(ctx, offset, limit)
		if err != nil {
			return err
		}
	}
	return nil
}
