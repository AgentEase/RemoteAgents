// Package handlers provides HTTP API request handlers.
package handlers

import (
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/remote-agent-terminal/backend/internal/model"
	"github.com/remote-agent-terminal/backend/internal/session"
)

// SessionHandler handles HTTP requests for session management.
type SessionHandler struct {
	sessionManager *session.Manager
}

// NewSessionHandler creates a new SessionHandler.
func NewSessionHandler(sessionManager *session.Manager) *SessionHandler {
	return &SessionHandler{
		sessionManager: sessionManager,
	}
}

// CreateSessionRequest represents the request body for creating a session.
type CreateSessionRequest struct {
	Command string            `json:"command" binding:"required"`
	Name    string            `json:"name"`
	Workdir string            `json:"workdir"`
	Env     map[string]string `json:"env"`
}

// SessionResponse represents a session in API responses.
type SessionResponse struct {
	ID          string            `json:"id"`
	UserID      string            `json:"userId"`
	Name        string            `json:"name"`
	Command     string            `json:"command"`
	Env         map[string]string `json:"env,omitempty"`
	Status      string            `json:"status"`
	ExitCode    *int              `json:"exitCode,omitempty"`
	PID         *int              `json:"pid,omitempty"`
	LogFilePath string            `json:"logFilePath"`
	PreviewLine string            `json:"previewLine,omitempty"`
	Duration    string            `json:"duration"`
	CreatedAt   string            `json:"createdAt"`
	UpdatedAt   string            `json:"updatedAt"`
}

// ErrorResponse represents an error response.
type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail contains error details.
type ErrorDetail struct {
	Code    string                 `json:"code"`
	Message string                 `json:"message"`
	Details map[string]interface{} `json:"details,omitempty"`
}


// toSessionResponse converts a model.Session to SessionResponse.
func toSessionResponse(s *model.Session) *SessionResponse {
	return &SessionResponse{
		ID:          s.ID,
		UserID:      s.UserID,
		Name:        s.Name,
		Command:     s.Command,
		Env:         s.Env,
		Status:      string(s.Status),
		ExitCode:    s.ExitCode,
		PID:         s.PID,
		LogFilePath: s.LogFilePath,
		PreviewLine: s.PreviewLine,
		Duration:    formatDuration(s.Duration()),
		CreatedAt:   s.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   s.UpdatedAt.Format(time.RFC3339),
	}
}

// formatDuration formats a duration as a human-readable string.
func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second

	if h > 0 {
		return time.Duration(h*time.Hour + m*time.Minute + s*time.Second).String()
	}
	if m > 0 {
		return time.Duration(m*time.Minute + s*time.Second).String()
	}
	return time.Duration(s * time.Second).String()
}

// getUserID extracts the user ID from the request context.
// In a real implementation, this would come from authentication middleware.
func getUserID(c *gin.Context) string {
	// Try to get from context (set by auth middleware)
	if userID, exists := c.Get("userID"); exists {
		if id, ok := userID.(string); ok {
			return id
		}
	}
	// Default user for development/testing
	return "default-user"
}

// sendError sends an error response with the appropriate status code.
func sendError(c *gin.Context, statusCode int, code, message string) {
	c.JSON(statusCode, ErrorResponse{
		Error: ErrorDetail{
			Code:    code,
			Message: message,
		},
	})
}

// Create handles POST /api/sessions - creates a new session.
// Requirements: 1.1, 1.4, 1.5
func (h *SessionHandler) Create(c *gin.Context) {
	var req CreateSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		sendError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request body: "+err.Error())
		return
	}

	userID := getUserID(c)

	// Create session request
	createReq := &model.CreateSessionRequest{
		Command: req.Command,
		Name:    req.Name,
		Workdir: req.Workdir,
		Env:     req.Env,
		UserID:  userID,
	}

	// Create session
	sess, err := h.sessionManager.Create(c.Request.Context(), createReq)
	if err != nil {
		if errors.Is(err, model.ErrCommandRequired) {
			sendError(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
			return
		}
		// Check for concurrency limit error
		if err.Error() == "maximum active sessions ("+string(rune(h.sessionManager.GetMaxSessionsPerUser()+'0'))+") reached for user" ||
			containsString(err.Error(), "maximum active sessions") {
			sendError(c, http.StatusTooManyRequests, "LIMIT_EXCEEDED", err.Error())
			return
		}
		sendError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to create session: "+err.Error())
		return
	}

	c.JSON(http.StatusCreated, toSessionResponse(sess))
}

