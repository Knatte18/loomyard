// status.go implements the paired host↔weft status view and host-pollution detection
// for fabric.
//
// Status enumerates all host worktrees via hubgeometry.List, pairs each with its weft
// sibling, reports branch, in-sync verdict, junction health, and scans the host index
// for any _lyx or _raddle paths that have been accidentally git-tracked (host pollution).
// Adapted from warpengine's status.go — same field set and pollution-scan behavior,
// package fabricengine. The branch delta: a pair is InSync when
// weftBranch == WeftBranchName(hostBranch) (warp requires equal names), and
// DriftReason states the expected suffixed branch rather than a bare mismatch.
//
// Status computes its in-sync verdict inline (branch correspondence via WeftBranchName,
// then junction health via checkJunctionHealth, both already defined in reconcile.go)
// rather than calling a shared PairInSync helper — drift.go (a later card in this batch)
// does not exist yet when this file is written, and Status already has hostBranch/
// weftBranch in hand from readBranch, so there is nothing to gain from a second
// rev-parse round trip through a shared helper.

package fabricengine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
)

// PollutionEntry describes a single tracked path in the host index that should never
// be committed there (e.g. _lyx or _raddle, which belong exclusively in the weft).
type PollutionEntry struct {
	// Path is the relative path reported by git ls-files.
	Path string `json:"path"`
	// Remedy is the suggested remediation command. Empty when the entry is report-only.
	Remedy string `json:"remedy,omitempty"`
	// ReportOnly is true when no automated remedy is available (e.g. _raddle).
	ReportOnly bool `json:"report_only"`
}

// PairStatus describes the relationship between one host worktree and its paired weft sibling.
type PairStatus struct {
	// HostWorktree is the absolute path to the host worktree.
	HostWorktree string `json:"host_worktree"`
	// WeftWorktree is the absolute path to the expected weft sibling worktree.
	WeftWorktree string `json:"weft_worktree"`
	// HostBranch is the current branch of the host worktree (empty if undetermined).
	HostBranch string `json:"host_branch"`
	// WeftBranch is the current branch of the weft worktree (empty if missing or undetermined).
	WeftBranch string `json:"weft_branch"`
	// InSync reports whether the pair is branch-synchronized (weftBranch ==
	// WeftBranchName(hostBranch)) and the junction is healthy.
	InSync bool `json:"in_sync"`
	// DriftReason describes why the pair is out of sync. Empty when InSync is true.
	DriftReason string `json:"drift_reason,omitempty"`
	// JunctionHealthy reports whether the host _lyx junction exists and points to the weft.
	JunctionHealthy bool `json:"junction_healthy"`
	// JunctionReason describes the junction problem. Empty when JunctionHealthy is true.
	JunctionReason string `json:"junction_reason,omitempty"`
	// Pollution lists host-index paths that should not be tracked there.
	Pollution []PollutionEntry `json:"pollution,omitempty"`
}

// StatusResult is the top-level result type returned by Status.
// It contains one PairStatus entry per discovered host worktree.
type StatusResult struct {
	// Pairs is the ordered list of host↔weft pair reports.
	Pairs []PairStatus `json:"pairs"`
}

