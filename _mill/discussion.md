# Discussion: Scoutengine: rewrite CONSTRAINTS.md as a seam rule, convert leaf test to banned-list, add LSP guard

```yaml
task: 'Scoutengine: rewrite CONSTRAINTS.md as a seam rule, convert leaf test to banned-list, add LSP guard'
slug: scout-seam-conversion
status: discussing
parent: main
```

## Problem

`internal/scoutengine` is governed by an import **allowlist** (stdlib, `internal/configengine`, `internal/lock`, `internal/proc`, `internal/logger`, `gopkg.in/yaml.v3`), recorded in `CONSTRAINTS.md` as the "Scoutengine Leaf Invariant" and enforced by `internal/scoutengine/leaf_enforcement_test.go`.
That allowlist excludes `internal/lyxcwd` while scout writes into lyx geometry (`DaemonStateFile`/`DaemonLock` under `.lyx/scout/<lang>/`),
so the anchor has to be resolved in `scoutcli` and threaded as a plain `string` through `Options` → `ensureServer` → `ensureSupervised` before it reaches the two path constructors — split ownership of one path, which the Cwd Resolution Invariant says must not happen.

The allowlist buys nothing in exchange for that rent.
`internal/logger` is already on the allowlist and imports `internal/lyxcwd` and `internal/proc` itself, and these checks police **direct** imports only, never the transitive closure — so `lyxcwd` is already in scoutengine's dependency graph.
The rule enforces naming discipline, not isolation.

**Why now:** the follow-up task `scout-lyxcwd-accessors` (already in the wiki, `depends_on: [scout-seam-conversion]`) re-signatures `DaemonStateFile`/`DaemonLock` to take a `*lyxcwd.Location` and deletes the threading.
It cannot land while the allowlist would reject the `lyxcwd` import it must add.
This task removes that blocker and is docs/tests only — no production code changes.

A parallel active task, `leaf-invariant-audit`, audits the seven **other** leaf/seam invariants in `CONSTRAINTS.md` and is explicitly forbidden from touching the scout section, so there is no same-file collision as long as this task edits only its own section.

## Scope

**In:**

- `CONSTRAINTS.md` — the scout section only, rewritten from "Scoutengine Leaf Invariant" to "Scout Engine-Seam Invariant".
  **Already applied in this branch during mill-start** (see the "CONSTRAINTS.md pre-staged" Decision below) — mill-plan must plan around the file as it now stands, not as the wiki task body describes it.
- `internal/scoutengine/leaf_enforcement_test.go` → renamed to `internal/scoutengine/seam_enforcement_test.go`, converted from allowlist to banned list, test function renamed to `TestEngineSeamInvariant_BannedImports`.
- New file `internal/scoutengine/lspclient_guard_test.go` with `TestLSPClientGuard_StdlibAndLoggerOnly` — a file-scoped guard asserting `lspclient.go` imports stdlib plus `internal/logger` and nothing else.
- `internal/scoutengine/doc.go` — the whole "The engine/CLI split" paragraph (lines 22–34; the allowlist enumeration itself sits at 24–26), which restates the allowlist and is already factually wrong (it omits `internal/logger`, which `ensureserver.go` and `lspclient.go` both import).
  Per `docs/overview.md:362` this package doc **is** scout's module doc — the design doc was deleted on landing — so the repo's docs-land-in-the-same-commit rule binds it.
- `docs/overview.md:252` — the phrase "`internal/scoutengine` is a cycle-free leaf" becomes "a cycle-free engine".
  One-word fix so the overview stops using a term with no invariant behind it.

**Out:**

- **Any production code change.**
  No `.go` file outside `doc.go` (documentation only) and the two test files is edited. `lspclient.go`'s `logger` import stays exactly as it is.
- The `lyxcwd` refactor itself — `DaemonStateFile`/`DaemonLock` signatures, `Options.AnchorRoot`, `resolveAnchorRoot`, `resolveWorktreeRoot`, the `scoutcli` call sites.
  All of that is `scout-lyxcwd-accessors`.
- Every other `CONSTRAINTS.md` section.
  The seven other leaf/seam invariants belong to `leaf-invariant-audit`, which is active in parallel;
  touching them would collide.
