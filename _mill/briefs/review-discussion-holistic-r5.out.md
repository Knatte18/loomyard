MILL_REVIEW_BEGIN
# Review: lyx loom step + external supervisor skill

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-09-12
```

## Findings

### [BLOCKING:consistency] Webster carve-out contradicts "skill names no producer"
**Section:** Scope § Out vs `interrupted-step-re-invokes-on-loom-s-own-crash-resume-with-a-consecutive-cap`
**Issue:** Scope § Out states "The skill never names a producer", while the interrupted-step rule requires the skill to branch on `current_producer` being the literal `Webster` (and the Testing § pass exercises that branch).
**Fix:** State the carve-out explicitly in Scope § Out — the skill knows exactly one producer name, `Webster`, as a non-reattaching-row marker, and no routing/phase knowledge beyond it — or move the Webster discrimination into the envelope (e.g. a Go-supplied attach/no-attach flag) so the skill stays name-free.

### [BLOCKING:scope] drive.go's shed-construction extraction is outside the scope inventory
**Section:** Scope § In vs Technical context (`internal/loomcli/drive.go`)
**Issue:** Technical context says `step` "reuses this verbatim through a shared helper" for the `fabricengine.Open` → … → `loomrecipe.New` block that today lives inline in `drive.go` (verified at `internal/loomcli/drive.go:78-127`), but Scope § In lists only the `run` seed/commit and strand helpers, and Scope § Out's behaviour-preservation guarantee names `run` alone.
**Fix:** Add the `drive` shed-construction extraction to Scope § In with an explicit "behaviour-preserving for `drive`" guarantee (or state that `step` builds its own shed and `drive` is untouched), and say which verb's tests cover the shared helper.

### [NIT:design] Step's pre-lock preamble is unstated
**Section:** `step-primitive-extracted-from-run-not-duplicated` / `run-lock-per-step-not-held-across-steps`
**Issue:** `Run` performs `validate()` plus `MkdirAll` of both lock parents before acquiring the run lock (`internal/shedengine/run.go:40-53`); the decisions describe `Step` only as "acquire the lock, call it once, release", leaving unsaid whether `Step` repeats that preamble — and `lock` opens with `O_CREATE` without creating parents.
**Fix:** State that `Step` runs the same `validate()` + both `MkdirAll` calls before acquiring, since `Step` is exported for every shed and cannot rely on a loom caller having made the ephemeral dir.

### [NIT:design] Ordering of the `busy` refusal against bootstrap is undecided
**Section:** `step-bootstraps-idempotently-but-never-spawns-a-driver` / `crash-cleanup-is-one-retry-then-hand-back`
**Issue:** `ErrShedBusy` surfaces from inside `shedengine.Step`, i.e. after `step` has already seeded, run `CommitAnchoredPaths`, `reed.Up()` and the strand work against a task a live driver owns; the discussion never says whether `step` probes the run lock earlier.
**Fix:** State the chosen ordering — bootstrap-then-refuse (matching `run`, relying on commit idempotence) or an early run-lock probe — so the `kind: "busy"` envelope's cost is a decision rather than an accident.

## Verdict

REQUEST_CHANGES
Two blocking gaps: the Webster naming contradiction and the unscoped `drive.go` extraction.
MILL_REVIEW_END
