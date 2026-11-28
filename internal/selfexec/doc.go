// doc.go carries the package-level doc comment for internal/selfexec.

// Package selfexec resolves and re-execs the running lym binary.
// A long-lived process calls its Watcher when its binary has been replaced on disk,
// and every command lym builds for another process to run later names its binary through LymBinary.
//
// LymBinary is the one lookup of "this lym binary".
// A pane's launch line, a tmux job or binding, a hook and a detached or child lym process all use it.
// Under go test it refuses with ErrTestBinary instead of returning the test binary's path.
// A test binary handed a lym verb runs its whole suite,
// and a tmux job that suite pins runs it again in every session it starts, without end.
// ResolveLymBinary applies the same refusal to an injectable lookup, so a test's stand-in path still passes.
// A lookup that only reads the binary, such as a build identity or a commit label, calls os.Executable itself.
//
// New resolves the process's executable path once and records that path with the file's identity:
// device, inode, size and modification time.
// Check re-stats the recorded path and never re-resolves it.
// On Linux a replaced binary's /proc/self/exe names a deleted file.
//
// A changed identity is acted on only when it is seen on two consecutive checks.
// A binary caught mid-write is therefore not exec'd.
// A failed stat counts as unchanged, logs at Debug and resets that streak.
// The exec replaces the process with the recorded path, its own os.Args and its own environment.
// It logs at Info first; that is the spawn's log.
// The new image keeps the process's children but none of the goroutines that waited on them,
// so a caller that starts children reaps them again with `proc.ReapInheritedChildren` before it starts one.
// A failed exec logs at Warn and marks that identity as failed, so no later check execs for it again;
// a different identity is tried again after its own two checks.
// Nothing a poll repeats logs above Debug.
//
// Two guards keep the helper from replacing the wrong process.
// Under go test the exec seam returns an error instead of exec'ing, so a test binary is never replaced.
// On Windows there is no exec: the identity stat fails, every Check reads as unchanged and the process keeps running.
//
// CheckWith is Check with two callbacks.
// beforeExec runs before the exec, once the two-sighting rule and the failed-identity rule have passed.
// failed runs with the exec's error after a failed exec, once that identity is marked failed.
// It runs at most once per identity.
// Callers are long-lived processes that call Check from their own poll;
// the package adds no wake of its own (PATTERN-daemon-wakeups).
//
// Leaf rule: production code imports the standard library and internal/logger only.
// leaf_enforcement_test.go enforces it.
// See PATTERN-leaf-packages.
package selfexec
