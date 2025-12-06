package driver

import (
	"strings"
	"testing"
)

func TestClaudeDriver_Name(t *testing.T) {
	driver := NewClaudeDriver()
	if driver.Name() != "claude" {
		t.Errorf("expected name 'claude', got '%s'", driver.Name())
	}
}

func TestClaudeDriver_Parse_QuestionPattern(t *testing.T) {
	driver := NewClaudeDriver()

	testCases := []struct {
		name            string
		input           []byte
		expectEvent     bool
		expectedKind    string
		expectedOptions []string
	}{
		{
			name:            "y/n question",
			input:           []byte("Continue? (y/n)"),
			expectEvent:     true,
			expectedKind:    "question",
			expectedOptions: []string{"y", "n"},
		},
		{
			name:            "Y/N question",
			input:           []byte("Proceed? (Y/N)"),
			expectEvent:     true,
			expectedKind:    "question",
			expectedOptions: []string{"y", "n"},
		},
		{
			name:            "yes/no question",
			input:           []byte("Do you want to continue? (yes/no)"),
			expectEvent:     true,
			expectedKind:    "question",
			expectedOptions: []string{"yes", "no"},
		},
		{
			name:            "Yes/No question",
			input:           []byte("Confirm action? (Yes/No)"),
			expectEvent:     true,
			expectedKind:    "question",
			expectedOptions: []string{"yes", "no"},
		},
		{
			name:        "no question pattern",
			input:       []byte("This is just regular output"),
			expectEvent: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Reset driver for each test
			driver.Reset()

			result, err := driver.Parse(tc.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result == nil {
				t.Fatal("result is nil")
			}

			// Check raw data is preserved
			if string(result.RawData) != string(tc.input) {
				t.Errorf("expected raw data '%s', got '%s'", string(tc.input), string(result.RawData))
			}

			// Check smart events
			if tc.expectEvent {
				if len(result.SmartEvents) == 0 {
					t.Fatal("expected smart event, got none")
				}

				event := result.SmartEvents[0]
				if event.Kind != tc.expectedKind {
					t.Errorf("expected kind '%s', got '%s'", tc.expectedKind, event.Kind)
				}

				if len(event.Options) != len(tc.expectedOptions) {
					t.Errorf("expected %d options, got %d", len(tc.expectedOptions), len(event.Options))
				} else {
					for i, opt := range tc.expectedOptions {
						if event.Options[i] != opt {
							t.Errorf("expected option[%d] '%s', got '%s'", i, opt, event.Options[i])
						}
					}
				}

				if event.Prompt == "" {
					t.Error("expected non-empty prompt")
				}
			} else {
				// For idle patterns, we might still get events
				// Only check if we explicitly don't expect any events
				if tc.name == "no question pattern" && len(result.SmartEvents) > 0 {
					// Check if it's not an idle event
					for _, event := range result.SmartEvents {
						if event.Kind == "question" {
							t.Errorf("unexpected question event: %+v", event)
						}
					}
				}
			}
		})
	}
}

func TestClaudeDriver_Parse_IdlePattern(t *testing.T) {
	// NOTE: Idle pattern detection is disabled in ClaudeDriver to avoid
	// false positives with Claude Code's UI elements. These tests are
	// kept for documentation but marked as skipped.
	t.Skip("Idle pattern detection is disabled in ClaudeDriver")

	driver := NewClaudeDriver()

	testCases := []struct {
		name         string
		input        []byte
		expectIdle   bool
		expectedKind string
	}{
		{
			name:         "question mark prompt",
			input:        []byte("Enter your choice? "),
			expectIdle:   true,
			expectedKind: "idle",
		},
		{
			name:         "greater than prompt",
			input:        []byte("> "),
			expectIdle:   true,
			expectedKind: "idle",
		},
		{
			name:         "dollar prompt",
			input:        []byte("$ "),
			expectIdle:   true,
			expectedKind: "idle",
		},
		{
			name:         "Continue prompt",
			input:        []byte("Continue? "),
			expectIdle:   true,
			expectedKind: "idle",
		},
		{
			name:         "Proceed prompt",
			input:        []byte("Proceed? "),
			expectIdle:   true,
			expectedKind: "idle",
		},
		{
			name:       "no idle pattern",
			input:      []byte("This is regular output\n"),
			expectIdle: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Reset driver for each test
			driver.Reset()

			result, err := driver.Parse(tc.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result == nil {
				t.Fatal("result is nil")
			}

			// Check for idle events
			if tc.expectIdle {
				foundIdle := false
				for _, event := range result.SmartEvents {
					if event.Kind == "idle" {
						foundIdle = true
						if event.Prompt == "" {
							t.Error("expected non-empty prompt for idle event")
						}
						break
					}
				}
				if !foundIdle {
					t.Error("expected idle event, got none")
				}
			}
		})
	}
}

