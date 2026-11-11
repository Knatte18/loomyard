// wiring.go implements wire, the wiring the CLI's persistent pre-run performs.
// wire resolves no cwd and spawns no process:
// every path it touches is caller-supplied (location, cwd) or a plain config read anchored at location.AnchorPath().
// So a test can drive it against a hand-built *lymcwd.Location and stay at Tier 1.

package loomcli

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/configreg"
	"github.com/Knatte18/loomyard/internal/discussionparser"
	"github.com/Knatte18/loomyard/internal/driftengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/languageconfig"
	"github.com/Knatte18/loomyard/internal/lenses"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomrecipe"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/orchcli"
	"github.com/Knatte18/loomyard/internal/partsparser"
	"github.com/Knatte18/loomyard/internal/planglyph"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/samplerengine"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedbuild"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrecipe"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/shuttleengines"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/statuscommit"
	"github.com/Knatte18/loomyard/internal/summaryparser"
	"github.com/Knatte18/loomyard/internal/verifytree"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// spawnStatusPush starts the detached push of the status records at a location, held under a gate slug.
// A test replaces it with a recorder.
var spawnStatusPush = fabricengine.SpawnStatusPush

// commitStatusDeps carries the fabric calls that the status commit makes.
// They are injected as plain function values rather than reached directly.
// Injected this way, the seam's branching is drivable in a Tier 1 test from stub closures.
// Such a test needs no hub fixture and no git spawn:
// a commit failure is a hard error, a spawn failure only warns, and a mid-merge state skips the step.
type commitStatusDeps struct {
	// MergeActive reports whether the fabric sibling worktree that carries the status file is mid-merge,
	// and the seam then skips its commit.
	MergeActive func() (bool, error)
	// Commit commits loom's own status file with msg.
	// When the reviews directory holds a file, the same commit also carries the review round record.
	// It reports whether it made a commit.
	Commit func(msg string) (committed bool, err error)
	// Spawn starts the detached push of the run records and, in a task pair, the task branch.
	// The status seam calls it after a commit that landed something.
	Spawn func() error
	// Push pushes the run records and, in a task pair, the task branch, and waits for the result.
	// An error from one side's push names that side.
	// A push lock that stayed busy names no side.
	// Only commit-records calls it.
	Push func() error
	// SetBoardStatus writes status onto the run's board entry, leaving an entry that is absent or already done untouched.
	// Nil writes nothing.
	SetBoardStatus func(status string) error
	// Finish writes the board change a finished run leaves on its entry; nil writes none.
	Finish func() error
}

// statusCommitPathspec returns the fabric-sibling pathspec one status commit stages.
// The status file is always included, and the settle records join when their file exists.
// The reviews directory, the durable directory and the usage directory each join when they hold a file.
// The driver records join per directory (drive reports, arbiter, tackler) when each holds a file.
// The durable directory is staged whole rather than its friction directory alone:
// staging the parent carries the friction directory and every timestamped archive sibling in one entry,
// where a friction-only entry would miss the archives.
// Directories are staged whole, not per file, since the staging step runs `git add -- <pathspec>`:
// an archive stages both the new timestamped sibling and the covered files' removal from the friction directory,
// and a transition whose commit was skipped mid-merge is caught up by the next one.
// Existence alone is not enough.
// The recipe's review entries create every review row's run directory when the recipe is built.
// So from a run's first transition the reviews root exists holding only empty segment directories.
// `git add` accepts an empty directory.
// The commit step then fails with:
// "pathspec did not match any file(s) known to git", which the seam would turn into a hard error.
// A directory with no file is nothing to commit, never an error.
// So an absent, unreadable, non-directory or file-less outcome omits the entry.
func statusCommitPathspec(location *lymcwd.Location, runID string) []string {
	paths := []string{shedrun.StatusRel(location, runID)}
	if shedrun.HoldsFile(loomengine.LoomReviewsDir(location)) {
		paths = append(paths, loomengine.LoomReviewsDirRel())
	}
	if shedrun.HoldsFile(loomengine.LoomDurableDir(location)) {
		paths = append(paths, loomengine.LoomDurableDirRel())
	}
	paths = append(paths, shedrun.DriverRecordsRel(location, runID)...)
	if _, err := os.Stat(shedrun.SettleRecords(location, runID)); err == nil {
		paths = append(paths, shedrun.SettleRecordsRel(location, runID))
	}
	if shedrun.HoldsFile(shedrun.UsageDir(location)) {
		paths = append(paths, shedrun.UsageDirRel())
	}
	return paths
}

// planCommitPathspec returns the pathspec the plan commit stages.
// It holds the plan directory, plus webster's durable directory when it holds a file or git tracks a path under it.
// Webster's directory is tracked, so a record the plan write's rotation moved out of it leaves pending deletions:
// this pathspec stages them together with the archive under the plan, in the same commit.
// Otherwise it is left out, because git refuses a pathspec that matches nothing in the index or the working tree.
// An archived run record alone does not match:
// once its deletions are committed, nothing under webster's directory is left.
// A failed tracked check keeps the path, since dropping it could leave the deletions unstaged.
// The plan amendment record joins when it exists or is tracked, so its removal is staged in the same commit.
func planCommitPathspec(location *lymcwd.Location) []string {
	return planCommitPathspecWith(location, func(rel string) (bool, error) {
		return fabricengine.PathTracked(location.AnchorPath(), rel)
	})
}

// planCommitPathspecWith builds the pathspec the plan commit stages:
// tracked answers whether a relative path is tracked in the git index.
func planCommitPathspecWith(location *lymcwd.Location, tracked func(rel string) (bool, error)) []string {
	paths := []string{planparser.PlanDirRel()}
	if shedrun.HoldsFile(websterengine.Dir(location.AnchorPath())) {
		paths = append(paths, websterengine.DirRel())
	} else if isTracked, err := tracked(websterengine.DirRel()); err != nil || isTracked {
		paths = append(paths, websterengine.DirRel())
	}
	return append(paths, amendmentPathspec(location, tracked)...)
}

// partsPlanCommitPathspec returns the pathspec the parts-plan write commit stages:
// the plan commit's pathspec, followed by the parts directory the part writers write under.
func partsPlanCommitPathspec(location *lymcwd.Location) []string {
	return partsPlanCommitPathspecWith(location, func(rel string) (bool, error) {
		return fabricengine.PathTracked(location.AnchorPath(), rel)
	})
}

// partsPlanCommitPathspecWith builds the pathspec the parts-plan write commit stages:
// tracked answers whether a relative path is tracked in the git index.
func partsPlanCommitPathspecWith(location *lymcwd.Location, tracked func(rel string) (bool, error)) []string {
	return append(planCommitPathspecWith(location, tracked), partsparser.DirRel())
}

// removeSettledAmendment removes the amendment record at amendmentPath when the live settle in planRunDir judged it:
// the settle's recorded round start is after the record's latest write.
// An absent record, or a settle with no recorded round start, removes nothing.
// A read or removal error is returned, so the commit seam fails and the settle is retried.
func removeSettledAmendment(amendmentPath, planRunDir string) error {
	record, found, err := loomshed.ReadAmendment(amendmentPath)
	if err != nil || !found {
		return err
	}
	roundStart, _, err := shedadapters.SettleRoundStart(planRunDir)
	if err != nil {
		return err
	}
	if !loomshed.AmendmentSettledBy(record, roundStart) {
		return nil
	}
	return loomshed.RemoveAmendment(amendmentPath)
}

// loomCommitStatusDeps builds a commitStatusDeps over location and runID, filling each field from fabric.
// MergeActive reports the merge state, and Commit commits the run's status pathspec.
// Spawn starts the detached push of the run records and, in a task pair, the task branch.
// Push runs that same push synchronously.
// Commit, Spawn and Push each pass the run's pre-push check first.
// A Commit the check stops commits nothing and returns its error, which the status seam reads as no commit made.
// A Spawn the check stops starts no push and returns nil, its commits staying local.
// A Push the check stops returns the check's error.
func loomCommitStatusDeps(location *lymcwd.Location, runID string) commitStatusDeps {
	return commitStatusDepsGuardedBy(newPrePushCheck(location, runID), location, runID)
}

