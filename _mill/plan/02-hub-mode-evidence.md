# Batch: hub-mode-evidence

```yaml
task: "scoutengine told-geometry (optional uniformity pass)"
batch: "hub-mode-evidence"
number: 2
cards: 3
verify: go build ./... && go vet -tags integration ./internal/scoutcli/... && go test -tags integration ./internal/scoutcli/...
depends-on: [1]
```

## Batch Scope

Batch 1's reshaped `lookupContext` test covers only the out-of-hub half of this task's sole acceptance property.
This batch adds the hub half: an `//go:build integration` test in `internal/scoutcli` driving real `hubforge.NewHub` fixtures at both an unanchored and a subpath anchor, plus the `TestMain` companion the Hermetic Git Test Environment Invariant makes mandatory the moment `hubforge.NewHub` enters this package.
It is a separate batch because both files are genuinely new, neither is needed for batch 1 to compile, and its `verify:` needs the `integration` tag batch 1's does not.

These two files are the only genuinely new tests this task adds; everything in batch 1 is a conversion.

The batch closes with the tagged-suite gate that batch 1's `verify:` deliberately deferred — the executed `-tags scout` run and the manual smoke check.

## Cards

### Card 11: the mandatory `TestMain` companion

- **Context:**
  - `internal/perchcli/testmain_test.go`
  - `cmd/lyx/hermeticenv_test.go`
- **Edits:** none
- **Creates:**
  - `internal/scoutcli/testmain_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/scoutcli/testmain_test.go` as a copy of `internal/perchcli/testmain_test.go`, changing only the package clause to `package scoutcli` and the file-header prose to name scoutcli's own fixture instead of perchcli's.
  It declares `func TestMain(m *testing.M)` calling `gitkit.HermeticGitEnv()` then `os.Exit(m.Run())`, and imports `os`, `testing`, and `github.com/Knatte18/loomyard/internal/gitkit`.
  Carry no build tag on this file: `cmd/lyx/hermeticenv_test.go` scans tag-agnostically and would not see a tagged `TestMain` as satisfying the requirement for the tagged file card 12 adds.
  This file is a direct, non-optional consequence of card 12 — `hubforge.NewHub` is one of `hermeticenv_test.go`'s git-spawn tokens and `internal/scoutcli` is not on its `allowedNonHermetic` allowlist, so without this file `TestHermeticGitEnv_GitSpawningPackagesHaveTestMain` fails.
- **Commit:** `test(scoutcli): add the hermetic git TestMain companion`

### Card 12: the hub-mode `lookupContext` integration test

- **Context:**
  - `internal/perchcli/cli_integration_test.go`
  - `internal/hubforge/hub.go`
  - `internal/hubforge/seed.go`
  - `internal/scoutcli/cli.go`
  - `internal/scoutengine/registry.go`
  - `internal/scoutengine/load.go`
  - `internal/lyxcwd/lyxcwd.go`
  - `internal/webstercli/verbs_test.go`
