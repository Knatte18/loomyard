# Batch: configengine-loadortemplate

```yaml
task: "config degrades to embedded template"
batch: "configengine-loadortemplate"
number: 1
cards: 3
verify: go test ./internal/configengine/...
depends-on: []
```

## Batch Scope

This batch delivers the whole new `internal/configengine` surface and nothing else: the exported `ErrNotInitialized` sentinel, the shared unexported `load` body both exported entry points route through, the new `LoadOrTemplate` function, the rewritten file-header comment, and the tests that pin all of it.
It is one batch because all three cards edit the same two files in the same package and share one mental model — the six-step flow and where its two fallback branches attach.

**External interface batch 2 consumes:** `configengine.LoadOrTemplate(baseDir, module string, template []byte) ([]byte, error)` — same three parameters and same return shape as `configengine.Load`, so a caller repoints by changing the function name alone.
Batch 3 consumes the same signature plus the exported `ErrNotInitialized` sentinel for its documentation pass.

Batch-local decision beyond `## Shared Decisions`: the fallback tail is deliberately **not** shared with the on-disk path's validation and error-wrapping steps.
It skips `yamlengine.MissingKeys` (a template compared against itself is vacuously satisfied) and wraps its own failures as `%s config template: %w` keyed on `module` rather than with the on-disk path's `config file %s:` prose (no such file exists on that path, so naming one would send an operator hunting a file that was never there).
What both paths share is the on-disk validation flow itself and the env-marker resolution pair `envsource.Build` then `yamlengine.Resolve`, in that order, so a template default and an on-disk value expand by identical rules.

## Cards

### Card 1: `ErrNotInitialized` sentinel wrapped by `FindBaseDir`

- **Context:**
  - `internal/lyxdirs/dirs.go`
- **Edits:**
  - `internal/configengine/config.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add an exported sentinel `ErrNotInitialized` to `internal/configengine`, declared as `errors.New("not initialized")`, with a godoc comment starting `// ErrNotInitialized` explaining that it marks a provably-absent `_lyx/` directory and that callers wanting to distinguish absence from a stat failure should use `errors.Is` rather than matching error text.
  Add `"errors"` to the file's import set.
  Change `FindBaseDir`'s `os.IsNotExist(err)` branch so its returned error wraps the sentinel with `%w`, while emitting a message byte-identical to today's.
  Today that branch reads:

```go
	if os.IsNotExist(err) {
		return "", fmt.Errorf("not initialized: %s/ directory not found", lyxdirs.LyxDirName)
	}
```

  It becomes a wrap whose rendered text is still exactly `not initialized: _lyx/ directory not found` — for example `fmt.Errorf("%w: %s/ directory not found", ErrNotInitialized, lyxdirs.LyxDirName)`, which renders the sentinel's own `"not initialized"` text in the leading position.
  The message must stay byte-identical because four strict callers outside this task match it with `strings.Contains(err.Error(), "not initialized")`.
  `FindBaseDir`'s other error branch — the non-`IsNotExist` stat failure returning `stat %s: %w` — stays exactly as written and must never wrap `ErrNotInitialized`.
  Extend `FindBaseDir`'s doc comment to state that an absent `_lyx/` yields an error satisfying `errors.Is(err, ErrNotInitialized)` and that a stat failure does not.
  Do not change the `configDirName` constant, `ConfigDir`, `ConfigFile`, or `Load` in this card.
- **Commit:** `feat(configengine): add ErrNotInitialized sentinel wrapped by FindBaseDir`

### Card 2: shared `load` body and the `LoadOrTemplate` entry point

- **Context:**
  - `CONSTRAINTS.md`
  - `internal/envsource/envsource.go`
  - `internal/logger/logger.go`
  - `internal/yamlengine/reconcile.go`
  - `internal/yamlengine/resolve.go`
