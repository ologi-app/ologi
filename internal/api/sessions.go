package api

import (
	"encoding/json"
	"time"
)

// PostSessionInput is what the engine sends after each completed dictation.
type PostSessionInput struct {
	Mode       string    `json:"mode"`
	StartedAt  time.Time `json:"started_at"`
	EndedAt    time.Time `json:"ended_at"`
	DurationMs int64     `json:"duration_ms"`
	SourceApp  *string   `json:"source_app,omitempty"`
	Text       string    `json:"text"`
}

// PostSessionResponse is the server's reply: the canonical post-replacements
// text + the current settings_version (so the CLI can decide whether to
// re-pull /config).
type PostSessionResponse struct {
	SessionID       string `json:"session_id"`
	CanonicalText   string `json:"canonical_text"`
	SettingsVersion int    `json:"settings_version"`
}

func (c *Client) PostSession(in PostSessionInput) (PostSessionResponse, error) {
	var out PostSessionResponse
	data, err := c.do("POST", "/api/voice/sessions", in)
	if err != nil {
		return out, err
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return out, err
	}
	return out, nil
}
