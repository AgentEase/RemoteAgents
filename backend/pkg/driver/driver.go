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

// NewGenericDriver creates a new generic driver instance.
func NewGenericDriver() AgentDriver {
	return driver.NewGenericDriver()
}
