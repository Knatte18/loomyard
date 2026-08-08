MILL_REVIEW_BEGIN
# Review: Audit the remaining leaf and seam import invariants — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnetxhigh
reviewer_self_id: claude-sonnet-5
reviewed_file: plan/
date: 2026-08-08
```

## Notes

Cross-checked every factual claim in batch 1's cards against the live source files.
All verified accurate:

- Card 1's three-entry allowlist (`configengine`, `lyxcwd`, `weftname`) matches `lyxtest.go`'s actual imports exactly; `hermetic.go`/`reexecguard.go`/`doc.go` are confirmed import-free or stdlib-only, so "four production files" is correct.
- Card 1's pattern-shape reference (`internal/pattern/leaf_enforcement_test.go`) matches the described allowlist/stdlib-test/WalkDir shape verbatim.
- Card 1(c)/(d)'s "Enforced by" and cross-reference edit targets match the current `CONSTRAINTS.md` and `seam_enforcement_test.go` text precisely, including the "already-committed, do not redo" carve-outs.
- Card 3's claim that `internal/logger` and `internal/shuttleengine` each import `lyxcwd` (making the treadle exclusion a direct-import-only discipline) is correct, and `internal/logger` does not import `lyxtest` (negative-control claim in card 1(f) holds).
- Card 4's stale contrast clause and the sibling "Like modelspec's..." sentences in pattern/lyxtest are exactly as described.
- Card 5's "four further sites" (perchengine doc.go/engine.go, two burlerengine.doc.go claims) and "two more, accurate as-is" (treadleengine seam_enforcement_test.go, tokenvocab/doc.go) were independently re-derived via a repo-wide grep for the overstatement pattern and all match; no unaudited site was found describing a false isolation claim.
- `docs/shared-libs/lyxcwd.md`'s "Dependency direction (Go enforces it)" line, cited in card 1(f)'s mandatory commit body, is verified verbatim.
- `All Files Touched` in the overview is the exact union of every card's `Edits:` list; no Moves anywhere so no rename-mechanic section is required, correctly absent.
- Batch Index DAG is a single, non-cyclic entry referencing an existing file.

No constraint violation, no context-completeness gap, no vague `Requirements:` prose, no over-engineering found.

## Verdict

APPROVE
Every factual and structural claim in the plan checks out against the source files.
MILL_REVIEW_END
