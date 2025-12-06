package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/creack/pty"
	"golang.org/x/term"

	"github.com/remote-agent-terminal/backend/pkg/driver"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run test_driver_messages.go <command>")
		fmt.Println("Example: go run test_driver_messages.go claude")
		os.Exit(1)
	}

	command := os.Args[1]
	args := os.Args[2:]

	fmt.Printf("Starting command: %s %v\n", command, args)
	fmt.Println("Testing ClaudeDriver message parsing...")
	fmt.Println("Press Ctrl+C to exit\n")
	fmt.Println("════════════════════════════════════════════════════════════\n")

	cmd := exec.Command(command, args...)

	ptmx, err := pty.Start(cmd)
	if err != nil {
		fmt.Printf("Failed to start PTY: %v\n", err)
		os.Exit(1)
	}
	defer ptmx.Close()

	// Handle window size changes
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGWINCH)
	go func() {
		for range ch {
			pty.InheritSize(os.Stdin, ptmx)
		}
	}()
	ch <- syscall.SIGWINCH
	defer func() { signal.Stop(ch); close(ch) }()

	// Set terminal to raw mode
	var oldState *term.State
	if term.IsTerminal(int(os.Stdin.Fd())) {
		oldState, err = term.MakeRaw(int(os.Stdin.Fd()))
		if err == nil {
			defer func() { _ = term.Restore(int(os.Stdin.Fd()), oldState) }()
		}
	}

	// Create ClaudeDriver
	claudeDriver := driver.NewClaudeDriver()

	// Collect all messages
	var allMessages []driver.Message

	// Forward stdin to PTY
	go func() {
		io.Copy(ptmx, os.Stdin)
	}()

	// Read from PTY and parse with driver
	buf := make([]byte, 4096)
	for {
		n, err := ptmx.Read(buf)
		if err != nil {
			if err != io.EOF {
				fmt.Printf("\r\n[Session ended]\r\n")
			}
			break
		}

		if n > 0 {
			data := buf[:n]
			// Write to stdout (display)
			os.Stdout.Write(data)

			// Parse with ClaudeDriver
			result, _ := claudeDriver.Parse(data)

			// Log smart events
			if len(result.SmartEvents) > 0 {
				for _, event := range result.SmartEvents {
					fmt.Printf("\r\n╔══════════════════════════════════════════════════════════╗\r\n")
					fmt.Printf("║ 🎯 SMART EVENT: %-40s ║\r\n", event.Kind)
					fmt.Printf("║ Options: %-48v ║\r\n", event.Options)
					fmt.Printf("╚══════════════════════════════════════════════════════════╝\r\n")
				}
			}

			// Log messages
			if len(result.Messages) > 0 {
				for _, msg := range result.Messages {
					allMessages = append(allMessages, msg)
					fmt.Printf("\r\n┌──────────────────────────────────────────────────────────┐\r\n")
					fmt.Printf("│ 📝 MESSAGE: %-45s │\r\n", msg.Type)
					// Truncate content for display
					content := msg.Content
					if len(content) > 50 {
						content = content[:50] + "..."
					}
					fmt.Printf("│ Content: %-48s │\r\n", content)
					fmt.Printf("└──────────────────────────────────────────────────────────┘\r\n")
				}
			}
		}
	}

	cmd.Wait()

	// Flush any remaining buffered output
	flushed := claudeDriver.Flush()
	if len(flushed) > 0 {
		for _, msg := range flushed {
			allMessages = append(allMessages, msg)
			fmt.Printf("\r\n┌──────────────────────────────────────────────────────────┐\r\n")
			fmt.Printf("│ 📝 FLUSHED: %-45s │\r\n", msg.Type)
			content := msg.Content
			if len(content) > 50 {
				content = content[:50] + "..."
			}
			fmt.Printf("│ Content: %-48s │\r\n", content)
			fmt.Printf("└──────────────────────────────────────────────────────────┘\r\n")
		}
	}

	// Save messages to file
	jsonData, _ := json.MarshalIndent(allMessages, "", "  ")
	os.WriteFile("/tmp/driver_messages.json", jsonData, 0644)

	fmt.Printf("\r\n\r\n════════════════════════════════════════════════════════════\r\n")
	fmt.Printf("Session ended. Total messages: %d\r\n", len(allMessages))
	fmt.Printf("Messages saved to: /tmp/driver_messages.json\r\n")
	fmt.Printf("════════════════════════════════════════════════════════════\r\n")
}
