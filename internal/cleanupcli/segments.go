// segments.go declares the cleanup verb's segments, one per module that owns leftover state, and the runner that drives them.
// Each segment only calls its owner's cleanup function in-process, so no module learns another's state.

package cleanupcli

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/battencli"
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/loggerconfig"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxtemp"
	"github.com/Knatte18/loomyard/internal/orchcli"
	"github.com/Knatte18/loomyard/internal/orchengine"
)

// segment is one module's cleanup.
// run computes the same candidates whatever apply is and removes them only when apply is true; listed is what it removes, or in a dry run would remove.
// A segment that fails still returns what it listed.
type segment struct {
	name string
	run  func(apply bool) (listed any, err error)
}

// segmentResult is one segment's outcome as the verb prints it.
type segmentResult struct {
	Name   string `json:"name"`
	Listed any    `json:"listed"`
	Error  string `json:"error,omitempty"`
}

// runSegments runs every segment in order and never stops early: a failing segment's error goes into its own result and the next segment still runs.
// It reports false when any segment failed.
func runSegments(segments []segment, apply bool) ([]segmentResult, bool) {
	results := make([]segmentResult, 0, len(segments))
	allSucceeded := true
	for _, s := range segments {
		listed, err := s.run(apply)
		result := segmentResult{Name: s.name, Listed: listed}
		if err != nil {
			result.Error = err.Error()
			allSucceeded = false
		}
		results = append(results, result)
	}
	return results, allSucceeded
}

// primeSegments returns the cleanup of the hub prime at location, in the order the verb runs them: batten, board, orch, scratch, logs and temp.
func primeSegments(location *lyxcwd.Location) []segment {
	openBoard := func() (*boardengine.Board, error) {
		return boardengine.OpenHub(location.HubPath)
	}

	return []segment{
		{name: "batten", run: func(apply bool) (any, error) {
			report, err := battencli.Cleanup(location, apply)
			if err != nil {
				return nil, err
			}
			return map[string]any{"removed": report.Removed, "emptied": report.Emptied, "skipped": report.Skipped, "push_error": report.PushError}, nil
		}},
		{name: "board", run: func(apply bool) (any, error) {
			board, err := openBoard()
			if err != nil {
				return nil, err
			}
			removed, err := board.Prune(apply)
			if err != nil {
				return nil, err
			}
			if removed == nil {
				removed = []string{}
			}
			return map[string]any{"removed": removed}, nil
		}},
		{name: "orch", run: func(apply bool) (any, error) {
			result, err := orchengine.CleanupHandoffs(orchcli.PrimePaths(location), apply)
			if err != nil {
				return nil, err
			}
			return map[string]any{"kept": result.Kept, "removed": result.Removed, "reason": result.Reason}, nil
		}},
		{name: "scratch", run: func(apply bool) (any, error) {
			readBoard := func() ([]boardengine.Task, error) {
				board, err := openBoard()
				if err != nil {
					return nil, err
				}
				return board.ListTasksFull()
			}
			removed, err := scratchSegment(filepath.Join(location.WorktreePath(), scratchDirName), readBoard, time.Now(), apply)
			if err != nil {
				return nil, err
			}
			return map[string]any{"removed": removed}, nil
		}},
		{name: "logs", run: func(apply bool) (any, error) {
			removed := []string{}
			for _, anchor := range []string{location.AnchorPath(), fabricengine.BoardDir(location.HubPath)} {
				paths, err := logger.SweepDir(logger.LogsDirAt(anchor), loggerconfig.LoadOrDefault(anchor), apply)
				if err != nil {
					return nil, err
				}
				removed = append(removed, paths...)
			}
			return map[string]any{"removed": removed}, nil
		}},
		{name: "temp", run: func(apply bool) (any, error) {
			root, err := lyxtemp.ProcessRoot()
			if err != nil {
				return nil, err
			}
			report := root.Sweep(apply)
			removed := report.Candidates
			if removed == nil {
				removed = []string{}
			}
			failures := make([]string, 0, len(report.Failures))
			for _, failure := range report.Failures {
				failures = append(failures, fmt.Sprintf("%s: %v", failure.Path, failure.Err))
			}
			listed := map[string]any{"removed": removed, "failures": failures}
			if len(failures) > 0 {
				return listed, fmt.Errorf("cleanup: %d temp entries could not be swept", len(failures))
			}
			return listed, nil
		}},
	}
}
