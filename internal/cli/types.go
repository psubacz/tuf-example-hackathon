package cli

import (
	"time"
)

// ServerStatus represents the server status response
type ServerStatus struct {
	Status       string                 `json:"status"`
	Version      string                 `json:"version"`
	Repository   string                 `json:"repository"`
	Uptime       string                 `json:"uptime"`
	Cache        map[string]interface{} `json:"cache,omitempty"`
	Metrics      map[string]interface{} `json:"metrics,omitempty"`
}

// ConnectivityResult represents connectivity check results
type ConnectivityResult struct {
	ServerURL    string    `json:"server_url"`
	Connected    bool      `json:"connected"`
	HealthStatus int       `json:"health_status"`
	ReadyStatus  int       `json:"ready_status"`
	AdminAccess  bool      `json:"admin_access"`
	Timestamp    time.Time `json:"timestamp"`
	Error        string    `json:"error,omitempty"`
}

// UploadResult represents file upload result
type UploadResult struct {
	LocalPath  string `json:"local_path"`
	RemotePath string `json:"remote_path"`
	Size       int64  `json:"size"`
	Success    bool   `json:"success"`
	StatusCode int    `json:"status_code"`
	Error      string `json:"error,omitempty"`
}

// DeleteResult represents file deletion result
type DeleteResult struct {
	Path       string `json:"path"`
	Success    bool   `json:"success"`
	StatusCode int    `json:"status_code"`
	Error      string `json:"error,omitempty"`
}

// VerifyResult represents file verification result
type VerifyResult struct {
	Path             string `json:"path"`
	Exists           bool   `json:"exists"`
	Size             int64  `json:"size"`
	Checksum         string `json:"checksum"`
	InMetadata       bool   `json:"in_metadata"`
	MetadataChecksum string `json:"metadata_checksum,omitempty"`
	Valid            bool   `json:"valid"`
}

// LoginResult represents login result
type LoginResult struct {
	Success    bool   `json:"success"`
	Token      string `json:"token,omitempty"`
	ExpiresAt  string `json:"expires_at,omitempty"`
	StatusCode int    `json:"status_code"`
	Error      string `json:"error,omitempty"`
}

// APIKeyResult represents API key generation result
type APIKeyResult struct {
	Success    bool   `json:"success"`
	Name       string `json:"name"`
	APIKey     string `json:"api_key,omitempty"`
	StatusCode int    `json:"status_code"`
	Error      string `json:"error,omitempty"`
}

// KeyRotationResult represents key rotation result
type KeyRotationResult struct {
	KeyType    string `json:"key_type"`
	Success    bool   `json:"success"`
	OldKeyID   string `json:"old_key_id,omitempty"`
	NewKeyID   string `json:"new_key_id,omitempty"`
	StatusCode int    `json:"status_code"`
	Error      string `json:"error,omitempty"`
}

// TargetsMetadata represents targets.json structure
type TargetsMetadata struct {
	Signed SignedTargets `json:"signed"`
}

// SignedTargets represents the signed portion of targets.json
type SignedTargets struct {
	Type    string                       `json:"_type"`
	Version int                          `json:"version"`
	Targets map[string]TargetFileMetadata `json:"targets"`
}

// TargetFileMetadata represents metadata for a target file
type TargetFileMetadata struct {
	Length int64             `json:"length"`
	Hashes map[string]string `json:"hashes"`
}