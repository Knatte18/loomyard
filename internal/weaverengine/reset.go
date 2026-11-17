// reset.go holds Reset, the engine of `lym weaver reset`.
// It starts a Weaver run over.
// It moves the task branch back to the run's start commit and restores the plan to what it was when the run began.
// It then archives the run record and commits the result.

package weaverengine

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/gatefixer"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/runplan"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// resetPendingFileName is the reset-pending marker's name inside the scratch directory.
// A reset writes it before it moves anything and removes it once its commit lands.
const resetPendingFileName = "reset-pending"

// resetCommitMessage is the message of the commit that holds a reset's plan restore and archived record.
const resetCommitMessage = "weaver: reset"

// resetRerun is the command a failed or interrupted reset is finished with.
const resetRerun = "lym weaver reset"

// ResetDeps is everything Reset is told.
type ResetDeps struct {
	// Geom is the told geometry.
	Geom Geometry
	// Runner probes for the strands of the run's recorded attempts and fixers.
	Runner StrandRunner
	// Reed lists the live strands, which a corrupt state file's reset refuses over.
	Reed shuttleengine.ReedOps
	// StateHistory returns the state file's committed versions, newest first.
	StateHistory func() ([]StateVersion, error)
	// ReadPlanAt returns the plan file name as the records commit rev held it, and false when it was absent.
	ReadPlanAt func(rev, name string) ([]byte, bool, error)
	// ResetCode moves the task branch to sha, discarding the run's own uncommitted paths.
	ResetCode func(sha string, ownPaths []string) error
	// RecordPending reports whether the run directory or the plan directory holds uncommitted changes.
	RecordPending func() (bool, error)
	// Commit commits the run directory and the plan directory with msg.
	Commit func(msg string) error
	// DirtyTracked returns the tracked paths of the task worktree that hold uncommitted changes.
	DirtyTracked func() ([]string, error)
	// Now stamps the archived names; nil takes the wall clock.
	Now func() time.Time
}

// ResetResult is what a Reset that went through did.
type ResetResult struct {
	// StartSHA is the commit the task branch moved to, empty when the call only finished an earlier reset.
	StartSHA string
	// PlanCommit is the records commit the plan was restored to, empty when the call only finished an earlier reset.
	PlanCommit string
	// Archived names the entries the run directory's record was renamed to.
	Archived []string
	// Finished reports that the call completed an earlier reset's commit instead of resetting.
	Finished bool
}

// resetRefusal returns a refusal that changes nothing, ending in its way forward.
func resetRefusal(reason, wayForward string) error {
	return fmt.Errorf("weaver: reset refused: %s; way forward: %s", reason, wayForward)
}

