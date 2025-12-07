package ws

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/remote-agent-terminal/backend/internal/driver"
	"github.com/remote-agent-terminal/backend/internal/pty"
)

const (
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer.
	maxMessageSize = 8192
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// TODO: Implement proper origin checking in production
		return true
	},
}

// Handler handles WebSocket connections for terminal sessions.
type Handler struct {
	hubManager     *HubManager
	ptyManager     *pty.Manager
	driver         driver.AgentDriver // Default driver
	sessionDrivers map[string]driver.AgentDriver // Session-specific drivers
	mu             sync.RWMutex
}

// NewHandler creates a new WebSocket handler.
func NewHandler(hubManager *HubManager, ptyManager *pty.Manager, agentDriver driver.AgentDriver) *Handler {
	if agentDriver == nil {
		agentDriver = driver.NewGenericDriver()
	}
	return &Handler{
		hubManager:     hubManager,
		ptyManager:     ptyManager,
		driver:         agentDriver,
		sessionDrivers: make(map[string]driver.AgentDriver),
	}
}

// SetSessionDriver sets a specific driver for a session.
func (h *Handler) SetSessionDriver(sessionID string, d driver.AgentDriver) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.sessionDrivers[sessionID] = d
}

// GetSessionDriver gets the driver for a session, or returns the default driver.
func (h *Handler) GetSessionDriver(sessionID string) driver.AgentDriver {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if d, ok := h.sessionDrivers[sessionID]; ok {
		return d
	}
	return h.driver
}

// HandleConnection handles a new WebSocket connection for a session.
// It upgrades the HTTP connection to WebSocket and manages the bidirectional communication.
func (h *Handler) HandleConnection(w http.ResponseWriter, r *http.Request, sessionID string) error {
	// Get or verify the PTY process exists
	ptyProcess, ok := h.ptyManager.Get(sessionID)
	if !ok {
		http.Error(w, "Session not found", http.StatusNotFound)
		return nil
	}

	// Upgrade to WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return err
	}

	// Get or create hub for this session
	hub := h.hubManager.GetOrCreate(sessionID)

	// Create client
	client := NewClient(hub, conn, sessionID)

	// Register client with hub
	hub.Register(client)

	// Set up message handler for the hub
	hub.SetOnMessage(func(c *Client, msg *Message) {
		h.handleMessage(c, msg, ptyProcess)
	})

	// Set up output callback to broadcast PTY output to WebSocket clients
	// This is critical for real-time terminal output (Requirement 3.3)
	ptyProcess.OutputCallback = func(data []byte) {
		h.BroadcastOutput(sessionID, data)
	}

	// Send history data for hot restore (Requirement 4.3)
	h.sendHistory(client, ptyProcess)

	// Start read and write pumps
	go h.writePump(client)
	go h.readPump(client, hub)

	return nil
}

// sendHistory sends the buffered history to the client for hot restore.
func (h *Handler) sendHistory(client *Client, ptyProcess *pty.PTYProcess) {
	history := ptyProcess.GetHistory()
	if len(history) == 0 {
		return
	}

	msg := &Message{
		Type: MessageTypeHistory,
		Data: string(history),
	}

	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("Failed to marshal history message: %v", err)
		return
	}

	client.Send(data)
}

// handleMessage processes incoming messages from clients.
func (h *Handler) handleMessage(client *Client, msg *Message, ptyProcess *pty.PTYProcess) {
	switch msg.Type {
	case MessageTypeStdin:
		h.handleStdin(msg, ptyProcess)
	case MessageTypeCommand:
		h.handleCommand(msg, ptyProcess)
	case MessageTypeResize:
		h.handleResize(msg, ptyProcess)
	case MessageTypePing:
		h.handlePing(client)
	}
}

// handleStdin handles stdin input from the client (Terminal view - real-time input).
func (h *Handler) handleStdin(msg *Message, ptyProcess *pty.PTYProcess) {
	if msg.Data == "" {
		return
	}

	// Write directly to PTY without any input clearing
	// This is for real-time terminal input where each keystroke is sent immediately
	err := ptyProcess.Write([]byte(msg.Data))
	if err != nil {
		log.Printf("Failed to write to PTY: %v", err)
	}
}

// handleCommand handles complete command input from the client (Chat view).
func (h *Handler) handleCommand(msg *Message, ptyProcess *pty.PTYProcess) {
	if msg.Data == "" {
		return
	}

	// Write data to PTY using WriteCommand for proper input handling
	// WriteCommand implements the three-step input clearing mechanism:
	// 1. Ctrl+U to clear current input buffer
	// 2. Send command text
	// 3. Send Enter
	// This prevents commands from being appended to existing input in CLI applications like Claude
	err := ptyProcess.WriteCommand([]byte(msg.Data))
	if err != nil {
		log.Printf("Failed to write to PTY: %v", err)
	}
}

