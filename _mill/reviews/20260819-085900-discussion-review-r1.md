MILL_REVIEW_BEGIN
# Review: loom: phase-machine scaffolding

```yaml
duration_s: 195.0
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Opus-class model, Anthropic)
reviewed_file: _mill/discussion.md
date: 2026-08-19
```

## Findings

### [BLOCKING:design] Preflight Stuck permanently deadlocks row 1
**Section:** Technical context → "Preflight's ordering" / `onstuck-routing`
**Issue:** `Preflight` has `OnStuck: ""`, so a Stuck there hits `run.go`'s Stuck branch, which appends a history entry before persisting `StateBlocked`; on the human-resumed re-run `checkCoherence` (`coherence.go:92`) fails `CheckHalfFinished` on `len(History) != 0`, so `Preflight` can never pass again — the "human-resumed halt at row 1" case the paragraph claims is consistent is exactly the broken one.
**Fix:** Decide how the rewritten coherence check reconciles the fresh-start invariant with Shed-appended history (e.g. tolerate history entries naming only `Preflight`), and state it in `rewrite-loom-status-in-place`.

### [NIT:consistency] Batchifier gate cannot gate what is already resolved
**Demoted-from:** BLOCKING
**Section:** `batchifier-is-a-gate`
**Issue:** The decision both makes row 9 a fail-fast gate over `batcher.Active` and injects the resolved `Batcher` into `websterengine.RunDeps` at construction (`shedadapters.NewWebsterProducer(name, run, deps)` takes `RunDeps` by value), so `Active` must already have succeeded before `Shed.Run` starts — the gate's stated value ("catch a broken config before Webster spawns") is unreachable, and on a mid-run config edit the gate and Webster's injected `Batcher` disagree silently.
**Fix:** State which resolution is authoritative and when each happens — e.g. the row resolves and the Webster producer is constructed lazily from it, or the gate is explicitly a re-validation with the stale-injection consequence accepted in writing.

### [NIT:decision] Status fields with no stated disposition
**Demoted-from:** BLOCKING
**Section:** `shed-schema-wins` / `rewrite-loom-status-in-place`
**Issue:** `narration` and `phase` get explicit dispositions, but `stage` (`produce|gate`), `next_action`, and `history[].bounced_to` — all pinned in `loom-status-spec.md` and read by `checkCoherence` (`next_action` in the fresh-start check) — are never mentioned; `shedengine.HistoryEntry` has no bounce-target field at all, so bounce provenance is silently lost.
**Fix:** Name each of the three as dropped, moved into `product`, or reconstructed, in the same decision that drops `narration`.

### [NIT:decision] Seed's initial state and existing-file behaviour undecided
**Demoted-from:** BLOCKING
**Section:** `loomshed-owns-seed` / Testing
**Issue:** The decision says "the initial state" without naming a value, though `Status.State` is a five-member enum whose empty string the read gate hard-rejects (`status.go:31`), and the existing-file behaviour is explicitly punted ("refuse vs. overwrite — pick one and test it"), which is a production-safety decision: overwrite silently destroys an in-flight run's history.
**Fix:** Pin the seeded `state` value and pick refuse-vs-overwrite here, in the decision, rather than deferring to planning.

### [NIT:consistency] Scope forbids the shedengine doc correction it forces
**Demoted-from:** BLOCKING
**Section:** Scope → Out
**Issue:** "Any change to `internal/shedengine`" is out, but `internal/shedengine/doc.go`'s "# Divergence from loom's status schema" section states that reconciling the schemas "is loom's own later rewiring work" and that "a Shed-written file would still fail loom's coherence check" — both false once this task lands; a doc-comment edit adds no import and so does not engage the Shed Producer-Seam Invariant the exclusion cites.
**Fix:** Carve the `doc.go` divergence paragraph out of the exclusion, or state explicitly that it is left stale and why.

### [NIT:design] `batcher.Active`'s error is undifferentiated
**Section:** `batchifier-is-a-gate` / Testing
**Issue:** `Active` returns a bare `error` for unknown-name, malformed YAML, and any I/O failure alike, so the gate cannot map "broken config" to `Stuck` while surfacing an infra failure as an error — the two produce different persisted states (`blocked` vs `failed`).
**Fix:** State the mapping rule (e.g. all `Active` errors map to `Stuck`) and accept the conflation explicitly.

### [NIT:scope] Doc-update list omits `docs/overview.md`
**Section:** Constraints → Documentation Lifecycle
**Issue:** Only `loom.md` and `loom-status-spec.md` are listed, but `docs/overview.md:148` describes loom's status as "current phase, review round, verdict history" (stale once `phase` disappears) and its tree at line 234 enumerates internal packages, which a new `internal/loomshed` joins.
**Fix:** Add `docs/overview.md` to the same-commit doc set, or state why it is unaffected.

### [NIT:decision] Two items explicitly deferred to planning
**Section:** Technical context → `Plan-never-reads-support-log`; Constraints → Told-Geometry
**Issue:** The build-time boundary assertion ("decide during planning whether the assertion is meaningful yet") and the `loomshed` import guard ("Consider adding a `leaf_enforcement_test.go`-style guard") are both left open.
**Fix:** Resolve both here — the second especially, since it converts a review obligation into a machine check.

## Verdict

REQUEST_CHANGES
Preflight retry deadlock and the self-defeating Batchifier gate must be resolved first.
_Note: 4 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 1._
MILL_REVIEW_END
