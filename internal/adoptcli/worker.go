// worker.go runs the steps of an adopt as the hidden worker does.
// It holds the worker's seams and its run under adopt.lock, the request checks and the judgement of a standing record,
// and the end envelope the worker merges from the parts its steps recorded.

package adoptcli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/adoptrecord"
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/proc"
)

// workerDeps holds the seams of the worker: the step deps, the process facts it records, and the record and lock access.
// ReadRecord and WriteRecord read and replace the adopt record.
// TryLock takes adopt.lock without blocking, and reports false when another process holds it;
// release gives it up.
// Alive reports whether the worker named by a pid and start ticks is running.
type workerDeps struct {
	Steps       adoptDeps
	PID         int
	StartTicks  string
	Now         func() time.Time
	ReadRecord  func() (adoptrecord.Record, bool, error)
	WriteRecord func(record adoptrecord.Record) error
	TryLock     func() (release func() error, acquired bool, err error)
	Alive       func(pid int, startTicks string) bool
}

// hubWorkerDeps fills workerDeps over the hub the prime belongs to, with the machine's name read through machine.
// The record and the lock sit in the hub's adopt scratch directory.
func hubWorkerDeps(prime *lymcwd.Location, machine func() (string, error)) (workerDeps, error) {
	steps, err := hubDepsAs(prime, machine)
	if err != nil {
		return workerDeps{}, err
	}
	hubPath := prime.HubPath
	pid := os.Getpid()
	ticks, _ := proc.StartTime(pid)
	return workerDeps{
		Steps:       steps,
		PID:         pid,
		StartTicks:  ticks,
		Now:         time.Now,
		ReadRecord:  func() (adoptrecord.Record, bool, error) { return adoptrecord.Read(hubPath) },
		WriteRecord: func(record adoptrecord.Record) error { return adoptrecord.Write(hubPath, record) },
		TryLock:     func() (func() error, bool, error) { return tryAdoptLock(hubPath) },
		Alive:       adoptrecord.WorkerAlive,
	}, nil
}

// tryAdoptLock takes the hub's adopt.lock without blocking, creating the adopt scratch directory it sits in.
// It reports false when another process holds the lock.
func tryAdoptLock(hubPath string) (release func() error, acquired bool, err error) {
	path := adoptrecord.LockPath(hubPath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, false, fmt.Errorf("create the adopt scratch directory: %w", err)
	}
	held, acquired, err := lock.TryAcquireWriteLock(path)
	if err != nil || !acquired {
		return nil, false, err
	}
	return held.Release, true, nil
}

// recorded returns the request as the adopt record stores it.
func (r request) recorded() adoptrecord.Request {
	return adoptrecord.Request(r)
}

// requestOf returns the request a recorded one stands for.
func requestOf(recorded adoptrecord.Request) request {
	return request(recorded)
}

// normalizedRequest returns req with the word `root` among its slugs turned into Root.
func normalizedRequest(req request) request {
	if !slices.Contains(req.Slugs, boardengine.RootSubject) {
		return req
	}
	req.Root = true
	req.Slugs = slices.DeleteFunc(slices.Clone(req.Slugs), func(slug string) bool { return slug == boardengine.RootSubject })
	return req
}

// validateRequest holds the checks an adopt makes before any step.
// It refuses a request that names no subject, one that pairs --all with slugs, and --keep-local with --drop-local.
// It then resolves the request over the board's owners and returns the beam slugs it adopts and whether it adopts root.
func validateRequest(deps adoptDeps, req request) (slugs []string, root bool, err error) {
	req = normalizedRequest(req)
	if req.KeepLocal && req.DropLocal {
		return nil, false, errors.New("lym adopt: --keep-local and --drop-local answer the same local commits in opposite ways: pass one of them")
	}
	if req.All && len(req.Slugs) > 0 {
		return nil, false, errors.New("lym adopt: --all takes no slug: drop the slugs, or drop --all")
	}
	if len(req.Slugs) == 0 && !req.Root && !req.All {
		return nil, false, errors.New("lym adopt: name a beam project or `root`, or pass --all: `lym board list` shows the entries")
	}
	owners, err := deps.ReadOwners()
	if err != nil {
		return nil, false, fmt.Errorf("lym adopt: read the board's owners: %w", err)
	}
	return namedEntries(deps, owners, req)
}

// standingVerdict says what a worker does with the adopt record it finds.
type standingVerdict string

const (
	// verdictFresh starts a new adopt: no record stands, or the one that does is done.
	verdictFresh standingVerdict = "fresh"
	// verdictFollow leaves an unfinished record of the same subjects to its running worker.
	verdictFollow standingVerdict = "follow"
	// verdictResume continues an unfinished record of the same subjects whose worker is gone or stopped.
	verdictResume standingVerdict = "resume"
)

