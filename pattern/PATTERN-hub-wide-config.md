# PATTERN-hub-wide-config

A config module that describes a hub-level fact is marked `HubWide` in `configreg`.

- Its file is read and written only at `<BoardDir>/_lyx/config/`.
- It is never read or written in a worktree's `_lyx/config/`.
- `<BoardDir>/_lyx/config/` also holds each per-worktree module's repo default, the base layer of that module's stack; `configreg.Module.HubWide` tells the two kinds of file apart.
- A hub-wide module also reads the reading worktree's local override, restricted to the module's `LocalKeys`.
