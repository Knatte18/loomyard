// record.go holds the adopt record's types, its path and lock path, and its lock-free read and atomic write.

package adoptrecord

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/state"
)

// FileName is the adopt record's file name within the adopt scratch directory.
const FileName = "adopt.json"

// LockName is the file name of the lock the worker and `lym adopt --discard` take, within the adopt scratch directory.
const LockName = "adopt.lock"

// The steps of an adopt, in the order the worker runs them.
const (
	StepReconcile = "reconcile"
	StepPairs     = "pairs"
	StepPause     = "pause"
	StepOwner     = "owner"
	StepDeploy    = "deploy"
	StepBeam      = "beam"
	StepCheck     = "check"
	StepDone      = "done"
)

// Steps lists every step in the order the worker runs them.
var Steps = []string{StepReconcile, StepPairs, StepPause, StepOwner, StepDeploy, StepBeam, StepCheck, StepDone}

// The states of an adopt record.
// StateInterrupted is never written: ReadState reports it for a running record whose worker is gone.
const (
	StateRunning     = "running"
	StateStopped     = "stopped"
	StateDone        = "done"
	StateInterrupted = "interrupted"
)

// ErrDecode is the sentinel a record that does not decode wraps, so a caller can tell it from an I/O error.
var ErrDecode = errors.New("the adopt record does not decode")

// Request is the request a `lym adopt` carries.
type Request struct {
	// Slugs names the beams to adopt.
	Slugs []string `json:"slugs"`
	// Root reports whether the request names root.
	Root bool `json:"root"`
	// All reports whether the request adopts root and every open beam project.
	All       bool `json:"all"`
	KeepLocal bool `json:"keep_local"`
	DropLocal bool `json:"drop_local"`
	NoOrch    bool `json:"no_orch"`
}

// SameSubjects reports whether two requests name the same subjects.
// Two `--all` requests match, whatever set each resolves to, and an `--all` request never matches a named one.
// Otherwise the requests match when they name the same slugs in any order and agree on Root.
func (q Request) SameSubjects(other Request) bool {
	if q.All || other.All {
		return q.All && other.All
	}
	return q.Root == other.Root && slices.Equal(sortedSlugs(q.Slugs), sortedSlugs(other.Slugs))
}

// Command renders the `lym adopt` command line of the request:
// its slugs sorted, `root`, `--all`, then each set flag in a fixed order.
// Every refusal and way forward that names the re-run of the same `lym adopt` uses it.
func (q Request) Command() string {
	parts := append([]string{"lym", "adopt"}, sortedSlugs(q.Slugs)...)
	for _, flag := range []struct {
		set  bool
		text string
	}{
		{q.Root, "root"},
		{q.All, "--all"},
		{q.KeepLocal, "--keep-local"},
		{q.DropLocal, "--drop-local"},
		{q.NoOrch, "--no-orch"},
	} {
		if flag.set {
			parts = append(parts, flag.text)
		}
	}
	return strings.Join(parts, " ")
}

// sortedSlugs returns a sorted copy of slugs.
func sortedSlugs(slugs []string) []string {
	sorted := slices.Clone(slugs)
	slices.Sort(sorted)
	return sorted
}

// StepRecord is one step that ended.
type StepRecord struct {
	Name    string    `json:"name"`
	EndedAt time.Time `json:"ended_at"`
	// Outcome is the part of the end envelope this step contributes.
	Outcome json.RawMessage `json:"outcome,omitempty"`
}

// Inputs holds what the steps from `deploy` on need, so a resume past `owner` never rereads the earlier process.
type Inputs struct {
	// LocalRuns and Moved are the runs before and after the reconcile.
	LocalRuns []string `json:"local_runs,omitempty"`
	Moved     []string `json:"moved,omitempty"`
	// Runs and Adoptable are the runs the pairs step resolved and the ones it found adoptable.
	Runs      []string `json:"runs,omitempty"`
	Adoptable []string `json:"adoptable,omitempty"`
	RootGoing bool     `json:"root_going,omitempty"`
	// HeldBack lists the projects a local-commit stop held back.
	HeldBack []string `json:"held_back,omitempty"`
	// OwnerStood lists the beams whose owner write stood.
	OwnerStood    []string `json:"owner_stood,omitempty"`
	PrimeRecorded bool     `json:"prime_recorded,omitempty"`
	// Paused lists the slugs the pause step paused, converted beam slugs included.
	Paused []string `json:"paused,omitempty"`
}

