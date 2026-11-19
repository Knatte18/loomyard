// status.go reports the paired warp and weft status of each worktree, and detects _lym paths tracked in the warp index.
//
// Status lists the warp worktrees and pairs each with its weft sibling.
// It reports each pair's branches, in-sync verdict, junction health and warp pollution.
// A pair is in sync when its weft branch is RecordsBranchName of its warp branch and its junction is healthy.
// DriftReason names the expected weft branch when the branches differ.
//
// Status computes the in-sync verdict from the branch names it has already read, so it runs no second rev-parse.

package fabricengine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// PollutionEntry describes a path tracked in the warp index that belongs in the weft, such as one under _lym.
type PollutionEntry struct {
	// Path is the relative path reported by git ls-files.
	Path string `json:"path"`
	// Remedy is the command that untracks the path and keeps it on disk. Empty when no remedy applies.
	Remedy string `json:"remedy,omitempty"`
}

// PairStatus describes the relationship between one warp worktree and its paired weft sibling.
type PairStatus struct {
	// CodeWorktree is the absolute path to the warp worktree.
	CodeWorktree string `json:"code_worktree"`
	// RecordsWorktree is the absolute path to the expected weft sibling worktree.
	RecordsWorktree string `json:"records_worktree"`
	// WarpBranch is the current branch of the warp worktree (empty if undetermined).
	WarpBranch string `json:"warp_branch"`
	// WeftBranch is the current branch of the weft worktree (empty if missing or undetermined).
	WeftBranch string `json:"weft_branch"`
	// InSync reports whether the weft branch is RecordsBranchName of the warp branch and the junction is healthy.
	InSync bool `json:"in_sync"`
	// DriftReason describes why the pair is out of sync. Empty when InSync is true.
	DriftReason string `json:"drift_reason,omitempty"`
	// JunctionHealthy reports whether the warp _lym junction exists and points to the weft.
	JunctionHealthy bool `json:"junction_healthy"`
	// JunctionReason describes the junction problem. Empty when JunctionHealthy is true.
	JunctionReason string `json:"junction_reason,omitempty"`
	// Pollution lists warp-index paths that should not be tracked there.
	Pollution []PollutionEntry `json:"pollution,omitempty"`
}

// StatusResult is the paired warp and weft status of the worktrees in one fabric layout.
// It holds one PairStatus per warp worktree, in the order the worktrees were listed.
type StatusResult struct {
	// Pairs is the ordered list of warp↔weft pair reports.
	Pairs []PairStatus `json:"pairs"`
}

// Status returns the paired warp and weft status of every worktree reachable from the given layout.
// It also runs warp-pollution detection on each warp index.
// For each warp worktree it reports the branches, in-sync verdict, junction health and tracked _lym paths.
// Per-worktree errors are recorded inline in the pair's DriftReason, JunctionReason or Pollution fields;
// only a failure to list the worktrees is returned as an error.
func (t *Topology) Status(l *lymcwd.Location) (StatusResult, error) {
	entries, err := List(l.WorktreePath())
	if err != nil {
		return StatusResult{}, fmt.Errorf("list worktrees: %w", err)
	}

	var result StatusResult

	for _, entry := range entries {
		warpPath := filepath.FromSlash(entry.Path)
		warpPath = filepath.Clean(warpPath)

		weftPath := RecordsWorktreePath(l, filepath.Base(warpPath))

		pair := PairStatus{
			CodeWorktree:    filepath.ToSlash(warpPath),
			RecordsWorktree: filepath.ToSlash(weftPath),
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

		// The verdict combines branch correspondence with junction health, and the first failing check sets DriftReason.
		expectedWeftBranch := RecordsBranchName(warpBranch)
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

		// Scan the warp index for _lym paths, scoped through this pair's anchor.
		// A subpath-anchored hub tracks its content under <anchor>/_lym, which a root-relative pathspec never matches.
		pollution, pollErr := detectWarpPollution(warpPath, warpLayout.AnchorRel)
		if pollErr != nil {
			// A scan failure does not abort Status. It is recorded inline, and its Remedy stays empty.
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

// detectWarpPollution scans the warp worktree index for _lym paths that must not be tracked in the warp repo.
//
// anchorRel scopes the pathspec through the pair's anchor.
// A subpath-anchored hub keeps its durable tree at <anchor>/_lym, which a bare "_lym" pathspec never matches.
//
// Every match has a junction wired to restore, so its remedy is the git rm --cached command.
// That command untracks the file and leaves it on disk.
// The remedy also says to restore the junction and git-exclude entry.
func detectWarpPollution(warpPath, anchorRel string) ([]PollutionEntry, error) {
	// git ls-files lists only tracked (index) files matching the given pathspecs.
	// Using -- prevents ambiguity when the pathspec looks like a branch name.
	args := []string{"ls-files", "--"}
	for _, spec := range ScopedPathspec(anchorRel, structuralCommittedDirs) {
		args = append(args, filepath.ToSlash(spec))
	}

	out, err := gitexec.Run(args, warpPath)
	if err != nil {
		return nil, fmt.Errorf("scan the warp index for lym-managed paths: %w", err)
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
