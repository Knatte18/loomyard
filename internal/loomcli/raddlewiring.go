// raddlewiring.go builds the seams of the raddle recipe's rows for the run at a location.
// Raddle-Write gets its paths, table source and proposal check;
// Raddle-File gets its board writes and the child process that makes a filed project a beam.

package loomcli

import (
	"errors"
	"fmt"
	"io/fs"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomrecipe"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/orchcli"
	"github.com/Knatte18/loomyard/internal/raddleparser"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/selfexec"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrecipe"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/state"
)

// raddleFileEngine is the engine of the recipe row that files the approved raddle proposal on the board.
const raddleFileEngine = "RaddleFile"

// raddleSettingModule and raddleAutoApproveKey name the raddle auto-approve setting.
// When it is true, a raddle run files without waiting for the operator's approval.
// A run whose recipe rows hold a split perch never reads it.
const (
	raddleSettingModule  = "loom"
	raddleAutoApproveKey = "raddle_auto_approve"
)

// raddleRunHistory returns the history of the run's status file at location, empty when the run has no status file yet.
func raddleRunHistory(location *lymcwd.Location) ([]shedengine.HistoryEntry, error) {
	status, found, err := state.ReadJSON[shedengine.Status](shedrun.StatusFile(location, shedrun.SelfRunID), shedrun.StatusLock(location, shedrun.SelfRunID))
	// A run directory not yet created fails the lock open with not-exist; that is an absent status file too.
	if errors.Is(err, fs.ErrNotExist) || (err == nil && !found) {
		return nil, nil
	}
	return status.History, err
}

// freshRaddleEntry reports whether the run's current attempt at Raddle-Write began with a return or a goto into the row.
// The last history entry decides.
// A return is a Raddle-File Stuck routed to Raddle-Write.
// A goto is the entry a goto or relaunch writes: Raddle-Write itself as its producer, outcome goto and no route.
// It returns the 1-based history position of that entry, zero when the entry is not fresh.
func freshRaddleEntry(history []shedengine.HistoryEntry) (fresh bool, position int) {
	if len(history) == 0 {
		return false, 0
	}
	last := history[len(history)-1]
	returned := last.Producer == loomshed.NameRaddleFile && last.Outcome == shedengine.Stuck && last.RoutedTo == loomshed.NameRaddleWrite
	wentTo := last.Producer == loomshed.NameRaddleWrite && last.Outcome == shedengine.OutcomeGoto
	if returned || wentTo {
		return true, len(history)
	}
	return false, 0
}

// raddleRunFiling returns the slugs the raddle run slug at location filed, read from the raddle store.
// The bool is false while the run has filed none.
// A store entry left by an earlier board entry of the same slug is not the run's, since its created stamp differs.
func raddleRunFiling(location *lymcwd.Location, slug string) ([]string, bool, error) {
	store, err := boardengine.ReadRaddleStore(fabricengine.BoardDir(location.HubPath))
	if err != nil {
		return nil, false, err
	}
	held, isHeld := store.Holds(slug)
	if !isHeld || held.Sketch != slug {
		return nil, false, nil
	}
	board, err := openHubBoard(location)
	if err != nil {
		return nil, false, err
	}
	task, found, err := board.GetTask(slug)
	if err != nil {
		return nil, false, fmt.Errorf("read the run's board entry: %w", err)
	}
	if found && !held.Created.Equal(task.Created) {
		return nil, false, nil
	}
	return held.Filed, true, nil
}

// openHubBoard opens the board of the hub at location, naming the board in the error.
func openHubBoard(location *lymcwd.Location) (*boardengine.Board, error) {
	board, err := boardengine.OpenHub(location.HubPath)
	if err != nil {
		return nil, fmt.Errorf("open the board: %w", err)
	}
	return board, nil
}

// commitRaddle commits the raddle directory of location's worktree through the fabric.
// A repeat commit over an unchanged directory is a no-op.
func commitRaddle(location *lymcwd.Location, slug string) error {
	_, _, err := fabricengine.CommitAnchoredPaths(fabricengine.NewMutations(""), location, []string{loomengine.RaddleDirRel()}, fmt.Sprintf("loom: raddle artifacts for %s", slug), fabricengine.EnvSyncOptions())
	return err
}

