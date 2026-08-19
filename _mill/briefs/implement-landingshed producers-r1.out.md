11 commits match all 11 declared cards (20 through 30) exactly, one commit per card. All cards completed and verified.

{"status":"success","commit_sha":"53c6e1358bc63e1ad9b5324c059e21d38f6c92a5","session_id":"65b22a31-1543-438d-9e2c-ed7a61e1b395","cards_done":[20,21,22,23,24,25,26,27,28,29,30]}

All 11 of 11 cards in this batch are committed (verified by matching each commit subject against the batch file's `## Cards` `Commit:` messages, range `ac1bf0760ae1ca8ffb4cc83eb8d74bfdf9567392..HEAD`). The batch's `verify:` command passed in full: `go test ./internal/landingshed/... ./internal/configreg/... ./internal/fabricengine/... ./cmd/lyx/...` all green, plus `go vet ./...` clean and `gofmt -l` clean on touched packages. Working tree is clean (no uncommitted tracked changes).

Key files touched:
- `/home/knatte/Code/loomyard/wts/landing-publish-finalize-producers/internal/landingshed/doc.go`, `deps.go`, `config.go`, `configtemplate.go`, `template.yaml`, `ctx.go`, `stuck.go`, `publish.go`, `finalize.go`, `seam_enforcement_test.go`, `publish_test.go`, `finalize_test.go`, `config_test.go`
- `/home/knatte/Code/loomyard/wts/landing-publish-finalize-producers/internal/fabricengine/mergeerrors.go` (added `MergeGuardError.WorktreeDirty()`), `mergeerrors_test.go`
- `/home/knatte/Code/loomyard/wts/landing-publish-finalize-producers/internal/configreg/configreg.go`, `configreg_test.go`
- `/home/knatte/Code/loomyard/wts/landing-publish-finalize-producers/cmd/lyx/configstrictness_test.go`

One notable design addition beyond the literal card text: since `Deps.OpenParentFabric` returns a concrete `*fabricengine.Fabric` (per card 20's spec) but the batch's own test-tier rules require hermetic, no-spawn tests, I introduced an unexported `parentMerger` single-method seam in `finalize.go` (mirroring the existing `resolver` seam) plus a `parentOpener` closure that adapts the concrete return type onto it -- this is what let `finalize_test.go` script merge outcomes (happy path, merge-in-required retry, dirty-worktree guard, unrecognized error) without touching real git.

{"status":"success","commit_sha":"53c6e1358bc63e1ad9b5324c059e21d38f6c92a5","session_id":"65b22a31-1543-438d-9e2c-ed7a61e1b395","cards_done":[20,21,22,23,24,25,26,27,28,29,30]}
