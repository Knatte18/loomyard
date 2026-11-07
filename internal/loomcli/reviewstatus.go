// reviewstatus.go fills shedverbs.Hooks.Waiting for loom.
// It makes `lym loom status` report a run that waits on a gate slot, the parent lock, a verify or a shuttle.

package loomcli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/parentlock"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedverbs"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/verifytree"
)

// elapsedPlaceholder replaces every elapsed time in a wait note's steady form.
// The steady form then changes only when what the run waits on changes.
const elapsedPlaceholder = "<elapsed>"

// formatElapsed renders d as whole seconds under a minute, whole minutes under an hour and hours with minutes beyond.
// Examples: `45s`, `6m`, `1h12m`.
func formatElapsed(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d/time.Second))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d/time.Minute))
	default:
		return fmt.Sprintf("%dh%dm", int(d/time.Hour), int((d%time.Hour)/time.Minute))
	}
}

// timedNote builds a wait note whose one time-varying part is elapsed:
// render receives elapsed formatted for the note's text and elapsedPlaceholder for its steady form.
func timedNote(render func(elapsed string) string, elapsed time.Duration) shedverbs.WaitNote {
	return shedverbs.WaitNote{Text: render(formatElapsed(elapsed)), Steady: render(elapsedPlaceholder)}
}

// formatWaitNote renders one wait as `<producer>: <what> running <elapsed> (<detail>)`, dropping the parenthesis when detail is empty.
func formatWaitNote(producer, what string, elapsed time.Duration, detail string) shedverbs.WaitNote {
	return timedNote(func(shown string) string {
		note := fmt.Sprintf("%s: %s running %s", producer, what, shown)
		if detail == "" {
			return note
		}
		return note + " (" + detail + ")"
	}, elapsed)
}

// formatHolders renders the gate-slot holders as `<worktree> <site>, <worktree> <site>`.
func formatHolders(holders []gateslot.Holder) string {
	named := make([]string, len(holders))
	for i, holder := range holders {
		named[i] = holder.Worktree + " " + holder.Site
	}
	return strings.Join(named, ", ")
}

// readHolders returns the holders clause body, empty when the read fails or no slot is held.
// The clause only explains a wait, so status must not fail on a failed read.
func readHolders(holders func() ([]gateslot.Holder, error)) string {
	held, err := holders()
	if err != nil {
		return ""
	}
	return formatHolders(held)
}

// gateWaiting returns the Waiting hook that reports each live gate wait record in waitDir, and appends next's note.
// Each record renders as `<producer>: <site> waiting for a gate slot <elapsed>`, oldest first and joined with `; `.
// When holders names any, `(holders: <worktree> <site>, ...)` follows the records once.
// With no live record the note is next's; with one or more, next's non-empty note is appended after `; `.
// A record whose process is dead reads as absent, so a crashed wait never sticks in the status line.
func gateWaiting(waitDir string, holders func() ([]gateslot.Holder, error), now func() time.Time, next func(st shedengine.Status) (shedverbs.WaitNote, error)) func(st shedengine.Status) (shedverbs.WaitNote, error) {
	return func(st shedengine.Status) (shedverbs.WaitNote, error) {
		waits, err := gateslot.ReadWaits(waitDir)
		if err != nil {
			return shedverbs.WaitNote{}, err
		}
		if len(waits) == 0 {
			return next(st)
		}
		sort.SliceStable(waits, func(i, j int) bool { return waits[i].Started.Before(waits[j].Started) })
		texts := make([]string, len(waits))
		steadies := make([]string, len(waits))
		for i, wait := range waits {
			clause := timedNote(func(shown string) string {
				return fmt.Sprintf("%s: %s waiting for a gate slot %s", st.CurrentProducer, wait.Site, shown)
			}, now().Sub(wait.Started))
			texts[i], steadies[i] = clause.Text, clause.Steady
		}
		note := shedverbs.WaitNote{Text: strings.Join(texts, "; "), Steady: strings.Join(steadies, "; ")}
		if named := readHolders(holders); named != "" {
			note.Text += " (holders: " + named + ")"
			note.Steady += " (holders: " + named + ")"
		}
		rest, err := next(st)
		if err != nil {
			return shedverbs.WaitNote{}, err
		}
		if rest.Text != "" {
			note.Text += "; " + rest.Text
			note.Steady += "; " + rest.Steady
		}
		return note, nil
	}
}

