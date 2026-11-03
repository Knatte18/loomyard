// pushlockholder.go holds the holder record of fabric's records push lock, fabric.push.lock:
// the process that holds the lock writes its pid and the time it took the lock beside it, and a bounded push that gives up on the lock names that holder.

package fabricengine

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/fsx"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
)

// pushLockHolderFile names the holder record beside weftPushLockFile in the records worktree's lock directory.
const pushLockHolderFile = "fabric.push.holder"

// pushLockHolder is the record a process writes beside fabric.push.lock while it holds the lock.
type pushLockHolder struct {
	// PID is the holding process's id.
	PID int `json:"pid"`
	// Since is when the process took the lock.
	Since time.Time `json:"since"`
}

// pushLockHolderPath returns the path of the holder record in lockDir, the lock directory that holds fabric.push.lock.
func pushLockHolderPath(lockDir string) string {
	return filepath.Join(lockDir, pushLockHolderFile)
}

// holdPushLock writes this process's holder record, taken at now, beside the push lock in lockDir that held guards, and returns the release of both.
// The release removes the record before it releases the lock, so a holder never removes the record of the holder after it.
// A record that cannot be written or removed is logged at Warn and the lock is still held and released, since the record only feeds a busy report.
func holdPushLock(held *lock.FileLock, lockDir string, now time.Time) func() error {
	path := pushLockHolderPath(lockDir)
	content, err := json.Marshal(pushLockHolder{PID: os.Getpid(), Since: now.UTC()})
	if err == nil {
		err = fsx.AtomicWriteBytes(path, content)
	}
	if err != nil {
		logger.Warn("fabricengine: push lock holder record not written; a busy report cannot name this holder", "path", path, "error", err)
	}
	return func() error {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			logger.Warn("fabricengine: push lock holder record not removed; the next holder overwrites it", "path", path, "error", err)
		}
		return held.Release()
	}
}

// describePushLockHolder returns the clause of a busy report that names the holder of the push lock in lockDir, read from its holder record at now.
// alive reports whether a pid names a running process.
// A record that decodes with no pid or no time it was taken is reported as unreadable.
// The push lock is an OS lock its holder's exit frees, so a record naming a pid for which no running process is found may be stale.
func describePushLockHolder(lockDir string, now time.Time, alive func(pid int) bool) string {
	content, err := os.ReadFile(pushLockHolderPath(lockDir))
	if errors.Is(err, os.ErrNotExist) {
		return "no holder record names the process that holds it"
	}
	var holder pushLockHolder
	if err == nil {
		err = json.Unmarshal(content, &holder)
	}
	if err == nil && (holder.PID <= 0 || holder.Since.IsZero()) {
		err = errors.New("it names no pid or no time the lock was taken")
	}
	if err != nil {
		return fmt.Sprintf("its holder record is unreadable: %v", err)
	}
	age := now.Sub(holder.Since).Round(time.Second)
	if !alive(holder.PID) {
		return fmt.Sprintf("its holder record names pid %d from %s ago, no running process by that pid was found, and the record may be stale", holder.PID, age)
	}
	return fmt.Sprintf("held by pid %d for %s", holder.PID, age)
}
