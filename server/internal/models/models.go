package models

import "time"

type User struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type Device struct {
	ID            string    `json:"id"`
	UserID        string    `json:"user_id"`
	OwnerName     string    `json:"owner_name,omitempty"`
	DeviceName    string    `json:"device_name"`
	MachineName   string    `json:"machine_name"`
	MACAddress    string    `json:"mac_address,omitempty"`
	Platform      string    `json:"platform"`
	AppVersion    string    `json:"app_version"`
	Status        string    `json:"status"`
	DeviceKeyHash string    `json:"-"`
	LastSeenAt    time.Time `json:"last_seen_at"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type SessionRequest struct {
	ID                string     `json:"id"`
	RequesterDeviceID string     `json:"requester_device_id"`
	TargetDeviceID    string     `json:"target_device_id"`
	Status            string     `json:"status"`
	CreatedAt         time.Time  `json:"created_at"`
	RespondedAt       *time.Time `json:"responded_at,omitempty"`
}

type RemoteSession struct {
	ID                string     `json:"id"`
	RequesterDeviceID string     `json:"requester_device_id"`
	TargetDeviceID    string     `json:"target_device_id"`
	SessionToken      string     `json:"session_token"`
	Status            string     `json:"status"`
	StartedAt         *time.Time `json:"started_at,omitempty"`
	EndedAt           *time.Time `json:"ended_at,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
}

type FileTransfer struct {
	ID                 string    `json:"id"`
	SessionID          string    `json:"session_id"`
	Filename           string    `json:"filename"`
	SizeBytes          int64     `json:"size_bytes"`
	Status             string    `json:"status"`
	StoragePath        string    `json:"-"`
	UploadedByDeviceID string    `json:"uploaded_by_device_id"`
	TargetDeviceID     string    `json:"target_device_id"`
	CreatedAt          time.Time `json:"created_at"`
}

type AuditLog struct {
	ID           string    `json:"id"`
	UserID       string    `json:"user_id"`
	DeviceID     string    `json:"device_id"`
	Action       string    `json:"action"`
	MetadataJSON string    `json:"metadata_json"`
	CreatedAt    time.Time `json:"created_at"`
}
