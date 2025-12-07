package session

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/remote-agent-terminal/backend/internal/model"
	"github.com/remote-agent-terminal/backend/internal/pty"
	"github.com/remote-agent-terminal/backend/internal/repository"
	"github.com/remote-agent-terminal/backend/pkg/driver"
)

// Manager manages terminal sessions.
type Manager struct {
	ptyManager *pty.Manager
	repo       *repository.SessionRepository
	logDir     string

	// Configuration
	maxSessionsPerUser int

	mu       sync.RWMutex
	sessions map[string]*SessionContext
}

// SessionContext holds the runtime context for a session.
type SessionContext struct {
	Session    *model.Session
	PTYProcess *pty.PTYProcess
	Driver     driver.AgentDriver
}

// Config holds configuration for the session manager.
type Config struct {
	LogDir             string
	MaxSessionsPerUser int
}

// NewManager creates a new session manager.
func NewManager(ptyManager *pty.Manager, repo *repository.SessionRepository, config Config) *Manager {
	if config.MaxSessionsPerUser == 0 {
		config.MaxSessionsPerUser = 10 // Default limit
	}

	return &Manager{
		ptyManager:         ptyManager,
		repo:               repo,
		logDir:             config.LogDir,
		maxSessionsPerUser: config.MaxSessionsPerUser,
		sessions:           make(map[string]*SessionContext),
	}
}

