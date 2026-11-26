// findingtext.go builds the text that audit and --fresh refusals share.
// That text includes the findings clause, which names each finding once with its path.
// It also covers the reason a path cannot be checked and the status of a records-side path.
// Each refusal ends in a numbered list of steps from the helpers here.

package websterengine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/runplan"
)

const (
	// reasonNoPath is the reason of a finding that names no path.
	reasonNoPath = "the finding names no path"
	// reasonNoPlanCopy is the reason of a plan path when the run recorded no plan hashes.
	reasonNoPlanCopy = "no plan copy recorded"
	// reasonOutsideWorktree is the reason of a path outside the task worktree.
	reasonOutsideWorktree = "outside the task worktree"
	// reasonScratch is the reason of a path under webster's scratch directory.
	reasonScratch = "under webster's scratch directory"
	// reasonIgnored is the reason of a path git ignores.
	reasonIgnored = "ignored by git"
	// reasonNoBatchHead is the reason of a tracked path when the run recorded no batch head to compare it with.
	reasonNoBatchHead = "no batch head recorded to compare against"

	// noteClearedContract is the note of a contract file the evidence clears.
	noteClearedContract = "cleared: absent, or Master wrote it last"
	// noteClearedRecords is the note of a records-side path the evidence clears.
	noteClearedRecords = "cleared: unchanged since Master spawned"
	// noteChangedRecords is the note of a records-side path that differs from what it held when Master spawned.
	noteChangedRecords = "changed since Master spawned"

	// stepAcceptAudit is the step that accepts the pending findings.
	stepAcceptAudit = "lym webster accept-audit"
	// stepRestorePlan is the step that restores the plan files the run recorded;
	// its text also names rebaseline, which accepts an edit to a card no batch has begun.
	stepRestorePlan = "lym webster restore-plan (or lym webster rebaseline --card NN for an edit to a card no batch has begun)"
	// stepResetToStart is the step that moves the task branch back to the run's start commit.
	stepResetToStart = "lym webster reset --to start"
	// stepRun is the plain step that runs, or starts, the run.
	stepRun = "lym webster run"
)

// noteWroteLast is the note of a contract file that the named writer wrote after Master's last write.
func noteWroteLast(writer string) string {
	return writer + " wrote it after Master's last write"
}

// findingItem is one correctness finding: its class, its detail, and the suspect paths it carries.
type findingItem struct {
	Class  string
	Detail string
	// Paths are the finding's suspect paths, empty for a pathless finding.
	Paths []string
}

// findingsClause renders "N correctness finding(s): 1) <class>: <detail>; 2) ...", each finding once.
// A path whose note is set appears as "(<path>: <note>)", a pathless finding carries reasonNoPath,
// and a path without a note is repeated only when the detail does not already name it.
func findingsClause(items []findingItem, notes map[string]string) string {
	parts := make([]string, len(items))
	for i, it := range items {
		parts[i] = fmt.Sprintf("%d) %s: %s%s", i+1, it.Class, it.Detail, pathSuffix(it, notes))
	}
	return fmt.Sprintf("%d correctness finding(s): %s", len(items), strings.Join(parts, "; "))
}

