// lyxtemp.go — the process-wide run directory: Dir, MkdirTemp, CreateTemp and Remove.

package lyxtemp

import (
	"fmt"
	"os"
	"sync"
)

var (
	// processMu guards processRun.
	processMu sync.Mutex
	// processRun is the process's run directory, nil until Dir first creates it.
	processRun *run
)

// Dir returns the process's run directory, creating it, and the temp root above it, on first use.
// Later calls return the same path until Remove clears it.
func Dir() (string, error) {
	processMu.Lock()
	defer processMu.Unlock()
	if processRun != nil {
		return processRun.dir, nil
	}
	root, err := ProcessRoot()
	if err != nil {
		return "", err
	}
	created, err := root.newRun()
	if err != nil {
		return "", err
	}
	processRun = created
	return created.dir, nil
}

// MkdirTemp creates a new directory under the process's run directory, as os.MkdirTemp does for pattern.
func MkdirTemp(pattern string) (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return os.MkdirTemp(dir, pattern)
}

// CreateTemp creates a new file under the process's run directory, as os.CreateTemp does for pattern.
func CreateTemp(pattern string) (*os.File, error) {
	dir, err := Dir()
	if err != nil {
		return nil, err
	}
	return os.CreateTemp(dir, pattern)
}

// Remove releases and deletes the process's run directory and forgets it, so a later Dir makes a fresh one.
// It returns nil when no run directory was created.
func Remove() error {
	processMu.Lock()
	defer processMu.Unlock()
	if processRun == nil {
		return nil
	}
	current := processRun
	processRun = nil
	if err := current.remove(); err != nil {
		return fmt.Errorf("remove run directory: %w", err)
	}
	return nil
}