// Create creates a new terminal session.
func (m *Manager) Create(ctx context.Context, req *model.CreateSessionRequest) (*model.Session, error) {
	// Validate request
	if err := req.Validate(); err != nil {
		return nil, err
	}

	// Check concurrent session limit
	activeCount, err := m.repo.CountActiveByUser(ctx, req.UserID)
	if err != nil {
		return nil, fmt.Errorf("failed to count active sessions: %w", err)
	}

	if activeCount >= m.maxSessionsPerUser {
		return nil, fmt.Errorf("maximum active sessions (%d) reached for user", m.maxSessionsPerUser)
	}

	// Generate session ID
	sessionID := uuid.New().String()

	// Generate log file path
	logFilePath := filepath.Join(m.logDir, fmt.Sprintf("%s.cast", sessionID))

	// Create session model
	now := time.Now()
	session := &model.Session{
		ID:          sessionID,
		UserID:      req.UserID,
		Name:        req.Name,
		Command:     req.Command,
		Workdir:     req.Workdir,
		Env:         req.Env,
		Status:      model.SessionStatusRunning,
		LogFilePath: logFilePath,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	// Set default name if not provided
	if session.Name == "" {
		session.Name = fmt.Sprintf("Session %s", sessionID[:8])
	}

	// Persist to database
	if err := m.repo.Create(ctx, session); err != nil {
		return nil, fmt.Errorf("failed to persist session: %w", err)
	}

	// Spawn PTY process
	ptyProcess, err := m.ptyManager.Spawn(ctx, pty.SpawnOptions{
		Session:     session,
		InitialRows: 24,
		InitialCols: 80,
		OutputCallback: func(data []byte) {
			// Output callback will be used by WebSocket hub
			// For now, we just need to ensure the process is spawned
		},
		ExitCallback: func(exitCode int, err error) {
			// Handle process exit
			m.handleProcessExit(sessionID, exitCode, err)
		},
	})
	if err != nil {
		// Rollback: delete from database
		m.repo.Delete(ctx, sessionID)
		return nil, fmt.Errorf("failed to spawn PTY: %w", err)
	}

	// Update session with PID
	pid := ptyProcess.PID()
	session.PID = &pid

	// Create driver based on command
	agentDriver := m.createDriver(req.Command)

	// Store session context
	m.mu.Lock()
	m.sessions[sessionID] = &SessionContext{
		Session:    session,
		PTYProcess: ptyProcess,
		Driver:     agentDriver,
	}
	m.mu.Unlock()

	return session, nil
}

// Get retrieves a session by ID.
func (m *Manager) Get(ctx context.Context, id string) (*model.Session, error) {
	// Try to get from memory first
	m.mu.RLock()
	sessionCtx, exists := m.sessions[id]
	m.mu.RUnlock()

	if exists {
		return sessionCtx.Session, nil
	}

	// Fall back to database
	session, err := m.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	return session, nil
}

// GetContext retrieves the session context (including PTY and Driver).
func (m *Manager) GetContext(id string) (*SessionContext, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ctx, exists := m.sessions[id]
	return ctx, exists
}

// List retrieves all sessions for a user.
func (m *Manager) List(ctx context.Context, userID string) ([]*model.Session, error) {
	return m.repo.List(ctx, userID)
}

// Delete terminates and removes a session.
func (m *Manager) Delete(ctx context.Context, id string) error {
	// Get session context
	m.mu.Lock()
	sessionCtx, exists := m.sessions[id]
	if exists {
		delete(m.sessions, id)
	}
	m.mu.Unlock()

	// Kill PTY process if running
	if exists && sessionCtx.PTYProcess != nil {
		if err := sessionCtx.PTYProcess.Close(); err != nil {
			// Log error but continue with deletion
			fmt.Printf("Error closing PTY process: %v\n", err)
		}
	}

	// Delete from database
	if err := m.repo.Delete(ctx, id); err != nil {
		return err
	}

	return nil
}

// handleProcessExit handles PTY process exit events.
func (m *Manager) handleProcessExit(sessionID string, exitCode int, err error) {
	ctx := context.Background()

	// Determine status
	status := model.SessionStatusExited
	if err != nil {
		status = model.SessionStatusFailed
	}

	// Update database
	if updateErr := m.repo.UpdateStatus(ctx, sessionID, status, &exitCode); updateErr != nil {
		fmt.Printf("Failed to update session status: %v\n", updateErr)
	}

	// Update in-memory session
	m.mu.Lock()
	if sessionCtx, exists := m.sessions[sessionID]; exists {
		sessionCtx.Session.Status = status
		sessionCtx.Session.ExitCode = &exitCode
		sessionCtx.Session.UpdatedAt = time.Now()
	}
	m.mu.Unlock()
}

// createDriver creates an appropriate driver based on the command.
func (m *Manager) createDriver(command string) driver.AgentDriver {
	// Check if command contains "claude"
	if contains(command, "claude") {
		return driver.NewClaudeDriver()
	}

	// Default to generic driver
	return driver.NewGenericDriver()
}

// contains checks if a string contains a substring (case-insensitive).
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && 
		(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || 
		findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// GetActiveCount returns the number of active sessions for a user.
func (m *Manager) GetActiveCount(ctx context.Context, userID string) (int, error) {
	return m.repo.CountActiveByUser(ctx, userID)
}

// GetMaxSessionsPerUser returns the maximum allowed sessions per user.
func (m *Manager) GetMaxSessionsPerUser() int {
	return m.maxSessionsPerUser
}

// IsSessionRunning checks if a session is currently running.
func (m *Manager) IsSessionRunning(id string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	sessionCtx, exists := m.sessions[id]
	if !exists {
		return false
	}

	// Check if PTY process actually exists and is not closed
	// Don't rely on Session.Status as it might not be updated yet
	return sessionCtx.PTYProcess != nil && !sessionCtx.PTYProcess.IsClosed()
}

// Restart restarts an exited session with the same configuration.
func (m *Manager) Restart(ctx context.Context, id string) (*model.Session, error) {
	// Get the existing session
	sess, err := m.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	// Check if session is actually running by checking the PTY process
	// Don't rely on database status as it might be stale
	if m.IsSessionRunning(id) {
		return nil, fmt.Errorf("session is already running")
	}

	// If database says running but process is not, update database status
	if sess.Status == model.SessionStatusRunning {
		// Process has exited but database wasn't updated
		// Update it now before restarting
		if err := m.repo.UpdateStatus(ctx, id, model.SessionStatusExited, nil); err != nil {
			return nil, fmt.Errorf("failed to update session status: %w", err)
		}
		sess.Status = model.SessionStatusExited
	}

	// For Claude sessions, add --resume flag if not already present
	command := sess.Command
	if contains(command, "claude") && !contains(command, "--resume") {
		command = "claude --resume"
	}

	// Update session status to running
	sess.Status = model.SessionStatusRunning
	sess.ExitCode = nil
	sess.UpdatedAt = time.Now()

	// Update in database
	if err := m.repo.UpdateStatus(ctx, id, model.SessionStatusRunning, nil); err != nil {
		return nil, fmt.Errorf("failed to update session status: %w", err)
	}

	// Create new PTY process with the same configuration
	ptyProcess, err := m.ptyManager.Spawn(ctx, pty.SpawnOptions{
		Session:      sess,
		InitialRows:  24,
		InitialCols:  80,
		OutputCallback: func(data []byte) {
			// Output callback will be set by WebSocket service
		},
		ExitCallback: func(exitCode int, err error) {
			m.handleProcessExit(id, exitCode, err)
		},
	})
	if err != nil {
		// Revert status on failure
		m.repo.UpdateStatus(ctx, id, model.SessionStatusExited, sess.ExitCode)
		return nil, fmt.Errorf("failed to spawn PTY: %w", err)
	}

	// Update session context
	m.mu.Lock()
	if sessionCtx, exists := m.sessions[id]; exists {
		sessionCtx.Session = sess
		sessionCtx.PTYProcess = ptyProcess
		sessionCtx.Driver = m.createDriver(command)
	} else {
		// Create new session context if it doesn't exist
		m.sessions[id] = &SessionContext{
			Session:    sess,
			PTYProcess: ptyProcess,
			Driver:     m.createDriver(command),
		}
	}
	m.mu.Unlock()

	return sess, nil
}

// Write writes data to a session's PTY.
func (m *Manager) Write(id string, data []byte) error {
	m.mu.RLock()
	sessionCtx, exists := m.sessions[id]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("session not found: %s", id)
	}

	if sessionCtx.PTYProcess == nil {
		return fmt.Errorf("session has no PTY process: %s", id)
	}

	return sessionCtx.PTYProcess.Write(data)
}

// WriteCommand writes a command to a session's PTY with proper input clearing.
func (m *Manager) WriteCommand(id string, command []byte) error {
	m.mu.RLock()
	sessionCtx, exists := m.sessions[id]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("session not found: %s", id)
	}

	if sessionCtx.PTYProcess == nil {
		return fmt.Errorf("session has no PTY process: %s", id)
	}

	return sessionCtx.PTYProcess.WriteCommand(command)
}

