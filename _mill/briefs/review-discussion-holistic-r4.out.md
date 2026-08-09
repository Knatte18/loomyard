MILL_REVIEW_BEGIN
# Review: Rename the fabric host vocabulary to warp, and name the composite repo Fabric

```yaml
verdict: GAPS_FOUND
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-09
```

## Findings

### [GAP] JSON struct tags are swept but never mentioned
**Section:** Scope "Out: Any behaviour change" / Technical context
**Issue:** `internal/fabricengine/status.go:40,44`, `reconcile.go:73` and `prune.go:22` carry `json:"host_worktree"` / `json:"host_branch"` tags; a blanket `host`→`warp` swap renames the `lyx fabric status/reconcile/prune --json` field names, which is an observable output-contract change, not "an identifier, a filename, a comment, a help string, or prose".
**Fix:** State explicitly whether the JSON tags rename (and that only `classify_test.go` consumes them, so nothing external breaks) or are preserved verbatim and excluded from the sweep.

### [GAP] Doc sweep mechanism unspecified — swap vs "Fabric" rewording
**Section:** Decisions "Commit granularity" (d) / Technical context "Documentation surface"
**Issue:** The vocabulary rule says consumer prose uses **Fabric** for the composite and warp/weft only for two-sided distinctions, yet line 477 implies `wordswap` runs over `.md` files and the historical-record exclusions are framed as "not swept"; a mechanical swap over `docs/overview.md`, `README.md` and the eight `SANDBOX-*-SUITE.md` templates yields "warp repo" exactly where the rule demands "the Fabric repo".
**Fix:** Split commit (d)'s doc work into the mechanically-swept set (identifier citations, `.claude/agents/*:16`, owner prose) and the hand-reworded consumer-prose set, and say which files fall in each.

### [GAP] `-skip` semantics contradict the exit-code rule
**Section:** Decisions "Ambiguous compounds are reported, not guessed" / Testing
**Issue:** Line 84/114 make exit-zero-with-an-explicit-`-skip`-set the completion condition, while the Testing scenario at line 503 requires `-skip` matches to be "left untouched **and reported**" — if reported occurrences drive the non-zero exit, the run can never clear.
**Fix:** Specify two distinct report buckets — unresolved AMBIGUOUS (fails the exit code) vs deliberately skipped (informational only) — and pin that split in the TDD tests.

### [GAP] Stale "exclude list is empty / no `-skip`" decision text
**Section:** Decisions "Exclude list is empty — reword the one verb-sense hit"; Q&A lines 541–542
**Issue:** That decision heading and line 134 ("this rename runs with no `-skip` argument"), plus the two Q&A entries ("No" to whether "host" appears in the skip pattern), directly contradict lines 116/291, which require a `-skip` naming `poll_test.go:212`'s "hosting" — a pattern that necessarily contains the word `host`.
**Fix:** Retitle and rewrite that decision to "exclude list is empty in the fabric packages only", and correct the two superseded Q&A answers so a plan writer cannot read the stale form as authoritative.

### [NOTE] `internal/lyxcwd` touch list is incomplete
**Section:** Constraints, "Cwd Resolution Invariant" bullet
**Issue:** It claims lyxcwd is touched "only in its enforcement test and in `anchor_test.go`'s `CopyHostHub` call sites", but `internal/lyxcwd/lyxcwd_test.go:25,68,91` also call `CopyHostHub`.
**Fix:** Add `lyxcwd_test.go` to the touched set so the `wordswap` file list is complete (a miss compiles-fails, so this is cost, not risk).

### [NOTE] Tightening also invalidates existing guard text and a sub-test
**Section:** Testing, "`TestEnforcement_FabricVocabulary` needs its own new cases"
**Issue:** Only new cases are listed, but the tightening also falsifies the `owner_set_file_with_all_of_the_above_passes` sub-test's second assertion and comment (`enforcement_test.go:787–798`) and the "card 26 scopes host to the same files" prose at `:591–595` and `:734–744`.
**Fix:** Name those three sites as edits belonging to commit (d), not just the added cases.

## Verdict

GAPS_FOUND
JSON tag renames, doc-sweep mechanism, and `-skip` exit semantics need resolving before planning.
MILL_REVIEW_END
