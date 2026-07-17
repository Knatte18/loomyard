MILL_REVIEW_BEGIN
# Review: loom: pin the spawn/handover status schema + discussion-format.md — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-07-17
```

## Findings

None.

Summary of what was verified against source:

- `docs/reference/status-schema.md` and `docs/reference/discussion-format.md` faithfully render
  every discussion Decision (`status-file-format-json`, `status-single-schema-superset`,
  `status-seed-writer-is-a-lyx-command`, `status-field-set`, `verdict-history-granularity`,
  `no-schema-version`, `discussion-on-disk-split`, `decision-record-sections`,
  `support-log-sections`, `doc-rigor-moderate`) — correct section order, house style matching
  `plan-format.md`/`builder-contract.md`, compact validation checklists, worked examples.
- `git mv`-style relocation of `plan-format.md`/`builder-contract.md` to `docs/reference/`
  landed with each file's own internal sibling links correctly fixed (`model-spec.md` sibling
  link, `loom.md` → `../modules/loom.md`, `../overview.md` unchanged, mutual sibling link
  unchanged).
- Repo-wide grep confirms zero surviving `modules/plan-format.md` / `modules/builder-contract.md`
  tokens outside `_mill/` (which legitimately retains historical plan/discussion/review text).
  loom.md's bare-sibling links all resolve via `../reference/…`.
- All six Go-file edits (`hubgeometry.go:223,247`, `buildercli/cli.go:117`, `validate.go:2`,
  `report.go:3`, `doc.go:3`, `template_test.go:285`) are comment/string-only path retargets —
  spot-checked the surrounding code, confirmed no functional change (Decision
  `spec-only-no-functional-go` honored).
- `implementer-template.md`, `SANDBOX-BUILDER-SUITE.md`, and `builder-review-prompt.md` all
  retargeted with surrounding prose intact.
- `docs/modules/loom.md`: status contract points at `../reference/status-schema.md`,
  per-phase-outcome wording corrected, JSON-via-`internal/state` stated, `pause_requested`
  in-status note kept, Setup→Preflight renamed at all three cited sites (phase diagram, prose,
  module table).
- `docs/overview.md`: doc-lifecycle section splits module-design vs durable-contract docs
  (names all five reference docs), loom phase blurb reads
  `Preflight → Discussion → Plan → Builder → Raddle → Finalize`, inbound links retargeted.
- `docs/roadmap.md`: milestone 12 "Contracts first" marked ✅ Done, linking both new docs;
  all `modules/…` inbound links to the relocated docs retargeted, `modules/loom.md` link on the
  same line left untouched as required.
- `docs/long-term-ideas.md`: inline `plan-format.md` path token retargeted.
- No out-of-plan files present; the "All Files Touched" list matches the actual edited set.

## Verdict

APPROVE
Every batch's cards are faithfully realized; link integrity, doc content, and Go-comment-only edits all verified against source.
MILL_REVIEW_END
