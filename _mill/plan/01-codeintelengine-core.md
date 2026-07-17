# Batch: codeintelengine-core

```yaml
task: Extend codeintel lookup to non-Go languages via LSP
batch: codeintelengine-core
number: 1
cards: 7
verify: go test ./internal/codeintelengine/...
depends-on: []
```

## Batch Scope

Delivers the offline, leaf-level foundation of the new `internal/codeintelengine` package:
the typed error vocabulary, the language-server registry (built-ins + `servers.yaml` overlay +
embedded seed template), and marker-based language detection. No subprocess, no LSP, no network —
every test here is untagged, offline, and spawn-free. This batch also records the new
`Codeintelengine Leaf Invariant` in `CONSTRAINTS.md` alongside its enforcement test, in the same
commit. The external interface batches 2 and 3 consume: the `Registry` type, `LoadRegistry(baseDir)`,
`BuiltinRegistry()`, `DetectLanguage(...)`, the `Entry` struct, and the sentinel errors.

Batch-local decision: the registry is keyed by a canonical **language name** (`"go"`, `"python"`,
`"csharp"`, `"typescript"`, `"rust"`), and detection precedence is a fixed slice in that order:
`[go, rust, csharp, typescript, python]` — pinned in code, not derived from map iteration.

## Cards

### Card 1: typed error vocabulary

- **Context:**
  - `internal/modelspec/registry.go`
- **Edits:** none
- **Creates:**
  - `internal/codeintelengine/errors.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Declare package `codeintelengine`. Define exported sentinel error values (via
  `errors.New`) and, where they carry data, small error types implementing `error`:
  `ErrNoLanguage` (plain sentinel), `ErrServerNotFound` (a struct type wrapping the language name +
  install-hint string, with an `Error()` naming both), `ErrSymbolNotFound` (struct wrapping the
  queried symbol + target dir), `ErrAmbiguousSymbol` (struct wrapping the symbol + a
  `[]string` of candidate `file:line:col` strings, `Error()` lists them), `ErrResolverUnsupported`
  (struct wrapping the language/server name), `ErrServerTimeout` (struct wrapping the stalled phase
  string — one of `"initialize"`, `"references"`, `"workspace/symbol"` — plus the timeout duration).
  Each data-carrying type also implements `Is(target error) bool` against a package-level sentinel
  so `errors.Is` works. Import stdlib only (`errors`, `fmt`). No `internal/output` import.
- **Commit:** `feat(codeintelengine): typed error vocabulary for codeintel lookup`

### Card 2: language-server registry — Entry, builtins, precedence

- **Context:**
  - `internal/modelspec/registry.go`
  - `internal/codeintelengine/errors.go`
- **Edits:** none
- **Creates:**
  - `internal/codeintelengine/registry.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Define `type Entry struct` with fields `Markers []string`, `Match string`
  (closed vocab: `"all"` | `"any"`), `Command []string` (launch argv, first element the binary),
  and `InstallHint string`. Define `type Registry map[string]Entry` keyed by canonical language
  name. Add `func builtins() Registry` returning the pinned defaults from the discussion's
  `language-server-registry` decision: `go`→(`[go.mod]`, any, `[gopls]`, install
  `go install golang.org/x/tools/gopls@latest`); `python`→(`[pyproject.toml, setup.py, setup.cfg]`,
  any, `[pyright-langserver, --stdio]`, install `npm install -g pyright`); `csharp`→(`[.sln, .csproj]`,
  any, `[csharp-ls]`, install `dotnet tool install --global csharp-ls`); `typescript`→
  (`[package.json, tsconfig.json]`, all, `[typescript-language-server, --stdio]`, install
  `npm install -g typescript-language-server typescript`); `rust`→(`[Cargo.toml]`, any,
  `[rust-analyzer]`, install via rustup component). Add
  `var precedence = []string{"go", "rust", "csharp", "typescript", "python"}` — the fixed
  detection order. Add `func validateEntry(name string, e Entry) error` enforcing:
  non-empty `Markers`, `Match` ∈ {`all`,`any`}, non-empty `Command`, non-empty `InstallHint` —
  loud error naming `name` on any violation. Add an exported `func BuiltinRegistry() Registry`
  returning `builtins()` (a one-line public accessor) — this is the registry the CLI layer uses when
  no lyx-hub overlay base is resolvable; `builtins()` itself stays unexported. Import stdlib + `fmt`
  only.
