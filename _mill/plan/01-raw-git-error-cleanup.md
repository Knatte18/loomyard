# Batch: raw-git-error-message-cleanup

```yaml
task: Fix lyx CLI defects + host-commit gap from the sandbox run
batch: raw-git-error-message-cleanup
number: 1
cards: 5
verify: go build ./... && go test -tags integration ./internal/hubgeometry/... ./internal/idecli/... ./internal/initcli/... && go test ./internal/configcli/... ./internal/muxpoccli/...
depends-on: []
```

## Batch Scope

Fixes GitHub issue #36 point 3: a raw git `fatal: not a git repository (or any of the
parent directories): .git` string leaks unwrapped through `lyx`'s JSON error envelope
whenever a command is run outside any git repository. The root cause is in
`internal/hubgeometry/hubgeometry.go`'s `Resolve()`, which interpolates git's raw
stderr into the wrapped `ErrNotAGitRepo` sentinel. Four CLI call sites
(`idecli`, `initcli`, `configcli`, `muxpoccli`) compound the problem by adding their own
redundant prefix on top of that already-leaky message — `muxpoccli` worst of all,
producing a literal double statement of "not a git repository". This batch is one unit
because every card shares the same root cause and the same verification shape (assert
the JSON `error` field is the clean bare sentinel text, with no `fatal:` substring and
no doubled prefix). No card in this batch touches the JSON-always error envelope format
itself (see Shared Decision "No new error-format surface" in the overview) — only the
*content* of one specific error message changes.

External interface for later batches: none. This batch is fully self-contained; Batch 2
and Batch 3 do not depend on it and touch disjoint files.

## Cards

### Card 1: Strip raw git stderr from hubgeometry.ErrNotAGitRepo

- **Context:** none
- **Edits:**
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/hubgeometry/hubgeometry_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - In `Resolve()`, the `exitCode != 0` branch currently reads
    `return nil, fmt.Errorf("%w: %s", ErrNotAGitRepo, stderr)`. Change it to
    `return nil, ErrNotAGitRepo` — return the bare sentinel with no appended text. This
    is the branch that fires when `git rev-parse --show-toplevel` runs successfully but
    reports failure (e.g. cwd is outside any git repository), which is the exact path
    that produced the #36 point 3 repro.
  - Leave the preceding `err != nil` branch
    (`return nil, fmt.Errorf("%w: %v", ErrNotAGitRepo, err)`) UNCHANGED. That branch
    fires when the `git` subprocess itself fails to spawn (e.g. binary missing) — `err`
    there is Go's own exec-layer error, not git's stderr, so it does not reproduce the
    leak and the diagnostic content is useful as-is.
  - In `hubgeometry_test.go`, extend `TestResolve_NotAGitRepo` (the existing test that
    asserts `errors.Is(err, hubgeometry.ErrNotAGitRepo)` against a `Resolve()` call in a
    non-git `t.TempDir()`) with two additional assertions on the same `err`: (1)
    `err.Error()` does not contain the substring `"fatal:"`; (2) `err.Error()` equals
    exactly `hubgeometry.ErrNotAGitRepo.Error()` (i.e. `"not a git repository"`, pinning
    the bare-sentinel behavior with no appended text).
- **Commit:** `fix(hubgeometry): stop leaking raw git stderr into ErrNotAGitRepo`

### Card 2: Drop redundant prefix in idecli's resolve-layout error

- **Context:**
  - `internal/configcli/reconcile_test.go`
- **Edits:**
  - `internal/idecli/cli.go`
  - `internal/idecli/cli_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - In `Command()`'s `PersistentPreRunE`, the `hubgeometry.Resolve(cwd)` failure branch
    currently reads
    `output.Err(cmd.OutOrStdout(), fmt.Sprintf("failed to resolve layout: %v", err))`.
    Change it to `output.Err(cmd.OutOrStdout(), err.Error())` — drop the prefix, since
    `hubgeometry.Resolve()`'s error (after Card 1) is already self-describing.
  - In `cli_test.go`, add a new test that runs `RunCLI` with `["menu"]` (a subcommand
    that requires layout resolution; the `PersistentPreRunE` aborts before `menu`'s body
    runs, so the test never reaches the interactive picker) from a non-git temp
    directory, and asserts the parsed JSON envelope's `error` field equals exactly
    `"not a git repository"` — confirming no `"failed to resolve layout:"` prefix and no
    `"fatal:"` substring survive. Follow the `os.Chdir`-into-`t.TempDir()` pattern used
    by `TestReconcile_DryRun` in `internal/configcli/reconcile_test.go` (read for
    pattern only, not edited) to change into a temp directory for the duration of the
    test.
