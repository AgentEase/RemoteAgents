// Package pty provides cross-platform PTY (pseudo-terminal) management.
package pty

import (
	"io"
	"os/exec"
)

// PTY represents a platform-independent pseudo-terminal interface.
// It provides methods for interacting with a PTY process.
type PTY interface {
	// Read reads data from the PTY output.
	io.Reader

	// Write writes data to the PTY input.
	io.Writer

	// Close closes the PTY and releases resources.
	io.Closer

	// Resize changes the PTY window size to the specified dimensions.
	Resize(rows, cols uint16) error

	// Fd returns the file descriptor of the PTY master.
	// This is used for platform-specific operations.
	Fd() uintptr
}

// StartOptions contains options for starting a PTY process.
type StartOptions struct {
	// Command is the command to execute.
	Command string

	// Args are the arguments to pass to the command.
	Args []string

	// Env is the environment variables for the process.
	// If nil, the current process environment is used.
	Env []string

	// Dir is the working directory for the process.
	// If empty, the current directory is used.
	Dir string

	// InitialRows is the initial number of rows for the PTY.
	InitialRows uint16

	// InitialCols is the initial number of columns for the PTY.
	InitialCols uint16
}

// Process represents a running PTY process.
type Process struct {
	// PTY is the pseudo-terminal interface.
	PTY PTY

	// Cmd is the underlying exec.Cmd.
	Cmd *exec.Cmd

	// pid is the process ID.
	pid int
}

// PID returns the process ID of the running process.
func (p *Process) PID() int {
	return p.pid
}

// Wait waits for the process to exit and returns the exit code.
// Returns -1 if the process was killed by a signal.
func (p *Process) Wait() (int, error) {
	err := p.Cmd.Wait()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode(), nil
		}
		return -1, err
	}
	return 0, nil
}

// Kill terminates the process.
func (p *Process) Kill() error {
	if p.Cmd.Process != nil {
		return p.Cmd.Process.Kill()
	}
	return nil
}

// Close closes the PTY and releases all resources.
func (p *Process) Close() error {
	return p.PTY.Close()
}