- **Commit:** `feat(codeintelengine): language-server registry with pinned built-ins`

### Card 3: servers.yaml overlay loader

- **Context:**
  - `internal/modelspec/load.go`
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/codeintelengine/registry.go`
- **Edits:** none
- **Creates:**
  - `internal/codeintelengine/load.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `func LoadRegistry(baseDir string) (Registry, error)` mirroring
  `modelspec.LoadRegistry`: resolve the overlay path as `hubgeometry.ConfigFile(baseDir, "servers")`
  (never hand-joined, per the Hub Geometry Invariant); an absent file returns `builtins()` with no
  error (`errors.Is(err, os.ErrNotExist)`); any other read error is wrapped with the path. Decode the
  present file into `map[string]Entry` with `yaml.NewDecoder` + `KnownFields(true)` (unknown field →
  loud error). An empty/comments-only file (io.EOF, no entries) yields `builtins()` unchanged. Build
  the result from `builtins()` with each file entry overlaid as a **whole-entry replacement**, then
  run `validateEntry` on every file-supplied entry (naming the offending alias + the file path on
  failure). Import stdlib + `internal/hubgeometry` + `gopkg.in/yaml.v3`. Do NOT import
  `internal/output`.
- **Commit:** `feat(codeintelengine): optional servers.yaml overlay loader`

### Card 4: embedded seed template

- **Context:**
  - `internal/modelspec/template.go`
  - `internal/modelspec/template.yaml`
  - `internal/codeintelengine/registry.go`
- **Edits:** none
- **Creates:**
  - `internal/codeintelengine/template.go`
  - `internal/codeintelengine/template.yaml`
- **Deletes:** none
- **Moves:** none
- **Requirements:** `template.yaml` holds the seed `servers.yaml` content — one YAML block per
  built-in language (`go`/`python`/`csharp`/`typescript`/`rust`) with `markers`, `match`, `command`,
  `install_hint` fields matching `builtins()` exactly, plus a header comment explaining the file is
  operator-editable and that entries whole-replace the built-ins. `template.go` embeds it verbatim
  (`//go:embed template.yaml`) and exposes `func ConfigTemplate() string`, mirroring
  `modelspec.ConfigTemplate`. Import `_ "embed"` only.
- **Commit:** `feat(codeintelengine): embedded servers.yaml seed template`

### Card 5: marker-based language detection

- **Context:**
  - `internal/codeintelengine/registry.go`
  - `internal/codeintelengine/errors.go`
