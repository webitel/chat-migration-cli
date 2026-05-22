package service

import (
	"context"
	"time"

	"github.com/google/uuid"
	modelnew "github.com/webitel/chat-migration-cli/internal/model/new"
	"github.com/webitel/chat-migration-cli/internal/model/old"
)

func (c *Converter) MigrateConversations(ctx context.Context) error {
	const perPage = 1000

	tx, err := c.newDB.Pool().Begin(ctx)
	if err != nil {
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
