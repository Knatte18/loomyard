Both card commit messages match the batch's `## Cards` `Commit:` values. All 4 cards (23, 24, 25, 26) are covered by these 2 combined commits. 4 of 4 cards committed.

{"status":"success","commit_sha":"3f092afae4912e5e60926b808830f01f275bc251","session_id":"b7c1ddcc-e85a-4000-b1cf-6269185ca5bb","cards_done":[23,24,25,26]}

Summary: all 4 cards of batch 06-logger-spans committed and verified.

- `/home/knatte/Code/loomyard/wts/trace-logging/internal/logger/span.go` (new) — `Span` type with `StartSpan`/`Child`/`End`, plus span-scoped `Debug`/`Info`/`Warn` methods stamping `span=<dotted path>` alongside `trace=`. Open/close records emit at Debug; `End(err)` with a non-nil error emits at Warn. Commit `90618dbd` (cards 23+24, combined since Card 23's skeleton and Card 24's emit methods are inseparable in one compilable file).
- `/home/knatte/Code/loomyard/wts/trace-logging/internal/logger/span_test.go` (new) — nesting/dotted-path, sibling independence (ended and unended), `End(err)` error text/level, and durable-sink visibility tests (open/close absent, `End(err)` present, `span=` riding on ordinary Info/Warn lines). Commit `3f092afa` (cards 25+26, combined for the same reason).

Verify command `go test ./internal/logger/...` passes (all prior-batch tests plus the new span tests). `gofmt`/`goimports -w` and `golangci-lint run` were clean on the changed files. No tracked working-tree changes remain outstanding; the only untracked item is the batch's own brief file at `_mill/briefs/implement-logger-spans-r1.md`, which is out of this batch's scope.
