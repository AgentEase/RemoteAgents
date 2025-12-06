package driver

import (
	"github.com/remote-agent-terminal/backend/internal/driver"
)

// Re-export types from internal/driver for external use
type (
	AgentDriver = driver.AgentDriver
	SmartEvent  = driver.SmartEvent
	ParseResult = driver.ParseResult
	Message     = driver.Message
	InputAction = driver.InputAction
)

// Re-export key constants
const (
	KeyEnter     = driver.KeyEnter
	KeyEscape    = driver.KeyEscape
	KeyCtrlC     = driver.KeyCtrlC
	KeyCtrlD     = driver.KeyCtrlD
	KeyBackspace = driver.KeyBackspace
	KeyTab       = driver.KeyTab
	KeyUp        = driver.KeyUp
	KeyDown      = driver.KeyDown
	KeyRight     = driver.KeyRight
	KeyLeft      = driver.KeyLeft
)

// ClaudeDriver wraps the internal ClaudeDriver to expose additional methods
type ClaudeDriver struct {
	*driver.ClaudeDriver
}

// NewClaudeDriver creates a new Claude driver instance.
func NewClaudeDriver() *ClaudeDriver {
	return &ClaudeDriver{
		ClaudeDriver: driver.NewClaudeDriver(),
	}
}

// Flush returns any pending buffered output as messages.
// Call this when the session ends to get remaining content.
func (d *ClaudeDriver) Flush() []Message {
	return d.ClaudeDriver.Flush()
}

// FormatInput formats an input action into bytes for PTY.
func (d *ClaudeDriver) FormatInput(action InputAction) []byte {
	return d.ClaudeDriver.FormatInput(action)
}

// RespondToEvent generates the appropriate input for a SmartEvent response.
func (d *ClaudeDriver) RespondToEvent(event SmartEvent, response string) []byte {
	return d.ClaudeDriver.RespondToEvent(event, response)
}

// SendCommand sends a command to Claude Code (text + Enter).
func (d *ClaudeDriver) SendCommand(command string) []byte {
	return d.ClaudeDriver.SendCommand(command)
}

// SendSlashCommand sends a slash command (e.g., /doctor, /resume).
func (d *ClaudeDriver) SendSlashCommand(command string) []byte {
	return d.ClaudeDriver.SendSlashCommand(command)
}

// SelectMenuItem selects a menu item by number or arrow navigation.
func (d *ClaudeDriver) SelectMenuItem(index int) []byte {
	return d.ClaudeDriver.SelectMenuItem(index)
}

// NewGenericDriver creates a new generic driver instance.
func NewGenericDriver() AgentDriver {
	return driver.NewGenericDriver()
}
