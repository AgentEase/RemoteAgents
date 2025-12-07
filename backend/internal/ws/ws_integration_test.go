package ws

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/remote-agent-terminal/backend/internal/driver"
	"github.com/remote-agent-terminal/backend/internal/model"
	"github.com/remote-agent-terminal/backend/internal/pty"
)

// TestHubClientManagement tests Hub client registration and broadcast
func TestHubClientManagement(t *testing.T) {
	hub := NewHub("test-session-1")
	defer hub.Close()

	// Create mock clients
	client1 := NewClient(hub, nil, "test-session-1")
	client2 := NewClient(hub, nil, "test-session-1")

	hub.Register(client1)
	hub.Register(client2)

	if hub.ClientCount() != 2 {
		t.Errorf("expected 2 clients, got %d", hub.ClientCount())
	}

	// Test broadcast
	testData := []byte("test broadcast message")
	hub.Broadcast(testData)

	// Verify both clients received the message
	received1 := receiveWithTimeoutTest(t, client1, 100*time.Millisecond)
	received2 := receiveWithTimeoutTest(t, client2, 100*time.Millisecond)

	if string(received1) != string(testData) {
		t.Errorf("client1 received wrong data: %s", received1)
	}
	if string(received2) != string(testData) {
		t.Errorf("client2 received wrong data: %s", received2)
	}

	// Test unregister
	hub.Unregister(client1)
	if hub.ClientCount() != 1 {
		t.Errorf("expected 1 client after unregister, got %d", hub.ClientCount())
	}
}

// TestMessageSerialization tests WebSocket message JSON handling
func TestMessageSerialization(t *testing.T) {
	// Test stdin message
	stdinMsg := Message{
		Type: MessageTypeStdin,
		Data: "ls -la\n",
	}

	data, err := json.Marshal(stdinMsg)
	if err != nil {
		t.Fatalf("failed to marshal stdin message: %v", err)
	}

	var parsed Message
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal stdin message: %v", err)
	}

	if parsed.Type != MessageTypeStdin || parsed.Data != stdinMsg.Data {
		t.Errorf("stdin message mismatch: got type=%s data=%s", parsed.Type, parsed.Data)
	}

	// Test resize message
	resizeMsg := Message{
		Type: MessageTypeResize,
		Rows: 40,
		Cols: 120,
	}

	data, err = json.Marshal(resizeMsg)
	if err != nil {
		t.Fatalf("failed to marshal resize message: %v", err)
	}

	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal resize message: %v", err)
	}

	if parsed.Type != MessageTypeResize || parsed.Rows != 40 || parsed.Cols != 120 {
		t.Errorf("resize message mismatch: got type=%s rows=%d cols=%d", parsed.Type, parsed.Rows, parsed.Cols)
	}

	// Test status message with exit code
	exitCode := 0
	statusMsg := Message{
		Type:  MessageTypeStatus,
		State: "exited",
		Code:  &exitCode,
	}

	data, err = json.Marshal(statusMsg)
	if err != nil {
		t.Fatalf("failed to marshal status message: %v", err)
	}

	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal status message: %v", err)
	}

	if parsed.Type != MessageTypeStatus || parsed.State != "exited" || *parsed.Code != 0 {
		t.Errorf("status message mismatch")
	}
}

// TestANSIPassthrough tests that ANSI sequences are preserved
func TestANSIPassthrough(t *testing.T) {
	ansiSequences := []string{
		"\x1b[31mRed Text\x1b[0m",
		"\x1b[1;32mBold Green\x1b[0m",
		"\x1b[H\x1b[2J", // Clear screen
		"\x1b[?25h",     // Show cursor
		"\x1b[38;5;196mExtended Color\x1b[0m",
	}

	for _, seq := range ansiSequences {
		msg := Message{
			Type: MessageTypeStdout,
			Data: seq,
		}

		data, err := json.Marshal(msg)
		if err != nil {
			t.Fatalf("failed to marshal ANSI message: %v", err)
		}

		var parsed Message
		if err := json.Unmarshal(data, &parsed); err != nil {
			t.Fatalf("failed to unmarshal ANSI message: %v", err)
		}

		if parsed.Data != seq {
			t.Errorf("ANSI sequence not preserved: expected %q, got %q", seq, parsed.Data)
		}
	}
}

