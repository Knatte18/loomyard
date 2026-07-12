All 5 cards of batch `parser-v2` are complete: 5 of 5 cards committed (across 3 commits — cards 1–3 combined into one commit since they all touched `plan.go`, per the brief's allowance for same-file combined commits; cards 4 and 5 each their own commit). The batch `verify:` (`go test ./internal/builderengine/...`) passes in full, including every sibling test file (chain, digest, poll, template, state, etc.), confirming the model change did not ripple.

Key files touched:
- `C:\Code\loomyard\wts\plan-format-file-ops\internal\builderengine\plan.go` — `PlanCard`/`MovePair` typed model, card sub-parser, `root:`/`//` normalization, `(C cards)` Batch Index segment
- `C:\Code\loomyard\wts\plan-format-file-ops\internal\builderengine\validate.go` — `recognizedFormat = 2`, `checkBatchOversized` adapted to typed card paths and `len(Cards)`
- `C:\Code\loomyard\wts\plan-format-file-ops\internal\builderengine\doc.go`, `fingerprint.go` — v1→v2 godoc wording
- `C:\Code\loomyard\wts\plan-format-file-ops\internal\builderengine\testdata\plan-valid\*.md`, `plan-unapproved\*.md`, `plan-broken-chain\*.md` — rewritten to v2 (root/`//` demo, Moves pair + Rename mechanic, Commit field, `(C cards)` segments)
- `C:\Code\loomyard\wts\plan-format-file-ops\internal\builderengine\plan_test.go`, `validate_test.go` — full v2 coverage

Commits: `ff694b0` (01.3, cards 1–3 combined), `930369b` (01.4), `127948f` (01.5). All pushed to `plan-format-file-ops`.

Note on tooling: `golangci-lint` is not installed in this environment (only `goimports`/`go vet`/`go build`/`go test` were available), so lint relied on those rather than golangci-lint; no lint configuration exists in the repo to check against.

{"status":"success","commit_sha":"127948f","session_id":"6c35f97d-e002-40a6-81a9-2836b01c3699"}
