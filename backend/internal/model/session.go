package model

import (
	"encoding/json"
	"time"
)

// SessionStatus represents the status of a terminal session.
type SessionStatus string

const (
	SessionStatusRunning SessionStatus = "running"
	SessionStatusExited  SessionStatus = "exited"
	SessionStatusFailed  SessionStatus = "failed"
)

// Session represents a terminal session in the system.
type Session struct {
	ID          string            `json:"id"`
	UserID      string            `json:"userId"`
	Name        string            `json:"name"`
	Command     string            `json:"command"`
	Env         map[string]string `json:"env,omitempty"`
	Status      SessionStatus     `json:"status"`
	ExitCode    *int              `json:"exitCode,omitempty"`
	PID         *int              `json:"pid,omitempty"`
	LogFilePath string            `json:"logFilePath"`
	PreviewLine string            `json:"previewLine,omitempty"`
	CreatedAt   time.Time         `json:"createdAt"`
	UpdatedAt   time.Time         `json:"updatedAt"`
}

// EnvToJSON converts the Env map to a JSON string for storage.
func (s *Session) EnvToJSON() (string, error) {
	if s.Env == nil {
		return "", nil
	}
	data, err := json.Marshal(s.Env)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// EnvFromJSON parses a JSON string into the Env map.
func (s *Session) EnvFromJSON(data string) error {
	if data == "" {
		s.Env = nil
		return nil
	}
	return json.Unmarshal([]byte(data), &s.Env)
}


// Duration returns the running duration of the session.
func (s *Session) Duration() time.Duration {
	return time.Since(s.CreatedAt)
}

// CreateSessionRequest represents a request to create a new session.
type CreateSessionRequest struct {
	Command string            `json:"command" binding:"required"`
	Name    string            `json:"name"`
	Env     map[string]string `json:"env"`
	UserID  string            `json:"-"`
}

// Validate validates the create session request.
func (r *CreateSessionRequest) Validate() error {
	if r.Command == "" {
		return ErrCommandRequired
	}
	return nil
}
