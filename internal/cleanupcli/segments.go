// segments.go declares the cleanup verb's segments, one per module that owns leftover state, and the runner that drives them.
// Each segment only calls its owner's cleanup function in-process, so no module learns another's state.

package cleanupcli

import (
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/battencli"
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/loggerconfig"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymtemp"
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

// battenListed renders a batten cleanup pass as the batten segment prints it.
// push_spawned says the records push was started and push_error that starting it failed, so an empty push_error alone never means the push landed.
func battenListed(report battencli.CleanupReport) map[string]any {
	return map[string]any{"removed": report.Removed, "emptied": report.Emptied, "skipped": report.Skipped, "push_spawned": report.PushSpawned, "push_error": report.PushError}
}

// primeSegments returns the cleanup of the hub prime at location and of the handoffs of every orch of the hub, in the order the verb runs them: batten, board, orch, scratch, logs and temp.
func primeSegments(location *lymcwd.Location) []segment {
	return segmentsOver(location, orchcli.HubOrchs)
}

// segmentsOver is primeSegments with hubOrchs, the lister of the worktrees the orch segment cleans.
func segmentsOver(location *lymcwd.Location, hubOrchs func(*lymcwd.Location) ([]*lymcwd.Location, error)) []segment {
	openBoard := func() (*boardengine.Board, error) {
		return boardengine.OpenHub(location.HubPath)
	}

	return []segment{
		{name: "batten", run: func(apply bool) (any, error) {
			report, err := battencli.Cleanup(location, apply)
			if err != nil {
				return nil, err
			}
			return battenListed(report), nil
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
			orchs, err := hubOrchs(location)
			if err != nil {
				return nil, err
			}
			worktrees := make([]map[string]any, 0, len(orchs))
			var failures []error
			for _, orch := range orchs {
				result, err := orchengine.CleanupHandoffs(orchcli.PrimePaths(orch), apply)
				if err != nil {
					failures = append(failures, fmt.Errorf("%s: %w", orch.WorktreeName, err))
					continue
				}
				worktrees = append(worktrees, map[string]any{"worktree": orch.WorktreeName, "kept": result.Kept, "removed": result.Removed, "reason": result.Reason})
			}
			return map[string]any{"worktrees": worktrees}, errors.Join(failures...)
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
			for _, dirs := range []configengine.Dirs{fabricengine.ConfigDirs(location), {Anchor: fabricengine.BoardDir(location.HubPath)}} {
				paths, err := logger.SweepDir(logger.LogsDirAt(dirs.Anchor), loggerconfig.LoadOrDefault(dirs), apply)
				if err != nil {
					return nil, err
				}
				removed = append(removed, paths...)
			}
			return map[string]any{"removed": removed}, nil
		}},
		{name: "temp", run: func(apply bool) (any, error) {
			root, err := lymtemp.ProcessRoot()
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