// commitStatusDepsGuardedBy returns the status-commit dependencies for the run at location.
// Its commit, spawn and push steps pass through check, the pre-push check.
func commitStatusDepsGuardedBy(check prePushCheck, location *lymcwd.Location, runID string) commitStatusDeps {
	return commitStatusDeps{
		MergeActive: func() (bool, error) {
			return fabricengine.MergeStateActive(location)
		},
		// A Commit over an already-clean tracked path is a no-op that reports committed false, not a failure.
		// The status seam spawns nothing for it.
		Commit: check.guardedCommit(func(msg string) (bool, error) {
			_, committed, err := fabricengine.CommitAnchoredPaths(fabricengine.NewMutations(""), location, statusCommitPathspec(location, runID), msg, fabricengine.EnvSyncOptions())
			return committed, err
		}),
		Spawn: check.guardedSpawn(func() error {
			_, err := spawnStatusPush(location, "")
			return err
		}),
		Push: check.guardedPush(func() error {
			_, err := fabricengine.PushPairAnchored(location, fabricengine.EnvSyncOptions(), fabricengine.StatusPushLockWait)
			return err
		}),
		SetBoardStatus: func(status string) error {
			board, err := boardengine.OpenHub(location.HubPath)
			if err != nil {
				return err
			}
			return board.SetRunStatus(shedrun.ResolveRunID(location, runID), status)
		},
	}
}

// finishesAsSketch reports whether a finished run of recipe makes its board entry a Sketch.
// A recipe with a Finalize row does not, since its landing marks the entry done.
// Neither does one with a Raddle-File row, which ends the entry done itself or turns it into a family's project.
// A recipe with neither lands nothing: its run ends at the Discussion approval with the decision record stored,
// so its entry becomes a Sketch, a note that leaves the run lock, never runs, and waits for the operator's triage.
func finishesAsSketch(recipe string) bool {
	return !recipeHasEngine(recipe, shedrecipe.LoomFinalizeEngine) && !recipeHasEngine(recipe, raddleFileEngine)
}

// commitStatusDeps returns the status-commit dependencies for the run at location.
// It sets Finish only for a recipe whose finished run leaves its board entry a Sketch.
// For a recipe with a RaddleFile row, its board status write stops once the run's filing mark stands.
func (c *loomCLI) commitStatusDeps(location *lymcwd.Location) commitStatusDeps {
	deps := loomCommitStatusDeps(location, c.runID)
	if finishesAsSketch(c.recipe) {
		deps.Finish = sketchFinish(location, c.runID, func(line string) error {
			_, err := orchcli.NotifyParent(location, loomProgram, line)
			return err
		})
	}
	if recipeHasEngine(c.recipe, raddleFileEngine) {
		deps.SetBoardStatus = skipAfterFamilyFiling(func() (bool, error) { return raddleFilingMarked(location, c.runID) }, deps.SetBoardStatus)
	}
	return deps
}

// skipAfterFamilyFiling wraps setBoardStatus so it writes nothing once marked reports the run's filing mark:
// the run's family filing turned its entry into the family's project, which no status of the run may change.
// A failed read of the mark writes nothing and returns the failure, so the seam's next transition retries.
func skipAfterFamilyFiling(marked func() (bool, error), setBoardStatus func(status string) error) func(status string) error {
	return func(status string) error {
		filed, err := marked()
		if err != nil || filed {
			return err
		}
		return setBoardStatus(status)
	}
}

// raddleFilingMarked reports whether the run at location has a filing mark among the slug's run records in the prime.
func raddleFilingMarked(location *lymcwd.Location, runID string) (bool, error) {
	prime, err := primeOfHub(location)
	if err != nil {
		return false, err
	}
	return shedrun.FilingMarked(prime, shedrun.ResolveRunID(location, runID))
}

// sketchFinish returns the Finish that makes the board entry of the run at location a Sketch.
// The board refuses the Sketch while another entry depends on the entry.
// In that case it warns naming the refusal and leaves the entry draft instead,
// so the run still lets go of the entry and nothing starts it.
// It then tells the run's parent through notify, since only the parent can settle such an entry;
// a failed notice only warns.
// Any other error is returned, so the seam's next finished transition retries the write.
func sketchFinish(location *lymcwd.Location, runID string, notify func(line string) error) func() error {
	return func() error {
		board, err := boardengine.OpenHub(location.HubPath)
		if err != nil {
			return err
		}
		slug := shedrun.ResolveRunID(location, runID)
		refusal := board.MakeSketch(slug)
		if !errors.Is(refusal, boardengine.ErrSketchRefused) {
			return refusal
		}
		logger.Warn("loomcli: the board refused to make the finished run's entry a Sketch; leaving it draft", "slug", slug, "error", refusal)
		if err := board.SetRunStatus(slug, boardengine.StatusDraft); err != nil {
			return err
		}
		if err := notify(sketchRefusedNotice(slug, refusal)); err != nil {
			logger.Warn("loomcli: the notice of the refused Sketch was not posted", "slug", slug, "error", err)
		}
		return nil
	}
}

// sketchRefusedNotice renders the line that tells the parent the finished run's entry slug stays a draft task.
// The line names the board's refusal, which names the dependent entries, and it gives the ways forward.
func sketchRefusedNotice(slug string, refusal error) string {
	return fmt.Sprintf("loom %s: the finished run's entry stays a draft task with its decision record stored: %v; way forward: drop or move those dependencies with `lym board set-deps '{\"slug\":\"<dependent>\",\"depends_on\":[...]}'` for each entry the refusal names, keeping its other dependencies, and make the entry a Sketch with `lym board upsert '{\"slug\":\"%s\",\"kind\":\"note\",\"labels\":[\"sketch\"],\"status\":null,\"depends_on\":[],\"isolated\":false,\"recipe\":\"\",\"profile\":\"\",\"run_settings\":null,\"pull_request\":\"\",\"notify_on_land\":false}'`, listing after `sketch` in `labels` the entry's labels that are not type labels, or give the task recipe raddle with `lym board upsert '{\"slug\":\"%s\",\"recipe\":\"raddle\"}'` and release it with `lym board set-status '{\"slug\":\"%s\",\"status\":null}'`", slug, refusal, slug, slug, slug)
}

// newCommitStatusSeam builds the shedengine.Shed.CommitStatus closure from deps.
// It first writes the run's board status, `<state> · <producer>`, when that status changed.
// On the run's finish it calls Finish until it succeeds once;
// a failed board write only warns, and the next transition retries it;
// the board is the hub's overview, never the run's own bookkeeping.
// It then calls the shared statuscommit core, which owns the dispositions:
// skip-while-mid-merge, commit-hard-errors and spawn-warns.
// This seam passes loom's own commit and log prefixes and no after-commit callback.
// A commit the pre-push check stopped reads as no commit made, so the core starts no push and the transition goes on.
func newCommitStatusSeam(deps commitStatusDeps) func(producer, state string) error {
	core := statuscommit.New(statuscommit.Deps{
		MergeActive: deps.MergeActive,
		Commit: func(msg string) (bool, error) {
			committed, err := deps.Commit(msg)
			if errors.Is(err, errPushStopped) {
				logger.Info("loomcli: the run's commit was stopped; the records stay as they are", "error", err)
				return false, nil
			}
			return committed, err
		},
		Spawn: deps.Spawn,
	}, "loom", "loomcli", nil)
	lastBoardStatus, finished := "", false
	return func(producer, state string) error {
		switch {
		case state == string(shedengine.StateDone):
			if deps.Finish != nil && !finished {
				if err := deps.Finish(); err != nil {
					logger.Warn("loomcli: the finished run's board write failed, next transition retries", "error", err)
				} else {
					finished = true
				}
			}
		case deps.SetBoardStatus != nil:
			if status := boardengine.RunStatus(state, producer); status != lastBoardStatus {
				if err := deps.SetBoardStatus(status); err != nil {
					logger.Warn("loomcli: board status write failed, next transition retries", "status", status, "error", err)
				} else {
					lastBoardStatus = status
				}
			}
		}
		return core(producer, state)
	}
}