func TestClaudeDriver_BufferManagement(t *testing.T) {
	driver := NewClaudeDriver()

	// Write data larger than buffer size
	largeData := make([]byte, driver.maxBufferSize+1000)
	for i := range largeData {
		largeData[i] = 'A'
	}

	result, err := driver.Parse(largeData)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result == nil {
		t.Fatal("result is nil")
	}

	// Buffer should be trimmed to max size
	if driver.buffer.Len() > driver.maxBufferSize {
		t.Errorf("buffer size %d exceeds max size %d", driver.buffer.Len(), driver.maxBufferSize)
	}
}

func TestClaudeDriver_Reset(t *testing.T) {
	driver := NewClaudeDriver()

	// Write some data
	driver.Parse([]byte("Some data"))

	if driver.buffer.Len() == 0 {
		t.Fatal("buffer should not be empty after parse")
	}

	// Reset
	driver.Reset()

	if driver.buffer.Len() != 0 {
		t.Errorf("buffer should be empty after reset, got length %d", driver.buffer.Len())
	}
}

func TestClaudeDriver_MultipleChunks(t *testing.T) {
	driver := NewClaudeDriver()

	// Simulate receiving output in multiple chunks
	chunks := [][]byte{
		[]byte("Do you want to "),
		[]byte("continue? "),
		[]byte("(y/n)"),
	}

	var lastResult *ParseResult
	for _, chunk := range chunks {
		result, err := driver.Parse(chunk)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		lastResult = result
	}

	// After all chunks, we should detect the question pattern
	foundQuestion := false
	for _, event := range lastResult.SmartEvents {
		if event.Kind == "question" {
			foundQuestion = true
			if len(event.Options) != 2 || event.Options[0] != "y" || event.Options[1] != "n" {
				t.Errorf("expected options [y, n], got %v", event.Options)
			}
			break
		}
	}

	if !foundQuestion {
		t.Error("expected to detect question pattern after multiple chunks")
	}
}


func TestClaudeDriver_Parse_Messages(t *testing.T) {
	driver := NewClaudeDriver()

	testCases := []struct {
		name            string
		input           []byte
		expectedMsgType string
		expectedContent string
	}{
		{
			name:            "user input",
			input:           []byte("> hello world\n"),
			expectedMsgType: "user_input",
			expectedContent: "hello world",
		},
		{
			name:            "claude response",
			input:           []byte("● This is a response from Claude.\n"),
			expectedMsgType: "claude_response",
			expectedContent: "This is a response from Claude.",
		},
		{
			name:            "claude action write",
			input:           []byte("● Write(test.txt)\n"),
			expectedMsgType: "claude_action",
			expectedContent: "Write(test.txt)",
		},
		{
			name:            "claude action read",
			input:           []byte("● Read(config.json)\n"),
			expectedMsgType: "claude_action",
			expectedContent: "Read(config.json)",
		},
		{
			name:            "agent interrupted",
			input:           []byte("⎿  Interrupted · What should Claude do instead?\n"),
			expectedMsgType: "agent_interrupted",
			expectedContent: "⎿  Interrupted · What should Claude do instead?",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			driver.Reset()
			// Reset deduplication state
			driver.lastUserInput = ""
			driver.lastClaudeAction = ""
			driver.lastResponse = ""
			driver.lastActionResult = ""

			result, err := driver.Parse(tc.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(result.Messages) == 0 {
				t.Fatalf("expected at least one message, got none")
			}

			msg := result.Messages[0]
			if msg.Type != tc.expectedMsgType {
				t.Errorf("expected message type %q, got %q", tc.expectedMsgType, msg.Type)
			}
			if msg.Content != tc.expectedContent {
				t.Errorf("expected content %q, got %q", tc.expectedContent, msg.Content)
			}
		})
	}
}

func TestClaudeDriver_Parse_Messages_BufferedOutput(t *testing.T) {
	// Test buffered output (⎿ results) - these are collected and flushed
	testCases := []struct {
		name            string
		input           []byte
		expectedMsgType string
		expectedContent string
	}{
		{
			name:            "action result wrote",
			input:           []byte("⎿  Wrote 10 lines to test.txt\n"),
			expectedMsgType: "action_result",
			expectedContent: "Wrote 10 lines to test.txt",
		},
		{
			name:            "command output",
			input:           []byte("⎿  Total cost: $0.01\n"),
			expectedMsgType: "command_output",
			expectedContent: "Total cost: $0.01",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			driver := NewClaudeDriver()

			// Parse the output (will be buffered)
			result, err := driver.Parse(tc.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Buffered output won't appear immediately
			// Need to flush or send a new prompt to get it
			flushed := driver.Flush()
			allMessages := append(result.Messages, flushed...)

			if len(allMessages) == 0 {
				t.Fatalf("expected at least one message after flush, got none")
			}

			msg := allMessages[0]
			if msg.Type != tc.expectedMsgType {
				t.Errorf("expected message type %q, got %q", tc.expectedMsgType, msg.Type)
			}
			if msg.Content != tc.expectedContent {
				t.Errorf("expected content %q, got %q", tc.expectedContent, msg.Content)
			}
		})
	}
}

