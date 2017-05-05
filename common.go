package server

import (
	"fmt"
)

const (
	DefaultTransport = "docker"
)

// DefaultDriverImageReference returns the default image reference for a driver
// given a language.
func DefaultDriverImageReference(transport, lang string) string {
	if transport == "" {
		transport = DefaultTransport
	}

	return fmt.Sprintf("%s:bblfsh/%s-driver:latest", transport, lang)
}
