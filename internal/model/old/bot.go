package old

import (
	"time"

	"github.com/webitel/chat-migration-cli/internal/model/old/proto"
)

type Bot struct {
	IDs       []int     `db:"ids"`
	DC        int       `db:"dc"`
	Name      string    `db:"name"`
	FlowID    int       `db:"flow_id"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}

type Provider[T any] struct {
	ID        int       `db:"id"`
	DC        int       `db:"dc"`
	URI       string    `db:"uri"`
	Name      string    `db:"name"`
	FlowID    int       `db:"flow_id"`
	Enabled   bool      `db:"enabled"`
	Metadata  *T        `db:"metadata"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
	Updates   []byte    `db:"updates"`
}

type FBProviderMetadata struct {
	FB           *proto.Messenger `json:"fb"`
	ClientID     string           `json:"client_id"`
	ClientSecret string           `json:"client_secret"`

	IG                     *proto.Messenger `json:"ig"`
	InstagramComments      bool             `json:"instagram_comments"`
	InstagramMentions      bool             `json:"instagram_mentions"`
	InstagramStoryMentions bool             `json:"instagram_story_mentions"`

	WA            string `json:"wa"`
	WhatsAppToken string `json:"whatsapp_token"`

	Version string `json:"version"`
}