func TestClaudeDriver_Parse_Messages_Deduplication(t *testing.T) {
	driver := NewClaudeDriver()

	// Parse the same user input twice
	input := []byte("> hello world\n")

	result1, _ := driver.Parse(input)
	if len(result1.Messages) != 1 {
		t.Fatalf("expected 1 message on first parse, got %d", len(result1.Messages))
	}

	result2, _ := driver.Parse(input)
	if len(result2.Messages) != 0 {
		t.Errorf("expected 0 messages on duplicate parse, got %d", len(result2.Messages))
	}
}

func TestClaudeDriver_Parse_Messages_UIFiltering(t *testing.T) {
	driver := NewClaudeDriver()

	// These should be filtered out as UI noise
	uiNoiseInputs := [][]byte{
		[]byte("· Thinking…\n"),
		[]byte("─────────────────\n"),
		[]byte("1. Yes\n"),
		[]byte("2. No\n"),
		[]byte("↓ More options\n"),
		[]byte("Esc to cancel\n"),
	}

	for _, input := range uiNoiseInputs {
		driver.Reset()
		result, _ := driver.Parse(input)
		if len(result.Messages) > 0 {
			t.Errorf("expected UI noise %q to be filtered, but got message: %+v", string(input), result.Messages[0])
		}
	}
}

func TestClaudeDriver_Parse_Messages_TreeOutput(t *testing.T) {
	// Test /doctor style tree output - collected and merged
	t.Run("doctor multi-line output", func(t *testing.T) {
		driver := NewClaudeDriver()

		// Simulate /doctor output coming in chunks
		driver.Parse([]byte("Diagnostics\n"))
		driver.Parse([]byte("└ Currently running: npm-global (2.0.59)\n"))
		driver.Parse([]byte("└ Path: /home/user/.nvm/versions/node/v24.11.1/bin/node\n"))
		driver.Parse([]byte("└ Auto-updates: default (true)\n"))

		// Flush to get the collected output
		flushed := driver.Flush()

		if len(flushed) == 0 {
			t.Fatalf("expected tree output to be captured, got no messages")
		}

		msg := flushed[0]
		if msg.Type != "command_output" {
			t.Errorf("expected message type 'command_output', got %q", msg.Type)
		}

		// Should contain all lines merged
		expectedLines := []string{
			"Diagnostics:",
			"Currently running: npm-global (2.0.59)",
			"Path: /home/user/.nvm/versions/node/v24.11.1/bin/node",
			"Auto-updates: default (true)",
		}
		for _, line := range expectedLines {
			if !strings.Contains(msg.Content, line) {
				t.Errorf("expected content to contain %q, got %q", line, msg.Content)
			}
		}
	})

	t.Run("doctor output flushed on new prompt", func(t *testing.T) {
		driver := NewClaudeDriver()

		// Simulate /doctor output followed by new prompt
		driver.Parse([]byte("Diagnostics\n"))
		driver.Parse([]byte("└ Currently running: npm-global (2.0.59)\n"))
		result, _ := driver.Parse([]byte("> next command\n"))

		// Should have flushed the doctor output and captured the new command
		if len(result.Messages) < 2 {
			t.Fatalf("expected at least 2 messages, got %d", len(result.Messages))
		}

		// First message should be the doctor output
		if result.Messages[0].Type != "command_output" {
			t.Errorf("expected first message type 'command_output', got %q", result.Messages[0].Type)
		}

		// Second message should be the user input
		if result.Messages[1].Type != "user_input" {
			t.Errorf("expected second message type 'user_input', got %q", result.Messages[1].Type)
		}
	})
}


func TestClaudeDriver_Parse_Messages_ResumeSession(t *testing.T) {
	driver := NewClaudeDriver()

	// Simulate resume session flow
	driver.Parse([]byte("Resume Session\n"))
	driver.Parse([]byte("❯ good, testing cursor move\n"))
	driver.Parse([]byte("3 minutes ago · 16 messages · main\n"))
	result, _ := driver.Parse([]byte("> next command\n"))

	// Should have session_resumed and user_input messages
	var foundResumed, foundUserInput bool
	for _, msg := range result.Messages {
		if msg.Type == "session_resumed" {
			foundResumed = true
			if !strings.Contains(msg.Content, "good, testing cursor move") {
				t.Errorf("expected session_resumed to contain selection, got %q", msg.Content)
			}
			if !strings.Contains(msg.Content, "3 minutes ago") {
				t.Errorf("expected session_resumed to contain timestamp, got %q", msg.Content)
			}
		}
		if msg.Type == "user_input" {
			foundUserInput = true
		}
	}

	if !foundResumed {
		t.Error("expected session_resumed message")
	}
	if !foundUserInput {
		t.Error("expected user_input message")
	}
}
