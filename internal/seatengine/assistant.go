// assistant.go writes and clears an assistant's end marker, names its endings, and renders the chair's assistant list.

package seatengine

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/fsx"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// The endings an assistant's end marker can name; each is the marker's first word or words.
const (
	// EndingDone marks a run that ended done with its seat gate passed; its outputs are the assistant's report.
	EndingDone = "done"
	// EndingGateFailed marks a run that ended done with its seat gate failed.
	EndingGateFailed = "gate failed"
	// EndingStopped marks a run the engine stopped when the chair ended.
	EndingStopped = "stopped"
	// EndingNotStarted marks an assistant that did not start; the marker carries the reason after a colon.
	EndingNotStarted = "not started"
	// EndingNotFound marks an assistant that a resume found neither live nor ended.
	EndingNotFound = "not found"
	// endingWaitError leads the marker of a run whose wait errored; the error follows after a colon.
	endingWaitError = "wait error"
)

// ReadEnding returns the ending the end marker at path names, without its trailing newline.
// It reports whether the marker exists; a missing marker is not an error.
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

// endingOf returns the ending an assistant's run came to.
// A failed wait reads as a wait error, unless the engine stopped the run, which reads stopped.
// A run that ended done reads done, or gate failed when its seat gate did not pass;
// other runs read their outcome, or stopped when the engine stopped them.
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

// writeEnding writes ending to the end marker at path, creating its directory.
// The write is atomic, so a reader never sees a part of the ending.
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

// clearAssistantFiles removes each assistant's end marker, outputs and subagent reports a previous run left.
// Neither the chair nor a caller of a fresh run reads a stale one.
// A file already absent is no error.
func (e *Engine) clearAssistantFiles(table Table) error {
	for _, seat := range table.Seats {
		if !seat.Assistant {
			continue
		}
		for _, path := range e.absolute(slices.Concat([]string{seat.EndMarker}, seat.Outputs, seat.SubagentReports)) {
			if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return fmt.Errorf("seatengine: seat %q: remove stale file %q: %w", table.seatLabel(seat), path, err)
			}
		}
	}
	return nil
}

// assistantList renders the chair's list of its assistants, one line per assistant.
// Each line names the assistant's strand, its outputs and its end marker.
// Relative paths are joined onto the worktree root of geom, and a table without assistants renders noneValue.
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
