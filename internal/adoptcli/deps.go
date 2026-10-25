// deps.go fills adoptDeps over a real hub: the origin reconcile with the live-pair reading adopt needs, the pair add, the owner write, the pause lift, the run resume, the child resume, the dobby start and the test suite start.

package adoptcli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/battencli"
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/dobbyengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/machinename"
	"github.com/Knatte18/loomyard/internal/pairteardown"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/tendercli"
	"github.com/Knatte18/loomyard/internal/tenderengine"
)

// hubDeps fills adoptDeps over the hub the prime belongs to.
// The reconcile's live-pair reading takes its pause cover from hubOnlyPaused, so a pair whose strands are alive under a slug-scope pause, an owner pause included, reads live.
func hubDeps(prime *lyxcwd.Location) (adoptDeps, error) {
	return hubDepsAs(prime, machinename.Name)
}

// hubDepsAs is hubDeps with the machine's name read through machine, so a test can stand two clones of one host for two machines.
func hubDepsAs(prime *lyxcwd.Location, machine func() (string, error)) (adoptDeps, error) {
	teardown, err := pairteardown.New(prime)
	if err != nil {
		return adoptDeps{}, err
	}
	geometry, err := hubgeom.PauseGeometry(prime)
	if err != nil {
		return adoptDeps{}, err
	}
	options := pairteardown.HubSeamOptions(prime)
	options.HubPaused = hubOnlyPaused(geometry.RecordPath)
	options.Machine = machine
	seams := teardown.OriginSeams(options)

	config, err := fabricengine.LoadConfig(fabricengine.ConfigDirs(prime))
	if err != nil {
		return adoptDeps{}, err
	}
	topology := fabricengine.NewTopology(config)
	boardDir := fabricengine.BoardDir(prime.HubPath)

	return adoptDeps{
		Machine:    machine,
		ReadOwners: func() (boardengine.Owners, error) { return boardengine.ReadOwners(boardDir) },
		ParentOf: func(slug string) (string, error) {
			return hubgeom.ParentOf(prime, slug)
		},
		Reconcile: func() (fabricengine.OriginReport, error) {
			return fabricengine.ReconcileOrigin(prime, fabricengine.ReconcileScope{}, seams, fabricengine.ReconcileOptions{})
		},
		LocalPair: func(slug string) bool {
			info, err := os.Stat(fabricengine.WorktreePath(prime, slug))
			return err == nil && info.IsDir()
		},
		LivePair: seams.LivePair,
		AddPair: func(slug string) (fabricengine.AddResult, error) {
			return topology.Add(prime, slug, fabricengine.AddOptions{})
		},
		DropLocal: func(rec *fabricengine.Mutations, slug string) (fabricengine.DropLocalResult, error) {
			return fabricengine.DropLocal(rec, prime, slug)
		},
		SetOwners: func(req boardengine.OwnerRequest) (boardengine.OwnerWrite, error) {
			return boardengine.SetOwners(boardDir, req)
		},
		PauseRecord: func() (pauserecord.Record, error) { return pauserecord.Read(geometry.RecordPath) },
		Lift: func(ref tenderengine.EntryRef) error {
			deps, _, err := tendercli.HubDeps(prime, callerLabel)
			if err != nil {
				return err
			}
			_, err = tenderengine.Lift(deps, ref)
			return err
		},
		RunRecord: func(slug string) (shedengine.Status, bool, error) {
			return readStatus(shedrun.StatusFile(prime, slug))
		},
		ResumeRun: func(slug string) (map[string]any, error) {
			return battencli.StartFirstPass(prime, slug)
		},
		ChildStatus: func(slug string) (shedengine.Status, bool, error) {
			pairPath := fabricengine.WorktreePath(prime, slug)
			if _, err := os.Stat(pairPath); errors.Is(err, fs.ErrNotExist) {
				return shedengine.Status{}, false, nil
			}
			pair, err := lyxcwd.ResolveWorktree(pairPath)
			if err != nil {
				return shedengine.Status{}, false, err
			}
			return readStatus(shedrun.StatusFile(pair, shedrun.SelfRunID))
		},
		ResumeChild: func(slug string) error {
			return resumeChild(fabricengine.WorktreePath(prime, slug))
		},
		MoveInProgress: func(worktree string) (string, bool, error) {
			if worktree == "" {
				worktree = prime.WorktreeName
			}
			pair, err := lyxcwd.NamedWorktree(prime.HubPath, worktree)
			if err != nil {
				return "", false, err
			}
			return fabricengine.MoveInProgress(worktree, fabricengine.RecordsWorktreePath(pair, worktree))
		},
		StartDobbies: func() error {
			_, err := dobbyengine.EnsureLive(hubgeom.DobbyStart(prime))
			return err
		},
		StartSuite: func() (suiteStart, error) {
			return startSuite(prime.AnchorPath(), filepath.Join(fabricengine.AdoptScratchDir(prime.HubPath), suiteLogName))
		},
		HubPath: prime.HubPath,
	}, nil
}

