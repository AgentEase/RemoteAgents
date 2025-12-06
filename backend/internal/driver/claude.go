package driver

import (
	"bytes"
	"regexp"
	"strings"
	"time"
)

// ClaudeDriver is a driver for parsing Claude CLI output.
// It detects question patterns, waiting-for-input states, and conversation messages.
type ClaudeDriver struct {
	// questionPattern matches patterns like "(y/n)", "(yes/no)", etc.
	questionPattern *regexp.Regexp

	// claudeMenuPattern matches Claude Code's specific confirmation menu
	claudeMenuPattern *regexp.Regexp

	// idlePattern matches patterns that indicate waiting for input.
	idlePattern *regexp.Regexp

	// Message parsing patterns
	userCommandPattern  *regexp.Regexp // "> command"
	claudeResponseStart *regexp.Regexp // "● response"
	claudeActionPattern *regexp.Regexp // "● Write(file.txt)"
	claudeResultPattern *regexp.Regexp // "⎿ result"

	// buffer accumulates recent output for pattern matching.
	buffer *bytes.Buffer

	// maxBufferSize limits the buffer size to prevent unbounded growth.
	maxBufferSize int

	// lastEventChunk tracks the last chunk that triggered an event to avoid duplicates
	lastEventChunk int

	// Deduplication state
	lastUserInput    string
	lastClaudeAction string
	lastResponse     string
	lastActionResult string
	lastActionTime   time.Time

	// Output block collector for multi-line outputs
	inOutputBlock     bool
	outputLines       []string
	outputStartTime   time.Time
	outputBlockHeader string // "Diagnostics:" or first line of ⎿ output

	// Resume session tracking
	inResumeMenu            bool
	lastResumeSelection     string
	resumeSelectionComplete bool
	lastSessionResumed      string
}

// NewClaudeDriver creates a new ClaudeDriver instance.
func NewClaudeDriver() *ClaudeDriver {
	return &ClaudeDriver{
		// Match patterns like (y/n), (yes/no), (Y/N), etc.
		questionPattern: regexp.MustCompile(`\(([yY])/([nN])\)|\(([yY]es)/([nN]o)\)`),

		// Match Claude Code's specific confirmation menu pattern
		// "Do you want to create/write/delete/modify X?"
		claudeMenuPattern: regexp.MustCompile(`Do you want to (create|write|delete|modify|update|remove|edit|overwrite) .+\?`),

		// Match patterns that indicate waiting for input
		// Common patterns: "? ", "> ", "$ ", "Continue? ", etc.
		idlePattern: regexp.MustCompile(`(\?\s*$|>\s*$|\$\s*$|Continue\?\s*$|Proceed\?\s*$)`),

		// Message parsing patterns
		userCommandPattern:  regexp.MustCompile(`^>\s+(.+)$`),
		claudeResponseStart: regexp.MustCompile(`●\s*(.+)`),
		claudeActionPattern: regexp.MustCompile(`●\s*(Write|Read|Edit|Delete|Bash|Search)\(([^)]+)\)`),
		claudeResultPattern: regexp.MustCompile(`⎿\s*(.+)`),

		buffer:        &bytes.Buffer{},
		maxBufferSize: 4096, // Keep last 4KB for pattern matching
	}
}

// Name returns the name of the driver.
func (d *ClaudeDriver) Name() string {
	return "claude"
}

