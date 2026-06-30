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

	tx, err := c.newDB.Pool().Begin(ctx)
	if err != nil {
		return err
	}
	err = c.newDB.MigrationStore().NullifyMigrationRowsExtraKey(ctx, tx, newThreadAfterSyncExtraKey, string(modelnew.EntityTypeFlowIDAndInitiatorIDToThread))
	if err != nil {
		tx.Rollback(ctx)
		return err
	}

	var lastInitiator, lastFlowID int
	for {
		groupedConversations, err := c.oldDB.ConversationStore().GetGroupedConversationsByUsersAndFlow(ctx, lastInitiator, lastFlowID, perPage)
		if err != nil {
			tx.Rollback(ctx)
			return err
		}
		if len(groupedConversations) == 0 {
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
				ExtraKey:   newThreadAfterSyncExtraKey,
			})
			threads = append(threads, converted)
		}

		if err := c.newDB.ThreadStore().InsertThreads(ctx, tx, threads); err != nil {
			tx.Rollback(ctx)
			return err
		}
		if err := c.newDB.MigrationStore().InsertMigrations(ctx, tx, migrationRows); err != nil {
			tx.Rollback(ctx)
			return err
		}

		if len(groupedConversations) < perPage {
			break
		}
		last := groupedConversations[len(groupedConversations)-1]
		lastInitiator = last.Initiator
		lastFlowID = last.FlowID
	}

	return tx.Commit(ctx)
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

	tx, err := c.newDB.Pool().Begin(ctx)
	if err != nil {
		return err
	}
	completedAt, err := c.GetStepCompletedAtInTx(ctx, tx, stepName)
	if err != nil {
		tx.Rollback(ctx)
		return err
	}

	var lastInitiator, lastFlowID int

	for {
		groupedConversations, err := c.oldDB.ConversationStore().GetGroupedConversationsByUsersAndFlowFromDate(ctx, lastInitiator, lastFlowID, perPage, completedAt)
		if err != nil {
			tx.Rollback(ctx)
			return err
		}
		if len(groupedConversations) == 0 {
			break
		}
		originalCount := len(groupedConversations)
		lastOnPage := groupedConversations[originalCount-1]
		c.log.Debug("conversations page fetched", "lastInitiator", lastInitiator, "lastFlowID", lastFlowID, "count", originalCount)

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
			return err
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
			migrationRows = append(migrationRows, &modelnew.MigrationRow{
				ID:         uuid.New(),
				EntityType: modelnew.EntityTypeFlowIDAndInitiatorIDToThread,
				OldID:      buildFlowIDAndInitiatorIdToThreadOldID(conversation.FlowID, conversation.Initiator),
				NewID:      converted.ID,
				DomainID:   conversation.DomainID,
				ExtraKey:   "sync_new",
			})
			threads = append(threads, converted)
		}

		if err := c.newDB.ThreadStore().InsertThreads(ctx, tx, threads); err != nil {
			tx.Rollback(ctx)
			return err
		}
		if err := c.newDB.MigrationStore().InsertMigrations(ctx, tx, migrationRows); err != nil {
			tx.Rollback(ctx)
			return err
		}

		if originalCount < perPage {
			break
		}
		lastInitiator = lastOnPage.Initiator
		lastFlowID = lastOnPage.FlowID
	}
	c.newDB.MigrationStore().SaveCursorProgress(ctx, stepName, lastInitiator, lastFlowID)

	return tx.Commit(ctx)
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
