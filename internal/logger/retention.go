// retention.go implements the durable trace-file retention sweep: a
// standalone function over a directory path that enforces the age and count
// bounds discussion.md's `retention` decision describes. It is deliberately
// independent of the durable sink (batch 4), lyxcwd (batch 1), and
// trace identity (batch 2) — Sweep takes a directory path from its caller
// and needs only internal/proc's liveness probe plus the standard library.

package logger

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"time"

	"github.com/Knatte18/loomyard/internal/proc"
)

const retentionAgeBound = 14 * 24 * time.Hour
const retentionCountBound = 50
const traceFileTimestampLayout = "20060102T150405Z"

var traceFilePattern = regexp.MustCompile(`^trace-(\d{8}T\d{6}Z)-[0-9a-f]{16}-(\d+)\.log$`)

type retentionCandidate struct {
	path      string
	timestamp time.Time
}

// Sweep enforces the trace-file directory's retention policy by removing old files and keeping only the newest files.
// Files are ranked by filename timestamp, never mtime. Live processes are never deleted. Non-matching files are untouched.
// Delete failures are silently tolerated. Empty or absent directories return nil.
func Sweep(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var candidates []retentionCandidate
	now := time.Now()
	selfPID := os.Getpid()
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		match := traceFilePattern.FindStringSubmatch(entry.Name())
		if match == nil {
			continue
		}
		ts, err := time.Parse(traceFileTimestampLayout, match[1])
		if err != nil {
			continue
		}
		pid, err := strconv.Atoi(match[2])
		if err != nil {
			continue
		}
		if pid == selfPID || proc.IsAlive(pid) {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		if now.Sub(ts) > retentionAgeBound {
			_ = os.Remove(path)
			continue
		}
		candidates = append(candidates, retentionCandidate{path: path, timestamp: ts})
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].timestamp.After(candidates[j].timestamp)
	})
	for _, candidate := range candidates[minInt(retentionCountBound, len(candidates)):] {
		_ = os.Remove(candidate.path)
	}
	return nil
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
