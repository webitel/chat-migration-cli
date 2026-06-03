package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
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
		c.log.Debug("members page fetched", "lastInitiator", lastInitiator, "lastFlowID", lastFlowID, "count", len(groupedConversations))
		for _, groupedConv := range groupedConversations {
			if len(groupedConv.ConvIDs) == 0 {
				c.log.Warn("grouped conversation has no conv IDs, skipping",
					"initiator", groupedConv.Initiator,
					"flow_id", groupedConv.FlowID,
				)
				continue
			}
			thread, err := c.resolver.ResolveMigrationRow(ctx, tx, modelnew.EntityTypeConversationThread, groupedConv.ConvIDs[0].String(), "", groupedConv.DomainID)
			if err != nil {
				tx.Rollback(ctx)
				return errors.Join(errors.New("failed to resolve migration row for conversation thread "+groupedConv.ConvIDs[0].String()), err)
			}
			dialogs, rows, err := c.buildThreadDialogsFromConversation(ctx, tx, groupedConv, thread.NewID)
			if err != nil {
				if errors.Is(err, errInitiatorNotFound) {
					c.log.Warn("initiator not found, skipping", slog.String("error", err.Error()))
					continue
				}
				tx.Rollback(ctx)
				return errors.Join(errors.New("failed to build thread dialogs from conversation"), err)
			}
			threadDialogs = append(threadDialogs, dialogs...)
			migrationRows = append(migrationRows, rows...)
		}
		if err := c.newDB.ThreadDialogStore().InsertThreadDialogs(ctx, tx, threadDialogs); err != nil {
			tx.Rollback(ctx)
			return errors.Join(errors.New("failed to insert thread dialogs"), err)
		}
		if err := c.newDB.MigrationStore().InsertMigrations(ctx, tx, migrationRows); err != nil {
			tx.Rollback(ctx)
			return errors.Join(errors.New("failed to insert migration rows for thread dialogs"), err)
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

	ownerDialogs, ownerRows, err := c.buildOwnerThreadDialogFromConversation(ctx, tx, conversation, newThreadID)
	if err != nil {
		return nil, nil, errors.Join(errors.New("failed to build owner thread dialog from conversation"), err)
	}

	dialogs, rows, err := c.buildInternalUsersThreadDialogs(ctx, tx, conversation, newThreadID)
	if err != nil {
		return nil, nil, errors.Join(errors.New("failed to build internal users thread dialogs"), err)
	}
	threadDialogs = append(threadDialogs, dialogs...)
	migrationRows = append(migrationRows, rows...)
	threadDialogs = append(threadDialogs, ownerDialogs...)
	migrationRows = append(migrationRows, ownerRows...)

	return threadDialogs, migrationRows, nil
}

var errInitiatorNotFound = errors.New("initiator not found")

func (c *Converter) buildOwnerThreadDialogFromConversation(ctx context.Context, tx pgx.Tx, conversation *old.GroupedConversation, newThreadID uuid.UUID) ([]*modelnew.ThreadDialog, []*modelnew.MigrationRow, error) {
	initiatorContact, err := c.resolver.ResolveMigrationRow(ctx, tx, modelnew.EntityTypeClientContact, strconv.Itoa(conversation.Initiator), "", conversation.DomainID)
	if err != nil {
		c.log.Error("failed to resolve initiator contact", slog.String("error", err.Error()), slog.Int("initiator", conversation.Initiator), slog.Int("domain_id", conversation.DomainID))
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil, errInitiatorNotFound
		}
		return nil, nil, err
	}
	botContact, err := c.resolver.ResolveMigrationRow(ctx, tx, modelnew.EntityTypeBotContact, strconv.Itoa(conversation.FlowID), "", conversation.DomainID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			botContact, err = c.restoreBotFromConversation(ctx, conversation, tx)
			if err != nil {
				c.log.Error("failed to restore bot from conversation", slog.Int("flow_id", conversation.FlowID), slog.Int("domain_id", conversation.DomainID))
				return nil, nil, errors.Join(errors.New("failed to restore bot from conversation"), err)
			}
		}
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

