package driver

import (
	"strings"
	"testing"
)

// TestClaudeDriver_Name tests the Name method
func TestClaudeDriver_Name(t *testing.T) {
	driver := NewClaudeDriver()
	if driver.Name() != "claude" {
		t.Errorf("Expected name 'claude', got '%s'", driver.Name())
	}
}

// TestClaudeDriver_Parse_QuestionPattern tests detection of (y/n) patterns
func TestClaudeDriver_Parse_QuestionPattern(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		expectEvent    bool
		expectedKind   string
		expectedOptions []string
	}{
		{
			name:           "y/n pattern",
			input:          "Do you want to continue? (y/n)",
			expectEvent:    true,
			expectedKind:   "question",
			expectedOptions: []string{"y", "n"},
		},
		{
			name:           "yes/no pattern",
			input:          "Proceed with operation? (yes/no)",
			expectEvent:    true,
			expectedKind:   "question",
			expectedOptions: []string{"yes", "no"},
		},
		{
			name:           "Y/N uppercase pattern",
			input:          "Confirm action? (Y/N)",
			expectEvent:    true,
			expectedKind:   "question",
			expectedOptions: []string{"y", "n"},
		},
		{
			name:        "no question pattern",
			input:       "This is just regular text",
			expectEvent: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			driver := NewClaudeDriver()
			result, err := driver.Parse([]byte(tt.input))
			
			if err != nil {
				t.Fatalf("Parse error: %v", err)
			}

			if tt.expectEvent {
				if len(result.SmartEvents) == 0 {
					t.Fatal("Expected smart event, got none")
				}
				event := result.SmartEvents[0]
				if event.Kind != tt.expectedKind {
					t.Errorf("Expected kind '%s', got '%s'", tt.expectedKind, event.Kind)
				}
				if len(event.Options) != len(tt.expectedOptions) {
					t.Errorf("Expected %d options, got %d", len(tt.expectedOptions), len(event.Options))
				}
				for i, opt := range tt.expectedOptions {
					if i < len(event.Options) && event.Options[i] != opt {
						t.Errorf("Expected option[%d] '%s', got '%s'", i, opt, event.Options[i])
					}
				}
			} else {
				if len(result.SmartEvents) > 0 {
					t.Errorf("Expected no smart events, got %d", len(result.SmartEvents))
				}
			}
		})
	}
}

// TestClaudeDriver_Parse_ClaudeMenuPattern tests Claude's confirmation menu
func TestClaudeDriver_Parse_ClaudeMenuPattern(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectEvent bool
	}{
		{
			name:        "create file menu",
			input:       "Do you want to create test.txt?",
			expectEvent: true,
		},
		{
			name:        "write file menu",
			input:       "Do you want to write to config.yaml?",
			expectEvent: true,
		},
		{
			name:        "delete file menu",
			input:       "Do you want to delete old_file.js?",
			expectEvent: true,
		},
		{
			name:        "modify file menu",
			input:       "Do you want to modify the database schema?",
			expectEvent: true,
		},
		{
			name:        "no menu pattern",
			input:       "This is a regular question?",
			expectEvent: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			driver := NewClaudeDriver()
			result, err := driver.Parse([]byte(tt.input))
			
			if err != nil {
				t.Fatalf("Parse error: %v", err)
			}

			if tt.expectEvent {
				if len(result.SmartEvents) == 0 {
					t.Fatal("Expected smart event, got none")
				}
				event := result.SmartEvents[0]
				if event.Kind != "claude_confirm" {
					t.Errorf("Expected kind 'claude_confirm', got '%s'", event.Kind)
				}
				expectedOptions := []string{"1", "2", "esc"}
				if len(event.Options) != len(expectedOptions) {
					t.Errorf("Expected %d options, got %d", len(expectedOptions), len(event.Options))
				}
			} else {
				hasClaudeConfirm := false
				for _, event := range result.SmartEvents {
					if event.Kind == "claude_confirm" {
						hasClaudeConfirm = true
						break
					}
				}
				if hasClaudeConfirm {
					t.Error("Expected no claude_confirm event")
				}
			}
		})
	}
}

