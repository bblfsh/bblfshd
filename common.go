package server

import (
	"fmt"
)

const (
	DefaultTransport = "docker"
)

// DefaultDriverImageReference returns the default image reference for a driver
// given a language.
func DefaultDriverImageReference(overrides map[string]string, transport, lang string) string {
	if override := overrides[lang]; override != "" {
		return override
	}
	if transport == "" {
		transport = DefaultTransport
	}

	ref := fmt.Sprintf("bblfsh/%s-driver:latest", lang)
	switch transport {
	case "docker":
		ref = "//" + ref
	}

	return fmt.Sprintf("%s:%s", transport, ref)
}
