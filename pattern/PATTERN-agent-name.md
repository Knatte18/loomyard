# PATTERN-agent-name

`internal/agentname` is the sole former, parser and validator of agent names, and imports the standard library only.

- A full name is `<shortname>:<role>` in the prime and in a standalone run, and `<shortname>:<slug>:<role>` in a task worktree.
- Reed forms it once, in `AddStrand` under its state lock, and stores it in the strand record, which is the only key delivery and lookup resolve through.
  Pane titles and Claude session names are display mirrors.
- A full name never names a tmux session or socket, since tmux rewrites `:` and `.` there.
- A run's parent is the worktree its pair was created from, recorded once in fabric's origin record.
  Its agent name is resolved at use as the orch role's name in that worktree through `internal/agentname`, and is never stored in a seed or read from the caller's environment.
- The hub's shortname is recorded in `.lyx-shortname` beside `.lyx-warp`, read and written by `internal/fabricengine` alone, and told to reed by `internal/hubgeom`.
  A standalone run's derived shortname is told by `internal/standalonegeom`.