- **Edits:**
  - `internal/configengine/config.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Extract the body of `Load` into a shared unexported function `load(baseDir, module string, template []byte, fallbackOnAbsent bool) ([]byte, error)` in the same file.
  Redefine `Load(baseDir, module string, template []byte) ([]byte, error)` as a one-line delegation to `load(baseDir, module, template, false)`, keeping its exported signature and its doc comment's promise of strict behaviour intact.
  Add `LoadOrTemplate(baseDir, module string, template []byte) ([]byte, error)` delegating to `load(baseDir, module, template, true)`, with a godoc comment starting `// LoadOrTemplate` that states it behaves identically to `Load` except that a provably-absent `_lyx/` directory or a provably-absent config file resolves the caller-supplied template instead of erroring, that a config file which exists but is invalid still errors, and that any non-absence failure propagates unchanged.

  `load`'s body consults `fallbackOnAbsent` at exactly two points, and at each one only on proven absence:

  1. **The `FindBaseDir` branch.** When `FindBaseDir(baseDir)` returns an error, fall through to the fallback tail only when `fallbackOnAbsent` is true **and** `errors.Is(err, ErrNotInitialized)` holds.
     Otherwise return the error unchanged, exactly as today.
  2. **The config-file read branch.** When `os.ReadFile(cfgPath)` fails with `os.IsNotExist(err)`, fall through to the fallback tail only when `fallbackOnAbsent` is true;
     otherwise return today's `config file %s not found; run "lyx config reconcile"` error unchanged.
     A read failure that is not `IsNotExist` keeps returning today's `read config file %s: %w` on both paths.

  Both fallback branches converge on **one shared fallback tail**, implemented once, which:

  - Emits `logger.Info` naming the module and which of the two conditions fired — an absent `_lyx/` directory versus an absent config file — so the two cases are distinguishable in the durable trace.
    This adds `"github.com/Knatte18/loomyard/internal/logger"` to the file's import set.
  - Skips `yamlengine.MissingKeys` entirely.
  - Calls `envsource.Build(baseDir)` with the same `baseDir` the caller passed, never a different directory and never skipped.
  - Calls `yamlengine.Resolve(template, env)` on the template bytes and returns the resolved result.
  - Wraps any failure of its own two steps as `%s config template: %w` keyed on `module`.
    Do not interpolate `ConfigFile(baseDir, module)` into a fallback-path error message.

  The on-disk path — config file present — stays byte-for-byte today's `Load`: `yamlengine.MissingKeys`, the comma-joined `missing keys:` message, `envsource.Build`, `yamlengine.Resolve`, all keeping the existing `config file %s:` error wording, reached identically whichever exported function the caller invoked.

  Rewrite the file-header comment at the top of `internal/configengine/config.go`.
  It currently opens "config.go implements strict YAML configuration loading backed by yamlengine and envsource" and names three wrappers — `board.LoadConfig`, `worktree.LoadConfig`, `fabric.LoadConfig` — none of which exists.
  The replacement states that the file implements two loading policies over one shared body: `Load`, which is strict and used by hub-scoped modules where an absent config means a broken hub, and `LoadOrTemplate`, which resolves the caller's embedded template on proven absence and is used by modules with a standalone entry point.
  Name the real caller sets rather than inventing wrappers, and point at `CONSTRAINTS.md`'s Config Strictness Invariant as the rule that decides which policy a new caller adopts.
  The file must not gain a `"strings"` import, and the sentinel added in card 1 must not be renamed.
- **Commit:** `feat(configengine): add LoadOrTemplate degrading onto the embedded template`

### Card 3: `configengine` unit tests for the degrading path

- **Context:**
  - `internal/configengine/config.go`
  - `internal/envsource/envsource.go`
  - `internal/lyxdirs/dirs.go`
  - `internal/yamlengine/resolve.go`
