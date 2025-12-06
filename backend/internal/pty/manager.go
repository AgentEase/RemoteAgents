package pty

import (
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/remote-agent-terminal/backend/internal/buffer"
	"github.com/remote-agent-terminal/backend/internal/logger"
	"github.com/remote-agent-terminal/backend/internal/model"
)

const (
	// DefaultRingBufferSize is the default size for the ring buffer (64KB).
	DefaultRingBufferSize = 64 * 1024

	// DefaultReadBufferSize is the buffer size for reading PTY output.
	DefaultReadBufferSize = 4096
)

// PTYProcess represents a running PTY process with associated resources.
type PTYProcess struct {
	ID         string
	Session    *model.Session
	Process    *Process
	RingBuffer *buffer.RingBuffer
	Logger     *logger.AsciinemaLogger

	// OutputCallback is called when PTY produces output.
	// This can be used to broadcast output to WebSocket clients.
	OutputCallback func(data []byte)

	// ExitCallback is called when the process exits.
	ExitCallback func(exitCode int, err error)

	mu       sync.RWMutex
	closed   bool
	closedCh chan struct{}
}

// Manager manages PTY processes for terminal sessions.
type Manager struct {
	processes map[string]*PTYProcess
	mu        sync.RWMutex

	// RingBufferSize is the size of the ring buffer for each process.
	RingBufferSize int

	// LogDir is the directory where log files are stored.
	LogDir string
}

// NewManager creates a new PTY manager.
func NewManager(logDir string) *Manager {
	return &Manager{
		processes:      make(map[string]*PTYProcess),
		RingBufferSize: DefaultRingBufferSize,
		LogDir:         logDir,
	}
}

// SpawnOptions contains options for spawning a PTY process.
type SpawnOptions struct {
	// Session is the session metadata.
	Session *model.Session

	// InitialRows is the initial number of rows.
	InitialRows uint16

	// InitialCols is the initial number of columns.
	InitialCols uint16

	// OutputCallback is called when PTY produces output.
	OutputCallback func(data []byte)

	// ExitCallback is called when the process exits.
	ExitCallback func(exitCode int, err error)
}

// Spawn creates and starts a new PTY process for the given session.
func (m *Manager) Spawn(ctx context.Context, opts SpawnOptions) (*PTYProcess, error) {
	if opts.Session == nil {
		return nil, fmt.Errorf("session is required")
	}

	if opts.Session.Command == "" {
		return nil, model.ErrCommandRequired
	}

	// Set default terminal size
	if opts.InitialRows == 0 {
		opts.InitialRows = 24
	}
	if opts.InitialCols == 0 {
		opts.InitialCols = 80
	}

	// Prepare environment variables
	var env []string
	if opts.Session.Env != nil {
		for k, v := range opts.Session.Env {
			env = append(env, fmt.Sprintf("%s=%s", k, v))
		}
	}

	// Create the Asciinema logger
	var asciinemaLogger *logger.AsciinemaLogger
	if opts.Session.LogFilePath != "" {
		var err error
		asciinemaLogger, err = logger.NewAsciinemaLogger(opts.Session.LogFilePath)
		if err != nil {
			return nil, fmt.Errorf("failed to create logger: %w", err)
		}

		// Write the header
		if err := asciinemaLogger.WriteHeader(int(opts.InitialCols), int(opts.InitialRows)); err != nil {
			asciinemaLogger.Close()
			return nil, fmt.Errorf("failed to write logger header: %w", err)
		}
	}

	// Start the PTY process
	process, err := Start(StartOptions{
		Command:     opts.Session.Command,
		Env:         env,
		InitialRows: opts.InitialRows,
		InitialCols: opts.InitialCols,
	})
	if err != nil {
		if asciinemaLogger != nil {
			asciinemaLogger.Close()
		}
		return nil, fmt.Errorf("failed to start PTY: %w", err)
	}

	// Create the PTY process wrapper
	ptyProcess := &PTYProcess{
		ID:             opts.Session.ID,
		Session:        opts.Session,
		Process:        process,
		RingBuffer:     buffer.NewRingBuffer(m.RingBufferSize),
		Logger:         asciinemaLogger,
		OutputCallback: opts.OutputCallback,
		ExitCallback:   opts.ExitCallback,
		closedCh:       make(chan struct{}),
	}

	// Register the process
	m.mu.Lock()
	m.processes[opts.Session.ID] = ptyProcess
	m.mu.Unlock()

	// Start the output reader goroutine
	go ptyProcess.readLoop()

	// Start the wait goroutine
	go ptyProcess.waitLoop(m)

	return ptyProcess, nil
}

