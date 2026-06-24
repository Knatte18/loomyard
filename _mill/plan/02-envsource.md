# Batch: envsource

```yaml
task: "Extract yamlengine and migrate config via lyx update"
batch: envsource
number: 2
cards: 1
verify: go test ./internal/envsource/...
depends-on: [3]
```

## Batch Scope

Deliver `internal/envsource`: the single place that decides HOW env vars enter the
system. It reads a repo-local `.env` and overlays the OS environment (OS wins),
returning a plain `map[string]string` that `internal/config.Load` feeds to the pure
`yamlengine.Resolve`. This isolates env-sourcing policy from the engine. Depends on
batch 3 only for `paths.DotEnv` (the centralized `.env` path helper).

## Cards

### Card 3: envsource.Build (.env + OS overlay, eager)

- **Context:**
  - `go.mod`
  - `internal/config/config.go`
  - `internal/paths/paths.go`
- **Edits:** none
- **Creates:**
  - `internal/envsource/envsource.go`
  - `internal/envsource/envsource_test.go`
- **Deletes:** none
- **Requirements:**
  - Create package `envsource`. Add `func Build(baseDir string) (map[string]string, error)`: read the file at `paths.DotEnv(baseDir)` into a map (absent file → empty map, not an error), then overlay the OS environment via `os.Environ()` so OS values win over `.env` values. Return the merged map. The read is eager (a single pass; no lazy per-key lookup).
  - Port the `.env` parsing semantics from the existing `loadDotEnv` in `internal/config/config.go`: skip blank lines and lines beginning with `#`; split on the FIRST `=` only (so `=` may appear in the value); lines without `=` are skipped. Do not trim values.
  - For the OS overlay: iterate `os.Environ()`, split each entry on the first `=`, and set into the map (overwriting any `.env` entry with the same key).
  - Godoc every exported symbol per the golang-comments skill.
  - envsource_test.go (use `t.TempDir` for the `.env` file and `t.Setenv` for OS vars): `.env` parsing (comment lines, blank lines, no-`=` lines, `=` inside a value); absent `.env` returns a map that still contains OS vars; OS overlay precedence (a key set in BOTH `.env` and OS resolves to the OS value); a key only in `.env` survives; a key only in OS is present. Assert against keys you control via `t.Setenv` to avoid depending on ambient environment.
- **Commit:** `feat(envsource): add Build (.env + OS overlay, eager)`

## Batch Tests

`verify: go test ./internal/envsource/...` runs the new package's tests, which use
`t.TempDir`/`t.Setenv` for isolation. Scope is the single new package.
