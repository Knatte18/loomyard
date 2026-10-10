// wait.go — the per-worktree wait records of gate runs that have no verifytree marker to show their wait.

package gateslot

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/fsx"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/proc"
)

const waitGlobPattern = "wait-*.yaml"

// Wait is one waiter's record: the site that waits for a slot, its process and when the wait began.
type Wait struct {
	// Site is the `lyx gate test` command or the Go-side site label.
	Site string `yaml:"site"`
	// PID is the waiting process.
	PID int `yaml:"pid"`
	// Started is when the wait began.
	Started time.Time `yaml:"started"`
}

// WaitDir returns the directory under anchorRoot that keeps a worktree's wait records.
func WaitDir(anchorRoot string) string {
	return filepath.Join(anchorRoot, lyxdirs.DotLyxDirName, gateDirName)
}

// WriteWait writes w as a uniquely named record in dir and returns its path, which the writer removes once the wait ends.
func WriteWait(dir string, w Wait) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create gate wait directory: %w", err)
	}
	data, err := yaml.Marshal(w)
	if err != nil {
		return "", fmt.Errorf("encode gate wait record: %w", err)
	}
	reserved, err := os.CreateTemp(dir, "wait-*.yaml")
	if err != nil {
		return "", fmt.Errorf("create gate wait record: %w", err)
	}
	path := reserved.Name()
	if err := reserved.Close(); err != nil {
		return "", fmt.Errorf("create gate wait record: %w", err)
	}
	if err := fsx.AtomicWriteBytes(path, data); err != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("write gate wait record: %w", err)
	}
	return path, nil
}

// ReadWaits returns the wait records in dir whose process is alive, so a crashed waiter never shows.
// A missing dir holds none.
func ReadWaits(dir string) ([]Wait, error) {
	records, err := filepath.Glob(filepath.Join(dir, waitGlobPattern))
	if err != nil {
		return nil, fmt.Errorf("list gate wait records: %w", err)
	}
	var waits []Wait
	for _, path := range records {
		data, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read gate wait record: %w", err)
		}
		var w Wait
		if err := yaml.Unmarshal(data, &w); err != nil {
			return nil, fmt.Errorf("parse gate wait record %s: %w", path, err)
		}
		// A record still empty from WriteWait's reservation has no process yet.
		if w.PID <= 0 || !proc.IsAlive(w.PID) {
			continue
		}
		waits = append(waits, w)
	}
	return waits, nil
}
