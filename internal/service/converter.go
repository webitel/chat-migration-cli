package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	modelnew "github.com/webitel/chat-migration-cli/internal/model/new"
	"github.com/webitel/chat-migration-cli/internal/store/newdb"
	"github.com/webitel/chat-migration-cli/internal/store/olddb"
)

const (
	StepClientsToContacts       = "clients_to_contacts"
	StepPortalClientsToContacts = "portal_client_to_contact"
	StepPortalAppsToAccounts    = "portal_apps_to_accounts"
	StepBotsToContacts          = "bots_to_contacts"
	StepConversations           = "conversations"
	StepMembers                 = "members"
	StepMessages                = "messages"
	StepGateways                = "gateways"

	StepFacebookAndWhatsApp = "facebook_and_whatsapp"
	StepSyncContactVias     = "sync_contact_vias"
)

const (
	SyncStepClientsToContacts       = "sync_mode_clients_to_contacts"
	SyncStepPortalClientsToContacts = "sync_mode_portal_client_to_contact"
	SyncStepBotsToContacts          = "sync_mode_bots_to_contacts"
	SyncStepConversations           = "sync_mode_conversations"
	SyncStepMembers                 = "sync_mode_members"
	SyncStepMessages                = "sync_mode_messages"
	SyncStepGateways                = "sync_mode_gateways"

	SyncStepFacebookAndWhatsApp = "sync_mode_facebook_and_whatsapp"
	SyncStepSyncContactVias     = "sync_mode_sync_contact_vias"
)

type Resolver struct {
	db *newdb.DB
}

func NewResolver(db *newdb.DB) *Resolver {
	return &Resolver{db: db}
}

func (r *Resolver) ResolveMigrationRow(ctx context.Context, tx pgx.Tx, entityType modelnew.EntityType, oldID string, extraKey *string, domainID int) (*modelnew.MigrationRow, error) {
	filters := &modelnew.MigrationRowFilters{
		Type:     []modelnew.EntityType{entityType},
		OldIDs:   []string{oldID},
		DomainID: domainID,
	}
	if extraKey != nil {
		filters.ExtraKeys = []string{*extraKey}
	}
	return r.db.MigrationStore().GetMigrationRow(ctx, tx, filters)
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

	isSyncMode           bool
	migratePortalClients bool
	stepRecordsMigrated  int64
}

type MigrationStep struct {
	Name string
	Run  func(ctx context.Context) error
}

// StepDuration records how long a single migration step took to execute,
// and how many records were migrated during that step.
type StepDuration struct {
	Name            string
	Duration        time.Duration
	RecordsMigrated int64
}

func NewConverter(oldDB *olddb.DB, modelnewDB *newdb.DB, encryptor *Encryptor, isSyncMode bool, migratePortalClients bool) *Converter {
	return &Converter{
		log:                  slog.Default(),
		oldDB:                oldDB,
		newDB:                modelnewDB,
		resolver:             NewResolver(modelnewDB),
		encryptor:            encryptor,
		isSyncMode:           isSyncMode,
		migratePortalClients: migratePortalClients,
	}
}

func (c *Converter) Migrate(ctx context.Context) error {
	return c.runSteps(ctx)
}

func (c *Converter) MigrateFromStep(ctx context.Context, stepName string) error {
	return c.runStepsFrom(ctx, stepName)
}

func (c *Converter) MigrateSingleStep(ctx context.Context, stepName string) error {
	return c.runSingleStep(ctx, stepName)
}

func (c *Converter) runSingleStep(ctx context.Context, stepName string) error {
	if stepName == "" {
		return errors.New("step name is required")
	}

	var steps []MigrationStep
	if c.isSyncMode {
		steps = c.getSyncModeMigrationSteps()
	} else {
		steps = c.getMigrationSteps()
	}

	var step *MigrationStep
	for i := range steps {
		if steps[i].Name == stepName {
			step = &steps[i]
			break
		}
	}
	if step == nil {
		return fmt.Errorf("unknown migration step: %s", stepName)
	}

	if !c.isSyncMode {
		completed, err := c.newDB.MigrationStore().GetCompletedSteps(ctx)
		if err != nil {
			return err
		}
		if _, ok := completed[step.Name]; ok {
			return fmt.Errorf("migration step %q is already completed; re-run is not supported outside sync mode - restore the target database and retry", step.Name)
		}
	}

	c.log.Info("migration step started", "step", step.Name)
	c.resetStepRecordsMigrated()
	stepStart := time.Now()
	if err := step.Run(ctx); err != nil {
		return err
	}
	duration := time.Since(stepStart)
	recordsMigrated := c.stepRecordsMigrated
	if err := c.newDB.MigrationStore().MarkStepCompleted(ctx, step.Name); err != nil {
		return err
	}
	c.log.Info("migration step completed", "step", step.Name)
	c.logStepDurations([]StepDuration{{Name: step.Name, Duration: duration, RecordsMigrated: recordsMigrated}})
	return nil
}

