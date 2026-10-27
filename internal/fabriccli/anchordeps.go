// anchordeps.go fills the verb's anchorDeps over a hub: the board's owners, the reed, run, gate and hibernate readers, the parent lock, the dobby end and start, and the engine's change.
// Every dobby, reed and run reader goes through the same internal package the rest of lym reads that state with.

package fabriccli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/dobbyengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/machinename"
	"github.com/Knatte18/loomyard/internal/parentlock"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/runproc"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// anchorLockOwnerRunID is the run id of the parent lock owner the verb takes the lock as.
const anchorLockOwnerRunID = "fabric-anchor"

// hibernateRequestFileName is the hibernate request file in the hub's hibernate directory.
// This package cannot import the hibernate packages: their import chain leads back to it through a test fixture.
const hibernateRequestFileName = "request.json"

// hibernateDaemonLive reports whether the hub's hibernate request names a daemon that is still running.
// A hub with no request file has none.
func hibernateDaemonLive(hub string) (bool, error) {
	path := filepath.Join(fabricengine.HubScratchDir(hub), lymdirs.HibernateDirName, hibernateRequestFileName)
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read the hibernate request: %w", err)
	}
	var request struct {
		DaemonPID   int    `json:"daemon_pid"`
		DaemonStart string `json:"daemon_start"`
	}
	if err := json.Unmarshal(data, &request); err != nil {
		return false, fmt.Errorf("decode the hibernate request: %w", err)
	}
	return request.DaemonPID != 0 && parentlock.ProcessAlive(request.DaemonPID, request.DaemonStart), nil
}

