// byname.go resolves a strand's fixed display name to its guid, and waits out a parent process, for
// `lyx reed remove --name`. A strand that removes itself (the ly-drive driver) cannot know its own
// guid when its prompt is composed, but it does know the name loom gave it.

package reedengine

import (
	"fmt"
	"time"
)

// Parent-exit wait tuning for WaitPIDGone: the cap is on attempt COUNT, not only elapsed time, per
// the Live-Substrate Spawn Observability invariant's retry-loop clause.
const (
	// ParentExitMaxAttempts caps the liveness polls before WaitPIDGone gives up.
	ParentExitMaxAttempts = 100
	// ParentExitPollInterval is the sleep between two liveness polls.
	ParentExitPollInterval = 100 * time.Millisecond
)

// ResolveStrandGUID returns the guid of the one strand of this worktree's session whose display name
// is name. An unknown name and an ambiguous one (two strands carrying it) are both refused, so a
// removal never lands on a guess.
func (e *Engine) ResolveStrandGUID(name string) (string, error) {
	res, err := e.Status()
	if err != nil {
		return "", err
	}
	return guidByName(res.Strands, name)
}

// guidByName is ResolveStrandGUID's pure lookup over an already-read strand list.
func guidByName(strands []StrandStatus, name string) (string, error) {
	var matches []StrandStatus
	for _, s := range strands {
		if s.Name == name {
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

// WaitPIDGone polls alive(pid) up to maxAttempts times, sleeping interval between polls, and reports
// whether the process was seen gone. Liveness, not a parent-pid change, is the predicate because
// Windows never reparents an orphan. A false return means the cap was reached with the process
// still alive.
func WaitPIDGone(pid int, alive func(int) bool, maxAttempts int, interval time.Duration) bool {
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if !alive(pid) {
			return true
		}
		time.Sleep(interval)
	}
	return !alive(pid)
}
