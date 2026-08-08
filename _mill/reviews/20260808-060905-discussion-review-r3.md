MILL_REVIEW_BEGIN
# Review: Scoutengine: rewrite CONSTRAINTS.md as a seam rule, convert leaf test to banned-list, add LSP guard

```yaml
verdict: APPROVE
reviewer_model: opus
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-08
```

Verified against source: `CONSTRAINTS.md:66-79` matches the discussion's description of the pre-staged section exactly (banned-list framing, `clihelp` named, "never be described as stdlib-only or hermetic"). `leaf_enforcement_test.go` line numbers all check out — header 1–7, `allowedImports` 22–28, stdlib heuristic 61–70, three predicates 77–88, catch-all 90–91, `t.Errorf` 101. `lspclient.go` imports 13 stdlib packages plus `internal/logger`, with `logger.Warn` at 564/567/572/595/598. `internal/logger` does import `lyxcwd` (sink.go:22) and `proc` (retention.go:18). `clihelp` imports cobra in `exec.go:21`/`jsonhelp.go:13` and does not end in `cli`. `doc.go:24-26` enumerates the allowlist and omits `logger`. `docs/overview.md:252` says "cycle-free leaf"; `:362` designates the package doc as scout's module doc. `shuttleengine/seam_enforcement_test.go` uses `os.ReadDir` with the quoted scope comment at 37–39. `probe.go` imports only `context`/`time`. No other doc, test, or production file restates scout's allowlist or names the old section title.

## Findings

### [NOTE] Seam test's own vacuity case unaddressed
**Section:** Testing / "scans with `os.ReadDir`" **Issue:** The guard test must `t.Fatal` when `lspclient.go` is absent, but nothing says the seam test must fail rather than pass silently if `os.ReadDir` yields zero non-test `.go` files. **Fix:** State whether a files-scanned assertion is required, or record that the `shuttleengine` model's vacuity behaviour is inherited deliberately.

### [NOTE] Failure-message path form under the ReadDir switch
**Section:** "Banned-list test: rename the file, reuse the three existing predicates" **Issue:** The predicates are "kept verbatim" but each appends `path` from `WalkDir`; only the deleted catch-all used `filepath.Rel`, so post-conversion every message prints an absolute path while the named model prints `entry.Name()`. **Fix:** Say which form the four messages use.

### [NOTE] `doc.go`'s modelspec cross-reference outlives the leaf framing
**Section:** "`doc.go` and `docs/overview.md` land in the same commit" **Issue:** Lines 30–34 call `internal/modelspec` "the shape this package mirrors most directly" and claim scout is cycle-free "the same way internal/modelspec already is"; modelspec stays an allowlisted leaf, so keeping those sentences re-imports the framing being removed. **Fix:** Note explicitly that the modelspec comparison goes with the rest of the paragraph.

## Verdict

APPROVE
Every source claim verified; three non-blocking wording and vacuity details remain for the plan.
MILL_REVIEW_END