// hubAnchorDeps fills anchorDeps over the hub of acting.
// The prime is resolved again at each call that needs it, since a move of the prime changes the anchor its geometry is told.
func hubAnchorDeps(acting *lymcwd.Location) (anchorDeps, error) {
	primeName, err := fabricengine.PrimeName(acting)
	if err != nil {
		return anchorDeps{}, err
	}
	hub := acting.HubPath
	boardDir := fabricengine.BoardDir(hub)
	runProcDir := fabricengine.RunProcDir(hub)
	prime := func() (*lymcwd.Location, error) { return lymcwd.NamedWorktree(hub, primeName) }
	// dobbyOwner is the owner a worktree's dobby registers its process under.
	dobbyOwner := func(worktree string) string {
		if worktree == primeName {
			return runproc.HubOwner
		}
		return worktree
	}

	return anchorDeps{
		GOOS:   runtime.GOOS,
		Acting: acting.WorktreeName,
		Prime:  primeName,
		Worktree: func(slug string) (*lymcwd.Location, bool, error) {
			if slug != primeName {
				present, err := pairPresent(acting, slug)
				if err != nil || !present {
					return nil, false, err
				}
			}
			location, err := lymcwd.NamedWorktree(hub, slug)
			return location, err == nil, err
		},
		IsBeam: func(slug string) (bool, error) {
			origin, found, err := fabricengine.ReadOriginFor(acting, slug)
			return found && origin.Beam, err
		},
		LocalBeams: func() ([]string, error) {
			primeLocation, err := prime()
			if err != nil {
				return nil, err
			}
			beams, err := hubgeom.DobbyStart(primeLocation).Beams()
			slugs := make([]string, len(beams))
			for i, beam := range beams {
				slugs[i] = beam.Slug
			}
			return slugs, err
		},
		ForeignOwner: func(slug string) (string, error) {
			primeLocation, err := prime()
			if err != nil {
				return "", err
			}
			return foreignOwner(primeLocation, boardDir, slug)
		},
		AdoptSubject: func(slug string) (string, error) {
			primeLocation, err := prime()
			if err != nil {
				return "", err
			}
			owners, err := boardengine.ReadOwners(boardDir)
			if err != nil {
				return "", err
			}
			return owners.AdoptSubject(slug, func(entry string) (string, error) { return hubgeom.ParentOf(primeLocation, entry) })
		},
		UpstreamAnchor: func(location *lymcwd.Location) (string, bool, error) {
			names, err := fabricengine.RepoWiredNames(location)
			if err != nil {
				return "", false, err
			}
			return fabricengine.UpstreamAnchor(location, names)
		},
		MoveRecord: func(location *lymcwd.Location) (fabricengine.MoveRecord, bool, error) {
			return fabricengine.ReadMoveRecord(fabricengine.RecordsWorktreePath(location, location.WorktreeName))
		},
		ReedHeld: reedHeld,
		DriverLockHeld: func(location *lymcwd.Location) (bool, error) {
			return lockHeld(shedrun.DriverLock(location, shedrun.SelfRunID))
		},
		BattenLive: func(location *lymcwd.Location) (bool, error) {
			records, err := runproc.List(runProcDir, location.WorktreeName)
			return hasProcessRole(records, runproc.RoleBatten), err
		},
		RunUnfinished: func(location *lymcwd.Location) (bool, error) {
			return runStatusUnfinished(shedrun.StatusFile(location, shedrun.SelfRunID))
		},
		UpstreamRunUnfinished: func(location *lymcwd.Location) (bool, error) {
			names, err := fabricengine.RepoWiredNames(location)
			if err != nil {
				return false, err
			}
			return fabricengine.UpstreamRunUnfinished(location, names)
		},
		PrimeRunUnfinished: func(slug string) (bool, error) {
			primeLocation, err := prime()
			if err != nil {
				return false, err
			}
			return runStatusUnfinished(shedrun.StatusFile(primeLocation, slug))
		},
		DobbyPID: func(worktree string) (int, bool, error) {
			records, err := runproc.List(runProcDir, dobbyOwner(worktree))
			if err != nil {
				return 0, false, err
			}
			for _, record := range records {
				if record.Role == runproc.RoleDobby {
					return record.PID, true, nil
				}
			}
			return 0, false, nil
		},
		HubReedHeld: func() ([]string, error) {
			worktrees, err := hubWorktrees(acting)
			if err != nil {
				return nil, err
			}
			var held []string
			for _, worktree := range worktrees {
				location, err := lymcwd.NamedWorktree(hub, worktree)
				if err != nil {
					return nil, err
				}
				if reed, err := reedHeld(location); err != nil {
					return nil, err
				} else if reed {
					held = append(held, worktree)
				}
			}
			return held, nil
		},
		HubBattenLive: func() ([]string, error) {
			owners, err := runproc.Owners(runProcDir)
			if err != nil {
				return nil, err
			}
			var runs []string
			for owner, records := range owners {
				if hasProcessRole(records, runproc.RoleBatten) {
					runs = append(runs, owner)
				}
			}
			sort.Strings(runs)
			return runs, nil
		},
		GateHolders: func() ([]string, error) {
			primeLocation, err := prime()
			if err != nil {
				return nil, err
			}
			holders, err := hubgeom.GateSlots(primeLocation).Holders()
			described := make([]string, len(holders))
			for i, holder := range holders {
				described[i] = fmt.Sprintf("slot %d (%s, %s, process %d)", holder.Slot, holder.Worktree, holder.Site, holder.PID)
			}
			return described, err
		},
		HibernateLive: func() (bool, error) { return hibernateDaemonLive(hub) },
		PassInFlight: func(worktree string) (string, error) {
			return dobbyPassInFlight(acting, primeName, runProcDir, dobbyOwner(worktree), worktree)
		},
		Sleep: func(ctx context.Context, d time.Duration) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(d):
				return nil
			}
		},
		Note: func(line string) { fmt.Fprintln(os.Stderr, "lym fabric anchor: "+line) },
		AcquireParentLock: func(ctx context.Context, location *lymcwd.Location) (func(), error) {
			return acquireBranchLock(ctx, location, boardDir)
		},
		EndDobbies: func(worktrees []string, spare ...string) ([]string, error) {
			var ended []string
			for _, worktree := range worktrees {
				processes, err := runproc.EndSparing(runProcDir, dobbyOwner(worktree), os.Getpid(), spare...)
				for _, process := range processes {
					if process.Role == runproc.RoleDobby {
						ended = append(ended, worktree)
						break
					}
				}
				if err != nil {
					return ended, fmt.Errorf("end the processes of the dobby of %s: %w", worktree, err)
				}
			}
			return ended, nil
		},
		EnsureDobbies: func(worktrees []string) error {
			primeLocation, err := prime()
			if err != nil {
				return err
			}
			_, err = dobbyengine.EnsureLive(hubgeom.DobbyStart(primeLocation), worktrees)
			return err
		},
		Begin: func(location *lymcwd.Location, target string) (anchorChange, error) {
			cfg, err := fabricengine.LoadConfig(fabricengine.ConfigDirs(location))
			if err != nil {
				return nil, err
			}
			change, err := fabricengine.NewTopology(cfg).BeginAnchorChange(location, target)
			if err != nil {
				return nil, err
			}
			return change, nil
		},
		ReopenLog: func(dir string) error {
			logger.SetDurableSinkDirWithWorktreeRoot(dir, acting.WorktreePath())
			return nil
		},
	}, nil
}

