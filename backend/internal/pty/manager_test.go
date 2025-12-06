package pty

import (
	"testing"
)

// TestKeyConstants tests that key constants are correct
func TestKeyConstants(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		expected string
	}{
		{"KeyCtrlU", KeyCtrlU, "\x15"},
		{"KeyEnter", KeyEnter, "\r"},
		{"KeyCtrlC", KeyCtrlC, "\x03"},
		{"KeyEscape", KeyEscape, "\x1b"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.key != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, tt.key)
			}
		})
	}
}

// TestDelayConstants tests that delay constants are reasonable
func TestDelayConstants(t *testing.T) {
	// Delays should be positive and reasonable (100-1000ms)
	delays := []struct {
		name  string
		value int
	}{
		{"InputClearDelay", InputClearDelay},
		{"InputTextDelay", InputTextDelay},
		{"DismissDelay", DismissDelay},
	}

	for _, d := range delays {
		t.Run(d.name, func(t *testing.T) {
			if d.value < 100 || d.value > 1000 {
				t.Errorf("Delay %s=%d is outside reasonable range (100-1000ms)", d.name, d.value)
			}
		})
	}
}

// TestNewManager tests manager creation
func TestNewManager(t *testing.T) {
	manager := NewManager("/tmp/logs")

	if manager == nil {
		t.Fatal("Expected non-nil manager")
	}

	if manager.LogDir != "/tmp/logs" {
		t.Errorf("Expected LogDir '/tmp/logs', got '%s'", manager.LogDir)
	}

	if manager.RingBufferSize != DefaultRingBufferSize {
		t.Errorf("Expected RingBufferSize %d, got %d", DefaultRingBufferSize, manager.RingBufferSize)
	}

	if manager.processes == nil {
		t.Error("Expected non-nil processes map")
	}
}

// TestManagerGetNotFound tests Get with non-existent ID
func TestManagerGetNotFound(t *testing.T) {
	manager := NewManager("/tmp/logs")

	_, ok := manager.Get("non-existent")
	if ok {
		t.Error("Expected Get to return false for non-existent ID")
	}
}

// TestManagerKillNotFound tests Kill with non-existent ID
func TestManagerKillNotFound(t *testing.T) {
	manager := NewManager("/tmp/logs")

	err := manager.Kill("non-existent")
	if err == nil {
		t.Error("Expected error for non-existent ID")
	}
}

// TestManagerResizeNotFound tests Resize with non-existent ID
func TestManagerResizeNotFound(t *testing.T) {
	manager := NewManager("/tmp/logs")

	err := manager.Resize("non-existent", 24, 80)
	if err == nil {
		t.Error("Expected error for non-existent ID")
	}
}

// TestManagerWriteNotFound tests Write with non-existent ID
func TestManagerWriteNotFound(t *testing.T) {
	manager := NewManager("/tmp/logs")

	err := manager.Write("non-existent", []byte("test"))
	if err == nil {
		t.Error("Expected error for non-existent ID")
	}
}

// TestManagerWriteCommandNotFound tests WriteCommand with non-existent ID
func TestManagerWriteCommandNotFound(t *testing.T) {
	manager := NewManager("/tmp/logs")

	err := manager.WriteCommand("non-existent", []byte("test"))
	if err == nil {
		t.Error("Expected error for non-existent ID")
	}
}

// TestManagerDismissOutputNotFound tests DismissOutput with non-existent ID
func TestManagerDismissOutputNotFound(t *testing.T) {
	manager := NewManager("/tmp/logs")

	err := manager.DismissOutput("non-existent")
	if err == nil {
		t.Error("Expected error for non-existent ID")
	}
}

// TestManagerList tests List with empty manager
func TestManagerList(t *testing.T) {
	manager := NewManager("/tmp/logs")

	list := manager.List()
	if len(list) != 0 {
		t.Errorf("Expected empty list, got %d items", len(list))
	}
}

// TestManagerClose tests Close with empty manager
func TestManagerClose(t *testing.T) {
	manager := NewManager("/tmp/logs")

	err := manager.Close()
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
}

// TestDefaultConstants tests default constant values
func TestDefaultConstants(t *testing.T) {
	if DefaultRingBufferSize != 64*1024 {
		t.Errorf("Expected DefaultRingBufferSize 64KB, got %d", DefaultRingBufferSize)
	}

	if DefaultReadBufferSize != 4096 {
		t.Errorf("Expected DefaultReadBufferSize 4096, got %d", DefaultReadBufferSize)
	}
}
