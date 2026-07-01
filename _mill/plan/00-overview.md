# Plan: CLI ergonomics from the sandbox run: config editor + warp error wrapping

```yaml
task: "CLI ergonomics from the sandbox run: config editor + warp error wrapping"
slug: sandbox-cli-ergonomics
approved: false
started: "20260701-080000"
parent: main
root: ""
verify: go build ./...
```

## Batch Index

```yaml
batches:
  - number: 1
    name: config-set-flag
    file: 01-config-set-flag.md
    depends-on: []
    verify: go test ./internal/yamlengine/... ./internal/configengine/... ./internal/configcli/...
  - number: 2
    name: warpengine-stderr-fix
    file: 02-warpengine-stderr-fix.md
    depends-on: []
    verify: go test ./internal/warpengine/...
  - number: 3
    name: weft-hubgeometry-stderr-fix
    file: 03-weft-hubgeometry-stderr-fix.md
    depends-on: []
    verify: go test ./internal/weftengine/... ./internal/hubgeometry/...
```

## Shared Decisions

### Decision: never let git's raw stderr reach a JSON-facing error message

- **Decision:** every error message touched by this task is composed only from data the
  Go code already has locally (action being performed, branch/module/path name, git exit
  code). Git's own `stderr` text is never interpolated into an error string, `fmt.Errorf`,
  or `fmt.Sprintf` that reaches `internal/output`'s JSON envelope or a result struct's
  `Error` field. Exit codes are fine to include — they are plain integers, not
  git-authored text.
- **Rationale:** matches the same-day precedent in `internal/hubgeometry/hubgeometry.go`
  `Resolve()` (commit `eeb539f`), whose test `TestResolve_NotAGitRepo` pins "no `fatal:`
  substring" in the error text. This task extends that convention everywhere else it was
  violated in the config/warp/weft surface.
- **Applies to:** `warpengine-stderr-fix`, `weft-hubgeometry-stderr-fix`. (Not
  `config-set-flag` — that batch does not touch any git-stderr call sites.)
  **Deliberately NOT applied to:** `internal/boardengine/git.go:27`, `:100`, and
  `internal/boardengine/sync.go:133`, which have the identical
  `"... failed: %s", stderr` shape but belong to a different module (board) outside this
  task's WARN-driven scope (config + warp/weft only, per `_mill/discussion.md`'s Scope →
  Out). Left for a separate task if desired — not a gap in this task's coverage of its
  own stated scope.

### Decision: `--set` never invokes the interactive editor

- **Decision:** the new `lyx config <module> --set key=value` path never calls
  `configengine.DefaultEditor` or any `EditorFunc`. It is a fully separate, non-interactive
  write path that reuses only `configengine`'s scaffold-on-missing behavior, not its
  editor-loop/validation-loop.
- **Rationale:** the entire point of `--set` is a script/agent-safe path with zero GUI/editor
  dependency (see `_mill/discussion.md` Problem section — the sandbox suite itself hit this
  gap).
- **Applies to:** `config-set-flag`.

### Decision: exact message wording is illustrative, not literal

- **Decision:** each card below gives an example replacement message (e.g. `"host switch to
  branch %q failed (git exit %d)"`). The exact wording is an implementation call — the
  hard constraint is only that no `stderr`/git-authored substring reaches the message. Tests
  assert absence of git's own wording (e.g. `"fatal:"`), not an exact string match against
  the example text.
- **Rationale:** avoids over-constraining phrasing across 16 near-identical mechanical
  edits; matches `_mill/discussion.md`'s Technical Context note on this same point.
- **Applies to:** `warpengine-stderr-fix`, `weft-hubgeometry-stderr-fix`.

### Decision: `verify:` commands use the native Go test runner, no `PYTHONPATH=` prefix

- **Decision:** this is a Go project (module `github.com/Knatte18/loomyard`); every
  `verify:` command in this plan is a plain `go test`/`go build` invocation with no
  `PYTHONPATH=` prefix.
- **Rationale:** the `PYTHONPATH=` isolation rule applies only to Python/mill-script
  projects; Go projects use the native toolchain directly per mill-plan's own verify-command
  guidance.
- **Applies to:** all batches.

## All Files Touched

- `docs/overview.md`
- `internal/configcli/configcli.go`
- `internal/configcli/configcli_integration_test.go`
- `internal/configcli/configcli_test.go`
- `internal/configengine/edit.go`
- `internal/configengine/set.go`
- `internal/configengine/set_test.go`
- `internal/hubgeometry/worktreelist.go`
- `internal/hubgeometry/worktreelist_test.go`
- `internal/warpengine/add.go`
- `internal/warpengine/add_test.go`
- `internal/warpengine/checkout.go`
- `internal/warpengine/checkout_test.go`
- `internal/warpengine/cleanup.go`
- `internal/warpengine/cleanup_test.go`
- `internal/warpengine/clone.go`
- `internal/warpengine/clone_test.go`
- `internal/warpengine/junction.go`
- `internal/warpengine/prune.go`
- `internal/warpengine/prune_test.go`
- `internal/warpengine/reconcile.go`
- `internal/warpengine/reconcile_test.go`
- `internal/warpengine/weftwiring.go`
- `internal/warpengine/weftwiring_test.go`
- `internal/weftengine/sync.go`
- `internal/weftengine/sync_test.go`
- `internal/yamlengine/set.go`
- `internal/yamlengine/set_test.go`
