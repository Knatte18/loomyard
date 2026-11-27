// upgrade.go is the first read after an upgrade.
// It turns the pause record's standing adopt, legacy adopt-hold and owner entries into pause intents and one Bolt hold.
// It reads and acts through seams, so the package imports no fabric, teardown or CLI package.

package dobbyengine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fixtries"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/pauserecord"
)

// UpgradeDeps is every seam the first read after an upgrade reads and acts through.
// The hub wiring fills them.
// A UpgradeDeps whose Entries is nil means no first read.
// Any other nil seam is a wiring error UpgradeRead returns at once.
type UpgradeDeps struct {
	// Lock is the path of the lock that makes the dobbies of one machine run the first read one at a time.
	Lock string
	// Now is the wall clock, which stamps the hold.
	Now func() time.Time
	// Entries lists the live entries of the pause record.
	Entries func() ([]pauserecord.Entry, error)
	// PullBolt fetches Bolt, so the owners, the registry and the holds the read judges are origin's.
	// An unreachable origin is an error wrapping ErrOriginUnreachable.
	PullBolt func() error
	// Classify reads the run, or the released task, that slug names.
	Classify func(slug string) (UpgradeRun, error)
	// PauseIntent records and pushes the run's pause intent synchronously; an owed push is an error.
	// It reports handedOver when the push met another machine's takeover entry, which reads the run as owned elsewhere.
	PauseIntent func(slug string) (handedOver bool, err error)
	// Machine returns this machine's name, which the hold records.
	Machine func() (string, error)
	// OpenFixTask returns the open adopt fix task of subject, found false when it has none.
	OpenFixTask func(subject string) (slug string, found bool, err error)
	// AdoptDir is the adopt scratch directory, which holds each subject's fix-try count in a directory of the subject's name.
	AdoptDir string
	// WriteHold appends the hold to Bolt and pushes it.
	// A hold of another machine that stands comes back in Standing, and nothing is written.
	// An unreachable origin is an error wrapping boardengine.ErrOwnerOriginUnreachable.
	WriteHold func(hold boardengine.Hold) (boardengine.HoldWrite, error)
	// Drop drops the live entry by its exact scope, caller and pause time.
	Drop func(entry pauserecord.Entry) error
	// Notice posts one line to the nearest live orch.
	Notice func(line string) error
}

// UpgradeRun is what the first read learns of the run, or the released task, a standing entry names.
type UpgradeRun struct {
	// OwnedHere reports whether this machine owns the run, judged through the machine registry.
	OwnedHere bool
	// HasRecord reports whether the run has a batten record.
	HasRecord bool
	// Terminal reports whether the run's state is landed or cancelled.
	Terminal bool
	// Queued reports whether the entry is a released task with no run yet.
	Queued bool
	// Subject is the beam or root subject `lym adopt` takes to move the run.
	Subject string
}

// UpgradeReport lists what a first read did.
type UpgradeReport struct {
	// PauseIntents are the runs whose pause intent the read recorded.
	PauseIntents []string
	// HoldWritten reports whether the read wrote a hold to Bolt.
	HoldWritten bool
	// Dropped counts the pause entries the read dropped.
	Dropped int
}

