package new

import (
	"time"

	"github.com/google/uuid"
)

type Gate struct {
	ID        uuid.UUID `db:"id"`
	DC        int64     `db:"dc"`
	Name      string    `db:"name"`
	Type      string    `db:"type"`
	Enabled   bool      `db:"enabled"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`

	FacebookPage    *Facebook
	WhatsAppAccount *GateWABA
	Bot             *Bot
}

type MetaApp struct {
	ID          uuid.UUID `db:"id"           json:"id"`
	Name        string    `db:"name"         json:"name"`
	AppID       string    `db:"app_id"       json:"app_id"`
	AppSecret   string    `db:"app_secret"   json:"app_secret"`
	RedirectURI string    `db:"redirect_uri" json:"redirect_uri"`
	URI         string    `db:"uri"          json:"uri"`
	Scopes      []string  `db:"scopes"       json:"scopes"`
	CreatedAt   time.Time `db:"created_at"   json:"created_at"`
	UpdatedAt   time.Time `db:"updated_at"   json:"updated_at"`
	VerifyToken string    `db:"verify_token" json:"verify_token"`

	DomainID int
}

type GateWABA struct {
	ID                   uuid.UUID  `db:"id"                      json:"id"`
	MetaAppID            uuid.UUID  `db:"meta_app_id"             json:"meta_app_id"`
	PhoneNumber          string     `db:"phone_number"            json:"phone_number"`
	PhoneNumberID        string     `db:"phone_number_id"         json:"phone_number_id"`
	AccessToken          []byte     `db:"access_token"            json:"access_token"`
	AccessTokenExpiresAt *time.Time `db:"access_token_expires_at" json:"access_token_expires_at,omitempty"`
	BusinessID           string     `db:"business_id"             json:"business_id"`
}

type Facebook struct {
	GateID    uuid.UUID `db:"gate_id"     json:"gate_id"`
	MetaAppID uuid.UUID `db:"meta_app_id" json:"meta_app_id"`
	PageID    string    `db:"page_id"     json:"page_id"`
	PageToken string    `db:"page_token"  json:"page_token"`
}

type Bot struct {
	ID        uuid.UUID `db:"id"         json:"id"`
	Sub       string    `db:"sub"        json:"sub"`
	Iss       string    `db:"iss"        json:"iss"`
	GateID    uuid.UUID `db:"gate_id"    json:"gate_id"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}
