// driftwiring.go builds the seams the three Drift rows are told: the run status, the git facts, the records commit, the two agents' tables and the capture and compare runs.
// Every seam opens the fabric or reads a file on demand, since wire also runs for status and pause.

package loomcli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/Knatte18/loomyard/internal/driftengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/state"
)

const (
	// driftCaptureRecordWait bounds how long a detached capture start waits for its child's running record.
	driftCaptureRecordWait = 30 * time.Second
	// driftCaptureRecordPoll is the interval a detached capture start reads that record at.
	driftCaptureRecordPoll = 50 * time.Millisecond
	// driftCaptureLockRetry bounds how long a capture retries a capture lock a reader's probe holds.
	driftCaptureLockRetry = 2 * time.Second
	// driftCompareLockRetry bounds how long a compare retries a compare marker a reader's probe holds.
	driftCompareLockRetry = 2 * time.Second
)

// driftTables carries the values the two Drift agents' tables are built from, which wire resolves once.
type driftTables struct {
	// stencilsDir is the directory the agents' stencils are read from.
	stencilsDir string
	// registry resolves the agents' model-specs.
	registry modelspec.Registry
	// subject is driftengine.SubjectDecisionRecord or driftengine.SubjectBoardEntry.
	subject string
	// slug is the board entry's slug, read for the board-entry subject.
	slug string
	// decisionRecordPath is the absolute path of the task's decision record.
	decisionRecordPath string
	// languageSkills are the hub's language skills the verify agent loads.
	languageSkills []string
}

