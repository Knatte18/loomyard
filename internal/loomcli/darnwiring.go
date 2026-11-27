// darnwiring.go holds the wiring only a tree-scoped or darn run needs.
// It holds the tree scope reader for `verify_source: tree_scope`, the verify source, and the Darn-Write row's seams.
// It also holds the writer's Spec source and the agent rows' record check.
// A tapestry run leaves all of it unwired, but shares the merge base readers `pairMergeBase` and `taskMergeBase` build.

package loomcli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/darnengine"
	"github.com/Knatte18/loomyard/internal/discussionparser"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/hardenshed"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/orchcli"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/stencilstore"
	"github.com/Knatte18/loomyard/internal/summaryparser"
)

// darnVerifySource returns the darn recipe's verify source: the call of the gate key a landing row names.
// The command resolves at each call, never at construction, so an edit between steps is seen.
// For a task diff wholly under the hub's configured trees, it is the key's call over the touched trees' targets.
// Otherwise it is the key's call alone.
// A gate config refusal and a malformed tree key are errors naming the way forward.
// baseCommand is the call of the `code-burler-fix` gate key, the fix rows' base command.
// A tree-scoped site keeps this base command.
// The tree map comes from trees, read at each call.
func darnVerifySource(l *lymcwd.Location, trees func() (map[string]string, error)) verifySource {
	dirs := fabricengine.ConfigDirs(l)
	treeScope := darnTreeScope(l, trees)
	call := gateCallSource(dirs)
	return verifySource{
		command: func(gateKey string) (string, error) {
			command, scoped, err := treeScope(gateKey)
			if err != nil {
				return "", err
			}
			if scoped {
				return command, nil
			}
			return call(gateKey)
		},
		baseCommand: func() (string, error) { return call("code-burler-fix") },
		gateTiers:   gateTiersLookup(dirs),
	}
}

// pairMergeBase returns the reader of the task branch's merge base with its recorded parent branch.
// The fabric is opened and the parent branch read on each call,
// so a parent that moved after bootstrap is read as it stands.
func pairMergeBase(l *lymcwd.Location) func() (string, error) {
	return func() (string, error) {
		parent, err := readRecordedParentBranch(l)
		if err != nil {
			return "", err
		}
		handle, err := fabricengine.Open(l)
		if err != nil {
			return "", err
		}
		return handle.MergeBase(parent)
	}
}

// darnTreeScope returns the tree scope reader of a recipe whose verify scope follows a tree map.
// The reader returns the tree-scoped command and true for a task diff wholly under the trees trees reads;
// otherwise it returns ("", false, nil).
// The command is the call of the row's gate key followed by the touched trees' targets,
// so a tree-scoped site runs its own key's tiers over those targets in place of the key's scope.
// Each call reads the tree map afresh, so an edit between steps is seen.
// It reads the diff from the pair's merge base to HEAD, counting a rename at both its paths.
// A read error of trees is returned as it is, carrying its own way forward.
// A scoped diff whose trees contribute no target answers an empty command, which runs nothing.
func darnTreeScope(l *lymcwd.Location, trees func() (map[string]string, error)) func(gateKey string) (string, bool, error) {
	mergeBase := taskMergeBase(l)
	call := gateCallSource(fabricengine.ConfigDirs(l))
	return func(gateKey string) (string, bool, error) {
		treeScope, err := trees()
		if err != nil {
			return "", false, err
		}
		if len(treeScope) == 0 {
			return "", false, nil
		}
		base, err := mergeBase()
		if err != nil {
			return "", false, err
		}
		handle, err := fabricengine.Open(l)
		if err != nil {
			return "", false, err
		}
		paths, err := handle.ChangedPathsBothSides(base, "HEAD")
		if err != nil {
			return "", false, err
		}
		targets, scoped := darnengine.TreeScopedTargets(darnengine.Config{TreeScope: treeScope}, paths)
		if !scoped || len(targets) == 0 {
			return "", scoped, nil
		}
		keyCall, err := call(gateKey)
		if err != nil {
			return "", false, err
		}
		return keyCall + " " + strings.Join(targets, " "), true, nil
	}
}

