package service

import (
	"context"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	modelnew "github.com/webitel/chat-migration-cli/internal/model/new"
	"github.com/webitel/chat-migration-cli/internal/model/old"
)

func (c *Converter) MigrateMembers(ctx context.Context) error {
	var (
		perPage = 1000
	)
	c.log.Debug("starting members migration")
	tx, err := c.newDB.Pool().Begin(ctx)
	if err != nil {
		return err
	}
	var lastInitiator, lastFlowID int
	for {
		var (
			threadDialogs []*modelnew.ThreadDialog
			migrationRows []*modelnew.MigrationRow
		)
		groupedConversations, err := c.oldDB.ConversationStore().GetGroupedConversationsByUsersAndFlow(ctx, lastInitiator, lastFlowID, perPage)
		if err != nil {
			tx.Rollback(ctx)
			return err
		}
		if len(groupedConversations) == 0 {
			break
		}
		for _, groupedConv := range groupedConversations {
			if len(groupedConv.ConvIDs) == 0 {
				c.log.Warn("grouped conversation has no conv IDs, skipping",
					"initiator", groupedConv.Initiator,
					"flow_id", groupedConv.FlowID,
				)
				continue
			}
			thread, err := c.resolver.ResolveMigrationRow(ctx, tx, modelnew.EntityTypeConversationThread, groupedConv.ConvIDs[0].String(), "")
			if err != nil {
				tx.Rollback(ctx)
				return err
			}
			dialogs, rows, err := c.buildThreadDialogsFromConversation(ctx, tx, groupedConv, thread.NewID)
			if err != nil {
				tx.Rollback(ctx)
				return err
			}
			threadDialogs = append(threadDialogs, dialogs...)
			migrationRows = append(migrationRows, rows...)
		}
		if err := c.newDB.ThreadDialogStore().InsertThreadDialogs(ctx, tx, threadDialogs); err != nil {
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

func (c *Converter) buildThreadDialogsFromConversation(ctx context.Context, tx pgx.Tx, conversation *old.GroupedConversation, newThreadID uuid.UUID) ([]*modelnew.ThreadDialog, []*modelnew.MigrationRow, error) {
	var (
		threadDialogs []*modelnew.ThreadDialog
		migrationRows []*modelnew.MigrationRow
	)

	dialogs, rows, err := c.buildInternalUsersThreadDialogs(ctx, tx, conversation, newThreadID)
	if err != nil {
		return nil, nil, err
	}
	threadDialogs = append(threadDialogs, dialogs...)
	migrationRows = append(migrationRows, rows...)

	ownerDialogs, ownerRows, err := c.buildOwnerThreadDialogFromConversation(ctx, tx, conversation, newThreadID)
	if err != nil {
		return nil, nil, err
	}
	threadDialogs = append(threadDialogs, ownerDialogs...)
	migrationRows = append(migrationRows, ownerRows...)

	return threadDialogs, migrationRows, nil
}

func (c *Converter) buildOwnerThreadDialogFromConversation(ctx context.Context, tx pgx.Tx, conversation *old.GroupedConversation, newThreadID uuid.UUID) ([]*modelnew.ThreadDialog, []*modelnew.MigrationRow, error) {
	initiatorContact, err := c.resolver.ResolveMigrationRow(ctx, tx, modelnew.EntityTypeClientContact, strconv.Itoa(conversation.Initiator), "")
	if err != nil {
		return nil, nil, err
	}
	botContact, err := c.resolver.ResolveMigrationRow(ctx, tx, modelnew.EntityTypeBotContact, strconv.Itoa(conversation.FlowID), "")
	if err != nil {
		return nil, nil, err
	}

	now := time.Now()
	initiatorDialog := &modelnew.ThreadDialog{
		ID:         uuid.New(),
		ThreadID:   newThreadID,
		MemberID:   initiatorContact.NewID,
		ThreadRole: modelnew.RoleOwner,
		DomainID:   conversation.DomainID,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	botDialog := &modelnew.ThreadDialog{
		ID:         uuid.New(),
		ThreadID:   newThreadID,
		MemberID:   botContact.NewID,
		ThreadRole: modelnew.RoleOwner,
		DomainID:   conversation.DomainID,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	threadDialogs := []*modelnew.ThreadDialog{initiatorDialog, botDialog}
	migrationRows := []*modelnew.MigrationRow{
		{
			ID:         uuid.New(),
			EntityType: modelnew.EntityTypeInitiatorChannelThreadDialog,
			OldID:      strconv.Itoa(conversation.Initiator),
			NewID:      initiatorDialog.ID,
			DomainID:   conversation.DomainID,
			ExtraKey:   newThreadID.String(),
		},
		{
			ID:         uuid.New(),
			EntityType: modelnew.EntityTypeBotChannelThreadDialog,
			OldID:      strconv.Itoa(conversation.FlowID),
			NewID:      botDialog.ID,
			DomainID:   conversation.DomainID,
			ExtraKey:   newThreadID.String(),
		},
	}

	return threadDialogs, migrationRows, nil
}

func (c *Converter) buildInternalUsersThreadDialogs(ctx context.Context, tx pgx.Tx, conversation *old.GroupedConversation, threadID uuid.UUID) ([]*modelnew.ThreadDialog, []*modelnew.MigrationRow, error) {
	var (
		threadDialogs  []*modelnew.ThreadDialog
		migrationRows  []*modelnew.MigrationRow
		webitelUserIDs []int
	)
	for _, user := range conversation.InternalUsers {
		webitelUserIDs = append(webitelUserIDs, user.UserID)
	}
	contacts, err := c.newDB.ContactStore().GetWebitelUsersByWebitelUserIDs(ctx, tx, webitelUserIDs)
	if err != nil {
		return nil, nil, err
	}
	for _, user := range conversation.InternalUsers {
		var foundContact *modelnew.Contact
		for _, contact := range contacts {
			if contact.SubjectID == strconv.Itoa(user.UserID) {
				foundContact = contact
				break
			}
		}
		if foundContact == nil {
			c.log.Warn("webitel user not found for conversation member, skipping",
				"user_id", user.UserID,
				"thread_id", threadID,
			)
			continue
		}
		var deletedAt *time.Time
		if !user.ClosedAt.IsZero() {
			deletedAt = &user.ClosedAt
		}
		threadDialog := &modelnew.ThreadDialog{
			ID:         uuid.New(),
			ThreadID:   threadID,
			MemberID:   foundContact.ID,
			ThreadRole: modelnew.RoleMember,
			DomainID:   conversation.DomainID,
			CreatedAt:  user.CreatedAt,
			UpdatedAt:  user.ClosedAt,
			DeletedAt:  deletedAt,
		}
		threadDialogs = append(threadDialogs, threadDialog)
		migrationRows = append(migrationRows, &modelnew.MigrationRow{
			ID:         uuid.New(),
			EntityType: modelnew.EntityTypeInternalChannelThreadDialog,
			OldID:      strconv.Itoa(user.UserID),
			NewID:      threadDialog.ID,
			DomainID:   conversation.DomainID,
			ExtraKey:   threadID.String(),
		})
	}

	return threadDialogs, migrationRows, nil
}
