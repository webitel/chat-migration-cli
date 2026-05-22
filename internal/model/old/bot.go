package old

import "time"

type Bot struct {
	ID        int       `db:"id"`
	DC        int       `db:"dc"`
	URI       string    `db:"uri"`
	Name      string    `db:"name"`
	FlowID    int       `db:"flow_id"`
	Enabled   bool      `db:"enabled"`
	Provider  string    `db:"provider"`
	Metadata  []byte    `db:"metadata"`
	CreatedAt time.Time `db:"created_at"`
	CreatedBy int       `db:"created_by"`
	UpdatedAt time.Time `db:"updated_at"`
	UpdatedBy int       `db:"updated_by"`
	Updates   []byte    `db:"updates"`
	StorageID int       `db:"storage_id"`
}
