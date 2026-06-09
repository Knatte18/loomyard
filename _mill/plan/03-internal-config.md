# Batch: internal/config — generic config loader

```yaml
task: "Extract shared infrastructure (config, git, lock)"
batch: "internal/config — generic config loader"
number: 3
cards: 2
verify: go test ./internal/config/...
depends-on: []
```

## Batch Scope

This batch creates the `internal/config` package with a generic two-layer config loader. `Load(baseDir, module string, defaults map[string]string) (map[string]string, error)` merges defaults with `<baseDir>/_mhgo/<module>.yaml`, loads `<baseDir>/.env` into a local map (no `os.Setenv`), and expands `$env:NAME` and `$env:NAME ? fallback` tokens using a three-phase positional algorithm that guarantees fallback strings are never re-scanned. No board files are touched here. Batch 4 will import this package and rewrite board's `LoadConfig` as a thin wrapper.

Batch-local decisions (differ from shared decisions):
- The `$env:NAME ? fallback` regex (`envOptRe`) uses `(.*)$` to capture the fallback as the remainder of the value; this is safe because YAML scalars are always single-line after the YAML library processes them.
- Required expansion (phase 2) operates on the prefix span only (`value[:matchStart]`), not the full string. This prevents `$env:` tokens inside a fallback from being re-expanded.
- OS env takes precedence over `.env` — `os.LookupEnv` is checked first, then the local dotenv map.

## Cards

### Card 7: Create internal/config/config.go

- **Context:**
  - `internal/board/config.go`
- **Edits:** none
- **Creates:**
  - `internal/config/config.go`
- **Deletes:** none
- **Requirements:** Create `internal/config/config.go` with `package config`. Implement the following:

  **Package-level vars:**
  ```go
  var envOptRe = regexp.MustCompile(`\$env:([A-Za-z_][A-Za-z0-9_]*)\s*\?\s*(.*)$`)
  var envReqRe = regexp.MustCompile(`\$env:([A-Za-z_][A-Za-z0-9_]*)`)
  ```

  **`Load(baseDir, module string, defaults map[string]string) (map[string]string, error)`:**
  1. Stat `filepath.Join(baseDir, "_mhgo")`. If the directory does not exist (`os.IsNotExist(err)`), return `nil, fmt.Errorf("not initialized: _mhgo/ directory not found in %s", baseDir)`.
  2. Call `dotenv, err := loadDotEnv(filepath.Join(baseDir, ".env"))`. Return on error.
  3. Start with `result := make(map[string]string, len(defaults))`, copy all defaults into it.
  4. Load `filepath.Join(baseDir, "_mhgo", module+".yaml")` via `loadYAMLLayer`. If the file is present, merge returned keys over `result`. Return on non-nil errors.
  5. For each key in `result`, call `expandEnv(v, dotenv)`. If any expansion returns an error, return `nil, fmt.Errorf("config key %q: %w", key, err)`. Update `result[key]` with the expanded value.
  6. Return `result, nil`.

  **`loadDotEnv(path string) (map[string]string, error)`:**
  - Open file. If `os.IsNotExist(err)` return empty map, nil. Return other errors.
  - Scan line by line. For each line: trim whitespace; skip if empty or starts with `#`; find first `=`; if no `=` skip; `key = line[:idx]`, `val = line[idx+1:]`; store in map.
  - Return map.

  **`loadYAMLLayer(path string) (map[string]string, error)`:**
  - Read file. If `os.IsNotExist(err)` return empty map, nil. Return other errors.
  - Unmarshal via `yaml.Unmarshal` into `map[string]string`. Return unmarshal error if any.
  - Return map.

  **`expandEnv(value string, dotenv map[string]string) (string, error)`:**
  Three-phase positional algorithm:
  1. Apply `envOptRe.FindStringSubmatchIndex(value)`. If match found, `matchStart = loc[0]`; capture `name = value[loc[2]:loc[3]]`, `fallback = value[loc[4]:loc[5]]`. Prefix span is `value[:matchStart]`.
  2. Expand required tokens in prefix span only: apply `envReqRe.ReplaceAllStringFunc(prefix, func(tok string) string {...})`. Lookup each name via `os.LookupEnv` first, then dotenv map. If unset, set an outer error variable to `fmt.Errorf("unset required env var %q", name)` and return the token unchanged. Check the error after `ReplaceAllStringFunc` returns.
  3. If step 1 found a match: look up `name` (OS first, then dotenv). If set, return `expandedPrefix + envVal, nil`. If unset, return `expandedPrefix + strings.TrimSpace(fallback), nil`. If no match: return the step-2 result.

  Imports: `bufio`, `fmt`, `os`, `path/filepath`, `regexp`, `strings`, `gopkg.in/yaml.v3`.
- **Commit:** `feat(config): add internal/config package`