// Status returns the paired host↔weft status view for all worktrees reachable from
// the given layout, plus host-pollution detection on the host index.
//
// For each host worktree discovered via hubgeometry.List, Status:
//   - Derives the paired weft worktree path via layout geometry
//   - Reads the host branch and weft branch (if the weft exists)
//   - Reports in-sync status: weftBranch == WeftBranchName(hostBranch) and the host
//     _lyx junction is valid
//   - Reports junction health (separate from the drift check) using checkJunctionHealth
//   - Scans the host index for any _lyx or _raddle paths via git ls-files; marks
//     _lyx entries as remediable (git rm --cached + restore junction/exclude) and
//     _raddle entries as report-only (no junction to restore in this task)
//
// Layout l is the resolved layout for the current working directory; it provides Hub
// and Prime fields for deriving the weft repo root and weft worktree names.
// Returns an error only on fatal system failures; per-worktree errors are recorded
// inline in PairStatus.DriftReason / PairStatus.JunctionReason.
func (t *Topology) Status(l *hubgeometry.Layout) (StatusResult, error) {
	// Enumerate all host worktrees from any worktree in the repository.
	entries, err := hubgeometry.List(l.WorktreeRoot)
	if err != nil {
		return StatusResult{}, fmt.Errorf("list worktrees: %w", err)
	}

	var result StatusResult

	for _, entry := range entries {
		hostPath := filepath.FromSlash(entry.Path)
		hostPath = filepath.Clean(hostPath)

		// Derive the paired weft worktree path from the host worktree base name.
		// e.g. <hub>/my-task → <hub>/my-task-weft
		weftPath := l.WeftWorktreePath(filepath.Base(hostPath))

		// Emit forward-slash paths in the JSON-tagged fields only; hostPath/weftPath
		// stay OS-native below for os.Stat, git subprocess calls, and junction checks.
		pair := PairStatus{
			HostWorktree: filepath.ToSlash(hostPath),
			WeftWorktree: filepath.ToSlash(weftPath),
		}

		// Read the host branch.
		hostBranch, hostBranchErr := readBranch(hostPath)
		if hostBranchErr != nil {
			pair.DriftReason = fmt.Sprintf("read host branch: %v", hostBranchErr)
			result.Pairs = append(result.Pairs, pair)
			continue
		}
		pair.HostBranch = hostBranch

		// Read the weft branch if the weft worktree exists; a missing weft is reported inline.
		weftStat, err := os.Stat(weftPath)
		if err != nil || !weftStat.IsDir() {
			pair.DriftReason = "weft worktree missing"
			pair.InSync = false
			result.Pairs = append(result.Pairs, pair)
			continue
		}

		weftBranch, weftBranchErr := readBranch(weftPath)
		if weftBranchErr != nil {
			pair.DriftReason = fmt.Sprintf("read weft branch: %v", weftBranchErr)
			result.Pairs = append(result.Pairs, pair)
			continue
		}
		pair.WeftBranch = weftBranch

		// Build a per-host-worktree layout to resolve junction geometry. hostLayoutFor
		// avoids a git spawn for the common hub-sibling case.
		hostLayout, layoutErr := hostLayoutFor(l, hostPath)
		if layoutErr != nil {
			pair.DriftReason = fmt.Sprintf("resolve host layout: %v", layoutErr)
			result.Pairs = append(result.Pairs, pair)
			continue
		}

		// Determine junction health independently of the drift verdict so callers
		// can distinguish "branches match but junction is broken" from full in-sync.
		hostLink := hostLayout.HostLyxLinkHere()
		weftLyxDir := hostLayout.WeftLyxDir()
		junctionHealthy, junctionReason := checkJunctionHealth(hostLink, weftLyxDir)
		pair.JunctionHealthy = junctionHealthy
		pair.JunctionReason = junctionReason

		// Determine pair in-sync status: branch correspondence uses WeftBranchName
		// (fabric's suffixed pairing) rather than warp's equal-name requirement, folded
		// together with junction health exactly as warp's PairInSync folds both checks
		// into one verdict.
		expectedWeftBranch := WeftBranchName(hostBranch)
		switch {
		case weftBranch != expectedWeftBranch:
			pair.InSync = false
			pair.DriftReason = fmt.Sprintf("host on %s, weft on %s (want %s)", hostBranch, weftBranch, expectedWeftBranch)
		case !junctionHealthy:
			pair.InSync = false
			pair.DriftReason = junctionReason
		default:
			pair.InSync = true
		}

		// Scan the host index for _lyx and _raddle paths that must never be tracked there.
		pollution, pollErr := detectHostPollution(hostPath)
		if pollErr != nil {
			// Non-fatal: record the error inline and continue.
			pair.Pollution = append(pair.Pollution, PollutionEntry{
				Path:       fmt.Sprintf("<scan error: %v>", pollErr),
				ReportOnly: true,
			})
		} else {
			pair.Pollution = pollution
		}

		result.Pairs = append(result.Pairs, pair)
	}

	return result, nil
}

// detectHostPollution scans the host worktree index for _lyx and _raddle paths
// that should never be tracked in the host repo.
//
// For each match under _lyx, the remedy is the git rm --cached command that removes
// the file from the index without deleting it from disk, plus a reminder to restore
// the junction/exclude entry. _raddle matches are report-only: no junction is wired
// for _raddle in this release so no automated restore step is offered.
func detectHostPollution(hostPath string) ([]PollutionEntry, error) {
	// git ls-files lists only tracked (index) files matching the given pathspecs.
	// Using -- prevents ambiguity when the pathspec looks like a branch name.
	out, _, exitCode, err := gitexec.RunGit(
		[]string{"ls-files", "--", "_lyx", "_raddle"},
		hostPath,
	)
	if err != nil {
		return nil, fmt.Errorf("ls-files: %w", err)
	}
	if exitCode != 0 {
		// A non-zero exit from ls-files means the command itself failed, not just
		// that no files matched; report as an error.
		return nil, fmt.Errorf("ls-files exited %d", exitCode)
	}

	output := strings.TrimSpace(out)
	if output == "" {
		return nil, nil
	}

	var entries []PollutionEntry
	for _, line := range strings.Split(output, "\n") {
		tracked := strings.TrimSpace(line)
		if tracked == "" {
			continue
		}

		// Determine whether the path is under _lyx or _raddle.
		if strings.HasPrefix(tracked, "_lyx") || tracked == "_lyx" {
			// Offer git rm --cached as the remedy, plus a reminder to restore the
			// junction and exclude entry so lyx topology is intact afterwards.
			remedy := fmt.Sprintf(
				"git -C %s rm --cached -- %s  # then restore junction and git-exclude entry",
				hostPath, tracked,
			)
			entries = append(entries, PollutionEntry{
				Path:   tracked,
				Remedy: remedy,
			})
		} else if strings.HasPrefix(tracked, "_raddle") || tracked == "_raddle" {
			// _raddle pollution is report-only: no junction is wired for _raddle yet.
			entries = append(entries, PollutionEntry{
				Path:       tracked,
				ReportOnly: true,
			})
		}
	}

	return entries, nil
}
