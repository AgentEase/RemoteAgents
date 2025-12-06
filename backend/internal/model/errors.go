package model

import "errors"

var (
	// ErrCommandRequired is returned when a session creation request is missing the command.
	ErrCommandRequired = errors.New("command is required")

	// ErrSessionNotFound is returned when a session is not found.
	ErrSessionNotFound = errors.New("session not found")

	// ErrUnauthorized is returned when a user is not authorized.
	ErrUnauthorized = errors.New("unauthorized")

	// ErrForbidden is returned when access to a resource is forbidden.
	ErrForbidden = errors.New("forbidden")

	// ErrConcurrencyLimit is returned when the maximum number of concurrent sessions is reached.
	ErrConcurrencyLimit = errors.New("concurrent session limit exceeded")
)
