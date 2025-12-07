package session

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/remote-agent-terminal/backend/internal/db"
	"github.com/remote-agent-terminal/backend/internal/model"
	"github.com/remote-agent-terminal/backend/internal/pty"
	"github.com/remote-agent-terminal/backend/internal/repository"
)

func setupTestManager(t *testing.T) (*Manager, func()) {
	// Create temp directory for logs
	tempDir, err := os.MkdirTemp("", "session-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	// Create a fresh test database (bypasses singleton)
	database, err := db.NewTestDB()
	if err != nil {
		os.RemoveAll(tempDir)
		t.Fatalf("Failed to create database: %v", err)
	}

	// Create repository
	repo := repository.NewSessionRepository(database)

	// Create PTY manager
	ptyManager := pty.NewManager(tempDir)

	// Create session manager
	manager := NewManager(ptyManager, repo, Config{
		LogDir:             tempDir,
		MaxSessionsPerUser: 5,
	})

	cleanup := func() {
		manager.Close()
		database.Close()
		os.RemoveAll(tempDir)
	}

	return manager, cleanup
}

func TestManager_Create(t *testing.T) {
	manager, cleanup := setupTestManager(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("create session successfully", func(t *testing.T) {
		req := &model.CreateSessionRequest{
			Command: "/usr/bin/echo hello",
			Name:    "Test Session",
			UserID:  "user1",
		}

		session, err := manager.Create(ctx, req)
		if err != nil {
			t.Fatalf("Failed to create session: %v", err)
		}

		if session.ID == "" {
			t.Error("Session ID should not be empty")
		}

		if session.Name != "Test Session" {
			t.Errorf("Expected name 'Test Session', got '%s'", session.Name)
		}

		if session.Command != "/usr/bin/echo hello" {
			t.Errorf("Expected command '/usr/bin/echo hello', got '%s'", session.Command)
		}

		if session.Status != model.SessionStatusRunning {
			t.Errorf("Expected status 'running', got '%s'", session.Status)
		}

		if session.PID == nil {
			t.Error("PID should not be nil")
		}

		if session.LogFilePath == "" {
			t.Error("LogFilePath should not be empty")
		}
	})

	t.Run("create session with default name", func(t *testing.T) {
		req := &model.CreateSessionRequest{
			Command: "/usr/bin/echo test",
			UserID:  "user1",
		}

		session, err := manager.Create(ctx, req)
		if err != nil {
			t.Fatalf("Failed to create session: %v", err)
		}

		if session.Name == "" {
			t.Error("Default name should be generated")
		}
	})

	t.Run("create session with environment variables", func(t *testing.T) {
		req := &model.CreateSessionRequest{
			Command: "/usr/bin/env",
			Name:    "Env Test",
			UserID:  "user1",
			Env: map[string]string{
				"TEST_VAR": "test_value",
			},
		}

		session, err := manager.Create(ctx, req)
		if err != nil {
			t.Fatalf("Failed to create session: %v", err)
		}

		if session.Env == nil {
			t.Error("Env should not be nil")
		}

		if session.Env["TEST_VAR"] != "test_value" {
			t.Error("Environment variable not set correctly")
		}
	})

	t.Run("reject session without command", func(t *testing.T) {
		req := &model.CreateSessionRequest{
			UserID: "user1",
		}

		_, err := manager.Create(ctx, req)
		if err == nil {
			t.Error("Expected error for missing command")
		}
	})

	// Note: concurrent session limit test is in a separate test function
	// to avoid database singleton issues
}

func TestManager_Get(t *testing.T) {
	manager, cleanup := setupTestManager(t)
	defer cleanup()

	ctx := context.Background()

	// Create a session
	req := &model.CreateSessionRequest{
		Command: "/usr/bin/echo test",
		Name:    "Get Test",
		UserID:  "user1",
	}

	created, err := manager.Create(ctx, req)
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	t.Run("get existing session", func(t *testing.T) {
		session, err := manager.Get(ctx, created.ID)
		if err != nil {
			t.Fatalf("Failed to get session: %v", err)
		}

		if session.ID != created.ID {
			t.Errorf("Expected ID '%s', got '%s'", created.ID, session.ID)
		}

		if session.Name != created.Name {
			t.Errorf("Expected name '%s', got '%s'", created.Name, session.Name)
		}
	})

	t.Run("get non-existent session", func(t *testing.T) {
		_, err := manager.Get(ctx, "non-existent-id")
		if err == nil {
			t.Error("Expected error for non-existent session")
		}
	})
}

func TestManager_GetContext(t *testing.T) {
	manager, cleanup := setupTestManager(t)
	defer cleanup()

	ctx := context.Background()

	// Create a session
	req := &model.CreateSessionRequest{
		Command: "/usr/bin/echo test",
		UserID:  "user1",
	}

	created, err := manager.Create(ctx, req)
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	t.Run("get context for existing session", func(t *testing.T) {
		sessionCtx, exists := manager.GetContext(created.ID)
		if !exists {
			t.Error("Session context should exist")
		}

		if sessionCtx.Session.ID != created.ID {
			t.Error("Session ID mismatch")
		}

		if sessionCtx.PTYProcess == nil {
			t.Error("PTY process should not be nil")
		}

		if sessionCtx.Driver == nil {
			t.Error("Driver should not be nil")
		}
	})

	t.Run("get context for non-existent session", func(t *testing.T) {
		_, exists := manager.GetContext("non-existent-id")
		if exists {
			t.Error("Session context should not exist")
		}
	})
}

func TestManager_List(t *testing.T) {
	manager, cleanup := setupTestManager(t)
	defer cleanup()

	ctx := context.Background()

	// Create multiple sessions for different users
	// Use sleep to keep processes running during test
	for i := 0; i < 3; i++ {
		req := &model.CreateSessionRequest{
			Command: "/usr/bin/sleep 30",
			UserID:  "user1",
		}
		_, err := manager.Create(ctx, req)
		if err != nil {
			t.Fatalf("Failed to create session: %v", err)
		}
	}

	for i := 0; i < 2; i++ {
		req := &model.CreateSessionRequest{
			Command: "/usr/bin/sleep 30",
			UserID:  "user2",
		}
		_, err := manager.Create(ctx, req)
		if err != nil {
			t.Fatalf("Failed to create session: %v", err)
		}
	}

	t.Run("list sessions for user1", func(t *testing.T) {
		sessions, err := manager.List(ctx, "user1")
		if err != nil {
			t.Fatalf("Failed to list sessions: %v", err)
		}

		if len(sessions) != 3 {
			t.Errorf("Expected 3 sessions, got %d", len(sessions))
		}
	})

	t.Run("list sessions for user2", func(t *testing.T) {
		sessions, err := manager.List(ctx, "user2")
		if err != nil {
			t.Fatalf("Failed to list sessions: %v", err)
		}

		if len(sessions) != 2 {
			t.Errorf("Expected 2 sessions, got %d", len(sessions))
		}
	})

	t.Run("list sessions for non-existent user", func(t *testing.T) {
		sessions, err := manager.List(ctx, "user3")
		if err != nil {
			t.Fatalf("Failed to list sessions: %v", err)
		}

		if len(sessions) != 0 {
			t.Errorf("Expected 0 sessions, got %d", len(sessions))
		}
	})
}

func TestManager_Delete(t *testing.T) {
	manager, cleanup := setupTestManager(t)
	defer cleanup()

	ctx := context.Background()

	// Create a session
	req := &model.CreateSessionRequest{
		Command: "/usr/bin/sleep 10",
		UserID:  "user1",
	}

	created, err := manager.Create(ctx, req)
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	t.Run("delete existing session", func(t *testing.T) {
		err := manager.Delete(ctx, created.ID)
		if err != nil {
			t.Fatalf("Failed to delete session: %v", err)
		}

		// Verify session is deleted
		_, err = manager.Get(ctx, created.ID)
		if err == nil {
			t.Error("Session should be deleted")
		}

		// Verify context is removed
		_, exists := manager.GetContext(created.ID)
		if exists {
			t.Error("Session context should be removed")
		}
	})

	t.Run("delete non-existent session", func(t *testing.T) {
		err := manager.Delete(ctx, "non-existent-id")
		if err == nil {
			t.Error("Expected error for non-existent session")
		}
	})
}

func TestManager_CreateDriver(t *testing.T) {
	manager, cleanup := setupTestManager(t)
	defer cleanup()

	tests := []struct {
		name        string
		command     string
		expectType  string
	}{
		{
			name:       "claude command",
			command:    "claude",
			expectType: "claude",
		},
		{
			name:       "claude with path",
			command:    "/usr/bin/claude",
			expectType: "claude",
		},
		{
			name:       "generic command",
			command:    "bash",
			expectType: "generic",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			driver := manager.createDriver(tt.command)
			if driver == nil {
				t.Error("Driver should not be nil")
			}

			driverName := driver.Name()
			if driverName != tt.expectType {
				t.Errorf("Expected driver type '%s', got '%s'", tt.expectType, driverName)
			}
		})
	}
}

func TestManager_ProcessExit(t *testing.T) {
	manager, cleanup := setupTestManager(t)
	defer cleanup()

	ctx := context.Background()

	// Create a session with a command that exits quickly
	req := &model.CreateSessionRequest{
		Command: "/usr/bin/echo test",
		UserID:  "user1",
	}

	created, err := manager.Create(ctx, req)
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// Wait for process to exit
	time.Sleep(2 * time.Second)

	// Check session status
	session, err := manager.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Failed to get session: %v", err)
	}

	if session.Status != model.SessionStatusExited {
		t.Errorf("Expected status 'exited', got '%s'", session.Status)
	}

	if session.ExitCode == nil {
		t.Error("Exit code should not be nil")
	}
}

