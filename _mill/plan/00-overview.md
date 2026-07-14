# Plan: Investigate the unexplained lyx mux server crash

```yaml
task: Investigate the unexplained lyx mux server crash
slug: mux-server-crash
approved: true
started: 20260714-184916
parent: cluster-fork-spike
root: ""
verify: go vet ./...
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to
schedule batches. Every batch lives at `NN-<batch-slug>.md` in this
directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: render-defaults
    file: 01-render-defaults.md
    depends-on: []
    verify: go test ./internal/muxengine/... ./internal/muxcli/... ./cmd/lyx/...
  - number: 2
    name: debug-logging
    file: 02-debug-logging.md
    depends-on: [1]
    verify: go test ./internal/hubgeometry/... ./internal/muxengine/... ./internal/muxcli/... ./cmd/lyx/...
  - number: 3
    name: resume-hint
    file: 03-resume-hint.md
    depends-on: [2]
    verify: go test ./internal/muxengine/... ./internal/muxcli/... ./cmd/lyx/...
  - number: 4
    name: review-prompt
    file: 04-review-prompt.md
    depends-on: [3]
    verify: null
```

## Shared Decisions

### Decision: no-root-cause-claims

- **Decision:** The server-death mechanism is unproven. No code comment, help text, or
  commit message may claim the render corruption caused the crashes — everything shipped
  here is mitigation and forensic preparedness. Word prose accordingly ("unexplained
  server death", "correlated with", never "caused by").
- **Rationale:** discussion.md § mitigation-only-scope; ~20 min of instrumented repro
  failed and every routine cause was ruled out.
- **Applies to:** all batches

### Decision: debug-log-key-semantics

- **Decision:** New mux.yaml key `debug_log: ${env:LYX_MUX_DEBUG:-0}`. On
  `muxengine.Config` the field is `DebugLog string` (yaml tag `debug_log`) — a string so
  yaml.Unmarshal never chokes on non-numeric env input. All parsing/validation lives in
  a pure Go helper: `"0"` → no flags, `"1"` → `["-v"]`, `"2"` → `["-vv"]`, anything else
  → error `invalid debug_log %q: must be 0, 1 or 2` (surfaced at boot; whitespace
  trimmed before comparison). The flags are prepended to the server-spawning psmux argv
  only (global flags before the `new-session` subcommand); no other psmux invocation
  gets them. Boot-winner semantics: the key takes effect only on the boot that actually
  spawns the shared per-hub server — documented in `up`'s `Long` and the template
  comment, never "fixed" by cross-worktree config merging.
- **Rationale:** discussion.md § debug-log-config-key (including the string-type and
  boot-winner Q&A entries).
- **Applies to:** debug-logging

### Decision: hub-logs-dir

- **Decision:** The tmux/psmux server log (`tmux-server-<pid>.log`, written by tmux to
  the server process's cwd — tmux has no redirect flag) is routed by setting `cmd.Dir`
  on the spawn to `<hub>/.lyx/logs/` (created with `os.MkdirAll` before spawn). The path
  comes from a new `hubgeometry` method `Layout.HubLogsDir() string` returning
  `filepath.Join(l.Hub, dotLyxDirName, "logs")` — hub-anchored because the server is
  per-hub (one deterministic forensic location), `.lyx` because logs are machine-local
  and never git-tracked. Pane cwd stays byte-for-byte unchanged by passing
  `-c <l.Cwd>` (the invoking worktree cwd, exactly what panes inherit today) on the
  `new-session` argv. The cwd change and `-c` pinning apply on every boot regardless of
  `debug_log`.
- **Rationale:** discussion.md § server-log-under-hub-dotlyx-logs; Hub Geometry
  Invariant (the `.lyx` token stays owned by hubgeometry).
- **Applies to:** debug-logging

### Decision: log-prune-keep-3

- **Decision:** At each server boot, before spawning, prune `tmux-server-*.log` in the
  hub logs dir down to the newest **2** by mtime, so including the fresh server's log at
  most 3 log files ever exist — this is the concrete reading of discussion.md's "keep
  the newest 3". The prune decision is a pure planning helper (file names + mtimes in,
  names-to-delete out); the caller does the `os.Remove` calls and ignores
  remove-of-vanished errors. No runtime rotation — the file is held open by the server
  and `-v` volume is low.
- **Rationale:** discussion.md § server-log-under-hub-dotlyx-logs.
- **Applies to:** debug-logging

### Decision: enriched-no-session-error

- **Decision:** `requireSessionLocked` keeps today's error `no mux session; run "lyx mux
  up"` when no persisted strands exist, and emits
  `no mux session (N strands persisted); run "lyx mux resume" to rebuild, or "lyx mux up" for a bare substrate`
  when the session is absent AND persisted mux.json holds ≥1 strand. The message choice
  is a pure helper (strand count in, message out). A `LoadState` failure falls back to
  the old message — never mask the primary "no session" signal with a state-read error.
- **Rationale:** discussion.md § resume-hint-in-requireSessionLocked; `resume` is the
  verb that rebuilds strands, `up` is substrate-only.
- **Applies to:** resume-hint

### Decision: docs-in-godoc-and-long

- **Decision:** No `docs/modules/mux.md` is created and `docs/roadmap.md` is untouched.
  Documentation lives in godoc (package + function comments) and cobra `Short`/`Long`
  texts, which must be reconciled in the same card as the behavior change (CLI/Cobra
  Invariant help-accuracy obligation). `docs/reviews/mux-review-prompt.md` is updated so
  future adversarial mux reviews drive the new behaviors.
- **Rationale:** discussion.md Scope Out + Q&A ("per-module docs always go stale");
  repo Documentation Lifecycle.
- **Applies to:** all batches

### Decision: test-tiering

- **Decision:** New unit tests (debug-level parsing, prune planning, no-session message
  helper, `HubLogsDir` path math) are untagged and spawn nothing (Test Tier Purity
  Invariant). Composed live behavior gets one `//go:build smoke` test in
  `internal/muxcli` (existing package `TestMain` already satisfies the Hermetic Git Env
  Invariant). Integration-tagged tests may use `lyxtest` fixtures. Smoke waits are
  deadline-based, never fixed sleeps.
- **Rationale:** CONSTRAINTS.md Test Tier Purity + Hermetic Git Test Environment;
  discussion.md Testing.
- **Applies to:** all batches

### Decision: commit-style

- **Decision:** Plain imperative commit subjects matching repo history (e.g. "Add
  per-strand TopBandRows override to mux render"), no conventional-commit prefixes.
- **Rationale:** `git log` shows sentence-style subjects ("Reduce git spawns in
  warpengine integration tests").
- **Applies to:** all batches

## All Files Touched

- `docs/reviews/mux-review-prompt.md`
- `internal/hubgeometry/hubgeometry.go`
- `internal/hubgeometry/hubgeometry_unit_test.go`
- `internal/muxcli/add.go`
- `internal/muxcli/attach.go`
- `internal/muxcli/cli_integration_test.go`
- `internal/muxcli/smoke_debuglog_test.go`
- `internal/muxcli/up.go`
- `internal/muxengine/config.go`
- `internal/muxengine/config_test.go`
- `internal/muxengine/doc.go`
- `internal/muxengine/lifecycle.go`
- `internal/muxengine/lifecycle_test.go`
- `internal/muxengine/render/rules.go`
- `internal/muxengine/render/rules_test.go`
- `internal/muxengine/render/types.go`
- `internal/muxengine/serverlog.go`
- `internal/muxengine/serverlog_test.go`
- `internal/muxengine/strand.go`
- `internal/muxengine/template.go`
- `internal/muxengine/template_posix.yaml`
- `internal/muxengine/template_windows.yaml`
