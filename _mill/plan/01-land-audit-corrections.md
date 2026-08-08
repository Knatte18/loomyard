# Batch: land-audit-corrections

```yaml
task: "Audit the remaining leaf and seam import invariants"
batch: "land-audit-corrections"
number: 1
cards: 5
verify: go build ./... && go test ./internal/lyxtest/... ./internal/modelspec/... ./internal/treadleengine/... ./internal/tokenvocab/... ./internal/pattern/... ./internal/shuttleengine/... ./internal/githubclient/...
depends-on: []
```

## Batch Scope

This batch lands every correction the audit found, and nothing else.
It is one batch because the whole task is one coherent correction pass over a handful of comment sites plus a single test-mechanism conversion, and because the two `CONSTRAINTS.md` rule-text corrections already landed during mill-start — what remains would not fill a second batch.
The five cards are ordered so the load-bearing change comes first: card 1 converts lyxtest's enforcement test from a denylist to an allowlist, renames it, and updates the two sites that name the function by name, all in one commit.
Cards 2-4 fix the comment sites that restate a corrected rule.
Card 5 is a zero-diff gate that re-runs the audit's sweep and the full regression bar.

There is no external interface for a later batch to consume — this is the only batch.

Batch-local notes beyond `## Shared Decisions` in the overview:

- **Two `CONSTRAINTS.md` edits are already committed and must not be redone.**
  The Treadle Runner-Seam import-allowlist bullet already carries the two sentences about transitive reachability and the told-never-derived posture, and the lyxtest Leaf Invariant opening sentence is already an allowlist statement.
  Card 1 makes a **third**, different `CONSTRAINTS.md` edit: the lyxtest section's "**Enforced by**" line.
  Do not touch the two already-corrected passages.
- **A known transient inconsistency exists on this branch and must not be "fixed" by reverting.** `CONSTRAINTS.md` already describes lyxtest's check as an allowlist while `internal/lyxtest/leaf_enforcement_test.go` is still a denylist.
  The file is deliberately ahead of the test for the window between the mill-start commit and card 1's commit.
  Card 1 closes the gap.
- **Use `internal/pattern/leaf_enforcement_test.go` as the single shape reference** for the conversion.
  Not scoutengine's — see the overview's scoutengine decision.

## Cards

### Card 1: Convert lyxtest's leaf enforcement test to an allowlist, rename it, and retarget both sites naming the function

- **Context:**
  - `internal/pattern/leaf_enforcement_test.go`
  - `internal/lyxtest/lyxtest.go`
  - `internal/lyxtest/hermetic.go`
  - `internal/lyxtest/reexecguard.go`
