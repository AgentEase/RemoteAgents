// Package ws provides WebSocket connection handling and message routing.
package ws

import (
	"encoding/json"
	"sync"

	"github.com/gorilla/websocket"
)

// MessageType represents the type of WebSocket message.
type MessageType string

const (
	// Client -> Server message types
	MessageTypeStdin   MessageType = "stdin"
	MessageTypeCommand MessageType = "command" // For complete commands from Chat view
	MessageTypeResize  MessageType = "resize"
	MessageTypePing    MessageType = "ping"

	// Server -> Client message types
	MessageTypeStdout       MessageType = "stdout"
	MessageTypeSmartEvent   MessageType = "smart_event"
	MessageTypeStatus       MessageType = "status"
	MessageTypeHistory      MessageType = "history"
	MessageTypePong         MessageType = "pong"
	MessageTypeError        MessageType = "error"
	MessageTypeConversation MessageType = "conversation"
)

// Message represents a WebSocket message.
type Message struct {
	Type    MessageType     `json:"type"`
	Data    string          `json:"data,omitempty"`
	Rows    uint16          `json:"rows,omitempty"`
	Cols    uint16          `json:"cols,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
	State   string          `json:"state,omitempty"`
	Code    *int            `json:"code,omitempty"`
	Error   string          `json:"error,omitempty"`
}

// Client represents a WebSocket client connection.
type Client struct {
	hub       *Hub
	conn      *websocket.Conn
	sessionID string
	send      chan []byte
	mu        sync.Mutex
	closed    bool
}

// NewClient creates a new WebSocket client.
func NewClient(hub *Hub, conn *websocket.Conn, sessionID string) *Client {
	return &Client{
		hub:       hub,
		conn:      conn,
		sessionID: sessionID,
		send:      make(chan []byte, 256),
	}
}

// Send queues a message to be sent to the client.
func (c *Client) Send(data []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return
	}

	select {
	case c.send <- data:
	default:
		// Buffer full, close the client
		c.closeLocked()
	}
}

// Close closes the client connection.
func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closeLocked()
}

func (c *Client) closeLocked() {
	if c.closed {
		return
	}
	c.closed = true
	close(c.send)
}

// IsClosed returns true if the client is closed.
func (c *Client) IsClosed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closed
}

// SessionID returns the session ID associated with this client.
func (c *Client) SessionID() string {
	return c.sessionID
}

// Conn returns the underlying WebSocket connection.
func (c *Client) Conn() *websocket.Conn {
	return c.conn
}

// SendChan returns the send channel for the client.
func (c *Client) SendChan() <-chan []byte {
	return c.send
}

// Hub manages WebSocket client connections for a session.
type Hub struct {
	sessionID string
	clients   map[*Client]bool
	mu        sync.RWMutex

	// Callbacks
	onMessage func(client *Client, msg *Message)
	onClose   func()
}

// NewHub creates a new Hub for the given session.
func NewHub(sessionID string) *Hub {
	return &Hub{
		sessionID: sessionID,
		clients:   make(map[*Client]bool),
	}
}

// SessionID returns the session ID for this hub.
func (h *Hub) SessionID() string {
	return h.sessionID
}

// SetOnMessage sets the callback for incoming messages.
func (h *Hub) SetOnMessage(callback func(client *Client, msg *Message)) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.onMessage = callback
}

// SetOnClose sets the callback for when all clients disconnect.
func (h *Hub) SetOnClose(callback func()) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.onClose = callback
}

// Register adds a client to the hub.
func (h *Hub) Register(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[client] = true
}

// Unregister removes a client from the hub.
func (h *Hub) Unregister(client *Client) {
	h.mu.Lock()
	h.clients[client] = false
	delete(h.clients, client)
	clientCount := len(h.clients)
	onClose := h.onClose
	h.mu.Unlock()

	client.Close()

	// Call onClose callback if no clients remain
	if clientCount == 0 && onClose != nil {
		onClose()
	}
}

// Broadcast sends a message to all connected clients.
func (h *Hub) Broadcast(data []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for client := range h.clients {
		client.Send(data)
	}
}

// BroadcastMessage sends a Message to all connected clients.
func (h *Hub) BroadcastMessage(msg *Message) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	h.Broadcast(data)
	return nil
}

// ClientCount returns the number of connected clients.
func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// HasClients returns true if there are connected clients.
func (h *Hub) HasClients() bool {
	return h.ClientCount() > 0
}

// HandleMessage processes an incoming message from a client.
func (h *Hub) HandleMessage(client *Client, msg *Message) {
	h.mu.RLock()
	callback := h.onMessage
	h.mu.RUnlock()

	if callback != nil {
		callback(client, msg)
	}
}

// Close closes all client connections and the hub.
func (h *Hub) Close() {
	h.mu.Lock()
	clients := make([]*Client, 0, len(h.clients))
	for client := range h.clients {
		clients = append(clients, client)
	}
	h.clients = make(map[*Client]bool)
	h.mu.Unlock()

	for _, client := range clients {
		client.Close()
	}
}

// HubManager manages multiple hubs for different sessions.
type HubManager struct {
	hubs map[string]*Hub
	mu   sync.RWMutex
}

// NewHubManager creates a new HubManager.
func NewHubManager() *HubManager {
	return &HubManager{
		hubs: make(map[string]*Hub),
	}
}

// GetOrCreate returns an existing hub or creates a new one for the session.
func (m *HubManager) GetOrCreate(sessionID string) *Hub {
	m.mu.Lock()
	defer m.mu.Unlock()

	if hub, ok := m.hubs[sessionID]; ok {
		return hub
	}

	hub := NewHub(sessionID)
	m.hubs[sessionID] = hub
	return hub
}

// Get returns the hub for the session, or nil if not found.
func (m *HubManager) Get(sessionID string) *Hub {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.hubs[sessionID]
}

// Remove removes the hub for the session.
func (m *HubManager) Remove(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if hub, ok := m.hubs[sessionID]; ok {
		hub.Close()
		delete(m.hubs, sessionID)
	}
}

// Close closes all hubs.
func (m *HubManager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, hub := range m.hubs {
		hub.Close()
	}
	m.hubs = make(map[string]*Hub)
}
