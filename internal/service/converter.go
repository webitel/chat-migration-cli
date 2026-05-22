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
)

type Resolver struct {
	db *newdb.DB
}

func NewResolver(db *newdb.DB) *Resolver {
	return &Resolver{db: db}
}

func (r *Resolver) ResolveMigrationRow(ctx context.Context, tx pgx.Tx, entityType modelnew.EntityType, oldID string, extraKey string) (*modelnew.MigrationRow, error) {
	return r.db.MigrationStore().GetMigrationRow(ctx, tx, &modelnew.MigrationRowFilters{
		Type:      []modelnew.EntityType{entityType},
		OldIDs:    []string{oldID},
		ExtraKeys: []string{extraKey},
	})
}

func (r *Resolver) ResolveMigrationRows(ctx context.Context, tx pgx.Tx, filters *modelnew.MigrationRowFilters) ([]*modelnew.MigrationRow, error) {
	return r.db.MigrationStore().GetMigrationRows(ctx, tx, filters)
}

type Converter struct {
	log      *slog.Logger
	oldDB    *olddb.DB
	newDB    *newdb.DB
	resolver *Resolver
}

type MigrationStep struct {
	Name string
	Run  func(ctx context.Context) error
}

func NewConverter(oldDB *olddb.DB, modelnewDB *newdb.DB) *Converter {
	return &Converter{
		log:      slog.Default(),
		oldDB:    oldDB,
		newDB:    modelnewDB,
		resolver: NewResolver(modelnewDB),
	}
}

func (c *Converter) Migrate(ctx context.Context) error {
	return c.runSteps(ctx, "")
}

func (c *Converter) MigrateFromStep(ctx context.Context, stepName string) error {
	return c.runSteps(ctx, stepName)
}

func (c *Converter) runSteps(ctx context.Context, startFrom string) error {
	steps := c.getMigrationSteps()

	completed, err := c.newDB.MigrationStore().GetCompletedSteps(ctx)
	if err != nil {
		return err
	}

	run := startFrom == ""
	for _, step := range steps {
		if !run {
			run = step.Name == startFrom
			if !run {
				continue
			}
		}

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

	if startFrom != "" && !run {
		return errors.New("start step not found")
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
