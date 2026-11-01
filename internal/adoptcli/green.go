// green.go is the green end of the adopt check: it lifts the subject's holds, brings its dobby and orch up, starts the lifted runs, resumes the children halted at Finalize's owner check and removes the subject's fix task.

package adoptcli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/battencli"
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fronttask"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/machinename"
	"github.com/Knatte18/loomyard/internal/orchcli"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/tendercli"
	"github.com/Knatte18/loomyard/internal/tenderengine"
)

// greenDeps holds the seams of one subject's green end: the subject, its scratch directory, the name of its worktree (the prime's for `root`) and its project (none for `root`).
// PauseRecord reads the pause record, and Lift lifts the entry a reference names.
// BeamUp brings up the dobby and orch of the named worktree, leaving a live pair alone.
// RunRecord reads the batten run record of a slug, reporting false when none exists, and StartFirstPass starts a slug's first pass detached and returns the envelope of that start, where an envelope whose "ok" is false is a refusal.
// Tasks reads the board's entries, ParentOf names the beam a slug runs under or "" for the prime, and ChildStatus reads the shed status of a slug's own run in its pair, reporting false when it has none.
// ResumeChild runs `lym loom resume` in the pair of a slug.
// Owner returns the machine that owns a slug's run when it is not this one, "" otherwise.
// FrontDeps are the seams a front task's removal reads.
type greenDeps struct {
	Subject        string
	Dir            string
	Worktree       string
	Project        string
	PauseRecord    func() (pauserecord.Record, error)
	Lift           func(ref tenderengine.EntryRef) error
	BeamUp         func(worktree string) error
	RunRecord      func(slug string) (shedengine.Status, bool, error)
	StartFirstPass func(slug string) (map[string]any, error)
	Tasks          func() ([]boardengine.Task, error)
	ParentOf       func(slug string) (string, error)
	ChildStatus    func(slug string) (shedengine.Status, bool, error)
	ResumeChild    func(slug string) error
	Owner          func(slug string) (string, error)
	FrontDeps      fronttask.Deps
}

// greenEnd runs the green end of the subject's adopt check and returns one line for each step that failed, each naming its way forward; the other steps go on.
// It lifts the holds the subject's holds file lists, brings the subject's dobby and orch up, starts the first pass of each lifted run that has a batten run record not done, resumes the children of the subject halted at Finalize's owner check whose run this machine owns, removes the subject's fix task and writes the subject's fix-try count to 0.
// A released task without a run record is left to the dobby, which starts it once its pause is gone.
// A hold lifted by hand before the end is gone and lifts nothing, and a start that races the dobby's own pass reads ok, since the pass that loses the driver lock exits on its own.
func greenEnd(deps greenDeps) []string {
	lifted, lines := liftHolds(deps)

	if err := deps.BeamUp(deps.Worktree); err != nil {
		lines = append(lines, fmt.Sprintf("the dobby and orch of %s were not brought up (%v); way forward: fix the cause it names, then run `lym beam start %s`", deps.Subject, err, deps.Subject))
	}
	lines = append(lines, startLiftedRuns(deps, lifted)...)
	lines = append(lines, resumeHaltedChildren(deps)...)
	lines = append(lines, removeFixTask(deps)...)

	if err := writeTries(deps.Dir, 0); err != nil {
		lines = append(lines, fmt.Sprintf("the fix-try count of %s was not reset (%v); way forward: re-run lym adopt %s", deps.Subject, err, deps.Subject))
	}
	return lines
}

// liftHolds lifts, by scope, caller and pause time, the live hold entry of each slug the subject's holds file lists, then drops every listed slug from the file in one update.
// It returns the slugs whose hold it lifted and a line for each failure.
// A slug a concurrent adopt appended after the read stays listed, for the check of that adopt to lift.
func liftHolds(deps greenDeps) (lifted, lines []string) {
	listed, err := readHolds(deps.Dir)
	if err != nil {
		return nil, []string{fmt.Sprintf("the holds of %s were not read (%v), so none was lifted; way forward: fix the cause it names, then re-run lym adopt %s", deps.Subject, err, deps.Subject)}
	}
	if len(listed) == 0 {
		return nil, nil
	}
	record, err := deps.PauseRecord()
	if err != nil {
		return nil, []string{fmt.Sprintf("the pause record was not read (%v), so no hold of %s was lifted; way forward: fix the cause it names, then re-run lym adopt %s", err, deps.Subject, deps.Subject)}
	}

	for _, slug := range listed {
		index := slices.IndexFunc(record.Live, func(entry pauserecord.Entry) bool {
			return entry.Scope == slug && entry.Caller == pauserecord.HoldCaller
		})
		if index < 0 {
			continue
		}
		entry := record.Live[index]
		if err := deps.Lift(tenderengine.EntryRef{Scope: entry.Scope, Caller: entry.Caller, PausedAt: entry.PausedAt}); err != nil {
			lines = append(lines, fmt.Sprintf("the hold of %s was not lifted (%v); way forward: run `lym tender resume %s`", slug, err, slug))
			continue
		}
		lifted = append(lifted, slug)
	}
	if err := removeHolds(deps.Dir, listed); err != nil {
		lines = append(lines, fmt.Sprintf("the holds file of %s was not cleaned (%v); the next green check of %s drops the slugs it lists", deps.Subject, err, deps.Subject))
	}
	return lifted, lines
}

