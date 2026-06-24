# Plan: ly-git-clone hub-creator (host, weft, board)

```yaml
task: "ly-git-clone hub-creator (host, weft, board)"
slug: "ly-git-clone"
approved: true
started: "20260624-162514"
parent: "main"
root: ""
verify: null
```

## Batch Index

```yaml
batches:
  - number: 1
    name: gitclone-command
    file: 01-gitclone-command.md
    depends-on: []
    verify: go test -tags=integration ./internal/gitclone/ ./cmd/lyx/ ./internal/paths/
  - number: 2
    name: durable-docs
    file: 02-durable-docs.md
    depends-on: []
    verify: null
```

## Shared Decisions

### Decision: go-native-verify

- **Decision:** This is a Go project; `verify:` commands use `go test` directly with **no**
  `PYTHONPATH=` prefix. Integration tests are gated behind a `//go:build integration` build
  tag and run with `-tags=integration` (the project convention — see
  `internal/weft/weft_integration_test.go`). A single `go test -tags=integration <pkgs>`
  run executes both the untagged unit tests and the tagged integration tests.
- **Rationale:** Matches the existing weft/worktree integration suites; keeps fast unit runs
  separate from git-fixture integration runs while a single verify covers both.
- **Applies to:** all batches.

### Decision: path-invariant-compliance

- **Decision:** No raw `os.Getwd` or `git rev-parse --show-toplevel` anywhere in
  `internal/gitclone`. The current working directory is obtained once via `paths.Getwd()`
  in `RunCLI` and passed as an explicit `cwd` parameter into `cloneHub`. The Hub root is
  plain path construction (`filepath.Join(cwd, name+"-HUB")`), not geometry resolution, so
  `paths.Resolve` is **not** called (no git repo exists at the Hub root).
- **Rationale:** `internal/paths/enforcement_test.go` fails the build if either banned
  primitive appears outside the two allowed files; the verify includes `./internal/paths/`
  to catch a violation immediately.
- **Applies to:** gitclone-command.

### Decision: dormant-hub

- **Decision:** `lyx git-clone` clones three repos and does nothing else. It creates **no**
  `_lyx`/`_codeguide` junctions, **no** `.git/info/exclude` entries, **no** config files,
  and does **not** run `lyx init` or any activation. The produced Hub is dormant until lyx
  is activated separately.
- **Rationale:** Operator decision (discussion §Out, §Decisions). Activation is a separate
  task; mixing it in would couple this command to junction/exclude wiring it must not own.
- **Applies to:** gitclone-command.

### Decision: json-output-contract

- **Decision:** `RunCLI(out io.Writer, args []string) int` emits a JSON envelope via
  `internal/output` (`output.Ok` / `output.Err`), exit 0 on success / 1 on error, matching
  every other `lyx` subcommand.
- **Rationale:** Uniform CLI contract across the `lyx` binary.
- **Applies to:** gitclone-command.

## All Files Touched

- `cmd/lyx/main.go`
- `cmd/lyx/main_test.go`
- `docs/overview.md`
- `docs/roadmap.md`
- `internal/gitclone/cli.go`
- `internal/gitclone/clone.go`
- `internal/gitclone/clone_integration_test.go`
- `internal/gitclone/gitclone.go`
- `internal/gitclone/gitclone_test.go`