- **Edits:** none
- **Creates:**
  - `internal/scoutcli/cli_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/scoutcli/cli_integration_test.go` whose first line is `//go:build integration` — mandatory, because `hubforge.NewHub` is banned in untagged test files by the Test Tier Purity Invariant — in `package scoutcli`, following `internal/perchcli/cli_integration_test.go`'s fixture shape.
  Two cases are required.
  Case 1, the unanchored hub: build `h := hubforge.NewHub(t, ".")`, call `lookupContext(h.PrimeWorktree(), <a separate t.TempDir()>)` — deliberately passing a `dir` that is not the worktree — and assert the returned anchor root is `h.Location.AnchorPath()` and is not `filepath.Abs(dir)`.
  The mismatched `dir` is what makes the assertion discriminating: it fails an implementation that wrongly took the out-of-hub branch, which a same-value fixture would not catch.
  Case 1 asserts the anchor root only and carries no registry assertion.
  Case 2, the subpath-anchored hub: build `h := hubforge.NewHub(t, "backend")`, seed a distinguishing overlay with `hubforge.SeedConfig(t, h, map[string]string{"servers": <yaml>})`, call `lookupContext(h.Location.AnchorPath(), <a separate t.TempDir()>)`, and assert both that the anchor root equals `h.Location.AnchorPath()` and is not `h.Location.WorktreePath()`, and that the seeded entry is present in the returned registry.
  Use `h.Location.AnchorPath()` as the `cwd` argument in case 2, never `h.PrimeWorktree()`: the latter returns `WorktreePath()`, which in a subpath-anchored hub sits outside the anchor and makes `lyxcwd.Resolve` return `ErrCwdOutsideAnchor`, so the test would exercise the degraded branch while appearing to test the hub branch.
  Case 2 is the load-bearing one and neither of its two assertions may be dropped: under the `"."` anchor `AnchorPath()` and `WorktreePath()` are byte-identical, so case 1 alone passes an implementation that wrote `layout.WorktreePath()`; and `hubforge.SeedConfig` writes at `h.WeftBase`, which coincides with `PrimeWeft()` at the `"."` anchor and diverges at `"backend"`, so only at a subpath anchor does the registry assertion prove `LoadRegistry` still reads at `AnchorPath()`.
  The seeded YAML must satisfy `validateEntry` or `LoadRegistry` returns an error and case 2 fails on the wrong axis: supply all four required keys — `markers` as a non-empty list, `match` as exactly `"all"` or `"any"`, `command` as a non-empty list, and `install_hint` as a non-empty string, noting the snake_case tag against the `InstallHint` field.
  `LoadRegistry` decodes under `KnownFields(true)`, so an unknown key is also an error; copy the shape of `builtins()`'s own `"go"` entry and use a recognisable non-builtin language key so its presence in the returned registry is unambiguous.
  `internal/webstercli/verbs_test.go`'s `seedPersistentPreRunFixture` is the working precedent for a `hubforge.SeedConfig` round trip read back through an anchored `AnchorPath()`, at both the `"."` and `"backend"` anchors.
- **Commit:** `test(scoutcli): pin lookupContext's hub branch at both anchors`

### Card 13: tagged-suite and smoke gate

- **Context:**
  - `internal/scoutengine/supervised_scout_test.go`
  - `internal/scoutengine/supervised_integration_test.go`
  - `internal/scoutengine/ensureserver_integration_test.go`
  - `internal/scoutengine/refs_integration_test.go`
- **Edits:** none
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** This is a zero-diff verification gate, run once at the end of the task rather than on every implementer round.
  Run `go test -tags scout ./internal/scoutengine/...` and confirm it passes.
  This is the executed run batch 1's `verify:` deferred to a `go vet -tags scout` compile check; the four files listed in this card's `Context:` are invisible to an untagged `go test` and this is the only place their behaviour is exercised.
  Expect it to be slow: each test spawns a real `gopls` daemon, and a cold machine additionally pays a `go install` of the pinned `gopls` version through the toolchain manager.
  Then run the manual acceptance smoke, on top of the tests and never as the only evidence for either half of the acceptance property: from a scratch directory outside any git repository, run `lyx scout symbol <name> --target-dir <dir>` and confirm it still resolves; and from this worktree, run `lyx scout refs` and confirm `.lyx/scout/go/daemon.json` appears at the same path as before the change.
  If either the tagged suite or the smoke check fails, the failure belongs to the card that owns the offending file; correct it there.
- **Commit:** none

## Batch Tests

`verify:` is `go build ./... && go vet -tags integration ./internal/scoutcli/... && go test -tags integration ./internal/scoutcli/...`.

- `go test -tags integration ./internal/scoutcli/...` runs card 12's new `cli_integration_test.go` — the only automated evidence for the hub half of the acceptance property — with card 11's `testmain_test.go` supplying the hermetic git environment it needs.
- `go vet -tags integration ./internal/scoutcli/...` is redundant with the `go test` line that follows it in the common case, and is kept as the cheap compile-only signal that fires first when the new file does not build at all.
- `go build ./...` re-confirms the untagged tree is still green after batch 1, since batch 2 is where a batch-1 regression would otherwise first surface at the repo level.
- Card 13's `go test -tags scout ./internal/scoutengine/...` is deliberately outside `verify:`: it is minutes-long and daemon-spawning, and paying it on every fixer round of this batch would buy nothing, since this batch touches no `scout`-tagged file.

The repo-wide `pipeline.done_gate` (`go test ./... && go test -tags integration ./...`) re-runs both the untagged tree and card 12's integration test before the task is marked done, catching any regression in a package outside either batch's verify scope.
The `scout` tag is not covered by that gate, which is precisely why card 13 exists.
