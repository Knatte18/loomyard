// spawnagain.go detects the host refusing a process start for a temporary resource shortage.

package proc

import (
	"errors"
	"sync/atomic"
	"syscall"
	"time"
)

// lastSpawnAgain holds the Unix-nanosecond time of the last refusal recorded, or zero before the first.
var lastSpawnAgain atomic.Int64

// IsSpawnAgain reports whether err is a refusal by the host to start a process for lack of a temporary resource.
// The refusal is EAGAIN from fork or exec.
// A process that started and exited nonzero never matches,
// so a caller may start the same command again without running it twice.
func IsSpawnAgain(err error) bool {
	return errors.Is(err, syscall.EAGAIN)
}

// NoteSpawnAgain reports whether err is a refusal by the host to start a process.
// When it is, it records the current time as the last refusal this process saw.
// The record lets a caller that sees a later failure judge whether a host resource shortage preceded it.
func NoteSpawnAgain(err error) bool {
	if !IsSpawnAgain(err) {
		return false
	}
	lastSpawnAgain.Store(time.Now().UnixNano())
	return true
}

// LastSpawnAgain returns the time of the last host refusal to start a process that this process recorded.
// It reports false when this process has recorded none.
func LastSpawnAgain() (time.Time, bool) {
	nanos := lastSpawnAgain.Load()
	if nanos == 0 {
		return time.Time{}, false
	}
	return time.Unix(0, nanos), true
}
