//go:build !windows
// +build !windows

package pty

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

// unixPTY implements the PTY interface for Unix-like systems (Linux, macOS).
type unixPTY struct {
	master *os.File
}

// Read reads data from the PTY output.
func (p *unixPTY) Read(b []byte) (int, error) {
	return p.master.Read(b)
}

// Write writes data to the PTY input.
func (p *unixPTY) Write(b []byte) (int, error) {
	return p.master.Write(b)
}

// Close closes the PTY master file descriptor.
func (p *unixPTY) Close() error {
	return p.master.Close()
}

// Fd returns the file descriptor of the PTY master.
func (p *unixPTY) Fd() uintptr {
	return p.master.Fd()
}

// Resize changes the PTY window size.
func (p *unixPTY) Resize(rows, cols uint16) error {
	ws := &unix.Winsize{
		Row: rows,
		Col: cols,
	}
	return unix.IoctlSetWinsize(int(p.master.Fd()), unix.TIOCSWINSZ, ws)
}

// Start starts a new PTY process with the given options.
// This is the Unix implementation using native PTY.
func Start(opts StartOptions) (*Process, error) {
	// Open a new PTY master/slave pair
	master, slave, err := openPTY()
	if err != nil {
		return nil, fmt.Errorf("failed to open PTY: %w", err)
	}

	// Set initial window size
	if opts.InitialRows > 0 && opts.InitialCols > 0 {
		ws := &unix.Winsize{
			Row: opts.InitialRows,
			Col: opts.InitialCols,
		}
		if err := unix.IoctlSetWinsize(int(master.Fd()), unix.TIOCSWINSZ, ws); err != nil {
			master.Close()
			slave.Close()
			return nil, fmt.Errorf("failed to set window size: %w", err)
		}
	}

	// Prepare the command
	cmd := exec.Command(opts.Command, opts.Args...)
	cmd.Env = opts.Env
	if cmd.Env == nil {
		cmd.Env = os.Environ()
	}
	if opts.Dir != "" {
		cmd.Dir = opts.Dir
	}

	// Set up the process to use the slave PTY
	cmd.Stdin = slave
	cmd.Stdout = slave
	cmd.Stderr = slave

	// Set up process group and controlling terminal
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid:  true,
		Setctty: true,
	}

	// Start the process
	if err := cmd.Start(); err != nil {
		master.Close()
		slave.Close()
		return nil, fmt.Errorf("failed to start process: %w", err)
	}

	// Close the slave in the parent process
	slave.Close()

	return &Process{
		PTY: &unixPTY{master: master},
		Cmd: cmd,
		pid: cmd.Process.Pid,
	}, nil
}

// openPTY opens a new PTY master/slave pair.
func openPTY() (master, slave *os.File, err error) {
	// Open the PTY master
	master, err = os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open /dev/ptmx: %w", err)
	}

	// Get the slave PTY name
	slaveName, err := ptsname(master)
	if err != nil {
		master.Close()
		return nil, nil, fmt.Errorf("failed to get slave name: %w", err)
	}

	// Unlock the slave PTY
	if err := unlockpt(master); err != nil {
		master.Close()
		return nil, nil, fmt.Errorf("failed to unlock PTY: %w", err)
	}

	// Open the slave PTY
	slave, err = os.OpenFile(slaveName, os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		master.Close()
		return nil, nil, fmt.Errorf("failed to open slave PTY: %w", err)
	}

	return master, slave, nil
}

// ptsname returns the name of the slave PTY.
func ptsname(master *os.File) (string, error) {
	var n uint32
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, master.Fd(), syscall.TIOCGPTN, uintptr(unsafe.Pointer(&n)))
	if errno != 0 {
		return "", errno
	}
	return fmt.Sprintf("/dev/pts/%d", n), nil
}

// unlockpt unlocks the slave PTY.
func unlockpt(master *os.File) error {
	var unlock int32 = 0
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, master.Fd(), syscall.TIOCSPTLCK, uintptr(unsafe.Pointer(&unlock)))
	if errno != 0 {
		return errno
	}
	return nil
}