- **Commit:** `fix(idecli): drop redundant prefix on resolve-layout error`

### Card 3: Drop redundant prefix in initcli's resolve-layout error

- **Context:**
  - `internal/configcli/reconcile_test.go`
- **Edits:**
  - `internal/initcli/initcli.go`
  - `internal/initcli/initcli_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - In `runInit`, the `hubgeometry.Resolve(cwd)` failure branch currently reads
    `return output.Err(out, fmt.Sprintf("failed to resolve layout: %v", err))`. Change
    it to `return output.Err(out, err.Error())`.
  - In `initcli_test.go`, add a new test that calls `initcli.RunInit(&buf, []string{})`
    from a non-git temp directory (use `t.Chdir`, the pattern already established by
    `TestRunInit_FirstRun` in this same file) and asserts the parsed JSON envelope's
    `error` field equals exactly `"not a git repository"`.
- **Commit:** `fix(initcli): drop redundant prefix on resolve-layout error`

### Card 4: Drop redundant prefix in configcli's reconcile resolve-layout error

- **Context:** none
- **Edits:**
  - `internal/configcli/configcli.go`
  - `internal/configcli/reconcile_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - In `runReconcile`, the `hubgeometry.Resolve(cwd)` failure branch currently reads
    `return output.Err(out, fmt.Sprintf("resolve layout: %v", err))`. Change it to
    `return output.Err(out, err.Error())`. Do NOT touch the second, unrelated
    `hubgeometry.Resolve(cwd)` call site elsewhere in `configcli.go` (around line
    284–286) — it already calls bare `err.Error()` with no prefix and needs no change.
  - In `reconcile_test.go`, add a new test that runs `RunCLI` with `["reconcile"]` from
    a non-git temp directory — following the existing `os.Chdir`-into-`t.TempDir()` +
    `defer os.Chdir(oldCwd)` pattern already used by `TestReconcile_DryRun` in this same
    file — and asserts the parsed JSON envelope's `error` field equals exactly
    `"not a git repository"`.
- **Commit:** `fix(configcli): drop redundant prefix on reconcile resolve-layout error`

### Card 5: Fix muxpoccli's doubled "not a git repository" message

- **Context:** none
- **Edits:**
  - `internal/muxpoccli/cli.go`
  - `internal/muxpoccli/cli_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - In the parent `muxpoc` command's `PersistentPreRunE`, the `hubgeometry.Resolve(cwd)`
    failure branch currently reads
    `output.Err(c.OutOrStdout(), fmt.Sprintf("not a git repository: %v", err))` — this
    doubles the sentinel's own "not a git repository" text on top of itself. Change it
    to `output.Err(c.OutOrStdout(), err.Error())`.
  - In `cli_test.go`, add a new test that runs `RunCLI` with `["status"]` from a non-git
    temp directory (use `t.Chdir(t.TempDir())` — no existing test in this package has a
    chdir helper to mirror, but `t.Chdir` is idiomatic and self-sufficient) and asserts
    the parsed JSON envelope's `error` field equals exactly `"not a git repository"`
    (i.e. NOT `"not a git repository: not a git repository"`).
- **Commit:** `fix(muxpoccli): stop doubling "not a git repository" in resolve-layout error`

## Batch Tests

`verify` runs `go build ./...` (whole-repo compile check) followed by
`go test -tags integration ./internal/hubgeometry/... ./internal/idecli/... ./internal/initcli/...`
(the three packages whose tests carry the `//go:build integration` tag) and then
`go test ./internal/configcli/... ./internal/muxpoccli/...` (untagged default test run,
since `internal/configcli/reconcile_test.go` and `internal/muxpoccli/cli_test.go` — the
two files this batch edits in those packages — carry no build tag and run by default;
`internal/configcli/configcli_integration_test.go` and
`internal/muxpoccli/muxpoc_smoke_test.go` are untouched by this batch and are not
exercised by the untagged run, which is fine since neither is affected by these
changes). Together these four invocations cover every test file this batch edits
(`hubgeometry_test.go`, `cli_test.go` in idecli/muxpoccli, `initcli_test.go`,
`reconcile_test.go`) plus a whole-repo compile check.
