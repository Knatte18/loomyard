package machinename

import (
	"fmt"
	"os"
	"strings"
)

// Name returns this machine's name, read from the operating system's hostname.
// It returns an error when the hostname cannot be read or is empty.
func Name() (string, error) {
	host, err := os.Hostname()
	if err != nil {
		return "", fmt.Errorf("read this machine's hostname: %w", err)
	}
	return FromHostname(host)
}

// FromHostname cuts host at its first dot, lowercases it and trims space, so Neve.local reads neve.
// It returns an error when nothing is left, since no owner can be written or compared without a name.
func FromHostname(host string) (string, error) {
	short, _, _ := strings.Cut(host, ".")
	name := strings.ToLower(strings.TrimSpace(short))
	if name == "" {
		return "", fmt.Errorf("this machine's hostname %q is empty, so no owner can be written or compared; give the machine a hostname", host)
	}
	return name, nil
}
