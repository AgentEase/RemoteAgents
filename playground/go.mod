module playground

go 1.24.0

replace github.com/remote-agent-terminal/backend => ../backend

require github.com/remote-agent-terminal/backend v0.0.0-00010101000000-000000000000

require (
	github.com/creack/pty v1.1.24 // indirect
	golang.org/x/sys v0.38.0 // indirect
	golang.org/x/term v0.37.0 // indirect
)
