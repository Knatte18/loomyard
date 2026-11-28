// pushlock.go holds the rules of the records push lock.
// They describe its holder for a busy report.
// They also decide whether a holder's step moved a side and what the holder loop does after an acquire.

package fabriccore

import (
	"errors"
	"fmt"
	"time"
)

// PushLockHolder is the record a process writes beside the push lock while it holds the lock.
type PushLockHolder struct {
	// PID is the holding process's id.
	PID int `json:"pid"`
	// Since is when the process took the lock.
	Since time.Time `json:"since"`
}

// DescribePushLockHolder returns the clause of a busy report that names the push lock holder.
// found reports whether a holder record exists, and readErr why an existing one could not be read.
// The age of the record is judged against now, and alive reports whether its pid names a running process.
// A record with no pid or no time it was taken is reported as unreadable.
// The push lock is an OS lock its holder's exit frees,
// so a record naming a pid for which no running process is found may be stale.
func DescribePushLockHolder(holder PushLockHolder, found bool, readErr error, now time.Time, alive bool) string {
	if !found {
		return "no holder record names the process that holds it"
	}
	if readErr == nil && (holder.PID <= 0 || holder.Since.IsZero()) {
		readErr = errors.New("it names no pid or no time the lock was taken")
	}
	if readErr != nil {
		return fmt.Sprintf("its holder record is unreadable: %v", readErr)
	}
	age := now.Sub(holder.Since).Round(time.Second)
	if !alive {
		return fmt.Sprintf("its holder record names pid %d from %s ago, no running process by that pid was found, and the record may be stale", holder.PID, age)
	}
	return fmt.Sprintf("held by pid %d for %s", holder.PID, age)
}

// SidesProgressed reports whether any side's HEAD in after differs from the one in before.
// A side a map does not hold reads as an empty HEAD.
func SidesProgressed(before, after map[string]string) bool {
	for side, head := range after {
		if before[side] != head {
			return true
		}
	}
	for side, head := range before {
		if after[side] != head {
			return true
		}
	}
	return false
}

// FlaggedLoopNext decides what the holder loop of the push lock does after trying to acquire it.
// stepRan reports whether a step ran on an earlier turn, and acquired whether this turn took the lock and ran its step.
// ownedFlagSet reports whether, after the release, a pending flag of a side the holder owes is set again.
// again reports whether the loop goes back to acquiring.
// reportAcquired is what the loop reports once it ends:
// a missed first acquire reports false, which leaves the flags to the current holder,
// and a missed re-acquire reports true, since an earlier step ran.
func FlaggedLoopNext(stepRan, acquired, ownedFlagSet bool) (again, reportAcquired bool) {
	if !acquired {
		return false, stepRan
	}
	return ownedFlagSet, true
}
