# Plan: reed: born-as-strand for loom start's operator attach

```yaml
task: "reed: born-as-strand for loom start's operator attach"
slug: "reed-born-as-strand"
approved: false
started: "20260919-094854"
parent: "main"
root: ""
verify: null
discussion_sha: 70d0fc694a0770a165f39a2f927ee77da9a2c8bf
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: watchdog-seam-and-focus-pin
    file: 01-watchdog-seam-and-focus-pin.md
    depends-on: []
    verify: go test ./internal/reedengine/... ./internal/reedcli/... ./internal/burlercli/... && go test -tags integration ./internal/reedengine/...
  - number: 2
    name: operator-strand-and-loom-wiring
    file: 02-operator-strand-and-loom-wiring.md
    depends-on: [1]
    verify: go test ./internal/loomcli/... ./cmd/lyx/... && go test -tags smoke ./internal/loomcli/...
```

## Shared Decisions

_Cross-cutting decisions every batch inherits: naming conventions, error-handling posture, test frameworks, style/lint constraints.
One subsection per decision.
Batch-local decisions live in each batch file._

### Decision: cgo-is-a-build-prerequisite

- **Decision:** every `go build`/`go test` in this plan runs with `CGO_ENABLED=1` and a C compiler on `PATH`.
- **Rationale:** the Quarry CGO Requirement Invariant — `lyx` links tree-sitter's C grammars through cgo, and `internal/cgoguard` fails the build outright under `CGO_ENABLED=0`. `CGO_ENABLED` already defaults to `1` for a native build when a compiler is present, so no `verify:` command sets it explicitly.
- **Applies to:** all batches

### Decision: real-tmux-tests-follow-each-packages-own-tag

- **Decision:** a new test needing a real tmux server goes under the tag its own package already uses for that tier — `integration` in `internal/reedengine`, `smoke` in `internal/loomcli`.
- **Rationale:** `_mill/discussion.md`'s Testing section says "smoke-tagged" for the empty-`Cmd` `send-keys` sanity, but `internal/reedengine` has no `smoke`-tagged file at all: its real-tmux tier is `//go:build integration` (`contract_integration_test.go`, `ensuresession_integration_test.go`, `watchdog_integration_test.go`). Introducing a lone `smoke` tag in that package would create a tier nothing else in it runs, and CI would never reach it. `internal/loomcli`'s real-substrate tier IS `smoke`, so the loom-side live assertions stay `smoke`-tagged exactly as the discussion describes.
- **Applies to:** all batches

### Decision: no-new-untagged-spawn

- **Decision:** no untagged (Tier 1) test file added by this plan calls `exec.Command`, `gitexec`, `hubforge.NewHub`, or drives a live tmux session. Every live-substrate assertion goes in an `integration`- or `smoke`-tagged file.
- **Rationale:** the Test Tier Purity Invariant. This plan adds Tier 1 tests in `internal/reedengine`, `internal/reedcli`, `internal/loomcli`, `internal/burlercli` and `internal/reedengine/render`, all of which must stay offline and fast.
- **Applies to:** all batches

### Decision: docs-land-with-the-behaviour-card-that-makes-them-true

- **Decision:** each user-visible doc line is edited by the same card that makes its new wording true, not by a trailing docs card. `internal/loomcli/start.go`'s `Long`, `manifest/designs/loom.md`'s `lyx loom start` step list and `docs/overview.md`'s four-step sentence are each touched twice across batch 2 — once by the operator-strand card, once by the watchdog card. `manifest/designs/reed-born-as-strand.md` and `manifest/roadmap.md` are edited once, by the final card, since both describe the item as a whole.
- **Rationale:** the project `CLAUDE.md`'s "docs land in the same commit" rule, reconciled with mill's one-commit-per-card execution model — a single trailing docs card would leave the step list stale for the span of two intermediate commits. `docs/overview.md`'s line 330 is in scope for the same reason `start.go`'s `Long` is: it enumerates the same four steps verbatim, so both this change's additions make it inaccurate. It was not named in `_mill/discussion.md`'s Scope inventory; it is the identical defect the discussion's round-4 review caught for `Long`, found in a second file.
- **Applies to:** operator-strand-and-loom-wiring

### Decision: verify-commands-are-package-scoped-not-repo-wide

- **Decision:** each batch's `verify:` names only the packages that batch touches, plus the tagged tier whose files it edits. Repo-wide regression coverage is the hub's configured `pipeline.done_gate` (`go test ./... && go test -tags integration ./...`), which runs once at Handoff.
- **Rationale:** `verify:` re-runs after every implementer and fixer round; a repo-wide `go test ./...` on a cgo binary is minutes per round. The tagged halves are chained with `&&` rather than comma-joined into one `-tags` flag, per the plan-validator's own guidance on tagged suites.
- **Applies to:** all batches

### Decision: watchdog-seam-signature

- **Decision:** the extracted function is `func SpawnWatchdog(hubPath, tmuxPath string, suppress bool)` in a new `internal/reedengine/spawnwatchdog.go`. It returns nothing, exactly as `ensureWatchdogSpawned` does today: every failure path logs and returns, and the spawn is best-effort by construction.
- **Rationale:** `_mill/discussion.md`'s `watchdog-seam-lives-in-reedengine` decision pins the package, the exported package-level form, and the three explicitly-passed inputs. A non-`error` return keeps both call sites one line and keeps "best-effort" a property of the seam rather than a discipline each caller must remember.
- **Applies to:** all batches

## All Files Touched

_Full union of every `Creates:` / `Edits:` / `Moves:` **target** path across every batch, sorted alphabetically (Move **source** paths are excluded — they disappear, like `Deletes:` tokens)._

- `docs/overview.md`
- `internal/burlercli/wiring_test.go`
- `internal/loomcli/bootstrap.go`
- `internal/loomcli/bootstrap_test.go`
- `internal/loomcli/cli.go`
- `internal/loomcli/cli_test.go`
- `internal/loomcli/smoke_operatorstrand_test.go`
- `internal/loomcli/smoke_test.go`
- `internal/loomcli/start.go`
- `internal/loomcli/start_watchdog_test.go`
- `internal/reedcli/spawnwatchdog.go`
- `internal/reedcli/spawnwatchdog_test.go`
- `internal/reedengine/emptycmd_integration_test.go`
- `internal/reedengine/render/focus_test.go`
- `internal/reedengine/spawnwatchdog.go`
- `internal/reedengine/spawnwatchdog_test.go`
- `manifest/designs/loom.md`
- `manifest/designs/reed-born-as-strand.md`
- `manifest/roadmap.md`
