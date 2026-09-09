# loom review — round 2 safety pass (fable5-high-r2)

Clean-room review of the two refactors (ref-shape registry centralization in `internal/planparser/shape.go`; quarry v0.2.0 `Status.Known()`/`ResolveResult.Rejected()` adoption in `internal/planglyph`), driven live through the real built `cmd/lyx` binary.
Round context: round 1 (`opus5-high-r1`) closed F1–F5 and drove eleven live scenarios; this round is a genuinely independent pass to find anything both it and the orchestrator's verification missed.

## Executive summary

(written last)

## Scope assessment

(written last)

## Code findings

(provisional entries appended as formed)

### F-R2-1 (provisional) — `.Status` tripwire does not match the `Unit` selector, the third Status-typed reading surface

- File: `internal/planglyph/status_enforcement_test.go:58` (`statusVocabularySelectors = {Status, Known, Rejected}`).
- Scenario: `quarry.ResolveResult.Unit` is a `Status`-typed field drawing from the same vocabulary (quarry's contract: set only on `not_found`, carrying `found`/`not_found`). A NEW planglyph consumer branching on `r.Unit` alone — e.g. `if r.Unit == quarry.StatusFound { treat member as merely missing } else { treat unit as gone }` — names no `Status`, `Known`, or `Rejected` selector anywhere and therefore ships invisible to the tripwire, exactly the blind-spot class round 1's F2 closed for `Rejected()`. The two existing `Unit` readers (`create.go:174` inside `createFindings`, `resolve.go:99` inside `statusFindings`) are both already inside allowlisted functions, so adding `"Unit"` to the selector set costs zero allowlist churn and closes the last unmatched spelling of the vocabulary.
- Severity: NIT (no current fail-open consumer exists; this is enforcement-machinery completeness, not a behavior defect).
- Suggested fix: add `"Unit": true` to `statusVocabularySelectors`, extend the file's doc comment, and add a seeded self-test for the Unit spelling mirroring `TestStatusHitsIn_CatchesRejectedConsumer`.
- CONFIRMED (traced: grep over planglyph production files shows `.Unit` read in exactly the two allowlisted functions; the tripwire's matcher provably cannot see a bare-Unit consumer since `statusHitsIn` matches only the three named selectors).

## Docs & operability findings

(provisional entries appended as formed)

## What was tested

Observations appended immediately after each command/scenario returns.

### Spec/contract reading notes

- quarry v0.2.0 `engine.Status.Known()` (module cache `internal/engine/answer.go:65`): true exactly for the four documented values `found`/`not_found`/`ambiguous`/`multipart`; false for `""` and any other string.
- `ResolveResult.Rejected()` (`answer.go:262`): `r.Status == ""` — the documented pre-resolution-rejection marker (Status and Error never both set).
- These match the call sites' assumptions: `donecheck.go:163` (`!r.Status.Known()` fail-closed before reading the two booleans), `resolve.go:139` (`r.Rejected()` selecting the rejection detail).
- Static call-site sweep (`grep` over `internal/planglyph` production files): every `Status` read is one of `donecheck.go` (Known-gated), `resolve.go`/`create.go`/`containment.go` (exhaustive switch with fail-closed default), `handle.go:94` (`!= StatusFound`, deliberately Found-only per its doc). The one `Rejected()` call is `unreadableStatusDetail`, reached only from the fail-closed arms. No hand-rolled `Status == ""` or `Error != ""` test survives in production code.
- Environment check: gcc, cc, tmux, go1.26.0 all present; no environment gap.

### Hermetic gates (pre-fix baseline)

- `go build ./...` — exit 0.
- `go vet ./internal/loomengine/... ./internal/loomcli/... ./internal/loomshed/... ./internal/planparser/... ./internal/planglyph/... ./internal/websterengine/... ./internal/webstercli/...` — exit 0.
- `go test -count=5` over the eight in-scope packages (`loomengine, loomcli, loomshed, planparser, planglyph, websterengine, webstercli, cmd/lyx`) — all `ok`, exit 0.

### Registry/meta-test reading notes

- `shape_test.go` covers both syncs (enum↔allRefKinds, refGate consts↔ledger keys, values not names), ledger completeness, and the fail-closed panic (both missing-kind and missing-gate).
- `shape_enforcement_test.go` covers both AST scans with seeded self-tests; exempt sets match CONSTRAINTS.md's Ref-Shape Registry Invariant exactly (`classify.go`+`shape.go` for refKind; those plus `planparser/handle.go` for plan:-ops; no planglyph exemption).