// containsString checks if a string contains a substring.
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && findSubstr(s, substr)
}

func findSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}


// List handles GET /api/sessions - lists all sessions for the user.
// Requirements: 2.1
func (h *SessionHandler) List(c *gin.Context) {
	userID := getUserID(c)

	sessions, err := h.sessionManager.List(c.Request.Context(), userID)
	if err != nil {
		sendError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list sessions: "+err.Error())
		return
	}

	// Convert to response format and verify status based on actual process state
	response := make([]*SessionResponse, len(sessions))
	for i, sess := range sessions {
		// Verify if the session is actually running
		// If the database says it's running but the process is not, correct the status
		if sess.Status == model.SessionStatusRunning {
			isRunning := h.sessionManager.IsSessionRunning(sess.ID)
			if !isRunning {
				// Process has exited but database wasn't updated yet
				// Return the correct status (exited) to the client
				sess.Status = model.SessionStatusExited
				// Note: We don't update the database here to avoid race conditions
				// The handleProcessExit callback should handle database updates
			}
		}
		response[i] = toSessionResponse(sess)
	}

	c.JSON(http.StatusOK, response)
}

// Get handles GET /api/sessions/:id - gets a specific session.
// Requirements: 2.2
func (h *SessionHandler) Get(c *gin.Context) {
	sessionID := c.Param("id")
	if sessionID == "" {
		sendError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Session ID is required")
		return
	}

	sess, err := h.sessionManager.Get(c.Request.Context(), sessionID)
	if err != nil {
		if errors.Is(err, model.ErrSessionNotFound) {
			sendError(c, http.StatusNotFound, "SESSION_NOT_FOUND", "Session "+sessionID+" not found")
			return
		}
		sendError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get session: "+err.Error())
		return
	}

	// Check ownership
	userID := getUserID(c)
	if sess.UserID != userID {
		sendError(c, http.StatusForbidden, "FORBIDDEN", "Access to session denied")
		return
	}

	// Verify if the session is actually running
	// If the database says it's running but the process is not, correct the status
	if sess.Status == model.SessionStatusRunning {
		isRunning := h.sessionManager.IsSessionRunning(sess.ID)
		log.Printf("Get session %s: DB status=%s, IsRunning=%v", sess.ID, sess.Status, isRunning)
		if !isRunning {
			// Process has exited but database wasn't updated yet
			// Return the correct status (exited) to the client
			log.Printf("Correcting status to exited for session %s", sess.ID)
			sess.Status = model.SessionStatusExited
			// Note: We don't update the database here to avoid race conditions
			// The handleProcessExit callback should handle database updates
		}
	}

	c.JSON(http.StatusOK, toSessionResponse(sess))
}

// Delete handles DELETE /api/sessions/:id - deletes a session.
// Requirements: 2.3
func (h *SessionHandler) Delete(c *gin.Context) {
	sessionID := c.Param("id")
	if sessionID == "" {
		sendError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Session ID is required")
		return
	}

	// First check if session exists and belongs to user
	sess, err := h.sessionManager.Get(c.Request.Context(), sessionID)
	if err != nil {
		if errors.Is(err, model.ErrSessionNotFound) {
			sendError(c, http.StatusNotFound, "SESSION_NOT_FOUND", "Session "+sessionID+" not found")
			return
		}
		sendError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get session: "+err.Error())
		return
	}

	// Check ownership
	userID := getUserID(c)
	if sess.UserID != userID {
		sendError(c, http.StatusForbidden, "FORBIDDEN", "Access to session denied")
		return
	}

	// Delete session
	if err := h.sessionManager.Delete(c.Request.Context(), sessionID); err != nil {
		if errors.Is(err, model.ErrSessionNotFound) {
			sendError(c, http.StatusNotFound, "SESSION_NOT_FOUND", "Session "+sessionID+" not found")
			return
		}
		sendError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to delete session: "+err.Error())
		return
	}

	c.Status(http.StatusNoContent)
}

