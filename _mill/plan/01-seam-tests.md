# Batch: seam-tests

```yaml
task: 'Scoutengine: rewrite CONSTRAINTS.md as a seam rule, convert leaf test to banned-list, add LSP guard'
batch: seam-tests
number: 1
cards: 3
verify: go test ./internal/scoutengine/ && go vet -tags scout ./internal/scoutengine/
depends-on: []
```

## Rename mechanic

_Include this section in any batch that contains at least one non-empty `Moves:` field.
The `move-mechanic-missing` validator check enforces this requirement.
For each `Moves:` pair the implementer MUST:_

1. _Run `git mv <old> <new>` FIRST, before making any other change to the moved file._
2. _Make ONLY surgical edits -- touch only the lines that must change after the move (package or module declaration, imports, identifier retargeting, seam splits)._
3. _Use a full-file `Creates:` entry only for genuinely new files that have no predecessor._
4. _Never write the relocated file from scratch and delete the original -- that breaks git rename history and inflates review diffs._

## Batch Scope

This batch delivers the whole test surface of the task: the package-wide import test is renamed and converted from an allowlist to a four-predicate banned list, a new narrow file-scoped guard is added for the ported stdio LSP client, and both are proven to actually fail.
It is one batch because the three cards are a single mechanical unit: the `isStdlib` heuristic physically moves from the converted file into the new guard file, and card 3 cannot prove either guard red until both exist.
Cards 1 and 2 do each carry their own commit, so the tree between them briefly has the heuristic in neither file — that transient state is harmless, since nothing in the repo references the heuristic and card 1's own commit already compiles and passes without it.
Splitting the cards across *batches* is what the plan avoids, because a batch boundary would put a verify gate and a review round in the middle of one indivisible move.
The external interface batch 2 consumes is the pair of final names: `TestEngineSeamInvariant_BannedImports` in `seam_enforcement_test.go` and `TestLSPClientGuard_StdlibAndLoggerOnly` in `lspclient_guard_test.go`, which the already-committed `CONSTRAINTS.md` "Enforced by" line names and card 6 verifies.

Batch-local decisions beyond `## Shared Decisions`:

- The converted test scans its own package directory with `os.ReadDir`, not `filepath.WalkDir`, matching the `internal/shuttleengine` model.
  The two are behaviourally identical today (the package has no subdirectories);
  the difference is that a future subpackage will not silently inherit the seam rule.
- All four predicates report the bare file name rather than the absolute path the old walk handed them.
  The package is implied by the test's own location, so the file name is the useful part.
- Both tests assert non-vacuity and `t.Fatal` rather than passing silently on an empty scan.
  This deliberately does not inherit the `internal/shuttleengine` model's behaviour, which passes silently.

## Cards

### Card 1: Rename the package import test and convert it to a banned list

- **Context:**
  - `internal/scoutengine/leaf_enforcement_test.go`
  - `internal/shuttleengine/seam_enforcement_test.go`
  - `CONSTRAINTS.md`
- **Edits:** none
- **Creates:** none
- **Deletes:** none
- **Moves:**
  - `internal/scoutengine/leaf_enforcement_test.go` -> `internal/scoutengine/seam_enforcement_test.go`
