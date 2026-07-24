# mux → reed (rename)

> **Status: Design — not built.** A pure rename, no behavior change. Per the [documentation
> lifecycle](../../docs/overview.md#documentation-lifecycle), this note folds into
> `internal/reedengine`'s package doc (carried over from `internal/muxengine`) when the rename
> lands, and this file is deleted.

## What changes

`mux` (tmux overlay + strand bookkeeping + render, `internal/muxcli` + `internal/muxengine`)
is renamed to **`reed`** — the comb that keeps warp threads separated in their own path during
weaving, matching mux's job of keeping each parallel strand visible in its own pane.

`harness` was considered and rejected as an alternative — already used at a higher level
(Loomyard describes itself as a Claude Code harness); reusing the word for one module would
create exactly the kind of ambiguity the project has otherwise avoided.

## What doesn't change

Every existing capability (strand bookkeeping, tmux overlay, `.lyx/mux.json` persistence, the
render layer, the `up|add|remove|status|attach|resume|header|down` CLI surface) carries over
unchanged under the new name. This is a rename, not a redesign — `internal/muxcli` →
`internal/reedcli`, `internal/muxengine` → `internal/reedengine`, `lyx mux ...` → `lyx reed ...`.

## Consequence for other planned/Someday items

The three previously-planned `mux:`-prefixed feature items (cross-worktree columns, daemon →
Slack relay, own-window strand anchoring — see the roadmap's Someday list) are renamed to
`reed:`-prefixed alongside this, since they land after the rename (or should be described under
the target name regardless of exact sequencing).

## Related

- [loom.md](loom.md) — references mux/reed as part of the execution stack (`proc` → reed →
  `shuttle`).
