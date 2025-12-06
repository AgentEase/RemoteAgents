package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/remote-agent-terminal/backend/internal/model"
)

// SessionRepository provides data access for sessions.
type SessionRepository struct {
	db *sql.DB
}

// NewSessionRepository creates a new SessionRepository.
func NewSessionRepository(db *sql.DB) *SessionRepository {
	return &SessionRepository{db: db}
}

// Create inserts a new session into the database.
func (r *SessionRepository) Create(ctx context.Context, session *model.Session) error {
	envJSON, err := session.EnvToJSON()
	if err != nil {
		return fmt.Errorf("failed to serialize env: %w", err)
	}

	query := `
		INSERT INTO sessions (id, user_id, name, command, env, status, pid, log_file_path, preview_line, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err = r.db.ExecContext(ctx, query,
		session.ID,
		session.UserID,
		session.Name,
		session.Command,
		envJSON,
		session.Status,
		session.PID,
		session.LogFilePath,
		session.PreviewLine,
		session.CreatedAt,
		session.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}

	return nil
}


// GetByID retrieves a session by its ID.
func (r *SessionRepository) GetByID(ctx context.Context, id string) (*model.Session, error) {
	query := `
		SELECT id, user_id, name, command, env, status, exit_code, pid, log_file_path, preview_line, created_at, updated_at
		FROM sessions
		WHERE id = ?
	`

	session := &model.Session{}
	var envJSON sql.NullString
	var exitCode sql.NullInt64
	var pid sql.NullInt64
	var previewLine sql.NullString

	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&session.ID,
		&session.UserID,
		&session.Name,
		&session.Command,
		&envJSON,
		&session.Status,
		&exitCode,
		&pid,
		&session.LogFilePath,
		&previewLine,
		&session.CreatedAt,
		&session.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, model.ErrSessionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	if envJSON.Valid {
		if err := session.EnvFromJSON(envJSON.String); err != nil {
			return nil, fmt.Errorf("failed to parse env: %w", err)
		}
	}

	if exitCode.Valid {
		code := int(exitCode.Int64)
		session.ExitCode = &code
	}

	if pid.Valid {
		p := int(pid.Int64)
		session.PID = &p
	}

	if previewLine.Valid {
		session.PreviewLine = previewLine.String
	}

	return session, nil
}

// List retrieves all sessions for a user.
func (r *SessionRepository) List(ctx context.Context, userID string) ([]*model.Session, error) {
	query := `
		SELECT id, user_id, name, command, env, status, exit_code, pid, log_file_path, preview_line, created_at, updated_at
		FROM sessions
		WHERE user_id = ?
		ORDER BY created_at DESC
	`

	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list sessions: %w", err)
	}
	defer rows.Close()

	var sessions []*model.Session
	for rows.Next() {
		session := &model.Session{}
		var envJSON sql.NullString
		var exitCode sql.NullInt64
		var pid sql.NullInt64
		var previewLine sql.NullString

		err := rows.Scan(
			&session.ID,
			&session.UserID,
			&session.Name,
			&session.Command,
			&envJSON,
			&session.Status,
			&exitCode,
			&pid,
			&session.LogFilePath,
			&previewLine,
			&session.CreatedAt,
			&session.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan session: %w", err)
		}

		if envJSON.Valid {
			if err := session.EnvFromJSON(envJSON.String); err != nil {
				return nil, fmt.Errorf("failed to parse env: %w", err)
			}
		}

		if exitCode.Valid {
			code := int(exitCode.Int64)
			session.ExitCode = &code
		}

		if pid.Valid {
			p := int(pid.Int64)
			session.PID = &p
		}

		if previewLine.Valid {
			session.PreviewLine = previewLine.String
		}

		sessions = append(sessions, session)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating sessions: %w", err)
	}

	return sessions, nil
}


// Delete removes a session from the database.
func (r *SessionRepository) Delete(ctx context.Context, id string) error {
	query := `DELETE FROM sessions WHERE id = ?`

	result, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return model.ErrSessionNotFound
	}

	return nil
}

// UpdateStatus updates the status of a session.
func (r *SessionRepository) UpdateStatus(ctx context.Context, id string, status model.SessionStatus, exitCode *int) error {
	query := `
		UPDATE sessions
		SET status = ?, exit_code = ?, updated_at = ?
		WHERE id = ?
	`

	result, err := r.db.ExecContext(ctx, query, status, exitCode, time.Now(), id)
	if err != nil {
		return fmt.Errorf("failed to update session status: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return model.ErrSessionNotFound
	}

	return nil
}

// UpdatePreviewLine updates the preview line of a session.
func (r *SessionRepository) UpdatePreviewLine(ctx context.Context, id string, previewLine string) error {
	query := `
		UPDATE sessions
		SET preview_line = ?, updated_at = ?
		WHERE id = ?
	`

	_, err := r.db.ExecContext(ctx, query, previewLine, time.Now(), id)
	if err != nil {
		return fmt.Errorf("failed to update preview line: %w", err)
	}

	return nil
}

// CountActiveByUser returns the number of active sessions for a user.
func (r *SessionRepository) CountActiveByUser(ctx context.Context, userID string) (int, error) {
	query := `
		SELECT COUNT(*)
		FROM sessions
		WHERE user_id = ? AND status = ?
	`

	var count int
	err := r.db.QueryRowContext(ctx, query, userID, model.SessionStatusRunning).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count active sessions: %w", err)
	}

	return count, nil
}

// Exists checks if a session exists.
func (r *SessionRepository) Exists(ctx context.Context, id string) (bool, error) {
	query := `SELECT 1 FROM sessions WHERE id = ? LIMIT 1`

	var exists int
	err := r.db.QueryRowContext(ctx, query, id).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to check session existence: %w", err)
	}

	return true, nil
}
