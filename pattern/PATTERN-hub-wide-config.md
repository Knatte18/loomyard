# PATTERN-hub-wide-config

A config module that describes a hub-level fact is marked `HubWide` in `configreg`.

- It is read and written only at `<BoardDir>/_lyx/config/`.
- It is never read or written in a worktree's `_lyx/config/`.
