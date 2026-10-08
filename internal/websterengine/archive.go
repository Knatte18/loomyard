// archive.go implements webster's own archive-never-refuse primitives: FirstFreeArchivePath and
// ArchiveStateFile, plus a webster-owned ArchiveReportsDir.
// These are deliberately module-local rather than shared, since archive layout is part of
// webster's own contract shape.
// firstFreeArchivePath is the shared same-second collision rule every archive helper in this
// package reuses (including outcome.go's archiveStaleOutcome);
// archiveStateFile and archiveReportsDir are the --fresh crash/resume escape's two halves, wired
// into state and runlevel in batch 7.

package websterengine

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// archiveTimestampFormat is the UTC compact timestamp format webster archive helpers share.
const archiveTimestampFormat = "20060102T150405Z"

// firstFreeArchivePath returns the first free path in the sequence candidate(""), candidate("-1"), ...
func firstFreeArchivePath(candidate func(suffix string) string) (string, error) {
	for n := 0; ; n++ {
		suffix := ""
		if n > 0 {
			suffix = fmt.Sprintf("-%d", n)
		}
		path := candidate(suffix)
		if _, err := os.Stat(path); err != nil {
			if os.IsNotExist(err) {
				return path, nil
			}
			return "", err
		}
	}
}

// archiveStateFile renames websterDir's state.json with a UTC timestamp, if present.
func archiveStateFile(websterDir string, now func() time.Time) (string, error) {
	path := filepath.Join(websterDir, stateFileName)
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("websterengine: stat state file %s: %w", path, err)
	}

	stamp := now().UTC().Format(archiveTimestampFormat)
	target, err := firstFreeArchivePath(func(suffix string) string {
		return filepath.Join(websterDir, fmt.Sprintf("state-%s%s.json", stamp, suffix))
	})
	if err != nil {
		return "", fmt.Errorf("websterengine: find archive target for state file %s: %w", path, err)
	}

	if err := os.Rename(path, target); err != nil {
		return "", fmt.Errorf("websterengine: archive stale state file %s: %w", path, err)
	}
	return target, nil
}

// archiveReportsDir renames reportsDir with a UTC timestamp and recreates an empty one.
func archiveReportsDir(reportsDir string, now func() time.Time) error {
	if _, err := os.Stat(reportsDir); err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("websterengine: stat reports dir %s: %w", reportsDir, err)
		}
	} else {
		stamp := now().UTC().Format(archiveTimestampFormat)
		parent := filepath.Dir(reportsDir)
		base := filepath.Base(reportsDir)
		target, err := firstFreeArchivePath(func(suffix string) string {
			return filepath.Join(parent, fmt.Sprintf("%s-%s%s", base, stamp, suffix))
		})
		if err != nil {
			return fmt.Errorf("websterengine: find archive target for reports dir %s: %w", reportsDir, err)
		}
		if err := os.Rename(reportsDir, target); err != nil {
			return fmt.Errorf("websterengine: archive stale reports dir %s: %w", reportsDir, err)
		}
	}

	if err := os.MkdirAll(reportsDir, 0o755); err != nil {
		return fmt.Errorf("websterengine: recreate reports dir %s: %w", reportsDir, err)
	}
	return nil
}

// ArchiveRunRecord moves every entry of geom.WebsterDir into dest, so the next run finds no state and starts fresh over the live plan only.
// dest is told and never derived;
// this function knows nothing of what it is archiving for.
// It refuses with ErrRunBusy while a run holds the run lock, then holds the state-mutation lease across the moves.
// It is idempotent for crash resume: an absent WebsterDir, or an entry already moved, is skipped.
// An entry present at both source and destination is an error and nothing is moved, since either copy may be the stale one.
// The rendered fork prompts are cleared as the --fresh escape does, since they are re-renderable and name the retired run's batches.
func ArchiveRunRecord(geom Geometry, dest string) error {
	if err := os.MkdirAll(geom.ScratchDir, 0o755); err != nil {
		return fmt.Errorf("websterengine: create webster scratch dir %s: %w", geom.ScratchDir, err)
	}
	runLock, locked, err := lock.TryAcquireWriteLock(filepath.Join(geom.ScratchDir, runLockName))
	if err != nil {
		return fmt.Errorf("websterengine: acquire run lock in %s: %w", geom.ScratchDir, err)
	}
	if !locked {
		return fmt.Errorf("%w: %q (run.lock held); way forward: wait for the run to finish, then retry", ErrRunBusy, geom.ScratchDir)
	}
	defer runLock.Release()

	lease, err := AcquireStateMutation(geom.ScratchDir)
	if err != nil {
		return err
	}
	defer lease.Release()

	entries, err := os.ReadDir(geom.WebsterDir)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("websterengine: read webster dir %s: %w", geom.WebsterDir, err)
	}

	for _, e := range entries {
		to := filepath.Join(dest, e.Name())
		if _, err := os.Lstat(to); err == nil {
			from := filepath.Join(geom.WebsterDir, e.Name())
			return fmt.Errorf("websterengine: archive run record: %s exists at both %s and %s; way forward: remove whichever copy is stale, then re-step", e.Name(), from, to)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("websterengine: stat archive target %s: %w", to, err)
		}
	}

	if len(entries) > 0 {
		if err := os.MkdirAll(dest, 0o755); err != nil {
			return fmt.Errorf("websterengine: create archive dir %s: %w", dest, err)
		}
	}
	for _, e := range entries {
		from := filepath.Join(geom.WebsterDir, e.Name())
		if err := os.Rename(from, filepath.Join(dest, e.Name())); err != nil {
			return fmt.Errorf("websterengine: archive %s into %s: %w", from, dest, err)
		}
	}

	return clearRenderedPrompts(geom.PromptsDir)
}