// wireLightweight fills c with what the lightweight verbs read: the location, the cwd and the two status-file paths.
// The validate verbs also get the c.env path fields they read.
// The committed-file seam that validate-plan --rework reads is filled for them too.
// It also fills the history seam and the record row's engine that validate-discussion uses;
// the seam reads committed history through the fabric at call time.
// It loads no module config, constructs no engine, and can fail only if the run's recipe selects no embedded recipe.
// validate-plan loads the hub-wide language.yaml itself for its rename-tool-missing check;
// the full wiring does not load that file.
//
// It exists because the full wiring loads every module config, a model-spec registry and the active batchifier:
// a fault in any of them, even one a spawned agent causes by rewriting a config, refuses status and pause outright.
// pause is the documented graceful-stop mechanism;
// losing it to an unrelated config problem inverts the cost.
//
// The validate verbs stay on this path because their writer agents run them as a pre-handoff self-check mid-run.
// A sibling process may be rewriting a config the full wiring loads at that moment.
// validate-discussion reads only `DecisionRecordPath`, `SupportLogPath`, `AnchorPath` and `WorktreeRoot`.
// On a tapestry run it also reads `PlanIndex`.
// validate-plan reads language.yaml alone, and validate-description reads `DescriptionPath` alone.
//
// approve is on this path because it builds no producer:
// it reads the status file, opens the fabric on demand, queries GitHub, then writes the approval record.
// None of that needs a module config.
//
// start and run build producers, so they keep the full wiring and fail early on a bad config:
// for them an unloadable config is a real refusal, not an unrelated fault.
func (c *loomCLI) wireLightweight(location *lymcwd.Location, cwd string) error {
	c.location = location
	c.cwd = cwd
	// CommitStatus is filled here too, though no verb on this path writes the status file and so none invokes it.
	// Filling both literals keeps them structurally identical:
	// a verb moved from this path to the full wiring cannot silently lose the hook.
	// Building these closures does no I/O, so this fill loads no config and opens no fabric.
	c.shedPaths = shedbuild.ShedPaths{
		StatusPath:     shedrun.StatusFile(location, c.runID),
		LockPath:       shedrun.RunLock(location, c.runID),
		StatusLockPath: shedrun.StatusLock(location, c.runID),
		CommitStatus:   newCommitStatusSeam(c.commitStatusDeps(location)),

		RunID:                   c.runID,
		MissingStatusWayForward: loomMissingStatusWayForward,
		RerouteCapWayForward:    loomRerouteCapWayForward,
		BudgetWayForward:        loomBudgetWayForward,
	}
	// The rest of c.env stays at its zero value: status, pause and approve read nothing from it.
	// Filling only the fields the validate verbs read keeps this path from re-acquiring the eight-config load.
	// This path exists to avoid that load.
	c.env.AnchorPath = location.AnchorPath()
	c.env.WorktreeRoot = location.WorktreePath()
	record := runRecordOf(location, c.recipe)
	c.env.DecisionRecordPath = record.path
	// A task run takes its record's PATTERN declarations; a beam's project record declares none of its own.
	if !record.project {
		c.env.PatternRecordPath = record.path
	}
	c.env.SupportLogPath = record.supportLog
	c.env.RelevantPath = loomengine.DiscussionRelevant(location)
	c.env.DescriptionPath = summaryparser.Path(loomengine.LandingDir(location))
	c.env.Rework.ReadCommitted = committedAnchoredReader(location)
	c.env.DiscussionHistory = recordHistory(location, record)
	recordEngine, err := loomrecipe.RecordEngine(c.recipe)
	if err != nil {
		return err
	}
	c.env.RecordEngine = recordEngine
	c.env.PlanIndex = planglyph.NewSlottedIndex(fabricengine.NewReferenceRule(), hubgeom.GateSlots(location), gateslot.WaitDir(location.AnchorPath()))
	return nil
}

// committedAnchoredReader returns the seam that reads an anchor-relative file as committed at HEAD for location.
// The seam reports found false when HEAD has no such file.
// Building it opens nothing: the fabric is read on each call.
func committedAnchoredReader(location *lymcwd.Location) func(anchorRel string) ([]byte, bool, error) {
	return func(anchorRel string) ([]byte, bool, error) {
		return fabricengine.CommittedAnchoredFile(location, anchorRel)
	}
}

// discussionCommitPathspec is the pathspec a discussion commit stages:
// the whole discussion directory and the plan amendment record.
// A parent-review round directory that an older binary left stays committed as it is,
// and this pathspec does not stage it.
func discussionCommitPathspec(location *lymcwd.Location) []string {
	paths := []string{loomengine.DiscussionDirRel()}
	return append(paths, amendmentPathspec(location, func(rel string) (bool, error) {
		return fabricengine.PathTracked(location.AnchorPath(), rel)
	})...)
}

// amendmentPathspec is the plan amendment record's pathspec:
// the record when it exists or git tracks it, and nothing otherwise.
// A tracked record that is absent is a deletion to stage.
// A failed tracked check adds nothing, so git is never handed a pathspec matching nothing.
func amendmentPathspec(location *lymcwd.Location, tracked func(rel string) (bool, error)) []string {
	rel := loomengine.LoomAmendmentRecordRel()
	if _, err := os.Stat(loomengine.LoomAmendmentRecord(location)); err == nil {
		return []string{rel}
	}
	if isTracked, err := tracked(rel); err == nil && isTracked {
		return []string{rel}
	}
	return nil
}

// commitDiscussion commits the discussion artifacts of location's worktree through the fabric.
// It is shared by every path that commits discussion artifacts or an appended decision record.
func commitDiscussion(location *lymcwd.Location) error {
	_, _, err := fabricengine.CommitAnchoredPaths(fabricengine.NewMutations(""), location, discussionCommitPathspec(location), fmt.Sprintf("loom: discussion artifacts for %s", seedSlug(location.WorktreeName)), fabricengine.EnvSyncOptions())
	return err
}

// commitDescription commits the landing directory of location's worktree through the fabric.
// It uses the change-description subject, and a repeat commit over an unchanged directory is a no-op.
// Every loom recipe makes its description commit through it.
func commitDescription(location *lymcwd.Location) error {
	_, _, err := fabricengine.CommitAnchoredPaths(fabricengine.NewMutations(""), location, []string{loomengine.LandingDirRel()}, fmt.Sprintf("loom: change description for %s", seedSlug(location.WorktreeName)), fabricengine.EnvSyncOptions())
	return err
}

// recordWriteDeps returns the Record-Write row's deps for the worktree at l.
// They cover the record's paths, the run's board entry body and stored decision record, and the record's commit.
// Building it opens nothing: the board is read on each call.
func recordWriteDeps(l *lymcwd.Location, record runRecord) loomshed.RecordWriteDeps {
	return loomshed.RecordWriteDeps{
		RecordPath:     record.path,
		SupportLogPath: record.supportLog,
		DiscussionDir:  record.dir,
		ReadBody: func() (string, bool, error) {
			board, err := boardengine.OpenHub(l.HubPath)
			if err != nil {
				return "", false, fmt.Errorf("open the board: %w", err)
			}
			task, found, err := board.GetTask(seedSlug(l.WorktreeName))
			if err != nil {
				return "", false, fmt.Errorf("read the run's board entry: %w", err)
			}
			return task.Body, found, nil
		},
		ReadStored: func() ([]byte, bool, error) {
			board, err := boardengine.OpenHub(l.HubPath)
			if err != nil {
				return nil, false, fmt.Errorf("open the board: %w", err)
			}
			stored, found, err := board.DecisionRecord(seedSlug(l.WorktreeName))
			if err != nil {
				return nil, false, fmt.Errorf("read the run's stored decision record: %w", err)
			}
			return stored, found, nil
		},
		Commit: func() error { return commitRunRecord(l, record) },
	}
}

// writeCarryOver writes entry into the decision record of location's worktree and commits the record alone.
// An unchanged record commits nothing, and a write or commit error is returned to the caller.
func writeCarryOver(location *lymcwd.Location, entry discussionparser.CarryOver) error {
	return writeCarryOverLocked(loomengine.DiscussionDecisionRecord(location), entry, func() error {
		_, _, err := fabricengine.CommitAnchoredPaths(fabricengine.NewMutations(""), location, []string{loomengine.DiscussionDecisionRecordRel()}, fmt.Sprintf("loom: review carry-over for %s", seedSlug(location.WorktreeName)), fabricengine.EnvSyncOptions())
		return err
	})
}

// writeCarryOverLocked writes entry into the decision record at recordPath and then runs commit.
// It holds the record's decision lock across both.
// A decision call in flight finishes its write and commit first, so neither write is lost.
func writeCarryOverLocked(recordPath string, entry discussionparser.CarryOver, commit func() error) error {
	release, err := holdDecisionLock(recordPath)
	if err != nil {
		return err
	}
	defer func() { _ = release() }()
	if err := discussionparser.WriteCarryOver(recordPath, entry); err != nil {
		return err
	}
	return commit()
}

// loomProgram is the program name loom signs its notices to the orch with.
const loomProgram = "LOOM"

// gateTierTags returns the sorted tier keys of cfg.
// They are the build tags the plan gates' `test-tier-unstated` check accepts.
func gateTierTags(cfg gateslot.Config) []string {
	return slices.Sorted(maps.Keys(cfg.Tiers))
}