// pairPresent reports whether the hub holds a pair called slug on this machine: a registered, unpruned worktree whose directory exists.
func pairPresent(acting *lymcwd.Location, slug string) (bool, error) {
	worktrees, err := hubWorktrees(acting)
	if err != nil {
		return false, err
	}
	return slices.Contains(worktrees, slug), nil
}

// hubWorktrees lists the names of the hub's warp worktrees that are registered, unpruned and present on disk, the prime included.
func hubWorktrees(acting *lymcwd.Location) ([]string, error) {
	entries, err := fabricengine.List(acting.AnchorPath())
	if err != nil {
		return nil, err
	}
	var names []string
	for _, entry := range entries {
		if entry.Prunable {
			continue
		}
		path := filepath.Clean(filepath.FromSlash(entry.Path))
		if _, err := os.Stat(path); err != nil {
			continue
		}
		names = append(names, filepath.Base(path))
	}
	return names, nil
}

// foreignOwner returns the machine other than this one that the board at boardDir names as the effective owner of slug, empty when none does.
// The prime reads as the board's prime owner.
func foreignOwner(prime *lymcwd.Location, boardDir, slug string) (string, error) {
	machine, err := machinename.Name()
	if err != nil {
		return "", fmt.Errorf("this machine's name is unknown: %w", err)
	}
	owners, err := boardengine.ReadOwners(boardDir)
	if err != nil {
		return "", err
	}
	owner := owners.Prime
	if slug != prime.WorktreeName {
		owner, err = owners.Effective(slug, func(entry string) (string, error) { return hubgeom.ParentOf(prime, entry) })
		if err != nil {
			return "", err
		}
	}
	if owner == machine {
		return "", nil
	}
	return owner, nil
}

// reedHeld reports whether the reed state under location's anchored `.lym` holds a session or a strand, live or stored.
// A worktree with no `.lym` directory holds none, and the read creates none.
func reedHeld(location *lymcwd.Location) (bool, error) {
	dotLym := filepath.Join(fabricengine.WorktreePath(location, location.WorktreeName), location.AnchorRel, lymdirs.DotLymDirName)
	if _, err := os.Stat(dotLym); errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	state, err := reedengine.LoadState(dotLym)
	if err != nil || state == nil {
		return false, err
	}
	return state.Session != "" || len(state.Strands) > 0, nil
}

// lockHeld reports whether the lock file at path is held, taking it once and releasing it at once when it is free.
// A lock whose directory does not exist is not held, and the probe creates no directory.
func lockHeld(path string) (bool, error) {
	if _, err := os.Stat(filepath.Dir(path)); errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	held, acquired, err := lock.TryAcquireWriteLock(path)
	if err != nil {
		return false, err
	}
	if !acquired {
		return true, nil
	}
	return false, held.Release()
}

// hasProcessRole reports whether any of records plays role.
func hasProcessRole(records []runproc.Record, role string) bool {
	return slices.ContainsFunc(records, func(record runproc.Record) bool { return record.Role == role })
}

