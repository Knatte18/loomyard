# fabric merge surface — independent review (round 6, tag `opus-high-r6`)

Reviewer: Opus, high effort. Clean-room: findings below were formed before reading any prior-round `_mill/fabric-merge-review-*` material.
Worktree: `/home/knatte/Code/loomyard/wts/fabric-merge-crucible-round4`, branch `fabric-merge-crucible-round4`.

## What was tested

Appended as each command/scenario returned.

### Baseline gates (before any edit)

- `go build ./...` — clean.
- `go vet ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitrepo/...` — clean.
- `go test -count=5 ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitrepo/... ./cmd/lyx/...` — all ok (fabricengine 0.355s, fabriccli 0.004s, gitrepo 0.005s, cmd/lyx 0.963s).
- `go test -tags integration -count=1 -timeout 30m ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitrepo/...` — all ok (fabricengine 31.4s, fabriccli 2.6s, gitrepo 1.6s).

## Findings

(in progress)