// UpgradeRead turns the pause record's standing adopt, adopt-hold and owner entries into the Bolt model.
// It holds Lock, so a second dobby that runs it afterwards finds no entry left.
// With no such entry it does nothing.
// Otherwise it fetches Bolt first, and an unreachable origin returns ErrOriginUnreachable with nothing written.
// Each adopt or adopt-hold entry of a run this machine owns that has a record and is not terminal becomes a pause intent.
// The subjects of those runs and of the released tasks the entries name form one hold.
// It is written unless another machine's hold stands.
// The hold is adopt-fix while an open adopt fix task of a subject stands, and adopt otherwise.
// One notice names the hold, its machine and the way forward.
// Only then does it drop every adopt, adopt-hold and owner entry.
// A failure before the drops returns first, so a cut read runs again from the entries it left.
func UpgradeRead(deps UpgradeDeps) (UpgradeReport, error) {
	var report UpgradeReport
	if err := deps.requireSeams(); err != nil {
		return report, err
	}
	if err := os.MkdirAll(filepath.Dir(deps.Lock), 0o755); err != nil {
		return report, fmt.Errorf("dobbyengine: the first read could not create the directory of its lock: %w", err)
	}
	held, err := lock.AcquireWriteLock(deps.Lock)
	if err != nil {
		return report, fmt.Errorf("dobbyengine: the first read could not take its lock: %w", err)
	}
	defer func() { _ = held.Release() }()

	live, err := deps.Entries()
	if err != nil {
		return report, fmt.Errorf("dobbyengine: the first read could not read the pause record: %w", err)
	}
	standing := slices.DeleteFunc(slices.Clone(live), func(entry pauserecord.Entry) bool { return !readsAsUpgradeEntry(entry.Caller) })
	if len(standing) == 0 {
		return report, nil
	}
	if err := deps.PullBolt(); err != nil {
		return report, fmt.Errorf("dobbyengine: the first read could not pull Bolt: %w", err)
	}

	intents, subjects, err := deps.recordPauseIntents(standing)
	report.PauseIntents = intents
	if err != nil {
		return report, err
	}
	if len(subjects) > 0 {
		report.HoldWritten, err = deps.writeHold(subjects, intents)
		if err != nil {
			return report, err
		}
	}

	var failures []error
	for _, entry := range standing {
		if err := deps.Drop(entry); err != nil {
			failures = append(failures, fmt.Errorf("dobbyengine: the first read could not drop the %s entry of %s: %w", entry.Caller, entry.Scope, err))
			continue
		}
		report.Dropped++
	}
	return report, errors.Join(failures...)
}

// readsAsUpgradeEntry reports whether an entry of caller is one the first read turns into the Bolt model.
func readsAsUpgradeEntry(caller string) bool {
	switch caller {
	case pauserecord.AdoptCaller, pauserecord.HoldCaller, pauserecord.OwnerCaller:
		return true
	default:
		return false
	}
}

// recordPauseIntents records the pause intent of each run the standing adopt entries name that this machine owns.
// It returns the runs it recorded, and the subjects of those runs and of the released tasks the entries name.
// An owner entry and a hub-scope entry name no run.
func (deps UpgradeDeps) recordPauseIntents(standing []pauserecord.Entry) (intents, subjects []string, err error) {
	addSubject := func(subject string) {
		if !slices.Contains(subjects, subject) {
			subjects = append(subjects, subject)
		}
	}
	for _, entry := range standing {
		if entry.Caller == pauserecord.OwnerCaller || entry.Scope == pauserecord.ScopeHub {
			continue
		}
		run, err := deps.Classify(entry.Scope)
		if err != nil {
			return intents, subjects, fmt.Errorf("dobbyengine: the first read could not read %s: %w", entry.Scope, err)
		}
		switch {
		case !run.OwnedHere:
		case run.Queued:
			addSubject(run.Subject)
		case run.HasRecord && !run.Terminal:
			handedOver, err := deps.PauseIntent(entry.Scope)
			if err != nil {
				return intents, subjects, fmt.Errorf("dobbyengine: the first read could not record the pause intent of %s: %w", entry.Scope, err)
			}
			if handedOver {
				continue
			}
			intents = append(intents, entry.Scope)
			addSubject(run.Subject)
		}
	}
	return intents, subjects, nil
}