// TestClaudeDriver_Parse_UserInput tests user command detection
func TestClaudeDriver_Parse_UserInput(t *testing.T) {
	tests := []struct {
		name            string
		input           string
		expectMessage   bool
		expectedType    string
		expectedContent string
	}{
		{
			name:            "user command",
			input:           "> hello world",
			expectMessage:   true,
			expectedType:    "user_input",
			expectedContent: "hello world",
		},
		{
			name:            "slash command",
			input:           "> /doctor",
			expectMessage:   true,
			expectedType:    "user_input",
			expectedContent: "/doctor",
		},
		{
			name:          "no user input",
			input:         "regular text",
			expectMessage: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			driver := NewClaudeDriver()
			result, err := driver.Parse([]byte(tt.input))
			
			if err != nil {
				t.Fatalf("Parse error: %v", err)
			}

			if tt.expectMessage {
				if len(result.Messages) == 0 {
					t.Fatal("Expected message, got none")
				}
				msg := result.Messages[0]
				if msg.Type != tt.expectedType {
					t.Errorf("Expected type '%s', got '%s'", tt.expectedType, msg.Type)
				}
				if msg.Content != tt.expectedContent {
					t.Errorf("Expected content '%s', got '%s'", tt.expectedContent, msg.Content)
				}
			} else {
				if len(result.Messages) > 0 {
					t.Errorf("Expected no messages, got %d", len(result.Messages))
				}
			}
		})
	}
}

// TestClaudeDriver_Parse_ClaudeAction tests Claude action detection
func TestClaudeDriver_Parse_ClaudeAction(t *testing.T) {
	tests := []struct {
		name            string
		input           string
		expectMessage   bool
		expectedContent string
	}{
		{
			name:            "write action",
			input:           "● Write(test.txt)",
			expectMessage:   true,
			expectedContent: "Write(test.txt)",
		},
		{
			name:            "read action",
			input:           "● Read(config.yaml)",
			expectMessage:   true,
			expectedContent: "Read(config.yaml)",
		},
		{
			name:            "edit action",
			input:           "● Edit(main.go)",
			expectMessage:   true,
			expectedContent: "Edit(main.go)",
		},
		{
			name:            "delete action",
			input:           "● Delete(old_file.js)",
			expectMessage:   true,
			expectedContent: "Delete(old_file.js)",
		},
		{
			name:            "bash action",
			input:           "● Bash(ls -la)",
			expectMessage:   true,
			expectedContent: "Bash(ls -la)",
		},
		{
			name:            "search action",
			input:           "● Search(TODO)",
			expectMessage:   true,
			expectedContent: "Search(TODO)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			driver := NewClaudeDriver()
			result, err := driver.Parse([]byte(tt.input))
			
			if err != nil {
				t.Fatalf("Parse error: %v", err)
			}

			if tt.expectMessage {
				if len(result.Messages) == 0 {
					t.Fatal("Expected message, got none")
				}
				msg := result.Messages[0]
				if msg.Type != "claude_action" {
					t.Errorf("Expected type 'claude_action', got '%s'", msg.Type)
				}
				if msg.Content != tt.expectedContent {
					t.Errorf("Expected content '%s', got '%s'", tt.expectedContent, msg.Content)
				}
			}
		})
	}
}

// TestClaudeDriver_Parse_ActionResult tests action result detection
func TestClaudeDriver_Parse_ActionResult(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		expectMessage bool
		expectedType  string
	}{
		{
			name:          "wrote result",
			input:         "⎿ Wrote 10 lines to test.txt",
			expectMessage: true,
			expectedType:  "action_result",
		},
		{
			name:          "created result",
			input:         "⎿ Created new file config.yaml",
			expectMessage: true,
			expectedType:  "action_result",
		},
		{
			name:          "deleted result",
			input:         "⎿ Deleted old_file.js",
			expectMessage: true,
			expectedType:  "action_result",
		},
		{
			name:          "command output",
			input:         "⎿ total 32",
			expectMessage: true,
			expectedType:  "command_output",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			driver := NewClaudeDriver()
			_, err := driver.Parse([]byte(tt.input))
			
			if err != nil {
				t.Fatalf("Parse error: %v", err)
			}

			if tt.expectMessage {
				// Flush to get the collected output
				flushed := driver.Flush()
				if len(flushed) == 0 {
					t.Fatal("Expected flushed message, got none")
				}
				msg := flushed[0]
				if msg.Type != tt.expectedType {
					t.Errorf("Expected type '%s', got '%s'", tt.expectedType, msg.Type)
				}
			}
		})
	}
}

