package old

import (
	"time"

	"github.com/google/uuid"
)

type GroupedConversation struct {
	ConvIDs       uuid.UUIDs          `db:"conv_ids"`
	Initiator     int                 `db:"initiator"`
	FlowID        int                 `db:"flow_id"`
	Title         string              `db:"title"`
	DomainID      int                 `db:"domain_id"`
	InternalUsers []*ConversationUser `db:"internal_users"`
}

type ConversationUser struct {
	ChannelIDs  []uuid.UUID `db:"channel_id"`
	UserID      int         `db:"user_id"`
	CreatedAt   time.Time   `db:"created_at"`
	ClosedAt    time.Time   `db:"closed_at"`
	LeaveReason *string     `db:"leave_reason"`
}
