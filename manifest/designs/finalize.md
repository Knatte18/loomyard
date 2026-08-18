# Finalize — Shed's merge-back step

> **Status: Design — not built. Planned, combined with the `Shed` task** (see `manifest/roadmap.md`) — building `Shed`'s skeleton and its Finalize step happen together, the same reasoning as the combined `Treadle` + `perch`-rewrite task. Renamed from `loom-finalize.md`: `Finalize` is an ordinary producer that both `loom`'s and `Hardener`'s producer lists name — one definition, named twice, never copied, and never something `Shed` special-cases (see [shed.md](shed.md)) — not loom-specific, though originally split out of [loom.md](loom.md) as a substantial, self-contained phase spec worth its own file. Per the [documentation lifecycle](../../docs/overview.md#documentation-lifecycle), when this lands the durable parts fold into the relevant package doc and this file is deleted.

## What it does

**Vital, not deferred** (unlike Raddle).
Go-first: the happy path (no conflicts) is pure Go — squash, push, done, zero LLM cost.
An LLM is spawned only on merge conflict (during merge-in from parent, or the merge to parent itself), escalating to a **fresh, higher-capability model in a clean session** (see `internal/websterengine`'s package documentation) — not a `/model` switch inside a polluted one.

Mostly wiring on top of the already-built `fabric` mechanics (see [`internal/fabricengine`](../../internal/fabricengine/doc.go));
worktree/branch/junction/portal teardown is explicitly **out of scope** — that's `lyx fabric cleanup`'s existing, separate job, which cannot run from inside the worktree being removed, the same reason `mill-cleanup` runs from the hub, never a task worktree.

## Fabric hands Finalize one of two conflict shapes — never "warp" or "weft"

**Correction (found in review, not machine-caught):** this section used to be titled "Two merge targets, not one — warp and weft, handled differently" and named "Warp side"/"Weft side" as Finalize's own vocabulary.
That violates the Fabric Vocabulary Invariant's consumer-doc rule (`CONSTRAINTS.md`): `Finalize` is not in the invariant's owner set (`internal/fabricengine`, `internal/fabriccli`, `internal/weftname`, `internal/gitkit`, `internal/hubforge`, `internal/boardengine`, `internal/configsync`), so it "does not know weft exists" — a doc describing Finalize's own behavior rewords to Fabric, it does not restate the owner's internal split.
`internal/fabricengine`'s own package documentation is the correct, and only correct, place for the warp/weft mechanics below — this section now describes only what `fabricengine` **hands Finalize**, never which internal component produced it.

Merge-back is not a single git-merge operation from `fabricengine`'s own perspective — a real internal distinction exists, owned entirely inside `fabricengine`, and it never surfaces past that boundary as vocabulary.
What Finalize's own contract sees is one of exactly two **conflict-artifact shapes**, handed to it by `fabricengine`:

- **An ordinary git conflict.** The agent operates in a real git worktree with normal conflict markers, `git diff`/`git status` behaving as usual.
  No special handling in Finalize's own instructions beyond "resolve this the way you'd resolve any git conflict."
- **A discrepancy document.** For content `fabricengine` cannot express as a git conflict from Finalize's vantage, `fabricengine` precomputes the diff itself and hands the agent a plain document describing the discrepancy — the agent reads it, resolves, and writes the final content back through the path `fabricengine` gave it, never invoking git directly for this shape.
  *Why this shape exists at all* is `fabricengine`'s own fact to state, not Finalize's — see its package documentation.

## Only Raddle's output forwards to the parent — not the rest of `_lyx`

`_lyx` is committed into every task's own Fabric branch **by design** — see `internal/fabricengine`'s package documentation and the [Fabric Git Invariant (warp + weft)](../../CONSTRAINTS.md#fabric-git-invariant-warp--weft) for the owner's own account of why, in the owner's own vocabulary.
From Finalize's side: it is the per-task session/orchestration state, correct to live there for the task's own lifetime, but never meant to propagate to the parent.
Merge-back forwards only **Raddle**'s regenerated output (see [raddle.md](raddle.md#when-it-runs-deferred-to-merge-time-not-mid-task) for when that regeneration runs), via a Fabric commit call scoped to a narrowed pathspec, `["_lyx"]` — raddle and PATTERN content are both inside `_lyx` now, which is precisely why the earlier per-directory scoping (`_raddle` vs. `_pattern`) is obsolete.
No new exclusion mechanism is needed — this is a call-site decision, not an architecture gap.
(The pathspec-scoped commit primitive this calls is `fabricengine`'s own, currently `commitWeft`/`commitWeftLocked` in `internal/fabricengine/weftgit.go` — unexported today, so it will need either an exported wrapper or to live as a method Finalize's own package calls through `fabricengine`'s public surface; verify the actual exported name against the tree when this task is picked up rather than trusting this note.)

Note: since Raddle and (eventually) `scout`'s own index are both pure functions of the current source code, they **regenerate** at merge-time rather than being merged/diffed across branches at all (see [raddle.md](raddle.md) for the reasoning) — so in practice the discrepancy-document conflict shape above is expected to matter mainly for genuinely hand/LLM-authored content like `PATTERN.md`, not for Raddle's own output.

## Raddle regeneration — part of the merge, not a step before it

Raddle-regeneration is scoped as part of the Finalize merge itself, not a separate producer or a reserved phase slot of its own, per [shed.md](shed.md) and [loom.md](loom.md#the-phase-machine--a-flat-producer-list-no-predefined-slots): updating Raddle before the merge is impractical given merge-conflict risk, so regenerating it happens inside the merge's own critical section instead.
The merge lock Finalize takes must span that whole critical section as one atomic unit — read the parent's current HEAD, run the leaf-fork and `Overview.md` regeneration against it, and commit the result via `SyncWeft` — never released and re-acquired partway through.
See [raddle.md](raddle.md) for the regeneration mechanics themselves — the parallel-fork structure, the `Overview.md` sequencing, and the `SyncWeft` commit shape all live there, not here.
This is the currently-landed shape of the fold, not the only one considered: an alternative giving Raddle its own `Shed` producer, with merge-in and locking lifted into `Shed` itself, surfaced during this task's discussion and remains a candidate for a future task.

## PR creation, when configured

If `require_pr_to_base` is set, the PR title/body is dumped **verbatim** from the prose summary artifact webster adds to its final action (see [webster-spec.md](../../contracts/specs/webster-spec.md#the-summary-artifact--_lyxwebstersummarymd)) — no dedicated LLM call needed in Finalize itself, since that summary is the only artifact with full oversight of what was actually built, including deviations from the original plan.

## Config

`finalize.yaml` holds safe defaults (e.g. no direct merge to main without a PR);
`loom.yaml` (or, eventually, a `hardener.yaml`) can override the same keys for orchestrated runs — same shape as the existing "per-phase profiles live in loom, not perch.yaml" precedent, generalized to any module `Shed` drives as a black-box gate.

## Related

- [shed.md](shed.md) — the generic outer phase-FSM `Finalize` is a producer within;
  both `loom`'s and the Someday `Hardener`'s producer lists name the same `Finalize` definition, never a copy.
- [loom.md](loom.md) — the mature, already-detailed phase machine this doc was originally split out of;
  `shed.md` owns `Shed`'s generic mechanism, while `loom.md` owns `loom`'s own concrete producer list built on top of it, per `shed.md`'s own split of authority.
- [raddle.md](raddle.md) — the regeneration mechanics (parallel-fork structure, `Overview.md` sequencing, `SyncWeft` commit shape) the section above points at;
  the fold decision itself now lives in this doc's own "Raddle regeneration" section above, not in this bullet.
- [webster-spec.md](../../contracts/specs/webster-spec.md#the-summary-artifact--_lyxwebstersummarymd) — the summary artifact Finalize consumes verbatim for PR bodies;
  `internal/websterengine`'s package documentation covers the escalation pattern Finalize mirrors.
- [`internal/fabricengine`](../../internal/fabricengine/doc.go) — the mechanics Finalize wires on top of, incl. `CommitWeft`'s pathspec parameter and `Warp-SHA` correspondence tracking.
