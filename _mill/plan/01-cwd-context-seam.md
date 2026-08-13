# Batch: cwd-context seam

```yaml
task: Unblock t.Parallel on hub-fixture tests that currently t.Chdir
batch: cwd-context seam
number: 1
cards: 5
verify: go test ./internal/lyxcwd/... ./internal/clihelp/... ./cmd/lyx/...
depends-on: []
```

## Batch Scope

This batch builds the infrastructure every later batch consumes and touches no module CLI at all: the context-carried cwd API owned by `internal/lyxcwd` (`WithCwd`/`CwdFrom`), the three `internal/clihelp` siblings that let a cwd reach a cobra command's context (`RunRootCtx`, `ExecuteIn`, `WrapRunCtx`), the compile-time assertion pinning the existing eleven-module `RunCLI` seam shape, and the doc updates those three surfaces cause.
It is one batch because all five cards live in exactly two packages plus two doc files, and because nothing here is useful in isolation — batch 2 cannot begin until `ExecuteIn` and `WrapRunCtx` both exist.
The external interface batch 2 consumes is exactly: `lyxcwd.WithCwd`, `lyxcwd.CwdFrom`, `clihelp.ExecuteIn`, `clihelp.RunRootCtx`, and `clihelp.WrapRunCtx`.
Batch-local decision differing from `## Shared Decisions`: none — this batch adds only additive API, so `cmd/lyx/main.go`, `clihelp.RunRoot`, `clihelp.Execute`, and `clihelp.WrapRun` all keep their current signatures and behaviour exactly.

## Cards

### Card 1: `lyxcwd` context-carried cwd API

- **Context:**
  - `internal/lyxcwd/anchor.go`
  - `internal/lyxcwd/leaf_enforcement_test.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/lyxcwd/lyxcwd.go`
- **Creates:**
  - `internal/lyxcwd/cwdcontext.go`
  - `internal/lyxcwd/cwdcontext_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add `WithCwd(ctx context.Context, dir string) context.Context` and `CwdFrom(ctx context.Context) (string, error)` in the new file `internal/lyxcwd/cwdcontext.go`, package `lyxcwd`.
  Key the value on an unexported empty-struct type declared in `cwdcontext.go` (e.g. `type cwdKey struct{}`), never on a bare string, so no other package can collide with it.
  `WithCwd` panics when `dir == ""` and when `!filepath.IsAbs(dir)`;
  the panic message must name both the rule and the offending value, e.g. `lyxcwd.WithCwd: dir must be an absolute path, got "relative/x"`.
  `WithCwd` never calls `filepath.Abs` and never returns an error.
  `CwdFrom` returns the stored value when the context carries one, and otherwise falls back to `Getwd()` (declared in `internal/lyxcwd/lyxcwd.go`), returning that call's error unchanged;
  the fallback exists in this one place and nowhere else.
  `cwdcontext.go` imports only `context`, `path/filepath`, and `fmt` — the file must keep `internal/lyxcwd` inside the import cap `internal/lyxcwd/leaf_enforcement_test.go` enforces (stdlib plus `internal/gitexec`).
  Extend the package doc comment at the top of `internal/lyxcwd/lyxcwd.go` with one sentence naming `WithCwd`/`CwdFrom` as the per-call cwd-injection seam and stating that the injected value is always absolute.
  Write `internal/lyxcwd/cwdcontext_test.go` first and watch it fail: it is untagged (no `//go:build` line), package `lyxcwd`, spawns nothing, and covers four cases — a context seeded via `WithCwd` returns that value from `CwdFrom`;
  a bare `context.Background()` makes `CwdFrom` return the same answer as `Getwd()`;
  `WithCwd(ctx, "")` panics;
  and `WithCwd(ctx, "relative/x")` panics.
  Use `t.Parallel()` in every test function in the new file — it is pure and has no process-global dependency.
- **Commit:** `feat(lyxcwd): add WithCwd/CwdFrom for context-carried cwd injection`

### Card 2: `clihelp.RunRootCtx` and `clihelp.ExecuteIn`

- **Context:**
  - `internal/lyxcwd/cwdcontext.go`
  - `internal/output/output.go`
  - `cmd/lyx/main.go`