// runStatusUnfinished reports whether the run status file at path names a state other than done; an absent file names none.
func runStatusUnfinished(path string) (bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var status shedengine.Status
	if err := json.Unmarshal(data, &status); err != nil {
		return false, fmt.Errorf("read the run status %s: %w", path, err)
	}
	return status.State != shedengine.StateDone, nil
}

// dobbyPassInFlight describes the pass of the dobby of worktree that is in flight, empty when none is.
// A pass is in flight while a run the dobby drives holds its driver lock, or while the dobby has a live child process other than a registered push or board sync, which a spawned pass not yet holding its lock is.
func dobbyPassInFlight(acting *lymcwd.Location, primeName, runProcDir, owner, worktree string) (string, error) {
	records, err := runproc.List(runProcDir, owner)
	if err != nil {
		return "", err
	}
	var dobbyPIDs []int
	for _, record := range records {
		if record.Role == runproc.RoleDobby {
			dobbyPIDs = append(dobbyPIDs, record.PID)
		}
	}
	if len(dobbyPIDs) == 0 {
		return "", nil
	}

	owners, err := runproc.Owners(runProcDir)
	if err != nil {
		return "", err
	}
	spared := map[int]bool{}
	for _, registered := range owners {
		for _, record := range registered {
			if record.Role == runproc.RolePush || record.Role == runproc.RoleSync {
				spared[record.PID] = true
			}
		}
	}
	stats, err := proc.ListStats()
	if err != nil {
		return "", err
	}
	for _, stat := range stats {
		if !stat.Zombie && !spared[stat.PID] && slices.Contains(dobbyPIDs, stat.PPID) {
			return fmt.Sprintf("a pass of the dobby of %s (process %d)", worktree, stat.PID), nil
		}
	}

	prime, err := lymcwd.NamedWorktree(acting.HubPath, primeName)
	if err != nil {
		return "", err
	}
	pairs, err := hubWorktrees(acting)
	if err != nil {
		return "", err
	}
	for _, pair := range pairs {
		if pair == primeName {
			continue
		}
		parent, err := hubgeom.ParentOf(prime, pair)
		if err != nil {
			return "", err
		}
		if parent == "" {
			parent = primeName
		}
		if parent != worktree {
			continue
		}
		location, err := lymcwd.NamedWorktree(acting.HubPath, pair)
		if err != nil {
			return "", err
		}
		if held, err := lockHeld(shedrun.DriverLock(location, shedrun.SelfRunID)); err != nil {
			return "", err
		} else if held {
			return fmt.Sprintf("the run of %s, which the dobby of %s drives", pair, worktree), nil
		}
	}
	return "", nil
}

// acquireBranchLock takes the parent lock of the branch checked out in location's code worktree as the calling process, queuing behind a holder, and returns the release.
func acquireBranchLock(ctx context.Context, location *lymcwd.Location, boardDir string) (func(), error) {
	branch, err := gitrepo.New(fabricengine.WorktreePath(location, location.WorktreeName)).CurrentBranch()
	if err != nil {
		return nil, fmt.Errorf("read the branch of %s to lock it: %w", location.WorktreeName, err)
	}
	store := &parentlock.Store{Dir: parentlock.Dir(boardDir)}
	startTime, _ := proc.StartTime(os.Getpid())
	owner := parentlock.Owner{
		Slug:      location.WorktreeName,
		Pair:      location.WorktreePath(),
		RunID:     anchorLockOwnerRunID,
		PID:       os.Getpid(),
		StartTime: startTime,
	}
	waiter := parentlock.Waiter{OnWait: func(holder parentlock.Record) {
		fmt.Fprintf(os.Stderr, "lym fabric anchor: waiting for the parent lock of %s, held by %s\n", branch, holder.Owner.Slug)
	}}
	if _, err := store.Acquire(ctx, branch, owner, waiter); err != nil {
		return nil, fmt.Errorf("take the parent lock of %s: %w", branch, err)
	}
	return func() {
		if _, err := store.Release(branch, owner); err != nil {
			logger.Warn("fabriccli: the parent lock of an anchor change is not released", "branch", branch, "error", err)
		}
	}, nil
}
