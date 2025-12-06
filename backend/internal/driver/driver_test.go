package driver

import (
	"testing"
)

func TestGenericDriver_Name(t *testing.T) {
	driver := NewGenericDriver()
	if driver.Name() != "generic" {
		t.Errorf("expected name 'generic', got '%s'", driver.Name())
	}
}

func TestGenericDriver_Parse(t *testing.T) {
	driver := NewGenericDriver()

	testCases := []struct {
		name  string
		input []byte
	}{
		{
			name:  "simple text",
			input: []byte("Hello, world!"),
		},
		{
			name:  "with ANSI codes",
			input: []byte("\x1b[31mRed text\x1b[0m"),
		},
		{
			name:  "empty input",
			input: []byte{},
		},
		{
			name:  "with question pattern",
			input: []byte("Continue? (y/n)"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := driver.Parse(tc.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result == nil {
				t.Fatal("result is nil")
			}

			// GenericDriver should return raw data unchanged
			if string(result.RawData) != string(tc.input) {
				t.Errorf("expected raw data '%s', got '%s'", string(tc.input), string(result.RawData))
			}

			// GenericDriver should not generate any smart events
			if len(result.SmartEvents) != 0 {
				t.Errorf("expected no smart events, got %d", len(result.SmartEvents))
			}
		})
	}
}