// Reset starts the Weaver run over, or finishes the commit of an earlier reset that died before it.
// It refuses, changing nothing, in this order:
// while the run lock is held;
// over an absent state file, unless a reset's marker stands with its commit still pending;
// over a corrupt state file with no committed version that parses, or while a Weaver strand is live;
// over a state with no begun batch;
// while a merge is in progress;
// while a probe by output file finds a strand of the run, unless stopStrands stops it;
// and while a tracked path the run did not leave is uncommitted.
// The uncommitted edits of a strand it stopped count as the run's own.
// It holds the run lock for its whole call.
// It then writes the marker and moves the task branch to the run's start commit.
// It restores the plan to its content at the commit the run recorded and archives the run record in place.
// It clears the scratch directory but its locks and the marker, and commits the restore and the record together.
// A failed commit is returned naming the re-run, which finishes it.
func Reset(deps ResetDeps, stopStrands bool) (ResetResult, error) {
	geom := deps.Geom
	if err := os.MkdirAll(geom.ScratchDir, 0o755); err != nil {
		return ResetResult{}, fmt.Errorf("weaver: reset: create %s: %w", geom.ScratchDir, err)
	}
	runLock, acquired, err := lock.TryAcquireWriteLock(filepath.Join(geom.ScratchDir, runLockName))
	if err != nil {
		return ResetResult{}, fmt.Errorf("weaver: reset: acquire %s in %s: %w", runLockName, geom.ScratchDir, err)
	}
	if !acquired {
		return ResetResult{}, resetRefusal(fmt.Sprintf("the row's run lock %s is held in %s", runLockName, geom.ScratchDir),
			"wait for the row to stop, or pause it with `lym loom pause --after` the implementer row, then re-run `"+resetRerun+"`")
	}
	defer func() { _ = runLock.Release() }()

	st, loadErr := LoadState(geom.RunDir, geom.ScratchDir)
	switch {
	case loadErr != nil:
		if st, err = resetOverCorruptState(deps, loadErr); err != nil {
			return ResetResult{}, err
		}
	case st == nil:
		return resetOverAbsentState(deps)
	}

	if !BegunBatches(st) {
		return ResetResult{}, resetRefusal("the state records no begun batch, so no start commit is recorded and nothing needs a reset",
			"run the row to begin a batch first")
	}
	merging, err := geom.git().MergeInProgress(geom.WorktreeRoot)
	if err != nil {
		return ResetResult{}, fmt.Errorf("weaver: reset: probe a merge in progress in %s: %w", geom.WorktreeRoot, err)
	}
	if merging {
		return ResetResult{}, resetRefusal(fmt.Sprintf("the task worktree %s has a merge in progress", geom.WorktreeRoot),
			"finish the merge or abort it, then re-run `"+resetRerun+"`")
	}

	stopped, err := settleRecordedStrands(deps, st, stopStrands)
	if err != nil {
		return ResetResult{}, err
	}
	own := ownUncommittedPaths(st)
	if stopped {
		dirty, err := deps.DirtyTracked()
		if err != nil {
			return ResetResult{}, fmt.Errorf("weaver: reset: read the task worktree's uncommitted tracked paths: %w", err)
		}
		for _, path := range dirty {
			if !slices.Contains(own, path) {
				own = append(own, path)
			}
		}
		sort.Strings(own)
	}
	if err := refuseForeignPaths(deps, own); err != nil {
		return ResetResult{}, err
	}

	startSHA := firstBatchStart(st)
	if startSHA == "" {
		return ResetResult{}, resetRefusal("no batch recorded a start commit", "move the task branch back by hand with git, then remove "+filepath.Join(geom.RunDir, stateFileName))
	}
	marker := filepath.Join(geom.ScratchDir, resetPendingFileName)
	if err := os.WriteFile(marker, []byte(startSHA+"\n"), 0o644); err != nil {
		return ResetResult{}, fmt.Errorf("weaver: reset: write the marker %s: %w", marker, err)
	}
	if err := deps.ResetCode(startSHA, own); err != nil {
		return ResetResult{}, fmt.Errorf("weaver: reset: move the task branch to %s: %w; way forward: repair what the error names, then re-run `%s`", startSHA, err, resetRerun)
	}
	if err := restorePlan(deps, st.PlanRecordsCommit); err != nil {
		return ResetResult{}, err
	}
	archived, err := archiveAndClear(deps)
	if err != nil {
		return ResetResult{}, err
	}
	if err := commitReset(deps, marker); err != nil {
		return ResetResult{}, err
	}
	return ResetResult{StartSHA: startSHA, PlanCommit: st.PlanRecordsCommit, Archived: archived}, nil
}

// resetOverAbsentState handles a reset that finds no state file.
// A standing marker with uncommitted changes in the records means an earlier reset's commit is still owed.
// It commits them and removes the marker.
// A marker with nothing pending is removed, since its reset's commit landed.
// With no marker nothing began, so it refuses and commits nothing, which keeps plan edits under review out of a commit.
func resetOverAbsentState(deps ResetDeps) (ResetResult, error) {
	marker := filepath.Join(deps.Geom.ScratchDir, resetPendingFileName)
	if _, err := os.Stat(marker); err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return ResetResult{}, fmt.Errorf("weaver: reset: stat the marker %s: %w", marker, err)
		}
		return ResetResult{}, resetRefusal("no Weaver run has begun, so nothing needs a reset", "run the row first")
	}
	pending, err := deps.RecordPending()
	if err != nil {
		return ResetResult{}, fmt.Errorf("weaver: reset: read whether the records hold uncommitted changes: %w", err)
	}
	if pending {
		if err := commitReset(deps, marker); err != nil {
			return ResetResult{}, err
		}
		return ResetResult{Finished: true}, nil
	}
	if err := os.Remove(marker); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return ResetResult{}, fmt.Errorf("weaver: reset: remove the marker %s: %w", marker, err)
	}
	return ResetResult{Finished: true}, nil
}

