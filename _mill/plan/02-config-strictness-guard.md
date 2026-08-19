# Batch: config-strictness-guard

```yaml
task: "invariants and docs for the told-geometry rule"
batch: "config-strictness-guard"
number: 2
cards: 3
verify: go test ./cmd/lyx/...
depends-on: [1]
```

## Batch Scope

This batch builds the Config Strictness set-equality grep guard that `CONSTRAINTS.md`'s Config Strictness Invariant already specifies and names this task as the home for, allowlists it in the Test Tier Purity guard, and flips that invariant's **Enforced by** line from review obligation to the new test.
It is one batch because the test, its allowlist entry, and the invariant edit are a single indivisible change — landing any one alone leaves the tree failing a guard or documenting a test that does not exist.

It depends on batch 1 only because both batches edit `CONSTRAINTS.md` and batch 1 restructures the region above;
the two edits are in different sections and do not conflict, but serialising them keeps the diffs reviewable.

The `verify:` command is `go test ./cmd/lyx/...` rather than a single test file because the new guard and the Test Tier Purity guard it must satisfy both live in `package main` under `cmd/lyx`, and Go's unit of test compilation is the package.

Batch-local decision: the guard is written against the two pinned sets **as `CONSTRAINTS.md` records them**, not against whatever the tree currently says.
Those sets were verified against the tree while this plan was written and they match exactly — `configengine.LoadOrTemplate(` appears in `internal/websterengine`, `internal/shuttleengine`, `internal/batcher`, `internal/reedengine`, `internal/perchengine`, and `configengine.Load(` appears in `internal/fabricengine`, `internal/loomengine`, `internal/boardengine`.
If the first run nonetheless disagrees, the pinned sets in both the test and `CONSTRAINTS.md` are what is wrong: verify against the tree and correct both in the same commit rather than loosening the assertion.

## Cards

### Card 4: the Config Strictness set-equality guard

- **Context:**
  - `cmd/lyx/gitrepoboundary_test.go`
  - `cmd/lyx/tierpurity_test.go`
  - `cmd/lyx/crosscompile_test.go`
  - `CONSTRAINTS.md`
  - `internal/configengine`
  - `internal/websterengine/config.go`
  - `internal/shuttleengine/config.go`
  - `internal/batcher/config.go`
  - `internal/reedengine/config.go`
  - `internal/perchengine/config.go`
  - `internal/fabricengine/config.go`
  - `internal/loomengine/config.go`
  - `internal/boardengine/config.go`