// raddleWriteEnv returns the Raddle-Write row's seams for the run slug of the recipe at location.
// It covers the run's raddle paths, the history seam and the commit.
// It covers the mode's sources too: the split perch read from the recipe's rows and the auto-approve setting.
// It also covers the seat table source and the proposal check.
// Every closure reads or writes on demand and opens nothing at build time.
func raddleWriteEnv(location *lymcwd.Location, cfg loomengine.Config, stencilsDir, specsDir string, reg modelspec.Registry, slug, recipe string, setting func(module, key string) (any, error)) shedrecipe.RaddleWriteEnv {
	statePath := loomengine.RaddleStatePath(location)
	stateLockPath := loomengine.RaddleStateLockPath(location)
	proposalPath := loomengine.RaddleProposal(location)
	coldPath := loomengine.RaddleColdProposal(location)
	return shedrecipe.RaddleWriteEnv{
		Deps: loomshed.RaddleWriteDeps{
			StatePath:     statePath,
			StateLockPath: stateLockPath,
			ProposalPath:  proposalPath,
			ReturnedPath:  loomengine.RaddleReturnedProposal(location),
			ColdPath:      coldPath,
			ApprovalPath:  loomengine.RaddleApproval(location),
			SplitPerch:    func() (bool, error) { return loomrecipe.HoldsSplitPerch(recipe) },
			AutoApprove: func() (bool, error) {
				value, err := setting(raddleSettingModule, raddleAutoApproveKey)
				if err != nil {
					return false, err
				}
				auto, ok := value.(bool)
				if !ok {
					return false, fmt.Errorf("setting %s.%s holds %v, want true or false; way forward: set %s in loom.yaml, or in the run profile's raddle block", raddleSettingModule, raddleAutoApproveKey, value, raddleAutoApproveKey)
				}
				return auto, nil
			},
			FreshEntry: func() (bool, int, error) {
				history, err := raddleRunHistory(location)
				if err != nil {
					return false, 0, err
				}
				fresh, position := freshRaddleEntry(history)
				return fresh, position, nil
			},
			// Interactive mode stores the cold proposal in the propose verb,
			// so the gate's pass stores it only in auto and perch mode.
			StoreCold: func() error {
				recorded, err := loomshed.ReadRaddleState(statePath, stateLockPath)
				if err != nil {
					return err
				}
				if recorded.Mode == loomengine.RaddleModeInteractive {
					return nil
				}
				return loomshed.StoreColdProposal(proposalPath, coldPath, statePath, stateLockPath)
			},
			Commit: func() error { return commitRaddle(location, slug) },
		},
		Table: func(mode loomengine.RaddleMode, returnReason string) (seatengine.Table, error) {
			return loomengine.RaddleTable(location, stencilsDir, specsDir, cfg, reg, slug, mode, returnReason)
		},
		Check: raddleCheckFor(location),
	}
}

