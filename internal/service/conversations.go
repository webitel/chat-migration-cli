package service

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	modelnew "github.com/webitel/chat-migration-cli/internal/model/new"
	"github.com/webitel/chat-migration-cli/internal/model/old"
)

const (
	newThreadAfterSyncExtraKey = "sync_new"
)

func (c *Converter) MigrateConversations(ctx context.Context) error {
	const perPage = 1000
	c.log.Debug("starting conversations migration")

	lastInitiator, lastFlowID, err := c.newDB.MigrationStore().GetCursorProgress(ctx, StepConversations)
	if err != nil {
		return err
	}
	if lastInitiator > 0 || lastFlowID > 0 {
		c.log.Info("resuming conversations migration", "lastInitiator", lastInitiator, "lastFlowID", lastFlowID)
	}

	fail := func(cause error) error {
		_ = c.newDB.MigrationStore().MarkStepFailed(ctx, StepConversations, 0, cause.Error())
		return cause
	}

	for {
		tx, err := c.newDB.Pool().Begin(ctx)
		if err != nil {
			return fail(err)
		}

		groupedConversations, err := c.oldDB.ConversationStore().GetGroupedConversationsByUsersAndFlow(ctx, lastInitiator, lastFlowID, perPage)
		if err != nil {
			tx.Rollback(ctx)
			return fail(err)
		}
		if len(groupedConversations) == 0 {
			tx.Rollback(ctx)
			break
		}
		c.log.Debug("conversations page fetched", "lastInitiator", lastInitiator, "lastFlowID", lastFlowID, "count", len(groupedConversations))

		var (
			threads       []*modelnew.Thread
			migrationRows []*modelnew.MigrationRow
		)
		for _, conversation := range groupedConversations {
			converted := convertGroupedConversationToThread(conversation)
			for _, convID := range conversation.ConvIDs {
				migrationRows = append(migrationRows, &modelnew.MigrationRow{
					ID:         uuid.New(),
					EntityType: modelnew.EntityTypeConversationThread,
					OldID:      convID.String(),
					NewID:      converted.ID,
					DomainID:   conversation.DomainID,
				})
			}
			migrationRows = append(migrationRows, &modelnew.MigrationRow{
				ID:         uuid.New(),
				EntityType: modelnew.EntityTypeFlowIDAndInitiatorIDToThread,
				OldID:      buildFlowIDAndInitiatorIdToThreadOldID(conversation.FlowID, conversation.Initiator),
				NewID:      converted.ID,
				DomainID:   conversation.DomainID,
			})
			threads = append(threads, converted)
		}

		if err := c.newDB.ThreadStore().InsertThreads(ctx, tx, threads); err != nil {
			tx.Rollback(ctx)
			return fail(err)
		}
		if err := c.newDB.MigrationStore().InsertMigrations(ctx, tx, migrationRows); err != nil {
			tx.Rollback(ctx)
			return fail(err)
		}

		// advance cursor to the max group on this page (result row order is not guaranteed)
		var maxInitiator, maxFlowID int
		for _, conv := range groupedConversations {
			if conv.Initiator > maxInitiator || (conv.Initiator == maxInitiator && conv.FlowID > maxFlowID) {
				maxInitiator = conv.Initiator
				maxFlowID = conv.FlowID
			}
		}
		lastInitiator, lastFlowID = maxInitiator, maxFlowID

		if err := c.newDB.MigrationStore().SaveCursorProgressInTx(ctx, tx, StepConversations, lastInitiator, lastFlowID); err != nil {
			tx.Rollback(ctx)
			return fail(err)
		}

		if err := tx.Commit(ctx); err != nil {
			return fail(err)
		}

		c.log.Debug("conversations page committed", "lastInitiator", lastInitiator, "lastFlowID", lastFlowID, "conversations", len(groupedConversations))

		if len(groupedConversations) < perPage {
			break
		}
	}

	return nil
}

func buildFlowIDAndInitiatorIdToThreadOldID(flowID, initiatorID int) string {
	return strconv.Itoa(flowID) + "_" + strconv.Itoa(initiatorID)
}

func deconstructFlowIDAndInitiatorId(recordedID string) (flowID, initiatorID int) {
	parts := strings.Split(recordedID, "_")
	if len(parts) != 2 {
		return 0, 0
	}
	flowID, _ = strconv.Atoi(parts[0])
	initiatorID, _ = strconv.Atoi(parts[1])
	return
}

