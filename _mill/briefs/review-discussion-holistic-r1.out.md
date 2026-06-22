MILL_REVIEW_BEGIN
# Review: Extract internal/vscode; keep ide IDE-generic

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-06-22
```

## Findings

### [GAP] Verification gate misses integration-tagged menu tests
**Section:** ## Testing → Verification gate / Constraints
**Issue:** `menu_test.go` carries `//go:build integration` (verified, line 1), so `go test ./internal/ide/...` runs neither `TestMenu*` nor `cli_test.go`; the gate lists `-tags integration` only as "optional," leaving the `menu.go`/`cli.go` guardrail unexercised by the mandatory gate.
**Fix:** Make `go test -tags integration ./internal/ide/...` a required gate step (not optional), and note menu_test.go's integration tag alongside cli_test.go's in Constraints.

### [NOTE] Constraints lists only cli_test.go integration tag
**Section:** ## Constraints (Other)
**Issue:** Line 194 says "preserve `//go:build integration` on cli_test.go" but `menu_test.go` also carries the tag (verified); the build-tag preservation requirement is stated for only one of the two files.
**Fix:** Add `menu_test.go` to the integration-tag preservation note.

### [NOTE] mainColor only consumed by tests, not pickColor
**Section:** ## Scope / Decisions (palette/mainColor unexported)
**Issue:** Discussion says `mainColor` is "used internally and by white-box tests," but `pickColor` never references `mainColor` (verified — it uses `palette[1]` and the index loop); the only `mainColor` consumers are `color_test.go:36,83`.
**Fix:** Note that `mainColor` survives the move solely as test-visible state, so it must stay in a non-`_test.go` file the white-box tests can reach (the suggested `color.go` placement satisfies this).

## Verdict

GAPS_FOUND
Mandatory test gate omits the integration-tagged menu/cli guardrails that protect the rewired package.
MILL_REVIEW_END