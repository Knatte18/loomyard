# Plan: Add lyx init --undo / deinit command

```yaml
task: "Add lyx init --undo / deinit command"
slug: "lyx-deinit"
approved: true
started: "20260701-085202"
parent: "main"
root: ""
verify: go build ./...
```

## Batch Index

```yaml
batches:
  - number: 1
    name: weftengine-commit-message
    file: 01-weftengine-commit-message.md
    depends-on: []
    verify: go test -tags integration ./internal/weftengine/... ./internal/weftcli/... -count=1
  - number: 2
    name: gitignore-remove
    file: 02-gitignore-remove.md
    depends-on: []
    verify: go test ./internal/gitignore/... -count=1
  - number: 3
    name: warpengine-unwire-junctions
    file: 03-warpengine-unwire-junctions.md
    depends-on: []
    verify: go test -tags integration ./internal/warpengine/... -count=1
  - number: 4
    name: initcli-undo
    file: 04-initcli-undo.md
    depends-on: [1, 2, 3]
    verify: go test -tags integration ./internal/initcli/... -count=1
```

## Shared Decisions

### Decision: `--undo` is a flag on `lyx init`, not a standalone command

- **Decision:** `lyx init --undo` reverses the scaffolding done by plain `lyx init`. It
  lives in the existing `internal/initcli` package (new file `undo.go`), not a new
  `deinit` CLI module.
- **Rationale:** explicit user preference — this is a rarely-used operation, mainly
  valuable for test/sandbox cleanup, so it doesn't warrant its own CLI surface.
  `initcli` is already a CONSTRAINTS.md-sanctioned "trivial wrapper" skip-engine
  package.
- **Applies to:** `initcli-undo`.

### Decision: any junction inconsistency is a hard error that aborts the entire run

- **Decision:** if the host `_lyx` path exists but is not a valid junction (real
  directory, or a junction pointing at the wrong/missing target), `--undo` returns an
  error immediately and performs **no other step** — no weft-content clearing, no
  `.gitignore` revert, no `.git/info/exclude` revert. This applies both inside
  `warpengine.UnwireJunctions` (junction removal vs. exclude-line removal) and at the
  `initcli.runUndo` orchestration level (junction step vs. weft-content/gitignore
  steps).
- **Rationale:** explicit user directive — these are serious, unexpected states that
  must not be silently worked around or partially papered over. A clean, fully-untouched
  abort is easier to diagnose than a partially-reverted mixed state.
- **Applies to:** `warpengine-unwire-junctions`, `initcli-undo`.

### Decision: all git operations against the weft repo go through `weftengine`

- **Decision:** `--undo` deletes the weft-side `_lyx` content via plain filesystem I/O
  (`os.RemoveAll`), but every git operation on that deletion (commit, push) goes through
  `weftengine.Commit` / `weftengine.Push` — never raw `gitexec` calls from `initcli` or
  `warpengine`.
- **Rationale:** explicit user directive; `weftengine` already owns all git operations
  into the paired weft worktree per its own package doc.
- **Applies to:** `weftengine-commit-message`, `initcli-undo`.

### Decision: `Push` runs unconditionally after `Commit`, never gated on `committed`

- **Decision:** after calling `weftengine.Commit`, always call `weftengine.Push` next —
  regardless of whether `Commit` returned `committed == true` this invocation. Mirrors
  `internal/weftcli/cli.go`'s existing `push` subcommand exactly (`Commit` then `Push`
  unconditionally, lines ~183-193 pre-change).
- **Rationale:** `weftengine.Push` already no-ops via its own internal `hasUnpushed`
  check when there's nothing to push, so calling it unconditionally is both correct and
  idempotent. Critically, gating `Push` behind `committed` would strand a prior partial
  run's "committed locally, push failed" state forever — a rerun would see
  `committed == false` (nothing new to stage) and never retry the stuck push.
- **Applies to:** `initcli-undo`.

### Decision: no separate "weft pairing" pre-gate for `--undo`

- **Decision:** unlike plain `init` (which hard-gates on "no weft pairing"), `--undo`
  has no equivalent early gate. Each step independently checks whether its own target
  exists and no-ops if absent. A never-initialized directory is a clean no-op; a
  partially-inconsistent state is still caught by the junction-validation hard-error
  guard above — no additional blanket gate is needed.
- **Rationale:** the proposal requires `--undo` to be safe to run on a directory that
  was never initialized, which an init-style hard pre-gate would prevent.
- **Applies to:** `initcli-undo`.

### Decision: Go project — no `PYTHONPATH=` prefix on `verify:` commands

- **Decision:** this is a Go repository (loomyard/lyx). All `verify:` commands in this
  plan use the native `go test` runner directly, with no `PYTHONPATH=` prefix (that
  rule is specific to Python/mill-tooling projects).
- **Rationale:** per mill-plan's own verify-command-shape guidance for non-Python
  projects.
- **Applies to:** all batches.

### Decision: top-level `verify: go build ./...` as a cross-package compile gate

- **Decision:** the overview frontmatter sets a module-wide `verify: go build ./...`,
  run after every batch's own scoped `verify:` passes.
- **Rationale:** this task changes a shared engine signature
  (`weftengine.Commit`) that other packages call into (`weftcli`). A cheap
  whole-repo compile check at every batch boundary catches any missed call site
  immediately, without paying for the full `-tags integration` suite at every step.
  `pipeline.done_gate` in `mill-config.yaml` is intentionally left unset — editing
  the hub-shared `mill-config.yaml` is out of scope for this task, and the per-batch
  compile gate already provides the safety net a done-gate would add here.
- **Applies to:** all batches.

## All Files Touched

- `docs/overview.md`
- `internal/gitignore/gitignore.go`
- `internal/gitignore/gitignore_test.go`
- `internal/initcli/initcli.go`
- `internal/initcli/undo.go`
- `internal/initcli/undo_test.go`
- `internal/warpengine/junction.go`
- `internal/warpengine/unjunction_test.go`
- `internal/weftcli/cli.go`
- `internal/weftengine/sync.go`
- `internal/weftengine/sync_test.go`
- `internal/weftengine/weft.go`
