# Plan: Audit and overhaul engine test suites

```yaml
task: Audit and overhaul engine test suites
slug: test-suite-overhaul
approved: true
started: 20260801-100527
parent: main
root: ""
verify: null
```

## Batch Index

```yaml
batches:
  - number: 1
    name: githubclient-timeout-seam
    file: 01-githubclient-timeout-seam.md
    depends-on: []
    verify: go test ./internal/githubclient/...
  - number: 2
    name: webstercli-await-wait-window
    file: 02-webstercli-await-wait-window.md
    depends-on: []
    verify: go test -tags integration ./internal/webstercli/...
  - number: 3
    name: benchmark-doc-update
    file: 03-benchmark-doc-update.md
    depends-on: [1, 2]
    verify: go test ./... && go test -tags integration ./...
```

## Shared Decisions

### Decision: no product-code behavior changes

- **Decision:** every card in this plan touches only test files (`*_test.go`) and one package-level timeout declaration (`const` → `var`, same value); no CLI flag, envelope shape, or command registration changes anywhere.
- **Rationale:** the task is a pure wall-clock reduction — same assertions, same code paths, faster tests. This keeps the CLI/Cobra Invariant, GitHub Auth Invariant, and Test Tier Purity Invariant all trivially satisfied (see each batch's `## Batch Scope` for the specific invariant check).
- **Applies to:** all batches.

### Decision: no test coverage is removed

- **Decision:** every existing test name/scenario in the touched files remains exercised after this plan lands, either unchanged or folded into a table-driven row that preserves the original assertions.
- **Rationale:** per `_mill/discussion.md`'s Scope → Out, bulk test deletion is explicitly forbidden and live-substrate coverage must not shrink. The one consolidation in this plan (batch 2, card 3) is a pure refactor: two existing test functions become two table rows of one new test function, same assertions.
- **Applies to:** batch webstercli-await-wait-window.

## All Files Touched

- `docs/benchmarks/test-suite-timing.md`
- `internal/githubclient/githubclient_test.go`
- `internal/githubclient/token.go`
- `internal/webstercli/verbs_test.go`