// Record is one adopt: its request, its worker, the steps ended so far and where it stands.
// It is safe to copy; the worker is its only writer.
type Record struct {
	Request Request `json:"request"`
	// Subjects are the resolved beam slugs, with `root` when root goes.
	Subjects []string `json:"subjects"`
	// PID and StartTicks name the worker; StartTicks is empty where the platform cannot read a start time.
	PID        int       `json:"pid"`
	StartTicks string    `json:"start_ticks,omitempty"`
	Start      time.Time `json:"start"`
	// Steps lists the steps that ended, and InFlight the step running now.
	Steps    []StepRecord `json:"steps"`
	InFlight string       `json:"in_flight,omitempty"`
	Inputs   Inputs       `json:"inputs"`
	State    string       `json:"state"`
	// Error and WayForward describe a stop; StoppedPart is what the stopped step gathered before its error.
	Error       string          `json:"error,omitempty"`
	WayForward  string          `json:"way_forward,omitempty"`
	StoppedPart json.RawMessage `json:"stopped_part,omitempty"`
	// Envelope is the end envelope the worker writes at `done` or a stop.
	Envelope map[string]any `json:"envelope,omitempty"`
}

// Done reports whether Steps lists step.
func (r Record) Done(step string) bool {
	return slices.ContainsFunc(r.Steps, func(done StepRecord) bool { return done.Name == step })
}

// ResumeStep returns the first step of Steps that is not done, or the empty string when every step is done.
// When that step is `owner` or earlier it returns `reconcile`, so a resumed owner write always follows a fresh reconcile.
func (r Record) ResumeStep() string {
	for index, step := range Steps {
		if r.Done(step) {
			continue
		}
		if index <= slices.Index(Steps, StepOwner) {
			return StepReconcile
		}
		return step
	}
	return ""
}

// ReadState returns State, except StateInterrupted for a running record whose worker alive reads gone.
func (r Record) ReadState(alive func(pid int, startTicks string) bool) string {
	if r.State == StateRunning && !alive(r.PID, r.StartTicks) {
		return StateInterrupted
	}
	return r.State
}

// Unfinished reports whether the record's state is anything but done.
func (r Record) Unfinished() bool {
	return r.State != StateDone
}

// Covers reports whether the record holds subject, a beam slug or `root`, under adopt:
// the record is unfinished, lists subject among its subjects, and records no owner write of subject that stood.
func (r Record) Covers(subject string) bool {
	if !r.Unfinished() || !slices.Contains(r.Subjects, subject) {
		return false
	}
	if subject == boardengine.RootSubject {
		return !r.Inputs.PrimeRecorded
	}
	return !slices.Contains(r.Inputs.OwnerStood, subject)
}

// WorkerAlive reports whether the worker with pid and startTicks is running.
// An exited worker its parent has not reaped yet reads gone.
// So does a pid now held by a process with other start ticks.
// A record without start ticks, or a platform that cannot read a start time, reads alive on the pid alone.
func WorkerAlive(pid int, startTicks string) bool {
	if !proc.IsRunning(pid) {
		return false
	}
	if startTicks == "" {
		return true
	}
	ticks, readable := proc.StartTime(pid)
	return !readable || ticks == startTicks
}

// Path returns the adopt record's file in the hub's adopt scratch directory.
func Path(hubPath string) string {
	return filepath.Join(fabricengine.AdoptScratchDir(hubPath), FileName)
}

// LockPath returns the lock file beside the adopt record that the worker and `lym adopt --discard` take.
func LockPath(hubPath string) string {
	return filepath.Join(fabricengine.AdoptScratchDir(hubPath), LockName)
}

// Read returns the adopt record of the hub and whether one exists.
// An absent record reads false and creates no directory or lock file.
// It takes no lock, since Write replaces the file atomically, and it tolerates unknown keys.
// A record that does not decode is an error wrapping ErrDecode, naming the path and `lym adopt --discard`.
func Read(hubPath string) (Record, bool, error) {
	path := Path(hubPath)
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Record{}, false, nil
	}
	if err != nil {
		return Record{}, false, fmt.Errorf("read the adopt record %s: %w", path, err)
	}
	var record Record
	if err := json.Unmarshal(data, &record); err != nil {
		return Record{}, false, fmt.Errorf("%w: %s: %v; way forward: remove it with `lym adopt --discard`", ErrDecode, path, err)
	}
	return record, true, nil
}

// Write replaces the adopt record atomically under the record's state lock, creating the scratch directory.
// It keeps the keys of the replaced file that Record does not declare, and it does not take the adopt lock.
func Write(hubPath string, record Record) error {
	path := Path(hubPath)
	if err := state.WriteJSON(path, path+".lock", record); err != nil {
		return fmt.Errorf("write the adopt record %s: %w", path, err)
	}
	return nil
}

// Remove deletes the adopt record; an absent record is no error.
// It does not take the adopt lock.
func Remove(hubPath string) error {
	path := Path(hubPath)
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove the adopt record %s: %w", path, err)
	}
	return nil
}