// writeHold writes the hold over subjects and posts the notice that names it.
// It reports whether it wrote a hold.
// A hold of another machine that stands writes none, and the notice names that machine.
// An identical hold of this machine that stands writes none and posts no notice.
func (deps UpgradeDeps) writeHold(subjects, intents []string) (bool, error) {
	machine, err := deps.Machine()
	if err != nil {
		return false, fmt.Errorf("dobbyengine: the first read could not read this machine's name: %w", err)
	}
	hold := boardengine.Hold{Kind: boardengine.HoldKindAdopt, Machine: machine, At: deps.Now().UTC()}
	for _, subject := range subjects {
		hold.Subjects = append(hold.Subjects, boardengine.HoldSubject{Subject: subject})
	}
	for _, subject := range subjects {
		fixTask, found, err := deps.OpenFixTask(subject)
		if err != nil {
			return false, fmt.Errorf("dobbyengine: the first read could not look for the fix task of %s: %w", subject, err)
		}
		if found {
			hold.Kind, hold.FixTask = boardengine.HoldKindAdoptFix, fixTask
			break
		}
	}
	if hold.Kind == boardengine.HoldKindAdoptFix {
		for i := range hold.Subjects {
			tries, err := fixtries.Read(filepath.Join(deps.AdoptDir, hold.Subjects[i].Subject))
			if err != nil {
				return false, fmt.Errorf("dobbyengine: the first read could not read the fix-try count of %s: %w", hold.Subjects[i].Subject, err)
			}
			hold.Subjects[i].Tries = tries
		}
	}

	written, err := deps.WriteHold(hold)
	if err != nil {
		if errors.Is(err, boardengine.ErrOwnerOriginUnreachable) {
			return false, fmt.Errorf("%w: %v", ErrOriginUnreachable, err)
		}
		return false, fmt.Errorf("dobbyengine: the first read could not write the hold: %w", err)
	}
	// An identical hold of this machine already stands, so an earlier run posted the notice.
	if written.Wrote || written.Standing != nil {
		if err := deps.Notice(upgradeNotice(hold, written.Standing, intents)); err != nil {
			logger.Warn("dobbyengine: the first read's notice was not posted", "error", err)
		}
	}
	return written.Wrote, nil
}

// upgradeNotice words the notice of a first read that found subjects to hold.
// A standing hold of another machine is named in place of the one this machine would have written.
// The way forward is a re-run of the adopt here, then the resume of each run that became a pause intent.
func upgradeNotice(hold boardengine.Hold, standing *boardengine.Hold, intents []string) string {
	subjects := make([]string, len(hold.Subjects))
	for i, subject := range hold.Subjects {
		subjects[i] = subject.Subject
	}
	list := strings.Join(subjects, " ")
	wayForward := fmt.Sprintf("re-run `lym adopt %s` there", list)
	if len(intents) > 0 {
		wayForward += fmt.Sprintf(", then, once that adopt's check is green, run `lym tender resume <slug>` there for each of %s",
			strings.Join(intents, ", "))
	}
	if standing != nil {
		return fmt.Sprintf("dobby upgrade: machine %s's %s hold stands, so none was written for %s; way forward: once it clears, %s",
			standing.Machine, standing.Kind, list, wayForward)
	}
	return fmt.Sprintf("dobby upgrade: a %s hold of machine %s now stands for %s; way forward: %s", hold.Kind, hold.Machine, list, wayForward)
}

// requireSeams returns a wiring error naming the first seam the deps leave unset.
func (deps UpgradeDeps) requireSeams() error {
	seams := []struct {
		name string
		set  bool
	}{
		{"Lock", deps.Lock != ""},
		{"Now", deps.Now != nil},
		{"Entries", deps.Entries != nil},
		{"PullBolt", deps.PullBolt != nil},
		{"Classify", deps.Classify != nil},
		{"PauseIntent", deps.PauseIntent != nil},
		{"Machine", deps.Machine != nil},
		{"OpenFixTask", deps.OpenFixTask != nil},
		{"WriteHold", deps.WriteHold != nil},
		{"Drop", deps.Drop != nil},
		{"Notice", deps.Notice != nil},
	}
	for _, seam := range seams {
		if !seam.set {
			return fmt.Errorf("dobbyengine: the first read has no %s seam; this is a wiring error", seam.name)
		}
	}
	return nil
}
