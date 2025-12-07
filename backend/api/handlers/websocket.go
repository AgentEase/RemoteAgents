// Package handlers provides HTTP API request handlers.
package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/remote-agent-terminal/backend/internal/model"
	"github.com/remote-agent-terminal/backend/internal/session"
	"github.com/remote-agent-terminal/backend/internal/ws"
)

// WebSocketHandler handles WebSocket connections for terminal sessions.
type WebSocketHandler struct {
	sessionManager *session.Manager
	wsHandler      *ws.Handler
}

// NewWebSocketHandler creates a new WebSocketHandler.
func NewWebSocketHandler(sessionManager *session.Manager, wsHandler *ws.Handler) *WebSocketHandler {
	return &WebSocketHandler{
		sessionManager: sessionManager,
		wsHandler:      wsHandler,
	}
}

// Attach handles WS /api/sessions/:id/attach - attaches to a session via WebSocket.
// Requirements: 3.1
func (h *WebSocketHandler) Attach(c *gin.Context) {
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

	// Check if session is running
	if sess.Status != model.SessionStatusRunning {
		sendError(c, http.StatusBadRequest, "SESSION_NOT_RUNNING", "Session is not running")
		return
	}

	// Get session context to retrieve the driver
	sessionCtx, exists := h.sessionManager.GetContext(sessionID)
	if exists && sessionCtx.Driver != nil {
		// Set session-specific driver for smart event parsing
		h.wsHandler.SetSessionDriver(sessionID, sessionCtx.Driver)
	}

	// Handle WebSocket connection
	if err := h.wsHandler.HandleConnection(c.Writer, c.Request, sessionID); err != nil {
		// Error already handled by WebSocket handler
		return
	}
}

// RegisterRoutes registers the WebSocket handler routes on a Gin router group.
func (h *WebSocketHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/sessions/:id/attach", h.Attach)
}