// latestDarnOutcome returns the latest Darn-Write history entry of the status file at statusPath.
// The status file is read under the lock at statusLockPath.
// The returned `FindingsPath` is findingsPath when the entry is a Stuck and that file exists, and empty otherwise:
// the kept findings outlive the resume write that clears the status file's error before the row is called.
// The file also outlives a later call whose gate passed, so an entry that did not halt names none.
// A Stuck whose spawn began after the file was last written wrote no findings of its own, so it names none either.
// Reroute, goto and skipped entries record no spawn that did work, so they are passed over.
// An absent status file, or a history with no Darn-Write entry besides the passed-over ones, reports not found.
func latestDarnOutcome(statusPath, statusLockPath, findingsPath string) (loomshed.DarnOutcome, bool, error) {
	st, found, err := state.ReadJSON[shedengine.Status](statusPath, statusLockPath)
	// A run directory not yet created fails the lock open with not-exist; that is an absent status file too.
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return loomshed.DarnOutcome{}, false, err
	}
	if err != nil || !found {
		return loomshed.DarnOutcome{}, false, nil
	}
	for i := len(st.History) - 1; i >= 0; i-- {
		entry := st.History[i]
		if entry.Producer != loomshed.NameDarnWrite || entry.Outcome == shedengine.OutcomeReroute || entry.Outcome == shedengine.OutcomeGoto || entry.Outcome == shedengine.OutcomeSkipped {
			continue
		}
		latest := loomshed.DarnOutcome{Outcome: entry.Outcome}
		if entry.Outcome != shedengine.Stuck {
			return latest, true, nil
		}
		info, err := os.Stat(findingsPath)
		if err == nil {
			if !fileWrittenBeforeSpawn(info.ModTime(), entry.StartedAt) {
				latest.FindingsPath = findingsPath
			}
		} else if !errors.Is(err, fs.ErrNotExist) {
			return loomshed.DarnOutcome{}, false, err
		}
		return latest, true, nil
	}

	return loomshed.DarnOutcome{}, false, nil
}

// fileWrittenBeforeSpawn reports whether a file last written at modTime is older than the spawn that began at startedAt.
// A start time that is empty or not RFC 3339 cannot be compared, so the file is not counted older.
func fileWrittenBeforeSpawn(modTime time.Time, startedAt string) bool {
	started, err := time.Parse(time.RFC3339, startedAt)
	return err == nil && modTime.Before(started)
}

// discussionRecordCheck returns the record check a recipe's agent rows run over the decision record.
// The check also reads the support log and the committed history of the record that recipe keeps.
// It reads on demand and opens nothing at build time.
func discussionRecordCheck(l *lymcwd.Location, recipe string) func() ([]discussionparser.Finding, error) {
	return func() ([]discussionparser.Finding, error) {
		return loomshed.ValidateDiscussion(loomshed.DiscussionCheck{
			RecordPath:     loomengine.DiscussionDecisionRecord(l),
			SupportLogPath: loomengine.DiscussionSupportLog(l),
			History:        recordHistory(l, runRecordOf(l, recipe)),
		})
	}
}

// darnDeps assembles the Darn-Write row's seams for location and the run addressed by runID, whose recipe is recipe.
// Every closure reads or writes on demand and opens nothing at build time.
func darnDeps(l *lymcwd.Location, runID, recipe string) loomshed.DarnDeps {
	mergeBase := taskMergeBase(l)
	return loomshed.DarnDeps{
		RecordPath:              loomengine.DiscussionDecisionRecord(l),
		SupportLogPath:          loomengine.DiscussionSupportLog(l),
		StopMarkerPath:          shedrun.StopMarker(l, runID),
		ConditionStopMarkerPath: shedrun.ConditionStopMarker(l, runID),
		CommittedRecord: func() (bool, error) {
			_, found, err := fabricengine.CommittedAnchoredFile(l, loomengine.DiscussionDecisionRecordRel())
			return found, err
		},
		Commit: func() error { return commitDescription(l) },
		LatestOutcome: func() (loomshed.DarnOutcome, bool, error) {
			return latestDarnOutcome(shedrun.StatusFile(l, runID), shedrun.StatusLock(l, runID), shedrun.GateFindingsFile(l, runID, loomshed.NameDarnWrite))
		},
		BranchCommits: func() ([]string, error) {
			base, err := mergeBase()
			if err != nil {
				return nil, err
			}
			handle, err := fabricengine.Open(l)
			if err != nil {
				return nil, err
			}
			return handle.CommitLog(base, "HEAD")
		},
		ScratchDir:  fabricengine.ScratchLinkPath(l),
		CheckRecord: discussionRecordCheck(l, recipe),
	}
}

