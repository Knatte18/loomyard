MILL_REVIEW_BEGIN
# Review: Rename the fabric host vocabulary to warp, and name the composite repo Fabric — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnetxhigh
reviewer_self_id: Claude (Sonnet 5, model id claude-sonnet-5)
reviewed_file: plan/
date: 2026-08-09
```

## Notes

Read the overview, all 7 batches, `_mill/discussion.md`, `CONSTRAINTS.md`, and cross-checked plan claims directly against source across ~20 files: `internal/lyxcwd/enforcement_test.go` (all cited line numbers for `fabricVocabularyOwners`, `hostPhrases`, `hostGeometryIdentifiers`, `shouldSkipBareVocabularyCheck`, `failsBareVocabularyCheck`, `fabricVocabularyHits`, and the falsified sub-test at lines 787–798 all match exactly), `internal/fabricengine/{status,prune,reconcile,coalesce,drift,hostclean,hostlayout,hostjunction_test}.go`, `internal/boardengine/board.go`, `internal/weftname/weftname.go`, `internal/lyxtest/lyxtest.go:128`, `internal/fabricengine/post-checkout.sh`, `internal/fabriccli/{fabric,clone}.go`, `internal/configcli/configcli.go:269`, `internal/builderengine/spawn.go`, `internal/buildercli/poll.go:321`/`poll_test.go:212`, `tools/sandbox/{main,suite,report}.go`, all 8 `SANDBOX-*-SUITE.md` occurrence counts, `docs/overview.md`, `README.md`, `docs/shared-libs/{lyxcwd,configengine}.md`, `manifest/designs/{loom,fabric-unified-view}.md`, and `.claude/agents/crucible-reviewer-low.md`.

Every specific line-number and exact-quote claim checked (~40 independent spot checks) matched the current source byte-for-byte, including subtle ones: the card-8 sweep's 94-file count, the exact 5-entry AMBIGUOUS bucket (`drift.go:3`, `hostclean.go:1`, `hostjunction_test.go:1`, `hostlayout.go:1`, `lyxtest.go:128`), the single `-skip` claim (`poll_test.go:212`), the JSON tag sites (`status.go:40/44`, `prune.go:22`, `reconcile.go:73` — correctly omitting a nonexistent `host_branch` tag on the latter two), card 14's correction of the discussion's sandbox-error-string undercount (`suite.go:323/325` duplicating `report.go:57/59`), and card 20's occurrence counts per `SANDBOX-*-SUITE.md` file, which exactly match current grep counts and correctly supersede the discussion's stale (pre-widening) figures.

Structural checks all pass: Batch Index DAG is a clean linear chain (1→2→…→7) with all `file:` references present; card numbering is globally sequential 1–25 with no gaps; each batch's `cards:` count matches its actual card count; `Moves:` pairs are well-formed backtick/` -> ` pairs and batch 4 carries the required `## Rename mechanic` section prescribing `git mv` + surgical edits (no full-file rewrites of moved files); `All Files Touched` correctly includes both pre-rename and post-rename `fabricengine` filenames (justified — the old names are independent `Edits:` targets in batch 3 before being moved in batch 4) and correctly excludes the pure move-source `host-visibility.md`; every `Decision:` in Shared Decisions is traceable to a concrete card requirement; `Requirements:` fields cite exact identifiers/lines throughout rather than vague prose; `Context:`/`Edits:` sets are self-contained for every card checked, including the judgment-heavy doc-sweep cards.

No constraint violations, no forward dependencies, no over-engineering, no missing edge-case handling found. This is a very thoroughly discussion-refined plan (4 rounds recorded) and it shows in the accuracy of every verifiable claim.

## Verdict

APPROVE
No findings; every checked claim verifies against source and the plan is structurally sound.
MILL_REVIEW_END
