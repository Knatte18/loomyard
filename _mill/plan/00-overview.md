# Plan: git-native-library: feasibility spike

```yaml
task: 'git-native-library: feasibility spike'
slug: git-native-library
approved: false
started: '20260725-125659'
parent: main
root: ""
verify: null
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to
schedule batches. Every batch lives at `NN-<batch-slug>.md` in this
directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: poc-foundation
    file: 01-poc-foundation.md
    depends-on: []
    verify: go test -tags integration ./internal/gitnativepoc/ && go test ./cmd/lyx/ -run 'TestTierPurity_UntaggedTestsSpawnNothing|TestHermeticGitEnv_GitSpawningPackagesHaveTestMain'
  - number: 2
    name: read-surface
    file: 02-read-surface.md
    depends-on: [1]
    verify: go test -tags integration ./internal/gitnativepoc/ && go test ./cmd/lyx/ -run 'TestTierPurity_UntaggedTestsSpawnNothing|TestHermeticGitEnv_GitSpawningPackagesHaveTestMain'
  - number: 3
    name: write-surface
    file: 03-write-surface.md
    depends-on: [1, 2]
    verify: go test -tags integration ./internal/gitnativepoc/ && go test ./cmd/lyx/ -run 'TestTierPurity_UntaggedTestsSpawnNothing|TestHermeticGitEnv_GitSpawningPackagesHaveTestMain'
  - number: 4
    name: writeup-and-doc-lifecycle
    file: 04-writeup-and-doc-lifecycle.md
    depends-on: [2, 3]
    verify: go test -tags integration ./internal/gitnativepoc/ && go test ./cmd/lyx/ -run 'TestTierPurity_UntaggedTestsSpawnNothing|TestHermeticGitEnv_GitSpawningPackagesHaveTestMain'
```

## Shared Decisions

_Cross-cutting decisions every batch inherits: naming conventions,
error-handling posture, test frameworks, style/lint constraints. One
subsection per decision. Batch-local decisions live in each batch file._

### Decision: package-shape-and-invariants

- **Decision:** `internal/gitnativepoc` is a plain (untagged) Go package whose
  non-test `.go` files compile in every `go test` run, plus a set of
  **`//go:build integration`-tagged** `_test.go` files that carry the entire
  parity harness. The package is **not** registered as a cobra module and is
  never wired into `cmd/lyx`.
- **Rationale:** Not registering sidesteps the CLI/Cobra and Sandbox Suite
  Coverage invariants (no help-tree, `Short`, or sandbox obligations attach to a
  spike). Integration-tagging every git-spawning `_test.go` keeps the package
  inside the Test Tier Purity Invariant (`cmd/lyx/tierpurity_test.go` fails if an
  *untagged* `_test.go` under the tree contains `gitexec.RunGit`, `exec.Command`,
  or `lyxtest.Copy`). A `TestMain` calling `lyxtest.HermeticGitEnv()` keeps it
  inside the Hermetic Git Test Environment Invariant
  (`cmd/lyx/hermeticenv_test.go`).
- **Applies to:** all batches

### Decision: differential-oracle

- **Decision:** The parity harness is differential: for each operation and
  fixture it runs the **go-git-backed poc method** and the **CLI/`gitexec`
  reference** and asserts the two agree (same SHA, same sorted file list, same
  typed error class, same boolean). The reference side is the real
  `internal/gitrepo.Repo` method wherever the surface is exported on `gitrepo`
  (`CurrentSHA`, `SHAExists`, `ChangedFilesSince`, `SnapshotSHA`,
  `StageAndCommit`, `StageAllAndCommit`, `Push`, `SetSnapshotSHA`); for
  `gitrepo`'s **unexported** helpers (`remoteName` fallback, `hasUnpushed`
  no-upstream, `isStrictDescendant` truth table) the reference is a direct git
  fixture assertion, since those behaviours are not reachable through a public
  `gitrepo` method. Importing `internal/gitrepo` and `internal/gitexec` from the
  test files is allowed (same module) and does not modify either package.
- **Rationale:** Using the shipping `gitrepo` method as the oracle grounds
  "behavioural parity with git" in the exact behaviour the crucible hardening
  cared about, rather than a re-derived expectation.
- **Applies to:** read-surface, write-surface

### Decision: cli-bound-is-a-recorded-outcome

- **Decision:** When go-git genuinely cannot perform an operation (or diverges
  on a hard-gate case), that is a **finding, not a test failure to fix**: the poc
  method records the limitation (returns a documented sentinel error /
  `CLI-BOUND` marker) and the corresponding test asserts *that* divergence
  explicitly. A real regression (an op that should match but doesn't) stays a red
  test. Every test comment states which case it is, so the two are never
  conflated.
- **Rationale:** The deliverable is the MIGRATE/CLI-BOUND classification; a
  CLI-BOUND verdict is a legitimate positive result, not a bug.
- **Applies to:** read-surface, write-surface, writeup-and-doc-lifecycle

### Decision: go-git-version-pin

- **Decision:** go-git is added via `go get github.com/go-git/go-git/v5` (the
  maintained fork; the module path **must** carry the `/v5` major-version
  suffix). The resolved version is committed in `go.mod`/`go.sum` (never
  hand-edited) and recorded verbatim in the `doc.go` write-up alongside the
  verdicts.
- **Rationale:** The dependency lands on `main` and the harness must be
  re-runnable (including the later Win11 pass), so the verdict-bearing version is
  pinned and reproducible.
- **Applies to:** all batches

### Decision: os-portable-verify-on-linux

- **Decision:** All poc code and tests are written OS-portable (no Linux-only
  path/permission assumptions); they are verified on Linux in this task. Any test
  or verdict whose deciding factor is Windows-specific behaviour is marked
  `Win11-pending` in a code comment and in the write-up. No Windows run happens
  in this task.
- **Applies to:** all batches

## All Files Touched

_Full union of every `Creates:` / `Edits:` / `Moves:` **target** path
across every batch, sorted alphabetically._

- `docs/overview.md`
- `go.mod`
- `go.sum`
- `internal/gitnativepoc/doc.go`
- `internal/gitnativepoc/gitnativepoc.go`
- `internal/gitnativepoc/harness_test.go`
- `internal/gitnativepoc/read.go`
- `internal/gitnativepoc/read_test.go`
- `internal/gitnativepoc/testmain_test.go`
- `internal/gitnativepoc/write.go`
- `internal/gitnativepoc/write_test.go`
- `manifest/roadmap.md`
