// byname.go resolves a strand's name to its guid, and waits out a parent process, for `lym reed remove --name`.
// A strand that removes itself (the loom driver) cannot know its own guid when its prompt is composed,
// but it does know its role segment.

package reedengine

import (
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/agentname"
)

// Parent-exit wait tuning for WaitPIDGone.
// The cap counts attempts, not only elapsed time.
// It follows the Live-Substrate Spawn Observability invariant's retry-loop clause.
const (
	// ParentExitMaxAttempts is the number of liveness polls a parent-exit wait makes before it gives up.
	ParentExitMaxAttempts = 100
	// ParentExitPollInterval is the sleep between two liveness polls of a parent-exit wait.
	ParentExitPollInterval = 100 * time.Millisecond
)

// ResolveStrandGUID returns the guid of the one strand of this worktree's session that name addresses:
// its full name, its role segment, or a legacy exact name (agentname.Matches).
// An unknown name and an ambiguous one (two strands carrying it) are both refused, so a removal never lands on a guess.
func (e *Engine) ResolveStrandGUID(name string) (string, error) {
	res, err := e.Status()
	if err != nil {
		return "", err
	}
	return guidByName(res.Strands, name)
}

// guidByName returns the guid of the one strand in strands that name addresses.
// It refuses an unknown name and an ambiguous one.
func guidByName(strands []StrandStatus, name string) (string, error) {
	var matches []StrandStatus
	for _, s := range strands {
		if agentname.Matches(s.Name, name) {
			matches = append(matches, s)
		}
	}
	switch len(matches) {
	case 0:
		return "", fmt.Errorf("no strand named %q in this worktree's reed session", name)
	case 1:
		return matches[0].GUID, nil
	default:
		guids := make([]string, len(matches))
		for i, m := range matches {
			guids[i] = m.GUID
		}
		return "", fmt.Errorf("strand name %q is ambiguous: %d strands carry it (%v); remove by guid instead", name, len(matches), guids)
	}
}

// WaitPIDGone reports whether the process is seen gone within maxAttempts polls of alive(pid).
// It sleeps interval between polls.
// Liveness, not a parent-pid change, is the predicate because Windows never reparents an orphan.
// A false return means the cap was reached with the process still alive.
func WaitPIDGone(pid int, alive func(int) bool, maxAttempts int, interval time.Duration) bool {
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if !alive(pid) {
			return true
		}
		time.Sleep(interval)
	}
	return !alive(pid)
}
