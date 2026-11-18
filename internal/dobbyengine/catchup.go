// catchup.go is the catch-up from origin that runs after an absence, before any act rule answer.
// It reads and acts through seams, so the package imports no fabric or teardown package.

package dobbyengine

import (
	"errors"
	"fmt"
)

// ErrOriginUnreachable is what a catch-up's seam wraps when origin does not answer.
// The catch-up then acts on no run, and its caller keeps the catch-up owed.
var ErrOriginUnreachable = errors.New("dobbyengine: origin is unreachable, so the catch-up acted on no run")

// CatchUpDeps is every seam a catch-up reads and acts through.
// The hub wiring fills them, and a nil seam is a wiring error CatchUp returns at once.
type CatchUpDeps struct {
	// PullBolt fetches Bolt and replays the local display commits.
	// An unreachable origin is an error wrapping ErrOriginUnreachable.
	PullBolt func() error
	// Register registers this machine when the registry holds no entry of its id, or checks the entry it holds.
	// A refusal is returned from CatchUp as given.
	Register func() error
	// Runs lists the slugs of the local pairs the catch-up covers.
	Runs func() ([]string, error)
	// FetchRun fetches the run's branches from origin.
	// An unreachable origin is an error wrapping ErrOriginUnreachable.
	FetchRun func(slug string) error
	// ReadRun reads the run as the fetch left it.
	ReadRun func(slug string) (CatchUpRun, error)
	// PushOwed pushes the run's records and then its code branch.
	// It hands the run over itself when origin holds another machine's takeover entry, and then answers nil.
	PushOwed func(slug string) error
	// HandOver ends the run's processes, archives its local tips and removes its local pair; owner is the machine owning it.
	HandOver func(slug, owner string) error
	// ApplyPauseIntents writes this machine's pause entries from the pause intents of the runs it owns.
	ApplyPauseIntents func(slugs []string) error
	// Launcher names the hub's desktop launcher, which the catch-up removes once this machine owns nothing in the hub.
	// Its zero value keeps no launcher.
	Launcher Launcher
}

// CatchUpRun is what the catch-up reads of one run after the fetch.
type CatchUpRun struct {
	// Owner is the machine that owns the run's entry.
	Owner string
	// OwnedElsewhere reports whether another machine owns the run, judged through the machine registry.
	OwnedElsewhere bool
	// Ahead reports whether this machine holds commits of the run, on either side, that origin lacks.
	Ahead bool
}

// CatchUpReport lists the runs a catch-up touched.
// A caller reads every listed run as touched.
type CatchUpReport struct {
	// Pushed are the runs owned here whose owed commits were pushed.
	Pushed []string
	// HandedOver are the runs another machine owns, which were handed over.
	HandedOver []string
	// OwnedHere are the runs this machine owns, the pushed ones included.
	OwnedHere []string
}

// CatchUp brings this machine's view of its runs up to origin's after an absence.
// It pulls Bolt and registers this machine, fetches and reads every run, then acts:
// a run owned here with commits origin lacks is pushed, a run owned elsewhere is handed over,
// and the runs owned here get their pause entries written from their pause intents.
// Last, it removes the hub's desktop launcher when the pulled board reads that this machine owns nothing in the hub;
// a launcher it could not remove is logged and fails nothing.
// An unreachable origin at the Bolt pull or a fetch, and a registration refusal, return before any run is acted on.
// A failed push or hand-over is joined into the returned error and the other runs go on,
// so the caller keeps the catch-up owed.
func CatchUp(deps CatchUpDeps) (CatchUpReport, error) {
	var report CatchUpReport
	if err := deps.requireSeams(); err != nil {
		return report, err
	}
	if err := deps.PullBolt(); err != nil {
		return report, fmt.Errorf("dobbyengine: the catch-up could not pull Bolt: %w", err)
	}
	if err := deps.Register(); err != nil {
		return report, err
	}
	slugs, err := deps.Runs()
	if err != nil {
		return report, fmt.Errorf("dobbyengine: the catch-up could not list the runs: %w", err)
	}

	runs := make([]CatchUpRun, len(slugs))
	for i, slug := range slugs {
		if err := deps.FetchRun(slug); err != nil {
			return report, fmt.Errorf("dobbyengine: the catch-up could not fetch run %s: %w", slug, err)
		}
		run, err := deps.ReadRun(slug)
		if err != nil {
			return report, fmt.Errorf("dobbyengine: the catch-up could not read run %s: %w", slug, err)
		}
		runs[i] = run
	}

	var failures []error
	for i, slug := range slugs {
		run := runs[i]
		switch {
		case run.OwnedElsewhere:
			if err := deps.HandOver(slug, run.Owner); err != nil {
				failures = append(failures, fmt.Errorf("dobbyengine: the catch-up could not hand over run %s: %w", slug, err))
				continue
			}
			report.HandedOver = append(report.HandedOver, slug)
		case run.Ahead:
			report.OwnedHere = append(report.OwnedHere, slug)
			if err := deps.PushOwed(slug); err != nil {
				failures = append(failures, fmt.Errorf("dobbyengine: the catch-up could not push run %s: %w", slug, err))
				continue
			}
			report.Pushed = append(report.Pushed, slug)
		default:
			report.OwnedHere = append(report.OwnedHere, slug)
		}
	}
	if len(report.OwnedHere) > 0 {
		if err := deps.ApplyPauseIntents(report.OwnedHere); err != nil {
			failures = append(failures, fmt.Errorf("dobbyengine: the catch-up could not apply the pause intents: %w", err))
		}
	}
	removeLauncherIfOwnsNothing(deps.Launcher)
	return report, errors.Join(failures...)
}

// requireSeams returns a wiring error naming the first seam the deps leave nil.
func (deps CatchUpDeps) requireSeams() error {
	seams := []struct {
		name string
		set  bool
	}{
		{"PullBolt", deps.PullBolt != nil},
		{"Register", deps.Register != nil},
		{"Runs", deps.Runs != nil},
		{"FetchRun", deps.FetchRun != nil},
		{"ReadRun", deps.ReadRun != nil},
		{"PushOwed", deps.PushOwed != nil},
		{"HandOver", deps.HandOver != nil},
		{"ApplyPauseIntents", deps.ApplyPauseIntents != nil},
	}
	for _, seam := range seams {
		if !seam.set {
			return fmt.Errorf("dobbyengine: the catch-up has no %s seam; this is a wiring error", seam.name)
		}
	}
	return nil
}
