package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// LoginStartResponse mirrors POST /api/voice/login/start.
type LoginStartResponse struct {
	DeviceCode      string `json:"device_code"`
	VerificationURL string `json:"verification_url"`
	IntervalMs      int    `json:"interval_ms"`
}

// LoginStart begins the device-code flow. Auth: none required.
func (c *Client) LoginStart(deviceName string) (LoginStartResponse, error) {
	var out LoginStartResponse
	body := map[string]string{
		"device_name": deviceName,
		"platform":    c.Platform,
		"cli_version": c.Version,
	}
	data, err := c.do("POST", "/api/voice/login/start", body)
	if err != nil {
		return out, err
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return out, err
	}
	return out, nil
}

// LoginPollResponse is the decoded body of /api/voice/login/complete.
// Status is one of "pending", "denied", "expired", or "ok".
// On "ok", APIKey and DeviceID are populated.
type LoginPollResponse struct {
	Status   string `json:"status"`
	APIKey   string `json:"api_key,omitempty"`
	DeviceID string `json:"device_id,omitempty"`
}

// LoginPoll polls /api/voice/login/complete once. The server returns
// 200 for pending/ok and 410 for denied/expired — both carry a
// decodable {"status":...} body the CLI needs to switch on. We
// therefore bypass the base do() wrapper (which would surface 410 as
// an APIError) and do the HTTP round-trip directly.
func (c *Client) LoginPoll(deviceCode string) (LoginPollResponse, error) {
	var out LoginPollResponse
	body, _ := json.Marshal(map[string]string{"device_code": deviceCode})
	req, err := http.NewRequest("POST", c.BaseURL+"/api/voice/login/complete", bytes.NewReader(body))
	if err != nil {
		return out, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", fmt.Sprintf("ologi/%s (darwin)", c.Version))

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return out, fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 && resp.StatusCode != 410 {
		data, _ := io.ReadAll(resp.Body)
		return out, &APIError{StatusCode: resp.StatusCode, Message: string(data)}
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return out, err
	}
	return out, nil
}

// DeleteDevice revokes the api_key tied to a device. Auth: required.
func (c *Client) DeleteDevice(deviceID string) error {
	_, err := c.do("DELETE", "/api/voice/devices/"+deviceID, nil)
	return err
}