// wire builds the whole engine stack onto c from location and cwd.
// It loads every module config anchored at location.AnchorPath(), then builds the reed engine and shuttle runner.
// It also assembles websterengine.RunDeps and the shedrecipe.Env/shedbuild.ShedPaths pair that wraps it.
func (c *loomCLI) wire(location *lymcwd.Location, cwd string) error {
	anchorPath := location.AnchorPath()

	loomCfg, err := loomengine.LoadConfig(fabricengine.ConfigDirs(location), "loom")
	if err != nil {
		return err
	}
	loomCfg, err = c.withTapestry(fabricengine.ConfigDirs(location), loomCfg)
	if err != nil {
		return err
	}
	reedCfg, err := reedengine.LoadConfig(fabricengine.ConfigDirs(location), "reed")
	if err != nil {
		return err
	}
	shuttleCfg, err := shuttleengine.LoadConfig(fabricengine.ConfigDirs(location), "shuttle")
	if err != nil {
		return err
	}
	websterCfg, err := websterengine.LoadConfig(fabricengine.ConfigDirs(location), "webster")
	if err != nil {
		return err
	}
	// landing is hub-wide: Publish and Finalize read the hub's file, never a pair's copy.
	// A pair's local override applies only to the keys that meet no category.
	landingCfg, err := landingshed.LoadConfig(fabricengine.ConfigDirs(location), "landing")
	if err != nil {
		return fmt.Errorf("%w; %s", err, hubConfigWayForward)
	}
	// The error already names the failing key and its own way forward.
	language, err := languageconfig.Load(fabricengine.ConfigDirs(location))
	if err != nil {
		return err
	}
	languageSkills := language.Skills
	// An absent gate.yaml renders the gate_tiers marker as none and refuses no raw command.
	// Any other load error already names the way forward.
	gateTiers := "none"
	var refuseRaw, tierTags []string
	gateCfg, err := gateslot.LoadConfig(fabricengine.ConfigDirs(location))
	if err == nil {
		gateTiers = gateCfg.TiersValue()
		refuseRaw = gateCfg.RefuseRaw
		tierTags = gateTierTags(gateCfg)
	} else if !errors.Is(err, gateslot.ErrConfigAbsent) {
		return err
	}
	// lenses.Load degrades to the template on an absent lenses.yaml rather than failing the load.
	lensesCfg, err := lenses.Load(fabricengine.ConfigDirs(location))
	if err != nil {
		return err
	}
	registry, err := modelspec.LoadRegistry(fabricengine.ConfigDirs(location))
	if err != nil {
		return err
	}
	roles, err := websterengine.ResolveRoles(websterCfg, registry)
	if err != nil {
		return err
	}
	activeBatcher, err := batcher.Active(fabricengine.ConfigDirs(location))
	if err != nil {
		return err
	}
	reviewSettings, err := loomengine.ResolveReview(loomCfg, registry)
	if err != nil {
		return err
	}
	judgeSettings, err := loomengine.ResolveJudge(loomCfg, registry)
	if err != nil {
		return err
	}
	reedGeom, err := hubgeom.ReedGeometry(location)
	if err != nil {
		return err
	}
	c.parentName = reedGeom.ParentName
	reedEngine := reedengine.New(reedCfg, reedGeom, shuttleengines.EnvScrub())
	engines, err := shuttleengines.New(shuttleCfg, fabricengine.ConfigDirs(location))
	if err != nil {
		return err
	}
	runner := shuttleengine.NewRunner(reedEngine, engines, reedGeom.AnchorPath, reedGeom.WorktreeRoot, shuttleCfg)
	pauseGeom, err := hubgeom.PauseGeometry(location)
	if err != nil {
		return err
	}
	runner.SetPause(pauseGeom)
	runner.SetNotifier(func(line string) error {
		_, err := orchcli.NotifyAsker(location, loomProgram, line)
		return err
	})
	runner.SetSessionEnd(shedadapters.UsageRecorder(location, shuttleengines.RecordReaders()))

	websterGeom := hubgeom.WebsterGeometry(location)
	websterGeom.Index = planglyph.NewSlottedIndex(fabricengine.NewReferenceRule(), websterGeom.GateSlots, websterGeom.GateWaitDir)
	websterGeom.VerifyHold = verifytree.NewPauseHold(pauseGeom, nil)

	// frictionDir is the single resolved value every told-friction consumer below reads.
	// It is non-empty only when loom.yaml sets the friction key: Tier 2 off is an empty path string, never a boolean.
	// Resolving it once here keeps every consumer from re-checking loomCfg.Friction and disagreeing about Tier 2.
	frictionDir := ""
	if loomCfg.Friction != "" {
		frictionDir = loomengine.LoomFrictionDir(location)
	}

	// BurlerGeometry builds the geometry burlerengine.Geometry declares, with burler's AnchorPath semantics.
	// Its field set differs from the webster geometry's, so the two builders are not interchangeable.
	burlerGeom, err := hubgeom.BurlerGeometry(location)
	if err != nil {
		return err
	}
	// A task run's review rows get the record, so the Code-Review perch checks that its declared PATTERN changes landed.
	// A beam's project record declares none of its own, so its review rows get no record.
	record := runRecordOf(location, c.recipe)
	patternRecordPath := ""
	if !record.project {
		patternRecordPath = record.path
	}
	burlerGeom.PatternRecordPath = patternRecordPath
	burlerGeom.DefaultEngine = shuttleCfg.Engine
	assistBounds := segmentAssistBounds(location, burlerGeom, loomCfg, time.Duration(shuttleCfg.RunTimeoutMin)*time.Minute, shuttleCfg.StartAllowance())
	burlerEngine := burlerengine.New(seatengine.RunnerShuttle(runner), burlerGeom, lensesCfg, websterGeom.StencilsDir, frictionDir, languageSkills)

	runDeps := websterengine.RunDeps{
		Starter:    runnerMasterStarter{runner: runner},
		Stopper:    runner,
		Engines:    engines,
		ShuttleCfg: shuttleCfg,
		Roles:      roles,
		Config:     websterCfg,
		Batcher:    activeBatcher,
		Geom:       websterGeom,
		// FrictionDir covers the Master and verify-fix prompts only.
		// The per-batch fork and recovery prompts are composed in a separate `lym webster` process:
		// its begin-batch and recover-batch commands resolve the same value themselves, in internal/webstercli.
		FrictionDir: frictionDir,
		// The reference matcher is a real fabricengine.NewRefScanner(location), built eagerly.
		// That constructor only compiles a regexp and cannot fail.
		// It must never be the never-matching stand-in.
		// That stand-in is permitted only in standalone, where no wired fabric exists for the guard to protect.
		// Loom is hub-only.
		RefMatcher: fabricengine.NewRefScanner(location),
		// ReadOnly makes a read-only fabric reference a policy warning at the run-exit audit.
		ReadOnly: fabricengine.IsReadOnlyCommand,
		// ParentBranch lets the verify gate's fix-commit check accept a clean parent merge made while fixing.
		ParentBranch: func() (string, error) {
			return readRecordedParentBranch(location)
		},
		LanguageSkills: languageSkills,
		// InstructionFiles looks Master's engine up lazily.
		// The lazy lookup keeps loom's status and pause, which only wire, from being refused by the master key.
		InstructionFiles: websterengine.MasterInstructionFiles(engines, roles[websterengine.RoleMaster]),
	}

	statusPath := shedrun.StatusFile(location, c.runID)
	statusLockPath := shedrun.StatusLock(location, c.runID)

	// reworkDeps opens nothing at wire time.
	// Every closure reads or writes on demand, since wire() also runs for status/pause.
	// The Plan-Review skip seam and Plan-Write's rotation share it.
	reworkDeps := loomshed.PRReworkDeps{
		PlanDir:       planparser.PlanDir(anchorPath),
		ReworkDir:     loomengine.LoomReworkDir(location),
		ReworkDirRel:  loomengine.LoomReworkDirRel(),
		ReviewsDir:    loomengine.LoomReviewsDir(location),
		AmendmentPath: loomengine.LoomAmendmentRecord(location),
		ReadCommitted: committedAnchoredReader(location),
		ArchiveWebster: func(dest string) error {
			return websterengine.ArchiveRunRecord(websterGeom, dest)
		},
	}

	// A pair run records its parent branch in the origin record,
	// so the record file's presence tells the headroom lint its merge base.
	// The check only stats, because reading the record would create the records-side lock directory and seed its exclude.
	var verifyMergeBase func() (string, error)
	if _, err := os.Stat(fabricengine.OriginRecordPath(location)); err == nil {
		verifyMergeBase = taskMergeBase(location)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("loomcli: stat the origin record: %w", err)
	}

	// Preflight loads batcher.yaml for a recipe with an Implementer row, so a broken file fails the first row.
	// A darn or pawl run leaves it nil and skips the load.
	var preflightBatcher func() error
	holdsImplementer, err := loomrecipe.HasEngine(c.recipe, "Implementer")
	if err != nil {
		return err
	}
	if holdsImplementer {
		configDirs := fabricengine.ConfigDirs(location)
		preflightBatcher = func() error {
			_, err := batcher.Active(configDirs)
			return err
		}
	}

	// Preflight checks drift.yaml for a recipe with Drift rows, so a command a Drift row would refuse fails the first row.
	var preflightDrift func() error
	holdsDrift, err := loomrecipe.HasEngine(c.recipe, driftBaselineEngine)
	if err != nil {
		return err
	}
	if holdsDrift {
		configDirs := fabricengine.ConfigDirs(location)
		preflightDrift = func() error { return driftengine.CheckRetiredPlaceholders(configDirs) }
	}

	recordEngine, err := loomrecipe.RecordEngine(c.recipe)
	if err != nil {
		return err
	}

	// A pair run's row gates may hand a failure to a cold fixer.
	// The prime and a run outside a hub have no pair to own one.
	pairSlug, err := pairSlugOf(location)
	if err != nil {
		logger.Warn("loomcli: could not read the fabric origin record; the row gate fixers are off for this wiring", "cause", err)
		pairSlug = ""
	}
	planDir := planparser.PlanDir(anchorPath)
	// Setting is one lookup per wiring, so every row of a recipe build reads one snapshot of each module's stack.
	settingLookup := configreg.SettingLookup(fabricengine.ConfigDirs(location))

	c.env = shedrecipe.Env{
		PreflightBatcher: preflightBatcher,
		PreflightDrift:   preflightDrift,
		Cwd:              cwd,
		AnchorPath:       anchorPath,
		ConfigDirs:       fabricengine.ConfigDirs(location),
		WorktreeRoot:     location.WorktreePath(),
		// RunScratchDir is the directory the step envelope reports as scratch_dir.
		RunScratchDir: shedrun.ScratchDir(location, c.runID),
		GateFindingsFile: func(row string) string {
			return shedrun.GateFindingsFile(location, c.runID, row)
		},
		VerifyDir:     verifytree.Dir(anchorPath),
		VerifyCommand: gateCallSource(fabricengine.ConfigDirs(location)),
		// VerifyMergeBase and ReviewBase are nil for a standalone run, which records no parent branch.
		VerifyMergeBase: verifyMergeBase,
		ReviewBase:      verifyMergeBase,
		// RedundancyReport reads the gate's report at each fill of a rubric carrying the marker, never at wire time.
		RedundancyReport: redundancyReportNote(location),
		VerifyHold:       verifytree.NewPauseHold(pauseGeom, nil),
		// GateFixer wraps the Darn-Write, Plan-Write and Discussion-Write gates of a pair run with a cold fixer.
		// It reads every value here at wire time and opens nothing.
		GateFixer: gateFixerSeam(gateFixerInputs{
			pairSlug:           pairSlug,
			runProcDir:         fabricengine.RunProcDir(location.HubPath),
			starter:            runner,
			stopper:            runner,
			reed:               reedEngine,
			engines:            engines,
			shuttleCfg:         shuttleCfg,
			anchorRoot:         reedGeom.AnchorPath,
			registry:           registry,
			gate:               gateCfg,
			stencilsDir:        websterGeom.StencilsDir,
			repoRoot:           location.WorktreePath(),
			languageSkills:     languageSkills,
			parentName:         c.parentName,
			frictionDir:        frictionDir,
			scratchDir:         shedrun.ScratchDir(location, c.runID),
			statusPath:         statusPath,
			statusLockPath:     statusLockPath,
			patternRecordPath:  patternRecordPath,
			worktreeRoot:       location.WorktreePath(),
			decisionRecordPath: record.path,
			supportLogPath:     record.supportLog,
			relevantPath:       loomengine.DiscussionRelevant(location),
			planDir:            planDir,
			lensReviewPath:     filepath.Join(planDir, planparser.LensReviewFileName),
		}),
		WebsterRecord: func() string {
			return loomshed.WebsterRecordNote(anchorPath)
		},
		GateTiers:          gateTiers,
		RenameTools:        language.RenameTools,
		RefuseRaw:          refuseRaw,
		TierTags:           tierTags,
		StatusPath:         statusPath,
		StatusLockPath:     statusLockPath,
		DecisionRecordPath: record.path,
		PatternRecordPath:  patternRecordPath,
		SupportLogPath:     record.supportLog,
		RelevantPath:       loomengine.DiscussionRelevant(location),
		ParentName:         c.parentName,
		// NotifyParent names the run first, since the orch reads the notices of every run of the hub.
		NotifyParent: func(line string) error {
			_, err := orchcli.NotifyParent(location, loomProgram, fmt.Sprintf("loom %s: %s", seedSlug(location.WorktreeName), line))
			return err
		},
		WebsterDeps: runDeps,
		PlanIndex:   websterGeom.Index,
		// ReflectFriction is a method value over the receiver, so frictionDir is read when the row runs, not when wire runs.
		ReflectFriction: c.reflectFrictionRow,
		// WebsterRun is filled explicitly, since websterEntry fails on a nil WebsterRun rather than defaulting it.
		WebsterRun: websterengine.Run,
		// CommitWebster commits webster's whole durable directory, so nothing in it stays untracked.
		// Otherwise Master's outcome.yaml, summary.md and the verify gate's report would refuse the task worktree's removal.
		// The plan directory is committed too, because webster rewrites card files during the run.
		// It rewrites them for handle binding and handle canonicalization.
		CommitWebster: func() error {
			_, _, err := fabricengine.CommitAnchoredPaths(fabricengine.NewMutations(""), location, []string{websterengine.DirRel(), planparser.PlanDirRel()}, fmt.Sprintf("loom: webster run record for %s", seedSlug(location.WorktreeName)), fabricengine.EnvSyncOptions())
			return err
		},
		// Shuttle is the runner, which satisfies shedadapters.Shuttle.
		// The Bouncer and the Burler probes read it; no writer row does.
		Shuttle: runner,
		// Seats runs the MultiLLM rows' seats over the same runner.
		Seats: seatengine.New(seatengine.RunnerShuttle(runner), seatengine.Geometry{
			WorktreeRoot:  location.WorktreePath(),
			AnchorPath:    anchorPath,
			StencilsDir:   websterGeom.StencilsDir,
			DefaultEngine: shuttleCfg.Engine,
			ParentName:    c.parentName,
			Shortname:     reedGeom.NameShortname,
			Slug:          reedGeom.NameSlug,
		}),
		Models: registry,
		// DiscussionTable is evaluated per Call, so the stencils are read at call time per the Stencil Ownership Invariant.
		// The lead's interactivity is loomCfg.DiscussionInteractive, read fresh on every wire() call.
		// Nothing compares it with the mode a live run was started with:
		// the resume decision rests on live-agent evidence alone: flipping the key between a crash and a resume is harmless,
		// and it only changes how the next spawn is interviewed.
		DiscussionTable: func() (seatengine.Table, error) {
			fixerOn, err := writerFixerOn(c.recipe, discussionWriteEngine, discussionGateEntry, c.env.Setting, pairSlug)
			if err != nil {
				return seatengine.Table{}, err
			}
			table, err := loomengine.DiscussionTable(location, fabricengine.ConfigDirs(location), websterGeom.StencilsDir, websterGeom.SpecsDir, loomCfg, registry, seedSlug(location.WorktreeName), fixerOn)
			if err != nil {
				return seatengine.Table{}, err
			}
			return withProjectDirective(table, location, websterGeom.StencilsDir)
		},
		// CommitDiscussion commits the whole discussion directory, with NewMutations("") and EnvSyncOptions().
		// The whole directory is committed so that archiveStaleOutputs' timestamped siblings are not left as untracked dirt.
		// A second Done over already-committed artifacts is a no-op, not an error:
		// CommitAnchoredPaths reports committed == false for an already-clean, already-tracked path,
		// and this closure drops that result along with the sha, returning only the error.
		// The Discussion-Bouncer row's approved settle reaches this closure too, through the commit_seam: discussion config key.
		CommitDiscussion: func() error { return commitDiscussion(location) },
		// DiscussionHistory and RecordEngine are filled for every recipe:
		// the "discussion" gate and Darn-Write's guard check the record against its history,
		// and the record row's engine selects the citation checks.
		DiscussionHistory: recordHistory(location, record),
		RecordEngine:      recordEngine,
		// RecordWrite is filled for every recipe: only a recipe with a RecordWrite row reads it,
		// and it opens the board on a call, never at wire time.
		RecordWrite: recordWriteDeps(location, record),
		// RaddleWrite and RaddleFile are filled for every recipe: only a recipe with a raddle row reads them,
		// and every closure reads the board, the status or the setting on a call, never at wire time.
		RaddleWrite: raddleWriteEnv(location, loomCfg, websterGeom.StencilsDir, websterGeom.SpecsDir, registry, seedSlug(location.WorktreeName), c.recipe, settingLookup),
		RaddleFile:  raddleFileDeps(location, seedSlug(location.WorktreeName)),
		// DescriptionPath is the change description Describe writes and its gate and the landing
		// rows read.
		DescriptionPath: summaryparser.Path(loomengine.LandingDir(location)),
		// DescribeTable is evaluated per Call, so the stencil is read at call time.
		// It opens the fabric lazily.
		// wire() also runs for status and pause, where opening the fabric must not happen.
		// Opening stat-checks the paired sibling.
		// An eager open would fail status and pause against a healthy but unwired location.
		DescribeTable: func() (seatengine.Table, error) {
			handle, err := fabricengine.Open(location)
			if err != nil {
				return seatengine.Table{}, err
			}
			taskBranch, parentBranch, err := landingBranches(handle, location)
			if err != nil {
				return seatengine.Table{}, err
			}
			mergeBase, err := taskMergeBase(location)()
			if err != nil {
				return seatengine.Table{}, err
			}
			priorDirs, err := loomshed.ArchivedWebsterDirs(loomengine.LoomReworkDir(location))
			if err != nil {
				return seatengine.Table{}, err
			}
			priorRecords := make([]string, len(priorDirs))
			for i, dir := range priorDirs {
				priorRecords[i] = summaryparser.Path(dir)
			}
			verdict, err := driftengine.Verdict(driftRecordsDeps(location))
			if err != nil {
				return seatengine.Table{}, err
			}
			return landingshed.DescribeTable(landingshed.DescribeInputs{
				DriftVerdict:        verdict,
				StencilsDir:         websterGeom.StencilsDir,
				DecisionRecordPath:  loomengine.DiscussionDecisionRecord(location),
				RunRecordPath:       summaryparser.Path(websterGeom.WebsterDir),
				PriorRunRecordPaths: priorRecords,
				DescriptionPath:     summaryparser.Path(loomengine.LandingDir(location)),
				TaskBranch:          taskBranch,
				ParentBranch:        parentBranch,
				MergeBase:           mergeBase,
				Slug:                seedSlug(location.WorktreeName),
			}, landingCfg, registry)
		},
		CommitDescription: func() error { return commitDescription(location) },
		// PlanTable is evaluated per Call, so the stencil is read at call time, as the Stencil Ownership Invariant requires.
		// It takes no autonomous argument: the Plan producer is autonomous by design and its table is never interactive.
		PlanTable: func() (seatengine.Table, error) {
			fixerOn, err := writerFixerOn(c.recipe, planWriteEngine, planGateEntry, c.env.Setting, pairSlug)
			if err != nil {
				return seatengine.Table{}, err
			}
			return loomengine.PlanTable(location, fabricengine.ConfigDirs(location), websterGeom.StencilsDir, websterGeom.SpecsDir, loomCfg, registry, gateTiers, fixerOn)
		},
		// CommitPlan keeps the working tree clean for the rows that follow.
		// It makes the artifact durable across a crash or a resume.
		// It also sweeps the decorator's archive subdirectory into git rather than leaving it as untracked dirt.
		// A second Done over already-committed artifacts is a no-op, not an error:
		// CommitAnchoredPaths reports committed == false for an already-clean, already-tracked path,
		// and this closure drops that result along with the sha.
		// The Plan-Bouncer row's approved settle reaches this closure too, through the commit_seam: plan config key.
		// The commit message names the artifact set rather than the producer that last touched it.
		// Both callers reach this closure only after a gate has judged the plan.
		// Plan-Write's gate runs inside its Call, ahead of its post-Done commit.
		// Plan-Burler-Fix's gate runs ahead of the Plan-Bouncer approved settle that commits its overlay fix.
		// The commit keeps the artifact durable; it does not certify it.
		// The pathspec is the whole plan directory from planparser.PlanDirRel().
		// Never use a hand-built filepath.Join naming the _lym literal.
		// The Lymdirs Single-Declarer Invariant forbids that in production path construction.
		// planCommitPathspec adds webster's durable directory when Plan-Write's rotation moved a run record out of it.
		// An amendment record the settle judged is removed first, so the deletion is staged by the same commit.
		CommitPlan: func() error {
			subdir, _, err := loomrecipe.BouncerRunSubdir(loomshed.NamePlanBouncer)
			if err != nil {
				return err
			}
			if err := removeSettledAmendment(loomengine.LoomAmendmentRecord(location), filepath.Join(loomengine.LoomReviewsDir(location), subdir)); err != nil {
				return err
			}
			_, _, err = fabricengine.CommitAnchoredPaths(fabricengine.NewMutations(""), location, planCommitPathspec(location), fmt.Sprintf("loom: plan artifacts for %s", seedSlug(location.WorktreeName)), fabricengine.EnvSyncOptions())
			return err
		},
		// PartsTable and PartsPlanTable are evaluated per Call, so the stencils and the parts file are read at call time.
		// PartsPlanTable receives shuttle's run_timeout_min as a duration; a plan_timeout_min of 0 resolves to that value.
		// Only the producer entry can build the per-part hooks.
		PartsTable: func() (seatengine.Table, error) {
			return loomengine.PartsTable(location, websterGeom.StencilsDir, websterGeom.SpecsDir, loomCfg, registry)
		},
		PartsPlanTable: func(seats shedrecipe.PartSeats) (seatengine.Table, error) {
			return loomengine.PartsPlanTable(location, websterGeom.StencilsDir, websterGeom.SpecsDir, loomCfg, registry, gateTiers, time.Duration(shuttleCfg.RunTimeoutMin)*time.Minute, seats.Gates, seats.Accept, seats.FindingsPath)
		},
		// CommitParts and CommitPartsPlan keep the working tree clean and the artifacts durable.
		// CommitParts commits the whole parts directory, the path partsparser.DirRel() returns.
		CommitParts: func() error {
			_, _, err := fabricengine.CommitAnchoredPaths(fabricengine.NewMutations(""), location, []string{partsparser.DirRel()}, fmt.Sprintf("loom: parts artifacts for %s", seedSlug(location.WorktreeName)), fabricengine.EnvSyncOptions())
			return err
		},
		CommitPartsPlan: func() error {
			_, _, err := fabricengine.CommitAnchoredPaths(fabricengine.NewMutations(""), location, partsPlanCommitPathspec(location), fmt.Sprintf("loom: plan artifacts for %s", seedSlug(location.WorktreeName)), fabricengine.EnvSyncOptions())
			return err
		},
		// ApprovePlan sets approved: true on the plan when the Plan-Bouncer row settles as approved.
		// It runs before that row's commit seam,
		// so the flag lands inside the commit rather than as working-tree dirt afterwards.
		// It is idempotent: a second run over an already-approved plan is a successful no-op.
		// That lets the failed-settle resume path converge.
		ApprovePlan: func() error {
			return planparser.SetApproved(planparser.PlanDir(location.AnchorPath()))
		},
		// ApproveDiscussion copies the discussion decision record onto the run's board entry.
		// It runs on Discussion-Bouncer's approved settle, before that row's commit seam.
		// Storing is idempotent, so the failed-settle resume path converges.
		ApproveDiscussion: func() error { return approveDiscussion(location, c.runID) },
		// SeedDiscussedRecord seeds a discussed board entry's stored record into the run.
		// It runs when the run enters the Motif block, so the block's gate passes over Discussion.
		SeedDiscussedRecord: func() (bool, error) { return seedDiscussedRecord(location, c.runID, record) },
		// CarryOver files each review segment's still-open findings in the decision record.
		// It commits that record alone, so no other dirty file rides along.
		CarryOver: func(entry discussionparser.CarryOver) error { return writeCarryOver(location, entry) },
		// The subject seams tell each Bouncer row what its segment judges, so a re-entry over an unchanged subject settles without a review.
		// Each reads on demand and opens nothing at wire time.
		DocumentSubject: documentSubjectWithHead(location, recordHead(location)),
		CodeSubject:     func() (shedadapters.Subject, bool, error) { return codeSubject(location, c.runID) },
		MergeInOnly:     parentMergeInOnly(location),
		RecordHead:      recordHead(location),
		// ReviewInput is the merge fixer's latest Parent-Align report.
		// A report newer than the Code-Perch's settled verdict re-opens the perch as an amendment over it.
		ReviewInput: fixerReportInput(loomengine.LoomScratchDir(location)),
		// SkipPlanReview lets Plan-Bouncer approve an exempt rework generation without a judge.
		// It reads committed state on demand and opens nothing at wire time.
		SkipPlanReview: func() (bool, error) { return loomshed.PlanReviewSkippable(reworkDeps) },
		// Rework opens nothing at wire time: every closure reads or writes on demand, since wire() also runs for status/pause.
		Rework: reworkDeps,
		// StencilsDir, SpecsDir, RunRoot, Burler, and Now are filled for both review segments:
		// the Discussion-Bouncer and Discussion-Burler-* rows, and the Plan-Bouncer and Plan-Burler-* rows.
		// StencilsDir and SpecsDir come from websterGeom, so each is one value read from one place.
		// Now is set to time.Now explicitly, since the underlying constructors also default a nil Now to time.Now.
		// The Bouncer's archive-filename collision suffix is the one place a test wants to inject a clock.
		StencilsDir:   websterGeom.StencilsDir,
		SpecsDir:      websterGeom.SpecsDir,
		DefaultEngine: shuttleCfg.Engine,
		// RunRoot is durable: the status seam commits it with every transition.
		RunRoot: loomengine.LoomReviewsDir(location),
		Burler:  burlerEngine,
		Now:     time.Now,

		// Slug and SegmentBounces tell each Bouncer row the verbs' slug and the live bounce budget its CIRCLING Reason names.
		Slug: seedSlug(location.WorktreeName),
		SegmentBounces: segmentBounces(statusPath, statusLockPath, func() (shedengine.Routing, error) {
			return loomrecipe.Routing(c.recipe, loomSegmentBudgets(loomCfg))
		}),

		Segments: loomSegmentSettings(loomCfg, reviewSettings, judgeSettings, assistBounds),
		Setting:  settingLookup,
		// SettingKnown and Resume serve a recipe row's entry gate: its operands are checked at build and its evaluation errors name the resume command.
		SettingKnown: configreg.CheckSettingKey,
		Resume:       "lym loom resume",

		ReviewCirclingCheckpoint: loomCfg.ReviewCirclingCheckpoint,
		ReviewTimeout:            reviewSettings.Timeout,

		// Landing is left unfilled here: wireLanding fills Env.Landing separately.
		// NewPublish and NewFinalize open their fabric pair eagerly at construction.
		// wire() runs for every verb, status and pause included, so that open must not happen here.
	}

	// Every loom run's Drift rows read the task from the decision record.
	recipeRows, err := loomrecipe.RowNames(c.recipe)
	if err != nil {
		return err
	}
	// Only a recipe with a Plan-Write row has a plan, so only its expect agent is told one.
	hasPlan, err := loomrecipe.HasEngine(c.recipe, planWriteEngine)
	if err != nil {
		return err
	}
	driftPlanDir := ""
	if hasPlan {
		driftPlanDir = planparser.PlanDir(location.AnchorPath())
	}
	c.env.Drift = driftDeps(location, statusPath, statusLockPath, driftTables{
		planDir:            driftPlanDir,
		rows:               recipeRows,
		runID:              c.runID,
		registry:           registry,
		subject:            driftengine.SubjectDecisionRecord,
		slug:               seedSlug(location.WorktreeName),
		decisionRecordPath: loomengine.DiscussionDecisionRecord(location),
		endRoles:           strandRoleStopper(reedEngine.Status, runner.StopStrand),
	})

	// c.shedPaths holds the values shedengine.Shed itself reads and no shedrecipe.Env registry entry reads.
	// StatusPath and StatusLockPath are set twice, here and in c.env.
	// The duplication is inherent to the split between loomrecipe.New's two argument types.
	// It must not be collapsed; loomrecipe.New errors if the two copies disagree.
	// Each pair is filled from the single statusPath and statusLockPath evaluation above,
	// so the two copies cannot drift.
	c.shedPaths = shedbuild.ShedPaths{
		StatusPath:     statusPath,
		LockPath:       shedrun.RunLock(location, c.runID),
		StatusLockPath: statusLockPath,
		// MaxBounces is left unset, so shedengine.Shed's own default applies.
		// Each producer inherits that default: a ProducerDef.MaxBounces of 0 falls back to it.
		// The final fallback is shedengine's internal default.
		// The budget is per producer and episode-scoped, not a run-wide total.
		// It is counted from the persisted history rather than held in memory.
		CommitStatus: newCommitStatusSeam(c.commitStatusDeps(location)),

		RunID:                   c.runID,
		MissingStatusWayForward: loomMissingStatusWayForward,
		RerouteCapWayForward:    loomRerouteCapWayForward,
		BudgetWayForward:        loomBudgetWayForward,
		RetiredWayForward:       loomRetiredWayForward,
	}

	// wireDarn and wireLanding pass the list on to the Darn-Write writer and the landing conflict session.
	c.languageSkills = languageSkills
	c.language = language

	// A recipe whose verify_source is tree_scope has the tree-scope verify seams filled.
	// A recipe with a Darn-Write row has its Darn-Write seams filled.
	// Any other recipe leaves both sets nil.
	verifySourceKey, err := loomrecipe.VerifySource(c.recipe)
	if err != nil {
		return err
	}
	if verifySourceKey == treeScopeVerifySource {
		if err := c.wireTreeScope(location); err != nil {
			return err
		}
	}
	hasDarnWrite, err := loomrecipe.HasEngine(c.recipe, darnWriteEngine)
	if err != nil {
		return err
	}
	if hasDarnWrite {
		if err := c.wireDarn(location, websterGeom.StencilsDir, websterGeom.SpecsDir, frictionDir, loomCfg.SubagentModel, pairSlug, registry); err != nil {
			return err
		}
	}

	// A recipe with a SamplerSession row has the row's seams filled from the hub's sampler.yaml.
	// The file loads strictly, so a malformed file is refused before any row builds.
	// The seams that need reed's strand table are filled by wireLanding.
	samplerRow, hasSampler, err := loomrecipe.EngineRow(c.recipe, samplerSessionEngine)
	if err != nil {
		return err
	}
	if hasSampler {
		samplerCfg, err := samplerengine.LoadConfig(fabricengine.ConfigDirs(location), "sampler")
		if err != nil {
			return fmt.Errorf("%w; %s", err, samplerConfigWayForward)
		}
		c.env.Sampler = samplerDeps(location, c.runID, c.recipe, samplerRow, func() (shuttleengine.Spec, error) {
			return samplerengine.SessionSpec(samplerengine.SessionInputs{
				StencilsDir:        websterGeom.StencilsDir,
				ParentName:         c.parentName,
				Slug:               seedSlug(location.WorktreeName),
				WorktreeRoot:       location.WorktreePath(),
				DecisionRecordPath: record.path,
			}, samplerCfg, registry)
		})
	}

	// A beam run's Go rows are told the hub seams of the beam's life.
	if record.project {
		pawl, err := pawlDeps(location, seedSlug(location.WorktreeName))
		if err != nil {
			return err
		}
		c.env.Pawl = pawl
		// A run moved off a row its recipe retired gets the landing's side effects, which only a beam run has.
		c.shedPaths.RetiredMove = pawlRetiredMove(location, seedSlug(location.WorktreeName), pawl)
	}

	c.location = location
	c.cwd = cwd
	c.cfg = loomCfg
	c.shuttleCfg = shuttleCfg
	c.stepIdleTimeout = time.Duration(loomCfg.StepIdleTimeoutMin) * time.Minute
	c.reed = reedEngine
	c.runDeps = runDeps
	c.registry = registry
	c.runner = runner
	c.reflectionShuttle = runner
	c.landingCfg = landingCfg
	c.frictionDir = frictionDir
	// driverStarter and driverPaneProbe wrap the runner and reed engine built above.
	// No second runner and no second reed engine is constructed here.
	c.driverStarter = runnerDriverStarter{runner: runner}
	c.driverSender = runnerDriverStarter{runner: runner}
	c.driverResumeWait = func() { time.Sleep(driverResumeSendInterval) }
	c.driverPaneProbe = newReedDriverPaneProbe(reedEngine)
	c.driverDirectory = newReedDriverDirectory(reedEngine)
	c.bouncerSubdir = loomrecipe.BouncerRunSubdir
	return nil
}