// startLiftedRuns starts the detached first pass of each slug in lifted whose batten run record exists and is not done, and returns a line for each that did not start.
func startLiftedRuns(deps greenDeps, lifted []string) []string {
	var lines []string
	for _, slug := range lifted {
		hand := fmt.Sprintf("run `lym batten run %s` from the prime", slug)
		status, found, err := deps.RunRecord(slug)
		if err != nil {
			lines = append(lines, fmt.Sprintf("the batten run record of %s was not read (%v), so its first pass did not start; way forward: %s", slug, err, hand))
			continue
		}
		if !found || status.State == shedengine.StateDone {
			continue
		}
		envelope, err := deps.StartFirstPass(slug)
		if err != nil {
			lines = append(lines, fmt.Sprintf("the first pass of %s did not start (%v); way forward: %s", slug, err, hand))
			continue
		}
		if ok, _ := envelope["ok"].(bool); !ok {
			message, _ := envelope["error"].(string)
			lines = append(lines, fmt.Sprintf("the first pass of %s was refused (%s); way forward: %s", slug, message, hand))
		}
	}
	return lines
}

// resumeHaltedChildren runs `lym loom resume` in the pair of each task entry not done that runs directly under the subject, under the prime for `root`, whose run is blocked with a reason opening landingshed.OwnerHaltPrefix and whose effective owner is this machine.
// A child halted for any other reason, a child under another beam and a child owned by another machine are left alone.
// It returns a line for each read or resume that failed.
func resumeHaltedChildren(deps greenDeps) []string {
	tasks, err := deps.Tasks()
	if err != nil {
		return []string{fmt.Sprintf("the board was not read (%v), so no child halted at Finalize's owner check was resumed; way forward: run `lym loom resume` in the pair of each child halted so under %s", err, deps.Subject)}
	}
	slugs := make([]string, 0, len(tasks))
	for _, task := range tasks {
		if task.Kind == boardengine.KindTask && !entryDone(task) {
			slugs = append(slugs, task.Slug)
		}
	}
	slices.Sort(slugs)

	var lines []string
	for _, slug := range slugs {
		resume := fmt.Sprintf("run `lym loom resume` in the pair of %s", slug)
		parent, err := deps.ParentOf(slug)
		if err != nil {
			lines = append(lines, fmt.Sprintf("the parent of %s was not read (%v), so it was not checked for a halt at Finalize's owner check; way forward: if it is halted so, %s", slug, err, resume))
			continue
		}
		if (parent == "" && deps.Subject != boardengine.RootSubject) || (parent != "" && parent != deps.Subject) {
			continue
		}
		status, found, err := deps.ChildStatus(slug)
		if err != nil {
			lines = append(lines, fmt.Sprintf("the status of %s was not read (%v), so it was not checked for a halt at Finalize's owner check; way forward: if it is halted so, %s", slug, err, resume))
			continue
		}
		if !found || status.State != shedengine.StateBlocked || !strings.HasPrefix(status.Error, landingshed.OwnerHaltPrefix) {
			continue
		}
		owner, err := deps.Owner(slug)
		if err != nil {
			lines = append(lines, fmt.Sprintf("the owner of %s was not read (%v), so its halt at Finalize's owner check was not resumed; way forward: %s", slug, err, resume))
			continue
		}
		if owner != "" {
			continue
		}
		if err := deps.ResumeChild(slug); err != nil {
			lines = append(lines, fmt.Sprintf("the run of %s halted at Finalize's owner check was not resumed (%v); way forward: %s", slug, err, resume))
		}
	}
	return lines
}

