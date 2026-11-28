// pushlockholder.go holds the holder record of fabric's records push lock, fabric.push.lock.
// The process that holds the lock writes its pid and the time it took the lock beside it,
// so a bounded push that gives up on the lock can name that holder.

package fabricengine

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/fabriccore"
	"github.com/Knatte18/loomyard/internal/fsx"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
)

// pushLockHolderFile is the name of the holder record kept beside fabric.push.lock in the lock directory.
const pushLockHolderFile = "fabric.push.holder"

// pushLockHolderPath returns the path of the holder record in lockDir, the lock directory that holds fabric.push.lock.
func pushLockHolderPath(lockDir string) string {
	return filepath.Join(lockDir, pushLockHolderFile)
}

// holdPushLock writes this process's holder record beside the push lock in lockDir and returns the release of both.
// The record holds the pid and the time now.
// The release removes the record before it releases the lock,
// so a holder never removes the record of the holder after it.
// A record that cannot be written or removed is logged at Warn,
// and the lock is still held and released, since the record only feeds a busy report.
func holdPushLock(held *lock.FileLock, lockDir string, now time.Time) func() error {
	path := pushLockHolderPath(lockDir)
	content, err := json.Marshal(fabriccore.PushLockHolder{PID: os.Getpid(), Since: now.UTC()})
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

// describePushLockHolder returns the clause of a busy report that names the push lock holder in lockDir.
// It reads the holder record and judges its age against now.
// alive reports whether a pid names a running process.
func describePushLockHolder(lockDir string, now time.Time, alive func(pid int) bool) string {
	var holder fabriccore.PushLockHolder
	content, err := os.ReadFile(pushLockHolderPath(lockDir))
	if errors.Is(err, os.ErrNotExist) {
		return fabriccore.DescribePushLockHolder(holder, false, nil, now, false)
	}
	if err == nil {
		err = json.Unmarshal(content, &holder)
	}
	running := err == nil && holder.PID > 0 && alive(holder.PID)
	return fabriccore.DescribePushLockHolder(holder, true, err, now, running)
}