- **Requirements:**
  Run the `git mv` first, per `## Rename mechanic`, then edit the relocated file in place.

  In the relocated file:

  - Delete the `allowedImports` map variable entirely.
  - Rename the test function `TestLeafInvariant_AllowlistOnly` to `TestEngineSeamInvariant_BannedImports`.
  - Replace the `filepath.WalkDir` traversal with an `os.ReadDir` enumeration of `pkgDir`, keeping the existing `runtime.Caller(0)` + `filepath.Dir` package-directory resolution unchanged.
    Skip entries where `entry.IsDir()` is true, where the name has suffix `_test.go`, or where the name does not have suffix `.go`.
  - Delete the stdlib heuristic (the `firstSegment` / `isStdlib` block).
    A banned list has no notion of stdlib.
    Card 2 re-establishes that heuristic in the guard test, its sole remaining consumer.
  - Delete the trailing catch-all `failures = append(failures, relPath+": "+importPath)` and its `filepath.Rel` call.
    Under a banned list it is not merely dead code — it would flag `internal/logger` and `gopkg.in/yaml.v3` as violations.
    Only the four predicates below may append a failure.
  - Keep the three existing violation predicates and add a fourth, all four appending `entry.Name()` rather than the absolute path:

  ```go
  		for _, imp := range astFile.Imports {
  			importPath := strings.Trim(imp.Path.Value, `"`)

  			if importPath == "github.com/Knatte18/loomyard/internal/output" {
  				failures = append(failures, entry.Name()+": banned internal/output import (engine must stay io.Writer/exit-code-free)")
  				continue
  			}
  			if strings.Contains(importPath, "spf13/cobra") {
  				failures = append(failures, entry.Name()+": banned cobra import (engine must stay cobra-free)")
  				continue
  			}
  			if strings.Contains(importPath, "/internal/") && strings.HasSuffix(importPath, "cli") {
  				failures = append(failures, entry.Name()+": banned internal/*cli import (cli imports engine, never the reverse)")
  				continue
  			}
  			if importPath == "github.com/Knatte18/loomyard/internal/clihelp" {
  				failures = append(failures, entry.Name()+": banned internal/clihelp import (carries cobra without the *cli suffix)")
  				continue
  			}
  		}
  ```

  - Count the non-test `.go` files actually parsed and `t.Fatal` when that count is zero, so the test cannot go green by finding nothing to check.
  - Rewrite the closing `t.Errorf` message.
    It currently names the allowlist ("imports outside the allowlist (stdlib + configengine + lock + proc + logger + yaml.v3)").
    The replacement names the invariant and the banned imports found, and mentions no allowlist and no enumerated allowed set — for example: `Scout Engine-Seam Invariant violated; banned imports found: %v`.
  - Rewrite the file's header comment (currently lines 1-7) to the seam / banned-list framing.
    It must state the seam rule, name `CONSTRAINTS.md`'s "Scout Engine-Seam Invariant" as the recorded invariant, and note that the check covers direct imports only, never the transitive closure.
    It must drop the enumerated allowlist and must drop the sentence claiming this check "keeps the LSP subprocess client stdlib-only" — that property belongs to card 2's guard, and even there it is stdlib plus logging, not stdlib-only.
  - Fix up the import block to the set the rewritten file actually uses: `go/parser`, `go/token`, `os`, `path/filepath`, `runtime`, `strings`, `testing`.
    `io/fs` is no longer needed once `filepath.WalkDir` is gone.
  - The file carries no `//go:build` tag and must contain none of the tokens `exec.Command`, `exec.CommandContext`, `gitexec.RunGit`, or `lyxtest.Copy`, even in a comment.

  Do not add any `_test.go`-scoped exception, allowlist, or "unrecognised import" reporting path.
- **Commit:** `test(scoutengine): convert the leaf allowlist test into a seam banned-list test`

### Card 2: Add the file-scoped guard for the stdio LSP client

- **Context:**
  - `internal/scoutengine/lspclient.go`
  - `internal/lyxcwd/raddle_guard_test.go`
  - `CONSTRAINTS.md`
- **Edits:** none
- **Creates:**
  - `internal/scoutengine/lspclient_guard_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create the new file in package `scoutengine`, with no `//go:build` tag, containing exactly one test function, `TestLSPClientGuard_StdlibAndLoggerOnly`.

  The test must:

  - Resolve its own package directory with `runtime.Caller(0)` + `filepath.Dir`, then build the target path with `filepath.Join(pkgDir, "lspclient.go")`, so the guard is independent of the working directory `go test` runs from.
  - `t.Fatal` when that target path does not exist, rather than passing on zero files scanned.
    A guard keyed to a file name must fail loudly when the file name moves.
  - Parse the target with `go/parser` in `parser.ImportsOnly` mode — never grep — so import-path strings appearing in the file's own doc comments cannot produce false positives.
  - Re-establish the stdlib heuristic deleted in card 1, unchanged in behaviour: an import path is stdlib when the segment before its first `/` contains no `.` character.
    Keep the explanatory comment that a registered-TLD domain always contains a dot.
  - Treat exactly one non-stdlib import as allowed: `github.com/Knatte18/loomyard/internal/logger`.
    Every other non-stdlib import is a failure, collected into a `failures []string` slice reported by a single closing `t.Errorf` naming the invariant and the offending imports.
  - Describe the property, in the file's header comment, in the test function's own comment, and in the `t.Errorf` message, as "no lyx dependency except logging" — pinning the ported stdio LSP client as liftable back out of lyx behind a single logging dependency.
    The strings "stdlib-only" and "hermetic" must not appear as an assertion about the file.
    `internal/logger` itself reaches `internal/lyxcwd` and `internal/proc`, so the file is neither.
  - Contain none of the tokens `exec.Command`, `exec.CommandContext`, `gitexec.RunGit`, or `lyxtest.Copy`, even in a comment or string literal.

  The guard covers this one file only.
  The guard must not touch `internal/scoutengine/probe.go` or any other file in the package, and must not be generalized into a per-file allowed-set table — that is an allowlist through the back door.

  The test must pass on the target as it stands today, including its existing `internal/logger` import.
