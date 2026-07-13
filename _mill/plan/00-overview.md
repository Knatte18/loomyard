# Plan: Speed up git-fixture tests: bench, analyse, hardlink

```yaml
task: 'Speed up git-fixture tests: bench, analyse, hardlink'
slug: 'faster-git-fixture-tests'
approved: true
started: '20260713-060023'
parent: 'main'
root: ""
verify: go build ./...
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to
schedule batches. Every batch lives at `NN-<batch-slug>.md` in this
directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: retier-builder-tests
    file: 01-retier-builder-tests.md
    depends-on: []
    verify: go test -tags integration -run TestTierPurity -count=1 ./cmd/lyx ./internal/buildercli ./internal/builderengine
  - number: 2
    name: hermetic-lyxtest
    file: 02-hermetic-lyxtest.md
    depends-on: []
    verify: go test -tags integration -count=1 ./internal/lyxtest
  - number: 3
    name: testmains-and-guard
    file: 03-testmains-and-guard.md
    depends-on: [1, 2]
    verify: go test -tags integration -run 'TestTierPurity|TestHermeticGitEnv' -count=1 ./...
  - number: 4
    name: bench-and-docs
    file: 04-bench-and-docs.md
    depends-on: [3]
    verify: go test -tags integration -run '^$' -bench BenchmarkCopy -benchtime 1x -count=1 ./internal/lyxtest
```

## Shared Decisions

### Decision: winning-lever-is-hermetic-git-env-not-hardlink

- **Decision:** The task's original hardlink-objects hypothesis was empirically
  refuted during discussion (fixture copy is ~1–2 % of Tier 2; see the
  Benchmark report in `_mill/discussion.md`). The implemented lever is the
  hermetic git test environment that kills `fsmonitor--daemon` and auto
  `maintenance` spawns (measured: warpengine 102–111 s → 62–72 s alone,
  ~152 s → 87 s under full Tier 2 contention). `copyDirRecursive` stays a
  plain byte-copy; no hardlink, no alternates.
- **Rationale:** Measured on the target machine; recorded in
  `_mill/discussion.md` and ported to `docs/benchmarks/fixture-copy.md` in
  batch 4. All numbers are Windows-only.
- **Applies to:** all batches

### Decision: helper-name-HermeticGitEnv

- **Decision:** The Layer B helper is `func HermeticGitEnv()` in
  `internal/lyxtest/hermetic.go`. The name is the guard's presence token,
  matched as a **bare substring** (`HermeticGitEnv`) so both the qualified
  `lyxtest.HermeticGitEnv()` call form (other packages) and the unqualified
  `HermeticGitEnv()` call form (lyxtest's own `package lyxtest` tests) match.
  Do not rename without updating the guard token.
- **Rationale:** Discussion decision `hermetic-guard-and-constraints-entry`
  (round-3 review gap): a qualified token would miss lyxtest itself, the most
  git-heavy fixture package.
- **Applies to:** all batches

### Decision: canonical-testmain-shape

- **Decision:** Every git-spawning test package gets one new file
  `testmain_test.go` (untagged, no build constraint) with exactly this shape,
  adjusting only the package clause and the lyxtest qualifier:

  ```go
  package <pkg>

  import (
      "os"
      "testing"

      "github.com/Knatte18/loomyard/internal/lyxtest"
  )

  func TestMain(m *testing.M) {
      lyxtest.HermeticGitEnv()
      os.Exit(m.Run())
  }
  ```

  Go allows one `TestMain` per test binary (internal `package foo` and
  external `package foo_test` files compile into the same binary), so each
  package gets exactly one such file. Package-clause rules: use the package
  form that already exists in that directory's test files; where both forms
  exist, use the internal form — EXCEPT `internal/hubgeometry`, which MUST
  use `package hubgeometry_test` (lyxtest imports hubgeometry, so an internal
  test file importing lyxtest closes an import cycle). `internal/lyxtest`
  itself gets its `TestMain` inside the existing integration-tagged
  `lyxtest_test.go` (`package lyxtest`, unqualified `HermeticGitEnv()` call)
  instead of a new file.
- **Rationale:** Discussion decisions `two-layer-hermetic-mechanism` and the
  TestMain placement caveats under Technical context.
- **Applies to:** hermetic-lyxtest, testmains-and-guard

### Decision: tierpurity-compliance-for-new-files

- **Decision:** All new untagged test files (every `testmain_test.go`, the
  new guard file) must not contain the tierpurity banned tokens
  (`gitexec.RunGit`, `exec.Command`, `lyxtest.Copy`) — except the guard file
  itself, which carries them as test data and is therefore added to
  `cmd/lyx/tierpurity_test.go`'s `allowedSpawners` map in the same commit
  that creates it. New git-spawning test code (batch 2's helper tests,
  batch 4's benchmarks) goes in integration-tagged files.
- **Rationale:** Test Tier Purity Invariant (CONSTRAINTS.md); round-4 review
  NOTE "New hermetic guard file trips the tierpurity guard".
- **Applies to:** all batches

### Decision: go-native-verify-commands

- **Decision:** This is a Go repo: `verify:` commands use the native Go
  toolchain directly, without the `PYTHONPATH= ` prefix (the prefix rule is
  Python-project-specific).
- **Rationale:** mill-plan verify-command shape rule for non-Python projects.
- **Applies to:** all batches

## All Files Touched

- `CONSTRAINTS.md`
- `cmd/lyx/hermeticenv_test.go`
- `cmd/lyx/testmain_test.go`
- `cmd/lyx/tierpurity_test.go`
- `docs/benchmarks/fixture-copy.md`
- `docs/benchmarks/running-tests.md`
- `docs/benchmarks/test-suite-timing.md`
- `internal/boardcli/testmain_test.go`
- `internal/boardengine/boardtest/testmain_test.go`
- `internal/buildercli/spawnbatch_test.go`
- `internal/buildercli/testmain_test.go`
- `internal/buildercli/validate_test.go`
- `internal/builderengine/config_test.go`
- `internal/builderengine/template_test.go`
- `internal/builderengine/testmain_test.go`
- `internal/burlerengine/testmain_test.go`
- `internal/configcli/testmain_test.go`
- `internal/gitexec/testmain_test.go`
- `internal/hubgeometry/testmain_test.go`
- `internal/idecli/testmain_test.go`
- `internal/ideengine/testmain_test.go`
- `internal/initcli/testmain_test.go`
- `internal/initengine/testmain_test.go`
- `internal/lyxtest/bench_test.go`
- `internal/lyxtest/doc.go`
- `internal/lyxtest/hermetic.go`
- `internal/lyxtest/lyxtest.go`
- `internal/lyxtest/lyxtest_test.go`
- `internal/muxcli/testmain_test.go`
- `internal/muxpoccli/testmain_test.go`
- `internal/perchcli/testmain_test.go`
- `internal/perchengine/testmain_test.go`
- `internal/shuttlecli/testmain_test.go`
- `internal/warpcli/testmain_test.go`
- `internal/warpengine/testmain_test.go`
- `internal/weftcli/testmain_test.go`
- `internal/weftengine/testmain_test.go`
