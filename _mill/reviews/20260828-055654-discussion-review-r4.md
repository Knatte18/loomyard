MILL_REVIEW_BEGIN
# Review: Audit internal/logger coverage across spawn/hard-error paths

```yaml
duration_s: 109.5
verdict: APPROVE
reviewer_model: opusmedium
reviewer_self_id: Claude Opus 5 (claude-opus-5)
reviewed_file: /home/knatte/Code/loomyard/wts/logger-coverage-audit/_mill/discussion.md
date: 2026-08-28
```

## Findings

### [NIT:scope] mergeresolve import allowlist edit missing
**Demoted-from:** BLOCKING
**Section:** Scope In (`mergeresolve` bullet) / Testing
**Issue:** `internal/mergeresolve/seam_enforcement_test.go` is a strict membership allowlist (`mergeresolveAllowedImports`, lines 27-33: `fabricengine`, `shuttleengine`, `modelspec`, `stencilstore`, `stencil` — no `logger`), so adding the `logger` import to `mergeresolve.go` fails that test; the discussion lists the `tierpurity_test.go` allowlist edit as its own deliverable precisely to avoid this class of surprise but never names this one, and the Testing section flags only `treadleengine/seam_enforcement_test.go`, whose allowlist already admits `logger`.
**Fix:** Either add the `mergeresolveAllowedImports` entry (with reason) as an explicit Scope In deliverable, or decide the `mergeresolve` log line is dropped — and state which.

### [NIT:consistency] Amended invariant text is violated on landing
**Demoted-from:** BLOCKING
**Section:** `constraints-md-prose-only` (replacement text) vs `spawn-site-verdicts`
**Issue:** The replacement prose says "Every production code path starting a real OS process logs its spawn", and its only exemption bullet is for sites *structurally barred* from importing `logger` — but the verdict table excludes `internal/hubforge/hub.go:371`, `cmd/testtiming/main.go:74` and seven `tools/` spawns (verified present, none logging, none structurally barred), so the amendment is violated the moment it lands, the same failure shape r3 caught for detached spawns.
**Fix:** Give the replacement text an explicit scope/exemption clause covering non-production-path spawns (fixture builders, dev/test tooling, `tools/`), matching the table's `excluded` category.

### [NIT:consistency] Hard-error selector filters comments only in `doc.go`
**Section:** `error-universe`
**Issue:** The spawn selector is AST (r3's fix for doc-comment prose) but the hard-error selector stays `grep 'shuttleengine\.Outcome[A-Z]'` "minus `doc.go` matches"; the tree also matches comments outside `doc.go` (`shedadapters/burler.go:38`, `burlerengine/engine.go:47,66,89`) and a non-comparison (`treadleengine/run.go:218`, a `string(...)` conversion), so residual judgment survives the "mechanical" claim.
**Fix:** State the post-grep filter explicitly (comment lines and non-comparison occurrences dropped) or make this selector AST-based too.

### [NIT:consistency] Yield count stated as nine and ten
**Section:** `error-universe` vs Q&A log (r2 entry)
**Issue:** `error-universe` says "ten production sites" and its table has ten rows; the r2 Q&A entry says "nine-site yield".
**Fix:** Reconcile to one number, or drop the count from the Q&A entry since the tables are declared non-authoritative anyway.

## Verdict

APPROVE
A missing mergeresolve allowlist edit and self-violating invariant text must be settled first.
_Note: 2 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 0._
MILL_REVIEW_END