// wireTreeScope fills the verify Env fields that a recipe whose verify_source is tree_scope reads.
// Those are in addition to the fields filled for every recipe.
// It loads the hub-wide darn.yaml strictly.
// The start, run, step and resume verbs therefore refuse an absent or malformed file before any row builds.
// A malformed tree_scope key gets the editor way forward instead of the hub reconcile one.
// The reason is that `--set` cannot remove a key the hub file holds.
// The Darn-Write key's told and tree-scoped commands carry `--stop-on-fail`.
// Its verify therefore stops at its first failed tier.
// Every other key keeps its whole call, the landing call included.
// The verify budget is darn.yaml's verify_budget_min, and its notice goes to the run's parent orch.
// A hardener run loads hardener.yaml in place of darn.yaml.
// It takes hardener.yaml's verify_attempts and sets no verify budget.
// Changing darn.yaml therefore leaves a hardener run's verify scope and attempts unchanged.
func (c *loomCLI) wireTreeScope(location *lymcwd.Location) error {
	dirs := fabricengine.ConfigDirs(location)
	verifyAttempts, budget, err := c.treeScopeSettings(location)
	if err != nil {
		return err
	}

	// Each gate row's told command is the call of its own gate key;
	// the tree-scoped command reaches the gate through `VerifyTreeScope`, never through this source.
	call := gateCallSource(dirs)
	c.env.VerifyCommand = func(key string) (string, error) {
		command, err := call(key)
		if err != nil {
			return "", err
		}
		if key != darnWriteGateKey {
			return command, nil
		}
		return gateslot.StopOnFailCall(command), nil
	}
	c.env.VerifyMergeBase = taskMergeBase(location)
	scope := darnTreeScope(location, recipeTrees(location, c.recipe))
	c.env.VerifyTreeScope = func(key string) (string, bool, error) {
		command, scoped, err := scope(key)
		if err != nil || command == "" || key != darnWriteGateKey {
			return command, scoped, err
		}
		return gateslot.StopOnFailCall(command), scoped, nil
	}
	c.env.DarnVerifyAttempts = verifyAttempts
	c.env.VerifyBudget = budget

	return nil
}

// treeScopeSettings returns the verify attempts and verify budget of a run whose verify source is tree_scope.
// A hardener run reads hardener.yaml's verify_attempts and has no verify budget.
// Any other run reads darn.yaml's verify_attempts and verify_budget_min, its notice going to the run's parent orch.
// The notice opens with the run's slug, since the orch reads the notices of every run of the hub.
// A load error carries its way forward.
// That is the editor's for a malformed tree key, and the hub reconcile's for darn.yaml otherwise.
func (c *loomCLI) treeScopeSettings(location *lymcwd.Location) (int, loomshed.VerifyBudget, error) {
	dirs := fabricengine.ConfigDirs(location)
	if recipeHasEngine(c.recipe, hardenFixEngine) {
		cfg, err := hardenshed.LoadConfig(dirs, hardenshed.ConfigModule)
		return cfg.VerifyAttempts, loomshed.VerifyBudget{}, err
	}
	cfg, err := darnengine.LoadConfig(dirs, "darn")
	if err != nil {
		if errors.Is(err, darnengine.ErrTreeScopeKey) {
			return 0, loomshed.VerifyBudget{}, darnengine.WithEditorWayForward(err)
		}
		return 0, loomshed.VerifyBudget{}, fmt.Errorf("%w; %s", err, hubConfigWayForward)
	}
	return cfg.VerifyAttempts, loomshed.VerifyBudget{
		Limit: time.Duration(cfg.VerifyBudgetMin) * time.Minute,
		Notify: func(line string) error {
			_, err := orchcli.NotifyParent(location, loomProgram, fmt.Sprintf("loom %s: %s", location.WorktreeName, line))
			return err
		},
	}, nil
}