- **Commit:** `test(scoutengine): guard the stdio LSP client to stdlib plus logging`

### Card 3: Prove both guards red

- **Context:**
  - `internal/scoutengine/probe.go`
  - `internal/scoutengine/lspclient.go`
  - `internal/scoutengine/seam_enforcement_test.go`
  - `internal/scoutengine/lspclient_guard_test.go`
- **Edits:** none
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  This card produces no net diff.
  Every edit it makes is temporary and must be reverted before the card ends;
  finish by confirming `git status --porcelain` reports a clean tree.

  Prove the converted seam test red by temporarily adding one banned import at a time to `internal/scoutengine/probe.go`, running the package's tests, confirming the failure message names that specific predicate rather than a generic mismatch, then reverting.
  Cover all four: `github.com/Knatte18/loomyard/internal/output`, a cobra import, an `internal/*cli` import, and `github.com/Knatte18/loomyard/internal/clihelp`.
  The `internal/clihelp` case matters most — it is the predicate with no counterpart in the old file, and the hole a pure banned list would otherwise open.

  Prove the specific non-regression the follow-up task depends on: temporarily add a `github.com/Knatte18/loomyard/internal/lyxcwd` import to `internal/scoutengine/probe.go`, confirm the seam test stays green, then revert.
  A red result here means a predicate is over-broad and the whole conversion is wrong.

  Prove the seam test's non-vacuity assertion fires — either by temporarily pointing its directory scan at an empty directory or by asserting the scanned-file count directly — then revert.

  Prove the guard red by temporarily adding, one at a time, a second lyx import to `internal/scoutengine/lspclient.go` that the package-level banned list permits (for example `github.com/Knatte18/loomyard/internal/lock` or `github.com/Knatte18/loomyard/internal/configengine`) and a third-party import (for example `gopkg.in/yaml.v3`), confirming the guard fails in each case while the seam test stays green, then reverting.
  That divergence between the two tests is the guard's entire reason to exist.

  Prove the guard's missing-target path by temporarily renaming `internal/scoutengine/lspclient.go`, confirming `internal/scoutengine/lspclient_guard_test.go` calls `t.Fatal` rather than passing, then restoring the name.

  Record the outcome of each check in the batch's implementation notes.
  Do not leave any temporary import, renamed file, or scratch file behind, and do not add a permanent negative-case test — the guards stay assertion-only.
- **Commit:** none

## Batch Tests

`verify:` runs `go test ./internal/scoutengine/` followed by `go vet -tags scout ./internal/scoutengine/`.

The first command is the real gate: it compiles the whole package's untagged test surface (which is where both new tests live) and runs it in under two seconds.
Green here means the converted seam test accepts `internal/configengine`, `internal/lock`, `internal/proc`, `internal/logger`, and `gopkg.in/yaml.v3` — the conversion's whole point — and that the guard accepts `internal/scoutengine/lspclient.go` as it stands.

The second command exists because the package has five `//go:build scout` test files that no pipeline gate compiles.
This batch changes the package's test surface (one file renamed, one added), so the tagged build is verified by hand here rather than being discovered broken much later.
It passes on the tree as it stands, so any failure is this batch's doing.

Repo-wide coverage — confirming no other package referenced the renamed file or the renamed function — is left to the configured done gate (`go test ./... && go test -tags integration ./...`), which mill-go runs from the repository root before marking the task done.
Duplicating it per batch would add minutes to every implementer and fixer round for no extra signal.

Card 3's proofs are manual and deliberately leave no artifact;
`verify:` cannot observe them, which is why the card requires a clean `git status --porcelain` as its own closing check.
