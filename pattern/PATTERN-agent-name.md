# PATTERN-agent-name

`internal/agentname` is the sole former, parser and validator of agent names, and imports the standard library only.

- A full name is `<shortname>:<role>` in the prime and in a standalone run, and `<shortname>:<slug>:<role>` in a task worktree.
- Reed forms it once, in `AddStrand` under its state lock, and stores it in the strand record, which is the only key delivery and lookup resolve through.
  Pane titles and Claude session names are display mirrors.
- The module that spawns an agent owns its role name as a constant declared in that module, never as an inline literal in the spawn spec.
  There is no shared list of roles, and `agentname`'s role set stays open.
  `cmd/lyx/spawnrole_test.go` is a tripwire that fails a `shuttleengine.Spec` or `reedengine.AddSpec` composite literal whose `Role` is a string literal.
- A full name never names a tmux session or socket, since tmux rewrites `:` and `.` there.
- A run's parent is the worktree its pair was created from, recorded once in fabric's origin record.
  Its agent name is resolved at use as the orch role's name in that worktree through `internal/agentname`, and is never stored in a seed or read from the caller's environment.
- The hub's shortname is recorded in `.lyx-shortname` beside `.lyx-warp`, read and written by `internal/fabricengine` alone, and told to reed by `internal/hubgeom`.
  A standalone run's derived shortname is told by `internal/standalonegeom`.
