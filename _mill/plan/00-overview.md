# Plan: reed: per-hub daemon reaps orphaned sessions

```yaml
task: 'reed: per-hub daemon reaps orphaned sessions'
slug: 'reed-per-hub-daemon-reap'
approved: false
started: '20260919-094639'
parent: 'main'
root: ""
verify: null
discussion_sha: 'ad21d9784fb2bdee9f4deb391598dc340f5882b8'
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: reedengine reap seam
    file: 01-reedengine-reap-seam.md
    depends-on: []
    verify: go test ./internal/reedengine/
  - number: 2
    name: reedcli pure decision seams
    file: 02-reedcli-decision-seams.md
    depends-on: []
    verify: go test ./internal/reedcli/
  - number: 3
    name: daemon wiring
    file: 03-daemon-wiring.md
    depends-on: [1, 2]
    verify: go test ./internal/reedcli/ ./internal/reedengine/ && go vet -tags integration ./internal/reedcli/
  - number: 4
    name: tagged reap tests
    file: 04-tagged-reap-tests.md
    depends-on: [3]
    verify: go test -tags integration -race -run 'TestWatchdogReap|TestWatchdogIntegration' ./internal/reedcli/
  - number: 5
    name: docs
    file: 05-docs.md
    depends-on: [3]
    verify: null
```

## Shared Decisions

### Decision: reap-logging-names-the-socket-not-the-hub

- **Decision:** `ReapSession`'s own two log lines name `socket` (its `socketKey` parameter), `session` and the captured pid count — never `hub`, because its named signature `ReapSession(tmuxPath, shellPath, socketKey, sessionName string) error` carries no hub parameter. The hub is named on the `internal/reedcli` side instead: the reap goroutine's `Warn` on a failed reap, and the `Warn` the daemon logs when it starts with an empty shell, both carry `hub`.
- **Rationale:** the discussion's Constraints section asks for "one line before the kill naming hub, session and the pid count", while its `ReapSession-is-a-second-engine-less-exported-function` decision fixes a signature with no hub in it. Adding a hub parameter purely to log it would make `reedengine` carry a hub concept it has no other use for, and `socketKey` is `ServerName(hub)` — hub-identifying on its own. Splitting the two halves across the layer boundary satisfies Live-Substrate Spawn Observability with no signature change, and the operator reading one hub's durable log has the hub from the log's own location anyway.
- **Applies to:** all batches

### Decision: go-native-verify-commands

- **Decision:** every non-null `verify:` in this plan is a bare Go test/vet command with no `PYTHONPATH=` prefix, scoped to the one or two packages the batch touches rather than `./...`.
- **Rationale:** the `PYTHONPATH=` isolation rule applies to Python projects; this is a Go repo and `go test` inherits nothing from the mill cache scripts dir. Package-scoped is the natural granularity for Go — a `-run` pattern narrower than the package would hide a regression the batch caused in a sibling test in the same package, which is exactly what a batch verify is for. The repo-wide sweep is `pipeline.done_gate`'s job, already configured.
- **Applies to:** all batches

### Decision: every-new-reedcli-seam-lives-in-watchdog.go

- **Decision:** `watchdogOrphanGoneCycles`, `watchdogTiming`, `watchdogDefaultTiming`, `validateWatchdogFlags`, `worktreeRootGone`, `hubIsLiveDir` and `planReapCycle` are all declared in `internal/reedcli/watchdog.go`, beside the existing `watchdogHubDiscoveryCycle`/`watchdogHubIdleCycles`/`sessionsAreIdle`/`planSessionDiff` seams. No new non-test file is created in `internal/reedcli`.
- **Rationale:** the discussion names `internal/reedcli/watchdog.go` explicitly as the declaration site for the new constant, and every one of these seams is a property of the one loop that file already owns. Splitting them across a second file would separate the constants from the loop that consumes them — the exact placement complaint the existing `watchdogHubDiscoveryCycle` doc comment already answers for itself.
- **Applies to:** 2, 3

### Decision: two-distinct-stat-predicates-not-one-negated

- **Decision:** the per-name gone check and the hub probe are two separate functions — `worktreeRootGone(path string) bool` and `hubIsLiveDir(hub string) bool` — and neither is expressed as the negation of the other.
- **Rationale:** they are genuinely different predicates, not complements. `worktreeRootGone` is true only for proven-gone (`fs.ErrNotExist`, or a successful stat of a non-directory); `hubIsLiveDir` is true only for proven-live (stat succeeds and reports a directory). A stat failing with EACCES or EIO makes both false, and that is correct on both sides: the name makes no progress toward a reap, and the hub does not clear the reap pass to run. Writing one as `!theOther` would silently turn each transient fault into the opposite verdict on one of the two paths.
- **Applies to:** 2, 3

### Decision: done-gate-unchanged

- **Decision:** `pipeline.done_gate` stays at its currently configured `go test ./... && go test -tags integration ./...`. No batch edits `mill-config.yaml`.
- **Rationale:** it is already a repo-wide test gate covering both tiers this task writes into, so the batch-verify scopes being package-narrow is already backstopped. Adding `golangci-lint run` was considered and skipped: the existing gate already runs both tiers, and widening it is not this task's change to make.
- **Applies to:** all batches

## All Files Touched

- `internal/reedcli/spawnwatchdog.go`
- `internal/reedcli/watchdog.go`
- `internal/reedcli/watchdog_integration_test.go`
- `internal/reedcli/watchdog_test.go`
- `internal/reedcli/watchdogreap_integration_test.go`
- `internal/reedengine/lock.go`
- `internal/reedengine/overlay.go`
- `internal/reedengine/overlay_test.go`
- `manifest/designs/reed-header-selvage.md`
- `manifest/roadmap.md`
