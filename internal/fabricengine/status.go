// status.go implements the paired warp↔weft status view and warp-pollution detection for fabric.
//
// Status enumerates all warp worktrees via List, pairs each with its weft sibling, reports branch,
// in-sync verdict, junction health, and scans the warp index for any _lyx
// paths that have been accidentally git-tracked (warp pollution).
// A pair is InSync when weftBranch == WeftBranchName(warpBranch),
// and DriftReason states the expected suffixed branch rather than a bare mismatch.
//
// Status computes its in-sync verdict inline (branch correspondence via WeftBranchName, then
// junction health via checkJunctionHealth, both already defined in reconcile.go) rather than
// calling the shared Healthy helper (drift.go) — Status already has warpBranch/weftBranch in hand
// from readBranch, so there is nothing to gain from a second rev-parse round trip through a shared
// helper.

package fabricengine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// PollutionEntry describes a single tracked path in the warp index that should never be committed
// there (e.g. _lyx, which belongs exclusively in the weft).
type PollutionEntry struct {
	// Path is the relative path reported by git ls-files.
	Path string `json:"path"`
	// Remedy is the suggested remediation command. Empty when the entry is report-only.
	Remedy string `json:"remedy,omitempty"`
}

// PairStatus describes the relationship between one warp worktree and its paired weft sibling.
type PairStatus struct {
	// WarpWorktree is the absolute path to the warp worktree.
	WarpWorktree string `json:"warp_worktree"`
	// WeftWorktree is the absolute path to the expected weft sibling worktree.
	WeftWorktree string `json:"weft_worktree"`
	// WarpBranch is the current branch of the warp worktree (empty if undetermined).
	WarpBranch string `json:"warp_branch"`
	// WeftBranch is the current branch of the weft worktree (empty if missing or undetermined).
	WeftBranch string `json:"weft_branch"`
	// InSync reports whether the pair is branch-synchronized (weftBranch ==
	// WeftBranchName(warpBranch)) and the junction is healthy.
	InSync bool `json:"in_sync"`
	// DriftReason describes why the pair is out of sync. Empty when InSync is true.
	DriftReason string `json:"drift_reason,omitempty"`
	// JunctionHealthy reports whether the warp _lyx junction exists and points to the weft.
	JunctionHealthy bool `json:"junction_healthy"`
	// JunctionReason describes the junction problem. Empty when JunctionHealthy is true.
	JunctionReason string `json:"junction_reason,omitempty"`
	// Pollution lists warp-index paths that should not be tracked there.
	Pollution []PollutionEntry `json:"pollution,omitempty"`
}

// StatusResult is the top-level result type returned by Status.
// It contains one PairStatus entry per discovered warp worktree.
type StatusResult struct {
	// Pairs is the ordered list of warp↔weft pair reports.
	Pairs []PairStatus `json:"pairs"`
}

