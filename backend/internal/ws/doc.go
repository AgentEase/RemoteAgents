// Package ws provides WebSocket connection handling and message routing
// for terminal sessions.
//
// The package implements:
//   - Hub: Manages WebSocket client connections for a session
//   - HubManager: Manages multiple hubs for different sessions
//   - Handler: Handles WebSocket message processing (stdin, resize, ping)
//   - Service: Integrates WebSocket with PTY for session lifecycle management
//
// Key features:
//   - Bidirectional communication between browser and PTY (Requirement 3.1)
//   - Hot restore: Sends Ring Buffer history on reconnect (Requirement 4.3)
//   - Session keepalive: PTY continues running when clients disconnect (Requirement 4.1)
//   - ANSI sequence passthrough: Preserves terminal formatting (Requirement 3.5)
//   - SmartEvent broadcasting: Forwards AgentDriver events to clients (Requirement 6.5)
package ws
