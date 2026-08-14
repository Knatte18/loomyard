# Batch: stencilstore-foundation

```yaml
task: "Relocate producer prompt files into a stencils/ directory"
batch: "stencilstore-foundation"
number: 1
cards: 6
verify: go test ./internal/stencil/... ./internal/stencilstore/... ./internal/fabricengine/...
depends-on: []
```

## Batch Scope

This batch builds the whole stencil lifecycle mechanism with **no file moves and no engine changes**, so every later batch has a finished, tested kernel to call into.
It delivers three things: the two new exported helpers in `internal/stencil`, the new `internal/stencilstore` package (hashing, the five-row edit-detection table, `Read`, `Reconcile`, `Validate`), and the `fabricengine.StencilsDir` resolver.
The external interface later batches consume is `stencilstore.Read(baseDir, name)`, `stencilstore.Reconcile(baseDir, registry, mode, sourceDir)`, and `fabricengine.StencilsDir(hub)`.

`internal/stencilstore` takes an explicit absolute `baseDir` and never joins `_board` or `_lyx` itself, so its tests run against a bare `t.TempDir()` with no git and no hub — the package stays Tier 1 pure and untagged.
The `Registry` type is declared here and passed in as a `Reconcile` argument, never imported, so the top-level `stencils` package (batch 2) depends on `stencilstore` and not the reverse.

## Cards

### Card 1: Export the leading-comment stripper and the top-level-marker lister from `internal/stencil`

- **Context:**
  - `internal/tokenvocab/render.go`
- **Edits:**
  - `internal/stencil/stencil.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add two exported functions to `package stencil`, both thin wrappers that keep the existing unexported implementations as the single logic site:
  - `func StripLeadingComment(text string) string` — returns `stripLeadingComment(text)`. Doc comment must state that it drops a leading `<!-- ... -->` block and returns `text` unchanged when there is none, and that it is the same stripper `Fill`/`FillOptional` apply before parsing.
  - `func TopLevelMarkers(template []byte) ([]string, error)` — parses `StripLeadingComment(string(template))` with `tmpl.New("stencil").Option("missingkey=error").Parse(...)`, returns the deduplicated, `sort.Strings`-sorted names of every top-level `{{.X}}` marker in the template, and returns the parse error wrapped as `fmt.Errorf("parse template: %w", err)` on failure.
    Implement it by extracting the AST-walking half of `unfilledTopLevelMarkers` into a new unexported helper `topLevelMarkerNames(t *tmpl.Template) []string` that returns every top-level marker name it finds, and rewrite `unfilledTopLevelMarkers` to call that helper and then apply its existing `optional` / non-empty-`values` filtering.
    `TopLevelMarkers` lists every top-level marker regardless of whether a value would fill it — it is the marker-set accessor `lyx stencil validate` compares two templates with, not a validity check.

  Do not change the behaviour, signature, or error text of `Fill` or `FillOptional`.

- **Commit:** `feat(stencil): export StripLeadingComment and TopLevelMarkers`

### Card 2: Unit-test the two new `internal/stencil` exports

- **Context:**
  - `internal/stencil/stencil.go`
- **Edits:** none
- **Creates:**
  - `internal/stencil/export_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  New file in `package stencil`, untagged (Tier 1, no git, no spawns).
  Cover:
  - `StripLeadingComment` drops a leading `<!-- ... -->` banner, returns text unchanged when the text does not open with one, and returns text unchanged when the block is unterminated.
  - `TopLevelMarkers` returns the sorted marker names of a template carrying `{{.a}}`, `{{.b}}`, and a repeated `{{.a}}`, with the duplicate collapsed.
  - `TopLevelMarkers` ignores markers inside the leading banner comment (it strips first, then parses).
  - `TopLevelMarkers` returns an error for an unparseable template.
  - A regression assertion that `Fill` still reports its existing unfilled-marker error for a template whose marker has no value, so the `unfilledTopLevelMarkers` refactor in card 1 is pinned.
- **Commit:** `test(stencil): cover StripLeadingComment and TopLevelMarkers`

### Card 3: Create `internal/stencilstore` — hashing, stamp parsing, and the edit-detection table

- **Context:**
  - `internal/stencil/stencil.go`
  - `internal/logger/logger.go`
