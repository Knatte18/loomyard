MILL_REVIEW_BEGIN
# Review: fabric: clone doesn't commit written module configs

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-26
```

## Findings

### [BLOCKING:design] SeedConfig can now hit "nothing to commit"
**Section:** Technical context → Consumers of the fix / Decision `fixture-state-change-is-the-point`
**Issue:** `internal/hubforge/seed.go:44-45` runs `gitkit.MustRun(tb, h.PrimeWeft(), "git","add",".")` then `git commit`; `gitkit.MustRun` (`internal/gitkit/gitkit.go:41-43`) `tb.Fatalf`s on non-zero exit, and today the nine untracked configs guarantee something is always staged — after the fix, an override byte-identical to the just-committed reconciled file stages nothing and the commit exits non-zero, a failure mode unreachable before this change.
**Fix:** State a disposition for `SeedConfig` (tolerate the empty-stage case, or assert it cannot arise) rather than folding it under "any test that breaks is asserting the bug", which covers dirty-state assertions only.

### [BLOCKING:decision] No stated disposition for already-minted hubs
**Section:** Problem / Scope → Out
**Issue:** The fix is clone-only; `lyx config reconcile` is explicitly not changed and `add` gains no materialisation, so for every hub minted before this lands the configs stay untracked forever and each new pair off it is still born without `_lyx/config/` — reconcile repairs that pair's own disk copy but commits nothing and does not help the next pair.
**Fix:** Say explicitly whether pre-existing hubs are in or out of scope and, if out, name the operator remedy (per-pair `reconcile --apply`, accepted as permanent manual step) so a plan writer does not invent a backfill.

### [NIT:consistency] Scenario 5 hard-codes nine, scenario 1 derives it
**Section:** Testing → 5 (Mutation record)
**Issue:** Scenario 1 deliberately derives the expected module set from `configreg.Modules()` "to keep the test honest when a tenth module is registered", while scenario 5 pins "the nine `file_written` entries" as a literal count.
**Fix:** Derive scenario 5's expected count from the same `configreg.Modules()`-minus-`fabric` source.

## Verdict

REQUEST_CHANGES
Two dispositions missing: hubforge SeedConfig's new empty-commit path, and pre-existing hubs.
MILL_REVIEW_END
