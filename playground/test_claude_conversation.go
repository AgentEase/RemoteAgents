package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/creack/pty"
	"golang.org/x/term"

	"github.com/remote-agent-terminal/backend/pkg/driver"
)

// ConversationEntry represents a single entry in the conversation log
type ConversationEntry struct {
	Timestamp  time.Time   `json:"timestamp"`
	Type       string      `json:"type"`
	Content    string      `json:"content"`
	SmartEvent *SmartEvent `json:"smart_event,omitempty"`
}

type SmartEvent struct {
	Kind    string   `json:"kind"`
	Options []string `json:"options,omitempty"`
	Prompt  string   `json:"prompt"`
}

// ConversationLog holds the entire conversation
type ConversationLog struct {
	StartTime time.Time           `json:"start_time"`
	Command   string              `json:"command"`
	Entries   []ConversationEntry `json:"entries"`
}

// ANSI escape sequence pattern
var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]|\x1b\][^\x07]*\x07|\x1b[PX^_][^\x1b]*\x1b\\|\x1b\[\?[0-9]+[hl]|\x1b\(B`)

// Claude output patterns
var (
	claudeResponseStart = regexp.MustCompile(`●\s*(.+)`)
	claudeActionPattern = regexp.MustCompile(`●\s*(Write|Read|Edit|Delete|Bash|Search)\(([^)]+)\)`)
	claudeResultPattern = regexp.MustCompile(`⎿\s*(.+)`)
	// Pattern to match user command from prompt echo: "> command"
	userCommandPattern = regexp.MustCompile(`^>\s+(.+)$`)
)

func stripANSI(data []byte) string {
	clean := ansiPattern.ReplaceAll(data, []byte{})
	return string(clean)
}

// cleanUserInput removes any escape sequence artifacts from user input
func cleanUserInput(input string) string {
	// Remove common escape sequence patterns that might slip through
	// [A = up, [B = down, [C = right, [D = left, [I = insert mode, etc.
	escapeArtifacts := regexp.MustCompile(`\[([A-Za-z]|[0-9;]*[A-Za-z~])`)
	input = escapeArtifacts.ReplaceAllString(input, "")
	// Remove any remaining control characters
	var cleaned strings.Builder
	for _, r := range input {
		if r >= 32 && r < 127 {
			cleaned.WriteRune(r)
		}
	}
	return strings.TrimSpace(cleaned.String())
}

// Deduplicator tracks seen content to avoid duplicates
type Deduplicator struct {
	lastUserInput      string
	lastClaudeAction   string
	lastSmartEvent     string
	lastResponse       string
	lastActionResult   string
	lastSessionResumed string
	lastActionTime     time.Time
}

func (d *Deduplicator) isDuplicate(entryType, content string) bool {
	now := time.Now()

	switch entryType {
	case "user_input":
		if content == d.lastUserInput {
			return true
		}
		d.lastUserInput = content
	case "claude_action":
		// Deduplicate actions within 2 seconds
		if content == d.lastClaudeAction && now.Sub(d.lastActionTime) < 2*time.Second {
			return true
		}
		d.lastClaudeAction = content
		d.lastActionTime = now
	case "smart_event":
		if content == d.lastSmartEvent {
			return true
		}
		d.lastSmartEvent = content
	case "claude_response":
		if content == d.lastResponse {
			return true
		}
		d.lastResponse = content
	case "session_resumed":
		if content == d.lastSessionResumed {
			return true
		}
		d.lastSessionResumed = content
	case "action_result":
		// Deduplicate results within 2 seconds
		if content == d.lastActionResult && now.Sub(d.lastActionTime) < 2*time.Second {
			return true
		}
		d.lastActionResult = content
	}
	return false
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run test_claude_conversation.go <command>")
		os.Exit(1)
	}

	command := os.Args[1]
	args := os.Args[2:]

	fmt.Printf("Starting command: %s %v\n", command, args)
	fmt.Println("Recording conversation to /tmp/claude_conversation.json")
	fmt.Println("Press Ctrl+C to exit\n")
	fmt.Println("════════════════════════════════════════════════════════════\n")

	convLog := &ConversationLog{
		StartTime: time.Now(),
		Command:   command + " " + strings.Join(args, " "),
		Entries:   []ConversationEntry{},
	}

	dedup := &Deduplicator{}

	cmd := exec.Command(command, args...)

	ptmx, err := pty.Start(cmd)
	if err != nil {
		fmt.Printf("Failed to start PTY: %v\n", err)
		os.Exit(1)
	}
	defer ptmx.Close()

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGWINCH)
	go func() {
		for range ch {
			pty.InheritSize(os.Stdin, ptmx)
		}
	}()
	ch <- syscall.SIGWINCH
	defer func() { signal.Stop(ch); close(ch) }()

	var oldState *term.State
	if term.IsTerminal(int(os.Stdin.Fd())) {
		oldState, err = term.MakeRaw(int(os.Stdin.Fd()))
		if err == nil {
			defer func() { _ = term.Restore(int(os.Stdin.Fd()), oldState) }()
		}
	}

	claudeDriver := driver.NewClaudeDriver()

	// Track user input with cursor position support
	var userInputRunes []rune
	var cursorPos int
	var escapeSeq bytes.Buffer
	inEscapeSeq := false
	escapeStartTime := time.Now()

	// Menu selection tracking
	inMenuMode := false
	menuSelection := 1 // Default to option 1

	// Track if we're waiting for agent response (for ESC cancel detection)
	waitingForAgent := false

	go func() {
		buf := make([]byte, 1)
		for {
			n, err := os.Stdin.Read(buf)
			if err != nil {
				return
			}
			if n > 0 {
				ptmx.Write(buf[:n])
				b := buf[0]

				// Handle escape sequences (arrow keys, etc.)
				if b == 0x1b { // ESC
					inEscapeSeq = true
					escapeSeq.Reset()
					escapeSeq.WriteByte(b)
					escapeStartTime = time.Now()
					continue
				}

				if inEscapeSeq {
					// Check for timeout - if more than 50ms passed, it was a standalone ESC
					if time.Since(escapeStartTime) > 50*time.Millisecond {
						inEscapeSeq = false
						// Record ESC cancel if we were waiting for agent or in menu
						if waitingForAgent || inMenuMode {
							convLog.Entries = append(convLog.Entries, ConversationEntry{
								Timestamp: time.Now(),
								Type:      "user_cancel",
								Content:   "ESC (cancelled)",
							})
							inMenuMode = false
							menuSelection = 1
						}
						// Clear input buffer on ESC
						userInputRunes = nil
						cursorPos = 0
					}

					escapeSeq.WriteByte(b)
					// Check if escape sequence is complete
					if (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') || b == '~' {
						seq := escapeSeq.String()
						inEscapeSeq = false

						// Track arrow key navigation
						if inMenuMode {
							// Menu mode: up/down changes selection
							if strings.HasSuffix(seq, "B") { // Down arrow
								if menuSelection < 3 {
									menuSelection++
								}
							} else if strings.HasSuffix(seq, "A") { // Up arrow
								if menuSelection > 1 {
									menuSelection--
								}
							}
						} else {
							// Command mode: left/right moves cursor
							if strings.HasSuffix(seq, "D") { // Left arrow
								if cursorPos > 0 {
									cursorPos--
								}
							} else if strings.HasSuffix(seq, "C") { // Right arrow
								if cursorPos < len(userInputRunes) {
									cursorPos++
								}
							} else if strings.HasSuffix(seq, "H") { // Home
								cursorPos = 0
							} else if strings.HasSuffix(seq, "F") { // End
								cursorPos = len(userInputRunes)
							} else if strings.HasSuffix(seq, "A") || strings.HasSuffix(seq, "B") {
								// Up/Down arrow in command mode = history recall
								// Clear current input since terminal will replace it
								userInputRunes = nil
								cursorPos = 0
							} else if strings.HasSuffix(seq, "3~") {
								// Delete key - delete char at cursor
								if cursorPos < len(userInputRunes) {
									userInputRunes = append(userInputRunes[:cursorPos], userInputRunes[cursorPos+1:]...)
								}
							}
						}
					}
					continue
				}

				// Track Enter key to capture menu selection
				if b == '\r' || b == '\n' {
					if inMenuMode {
						// Record menu selection
						var selectionText string
						switch menuSelection {
						case 1:
							selectionText = "Yes (option 1)"
						case 2:
							selectionText = "Yes, allow all (option 2)"
						case 3:
							selectionText = "Custom response (option 3)"
						}
						convLog.Entries = append(convLog.Entries, ConversationEntry{
							Timestamp: time.Now(),
							Type:      "user_selection",
							Content:   selectionText,
						})
						inMenuMode = false
						menuSelection = 1 // Reset for next menu
					}
					// User input is now captured from PTY output ("> command" pattern)
					// which is more reliable than keystroke tracking
					waitingForAgent = true
					userInputRunes = nil
					cursorPos = 0
				} else if b == 127 || b == 8 { // Backspace - delete char before cursor
					if cursorPos > 0 && len(userInputRunes) > 0 {
						// Remove character at cursorPos-1
						userInputRunes = append(userInputRunes[:cursorPos-1], userInputRunes[cursorPos:]...)
						cursorPos--
					}
				} else if b == 0x04 { // Ctrl+D - delete char at cursor (like Delete key)
					if cursorPos < len(userInputRunes) {
						userInputRunes = append(userInputRunes[:cursorPos], userInputRunes[cursorPos+1:]...)
					}
				} else if b >= 32 && b < 127 { // Printable ASCII characters
					// Insert character at cursor position
					r := rune(b)
					if cursorPos >= len(userInputRunes) {
						userInputRunes = append(userInputRunes, r)
					} else {
						// Insert in the middle
						userInputRunes = append(userInputRunes[:cursorPos], append([]rune{r}, userInputRunes[cursorPos:]...)...)
					}
					cursorPos++
					// User is typing, no longer just waiting
					waitingForAgent = false
				}
			}
		}
	}()



	// Read from PTY
	buf := make([]byte, 4096)
	var outputBuffer bytes.Buffer

	for {
		n, err := ptmx.Read(buf)
		if err != nil {
			if err != io.EOF {
				fmt.Printf("\r\n[Session ended]\r\n")
			}
			break
		}

		if n > 0 {
			data := buf[:n]
			os.Stdout.Write(data)
			outputBuffer.Write(data)

			// Parse with ClaudeDriver
			result, _ := claudeDriver.Parse(data)

			// Log smart events (deduplicated)
			if len(result.SmartEvents) > 0 {
				for _, event := range result.SmartEvents {
					if !dedup.isDuplicate("smart_event", event.Prompt) {
						convLog.Entries = append(convLog.Entries, ConversationEntry{
							Timestamp: time.Now(),
							Type:      "smart_event",
							Content:   event.Prompt,
							SmartEvent: &SmartEvent{
								Kind:    event.Kind,
								Options: event.Options,
								Prompt:  event.Prompt,
							},
						})
						// Enter menu mode when we detect a confirmation prompt
						if event.Kind == "claude_confirm" {
							inMenuMode = true
							menuSelection = 1 // Reset to default
						}
						fmt.Printf("\r\n╔══════════════════════════════════════════════════════════╗\r\n")
						fmt.Printf("║ 🎯 SMART EVENT: %-40s ║\r\n", event.Kind)
						fmt.Printf("╚══════════════════════════════════════════════════════════╝\r\n")
					}
				}
			}

			// Process output for Claude responses and actions
			processClaudeOutput(convLog, dedup, &outputBuffer)
		}
	}

	// Process remaining output
	processClaudeOutput(convLog, dedup, &outputBuffer)

	cmd.Wait()
	saveConversationLog(convLog)

	fmt.Printf("\r\n\r\n════════════════════════════════════════════════════════════\r\n")
	fmt.Printf("Session ended. Entries: %d\r\n", len(convLog.Entries))
	fmt.Printf("Conversation saved to: /tmp/claude_conversation.json\r\n")
	fmt.Printf("════════════════════════════════════════════════════════════\r\n")
}

// OutputBlockCollector collects multi-line output blocks
type OutputBlockCollector struct {
	inOutputBlock   bool
	outputLines     []string
	outputStartTime time.Time
}

func (c *OutputBlockCollector) reset() {
	c.inOutputBlock = false
	c.outputLines = nil
}

// isUINoiseOrLoading checks if a line is UI noise or loading indicator
func isUINoiseOrLoading(line string) bool {
	// Don't filter out selected resume items (starts with ❯)
	if strings.HasPrefix(line, "❯") {
		return false
	}
	
	// Loading indicators
	if strings.HasPrefix(line, "·") && strings.Contains(line, "…") {
		return true
	}
	// UI border elements (but not diagnostic tree elements like └ with content)
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
	// Menu/dialog elements
	if strings.Contains(line, "shortcuts") ||
		strings.Contains(line, "Tip:") ||
		strings.Contains(line, "Thinking") ||
		strings.Contains(line, "Esc to") ||
		strings.Contains(line, "Press Enter to continue") ||
		strings.HasPrefix(line, "↓") ||
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
	// Menu options
	if strings.HasPrefix(line, "1.") ||
		strings.HasPrefix(line, "2.") ||
		strings.HasPrefix(line, "3.") {
		return true
	}
	return false
}

// isResumeSelection checks if a line indicates a selected resume session
func isResumeSelection(line string) bool {
	// Check for selected item in resume menu (starts with ❯)
	if strings.HasPrefix(line, "❯") &&
		(strings.Contains(line, "messages · main") ||
			strings.Contains(line, "ago ·")) {
		return true
	}
	return false
}

// extractResumeSessionInfo extracts the session description from a resume selection line
func extractResumeSessionInfo(line string) string {
	// Remove the ❯ prefix and ✔ suffix if present
	line = strings.TrimPrefix(line, "❯ ")
	line = strings.TrimSuffix(line, " ✔")
	return strings.TrimSpace(line)
}

// State for tracking resume menu
var (
	inResumeMenu            bool
	lastResumeSelection     string
	resumeSelectionComplete bool
)

func processClaudeOutput(convLog *ConversationLog, dedup *Deduplicator, buffer *bytes.Buffer) {
	content := stripANSI(buffer.Bytes())
	lines := strings.Split(content, "\n")

	var outputCollector OutputBlockCollector

	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)
		if trimmedLine == "" {
			continue
		}

		// Detect entering resume menu
		if strings.Contains(trimmedLine, "Resume Session") {
			inResumeMenu = true
			lastResumeSelection = ""
		}

		// Track the current selection in resume menu (starts with ❯)
		// Format: "❯ good, testing cursor move" or "❯ good, testing cursor move ✔"
		if inResumeMenu && strings.Contains(trimmedLine, "❯") {
			idx := strings.Index(trimmedLine, "❯")
			if idx >= 0 {
				selection := strings.TrimSpace(trimmedLine[idx+len("❯"):])
				selection = strings.ReplaceAll(selection, " ✔", "")
				if len(selection) > 0 {
					lastResumeSelection = selection
					resumeSelectionComplete = false
				}
			}
		}
		
		// Capture the timestamp line that follows the selected item
		// Format: "3 minutes ago · 16 messages · main"
		// This line appears right after the ❯ line
		if inResumeMenu && lastResumeSelection != "" && !resumeSelectionComplete {
			isTimestampLine := strings.Contains(trimmedLine, " ago · ") && 
				strings.Contains(trimmedLine, " messages · ") &&
				!strings.Contains(trimmedLine, "❯") &&
				!strings.Contains(trimmedLine, "↑") &&
				!strings.Contains(trimmedLine, "↓")
			if isTimestampLine {
				// This is the timestamp for the current selection
				lastResumeSelection = lastResumeSelection + "\n" + trimmedLine
				resumeSelectionComplete = true
			}
		}
		
		// Exit resume menu when new prompt appears - record the selection
		if inResumeMenu && strings.HasPrefix(trimmedLine, ">") {
			if lastResumeSelection != "" {
				if !dedup.isDuplicate("session_resumed", lastResumeSelection) {
					convLog.Entries = append(convLog.Entries, ConversationEntry{
						Timestamp: time.Now(),
						Type:      "session_resumed",
						Content:   lastResumeSelection,
					})
				}
			}
			inResumeMenu = false
			lastResumeSelection = ""
			resumeSelectionComplete = false
		}

		// Detect /doctor diagnostic output (lines starting with └ that contain :)
		if strings.HasPrefix(trimmedLine, "└") && strings.Contains(trimmedLine, ":") {
			// This is a diagnostic line, add to output collector
			if !outputCollector.inOutputBlock {
				outputCollector.inOutputBlock = true
				outputCollector.outputStartTime = time.Now()
				outputCollector.outputLines = []string{}
			}
			// Clean up the line (remove └ prefix)
			diagLine := strings.TrimPrefix(trimmedLine, "└ ")
			outputCollector.outputLines = append(outputCollector.outputLines, diagLine)
			continue
		}
		
		// Also detect "Diagnostics" header for /doctor
		if trimmedLine == "Diagnostics" {
			if !outputCollector.inOutputBlock {
				outputCollector.inOutputBlock = true
				outputCollector.outputStartTime = time.Now()
				outputCollector.outputLines = []string{"Diagnostics:"}
			}
			continue
		}

		// Check if we hit a new prompt or UI element (end of output block)
		isNewPrompt := strings.HasPrefix(trimmedLine, ">")
		isUIElement := isUINoiseOrLoading(trimmedLine)
		
		if (isNewPrompt || isUIElement) && outputCollector.inOutputBlock {
			// Save collected output block
			if len(outputCollector.outputLines) > 0 {
				fullOutput := strings.Join(outputCollector.outputLines, "\n")
				// Categorize the result
				resultType := "command_output"
				firstLine := outputCollector.outputLines[0]
				if strings.HasPrefix(firstLine, "Wrote") || strings.HasPrefix(firstLine, "Created") ||
					strings.HasPrefix(firstLine, "Deleted") || strings.HasPrefix(firstLine, "Modified") ||
					strings.HasPrefix(firstLine, "Updated") || strings.HasPrefix(firstLine, "Read") {
					resultType = "action_result"
				}
				if !dedup.isDuplicate(resultType, fullOutput) {
					convLog.Entries = append(convLog.Entries, ConversationEntry{
						Timestamp: outputCollector.outputStartTime,
						Type:      resultType,
						Content:   fullOutput,
					})
				}
			}
			outputCollector.reset()
		}
		
		// Skip UI noise
		if isUIElement {
			continue
		}

		// Detect interrupted/cancelled operations
		if strings.Contains(trimmedLine, "Interrupted") {
			if !dedup.isDuplicate("claude_response", "Interrupted") {
				convLog.Entries = append(convLog.Entries, ConversationEntry{
					Timestamp: time.Now(),
					Type:      "agent_interrupted",
					Content:   trimmedLine,
				})
			}
			continue
		}

		// Extract user command from prompt echo (more reliable than keystroke tracking)
		if matches := userCommandPattern.FindStringSubmatch(trimmedLine); matches != nil {
			cmd := strings.TrimSpace(matches[1])
			if len(cmd) > 0 && !dedup.isDuplicate("user_input", cmd) {
				convLog.Entries = append(convLog.Entries, ConversationEntry{
					Timestamp: time.Now(),
					Type:      "user_input",
					Content:   cmd,
				})
			}
			continue
		}

		// Skip UI elements and noise (already handled above, but double-check)
		if isUINoiseOrLoading(trimmedLine) {
			continue
		}

		// Detect Claude action
		if matches := claudeActionPattern.FindStringSubmatch(trimmedLine); matches != nil {
			action := fmt.Sprintf("%s(%s)", matches[1], matches[2])
			if !dedup.isDuplicate("claude_action", action) {
				convLog.Entries = append(convLog.Entries, ConversationEntry{
					Timestamp: time.Now(),
					Type:      "claude_action",
					Content:   action,
				})
			}
			continue
		}

		// Detect Claude response (but not actions)
		if matches := claudeResponseStart.FindStringSubmatch(trimmedLine); matches != nil {
			response := strings.TrimSpace(matches[1])
			// Skip if it's an action pattern
			if strings.Contains(response, "(") && strings.Contains(response, ")") {
				continue
			}
			if len(response) > 10 && !dedup.isDuplicate("claude_response", response) {
				convLog.Entries = append(convLog.Entries, ConversationEntry{
					Timestamp: time.Now(),
					Type:      "claude_response",
					Content:   response,
				})
			}
			continue
		}

		// Detect start of output block (starts with ⎿)
		if matches := claudeResultPattern.FindStringSubmatch(trimmedLine); matches != nil {
			result := strings.TrimSpace(matches[1])
			if len(result) >= 3 {
				outputCollector.inOutputBlock = true
				outputCollector.outputStartTime = time.Now()
				outputCollector.outputLines = []string{result}
			}
			continue
		}

		// If we're in an output block, collect the line
		if outputCollector.inOutputBlock && len(trimmedLine) > 0 {
			outputCollector.outputLines = append(outputCollector.outputLines, trimmedLine)
		}
	}

	// Save any remaining output block at end of buffer
	if outputCollector.inOutputBlock && len(outputCollector.outputLines) > 0 {
		fullOutput := strings.Join(outputCollector.outputLines, "\n")
		resultType := "command_output"
		firstLine := outputCollector.outputLines[0]
		if strings.HasPrefix(firstLine, "Wrote") || strings.HasPrefix(firstLine, "Created") ||
			strings.HasPrefix(firstLine, "Deleted") || strings.HasPrefix(firstLine, "Modified") ||
			strings.HasPrefix(firstLine, "Updated") || strings.HasPrefix(firstLine, "Read") {
			resultType = "action_result"
		}
		if !dedup.isDuplicate(resultType, fullOutput) {
			convLog.Entries = append(convLog.Entries, ConversationEntry{
				Timestamp: outputCollector.outputStartTime,
				Type:      resultType,
				Content:   fullOutput,
			})
		}
	}

	buffer.Reset()
}

func saveConversationLog(convLog *ConversationLog) {
	jsonData, err := json.MarshalIndent(convLog, "", "  ")
	if err != nil {
		fmt.Printf("\r\nError marshaling JSON: %v\r\n", err)
		return
	}

	os.WriteFile("/tmp/claude_conversation.json", jsonData, 0644)

	// Save readable text version
	var textLog strings.Builder
	textLog.WriteString("=== Claude Conversation Log ===\n")
	textLog.WriteString(fmt.Sprintf("Started: %s\n", convLog.StartTime.Format("2006-01-02 15:04:05")))
	textLog.WriteString(fmt.Sprintf("Command: %s\n", convLog.Command))
	textLog.WriteString("================================\n\n")

	for _, entry := range convLog.Entries {
		timestamp := entry.Timestamp.Format("15:04:05")
		switch entry.Type {
		case "user_input":
			textLog.WriteString(fmt.Sprintf("[%s] 👤 User: %s\n", timestamp, entry.Content))
		case "claude_response":
			textLog.WriteString(fmt.Sprintf("[%s] 🤖 Claude: %s\n", timestamp, entry.Content))
		case "claude_action":
			textLog.WriteString(fmt.Sprintf("[%s] ⚡ Action: %s\n", timestamp, entry.Content))
		case "action_result":
			textLog.WriteString(fmt.Sprintf("[%s]    Result: %s\n", timestamp, entry.Content))
		case "smart_event":
			textLog.WriteString(fmt.Sprintf("[%s] 🎯 Confirm: %s\n", timestamp, entry.SmartEvent.Prompt))
			if len(entry.SmartEvent.Options) > 0 {
				textLog.WriteString(fmt.Sprintf("           Options: %v\n", entry.SmartEvent.Options))
			}
		case "user_selection":
			textLog.WriteString(fmt.Sprintf("[%s] ✅ Selected: %s\n", timestamp, entry.Content))
		case "user_cancel":
			textLog.WriteString(fmt.Sprintf("[%s] ❌ Cancelled: %s\n", timestamp, entry.Content))
		case "agent_interrupted":
			textLog.WriteString(fmt.Sprintf("[%s] ⏹️ Interrupted: %s\n", timestamp, entry.Content))
		case "command_output":
			textLog.WriteString(fmt.Sprintf("[%s] 📋 Output: %s\n", timestamp, entry.Content))
		case "session_resumed":
			textLog.WriteString(fmt.Sprintf("[%s] 🔄 Resumed: %s\n", timestamp, entry.Content))
		}
	}

	textLog.WriteString(fmt.Sprintf("\n================================\nTotal entries: %d\n", len(convLog.Entries)))
	os.WriteFile("/tmp/claude_conversation.txt", []byte(textLog.String()), 0644)
}