func (c *Converter) MigrateConversationsSyncMode(ctx context.Context) error {
	const (
		perPage  = 1000
		stepName = SyncStepConversations
	)
	c.log.Debug("starting conversations migration")

	lastInitiator, lastFlowID, err := c.newDB.MigrationStore().GetCursorProgress(ctx, stepName)
	if err != nil {
		return err
	}
	isResuming := lastInitiator > 0 || lastFlowID > 0
	if isResuming {
		c.log.Info("resuming conversations migration", "lastInitiator", lastInitiator, "lastFlowID", lastFlowID)
	} else {
		// Run NullifyMigrationRowsExtraKey in a separate transaction, but only on a
		// fresh start. If we're resuming, a prior run of this step already committed
		// some pages tagged with newThreadAfterSyncExtraKey; nullifying now would wipe
		// those tags without ever re-tagging them (their conversations are already
		// migrated and won't be seen again), which makes MigrateMembersSyncMode build
		// the wrong dialog set for those threads.
		nullifyTx, err := c.newDB.Pool().Begin(ctx)
		if err != nil {
			return err
		}
		err = c.newDB.MigrationStore().NullifyMigrationRowsExtraKey(ctx, nullifyTx, newThreadAfterSyncExtraKey, string(modelnew.EntityTypeFlowIDAndInitiatorIDToThread))
		if err != nil {
			nullifyTx.Rollback(ctx)
			return err
		}
		if err := nullifyTx.Commit(ctx); err != nil {
			return err
		}
	}

	completedAt, err := c.GetStepCompletedAt(ctx, stepName)
	if err != nil {
		return err
	}

	fail := func(cause error) error {
		_ = c.newDB.MigrationStore().MarkStepFailed(ctx, stepName, 0, cause.Error())
		return cause
	}

	for {
		tx, err := c.newDB.Pool().Begin(ctx)
		if err != nil {
			return fail(err)
		}

		groupedConversations, err := c.oldDB.ConversationStore().GetGroupedConversationsByUsersAndFlowFromDate(ctx, lastInitiator, lastFlowID, perPage, completedAt)
		if err != nil {
			tx.Rollback(ctx)
			return fail(err)
		}
		if len(groupedConversations) == 0 {
			tx.Rollback(ctx)
			break
		}
		originalCount := len(groupedConversations)
		c.log.Debug("conversations page fetched", "lastInitiator", lastInitiator, "lastFlowID", lastFlowID, "count", originalCount)

		// compute the max cursor from the originally fetched page, before the
		// already-migrated dedup loop below mutates groupedConversations; result row
		// order is not guaranteed, so take the max rather than the last element.
		var maxInitiator, maxFlowID int
		for _, conv := range groupedConversations {
			if conv.Initiator > maxInitiator || (conv.Initiator == maxInitiator && conv.FlowID > maxFlowID) {
				maxInitiator = conv.Initiator
				maxFlowID = conv.FlowID
			}
		}

		var (
			threads       []*modelnew.Thread
			migrationRows []*modelnew.MigrationRow

			idsToCheck []string
		)
		for _, conv := range groupedConversations {
			idsToCheck = append(idsToCheck, buildFlowIDAndInitiatorIdToThreadOldID(conv.FlowID, conv.Initiator))
		}

		alreadyMigratedThreads, err := c.newDB.MigrationStore().GetMigrationRows(ctx, tx, &modelnew.MigrationRowFilters{
			OldIDs: idsToCheck,
			Type:   []modelnew.EntityType{modelnew.EntityTypeFlowIDAndInitiatorIDToThread},
		})
		if err != nil {
			tx.Rollback(ctx)
			return fail(err)
		}

		for _, thread := range alreadyMigratedThreads {
			var (
				j     int
				found bool
			)
			flowID, initiatorID := deconstructFlowIDAndInitiatorId(thread.OldID)
			for i, conv := range groupedConversations {
				if flowID == conv.FlowID && initiatorID == conv.Initiator && thread.DomainID == conv.DomainID {
					for _, convID := range conv.ConvIDs {
						migrationRows = append(migrationRows, &modelnew.MigrationRow{
							ID:         uuid.New(),
							EntityType: modelnew.EntityTypeConversationThread,
							OldID:      convID.String(),
							NewID:      thread.NewID,
							DomainID:   thread.DomainID,
						})
					}

					j = i
					found = true
					break
				}
			}
			if found {
				groupedConversations = append(groupedConversations[:j], groupedConversations[j+1:]...)
			}

		}

		for _, conversation := range groupedConversations {
			converted := convertGroupedConversationToThread(conversation)
			for _, convID := range conversation.ConvIDs {
				migrationRows = append(migrationRows, &modelnew.MigrationRow{
					ID:         uuid.New(),
					EntityType: modelnew.EntityTypeConversationThread,
					OldID:      convID.String(),
					NewID:      converted.ID,
					DomainID:   conversation.DomainID,
				})
			}
			syncExtraKey := newThreadAfterSyncExtraKey
			migrationRows = append(migrationRows, &modelnew.MigrationRow{
				ID:         uuid.New(),
				EntityType: modelnew.EntityTypeFlowIDAndInitiatorIDToThread,
				OldID:      buildFlowIDAndInitiatorIdToThreadOldID(conversation.FlowID, conversation.Initiator),
				NewID:      converted.ID,
				DomainID:   conversation.DomainID,
				ExtraKey:   &syncExtraKey,
			})
			threads = append(threads, converted)
		}

		if err := c.newDB.ThreadStore().InsertThreads(ctx, tx, threads); err != nil {
			tx.Rollback(ctx)
			return fail(err)
		}
		if err := c.newDB.MigrationStore().InsertMigrations(ctx, tx, migrationRows); err != nil {
			tx.Rollback(ctx)
			return fail(err)
		}

		// advance cursor to the max group on this page (computed above, before dedup)
		lastInitiator, lastFlowID = maxInitiator, maxFlowID

		if err := c.newDB.MigrationStore().SaveCursorProgressInTx(ctx, tx, stepName, lastInitiator, lastFlowID); err != nil {
			tx.Rollback(ctx)
			return fail(err)
		}

		if err := tx.Commit(ctx); err != nil {
			return fail(err)
		}

		c.log.Debug("conversations page committed", "lastInitiator", lastInitiator, "lastFlowID", lastFlowID, "conversations", originalCount)

		if originalCount < perPage {
			break
		}
	}
	return nil
}

func convertGroupedConversationToThread(groupedConversation *old.GroupedConversation) *modelnew.Thread {
	return &modelnew.Thread{
		ID:        uuid.New(),
		DomainID:  groupedConversation.DomainID,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Subject:   groupedConversation.Title,
		Kind:      modelnew.ThreadDirect,
	}
}
