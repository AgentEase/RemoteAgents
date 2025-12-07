package db

import (
	"database/sql"
	"fmt"
	"sync"

	_ "github.com/mattn/go-sqlite3"
)

var (
	db   *sql.DB
	once sync.Once
)

// InitDB initializes the SQLite database connection and runs schema migrations.
func InitDB(dbPath string) (*sql.DB, error) {
	var initErr error
	once.Do(func() {
		var err error
		db, err = sql.Open("sqlite3", dbPath)
		if err != nil {
			initErr = fmt.Errorf("failed to open database: %w", err)
			return
		}

		// Enable WAL mode for better concurrent access
		if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
			initErr = fmt.Errorf("failed to enable WAL mode: %w", err)
			return
		}

		// Enable foreign keys
		if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
			initErr = fmt.Errorf("failed to enable foreign keys: %w", err)
			return
		}

		// Run schema migrations
		if err := runMigrations(db); err != nil {
			initErr = fmt.Errorf("failed to run migrations: %w", err)
			return
		}
	})

	if initErr != nil {
		return nil, initErr
	}
	return db, nil
}

// GetDB returns the initialized database connection.
func GetDB() *sql.DB {
	return db
}


// runMigrations executes the database schema migrations.
func runMigrations(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS sessions (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		name TEXT NOT NULL,
		command TEXT NOT NULL,
		env TEXT,
		status TEXT NOT NULL DEFAULT 'running',
		exit_code INTEGER,
		pid INTEGER,
		log_file_path TEXT NOT NULL,
		preview_line TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id);
	CREATE INDEX IF NOT EXISTS idx_sessions_status ON sessions(status);
	`

	if _, err := db.Exec(schema); err != nil {
		return fmt.Errorf("failed to create schema: %w", err)
	}

	return nil
}

// CloseDB closes the database connection.
func CloseDB() error {
	if db != nil {
		return db.Close()
	}
	return nil
}

// ResetDB resets the singleton for testing purposes.
func ResetDB() {
	if db != nil {
		db.Close()
	}
	once = sync.Once{}
	db = nil
}

// NewTestDB creates a new in-memory database for testing.
// This bypasses the singleton pattern and creates a fresh database each time.
func NewTestDB() (*sql.DB, error) {
	testDB, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		return nil, fmt.Errorf("failed to open test database: %w", err)
	}

	// Run schema migrations
	if err := runMigrations(testDB); err != nil {
		testDB.Close()
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	return testDB, nil
}
