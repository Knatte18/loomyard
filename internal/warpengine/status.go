// status.go implements the paired host↔weft status view and host-pollution detection for warp.
//
// Status enumerates all host worktrees via paths.List, pairs each with its weft sibling,
// reports branch, in-sync verdict, junction health, and scans the host index for any
// _lyx or _codeguide paths that have been accidentally git-tracked (host pollution).

package warpengine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/paths"
)

// PollutionEntry describes a single tracked path in the host index that should never
// be committed there (e.g. _lyx or _codeguide, which belong exclusively in the weft).
type PollutionEntry struct {
	// Path is the relative path reported by git ls-files.
	Path string `json:"path"`
	// Remedy is the suggested remediation command. Empty when the entry is report-only.
	Remedy string `json:"remedy,omitempty"`
	// ReportOnly is true when no automated remedy is available (e.g. _codeguide).
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
	// InSync reports whether the pair is branch-synchronized and the junction is healthy.
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
// For each host worktree discovered via paths.List, Status:
//   - Derives the paired weft worktree path via layout geometry
//   - Reads the host branch and weft branch (if the weft exists)
//   - Reports in-sync status via PairInSync from the host's layout
//   - Reports junction health (separate from the drift check) using checkJunctionHealth
//   - Scans the host index for any _lyx or _codeguide paths via git ls-files; marks
//     _lyx entries as remediable (git rm --cached + restore junction/exclude) and
//     _codeguide entries as report-only (no junction to restore in this task)
//
// Layout l is the resolved layout for the current working directory; it provides Hub
// and Prime fields for deriving the weft repo root and weft worktree names.
// Returns an error only on fatal system failures; per-worktree errors are recorded
// inline in PairStatus.DriftReason / PairStatus.JunctionReason.
func (w *Worktree) Status(l *paths.Layout) (StatusResult, error) {
	// Enumerate all host worktrees from any worktree in the repository.
	entries, err := paths.List(l.WorktreeRoot)
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

		pair := PairStatus{
			HostWorktree: hostPath,
			WeftWorktree: weftPath,
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

		// Build a per-host-worktree layout to call PairInSync. PairInSync requires a
		// Layout whose WorktreeRoot is the host worktree being inspected, so we derive
		// one from the host path rather than reusing l (which points to the cwd worktree).
		hostLayout, layoutErr := paths.Resolve(hostPath)
		if layoutErr != nil {
			pair.DriftReason = fmt.Sprintf("resolve host layout: %v", layoutErr)
			result.Pairs = append(result.Pairs, pair)
			continue
		}

		// Determine pair in-sync status.
		inSync, driftReason, driftErr := PairInSync(hostLayout)
		if driftErr != nil {
			pair.DriftReason = fmt.Sprintf("pair sync check: %v", driftErr)
		} else {
			pair.InSync = inSync
			pair.DriftReason = driftReason
		}

		// Determine junction health independently of the drift verdict so callers
		// can distinguish "branches match but junction is broken" from full in-sync.
		hostLink := hostLayout.HostLyxLinkHere()
		weftLyxDir := hostLayout.WeftLyxDir()
		junctionHealthy, junctionReason := checkJunctionHealth(hostLink, weftLyxDir)
		pair.JunctionHealthy = junctionHealthy
		pair.JunctionReason = junctionReason

		// Scan the host index for _lyx and _codeguide paths that must never be tracked there.
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

// readBranch returns the current branch name for the worktree at dir via rev-parse.
// Returns an error if the git command fails.
func readBranch(dir string) (string, error) {
	out, _, exitCode, err := gitexec.RunGit(
		[]string{"rev-parse", "--abbrev-ref", "HEAD"},
		dir,
	)
	if err != nil {
		return "", fmt.Errorf("rev-parse: %w", err)
	}
	if exitCode != 0 {
		return "", fmt.Errorf("rev-parse exited %d", exitCode)
	}
	return strings.TrimSpace(out), nil
}

// checkJunctionHealth verifies that hostLink is a junction/symlink pointing to weftLyxDir.
//
// This function mirrors the logic formerly in weft/status.go's checkJunction, relocated
// to warp because junction topology is warp's responsibility. Returns (ok, reason) where
// ok is true only if the junction is correctly configured.
func checkJunctionHealth(hostLink, weftLyxDir string) (bool, string) {
	// Check whether the host link exists at all.
	_, err := os.Lstat(hostLink)
	if err != nil {
		if os.IsNotExist(err) {
			return false, "host _lyx junction missing"
		}
		return false, fmt.Sprintf("lstat error: %v", err)
	}

	// Verify the path is a link (junction or symlink), not a plain directory.
	isLink, err := fslink.IsLink(hostLink)
	if err != nil || !isLink {
		return false, "host _lyx is not a junction"
	}

	// Resolve both ends and compare canonicalized paths.
	hostResolved, err := fslink.PointsTo(hostLink)
	if err != nil {
		return false, fmt.Sprintf("resolve host link: %v", err)
	}

	weftResolved, err := filepath.EvalSymlinks(filepath.Clean(weftLyxDir))
	if err != nil {
		return false, fmt.Sprintf("resolve weft target: %v", err)
	}

	if filepath.Clean(hostResolved) != filepath.Clean(weftResolved) {
		return false, "host _lyx junction points elsewhere"
	}

	return true, ""
}

// detectHostPollution scans the host worktree index for _lyx and _codeguide paths
// that should never be tracked in the host repo.
//
// For each match under _lyx, the remedy is the git rm --cached command that removes
// the file from the index without deleting it from disk, plus a reminder to restore
// the junction/exclude entry. _codeguide matches are report-only: no junction is wired
// for _codeguide in this release so no automated restore step is offered.
func detectHostPollution(hostPath string) ([]PollutionEntry, error) {
	// git ls-files lists only tracked (index) files matching the given pathspecs.
	// Using -- prevents ambiguity when the pathspec looks like a branch name.
	out, _, exitCode, err := gitexec.RunGit(
		[]string{"ls-files", "--", "_lyx", "_codeguide"},
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

		// Determine whether the path is under _lyx or _codeguide.
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
		} else if strings.HasPrefix(tracked, "_codeguide") || tracked == "_codeguide" {
			// _codeguide pollution is report-only: no junction is wired for _codeguide yet.
			entries = append(entries, PollutionEntry{
				Path:       tracked,
				ReportOnly: true,
			})
		}
	}

	return entries, nil
}