func TestManager_LogFilePath(t *testing.T) {
	manager, cleanup := setupTestManager(t)
	defer cleanup()

	ctx := context.Background()

	req := &model.CreateSessionRequest{
		Command: "/usr/bin/echo test",
		UserID:  "user1",
	}

	session, err := manager.Create(ctx, req)
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// Check log file path format
	expectedExt := ".cast"
	if filepath.Ext(session.LogFilePath) != expectedExt {
		t.Errorf("Expected log file extension '%s', got '%s'", expectedExt, filepath.Ext(session.LogFilePath))
	}

	// Check if log file is created
	time.Sleep(1 * time.Second)
	if _, err := os.Stat(session.LogFilePath); os.IsNotExist(err) {
		t.Error("Log file should be created")
	}
}

func TestManager_ConcurrentSessionLimit(t *testing.T) {
	manager, cleanup := setupTestManager(t)
	defer cleanup()

	ctx := context.Background()

	// Create maximum allowed sessions (5)
	for i := 0; i < 5; i++ {
		req := &model.CreateSessionRequest{
			Command: "/usr/bin/sleep 10",
			UserID:  "user_limit_test",
		}
		_, err := manager.Create(ctx, req)
		if err != nil {
			t.Fatalf("Failed to create session %d: %v", i, err)
		}
	}

	// Try to create one more - should fail
	req := &model.CreateSessionRequest{
		Command: "/usr/bin/sleep 10",
		UserID:  "user_limit_test",
	}
	_, err := manager.Create(ctx, req)
	if err == nil {
		t.Error("Expected error for exceeding session limit")
	}
}