// TestClaudeDriver_FormatInput tests input formatting
func TestClaudeDriver_FormatInput(t *testing.T) {
	tests := []struct {
		name     string
		action   InputAction
		expected string
	}{
		{
			name:     "text input",
			action:   InputAction{Type: "text", Content: "hello"},
			expected: "hello",
		},
		{
			name:     "command input",
			action:   InputAction{Type: "command", Content: "test"},
			expected: "test\r",
		},
		{
			name:     "key enter",
			action:   InputAction{Type: "key", Content: "enter"},
			expected: "\r",
		},
		{
			name:     "key escape",
			action:   InputAction{Type: "key", Content: "esc"},
			expected: "\x1b",
		},
		{
			name:     "key ctrl+c",
			action:   InputAction{Type: "key", Content: "ctrl+c"},
			expected: "\x03",
		},
		{
			name:     "confirm yes",
			action:   InputAction{Type: "confirm", Content: "yes"},
			expected: "1",
		},
		{
			name:     "confirm no",
			action:   InputAction{Type: "confirm", Content: "no"},
			expected: "\x1b",
		},
		{
			name:     "cancel",
			action:   InputAction{Type: "cancel"},
			expected: "\x1b",
		},
		{
			name:     "interrupt",
			action:   InputAction{Type: "interrupt"},
			expected: "\x03",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			driver := NewClaudeDriver()
			result := driver.FormatInput(tt.action)
			
			if string(result) != tt.expected {
				t.Errorf("Expected '%v', got '%v'", []byte(tt.expected), result)
			}
		})
	}
}

// TestClaudeDriver_SendCommand tests command formatting
func TestClaudeDriver_SendCommand(t *testing.T) {
	driver := NewClaudeDriver()
	
	result := driver.SendCommand("hello world")
	expected := "hello world\r"
	
	if string(result) != expected {
		t.Errorf("Expected '%s', got '%s'", expected, string(result))
	}
}

// TestClaudeDriver_SendSlashCommand tests slash command formatting
func TestClaudeDriver_SendSlashCommand(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "without slash",
			input:    "doctor",
			expected: "/doctor\r",
		},
		{
			name:     "with slash",
			input:    "/cost",
			expected: "/cost\r",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			driver := NewClaudeDriver()
			result := driver.SendSlashCommand(tt.input)
			
			if string(result) != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, string(result))
			}
		})
	}
}

// TestClaudeDriver_SelectMenuItem tests menu item selection
func TestClaudeDriver_SelectMenuItem(t *testing.T) {
	tests := []struct {
		name     string
		index    int
		expected string
	}{
		{
			name:     "select 1",
			index:    1,
			expected: "1",
		},
		{
			name:     "select 2",
			index:    2,
			expected: "2",
		},
		{
			name:     "select 9",
			index:    9,
			expected: "9",
		},
		{
			name:     "select 10 with arrows",
			index:    10,
			expected: strings.Repeat("\x1b[B", 9) + "\r",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			driver := NewClaudeDriver()
			result := driver.SelectMenuItem(tt.index)
			
			if string(result) != tt.expected {
				t.Errorf("Expected '%v', got '%v'", []byte(tt.expected), result)
			}
		})
	}
}

// TestClaudeDriver_RespondToEvent tests event response formatting
func TestClaudeDriver_RespondToEvent(t *testing.T) {
	tests := []struct {
		name     string
		event    SmartEvent
		response string
		expected string
	}{
		{
			name: "y/n question - yes",
			event: SmartEvent{
				Kind:    "question",
				Options: []string{"y", "n"},
			},
			response: "yes",
			expected: "y\r",
		},
		{
			name: "y/n question - no",
			event: SmartEvent{
				Kind:    "question",
				Options: []string{"y", "n"},
			},
			response: "no",
			expected: "n\r",
		},
		{
			name: "yes/no question - yes",
			event: SmartEvent{
				Kind:    "question",
				Options: []string{"yes", "no"},
			},
			response: "yes",
			expected: "yes\r",
		},
		{
			name: "claude confirm - yes",
			event: SmartEvent{
				Kind:    "claude_confirm",
				Options: []string{"1", "2", "esc"},
			},
			response: "yes",
			expected: "1",
		},
		{
			name: "claude confirm - all",
			event: SmartEvent{
				Kind:    "claude_confirm",
				Options: []string{"1", "2", "esc"},
			},
			response: "all",
			expected: "2",
		},
		{
			name: "claude confirm - cancel",
			event: SmartEvent{
				Kind:    "claude_confirm",
				Options: []string{"1", "2", "esc"},
			},
			response: "esc",
			expected: "\x1b",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			driver := NewClaudeDriver()
			result := driver.RespondToEvent(tt.event, tt.response)
			
			if string(result) != tt.expected {
				t.Errorf("Expected '%v', got '%v'", []byte(tt.expected), result)
			}
		})
	}
}