// Get returns the PTY process for the given session ID.
func (m *Manager) Get(id string) (*PTYProcess, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	p, ok := m.processes[id]
	return p, ok
}

// Kill terminates the PTY process for the given session ID.
func (m *Manager) Kill(id string) error {
	m.mu.RLock()
	p, ok := m.processes[id]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("process not found: %s", id)
	}

	return p.Close()
}

// Resize changes the PTY window size for the given session ID.
func (m *Manager) Resize(id string, rows, cols uint16) error {
	m.mu.RLock()
	p, ok := m.processes[id]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("process not found: %s", id)
	}

	return p.Resize(rows, cols)
}

// Write writes data to the PTY input for the given session ID.
func (m *Manager) Write(id string, data []byte) error {
	m.mu.RLock()
	p, ok := m.processes[id]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("process not found: %s", id)
	}

	return p.Write(data)
}

// Remove removes the process from the manager.
// This should be called after the process has exited.
func (m *Manager) Remove(id string) {
	m.mu.Lock()
	delete(m.processes, id)
	m.mu.Unlock()
}

// List returns all active PTY processes.
func (m *Manager) List() []*PTYProcess {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*PTYProcess, 0, len(m.processes))
	for _, p := range m.processes {
		result = append(result, p)
	}
	return result
}

// Close closes all PTY processes and releases resources.
func (m *Manager) Close() error {
	m.mu.Lock()
	processes := make([]*PTYProcess, 0, len(m.processes))
	for _, p := range m.processes {
		processes = append(processes, p)
	}
	m.mu.Unlock()

	var firstErr error
	for _, p := range processes {
		if err := p.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}

	return firstErr
}

// readLoop reads output from the PTY and distributes it.
func (p *PTYProcess) readLoop() {
	buf := make([]byte, DefaultReadBufferSize)

	for {
		n, err := p.Process.PTY.Read(buf)
		if err != nil {
			if err != io.EOF {
				// Log error but don't propagate
			}
			return
		}

		if n > 0 {
			data := buf[:n]

			// Write to ring buffer for hot restore
			p.RingBuffer.Write(data)

			// Write to logger
			if p.Logger != nil {
				p.Logger.WriteOutput(data)
			}

			// Call output callback (for WebSocket broadcast)
			if p.OutputCallback != nil {
				p.OutputCallback(data)
			}
		}
	}
}

// waitLoop waits for the process to exit and handles cleanup.
func (p *PTYProcess) waitLoop(m *Manager) {
	exitCode, err := p.Process.Wait()

	// Call exit callback
	if p.ExitCallback != nil {
		p.ExitCallback(exitCode, err)
	}

	// Close resources
	p.Close()

	// Remove from manager
	m.Remove(p.ID)
}

// Write writes data to the PTY input.
func (p *PTYProcess) Write(data []byte) error {
	p.mu.RLock()
	if p.closed {
		p.mu.RUnlock()
		return fmt.Errorf("process is closed")
	}
	p.mu.RUnlock()

	_, err := p.Process.PTY.Write(data)
	if err != nil {
		return fmt.Errorf("failed to write to PTY: %w", err)
	}

	// Log input
	if p.Logger != nil {
		p.Logger.WriteInput(data)
	}

	return nil
}

// Resize changes the PTY window size.
func (p *PTYProcess) Resize(rows, cols uint16) error {
	p.mu.RLock()
	if p.closed {
		p.mu.RUnlock()
		return fmt.Errorf("process is closed")
	}
	p.mu.RUnlock()

	return p.Process.PTY.Resize(rows, cols)
}

// Close closes the PTY process and releases resources.
func (p *PTYProcess) Close() error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	close(p.closedCh)
	p.mu.Unlock()

	var firstErr error

	// Kill the process
	if err := p.Process.Kill(); err != nil && firstErr == nil {
		firstErr = err
	}

	// Close the PTY
	if err := p.Process.Close(); err != nil && firstErr == nil {
		firstErr = err
	}

	// Close the logger
	if p.Logger != nil {
		if err := p.Logger.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}

	return firstErr
}

// IsClosed returns true if the process has been closed.
func (p *PTYProcess) IsClosed() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.closed
}

// ClosedChan returns a channel that is closed when the process exits.
func (p *PTYProcess) ClosedChan() <-chan struct{} {
	return p.closedCh
}

// GetHistory returns the buffered output history from the ring buffer.
func (p *PTYProcess) GetHistory() []byte {
	return p.RingBuffer.ReadAll()
}

// PID returns the process ID.
func (p *PTYProcess) PID() int {
	return p.Process.PID()
}