- **Edits:**
  - `internal/clihelp/exec.go`
  - `internal/clihelp/exec_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `internal/clihelp/exec.go`, add `RunRootCtx(ctx context.Context, cmd *cobra.Command, out io.Writer) int` holding the entire current body of `RunRoot` with one change: it derives its exit context from the supplied `ctx` via `NewExitContext(ctx)` instead of `NewExitContext(context.Background())`.
  Rewrite `RunRoot(cmd *cobra.Command, out io.Writer) int` as exactly `return RunRootCtx(context.Background(), cmd, out)` — its signature, its two call sites in `cmd/lyx/main.go`, and its observable behaviour all stay unchanged.
  Add `ExecuteIn(cmd *cobra.Command, cwd string, out io.Writer, args []string) int` beside `Execute`: it performs the same `cmd.SetOut(out)` / `cmd.SetErr(out)` / `cmd.SetArgs(args)` wiring `Execute` does, then calls `RunRootCtx(lyxcwd.WithCwd(context.Background(), cwd), cmd, out)`.
  `ExecuteIn` never receives an empty `cwd` and needs no empty-string tolerance;
  do not add one, and do not weaken `WithCwd`'s panic to accommodate a caller.
  Leave `Execute` itself unchanged.
  Adding the `internal/lyxcwd` import to `internal/clihelp` is safe and intended: `lyxcwd` imports only stdlib plus `internal/gitexec`, so no cycle is possible.
  Write the tests first in `internal/clihelp/exec_test.go` and watch them fail: a synthetic cobra command whose `RunE` reads `lyxcwd.CwdFrom(cmd.Context())` observes the exact directory passed to `ExecuteIn`;
  the same command driven through `Execute` observes the process cwd instead;
  and `RunRootCtx` given a context carrying a value propagates that value into the command's context.
  Keep the new tests untagged and call `t.Parallel()` in each — they build a cobra command in memory and spawn nothing.
- **Commit:** `feat(clihelp): add RunRootCtx and ExecuteIn to seed an injected cwd`

### Card 3: `clihelp.WrapRunCtx`

- **Context:**
  - `internal/output/output.go`
- **Edits:**
  - `internal/clihelp/exec.go`
  - `internal/clihelp/exec_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `internal/clihelp/exec.go`, add `WrapRunCtx(fn func(ctx context.Context, out io.Writer, args []string) int) func(*cobra.Command, []string) error` immediately beside `WrapRun`.
  Its body mirrors `WrapRun` exactly — the same `ShouldAbort(cmd.Context())` short-circuit returning `nil` without calling `fn`, the same `SetExit(cmd.Context(), …)` of the handler's exit code, the same `return nil` so cobra does not double-print — differing only in passing `cmd.Context()` to `fn` as its first argument.
  Leave `WrapRun` unchanged: it stays the registration shape for handlers that resolve no cwd, which is every `internal/boardcli` and `internal/selfreportcli` registration site and most of `internal/fabriccli`'s.
  Its doc comment must say why both exist rather than one replacing the other.
  Write the test first in `internal/clihelp/exec_test.go` and watch it fail: a `WrapRunCtx`-wrapped handler receives the command's own context (assert by reading a value seeded into it), and a `WrapRunCtx`-wrapped handler short-circuits without running when `Abort` was called on that context.
  Keep the new tests untagged with `t.Parallel()`.
- **Commit:** `feat(clihelp): add WrapRunCtx carrying the command context into plain handlers`

### Card 4: pin the `RunCLI` seam signature at compile time

- **Context:**
  - `cmd/lyx/main.go`
  - `cmd/lyx/drift_test.go`
  - `cmd/lyx/tierpurity_test.go`
  - `CONSTRAINTS.md`
