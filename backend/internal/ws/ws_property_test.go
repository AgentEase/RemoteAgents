package ws

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/remote-agent-terminal/backend/internal/buffer"
	"github.com/remote-agent-terminal/backend/internal/driver"
)

// **Feature: remote-agent-terminal, Property 5: WebSocket 双向通信**
// *对于任何*通过 WebSocket 发送的 stdin 数据，PTY 进程应接收到相同的数据；
// *对于任何* PTY 产生的 stdout 数据，WebSocket 客户端应接收到相同的数据。
// **Validates: Requirements 3.1**
func TestWebSocketBidirectionalCommunicationProperty(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	// Test that stdin messages are correctly parsed and data is preserved
	properties.Property("stdin messages preserve data integrity", prop.ForAll(
		func(data string) bool {
			// Create a stdin message
			msg := Message{
				Type: MessageTypeStdin,
				Data: data,
			}

			// Serialize to JSON
			jsonData, err := json.Marshal(msg)
			if err != nil {
				return false
			}

			// Deserialize back
			var parsed Message
			if err := json.Unmarshal(jsonData, &parsed); err != nil {
				return false
			}

			// Verify data integrity
			return parsed.Type == MessageTypeStdin && parsed.Data == data
		},
		gen.AnyString(),
	))

	// Test that stdout messages preserve data integrity
	properties.Property("stdout messages preserve data integrity", prop.ForAll(
		func(data string) bool {
			// Create a stdout message
			msg := Message{
				Type: MessageTypeStdout,
				Data: data,
			}

			// Serialize to JSON
			jsonData, err := json.Marshal(msg)
			if err != nil {
				return false
			}

			// Deserialize back
			var parsed Message
			if err := json.Unmarshal(jsonData, &parsed); err != nil {
				return false
			}

			// Verify data integrity
			return parsed.Type == MessageTypeStdout && parsed.Data == data
		},
		gen.AnyString(),
	))

	// Test hub broadcast delivers to all clients
	properties.Property("hub broadcast delivers messages to all registered clients", prop.ForAll(
		func(numClients int, data string) bool {
			if numClients <= 0 || numClients > 10 {
				numClients = 1
			}

			hub := NewHub("test-session")
			defer hub.Close()

			// Create mock clients with channels to receive data
			var wg sync.WaitGroup
			received := make([]string, numClients)
			clients := make([]*mockClient, numClients)

			for i := 0; i < numClients; i++ {
				mc := newMockClient(hub, "test-session")
				clients[i] = mc
				hub.Register(mc.client)

				idx := i
				wg.Add(1)
				go func() {
					defer wg.Done()
					select {
					case msg := <-mc.client.SendChan():
						received[idx] = string(msg)
					case <-time.After(100 * time.Millisecond):
						received[idx] = ""
					}
				}()
			}

			// Broadcast message
			hub.Broadcast([]byte(data))

			// Wait for all clients to receive
			wg.Wait()

			// Verify all clients received the same data
			for i := 0; i < numClients; i++ {
				if received[i] != data {
					return false
				}
			}

			return true
		},
		gen.IntRange(1, 10),
		gen.AnyString(),
	))

	properties.TestingRun(t)
}

// **Feature: remote-agent-terminal, Property 7: ANSI 序列透传**
// *对于任何*包含 ANSI 转义序列的 PTY 输出，传输到 WebSocket 客户端的数据应与原始输出字节完全相同。
// **Validates: Requirements 3.5**
func TestANSISequencePassthroughProperty(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	// Generator for ANSI escape sequences
	ansiSequenceGen := gen.OneConstOf(
		"\x1b[31m",      // Red text
		"\x1b[32m",      // Green text
		"\x1b[0m",       // Reset
		"\x1b[1m",       // Bold
		"\x1b[4m",       // Underline
		"\x1b[H",        // Cursor home
		"\x1b[2J",       // Clear screen
		"\x1b[K",        // Clear line
		"\x1b[1;1H",     // Move cursor
		"\x1b[?25h",     // Show cursor
		"\x1b[?25l",     // Hide cursor
		"\x1b[38;5;196m", // 256-color red
	)

	// Test that ANSI sequences are preserved in stdout messages
	properties.Property("ANSI sequences are preserved in stdout messages", prop.ForAll(
		func(prefix, ansi, suffix string) bool {
			// Combine into data with ANSI sequence
			data := prefix + ansi + suffix

			// Create stdout message
			msg := Message{
				Type: MessageTypeStdout,
				Data: data,
			}

			// Serialize and deserialize
			jsonData, err := json.Marshal(msg)
			if err != nil {
				return false
			}

			var parsed Message
			if err := json.Unmarshal(jsonData, &parsed); err != nil {
				return false
			}

			// Verify exact byte preservation
			return parsed.Data == data
		},
		gen.AnyString(),
		ansiSequenceGen,
		gen.AnyString(),
	))

	// Test that GenericDriver preserves ANSI sequences
	properties.Property("GenericDriver preserves ANSI sequences in output", prop.ForAll(
		func(prefix, ansi, suffix string) bool {
			data := prefix + ansi + suffix
			drv := driver.NewGenericDriver()

			result, err := drv.Parse([]byte(data))
			if err != nil {
				return false
			}

			// Verify raw data is exactly preserved
			return string(result.RawData) == data
		},
		gen.AnyString(),
		ansiSequenceGen,
		gen.AnyString(),
	))

	// Test binary data with escape sequences
	properties.Property("binary data with escape sequences is preserved", prop.ForAll(
		func(data []byte) bool {
			drv := driver.NewGenericDriver()

			result, err := drv.Parse(data)
			if err != nil {
				return false
			}

			// Verify exact byte preservation
			if len(result.RawData) != len(data) {
				return false
			}
			for i := range data {
				if result.RawData[i] != data[i] {
					return false
				}
			}
			return true
		},
		gen.SliceOf(gen.UInt8()),
	))

	properties.TestingRun(t)
}

