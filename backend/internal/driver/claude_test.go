package driver

import (
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