// planReviewSegment is the name of the recipe segment whose rows read tapestry.yaml's plan perch keys.
const planReviewSegment = "Plan-Review"

// recipeHoldsPlanReview reports whether the named recipe has a row of the Plan-Review segment;
// an empty name reads as the tapestry recipe.
func recipeHoldsPlanReview(recipe string) (bool, error) {
	if recipe == "" {
		recipe = shedrun.RecipeTapestry
	}
	rows, err := loomrecipe.Rows(recipe)
	if err != nil {
		return false, err
	}
	for _, row := range rows {
		if row.Segment == planReviewSegment {
			return true, nil
		}
	}
	return false, nil
}

// withTapestry returns cfg with tapestry.yaml's plan values applied when the run's recipe holds a Plan-Review row.
// Otherwise it returns cfg unchanged.
// A recipe without that segment never reads tapestry.yaml, so a fault in the file refuses none of its runs.
func (c *loomCLI) withTapestry(dirs configengine.Dirs, cfg loomengine.Config) (loomengine.Config, error) {
	holds, err := recipeHoldsPlanReview(c.recipe)
	if err != nil || !holds {
		return cfg, err
	}
	tapestry, err := loomengine.LoadTapestryConfig(dirs, "tapestry")
	if err != nil {
		return loomengine.Config{}, err
	}
	return cfg.WithTapestry(tapestry), nil
}