// Restart handles POST /api/sessions/:id/restart - restarts an exited session.
func (h *SessionHandler) Restart(c *gin.Context) {
	sessionID := c.Param("id")
	if sessionID == "" {
		sendError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Session ID is required")
		return
	}

	// Get the session
	sess, err := h.sessionManager.Get(c.Request.Context(), sessionID)
	if err != nil {
		if errors.Is(err, model.ErrSessionNotFound) {
			sendError(c, http.StatusNotFound, "SESSION_NOT_FOUND", "Session "+sessionID+" not found")
			return
		}
		sendError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get session: "+err.Error())
		return
	}

	// Check ownership
	userID := getUserID(c)
	if sess.UserID != userID {
		sendError(c, http.StatusForbidden, "FORBIDDEN", "Access to session denied")
		return
	}

	// Check if session is actually running by checking the PTY process
	// Don't rely on database status as it might be stale
	if h.sessionManager.IsSessionRunning(sessionID) {
		sendError(c, http.StatusBadRequest, "INVALID_STATE", "Session is already running")
		return
	}

	// Restart the session
	restartedSess, err := h.sessionManager.Restart(c.Request.Context(), sessionID)
	if err != nil {
		sendError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to restart session: "+err.Error())
		return
	}

	c.JSON(http.StatusOK, toSessionResponse(restartedSess))
}

// RegisterRoutes registers the session handler routes on a Gin router group.
func (h *SessionHandler) RegisterRoutes(rg *gin.RouterGroup) {
	sessions := rg.Group("/sessions")
	{
		sessions.POST("", h.Create)
		sessions.GET("", h.List)
		sessions.GET("/:id", h.Get)
		sessions.DELETE("/:id", h.Delete)
		sessions.POST("/:id/restart", h.Restart)
	}
}


// GetLogs handles GET /api/sessions/:id/logs - downloads session logs.
// Requirements: 5.4
func (h *SessionHandler) GetLogs(c *gin.Context) {
	sessionID := c.Param("id")
	if sessionID == "" {
		sendError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Session ID is required")
		return
	}

	// Get session to verify existence and ownership
	sess, err := h.sessionManager.Get(c.Request.Context(), sessionID)
	if err != nil {
		if errors.Is(err, model.ErrSessionNotFound) {
			sendError(c, http.StatusNotFound, "SESSION_NOT_FOUND", "Session "+sessionID+" not found")
			return
		}
		sendError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get session: "+err.Error())
		return
	}

	// Check ownership
	userID := getUserID(c)
	if sess.UserID != userID {
		sendError(c, http.StatusForbidden, "FORBIDDEN", "Access to session denied")
		return
	}

	// Check if log file exists
	if sess.LogFilePath == "" {
		sendError(c, http.StatusNotFound, "LOG_NOT_FOUND", "Log file not found for session "+sessionID)
		return
	}

	// Set headers for file download
	c.Header("Content-Type", "application/x-asciicast")
	c.Header("Content-Disposition", "attachment; filename="+sessionID+".cast")

	// Stream the file
	c.File(sess.LogFilePath)
}

// RegisterLogsRoute registers the logs download route.
func (h *SessionHandler) RegisterLogsRoute(rg *gin.RouterGroup) {
	rg.GET("/sessions/:id/logs", h.GetLogs)
}
