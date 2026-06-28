# Batch: config-print-and-help

```yaml
task: "CLI help & error ergonomics from sandbox run"
batch: "config-print-and-help"
number: 5
cards: 4
verify: go test ./internal/configcli/...
depends-on: [1]
```

## Batch Scope

Delivers the config-module ergonomics: a non-interactive `--print` read-only mode (W12), a
dynamically-built `Long` that lists the valid module names (W5), and harmonization of
config's existing plain-text errors to the `output.Err` JSON envelope (W12/W5 consistency).
Depends only on batch 1 (it adopts the `output.Err` JSON convention and the W15 trim).
Independent of the W16 batch — `config` is a leaf command (`config [module]`), not a parent
group, so it gets no W16 RunE. Batch-local decision: `--print` emits **raw on-disk YAML** on
success (exit 0, not wrapped); only errors are JSON. Path access goes through
`paths.ConfigFile` per the Path Invariant.

## Cards

### Card 17: config --print flag and single/aggregate print logic (W12)

- **Context:**
  - `internal/configreg/configreg.go`
  - `internal/paths/paths.go`
  - `internal/config/edit.go`
  - `internal/output/output.go`
- **Edits:**
  - `internal/configcli/configcli.go`
  - `internal/configcli/configcli_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:**
  - In `Command()`, switch to a `configCmd` variable (closure pattern, like warp's
    `removeCmd`) so the flag is readable in the handler. Register a local bool flag:
    `configCmd.Flags().Bool("print", false, "print on-disk config as YAML without launching the editor")`.
    The `RunE` reads `print` from the flag set and passes it into the handler.
  - Add the print path, gated on the `print` flag, evaluated **before** the existing
    edit/menu dispatch. Compute `baseDir := filepath.Join(l.WorktreeRoot, l.RelPath)` and
    resolve each file with `paths.ConfigFile(baseDir, module)` (never a literal).
  - Preserve the existing `Args: cobra.MaximumNArgs(1)` on the `config` command when
    switching to the `configCmd` variable, so `config a b c` still rejects extra positionals.
  - Single-module form (`config <module> --print`, `len(args) >= 1`): if `module` is not a
    known registry name (`configreg.Template(module)` returns `ok == false`), return the
    harmonized unknown-module JSON error (card 19). Otherwise `os.ReadFile` the resolved
    path; on `os.IsNotExist` return `output.Err` (exit 1) with a clear "not configured /
    missing" message naming the module and path; on any other read error return `output.Err`
    (exit 1); on success write the raw bytes to `out` verbatim and return 0.
  - Aggregate form (`config --print`, no module arg): iterate `configreg.Names()` in registry
    order. For each name write a `# <name>` delimiter header line; `os.ReadFile` the resolved
    path; if present write the file's YAML (followed by a trailing newline if the file lacks
    one); if `os.IsNotExist` write a single `# (not configured)` line; on any other read
    error return `output.Err` (exit 1). After the loop return 0. The aggregate form never
    errors on absence — it is deterministic regardless of which files exist.
  - Thread the `print` flag from the cobra layer through `runConfig`/`dispatch` to this logic
    (extend the handler signature or read it via the closure — implementer's choice — but do
    not consult `os.Args` directly).
- **Commit:** `feat(config): add --print read-only mode`

### Card 18: Dynamic config Long listing module names (W5)

- **Context:**
  - `internal/configreg/configreg.go`
- **Edits:**
  - `internal/configcli/configcli.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Build the `config` command's `Long` at `Command()` construction so it
  includes the live module list from `configreg.Names()` (e.g. append a line like
  `Known modules: <comma-joined names>.`), rather than a hardcoded list. Keep the existing
  prose about the interactive menu and per-module editing, and mention the `--print` flag.
  `ValidArgs = configreg.Names()` stays. Keep `Short` non-empty (drift guard).
- **Commit:** `docs(config): list known modules in config help`

### Card 19: Harmonize config errors to output.Err (W12/W5 consistency)

- **Context:**
  - `internal/output/output.go`
  - `internal/configreg/configreg.go`
- **Edits:**
  - `internal/configcli/configcli.go`
  - `cmd/lyx/main_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Replace the plain-text `fmt.Fprintf(out, ...); return 1` error emissions
  with `return output.Err(out, <msg>)` so every config error is the JSON envelope:
  - `editOne` unknown-module branch — message must still include the known-module list
    (e.g. `fmt.Sprintf("unknown config module: %s (known: %v)", module, configreg.Names())`)
    so W5 discoverability survives on the error path.
  - `editOne` `ErrAborted` branch and the generic edit-error branch.
  - `editOne` sync-failure branch (preserve the captured sync output in the message).
  - `runConfig`'s `paths.Getwd` and `paths.Resolve` failure branches.
  - Leave success output untouched: the `edited and synced ...` line, the interactive menu's
    prompts/success text, and the `--print` raw-YAML output are NOT errors and stay as-is.
    Add the `internal/output` import; drop the now-unused `fmt` import only if nothing else
    uses it (the success `Fprintf` lines likely still need `fmt`).
  - Fix the now-stale comment in `cmd/lyx/main_test.go` `TestRunDispatchesToConfig` that
    says config output is human-readable text / not JSON — config errors are now JSON. Update
    the comment text only (the test's exit-code assertion is unaffected); do not weaken the
    test.
- **Commit:** `fix(config): emit config errors as JSON envelope`

### Card 20: Config tests — print, dynamic Long, JSON errors

- **Context:**
  - `internal/configcli/configcli.go`
  - `internal/configreg/configreg.go`
  - `internal/paths/paths.go`
  - `internal/output/output.go`
- **Edits:**
  - `internal/configcli/configcli_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Add tests (follow existing `configcli_test.go` fixture/seam patterns;
  use the `paths` helpers to seed `_lyx/config/<module>.yaml`, and inject the editor seam so
  the print path can assert the editor is never called):
  - `config <module> --print` for a seeded module emits the file's YAML verbatim at exit 0
    and never invokes the editor; for a known-but-unseeded module it returns `ok:false` JSON,
    exit 1.
  - `config --print` (aggregate) with only a subset of modules seeded emits a `# <module>`
    header for every `configreg.Names()` entry, the YAML for seeded ones, `# (not configured)`
    for absent ones, and exits 0 (assert determinism with a partial seed).
  - `config bogus --print` (unknown module name) returns `ok:false` JSON, exit 1.
  - `config bogus` (unknown module, no `--print`) now returns the JSON envelope `ok:false`
    whose `error` contains the known-module names (update any existing assertion that
    expected the old plain-text form).
  - `config --help` `Long` contains every name from `configreg.Names()` (assert membership
    by iterating the registry, not a hardcoded list).
- **Commit:** `test(config): cover --print, dynamic Long, and JSON errors`

## Batch Tests

`verify: go test ./internal/configcli/...` covers the new `--print` single/aggregate logic,
the dynamic `Long`, and the harmonized JSON error envelope. Tests seed config files through
the `paths.ConfigFile` helper (Path Invariant) and inject the editor seam to prove `--print`
never launches the editor. The existing `configcli_integration_test.go` is re-run by the
same package scope as a regression guard.
