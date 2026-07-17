# Batch: cli-wiring-and-docs

```yaml
task: Extend codeintel lookup to non-Go languages via LSP
batch: cli-wiring-and-docs
number: 3
cards: 4
verify: go test ./internal/codeintelcli/... ./cmd/lyx/...
depends-on: [2]
```

## Batch Scope

Delivers the user-facing `lyx codeintel refs` verb (`internal/codeintelcli`), wires it into the cobra
root, satisfies the CLI/Cobra and Sandbox Suite Coverage invariants, and writes the module design doc
+ overview update. After this batch the module is registered, discoverable via `--help`, and
end-to-end runnable (the driver batch 4's measurement needs). Batch-local decision: the CLI is the
sole `internal/output` consumer — it resolves cwd via `hubgeometry.Getwd()`, resolves the lyx-hub base
for the optional `servers.yaml` overlay via `hubgeometry.Resolve` (degrading to `builtins()` when the
cwd is not inside a lyx hub), and maps every engine typed error to `output.Err`.

## Cards

### Card 13: codeintel CLI verb

- **Context:**
  - `internal/weftcli/cli.go`
  - `internal/buildercli/cli.go`
  - `internal/codeintelengine/refs.go`
  - `internal/codeintelengine/load.go`
  - `internal/codeintelengine/errors.go`
  - `internal/clihelp/exec.go`
  - `internal/output/output.go`
  - `internal/hubgeometry/hubgeometry.go`
- **Edits:** none
- **Creates:**
  - `internal/codeintelcli/cli.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Package `codeintelcli`. `func Command() *cobra.Command` returns a parent `codeintel`
  command (`Use: "codeintel"`, non-empty `Short`, `RunE: clihelp.GroupRunE` so bare `lyx codeintel`
  lists subcommands and an unknown subcommand emits a JSON error). Add a `refs` subcommand
  (`Use: "refs <symbol|file:line:col>"`, non-empty `Short`, a `Long` with concrete examples of both
  the name form and the `file:line:col` form) with `Args: cobra.ExactArgs(1)` (so bare
  `lyx codeintel refs` or a 2-arg call fails through the JSON envelope, matching Card 14's assertions)
  and flags `--target-dir` (string; default cwd), `--lang` (string; override detection), `--timeout`
  (duration; default 30s). Its `RunE`: resolve the target dir (flag value, or `hubgeometry.Getwd()`
  when empty — never raw `os.Getwd`); parse the positional arg into a `codeintelengine.Query`
  (`file:line:col` when it matches that shape, else a symbol name); resolve the overlay base by
  attempting `hubgeometry.Resolve(cwd)` and, on success, loading
  `codeintelengine.LoadRegistry(layout.Cwd)` — pass the resolved `*hubgeometry.Layout`'s **`Cwd`**
  field as `baseDir` (the worktree cwd, exactly as `internal/buildercli/cli.go:187` calls
  `modelspec.LoadRegistry(layout.Cwd)`; `ConfigFile(baseDir, "servers")` resolves
  `<baseDir>/_lyx/config/servers.yaml`, so passing `l.Hub` would silently miss every overlay) — else
  falling back to `codeintelengine.BuiltinRegistry()` (defined in batch 1 Card 2); call
  `codeintelengine.References(ctx, opts)`; on success
  `output.Ok` with the reference list (each `{file,line,character}`); on error `output.Err(err.Error())`
  and non-zero exit via the `clihelp.SetExit`/`output.Err` pattern used in `weftcli`. `func RunCLI(out
  io.Writer, args []string) int = clihelp.Execute(Command(), out, args)`. Import `internal/codeintelengine`,
  `internal/clihelp`, `internal/output`, `internal/hubgeometry`, cobra. Engine typed errors surface via
  their `Error()` text; no need to branch per error type unless a distinct exit code is wanted (keep it
  uniform: any engine error → `output.Err`, exit 1).
- **Commit:** `feat(codeintelcli): lyx codeintel refs verb`

### Card 14: codeintel CLI seam tests

- **Context:**
  - `internal/weftcli/cli_test.go`
  - `internal/codeintelcli/cli.go`
  - `internal/codeintelengine/errors.go`
- **Edits:** none
- **Creates:**
  - `internal/codeintelcli/cli_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Untagged, spawn-free (no language-server launch, no git). Drive `RunCLI` through
  its seam: (1) `lyx codeintel --help` / bare `lyx codeintel` lists the `refs` subcommand and every
  command carries a `Short`; (2) `lyx codeintel refs <symbol> --target-dir <empty t.TempDir()>` returns
  the `ErrNoLanguage` path as a JSON `output.Err` envelope with a non-zero exit (an empty temp dir has
  no markers, so detection fails before any server launch — this exercises the error mapping without a
  live server); (3) assert the JSON envelope shape (one object per line, error field populated). Do NOT
  attempt a real `refs` query needing a server here — that is Card 12's integration test and batch 4's
  measurement. No `exec.Command`, no `gitexec`, no `lyxtest.Copy` (keep the file untagged and tier-1
  pure).
- **Commit:** `test(codeintelcli): CLI seam and error-envelope tests`

### Card 15: register codeintel in the cobra root + sandbox exclusion

- **Context:**
  - `internal/codeintelcli/cli.go`
  - `cmd/lyx/registration_test.go`
  - `cmd/lyx/longlist_test.go`
  - `cmd/lyx/drift_test.go`
- **Edits:**
  - `cmd/lyx/main.go`
  - `cmd/lyx/sandbox_coverage_test.go`
  - `cmd/lyx/helptree_test.go`
  - `internal/codeintelcli/cli_test.go`
  - `internal/codeintelengine/detect_test.go`
  - `internal/codeintelengine/lspclient_test.go`
  - `internal/codeintelengine/position_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** `go test ./cmd/lyx/...`'s repo-wide `tierpurity_test.go`/`hermeticenv_test.go`
  guards do a raw-substring scan for tokens like `exec.Command`/`gitexec.RunGit`/`lyxtest.Copy`
  across every untagged `*_test.go` file, including ones this batch does not otherwise touch — a
  disclaiming comment mention (e.g. "no exec.Command anywhere in this file") trips the same guard
  as a real call, by the guard's own documented design ("Comment or string-literal mentions trip
  the guard too — that is accepted (rename the mention or tag the file)"). Reword the four listed
  comment-only mentions (in `internal/codeintelcli/cli_test.go`, created by Card 14, and three
  batch-1/2 `internal/codeintelengine` test files) so the banned substrings no longer appear
  literally, with no change to test behavior. In `cmd/lyx/main.go` `newRoot()`: add the import for
  `github.com/Knatte18/loomyard/internal/codeintelcli` and add `codeintelcli.Command()` to the
  `root.AddCommand(...)` call; append `codeintel` to the root `Long` "Available modules:" list. In
  `cmd/lyx/sandbox_coverage_test.go`, add an `excludedModules` entry:
  `"codeintel": "requires an external language-server binary (gopls/pyright/csharp-ls) on $PATH; exercised by //go:build integration tests, not the black-box sandbox suite"`.
  In `cmd/lyx/helptree_test.go`, update the **pinned sets** (per the CLI/Cobra Invariant's "update the
  pinned sets in the same commit"): add `"codeintel"` to the `requiredModules` slice and add a
  `{module: "codeintel", wantSubs: []string{"refs"}}` case to `TestHelpTree_VerbModuleSubcommands`. No
  edits to `registration_test.go`/`longlist_test.go`/`drift_test.go` are needed — those three
  auto-derive from the live root (registration auto-discovers `internal/*cli` `Command()` packages;
  longlist asserts `root.Long` names each registered module; drift asserts every command has a
  `Short`), so registering the command + naming it in `Long` + the `Short` on every command (Card 13)
  satisfies them. Confirm the full `go test ./cmd/lyx/...` passes.
- **Commit:** `feat(cmd/lyx): register codeintel module + sandbox exclusion`

### Card 16: module design doc + overview update

- **Context:**
  - `docs/modules/README.md`
  - `docs/modules/loom.md`
  - `docs/overview.md`
  - `internal/codeintelengine/refs.go`
  - `internal/codeintelcli/cli.go`
  - `internal/codeintelengine/registry.go`
- **Edits:**
  - `docs/overview.md`
- **Creates:**
  - `docs/modules/codeintel.md`
- **Deletes:** none
- **Moves:** none
- **Requirements:** `docs/modules/codeintel.md`: document the module design — the engine/CLI split, the
  generalized LSP client (references-only + `workspace/symbol` resolver, deadline/hard-kill), the
  language-server registry (built-ins + `servers.yaml` overlay + `match` all/any semantics + pinned
  detection precedence), the typed error vocabulary, the `lyx codeintel refs` verb surface, and the
  explicit scope boundaries (uniform LSP path with `go.mod → gopls`; in-process `go/packages` arm,
  callHierarchy, implementation, and a lyx-owned server install/pin story all deferred). Cross-link
  the in-tree `docs/research/codeintel-spike.md` (#008) and `docs/research/codeintel-multilang.md`
  (created in batch 4). Refer to `docs/modules/websterv2_extension.md` for the origin reasoning **by
  name in prose** — that doc lives on `main` (not in this worktree), so mention it rather than writing
  a relative link that would dangle at this branch's HEAD. In `docs/overview.md`, add
  `codeintel` to the module table / execution-stack listing in the same style as the existing entries.
- **Commit:** `docs(codeintel): module design doc + overview entry`

## Batch Tests

`verify: go test ./internal/codeintelcli/... ./cmd/lyx/...` covers Card 14's untagged CLI seam/error
tests and, in `cmd/lyx`, the registration/longlist/helptree/drift/sandbox-coverage guards that Card 15
must satisfy (every command has `Short`; the module is registered, named in `root.Long`, and either
sandbox-covered or on the `excludedModules` allowlist). Scope is the two packages this batch touches —
`cmd/lyx` must be included because registration is validated there. The untagged run stays offline and
spawn-free.
