// start.go starts the dobby of every worktree this machine owns: the claim of the prime's owner, the liveness read of a dobby's lock, and the detached spawn of one dobby process.

package dobbyengine

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/proc"
)

const (
	// LogFileName is the dobby log file in the marker directory, which a spawned dobby appends its output to.
	LogFileName = "dobby.log"
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

// EnsureLive starts the dobby of the prime and of every local beam pair this machine owns, each only when it is not live already.
// A prime with no recorded owner is claimed for this machine first; a claim that fails is reported for the prime with its cause and the `lyx beam start` re-run, the prime's dobby is not started, and the beams go on.
// A prime another machine owns is reported with its owner and starts no prime dobby, and a beam another machine owns is left out of the report.
// A failed spawn is reported for its worktree and the others still start; the returned error lists every failure.
// A second call reports every dobby live and changes nothing.
func EnsureLive(deps StartDeps) (StartReport, error) {
	machine, err := deps.Machine()
	if err != nil {
		return StartReport{}, fmt.Errorf("dobbyengine: this machine's name is unknown: %w", err)
	}
	primeOwner, err := deps.PrimeOwner()
	if err != nil {
		return StartReport{}, fmt.Errorf("dobbyengine: read the prime owner: %w", err)
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

	primeClaimFailed := false
	if primeOwner == "" {
		lostTo, claimErr := deps.ClaimPrime(machine)
		switch {
		case claimErr != nil:
			primeClaimFailed = true
			report.Entries = append(report.Entries, StartEntry{
				Worktree: PrimeLabel,
				Outcome:  OutcomeFailed,
				Detail:   fmt.Sprintf("this machine was not recorded as the prime owner: %v; re-run `lyx beam start` once that works", claimErr),
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
			Detail:   fmt.Sprintf("owned by %s; lyx adopt --prime takes it over here", primeOwner),
		})
	}

	for _, beam := range beams {
		if beam.Owner != "" && beam.Owner != machine {
			continue
		}
		startOne(beam.Slug, beam.Slug)
	}
	return report, errors.Join(failures...)
}

// ensureOne starts the dobby of the worktree named worktree unless it is live, and reports it under label.
// A worktree with an anchor change standing is refused with the change's way forward as the detail and starts nothing, and an unreadable record is a failure.
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
// A worktree whose marker directory does not exist yet has never run a dobby.
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

// SpawnDetached starts `<lyx> beam dobby <worktree root>` for geom as a detached process with the hub root as its working directory.
// The child's output is appended to the marker directory's log, and the child is started but never waited for.
// It returns ErrSpawnUnderTest, starting nothing, in a test binary.
func SpawnDetached(geom Geometry) error {
	if testing.Testing() {
		return ErrSpawnUnderTest
	}
	if err := os.MkdirAll(geom.MarkerDir, 0o755); err != nil {
		return err
	}
	logPath := filepath.Join(geom.MarkerDir, LogFileName)
	logFile, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer logFile.Close()

	args := []string{"beam", "dobby", geom.WorktreeRoot}
	cmd := exec.Command(geom.LyxExecutable, args...)
	cmd.Dir = geom.HubRoot
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	proc.Detach(cmd)
	if err := cmd.Start(); err != nil {
		logger.Warn("dobbyengine: spawn dobby failed", "exe", geom.LyxExecutable, "args", args, "dir", geom.HubRoot, "error", err)
		return err
	}
	logger.Info("dobbyengine: spawned dobby", "exe", geom.LyxExecutable, "args", args, "dir", geom.HubRoot, "pid", cmd.Process.Pid, "log", logPath)
	return nil
}