// Parse processes a chunk of PTY output and detects smart events and messages.
func (d *ClaudeDriver) Parse(chunk []byte) (*ParseResult, error) {
	result := &ParseResult{
		RawData:     chunk,
		SmartEvents: []SmartEvent{},
		Messages:    []Message{},
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

	// Strip ANSI escape sequences for pattern matching
	cleanContent := d.stripANSI(bufferContent)

	// Check for standard question patterns (y/n), (yes/no)
	if matches := d.questionPattern.FindSubmatch(cleanContent); matches != nil {
		prompt := d.extractPrompt(cleanContent)

		var options []string
		if len(matches[1]) > 0 && len(matches[2]) > 0 {
			// Matched (y/n) or (Y/N)
			options = []string{"y", "n"}
		} else if len(matches[3]) > 0 && len(matches[4]) > 0 {
			// Matched (yes/no) or (Yes/No)
			options = []string{"yes", "no"}
		}

		if len(options) > 0 {
			result.SmartEvents = append(result.SmartEvents, SmartEvent{
				Kind:    "question",
				Options: options,
				Prompt:  prompt,
			})
		}
	}

	// Check for Claude Code's specific menu pattern
	if matches := d.claudeMenuPattern.FindSubmatch(cleanContent); matches != nil {
		prompt := string(matches[0])
		// Claude Code's menu options: 1=Yes, 2=Yes allow all, Esc=Cancel
		result.SmartEvents = append(result.SmartEvents, SmartEvent{
			Kind:    "claude_confirm",
			Options: []string{"1", "2", "esc"},
			Prompt:  prompt,
		})
	}

	// Parse conversation messages from the chunk
	d.parseMessages(chunk, result)

	return result, nil
}

// parseMessages extracts conversation messages from the output chunk.
func (d *ClaudeDriver) parseMessages(chunk []byte, result *ParseResult) {
	content := string(d.stripANSI(chunk))
	lines := strings.Split(content, "\n")
	now := time.Now()

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || len(line) < 3 {
			continue
		}

		// Detect entering resume menu
		if strings.Contains(line, "Resume Session") {
			d.inResumeMenu = true
			d.lastResumeSelection = ""
			d.resumeSelectionComplete = false
		}

		// Track the current selection in resume menu (starts with ❯)
		// Format: "❯ good, testing cursor move" or "❯ good, testing cursor move ✔"
		if d.inResumeMenu && strings.Contains(line, "❯") {
			idx := strings.Index(line, "❯")
			if idx >= 0 {
				selection := strings.TrimSpace(line[idx+len("❯"):])
				selection = strings.ReplaceAll(selection, " ✔", "")
				if len(selection) > 0 {
					d.lastResumeSelection = selection
					d.resumeSelectionComplete = false
				}
			}
		}

		// Capture the timestamp line that follows the selected item
		// Format: "3 minutes ago · 16 messages · main"
		if d.inResumeMenu && d.lastResumeSelection != "" && !d.resumeSelectionComplete {
			isTimestampLine := strings.Contains(line, " ago · ") &&
				strings.Contains(line, " messages · ") &&
				!strings.Contains(line, "❯") &&
				!strings.Contains(line, "↑") &&
				!strings.Contains(line, "↓")
			if isTimestampLine {
				d.lastResumeSelection = d.lastResumeSelection + "\n" + line
				d.resumeSelectionComplete = true
			}
		}

		// Exit resume menu when new prompt appears - record the selection
		if d.inResumeMenu && strings.HasPrefix(line, ">") {
			// Only record if we have a complete selection (with timestamp)
			if d.lastResumeSelection != "" && d.resumeSelectionComplete && d.lastResumeSelection != d.lastSessionResumed {
				d.lastSessionResumed = d.lastResumeSelection
				result.Messages = append(result.Messages, Message{
					Timestamp: now,
					Type:      "session_resumed",
					Content:   d.lastResumeSelection,
				})
			}
			d.inResumeMenu = false
			d.lastResumeSelection = ""
			d.resumeSelectionComplete = false
		}

		// Detect "Diagnostics" header for /doctor - start collecting
		if line == "Diagnostics" {
			d.flushOutputBlock(result) // Flush any previous block
			d.inOutputBlock = true
			d.outputStartTime = now
			d.outputLines = []string{"Diagnostics:"}
			d.outputBlockHeader = "Diagnostics:"
			continue
		}

		// Detect tree-structured output (like /doctor): "└ content"
		if strings.HasPrefix(line, "└") && strings.Contains(line, ":") {
			diagLine := strings.TrimPrefix(line, "└")
			diagLine = strings.TrimSpace(diagLine)
			if d.inOutputBlock {
				// Add to current block
				d.outputLines = append(d.outputLines, diagLine)
			} else {
				// Start new block
				d.inOutputBlock = true
				d.outputStartTime = now
				d.outputLines = []string{diagLine}
				d.outputBlockHeader = diagLine
			}
			continue
		}

		// Check if we hit a new prompt (end of output block)
		isNewPrompt := strings.HasPrefix(line, ">")
		if isNewPrompt && d.inOutputBlock {
			d.flushOutputBlock(result)
		}

		// Skip UI elements and noise
		if d.isUINoiseOrLoading(line) {
			continue
		}

		// Detect interrupted/cancelled operations
		if strings.Contains(line, "Interrupted") {
			d.flushOutputBlock(result) // Flush any pending block
			result.Messages = append(result.Messages, Message{
				Timestamp: now,
				Type:      "agent_interrupted",
				Content:   line,
			})
			continue
		}

		// Extract user command from prompt echo: "> command"
		if matches := d.userCommandPattern.FindStringSubmatch(line); matches != nil {
			d.flushOutputBlock(result) // Flush any pending block
			cmd := strings.TrimSpace(matches[1])
			if len(cmd) > 0 && cmd != d.lastUserInput {
				d.lastUserInput = cmd
				result.Messages = append(result.Messages, Message{
					Timestamp: now,
					Type:      "user_input",
					Content:   cmd,
				})
			}
			continue
		}

		// Detect Claude action: "● Write(file.txt)"
		if matches := d.claudeActionPattern.FindStringSubmatch(line); matches != nil {
			d.flushOutputBlock(result) // Flush any pending block
			action := matches[1] + "(" + matches[2] + ")"
			if action != d.lastClaudeAction || now.Sub(d.lastActionTime) > 2*time.Second {
				d.lastClaudeAction = action
				d.lastActionTime = now
				result.Messages = append(result.Messages, Message{
					Timestamp: now,
					Type:      "claude_action",
					Content:   action,
				})
			}
			continue
		}

		// Detect Claude response: "● response text"
		if matches := d.claudeResponseStart.FindStringSubmatch(line); matches != nil {
			d.flushOutputBlock(result) // Flush any pending block
			response := strings.TrimSpace(matches[1])
			// Skip if it's an action pattern (contains parentheses)
			if strings.Contains(response, "(") && strings.Contains(response, ")") {
				continue
			}
			if len(response) > 10 && response != d.lastResponse {
				d.lastResponse = response
				result.Messages = append(result.Messages, Message{
					Timestamp: now,
					Type:      "claude_response",
					Content:   response,
				})
			}
			continue
		}

		// Detect action result or command output: "⎿ result"
		if matches := d.claudeResultPattern.FindStringSubmatch(line); matches != nil {
			resultText := strings.TrimSpace(matches[1])
			if len(resultText) < 3 {
				continue
			}

			if d.inOutputBlock {
				// Add to current block
				d.outputLines = append(d.outputLines, resultText)
			} else {
				// Start new output block
				d.inOutputBlock = true
				d.outputStartTime = now
				d.outputLines = []string{resultText}
				d.outputBlockHeader = resultText
			}
			continue
		}

		// If we're in an output block, collect the line
		if d.inOutputBlock && len(line) > 0 {
			d.outputLines = append(d.outputLines, line)
		}
	}
}

