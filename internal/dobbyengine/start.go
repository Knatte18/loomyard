// start.go starts the dobby of every worktree this machine owns: the claim of the prime's owner, the liveness read of a dobby's lock, and the detached spawn of one dobby process.

package dobbyengine

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/proc"
)

const (
	// LogFileName is the dobby log file in the worktree's `.lym/beam/` directory, which holds the dobby's own lines only.
	LogFileName = "dobby.log"
	// OutFileName is the file in the worktree's `.lym/beam/` directory that a spawned dobby's raw standard output and error go to.
	OutFileName = "dobby.out"
	// PrimeLabel is the worktree label a StartReport gives the prime's dobby.
	PrimeLabel = "prime"
	// OutcomeStarted is the outcome of a dobby EnsureLive spawned.
	OutcomeStarted = "started"
	// OutcomeLive is the outcome of a dobby that was live already.
	OutcomeLive = "live"
	// OutcomeRefused is the outcome of a dobby EnsureLive did not start, with the reason in the entry's detail.
	OutcomeRefused = "refused"
	// OutcomeFailed is the outcome of a dobby whose start failed, with the cause in the entry's detail.
	OutcomeFailed = "failed"
)

// ErrSpawnUnderTest is returned by SpawnDetached in a test binary, where forking the running binary would re-run the test suite.
var ErrSpawnUnderTest = errors.New("dobbyengine: dobby not spawned under test")

// BeamPair is a local beam pair and the owner it reads as.
type BeamPair struct {
	// Slug is the beam's slug, the name of its worktree.
	Slug string
	// Owner is the machine the pair reads as owned by; empty means no other machine owns it.
	Owner string
}

// StartDeps is every seam EnsureLive reads and writes through.
type StartDeps struct {
	// Machine returns this machine's name.
	Machine func() (string, error)
	// PrimeOwner returns the machine recorded as the prime's owner, empty when none is recorded.
	PrimeOwner func() (string, error)
	// ClaimPrime records machine as the prime's owner and returns the machine that owns it instead when that claim lost, empty otherwise.
	ClaimPrime func(machine string) (lostTo string, err error)
	// PrimeName is the prime worktree's directory name.
	PrimeName string
	// Beams lists the local beam pairs with the owner each reads as.
	Beams func() ([]BeamPair, error)
	// Geometry tells the paths of the dobby of the worktree named worktree.
	Geometry func(worktree string) (Geometry, error)
	// MoveInProgress reports whether an anchor change stands on the worktree named worktree or on the prime, with the way forward that settles it.
	// A nil MoveInProgress reads as no change standing.
	MoveInProgress func(worktree string) (wayForward string, standing bool, err error)
	// LandingHeld reports whether the landing hold of the worktree named worktree stands, which keeps its dobby unstarted.
	// A nil LandingHeld reads as no hold standing.
	LandingHeld func(worktree string) (held bool, err error)
	// Live reports whether the dobby described by geom is running.
	Live func(geom Geometry) (bool, error)
	// Spawn starts the dobby described by geom as a detached process.
	Spawn func(geom Geometry) error
}

// StartEntry is the outcome of EnsureLive for one worktree.
type StartEntry struct {
	// Worktree is PrimeLabel for the prime, else the beam's slug.
	Worktree string
	// Outcome is one of the Outcome constants.
	Outcome string
	// Detail says why a dobby was refused or failed, and what to do next; empty for started and live.
	Detail string
}

// StartReport is the outcome of one EnsureLive call, one entry per worktree it considered, the prime first.
type StartReport struct {
	Entries []StartEntry
}

// OwnedWorktrees lists the worktrees this machine may start a dobby for: the prime's name when this machine owns the prime or none is recorded, then every local beam pair not owned by another machine.
func OwnedWorktrees(deps StartDeps) ([]string, error) {
	machine, err := deps.Machine()
	if err != nil {
		return nil, fmt.Errorf("dobbyengine: this machine's name is unknown: %w", err)
	}
	primeOwner, err := deps.PrimeOwner()
	if err != nil {
		return nil, fmt.Errorf("dobbyengine: read the prime owner: %w", err)
	}
	beams, err := deps.Beams()
	if err != nil {
		return nil, fmt.Errorf("dobbyengine: list the local beams: %w", err)
	}
	var worktrees []string
	if primeOwner == "" || primeOwner == machine {
		worktrees = append(worktrees, deps.PrimeName)
	}
	for _, beam := range beams {
		if beam.Owner == "" || beam.Owner == machine {
			worktrees = append(worktrees, beam.Slug)
		}
	}
	return worktrees, nil
}