- `internal/scoutcli` — untouched entirely.
- The surrounding prose of `docs/overview.md:252` beyond the one-word "leaf" → "engine" swap.
- Adding a guard to `probe.go` or any other scoutengine file.

## Decisions

### CONSTRAINTS.md pre-staged during mill-start

- Decision: the `CONSTRAINTS.md` scout-section rewrite was written **during mill-start**, before the discussion-review rounds, rather than being left for mill-go.
  It is **already committed and pushed on this branch** — first written in `5748a22f` ("mill-start: write discussion.md for scout-seam-conversion") alongside this discussion file, then amended during discussion-review round 2 to drop the word "hermeticity" and to name `internal/clihelp` in the banned-list bullet.
  (A reviewer working from a session-start git snapshot will not see these commits;
  `git log` on the branch is the authority, not the snapshot.
  The section's *content* is verifiable directly in the working tree either way.)
  The section now reads "## Scout Engine-Seam Invariant" with the banned-list framing, the no-allowlist bullet, the file-scoped-guard bullet, and an "Enforced by" line naming both new test paths.
  **mill-go does not re-apply or re-commit that hunk** — no plan card owns it.
  The plan's `CONSTRAINTS.md` obligation is limited to *verifying* the committed section still matches what the tests end up named, and amending only if a later decision changes a test path or function name.
- Rationale: operator instruction. `CONSTRAINTS.md` is authoritative and is read by every reviewer at the start of every session (per the repo's own `CLAUDE.md`).
  If the discussion said "scout has no allowlist" while `CONSTRAINTS.md` still said "imports only stdlib, configengine, lock, proc, logger, yaml", every review round would spend its findings on that contradiction instead of on the actual design.
- Consequence mill-plan must account for: `CONSTRAINTS.md`'s "Enforced by" line already names `seam_enforcement_test.go` (`TestEngineSeamInvariant_BannedImports`) and `lspclient_guard_test.go` (`TestLSPClientGuard_StdlibAndLoggerOnly`), **neither of which exists yet**.
  The tree still compiles and `go test` still passes — the old `leaf_enforcement_test.go` is untouched and green — but the doc is deliberately ahead of the tests until mill-go lands them.
  This is not a defect to be "fixed" by reverting the doc;
  it is pre-staging, and the plan closes the gap.
- Rejected: writing the `CONSTRAINTS.md` change in mill-go with the rest of the task (the normal mill flow) — rejected because it guarantees the contradiction above during review.

### Drop the allowlist entirely; the seam is the whole rule

- Decision: `internal/scoutengine` gets no import allowlist.
  The only import rule is the negative one: never `internal/output`, never `cobra`, never any `internal/*cli` package.
- Rationale: direct peer precedent.
  No other engine module in the repo carries an import **allowlist**;
  they draw freely on the shared-infrastructure layer.
  (This is a claim about allowlists specifically, not about guards in general — `internal/shuttleengine` does have an import guard, a single-import **banned** check, and this task deliberately mirrors it. `internal/boardengine`, `internal/websterengine`, and `internal/builderengine` likewise carry call-site guards over in `cmd/lyx`. `internal/treadleengine` is the sole engine with an allowlist, and `leaf-invariant-audit` is separately examining it.)
  The peer non-stdlib import sets, surveyed during exploration:
  - `websterengine`: batcher, configengine, fabricengine, gitexec, gitrepo, lock, lyxcwd, modelspec, pattern, planparser, shuttleengine, state, stencil, yaml
  - `builderengine`: configengine, fabricengine, gitexec, lock, lyxcwd, modelspec, pattern, shuttleengine, state, stencil, yaml
  - `loomengine`: configengine, fabricengine, lyxcwd, modelspec, pattern, planparser, shuttleengine, state, stencil, yaml
  - `perchengine`: burlerengine, configengine, logger, lyxcwd, modelspec, treadleengine, yaml
  - `burlerengine`: configengine, logger, lyxcwd, pattern, shuttleengine, stencil, yaml
  - `reedengine`: configengine, lock, logger, lyxcwd, proc, reedengine/render, shell, state, tokenvocab, yaml
  - `fabricengine`: configengine, fslink, gitexec, gitignore, gitrepo, lock, logger, lyxcwd, proc, state, weftname, yaml

  The single property all of them share is the absence of `output`/`cobra`/`*cli`.
  That is what "a full lyx module" means structurally, and it is exactly the banned list.
- Rejected: keeping a trimmed allowlist that merely adds `internal/lyxcwd` — rejected because it leaves the same rent-charging structure in place for whatever the *next* legitimate dependency turns out to be, and the allowlist's isolation claim is already false via `logger`.

### Section renamed to "Scout Engine-Seam Invariant"

- Decision: `## Scoutengine Leaf Invariant` → `## Scout Engine-Seam Invariant`.
- Rationale: it stops being a leaf rule the moment the allowlist goes.
  The `<Module> <What>-Seam Invariant` shape matches the two existing seam sections, "Shuttle Provider-Seam Invariant" and "Treadle Runner-Seam Invariant".
- Rejected: "Scoutengine Seam Invariant" (closer to the old name but off-shape);
  "Scout CLI-Seam Invariant" (most literal, but reads as a duplicate of the CLI/Cobra Invariant).

### Banned-list test: rename the file, reuse the three existing predicates

- Decision: `leaf_enforcement_test.go` is **renamed** to `seam_enforcement_test.go` (git mv, not a new file plus a delete), the function becomes `TestEngineSeamInvariant_BannedImports`, and the `allowedImports` map is deleted.
  The three violation predicates already present in the current file are kept verbatim, plus one new exact match, as the whole check:
  - exact match on `github.com/Knatte18/loomyard/internal/output`
  - `strings.Contains(importPath, "spf13/cobra")`
  - `strings.Contains(importPath, "/internal/") && strings.HasSuffix(importPath, "cli")`
  - **new:** exact match on `github.com/Knatte18/loomyard/internal/clihelp`

  The fourth predicate closes a hole the conversion would otherwise open. `internal/clihelp` imports `spf13/cobra` directly (`exec.go`, `jsonhelp.go`) but does **not** end in `cli`, so the `*cli` suffix predicate misses it.
  The old allowlist rejected a `scoutengine` → `clihelp` import simply because `clihelp` was not on the list;
  a pure banned list would let it through.
  Every peer engine shares this hole today, which is a reason to close it here rather than a reason to inherit it.

  Four further changes to the same file, each load-bearing:
  - **The file's header comment (lines 1–7) is rewritten** to the seam/banned-list framing: it states the seam rule, names `CONSTRAINTS.md`'s "Scout Engine-Seam Invariant" as the recorded invariant, and drops both the enumerated allowlist and the false claim that this check "keeps the LSP subprocess client stdlib-only" (that property now belongs to `lspclient_guard_test.go`, and even there it is stdlib **plus `logger`**).
    Leaving that sentence in a file this task is rewriting would be the same defect the task is fixing in `doc.go`.
  - **The closing `t.Errorf` (line 101) is rewritten** — it currently prints "imports outside the allowlist (stdlib + configengine + lock + proc + logger + yaml.v3)".
    The new message names the invariant and the banned imports found.
    The failure message is what a future violator actually reads, so it must not describe a rule that no longer exists.
  - **The trailing catch-all** at lines 90–91 (`failures = append(failures, relPath+": "+importPath)`, reached by any import that is neither stdlib nor allowlisted) **is deleted.**
    Under a banned list it is not merely dead — it is wrong: it would flag `internal/logger` and `gopkg.in/yaml.v3` as violations.
    Only the three predicates may append a failure.
  - **The `isStdlib` heuristic** (lines 61–70) **is removed from this file and moved into `lspclient_guard_test.go`.** A banned list has no notion of stdlib — it asks only "is this import one of the three banned shapes?" — so the seam test needs the heuristic for nothing.
    The guard test is its sole remaining consumer, so the helper lives there rather than becoming a package-scope helper parked in a file that does not use it.
- Rationale: the file name must stop saying `leaf_`;
  `shuttleengine` and `treadleengine` both use `seam_enforcement_test.go` for this exact shape.
  The three predicates are already written, already produce violation-specific failure messages naming which of the three rules broke, and the `*cli` suffix predicate catches future `internal/*cli` packages with zero list maintenance — the one maintenance-free property worth keeping from the old file.
- Rejected: a flat exact-match `bannedImports []string` in `lyxtest`'s shape — more uniform with the repo's other banned list, but every new `*cli` module would have to be added by hand or slip through silently.
  Also rejected: banning the whole `github.com/spf13/...` tree (catches pflag/viper too, broader than the invariant claims);
  keeping the catch-all as a "report anything unrecognised" safety net (that reintroduces the allowlist under another name, and would fail on `logger` today).

### The converted test scans with `os.ReadDir`, not `filepath.WalkDir`

- Decision: `seam_enforcement_test.go` locates its package directory with `runtime.Caller(0)` + `filepath.Dir` and enumerates it with `os.ReadDir`, skipping directory entries, `*_test.go`, and non-`.go` files — replacing the current file's `filepath.WalkDir`.
- Rationale: the task body names `internal/shuttleengine/seam_enforcement_test.go` as the shape to mirror, and that file uses `os.ReadDir` specifically "so the scan matches the rule's scope: the seam package, not everything beneath it" (its own comment, lines 37–39).
  Matching it removes a discrepancy that would otherwise sit between this discussion's two sections.
  The two are behaviourally identical **today** — `internal/scoutengine` has no subdirectories — so this is a decision about the future, not a bug fix: under `WalkDir`, a hypothetical `internal/scoutengine/<sub>` package would silently inherit scout's seam rule without anyone having decided that.
- Rejected: keeping `filepath.WalkDir` (smaller diff, but it silently extends the rule's scope to future subpackages, and it contradicts the named model).
- **Failure-message path form:** with the switch to `os.ReadDir`, all four predicates report `entry.Name()` (e.g. `lspclient.go`), matching the named model.
  This is a behaviour change worth stating: the three inherited predicates currently append `path`, the absolute path `WalkDir` hands them, and only the now-deleted catch-all used `filepath.Rel`.
  Keeping them "verbatim" would otherwise mean every violation printing a full absolute path.
  The package is implied by the test's own location, so the bare filename is the useful part.
- **Vacuity:** the test asserts it scanned at least one non-test `.go` file and `t.Fatal`s otherwise.
  This deliberately does **not** inherit the `shuttleengine` model's behaviour, which passes silently on an empty scan.
  The same requirement is placed on `lspclient_guard_test.go` (see below), and a guard that goes green because it found nothing to check is the failure mode both tests exist to prevent.

### The `lspclient.go` guard allows `internal/logger`, and is never called "stdlib-only"

- Decision: `TestLSPClientGuard_StdlibAndLoggerOnly` asserts `internal/scoutengine/lspclient.go` imports **stdlib plus `github.com/Knatte18/loomyard/internal/logger`, and nothing else**.
  Every doc string, header comment, and failure message describes the property as *"no lyx dependency except logging"* — never *"stdlib-only"* or *"hermetic"* as an assertion about the file.
  (Using either word in an explicit **denial** — "the file is neither stdlib-only nor hermetic" — is exactly the point and is what the committed `CONSTRAINTS.md` bullet now does.)
- Rationale, and this is the load-bearing finding of the whole discussion: **`lspclient.go` already imports `internal/logger` today** (five `logger.Warn` calls at lines 564, 567, 572, 595, 598).
  The old allowlist's header comment claimed it was keeping the LSP subprocess client "stdlib-only";
  that claim was already false, because `logger` was on the allowlist alongside it.
  A literally stdlib-only guard would fail on the first run, and fixing it would need a production change this task forbids.
  Worse, `internal/logger` imports `internal/lyxcwd` and `internal/proc`, so the file is not hermetic even with the guard.
  Describing it as "stdlib-only" would reproduce, at file scope, the exact mislabelling this task exists to correct.
  The guard's real and defensible value is narrower: it pins the ported stdio LSP client (ported from `tools/codeintel-poc/gopls.go`, per its own header comment) as liftable back out of lyx behind a single logging dependency.
- Rejected: stdlib-only with the `logger.Warn` calls deleted (a production change, out of scope);
  stdlib + `logger` + `proc` (pre-authorizes a dependency the file does not have, weakening the guard for nothing).

### The guard covers `lspclient.go` alone

- Decision: the guard walks exactly one file. `probe.go` and every other scoutengine file are uncovered.
- Rationale: `lspclient.go` is the only file in the package whose contract is "speaks LSP over stdio, knows nothing about lyx". `probe.go` is currently pure stdlib (`context`, `time`) but is protocol-agnostic readiness glue that could legitimately grow a `lock` or `proc` dependency;
  guarding it would charge exactly the kind of rent this task is removing.
- Rejected: covering `probe.go` too;
  covering "every file not already known to need a non-stdlib import" (an implicit allowlist through the back door — the thing being deleted).

### Guard file naming

- Decision: new file `internal/scoutengine/lspclient_guard_test.go`, function `TestLSPClientGuard_StdlibAndLoggerOnly`.
- Rationale: `*_guard_test.go` is the repo's established name for a **narrow, file-scoped or call-site-scoped** guard — `internal/lyxcwd/raddle_guard_test.go`, `tools/sandbox/pathresolve_guard_test.go`, `cmd/lyx/boardguard_test.go`, `cmd/lyx/ghguard_test.go`.
  `*_enforcement_test.go` is reserved for package-wide invariants, which this is not.
  The function name states the actual allowed set rather than an aspirational one.
- Rejected: `lspclient_imports_test.go` / `TestLSPClientImports_StdlibOnly` (the name would be a lie);
  folding it into `seam_enforcement_test.go` (the task body explicitly requires a new file).

### The guard is recorded as a bullet inside the scout seam section

- Decision: the `lspclient.go` guard gets its own bullet inside "## Scout Engine-Seam Invariant", clearly marked as a separate and narrower rule, and the section's "Enforced by" line names both tests.
- Rationale: the task body bans re-adding an *allowlist line* to the rewritten section, not a mention of the guard.
  An unrecorded guard is exactly the kind of thing a future audit deletes as unexplained.
  One section per module keeps `CONSTRAINTS.md` navigable.
- Rejected: a second top-level section, "Scout LSP Client Hermeticity" (cleaner separation, but a second scout section in a file that runs one section per concern — and "Hermeticity" would be the wrong word anyway, per the `logger` finding above);
  recording it nowhere but the test's header comment.

### `doc.go` and `docs/overview.md` land in the same commit

- Decision: `doc.go`'s "The engine/CLI split" paragraph is rewritten to state the seam without enumerating imports — roughly "scoutengine returns typed Go results and typed errors and never imports `internal/output`, cobra, or any `internal/*cli` package; `internal/scoutcli` is the sole consumer that maps engine results/errors onto the JSON envelope".
  The words "leaf package" go with it.
  **The `internal/modelspec` cross-reference at lines 30–34 goes with the paragraph too** — it calls modelspec "the shape this package mirrors most directly" and says scout is cycle-free "the same way `internal/modelspec` already is". `modelspec` remains an allowlisted leaf (its own `CONSTRAINTS.md` section is untouched, and `leaf-invariant-audit` expects to KEEP it), so leaving those two sentences would re-import through the back door exactly the framing the rest of the edit removes.
  Scout's shape is now the engine/cli seam that every feature module follows, not modelspec's leaf shape.
  `docs/overview.md:252`'s "a cycle-free leaf" becomes "a cycle-free engine".
- Rationale: `doc.go` lines 22–34 currently enumerate the allowlist (at 24–26) and are **already wrong** — they omit `internal/logger`, which two production files import.
  Leaving them ships a knowingly false module doc, and `docs/overview.md:362` designates this package doc as scout's module doc, so the repo's "docs land in the same commit" rule applies directly.
  `docs/overview.md` is not touched by `leaf-invariant-audit`, so the one-word edit carries no collision risk.
- Rejected: leaving both files alone to keep the diff to the three files the task body names.

### Proving the guards actually fail

- Decision: assertion-only tests, matching every existing guard in the repo (none of `lyxtest`, `shuttleengine`, `pattern`, `modelspec`, `githubclient` has a negative case).
  During implementation, each new test is proven red by temporarily adding a violating import, observing the failure, and reverting.
- Rationale: consistency with every existing guard in the repo;
  a table-driven negative case would require refactoring the matcher into a separately-testable predicate, a shape no other guard in the repo uses.
- Rejected: the table-driven negative case (novel shape, extra surface);
  assertion-only with no red-check at all (how a guard silently rots into a no-op).

## Technical context

**Files in play**

- `CONSTRAINTS.md` — scout section, **already rewritten in this branch**. Currently reads `## Scout Engine-Seam Invariant` with four bullets: allowed direction, no-allowlist, the file-scoped guard, and "Enforced by" naming both future test paths.
- `internal/scoutengine/leaf_enforcement_test.go` (103 lines) — the file to rename and convert.
  Its structure: a package doc comment claiming the allowlist keeps the LSP client stdlib-only, an `allowedImports` map, and `TestLeafInvariant_AllowlistOnly` walking the package dir with `filepath.WalkDir` + `go/parser` in `parser.ImportsOnly` mode, skipping `*_test.go` and non-`.go` files, with a stdlib heuristic (`no '.' in the first path segment`) and the three violation predicates at lines 77–88.
- `internal/scoutengine/lspclient.go` — 600+ lines, imports `bufio, bytes, context, encoding/json, fmt, io, net, os, os/exec, sort, strconv, strings, time` plus `internal/logger`.
- `internal/scoutengine/doc.go` — 291 lines;
  the paragraph to edit is under `# The engine/CLI split`, lines 22–34.
- `docs/overview.md:252` — the scout module-table entry.

**Shape to mirror**

`internal/shuttleengine/seam_enforcement_test.go` is the named model in the task body.
It uses `runtime.Caller(0)` + `filepath.Dir` to locate the package, `os.ReadDir` (not `WalkDir`) so it scans only the package's own files and not subpackages, skips dirs / `*_test.go` / non-`.go`, parses with `parser.ImportsOnly`, and collects `failures []string` for one `t.Errorf` at the end naming the invariant.
`internal/lyxtest/leaf_enforcement_test.go` is the repo's other banned-list test and uses a flat `bannedImports []string` — the shape explicitly rejected above in favour of the three predicates.

**Gotchas**

- `runtime.Caller(0)` + `filepath.Dir` is how every guard in the repo locates its own package directory;
  it makes the test independent of the working directory `go test` runs from.
  The new `lspclient_guard_test.go` should resolve `filepath.Join(dir, "lspclient.go")` the same way, and should `t.Fatal` if that file is missing rather than silently passing on zero files scanned — a guard that vacuously passes after a rename is worse than no guard.
- The stdlib heuristic (`first path segment contains no '.'`) is already written and correct at `leaf_enforcement_test.go` lines 61–70;
  **move** it into `lspclient_guard_test.go` rather than inventing a second one — and delete it from the converted seam test, which no longer has any use for it. `go/parser` is used specifically to avoid false positives from import-path strings appearing in doc comments — both new tests must keep parsing rather than grepping.
- `scoutengine` has three test files behind `//go:build scout`, a tag no pipeline gate compiles (noted in the `scout-lyxcwd-accessors` body).
  Neither new test file should carry a build tag — both must run in the default untagged build.
- Test Tier Purity Invariant: both test files are untagged, so neither may call `gitexec.RunGit`, `exec.Command`/`exec.CommandContext`, or `lyxtest.Copy*`, and neither may contain those tokens even in a comment or string literal (the check is a raw substring match).
  Pure `go/parser` guards satisfy this trivially — just do not mention `exec.Command` in a comment.
- Hermetic Git Test Environment Invariant: not triggered — neither new test spawns git, so no `TestMain` obligation is added.
- Fabric Vocabulary Invariant: the tokens `weft`/`warp` must not appear in any new text.
- Nothing machine-reads `CONSTRAINTS.md` section titles;
  the only references are prose mentions in `doc.go`/test comments across other packages, none of which name the scout section.

## Constraints

From `CONSTRAINTS.md` (as it now stands in this branch):

- **Scout Engine-Seam Invariant** — the section this task authors. `scoutcli` → `scoutengine` only;
  no `output`/`cobra`/`*cli`;
  no allowlist;
  `lspclient.go` limited to stdlib + `logger`.
- **Cwd Resolution Invariant** — the reason the follow-up task exists.
  Not exercised by this task directly, but the rewrite must not say anything that would forbid `scoutengine` from importing `internal/lyxcwd`, since `scout-lyxcwd-accessors` depends on exactly that becoming legal.
- **Test Tier Purity Invariant** — both new/converted test files are untagged and must spawn nothing.
- **CLI / Cobra Invariant** — its "engine never imports cli or cobra" clause is the general rule the scout section specialises;
  the rewritten section must stay consistent with it, not contradict it.
- **Documentation Lifecycle** (`docs/overview.md#documentation-lifecycle`) — scout's design doc was deleted on landing and `doc.go` is its durable replacement, which is why `doc.go` is in scope.

From the repo's `CLAUDE.md`:

- Docs land in the same commit as the change (module doc, `docs/overview.md`, `CONSTRAINTS.md`).
- `manifest/roadmap.md` does **not** move — this is a hardening/cleanup pass, not a planned-item completion.
- Markdown uses semantic line breaks, not fixed-column wrapping.
- Worktree isolation: everything happens in `wts/scout-seam-conversion`;
  no pushing to `main`.

Coordination constraint: `leaf-invariant-audit` is active in parallel and edits the seven other `CONSTRAINTS.md` sections.
Do not touch them.

## Testing

No production behaviour changes, so there is nothing to test at the behaviour level.
The deliverable *is* two guard tests, and the testing work is proving they are not vacuous.

**`internal/scoutengine/seam_enforcement_test.go` — `TestEngineSeamInvariant_BannedImports`** (converted, TDD candidate)

- Green on the tree as it stands: `configengine`, `lock`, `proc`, `logger`, `yaml.v3` must all pass, because none is banned.
  This is the conversion's whole point and is the first thing to verify.
- Must go red when a production file imports `internal/output` — verify by temporary local edit, observe the `internal/output` violation message, revert.
- Must go red on a `cobra` import, on an `internal/*cli` import, and on an `internal/clihelp` import — same temporary-edit method, confirming each of the four predicates produces its own distinct message rather than a generic mismatch.
  The `clihelp` case matters most: it is the one predicate with no counterpart in the old file, and it is the hole the conversion would otherwise open.
- Must **not** flag a hypothetical `internal/lyxcwd` import — this is the specific regression the follow-up task depends on, and it is worth confirming explicitly by temporary edit before this task is called done.

**`internal/scoutengine/lspclient_guard_test.go` — `TestLSPClientGuard_StdlibAndLoggerOnly`** (new, TDD candidate)

- Green on `lspclient.go` as it stands, including its existing `internal/logger` import.
  Writing this test first and watching it pass against the real file is what catches a mis-stated allowed set.
- Must go red on any second lyx import added to `lspclient.go` — including one the package-level banned list permits, e.g. `internal/lock` or `internal/configengine`.
  That divergence between the two tests is the guard's entire reason to exist and must be demonstrated.
- Must go red on a third-party import (e.g. `gopkg.in/yaml.v3`).
- Must `t.Fatal`, not silently pass, if `lspclient.go` is absent from the package directory.
  Worth a deliberate check during implementation (temporarily rename the file) — a guard keyed to a filename must fail loudly when the filename moves.
- The seam test carries the matching vacuity assertion: it must `t.Fatal` if `os.ReadDir` yielded zero non-test `.go` files.
  Verify the same way — temporarily point it at an empty directory, or assert the scanned-file count directly.

**Suite-level**

- `go test ./internal/scoutengine/...` green.
- `go test ./...` green — confirms no other package referenced the renamed test file or function.
- `go vet -tags scout ./internal/scoutengine/...` — the `//go:build scout` files are not compiled by any pipeline gate, and the rename touches the package's test surface, so verify manually.

## Q&A log

- **Q:** Include `internal/scoutengine/doc.go` in scope, given its lines 22–34 restate the allowlist and already omit `internal/logger`? **A:** Yes — rewrite the paragraph as a seam statement, dropping the enumerated import list.
- **Q:** What may `lspclient.go` import under the new guard, given it already imports `internal/logger` and a literal stdlib-only guard would fail immediately? **A:** Answered by peer-norm survey: no other engine module in the repo carries an import *allowlist*, and the only property they all share is the absence of `output`/`cobra`/`*cli`. A single-file allowed-set guard therefore has no peer precedent and survives only as a deliberate, accurately-named exception — stdlib plus `internal/logger`, described as "no lyx dependency except logging", never "stdlib-only".
- **Q:** Which files does the new guard cover — `lspclient.go` alone, or also `probe.go`? **A:** `lspclient.go` alone. `probe.go` is stdlib today but is glue that could legitimately grow a dependency.
- **Q:** Guard file and function name? **A:** `lspclient_guard_test.go` / `TestLSPClientGuard_StdlibAndLoggerOnly`, matching the repo's `*_guard_test.go` convention for narrow guards.
- **Q:** Rename the converted package test file, or keep `leaf_enforcement_test.go` and rename only the function? **A:** Rename the file to `seam_enforcement_test.go`, matching `shuttleengine` and `treadleengine`.
- **Q:** How does the banned list match import paths? **A:** Reuse the three predicates already in the current file — exact match on `internal/output`, `Contains("spf13/cobra")`, and `Contains("/internal/") && HasSuffix("cli")` — rather than a flat exact-match list that would need hand-maintenance for each new `*cli` module.
- **Q:** Should `docs/overview.md:252`'s "cycle-free leaf" be reworded? **A:** Yes — one-word change to "cycle-free engine".
- **Q:** New `CONSTRAINTS.md` section name? **A:** "Scout Engine-Seam Invariant", matching the `<Module> <What>-Seam Invariant` shape of the two existing seam sections.
- **Q:** Where is the `lspclient.go` guard recorded in `CONSTRAINTS.md`? **A:** A bullet inside the new scout seam section, marked as a separate narrower rule, with "Enforced by" naming both tests.
- **Q:** How are the new guards proven to actually fail? **A:** Assertion-only tests, matching every existing guard in the repo, each proven red during implementation by a temporary violating import that is then reverted.
- **Q:** (review r1) The old test file's header comment and closing `t.Errorf` both restate the allowlist, including the false "keeps the LSP subprocess client stdlib-only" claim — are they in scope? **A:** Yes. Both are rewritten to the seam/banned-list framing; "stdlib-only" is removed from the file entirely. The `t.Errorf` is what a future violator reads, so it must not describe a deleted rule.
- **Q:** (review r1) What happens to the `isStdlib` heuristic and the trailing catch-all once `allowedImports` is gone? **A:** The catch-all is deleted — under a banned list it would flag `logger` and `yaml.v3` as violations. The stdlib heuristic moves into `lspclient_guard_test.go`, its only remaining consumer, rather than becoming a package-scope helper in a file that no longer needs it.
- **Q:** (review r1) Does the converted test scan with `filepath.WalkDir` or `os.ReadDir`? **A:** `os.ReadDir`, matching the `shuttleengine` model the task body names. Identical behaviour today (scoutengine has no subdirectories); the difference is that a future subpackage does not silently inherit the seam rule.
- **Q:** (review r1, note) Is "no import guard of any kind" accurate for peer engines? **A:** No — `shuttleengine` has a banned-import guard, and `boardengine`/`websterengine`/`builderengine` have call-site guards in `cmd/lyx`. The claim is narrowed to "no import **allowlist**", which is what the decision actually rests on.
- **Q:** (review r1, note) Is the pre-staged `CONSTRAINTS.md` hunk committed or only in the working tree? **A:** Committed and pushed, in `5748a22f`, and amended again in round 2. No plan card owns re-applying it; the plan only verifies it still matches the final test names.
- **Q:** (review r2) The committed `CONSTRAINTS.md` bullet called the guard "a hermeticity rule" while this discussion forbids the word — who reconciles it? **A:** mill-start did, in this round. The pre-staged section is mill-start's to amend, not mill-go's; the bullet now reads "the file must never be described as stdlib-only or hermetic — it is neither", so the word survives only as a denial. No plan obligation is widened.
- **Q:** (review r2, note) `internal/clihelp` imports cobra but does not end in `cli`, so the banned list would let `scoutengine` → `clihelp` through where the allowlist rejected it — is that acceptable? **A:** No. A fourth exact-match predicate on `internal/clihelp` is added, and the direct-imports-only scope is stated in `CONSTRAINTS.md`. Every peer engine has the same hole, which is a reason to close it here, not to inherit it.
- **Q:** (review r2, note) The cited pre-staging commit hash was not visible in the reviewer's git snapshot — is it real? **A:** Yes; `git cat-file -t 5748a22f` resolves and it is the branch's second-newest commit. The reviewer's snapshot was taken at session start, before the commit existed.
- **Q:** When is `CONSTRAINTS.md` rewritten? **A:** Immediately, during mill-start, before the discussion-review rounds — so reviewers do not spend their findings on a contradiction between the discussion and a still-stale authoritative doc.
