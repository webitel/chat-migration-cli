package new

import "github.com/google/uuid"

type EntityType string

const (
	EntityTypeClientContact                EntityType = "client_contact"
	EntityTypeBotContact                   EntityType = "bot_contact"
	EntityTypeConversationThread           EntityType = "conversation_thread"
	EntityTypeInitiatorChannelThreadDialog EntityType = "initiator_channel_thread_dialog"
	EntityTypeBotChannelThreadDialog       EntityType = "bot_channel_thread_dialog"
	EntityTypeInternalChannelThreadDialog  EntityType = "internal_channel_thread_dialog"
	EntityTypeMessage                      EntityType = "message"
)

type MigrationRowFilters struct {
	Offset    int
	Limit     int
	Type      []EntityType
	OldIDs    []string
	ExtraKeys []string
	DomainID  int
}

type MigrationRow struct {
	ID         uuid.UUID  `db:"id"`
	EntityType EntityType `db:"entity_type"`
	OldID      string     `db:"old_id"`
	NewID      uuid.UUID  `db:"new_id"`
	DomainID   int        `db:"domain_id"`
	ExtraKey   string     `db:"extra_key"`
}
