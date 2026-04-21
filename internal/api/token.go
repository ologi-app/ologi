package api

import "encoding/json"

func (c *Client) MintRealtimeToken() (string, error) {
	data, err := c.do("POST", "/api/voice/realtime-token", struct{}{})
	if err != nil {
		return "", err
	}
	var out struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return "", err
	}
	if out.Token == "" {
		return "", &APIError{StatusCode: 500, Message: "empty token in response"}
	}
	return out.Token, nil
}
