# Finalize — Shed's merge-back step

> **Status: Design — not built.** The merge primitive it depends on now exists (see the Done `fabric: merge-conflict primitive` item in `manifest/roadmap.md`); `Finalize` itself is still the Planned `loom: phase-machine scaffolding` item's own row, not built yet. Renamed from `loom-finalize.md`: `Finalize` is an ordinary producer both `loom`'s and `Hardener`'s producer lists name — one definition, named twice, never copied, never `Shed`-special-cased (see [shed.md](shed.md)). Per the [documentation lifecycle](../../docs/overview.md#documentation-lifecycle), durable parts fold into the relevant package doc and this file deletes when Finalize lands.

## What it does

**Vital, not deferred** (unlike Raddle).
Go-first: the happy path (no conflicts) is pure Go — squash, push, done, zero LLM cost.
An LLM is spawned only on merge conflict, escalating to a **fresh, higher-capability model in a clean session** (see `internal/websterengine`'s package documentation) — not a `/model` switch inside a polluted one.

`lyx fabric cleanup` owns worktree/branch/junction/portal teardown — out of scope here, same reason `mill-cleanup` runs from the hub, never a task worktree.

## Finalize sees only Fabric — never "warp" or "weft"

`Finalize` is not in the Fabric Vocabulary Invariant's owner set (`CONSTRAINTS.md`), so per that invariant it does not know weft exists.
Fabric hands it the one conflict-artifact shape that ships: an ordinary git conflict, reported as unified, worktree-relative paths (`Fabric.MergeIn`/`Fabric.Merge`).
`Finalize` resolves it like any git conflict, concludes with `Fabric.MergeContinue`, or discards with `Fabric.MergeAbort`.

A conflict Fabric cannot express in its unified namespace at all — a path neither side's mapping can resolve — does not hand Finalize a document to reconcile by hand.
Fabric refuses the whole attempt with `*fabricengine.ErrUnmergeableState` instead, restoring both sides and requiring operator intervention below the `Finalize` layer.
A second, precomputed "discrepancy document" shape was sketched early on but is not built — see the `fabric: merge-conflict primitive` item's Someday follow-up in `manifest/roadmap.md`.

## Only Raddle's output forwards to the parent

`_lyx` commits into every task's own Fabric branch by design (`internal/fabricengine`'s package documentation; [Fabric Git Invariant](../../CONSTRAINTS.md#fabric-git-invariant-warp--weft)) and never propagates to the parent on its own.
Fabric's own merge is content-blind and ordinary — it has no `["_lyx"]`-scoped commit call, and none is being added for this.
Instead, forwarding only Raddle's regenerated output ([raddle.md](raddle.md#when-it-runs-deferred-to-merge-time-not-mid-task)) is `Finalize`'s own content policy: before calling Fabric, `Finalize` deletes or unwires whatever task-local content must not cross the merge boundary, then calls an ordinary `Fabric.MergeIn`/`Fabric.Merge` that has no idea anything was filtered.

Raddle and (eventually) `scout`'s index regenerate at merge-time rather than being diffed across branches ([raddle.md](raddle.md)) — the discrepancy-document shape above mainly matters for hand/LLM-authored content like `PATTERN.md`, not Raddle's own output.

## Raddle regeneration — part of the merge, not a step before it

Scoped inside the merge's own critical section, not a separate producer ([shed.md](shed.md), [loom.md](loom.md#the-phase-machine--a-flat-producer-list-no-predefined-slots)): read parent HEAD, run leaf-fork + `Overview.md` regeneration against it, commit — one atomic lock span, never released mid-way.
See [raddle.md](raddle.md) for the regeneration mechanics themselves.
Alternative considered and rejected for now: Raddle as its own `Shed` producer, with merge-in/locking lifted into `Shed` — candidate for a future task.

## PR creation, when configured

If `require_pr_to_base` is set, the PR title/body is dumped **verbatim** from Webster's own summary artifact ([webster-spec.md](../../contracts/specs/webster-spec.md#the-summary-artifact--_lyxwebstersummarymd)) — no LLM call in Finalize itself.

## Config

`finalize.yaml` holds safe defaults (e.g. no direct merge to main without a PR); `loom.yaml`/`hardener.yaml` override per orchestrated run — same shape as the existing "profiles live in the caller, not the callee" precedent.

## Related

- [shed.md](shed.md) — the generic phase-FSM `Finalize` is a producer within.
- [loom.md](loom.md) — `loom`'s concrete producer list built on `Shed`.
- [raddle.md](raddle.md) — regeneration mechanics the sections above point at.
- [webster-spec.md](../../contracts/specs/webster-spec.md#the-summary-artifact--_lyxwebstersummarymd) — the PR-body source artifact.
- [`internal/fabricengine`](../../internal/fabricengine/doc.go) — `Diff`/`Status`/correspondence tracking, and the merge-conflict primitive (`MergeIn`/`Merge`/`MergeContinue`/`MergeAbort`/`MergeInProgress`) this doc depends on.
