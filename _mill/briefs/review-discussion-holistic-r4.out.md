MILL_REVIEW_BEGIN
# Review: fabric: clone doesn't commit written module configs

```yaml
verdict: APPROVE
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-26
```

## Findings

### [NIT:scope] Two doc surfaces named implicitly, not explicitly
**Section:** `docs-in-the-same-commit` **Issue:** `internal/hubforge/seed.go`'s `SeedConfig` doc comment currently explains its commit shape (where it runs and why) and gains an unexplained `--allow-empty`, and `internal/fabriccli/clone.go`'s file-header comment (lines 1-6) enumerates the same wiring sequence as `CloneAndWire`'s doc; neither is on the doc list. **Fix:** name both alongside `CloneAndWire`'s doc comment so the plan writer does not leave a stale sequence enumeration or an unjustified flag.

### [NIT:design] Error-path Decision carries no named verification
**Section:** `error-path-returns-zero-result` / Testing **Issue:** the Decision pins that a commit failure returns `fabricengine.CloneResult{}, err` with the accumulated record intact, but no Testing scenario covers it — §8's sweep only exercises the success path. **Fix:** either name a scenario (or a seam for injecting the failure) or state explicitly that the path is covered by the shared `defer` idiom and needs no new test.

### [NIT:consistency] Adopt-path claim stated absolutely
**Section:** `commit-only-applied-modules` **Issue:** "`ReconcileAll` then reports `Applied: false` for every module" holds only when the adopted `origin/<branch>-weft` carries configs already matching current templates; a drifted config still reports `Applied: true` (`configsync_test.go`'s stale-key case), so the empty-`relPaths` no-op is possible, not guaranteed. **Fix:** reword to "reports `Applied: false` for every module whose committed config already matches the template" — the resulting behaviour is correct either way.

## Verdict

APPROVE
Decisions complete and source-verified; three non-blocking wording and doc-scope nits.
MILL_REVIEW_END