- **Edits:** none
- **Creates:**
  - `cmd/lyx/configstrictness_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `cmd/lyx/configstrictness_test.go` in `package main`, following `cmd/lyx/gitrepoboundary_test.go`'s pinned-set style throughout: a file-level doc comment stating what the guard enforces and its blind spots, package-level pinned-set variables with their own doc comments, a vacuous-scan floor constant, and one exported test function.

  Declare two pinned sets as package-level `map[string]bool` variables keyed by module-relative, slash-separated **package directory** (e.g. `internal/reedengine`), each with a doc comment naming the invariant:
  the degrading set is `internal/shuttleengine`, `internal/reedengine`, `internal/perchengine`, `internal/websterengine`, `internal/batcher`;
  the strict set is `internal/fabricengine`, `internal/boardengine`, `internal/loomengine`.

  Declare the test function `TestConfigStrictness_PinnedCallSiteSets`.
  It must:

  - Skip cleanly when the go toolchain is absent from `PATH`, using the same `exec.LookPath("go")` + `t.Skip` shape `cmd/lyx/gitrepoboundary_test.go`, `cmd/lyx/crosscompile_test.go`, and `cmd/lyx/tierpurity_test.go` already use.
  - Resolve the module root through `go env GOMOD` (`exec.Command("go", "env", "GOMOD")`), skipping cleanly when the output is empty or `os.DevNull`, exactly as `cmd/lyx/gitrepoboundary_test.go` does.
  - Walk every non-test `*.go` file under the module root with `filepath.WalkDir`, skipping directory entries whose name begins with `.` and any `vendor` directory, and skipping any file whose name ends in `_test.go`.
  - For each scanned file, record its containing package directory (module-relative, slash-separated) into the collected-degrading set when the file's contents contain the substring `configengine.LoadOrTemplate(`, and into the collected-strict set when they contain `configengine.Load(`.
    Substring matching is deliberate and matches the specification;
    note in the doc comment that `configengine.LoadOrTemplate(` does not also satisfy the `configengine.Load(` substring, since the two tokens differ before the open paren.
  - Exclude `internal/configengine` itself from both collected sets as the declaration site.
  - Assert set equality in **both** directions for each pair, so a pinned-set member with no matching call anywhere fails just as loudly as an unpinned package that gained a call.
    Emit a diff naming the missing and the unexpected package directories, in the style of `cmd/lyx/gitrepoboundary_test.go`'s `diffMethodSets`.
    Declare your own helper rather than reusing `diffMethodSets` — its failure text is worded for that guard — and give it a distinct name such as `configStrictnessDiffSets` so `package main` has no redeclaration conflict.
  - Carry a vacuous-scan floor: fail rather than pass when the walk scanned implausibly few non-test `*.go` files, following the `gitrepoBoundaryMinScannedFiles` pattern with its own distinctly-named constant.

  The file's doc comment must record three things the invariant's own text is careful about:

  - The three own-loader modules — `internal/burlerengine`, `internal/modelspec`, `internal/scoutengine` — call neither entry point and are structurally invisible to a substring scan, so they need no exclusion.
  - This guard's own file is skipped as a `_test.go` file, so the literal `configengine.Load(` and `configengine.LoadOrTemplate(` tokens it carries as scan data are harmless — matching how the other pinned-set guards document the same self-reference.
  - The known blind spot the invariant already records: a substring scan cannot see a call reached through an alias or a function value.

  Assert nothing about `internal/burlerengine`, `internal/modelspec`, or `internal/scoutengine` — they are outside this guard's subject by construction, not by exclusion.
- **Commit:** `test(cmd/lyx): add the Config Strictness pinned-call-site guard`

### Card 5: allowlist the new guard in the Test Tier Purity map

- **Context:**
  - `cmd/lyx/configstrictness_test.go`
- **Edits:**
  - `cmd/lyx/tierpurity_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add one entry to the `allowedSpawners` map in `cmd/lyx/tierpurity_test.go`, keyed `"cmd/lyx/configstrictness_test.go"`, with a one-line reason in the style of the fourteen entries already there.
  The reason must name the actual cause: the guard resolves its scan root via `go env GOMOD`, so it contains `exec.Command`.
  Mention that it is the Config Strictness Invariant guard, matching how the neighbouring entries name their own invariant.

  Keep the map's existing gofmt alignment intact — the entries are key-aligned, so adding a longer key realigns the block.
  Run `gofmt` on the file (or let `go test` surface the formatting) rather than hand-aligning.

  Change nothing else in the file: no new banned token, no change to `knownTierTags`, no change to the existing entries' reasons.
- **Commit:** `test(cmd/lyx): allowlist configstrictness_test.go in the tier-purity spawner map`

### Card 6: flip the Config Strictness Invariant's Enforced by line

- **Context:**
  - `cmd/lyx/configstrictness_test.go`
  - `cmd/lyx/tierpurity_test.go`
  - `cmd/lyx/gitrepoboundary_test.go`
  - `internal/configengine`
- **Edits:**
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In the `## Config Strictness Invariant` section, replace the entire final **Enforced by** bullet — all three of its lines — with a new **Enforced by** line naming the shipped guard.

  The bullet to replace, in full, byte-exactly as it stands today:

```
- **Enforced by** review obligation today, with a set-equality grep guard named as a candidate and T10 named as its home.
  The guard's shape, recorded here so T10 inherits a specification rather than re-deriving one: following `cmd/lyx/gitrepoboundary_test.go`'s pinned-set style, walk non-test `*.go` files under the module root, collect every package directory containing a `configengine.Load(` call and every one containing a `configengine.LoadOrTemplate(` call, compare each collected set against its pinned set, exclude `internal/configengine` itself as the declaration site, and skip `_test.go` files.
  Resolving the scan root through `go env GOMOD` spawns a process, so the new guard must be allowlisted in `cmd/lyx/tierpurity_test.go`'s `allowedSpawners` map alongside the entries already there for that same reason, with a one-line reason in their style.
```

  The replacement is a single bullet naming `cmd/lyx/configstrictness_test.go` and its test function `TestConfigStrictness_PinnedCallSiteSets`, in the same **Enforced by** style every other section of the file uses.

  The two specification lines are **deleted, not compressed and not kept**.
  Once the guard exists they are a specification for work already done, addressed to a task that has completed — exactly the historical narrative the file's own header bans — and the task-number reference becomes a dangling pointer.
  The guard's shape lives in the new test file's own doc comment from then on, matching how `cmd/lyx/gitrepoboundary_test.go` and the other pinned-set guards document themselves.

  Do not touch the separate known-blind-spot bullet immediately above it ("a substring scan cannot see a call reached through an alias or a function value").
  It is a live caveat about the shipped guard rather than a record of how the guard came to be, and it stays exactly as written.

  Leave the two pinned sets stated in the invariant's own text unchanged unless card 4's first run proved them wrong against the tree, in which case correct the invariant's sets and the test's sets together in this same commit.
- **Commit:** `docs(constraints): flip Config Strictness enforcement onto the shipped guard`

## Batch Tests

`verify: go test ./cmd/lyx/...` compiles and runs `package main`'s whole untagged test set, which is exactly the surface this batch changes:
the new `TestConfigStrictness_PinnedCallSiteSets` proves the two pinned sets match the tree, and `TestTierPurity_UntaggedTestsSpawnNothing` in `cmd/lyx/tierpurity_test.go` proves the new guard's `allowedSpawners` entry is present and correctly keyed — without it, the new file's `exec.Command` trips the tier-purity guard and the batch fails.

The scope is the package rather than a single file because `cmd/lyx` is one Go package and the two guards that must both pass live in it.
No other package is affected: the batch adds one test file, edits one test file, and edits one markdown file that no test parses.

`CONSTRAINTS.md`'s edit has no runnable surface of its own — it is covered by review, and by `internal/lyxcwd/docslink_test.go` in the batches that link into it.
