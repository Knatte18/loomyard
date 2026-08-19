# Landing — Shed's generic Publish and Finalize producers

> **Status: Design — not built.** Depends on the Planned `fabric: merge-conflict primitive` task (see `manifest/roadmap.md`) — that task does not exist as code yet. Neither `Publish` nor `Finalize` is `loom`'s own: each is an ordinary producer any `Shed` producer list can name — one definition per producer, named twice (`loom`'s and the Someday `Hardener`'s lists), never copied, never `Shed`-special-cased (see [shed.md](shed.md)). Renamed from `finalize.md`, which covered only one of the two producers landing here now. Per the [documentation lifecycle](../../docs/overview.md#documentation-lifecycle), durable parts fold into the relevant package docs and this file deletes when both producers land.

## What it is

Two generic, mechanical `ShedProducer`s close out a `Shed` run, plus the merge-in/conflict-resolution engine both call:

- **`Publish`** — checks whether a PR is required, and if so opens one.
  Returns `Done` once the PR exists (or once it determines none is needed) — it never waits on review.
- **`Finalize`** — the actual merge back to the parent, `_lyx` teardown, done.
- **`internal/mergeresolve`** — not a producer itself, the shared engine both call to bring the branch up to date with its parent and resolve whatever conflict shape results.

Splitting these apart (rather than one "Finalize does merge-back and PR" producer, as an earlier draft of this doc had it) follows from a single fact: `Finalize` and `Publish` can each need `mergeresolve`'s merge-in step independently of the other, on different schedules — `Publish` before a PR goes up, `Finalize` after it (if any) comes down — so the merge-in/conflict step has to be a shared call, not owned by either producer.

## Publish

Go-first, mechanical, no LLM call of its own.

1. Check `require_pr_to_base` (from `landing.yaml`, see [Config](#config) below).
2. **Unset:** no-op. `Done` immediately — no merge-in, nothing else happens here.
3. **Set:** call `mergeresolve`'s merge-in (see below) against the parent, to bring the branch current before a PR goes up.
   Then open the PR — title/body dumped **verbatim** from Webster's own summary artifact ([webster-spec.md](../../contracts/specs/webster-spec.md#the-summary-artifact--_lyxwebstersummarymd)), no LLM call in `Publish` itself.

Either way, `Publish` returns `Done` once its own job is finished — it does not wait for review, approval, or merge.
What happens to an open PR afterward (comments, approval, an eventual merge on GitHub's side) is out of `Shed`'s view entirely: progress resumes only when a human explicitly asks for it, through a separate, out-of-`Shed` CLI flow that spawns an interactive LLM session to read the PR's comments and work through them with the user.
Not designed here; not a `Shed` producer, since it only ever runs on request.

## Finalize

**Vital, not deferred** (unlike Raddle, see below).

Always calls `mergeresolve`'s merge-in against the parent itself — regardless of which branch `Publish` took.
In the no-PR case this is *the* merge-in;
in the PR case it is a second one, catching whatever happened to the parent while the PR sat out for review (low-probability, but not zero).
Then: `_lyx` teardown, and the actual `Fabric` merge back to the parent.

`lyx fabric cleanup` owns worktree/branch/junction/portal teardown — out of scope here, same reason `mill-cleanup` runs from the hub, never a task worktree.

## Shared: the merge-in / conflict-resolution engine (`internal/mergeresolve`)

Not a `Shed` producer — a plain package both `Publish` and `Finalize` call, each through its own thin `ShedProducer` wrapper.

- Calls `Fabric.MergeIn(parent)`.
- On a clean merge: done, no LLM cost.
- On conflict: spawns a **fresh, higher-capability model in a clean session** (see `internal/websterengine`'s package documentation) — not a `/model` switch inside a polluted one — to resolve it.
- Returns a plain result (resolved / stuck) to its caller;
  neither producer's own `ShedProducer.Call` needs to know which shape of conflict occurred, only whether it's resolved.

## Landing sees only Fabric — never "warp" or "weft"

Neither `Publish` nor `Finalize` — nor `mergeresolve` — is in the Fabric Vocabulary Invariant's owner set (`CONSTRAINTS.md`), so per that invariant none of them know weft exists.
`Fabric` hands `mergeresolve` one of two conflict-artifact shapes, never which internal side produced them:

- **An ordinary git conflict.** Resolve it like any git conflict.
- **A discrepancy document.** `Fabric` precomputes a diff it cannot express as a git conflict, hands the agent a document; the agent resolves and writes back through the path `Fabric` gave it — no git.

Producing these two shapes — attempting the merge, detecting which shape applies, building the document for the second — is **not implemented anywhere today**. No `Merge` function exists in `internal/gitrepo`; `Fabric.Diff`/`Fabric.Status` are read-only reporting, not conflict detection. This is the Planned `fabric: merge-conflict primitive` task, and `mergeresolve` depends on it.

## Only Raddle's output forwards to the parent

`_lyx` commits into every task's own Fabric branch by design (`internal/fabricengine`'s package documentation; [Fabric Git Invariant](../../CONSTRAINTS.md#fabric-git-invariant-warp--weft)) and never propagates to the parent on its own.
`Finalize`'s merge-back forwards only Raddle's regenerated output ([raddle.md](raddle.md#when-it-runs-deferred-to-merge-time-not-mid-task)), via a Fabric commit scoped to `["_lyx"]` — raddle and PATTERN content both live in `_lyx` now, so no separate exclusion mechanism is needed.
The exact commit call this uses is part of the `fabric: merge-conflict primitive` task's scope, not fixed here.
`Publish`'s own merge-in is a pre-PR sync only — it never commits or forwards anything.

Raddle and (eventually) `scout`'s index regenerate at merge-time rather than being diffed across branches ([raddle.md](raddle.md)) — the discrepancy-document shape above mainly matters for hand/LLM-authored content like `PATTERN.md`, not Raddle's own output.

## Raddle regeneration — part of the merge, not a step before it

Scoped inside `Finalize`'s own merge critical section, not a separate producer, and not part of `Publish`'s pre-PR sync ([shed.md](shed.md), [loom.md](loom.md#the-phase-machine--a-flat-producer-list-no-predefined-slots)): read parent HEAD, run leaf-fork + `Overview.md` regeneration against it, commit — one atomic lock span, never released mid-way.
See [raddle.md](raddle.md) for the regeneration mechanics themselves.
Alternative considered and rejected for now: Raddle as its own `Shed` producer, with merge-in/locking lifted into `Shed` — candidate for a future task.

## Config

`landing.yaml` holds safe defaults for both producers (e.g. `require_pr_to_base`, no direct merge to `main` without a PR); `loom.yaml`/`hardener.yaml` override per orchestrated run — same shape as the existing "profiles live in the caller, not the callee" precedent.

## Related

- [shed.md](shed.md) — the generic phase-FSM `Publish` and `Finalize` are producers within.
- [loom.md](loom.md) — `loom`'s concrete producer list, naming both by reference.
- [raddle.md](raddle.md) — regeneration mechanics the sections above point at.
- [webster-spec.md](../../contracts/specs/webster-spec.md#the-summary-artifact--_lyxwebstersummarymd) — the PR-body source artifact.
- [`internal/fabricengine`](../../internal/fabricengine/doc.go) — `Diff`/`Status`/correspondence tracking `mergeresolve` builds on; does not yet include the merge-conflict primitive this doc depends on.