// TestPTYSessionIntegration tests PTY spawn with output callback
func TestPTYSessionIntegration(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "ws_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	ptyManager := pty.NewManager(tempDir)
	defer ptyManager.Close()

	sessionID := "test-pty-session"
	logPath := filepath.Join(tempDir, sessionID+".cast")

	session := &model.Session{
		ID:          sessionID,
		UserID:      "test-user",
		Name:        "Test PTY Session",
		Command:     "echo hello",
		Status:      model.SessionStatusRunning,
		LogFilePath: logPath,
	}

	var outputReceived []byte
	var outputMu sync.Mutex
	exitCh := make(chan int, 1)

	opts := pty.SpawnOptions{
		Session:     session,
		InitialRows: 24,
		InitialCols: 80,
		OutputCallback: func(data []byte) {
			outputMu.Lock()
			outputReceived = append(outputReceived, data...)
			outputMu.Unlock()
		},
		ExitCallback: func(exitCode int, err error) {
			exitCh <- exitCode
		},
	}

	_, err = ptyManager.Spawn(context.Background(), opts)
	if err != nil {
		t.Fatalf("failed to spawn PTY: %v", err)
	}

	// Wait for process to exit
	select {
	case code := <-exitCh:
		if code != 0 {
			t.Errorf("process exited with code %d", code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for process to exit")
	}

	// Check output
	outputMu.Lock()
	output := string(outputReceived)
	outputMu.Unlock()

	if !strings.Contains(output, "hello") {
		t.Errorf("expected output to contain 'hello', got: %s", output)
	}
}

// TestHotRestoreHistory tests Ring Buffer history for hot restore
func TestHotRestoreHistory(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "ws_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	ptyManager := pty.NewManager(tempDir)
	defer ptyManager.Close()

	sessionID := "test-history-session"
	logPath := filepath.Join(tempDir, sessionID+".cast")

	session := &model.Session{
		ID:          sessionID,
		UserID:      "test-user",
		Name:        "Test History Session",
		Command:     "echo 'line1' && echo 'line2' && echo 'line3'",
		Status:      model.SessionStatusRunning,
		LogFilePath: logPath,
	}

	exitCh := make(chan int, 1)

	opts := pty.SpawnOptions{
		Session:     session,
		InitialRows: 24,
		InitialCols: 80,
		ExitCallback: func(exitCode int, err error) {
			exitCh <- exitCode
		},
	}

	ptyProcess, err := ptyManager.Spawn(context.Background(), opts)
	if err != nil {
		t.Fatalf("failed to spawn PTY: %v", err)
	}

	// Wait for process to exit
	select {
	case <-exitCh:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for process to exit")
	}

	// Get history from Ring Buffer
	history := ptyProcess.GetHistory()
	historyStr := string(history)

	// Verify history contains expected output
	if !strings.Contains(historyStr, "line1") ||
		!strings.Contains(historyStr, "line2") ||
		!strings.Contains(historyStr, "line3") {
		t.Errorf("history missing expected content: %s", historyStr)
	}
}

// TestBidirectionalCommunication tests stdin/stdout through PTY
func TestBidirectionalCommunication(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "ws_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	ptyManager := pty.NewManager(tempDir)
	defer ptyManager.Close()

	sessionID := "test-bidir-session"
	logPath := filepath.Join(tempDir, sessionID+".cast")

	session := &model.Session{
		ID:          sessionID,
		UserID:      "test-user",
		Name:        "Test Bidirectional Session",
		Command:     "cat",
		Status:      model.SessionStatusRunning,
		LogFilePath: logPath,
	}

	var outputReceived []byte
	var outputMu sync.Mutex

	opts := pty.SpawnOptions{
		Session:     session,
		InitialRows: 24,
		InitialCols: 80,
		OutputCallback: func(data []byte) {
			outputMu.Lock()
			outputReceived = append(outputReceived, data...)
			outputMu.Unlock()
		},
	}

	ptyProcess, err := ptyManager.Spawn(context.Background(), opts)
	if err != nil {
		t.Fatalf("failed to spawn PTY: %v", err)
	}
	defer ptyProcess.Close()

	// Send input through PTY
	testInput := "hello world\n"
	if err := ptyProcess.Write([]byte(testInput)); err != nil {
		t.Fatalf("failed to write to PTY: %v", err)
	}

	// Wait for output
	time.Sleep(500 * time.Millisecond)

	outputMu.Lock()
	output := string(outputReceived)
	outputMu.Unlock()

	// cat should echo back the input
	if !strings.Contains(output, "hello world") {
		t.Errorf("expected output to contain 'hello world', got: %s", output)
	}
}

// TestSessionKeepalive tests that Hub persists after client disconnect
func TestSessionKeepalive(t *testing.T) {
	hub := NewHub("keepalive-session")

	// Track if onClose was called
	onCloseCalled := false
	hub.SetOnClose(func() {
		onCloseCalled = true
	})

	// Register and unregister a client
	client := NewClient(hub, nil, "keepalive-session")
	hub.Register(client)

	if hub.ClientCount() != 1 {
		t.Errorf("expected 1 client, got %d", hub.ClientCount())
	}

	hub.Unregister(client)

	if hub.ClientCount() != 0 {
		t.Errorf("expected 0 clients after unregister, got %d", hub.ClientCount())
	}

	// onClose should have been called
	if !onCloseCalled {
		t.Error("onClose callback was not called")
	}
}

// TestMultipleClientsBroadcast tests broadcast to multiple clients
func TestMultipleClientsBroadcast(t *testing.T) {
	hub := NewHub("multi-client-session")
	defer hub.Close()

	numClients := 5
	clients := make([]*Client, numClients)

	for i := 0; i < numClients; i++ {
		clients[i] = NewClient(hub, nil, "multi-client-session")
		hub.Register(clients[i])
	}

	if hub.ClientCount() != numClients {
		t.Errorf("expected %d clients, got %d", numClients, hub.ClientCount())
	}

	// Broadcast a message
	msg := &Message{
		Type: MessageTypeStdout,
		Data: "broadcast test data",
	}
	if err := hub.BroadcastMessage(msg); err != nil {
		t.Fatalf("failed to broadcast message: %v", err)
	}

	// Verify all clients received the message
	for i, client := range clients {
		received := receiveWithTimeoutTest(t, client, 100*time.Millisecond)
		if received == nil {
			t.Errorf("client %d did not receive message", i)
			continue
		}

		var parsed Message
		if err := json.Unmarshal(received, &parsed); err != nil {
			t.Errorf("client %d received invalid JSON: %v", i, err)
			continue
		}

		if parsed.Type != MessageTypeStdout || parsed.Data != "broadcast test data" {
			t.Errorf("client %d received wrong message: type=%s data=%s", i, parsed.Type, parsed.Data)
		}
	}
}

// TestServiceIntegration tests the WebSocket service integration
func TestServiceIntegration(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "ws_service_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	ptyManager := pty.NewManager(tempDir)
	defer ptyManager.Close()

	wsService := NewService(ptyManager, driver.NewGenericDriver())
	defer wsService.Close()

	sessionID := "test-service-session"
	logPath := filepath.Join(tempDir, sessionID+".cast")

	session := &model.Session{
		ID:          sessionID,
		UserID:      "test-user",
		Name:        "Test Service Session",
		Command:     "echo 'service test'",
		Status:      model.SessionStatusRunning,
		LogFilePath: logPath,
	}

	// Track status changes
	var statusChanged bool
	var finalStatus model.SessionStatus
	wsService.SetOnStatusChange(func(sid string, status model.SessionStatus, exitCode *int) {
		if sid == sessionID {
			statusChanged = true
			finalStatus = status
		}
	})

	opts := pty.SpawnOptions{
		Session:     session,
		InitialRows: 24,
		InitialCols: 80,
	}

	_, err = wsService.AttachSession(context.Background(), session, opts)
	if err != nil {
		t.Fatalf("failed to attach session: %v", err)
	}

	// Wait for process to complete
	time.Sleep(2 * time.Second)

	if !statusChanged {
		t.Error("status change callback was not called")
	}

	if finalStatus != model.SessionStatusExited {
		t.Errorf("expected status 'exited', got '%s'", finalStatus)
	}
}

// Helper function
func receiveWithTimeoutTest(t *testing.T, client *Client, timeout time.Duration) []byte {
	t.Helper()
	select {
	case data := <-client.SendChan():
		return data
	case <-time.After(timeout):
		return nil
	}
}
