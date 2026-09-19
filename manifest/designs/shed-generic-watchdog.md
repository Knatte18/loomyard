# generalize `ly-drive` and loom's `start`/`run`/`step` CLI verbs into a Shed-generic watchdog

## The idea

`shedengine`/`shedbuild`/`shedrecipe` are already fully generic — the Told-Geometry Invariant — and `loomrecipe.New` is the only loom-specific glue. `internal/loomcli`'s `start.go`/`run.go`/`step.go` are the one place hardcoding loom's own recipe/paths, and no sibling CLI package has an equivalent `start`/`run`/`step` trio.

`ly-drive` and these three verbs could, in principle, become a watchdog over any `Shed`, not just loom's — since loom is really just `Shed` plus a specific recipe.

## The shape it would take

Even speculatively, four things about the generalized end state are settled by this task's own rename.

First, the three verbs generalise rather than collapse: the end state is a verb set on a generic `shed`, armed with an FSM recipe supplied from outside — from the task description or an equivalent carrier — rather than a single merged watchdog command.

Second, under that set the verb/engine symmetry this task established becomes literal, since the generic `run` calls `Shed.Run` and the generic `step` calls `Shed.Step`.
Landing the rename first is what makes this true, because the pre-rename names would have carried the asymmetry into the abstraction, where it is harder to unpick than in `loomcli` alone.

Third, `start` is the one verb with no engine counterpart.
There is no `Shed.Start`, because it seeds, commits, spawns the detached driver, and hands the terminal over, all of which sit above the engine — so whether it belongs on a generic `shed` at all, or stays loom-specific, is itself open.

Fourth, the skill's role in that end state is to loop the generic `step` until the recipe's own producer list is exhausted or the run reaches a non-`running` state, which is what it already does over `lyx loom step` today.

This section describes a shape, not a commitment: the verdict below is unchanged, and a second `shedrecipe` consumer beyond loom is still what the generalisation needs before it can be validated.

## Why still speculative

Genuinely speculative until a second `shedrecipe` consumer beyond loom exists to validate the generalization against — nothing today proves the abstraction is right instead of just theoretically possible.

## Related

- Separate from the loom CLI rename (`run`/`drive`/`step` renamed to `start`/`run`/`step`, `ly-supervise` renamed to `ly-drive`) — orthogonal axis of change (genericity vs. naming), and shipped.
- Separate from the shipped `reed: born-as-strand` item.