- **Edits:**
  - `internal/configengine/config_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `LoadOrTemplate` tests to the existing external test package `configengine_test`, reusing its established fixture style — a bare `t.TempDir()` plus, where a config file is wanted, the same `os.Mkdir`/`os.WriteFile` shape `TestLoad_HappyPath` uses.
  Every existing `TestLoad_*` and `TestFindBaseDir_*` function stays byte-identical;
  they are the regression proof that the shared-body refactor preserved `Load`'s strict behaviour.
  Add coverage for these scenarios, one test function each, named for what they assert:

  - Absent `_lyx/` returns template-derived bytes and a nil error.
  - `_lyx/` present but the module's config file absent returns template-derived bytes and a nil error.
  - `_lyx/` and config file both present returns a result identical to `configengine.Load` on the same inputs, proving the fallback never shadows a real file.
  - Config file present but missing a template key still returns an error naming the missing key — the strict-when-present boundary, and the single most important negative assertion in this batch.
  - Config file present but empty, and separately present but comments-only, still returns an error.
  - The fallback path honours an env override: set a variable referenced by an `${env:NAME:-default}` marker in the template via `t.Setenv`, and assert the override lands in the returned bytes with no `_lyx/` anywhere on disk.
  - The fallback path with an absent `.env` and an absent `baseDir` returns no error.
  - The fallback path's own error wrap is exercised: with no `_lyx/` on disk, pass a synthetic template containing a required `${env:NAME}` marker whose variable is unset, and assert the returned error is non-nil and its message contains `config template:` and the module name, and that it does not contain `config file`.
    This pins the `%s config template: %w` wrap keyed on `module` and pins that a fallback-path error never names a config-file path that does not exist.
  - `ErrNotInitialized` is wrapped rather than returned bare: assert `errors.Is(err, configengine.ErrNotInitialized)` holds for `FindBaseDir` on an absent `_lyx/`, and separately that the rendered message still contains `not initialized`, since four strict callers outside this task depend on that text.
  - A hand-constructed non-sentinel error does not satisfy `errors.Is(err, configengine.ErrNotInitialized)`.
  - Absence-only discrimination: an `_lyx/` that exists but cannot be stat'd makes `LoadOrTemplate` return an error rather than falling back.
    Construct it by `os.Chmod`-ing the containing directory to `0o000`, guarded by `runtime.GOOS` so the test skips on Windows, and skipped when `os.Geteuid() == 0` since root ignores the mode.
    Register the mode-restoring `t.Cleanup` immediately after `t.TempDir()` and before the chmod, because cleanups run LIFO and a restore registered later would run after `TempDir`'s own `RemoveAll` and leave teardown failing.
    Assert the returned error fails `errors.Is(err, configengine.ErrNotInitialized)` and that the returned bytes are nil.

  Also reword the doc comment on `TestLyxDirNameConstant`, which today reads "moved here from lyxcwd's own unit test now that configengine is the single declarer of the `_lyx` token" while the assertion three lines below reads `lyxdirs.LyxDirName`.
  Name `internal/lyxdirs` as the declarer instead, matching the Lyxdirs Single-Declarer Invariant.
  The test's name and body are correct as they stand and stay unchanged.
  Update the file-header comment to mention the degrading `LoadOrTemplate` contract alongside the strict `Load` contract it already describes.
  Every test added here stays untagged Tier 1 per the Test Tier Purity Invariant: no `git init`, no `exec.Command`, no `hubforge.NewHub`, no `gitkit.Copy*`, and no sleep of a second or more.
- **Commit:** `test(configengine): cover LoadOrTemplate fallback and absence-only discrimination`

## Batch Tests

`verify: go test ./internal/configengine/...` runs the whole `internal/configengine` package, which is exactly the two files this batch touches (`config.go`, `config_test.go`) plus the package's untouched `set.go`/`edit.go` and their tests.
Scoping to the single package is correct here: the shared-body refactor's blast radius inside the package is total, and no other package's behaviour changes in this batch.

The batch's own regression gate is the pre-existing `TestLoad_NotInitialized` and `TestLoad_AbsentFile`, which stay unmodified and must keep passing — they pin that `Load` did not change.
The new `LoadOrTemplate` tests cover both fallback triggers, the strict-when-present boundary in three forms (missing key, empty, comments-only), env resolution on the fallback path, the fallback tail's own `<module> config template:` error wrap, and the absence-only discrimination the `fallback-only-on-proven-absence` decision exists for.
`set.go`'s `Set` and `edit.go`'s `Edit` are write paths and stay strict;
their existing tests passing unchanged confirms this batch left them alone.
