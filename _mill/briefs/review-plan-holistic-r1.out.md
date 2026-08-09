MILL_REVIEW_BEGIN
# Review: plan-format: drop the v3 suffix and sweep every reference by script — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnetxhigh
reviewer_self_id: claude-sonnet-4-5 (Sonnet, "Sonnet 5" per harness naming)
reviewed_file: plan/
date: 2026-08-09
```

## Verification performed

Read the overview and all four batches, plus every source file the batches reference (docs/reference/plan-format-v3.md, manifest/designs/loom.md, manifest/roadmap.md, manifest/designs/shed-followups.md, internal/planparser/{validate,validate_test,parse_test}.go, internal/websterengine/{classify,template_test}.go, internal/webstercli/cli_test.go, tools/sandbox/SANDBOX-WEBSTER-SUITE.md, CONSTRAINTS.md, docs/overview.md, docs/reference/model-spec.md, docs/reference/webster-contract.md, and the five "deliberately-untouched v2" files). Cross-checked with direct greps:

- The six-pattern sweep regex against the whole tree reproduces exactly the plan's claimed 30-file hit set (Card 2's `Edits:` list, 31 entries, minus `.scratch/sweep/main.go` which has no current hit) plus `manifest/designs/shed-followups.md`, correctly excluded.
- `gopkg.in/yaml.v3` import count = 32 (confirmed by grep), matching Card 3/12/13's "corrected from ten to 32" claim exactly, and CONSTRAINTS.md's only two `v3` occurrences are that same import, matching the override note in Card 13 verbatim.
- Every quoted comment/string in Cards 5, 6, 9, 10, 11, 13 was checked character-for-character against the live files (`plan-format-v3.md`, `loom.md`, `validate.go`, `validate_test.go`, `parse_test.go`, `classify.go`, `template_test.go`, `cli_test.go`) — all quotes are exact, including line-shift-sensitive ones (Card 4 vs Card 5 ordering) and the two-line "moved-since-shed-followups" correction for `loom.md:58`'s `plan-v3` site.
- `docs/reference/plan-format-v3.md`'s every `v2` occurrence (9 lines) is accounted for exactly by Card 4 (blockquote deletion) + Card 5's seven numbered items — no residue, no missed site.
- The "collapsed both links onto the same target" defect Card 6 describes in `loom.md:29` is real and reproducible: the sweep's `plan-format v3` → `plan-format` and `plan-format-v3.md` → `plan-format.md` substitutions do independently collapse both links onto `../../docs/reference/plan-format.md`.
- The five "deliberately-untouched v2" Go files (`state_test.go`, `reset_test.go`, `reconcile_test.go`, `command.go`, `burlerengine/doc.go`) all contain only unrelated `v2` tokens (a variable name, git-fixture content, a YAML test value, and Claude-Code version strings) — correctly excluded.
- Batch Index DAG is acyclic, `depends-on` accurate (1←nothing; 2,3←1; 4←2,3), all four `file:` targets exist, card numbering 1–14 is unique/sequential, `## All Files Touched` (34 entries) matches the derived union of every card's Edits/Creates/Move-targets exactly.

No constraint violation, no vague `Requirements:`, no Context-completeness gap, no full-file rewrite of the moved file, and the Rename mechanic section is present and correctly scoped to the one batch with a non-empty `Moves:`. The plan's own override-note mechanism (Card 13) correctly reconciles every place it diverges from `shed-followups.md`'s original scoping, and each divergence claim I checked (pattern count, hard-exclusion count, the Invariant-wording claim, the two prose-repair claims) held up against the live tree.

## Findings

None.

## Verdict

APPROVE
Every checked claim, quote, count, and file-list cross-reference in the plan matched the live tree exactly.
MILL_REVIEW_END