- **Edits:** none
- **Creates:**
  - `internal/codeintelengine/detect.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `func DetectLanguage(targetDir string, reg Registry, langOverride string)
  (string, Entry, error)`. If `langOverride != ""`: look it up in `reg` directly (unknown →
  loud error naming known languages, sorted); return it. Otherwise iterate `precedence`; for each
  language present in `reg`, evaluate its `Entry.Match`: `all` → every marker file must exist under
  `targetDir`; `any` → at least one marker file exists. Existence check via `os.Stat` on
  `filepath.Join(targetDir, marker)`. First satisfied entry wins; return its name + Entry. No entry
  matched → return `ErrNoLanguage` naming the markers searched. Import stdlib (`os`, `path/filepath`,
  `sort`, `fmt`) only. `targetDir` is a plain path argument — do NOT call `os.Getwd`/`hubgeometry.Getwd`
  here (the caller resolves cwd; see batch 3).
- **Commit:** `feat(codeintelengine): marker-based language detection with pinned precedence`

### Card 6: registry, load, and detection unit tests

- **Context:**
  - `internal/modelspec/load_test.go`
  - `internal/modelspec/registry_test.go`
  - `internal/codeintelengine/registry.go`
  - `internal/codeintelengine/load.go`
  - `internal/codeintelengine/detect.go`
  - `internal/codeintelengine/errors.go`
- **Edits:** none
- **Creates:**
  - `internal/codeintelengine/registry_test.go`
  - `internal/codeintelengine/load_test.go`
  - `internal/codeintelengine/detect_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** `registry_test.go`: assert `builtins()` contains all five languages with the
  exact markers/match/command/install_hint from Card 2; assert `validateEntry` rejects empty markers,
  out-of-vocab `match`, empty command, empty install_hint (each naming the entry). `load_test.go`:
  write a temp `servers.yaml` under a temp `_lyx/config/`-shaped dir (resolve the write path with
  `hubgeometry.ConfigFile(baseDir, "servers")` so the fixture matches the loader, per the Hub
  Geometry Invariant note); assert absent-file → `builtins()` (no error); whole-entry overlay replaces
  a built-in; unknown YAML field → error; out-of-vocab `match` in the file → error naming alias +
  path; empty file → `builtins()`. `detect_test.go`: table test building synthetic marker trees under
  `t.TempDir()` (touch marker files with `os.WriteFile`, no git spawn); cover each single-language
  case, the AND case (TypeScript needs both `package.json` and `tsconfig.json` — one alone does not
  match), the precedence case (a dir with both `go.mod` and `package.json`+`tsconfig.json` resolves to
  `go`), the `langOverride` path, unknown override → error, and no-marker → `ErrNoLanguage` (assert via
  `errors.Is`). All untagged, offline, spawn-free (no `exec.Command`, no `gitexec`, no `lyxtest.Copy`).
- **Commit:** `test(codeintelengine): unit tests for registry, overlay, and detection`

### Card 7: leaf-invariant enforcement test + CONSTRAINTS entry

- **Context:**
  - `internal/modelspec/leaf_enforcement_test.go`
  - `internal/tokenvocab/leaf_enforcement_test.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `CONSTRAINTS.md`
- **Creates:**
  - `internal/codeintelengine/leaf_enforcement_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** `leaf_enforcement_test.go` (`TestLeafInvariant_AllowlistOnly`, mirroring
  `modelspec`'s): parse every non-test `.go` file under `internal/codeintelengine`, collect imports,
  and fail if any import falls outside the allowlist — stdlib, `github.com/Knatte18/loomyard/internal/hubgeometry`,
  `gopkg.in/yaml.v3`. Explicitly assert `internal/output`, cobra, and any `internal/*cli` are NOT
  imported. Note: this test runs untagged but batch 2 adds subprocess code to the same package — the
  allowlist must still hold (the LSP client uses only stdlib `os/exec`, `bufio`, `encoding/json`,
  etc.), so keep the allowlist stdlib-inclusive. In `CONSTRAINTS.md`, add a `## Codeintelengine Leaf
  Invariant` section (styled like the `Modelspec Leaf Invariant`) stating the allowlist and that
  `codeintelcli` → `codeintelengine` is the only allowed direction, enforced by this test.
- **Commit:** `test(codeintelengine): leaf-invariant guard + CONSTRAINTS entry`

## Batch Tests

`verify: go test ./internal/codeintelengine/...` runs Cards 6 and 7's untagged tests — registry
built-ins/validation, overlay load semantics, detection precedence/AND-OR, and the leaf-import guard.
Scope is the single new package, so the full-package run is cheap and correct. No integration-tagged
tests exist yet (batch 2 adds them); the untagged run stays offline and spawn-free per the Test Tier
Purity Invariant.
