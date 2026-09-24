# batten — review round opus-medium-r4

Round 4 of the batten follow-up crucible campaign.
Worktree `crucible-batten-followup`, seeded at `b1e5ab9af`.
Clean-room: no `_mill/**/batten-review-*` file other than the prompt was opened before this report's findings were complete.

## Executive summary

(pending)

## Scope assessment

(pending)

## Code findings

(provisional, severity ordering pending)

## Focus-1 enumeration (SIGKILL windows in Topology.Add / Topology.Remove)

(pending)

## Focus-2 enumeration (youngest fixes)

(pending)

## Docs & operability findings

(pending)

## What was tested

### Hermetic baseline (start of Job 1)

- `go build ./... && go vet ./... && go test -count=1 ./...` at `b1e5ab9af`: exit 0, every package `ok`.
- Third self-report path sweep: `ls -d internal/*selfreport* internal/*friction*` plus a repo-wide grep for `CreateIssue`/issue-filing call sites.
  Only Tier 1 (`internal/loomcli/selfreport.go`, wired at `internal/loomcli/arm.go:326`) and Tier 2 (`internal/frictionengine` via `lyx selfreport create` in `internal/selfreportcli`).
  No third path.
- Environment: `which claude tmux gh` all resolve.
  `--child-driver llm`: the `ly@loomyard` plugin is absent on this host, an operator-accepted environment gap (focus 4); not driven this round.
- Windows path behaviour: not touched, unreachable from this Linux host.

## Teardown

(pending)