// driftDeps builds the seams the Drift rows read over location's worktree.
// statusPath and statusLockPath locate the run status; awaitRow is true for the recipe that has a Drift-Await row.
// The seat runner, clock and gate findings file are left unset, since the registry entries fill them from the recipe's environment.
func driftDeps(location *lyxcwd.Location, statusPath, statusLockPath string, awaitRow bool, tables driftTables) driftengine.Deps {
	paths := driftengine.NewPaths(location.AnchorPath())
	pool, waitDir := hubgeom.GateSlots(location), gateslot.WaitDir(location.AnchorPath())
	config := func() (driftengine.Config, error) {
		return driftengine.LoadConfig(fabricengine.ConfigDirs(location))
	}
	status := func() (shedengine.Status, error) {
		st, found, err := state.ReadJSONNoMkdir[shedengine.Status](statusPath, statusLockPath)
		if err != nil {
			return shedengine.Status{}, err
		}
		if !found {
			return shedengine.Status{}, errors.New("drift: this worktree has no run status; way forward: lyx loom resume")
		}
		return st, nil
	}
	head := func() (string, error) {
		fabric, err := fabricengine.Open(location)
		if err != nil {
			return "", err
		}
		return fabric.HeadSHA()
	}
	branchPoint := pairMergeBase(location)
	dirtyPaths := func() ([]string, error) { return fabricengine.SourceDirtyPaths(location) }
	checkMain := func() (string, error) {
		st, err := status()
		if err != nil {
			return "", err
		}
		headSHA, err := head()
		if err != nil {
			return "", err
		}
		base, err := branchPoint()
		if err != nil {
			return "", err
		}
		dirty, err := dirtyPaths()
		if err != nil {
			return "", err
		}
		if err := driftengine.CheckMainCapture(st, headSHA, base, dirty); err != nil {
			return "", err
		}
		return headSHA, nil
	}
	stepOf := func(name string) (driftengine.CommandStep, error) {
		cfg, err := config()
		if err != nil {
			return driftengine.CommandStep{}, err
		}
		return cfg.Step(name)
	}

	return driftengine.Deps{
		Paths:        paths,
		WorktreeRoot: location.WorktreePath(),
		Config:       config,
		Status:       status,
		AwaitRow:     awaitRow,
		Head:         head,
		BranchPoint:  branchPoint,
		DirtyPaths:   dirtyPaths,
		Commit: func(relPaths []string) error {
			_, _, err := fabricengine.CommitAnchoredPaths(fabricengine.NewMutations(""), location, relPaths, fmt.Sprintf("loom: drift records for %s", seedSlug(location.WorktreeName)), fabricengine.EnvSyncOptions())
			return err
		},
		BaselineTable: func(scopeNote string) (seatengine.Table, error) {
			cfg, err := config()
			if err != nil {
				return seatengine.Table{}, err
			}
			return driftengine.BaselineTable(driftengine.BaselineInputs{
				StencilsDir:        tables.stencilsDir,
				WorktreeRoot:       location.WorktreePath(),
				Subject:            tables.subject,
				Slug:               tables.slug,
				DecisionRecordPath: tables.decisionRecordPath,
				BaselineReportPath: paths.BaselineReport,
				ScopeNote:          scopeNote,
			}, cfg, tables.registry)
		},
		StartCapture: func() error {
			executable, err := os.Executable()
			if err != nil {
				return fmt.Errorf("drift: resolve this binary: %w", err)
			}
			_, err = driftengine.StartCapture(driftengine.SpawnRequest{
				Executable:   executable,
				WorktreeRoot: location.WorktreePath(),
				Paths:        paths,
				RunProcDir:   fabricengine.RunProcDir(location.HubPath),
				Owner:        location.WorktreeName,
				RecordWait:   driftCaptureRecordWait,
				Poll:         driftCaptureRecordPoll,
			})
			return err
		},
		RunCapture: func(ctx context.Context) (driftengine.CaptureRecord, error) {
			step, err := stepOf("capture")
			if err != nil {
				return driftengine.CaptureRecord{}, err
			}
			scope, _, err := driftengine.ReadScope(paths)
			if err != nil {
				return driftengine.CaptureRecord{}, err
			}
			return driftengine.RunCapture(ctx, driftengine.CaptureRequest{
				Paths:        paths,
				WorktreeRoot: location.WorktreePath(),
				Step:         step,
				Pool:         pool,
				WaitDir:      waitDir,
				Skip:         scope.Skip,
				Check:        checkMain,
				Timeout:      time.Duration(step.TimeoutMin) * time.Minute,
				Now:          time.Now,
				LockRetry:    driftCaptureLockRetry,
			})
		},
		Sleep: sleepContext,
		VerifyTable: func(told driftengine.VerifyTold) (seatengine.Table, error) {
			cfg, err := config()
			if err != nil {
				return seatengine.Table{}, err
			}
			return driftengine.VerifyTable(driftengine.VerifyInputs{
				StencilsDir:        tables.stencilsDir,
				WorktreeRoot:       location.WorktreePath(),
				Subject:            tables.subject,
				Slug:               tables.slug,
				DecisionRecordPath: tables.decisionRecordPath,
				ScopePath:          paths.Scope,
				BaselineReportPath: paths.BaselineReport,
				VerifyReportPath:   paths.VerifyReport,
				CompareLog:         told.CompareLog,
				BaselineHead:       told.BaselineHead,
				LanguageSkills:     tables.languageSkills,
			}, cfg, tables.registry)
		},
		Compare: func(ctx context.Context) (driftengine.CompareResult, error) {
			step, err := stepOf("compare")
			if err != nil {
				return driftengine.CompareResult{}, err
			}
			scope, _, err := driftengine.ReadScope(paths)
			if err != nil {
				return driftengine.CompareResult{}, err
			}
			return driftengine.RunCompare(ctx, driftengine.CompareRequest{
				Paths:        paths,
				WorktreeRoot: location.WorktreePath(),
				Step:         step,
				Pool:         pool,
				WaitDir:      waitDir,
				Skip:         driftengine.SkipList(scope),
				Timeout:      time.Duration(step.TimeoutMin) * time.Minute,
				Now:          time.Now,
				LockRetry:    driftCompareLockRetry,
			})
		},
	}
}

// sleepContext waits d, or returns the context's error once ctx ends first.
func sleepContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
