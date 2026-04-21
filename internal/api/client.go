// Package api is the Ologi web API client used by the ologi CLI.
// It speaks HTTPS to voice.ologi.app (or an override via OLOGI_SERVER_URL
// or the ServerURL field of the config) using ht_* API keys.
package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// APIError is returned for any non-2xx HTTP response. The Message is
// the decoded `error` field of the JSON body, or the raw body prefix
// if the body wasn't JSON.
type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("api: %d %s", e.StatusCode, e.Message)
}

// IsAuthError returns true if err is an APIError with status 401 or 403.
func IsAuthError(err error) bool {
	if err == nil {
		return false
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		return false
	}
	return apiErr.StatusCode == 401 || apiErr.StatusCode == 403
}

// Client is the Ologi API client. Construct with NewClient.
type Client struct {
	BaseURL string
	APIKey  string
	// Version is the ologi-cli version, sent in the User-Agent.
	// Defaults to "unknown" if unset.
	Version string
	// Platform is reported in login/start. Defaults to "darwin".
	Platform string
	// HTTP allows overriding the HTTP client (e.g. for tests).
	HTTP *http.Client
}

func NewClient(baseURL, apiKey string) *Client {
	return &Client{
		BaseURL:  baseURL,
		APIKey:   apiKey,
		Version:  "unknown",
		Platform: "darwin",
		HTTP: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// do performs a single HTTP round-trip and returns the raw body bytes
// on success. On any non-2xx response, returns an *APIError.
//
// If body is non-nil, it is JSON-encoded and sent as application/json.
func (c *Client) do(method, path string, body any) ([]byte, error) {
	var reqBody io.Reader
	if body != nil {
		enc, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("encode request: %w", err)
		}
		reqBody = bytes.NewReader(enc)
	}

	req, err := http.NewRequest(method, c.BaseURL+path, reqBody)
	if err != nil {
		return nil, err
	}
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}
	req.Header.Set("User-Agent", fmt.Sprintf("ologi/%s (darwin)", c.Version))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return data, nil
	}

	// Try to extract {"error": "..."} from the body.
	var errBody struct {
		Error string `json:"error"`
	}
	msg := ""
	if jsonErr := json.Unmarshal(data, &errBody); jsonErr == nil && errBody.Error != "" {
		msg = errBody.Error
	} else if len(data) > 0 {
		if len(data) > 200 {
			msg = string(data[:200]) + "…"
		} else {
			msg = string(data)
		}
	} else {
		msg = http.StatusText(resp.StatusCode)
	}

	return nil, &APIError{StatusCode: resp.StatusCode, Message: msg}
}
