# Plan: reed: attach doesn't reconcile session geometry with the terminal

```yaml
task: 'reed: attach doesn''t reconcile session geometry with the terminal'
slug: 'reed-attach-geometry-reconcile'
approved: true
started: '20260826-121937'
parent: 'main'
root: ""
verify: null
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: engine-live-geometry
    file: 01-engine-live-geometry.md
    depends-on: []
    verify: go test ./internal/reedengine/...
  - number: 2
    name: engine-attach-argv
    file: 02-engine-attach-argv.md
    depends-on: [1]
    verify: go test ./internal/reedengine/...
  - number: 3
    name: cli-convergence
    file: 03-cli-convergence.md
    depends-on: [2]
    verify: go build ./... && go test ./internal/reedcli/... ./internal/loomcli/... ./cmd/lyx/...
  - number: 4
    name: docs-and-live-proof
    file: 04-docs-and-live-proof.md
    depends-on: [3]
    verify: go test ./internal/reedengine/... && go test -tags integration -run TestAttachGeometry ./internal/reedengine/...
```

## Shared Decisions

### Decision: told-box-wins-live-query-is-the-fallback

- **Decision:** `planLayout` takes an explicit `render.Box` parameter and issues no tmux query of its own.
  `applyLayoutLocked` passes `e.liveBoxLocked()` (the `display-message` live-window query, falling back to `cfg.Width`/`cfg.Height`);
  `AttachArgv` passes the box it computes from the attaching client's own told terminal size and never calls `liveBoxLocked`.
- **Rationale:** at attach-argv-build time the live window is still the pre-attach size, so routing the attach path through the live query would reintroduce the exact rescale this task removes.
  Separating "what box" from "how to plan" is what keeps the two callers from converging on the wrong source.
- **Applies to:** all batches

### Decision: geometry-tmux-failures-are-non-fatal-everywhere

- **Decision:** every new tmux interaction this task adds — the two option pins (`status off`, `window-size latest`), both `display-message` readbacks, and the live-window-size query — is non-fatal.
  A failure is logged via `logger.Warn` and the caller continues: boot completes, `applyLayoutLocked` falls back to the configured box, and `AttachArgv` degrades to the bare `attach-session` argv.
  This deliberately breaks with the adjacent `remain-on-exit`/`mouse` pins in `lifecycle.go`, which return an error and fail the boot — do not copy the neighbouring pattern.
- **Rationale:** `remain-on-exit` and `mouse` are correctness dependencies; the two new pins are geometry-**quality** options whose absence degrades to today's behaviour — a working session.
  Failing the boot over them would trade a cosmetic degradation for a total outage, and would do it first on Windows, where psmux's support for `status` and `window-size` is unverified anywhere in this repo.
- **Applies to:** all batches

### Decision: readback-not-exit-status-gates-the-chain

- **Decision:** the attach chain is gated on the `display-message` readbacks, never on `set-option`'s exit status.
  `#{window-size}` other than `latest`, an errored readback, or an unrecognised `#{status}` value suppresses the chain (bare argv, warning logged);
  a `#{status}` other than `off` does not suppress it — the reserved-row count is taken from that value instead (`off`→0, `on`→1, numeric N→N).
- **Rationale:** probed live — a session-scoped `status on` survives `set-option -g status off` with exit 0, and a window-scoped `window-size manual` survives the global `latest` the same way.
  Trusting the exit status would leave the "suppress the chain" safeguard permanently unfired while the told box was off by one row.
  The pins are therefore session/window-targeted (`-t '=<session>:'`, and `-w` for `window-size`) AND confirmed by readback.
- **Applies to:** batches 1, 2

### Decision: no-new-required-subcommand-and-no-probe-change

- **Decision:** `requiredSubcommands` in `internal/reedengine/probe.go` must not grow.
  Every tmux subcommand this task spends — `display-message`, `select-layout`, `set-option`, `list-panes` — is already listed;
  `attach-session` is built into an argv this process never executes itself and was never probed.
- **Rationale:** growing the list adds a psmux-compatibility risk with no benefit, and would turn a missing option into a hard capability refusal at server-ensure.
- **Applies to:** all batches

### Decision: go-conventions-not-python

- **Decision:** this is a Go repo: every batch `verify:` is a native `go test` / `go build` invocation with no `PYTHONPATH=` prefix, and every new test file follows the repo's existing table-test and doc-comment conventions.
  Tier-1 (untagged) test files must not call `exec.Command`/`gitexec.Run` or sleep ≥1s on a constant duration (Test Tier Purity Invariant);
  every live-tmux or pty test carries a `//go:build` constraint naming `integration`.
- **Rationale:** `cmd/lyx/tierpurity_test.go` fails the build otherwise, and the existing suites (`apply_test.go`, `generation_test.go`, `strand_test.go`) already drive composed engine call sites through `TmuxCmd.execHook` with no substrate.
- **Applies to:** all batches

### Decision: done-gate-left-as-configured

- **Decision:** `mill-config.yaml`'s `pipeline.done_gate` is left exactly as it stands — `go test ./... && go test -tags integration ./...` — and no batch edits `mill-config.yaml`.
- **Rationale:** the configured command is already the right repo-wide gate for this task: the batch verifies are package-scoped, and the tier-2 proof this task adds is `integration`-tagged, which the untagged half of the gate would never execute.
  Its inline comment names an earlier task's specifics, but the command itself needs no change, and editing the hub config mid-flight would trip the `wiki-config-mutation` gate for a comment's worth of accuracy.
- **Applies to:** all batches

### Decision: doc-inventory-is-closed-except-by-discovery

- **Decision:** the documentation inventory for this task is `internal/reedengine/doc.go`, both `reed.yaml` templates' `width`/`height` comments, and `lyx reed attach`'s own `Long`.
  `docs/overview.md:298`/`:318` and `lyx loom run`'s `Long` were checked and are deliberately left unchanged: each describes the handover only as "hands the operator's stdio to a `tmux attach-session` child", which stays true when that child's argv gains a chained `select-layout`, and no module is added or removed from the module table.
- **Rationale:** Documentation Lifecycle requires docs in the same commit, and help accuracy is a review obligation under the CLI/Cobra Invariant.
  An implementer that finds one of the three left-unchanged surfaces has in fact drifted should update it rather than assume this list is closed.
- **Applies to:** batches 3, 4

## All Files Touched

- `go.mod`
- `go.sum`
- `internal/loomcli/bootstrap.go`
- `internal/loomcli/bootstrap_test.go`
- `internal/loomcli/run.go`
- `internal/reedcli/attach.go`
- `internal/reedcli/cli_test.go`
- `internal/reedengine/apply.go`
- `internal/reedengine/apply_test.go`
- `internal/reedengine/attach.go`
- `internal/reedengine/attach_test.go`
- `internal/reedengine/attachgeometry_integration_test.go`
- `internal/reedengine/doc.go`
- `internal/reedengine/lifecycle.go`
- `internal/reedengine/render/rules_test.go`
- `internal/reedengine/template_posix.yaml`
- `internal/reedengine/template_windows.yaml`
- `internal/reedengine/windowsize.go`
- `internal/reedengine/windowsize_test.go`
