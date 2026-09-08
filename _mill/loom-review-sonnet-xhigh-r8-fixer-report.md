# `loom` fixer report — round 8 (sonnet-xhigh-r8)

Job 2: implementing and verifying every finding from `_mill/loom-review-sonnet-xhigh-r8.md`.
One commit per fix, filled in as each lands green — not reconstructed from memory at the end.

## Table

| ID | Severity | Status | Commit | Change | Test |
|---|---|---|---|---|---|
| SF-1 | BLOCKING | FIXED | `a55b4997b` | `internal/shuttleengine/claudeengine/startup.go` — `startupGateNeedles` is now built from `gateAcceptNeedles` plus the prose-only `filesInThisFolderNeedle`, instead of an independently hand-typed list, closing the "Yes, proceed" gap and making the two lists structurally unable to diverge again | New fixture `TestStartup_Classification/trust_prompt_older_wording_no_prose_in_capture` (option list + footer only, no prose) — fails pre-fix (`StartupReady`), passes post-fix (`StartupTrustPrompt`) |
| PG-1 | BLOCKING | FIXED | `68fa387a3` | `internal/planparser/validate.go` — new `checkGlyphMalformed` implementing `glyph-malformed`: a `#`-shaped ref that fails `glyph.Parse` is now a hard finding, card-generic over Targets/Uses, skipped under `language: none`. Wired into `validate()` right after `checkDirectoryTarget`. Renumbered `contracts/specs/loom-plan-spec.md`'s 27→28-check list (new row 11), updated the 26/27-count prose in `internal/planparser/doc.go`, `contracts/stencils/loom/loom-rubric-plan-review.md`, and `contracts/recipes/loom-recipe.yaml` | New `TestValidate_GlyphMalformed` (5 subtests: clean, doubled-`#` fires exactly once and none of the other shape checks misfire on it, malformed-in-Uses, malformed-in-a-Prosa-group still fires (card-generic, not group-scoped — double-reports alongside `prosa-symbol-target` by design), `language: none` skips it entirely) |
| CW-1 | MEDIUM | FIXED | `dda4dba4c` | `internal/cliwire/callerset_enforcement_test.go` — `callsDerive` now matches any `*ast.SelectorExpr` naming `standalonestate.Derive`, not only ones sitting inside a `CallExpr.Fun` position, catching a function-value capture (`var deriveFn = standalonestate.Derive`) that reaches `Derive` exactly as much as a direct call | New `TestCallsDerive_CatchesFunctionValueIndirection` (fails pre-fix), `TestCallsDerive_DirectCallStillCaught` (plain-shape sanity), `TestCallsDerive_UnrelatedSelectorNotCaught` (no false positive on an unrelated `.Derive()` method) — whole-repo `TestDeriveCallerSet_CliwireOnly` stays green, confirming no false positive against the real tree either |
| CW-2 | MEDIUM | FIXED | `6b8b4ce62` | `internal/cliwire/bannedecl_enforcement_test.go` — extracted `bannedDeclNamesIn`, now walking top-level `var`/`const` `*ast.GenDecl` specs alongside `*ast.FuncDecl`, catching a banned helper re-declared as `var resolveStandaloneTarget = func(...){...}` | New `TestBannedDeclNamesIn_CatchesVarFuncLiteral` (fails pre-fix), `TestBannedDeclNamesIn_FuncDeclStillCaught` (plain-shape sanity), `TestBannedDeclNamesIn_UnrelatedVarNotCaught` (no false positive) — whole-repo `TestBannedDeclarations_CliPackagesCallIntoCliwire` stays green |
| PG-2 | MEDIUM | FIXED | `d52994b8e` | `internal/planglyph/handle.go` — new `cardOwnHandles` folds a Rename pair's still-handle-shaped New side in alongside Create declarations; `BindHandles` now matches those against `delta.Renamed`'s own `To.ID` (a rename is not a create) in addition to `delta.Created`, so a Rename-only card's own handle is no longer permanently skipped | New `TestBindHandles_RenameNewSideHandleBinds` (fails pre-fix — asserts both the declaring AND a referencing card lose the `plan:` prefix), `TestBindHandles_RenameFileSidePairIsNotAHandle` (a file-rename pair's already-glyph sides are correctly left alone), `TestBindHandles_RenameNewSideUnmatchedMismatches` (bind-count-mismatch fires correctly for an unmatched Rename handle too) |
| WS-1 | MEDIUM | FIXED | `743ae60dd` | `internal/websterengine/fingerprint.go` — new exported `Fingerprint` seam; `internal/webstercli/validate.go` — `validate` now acquires the state-mutation lease, loads state, runs `scopedValidate`, then unconditionally re-baselines `state.json`'s `PlanFingerprint` via the existing `persistPlanFingerprintRebaseline` helper, mirroring begin-batch/record-batch's own restamp-immediately-after discipline; corrected the verb's own help text | New `TestValidateCmd_RebaselinesStalePlanFingerprint` (fails pre-fix — a stale seeded fingerprint is left untouched by the old code, corrected by the new code; also asserts other state fields survive the restamp untouched) |
| WS-2 | LOW | FIXED | `1cbfc7c1c` | `internal/websterengine/geometry.go` — corrected `Geometry.WorktreeRoot`'s doc comment, scoping the "anchor-anchored value every CLI call site passes" claim to hub mode explicitly (false for standalone, where it is deliberately the disjoint target) | Doc-only, no behavior change; `go build`/`go vet` clean |
| LS-1 | LOW | FIXED | `4b2099711` | `internal/shedadapters/bouncerfiles.go`/`focus.go` — `recordedVerdict`/`readRoundFocus` now reject a ledger/focus file whose own `round:` frontmatter disagrees with the round number its own filename encodes. Fixing this surfaced and fixed a real, separate, pre-existing stencil bug: `contracts/stencils/bouncer/bouncer-template-judge.md`'s ledger and focus sections both reused the same `{{.round}}` marker despite the two files' own round semantics differing by one (`focusPath(runDir, round+1)`); added a distinct `{{.next_round}}` marker, filled from `judgeCall`'s `n+1` in `bouncer.go` | New `TestRecordedVerdict_LedgerRoundMustMatchItsOwnFilename` (2 subtests: matching round trusted, mismatched round rejected — fails pre-fix), `TestReadRoundFocus_FrontmatterRoundMustMatchItsOwnFilename` (fails pre-fix); updated `TestBouncer_MarkerCompleteness_BothTemplates`/`TestBouncer_StampLeakRegression_BothTemplates` fixtures for the new `next_round` marker |
| CW-4 | NIT | FIXED | `07cbe3d2d` | `internal/cliwire/paths.go` — documented `RepositoryRootOf`/`ResolveToldDir`'s unenforced empty/relative-input precondition explicitly, rather than adding an assertion with no live call path to protect | Doc-only, no behavior change; `go build`/`go test` clean |
| CW-3 | LOW | FIXED | `a66e05573` | `internal/cliwire/seam_enforcement_test.go` (new file) — a Told-Geometry import-allowlist enforcement test for `internal/cliwire`, mirroring `internal/shedrecipe`'s own `seam_enforcement_test.go` pattern | The new `TestToldGeometryInvariant_AllowlistOnly` passes clean against the real package (confirming `doc.go`'s own dependency-set claim is accurate today) |

