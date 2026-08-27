{"status":"success","commit_sha":"fda2beb63aeae716071fda66ef733c1bfdefcee1","session_id":"601363a9-adb5-44ba-956c-31a22a7f08ab","cards_done":[14,15,16,17]}

Summary: All 4 of 4 cards in this batch (`pull-non-fatal-weft`) are committed and verified.

- `internal/fabricengine/pull.go` — Card 14: `Fabric.Pull`'s weft arm (`weftHasUpstream` probe and `PullWeft` call) is now non-fatal, warning and falling through to the warp side instead of returning immediately; `PullResult.WeftPulled` is set true only on the two genuine success paths; every `*PartialPullError` construction now carries `result.WeftPulled` instead of a hardcoded `true`. Card 15: `PartialPullError`'s doc comments and `Error()` corrected — `WeftPulled` can now legitimately be `false`, and `Error()` branches on it.
- `internal/fabricengine/doc.go`, `internal/fabriccli/weft_verbs.go` — Card 16: package narrative and `pullResultMap`'s doc comment updated to document the non-fatal weft arm and the observable `weft_pulled: false` inside a success envelope.
- `internal/fabricengine/pull_integration_test.go` — Card 17: rewrote the stale zero-result test, added two new tests (diverged weft with warp advancing cleanly; healthy pair both sides clean).
- `internal/fabriccli/merge_cli_integration_test.go` — extended scope (plan-edit commit `e4dd6b34ea...`, pushed first per protocol): fixed three tests that assumed weft-side merge participation/refusals batch 2's `weft-guards-drop` had already removed, but whose narrower `verify:` never caught since it excluded `fabriccli`. Fixed by inverting per Card 12's established playbook rather than deleting, confirmed not pre-existing on `main` via a temporary read-only worktree (removed after use).
- `_mill/plan/03-pull-non-fatal-weft.md` — plan amendment recording the added file and the reason.

Verify: `go build ./cmd/lyx`, `go test ./internal/fabricengine/... ./internal/fabriccli/...`, and `go test -tags integration ./internal/fabricengine/... ./internal/fabriccli/...` all pass. Working tree is clean (pre-report self-check confirmed no uncommitted tracked changes).