// handleResize handles terminal resize events.
func (h *Handler) handleResize(msg *Message, ptyProcess *pty.PTYProcess) {
	if msg.Rows == 0 || msg.Cols == 0 {
		return
	}

	// Resize PTY (Requirement 3.4)
	err := ptyProcess.Resize(msg.Rows, msg.Cols)
	if err != nil {
		log.Printf("Failed to resize PTY: %v", err)
	}
}

// handlePing handles ping messages from the client.
func (h *Handler) handlePing(client *Client) {
	msg := &Message{Type: MessageTypePong}
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}
	client.Send(data)
}

// readPump pumps messages from the WebSocket connection to the hub.
func (h *Handler) readPump(client *Client, hub *Hub) {
	defer func() {
		hub.Unregister(client)
		client.Conn().Close()
	}()

	client.Conn().SetReadLimit(maxMessageSize)
	client.Conn().SetReadDeadline(time.Now().Add(pongWait))
	client.Conn().SetPongHandler(func(string) error {
		client.Conn().SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, message, err := client.Conn().ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			break
		}

		var msg Message
		if err := json.Unmarshal(message, &msg); err != nil {
			log.Printf("Failed to unmarshal message: %v", err)
			continue
		}

		hub.HandleMessage(client, &msg)
	}
}

// writePump pumps messages from the hub to the WebSocket connection.
func (h *Handler) writePump(client *Client) {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		client.Conn().Close()
	}()

	for {
		select {
		case message, ok := <-client.SendChan():
			client.Conn().SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// The hub closed the channel
				client.Conn().WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			// Send each message in a separate WebSocket frame
			// This ensures JSON.parse() works correctly on the frontend
			if err := client.Conn().WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}

			// Process any queued messages, sending each in its own frame
			n := len(client.SendChan())
			for i := 0; i < n; i++ {
				queuedMsg := <-client.SendChan()
				client.Conn().SetWriteDeadline(time.Now().Add(writeWait))
				if err := client.Conn().WriteMessage(websocket.TextMessage, queuedMsg); err != nil {
					return
				}
			}
		case <-ticker.C:
			client.Conn().SetWriteDeadline(time.Now().Add(writeWait))
			if err := client.Conn().WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// BroadcastOutput broadcasts PTY output to all connected clients.
// This should be called from the PTY output callback.
func (h *Handler) BroadcastOutput(sessionID string, data []byte) {
	hub := h.hubManager.Get(sessionID)
	if hub == nil {
		return
	}

	// Get session-specific driver (Requirement 6.1)
	sessionDriver := h.GetSessionDriver(sessionID)

	// Parse output through driver for smart events
	result, err := sessionDriver.Parse(data)
	if err != nil {
		log.Printf("Driver parse error: %v", err)
		result = &driver.ParseResult{RawData: data}
	}

	// Send stdout message (Requirement 3.3, 3.5 - ANSI sequences preserved)
	stdoutMsg := &Message{
		Type: MessageTypeStdout,
		Data: string(result.RawData),
	}
	hub.BroadcastMessage(stdoutMsg)

	// Send smart events if any (Requirement 6.2, 6.5)
	for _, event := range result.SmartEvents {
		payload, err := json.Marshal(event)
		if err != nil {
			continue
		}
		eventMsg := &Message{
			Type:    MessageTypeSmartEvent,
			Payload: payload,
		}
		hub.BroadcastMessage(eventMsg)
	}

	// Send parsed conversation messages if any
	for _, msg := range result.Messages {
		payload, err := json.Marshal(msg)
		if err != nil {
			continue
		}
		conversationMsg := &Message{
			Type:    MessageTypeConversation,
			Payload: payload,
		}
		hub.BroadcastMessage(conversationMsg)
	}
}

// BroadcastStatus broadcasts session status changes to all connected clients.
func (h *Handler) BroadcastStatus(sessionID string, state string, exitCode *int) {
	hub := h.hubManager.Get(sessionID)
	if hub == nil {
		return
	}

	msg := &Message{
		Type:  MessageTypeStatus,
		State: state,
		Code:  exitCode,
	}
	hub.BroadcastMessage(msg)
}

// BroadcastError broadcasts an error message to all connected clients.
func (h *Handler) BroadcastError(sessionID string, errMsg string) {
	hub := h.hubManager.Get(sessionID)
	if hub == nil {
		return
	}

	msg := &Message{
		Type:  MessageTypeError,
		Error: errMsg,
	}
	hub.BroadcastMessage(msg)
}

// GetUpgrader returns the WebSocket upgrader for custom configuration.
func GetUpgrader() *websocket.Upgrader {
	return &upgrader
}

// SetCheckOrigin sets a custom origin checker for the WebSocket upgrader.
func SetCheckOrigin(fn func(r *http.Request) bool) {
	upgrader.CheckOrigin = fn
}