// archiveRunInPlace archives the run record inside the webster dir, so the next run finds no state and starts a new one.
// state.json and the reports dir are renamed with a UTC stamp, the reports dir is recreated empty, and the rendered fork prompts are cleared.
func archiveRunInPlace(geom Geometry, now func() time.Time) error {
	if _, err := archiveStateFile(geom.WebsterDir, now); err != nil {
		return err
	}
	if err := archiveReportsDir(geom.ReportsDir, now); err != nil {
		return err
	}
	return clearRenderedPrompts(geom.PromptsDir)
}

// ArchiveRunAfterReset archives the run record in place once a reset to the run's start has moved the branch, or has no branch move to make.
// It first judges every pending audit finding against the worktree's HEAD, which is the tree the next run starts from:
// it refuses with ErrPendingAuditFindings, archiving nothing, only where the caller can clear the state, each refusal naming the clearing step and then `lyx webster reset --to start`.
// Those are an uncleared contract file a fork wrote last, a plan path that differs from the recorded plan, and a suspect path that differs from HEAD.
// Every other pending finding, and every batch record's uncheckable entries, is dropped with the archived record, and the returned warnings name each drop.
// A nil state has nothing to judge, and archives whatever record is on disk.
func ArchiveRunAfterReset(engine shuttleengine.Engine, geom Geometry, st *State) ([]string, error) {
	var warnings []string
	if st != nil && hasPendingFindings(st) {
		head, err := geom.git().HeadSHA(geom.WorktreeRoot)
		if err != nil {
			return nil, err
		}
		if warnings, err = checkPendingFindings(engine, geom, st, head, resetPendingGuard()); err != nil {
			return nil, err
		}
	}
	if err := archiveRunInPlace(geom, time.Now); err != nil {
		return nil, err
	}
	return warnings, nil
}

// uncheckableBatchNumbers returns, sorted, the numbers of st's batches whose record lists uncheckable entries, each of which counts as a pending finding.
func uncheckableBatchNumbers(st *State) []int {
	var numbers []int
	for number, bs := range st.Batches {
		if bs != nil && len(bs.Uncheckable) > 0 {
			numbers = append(numbers, number)
		}
	}
	sort.Ints(numbers)
	return numbers
}

// hasPendingFindings reports whether st holds a pending audit finding or a batch with uncheckable entries.
func hasPendingFindings(st *State) bool {
	return len(st.PendingAuditFindings) > 0 || len(uncheckableBatchNumbers(st)) > 0
}

// pendingGuard is the caller-specific wording of checkPendingFindings.
type pendingGuard struct {
	// verb is what drops the findings, opening each refusal and warning.
	verb string
	// rerun is the command the caller's way forward ends in.
	rerun string
	// baseName names the commit the suspect paths are judged against in a refusal.
	baseName string
	// suspectWayForward is the way forward for suspect paths that differ from base.
	suspectWayForward func(base string, differing []string) string
}

// freshPendingGuard is the wording of `run --fresh`, where a differing suspect path is reset away with the branch.
// The reset route ends in a plain run, since the reset archives the run record, while the steps that follow no reset rerun `--fresh`.
func freshPendingGuard() pendingGuard {
	return pendingGuard{
		verb:              "--fresh",
		rerun:             stepRun + " --fresh",
		baseName:          "the run's start commit",
		suspectWayForward: func(string, []string) string { return resetToStartSteps(stepRun) },
	}
}

