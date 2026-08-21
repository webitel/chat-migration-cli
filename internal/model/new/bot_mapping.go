package new

import "github.com/google/uuid"

// BotMapping mirrors a row of the client-managed table named by
// MIGRATION_BOT_MAPPING_TABLE. The tool never creates, seeds, or validates
// this table's contents against existing contacts.
type BotMapping struct {
	OldBotID int       `db:"old_bot_id"`
	NewBotID uuid.UUID `db:"new_bot_id"`
}
