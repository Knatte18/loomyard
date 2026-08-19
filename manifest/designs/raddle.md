# raddle — codeguide's woven-in successor (Someday, deprioritized)

> **Status: Design partially exists, not scheduled.** Deprioritized — not required to land a first `loom` plan. Raddle-regeneration is folded into `Finalize`'s own contract, not a reserved phase slot of its own (see [loom.md](loom.md#the-phase-machine--a-flat-producer-list-no-predefined-slots)). This doc covers the parts of raddle's design settled during the vacation-time discussion, not the whole module.

## What it is

Raddle is codeguide's weaving-vocabulary successor, living in `weft`: it generates docs over the diff a plan produced, building heavily on Millhouse's `codeguide-update`, deliberately not the implementer's job — implementers, busy with code, forget the docs.
See [When it runs](#when-it-runs-deferred-to-merge-time-not-mid-task) below for when regeneration actually happens.

## Geometry — where raddle content lives

Every raddle file lives under `_lyx/raddle/` inside each worktree, mirroring that worktree's own code tree.
It is resolved by plain path lookup joined onto the anchor path, with no junction of its own and no hub-level presence at all.
It reaches the weft through the already-wired `_lyx` junction, like every other `_lyx` subtree.

`_raddle` is **not** a reserved hub name and is never junction-reached.

Raddle content is tracked `_lyx` content and therefore needs no `.lyx` mirror, per the Durable-vs-Ephemeral State Invariant.

This section records the geometry only — raddle's actual implementation, including the shadow tree's path-lookup code and any accessor, is explicitly out of scope here.

## Parallel regeneration — unlike card implementation, this is safe

Unlike webster's card implementation (which needs real worktree isolation for safe parallelism — see [webster-parallel-execution.md](webster-parallel-execution.md)), raddle's doc regeneration can safely run as **parallel forks**, for reasons specific to what raddle does:

1. **No per-fork git commit needed mid-flight.**
   Card parallelism failed partly because concurrent `git add`+commit from multiple forks race on the same git index.
   Raddle forks don't need this — each fork writes its own module doc to disk;
   **one combining commit** happens at the end (`fabric.SyncWeft(...)`, see [`internal/fabricengine`](../../internal/fabricengine/doc.go)), covering all raddle changes at once.
   This eliminates the git-index race entirely.
2. **Raddle only reads code, never writes it.**
   Card forks editing code concurrently risked seeing each other's unstable, half-finished intermediate states.
   Raddle forks read an already-landed, stable code state — no "who sees what when" problem.
3. **True file-disjointness is easy to guarantee.**
   With a clean module → doc mapping (one changed module → one raddle `.md` file), there's no ambiguity about who writes where, unlike card `changes-files` declarations which could turn out wrong after the fact.

**Structure:** parallel leaf forks (one per changed module, no dependencies between them) → one final step regenerating any top-level summary (`Overview.md`) from the now-updated module docs (must wait for all leaves) → one combined `fabric.SyncWeft(...)` commit covering everything.

**No race on `Overview.md`:** the "must wait for all leaves" sequencing above prevents concurrent writers to `Overview.md` within one regeneration run — it's a single, serial final step, never touched by the leaf forks themselves.
No additional lock needed there.

## When it runs: deferred to merge-time, not mid-task

Regenerating raddle is token-heavy and takes real wall-clock time, so it should run **once**, not twice.
Running it right after Webster (against the task's own fork-point) and then again at actual merge (against parent's real, possibly-since-moved HEAD) would do exactly that: two regenerations, the first thrown away the moment parent has advanced.

**Decision: raddle regenerates once, at merge-time, against parent's actual current HEAD** — not mid-task.
This collapses the two potential runs into one and guarantees the output describes the real merge result, not a stale fork-point.

**Consequence for the merge lock:** the lock protecting this step must span the **entire** critical section — read parent's current HEAD, run the leaf-fork + `Overview.md` regeneration against it, commit via `SyncWeft` — as one atomic unit, not just around the final git write.
If another task's merge landed in parent partway through regeneration, the docs would be stale against the HEAD they're about to be committed onto.
Same "advance only on confirmed success" discipline the `Warp-SHA` trailer mechanism uses elsewhere in `fabric` for recording a baseline, extended to cover the compute step, not just the write step.

**Decided:** raddle has no reserved phase slot of its own in [loom.md](loom.md#the-phase-machine--a-flat-producer-list-no-predefined-slots) — regeneration is folded into `Finalize`'s own contract instead, landed at [shed.md](shed.md) and [loom.md](loom.md#the-phase-machine--a-flat-producer-list-no-predefined-slots).
See [landing.md](landing.md#raddle-regeneration--part-of-the-merge-not-a-step-before-it) for Finalize's side of the contract.

## Staleness tracking, via `fabric`

Raddle's snapshot files describe the codebase as of some SHA, and only get regenerated by an explicit, separate command — they can silently drift out of date as cards land.
Using `fabric`'s Warp-SHA trailer mechanism (see [`internal/fabricengine`](../../internal/fabricengine/doc.go)):

- Raddle's regeneration command records its baseline by passing `snapshotTags=["raddle"]` to `Fabric.Commit`, alongside whatever raddle files the regeneration produced.
  **The warp SHA recorded should be the warp code SHA raddle describes — the last warp commit *before* the regeneration — not raddle's own resulting weft commit SHA**, otherwise later staleness checks compute against the wrong baseline;
  the `Warp-SHA` trailer holds exactly that value, so the trailer form satisfies this requirement naturally.
- A regeneration that finds the codebase unchanged since its last recorded baseline still needs to advance that baseline, even though it produces no weft content — this is exactly the case that motivated the empty-commit rule (see the tags-force-a-weft-commit Shared Decision in [`internal/fabricengine`](../../internal/fabricengine/doc.go)): tagging a `Commit` call with `snapshotTags=["raddle"]` and no changed files still lands an empty weft commit carrying the baseline, so a no-op regeneration advances the baseline instead of reporting drift forever.
- A staleness check is the **three-step** idiom `Fabric.SnapshotWarpSHA`'s own doc comment specifies, not the naive two-step composition: read the baseline via `SnapshotWarpSHA("raddle")`, confirm it still resolves via `f.Warp.SHAExists`, and only then call `f.Warp.ChangedFilesSince` to get a precise answer — *"raddle's map is N commits behind current HEAD, covering these files"* — instead of a vague, always-true warning that raddle "might" be outdated.
  Skipping the `SHAExists` check and calling `ChangedFilesSince` directly hard-errors on a warp SHA a rebase has since rewritten, exactly the case this mechanism exists to survive: the check lets a post-rebase raddle regenerate cleanly instead.

## Boundary with scout (deliberately not coupled)

- **scout** only cares about source code files, never markdown, and has **no knowledge of raddle whatsoever**.
- **Raddle never modifies code and therefore has no reason to notify scout** — that coupling was proposed once during design and correctly rejected.
  Raddle's own use of `fabric` is purely for its own staleness tracking, unrelated to scout.
- Both consumers independently depend on `fabric`;
  they do not depend on each other.

## What Master must know about raddle content (see `internal/websterengine`'s package documentation)

Raddle files are a **snapshot** of the codebase from *before* a plan started — they are only regenerated by a separate, explicit command, not continuously.
Master (and any fork inheriting its context) must treat raddle content as "how things were before this plan," never outranking a fresh scout query or an actual file read once cards have started landing.

## Related

- [`internal/fabricengine`](../../internal/fabricengine/doc.go) — the `Warp-SHA`/`Snapshot` trailer and `SyncWeft` mechanics this design relies on.
- [loom.md](loom.md#the-phase-machine--a-flat-producer-list-no-predefined-slots) — the flat producer list Raddle has no slot of its own in;
  regeneration is folded into `Finalize`'s contract instead.
- The `internal/boardengine` package documentation — `PATTERN.md` (raddle's neighbor in `weft`) mentioned there.
