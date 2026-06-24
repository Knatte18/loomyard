All three holistic review findings addressed.

1. [BLOCKING] cloneHub returns non-empty hubPath on error — internal/gitclone/clone.go: changed three error returns to return "" instead of hubPath.
2. [NIT] TeardownFailure test now asserts residual hub path appears in error message — internal/gitclone/clone_integration_test.go.
3. [NIT] overview.md module-tree diagram now includes internal/gitclone/.

Verified with: go test -tags=integration ./internal/gitclone/ ./cmd/lyx/ ./internal/paths/ — all pass.

{"status":"success","commit_sha":"a311c9026227189b6e5e4f4e93b16ea8a3941dca","session_id":"5be9a8a8-4b57-4f16-952e-3066953b5c7b"}