- **Edits:** none
- **Creates:**
  - `cmd/lyx/seamsignature_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `cmd/lyx/seamsignature_test.go`, untagged (no `//go:build` line), package `main`, holding a single package-level compile-time assertion over every module's `RunCLI`:
  a `var _ = []func(io.Writer, []string) int{…}` listing `boardcli.RunCLI`, `burlercli.RunCLI`, `configcli.RunCLI`, `fabriccli.RunCLI`, `idecli.RunCLI`, `perchcli.RunCLI`, `reedcli.RunCLI`, `scoutcli.RunCLI`, `selfreportcli.RunCLI`, `shuttlecli.RunCLI`, and `webstercli.RunCLI` — eleven entries, one per module exposing the seam.
  Take the module import paths from `cmd/lyx/main.go`'s own import block.
  The file has no test function and no runtime body: the assertion is that the package compiles, so a drifted signature is a build failure rather than a silent divergence from `CONSTRAINTS.md`.
  Its doc comment must state that plainly, state that the CLI/Cobra Invariant's seam clause was previously unenforced (`cmd/lyx/drift_test.go` asserts only that every command carries a non-empty `Short`, and no test under `cmd/lyx` referenced `RunCLI` at all), and note that the `RunCLIIn` half of the assertion lands in batch 2 alongside the functions it pins.
  The file introduces no `exec.Command`, no `gitexec.RunGit`, no `gitkit.Copy*`, and no `hubforge.NewHub`, so it needs no `cmd/lyx/tierpurity_test.go` allowlist entry.
- **Commit:** `test(cmd/lyx): pin the RunCLI seam signature at compile time`

### Card 5: record the new seam surface in the docs

- **Context:**
  - `internal/clihelp/exec.go`
  - `internal/lyxcwd/cwdcontext.go`
  - `cmd/lyx/seamsignature_test.go`
  - `cmd/lyx/drift_test.go`
  - `cmd/lyx/main.go`
- **Edits:**
  - `CONSTRAINTS.md`
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `CONSTRAINTS.md`'s Cwd Resolution Invariant, add one bullet naming `lyxcwd.WithCwd(ctx, dir)` / `lyxcwd.CwdFrom(ctx)` as the context-carried per-call cwd-injection seam, stating that the injected value must be absolute, that `WithCwd` panics otherwise, and that `CwdFrom` falls back to `Getwd()` so `Getwd` remains the single raw `os.Getwd` site.
  State explicitly that `context` is stdlib, so the import cap is unaffected.
  In `CONSTRAINTS.md`'s CLI / Cobra Invariant, correct the "Enforced by" line: `cmd/lyx/drift_test.go` enforces the non-empty `Short` rule only, and the seam signature is now pinned by `cmd/lyx/seamsignature_test.go`.
  Do not name `RunCLIIn` anywhere in this card — that amendment lands in batch 2 with the functions it describes.
  In `docs/overview.md`, extend the "Module dispatch" paragraph that currently states the seam as `RunCLI(out io.Writer, args []string) int` = `clihelp.Execute(Command(), out, args)` with a sentence naming `clihelp.ExecuteIn`, `clihelp.RunRootCtx`, and `clihelp.WrapRunCtx` as the context-carrying siblings, and stating that `cmd/lyx/main.go` uses `RunRoot` unchanged.
  Every edited line uses semantic line breaks — one sentence per line, breaking inside a long sentence only at an internal independent-clause boundary, never at a fixed column.
  Add no new inline markdown link in either file unless it resolves, anchor included.
- **Commit:** `docs: record the clihelp context siblings and correct the CLI seam enforcement line`

## Batch Tests

`verify: go test ./internal/lyxcwd/... ./internal/clihelp/... ./cmd/lyx/...` covers the three packages this batch touches, scoped per-batch rather than repo-wide.
It runs `internal/lyxcwd/cwdcontext_test.go` (card 1), the new `ExecuteIn`/`RunRootCtx`/`WrapRunCtx` cases added to `internal/clihelp/exec_test.go` (cards 2 and 3), and `cmd/lyx`'s guard suite including the compile of `cmd/lyx/seamsignature_test.go` (card 4).
No `-tags integration` is needed: this batch edits no tagged test file, and every test it adds is pure Tier 1 with no fixture, no git spawn, and no chdir.
`internal/lyxcwd/lyxcwd_test.go` is integration-tagged and therefore not exercised here;
the module-wide `go vet -tags integration ./...` in the overview frontmatter type-checks it and every other tagged file at the batch boundary, which is the compile gate that matters for an additive-API batch.
Card 5 is a docs-only card with no runnable surface of its own;
`CONSTRAINTS.md`'s Markdown Link Integrity guard for `docs/overview.md` runs inside `go test ./internal/lyxcwd/...` via `TestEnforcement_MarkdownLinks`.
