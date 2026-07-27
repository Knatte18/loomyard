# Finalize — Shed's merge-back step

> **Status: Design — not built. Planned, combined with the `Shed` task** (see `manifest/roadmap.md`) — building `Shed`'s skeleton and its Finalize step happen together, the same reasoning as the combined `Treadle` + `perch`-rewrite task. Renamed from `loom-finalize.md`: Finalize is **`Shed`'s** literally-shared code (identical for `loom` and `Hardener`, not a swappable per-instance slot the way Preflight and the producer are — see [shed.md](shed.md)), not loom-specific, even though it was originally split out of [loom.md](loom.md) (still a substantial, fairly self-contained phase specification, worth its own file rather than folded into `shed.md`'s own doc). Per the [documentation lifecycle](../../docs/overview.md#documentation-lifecycle), when this lands the durable parts fold into the relevant package doc and this file is deleted.

## What it does

**Vital, not deferred** (unlike Raddle). Go-first: the happy path (no conflicts) is pure Go — squash, push, done, zero LLM cost. An LLM is spawned only on merge conflict (during merge-in from parent, or the merge to parent itself), escalating **the same way Builder escalates a stuck batch to a fresh higher-capability model** (see `internal/websterengine`'s package documentation) — not a `/model` switch inside a polluted session.

Mostly wiring on top of the already-built `warp` mechanics (absorbed into `fabric` once that lands — see [fabric.md](fabric.md)); worktree/branch/junction/portal teardown is explicitly **out of scope** — that's `warp cleanup`'s (future: `fabric`'s) already-existing, separate job, which cannot run from inside the worktree being removed, the same reason `mill-cleanup` runs from the hub, never a task worktree.

## Two merge targets, not one — warp and weft, handled differently

Merge-back is not a single git-merge operation — it is two, with genuinely different conflict mechanics, because `_raddle`/`_pattern` content is reached through a **filesystem junction** (not git-aware) inside the warp worktree:

- **Warp side** — an ordinary git merge conflict. The agent operates in its own worktree, where `git diff`/`git status` behave normally. No special handling.
- **Weft side** — deliberately **not** represented as a git conflict at all, even though the files are reachable at what looks like an ordinary path inside warp (via the `_raddle` junction). `git diff` run from the warp worktree cannot see across a junction boundary into weft's own, separate `.git` history — it would silently report nothing, which is worse than an error, since nothing signals the agent it asked the wrong tool the wrong question. So weft-side conflicts are never given git conflict markers: Go precomputes the diff directly against the real weft worktree (both weft SHAs are already known via `fabric`'s `Warp-SHA` correspondence tracking) and hands the agent a plain **document** describing the discrepancy — the agent reads it, resolves, and writes the final content via the junction path (a transparent write-through to the real weft files) — never invoking git for the weft side at all. This avoids inviting the agent's normal git-diff reflex into a path where that reflex silently fails.

## Only Raddle forwards from child weft to parent weft — not `_lyx`

`_lyx` is committed into every task's own weft branch **by design** (see `internal/weftengine`'s package documentation and CONSTRAINTS.md's Weft-git ownership section) — it is the per-task session/orchestration state, and it is correct for it to live there for the task's own lifetime. It was never meant to propagate to parent, though — merge-back only forwards **Raddle**'s regenerated output (see [raddle.md](raddle.md#when-it-runs-deferred-to-merge-time-not-mid-task) for when that regeneration actually runs) using a **narrowed pathspec**: `fabric.CommitWeft` already accepts an arbitrary pathspec (it is not hardwired to `_lyx` — see `internal/weftengine/weft.go`'s own package doc, whose own example already lists `["_lyx", "_raddle"]`), so the merge-back commit simply calls it with `["_raddle"]` (and, eventually, `["_pattern"]` for `PATTERN.md`), never `_lyx`. No new exclusion mechanism is needed — this is a call-site decision, not an architecture gap. `_lyx` simply stays in the child's own weft branch forever, unused by parent, exactly as intended.

Note: since Raddle and (eventually) `codeintel`'s own index are both pure functions of the current source code, they **regenerate** at merge-time rather than being merged/diffed across branches at all (see [raddle.md](raddle.md) for the reasoning) — so in practice the weft-side document-driven conflict path above is expected to matter mainly for genuinely hand/LLM-authored weft content like `PATTERN.md`, not for Raddle's own output.

## PR creation, when configured

If `require_pr_to_base` is set, the PR title/body is dumped **verbatim** from the prose summary artifact webster adds to its final action (see [builder-contract.md](../../docs/reference/builder-contract.md#webster-the-fork-based-sibling)) — no dedicated LLM call needed in Finalize itself, since that summary is the only artifact with full oversight of what was actually built, including deviations from the original plan.

## Config

`finalize.yaml` holds safe defaults (e.g. no direct merge to main without a PR); `loom.yaml` (or, eventually, a `hardener.yaml`) can override the same keys for orchestrated runs — same shape as the existing "per-phase profiles live in loom, not perch.yaml" precedent, generalized to any module `Shed` drives as a black-box gate.

## Related

- [shed.md](shed.md) — the generic outer phase-FSM Finalize is the last step of; both `loom` and the Someday `Hardener` share this exact code.
- [loom.md](loom.md) — the mature, already-detailed phase machine this doc was originally split out of; `Shed` hasn't been extracted from it yet (see that doc's own naming note).
- [raddle.md](raddle.md) — the merge-time regeneration decision and merge-lock scope Finalize's Raddle-regeneration step must honor.
- [builder-contract.md](../../docs/reference/builder-contract.md#webster-the-fork-based-sibling) — the summary artifact Finalize consumes verbatim for PR bodies; `internal/websterengine`'s package documentation covers the escalation pattern Finalize mirrors.
- [fabric.md](fabric.md) — the mechanics Finalize wires on top of, incl. `CommitWeft`'s pathspec parameter and `Warp-SHA` correspondence tracking.
