package api

import "encoding/json"

// ConfigResponse mirrors GET /api/voice/config's JSON body.
type ConfigResponse struct {
	SettingsVersion int                `json:"settings_version"`
	Hotkey          string             `json:"hotkey"`
	Language        string             `json:"language"`
	MicDevice       *string            `json:"mic_device"`
	StartSound      string             `json:"start_sound"`
	StopSound       string             `json:"stop_sound"`
	Replacements    []ReplacementEntry `json:"replacements"`
}

// ReplacementEntry is one row of the user's personal dictionary.
type ReplacementEntry struct {
	Pattern     string `json:"pattern"`
	Replacement string `json:"replacement"`
}

func (c *Client) GetConfig() (ConfigResponse, error) {
	var out ConfigResponse
	data, err := c.do("GET", "/api/voice/config", nil)
	if err != nil {
		return out, err
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return out, err
	}
	return out, nil
}