// hubOnlyPaused returns a pause reading that covers a slug only by a live hub-scope entry of the record at recordPath.
// A slug-scope entry, an owner pause among them, does not cover its slug, so a pair running under one reads live.
func hubOnlyPaused(recordPath string) func(slug string) (bool, error) {
	return func(string) (bool, error) {
		record, err := pauserecord.Read(recordPath)
		if err != nil {
			return false, err
		}
		for _, entry := range record.Live {
			if entry.Scope == pauserecord.ScopeHub {
				return true, nil
			}
		}
		return false, nil
	}
}

// readStatus reads the status file at path, reporting false when it does not exist.
func readStatus(path string) (shedengine.Status, bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return shedengine.Status{}, false, nil
	}
	if err != nil {
		return shedengine.Status{}, false, err
	}
	status, err := shedengine.ParseStatusTolerant(data)
	if err != nil {
		return shedengine.Status{}, false, fmt.Errorf("decode %s: %w", path, err)
	}
	return status, true, nil
}

// suiteLogName is the file the adopt's test suite writes its output to.
const suiteLogName = "suite.log"

// suiteArgs is the lyx command line of the adopt's test suite: the hub's finalize gate, waiting for gate slots.
var suiteArgs = []string{"gate", "test", "--gate", "finalize", "--wait"}

// startSuite starts the adopt's test suite detached in dir, with its output in the log at logPath, truncated first, and returns its pid and the log.
// The suite is started and never waited for, so the adopt returns while it runs.
// It starts nothing and returns an error in a test binary, where forking the running binary would re-run the test suite.
func startSuite(dir, logPath string) (suiteStart, error) {
	if testing.Testing() {
		return suiteStart{}, errors.New("the test suite is not started under go test")
	}
	executable, err := os.Executable()
	if err != nil {
		return suiteStart{}, fmt.Errorf("locate the lyx executable: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return suiteStart{}, fmt.Errorf("create the suite log directory: %w", err)
	}
	logFile, err := os.Create(logPath)
	if err != nil {
		return suiteStart{}, fmt.Errorf("open the suite log: %w", err)
	}
	defer logFile.Close()

	command := exec.Command(executable, suiteArgs...)
	command.Dir = dir
	command.Stdout = logFile
	command.Stderr = logFile
	proc.Detach(command)
	if err := command.Start(); err != nil {
		logger.Warn("adoptcli: the test suite did not start", "exe", executable, "args", suiteArgs, "dir", dir, "error", err)
		return suiteStart{}, err
	}
	pid := command.Process.Pid
	logger.Info("adoptcli: spawned the test suite", "exe", executable, "args", suiteArgs, "dir", dir, "pid", pid, "log", logPath)
	return suiteStart{PID: pid, Log: logPath}, nil
}

// resumeChild runs "lyx loom resume" in the task worktree at pairPath and waits for it.
func resumeChild(pairPath string) error {
	if testing.Testing() {
		return errors.New("resuming a child spawns the lyx binary, which a test binary must not re-exec")
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	args := []string{"loom", "resume"}
	command := exec.Command(executable, args...)
	command.Dir = pairPath
	logger.Info("adoptcli: spawning a child resume", "exe", executable, "args", args, "dir", pairPath)
	output, err := command.CombinedOutput()
	logger.Info("adoptcli: child resume ended", "dir", pairPath, "error", err)
	if err != nil {
		return fmt.Errorf("lyx loom resume in %s: %w: %s", pairPath, err, output)
	}
	return nil
}
