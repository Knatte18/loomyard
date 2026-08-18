MILL_REVIEW_BEGIN
# Review: invariants and docs for the told-geometry rule

```yaml
duration_s: 165.0
verdict: APPROVE
reviewer_model: opus
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-18
```

Spot-verified against source: `preflight/predicates.go:110-134` (degrade/refuse branches exactly as described),
`lyxcwd/lyxcwd.go:109-184` (`hubPath := filepath.Dir(workTreeRoot)` unconditional, `RepoName` = `TrimSuffix(Base(hubPath), hubSuffix)`, `ErrNotAGitRepo`),
CONSTRAINTS.md section line numbers (8 / 47 / 499, 39 `##` sections), `docs/overview.md:63/266/318/320`,
the five `manifest/roadmap.md` links (14, 108, 111, 114, 117) and `internal/buildinfo/doc.go:5` as the only other reference to the deleted doc,
`docslink_test.go:396-399`'s two allowlist entries, `tierpurity_test.go:28-43`'s fourteen `allowedSpawners` entries,
the absence of `doc.go` in `configengine`/`webstercli`/`scoutcli`, `hubgeom`'s production `lyxcwd` import (`hubgeom.go:9`, `webstergeom.go:8`),
and `scoutengine`'s two remaining `lyxcwd` mentions being comments only.
The Config Strictness pinned sets match the tree exactly today: degrading `{perchengine, websterengine, batcher, shuttleengine, reedengine}`, strict `{boardengine, loomengine, fabricengine}` — the guard will pass as pinned, no drift to reconcile.

## Findings

### [NIT:consistency] Enforcement/audit set delta omits two packages
**Section:** "Enforcement basis" + "`doc.go` audit"
**Issue:** Both lists are described as "the packages converted by the producers-standalone waves", but the machine-enforced list contains `internal/shedengine` and `internal/treadleengine`, which are absent from the fifteen-package audit set; the audit section enumerates the delta in the other direction only (`preflight`, `webstercli`, `scoutcli`, geom adapters) and never accounts for these two.
**Fix:** Add one clause noting that `shedengine`/`treadleengine` are bound and machine-enforced but were never in the converted set, so they are deliberately outside the `doc.go` audit.

### [NIT:consistency] "do not mention it at all" is false for configengine
**Section:** Technical context, verified-state bullet 1
**Issue:** The claim that `configengine` does not mention `lyxcwd` at all holds for production only — `internal/configengine/config_test.go:678,693,708` carry the bare token in comments, which will confuse the re-verifier the same bullet instructs to grep bare `lyxcwd`.
**Fix:** Scope the sentence to production files, matching how the rest of the bullet is worded.

## Verdict

APPROVE
Decisions complete, source claims verified accurate; two wording nits only.
MILL_REVIEW_END
