Both commit subjects match the two cards' `Commit:` messages exactly. 2 of 2 cards committed — all cards complete, verify passes, working tree clean.

{"status":"success","commit_sha":"20c472c0","session_id":"1797c1af-2f7b-4a1e-abb7-8d08fdeef1c4","cards_done":[1,2]}

Summary: 2 of 2 cards committed. Card 1 (`f1c8da86`) added `FillOptional` to `internal/stencil/stencil.go` (exempting listed names from both the top-level unfilled-marker guard and `missingkey=error`, with whitespace-only normalisation and no caller-map mutation), redefined `Fill` as `FillOptional(t, v, nil)`, and extended `internal/stencil/stencil_test.go` with the full set of required cases. Card 2 (`20c472c0`) updated `docs/shared-libs/stencil.md` to document `FillOptional`'s signature, the `Fill`-as-`FillOptional(t,v,nil)` definition, and the optional-marker exemption, stating explicitly that optionality is a property of the caller's argument list rather than template text. `go test ./internal/stencil/...` passes. Working tree is clean; both commits pushed to `pattern-wiring`.

{"status":"success","commit_sha":"20c472c0","session_id":"1797c1af-2f7b-4a1e-abb7-8d08fdeef1c4","cards_done":[1,2]}