func (c *Converter) restoreBotFromConversation(ctx context.Context, conversation *old.GroupedConversation, tx pgx.Tx) (*modelnew.MigrationRow, error) {
	var (
		flowID = strconv.Itoa(conversation.FlowID)
		now    = time.Now()
		bot    *modelnew.Contact
	)
	bots, err := c.newDB.ContactStore().GetByFlowIDs(ctx, tx, []string{flowID})
	if err != nil {
		return nil, err
	}
	if len(bots) == 0 {
		bot = &modelnew.Contact{
			BaseModel: modelnew.BaseModel{
				ID:        uuid.New(),
				DomainID:  conversation.DomainID,
				CreatedAt: now,
				UpdatedAt: now,
			},
			IssuerID:  BotIssuerID,
			SubjectID: flowID,
			Type:      "bot",
			Name:      fmt.Sprintf("Flow %d Bot", conversation.FlowID),
			Username:  fmt.Sprintf("flow_%d_bot", conversation.FlowID),
			IsBot:     true,
		}
		err := c.newDB.ContactStore().InsertContactsIgnoreConflicts(ctx, tx, []*modelnew.Contact{bot})
		if err != nil {
			return nil, err
		}
	} else {
		bot = bots[0]
	}

	migrationRow := &modelnew.MigrationRow{
		ID:         uuid.New(),
		EntityType: modelnew.EntityTypeBotContact,
		OldID:      strconv.Itoa(conversation.FlowID),
		NewID:      bot.ID,
		DomainID:   conversation.DomainID,
	}
	err = c.newDB.MigrationStore().InsertMigrations(ctx, tx, []*modelnew.MigrationRow{migrationRow})
	if err != nil {
		return nil, err
	}
	return migrationRow, nil
}

func (c *Converter) buildInternalUsersThreadDialogs(ctx context.Context, tx pgx.Tx, conversation *old.GroupedConversation, threadID uuid.UUID) ([]*modelnew.ThreadDialog, []*modelnew.MigrationRow, error) {
	var (
		threadDialogs  []*modelnew.ThreadDialog
		migrationRows  []*modelnew.MigrationRow
		webitelUserIDs []string
	)
	for _, user := range conversation.InternalUsers {
		webitelUserIDs = append(webitelUserIDs, strconv.Itoa(user.UserID))
	}
	contacts, err := c.newDB.ContactStore().GetByWebitelUserIDs(ctx, tx, webitelUserIDs)
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
			foundContact, err = c.restoreWebitelUser(ctx, tx, user, threadID, conversation.DomainID)
			if err != nil {
				c.log.Warn("webitel user can't be restored, skipping", slog.Int("user_id", user.UserID), slog.String("thread_id", threadID.String()), slog.String("err", err.Error()))
				err = nil
				continue
			}
		}

		var deletedAt *time.Time
		if !user.ClosedAt.IsZero() {
			deletedAt = &user.ClosedAt
		}
		threadDialog := &modelnew.ThreadDialog{
			ID:          uuid.New(),
			ThreadID:    threadID,
			MemberID:    foundContact.ID,
			ThreadRole:  modelnew.RoleMember,
			DomainID:    conversation.DomainID,
			CreatedAt:   user.CreatedAt,
			UpdatedAt:   user.ClosedAt,
			DeletedAt:   deletedAt,
			LeaveReason: user.LeaveReason,
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

func (c *Converter) restoreWebitelUser(ctx context.Context, tx pgx.Tx, user *old.ConversationUser, threadID uuid.UUID, domainID int) (*modelnew.Contact, error) {
	now := time.Now()
	name := "Deleted user"
	if user.Name != nil {
		name = *user.Name
	}
	c.log.Warn("webitel user not found for conversation member, try to restore user",
		"user_id", user.UserID,
		"thread_id", threadID,
	)
	newContact := &modelnew.Contact{
		BaseModel: modelnew.BaseModel{
			ID:        uuid.New(),
			DomainID:  domainID,
			CreatedAt: now,
			UpdatedAt: now,
		},
		IssuerID:  "webitel",
		SubjectID: strconv.Itoa(user.UserID),
		Type:      "webitel",
		Name:      name,
		Username:  buildUsername(name, "webitel", strconv.Itoa(user.UserID)),
		IsBot:     false,
	}
	err := c.newDB.ContactStore().InsertContactsIgnoreConflicts(ctx, tx, []*modelnew.Contact{newContact})
	if err != nil {
		return nil, err
	}
	return newContact, nil
}
