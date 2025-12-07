package ws

import (
	"context"
	"log"
	"sync"

	"github.com/remote-agent-terminal/backend/internal/driver"
	"github.com/remote-agent-terminal/backend/internal/model"
	"github.com/remote-agent-terminal/backend/internal/pty"
)

// Service manages the integration between WebSocket connections and PTY processes.
// It handles session lifecycle, hot restore, and process keepalive.
type Service struct {
	hubManager *HubManager
	ptyManager *pty.Manager
	handler    *Handler

	// Session status callbacks
	onStatusChange func(sessionID string, status model.SessionStatus, exitCode *int)

	mu sync.RWMutex
}

// NewService creates a new WebSocket service.
func NewService(ptyManager *pty.Manager, agentDriver driver.AgentDriver) *Service {
	hubManager := NewHubManager()
	handler := NewHandler(hubManager, ptyManager, agentDriver)

	return &Service{
		hubManager: hubManager,
		ptyManager: ptyManager,
		handler:    handler,
	}
}

// SetOnStatusChange sets the callback for session status changes.
func (s *Service) SetOnStatusChange(callback func(sessionID string, status model.SessionStatus, exitCode *int)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onStatusChange = callback
}

// Handler returns the WebSocket handler.
func (s *Service) Handler() *Handler {
	return s.handler
}

// HubManager returns the hub manager.
func (s *Service) HubManager() *HubManager {
	return s.hubManager
}

// AttachSession attaches WebSocket handling to a PTY session.
// This sets up the output callback for broadcasting and the exit callback for status updates.
// The PTY process continues running even when no WebSocket clients are connected (Requirement 4.1).
func (s *Service) AttachSession(ctx context.Context, session *model.Session, opts pty.SpawnOptions) (*pty.PTYProcess, error) {
	sessionID := session.ID

	// Set up output callback to broadcast to WebSocket clients
	opts.OutputCallback = func(data []byte) {
		s.handler.BroadcastOutput(sessionID, data)
	}

	// Set up exit callback to update status and notify clients
	opts.ExitCallback = func(exitCode int, err error) {
		s.handleProcessExit(sessionID, exitCode, err)
	}

	// Spawn the PTY process
	ptyProcess, err := s.ptyManager.Spawn(ctx, opts)
	if err != nil {
		return nil, err
	}

	// Create hub for this session (even if no clients yet)
	hub := s.hubManager.GetOrCreate(sessionID)

	// Set up hub close callback - but don't kill the process (Requirement 4.1)
	hub.SetOnClose(func() {
		// Process keeps running when all clients disconnect
		// This is the key to session keepalive
		log.Printf("All clients disconnected from session %s, process continues running", sessionID)
	})

	return ptyProcess, nil
}

// handleProcessExit handles PTY process exit.
func (s *Service) handleProcessExit(sessionID string, exitCode int, err error) {
	var status model.SessionStatus
	var code *int

	if err != nil {
		status = model.SessionStatusFailed
		log.Printf("Session %s failed: %v", sessionID, err)
	} else {
		status = model.SessionStatusExited
		code = &exitCode
		log.Printf("Session %s exited with code %d", sessionID, exitCode)
	}

	// Broadcast status to connected clients
	s.handler.BroadcastStatus(sessionID, string(status), code)

	// Call status change callback
	s.mu.RLock()
	callback := s.onStatusChange
	s.mu.RUnlock()

	if callback != nil {
		callback(sessionID, status, code)
	}
}

// DetachSession removes WebSocket handling from a session.
// This should be called when a session is deleted.
func (s *Service) DetachSession(sessionID string) {
	// Close all WebSocket connections for this session
	s.hubManager.Remove(sessionID)
}

// GetSessionClientCount returns the number of connected clients for a session.
func (s *Service) GetSessionClientCount(sessionID string) int {
	hub := s.hubManager.Get(sessionID)
	if hub == nil {
		return 0
	}
	return hub.ClientCount()
}

// IsSessionConnected returns true if there are WebSocket clients connected to the session.
func (s *Service) IsSessionConnected(sessionID string) bool {
	return s.GetSessionClientCount(sessionID) > 0
}

// Close closes all WebSocket connections and cleans up resources.
func (s *Service) Close() {
	s.hubManager.Close()
}
