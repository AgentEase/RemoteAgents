package logger

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// AsciinemaHeader represents the header of an Asciinema v2 recording.
type AsciinemaHeader struct {
	Version   int               `json:"version"`
	Width     int               `json:"width"`
	Height    int               `json:"height"`
	Timestamp int64             `json:"timestamp"`
	Env       map[string]string `json:"env,omitempty"`
}

// AsciinemaEvent represents a single event in an Asciinema v2 recording.
// Format: [time_offset, event_type, data]
type AsciinemaEvent struct {
	TimeOffset float64
	EventType  string // "o" for output, "i" for input
	Data       string
}

// MarshalJSON implements custom JSON marshaling for AsciinemaEvent.
func (e AsciinemaEvent) MarshalJSON() ([]byte, error) {
	return json.Marshal([]interface{}{e.TimeOffset, e.EventType, e.Data})
}

// UnmarshalJSON implements custom JSON unmarshaling for AsciinemaEvent.
func (e *AsciinemaEvent) UnmarshalJSON(data []byte) error {
	var arr []interface{}
	if err := json.Unmarshal(data, &arr); err != nil {
		return err
	}
	if len(arr) != 3 {
		return fmt.Errorf("invalid event format: expected 3 elements, got %d", len(arr))
	}

	timeOffset, ok := arr[0].(float64)
	if !ok {
		return fmt.Errorf("invalid time offset type")
	}
	e.TimeOffset = timeOffset

	eventType, ok := arr[1].(string)
	if !ok {
		return fmt.Errorf("invalid event type")
	}
	e.EventType = eventType

	eventData, ok := arr[2].(string)
	if !ok {
		return fmt.Errorf("invalid event data type")
	}
	e.Data = eventData

	return nil
}


// AsciinemaLogger records terminal sessions in Asciinema v2 JSON-Lines format.
type AsciinemaLogger struct {
	writer    io.Writer
	file      *os.File // only set if we own the file
	startTime time.Time
	mu        sync.Mutex
}

// NewAsciinemaLogger creates a new AsciinemaLogger that writes to the given file path.
func NewAsciinemaLogger(filePath string) (*AsciinemaLogger, error) {
	file, err := os.Create(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to create log file: %w", err)
	}

	return &AsciinemaLogger{
		writer:    file,
		file:      file,
		startTime: time.Now(),
	}, nil
}

// NewAsciinemaLoggerWithWriter creates a new AsciinemaLogger that writes to the given writer.
// This is useful for testing.
func NewAsciinemaLoggerWithWriter(w io.Writer) *AsciinemaLogger {
	return &AsciinemaLogger{
		writer:    w,
		startTime: time.Now(),
	}
}

// WriteHeader writes the Asciinema v2 header to the log file.
// This should be called once at the beginning of the recording.
func (l *AsciinemaLogger) WriteHeader(cols, rows int) error {
	return l.WriteHeaderWithEnv(cols, rows, nil)
}

// WriteHeaderWithEnv writes the Asciinema v2 header with environment variables.
func (l *AsciinemaLogger) WriteHeaderWithEnv(cols, rows int, env map[string]string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	header := AsciinemaHeader{
		Version:   2,
		Width:     cols,
		Height:    rows,
		Timestamp: l.startTime.Unix(),
		Env:       env,
	}

	data, err := json.Marshal(header)
	if err != nil {
		return fmt.Errorf("failed to marshal header: %w", err)
	}

	if _, err := l.writer.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("failed to write header: %w", err)
	}

	return nil
}

// WriteOutput writes an output event ("o") to the log file.
func (l *AsciinemaLogger) WriteOutput(data []byte) error {
	return l.writeEvent("o", data)
}

// WriteInput writes an input event ("i") to the log file.
func (l *AsciinemaLogger) WriteInput(data []byte) error {
	return l.writeEvent("i", data)
}

// writeEvent writes an event to the log file with the given type.
func (l *AsciinemaLogger) writeEvent(eventType string, data []byte) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	timeOffset := time.Since(l.startTime).Seconds()

	event := AsciinemaEvent{
		TimeOffset: timeOffset,
		EventType:  eventType,
		Data:       string(data),
	}

	eventData, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	if _, err := l.writer.Write(append(eventData, '\n')); err != nil {
		return fmt.Errorf("failed to write event: %w", err)
	}

	return nil
}

// Close closes the log file.
func (l *AsciinemaLogger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.file != nil {
		return l.file.Close()
	}
	return nil
}

// StartTime returns the start time of the recording.
func (l *AsciinemaLogger) StartTime() time.Time {
	return l.startTime
}
