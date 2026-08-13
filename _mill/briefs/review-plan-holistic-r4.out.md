MILL_REVIEW_BEGIN
# Review: lyxtest: build real fabric hubs, invert the lyxtest/fabric dependency — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Sonnet 5 (self-assessed)
reviewed_file: plan/
date: 2026-08-13
```

## Findings

### [BLOCKING:scope] NewPairedForTest call-expression count is wrong, and the counting method it claims avoids the error actually reproduces it
**Location:** batch 8 `## Batch Scope` and card 50/51
**Issue:** The scope narrative claims "eleven real call expressions... five in warpforward_integration_test.go, four in weftgit_exclude_test.go, one in checkout_index_refresh_test.go, and one in fabric_test.go," obtained via `grep -o 'fabricengine\.NewPairedForTest('` specifically to avoid over-counting `t.Fatalf` message-string mentions. Reading `weftgit_exclude_test.go` directly (lines 70/72 and 95/97) shows exactly **two** real call expressions (`f, err := fabricengine.NewPairedForTest(...)`), each paired with a `t.Fatalf("fabricengine.NewPairedForTest(%q, %q): %v", ...)` whose format string is a textual, not call, match — `grep -o` on the literal string does not distinguish these, so it double-counts. Actual total is nine, not eleven (5+2+1+1); "ten go away, one stays" should read "eight go away, one stays."
**Fix:** Correct the count in the Batch Scope prose and card 50's Requirements ("the four `fabricengine.NewPairedForTest(` calls" → "the two calls"), so the implementer isn't sent looking for two call sites that don't exist.

### [BLOCKING:scope] The lyxtest/fabrictest-prose sweep leaves several production/test files with stale references that card 69's zero-hit grep gates require to be gone
**Location:** batch 1 card 2 (sweep mechanism); batch 11 card 69 (`grep -rn 'lyxtest'`/`'fabrictest'` — zero hits required)
**Issue:** Card 2's sweep is `s{internal/lyxtest}{internal/gitkit}g; s{\blyxtest\.}{gitkit.}g` — it only rewrites the import-path substring and the dot-qualified selector form. It misses possessive/bare-word prose ("lyxtest's...", "a lyxtest synthetic hub", "lyxtest fixtures live under..."). Concretely, these files are never named in any batch's `Context:` or `Edits:` anywhere in the plan, yet contain such prose today: `internal/lyxcwd/lyxcwd.go:97` ("lyxtest injects anchors into synthetic hubs"), `internal/lyxcwd/anchor.go:108,148`, `internal/weftname/weftname.go:2,31`, `internal/weftname/weftname_test.go:3,60`, `internal/gitrepo/keyvalidation_test.go:2`, `internal/perchcli/run_test.go:10`, `internal/perchcli/cli_test.go:9` (the perchcli files, distinct from the migrated `*_integration_test.go` siblings). Batch 12 card 75 fixes the *doc* `docs/shared-libs/lyxcwd.md`'s mirrored sentence but not the source file it mirrors. Card 69's literal "zero hits" grep will fail on these, with no card owning the fix.
**Fix:** Add these files (at minimum `internal/lyxcwd/lyxcwd.go`, `internal/lyxcwd/anchor.go`, `internal/weftname/weftname.go`, `internal/weftname/weftname_test.go`) to a card's `Edits:`/`Context:` for a targeted prose fix, or widen card 2's sweep/verification step to catch bare-word and possessive forms before card 69 runs.

### [NIT:consistency] Batch 8 card 52 miscounts its own batch's commit-bearing cards
**Location:** batch 8, card 52
**Issue:** Card 52 says "cross-read this batch's twelve commit messages against the diff." Batch 8 has 11 cards (42–52); card 52 itself carries `Commit: none`, so only ten cards (42–51) produce a commit.
**Fix:** Change "twelve" to "ten."

### [NIT:consistency] Batch 4's coverage-gap count is off by one
**Location:** batch 4, `## Batch Tests`
**Issue:** States "a real coverage gap for those five call sites" for the compile-checked-only `internal/burlerengine`/`internal/shuttlecli` files. Card 25 migrates 2 `CopyPaired` sites (one per file) and card 26 migrates 4, for 6 total, matching source (`smoke_cluster_test.go`+`smoke_round_test.go`: 2; `smoke_guardrail_test.go`+`smoke_interrupt_test.go`+`smoke_run_test.go`: 4).
**Fix:** Change "five" to "six."

## Verdict

REQUEST_CHANGES
Two scope-class miscounts threaten batch 11's own completion gate and a card's call-site instruction; fix before proceeding.
MILL_REVIEW_END
