All 4 cards (4, 5, 6, 7) committed, matching the batch's declared 4 cards exactly — 4 of 4 cards committed, all complete.

{"status":"success","commit_sha":"de401a974c9027edd9ed54b4e5114a86a2e83002","session_id":"0b663071-2d65-4123-973a-66e346aae48c","cards_done":[4,5,6,7]}

Summary: Implemented all 4 cards of batch 02-group-scoped-validation in `/home/knatte/Code/loomyard/wts/plan-custom-card-skips-path-check/internal/planparser/validate.go` and `validate_test.go`:

- Card 4 (`a254423a`): rewrote `checkPathMissing` to loop over `c.TargetGroups`, added `cardOfType` test helper, extended `validCard`, swept existing test fixtures.
- Card 5 (`93da9782`): scoped `createTargetsUnion` and `checkProsaSymbolTarget` to individual `TargetGroup`s.
- Card 6 (`9c3abc0f`): scoped `checkCardFieldEmpty`, `checkCardMissingField`'s ImpactSummary rule, and `checkRenameMechanicMissing` to target groups.
- Card 7 (`de401a97`): relaxed `checkCardTypeMissing` to allow multiple labels, added `checkCustomNotAlone` (new check `card-custom-not-alone`, dispatch row 5), updated doc-comment check counts to seventeen/sixteen.

Verify (`go build ./...` and `go test ./internal/planparser/...`) passes cleanly. Working tree is clean; all commits pushed to `plan-custom-card-skips-path-check`.

{"status":"success","commit_sha":"de401a974c9027edd9ed54b4e5114a86a2e83002","session_id":"0b663071-2d65-4123-973a-66e346aae48c","cards_done":[4,5,6,7]}