// removeFixTask removes the subject's adopt-check front task, in whatever status it stands not done, and returns a line when its removal fails.
// The removal ends a running task's run first, and the line carries the failing step's error with the command that finishes it by hand.
func removeFixTask(deps greenDeps) []string {
	tasks, err := deps.Tasks()
	if err != nil {
		return []string{fmt.Sprintf("the board was not read (%v), so the fix task of %s was not looked for; way forward: re-run lym adopt %s", err, deps.Subject, deps.Subject)}
	}
	task, found := fronttask.FrontOf(tasks, deps.Project, fronttask.ActionAdoptCheck)
	if !found {
		return nil
	}
	if err := fronttask.Remove(deps.FrontDeps, task.Slug); err != nil {
		var removal *fronttask.RemoveError
		if errors.As(err, &removal) {
			return []string{fmt.Sprintf("the fix task %s of %s was not removed: the %s step failed (%v); way forward: %s", task.Slug, deps.Subject, removal.Step, removal.Err, removal.HandRemoval())}
		}
		return []string{fmt.Sprintf("the fix task %s of %s was not removed (%v); way forward: run `lym batten cancel %s --remove`, then `lym board remove %s`", task.Slug, deps.Subject, err, task.Slug, task.Slug)}
	}
	return nil
}

// entryDone reports whether task carries the done status.
func entryDone(task boardengine.Task) bool {
	return task.Status != nil && *task.Status == "done"
}

// hubGreenDeps fills greenDeps for the green end of subject over the hub of prime.
// The subject's worktree is the prime for `root` and the beam's pair otherwise, and its project is empty for `root`.
func hubGreenDeps(prime *lymcwd.Location, subject string) (greenDeps, error) {
	frontDeps, err := fronttask.HubDeps(prime)
	if err != nil {
		return greenDeps{}, err
	}
	boardDir := fabricengine.BoardDir(prime.HubPath)
	parentOf := func(slug string) (string, error) { return hubgeom.ParentOf(prime, slug) }
	worktree, project := prime.WorktreeName, ""
	if subject != boardengine.RootSubject {
		worktree, project = subject, subject
	}
	return greenDeps{
		Subject:  subject,
		Dir:      subjectDir(prime.HubPath, subject),
		Worktree: worktree,
		Project:  project,
		PauseRecord: func() (pauserecord.Record, error) {
			geometry, err := hubgeom.PauseGeometry(prime)
			if err != nil {
				return pauserecord.Record{}, err
			}
			return pauserecord.Read(geometry.RecordPath)
		},
		Lift: func(ref tenderengine.EntryRef) error {
			deps, _, err := tendercli.HubDeps(prime, callerLabel)
			if err != nil {
				return err
			}
			_, err = tenderengine.Lift(deps, ref)
			return err
		},
		BeamUp: func(worktree string) error { return orchcli.BeamUpWorktree(prime, worktree) },
		RunRecord: func(slug string) (shedengine.Status, bool, error) {
			return readStatus(shedrun.StatusFile(prime, slug))
		},
		StartFirstPass: func(slug string) (map[string]any, error) {
			return battencli.StartFirstPass(prime, slug, battencli.FirstPassOptions{})
		},
		Tasks:    func() ([]boardengine.Task, error) { return boardengine.ReadTasks(boardDir) },
		ParentOf: parentOf,
		ChildStatus: func(slug string) (shedengine.Status, bool, error) {
			pairPath := fabricengine.WorktreePath(prime, slug)
			if _, err := os.Stat(pairPath); errors.Is(err, fs.ErrNotExist) {
				return shedengine.Status{}, false, nil
			}
			pair, err := lymcwd.ResolveWorktree(pairPath)
			if err != nil {
				return shedengine.Status{}, false, err
			}
			return readStatus(shedrun.StatusFile(pair, shedrun.SelfRunID))
		},
		ResumeChild: func(slug string) error { return resumeChild(fabricengine.WorktreePath(prime, slug)) },
		Owner: func(slug string) (string, error) {
			owners, err := boardengine.ReadOwners(boardDir)
			if err != nil {
				return "", err
			}
			here, err := machinename.Name()
			if err != nil {
				return "", err
			}
			owner, err := owners.Effective(slug, parentOf)
			if err != nil {
				return "", err
			}
			if !boardengine.OwnedElsewhere(owner, here) {
				return "", nil
			}
			return owner, nil
		},
		FrontDeps: frontDeps,
	}, nil
}
