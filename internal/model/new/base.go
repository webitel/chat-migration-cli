package new

import (
	"time"

	"github.com/google/uuid"
)

type BaseModel struct {
	ID       uuid.UUID `json:"id" db:"id"`
	DomainID int       `json:"domain_id" db:"domain_id"`

	CreatedBy int       `json:"created_by" db:"created_by"`
	UpdatedBy int       `json:"updated_by" db:"updated_by"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}