// Status returns the paired warp↔weft status view for all worktrees reachable from the given
// layout, plus warp-pollution detection on the warp index.
// For each warp worktree, it reports branch status, in-sync verdict, junction health, and
// warp-tracked _lyx paths.
// Per-worktree errors are recorded inline in PairStatus.DriftReason / PairStatus.JunctionReason.
func (t *Topology) Status(l *lyxcwd.Location) (StatusResult, error) {
	entries, err := List(l.WorktreePath())
	if err != nil {
		return StatusResult{}, fmt.Errorf("list worktrees: %w", err)
	}

	var result StatusResult

	for _, entry := range entries {
		warpPath := filepath.FromSlash(entry.Path)
		warpPath = filepath.Clean(warpPath)

		weftPath := WeftWorktreePath(l, filepath.Base(warpPath))

		pair := PairStatus{
			WarpWorktree: filepath.ToSlash(warpPath),
			WeftWorktree: filepath.ToSlash(weftPath),
		}

		warpBranch, warpBranchErr := readBranch(warpPath)
		if warpBranchErr != nil {
			pair.DriftReason = fmt.Sprintf("read warp branch: %v", warpBranchErr)
			result.Pairs = append(result.Pairs, pair)
			continue
		}
		pair.WarpBranch = warpBranch

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

		warpLayout, layoutErr := warpLayoutFor(l, warpPath)
		if layoutErr != nil {
			pair.DriftReason = fmt.Sprintf("resolve warp layout: %v", layoutErr)
			result.Pairs = append(result.Pairs, pair)
			continue
		}

		junctionHealthy, junctionReason := checkJunctionHealth(warpLayout)
		pair.JunctionHealthy = junctionHealthy
		pair.JunctionReason = junctionReason

		// Determine pair in-sync status: branch correspondence uses WeftBranchName
		// (fabric's suffixed pairing) rather than warp's equal-name requirement, folded
		// together with junction health exactly as warp's pair-in-sync check (fabric's
		// Healthy) folds both checks into one verdict.
		expectedWeftBranch := WeftBranchName(warpBranch)
		switch {
		case weftBranch != expectedWeftBranch:
			pair.InSync = false
			pair.DriftReason = fmt.Sprintf("warp on %s, weft on %s (want %s)", warpBranch, weftBranch, expectedWeftBranch)
		case !junctionHealthy:
			pair.InSync = false
			pair.DriftReason = junctionReason
		default:
			pair.InSync = true
		}

		// Scan the warp index for _lyx paths that must never be tracked there, scoped through this
		// pair's own anchor — on a subpath-anchored hub the tracked content sits at
		// <anchor>/_lyx/..., which a root-relative pathspec never matches.
		pollution, pollErr := detectWarpPollution(warpPath, warpLayout.AnchorRel)
		if pollErr != nil {
			// Non-fatal: record the error inline and continue. Remedy stays
			// empty since no automated remedy applies to a scan failure.
			pair.Pollution = append(pair.Pollution, PollutionEntry{
				Path: fmt.Sprintf("<scan error: %v>", pollErr),
			})
		} else {
			pair.Pollution = pollution
		}

		result.Pairs = append(result.Pairs, pair)
	}

	return result, nil
}

// detectWarpPollution scans the warp worktree index for _lyx paths that should
// never be tracked in the warp repo.
//
// anchorRel scopes the pathspec through the pair's recorded anchor, exactly as Fabric.Commit scopes
// its own routing prefixes. This is load-bearing rather than tidy: a subpath-anchored hub keeps its
// durable tree at <anchor>/_lyx, which a bare "_lyx" pathspec never matches, so the scan reported a
// genuinely polluted index as clean — a false negative in the one verb that advertises the check.
//
// Every match under _lyx has a junction wired to restore, so the remedy is
// always the git rm --cached command that removes the file from the index
// without deleting it from disk, plus a reminder to restore the
// junction/exclude entry. No pollution class in this scan lacks an automated
// remedy, which is why PollutionEntry has no report-only signal beyond an
// empty Remedy.
func detectWarpPollution(warpPath, anchorRel string) ([]PollutionEntry, error) {
	// git ls-files lists only tracked (index) files matching the given pathspecs.
	// Using -- prevents ambiguity when the pathspec looks like a branch name.
	args := []string{"ls-files", "--"}
	for _, spec := range ScopedPathspec(anchorRel, structuralCommittedDirs) {
		args = append(args, filepath.ToSlash(spec))
	}

	out, lsFilesStderr, exitCode, err := gitexec.RunGit(args, warpPath)
	if err != nil {
		return nil, fmt.Errorf("ls-files: %w", err)
	}
	if exitCode != 0 {
		// A non-zero exit from ls-files means the command itself failed, not just
		// that no files matched; report as an error.
		return nil, fmt.Errorf("ls-files exited %d: %s", exitCode, strings.TrimSpace(lsFilesStderr))
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

		// Offer git rm --cached as the remedy, plus a reminder to restore the
		// junction and exclude entry so lyx topology is intact afterwards.
		remedy := fmt.Sprintf(
			"git -C %s rm --cached -- %s  # then restore junction and git-exclude entry",
			warpPath, tracked,
		)
		entries = append(entries, PollutionEntry{
			Path:   tracked,
			Remedy: remedy,
		})
	}

	return entries, nil
}