### Card 8: Create internal/config/config_test.go

- **Context:**
  - `internal/board/config_test.go`
- **Edits:** none
- **Creates:**
  - `internal/config/config_test.go`
- **Deletes:** none
- **Requirements:** Create `internal/config/config_test.go` with `package config_test`. Implement all 15 tests listed below. Each test that needs a filesystem creates a temp dir with `t.TempDir()` and sets up `_mhgo/` as needed. Use `t.Setenv` for env var manipulation (auto-restores after test). Choose env var names unlikely to be set in CI (e.g. `TEST_MHGO_EXTRACT_CONFIG_XYZ`).

  - `TestLoad_UninitializedDir`: call `config.Load` with a fresh temp dir (no `_mhgo/` subdirectory). Assert error is non-nil and contains "not initialized".
  - `TestLoad_Defaults`: create `<tmpDir>/_mhgo/` (mkdir), do NOT create `<tmpDir>/_mhgo/board.yaml`. Call `config.Load(tmpDir, "board", map[string]string{"path": "_board"})`. Assert no error and returned map has `"path": "_board"`.
  - `TestLoad_YAMLOverride`: create `<tmpDir>/_mhgo/board.yaml` with `path: custom_path`. Call `config.Load` with defaults `{"path": "default_path", "home": "Home.md"}`. Assert `path == "custom_path"` and `home == "Home.md"`.
  - `TestLoad_DotMhgoIgnored`: create `<tmpDir>/_mhgo/board.yaml` with `path: correct` and `<tmpDir>/.mhgo/board.yaml` with `path: wrong`. Call `config.Load`. Assert `path == "correct"` (the `.mhgo/` file is silently ignored).
  - `TestLoad_EnvRequired_Set`: write YAML `path: $env:TEST_EXTRACT_REQ_VAR`. `t.Setenv("TEST_EXTRACT_REQ_VAR", "expanded")`. Assert no error and `path == "expanded"`.
  - `TestLoad_EnvRequired_Unset`: write YAML `path: $env:TEST_EXTRACT_MISSING_VAR_XYZ123`. Ensure env var is unset. Assert error is non-nil and contains `"TEST_EXTRACT_MISSING_VAR_XYZ123"`.
  - `TestLoad_EnvOptional_Set`: write YAML `path: $env:TEST_EXTRACT_OPT_VAR ? fallback`. `t.Setenv("TEST_EXTRACT_OPT_VAR", "set_value")`. Assert `path == "set_value"`.
  - `TestLoad_EnvOptional_Unset`: write YAML `path: $env:TEST_EXTRACT_OPT_ABSENT ? my_fallback`. Ensure var is unset. Assert `path == "my_fallback"`.
  - `TestLoad_EnvOptional_WithPrefix`: write YAML `path: prefix/$env:TEST_EXTRACT_PREFIX_VAR ? default_name`. Ensure var is unset. Assert `path == "prefix/default_name"`.
  - `TestLoad_DotEnv_FillsUnset`: create `<tmpDir>/.env` with line `TEST_EXTRACT_DOTENV_KEY=from_dotenv`. Write YAML `path: $env:TEST_EXTRACT_DOTENV_KEY`. Ensure OS env var is unset. Assert `path == "from_dotenv"`.
  - `TestLoad_DotEnv_OSEnvWins`: create `<tmpDir>/.env` with `TEST_EXTRACT_OS_WINS=dotenv_val`. `t.Setenv("TEST_EXTRACT_OS_WINS", "os_val")`. Write YAML `path: $env:TEST_EXTRACT_OS_WINS`. Assert `path == "os_val"`.
  - `TestLoad_DotEnv_MalformedLine`: create `<tmpDir>/.env` with lines `GOOD_KEY=val` and `MALFORMED_NO_EQUALS`. Call `config.Load` without referencing either. Assert no error (malformed line silently skipped).
  - `TestLoad_DotEnv_Comment`: create `<tmpDir>/.env` with `# this is a comment` and `REAL_KEY=real_val`. Assert no error.
  - `TestLoad_DotEnv_Absent`: create `_mhgo/` but no `.env` file. Assert no error.
  - `TestLoad_LiteralQuestionMark`: write YAML `url: "http://host?q=1"`. Assert no error and `url == "http://host?q=1"` (literal `?` not preceded by `$env:NAME` is treated as a literal character).
- **Commit:** `test(config): add internal/config package tests`

## Batch Tests

`verify: go test ./internal/config/...` runs all 15 tests in `config_test.go`. Tests cover: uninitialized dir, defaults passthrough, YAML override, `.mhgo/` ignored (regression guard), required env (set and unset), optional env (set, unset, with prefix), `.env` loading (fills unset, OS wins, malformed line, comment, absent), and literal `?` character. Board is not touched in this batch.