// flushOutputBlock saves the collected output block as a single message
func (d *ClaudeDriver) flushOutputBlock(result *ParseResult) {
	if !d.inOutputBlock || len(d.outputLines) == 0 {
		return
	}

	fullOutput := strings.Join(d.outputLines, "\n")

	// Categorize the result
	msgType := "command_output"
	firstLine := d.outputLines[0]
	if strings.HasPrefix(firstLine, "Wrote") || strings.HasPrefix(firstLine, "Created") ||
		strings.HasPrefix(firstLine, "Deleted") || strings.HasPrefix(firstLine, "Modified") ||
		strings.HasPrefix(firstLine, "Updated") || strings.HasPrefix(firstLine, "Read") {
		msgType = "action_result"
	}

	if fullOutput != d.lastActionResult {
		d.lastActionResult = fullOutput
		d.lastActionTime = d.outputStartTime
		result.Messages = append(result.Messages, Message{
			Timestamp: d.outputStartTime,
			Type:      msgType,
			Content:   fullOutput,
		})
	}

	// Reset output block state
	d.inOutputBlock = false
	d.outputLines = nil
	d.outputBlockHeader = ""
}

// isUINoiseOrLoading checks if a line is UI noise or loading indicator
func (d *ClaudeDriver) isUINoiseOrLoading(line string) bool {
	// Don't filter out selected resume items (starts with ❯)
	// These are handled separately for session_resumed tracking
	if strings.HasPrefix(line, "❯") {
		return false
	}

	// Loading indicators
	if strings.HasPrefix(line, "·") && strings.Contains(line, "…") {
		return true
	}
	// UI border elements (but not tree output like └ with content)
	// Only filter pure border lines, not tree-structured content
	if strings.HasPrefix(line, "─") || strings.HasPrefix(line, "│") ||
		strings.HasPrefix(line, "╭") || strings.HasPrefix(line, "╰") ||
		strings.HasPrefix(line, "╔") || strings.HasPrefix(line, "╚") ||
		strings.HasPrefix(line, "├") {
		return true
	}
	// └ is used for diagnostic tree - only filter if it's just decoration
	if strings.HasPrefix(line, "└") && !strings.Contains(line, ":") {
		return true
	}
	// Menu/dialog elements and navigation hints
	if strings.Contains(line, "shortcuts") ||
		strings.Contains(line, "Tip:") ||
		strings.Contains(line, "Thinking") ||
		strings.Contains(line, "Esc to") ||
		strings.Contains(line, "Press Enter to continue") ||
		strings.HasPrefix(line, "↓") ||
		strings.HasPrefix(line, "↑") ||
		strings.Contains(line, "A to show") ||
		strings.Contains(line, "B to toggle") ||
		strings.Contains(line, "/ to search") {
		return true
	}
	// Resume session menu items (not selected - doesn't start with ❯)
	if (strings.Contains(line, "messages · main") ||
		strings.Contains(line, "seconds ago") ||
		strings.Contains(line, "minutes ago")) &&
		!strings.HasPrefix(line, "❯") {
		return true
	}
	// Resume Session header
	if line == "Resume Session" || strings.HasPrefix(line, "Resume Session") {
		return true
	}
	// Menu options (numbered list items)
	if strings.HasPrefix(line, "1.") ||
		strings.HasPrefix(line, "2.") ||
		strings.HasPrefix(line, "3.") {
		return true
	}
	return false
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
	d.inOutputBlock = false
	d.outputLines = nil
	d.outputBlockHeader = ""
	d.inResumeMenu = false
	d.lastResumeSelection = ""
	d.resumeSelectionComplete = false
}

