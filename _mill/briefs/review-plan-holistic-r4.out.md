MILL_REVIEW_BEGIN
# Review: PATTERN directives: move from Go constants to stencil files — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Sonnet 4.5 (Anthropic), self-assessed
reviewed_file: plan/
date: 2026-08-16
```

## Findings

No findings.

Verification performed: read all four plan files (00-overview.md, 01/02/03), the full `_mill/discussion.md`, and every source file named across all batches' `Context:`/`Edits:` lists (`internal/pattern/pattern.go`, `pattern_test.go`, `leaf_enforcement_test.go`, `doc.go`; `internal/loomengine/plan.go`, `prompt_test.go`, `plan_test.go`, `discussion_test.go`; `internal/burlerengine/engine.go`, `prompt_test.go`; `internal/websterengine/render.go`, `template_test.go`; `internal/stencilstore/{stencilstore,reconcile,validate}.go`; `internal/stencil/stencil.go`; `stencils/stencils.go`, `registry_test.go`; `.gitattributes`; `internal/fabricengine/junctionnames.go`; `manifest/designs/pattern-directive-stencils.md`; `manifest/roadmap.md`; `tools/sandbox/SANDBOX-CORE-SUITE.md`; `docs/shared-libs/stencil.md`).

Cross-checks that passed:
- **Batch Index DAG**: 1→2→3 linear, no cycle, all three `file:` targets exist, batch-local `cards:` counts (2+7+4=13) match global step numbering 1–13 with no gaps.
- **All Files Touched** union: hand-derived the union of every card's `Creates:`/`Edits:` targets across all 13 cards and it matches the overview's `## All Files Touched` list exactly (19 entries, alphabetical).
- **Moves**: every card states `Moves: none`; no `## Rename mechanic` section is present or required — correct per the criterion.
- **Context completeness**: spot-checked every card's `Requirements:` against its `Context:`/`Edits:` list (e.g. card 3's `stencilstore.Read`/`stencil.StripLeadingComment` → both in Context; card 9's `fabricengine.StencilsDir` → `junctionnames.go` in Context; card 6's `TestDiscussionSpec_MissingStencilsDirIsHardError` precedent → `discussion_test.go` in Context); no gaps found.
- **Source-grounded factual claims**: verified against live source rather than trusted — `Directive`'s current signature/constants (pattern.go:56-121), `stencilstore.Read`'s signature and error-wrap shape (reconcile.go:28-34), `stencil.StripLeadingComment`/`TopLevelMarkers` (stencil.go), `internal/stencilstore`'s actual imports (`internal/stencil` + `internal/logger`, confirming the plan's "not a leaf" claim), `internal/logger`'s imports (lyxcwd/lyxdirs/proc — no cycle back to `pattern`, confirming the Decision's acyclic-closure claim), both webster call sites' current inline map-literal shape (render.go:173-189, 230-248, confirming the design doc's "not plumbing-free" correction is accurate and the pre-existing design doc is indeed wrong), burler's `e.stencilsDir`/`e.layout` fields and call-site position before instruction-dir creation (engine.go), loom's `PlanSpec` signature already taking `stencilsDir` (plan.go), the `.gitattributes` insertion point (line 21/22 boundary), `stencils.go`'s 15 existing `entries` rows (→18 after this task, matching card 13's "eighteen" correction and the single `SANDBOX-CORE-SUITE.md:232` occurrence of "fifteen" — no second stale count found), `seedHubStencils`'s two call sites in `template_test.go` (testLayout/patternActiveLayout, matching card 9's split-and-reuse plan), and `stencilstore.Reconcile`'s actual `writeStamped`/`ApplyStamp(shipped, BodyHash(shipped))` sequence (matching card 5's stamped-fixture helper design exactly).
- **CONSTRAINTS.md Pattern Leaf Invariant**: current text confirmed to read exactly "stdlib, `internal/lyxcwd`, and `internal/lyxdirs`"; card 4's planned amendment (add two entries, distinct justifications, leave the feature-package clause and Enforced-by line untouched) is precisely scoped to what needs to change and nothing else.
- **Decision alignment**: every `### Decision:` in the overview's Shared Decisions traces to a `_mill/discussion.md` Decision with matching rationale, and every batch/card implements it faithfully (fail-loud error return, lazy read matrix, strip-only no LF-normalise, two-entry leaf allowlist with split justifications, `(l, stencilsDir, role)` order, full-suite verify, marker/vocabulary-clean stencils, keep-and-correct design doc).
- **Requirements specificity**: every card names concrete identifiers (function/const/field names) rather than vague prose; no BLOCKING-vague requirement found.

This is an unusually tightly source-verified plan (round 4) — every factual claim I checked against live source held exactly as stated, including several claims that correct an earlier, now-superseded design doc.

## Verdict

APPROVE
All decisions, DAG integrity, file-touched accounting, and cross-batch facts verified against source; no findings.
MILL_REVIEW_END