func (c *Converter) runStepsFrom(ctx context.Context, startFrom string) error {
	if startFrom == "" {
		return errors.New("step requires to start from it")
	}
	var (
		steps     []MigrationStep
		completed map[string]struct{}
		err       error
	)

	if c.isSyncMode {
		steps = c.getSyncModeMigrationSteps()
		completed = make(map[string]struct{})

	} else {
		steps = c.getMigrationSteps()
		completed, err = c.newDB.MigrationStore().GetCompletedSteps(ctx)
		if err != nil {
			return err
		}
	}

	var (
		firstStepIndex int
		found          bool
	)
	for i, step := range steps {
		if step.Name == startFrom {
			found = true
			if _, alreadyCompleted := completed[step.Name]; alreadyCompleted {
				if i > 0 {
					for s, nextUncompletedStep := range steps[i-1:] {
						if _, alreadyCompleted := completed[nextUncompletedStep.Name]; !alreadyCompleted {
							firstStepIndex = s
							break
						}
					}
				}
			} else {
				firstStepIndex = i
			}
			break
		}
	}

	if !found {
		return fmt.Errorf("unknown migration step: %s", startFrom)
	}

	durations := make([]StepDuration, 0, len(steps)-firstStepIndex)
	for _, step := range steps[firstStepIndex:] {
		if _, ok := completed[step.Name]; ok {
			c.log.Info("migration step already completed, skipping", "step", step.Name)
			continue
		}

		c.log.Info("migration step started", "step", step.Name)
		c.resetStepRecordsMigrated()
		stepStart := time.Now()
		if err := step.Run(ctx); err != nil {
			c.logStepDurations(durations)
			return err
		}
		durations = append(durations, StepDuration{Name: step.Name, Duration: time.Since(stepStart), RecordsMigrated: c.stepRecordsMigrated})

		if err := c.newDB.MigrationStore().MarkStepCompleted(ctx, step.Name); err != nil {
			c.logStepDurations(durations)
			return err
		}
		c.log.Info("migration step completed", "step", step.Name)
	}
	c.logStepDurations(durations)
	return nil
}

func (c *Converter) runSteps(ctx context.Context) error {
	var (
		steps     []MigrationStep
		completed map[string]struct{}
		err       error
	)

	if c.isSyncMode {
		steps = c.getSyncModeMigrationSteps()
		completed = make(map[string]struct{})

	} else {
		steps = c.getMigrationSteps()
		completed, err = c.newDB.MigrationStore().GetCompletedSteps(ctx)
		if err != nil {
			return err
		}
	}

	durations := make([]StepDuration, 0, len(steps))
	for _, step := range steps {
		if _, ok := completed[step.Name]; ok {
			c.log.Info("migration step already completed, skipping", "step", step.Name)
			continue
		}

		c.log.Info("migration step started", "step", step.Name)
		c.resetStepRecordsMigrated()
		stepStart := time.Now()
		if err := step.Run(ctx); err != nil {
			c.logStepDurations(durations)
			return err
		}
		durations = append(durations, StepDuration{Name: step.Name, Duration: time.Since(stepStart), RecordsMigrated: c.stepRecordsMigrated})

		if err := c.newDB.MigrationStore().MarkStepCompleted(ctx, step.Name); err != nil {
			c.logStepDurations(durations)
			return err
		}
		c.log.Info("migration step completed", "step", step.Name)
	}
	c.logStepDurations(durations)
	return nil
}

func (c *Converter) resetStepRecordsMigrated() {
	c.stepRecordsMigrated = 0
}

func (c *Converter) addRecordsMigrated(n int) {
	c.stepRecordsMigrated += int64(n)
}