// TestClaudeDriver_StripANSI tests ANSI sequence removal
func TestClaudeDriver_StripANSI(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "no ANSI",
			input:    "hello world",
			expected: "hello world",
		},
		{
			name:     "color codes",
			input:    "\x1b[31mRed\x1b[0m Text",
			expected: "Red Text",
		},
		{
			name:     "cursor movement",
			input:    "\x1b[2J\x1b[HClear screen",
			expected: "Clear screen",
		},
		{
			name:     "OSC sequence",
			input:    "\x1b]0;Title\x07Text",
			expected: "Text",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			driver := NewClaudeDriver()
			result := driver.stripANSI([]byte(tt.input))
			
			if string(result) != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, string(result))
			}
		})
	}
}

// TestClaudeDriver_Reset tests buffer reset
func TestClaudeDriver_Reset(t *testing.T) {
	driver := NewClaudeDriver()
	
	// Add some data
	driver.Parse([]byte("test data"))
	
	if driver.buffer.Len() == 0 {
		t.Fatal("Buffer should have data before reset")
	}
	
	// Reset
	driver.Reset()
	
	if driver.buffer.Len() != 0 {
		t.Errorf("Buffer should be empty after reset, got %d bytes", driver.buffer.Len())
	}
}

// TestClaudeDriver_Flush tests flushing pending messages
func TestClaudeDriver_Flush(t *testing.T) {
	driver := NewClaudeDriver()
	
	// Parse some output that creates a pending block
	driver.Parse([]byte("⎿ Wrote file"))
	
	// Flush should return the pending message
	messages := driver.Flush()
	
	if len(messages) == 0 {
		t.Fatal("Expected flushed messages, got none")
	}
	
	msg := messages[0]
	if msg.Type != "action_result" {
		t.Errorf("Expected type 'action_result', got '%s'", msg.Type)
	}
	
	// Second flush should return nothing
	messages = driver.Flush()
	if len(messages) != 0 {
		t.Errorf("Expected no messages on second flush, got %d", len(messages))
	}
}

// TestClaudeDriver_BufferSizeLimit tests buffer size management
func TestClaudeDriver_BufferSizeLimit(t *testing.T) {
	driver := NewClaudeDriver()
	
	// Create data larger than maxBufferSize
	largeData := make([]byte, driver.maxBufferSize+1000)
	for i := range largeData {
		largeData[i] = 'A'
	}
	
	driver.Parse(largeData)
	
	if driver.buffer.Len() > driver.maxBufferSize {
		t.Errorf("Buffer size %d exceeds max %d", driver.buffer.Len(), driver.maxBufferSize)
	}
}

// TestClaudeDriver_MultiLineOutput tests multi-line output collection
func TestClaudeDriver_MultiLineOutput(t *testing.T) {
	driver := NewClaudeDriver()
	
	// Parse diagnostic header
	driver.Parse([]byte("Diagnostics\n"))
	
	// Should start collecting output
	if !driver.inOutputBlock {
		t.Error("Expected to be in output block after 'Diagnostics'")
	}
	
	// Parse more lines
	driver.Parse([]byte("└ Currently running: npm-global (2.0.60)\n"))
	driver.Parse([]byte("└ Path: /usr/local/bin/node\n"))
	
	// Should still be collecting
	if !driver.inOutputBlock {
		t.Error("Expected to still be in output block")
	}
	
	// Flush should return the collected output
	flushed := driver.Flush()
	if len(flushed) == 0 {
		t.Fatal("Expected flushed diagnostic output")
	}
	
	msg := flushed[0]
	if !strings.Contains(msg.Content, "Diagnostics") {
		t.Error("Expected flushed message to contain 'Diagnostics'")
	}
}

// TestClaudeDriver_Deduplication tests message deduplication
func TestClaudeDriver_Deduplication(t *testing.T) {
	driver := NewClaudeDriver()
	
	// Send same user input twice quickly
	result1, _ := driver.Parse([]byte("> hello"))
	result2, _ := driver.Parse([]byte("> hello"))
	
	// First should have message
	if len(result1.Messages) == 0 {
		t.Fatal("Expected message in first parse")
	}
	
	// Second should be deduplicated
	if len(result2.Messages) > 0 {
		t.Error("Expected no message in second parse (should be deduplicated)")
	}
}
