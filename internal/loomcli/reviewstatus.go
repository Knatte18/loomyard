// reviewstatus.go fills shedverbs.Hooks.Waiting for loom, so `lyx loom status` reports a run that waits on its reviewer.

package loomcli

import (
	"fmt"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/parentreview"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/verifytree"
)

// reviewWaitPrefix leads the note so a status reader sees which wait it is.
const reviewWaitPrefix = "parent review: "

// reviewWaiting returns the Waiting hook over a parent-review store rooted at root, with its lock files under lockDir.
// Only the latest round is reported, and an absent root yields an empty note rather than an error.
func reviewWaiting(root, lockDir string) func() (string, error) {
	store := parentreview.Store{Root: root, LockDir: lockDir}
	return func() (string, error) {
		note, err := store.WaitNote()
		if err != nil || note == "" {
			return "", err
		}
		return reviewWaitPrefix + note, nil
	}
}

// formatElapsed renders d as whole seconds under a minute, whole minutes under an hour and hours with minutes beyond: `45s`, `6m`, `1h12m`.
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

// formatWaitNote renders one wait as `<producer>: <what> running <elapsed> (<detail>)`, dropping the parenthesis when detail is empty.
func formatWaitNote(producer, what string, elapsed time.Duration, detail string) string {
	note := fmt.Sprintf("%s: %s running %s", producer, what, formatElapsed(elapsed))
	if detail == "" {
		return note
	}
	return note + " (" + detail + ")"
}

// verifyWaiting returns the Waiting hook that reports a running verify from the marker at markerPath, and falls through to next otherwise.
// The note names the run's current producer and measures elapsed time from now.
// A marker whose pid is dead reads as absent, so a crashed verify never sticks in the status line.
func verifyWaiting(markerPath string, now func() time.Time, next func() (string, error)) func(st shedengine.Status) (string, error) {
	return func(st shedengine.Status) (string, error) {
		m, live, err := verifytree.ReadMarker(markerPath)
		if err != nil {
			return "", err
		}
		if !live {
			return next()
		}
		var detail []string
		if m.Attempt > 0 {
			detail = append(detail, fmt.Sprintf("attempt %d", m.Attempt))
		}
		detail = append(detail, m.Command)
		return formatWaitNote(st.CurrentProducer, "verify", now().Sub(m.Started), strings.Join(detail, "; ")), nil
	}
}

// reviewWaitingFor builds the Waiting hook for loc's worktree, or nil when no location is wired.
// A running verify is reported ahead of the parent-review note.
func reviewWaitingFor(loc *lyxcwd.Location) func(st shedengine.Status) (string, error) {
	if loc == nil {
		return nil
	}
	review := reviewWaiting(loomengine.LoomParentReviewDir(loc), loomengine.LoomParentReviewLockDir(loc))
	markerPath := verifytree.NewPaths(loc.WorktreePath(), verifytree.Dir(loc.AnchorPath())).Marker
	return verifyWaiting(markerPath, time.Now, review)
}