- **Edits:** none
- **Creates:**
  - `internal/stencilstore/doc.go`
  - `internal/stencilstore/stencilstore.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  New package `stencilstore`, importing only stdlib plus `internal/stencil` and `internal/logger`.
  Do not import `internal/fabricengine`, `internal/lyxcwd`, or `internal/lyxdirs` from this package — `baseDir` always arrives from the caller fully resolved.

  `doc.go` carries the package doc: `stencilstore` owns the entire stencil lifecycle (seed, hash-stamp, edit detection, read, validate) against a caller-supplied absolute stencils directory, and never derives geometry.

  `stencilstore.go` declares:

  - `const StampPrefix = "<!-- lyx-stencil: sha256="` and `const StampSuffix = " -->"`, plus the assembled stamp line written into a file's leading banner.
  - `func NormalizeLF(b []byte) []byte` — replaces every `\r\n` with `\n` and every remaining lone `\r` with `\n`. Every hash and every comparison in this package routes through it.
  - `func BodyHash(content []byte) string` — returns the lowercase hex `sha256` of `NormalizeLF([]byte(stencil.StripLeadingComment(string(content))))`. The hash therefore covers exactly the text `stencil.Fill` parses, and never covers the stamp that stores it.
  - `func ParseStamp(content []byte) (string, bool)` — extracts the `sha256=<hex>` value from the file's leading `<!-- ... -->` banner. Returns `("", false)` when the file has no leading banner, when the banner carries no `lyx-stencil:` line, or when the hex value is not 64 lowercase hex characters.
  - `func ApplyStamp(content []byte, hash string) []byte` — returns `content` with the stamp line present in its leading banner set to `hash`. When the file already has a leading `<!-- ... -->` banner, replace an existing `lyx-stencil:` line in place, or insert the stamp line as the banner's last line when absent. When the file has no leading banner, prepend a new one-line banner `<!-- lyx-stencil: sha256=<hash> -->` followed by a blank line. `ApplyStamp` must never alter the body, so `BodyHash(ApplyStamp(c, h)) == BodyHash(c)` always holds.

  - `type Mode int` with `const ( ModeProduction Mode = iota; ModeDev )`. `ModeProduction` is the zero value, so an unstamped binary and a `Mode{}` default both classify as production.
  - `type Registry interface { Names() []string; Default(name string) ([]byte, bool) }` — the name-to-shipped-default lookup, supplied by the caller. `Names()` returns the registry's stencil names in a stable order.
  - `func RelPath(name string) string` — returns `<family>/<name>.md`, where `<family>` is the substring of `name` up to its first `-`. Returns `name + ".md"` (no family directory) when `name` contains no `-`.
  - `func Path(baseDir, name string) string` — `filepath.Join(baseDir, filepath.FromSlash(RelPath(name)))`.

  - `type State int` with members `StateAbsent`, `StateUntouched`, `StateReconciled`, `StateEdited` and a `String()` method, plus:
    `func Classify(onDisk []byte, exists bool, shipped []byte) State` implementing the five-row table, evaluated strictly in this order:
    1. `!exists` → `StateAbsent`.
    2. stamp present and `BodyHash(onDisk) == stamp` → `StateUntouched`.
    3. `BodyHash(onDisk) == BodyHash(shipped)` → `StateReconciled` (the file's body has caught up with the shipped default, whatever the stamp said; it restamps and is treated as untouched from now on).
    4. stamp present and both comparisons failed → `StateEdited`.
    5. stamp missing or unparseable → `StateEdited`.

- **Commit:** `feat(stencilstore): add hashing, stamp parsing, and edit-detection classification`

### Card 4: Add `Read`, `Reconcile`, and `Validate` to `internal/stencilstore`

- **Context:**
  - `internal/stencil/stencil.go`
  - `internal/logger/logger.go`
- **Edits:**
  - `internal/stencilstore/stencilstore.go`
- **Creates:**
  - `internal/stencilstore/reconcile.go`
  - `internal/stencilstore/validate.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  `reconcile.go` declares:

  - `func Read(baseDir, name string) ([]byte, error)` — a pure `os.ReadFile(Path(baseDir, name))` on every call, with no caching, so an edit takes effect immediately. On failure wrap as `fmt.Errorf("stencilstore: read stencil %q from %s: %w", name, baseDir, err)`, so the error names both the stencil and the directory that was missing. `Read` never falls back to a shipped default and never writes.

  - `func Reconcile(baseDir string, registry Registry, mode Mode, sourceDir string) ([]string, error)` — the once-per-process seed/refresh pass. It returns the repo-relative-to-`baseDir` slash-separated paths it actually wrote, in `registry.Names()` order, and writes nothing at all when every file is already correct.
    For each name in `registry.Names()`, read the on-disk file at `Path(baseDir, name)`, `Classify` it against the registry default, and act:
    - `StateAbsent` → write `ApplyStamp(shipped, BodyHash(shipped))`, creating parent directories with `os.MkdirAll(..., 0o755)`. Record the path.
    - `StateUntouched` → when `mode == ModeDev`, write nothing; if the shipped default's body hash differs from the on-disk body hash, emit one `logger.Warn` naming the stencil and stating that a dev build does not refresh an untouched stencil. When `mode == ModeProduction` and the shipped body differs, overwrite with `ApplyStamp(shipped, BodyHash(shipped))` and record the path; when it does not differ, write nothing.
    - `StateReconciled` → rewrite the stamp only, via `ApplyStamp(onDisk, BodyHash(onDisk))`, and record the path when the bytes actually changed. This row runs in **both** modes: it only touches the banner line the hash excludes by construction, and it fires only when the body already equals that binary's own shipped default, so two binaries can never restamp the same file in opposite directions.
    - `StateEdited` → never write. When the shipped default's body hash differs from the on-disk body hash, emit one `logger.Warn` naming the stencil and stating that an edited stencil has fallen behind a newer shipped default and that `lyx stencil diff <name>` shows the change.

    After the per-stencil loop, when `sourceDir != ""`, run the port-back drift comparison: for each name, compare `NormalizeLF(stencil.StripLeadingComment(...))` of the on-disk board copy against the same normalisation of `filepath.Join(sourceDir, filepath.FromSlash(RelPath(name)))`, and emit one `logger.Warn` per differing stencil naming it and pointing at `lyx stencil promote <name>`. A missing source file is skipped silently, and `sourceDir == ""` skips the whole comparison silently — that is what keeps the warning quiet in a consumer repo with no `stencils/` tree. This comparison never returns an error and never affects an exit code.

    `Reconcile` also seeds `<baseDir>/.gitattributes` with the single line `*.md text eol=lf` when that file is absent, and never rewrites it when present.
    The seeded `.gitattributes` is never stamped, never in the registry, and invisible to `Names()`, but its path IS included in the returned written-paths slice when it was created, so the caller's commit pathspec covers it.

  - `func ForceRefresh(baseDir string, registry Registry, sourceDir string) ([]string, error)` — `Reconcile(baseDir, registry, ModeProduction, sourceDir)`. This is the entry point `lyx stencil sync` calls, which is why an explicit `sync` performs the refresh row even from a `-dev`-stamped binary.

  `validate.go` declares:

  - `type Finding struct { Name string; Marker string; Severity string }` with `Severity` one of the exported constants `SeverityError` and `SeverityWarning`.
  - `func Validate(baseDir string, registry Registry) ([]Finding, error)` — for each registry name, read the on-disk body and the shipped default, take both marker sets via `stencil.TopLevelMarkers`, and report: a marker present in the on-disk body but absent from the shipped default as `SeverityError` (it will break `stencil.Fill` at the point of use), and a shipped-default marker absent from the on-disk body as `SeverityWarning` (legal customisation that silently drops that content). Findings are sorted by name then marker. A stencil missing from disk is an error return, not a finding.

