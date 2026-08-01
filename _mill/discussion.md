# Discussion: fabric: warp-rebase / remote-reconcile recovery

```yaml
task: 'fabric: warp-rebase / remote-reconcile recovery'
slug: fabric-rebase-reconcile
status: discussing
parent: main
```

## Problem

`fabric` operates two git repos (warp and weft) but exists to make them look like one repo through a single interface. `Fabric.Commit` already delivers that illusion for writes — it dispatches any path to the correct underlying repo, so nothing lyx/LoomYard-initiated has to know two repos exist (shipped in an earlier slice). Pull has no such unification today: `lyx fabric pull` only fast-forwards weft; there is no warp-pull path through fabric at all, so any lyx/LoomYard-driven code that needs to bring warp's remote state down still has to fall back to raw git, breaking the "one repo, one interface: fabric" contract for reads the same way it was already broken for writes before `Fabric.Commit` landed.

Separately, fabric can never force *other* collaborators through it — someone who doesn't even know weft exists can commit, pull, or rebase warp directly, on this machine or another. When that happens, weft's `Warp-SHA` correspondence trailers can end up pointing at warp SHAs that no longer exist (a rebased/force-pushed warp history). This is slice 6 (the last slice) of the `fabric` unified-view campaign (`manifest/designs/fabric-unified-view.md`), explicitly called out there as "the hardest part, but bounded": most weft content self-heals (raddle/scout regenerate at merge-time — not built yet, out of scope here) or never propagates to a parent (`_lyx`), leaving hand/LLM-authored content — `PATTERN` specifically — as the one real residue that needs re-alignment after a warp rebase.

## Scope

**In:**
- A unified pull path through fabric: extend the existing `lyx fabric pull` CLI verb and add an unprefixed `Fabric.Pull(opts SyncOptions) (PullResult, error)` Go method that pulls **both** warp and weft in one call. This follows the existing naming convention in `internal/fabricengine`, where unprefixed methods (`Commit`, `Diff`, `Status`) dispatch across both repos and `*Weft`-suffixed methods (`PullWeft`, `PushWeft`, `CommitWeft`, `StatusWeft`) are weft-only building blocks the unprefixed ones compose. Weft's existing fast-forward-only pull behavior is unchanged; warp pull is the new half.
- Rebase detection: after refreshing warp, check whether the single latest correspondence entry's `Warp-SHA` still exists in warp's history. If it doesn't, warp moved out from under fabric's last known state.
- Safe automatic reconciliation: when the warp pull is a non-fast-forward (a rebase happened) **and** the local warp worktree has no unpushed local commits, reset warp to the new remote history and re-anchor weft's correspondence by walking the correspondence index newest-to-oldest until finding the first `Warp-SHA` that still exists in the new warp history. That becomes the new confirmed anchor point.
- A `PullResult` return value for any `Fabric.Pull` call that touched warp: what was pulled, whether a non-fast-forward rewrite was detected, what the new anchor point is, and — the one thing genuinely needing human/agent attention — which weft commits made after the new anchor point touch PATTERN content (`_pattern/...` paths). No on-disk file — the CLI surfaces this via the existing JSON output envelope. This follows finalize.md's "plain document, not git conflict markers" spirit for describing weft-side discrepancies without git conflict markers, produced standalone since the module that would eventually consume a persisted version of it (`Shed`/Finalize) isn't built yet.
- A hard safety invariant: if local warp has unpushed commits **and** the remote diverged, `Fabric.Pull` aborts loudly and changes nothing. This "double conflict" case is explicitly out of scope for automatic handling.

