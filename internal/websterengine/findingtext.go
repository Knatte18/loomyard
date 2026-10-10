// findingtext.go holds the pure builders every audit and --fresh refusal shares:
// the findings clause, which names each finding once with the path it carries,
// and the reason a path cannot be checked, which replaces a bare "cannot be checked".
// Each refusal ends in wayForwardSteps' numbered list, built from the step helpers here.

package websterengine

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/lyxdirs"
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

	// noteForkWroteLast is the note of a contract file a fork wrote after Master's last write.
	noteForkWroteLast = "a fork wrote it after Master's last write"
	// noteClearedContract is the note of a contract file the evidence clears.
	noteClearedContract = "cleared: absent, or Master wrote it last"

	// stepAcceptAudit is the step that accepts the pending findings.
	stepAcceptAudit = "lyx webster accept-audit"
	// stepRestorePlan is the step that restores the plan files the run recorded, or accepts an edit to a card no batch has begun.
	stepRestorePlan = "lyx webster restore-plan (or lyx webster rebaseline --card NN for an edit to a card no batch has begun)"
	// stepResetToStart is the step that moves the task branch back to the run's start commit.
	stepResetToStart = "lyx webster reset --to start"
	// stepRun is the plain step that runs, or starts, the run.
	stepRun = "lyx webster run"
)

// findingItem is one correctness finding as the findings clause names it.
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

// pathSuffix renders the parenthetical that carries the finding's paths and their notes, or "" when the detail already says it all.
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

// uncheckableReason says why nothing the run recorded can check path, or reports uncheckable == false when something can.
// It mirrors checkSuspectPaths' unverifiable cases that do not depend on a base commit:
// a finding with no path, a plan path with no recorded hashes, and a path outside the task worktree's tracked tree.
// The error is a link-resolution or git probe failure.
func uncheckableReason(geom Geometry, st *State, path string) (reason string, uncheckable bool, err error) {
	if _, pathless := pathlessEntryClass(path); path == "" || pathless {
		return reasonNoPath, true, nil
	}
	lexical := resolveWritePath(geom.WorktreeRoot, path)
	canon, err := canonicalPath(lexical)
	if err != nil {
		return "", false, err
	}
	planDir, err := canonicalPath(geom.PlanDir)
	if err != nil {
		return "", false, err
	}
	if pathWithin(planDir, canon) {
		if len(st.PlanFileHashes) == 0 {
			return reasonNoPlanCopy, true, nil
		}
		return "", false, nil
	}
	scratch, err := canonicalPath(geom.ScratchDir)
	if err != nil {
		return "", false, err
	}
	if pathWithin(scratch, canon) {
		return reasonScratch, true, nil
	}
	if _, ok, err := trackedRel(geom.git(), geom.WorktreeRoot, path); err != nil || ok {
		return "", false, err
	}
	root, err := canonicalPath(geom.WorktreeRoot)
	if err != nil {
		return "", false, err
	}
	lyxLink := filepath.Join(geom.WorktreeRoot, lyxdirs.LyxDirName)
	lyxReal, err := canonicalPath(lyxLink)
	if err != nil {
		return "", false, err
	}
	dotLyx, err := canonicalPath(filepath.Join(geom.WorktreeRoot, lyxdirs.DotLyxDirName))
	if err != nil {
		return "", false, err
	}
	switch {
	case !pathWithin(root, canon):
		return reasonOutsideWorktree, true, nil
	case pathWithin(lyxReal, canon), pathWithin(lyxLink, lexical):
		return fmt.Sprintf("under `%s`, outside the task worktree's tracked tree", lyxdirs.LyxDirName), true, nil
	case pathWithin(dotLyx, canon):
		return fmt.Sprintf("under `%s`", lyxdirs.DotLyxDirName), true, nil
	default:
		return reasonIgnored, true, nil
	}
}

// restoreStep is the step that restores paths to head with git.
func restoreStep(head string, paths []string) string {
	return fmt.Sprintf("git checkout %s -- %s (rm a path the head does not hold)", head, strings.Join(paths, " "))
}

// resetToStartSteps is the reset route: move the branch back to the run's start commit, then rerun.
func resetToStartSteps(rerun string) string {
	return wayForwardSteps(stepResetToStart, rerun)
}
