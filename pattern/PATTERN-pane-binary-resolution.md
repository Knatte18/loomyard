# PATTERN-pane-binary-resolution

A strand pane or detached `lyx` window reed creates resolves `lyx` to the binary that spawned it.

- `panebin.go` owns the seam, and its call sites are `launchStrandLocked` for strand panes and `OpenWindow` for the detached `lyx` window.
- Every shell token is emitted through `internal/shell`, per the shell-mechanics-seam entry.
- The dialect is `shell.ForGOOS()`, the same selector as the launch command the prelude is joined onto.
  Reed neither derives a dialect of its own nor changes how a pane's shell is started.
- Scope is strand panes and the detached `lyx` window `OpenWindow` opens, whose command `panebin.go` composes with the same prelude and no strand name exports.
  Selvage's split and the `new-session` first pane are exempt by name.
  The detached `lyx loom run` and watchdog daemon spawns are excluded, because both are already spawned from the executable path and neither resolves `lyx` from `PATH`.
- The prelude and launch command ride a per-strand launch script at `<AnchorPath>/.lyx/reed/launch/<guid><ext>`, which the send-keys line sources through `internal/shell`'s `Source`.
  The script exists only while its GUID is in reed's strand table.
- A script write failure degrades to typing the composed line plus a named `logger.Warn`, never a failed launch.
- An unresolvable executable path degrades to a pane with no prelude plus a named `logger.Warn`, never a failed launch.
- Backed by `internal/reedengine/panebin_enforcement_test.go`, which fails if a `split-window` or `new-window` pane-creation site appears outside its chokepoint and outside the named allowlist.