// Flush returns any pending output block as messages.
// Call this when the session ends to get remaining buffered content.
func (d *ClaudeDriver) Flush() []Message {
	var messages []Message
	if d.inOutputBlock && len(d.outputLines) > 0 {
		fullOutput := strings.Join(d.outputLines, "\n")

		msgType := "command_output"
		firstLine := d.outputLines[0]
		if strings.HasPrefix(firstLine, "Wrote") || strings.HasPrefix(firstLine, "Created") ||
			strings.HasPrefix(firstLine, "Deleted") || strings.HasPrefix(firstLine, "Modified") ||
			strings.HasPrefix(firstLine, "Updated") || strings.HasPrefix(firstLine, "Read") {
			msgType = "action_result"
		}

		if fullOutput != d.lastActionResult {
			d.lastActionResult = fullOutput
			messages = append(messages, Message{
				Timestamp: d.outputStartTime,
				Type:      msgType,
				Content:   fullOutput,
			})
		}

		d.inOutputBlock = false
		d.outputLines = nil
		d.outputBlockHeader = ""
	}
	return messages
}

// ansiPattern matches ANSI escape sequences
// Includes: CSI sequences, OSC sequences, DCS/SOS/PM/APC sequences, and private mode sequences
var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;?]*[a-zA-Z]|\x1b\][^\x07]*\x07|\x1b[PX^_][^\x1b]*\x1b\\|\x1b\[\?[0-9]+[hl]|\x1b\(B`)

// stripANSI removes ANSI escape sequences from the input
func (d *ClaudeDriver) stripANSI(data []byte) []byte {
	return ansiPattern.ReplaceAll(data, []byte{})
}
