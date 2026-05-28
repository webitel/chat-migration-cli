package old

import "time"

type Bot struct {
	IDs       []int     `db:"ids"`
	DC        int       `db:"dc"`
	Name      string    `db:"name"`
	FlowID    int       `db:"flow_id"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}

type Gateway struct {
	ID        int       `db:"id"`
	DC        int       `db:"dc"`
	URI       string    `db:"uri"`
	Name      string    `db:"name"`
	FlowID    int       `db:"flow_id"`
	Enabled   bool      `db:"enabled"`
	Metadata  []byte    `db:"metadata"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
	Updates   []byte    `db:"updates"`
}

type FacebookGateway struct {
	ID        int
	DC        int
	URI       string
	Name      string
	FlowID    int
	Enabled   bool
	CreatedAt time.Time
	UpdatedAt time.Time
}
