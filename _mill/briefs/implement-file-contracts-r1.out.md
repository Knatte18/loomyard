3 of 3 cards committed — matches the batch's declared card count exactly. All three commit subjects match the batch file's Cards' Commit: messages, and verify (`go test ./internal/shedadapters/...`) passes.

{"status":"success","commit_sha":"71664f4b1dcc59f5dc0d64108e9699d7cd28179f","session_id":"b362f527-f740-47a0-851d-7fdc2b302992","cards_done":[1,2,3]}

Summary: 3 of 3 cards committed and pushed.

- `/home/knatte/Code/loomyard/wts/shedadapters-generic-bouncer-producer/internal/shedadapters/bouncerfiles.go` — the three fail-loud parsers (`parseVerdict`, `parseLedger`, `parseFocus`), shared `splitFrontmatter`/`frontmatterProse` helpers, and the focus-file writer (`renderFocus`, `writeFocus`).
- `/home/knatte/Code/loomyard/wts/shedadapters-generic-bouncer-producer/internal/shedadapters/bouncerfiles_test.go` — table-driven parser tests plus focus round-trip and `writeFocus` tests.
- `/home/knatte/Code/loomyard/wts/shedadapters-generic-bouncer-producer/internal/shedadapters/round.go` — `ResolveRound` and the `verdictPath`/`ledgerPath`/`focusPath` helpers.
- `/home/knatte/Code/loomyard/wts/shedadapters-generic-bouncer-producer/internal/shedadapters/round_test.go` — scan/error/path-spelling tests for `round.go`.
- `/home/knatte/Code/loomyard/wts/shedadapters-generic-bouncer-producer/go.mod` — promoted `github.com/google/go-cmp` from indirect to direct (used in the round-trip test comparisons).

{"status":"success","commit_sha":"71664f4b1dcc59f5dc0d64108e9699d7cd28179f","session_id":"b362f527-f740-47a0-851d-7fdc2b302992","cards_done":[1,2,3]}
