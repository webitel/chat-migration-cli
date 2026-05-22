package old

import "time"

type Client struct {
	ID         int       `db:"id"`
	Name       string    `db:"name"`
	Number     string    `db:"number"`
	CreatedAt  time.Time `db:"created_at"`
	ExternalID string    `db:"external_id"`
	FirstName  string    `db:"first_name"`
	LastName   string    `db:"last_name"`
	Type       string    `db:"type"`
	DomainIDs  []int     `db:"domain_ids"`
}
