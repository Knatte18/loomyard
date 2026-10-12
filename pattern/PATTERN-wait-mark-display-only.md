# PATTERN-wait-mark-display-only

The `@lyx_wait` and `@lyx_wait_start` pane options mark a wait on screen and nothing else.

- They are set only through reed's `SetWaitMark`, by `shuttleengine` and the landing wait-mark callback, which marks the verify and the parent lock wait with the label `parent lock <branch>`.
- No Go decision reads them.
