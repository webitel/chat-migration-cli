package old

import (
	"encoding/json"
	"fmt"
)

// BaseConfig holds fields common to every gateway configuration.
// client_id and client_secret identify the app on the platform.
// The instagram_* booleans control which Instagram events are subscribed to.
type BaseConfig struct {
	ClientID          string `json:"client_id"`
	ClientSecret      string `json:"client_secret"`
	InstagramComments bool   `json:"instagram_comments,string"`
	InstagramMentions bool   `json:"instagram_mentions,string"`
}

// FacebookConfig is used when only Facebook Messenger (and optionally
// Instagram DM) tokens are present — no WhatsApp integration.
type FacebookConfig struct {
	BaseConfig
	// FB holds the encoded Facebook page access token (protobuf-base64).
	FB string `json:"fb"`
	// IG holds the encoded Instagram account token (protobuf-base64).
	// Empty when Instagram is not connected.
	IG string `json:"ig,omitempty"`
	// Version is the Graph API version override, e.g. "v23.0".
	// Defaults to the platform default when absent.
	Version string `json:"version,omitempty"`
}

// FullGatewayConfig extends FacebookConfig with WhatsApp Cloud API support.
// This is the most complete gateway type and is used by production accounts
// that operate across all three platforms simultaneously.
type FullGatewayConfig struct {
	FacebookConfig
	// WA holds the encoded WhatsApp Business Account token (protobuf-base64).
	WA string `json:"wa,omitempty"`
	// WhatsAppToken is the long-lived WhatsApp Cloud API bearer token.
	WhatsAppToken string `json:"whatsapp_token,omitempty"`
	// InstagramStoryMentions enables story mention webhook subscriptions.
	InstagramStoryMentions bool `json:"instagram_story_mentions,string,omitempty"`
}

// GatewayType enumerates the three configuration shapes found in the data.
type GatewayType string

const (
	GatewayTypeMinimal  GatewayType = "minimal"  // client credentials only, no tokens
	GatewayTypeFacebook GatewayType = "facebook" // FB (+ optional IG) tokens
	GatewayTypeFull     GatewayType = "full"     // FB + IG + WhatsApp tokens
)

// GatewayConfig is a discriminated-union wrapper that holds exactly one of
// the three config variants. Detect the type before calling the typed getter.
type GatewayConfig struct {
	Type GatewayType

	minimal  *BaseConfig
	facebook *FacebookConfig
	full     *FullGatewayConfig
}

// Minimal returns the base config for minimal-type gateways.
// Panics if Type != GatewayTypeMinimal.
func (g *GatewayConfig) Minimal() *BaseConfig {
	if g.Type != GatewayTypeMinimal {
		panic("GatewayConfig is not of type minimal")
	}
	return g.minimal
}

// Facebook returns the Facebook config.
// Panics if Type != GatewayTypeFacebook.
func (g *GatewayConfig) Facebook() *FacebookConfig {
	if g.Type != GatewayTypeFacebook {
		panic("GatewayConfig is not of type facebook")
	}
	return g.facebook
}

// Full returns the full gateway config.
// Panics if Type != GatewayTypeFull.
func (g *GatewayConfig) Full() *FullGatewayConfig {
	if g.Type != GatewayTypeFull {
		panic("GatewayConfig is not of type full")
	}
	return g.full
}

// UnmarshalJSON detects the gateway type from the raw JSON and unmarshals
// into the appropriate struct. Detection logic:
//   - has "wa" or "whatsapp_token" field  → FullGatewayConfig
//   - has "fb" field                       → FacebookConfig
//   - otherwise                            → BaseConfig (minimal)
func (g *GatewayConfig) UnmarshalJSON(data []byte) error {
	// Peek at the raw fields.
	var probe struct {
		FB            *json.RawMessage `json:"fb"`
		WA            *json.RawMessage `json:"wa"`
		WhatsAppToken *json.RawMessage `json:"whatsapp_token"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return fmt.Errorf("gateway config probe: %w", err)
	}

	switch {
	case probe.WA != nil || (probe.WhatsAppToken != nil && isNonEmptyString(probe.WhatsAppToken)):
		g.Type = GatewayTypeFull
		g.full = new(FullGatewayConfig)
		return json.Unmarshal(data, g.full)

	case probe.FB != nil:
		g.Type = GatewayTypeFacebook
		g.facebook = new(FacebookConfig)
		return json.Unmarshal(data, g.facebook)

	default:
		g.Type = GatewayTypeMinimal
		g.minimal = new(BaseConfig)
		return json.Unmarshal(data, g.minimal)
	}
}

// isNonEmptyString returns true when msg is a non-null, non-empty JSON string.
func isNonEmptyString(msg *json.RawMessage) bool {
	if msg == nil {
		return false
	}
	var s string
	if err := json.Unmarshal(*msg, &s); err != nil {
		return false
	}
	return s != ""
}
