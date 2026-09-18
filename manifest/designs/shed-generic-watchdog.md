# generalize `ly-supervise` and loom's `run`/`drive`/`step` CLI verbs into a Shed-generic watchdog

> **Status: Next Up, genuinely speculative.**

## The idea

`shedengine`/`shedbuild`/`shedrecipe` are already fully generic — the Told-Geometry Invariant — and `loomrecipe.New` is the only loom-specific glue. `internal/loomcli`'s `run.go`/`drive.go`/`step.go` are the one place hardcoding loom's own recipe/paths, and no sibling CLI package has an equivalent `run`/`drive`/`step` trio.

`ly-supervise` and these three verbs could, in principle, become a watchdog over any `Shed`, not just loom's — since loom is really just `Shed` plus a specific recipe.

## Why not Planned

Genuinely speculative until a second `shedrecipe` consumer beyond loom exists to validate the generalization against — nothing today proves the abstraction is right instead of just theoretically possible.

## Related

- Separate from the Next Up [`loom CLI: rename run/drive/step`](loom-cli-rename.md) item — orthogonal axis of change (genericity vs. naming).
- Separate from the Next Up [`reed: born-as-strand`](reed-born-as-strand.md) item.