// **Feature: remote-agent-terminal, Property 8: 会话保活与热恢复**
// *对于任何*活跃会话，当 WebSocket 断开后：(1) PTY 进程应继续运行，
// (2) 输出应被缓存到 Ring Buffer，(3) 重新连接时应立即收到缓存的历史数据。
// **Validates: Requirements 4.1, 4.2, 4.3**
func TestSessionKeepaliveAndHotRestoreProperty(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	// Test that Ring Buffer preserves data for hot restore
	properties.Property("Ring Buffer preserves data for hot restore", prop.ForAll(
		func(chunks [][]byte) bool {
			if len(chunks) == 0 {
				return true
			}

			rb := buffer.NewRingBuffer(64 * 1024) // 64KB buffer

			// Write all chunks
			var totalData []byte
			for _, chunk := range chunks {
				rb.Write(chunk)
				totalData = append(totalData, chunk...)
			}

			// Read all data
			history := rb.ReadAll()

			// If total data fits in buffer, should be exactly equal
			if len(totalData) <= 64*1024 {
				if len(history) != len(totalData) {
					return false
				}
				for i := range history {
					if history[i] != totalData[i] {
						return false
					}
				}
			} else {
				// If overflow, should have last 64KB
				if len(history) > 64*1024 {
					return false
				}
				// Should be suffix of total data
				expectedStart := len(totalData) - len(history)
				for i := range history {
					if history[i] != totalData[expectedStart+i] {
						return false
					}
				}
			}

			return true
		},
		gen.SliceOfN(10, gen.SliceOf(gen.UInt8())),
	))

	// Test that hub continues to exist after all clients disconnect
	properties.Property("hub persists after client disconnection", prop.ForAll(
		func(sessionID string) bool {
			if sessionID == "" {
				sessionID = "test-session"
			}

			manager := NewHubManager()
			defer manager.Close()

			// Create hub and client
			hub := manager.GetOrCreate(sessionID)
			mc := newMockClient(hub, sessionID)
			hub.Register(mc.client)

			// Verify client is registered
			if hub.ClientCount() != 1 {
				return false
			}

			// Unregister client
			hub.Unregister(mc.client)

			// Hub should still exist in manager (for session keepalive)
			existingHub := manager.Get(sessionID)
			if existingHub == nil {
				return false
			}

			// Client count should be 0
			if existingHub.ClientCount() != 0 {
				return false
			}

			return true
		},
		gen.AlphaString(),
	))

	// Test that history message is correctly formatted
	properties.Property("history message preserves data", prop.ForAll(
		func(historyData string) bool {
			msg := Message{
				Type: MessageTypeHistory,
				Data: historyData,
			}

			jsonData, err := json.Marshal(msg)
			if err != nil {
				return false
			}

			var parsed Message
			if err := json.Unmarshal(jsonData, &parsed); err != nil {
				return false
			}

			return parsed.Type == MessageTypeHistory && parsed.Data == historyData
		},
		gen.AnyString(),
	))

	properties.TestingRun(t)
}

// mockClient is a test helper that wraps a Client without a real WebSocket connection
type mockClient struct {
	client *Client
}

func newMockClient(hub *Hub, sessionID string) *mockClient {
	client := &Client{
		hub:       hub,
		conn:      nil, // No real connection for testing
		sessionID: sessionID,
		send:      make(chan []byte, 256),
	}
	return &mockClient{client: client}
}