// resetOverCorruptState returns the newest committed version of the state file that parses.
// It refuses with the manual repair when none does, and while reed lists a live strand of a Weaver role.
func resetOverCorruptState(deps ResetDeps, loadErr error) (*State, error) {
	var versions []StateVersion
	if deps.StateHistory != nil {
		var err error
		if versions, err = deps.StateHistory(); err != nil {
			versions = nil
		}
	}
	var parsed *State
	for _, version := range versions {
		var candidate State
		if json.Unmarshal(version.Data, &candidate) == nil {
			parsed = &candidate
			break
		}
	}
	if parsed == nil {
		return nil, resetRefusal(fmt.Sprintf("the state file is corrupt (%v) and no committed version of it parses", loadErr),
			"remove the corrupt state file "+filepath.Join(deps.Geom.RunDir, stateFileName)+
				" and move the task branch and the plan directory back by hand with git, to where each stood before the run's first commit on it")
	}
	guid, err := liveWeaverStrand(deps.Reed)
	if err != nil {
		return nil, err
	}
	if guid != "" {
		return nil, resetRefusal(fmt.Sprintf("the state file is corrupt and the Weaver strand %s is live", guid),
			fmt.Sprintf("run `lym reed remove %s`, then re-run `%s`", guid, resetRerun))
	}
	return parsed, nil
}

// liveWeaverStrand returns the guid of a live strand of the batch role or the verify-fixer role, empty when none is live.
func liveWeaverStrand(reed shuttleengine.ReedOps) (string, error) {
	status, err := reed.Status()
	if err != nil {
		return "", fmt.Errorf("weaver: reset: read the strands reed lists: %w", err)
	}
	for _, strand := range status.Strands {
		if !strand.Live {
			continue
		}
		parsed, err := agentname.Parse(strand.Name)
		if err != nil {
			continue
		}
		if agentname.MatchesRole(parsed.Role, BatchStrandRole) || agentname.MatchesRole(parsed.Role, gatefixer.RoleVerifyFixer) {
			return strand.GUID, nil
		}
	}
	return "", nil
}

// ownUncommittedPaths returns the union of the paths the run's attempts left uncommitted, sorted.
func ownUncommittedPaths(st *State) []string {
	var own []string
	for _, attempt := range st.Attempts {
		for _, path := range attempt.Uncommitted {
			if !slices.Contains(own, path) {
				own = append(own, path)
			}
		}
	}
	sort.Strings(own)
	return own
}

// refuseForeignPaths refuses while a tracked path outside own holds an uncommitted change.
func refuseForeignPaths(deps ResetDeps, own []string) error {
	dirty, err := deps.DirtyTracked()
	if err != nil {
		return fmt.Errorf("weaver: reset: read the task worktree's uncommitted tracked paths: %w", err)
	}
	var foreign []string
	for _, path := range dirty {
		if !slices.Contains(own, path) {
			foreign = append(foreign, path)
		}
	}
	if len(foreign) == 0 {
		return nil
	}
	return resetRefusal("tracked changes the run did not leave would be lost: "+strings.Join(foreign, ", "),
		fmt.Sprintf("commit them on the task branch, or restore each with `git checkout -- <path>`, then re-run `%s`", resetRerun))
}

// settleRecordedStrands probes by output file for the strand of every attempt and fixer the state records.
// A probe error refuses with the error.
// A run found refuses unless stopStrands is set, and with it each run found is stopped through its handle.
// It reports whether it stopped at least one run.
func settleRecordedStrands(deps ResetDeps, st *State, stopStrands bool) (stopped bool, err error) {
	var outputs []string
	for _, attempt := range st.Attempts {
		if attempt.ReportPath != "" {
			outputs = append(outputs, attempt.ReportPath)
		}
	}
	for _, fixer := range st.Fixers {
		if fixer.ReplyPath != "" {
			outputs = append(outputs, fixer.ReplyPath)
		}
	}

	var found []*shuttleengine.Run
	for _, output := range outputs {
		abs, err := filepath.Abs(output)
		if err != nil {
			return false, fmt.Errorf("weaver: reset: resolve %s: %w", output, err)
		}
		run, ok, err := deps.Runner.ProbeGated(shuttleengine.Spec{OutputFiles: []string{abs}}, nil)
		if err != nil {
			return false, resetRefusal(fmt.Sprintf("probing for a run that wrote %s failed: %v", output, err),
				"follow the way forward the probe's error names, then re-run `"+resetRerun+"`")
		}
		if ok {
			found = append(found, run)
		}
	}
	if len(found) == 0 {
		return false, nil
	}
	if !stopStrands {
		return false, resetRefusal(fmt.Sprintf("%d strand(s) of this run are still live, such as %s", len(found), found[0].StrandGUID()),
			"re-step the row so it attaches, under `lym loom pause --after` the implementer row, or run `"+resetRerun+" --stop-strand` to stop them")
	}
	for _, run := range found {
		if err := run.Stop(); err != nil {
			guid := run.StrandGUID()
			return false, resetRefusal(fmt.Sprintf("could not stop strand %s: %v", guid, err),
				fmt.Sprintf("run `lym reed remove %s`, then re-run `%s`", guid, resetRerun))
		}
	}
	return true, nil
}