**Out:**
- No LLM spawn from fabric. Any agent session depends on `reed` (see CLAUDE.md's "Agent execution" section), which sits above fabric in the dependency stack — fabric spawning an LLM would be a circular dependency. Fabric produces the plain document; resolving it is a future orchestration-layer (`loom`/`Shed`) concern.
- No raddle regeneration call site. `raddle.md` is "Someday, deprioritized... not scheduled" — not built. This slice ships the fabric-side primitives (the existing `SnapshotWarpSHA` staleness idiom) a future raddle consumer would need, but wires nothing to an actual regeneration trigger.
- No new CLI verb name. The existing `lyx fabric pull` is extended in place, not replaced by e.g. `fabric reconcile-warp`. This also deliberately avoids the name `reconcile` — `lyx fabric reconcile` already exists (`internal/fabriccli/fabric.go:205`) and repairs host↔weft *topology* pairing (a missing weft worktree, a broken junction), a different problem from warp content drift; reusing the name would be misleading.
- The "local warp has unpushed commits + remote rebased" double-conflict case is detected and aborted loudly, never auto-resolved, in this slice.
- The exact reset algorithm's internal plumbing beyond what's specified under Decisions below (e.g. whether `RevertWithWeft` is refactored or a new sibling function is added) — left to `/mill-plan`.
- Any change to how *other* (non-lyx-aware) collaborators use warp. They keep using plain git; fabric detects and reconciles after the fact instead of gating them.

## Decisions

### unified-pull-dispatch

- Decision: add an unprefixed `Fabric.Pull(opts SyncOptions) (PullResult, error)` that pulls warp and weft in one call (warp-first, matching `Commit`'s warp-first convention, unless `/mill-plan` finds a concrete reason to order otherwise); extend the existing `pull` subcommand in `internal/fabriccli/weft_verbs.go:237` (currently calls `fab.PullWeft` directly) to call `Fabric.Pull` instead.
- Rationale: matches the shipped naming convention (`Commit`, `Diff`, `Status` are unprefixed and dispatch across both repos); keeps "one repo, one interface" consistent for pull the way it already is for commit.
- Rejected: a brand-new CLI verb name — unnecessary, `fabric pull` already exists and is the obvious single name to extend.

### rebase-detection-scope

- Decision: detection checks only whether the single latest correspondence entry's `Warp-SHA` still exists in the refreshed warp history (one `SHAExists` lookup), not a full-index scan. This is deliberately not limited to rebases specifically — any warp history rewrite that makes the latest recorded `Warp-SHA` disappear (a rebase, an amend, an unrelated force-push) trips the same check and the same safe-reconcile path below; "rebase" in this doc's prose is the motivating case, not a narrower condition the code checks for.
- Rationale: cheap, and answers the actual question this slice cares about — has warp moved out from under fabric's last known state. The nearest-older walk (done during reconciliation, not detection) is what determines how far back the damage goes.
- Rejected: scanning the whole correspondence index for every entry with a dead `Warp-SHA` up front — more expensive, and redundant with the reconciliation walk.

### warp-refresh-primitives

- Decision: this slice adds three new primitives to `internal/gitrepo` that don't exist today: (1) a fetch-without-merge method (refresh remote-tracking refs without touching the local branch — `Repo.Pull()` today is `git pull --ff-only` and cannot be reused for this, since it merges immediately and hides stderr, per `pull.go:18`); (2) a fast-forward/divergence classifier that compares local warp HEAD against the fetched remote ref's ancestry to answer "clean fast-forward" vs. "history rewritten underneath us"; (3) an exported `HasUnpushed()`, promoting the existing unexported `hasUnpushed` (`push.go:233`) so `Fabric.Pull` can check it before deciding auto-reconcile is safe.
- Rationale: without naming these, "detection" and "safe-vs-unsafe-reconcile" above have no way to actually observe fast-forward-vs-rewrite or local-unpushed state — `gitrepo`'s existing surface only supports an immediate ff-only pull, not a fetch-then-inspect-then-decide flow.
- Rejected: reusing `Repo.Pull()` directly and treating any non-zero exit as "assume rebase" — too coarse (per the reviewer's finding, `git exited N` doesn't distinguish a diverged/rebased remote from "no remote configured" or a network failure), and hides the ancestry information needed to find the nearest-older surviving `Warp-SHA`.
- Exact method signatures and whether the classifier is exposed as a boolean or a richer ancestry result are left to `/mill-plan`.

### pull-partial-failure-contract

- Decision: when warp's side of `Fabric.Pull` succeeds (pulled cleanly, or safely reconciled after a detected rewrite) but weft's fast-forward pull subsequently fails, `Fabric.Pull` returns a typed partial result (e.g. `*PartialPullError` alongside `PullResult`, mirroring `*PartialCommitError`'s shape) stating exactly what succeeded (warp) and what failed (weft) — it never attempts to unwind the already-completed warp side.
- Rationale: mirrors `Fabric.Commit`'s own shipped "report-not-rollback" precedent (`commit.go:159`, warp-first-then-weft ordering, hard-error-on-warp-failure, three-outcome `*PartialCommitError`) — consistent behavior for the sibling read-path operation. Rolling back an already-reset warp state is a new risk in itself: the new warp HEAD may already be visible to another process or another machine by the time weft's pull fails.
- Rejected: attempting a full rollback of warp on weft failure, mirroring `RevertWithWeft`'s rollback-on-failure behavior — rejected because unlike `RevertWithWeft` (which resets warp to a caller-chosen target it fully controls), `Fabric.Pull`'s warp side already reflects the real remote's current state; unwinding it re-diverges local warp from origin for no safety benefit.

### safe-vs-unsafe-reconcile

- Decision: `Fabric.Pull` auto-reconciles only when the warp pull is a non-fast-forward **and** the local warp worktree has no unpushed local commits — it resets warp to the new remote HEAD and re-anchors weft's correspondence to the nearest-older surviving `Warp-SHA`. When local warp has unpushed commits and the remote diverged, `Fabric.Pull` aborts loudly with a clear error and makes no changes to either repo.
- Rationale: resetting warp while local unpushed work exists risks silently discarding a human's or agent's commits, which is unacceptable for a tool whose whole premise is routing everything through fabric safely. The clean-local case is unambiguously safe and is also the common case.
- Rejected: always requiring an explicit extra flag/command even in the safe case — adds ceremony to the common path for no safety benefit, since the unsafe case is separately and loudly gated anyway.

### pattern-conflict-reporting

- Decision: `Fabric.Pull`'s `PullResult` return value (Go struct fields only, no on-disk file) enumerates weft commits made after the newly re-anchored point and flags any touching `_pattern/...` paths as needing manual or agent review; the CLI surfaces this via the existing JSON output envelope. This follows finalize.md's "plain document, not git conflict markers" spirit — a described discrepancy, not raw git conflict markers — produced standalone since `Shed`/Finalize don't exist yet to consume a persisted version of it.
- Rationale: per the design doc's own bounded analysis, raddle/scout self-heal by regeneration (out of scope, raddle unbuilt) and `_lyx` never propagates to a parent, so PATTERN is the only genuine residue needing real re-alignment after a warp rebase. No LLM spawn is available at this layer, so the document is the mechanical output this slice can produce.
- Rejected: silently ignoring PATTERN-touching commits (lets real drift go unnoticed); spawning an LLM to resolve them inline (rejected — circular dependency on `reed`).
- Test-fixture note: PATTERN has no real content in any live LYX deployment yet (LoomYard itself isn't initialized as a fabric-managed repo), so this must be tested against a synthetic fixture `_pattern/PATTERN.md`, the same precedent the original PATTERN-wiring slice used.

### out-of-scope-llm-and-raddle

- Decision: no LLM spawn and no raddle regeneration call site in this slice.
- Rationale: `reed` (which any LLM agent session depends on) sits above fabric in the dependency stack — fabric spawning an LLM would be circular. Raddle is design-only and explicitly deprioritized.
- Rejected: building a minimal ad hoc orchestrator just for this slice — would duplicate work `loom`/`Shed` will eventually own, with no consumer for it yet.

## Technical context

Design doc: `manifest/designs/fabric-unified-view.md`, the "Warp-rebase and remote-reconcile" section and the slice-6 build-order entry name this task; the doc's still-open question "Exact rebase/remote-reconcile orchestration — which layer drives pull → conflict-resolve → raddle-regen" is resolved for the fabric-layer half by this discussion (`Fabric.Pull` does detection + safe re-anchor + document the residue); the orchestration-layer half (who eventually reads that document and drives raddle-regen) stays open until `loom`/`Shed` exist.

Existing building blocks in `internal/fabricengine` (confirmed present in the codebase):

- `RevertWithWeft(warpSHA string) (RevertResult, error)` (`revert.go:122`) — two-sided reset with an existing "nearest-older" fallback when the exact target SHA isn't in the correspondence index (see `TestRevertWithWeft_Gap_ResetsToNearestOlderAndReportsRange`, `TestRevertWithWeft_WeftResetFailure_RollsWarpBack` in `syncweft_integration_test.go`). This slice's use case *discovers* its target (the nearest surviving SHA) rather than being handed one, so the nearest-older walk likely needs extracting into something both callers share — leave the exact refactor shape to `/mill-plan`.
- `RebuildIndex()` (`index.go:332`) and `scanWarpSHATrailers()` (`index.go:208`) — how the correspondence index is built from `Warp-SHA`/`Snapshot:` trailers.
- `SnapshotWarpSHA(tag string)` (`snapshot.go:62`) — the existing staleness-check idiom a future raddle consumer already has available; no changes needed here.
- `PullWeft(opts SyncOptions) error` (`weftgit.go:534`) — today's weft-only fast-forward pull; stays as the weft-side building block `Fabric.Pull` composes.
- `gitrepo.Repo.SHAExists(sha string) bool` (`internal/gitrepo/gitrepo.go:410`) — the existence check both detection and re-anchoring depend on.

CLI surface: `internal/fabriccli/weft_verbs.go` wires today's weft-only `status/commit/push/pull/sync`. The `pull` subcommand (line 237) currently calls `fab.PullWeft` directly — this is the extension point. Its `Short`/`Long` help text needs updating to describe the new both-sides behavior (CONSTRAINTS.md's CLI/Cobra Invariant treats stale help as review-blocking).

`manifest/designs/finalize.md` is itself unbuilt ("Design — not built," bundled with the future `Shed` task) — this slice cannot call into it. It exists only as a design precedent: Go precomputes a diff/discrepancy and hands a plain document to whatever resolves it, never git conflict markers across the weft junction (a junction boundary is invisible to `git diff` run from warp).

`manifest/designs/pattern.md` confirms PATTERN's shape: a weft-backed `_pattern/` folder, index file `_pattern/PATTERN.md` (short two-line entries) plus linked detail docs; "active" iff `PATTERN.md` exists. Wiring is shipped but no real content exists anywhere yet (content migration is deferred, and LoomYard itself isn't initialized as a fabric-managed repo) — hence the synthetic-fixture testing approach.

Existing but unrelated: `lyx fabric reconcile` (`internal/fabriccli/fabric.go:205`) repairs host↔weft topology pairing — do not confuse with or reuse this name for the new warp-content-drift handling.

## Constraints

- **Weft Git Invariant** — all weft git operations go through `internal/fabricengine`; this slice's new code lives there, no exception needed.
- **Hub Geometry Invariant** — no new path-construction literals outside `hubgeometry`.
- **gitrepo Client Boundary Invariant** — `internal/gitrepo` splits local-vs-remote by client: go-git owns local reads, `gitexec` (CLI shell-out) owns anything touching a remote or mutating the working tree, and the CLI-bound method set is named exhaustively in CONSTRAINTS.md. The new fetch-without-merge primitive and any divergence-classification method that shells out (see the `warp-refresh-primitives` Decision) are `gitexec`-bound and must be added to that pinned list and to `TestGitrepoBoundary_PinnedRunCallSites` in the same commit — widening the CLI-bound set without updating both is itself a violation.
- **CLI/Cobra Invariant** — the extended `fabric pull` command's `Short`/`Long` must be re-read and kept accurate to its new both-sides behavior, since observable behavior changes (help accuracy is a review obligation, not just a presence check).
- **Weft Git Invariant's "orchestration, not agent" principle** — no LLM decides weft-commit timing; any commit this slice makes to record a new anchor point is orchestration-triggered (the `Fabric.Pull` call itself), never agent-triggered.
- **Test Tier Purity / Hermetic Git Test Environment invariants** — new integration tests need real git to simulate an external rebase, so they must be tagged `integration` and use `lyxtest.HermeticGitEnv()` via `TestMain`, matching the rest of `fabricengine`'s test suite.
- **Sandbox Suite Coverage** — `fabric` is already covered by `SANDBOX-FABRIC-SUITE.md`; a new scenario exercising the rebase-recovery path should be added there with a `**Covers:** fabric` tag rather than left uncovered.

## Testing

- TDD candidate: the nearest-older-surviving-anchor walk is the cleanest unit-testable piece — extract it as a pure function over the correspondence index given an injectable "does this SHA exist" predicate, so it's testable without a real git repo.
- Integration tests (tagged `integration`, hermetic git env, following the existing `syncweft_integration_test.go`/`revert_integration_test.go` pattern): build a real warp+weft pair, make several weft commits carrying `Warp-SHA` trailers, rewrite warp history underneath (simulate an external rebase against a bare "remote" clone, then fetch), and assert `Fabric.Pull`:
  - Detects the drift (latest correspondence entry's `Warp-SHA` no longer exists).
  - Re-anchors to the correct nearest-older surviving SHA — cover both a single-commit-back case and a multi-commit-back case.
  - Correctly identifies which orphaned weft commits touch `_pattern/...` (using a synthetic fixture `_pattern/PATTERN.md`) vs. which don't.
  - Aborts loudly and mutates nothing when local warp has unpushed commits and the remote diverged.
  - Leaves weft's history untouched and reports no drift when warp's pull is a clean fast-forward (no-op regression guard).
- CLI-level: add a scenario to `SANDBOX-FABRIC-SUITE.md` exercising `fabric pull` against a rebased warp remote.
- No new trust boundary or external input beyond existing git remotes — no dedicated security review needed for this slice.

## Q&A log

- **Q:** What's the scope surface — Go-API only or a new CLI verb? **A:** Extend the existing `fabric pull` CLI verb + Go `Fabric.Pull`, unified across warp+weft — not a new verb name, matching `Fabric.Commit`'s existing shape.
- **Q:** Is raddle regeneration in scope? **A:** No — raddle isn't built yet ("Someday, deprioritized"); this slice ships primitives a future raddle consumer would use.
- **Q:** Is spawning an LLM to resolve PATTERN conflicts in scope? **A:** No — an LLM agent needs `reed` and its dependencies, which sit above fabric; spawning one from fabric would be circular. Fabric only produces the plain document.
- **Q:** Should the document-driven PATTERN mechanism be tested against a synthetic fixture? **A:** Yes — PATTERN has no real content anywhere yet (LoomYard itself isn't initialized as a fabric-managed repo), so a fixture `_pattern/PATTERN.md` is the only way to test it, same precedent as the original PATTERN-wiring slice.
- **Q:** Should all lyx/LoomYard-initiated git operations, not just commit, go through fabric for both warp and weft? **A:** Yes, explicitly — `Fabric.Commit` already does this for writes; this slice closes the same gap for pull. External collaborators who don't know weft exists are not gated — fabric detects and reconciles after the fact instead.
- **Q:** What happens when warp's pull is a non-fast-forward (rebase detected)? **A:** Auto-reconcile when local warp is clean (no unpushed commits); abort loudly with no changes when local warp has unpushed commits and the remote diverged — that double-conflict case is out of scope for this slice.
- **Q:** How much of the reset algorithm should this doc pin down vs. leave to `/mill-plan`? **A:** Record the contract (inputs/outputs, safety invariant, reused building blocks) here; leave the concrete algorithm/refactor shape to `/mill-plan`.
- **Q:** (discussion-review r1 gap) `gitrepo` has no fetch/divergence/unpushed-check primitives named — what should this slice add? **A:** Name three new primitives explicitly: a fetch-without-merge method, a fast-forward/divergence classifier, and an exported `HasUnpushed()` — exact signatures left to `/mill-plan`.
- **Q:** (discussion-review r1 gap) Constraints omitted the `gitrepo Client Boundary Invariant` even though this slice adds new `gitexec`-bound calls — fix? **A:** Yes, added to Constraints with the explicit same-commit pinned-list/guard-test update requirement.
- **Q:** (discussion-review r1 gap) Is the reconciliation outcome a return struct, a written file, or both? **A:** Return-struct only (`PullResult`), surfaced via the CLI's existing JSON output envelope — no on-disk file, no new hubgeometry-resolved path needed.
- **Q:** (discussion-review r1 gap) What's the two-sided partial-failure contract if weft's pull fails after warp already succeeded/reconciled? **A:** Report-not-rollback, mirroring `Fabric.Commit`'s shipped precedent — a typed partial result, never an attempt to unwind the already-completed warp side.