// loomPerches maps each loom Bouncer row's name to the review perch it belongs to.
var loomPerches = map[string]loomengine.Perch{
	loomshed.NameDiscussionBouncer: loomengine.PerchDiscussion,
	loomshed.NamePlanBouncer:       loomengine.PerchPlan,
	loomshed.NameCodeBouncer:       loomengine.PerchCode,
	loomshed.NameRaddleBouncer:     loomengine.PerchSplit,
}

// loomSegmentPerches maps each loom review segment's name to its review perch.
// The segment settings are read through this table.
// A new loom perch also needs a row in loomPerches, which keys the circling lookup by Bouncer row.
var loomSegmentPerches = map[string]loomengine.Perch{
	"Discussion-Review": loomengine.PerchDiscussion,
	planReviewSegment:   loomengine.PerchPlan,
	"Code-Review":       loomengine.PerchCode,
	"Raddle-Review":     loomengine.PerchSplit,
}

// loomSegmentBudgets maps each loom review segment to its perch's resolved soft bounce cap.
// The status path and the build share this table and its resolution.
func loomSegmentBudgets(cfg loomengine.Config) map[string]int {
	budgets := make(map[string]int, len(loomSegmentPerches))
	for segment, perch := range loomSegmentPerches {
		budgets[segment] = cfg.Caps(perch).Soft
	}

	return budgets
}

