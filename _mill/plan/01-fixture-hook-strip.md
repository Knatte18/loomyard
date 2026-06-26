# Batch: fixture-hook-strip

```yaml
task: "Speed up internal/warp integration tests"
batch: "fixture-hook-strip"
number: 1
cards: 1
verify: go test ./internal/lyxtest/ && go test -tags integration -run TestList ./internal/warp/
depends-on: []
```

## Batch Scope

This batch delivers the single highest-leverage, suite-wide speedup: removing the inert
`*.sample` git hook files (28 of 45 files, ~62%) from every fixture template the `lyxtest`
builders produce, so every per-test fixture copy scans ~60% fewer files. It is its own
batch because it touches the shared fixture builder (`internal/lyxtest/lyxtest.go`) that
all consolidation batches depend on; landing it first means every subsequent batch's
`verify:` exercises the slimmer templates. No test behaviour changes — only inert files
are removed. The other batches depend on this one (`depends-on: [1]`).

## Cards

### Card 1: Strip inert git hook samples from lyxtest templates

- **Context:**
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/lyxtest/lyxtest.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Add an unexported helper `func stripHookSamples(hooksDir string)` that
  globs `filepath.Join(hooksDir, "*.sample")` and `os.Remove`s each match, ignoring errors
  (the strip is best-effort; missing/locked samples must not panic a fixture build). Call
  it from `initRepo` after the existing `mustGit` config calls, passing
  `filepath.Join(dir, ".git", "hooks")`; and from `initBareRemote` immediately after
  `mustGit(dir, "init", "--bare")`, passing `filepath.Join(dir, "hooks")` (a bare repo's
  hooks live at `<dir>/hooks`, not `<dir>/.git/hooks`). Use only `os` and `filepath` (both
  already imported) — do not add any import outside stdlib + `internal/paths`, preserving
  the lyxtest leaf invariant enforced by `internal/lyxtest/leaf_enforcement_test.go`.
- **Commit:** `perf(lyxtest): strip inert git hook samples from fixture templates`

## Batch Tests

`verify` runs the `internal/lyxtest` package's own unit tests (fast, untagged — confirms
the builders and leaf-enforcement test still pass) and then a single cheap warp
integration test, `TestList`, which builds a `CopyHostHub` fixture — proving a template
with stripped hooks still produces a working git repo. The full timed warp suite is run
by the operator (per the discussion's operator-run verification protocol), not here.
