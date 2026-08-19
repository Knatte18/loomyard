MILL_REVIEW_BEGIN
# Review: loom: phase-machine scaffolding

```yaml
duration_s: 223.0
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic) — Opus-class model, exact build unknown to me
reviewed_file: _mill/discussion.md
date: 2026-08-19
```

## Findings

### [NIT:decision] Two spec-pinned history rules have no disposition
**Demoted-from:** BLOCKING
**Section:** `rewrite-loom-status-in-place` → "Field-by-field disposition … so nothing is silently dropped"
**Issue:** `coherence.go:71-90` implements two rules `loom-status-spec.md:96-97` pins — `history[].outcome ∈ {approved,stuck}` and every timestamp RFC3339 UTC — and the disposition list names only `history[].bounced_to`; under `shedengine.HistoryEntry` the vocabulary becomes `done|stuck` and `ts` becomes `at`, so both rules silently change meaning or vanish.
**Fix:** State whether the rewritten check keeps these per-entry rules against the new field names, or drops them on the same "Shed composes it, validating it asserts Shed against itself" ground already used for `activity`.

### [BLOCKING:design] Preflight wrapper's owning package left ambiguous
**Section:** `preflight-signature-unchanged` + `explicit-deps-struct` + Testing
**Issue:** "A small wrapper **in `loomshed`** adapts it to `ShedProducer`" and "`loomshed` receives that wrapper already constructed", with `Deps.Preflight` typed as a bare `shedengine.ShedProducer` and the caller closing over `cwd` — so whether `internal/loomshed` imports `internal/loomengine` at all is unstated, and the integration test described as covering "the real `loomengine.Preflight` wrapper" has no named home package.
**Fix:** Say explicitly whether `loomshed` exports the wrapper constructor (and therefore imports `loomengine`, which the direct-only `lyxcwd` guard permits) or whether the wrapper belongs to the not-yet-built caller — and name the package the integration test lands in.

### [NIT:decision] `loom.md`'s "This assertion lands with `Shed`" left standing
**Section:** Technical context → `Plan-never-reads-support-log`
**Issue:** The discussion decides the assertion lands in `loom: write and wire in the real LLM producers`, but `manifest/designs/loom.md:79` still reads "This assertion lands with `Shed`" — already false, and the status-file-keyed grep enumeration method cannot reach it.
**Fix:** Either add that line to the same-commit doc set or record it as deliberately left to the later task.

## Verdict

REQUEST_CHANGES
Two unresolved dispositions: history-entry validation rules and Preflight-wrapper package ownership.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 1._
MILL_REVIEW_END
