Both card commits (card 7's `feat(...)` and card 8's `test(...)`) match exactly the batch's declared `Commit:` messages. 2 of 2 cards committed — all complete.

Card-count self-check: 2 of 2 cards committed (card 7 and card 8), matching the batch's declared total of 2 cards. Both `verify:` sub-invocations (`go test ./internal/landingshed/...` and `go test -tags integration ./internal/landingshed/...`) pass. No uncommitted tracked changes remain.

{"status":"success","commit_sha":"fe48118b28410ea99bc263d278b696efbd0a23e4","session_id":"1a82793e-b4bc-4351-a8b4-0cc019799e53","cards_done":[7,8]}

Relevant files:
- `internal/landingshed/finalize.go`
- `internal/landingshed/publish.go`
- `internal/landingshed/finalize_test.go`
- `internal/landingshed/finalize_integration_test.go`
- `internal/landingshed/publish_test.go`

{"status":"success","commit_sha":"fe48118b28410ea99bc263d278b696efbd0a23e4","session_id":"1a82793e-b4bc-4351-a8b4-0cc019799e53","cards_done":[7,8]}