## Notes

- SF-1: confirmed pre-fix via a throwaway test during the review phase (removed before Job 2 began,
  per the sequencing rule); the permanent regression test added in Job 2 reproduces the same failure
  shape as a named, committed fixture.
- LS-1: implementing the reviewed finding (a missing round cross-check) surfaced a genuine, separate,
  pre-existing production defect (the judge stencil's own `{{.round}}`/`{{.next_round}}` marker
  conflation) that a naive fix would have turned into a live regression — every compliant judge call
  would have had its real focus-file directive silently discarded. Fixed both together rather than
  applying the narrower fix and letting it regress; see the finding's own commit message for the full
  trace.
- All ten findings: FIXED, zero deferred, zero NOT-FIXED-THIS-ROUND.

## Summary of changed files (production + test, this round only)

- `internal/shuttleengine/claudeengine/startup.go`, `startup_test.go` (SF-1)
- `internal/planparser/validate.go`, `validate_test.go`, `doc.go` (PG-1)
- `contracts/specs/loom-plan-spec.md`, `contracts/stencils/loom/loom-rubric-plan-review.md`,
  `contracts/recipes/loom-recipe.yaml` (PG-1 doc/count updates)
- `internal/cliwire/callerset_enforcement_test.go` (CW-1)
- `internal/cliwire/bannedecl_enforcement_test.go` (CW-2)
- `internal/planglyph/handle.go`, `handle_test.go`, `doc.go` (PG-2)
- `internal/websterengine/fingerprint.go` (WS-1, WS-2)
- `internal/webstercli/validate.go`, `cli_test.go` (WS-1)
- `internal/shedadapters/bouncerfiles.go`, `bouncerfiles_test.go`, `focus.go`, `focus_test.go`,
  `bouncer.go`, `bouncer_seed_test.go` (LS-1)
- `contracts/stencils/bouncer/bouncer-template-judge.md` (LS-1)
- `internal/cliwire/paths.go` (CW-4)
- `internal/cliwire/seam_enforcement_test.go` (new file, CW-3)

## Hermetic gates — final full re-run, all fixes applied

See the review report's own closing section for the final cold-gate re-run and the merge-readiness /
convergence verdict.