// assistBound is the capped run bound of one review segment's assistant reviewer, with the reason it must not start.
// The reason is empty when the assistant may start.
type assistBound struct {
	timeout    time.Duration
	skipReason string
}

// segmentAssistBounds returns each review segment's assistant bound.
// The bound is capped so the assistant ends before that segment's chair does.
// A segment's recipe rows set role_prefix to the name of its perch,
// so its chair runs as the role burlerengine.ReviewChairRole forms from that name.
// The await cap is the one the keep-warm resolver gives that role, since the chair's `lym shuttle await` runs under it.
// A chair name that cannot be formed leaves the default await cap.
func segmentAssistBounds(location *lymcwd.Location, geom burlerengine.Geometry, cfg loomengine.Config, runTimeout, startAllowance time.Duration) map[loomengine.Perch]assistBound {
	bounds := make(map[loomengine.Perch]assistBound, len(loomSegmentPerches))
	for _, perch := range loomSegmentPerches {
		awaitCap := shuttleengine.DefaultAwaitCap
		if chair, err := agentname.Format(geom.Shortname, geom.Slug, burlerengine.ReviewChairRole(string(perch))); err == nil {
			awaitCap = shuttleengines.KeepWarm(fabricengine.ConfigDirs(location), chair).Cap
		}
		timeout, skipReason := loomengine.CapAssistTimeout(cfg, runTimeout, startAllowance, awaitCap)
		bounds[perch] = assistBound{timeout: timeout, skipReason: skipReason}
	}
	return bounds
}