// resetPendingGuard is the wording of `reset --to start`, where a differing suspect path is restored to HEAD with git.
func resetPendingGuard() pendingGuard {
	return pendingGuard{
		verb:     "reset --to start",
		rerun:    stepResetToStart,
		baseName: "HEAD",
		suspectWayForward: func(base string, differing []string) string {
			return wayForwardSteps(restoreStep(base, differing), stepResetToStart)
		},
	}
}

// checkPendingFindings judges st's pending audit findings, and the suspect paths of batches with uncheckable entries, against base, and returns one drop warning per finding.
// It refuses with ErrPendingAuditFindings where the caller can clear the state:
// a contract file a fork wrote last, a suspect path that differs from base, and a plan path that differs from the recorded plan while restore-plan can undo that, either because the store holds the recorded copy or because the file was never recorded.
// An unverifiable path, a pathless finding and a differing plan path whose recorded copy is missing from the store are dropped, since no verb could restore the last; its warning says so.
func checkPendingFindings(engine shuttleengine.Engine, geom Geometry, st *State, base string, guard pendingGuard) ([]string, error) {
	uncheckableBatches := uncheckableBatchNumbers(st)
	var allPaths []string
	seen := map[string]bool{}
	for _, f := range st.PendingAuditFindings {
		for _, p := range f.Paths {
			if !seen[p] {
				seen[p] = true
				allPaths = append(allPaths, p)
			}
		}
	}
	for _, n := range uncheckableBatches {
		for _, sp := range st.Batches[n].SuspectPaths {
			if !seen[sp.Path] {
				seen[sp.Path] = true
				allPaths = append(allPaths, sp.Path)
			}
		}
	}
	writes, err := contractWritesFor(engine, st, geom, allPaths)
	if err != nil {
		return nil, err
	}
	contracts, err := splitContractPaths(geom, writes, allPaths)
	if err != nil {
		return nil, err
	}
	if len(contracts.Uncleared) > 0 {
		return nil, fmt.Errorf("%w: %s would drop pending audit findings while %s", ErrPendingAuditFindings, guard.verb, contractDeleteClause(contracts.Uncleared, guard.rerun))
	}
	planPaths, paths, err := splitPlanPaths(geom, contracts.Rest)
	if err != nil {
		return nil, err
	}
	differing, _, err := checkSuspectPaths(geom, st, base, paths)
	if err != nil {
		return nil, err
	}
	if len(differing) > 0 {
		return nil, fmt.Errorf("%w: %s would drop pending audit findings while their suspect paths still differ from %s %s: %s; %s", ErrPendingAuditFindings, guard.verb, guard.baseName, base, strings.Join(differing, ", "), guard.suspectWayForward(base, differing))
	}
	planDiffering, _, err := checkSuspectPaths(geom, st, base, planPaths)
	if err != nil {
		return nil, err
	}
	var restorable []string
	noCopy := map[string]bool{}
	for _, p := range planDiffering {
		name, err := planFileName(geom, p)
		if err != nil {
			return nil, err
		}
		hash, recorded := st.PlanFileHashes[name]
		if !recorded {
			restorable = append(restorable, p)
			continue
		}
		has, err := planBaselineHas(geom.WebsterDir, hash)
		if err != nil {
			return nil, err
		}
		if has {
			restorable = append(restorable, p)
		} else {
			noCopy[p] = true
		}
	}
	if len(restorable) > 0 {
		return nil, fmt.Errorf("%w: %s would drop pending audit findings while plan file(s) differ from the plan the run recorded: %s; way forward: %s", ErrPendingAuditFindings, guard.verb, strings.Join(restorable, ", "), planPathClause(strconv.Quote(guard.rerun)))
	}
	var warnings []string
	for _, f := range st.PendingAuditFindings {
		w := fmt.Sprintf("%s dropped pending audit finding %s: %s", guard.verb, f.ID, f.Detail)
		var lost []string
		for _, p := range f.Paths {
			if noCopy[p] {
				lost = append(lost, p)
			}
		}
		if len(lost) > 0 {
			w += fmt.Sprintf("; plan file(s) %s differ from the recorded plan and their recorded copy is missing from the plan baseline store, so no verb could restore them", strings.Join(lost, ", "))
		}
		warnings = append(warnings, w)
	}
	for _, n := range uncheckableBatches {
		warnings = append(warnings, fmt.Sprintf("%s dropped batch %02d's uncheckable findings: %s", guard.verb, n, strings.Join(st.Batches[n].Uncheckable, ", ")))
	}
	return warnings, nil
}
