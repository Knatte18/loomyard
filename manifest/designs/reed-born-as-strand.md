# reed: born-as-strand for the operator's `loom start` attach

> **Status: Next Up, not yet built.**

## The problem

Every LLM agent lyx's own Go code launches goes through `internal/shuttleengine.Runner`, whose `Start` calls `AddStrand` unconditionally — with one exception. `loom start`'s own terminal handoff does a bare `tmux attach-session` today, with no `AddStrand` call at all, so it never becomes a Strand no matter how it's launched.

This matters once reed grows features that only work for Strands (the watchdog daemon's reap/reconcile loop, and eventually `reed: strand-based mailbox/addressing system`) — an un-tracked pane is invisible to all of it.

## The fix

Spawn the pane as a Strand first, then attach — mirroring what `reed add` already does for everything else. Never adopt an already-running pane after the fact: `internal/reedengine/spawn.go`'s `planPaneTarget` doc comment records two real bugs from a prior adoption attempt (**R4-F5**: adopted the old header pane, typed a command into it that never ran, but reported `live:true`; **M16**: claimed an operator's own manually-split pane) — adoption was tried and deliberately removed, not merely unbuilt.

Running `loom start` outside reed remains a valid escape hatch for debugging/CI, mirroring `loom run`'s relationship to `loom start` — the "escape hatch" pattern is already established.

## Open items

- Exactly what "repo-orchestrator" means as a concept is not yet defined.
- Whether reed's core pane lifecycle is solid enough yet to trust with this depends on the header-pane split (see [reed-header-selvage.md](reed-header-selvage.md)) landing first.

## Related

- `ly-drive + orchestrator: launch via lyx reed add` covers the watchdog and orchestrator halves of "born as a Strand" — this item is the one remaining code gap, for the operator's own `loom start` attach.
- `reed: strand-based mailbox/addressing system` depends on this one for its receive side.
- Distinct from `AddStrand`/`attach` self-heal: that one is about a session not existing yet; this one is about a pane never becoming a Strand at all, even once a session exists.
