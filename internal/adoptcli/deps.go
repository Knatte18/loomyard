// deps.go fills adoptDeps over a real hub.
// It covers the Bolt pull, the adopt write, the pair add and the takeover of a run.
// It also covers the catch-up, the deploy, the workspace, the adopt check, the beam steps and the landing-hold read.

package adoptcli

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/battencli"
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/dobbyengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/machinename"
	"github.com/Knatte18/loomyard/internal/orchcli"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// hubDepsAs fills adoptDeps over the hub the prime belongs to, with the machine's name read through machine,
// so a test can stand two clones of one host for two machines.
// The identity carries the machine file's id only while machine names the machine's own name.
// Its HoldCheck runs the adopt check with the gate's output on the terminal and brings no subject up,
// and its BeamUp refuses under a test binary, which must start no session or process.
func hubDepsAs(prime *lymcwd.Location, machine func() (string, error)) (adoptDeps, error) {
	identity := func() (machinename.Identity, error) {
		name, err := machine()
		if err != nil {
			return machinename.Identity{}, err
		}
		own, err := machinename.ReadIdentity()
		if err != nil {
			return machinename.Identity{}, err
		}
		if own.Name != name {
			return machinename.Identity{Name: name}, nil
		}
		return own, nil
	}

	config, err := fabricengine.LoadConfig(fabricengine.ConfigDirs(prime))
	if err != nil {
		return adoptDeps{}, err
	}
	topology := fabricengine.NewTopology(config)
	boardDir := fabricengine.BoardDir(prime.HubPath)

	return adoptDeps{
		Machine:    machine,
		Identity:   identity,
		ReadOwners: func() (boardengine.Owners, error) { return boardengine.ReadOwners(boardDir) },
		PullBolt:   func() error { return boardengine.PullBolt(boardDir) },
		ParentOf: func(slug string) (string, error) {
			return hubgeom.ParentOf(prime, slug)
		},
		RunRecord: func(slug string) (shedengine.Status, bool, error) {
			return readStatus(shedrun.StatusFile(prime, slug))
		},
		OriginRun: func(slug, parent string) (shedengine.Status, bool, error) {
			return fabricengine.OriginParentRunStatus(prime, slug, parent)
		},
		LocalPair: func(slug string) bool {
			info, err := os.Stat(fabricengine.WorktreePath(prime, slug))
			return err == nil && info.IsDir()
		},
		AddPair: func(slug string) (fabricengine.AddResult, error) {
			return topology.Add(prime, slug, fabricengine.AddOptions{})
		},
		PickAccount: func(slug string) { battencli.PickWorktreeAccount(prime, slug, false) },
		WriteAdopt: func(req boardengine.AdoptRequest) (boardengine.AdoptWrite, error) {
			own, err := boardengine.EnsureIdentity()
			if err != nil {
				return boardengine.AdoptWrite{}, err
			}
			// A test stands another name for the machine through the machine seam, and only the machine's own name registers it.
			if req.Machine == own.Name {
				req.Identity = own
			}
			return boardengine.WriteAdopt(boardDir, req)
		},
		TakeOver: func(slug, subject string) (fabricengine.TakeOverResult, error) {
			return takeOverRun(prime, machine, slug, subject)
		},
		CatchUp: func() error {
			_, err := dobbyengine.CatchUp(battencli.HubCatchUp(prime, func() ([]string, error) { return battencli.HubRunSlugs(prime) }))
			return err
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
		DeployState: func() (deployState, error) { return readDeployState(prime, runningLymBuild) },
		RunDeploy:   func(command string) (string, error) { return runAdoptDeploy(prime, command) },
		Workspace: func() error {
			_, err := fabricengine.RegenerateHubWorkspace(fabricengine.NewMutations(prime.HubPath), prime)
			return err
		},
		HoldCheck: func(first string, noOrch bool) (holdCheckResult, error) {
			check, err := hubHoldCheckDeps(prime, os.Stdout, noOrch)
			if err != nil {
				return holdCheckResult{}, err
			}
			check.BeamUp = nil
			return runHoldCheck(context.Background(), check, first)
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
		PrimeName: prime.WorktreeName,
		HubPath:   prime.HubPath,
		Now:       time.Now,
	}, nil
}

// takeOverRun moves the run slug, with subject the adopt subject that moves it, to the machine named by machine.
// It resolves the run's pair and its parent's pair, `nil` for the prime, and calls fabric's takeover.
func takeOverRun(prime *lymcwd.Location, machine func() (string, error), slug, subject string) (fabricengine.TakeOverResult, error) {
	name, err := machine()
	if err != nil {
		return fabricengine.TakeOverResult{}, err
	}
	pair, err := lymcwd.NamedWorktree(prime.HubPath, slug)
	if err != nil {
		return fabricengine.TakeOverResult{}, err
	}
	parentSlug, err := hubgeom.ParentOf(prime, slug)
	if err != nil {
		return fabricengine.TakeOverResult{}, err
	}
	var parent *lymcwd.Location
	if parentSlug != "" {
		if parent, err = lymcwd.NamedWorktree(prime.HubPath, parentSlug); err != nil {
			return fabricengine.TakeOverResult{}, err
		}
	}
	return fabricengine.TakeOverRun(prime, fabricengine.TakeOverRequest{
		Slug: slug, Subject: subject, Machine: name, Pair: pair, Parent: parent, At: time.Now(),
	})
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