// EnsureLive starts the dobby of each named worktree, the prime's name or a beam's slug, each only when it is not live already.
// The prime is handled first when named; a prime with no recorded owner is claimed for this machine first, and a claim that fails is reported for the prime with its cause and the `lym beam start` re-run, the prime's dobby is not started, and the beams go on.
// A named prime another machine owns is reported with its owner and `lym adopt root`, and a named beam another machine owns is reported with its owner and `lym adopt <slug>`; neither starts a dobby.
// A named beam whose landing hold stands is reported naming `lym batten status <slug>` and starts no dobby.
// A failed spawn is reported for its worktree and the others still start; the returned error lists every failure.
// A second call reports every dobby live and changes nothing.
func EnsureLive(deps StartDeps, worktrees []string) (StartReport, error) {
	machine, err := deps.Machine()
	if err != nil {
		return StartReport{}, fmt.Errorf("dobbyengine: this machine's name is unknown: %w", err)
	}
	primeNamed := slices.Contains(worktrees, deps.PrimeName)
	var primeOwner string
	if primeNamed {
		primeOwner, err = deps.PrimeOwner()
		if err != nil {
			return StartReport{}, fmt.Errorf("dobbyengine: read the prime owner: %w", err)
		}
	}
	beams, err := deps.Beams()
	if err != nil {
		return StartReport{}, fmt.Errorf("dobbyengine: list the local beams: %w", err)
	}

	var report StartReport
	var failures []error
	// A start's failure joins the returned error; a prime claim that did not land is only reported, as a batten resume warns and goes on.
	startOne := func(label, worktree string) {
		entry := ensureOne(deps, label, worktree)
		report.Entries = append(report.Entries, entry)
		if entry.Outcome == OutcomeFailed {
			failures = append(failures, fmt.Errorf("%s: %s", entry.Worktree, entry.Detail))
		}
	}

	if primeNamed {
		primeClaimFailed := false
		if primeOwner == "" {
			lostTo, claimErr := deps.ClaimPrime(machine)
			switch {
			case claimErr != nil:
				primeClaimFailed = true
				report.Entries = append(report.Entries, StartEntry{
					Worktree: PrimeLabel,
					Outcome:  OutcomeFailed,
					Detail:   fmt.Sprintf("this machine was not recorded as the prime owner: %v; re-run `lym beam start root` once that works", claimErr),
				})
			case lostTo != "":
				primeOwner = lostTo
			default:
				primeOwner = machine
			}
		}
		switch {
		case primeClaimFailed:
		case primeOwner == machine:
			startOne(PrimeLabel, deps.PrimeName)
		default:
			report.Entries = append(report.Entries, StartEntry{
				Worktree: PrimeLabel,
				Outcome:  OutcomeRefused,
				Detail:   fmt.Sprintf("owned by %s; lym adopt root takes it over here", primeOwner),
			})
		}
	}

	for _, slug := range worktrees {
		if slug == deps.PrimeName {
			continue
		}
		owner := ""
		for _, beam := range beams {
			if beam.Slug == slug {
				owner = beam.Owner
				break
			}
		}
		if owner != "" && owner != machine {
			report.Entries = append(report.Entries, StartEntry{
				Worktree: slug,
				Outcome:  OutcomeRefused,
				Detail:   fmt.Sprintf("owned by %s; lym adopt %s takes it over here", owner, slug),
			})
			continue
		}
		startOne(slug, slug)
	}
	return report, errors.Join(failures...)
}

