package ws

import "encoding/json"

type IncomingMessage struct {
	Type         string          `json:"type"`
	SessionID    string          `json:"session_id,omitempty"`
	SessionToken string          `json:"session_token,omitempty"`
	Kind         string          `json:"kind,omitempty"`
	Payload      json.RawMessage `json:"payload,omitempty"`
	ToDeviceID   string          `json:"to_device_id,omitempty"`
}

type OutgoingMessage struct {
	Type       string `json:"type"`
	FromDevice string `json:"from_device_id,omitempty"`
	SessionID  string `json:"session_id,omitempty"`
	Kind       string `json:"kind,omitempty"`
	Payload    any    `json:"payload,omitempty"`
}