// raddleFileDeps returns the Raddle-File row's deps for the run slug at location.
// It covers the run's raddle paths and the board writes of the filing.
// Every closure opens the board or reads the run on a call, never at build time.
func raddleFileDeps(location *lymcwd.Location, slug string) loomshed.RaddleFileDeps {
	boardDir := fabricengine.BoardDir(location.HubPath)
	return loomshed.RaddleFileDeps{
		ProposalPath:   loomengine.RaddleProposal(location),
		ColdPath:       loomengine.RaddleColdProposal(location),
		RecordPath:     loomengine.DiscussionDecisionRecord(location),
		StageDir:       loomengine.RaddleStageDir(location),
		RecordsDir:     filepath.Dir(loomengine.RaddlePartRecord(location, slug)),
		PartRecordPath: func(partSlug string) string { return loomengine.RaddlePartRecord(location, partSlug) },
		StatePath:      loomengine.RaddleStatePath(location),
		StateLockPath:  loomengine.RaddleStateLockPath(location),
		Check: func() ([]raddleparser.Finding, error) {
			return loomshed.CheckRaddleProposal(raddleCheckFor(location), true)
		},
		Recheck: func() (raddleparser.Recheck, error) {
			return loomshed.RecheckRaddleBoard(raddleCheckFor(location))
		},
		Entry: func() (loomshed.RaddleRunEntry, error) {
			board, err := openHubBoard(location)
			if err != nil {
				return loomshed.RaddleRunEntry{}, err
			}
			task, found, err := board.GetTask(slug)
			if err != nil {
				return loomshed.RaddleRunEntry{}, fmt.Errorf("read the run's board entry: %w", err)
			}
			if !found {
				return loomshed.RaddleRunEntry{}, fmt.Errorf("the board holds no entry for the run %q; way forward: restore the entry with \"lym board upsert\", then re-step the Raddle-File row", slug)
			}
			boardConfig, err := boardengine.LoadConfig(fabricengine.ConfigDirs(location), "board")
			if err != nil {
				return loomshed.RaddleRunEntry{}, err
			}
			vocabulary := boardConfig.Vocabulary()
			entry := loomshed.RaddleRunEntry{Slug: task.Slug, Created: task.Created, Project: task.Project}
			for _, label := range task.Labels {
				if !vocabulary.IsType(label) {
					entry.ModuleLabels = append(entry.ModuleLabels, label)
				}
			}
			return entry, nil
		},
		StoreDone: func(created time.Time, hash string) (bool, error) {
			store, err := boardengine.ReadRaddleStore(boardDir)
			if err != nil {
				return false, err
			}
			held, isHeld := store.Holds(slug)
			return isHeld && held.Sketch == slug && held.Hash == hash && held.Created.Equal(created), nil
		},
		AlreadyFiled: func(sketch string, slugs []string) (bool, error) {
			board, err := openHubBoard(location)
			if err != nil {
				return false, err
			}
			return board.HoldsRaddleFiling(sketch, slugs)
		},
		File: func(filing boardengine.RaddleFiling) error {
			board, err := openHubBoard(location)
			if err != nil {
				return err
			}
			return board.FileRaddle(filing)
		},
		CreateBeam: func(project string) error { return createBeamFromParent(location, project, runLymChild) },
		// Notify names the run first, since the orch reads the notices of every run of the hub.
		Notify: func(line string) error {
			_, err := orchcli.NotifyParent(location, loomProgram, fmt.Sprintf("loom %s: %s", slug, line))
			return err
		},
		EndEntry: func() error {
			board, err := openHubBoard(location)
			if err != nil {
				return err
			}
			return board.EndRaddleEntry(slug)
		},
		Commit: func() error { return commitRaddle(location, slug) },
	}
}

// createBeamFromParent makes the project a beam by running `lym beam create <project>` through run.
// It runs in the worktree that is the parent of the run's slug, or in the prime when that parent names none.
// The beam create verb skips each step whose result exists, so a retry after a failed run finishes it.
func createBeamFromParent(location *lymcwd.Location, project string, run func(dir string, args ...string) error) error {
	prime, err := primeOfHub(location)
	if err != nil {
		return err
	}
	parent, err := hubgeom.ParentOf(prime, seedSlug(location.WorktreeName))
	if err != nil {
		return fmt.Errorf("resolve the parent worktree of the run: %w", err)
	}
	dir := prime.WorktreePath()
	if parent != "" {
		dir = fabricengine.WorktreePath(prime, parent)
	}
	return run(dir, "beam", "create", project)
}

// runLymChild runs this binary with args in dir and waits for it, returning its output in the error when it fails.
func runLymChild(dir string, args ...string) error {
	executable, err := selfexec.LymBinary()
	if err != nil {
		return fmt.Errorf("find the lym binary: %w", err)
	}
	logger.Info("loom: spawning a lym child", "dir", dir, "args", strings.Join(args, " "))
	command := exec.Command(executable, args...)
	command.Dir = dir
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("lym %s in %s: %w: %s", strings.Join(args, " "), dir, err, strings.TrimSpace(string(output)))
	}
	return nil
}