// firstBatchStart returns the start commit the lowest-numbered batch recorded, empty when none recorded one.
func firstBatchStart(st *State) string {
	first, start := 0, ""
	for number, record := range st.Batches {
		if record == nil || record.StartSHA == "" {
			continue
		}
		if start == "" || number < first {
			first, start = number, record.StartSHA
		}
	}
	return start
}

// restorePlan writes the plan directory back to its content at the records commit rev.
// The plan is parsed at that commit through ReadPlanAt, each plan file is written back,
// and each plan file the directory holds that was absent there is removed.
// An empty rev restores nothing.
func restorePlan(deps ResetDeps, rev string) error {
	if rev == "" {
		return nil
	}
	planDir := deps.Geom.PlanDir
	names := map[string][]byte{}
	read := func(name string) ([]byte, error) {
		data, found, err := deps.ReadPlanAt(rev, name)
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, fmt.Errorf("%s: %w", name, fs.ErrNotExist)
		}
		names[name] = data
		return data, nil
	}
	if _, err := planparser.ParsePlanFrom(planDir, read); err != nil {
		return fmt.Errorf("weaver: reset: parse the plan as records commit %s held it: %w", rev, err)
	}

	entries, err := os.ReadDir(planDir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("weaver: reset: read %s: %w", planDir, err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || filepath.Ext(name) != ".md" {
			continue
		}
		if _, held := names[name]; held {
			continue
		}
		data, found, err := deps.ReadPlanAt(rev, name)
		if err != nil {
			return fmt.Errorf("weaver: reset: read plan file %s at records commit %s: %w", name, rev, err)
		}
		if found {
			names[name] = data
			continue
		}
		if err := os.Remove(filepath.Join(planDir, name)); err != nil {
			return fmt.Errorf("weaver: reset: remove plan file %s, which the run's start did not hold: %w", name, err)
		}
	}
	for name, data := range names {
		if err := runplan.WriteFileAtomic(planDir, filepath.Join(planDir, name), data); err != nil {
			return fmt.Errorf("weaver: reset: restore plan file %s: %w", name, err)
		}
	}
	return nil
}

// archiveAndClear archives the run record in place and clears the scratch directory but its locks and the marker.
// It returns the names the record's entries were renamed to.
func archiveAndClear(deps ResetDeps) ([]string, error) {
	geom := deps.Geom
	now := deps.Now
	if now == nil {
		now = time.Now
	}
	before, err := entryNames(geom.RunDir)
	if err != nil {
		return nil, err
	}
	if err := archiveRunInPlace(geom, now); err != nil {
		return nil, err
	}
	after, err := entryNames(geom.RunDir)
	if err != nil {
		return nil, err
	}
	var archived []string
	for _, name := range after {
		if !slices.Contains(before, name) {
			archived = append(archived, name)
		}
	}

	entries, err := os.ReadDir(geom.ScratchDir)
	if err != nil {
		return nil, fmt.Errorf("weaver: reset: read %s: %w", geom.ScratchDir, err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if name == resetPendingFileName || strings.HasSuffix(name, ".lock") {
			continue
		}
		if err := os.RemoveAll(filepath.Join(geom.ScratchDir, name)); err != nil {
			return nil, fmt.Errorf("weaver: reset: clear %s: %w", name, err)
		}
	}
	return archived, nil
}

// entryNames returns the names of the entries of dir, none for an absent directory.
func entryNames(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("weaver: reset: read %s: %w", dir, err)
	}
	names := make([]string, len(entries))
	for i, entry := range entries {
		names[i] = entry.Name()
	}
	return names, nil
}

// commitReset commits the plan restore and the archived record, then removes the marker.
// A failed commit leaves the marker and names the re-run that finishes it.
func commitReset(deps ResetDeps, marker string) error {
	if err := deps.Commit(resetCommitMessage); err != nil {
		return fmt.Errorf("weaver: reset: the branch moved and the plan was restored, but committing the restore and the archived record failed: %w; way forward: re-run `%s`, which finishes the commit", err, resetRerun)
	}
	if err := os.Remove(marker); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("weaver: reset: remove the marker %s: %w", marker, err)
	}
	return nil
}
