# Plan: Decide tmux mouse-mode default for lyx mux

```yaml
task: "Decide tmux mouse-mode default for lyx mux"
slug: "mux-mouse-default"
approved: true
started: "20260715-072956"
parent: "cluster-fork-spike"
root: ""
verify: null
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to
schedule batches. Every batch lives at `NN-<batch-slug>.md` in this
directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: mouse-default
    file: 01-mouse-default.md
    depends-on: []
    verify: go test ./internal/muxengine/
```

## Shared Decisions

### Decision: mouse-value-contract

- **Decision:** The `mouse` config value is a string accepting only `on`/`off`
  (case-insensitive, surrounding whitespace trimmed). Every other value — the empty
  string included — is a hard error surfaced up front at boot, before any psmux
  round-trip. `mouseOption("")` errors; it never silently defaults to `off`. The
  `${env:LYX_MUX_MOUSE:-off}` template default supplies `off` only when the env var is
  unset, so a well-formed config never reaches the helper empty; an empty value means a
  misconfiguration and must fail loud.
- **Rationale:** Mirrors the `debug_log`/`debugLogArgs` precedent (validate-up-front,
  fail loud), while removing `debug_log`'s comment-vs-code ambiguity about the empty case
  (see discussion.md, Decision: validate-up-front).
- **Applies to:** all batches

### Decision: explicit-set-both-ways-at-boot

- **Decision:** Boot always runs `set-option -g mouse <on|off>` with the resolved value —
  including explicitly setting `off`. It is never a skipped call. The call sits alongside
  the existing `set-option -g remain-on-exit on` in
  `Engine.ensureServerAndSessionLocked` (lifecycle.go), and — like `remain-on-exit` — only
  runs on a fresh boot that spawns a `new-session`; a session already up with live panes
  hits the early return and does not re-apply it.
- **Rationale:** Makes the live mouse state deterministic regardless of the psmux/tmux
  backend default, in both directions (discussion.md, Decisions: psmux-default-moot and
  explicit-set-both-ways).
- **Applies to:** all batches

### Decision: helper-lives-in-mouse.go

- **Decision:** The value parser/validator `mouseOption` and its table-driven unit test
  are new files `internal/muxengine/mouse.go` and `internal/muxengine/mouse_test.go`,
  package `muxengine` (internal, so the test reaches the unexported helper). This mirrors
  the `debugLogArgs`-in-`serverlog.go` precedent: an unexported boot-time validator with
  its own package-internal test.
- **Rationale:** Keeps the mouse concern self-contained and testable without touching the
  larger `serverlog.go`/`lifecycle.go` bodies.
- **Applies to:** all batches

### Decision: docs-target-reconciliation

- **Decision:** discussion.md refers to updating "the mux module doc under
  `docs/modules/`" — **no such file exists** (`docs/modules/` holds only builder,
  hardener, loom, plan-format). The mux module's design documentation is its package
  godoc in `internal/muxengine/doc.go`, which is where the boot-option contract
  (`set-option -g remain-on-exit`, and the `debug_log` verbose-flag opt-in) is already
  documented. The mouse boot option and its fresh-boot-only / no-live-toggle semantics are
  therefore documented in `doc.go`. The config-key documentation is the inline comment on
  the `mouse:` line in both template yamls (exactly as `debug_log` is self-documented).
  `docs/overview.md` carries no mux boot-behavior content (verified: zero grep hits for
  `mouse`/`remain-on-exit`/`mux.yaml`), so it needs no change; `docs/roadmap.md` is not
  touched (delivered work, not a milestone). No new `CONSTRAINTS.md` invariant.
- **Rationale:** Honors CLAUDE.md's Documentation Lifecycle ("update the module doc")
  against the repo's actual layout — the module doc for muxengine is `doc.go`, not a
  nonexistent `docs/modules/mux.md`.
- **Applies to:** all batches

### Decision: integration-test-gating

- **Decision:** The real-`Up()` boot test is a new `//go:build integration`-tagged file,
  self-skipping when the configured multiplexer binary is absent, run via
  `go test -tags integration ./internal/muxengine/` — consistent with the existing
  `contract_integration_test.go`. It is NOT part of the batch `verify:` (which runs the
  default, untagged unit tests), matching how the existing integration contract test is
  excluded from fast unit runs.
- **Rationale:** discussion.md, Testing section ("Gate/skip consistently with the existing
  integration tests"). The guarantee is pinned in code; it runs in the integration lane,
  not the fast lane.
- **Applies to:** all batches

## All Files Touched

- `internal/muxengine/config.go`
- `internal/muxengine/config_test.go`
- `internal/muxengine/doc.go`
- `internal/muxengine/lifecycle.go`
- `internal/muxengine/mouse.go`
- `internal/muxengine/mouse_boot_integration_test.go`
- `internal/muxengine/mouse_test.go`
- `internal/muxengine/template_posix.yaml`
- `internal/muxengine/template_windows.yaml`
