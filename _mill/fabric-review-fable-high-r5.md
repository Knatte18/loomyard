# fabric — independent review, round 5 (`fable-high-r5`)

Reviewer-fixer round agent, Fable/high.
Clean-room review per `_mill/fabric-review-prompt.md`: no prior `fabric-review-*` material read before this findings list was complete.
Primary target: the create-side containment gap in `createExclusiveDir`/`createGitWorktree` (seeded, orchestrator-confirmed residual).

## Executive summary

(to be completed at end of Job 1)

## Scope assessment (plan vs shipped)

(to be completed)

## Code findings (severity-ranked)

(provisional findings appended as formed)

## Docs & operability findings

(provisional)

## What was tested

Exact commands and observations, appended incrementally as each scenario returns.

### Hermetic gates (pre-review baseline, clean tree at 08520a1b + report skeleton)

- `go build ./...` — rc=0.
- `go vet ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitexec/... ./internal/gitrepo/...` — rc=0, no output.
- `go test ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitexec/... ./internal/gitrepo/... ./cmd/lyx/... -count=5` — all ok.
