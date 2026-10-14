// doc.go carries the package-level doc comment for internal/selfexec.

// Package selfexec is the self-re-exec helper a long-lived process calls when its binary has been replaced on disk.
// It is a leaf over the standard library and internal/logger.
//
// New resolves the process's executable path once and records that path with the file's identity: device, inode, size and modification time.
// Check re-stats the recorded path and never re-resolves it, since on Linux a replaced binary's /proc/self/exe names a deleted file.
//
// A changed identity is acted on only when it is seen on two consecutive checks, so a binary caught mid-write is not exec'd.
// A failed stat counts as unchanged, logs at Debug and resets that streak.
// The exec replaces the process with the recorded path, its own os.Args and its own environment, logging at Info first; that is the spawn's log.
// A failed exec logs at Warn and marks that identity as failed, so no later check execs for it again; a different identity is tried again after its own two checks.
// Nothing a poll repeats logs above Debug.
//
// Two guards keep the helper from replacing the wrong process.
// Under go test the exec seam returns an error instead of exec'ing, so a test binary is never replaced.
// On Windows there is no exec: the identity stat fails, every Check reads as unchanged and the process keeps running.
//
// Callers are long-lived processes, the pane processes and the orch watcher daemon; each calls Check from its own poll and the package adds no wake of its own (PATTERN-daemon-wakeups).
//
// Leaf rule: production code imports the standard library and internal/logger only, enforced by leaf_enforcement_test.go.
// See PATTERN-leaf-packages.
package selfexec
