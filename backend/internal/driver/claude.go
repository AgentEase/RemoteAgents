package driver

import (
	"bytes"
	"regexp"
)

// ClaudeDriver is a driver for parsing Claude CLI output.
// It detects question patterns and waiting-for-input states.
type ClaudeDriver struct {
	// questionPattern matches patterns like "(y/n)", "(yes/no)", etc.
	questionPattern *regexp.Regexp

	// idlePattern matches patterns that indicate waiting for input.
	idlePattern *regexp.Regexp

	// buffer accumulates recent output for pattern matching.
	buffer *bytes.Buffer

	// maxBufferSize limits the buffer size to prevent unbounded growth.
	maxBufferSize int
}

// NewClaudeDriver creates a new ClaudeDriver instance.
func NewClaudeDriver() *ClaudeDriver {
	return &ClaudeDriver{
		// Match patterns like (y/n), (yes/no), (Y/N), etc.
		questionPattern: regexp.MustCompile(`\(([yY])/([nN])\)|\(([yY]es)/([nN]o)\)`),

		// Match patterns that indicate waiting for input
		// Common patterns: "? ", "> ", "$ ", "Continue? ", etc.
		idlePattern: regexp.MustCompile(`(\?\s*$|>\s*$|\$\s*$|Continue\?\s*$|Proceed\?\s*$)`),

		buffer:        &bytes.Buffer{},
		maxBufferSize: 4096, // Keep last 4KB for pattern matching
	}
}

// Name returns the name of the driver.
func (d *ClaudeDriver) Name() string {
	return "claude"
}

// Parse processes a chunk of PTY output and detects smart events.
func (d *ClaudeDriver) Parse(chunk []byte) (*ParseResult, error) {
	result := &ParseResult{
		RawData:     chunk,
		SmartEvents: []SmartEvent{},
	}

	// Append to buffer for pattern matching
	d.buffer.Write(chunk)

	// Trim buffer if it exceeds max size
	if d.buffer.Len() > d.maxBufferSize {
		// Keep only the last maxBufferSize bytes
		data := d.buffer.Bytes()
		d.buffer.Reset()
		d.buffer.Write(data[len(data)-d.maxBufferSize:])
	}

	// Get the current buffer content for pattern matching
	bufferContent := d.buffer.Bytes()

	// Check for question patterns
	if matches := d.questionPattern.FindSubmatch(bufferContent); matches != nil {
		// Extract the prompt text (last line or last N characters)
		prompt := d.extractPrompt(bufferContent)

		// Determine the options based on what was matched
		var options []string
		if len(matches[1]) > 0 && len(matches[2]) > 0 {
			// Matched (y/n) or (Y/N)
			options = []string{"y", "n"}
		} else if len(matches[3]) > 0 && len(matches[4]) > 0 {
			// Matched (yes/no) or (Yes/No)
			options = []string{"yes", "no"}
		}

		result.SmartEvents = append(result.SmartEvents, SmartEvent{
			Kind:    "question",
			Options: options,
			Prompt:  prompt,
		})
	}

	// Check for idle/waiting patterns
	if d.idlePattern.Match(bufferContent) {
		prompt := d.extractPrompt(bufferContent)

		result.SmartEvents = append(result.SmartEvents, SmartEvent{
			Kind:    "idle",
			Options: nil,
			Prompt:  prompt,
		})
	}

	return result, nil
}

// extractPrompt extracts the prompt text from the buffer.
// It returns the last line or last reasonable chunk of text.
func (d *ClaudeDriver) extractPrompt(data []byte) string {
	// Find the last newline
	lastNewline := bytes.LastIndexByte(data, '\n')

	var prompt []byte
	if lastNewline >= 0 && lastNewline < len(data)-1 {
		// Get text after the last newline
		prompt = data[lastNewline+1:]
	} else if lastNewline < 0 {
		// No newline found, use the entire buffer (up to a reasonable limit)
		if len(data) > 200 {
			prompt = data[len(data)-200:]
		} else {
			prompt = data
		}
	} else {
		// Last newline is at the end, look for the previous line
		prevNewline := bytes.LastIndexByte(data[:lastNewline], '\n')
		if prevNewline >= 0 {
			prompt = data[prevNewline+1 : lastNewline]
		} else {
			prompt = data[:lastNewline]
		}
	}

	// Trim whitespace and return
	return string(bytes.TrimSpace(prompt))
}

// Reset clears the internal buffer.
// This can be called when starting a new session or after significant events.
func (d *ClaudeDriver) Reset() {
	d.buffer.Reset()
}