// loomSegmentSettings builds one shedrecipe.SegmentSettings per loom review segment.
// Models and their keys come from review, and the Bouncer pair comes from judge.
// The budget and hard cap, with their keys, come from cfg.Caps, which resolves them through reviewbudget.
// The assistant's lists come from review too, and each segment's assistant takes its perch's entry of assistBounds.
// The split perch has no assistant, so its segment's assistant lists are empty.
func loomSegmentSettings(cfg loomengine.Config, review loomengine.ReviewSettings, judge loomengine.JudgeSettings, assistBounds map[loomengine.Perch]assistBound) map[string]shedrecipe.SegmentSettings {
	assists := map[loomengine.Perch]loomengine.AssistModels{
		loomengine.PerchDiscussion: review.DiscussionAssist,
		loomengine.PerchPlan:       review.PlanAssist,
		loomengine.PerchCode:       review.CodeAssist,
	}
	models := map[loomengine.Perch]burlerengine.RoundModels{
		loomengine.PerchDiscussion: review.Discussion,
		loomengine.PerchPlan:       review.Plan,
		loomengine.PerchCode:       review.Code,
		loomengine.PerchSplit:      review.Split,
	}
	keys := map[loomengine.Perch]loomengine.SegmentKeys{
		loomengine.PerchDiscussion: review.DiscussionKeys,
		loomengine.PerchPlan:       review.PlanKeys,
		loomengine.PerchCode:       review.CodeKeys,
		loomengine.PerchSplit:      review.SplitKeys,
	}
	bouncers := map[loomengine.Perch]loomengine.BouncerModels{
		loomengine.PerchDiscussion: judge.Discussion,
		loomengine.PerchPlan:       judge.Plan,
		loomengine.PerchCode:       judge.Code,
		loomengine.PerchSplit:      judge.Split,
	}
	settings := make(map[string]shedrecipe.SegmentSettings, len(loomSegmentPerches))
	for segment, perch := range loomSegmentPerches {
		caps := cfg.Caps(perch)
		settings[segment] = shedrecipe.SegmentSettings{
			Models:            models[perch],
			ReviewKey:         keys[perch].Review,
			FixKey:            keys[perch].Fix,
			Bouncer:           shedadapters.BouncerModels(bouncers[perch]),
			MaxBounces:        caps.Soft,
			MaxBouncesKey:     caps.SoftKey,
			HardMaxBounces:    caps.Hard,
			HardMaxBouncesKey: caps.HardKey,
			AmendMaxBounces:   cfg.ReviewAmendMaxBounces,
			ConfigFile:        segmentConfigFile(perch),
			Assist: shedadapters.AssistSettings{
				Models:     assists[perch].Models,
				Fans:       []string(assists[perch].Fans),
				ModelKey:   assists[perch].ModelKey,
				FanKey:     assists[perch].FanKey,
				Timeout:    assistBounds[perch].timeout,
				SkipReason: assistBounds[perch].skipReason,
			},
		}
	}
	return settings
}

// segmentConfigFile returns the config file that holds perch's keys:
// tapestry.yaml for the plan perch and loom.yaml for the others.
func segmentConfigFile(perch loomengine.Perch) string {
	if perch == loomengine.PerchPlan {
		return "tapestry.yaml"
	}
	return "loom.yaml"
}

// segmentBounces returns the Env.SegmentBounces seam over the status file at statusPath, locked by statusLockPath.
// The history is read on each call, never at wire time, because it grows during the run;
// an absent status file reports not-in-segment.
// routing projects the run's recipe with the review budget its builder applies,
// so the reported budget is the one Shed blocks on.
func segmentBounces(statusPath, statusLockPath string, routing func() (shedengine.Routing, error)) func(row string) (int, int, bool, error) {
	return func(row string) (int, int, bool, error) {
		st, found, err := state.ReadJSON[shedengine.Status](statusPath, statusLockPath)
		// A run directory not yet created fails the lock open with not-exist; that is an absent status file too.
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return 0, 0, false, err
		}
		if err != nil || !found {
			return 0, 0, false, nil
		}
		projected, err := routing()
		if err != nil {
			return 0, 0, false, err
		}
		count, budget, inSegment := projected.Bounces(row, st.History)
		return count, budget, inSegment, nil
	}
}

// loomMissingStatusWayForward is the told trailing clause for a missing status file:
// loom's own start verb is what bootstraps one.
const loomMissingStatusWayForward = "way forward: run \"lym loom start\" first to bootstrap this task"

// loomRerouteCapWayForward is the told clause a reroute-cap halt ends with, whichever row's reroute hit the cap.
// The halt is resumed once the cause its reason quotes is cleared.
const loomRerouteCapWayForward = "run \"lym loom resume\" in the task worktree once the cause the halt's reason quotes is cleared: for a Webster plan conflict, once the run's parent has recorded the fix with \"lym loom decision add\" and \"--amends-plan\", then run \"lym loom goto --to Plan-Burler-Review\""

// loomBudgetWayForward is the told clause a bounce-budget-exhausted halt ends with, whichever row's budget ran out.
// Only the resume grants a budget; a goto keeps the spent one.
const loomBudgetWayForward = "run \"lym loom resume\" in the task worktree: the resume grants the row one more budget; a goto keeps the spent budget and grants none"

// loomRetiredWayForward is the command a move off a retired row names as the way forward.
// The resume steps the run again, so it is the command to name.
const loomRetiredWayForward = "lym loom resume"

// hubConfigWayForward is the trailing clause for a failed load of the hub-wide landing config:
// reconciling the hub's config is what restores it.
const hubConfigWayForward = "way forward: run lym config reconcile --apply in the hub, then step again"
