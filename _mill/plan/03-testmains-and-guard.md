# Batch: testmains-and-guard

```yaml
task: 'Speed up git-fixture tests: bench, analyse, hardlink'
batch: 'testmains-and-guard'
number: 3
cards: 6
verify: go test -tags integration -run 'TestTierPurity|TestHermeticGitEnv' -count=1 ./...
depends-on: [1, 2]
```

## Batch Scope

Wire `lyxtest.HermeticGitEnv()` into every git-spawning test package via
`TestMain`, add the machine-check guard that keeps it that way, and record the
new invariant in `CONSTRAINTS.md`. TDD order inside the batch: the guard lands
first (card 5) and fails, enumerating every package missing a `TestMain`;
cards 6–9 then add the 22 `testmain_test.go` files (grouped by subsystem) until
the guard is green; card 10 records the invariant. The authoritative package
list is what the guard's first failing run prints — the card lists below were
derived by running the same token scan on the current tree and are expected to
match it exactly; if the guard flags a package not listed here, add the same
canonical `TestMain` there too (same commit as the card whose group it fits).

Depends on batch 1 (tier-purity green, so the guard's companion checks pass
repo-wide) and batch 2 (the helper exists). The batch verify compiles **every**
package's test binary under `-tags integration` (catching a syntactically
broken `testmain_test.go` anywhere) while running only the two `cmd/lyx`
guards — the full suites are not run per round.

## Cards

### Card 5: Hermetic guard test + reciprocal tierpurity allowlist

- **Context:**
  - `CONSTRAINTS.md`
  - `_mill/discussion.md`
- **Edits:**
  - `cmd/lyx/tierpurity_test.go`
- **Creates:**
  - `cmd/lyx/hermeticenv_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** New untagged test file, `package main`, containing
  `TestHermeticGitEnv_GitSpawningPackagesHaveTestMain`. Mechanics (mirror
  `tierpurity_test.go`'s structure): resolve the module root via
  `go env GOMOD` (cwd-independent), walk every `*_test.go` under it, and —
  unlike tierpurity, which skips tag-gated files — scan **all** test files
  regardless of build constraints (the git-spawning set is almost exactly the
  integration-tagged set; skipping tagged files would make the guard vacuous).
  A package is "git-spawning" when any of its test files contains one of the
  raw substrings `gitexec.RunGit`, `exec.Command`, `lyxtest.Copy`,
  `lyxtest.MustRun`, `lyxtest.SeedConfig` (the last two spawn git inside
  lyxtest, invisible to the first three). Every git-spawning package must
  have at least one `*_test.go` containing the raw substring `HermeticGitEnv`
  (bare name — matches both the qualified call in other packages and the
  unqualified call in `package lyxtest`), OR be named on an
  `allowedNonHermetic` map (path or path-prefix → one-line reason) with
  exactly these entries: `internal/proc` (spawns generic non-git processes —
  process control is the package's subject) and `cmd/lyx/hermeticenv_test.go`
  (this guard file itself; carries the tokens as its own test data).
  Vacuous-scan floor: fail if the scan finds zero git-spawning packages.
  Failure message must be actionable like tierpurity's: name the package,
  the triggering token, and the fix ("add a testmain_test.go calling
  lyxtest.HermeticGitEnv(), or add an allowedNonHermetic entry with a
  reason"). Also verify the raw-substring presence check is documented in the
  test's comment as the mechanical half, with the semantic half (a real
  `TestMain` invoking the helper before `m.Run()`) a review obligation.
  In `cmd/lyx/tierpurity_test.go`, add `cmd/lyx/hermeticenv_test.go` to the
  existing `allowedSpawners` map with the same "contains the banned token
  strings as its own test data" reason `tierpurity_test.go` gives itself —
  the new guard file is untagged and carries tierpurity's banned tokens, so
  without this entry batch verify fails.
- **Commit:** `test(lyx): guard that git-spawning test packages run HermeticGitEnv`

### Card 6: TestMains — warp/weft subsystem

- **Context:**
  - `_mill/discussion.md`
- **Edits:** none
- **Creates:**
  - `internal/warpengine/testmain_test.go`
  - `internal/warpcli/testmain_test.go`
  - `internal/weftengine/testmain_test.go`
  - `internal/weftcli/testmain_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Each file follows the canonical-testmain-shape Shared
  Decision exactly. Package clauses: `package warpengine`,
  `package warpcli`, `package weftengine`, `package weftcli` (each directory
  already has internal-package test files; warpengine/warpcli also have
  external ones — the internal form is the chosen one per the Shared
  Decision, and only one `TestMain` per test binary is allowed).