- **Edits:**
  - `internal/lyxtest/leaf_enforcement_test.go`
  - `CONSTRAINTS.md`
  - `internal/shuttleengine/seam_enforcement_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Four changes, all in one commit.

  **(a) Convert the check in `internal/lyxtest/leaf_enforcement_test.go` from a denylist to an allowlist.**
  Delete the `bannedImports` slice and the nested inner loop that compares each import against it.
  Replace them with a package-level `allowedImports map[string]bool` declared above the test function, holding exactly three entries: `github.com/Knatte18/loomyard/internal/configengine`, `github.com/Knatte18/loomyard/internal/lyxcwd`, and `github.com/Knatte18/loomyard/internal/weftname`.
  These are the complete non-stdlib import set of the package's four production files — only `internal/lyxtest/lyxtest.go` has internal imports at all;
  `internal/lyxtest/hermetic.go` and `internal/lyxtest/reexecguard.go` are stdlib-only.
  Copy the allowlist shape verbatim from `internal/pattern/leaf_enforcement_test.go`: keep the `runtime.Caller(0)` + `filepath.Dir` directory resolution, the `filepath.WalkDir` skipping directories, `*_test.go`, and non-`.go` files, and the `go/parser.ParseFile(..., parser.ImportsOnly)` parse.
  Add pattern's stdlib test — take the first path segment (up to the first `/`), and treat the import as stdlib when that segment contains no `.` character.
  Continue on stdlib or an `allowedImports` hit;
  otherwise append `relPath + ": " + importPath` to `failures`.
  Keep the single trailing `t.Errorf` and make its message name the allowed set, in the style of pattern's own message.
  Do **not** extract a table-driven matcher helper to test the check directly — no sibling enforcement test does that, and introducing the pattern in the one file this task exists to bring into line with its siblings would be self-defeating.
  Do not add a `//go:build` tag: this test stays untagged, like every other enforcement test.

  **(b) Rename the test function and rewrite the two stale comment blocks in the same file.**
  Rename `TestLeafInvariant` to `TestLeafInvariant_AllowlistOnly`.
  Rewrite the file-header comment: today it says the file enforces the invariant by naming banned imports, which the conversion makes false.
  Rewrite the function doc comment: today it claims lyxtest "imports only stdlib and internal/lyxcwd", which is stale on two counts — `weftname` and `configengine` are both imported and both legitimate.
  Both rewrites must keep, in **one clause**, the reason the allowlist must never be widened: feature packages' own tests import lyxtest, so a reverse import would close a test-build cycle.
  The conversion replaces the *mechanism*, never the *reason* — and an allowlist is easier to widen than a denylist, so the reason to resist belongs at the edit site rather than one file away in `CONSTRAINTS.md`.
  Keep the note that `go/parser` with `ImportsOnly` is used so string literals in doc comments cannot produce false positives.

  **(c) Update the "Enforced by" line in `CONSTRAINTS.md`.**
  In the lyxtest Leaf Invariant section, the line currently names the file only.
  Add the test-function name in parentheses after the path so it reads in the same shape as the six sibling sections that already name their test function — compare the Modelspec Leaf Invariant and Treadle Runner-Seam Invariant "Enforced by" lines in the same file.
  This edit must land in the same commit as the rename in (b);
  splitting them would leave `CONSTRAINTS.md` naming a function that does not yet exist.
  Change nothing else in `CONSTRAINTS.md` — in particular, leave the lyxtest section's opening sentences and the Treadle Runner-Seam import-allowlist bullet exactly as they are.

  **(d) Update the style cross-reference in `internal/shuttleengine/seam_enforcement_test.go`.**
  The doc comment on `TestProviderSeamImportRule` cites lyxtest's test by function name.
  Change that name to `TestLeafInvariant_AllowlistOnly` and change nothing else on the line.
  Keep the citation pointing at lyxtest — do **not** retarget it to pattern or modelspec.
  The comment cites lyxtest as the origin of the `go/parser` `ImportsOnly` idiom, and lyxtest genuinely is that origin: modelspec's test cites lyxtest, and pattern's in turn cites modelspec, so retargeting would name a file downstream of the citation chain it would be claiming to head.
  Two distinct things are in play and must not be conflated — lyxtest originates the *idiom*, and it is separately adopting pattern's allowlist *shape*.
  This card changes only the second.

  **(e) Run the negative control before committing, and record it in the commit body.**
  Temporarily add a **blank** import line to a production file in the package — the literal form `_ "github.com/Knatte18/loomyard/internal/logger"` — then run `go test ./internal/lyxtest/` and confirm the new allowlist test **fails** naming that import.
  Then revert the temporary import and confirm the test passes again.
  The form matters: a named import would be an unused-import compile error, so the run would report a build failure and never reach the guard test, whereas a blank import compiles and `parser.ImportsOnly` still records it in the parsed file's imports, so the guard sees it exactly as it would see a real stray dependency.
  The package matters too: `internal/logger` closes no cycle (it does not import lyxtest), whereas any feature package would produce a genuine test-build cycle and again fail at build time rather than at the guard.
  Do not leave the temporary import in the tree — verify with `git status` that the working tree is clean of it before committing.

  **(f) The commit body is mandatory content, not the implementer's choice.**
  Use the subject line in `Commit:` below, then a blank line, then this body verbatim:

  ```text
  Audit of every CONSTRAINTS.md section that constrains one package's whole
  import set and names a dedicated enforcement test. Seven such sections were
  audited (scoutengine is excluded — the parallel scout-seam-conversion task
  owns it). Verdicts:

    lyxtest Leaf           — keep; rule text asserted an import set the check
                             never enforced. Converted denylist to allowlist
                             (stdlib + configengine, lyxcwd, weftname) and
                             renamed TestLeafInvariant to
                             TestLeafInvariant_AllowlistOnly.
    Modelspec Leaf         — keep, zero cost; lyxcwd absent from the closure.
    Treadle Runner-Seam    — keep and sharpen; rule text corrected.
    Tokenvocab Leaf        — keep; permits lyxcwd outright, not scout-shaped.
    Pattern Leaf           — keep; permits lyxcwd outright, not scout-shaped.
    Shuttle Provider-Seam  — keep; interface segregation, different family.
    GitHub Auth            — keep, zero cost; real reason never to need lyxcwd.

  Treadle evidence: the allowlist excludes internal/lyxcwd as a direct import
  but permits internal/logger AND internal/shuttleengine, each of which
  imports lyxcwd directly. Two independent transitive paths, so the exclusion
  buys no isolation at the dependency-graph level. What it does enforce is
  real and unchanged: treadle is told its geometry and never derives it —
  Engine.Run takes a caller-supplied absolute runDir, a block's Profile
  carries a caller-supplied GateDir, and every filepath.Join in the package
  joins onto one of those told values.

  Found and deliberately NOT fixed here — internal/lyxcwd's own import cap.
  CONSTRAINTS.md states lyxcwd's imports are capped at stdlib plus
  internal/gitexec. That is true today, but it is enforced by nothing:
  internal/lyxcwd/enforcement_test.go checks comment stripping, geometry
  literals, and fabric vocabulary, and never lyxcwd's own import set. This is
  exactly the lyxtest finding's shape. Worse, docs/shared-libs/lyxcwd.md
  asserts "Dependency direction (Go enforces it)" — Go enforces acyclicity,
  not the cap, so a stray non-cyclic import would pass silently while the doc
  claims the compiler prevents it. Deferred because the rule is a bullet
  inside the Cwd Resolution Invariant rather than a section meeting the audit
  criterion, and because it sits in the CONSTRAINTS.md region
  scout-seam-conversion is actively working. It warrants a follow-up task:
  add an allowlist enforcement test for internal/lyxcwd (stdlib + gitexec)
  and correct the "Go enforces it" claim.

  Sweep blind spot, so a re-run is not mistaken for exhaustive: the sweep's
  overstatement pattern (never imports|does not import|imports only) does not
  match "capped at" or "Go enforces it" phrasings. The lyxcwd doc claim above
  was found by reading, not by the sweep. Extending the sweep means adding
  those phrasings.

  Negative control for the new allowlist: a blank import of
  internal/logger was temporarily added to a production file, the test was
  confirmed to fail naming it, and the import was reverted.
  ```

