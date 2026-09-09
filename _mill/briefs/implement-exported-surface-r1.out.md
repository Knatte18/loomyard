{"status":"success","commit_sha":"b41ddde5eb09b67408860b34dd2822d448a8d123","session_id":"5b89aca2-482b-4c05-9acc-78e976104169","cards_done":[3,4]}

Both of this batch's 2 cards are committed (2 of 2): Card 3 (exported handle vocabulary in `internal/planparser/handle.go` / `handle_test.go`) and Card 4 (`Plan.GlyphLanguage` in `internal/planparser/glyphref.go`/`glyphref_test.go` and `Card.ID` in `internal/planparser/plan.go`, with `validate.go`'s `cardID` doc comment updated and a new test in `validate_test.go`). Verify (`go test ./internal/planparser/ ./internal/planglyph/`) passes. Working tree is clean, both commits pushed.

One judgment call worth flagging: Card 4's requirement text for `internal/planparser/validate_test.go` asked for a test asserting `c.ID() == cardID(c)`, but that file is package `planparser_test` (external), so it has no access to the unexported `cardID`. I implemented the equivalent check against the literal `"N-<slug>"` format string instead, with a comment explaining why, preserving the intended coverage without breaking package visibility.

{"status":"success","commit_sha":"b41ddde5eb09b67408860b34dd2822d448a8d123","session_id":"5b89aca2-482b-4c05-9acc-78e976104169","cards_done":[3,4]}
