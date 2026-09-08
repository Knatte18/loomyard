# loom glyph-plan-format surface — round 10 fixer report (fable-high-r10)

Companion to `_mill/loom-review-fable-high-r10.md`. One row per finding, filled in as each fix lands, never reconstructed at the end.

## Fix table

| Finding | Severity | Status | Commit | Tests added/extended | Files changed |
|---|---|---|---|---|---|
| F1 | MEDIUM | fixed | `f849ab6d2` + `faf954e3c` | `TestDoneCheckVerdicts_UnreadableStatusFailsClosed` (new), `TestDoneCheckVerdicts_Rules` (+4 ambiguous rows), `TestDoneChecks_RootFilenameCreateLanded` (expectation updated: the bare-token rejection now reports glyph-rejected, still blocking) | `internal/planglyph/donecheck.go`, `internal/planglyph/doc.go`, `internal/planglyph/resolve.go`, `internal/planglyph/donecheck_test.go`, `internal/planglyph/donecheck_integration_test.go` |
| F2 | MEDIUM | fixed | `5589a619c` | `TestEnsureResolveCoverage` (new) | `internal/planglyph/repo.go`, `internal/planglyph/repo_test.go` |
| F3 | MEDIUM | fixed | `d2cc7df2b` | `TestMatchHandleResults` (new) | `internal/planglyph/create.go`, `internal/planglyph/create_test.go` |
| F4 | MEDIUM | fixed | `e6e1d26bb` | `TestSyntacticContainment` (+4 self-vs-self subtests) | `internal/planparser/containment.go`, `internal/planparser/containment_test.go`, `contracts/specs/loom-plan-spec.md` (row 23), `manifest/designs/quarry-glyph-plan-alphabet.md` |
| F5 | LOW | fixed | `c1c2e5aa8` | `TestValidate_RenamePairShape` (+1 self-old/handle-new subtest), `TestCanonicalizeHandles_RenameOldSelfGlyphNamesTheShapeMistake` (new) | `internal/planparser/validate.go`, `internal/planparser/validate_test.go`, `internal/planglyph/handle.go`, `internal/planglyph/handle_test.go`, `contracts/specs/loom-plan-spec.md` (row 18) |
| F6 | LOW | fixed | `097a4a691` | `TestCanonicalizeHandles_OneDraftTwoCanonicalsRewritesNothing` (new) | `internal/planglyph/handle.go`, `internal/planglyph/handle_test.go` |
| F7 | LOW | fixed | `ed3669e91` | `TestValidate_DirectoryTarget` (+1 detail-remedy subtest) | `internal/planparser/classify.go`, `internal/planparser/validate.go`, `internal/planparser/validate_test.go`, `contracts/specs/loom-plan-spec.md` (directory-target row) |

## Deferred (with reasons)

- OBS-1, OBS-2, OBS-3 — remain deferred/accepted per the review report's "Deferred items" section (re-evaluated this round; unchanged disposition).

## Verification

All commands run with `CGO_ENABLED=1`, after the last fix (`faf954e3c`), all green:

- `go build ./...` — exit 0 (also run after every individual fix).
- `go vet ./internal/planparser/... ./internal/planglyph/... ./internal/loomcli/... ./internal/websterengine/... ./internal/webstercli/...` — clean.
- `go test -count=5` over the same five packages — all ok.
- `go test -tags integration` over the same five packages — all ok (one expectation deliberately updated: `TestDoneChecks_RootFilenameCreateLanded` now pins F1's accurate glyph-rejected refusal for the bare-token rejection, in place of the create-not-done misreading — the refusal stays blocking either way).
- `go test ./...` (whole repo) — exit 0, nothing outside this round's scope regressed.

Behavioral changes worth the operator's eye (all deliberate, all documented at the change site):

- Done-checks: an ambiguous answer now BLOCKS `delete-not-done`/`rename-not-done-old` (previously passed); a rejection or unknown status now yields a blocking `glyph-rejected` for every done-check direction (previously passed the Delete direction and misreported the Create direction).
- `containment-unit-overlap` gained the file-self-vs-directory-self pairing (spec row 23 and the design doc updated in the same commit).
- `rename-from-not-glyph` gained the self-glyph-old/handle-new arm (spec row 18 updated in the same commit).
- `directory-target`'s detail now offers the file-self-glyph remedy alongside the package remedy.

No substrate sessions were started at any point; `ps` shows no stray tmux/lyx/claude processes attributable to this round. The one live probe (`lyx quarry resolve`, zero-LLM) ran against a freshly redeployed binary at review time.