// wireDarn fills the Env fields that a recipe with a Darn-Write row reads:
// the writer's seams, its table, and Preflight's stencils check.
// That check builds the table a first spawn reads and refuses a marker of its stencils no value fills.
// A stencil newer than the running binary, or a blank value, halts the run at Preflight, before any agent row.
// The newer stencil's refusal keeps its own way forward alone.
// It loads the hub-wide darn.yaml strictly.
// So start, run, step and resume refuse an absent or malformed file before any row builds.
// A malformed darn.tree_scope key is refused with the editor way forward.
// The reason is that --set cannot remove a key the hub file holds.
// subagentModel is loom.subagent_model: the model the writer's prompt names for the subagents it hands work to.
// pairSlug is the worktree's pair slug, empty on the prime.
// When it is set, a verify gate whose fixer setting holds a model-spec has the writer's prompt drop its gate runs.
func (c *loomCLI) wireDarn(location *lymcwd.Location, stencilsDir, specsDir, frictionDir, subagentModel, pairSlug string, registry modelspec.Registry) error {
	dirs := fabricengine.ConfigDirs(location)
	cfg, err := darnengine.LoadConfig(dirs, "darn")
	if err != nil {
		if errors.Is(err, darnengine.ErrTreeScopeKey) {
			return darnengine.WithEditorWayForward(err)
		}
		return fmt.Errorf("%w; %s", err, hubConfigWayForward)
	}

	c.env.Darn = darnDeps(location, c.runID, c.recipe)
	darnTable := func(told loomshed.DarnTold) (seatengine.Table, error) {
		gateFixed, err := writerFixerOn(c.recipe, darnWriteEngine, darnGateEntry, c.env.Setting, pairSlug)
		if err != nil {
			return seatengine.Table{}, err
		}
		// Only a writer whose gate has a cold fixer names the measuring call, so only its table needs one.
		// The call follows the hub's gate config, so the prompt names only a form that config can run.
		var measureCall string
		if gateFixed {
			gateCfg, err := gateslot.LoadConfig(dirs)
			if err != nil {
				return seatengine.Table{}, err
			}
			measureCall, err = gateCfg.MeasureCall(darnWriteGateKey)
			if err != nil {
				return seatengine.Table{}, err
			}
		}
		table, err := darnengine.DarnTable(darnengine.DarnInputs{
			StencilsDir:        stencilsDir,
			SpecsDir:           specsDir,
			WorktreeRoot:       location.WorktreePath(),
			FrictionDir:        frictionDir,
			Slug:               seedSlug(location.WorktreeName),
			DescriptionPath:    summaryparser.Path(loomengine.LandingDir(location)),
			DecisionRecordPath: loomengine.DiscussionDecisionRecord(location),
			StopMarkerPath:     shedrun.StopMarker(location, c.runID),
			PriorWork:          told.PriorWork,
			LanguageSkills:     c.languageSkills,
			SubagentModel:      subagentModel,
			GateFixed:          gateFixed,
			MeasureCall:        measureCall,

			ConditionStopMarkerPath: shedrun.ConditionStopMarker(location, c.runID),
		}, cfg, registry)
		if err != nil {
			return seatengine.Table{}, err
		}
		return withProjectDirective(table, location, stencilsDir)
	}
	c.env.DarnTable = darnTable
	c.env.PreflightStencils = func() error {
		table, err := c.env.DarnTable(loomshed.DarnTold{})
		if err != nil {
			return preflightTableError(err)
		}
		return table.CheckMarkers(stencilsDir, c.shuttleCfg.Engine)
	}

	return nil
}

// preflightTableError wraps a failed build of the darn writer's table for the Preflight stencils check.
// A refusal of a stencil that a newer lym build wrote keeps its own way forward alone, since no config is at fault.
// Any other build error gains the way forward that fixes the config or model spec it names.
func preflightTableError(err error) error {
	var newer *stencilstore.NewerBoardCopyError
	if errors.As(err, &newer) {
		return fmt.Errorf("build the darn writer table: %w", err)
	}
	return fmt.Errorf("build the darn writer table: %w; way forward: fix the config or model spec it names, then re-step", err)
}