// ensureOne starts the dobby of the worktree named worktree unless it is live, and reports it under label.
// A worktree with an anchor change standing is refused with the change's way forward as the detail and starts nothing, a worktree whose landing hold stands is refused naming `lym batten status`, and an unreadable record is a failure.
func ensureOne(deps StartDeps, label, worktree string) StartEntry {
	geom, err := deps.Geometry(worktree)
	if err != nil {
		return StartEntry{Worktree: label, Outcome: OutcomeFailed, Detail: err.Error()}
	}
	if deps.MoveInProgress != nil {
		wayForward, standing, err := deps.MoveInProgress(worktree)
		if err != nil {
			return StartEntry{Worktree: label, Outcome: OutcomeFailed, Detail: fmt.Sprintf("read whether an anchor change stands: %v", err)}
		}
		if standing {
			return StartEntry{Worktree: label, Outcome: OutcomeRefused, Detail: wayForward}
		}
	}
	if deps.LandingHeld != nil {
		held, err := deps.LandingHeld(worktree)
		if err != nil {
			return StartEntry{Worktree: label, Outcome: OutcomeFailed, Detail: fmt.Sprintf("read whether the landing hold stands: %v", err)}
		}
		if held {
			return StartEntry{Worktree: label, Outcome: OutcomeRefused, Detail: fmt.Sprintf("the landing hold stands; see `lym batten status %s`", worktree)}
		}
	}
	live, err := deps.Live(geom)
	if err != nil {
		return StartEntry{Worktree: label, Outcome: OutcomeFailed, Detail: fmt.Sprintf("read whether the dobby is live: %v", err)}
	}
	if live {
		return StartEntry{Worktree: label, Outcome: OutcomeLive}
	}
	if err := deps.Spawn(geom); err != nil {
		return StartEntry{Worktree: label, Outcome: OutcomeFailed, Detail: fmt.Sprintf("start the dobby: %v", err)}
	}
	return StartEntry{Worktree: label, Outcome: OutcomeStarted}
}

// Live reports whether the dobby of geom is running: its lock is held.
// The lock is tried without blocking and released at once when it was free, so a free lock reads not live.
// A worktree whose `.lym/beam/` directory does not exist yet has never run a dobby.
func Live(geom Geometry) (bool, error) {
	if _, err := os.Stat(filepath.Dir(geom.LockPath)); errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	held, acquired, err := lock.TryAcquireWriteLock(geom.LockPath)
	if err != nil {
		return false, err
	}
	if !acquired {
		return true, nil
	}
	return false, held.Release()
}

// SpawnDetached starts `<lym> beam dobby <worktree root>` for geom as a detached process with the hub root as its working directory.
// The child's output is appended to the file at geom.OutPath, and the child is started but never waited for.
// It returns ErrSpawnUnderTest, starting nothing, in a test binary.
func SpawnDetached(geom Geometry) error {
	if testing.Testing() {
		return ErrSpawnUnderTest
	}
	return spawnDetached(geom, (*exec.Cmd).Start)
}

// spawnDetached rotates the raw-output file where the platform needs it, opens geom.OutPath for append, builds the dobby command with stdout and stderr on that file and starts it through start.
func spawnDetached(geom Geometry, start func(cmd *exec.Cmd) error) error {
	rotateOutBeforeSpawn(geom, runtime.GOOS)
	if err := os.MkdirAll(filepath.Dir(geom.OutPath), 0o755); err != nil {
		return err
	}
	outFile, err := os.OpenFile(geom.OutPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer outFile.Close()

	args := []string{"beam", "dobby", geom.WorktreeRoot}
	cmd := exec.Command(geom.LymExecutable, args...)
	cmd.Dir = geom.HubRoot
	cmd.Stdout = outFile
	cmd.Stderr = outFile
	proc.Detach(cmd)
	if err := start(cmd); err != nil {
		logger.Warn("dobbyengine: spawn dobby failed", "exe", geom.LymExecutable, "args", args, "dir", geom.HubRoot, "error", err)
		return err
	}
	pid := 0
	if cmd.Process != nil {
		pid = cmd.Process.Pid
	}
	logger.Info("dobbyengine: spawned dobby", "exe", geom.LymExecutable, "args", args, "dir", geom.HubRoot, "pid", pid, "out", geom.OutPath)
	return nil
}

// rotateOutBeforeSpawn rotates the raw-output file past its bound on windows alone, where a running dobby holds the file open and cannot rotate it itself, and only when the dobby's lock reads free.
// This is the one rotation a process without the dobby lock makes; a failed rename, as when a refused process still holds the file, is logged and leaves the file for the next spawn.
func rotateOutBeforeSpawn(geom Geometry, goos string) {
	if goos != "windows" {
		return
	}
	live, err := Live(geom)
	if err != nil {
		logger.Warn("dobbyengine: could not read the dobby lock before rotating its output file", "path", geom.OutPath, "error", err)
		return
	}
	if live {
		return
	}
	if _, err := RotatePastBound(geom.OutPath); err != nil {
		logger.Warn("dobbyengine: could not rotate the dobby output file", "path", geom.OutPath, "error", err)
	}
}
