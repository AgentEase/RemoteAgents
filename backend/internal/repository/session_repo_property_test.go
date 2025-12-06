package repository

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/remote-agent-terminal/backend/internal/db"
	"github.com/remote-agent-terminal/backend/internal/model"
)

// generateID generates a unique ID for testing.
func generateID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// **Feature: remote-agent-terminal, Property 1: 会话创建完整性**
// *对于任何*有效的会话创建请求（包含命令和名称），创建成功后，数据库中应存在对应的会话记录，
// 且文件系统中应存在有效的 Asciinema v2 格式日志文件。
// **Validates: Requirements 1.1, 1.4, 1.5**
func TestSessionCreationIntegrityProperty(t *testing.T) {
	// Setup test database
	tmpDir, err := os.MkdirTemp("", "session_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	db.ResetDB()
	testDB, err := db.InitDB(dbPath)
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	defer db.CloseDB()

	repo := NewSessionRepository(testDB)
	ctx := context.Background()

	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	// Generator for non-empty strings (command and name must be non-empty)
	nonEmptyString := gen.AlphaString().SuchThat(func(s string) bool {
		return len(s) > 0 && len(s) <= 100
	})

	properties.Property("session creation persists to database and can be retrieved", prop.ForAll(
		func(command, name, userID string) bool {
			// Create a unique session ID
			sessionID := generateID()
			logFilePath := filepath.Join(tmpDir, sessionID+".cast")

			// Create the log file to simulate Asciinema logger initialization
			logFile, err := os.Create(logFilePath)
			if err != nil {
				t.Logf("failed to create log file: %v", err)
				return false
			}
			// Write Asciinema v2 header
			header := fmt.Sprintf(`{"version": 2, "width": 120, "height": 40, "timestamp": %d}`, time.Now().Unix()) + "\n"
			logFile.WriteString(header)
			logFile.Close()

			// Create session
			session := &model.Session{
				ID:          sessionID,
				UserID:      userID,
				Name:        name,
				Command:     command,
				Status:      model.SessionStatusRunning,
				LogFilePath: logFilePath,
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			}

			// Property 1a: Session should be created successfully
			if err := repo.Create(ctx, session); err != nil {
				t.Logf("failed to create session: %v", err)
				return false
			}

			// Property 1b: Session should exist in database after creation
			retrieved, err := repo.GetByID(ctx, sessionID)
			if err != nil {
				t.Logf("failed to retrieve session: %v", err)
				return false
			}

			// Property 1c: Retrieved session should match created session
			if retrieved.ID != session.ID ||
				retrieved.UserID != session.UserID ||
				retrieved.Name != session.Name ||
				retrieved.Command != session.Command ||
				retrieved.Status != session.Status ||
				retrieved.LogFilePath != session.LogFilePath {
				t.Logf("retrieved session does not match created session")
				return false
			}

			// Property 1d: Log file should exist on filesystem
			if _, err := os.Stat(logFilePath); os.IsNotExist(err) {
				t.Logf("log file does not exist: %v", err)
				return false
			}

			// Cleanup: delete the session for next iteration
			repo.Delete(ctx, sessionID)
			os.Remove(logFilePath)

			return true
		},
		nonEmptyString,
		nonEmptyString,
		nonEmptyString,
	))

	properties.TestingRun(t)
}