// judgeStanding judges the record found, if any, against a call, over the state the record reads in.
// An unfinished record of other subjects is refused with the re-run that finishes it and the discard that removes it.
func judgeStanding(record adoptrecord.Record, found bool, state string, call adoptrecord.Request) (standingVerdict, error) {
	if !found || state == adoptrecord.StateDone {
		return verdictFresh, nil
	}
	if !record.Request.SameSubjects(call) {
		step := record.InFlight
		if step == "" {
			step = record.ResumeStep()
		}
		return "", fmt.Errorf("lym adopt: an unfinished adopt of %s stands at the %s step: finish it with `%s`, or remove its record with `lym adopt --discard`, before adopting other subjects",
			strings.Join(record.Subjects, ", "), step, record.Request.Command())
	}
	if state == adoptrecord.StateRunning {
		return verdictFollow, nil
	}
	return verdictResume, nil
}

// mergeRequest returns the request a resumed adopt runs: the recorded one with the call's flags added.
// A call's --keep-local or --drop-local replaces the recorded other one.
// restart reports that the merged request answers local commits differently from the recorded one,
// so the resume starts again at the reconcile.
func mergeRequest(recorded, call adoptrecord.Request) (merged adoptrecord.Request, restart bool) {
	merged = recorded
	if call.KeepLocal {
		merged.KeepLocal, merged.DropLocal = true, false
	}
	if call.DropLocal {
		merged.KeepLocal, merged.DropLocal = false, true
	}
	merged.NoOrch = recorded.NoOrch || call.NoOrch
	restart = merged.KeepLocal != recorded.KeepLocal || merged.DropLocal != recorded.DropLocal
	return merged, restart
}

// runWorker runs the steps of an adopt for req under adopt.lock and records each in the adopt record.
// A refused request, a held lock and a record of other subjects, or one whose worker still runs, return before any step.
// An unfinished record of the same subjects is resumed from its first step not done, over the subjects it recorded.
// The resume keeps the steps already listed, unless the call changed how local commits are answered, which clears them.
// It returns the end envelope, the final record, and the error of the step that stopped the adopt, if one did.
func runWorker(deps workerDeps, req request) (outcome, adoptrecord.Record, error) {
	req = normalizedRequest(req)
	if _, _, err := validateRequest(deps.Steps, req); err != nil {
		return outcome{}, adoptrecord.Record{}, err
	}
	release, acquired, err := deps.TryLock()
	if err != nil {
		return outcome{}, adoptrecord.Record{}, fmt.Errorf("lym adopt: take the adopt lock: %w", err)
	}
	if !acquired {
		return outcome{}, adoptrecord.Record{}, errors.New("lym adopt: another adopt holds the adopt lock, so this one did nothing: `lym beam status` shows it")
	}
	defer func() { _ = release() }()

	record, found, err := deps.ReadRecord()
	if err != nil {
		return outcome{}, adoptrecord.Record{}, err
	}
	verdict, err := judgeStanding(record, found, record.ReadState(deps.Alive), req.recorded())
	if err != nil {
		return outcome{}, adoptrecord.Record{}, err
	}
	if verdict == verdictFollow {
		return outcome{}, adoptrecord.Record{}, fmt.Errorf("lym adopt: an adopt of the same subjects is running as process %d, so this one did nothing: `lym beam status` shows it, and `lym adopt --discard` removes its record if that process is not an adopt", record.PID)
	}

	from := adoptrecord.StepReconcile
	restart := false
	if verdict == verdictResume {
		var merged adoptrecord.Request
		merged, restart = mergeRequest(record.Request, req.recorded())
		req = requestOf(merged)
		if next := record.ResumeStep(); !restart && next != "" {
			from = next
		}
	}

	here, err := deps.Steps.Machine()
	if err != nil {
		return outcome{}, adoptrecord.Record{}, fmt.Errorf("lym adopt: read this machine's name: %w", err)
	}
	owners, err := deps.Steps.ReadOwners()
	if err != nil {
		return outcome{}, adoptrecord.Record{}, fmt.Errorf("lym adopt: read the board's owners: %w", err)
	}
	var slugs []string
	var rootNamed bool
	if verdict == verdictResume {
		slugs, rootNamed = splitSubjects(record.Subjects)
		// A beam the board no longer holds, landed or removed since the interruption, is not adopted.
		slugs = slices.DeleteFunc(slugs, func(slug string) bool {
			_, held := owners.Entries[slug]
			return !held
		})
	} else {
		slugs, rootNamed, err = namedEntries(deps.Steps, owners, req)
		if err != nil {
			return outcome{}, adoptrecord.Record{}, err
		}
		subjects := slices.Clone(slugs)
		if rootNamed {
			subjects = append(subjects, boardengine.RootSubject)
		}
		record = adoptrecord.Record{Subjects: subjects}
	}
	if restart {
		record.Steps, record.StoppedPart = nil, nil
	}
	record.Request = req.recorded()
	record.PID, record.StartTicks, record.Start = deps.PID, deps.StartTicks, deps.Now()
	record.State, record.InFlight, record.Envelope = adoptrecord.StateRunning, from, nil

	var result outcome
	var final adoptrecord.Record
	run := &stepRun{
		Deps:      deps.Steps,
		Req:       req,
		Here:      here,
		Slugs:     slugs,
		RootNamed: rootNamed,
		Owners:    owners,
		Outcomes:  map[string]*slugOutcome{},
		Mutations: fabricengine.NewMutations(deps.Steps.HubPath),
		Record:    record,
		Now:       deps.Now,
	}
	run.Save = func(saved adoptrecord.Record) error {
		if saved.State != adoptrecord.StateRunning {
			merged, err := envelopeOf(saved)
			if err != nil {
				return err
			}
			result = merged
			saved.Envelope = adoptFields(merged)
		}
		final = saved
		return deps.WriteRecord(saved)
	}
	if err := run.save(); err != nil {
		return outcome{}, adoptrecord.Record{}, err
	}
	if err := runSteps(run, from); err != nil {
		return result, final, err
	}
	return result, final, run.Stopped
}

