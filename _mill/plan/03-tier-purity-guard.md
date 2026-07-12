# Batch: tier-purity-guard

```yaml
task: 'Fix test-suite regression: slow Tier 1 + 2 red packages + stale benchmarks'
batch: tier-purity-guard
number: 3
cards: 1
verify: go test ./cmd/lyx -run TestTierPurity -count=1
depends-on: [2]
```

## Batch Scope

Machine-enforce the offline tier's premise so it cannot rot silently again:
one untagged repo-wide grep-guard test in `cmd/lyx` (the established home for
repo-wide guards — sandbox coverage, drift, help-tree) plus the matching
CONSTRAINTS.md invariant entry in the same commit (Documentation Lifecycle
rule). Depends on batch 2: the guard fails on any untagged banned-token test
file, so it can only land green after the re-tiering. This is the durable
deliverable that turns the whole task from a one-off cleanup into an invariant.

## Cards

### Card 9: tier purity grep-guard test + CONSTRAINTS.md invariant

- **Context:**
  - `_mill/discussion.md`
  - `cmd/lyx/sandbox_coverage_test.go`
  - `cmd/lyx/crosscompile_test.go`
  - `internal/hubgeometry/enforcement_test.go`
  - `internal/lyxtest/leaf_enforcement_test.go`
- **Edits:**
  - `CONSTRAINTS.md`
  - `cmd/lyx/main_test.go`
  - `internal/perchcli/cli_test.go`
  - `internal/perchcli/run_test.go`
- **Creates:**
  - `cmd/lyx/tierpurity_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `TestTierPurity_UntaggedTestsSpawnNothing` in
  `cmd/lyx/tierpurity_test.go` (package `main`, untagged — it must run on
  every `go test`). Mechanics: walk every `*_test.go` file under the module
  root (resolve the root via `go env GOMOD` exactly as
  `crosscompile_test.go` does — its `go env GOMOD` + `os.DevNull` idiom is in
  this card's Context — prefer the GOMOD approach for cwd-independence; skip
  `.git`, `_lyx`, `_mill`, `.scratch`, `.wiki`, `_raddle` directories).
  Normalize every walked module-relative path with `filepath.ToSlash` BEFORE
  matching it against skip-dir names or `allowedSpawners` prefixes —
  `filepath.WalkDir` yields backslash paths on Windows (the primary dev OS),
  and un-normalized matching would silently miss the slash-separated
  allowlist prefixes and falsely trip the guard. For each file, read the
  source; if the first non-empty line is NOT a `//go:build` constraint
  containing `integration` or `smoke`, the file is "untagged" (platform-only
  constraints like `//go:build windows` count as untagged — they still run
  in Tier 1 on that platform). An untagged file FAILS the test if its source
  contains any banned token as a **raw substring**: `gitexec.RunGit`,
  `exec.Command` (which also matches `exec.CommandContext`), or
  `lyxtest.Copy` (prefix-matches `CopyPaired`, `CopyPairedLocal`,
  `CopyHostHub`, and any future `Copy*` fixture). Raw-substring matching is
  deliberate: comment/string mentions in untagged files trip the guard too
  (rename the mention or tag the file). Allowlist, as a package-level
  `allowedSpawners` map keyed by slash-separated module-relative path prefix
  with a one-line reason each (mirroring `sandbox_coverage_test.go`'s
  `excludedModules` style): `internal/proc` (process control is the
  package's subject — its tests must spawn) and `cmd/lyx/tierpurity_test.go`
  itself (contains the banned token strings as its own test data). Vacuous-
  scan protection: fail if the walk finds fewer than 20 `*_test.go` files
  (the repo has ~60). Failure output lists every offending file with the
  first banned token found and says how to fix (move the test behind
  `//go:build integration` or add an allowlist entry with a reason). Also
  add a `## Test Tier Purity Invariant` section to `CONSTRAINTS.md`
  (between the Sandbox Suite Coverage section and the Documentation
  Lifecycle section, matching the established format): statement worded as
  "untagged test files perform no expensive spawns — no `git init` /
  `git worktree add` / fixture-tree copies; Tier 1 stays offline and fast"
  (NOT "spawn no processes": untagged tests that reach
  `hubgeometry.Resolve` still spawn one cheap failing `git rev-parse`,
  which the token guard deliberately does not ban), the banned-token +
  raw-substring semantics, the
  allowlist rule (exists ⇒ tagged or allowlisted with reason), and an
  **Enforced by** line naming `cmd/lyx/tierpurity_test.go`
  (`TestTierPurity_UntaggedTestsSpawnNothing`) on every `go test`.
- **Commit:** `test(cmd/lyx): enforce Test Tier Purity Invariant with untagged grep-guard`

## Batch Tests

`verify:` runs the new guard by name (`-run TestTierPurity`) in `cmd/lyx`
with `-count=1`. Green means the whole tree is clean: batch 2 removed every
untagged spawner, and the guard's own allowlist covers the two deliberate
exceptions. The guard's fail-first property was demonstrated pre-batch-2 by
construction (the five packages it now passes on were failing matches — see
`_mill/discussion.md` § test-tier-purity-invariant); no separate red run is
required in this batch. The module-wide overview verify (`go test ./...
-count=1`) additionally proves the guard runs and passes inside the normal
Tier 1 loop.
