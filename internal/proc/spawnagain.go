// spawnagain.go holds the test for a process start the host refused for a temporary resource shortage, and this process's record of the last such refusal.

package proc

import (
	"errors"
	"sync/atomic"
	"syscall"
	"time"
)

// lastSpawnAgain is the Unix-nanosecond time NoteSpawnAgain last recorded a refusal, zero before the first.
var lastSpawnAgain atomic.Int64

// IsSpawnAgain reports whether err is a refusal by the host to start a process because it was temporarily out of a resource, EAGAIN from fork or exec.
// A process that started and exited nonzero never matches, so a caller may start the same command again without running it twice.
func IsSpawnAgain(err error) bool {
	return errors.Is(err, syscall.EAGAIN)
}

// NoteSpawnAgain records now as the time this process last saw a spawn refusal when err is one, and reports whether it was.
// The record lets a caller that sees a later failure judge whether a host resource shortage preceded it.
func NoteSpawnAgain(err error) bool {
	if !IsSpawnAgain(err) {
		return false
	}
	lastSpawnAgain.Store(time.Now().UnixNano())
	return true
}

// LastSpawnAgain returns the time NoteSpawnAgain last recorded a refusal in this process, and false when it recorded none.
func LastSpawnAgain() (time.Time, bool) {
	nanos := lastSpawnAgain.Load()
	if nanos == 0 {
		return time.Time{}, false
	}
	return time.Unix(0, nanos), true
}