- **Commit:** `feat(stencilstore): add Read, Reconcile, and Validate`

### Card 5: Unit-test `internal/stencilstore` against a bare `t.TempDir()`

- **Context:**
  - `internal/stencilstore/stencilstore.go`
  - `internal/stencilstore/reconcile.go`
  - `internal/stencilstore/validate.go`
- **Edits:** none
- **Creates:**
  - `internal/stencilstore/stencilstore_test.go`
  - `internal/stencilstore/reconcile_test.go`
  - `internal/stencilstore/validate_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  All three files are `package stencilstore`, untagged, and use only `t.TempDir()` — no `git`, no hub fixture, no `exec.Command`, no `hubforge`, so the package stays inside the Test Tier Purity Invariant.
  Declare a `fakeRegistry` test helper implementing `Registry` over a `map[string][]byte`, so no test depends on the real `stencils` package.

  `stencilstore_test.go` covers `NormalizeLF`, `BodyHash`, `ParseStamp`, `ApplyStamp`, and `Classify`:
  - a body written with CRLF line endings hashes identically to the same body with LF
  - `BodyHash` ignores every change confined to the leading banner comment and reacts to any change below it
  - `ApplyStamp` on a file with a banner replaces the stamp line in place; on a file with no banner it prepends a new one-line banner; in both cases `BodyHash` is unchanged
  - `ParseStamp` returns `false` for a missing banner, a banner with no `lyx-stencil:` line, and a malformed hex value
  - `Classify` returns each of the five states for the corresponding input, including the ordering assertion that a body equal to the shipped default classifies `StateReconciled` even when its stamp names a different hash

  `reconcile_test.go` covers:
  - an absent file is seeded with the default and carries a stamp equal to `BodyHash` of that default
  - an untouched file whose default is unchanged is left byte-identical and its path is not in the returned slice
  - an untouched file whose default changed is overwritten and restamped in `ModeProduction`
  - the same file in `ModeDev` is left byte-identical on disk, and the returned slice is empty
  - `ModeDev` still performs the reconciliation row: a file whose body equals the dev default but whose stamp names an older hash is restamped and thereafter classifies `StateUntouched`
  - `ForceRefresh` performs the refresh row on a file that `ModeDev` would have skipped
  - an edited file (`BodyHash != stamp` and `!= BodyHash(shipped)`) is never modified, and `Read` returns its edited content
  - a file with a missing or malformed stamp is treated as edited and left alone
  - a file edited then reverted to the exact default body is restamped and returns to the untouched state
  - a run whose defaults are all unchanged returns an empty slice and writes nothing (assert by comparing every file's modification-time-independent bytes before and after)
  - `.gitattributes` is created with `*.md text eol=lf` when absent, is listed in the returned slice on that run, is never rewritten when present, and never appears in `Names()`
  - `sourceDir == ""` skips the drift comparison and returns no error
  - a `sourceDir` whose file differs from the board copy still returns successfully — the drift signal is a `logger.Warn`, never an error and never a non-zero result

  `validate_test.go` covers a stencil whose body adds a top-level marker unknown to its shipped default (reported `SeverityError`, naming the offending marker) and one that deletes a default marker (reported `SeverityWarning`).

- **Commit:** `test(stencilstore): cover the edit-detection table, dev/prod modes, and validate`

### Card 6: Add `fabricengine.StencilsDir` beside `BoardDir`

- **Context:**
  - `internal/lyxdirs/dirs.go`
  - `internal/configengine/config.go`
- **Edits:**
  - `internal/fabricengine/junctionnames.go`
- **Creates:**
  - `internal/fabricengine/stencilsdir_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add to `package fabricengine`, immediately after the existing `BoardDir` function in `junctionnames.go`:

  - `const stencilsDirName = "stencils"` — unexported; `stencils` is not a policed geometry token, so it needs no `geometryTokenOwners` row.
  - `func StencilsDir(hub string) string` returning `filepath.Join(BoardDir(hub), lyxdirs.LyxDirName, stencilsDirName)`, i.e. `<hub>/_board/_lyx/stencils`.

  `fabricengine` is the right owner because it already declares `BoardDirName`, and the `_lyx` component must come from `lyxdirs.LyxDirName` rather than a literal or `TestEnforcement_GeometryLiterals` fails.
  Add the `internal/lyxdirs` import to `junctionnames.go` if it is not already present; `lyxdirs` is a stdlib-only zero-import leaf, so it cannot create a cycle.
  The doc comment must state that `StencilsDir` returns the hub-wide stencils directory shared by every worktree in the hub, and that `internal/stencilstore` receives this value as its `baseDir` and never joins `_board` or `_lyx` itself.

  `stencilsdir_test.go` is `package fabricengine`, untagged, with no git spawn: assert `StencilsDir("/h")` equals `filepath.Join("/h", "_board", "_lyx", "stencils")` and that it is a child of `BoardDir("/h")`.

- **Commit:** `feat(fabricengine): add StencilsDir resolver beside BoardDir`

## Batch Tests

`verify: go test ./internal/stencil/... ./internal/stencilstore/... ./internal/fabricengine/...` runs exactly the three packages this batch creates or changes.
Every test in this batch is untagged and hermetic: `t.TempDir()` only, no git spawn, no hub fixture, so the Test Tier Purity Invariant and the Hermetic Git Test Environment Invariant both hold without a `TestMain`.
The scope is deliberately narrow — no other package compiles against these two yet, so a repo-wide run would add minutes and catch nothing this batch can break.
