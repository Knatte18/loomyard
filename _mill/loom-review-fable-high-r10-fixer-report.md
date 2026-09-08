# loom glyph-plan-format surface — round 10 fixer report (fable-high-r10)

Companion to `_mill/loom-review-fable-high-r10.md`. One row per finding, filled in as each fix lands, never reconstructed at the end.

## Fix table

| Finding | Severity | Status | Commit | Tests added/extended | Files changed |
|---|---|---|---|---|---|
| F1 | MEDIUM | fixed | `f849ab6d2` | `TestDoneCheckVerdicts_UnreadableStatusFailsClosed` (new), `TestDoneCheckVerdicts_Rules` (+4 ambiguous rows) | `internal/planglyph/donecheck.go`, `internal/planglyph/doc.go`, `internal/planglyph/resolve.go`, `internal/planglyph/donecheck_test.go` |
| F2 | MEDIUM | fixed | `5589a619c` | `TestEnsureResolveCoverage` (new) | `internal/planglyph/repo.go`, `internal/planglyph/repo_test.go` |
| F3 | MEDIUM | fixed | `d2cc7df2b` | `TestMatchHandleResults` (new) | `internal/planglyph/create.go`, `internal/planglyph/create_test.go` |
| F4 | MEDIUM | fixed | `e6e1d26bb` | `TestSyntacticContainment` (+4 self-vs-self subtests) | `internal/planparser/containment.go`, `internal/planparser/containment_test.go`, `contracts/specs/loom-plan-spec.md` (row 23), `manifest/designs/quarry-glyph-plan-alphabet.md` |
| F5 | LOW | fixed | `c1c2e5aa8` | `TestValidate_RenamePairShape` (+1 self-old/handle-new subtest), `TestCanonicalizeHandles_RenameOldSelfGlyphNamesTheShapeMistake` (new) | `internal/planparser/validate.go`, `internal/planparser/validate_test.go`, `internal/planglyph/handle.go`, `internal/planglyph/handle_test.go`, `contracts/specs/loom-plan-spec.md` (row 18) |
| F6 | LOW | fixed | `097a4a691` | `TestCanonicalizeHandles_OneDraftTwoCanonicalsRewritesNothing` (new) | `internal/planglyph/handle.go`, `internal/planglyph/handle_test.go` |
| F7 | LOW | pending | — | — | — |

## Deferred (with reasons)

- OBS-1, OBS-2, OBS-3 — remain deferred/accepted per the review report's "Deferred items" section (re-evaluated this round; unchanged disposition).

## Verification

- Gate commands re-run after every fix; final full-suite run recorded at the bottom once all fixes land.