// splitSubjects returns the beam slugs a record's subjects name and whether they name root.
func splitSubjects(subjects []string) (slugs []string, rootNamed bool) {
	for _, subject := range subjects {
		if subject == boardengine.RootSubject {
			rootNamed = true
			continue
		}
		slugs = append(slugs, subject)
	}
	return slugs, rootNamed
}

// reconcileWire is the part of a reconcile report a recorded step part holds that decodes back.
// The report's mutation record is left out: it has no decoder, and the part's own mutations list the same entries.
// So the envelope's reconcile object carries no mutations, and its top-level mutations field holds the record.
type reconcileWire struct {
	Unreachable bool
	Branches    []fabricengine.BranchOutcome
	Entries     []fabricengine.EntryOutcome
}

// partWire is an outcomePart as decoded from a record, its reconcile report read through reconcileWire.
type partWire struct {
	outcomePart
	Reconcile *reconcileWire `json:"reconcile,omitempty"`
}

// decodePart decodes the outcome part a step recorded.
func decodePart(raw json.RawMessage) (outcomePart, error) {
	var wire partWire
	if err := json.Unmarshal(raw, &wire); err != nil {
		return outcomePart{}, err
	}
	part := wire.outcomePart
	if wire.Reconcile != nil {
		part.Reconcile = &fabricengine.OriginReport{Unreachable: wire.Reconcile.Unreachable, Branches: wire.Reconcile.Branches, Entries: wire.Reconcile.Entries}
	}
	return part, nil
}

// envelopeOf merges the outcome parts of the record into the adopt's end envelope.
// It takes the part of every step the record lists as done, an earlier process's steps included, in step order.
// It then takes the part a stopped step gathered before its error, without listing that step as done.
// A scalar field of a later part replaces an earlier one.
// Slugs merge per slug and per field, and list fields concatenate.
// Every slug the record's subjects, runs and paused slugs name has an entry, even when no part holds a field of it.
// The slugs the record's subjects name, root apart, and that a part holds come first, in the subjects' order.
func envelopeOf(record adoptrecord.Record) (outcome, error) {
	var result outcome
	outcomes := map[string]*slugOutcome{}
	merge := func(step string, raw json.RawMessage) error {
		if len(raw) == 0 {
			return nil
		}
		part, err := decodePart(raw)
		if err != nil {
			return fmt.Errorf("lym adopt: decode the part of the %s step: %w", step, err)
		}
		return mergePart(&result, outcomes, part)
	}
	for _, step := range adoptrecord.Steps {
		index := slices.IndexFunc(record.Steps, func(done adoptrecord.StepRecord) bool { return done.Name == step })
		if index < 0 {
			continue
		}
		if err := merge(step, record.Steps[index].Outcome); err != nil {
			return outcome{}, err
		}
	}
	if err := merge(record.InFlight, record.StoppedPart); err != nil {
		return outcome{}, err
	}

	if result.Mutations == nil {
		result.Mutations = []fabricengine.Mutation{}
	}
	seedOutcomes(outcomes, record)
	var named []string
	for _, subject := range record.Subjects {
		if subject != boardengine.RootSubject {
			named = append(named, subject)
		}
	}
	result.Slugs = collect(named, outcomes)
	return result, nil
}
