// deps.go fills adoptDeps over a real hub.
// It covers the origin reconcile, the pair add, the owner write, and the adopt pause.
// It also covers the tries file, the deploy, the beam steps, and the landing-hold read.
// The adopt check start and the reads behind the run reports make up the rest.

package adoptcli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/orchcli"
	"github.com/Knatte18/loomyard/internal/pairteardown"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/selfexec"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// hubDepsAs fills adoptDeps over the hub the prime belongs to, with the machine's name read through machine,
// so a test can stand two clones of one host for two machines.
// The reconcile's live-pair reading treats a slug as paused only under a live hub-scope pause,
// so a pair whose strands are alive under a slug-scope pause, an owner pause included, reads live.
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
		RunRecord: func(slug string) (shedengine.Status, bool, error) {
			return readStatus(shedrun.StatusFile(prime, slug))
		},
		Tasks: func() ([]boardengine.Task, error) { return boardengine.ReadTasks(boardDir) },
		UpdatePause: func(mutate func(record *pauserecord.Record, now time.Time) error) error {
			now := time.Now()
			_, err := pauserecord.Update(geometry.RecordPath, now, func(record *pauserecord.Record) error { return mutate(record, now) })
			return err
		},
		ResetTries: func(subject string) error { return writeTries(subjectDir(prime.HubPath, subject), 0) },
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
		BeamUp: func(worktree string, noOrch bool) (orchcli.BeamUpReport, error) {
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
			return orchcli.BeamUp(location, orchcli.BeamUpOptions{Here: here, SkipOrch: noOrch})
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
		PrimeName:          prime.WorktreeName,
		EscalationEligible: hubEscalationEligible(prime),
		DeployState:        func() (deployState, error) { return readDeployState(prime, runningLymBuild) },
		RunDeploy:          func(command string) (string, error) { return runAdoptDeploy(prime, command) },
		StartSuite: func(subject string) (suiteStart, error) {
			dir := subjectDir(prime.HubPath, subject)
			resolve := func() error {
				_, err := resolveSubjectWorktree(prime, subject)
				return err
			}
			start, err := startCheckWithTiers(prime, dir, resolve, checkAlive, func() (int, error) {
				return startSuite(prime.AnchorPath(), filepath.Join(dir, checkOutName), subject)
			})
			start.Subject = subject
			return start, err
		},
		HubPath: prime.HubPath,
	}, nil
}

// hubArchiveDeps fills archiveDeps over the hub the prime belongs to.
// The archives are read through fabric, with `root` naming the prime's pair.
func hubArchiveDeps(prime *lymcwd.Location) (archiveDeps, error) {
	config, err := fabricengine.LoadConfig(fabricengine.ConfigDirs(prime))
	if err != nil {
		return archiveDeps{}, err
	}
	topology := fabricengine.NewTopology(config)
	boardDir := fabricengine.BoardDir(prime.HubPath)
	return archiveDeps{
		ReadOwners: func() (boardengine.Owners, error) { return boardengine.ReadOwners(boardDir) },
		ParentOf:   func(slug string) (string, error) { return hubgeom.ParentOf(prime, slug) },
		ReadArchives: func(keep func(slug string) bool) ([]fabricengine.OwnerArchive, error) {
			return topology.ReadOwnerArchives(prime, boardengine.RootSubject, keep)
		},
	}, nil
}

// hubOnlyPaused returns a pause reading that covers a slug only by a live hub-scope entry of the record at recordPath.
// A slug-scope entry, an owner pause among them, does not cover its slug,
// so a pair running under one reads live.
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

// suiteLogName is the file the adopt check's gate writes its output to.
const suiteLogName = "suite.log"

// suiteArgs returns the lym command line of the adopt check of subject.
// The hidden check runs the hub's adopt gate in the subject's worktree.
func suiteArgs(subject string) []string {
	return []string{"adopt", "check", subject}
}

// startSuite starts the adopt check of subject detached in dir and returns its pid.
// Its stdout and stderr are appended to the file at outPath.
// The check is reaped in the background through `proc.Reap` rather than waited for,
// so the adopt returns while it runs.
// The file holds a losing check's envelope and a panic,
// and the gate's own output goes to the suite log the check truncates.
// In a test binary it starts nothing and returns an error.
// Forking the running binary there would re-run the test binary.
func startSuite(dir, outPath, subject string) (int, error) {
	if testing.Testing() {
		return 0, errors.New("the adopt check is not started under go test")
	}
	executable, err := selfexec.LymBinary()
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

	args := suiteArgs(subject)
	command := exec.Command(executable, args...)
	command.Dir = dir
	command.Stdout = outFile
	command.Stderr = outFile
	proc.Detach(command)
	if err := command.Start(); err != nil {
		logger.Warn("adoptcli: the adopt check did not start", "exe", executable, "args", args, "dir", dir, "error", err)
		return 0, err
	}
	proc.Reap(command)
	pid := command.Process.Pid
	logger.Info("adoptcli: spawned the adopt check", "exe", executable, "args", args, "dir", dir, "pid", pid, "out", outPath)
	return pid, nil
}

// startCheckWithTiers returns the adopt check of the scratch directory dir: the running one, or a new one.
// It sets the tiers of the prime's adopt gate key.
// resolve runs first and reports whether the subject's worktree resolves.
// When it fails, nothing is spawned and its error is returned,
// so a check that could never write a record is never reported started.
func startCheckWithTiers(prime *lymcwd.Location, dir string, resolve func() error, alive func(CheckRecord) bool, spawn func() (int, error)) (suiteStart, error) {
	if err := resolve(); err != nil {
		return suiteStart{}, err
	}
	start, err := ensureCheck(dir, alive, spawn)
	if err != nil {
		return suiteStart{}, err
	}
	start.Tiers = adoptGateTiers(prime)
	return start, nil
}

// adoptGateTiers returns the tiers of the hub's adopt gate key as the prime's gate config reads at the call.
// It returns none when the config does not read, or when the adopt key names no tier the hub declares;
// the check's own reconcile may still repair that.
func adoptGateTiers(prime *lymcwd.Location) []int {
	cfg, err := gateslot.LoadConfig(fabricengine.ConfigDirs(prime))
	if err != nil {
		logger.Warn("adoptcli: the gate config did not read for the adopt check's tiers", "error", err)
		return nil
	}
	tiers, err := cfg.TierList(checkGateKey)
	if err != nil {
		logger.Warn("adoptcli: the gate config names no adopt tiers yet", "error", err)
		return nil
	}
	return tiers
}

// checkAlive reports whether the process that wrote record is still running.
// Its pid is alive, and when both sides can read it, the live process started when the record says.
// A record without start ticks, or a platform that cannot read a start time, reads alive on the pid alone;
// a reused pid after a reboot reads dead.
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

// ensureCheck returns the adopt check of the scratch directory dir.
// It is the running check when check.json records it as running and alive confirms it;
// otherwise spawn starts a new one.
// The returned suiteStart holds the check's pid, the paths of its log and record, and whether it was found running.
func ensureCheck(dir string, alive func(CheckRecord) bool, spawn func() (int, error)) (suiteStart, error) {
	start := suiteStart{Log: filepath.Join(dir, suiteLogName), Record: filepath.Join(dir, checkRecordName)}
	record, found, err := ReadCheck(start.Record)
	if err != nil {
		return suiteStart{}, err
	}
	if found && record.State == checkRunning && alive(record) {
		start.PID = record.PID
		start.AlreadyRunning = true
		return start, nil
	}
	pid, err := spawn()
	if err != nil {
		return suiteStart{}, err
	}
	start.PID = pid
	return start, nil
}
