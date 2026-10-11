# PATTERN-wait-mark-display-only

The `@lyx_wait` and `@lyx_wait_start` pane options mark a wait on screen and nothing else.

- They are set only through reed's `SetWaitMark`, by `shuttleengine` and the landing verify callback.
- No Go decision reads them.