// pathSuffix renders the parenthetical that carries the finding's paths and their notes.
// It returns "" when no path needs writing: none has a note, and the detail names them all.
func pathSuffix(it findingItem, notes map[string]string) string {
	if len(it.Paths) == 0 {
		return " (" + reasonNoPath + ")"
	}
	var parts []string
	for _, p := range it.Paths {
		switch {
		case notes[p] != "":
			parts = append(parts, p+": "+notes[p])
		case !strings.Contains(it.Detail, p):
			parts = append(parts, p)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return " (" + strings.Join(parts, "; ") + ")"
}

// uncheckableReason reports whether what the run recorded cannot check path, and gives the reason when it cannot.
// It covers the unverifiable cases that do not depend on a base commit:
// a finding with no path, a plan path with no recorded hashes, and a path outside the task worktree's tracked tree.
// The error is a link-resolution or git probe failure.
func uncheckableReason(geom Geometry, st *State, path string) (reason string, uncheckable bool, err error) {
	if _, pathless := pathlessEntryClass(path); path == "" || pathless {
		return reasonNoPath, true, nil
	}
	lexical := runplan.ResolveWritePath(geom.PaneCwd, path)
	canon, err := runplan.CanonicalPath(lexical)
	if err != nil {
		return "", false, err
	}
	planDir, err := runplan.CanonicalPath(geom.PlanDir)
	if err != nil {
		return "", false, err
	}
	if runplan.PathWithin(planDir, canon) {
		if len(st.PlanFileHashes) == 0 {
			return reasonNoPlanCopy, true, nil
		}
		return "", false, nil
	}
	scratch, err := runplan.CanonicalPath(geom.ScratchDir)
	if err != nil {
		return "", false, err
	}
	if runplan.PathWithin(scratch, canon) {
		return reasonScratch, true, nil
	}
	if _, ok, err := runplan.TrackedRel(geom.git(), geom.PaneCwd, geom.WorktreeRoot, path); err != nil || ok {
		return "", false, err
	}
	root, err := runplan.CanonicalPath(geom.WorktreeRoot)
	if err != nil {
		return "", false, err
	}
	lymLink := filepath.Join(geom.PaneCwd, lymdirs.LymDirName)
	lymReal, err := runplan.CanonicalPath(lymLink)
	if err != nil {
		return "", false, err
	}
	dotLym, err := runplan.CanonicalPath(filepath.Join(geom.PaneCwd, lymdirs.DotLymDirName))
	if err != nil {
		return "", false, err
	}
	switch {
	case !runplan.PathWithin(root, canon):
		return reasonOutsideWorktree, true, nil
	case runplan.PathWithin(lymReal, canon), runplan.PathWithin(lymLink, lexical):
		return fmt.Sprintf("under `%s`, outside the task worktree's tracked tree", lymdirs.LymDirName), true, nil
	case runplan.PathWithin(dotLym, canon):
		return fmt.Sprintf("under `%s`", lymdirs.DotLymDirName), true, nil
	default:
		return reasonIgnored, true, nil
	}
}

// recordsBaseline returns the records side's HEAD commit, which a records-side path is later compared against.
// It returns "" when there is no `_lym` directory, or none that a commit holds yet.
func recordsBaseline(geom Geometry) (string, error) {
	records := filepath.Join(geom.PaneCwd, lymdirs.LymDirName)
	if _, err := os.Stat(records); errors.Is(err, os.ErrNotExist) {
		return "", nil
	} else if err != nil {
		return "", fmt.Errorf("webster: stat %s: %w", records, err)
	}
	return geom.git().EnclosingCommit(records, "HEAD")
}

// recordsSideStatus reports whether path lies on the run's records side, and whether it is clean there.
// The records side is the `_lym` directory the pane sees, wherever its link resolves.
// Webster's run, reports and plan directories are no part of it.
// Those hold the run's own state and have checks of their own.
// base is the records side's commit from before Master could write.
// Without a base its repository holds, no path counts as on the records side, so none clears.
// A path is clean when its content is what base holds, absence included.
// The error is a link-resolution, stat or git probe failure.
func recordsSideStatus(geom Geometry, base, path string) (onRecords, clean bool, err error) {
	if _, pathless := pathlessEntryClass(path); base == "" || path == "" || pathless {
		return false, false, nil
	}
	canon, err := runplan.CanonicalPath(runplan.ResolveWritePath(geom.PaneCwd, path))
	if err != nil {
		return false, false, err
	}
	records, err := runplan.CanonicalPath(filepath.Join(geom.PaneCwd, lymdirs.LymDirName))
	if err != nil {
		return false, false, err
	}
	if !runplan.PathWithin(records, canon) {
		return false, false, nil
	}
	for _, dir := range []string{geom.WebsterDir, geom.ReportsDir, geom.PlanDir} {
		own, err := runplan.CanonicalPath(dir)
		if err != nil {
			return false, false, err
		}
		if runplan.PathWithin(own, canon) {
			return false, false, nil
		}
	}
	known, err := geom.git().EnclosingCommit(records, base)
	if err != nil || known == "" {
		return false, false, err
	}
	rel, err := filepath.Rel(records, canon)
	if err != nil {
		return false, false, fmt.Errorf("webster: relate %s to %s: %w", canon, records, err)
	}
	now, err := geom.git().WorktreeBlob(records, canon)
	if err != nil {
		return false, false, err
	}
	// The leading "./" makes git read the path against the records directory, not its repository's root.
	then, err := geom.git().CommitBlob(records, base, "./"+filepath.ToSlash(rel))
	if err != nil {
		return false, false, err
	}
	return true, now == then, nil
}

// splitRecordsSidePaths drops every clean records-side path from paths, judged against base.
// It returns the changed records-side paths apart from the rest, each in input order.
func splitRecordsSidePaths(geom Geometry, base string, paths []string) (rest, changed []string, err error) {
	for _, p := range paths {
		onRecords, clean, err := recordsSideStatus(geom, base, p)
		if err != nil {
			return nil, nil, err
		}
		switch {
		case !onRecords:
			rest = append(rest, p)
		case !clean:
			changed = append(changed, p)
		}
	}
	return rest, changed, nil
}

// recordsRestoreStep is the step that restores records-side paths to the records side's commit base.
func recordsRestoreStep(base string, paths []string) string {
	return fmt.Sprintf("restore %s to records-side commit %s (rm a path that commit does not hold)", strings.Join(paths, " "), base)
}

// restoreStep is the step that restores paths to head with git.
func restoreStep(head string, paths []string) string {
	return fmt.Sprintf("git checkout %s -- %s (rm a path the head does not hold)", head, strings.Join(paths, " "))
}

// resetToStartSteps is the reset route: move the branch back to the run's start commit, then rerun.
func resetToStartSteps(rerun string) string {
	return wayForwardSteps(stepResetToStart, rerun)
}
