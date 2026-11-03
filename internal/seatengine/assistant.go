// assistant.go writes and clears an assistant seat's end marker, words the endings it names, and renders the chair's list of its assistants.

package seatengine

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/fsx"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// The endings an assistant's end marker names, each the marker's first word or words.
const (
	// EndingDone is a run that ended done with its seat gate passed: its outputs are the assistant's report.
	EndingDone = "done"
	// EndingGateFailed is a run that ended done with its seat gate failed.
	EndingGateFailed = "gate failed"
	// EndingStopped is a run the engine stopped when the chair ended.
	EndingStopped = "stopped"
	// EndingNotStarted is an assistant that did not start; the marker carries the reason after a colon.
	EndingNotStarted = "not started"
	// EndingNotFound is an assistant a resume found neither live nor ended.
	EndingNotFound = "not found"
	// endingWaitError leads the marker of a run whose wait errored; the error follows after a colon.
	endingWaitError = "wait error"
)

// ReadEnding returns the ending an end marker at path names, its text without the trailing newline, and false when the marker does not exist.
func ReadEnding(path string) (string, bool, error) {
	content, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("seatengine: read end marker %q: %w", path, err)
	}
	return strings.TrimRight(string(content), "\n"), true, nil
}

// endingOf returns the ending an assistant's run came to: done only with its seat gate passed, gate failed for a done run whose gate failed, the outcome's own name otherwise, and the wait error when the wait failed.
// stopped marks a run the engine stopped, which reads stopped unless it had already ended done.
func endingOf(result shuttleengine.Result, waitErr error, stopped bool) string {
	switch {
	case waitErr != nil && stopped:
		return EndingStopped
	case waitErr != nil:
		return endingWaitError + ": " + waitErr.Error()
	case result.Outcome == shuttleengine.OutcomeDone && result.Gate != nil && !result.Gate.Passed:
		return EndingGateFailed
	case result.Outcome == shuttleengine.OutcomeDone:
		return EndingDone
	case stopped:
		return EndingStopped
	}
	return string(result.Outcome)
}

// notStartedEnding returns the ending of an assistant that did not start, for reason.
func notStartedEnding(reason string) string {
	return EndingNotStarted + ": " + reason
}

// writeEnding writes ending to the end marker at path, creating its directory, in one atomic write so a reader never sees a part of it.
// A failed write is logged, since the chair then waits until its own timeout or a stop, and is returned.
func writeEnding(path, ending string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		logger.Warn("seatengine: end marker directory not created", "marker", path, "error", err)
		return fmt.Errorf("seatengine: create end marker directory for %q: %w", path, err)
	}
	if err := fsx.AtomicWriteBytes(path, []byte(ending+"\n")); err != nil {
		logger.Warn("seatengine: end marker not written", "marker", path, "ending", ending, "error", err)
		return fmt.Errorf("seatengine: write end marker %q: %w", path, err)
	}
	logger.Info("seatengine: assistant ended", "marker", path, "ending", ending)
	return nil
}

// clearAssistantFiles removes each assistant's end marker and outputs a previous run left, so the chair of a fresh run never reads a stale one.
// A file already absent is no error.
func (e *Engine) clearAssistantFiles(table Table) error {
	for _, seat := range table.Seats {
		if !seat.Assistant {
			continue
		}
		for _, path := range e.absolute(append([]string{seat.EndMarker}, seat.Outputs...)) {
			if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return fmt.Errorf("seatengine: seat %q: remove stale file %q: %w", table.seatLabel(seat), path, err)
			}
		}
	}
	return nil
}

// assistantList renders the chair's list of its assistants: one line per assistant naming its strand, its outputs and its end marker, relative paths joined onto geom's worktree root, or noneValue for a table without one.
func assistantList(geom Geometry, table Table, names map[string]string) string {
	var lines []string
	for _, seat := range table.Seats {
		if !seat.Assistant {
			continue
		}
		outputs := absoluteUnder(geom.WorktreeRoot, seat.Outputs)
		marker := absoluteUnder(geom.WorktreeRoot, []string{seat.EndMarker})[0]
		lines = append(lines, fmt.Sprintf("- %s: report `%s`, end marker `%s`", names[seat.Name], strings.Join(outputs, "`, `"), marker))
	}
	return joinLines(lines)
}

// absoluteUnder returns paths with each relative one joined onto root.
func absoluteUnder(root string, paths []string) []string {
	resolved := make([]string, len(paths))
	for i, path := range paths {
		resolved[i] = path
		if !filepath.IsAbs(path) {
			resolved[i] = filepath.Join(root, path)
		}
	}
	return resolved
}