// logStepDurations emits a summary report of how long each executed
// migration step took, how many records were migrated, plus the total
// elapsed time and record count across all of them.
// It is a no-op when durations is empty (e.g. every step in the run was
// already completed and skipped).
func (c *Converter) logStepDurations(durations []StepDuration) {
	if len(durations) == 0 {
		return
	}
	var (
		total        time.Duration
		totalRecords int64
	)
	for _, d := range durations {
		total += d.Duration
		totalRecords += d.RecordsMigrated
		c.log.Info("migration step duration", "step", d.Name, "duration", d.Duration.String(), "records_migrated", d.RecordsMigrated)
	}
	c.log.Info("migration duration summary", "steps", len(durations), "total_duration", total.String(), "total_records_migrated", totalRecords)
}

func (c *Converter) getMigrationSteps() []MigrationStep {
	steps := []MigrationStep{
		{Name: StepClientsToContacts, Run: c.MigrateClientsToContacts},
	}
	if c.migratePortalClients {
		steps = append(steps, MigrationStep{Name: StepPortalClientsToContacts, Run: c.MigratePortalClientsToContacts})
		steps = append(steps, MigrationStep{Name: StepPortalAppsToAccounts, Run: c.MigratePortalAppsToAccounts})
	}
	steps = append(steps, []MigrationStep{
		{Name: StepBotsToContacts, Run: c.MigrateBotsToContacts},
		{Name: StepConversations, Run: c.MigrateConversations},
		{Name: StepMembers, Run: c.MigrateMembers},
		{Name: StepMessages, Run: c.MigrateMessages},
		{Name: StepFacebookAndWhatsApp, Run: c.MigrateFacebookProviders},
		{Name: StepSyncContactVias, Run: c.SyncContactsVias},
	}...)
	return steps
}
func (c *Converter) getSyncModeMigrationSteps() []MigrationStep {
	steps := []MigrationStep{
		{Name: SyncStepClientsToContacts, Run: c.MigrateClientsToContactsSyncMode},
	}
	if c.migratePortalClients {
		steps = append(steps, MigrationStep{Name: SyncStepPortalClientsToContacts, Run: c.MigratePortalClientsToContactsSyncMode})
	}
	steps = append(steps, []MigrationStep{
		{Name: SyncStepBotsToContacts, Run: c.MigrateBotsToContactsSyncMode},
		{Name: SyncStepConversations, Run: c.MigrateConversationsSyncMode},
		{Name: SyncStepMembers, Run: c.MigrateMembersSyncMode},
		{Name: SyncStepMessages, Run: c.MigrateMessagesSyncMode},
		{Name: SyncStepFacebookAndWhatsApp, Run: c.MigrateFacebookProvidersSyncMode},
		{Name: SyncStepSyncContactVias, Run: c.SyncContactsVias},
	}...)
	return steps
}

var stepNameMap = map[string]string{
	SyncStepMembers:                 StepMembers,
	SyncStepConversations:           StepConversations,
	SyncStepBotsToContacts:          StepBotsToContacts,
	SyncStepClientsToContacts:       StepClientsToContacts,
	SyncStepPortalClientsToContacts: StepPortalClientsToContacts,
	SyncStepFacebookAndWhatsApp:     StepFacebookAndWhatsApp,
	SyncStepMessages:                StepMessages,
	SyncStepSyncContactVias:         StepSyncContactVias,
	SyncStepGateways:                StepGateways,
}

func (c *Converter) GetStepCompletedAtInTx(ctx context.Context, tx pgx.Tx, step string) (time.Time, error) {
	analogStepName := stepNameMap[step]
	if analogStepName == "" {
		analogStepName = step
	}
	completedAt, err := c.newDB.MigrationStore().GetStepCompletedAtInTx(ctx, tx, step, analogStepName)
	if err != nil {
		return time.Time{}, err
	}
	return completedAt, nil
}

func (c *Converter) GetStepCompletedAt(ctx context.Context, step string) (time.Time, error) {
	analogStepName := stepNameMap[step]
	if analogStepName == "" {
		analogStepName = step
	}
	completedAt, err := c.newDB.MigrationStore().GetStepCompletedAt(ctx, step, analogStepName)
	if err != nil {
		return time.Time{}, err
	}
	return completedAt, nil
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
