// deps.go fills adoptDeps over a real hub: the origin reconcile with the live-pair reading adopt needs, the pair add, the owner write, the pause lift, the run resume, the child resume, the beam steps, the landing-hold read and the adopt check start.

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
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/machinename"
	"github.com/Knatte18/loomyard/internal/orchcli"
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
func hubDeps(prime *lymcwd.Location) (adoptDeps, error) {
	return hubDepsAs(prime, machinename.Name)
}

// hubDepsAs is hubDeps with the machine's name read through machine, so a test can stand two clones of one host for two machines.
func hubDepsAs(prime *lymcwd.Location, machine func() (string, error)) (adoptDeps, error) {
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
			return battencli.StartFirstPass(prime, slug, battencli.FirstPassOptions{})
		},
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
		ResumeChild: func(slug string) error {
			return resumeChild(fabricengine.WorktreePath(prime, slug))
		},
		MoveInProgress: func(worktree string) (string, bool, error) {
			if worktree == "" {
				worktree = prime.WorktreeName
			}
			pair, err := lymcwd.NamedWorktree(prime.HubPath, worktree)
			if err != nil {
				return "", false, err
			}
			return fabricengine.MoveInProgress(worktree, fabricengine.RecordsWorktreePath(pair, worktree))
		},
		BeamUp: func(worktree string) (orchcli.BeamUpReport, error) {
			if testing.Testing() {
				return orchcli.BeamUpReport{}, errors.New("the beam steps start a tmux session and detached processes, which a test binary must not")
			}
			location, err := lymcwd.ResolveWorktree(fabricengine.WorktreePath(prime, worktree))
			if err != nil {
				return orchcli.BeamUpReport{}, err
			}
			here, err := machine()
			if err != nil {
				return orchcli.BeamUpReport{}, err
			}
			return orchcli.BeamUp(location, orchcli.BeamUpOptions{Here: here})
		},
		LandingHeld: func(slug string) (bool, error) {
			// ReadOwners carries no run status, so the entry is read whole and judged by RunHolds.
			tasks, err := boardengine.ReadTasks(boardDir)
			if err != nil {
				return false, err
			}
			for _, task := range tasks {
				if task.Slug == slug {
					return boardengine.RunHolds(task), nil
				}
			}
			return false, nil
		},
		PrimeName: prime.WorktreeName,
		StartSuite: func() (suiteStart, error) {
			dir := fabricengine.AdoptScratchDir(prime.HubPath)
			return ensureCheck(dir, checkAlive, func() (int, error) {
				return startSuite(prime.AnchorPath(), filepath.Join(dir, checkOutName))
			})
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

// suiteLogName is the file the adopt check writes its output to.
const suiteLogName = "suite.log"

// suiteArgs is the lym command line of the adopt check: the hidden check, which runs the hub's adopt gate.
var suiteArgs = []string{"adopt", "check"}

// startSuite starts the adopt check detached in dir, with its stdout and stderr appended to the file at outPath, and returns its pid.
// The check is started and never waited for, so the adopt returns while it runs; the file holds a losing check's envelope and a panic, and the gate's own output goes to the suite log the check truncates.
// It starts nothing and returns an error in a test binary, where forking the running binary would re-run the test binary.
func startSuite(dir, outPath string) (int, error) {
	if testing.Testing() {
		return 0, errors.New("the adopt check is not started under go test")
	}
	executable, err := os.Executable()
	if err != nil {
		return 0, fmt.Errorf("locate the lym executable: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return 0, fmt.Errorf("create the adopt check directory: %w", err)
	}
	outFile, err := os.OpenFile(outPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return 0, fmt.Errorf("open the adopt check output: %w", err)
	}
	defer outFile.Close()

	command := exec.Command(executable, suiteArgs...)
	command.Dir = dir
	command.Stdout = outFile
	command.Stderr = outFile
	proc.Detach(command)
	if err := command.Start(); err != nil {
		logger.Warn("adoptcli: the adopt check did not start", "exe", executable, "args", suiteArgs, "dir", dir, "error", err)
		return 0, err
	}
	pid := command.Process.Pid
	logger.Info("adoptcli: spawned the adopt check", "exe", executable, "args", suiteArgs, "dir", dir, "pid", pid, "out", outPath)
	return pid, nil
}

// checkAlive reports whether the process that wrote record is still running: its pid is alive and, when both sides can read it, the live process started when the record says.
// A record without start ticks, or a platform that cannot read a start time, reads alive on the pid alone; a reused pid after a reboot reads dead.
func checkAlive(record CheckRecord) bool {
	if !proc.IsAlive(record.PID) {
		return false
	}
	if record.StartTicks == "" {
		return true
	}
	ticks, readable := proc.StartTime(record.PID)
	return !readable || ticks == record.StartTicks
}

// ensureCheck returns the adopt check of the scratch directory dir: the running one when check.json reads running and alive passes, else a new one started by spawn.
// The returned suiteStart names the check's pid and the paths of its log and record.
func ensureCheck(dir string, alive func(CheckRecord) bool, spawn func() (int, error)) (suiteStart, error) {
	start := suiteStart{Log: filepath.Join(dir, suiteLogName), Record: filepath.Join(dir, checkRecordName)}
	record, found, err := ReadCheck(start.Record)
	if err != nil {
		return suiteStart{}, err
	}
	if found && record.State == checkRunning && alive(record) {
		start.PID = record.PID
		return start, nil
	}
	pid, err := spawn()
	if err != nil {
		return suiteStart{}, err
	}
	start.PID = pid
	return start, nil
}

// resumeChild runs "lym loom resume" in the task worktree at pairPath and waits for it.
func resumeChild(pairPath string) error {
	if testing.Testing() {
		return errors.New("resuming a child spawns the lym binary, which a test binary must not re-exec")
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
		return fmt.Errorf("lym loom resume in %s: %w: %s", pairPath, err, output)
	}
	return nil
}