// verifyWaiting returns the Waiting hook that reports a verify from the marker at markerPath, and falls through to next otherwise.
// A running verify reads as `<producer>: verify running <elapsed> (<detail>)`.
// A verify waiting for a gate slot reads as `<producer>: verify waiting for a gate slot <elapsed> (<site>)`.
// When holders names any, a `; holders: <worktree> <site>, ...` clause is added to the detail.
// A verify held by a pause reads as `<producer>: verify held by a pause <elapsed> (<site>)`.
// Elapsed counts from the run's start while it runs, and from the wait's or hold's start otherwise.
// The holders clause is dropped when holders names none or fails.
// The note names the run's current producer.
// A marker whose pid is dead reads as absent, so a crashed verify never sticks in the status line.
func verifyWaiting(markerPath string, holders func() ([]gateslot.Holder, error), now func() time.Time, next func(st shedengine.Status) (shedverbs.WaitNote, error)) func(st shedengine.Status) (shedverbs.WaitNote, error) {
	return func(st shedengine.Status) (shedverbs.WaitNote, error) {
		m, live, err := verifytree.ReadMarker(markerPath)
		if err != nil {
			return shedverbs.WaitNote{}, err
		}
		if !live {
			return next(st)
		}
		if m.State == verifytree.MarkerStatePaused {
			return timedNote(func(shown string) string {
				return fmt.Sprintf("%s: verify held by a pause %s (%s)", st.CurrentProducer, shown, m.Site)
			}, now().Sub(m.WaitStarted)), nil
		}
		if m.State == verifytree.MarkerStateWaiting {
			detail := m.Site
			if named := readHolders(holders); named != "" {
				detail += "; holders: " + named
			}
			return timedNote(func(shown string) string {
				return fmt.Sprintf("%s: verify waiting for a gate slot %s (%s)", st.CurrentProducer, shown, detail)
			}, now().Sub(m.WaitStarted)), nil
		}
		var detail []string
		if m.Attempt > 0 {
			detail = append(detail, fmt.Sprintf("attempt %d", m.Attempt))
		}
		detail = append(detail, m.Command)
		return formatWaitNote(st.CurrentProducer, "verify", now().Sub(m.Started), strings.Join(detail, "; ")), nil
	}
}

// shuttleWaiting returns the Waiting hook that reports a shuttle wait from the marker readMarker finds, and falls through to next otherwise.
// The note names the run's current producer and the wait's kind, with no detail clause.
// readMarker returns false for a marker whose pid is dead,
// so a crashed wait never sticks in the status line.
func shuttleWaiting(readMarker func() (shuttleengine.WaitMarker, bool, error), now func() time.Time, next func(st shedengine.Status) (shedverbs.WaitNote, error)) func(st shedengine.Status) (shedverbs.WaitNote, error) {
	return func(st shedengine.Status) (shedverbs.WaitNote, error) {
		marker, live, err := readMarker()
		if err != nil {
			return shedverbs.WaitNote{}, err
		}
		if !live {
			return next(st)
		}
		return formatWaitNote(st.CurrentProducer, marker.Kind, now().Sub(marker.Started), ""), nil
	}
}

// parentLockWaiting returns the Waiting hook that reports a live parent lock wait from the record at waitPath, and falls through to next otherwise.
// The note reads `<producer>: waiting for the parent lock on <branch> held by <slug> <elapsed>`.
// Elapsed counts from the record's start.
// A record whose writer `alive` reports as not alive reads as absent, so a wait left by a dead step is ignored.
func parentLockWaiting(waitPath string, alive func(pid int, startTime string) bool, now func() time.Time, next func(st shedengine.Status) (shedverbs.WaitNote, error)) func(st shedengine.Status) (shedverbs.WaitNote, error) {
	return func(st shedengine.Status) (shedverbs.WaitNote, error) {
		wait, found, err := parentlock.ReadWait(waitPath, alive)
		if err != nil {
			return shedverbs.WaitNote{}, err
		}
		if !found {
			return next(st)
		}
		return timedNote(func(shown string) string {
			return fmt.Sprintf("%s: waiting for the parent lock on %s held by %s %s", st.CurrentProducer, wait.Branch, wait.Holder, shown)
		}, now().Sub(wait.Started)), nil
	}
}

// reviewWaitingFor builds the Waiting hook for loc's worktree, or nil when no location is wired.
// A gate wait is reported first and never hides the rest.
// After it come a parent lock wait, a verify and a shuttle wait; the first live one wins.
func reviewWaitingFor(loc *lymcwd.Location) func(st shedengine.Status) (shedverbs.WaitNote, error) {
	if loc == nil {
		return nil
	}
	noWait := func(shedengine.Status) (shedverbs.WaitNote, error) { return shedverbs.WaitNote{}, nil }
	readShuttleMarker := func() (shuttleengine.WaitMarker, bool, error) {
		cfg, err := shuttleengine.LoadConfig(fabricengine.ConfigDirs(loc), "shuttle")
		if err != nil {
			return shuttleengine.WaitMarker{}, false, err
		}
		return shuttleengine.ReadWaitMarker(cfg, loc.AnchorPath())
	}
	markerPath := verifytree.NewPaths(loc.WorktreePath(), verifytree.Dir(loc.AnchorPath())).Marker
	holders := hubgeom.GateSlots(loc).Holders
	verify := verifyWaiting(markerPath, holders, time.Now, shuttleWaiting(readShuttleMarker, time.Now, noWait))
	parentLock := parentLockWaiting(parentlock.WaitRecordPath(loomengine.LoomScratchDir(loc)), parentlock.ProcessAlive, time.Now, verify)
	return gateWaiting(gateslot.WaitDir(loc.AnchorPath()), holders, time.Now, parentLock)
}
