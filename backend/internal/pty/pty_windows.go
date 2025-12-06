//go:build windows
// +build windows

package pty

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	kernel32                     = windows.NewLazySystemDLL("kernel32.dll")
	procCreatePseudoConsole      = kernel32.NewProc("CreatePseudoConsole")
	procResizePseudoConsole      = kernel32.NewProc("ResizePseudoConsole")
	procClosePseudoConsole       = kernel32.NewProc("ClosePseudoConsole")
	procInitializeProcThreadAttr = kernel32.NewProc("InitializeProcThreadAttributeList")
	procUpdateProcThreadAttr     = kernel32.NewProc("UpdateProcThreadAttribute")
	procDeleteProcThreadAttr     = kernel32.NewProc("DeleteProcThreadAttributeList")
)

const (
	PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE = 0x00020016
)

// windowsPTY implements the PTY interface for Windows using ConPTY.
type windowsPTY struct {
	hPC        windows.Handle // Pseudo console handle
	inputRead  *os.File       // Read end of input pipe (for reading from PTY)
	outputWrite *os.File      // Write end of output pipe (for writing to PTY)
}

// Read reads data from the PTY output.
func (p *windowsPTY) Read(b []byte) (int, error) {
	return p.inputRead.Read(b)
}

// Write writes data to the PTY input.
func (p *windowsPTY) Write(b []byte) (int, error) {
	return p.outputWrite.Write(b)
}

// Close closes the PTY and releases resources.
func (p *windowsPTY) Close() error {
	var firstErr error

	if p.inputRead != nil {
		if err := p.inputRead.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}

	if p.outputWrite != nil {
		if err := p.outputWrite.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}

	if p.hPC != 0 {
		procClosePseudoConsole.Call(uintptr(p.hPC))
	}

	return firstErr
}

// Fd returns the file descriptor (not applicable for Windows ConPTY).
func (p *windowsPTY) Fd() uintptr {
	return uintptr(p.hPC)
}

// Resize changes the PTY window size.
func (p *windowsPTY) Resize(rows, cols uint16) error {
	size := (int32(rows) << 16) | int32(cols)
	ret, _, err := procResizePseudoConsole.Call(uintptr(p.hPC), uintptr(size))
	if ret != 0 {
		return fmt.Errorf("ResizePseudoConsole failed: %w", err)
	}
	return nil
}

// Start starts a new PTY process with the given options.
// This is the Windows implementation using ConPTY.
func Start(opts StartOptions) (*Process, error) {
	// Check if ConPTY is available (Windows 10 1809+)
	if err := procCreatePseudoConsole.Find(); err != nil {
		return nil, fmt.Errorf("ConPTY not available: %w", err)
	}

	// Create pipes for PTY communication
	// inputRead/inputWrite: PTY output -> our read
	// outputRead/outputWrite: our write -> PTY input
	var inputRead, inputWrite, outputRead, outputWrite windows.Handle

	if err := windows.CreatePipe(&inputRead, &inputWrite, nil, 0); err != nil {
		return nil, fmt.Errorf("failed to create input pipe: %w", err)
	}

	if err := windows.CreatePipe(&outputRead, &outputWrite, nil, 0); err != nil {
		windows.CloseHandle(inputRead)
		windows.CloseHandle(inputWrite)
		return nil, fmt.Errorf("failed to create output pipe: %w", err)
	}

	// Set initial size
	rows := opts.InitialRows
	cols := opts.InitialCols
	if rows == 0 {
		rows = 24
	}
	if cols == 0 {
		cols = 80
	}
	size := (int32(rows) << 16) | int32(cols)

	// Create the pseudo console
	var hPC windows.Handle
	ret, _, err := procCreatePseudoConsole.Call(
		uintptr(size),
		uintptr(outputRead),
		uintptr(inputWrite),
		0,
		uintptr(unsafe.Pointer(&hPC)),
	)
	if ret != 0 {
		windows.CloseHandle(inputRead)
		windows.CloseHandle(inputWrite)
		windows.CloseHandle(outputRead)
		windows.CloseHandle(outputWrite)
		return nil, fmt.Errorf("CreatePseudoConsole failed: %w", err)
	}

	// Close the handles that are now owned by the pseudo console
	windows.CloseHandle(outputRead)
	windows.CloseHandle(inputWrite)

	// Create the process with the pseudo console
	cmd := exec.Command(opts.Command, opts.Args...)
	cmd.Env = opts.Env
	if cmd.Env == nil {
		cmd.Env = os.Environ()
	}
	if opts.Dir != "" {
		cmd.Dir = opts.Dir
	}

	// Set up process attributes for ConPTY
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_UNICODE_ENVIRONMENT,
	}

	// Start the process
	if err := cmd.Start(); err != nil {
		procClosePseudoConsole.Call(uintptr(hPC))
		windows.CloseHandle(inputRead)
		windows.CloseHandle(outputWrite)
		return nil, fmt.Errorf("failed to start process: %w", err)
	}

	return &Process{
		PTY: &windowsPTY{
			hPC:         hPC,
			inputRead:   os.NewFile(uintptr(inputRead), "pty-input"),
			outputWrite: os.NewFile(uintptr(outputWrite), "pty-output"),
		},
		Cmd: cmd,
		pid: cmd.Process.Pid,
	}, nil
}