// Resize resizes a session's PTY window.
func (m *Manager) Resize(id string, rows, cols uint16) error {
	m.mu.RLock()
	sessionCtx, exists := m.sessions[id]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("session not found: %s", id)
	}

	if sessionCtx.PTYProcess == nil {
		return fmt.Errorf("session has no PTY process: %s", id)
	}

	return sessionCtx.PTYProcess.Resize(rows, cols)
}

// GetHistory returns the buffered output history for a session.
func (m *Manager) GetHistory(id string) ([]byte, error) {
	m.mu.RLock()
	sessionCtx, exists := m.sessions[id]
	m.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("session not found: %s", id)
	}

	if sessionCtx.PTYProcess == nil {
		return nil, fmt.Errorf("session has no PTY process: %s", id)
	}

	return sessionCtx.PTYProcess.GetHistory(), nil
}

// SetOutputCallback sets the output callback for a session.
// This is used by WebSocket to receive PTY output.
func (m *Manager) SetOutputCallback(id string, callback func(data []byte)) error {
	m.mu.RLock()
	sessionCtx, exists := m.sessions[id]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("session not found: %s", id)
	}

	if sessionCtx.PTYProcess == nil {
		return fmt.Errorf("session has no PTY process: %s", id)
	}

	sessionCtx.PTYProcess.OutputCallback = callback
	return nil
}

// Close closes all sessions and releases resources.
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var firstErr error
	for id, sessionCtx := range m.sessions {
		if sessionCtx.PTYProcess != nil {
			if err := sessionCtx.PTYProcess.Close(); err != nil && firstErr == nil {
				firstErr = err
			}
		}
		delete(m.sessions, id)
	}

	return firstErr
}