- **Commit:** `test(warp,weft): hermetic git env via TestMain`

### Card 7: TestMains — board/builder subsystem

- **Context:**
  - `_mill/discussion.md`
- **Edits:** none
- **Creates:**
  - `internal/boardcli/testmain_test.go`
  - `internal/boardengine/boardtest/testmain_test.go`
  - `internal/buildercli/testmain_test.go`
  - `internal/builderengine/testmain_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Canonical shape. Package clauses: `package boardcli`,
  `package boardtest`, `package buildercli`, `package builderengine`.
- **Commit:** `test(board,builder): hermetic git env via TestMain`

### Card 8: TestMains — perch/mux/shuttle/burler subsystem

- **Context:**
  - `_mill/discussion.md`
- **Edits:** none
- **Creates:**
  - `internal/perchcli/testmain_test.go`
  - `internal/perchengine/testmain_test.go`
  - `internal/muxcli/testmain_test.go`
  - `internal/muxpoccli/testmain_test.go`
  - `internal/shuttlecli/testmain_test.go`
  - `internal/burlerengine/testmain_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Canonical shape. Package clauses: `package perchcli`,
  `package perchengine`, `package muxcli`, `package muxpoccli`,
  `package shuttlecli`, `package burlerengine`.
- **Commit:** `test(perch,mux,shuttle,burler): hermetic git env via TestMain`

### Card 9: TestMains — init/config/ide/geometry/gitexec/cmd

- **Context:**
  - `_mill/discussion.md`
- **Edits:** none
- **Creates:**
  - `internal/initengine/testmain_test.go`
  - `internal/initcli/testmain_test.go`
  - `internal/configcli/testmain_test.go`
  - `internal/idecli/testmain_test.go`
  - `internal/ideengine/testmain_test.go`
  - `internal/hubgeometry/testmain_test.go`
  - `internal/gitexec/testmain_test.go`
  - `cmd/lyx/testmain_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Canonical shape. Package clauses: `package initengine`,
  `package initcli_test` (the directory's tests are external-only),
  `package configcli`, `package idecli`, `package ideengine`,
  `package hubgeometry_test` (**must** be external — lyxtest imports
  hubgeometry, so an internal test file importing lyxtest closes an import
  cycle), `package gitexec_test` (external-only today; no cycle either way,
  match what exists), `package main` (for `cmd/lyx`).
- **Commit:** `test(init,config,ide,hubgeometry,gitexec,lyx): hermetic git env via TestMain`

### Card 10: Record the invariant — CONSTRAINTS.md + lyxtest godoc

- **Context:**
  - `_mill/discussion.md`
- **Edits:**
  - `CONSTRAINTS.md`
  - `internal/lyxtest/doc.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add a new `## Hermetic Git Test Environment Invariant`
  section to `CONSTRAINTS.md`, following the file's established entry shape
  (statement, mechanics, allowlist discipline, **Enforced by** line). Content:
  every package whose tests spawn git (directly or via the lyxtest helpers)
  runs under the hermetic git env — a `TestMain` calling
  `lyxtest.HermeticGitEnv()` — so no test behaviour depends on the operator's
  `~/.gitconfig` or the system gitconfig (the concrete failure this kills:
  a global `core.fsmonitor=true` spawning hundreds of fsmonitor daemons per
  integration run; numbers in `docs/benchmarks/fixture-copy.md`). Exists ⇒
  hermetic or allowlisted with a reason. Enforced by
  `cmd/lyx/hermeticenv_test.go`
  (`TestHermeticGitEnv_GitSpawningPackagesHaveTestMain`) on every `go test`;
  the semantic half (helper actually called before `m.Run()`) is a review
  obligation, like the repo's other grep-guards. Update
  `internal/lyxtest/doc.go`'s package comment with a short paragraph naming
  the two layers (template quiet-config + `HermeticGitEnv`) and pointing at
  the CONSTRAINTS entry.
- **Commit:** `docs(constraints): record the Hermetic Git Test Environment Invariant`

## Batch Tests

`verify:` = `go test -tags integration -run 'TestTierPurity|TestHermeticGitEnv' -count=1 ./...`.
Scope justification (this is the deliberate cross-cutting exception): the
pattern compiles **every** package's test binary with the integration tag —
the only per-round way to catch a broken `testmain_test.go` in any of the 22
packages — while executing just the two repo-walking guards in `cmd/lyx`
(every other package has zero matching test names). The full integration
suites do not run per round; they run once as batch 4's recorded timing.