- **Commit:** `test(lyxtest): convert leaf invariant to an allowlist and rename to TestLeafInvariant_AllowlistOnly`

### Card 2: Rewrite the lyxtest package doc's Leaf Invariant paragraph

- **Context:**
  - `CONSTRAINTS.md`
  - `internal/lyxtest/leaf_enforcement_test.go`
- **Edits:**
  - `internal/lyxtest/doc.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `internal/lyxtest/doc.go`, rewrite **only** the denylist framing inside the "Leaf Invariant:" paragraph of the package doc comment.
  That is the opening sentence asserting the package is policed by a banned-imports list rather than an allowlist, together with the sentences that continue it — the one stating the import set as prose and the one enumerating the packages it must not import.

  Replace them with an allowlist statement that mirrors the corrected `CONSTRAINTS.md` sentence: production code imports only stdlib, `internal/lyxcwd`, `internal/weftname`, and `internal/configengine`, with `internal/configreg` and the feature packages excluded by construction.
  Keep the test-build-cycle reason in one clause — feature packages' own tests import lyxtest, so a reverse import would close the cycle.
  Name the enforcing test the way sibling package docs do, using the function name `TestLeafInvariant_AllowlistOnly` that card 1 introduces.

  The `SeedConfig` sentence that follows the paragraph — the one about seeding real configuration from a configreg-free map of module name to YAML content — is accurate today, mirrors the `CONSTRAINTS.md` bullet, and is unaffected by the conversion.
  Leave it exactly as it is.
  Leave the package summary above the paragraph and the "Hermetic Git Test Environment" section below it untouched.

  Format the new text as semantic line breaks matching the reflow the repo already applied to Go doc comments.
- **Commit:** `docs(lyxtest): restate the Leaf Invariant as an allowlist in the package doc`

### Card 3: Remove the isolation reading from treadleengine's two geometry-blindness comments

- **Context:**
  - `CONSTRAINTS.md`
  - `internal/treadleengine/seam_enforcement_test.go`
  - `internal/lyxcwd/enforcement_test.go`
- **Edits:**
  - `internal/treadleengine/doc.go`
  - `internal/treadleengine/engine.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Both sites state or imply that not importing lyxcwd gives treadle isolation from cwd resolution.
  It does not — the package's own import allowlist permits `internal/logger` and `internal/shuttleengine`, each of which imports lyxcwd directly.
  Correct the framing at both sites so the claim made is the true one: the ban is on the **direct** import, and what it enforces is that treadle is told its geometry rather than deriving it.

  **(a) `internal/treadleengine/doc.go`** — the "Geometry-blindness and fabric-blindness" section opens by asserting the package never imports lyxcwd and never constructs a `_lyx` path itself, then correctly describes the told-never-derived mechanism in the clause that follows.
  Only the opening framing is wrong.
  Reword it to say the package never imports lyxcwd **directly** and never constructs a `_lyx` path itself, and make clear that the direct-import ban is a discipline rather than an isolation guarantee.
  Leave the rest of the section — the `runDir` / `Profile` / `GateDir` mechanism sentence and the fabric-git sentences after it — unchanged.

  **(b) `internal/treadleengine/engine.go`** — the `Engine` type doc carries a compressed version of the same claim, saying the type never imports fabricengine or lyxcwd and never constructs a `_lyx` path.
  Apply the same correction: the lyxcwd half becomes a direct-import statement.
  Do not touch the fabricengine half — that claim is true and was verified during the audit.
  The surrounding prose already carries the mechanism, so neither site needs the full two-sentence treatment the `CONSTRAINTS.md` bullet received;
  removing the isolation reading is the whole job.

  Both rewrites are bound by the Fabric Vocabulary Invariant — `internal/treadleengine` is not in its owner set, and its enforcement test scans comments as well as identifiers and string literals.
  Introduce no bare `weft` or `warp` token and no fabric-sense `host` phrase.
  The bare word `fabric` is unpoliced, so the existing "fabric-blind and geometry-blind" phrasing stays as it is.
  Run `go test ./internal/lyxcwd/` after editing to confirm the vocabulary check still passes.
