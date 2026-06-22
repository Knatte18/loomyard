MILL_REVIEW_BEGIN
# Review: Prune and consolidate the test suite (board first) — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-06-22
```

## Findings

### [BLOCKING] TestRenderLayerBuckets unaccounted in fold and name-map

**Location:** `internal/board/render_test.go`, `docs/benchmarks/test-suite-timing.md`
**Issue:** `TestRenderLayerBuckets` is in the baseline (`_mill/plan/baseline/board.txt`) but does not appear as a top-level function, named subtest, or documented drop anywhere — it is silently absent from both the implementation and the Card 17 equivalence guardrail. Card 1(a) explicitly lists it as a fold target for `TestRenderProposalAndShapesHomepage`, yet no case with that name was added to the table.
**Fix:** Add a `TestRenderLayerBuckets` case to `TestRenderProposalAndShapesHomepage` asserting bucket-header substrings (e.g., `# Layer A`, `# Layer B`), then add the corresponding entry to the timing.md name-map.

### [BLOCKING] Equivalence guardrail name-map is intentionally incomplete

**Location:** `docs/benchmarks/test-suite-timing.md:152`
**Issue:** The board folded-names section ends at `TestRenderIsolatedTask` with the placeholder "(Additional folded names continue below; complete list preserved in the subtest names themselves.)" — this is not a complete name-map. Card 17 requires every removed baseline name to map to a surviving subtest or a documented drop; the trailing handwave satisfies neither condition and leaves at least the board names `TestRenderMissingDependency`, `TestRenderTaskIDFormatting`, `TestRenderBrief`, `TestRenderLayerBuckets`, `TestRenderSidebarBlanks`, `TestRenderExtendedTitle`, `TestRenderOrphanDetection`, `TestRenderSingleTask`, `TestRenderStatusVariants`, `TestRenderToDisk`, `TestUpsertTask`, and `TestRerender` unmapped.
**Fix:** Complete the board name-map by listing every baseline name removed from the top-level `-list` output with its surviving `t.Run` path or drop justification; remove the placeholder sentence.

### [NIT] Count headers in guardrail section contradict their own lists

**Location:** `docs/benchmarks/test-suite-timing.md:158,178`
**Issue:** The header "weft (5 dropped)" precedes a list of 6 names; "muxpoc (5 dropped)" precedes a list of 8 names. The counts conflate "dropped" (removed without subtest) with "folded" (renamed to subtest), making the guardrail harder to audit.
**Fix:** Correct the header counts or split each package's section into "folded" and "dropped" sub-lists matching the actual item counts.

### [NIT] os.Getwd introduced in newly-written TestRunCLIErrors

**Location:** `internal/ide/cli_test.go:71-73`
**Issue:** The new `TestRunCLIErrors` table loop uses `oldCwd, _ := os.Getwd()` / `defer os.Chdir(oldCwd)` in each subtest body — raw `os.Getwd` is banned outside `internal/paths` and `cmd/lyx/main.go` per the path invariant. The pre-existing `TestRunCLISpawnDispatch` (not edited) has the same pattern; if enforcement_test.go already passes for that file, this fold perpetuates rather than introduces the violation, but the shared-decision says "test edits must not introduce" new occurrences.
**Fix:** Replace `os.Getwd()` + `defer os.Chdir(oldCwd)` with `t.Chdir(gitRepo)` (which auto-restores on cleanup), consistent with the `t.Chdir` pattern used throughout board and muxpoc tests.

## Verdict

REQUEST_CHANGES
`TestRenderLayerBuckets` is unaccounted (missing subtest + missing name-map entry), and the equivalence guardrail is explicitly marked incomplete.
MILL_REVIEW_END