- **Commit:** `docs(treadleengine): scope the lyxcwd claim to direct imports in both geometry-blindness comments`

### Card 4: Delete the dead lyxtest contrast clause from modelspec's enforcement test

- **Context:**
  - `internal/pattern/leaf_enforcement_test.go`
  - `internal/lyxtest/leaf_enforcement_test.go`
  - `internal/shuttleengine/seam_enforcement_test.go`
- **Edits:**
  - `internal/modelspec/leaf_enforcement_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  The file-header comment contrasts this check against lyxtest's, describing lyxtest's as a banned-import denylist.
  Card 1 makes both allowlists, so the contrast is dead.

  Delete the contrast clause outright.
  Do **not** reword it into a comparison with some other file: no *useful* contrast remains among the leaf and seam allowlist tests, all of which now share one shape.
  Note that this is not the same as claiming every enforcement test in the repo is an allowlist — that would be false, since `internal/shuttleengine/seam_enforcement_test.go` is a single-import ban and the `cmd/lyx` guards are grep-style denylists.

  Keep the sentence's actual payload, which stands on its own: this check is an ALLOWLIST, so any import outside the allowed set fails the test and a future stray dependency is caught with no list maintenance required.
  Leave the first sentence — the one naming the Modelspec Leaf Invariant and its allowed import set — untouched.
  Do not touch the sibling files that open with "Like modelspec's ... leaf_enforcement_test.go, this check is an ALLOWLIST";
  those stay correct, and modelspec's own file simply cannot use that construction about itself.
- **Commit:** `docs(modelspec): drop the stale lyxtest denylist contrast from the leaf enforcement test`

### Card 5: Verify the sweep boundary and the full regression bar

- **Context:**
  - `CONSTRAINTS.md`
  - `internal/perchengine/doc.go`
  - `internal/perchengine/engine.go`
  - `internal/burlerengine/doc.go`
  - `internal/treadleengine/seam_enforcement_test.go`
  - `internal/tokenvocab/doc.go`
  - `internal/lyxtest/doc.go`
  - `internal/treadleengine/doc.go`
  - `internal/treadleengine/engine.go`
  - `internal/modelspec/leaf_enforcement_test.go`
  - `internal/shuttleengine/seam_enforcement_test.go`
  - `internal/lyxtest/leaf_enforcement_test.go`
- **Edits:** none
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  A zero-diff gate.
  Change no file;
  if this card finds something wrong, fix it by amending the card that owns the file, not here.

  **(a) Re-run the audit's three sweep passes** over `docs/`, `manifest/`, `internal/`, `cmd/`, and `tools/`, and confirm no comment left in the tree asserts an isolation property the import graph does not provide:
  1. `grep -rn` for treadle, filtered for lyxcwd, geometry, leaf, or seam — treadle claims outside the package.
  2. `grep -rn` for lyxtest, filtered for leaf, banned, allowlist, denylist, or import set — lyxtest rule restatements.
  3. `grep -rn` for the overstatement pattern `never imports`, `does not import`, `imports only` across `.go` and `.md` files repo-wide.

  Expect exactly these results, and treat any additional hit as a finding to route back to cards 1-4.
  The five sites the sweep found are all addressed by this batch: the lyxtest package doc, the lyxtest enforcement test header, treadle's two comment sites, and modelspec's contrast clause.
  Four further sites pattern-match but were verified accurate during the audit and must remain untouched — `internal/perchengine/engine.go`, `internal/perchengine/doc.go`, and two claims in `internal/burlerengine/doc.go`, all asserting the package never imports fabricengine and never constructs a `_lyx` path.
  A `go list -deps` check confirmed fabricengine is absent from both packages' transitive closures.
  Two more are accurate as written and also stay untouched: `internal/treadleengine/seam_enforcement_test.go`, whose header already says "as a direct import" and so already carries the reading card 3 makes explicit, and `internal/tokenvocab/doc.go`, which restates its own rule correctly.

  **(b) Confirm nothing under `internal/scoutengine` was touched** by any card in this batch — run `git diff --name-only` against the batch's base and confirm no scoutengine path appears, and confirm no scoutengine section of `CONSTRAINTS.md` changed.

  **(c) Run the full regression bar**, beyond what the batch `verify:` command covers: `go build ./...`, then the untagged `go test ./...` tier in full.
  The full tier matters here because `internal/lyxtest` is imported by most test packages in the repo, so a mistake in its allowlist surfaces as a compile failure across many of them rather than inside lyxtest's own package.
  Report the result;
  do not commit.
- **Commit:** none

## Batch Tests

The batch `verify:` command is `go build ./...` followed by `go test` over all seven audited packages: `internal/lyxtest`, `internal/modelspec`, `internal/treadleengine`, `internal/tokenvocab`, `internal/pattern`, `internal/shuttleengine`, and `internal/githubclient`.
This is the regression bar agreed during discussion, scoped to the packages this batch touches plus the audited siblings whose enforcement tests must stay green.
The `go build ./...` half is not redundant with the test half: every card but one edits a doc comment sitting directly above a `package` clause, where a malformed edit breaks compilation rather than a test assertion.

No new test *cases* are added.
The only test change is `internal/lyxtest/leaf_enforcement_test.go`'s enforcement mechanism, whose correctness has two halves — it must pass unmodified against the package's current four production files, and it must fail on a violation.
The second half is not expressible as a committed test case (a guard test cannot assert on a tree state that does not exist), so card 1 requires the negative control to be run by hand and recorded in the commit body.

The allowlist is strictly stronger than the denylist it replaces: it still rejects every path the old `bannedImports` slice rejected — `internal/configreg`, `boardengine`/`boardcli`, `ideengine`/`idecli`, `selfreportengine`/`selfreportcli`, `fabricengine`/`fabriccli` — by construction, since none of them is in the allowed set, and it additionally rejects anything else outside the three allowed internal packages, which the denylist silently permitted.

Two checks fall outside the batch `verify:` scope and are covered elsewhere.
The Fabric Vocabulary Invariant's comment scan lives in `internal/lyxcwd`, which the seven-package scope does not include, so card 3 runs `go test ./internal/lyxcwd/` explicitly after the treadle comment rewrites.
The full untagged `go test ./...` tier is card 5's job and is also covered by the configured `pipeline.done_gate`, which mill-go runs from the repo root before marking the task done — that is where a lyxtest allowlist mistake rippling into other packages' test builds would surface.

No test is needed for the comment edits themselves.
Their correctness is a review obligation, and the reviewer's checklist is card 5's sweep: no remaining comment in the tree may assert an isolation property that the import graph does not provide.
