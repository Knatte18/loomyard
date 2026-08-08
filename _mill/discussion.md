# Discussion: Audit the remaining leaf and seam import invariants

```yaml
task: Audit the remaining leaf and seam import invariants
slug: leaf-invariant-audit
status: discussing
parent: main
```

## Problem

`scout-seam-conversion` found that a leaf import-allowlist can outlive its own rationale and start charging rent.
`internal/scoutengine`'s allowlist excluded `internal/lyxcwd` while permitting `internal/logger`, which imports `lyxcwd` anyway — so the rule bought naming discipline rather than the isolation its wording implied, and it cost a whole plan card in split path ownership (`dotlyx-scratch-hygiene` batch 4 card 29).

The repo has seven other invariants of this family in `CONSTRAINTS.md` and nobody had checked whether they are in the same state.
This task is that check.
The audit has now been performed during discussion (see **Audit results** below);
what remains is to land the corrections it found.

The audit's conclusion is that **no invariant needs removing or weakening**.
What it found instead is rule *text* that misdescribes what the rule enforces, in two places, plus a rule whose stated import set was never actually enforced.
Those are the corrections this task lands.

**Why now:** reviewers follow `CONSTRAINTS.md` slavishly.
A rule that reads as an isolation guarantee when it is really a told-never-derived posture will be defended in code review as if it were the former, and other tasks' reviewers keep enforcing the wrong reading until this branch merges.
Keeping this task to one batch and merging it quickly is the point.

## Scope

**In:**

- `CONSTRAINTS.md` — **two edits already landed during mill-start** (Treadle Runner-Seam bullet, lyxtest Leaf section — see "Work already landed"), plus **one edit still to make in mill-go**: the lyxtest section's "**Enforced by**" line gains the test-function name, per the "lyxtest's test is renamed" decision.
- `internal/treadleengine/doc.go` (~line 147) — the "never imports internal/lyxcwd" clause.
- `internal/treadleengine/engine.go` (~line 6) — the same clause on the `Engine` type doc.
- `internal/lyxtest/doc.go` — **lines 7-13 only**, the denylist framing: the opening "policed by a banned-imports list … not an allowlist" sentence *and* the "It must not import internal/configreg or any feature package … would close a test-build cycle" sentences that continue it.
  **Lines 14-16 stay as-is** — the `SeedConfig`/configreg-free-map sentence is accurate today, mirrors the `CONSTRAINTS.md` bullet, and is unaffected by the conversion.
- `internal/lyxtest/leaf_enforcement_test.go` — convert the banned-imports denylist to an allowlist, rename `TestLeafInvariant` to `TestLeafInvariant_AllowlistOnly`, and rewrite the file header + test doc comments, which are stale today independent of the conversion.
- `internal/modelspec/leaf_enforcement_test.go` (~line 4) — its "Unlike lyxtest's leaf_enforcement_test.go (a banned-import denylist)" cross-reference dies with the conversion.
- `internal/shuttleengine/seam_enforcement_test.go` (~line 22) — names `TestLeafInvariant` by function name;
  pulled into scope by the rename, and **only** by the rename.

**Out:**

- **The Scoutengine Leaf Invariant, everywhere.** `CONSTRAINTS.md` lines ~65-72, `internal/scoutengine/doc.go`, and `internal/scoutengine/leaf_enforcement_test.go` are all owned by the parallel `scout-seam-conversion` task.
  Do not edit, do not add a verdict line, do not "fix" its wording — a same-file collision between the two tasks is exactly what this exclusion exists to prevent.
- **No audit document.** No new file under `manifest/designs/`, `docs/`, or `_raddle/`.
  See the "No stored audit artifact" decision.
- **No verdict lines in `CONSTRAINTS.md`.** No `KEEP` / `AUDITED` / date markers in any section.
- **No invariant is removed, and none is weakened.** All seven survive.
- **`manifest/roadmap.md` does not move** — per `CLAUDE.md`, the roadmap moves only on completing or adding a planned item;
  this is a polish/correction pass covered by git history.
- **`docs/overview.md` does not change** — no module table entry, execution-stack entry, or observable CLI behaviour changes.
- **No production source behaviour changes.** The only non-comment change in the whole task is a test file's enforcement mechanism.
- `internal/perchengine` and `internal/burlerengine` doc claims — investigated and found true, see **Audit results**.
  Explicitly not touched.
- Every other invariant in `CONSTRAINTS.md` outside the seven listed in **Audit results**.

## Audit results

This is the audit itself, performed during discussion.
It is recorded here (in a task-state file that dies with the task) rather than in a durable repo doc — that is a deliberate decision, see "No stored audit artifact".

Method: for each package, `go list -deps ./internal/<pkg>` for the transitive internal closure and `go list -f '{{range .Imports}}...'` for direct imports, plus reading each enforcement test in full.

| Invariant | Direct internal imports | `lyxcwd` in closure? | Check shape | Finding |
|---|---|---|---|---|
| lyxtest Leaf | `configengine`, `lyxcwd`, `weftname` | direct | **denylist** | rule text asserts an import set the check never enforces |
| Modelspec Leaf | `configengine` | **no** | allowlist | genuinely leaf, nobody pays |
| Treadle Runner-Seam | `lock`, `logger`, `shuttleengine`, `state`, `stencil` | **yes, two paths** | allowlist | exclusion buys no isolation; enforces something else, and correctly |
| Tokenvocab Leaf | `lyxcwd`, `stencil` | direct | allowlist | permits `lyxcwd` outright — not scout-shaped |
| Pattern Leaf | `lyxcwd` | direct | allowlist | permits `lyxcwd` outright — not scout-shaped |
| Shuttle Provider-Seam | …incl. `lyxcwd` | direct | interface segregation | different invariant family; the `lyxcwd` question does not apply |
| GitHub Auth / githubclient | `proc` | **no** | allowlist | real reason never to need `lyxcwd` |

Detail on the three that matter:

**Treadle is scout-shaped, and worse than the task brief anticipated.**
The brief expected the `logger` path.
The allowlist *also* permits `internal/shuttleengine`, which imports `internal/lyxcwd` **directly** — two independent transitive paths, not one.
So the exclusion provides no isolation whatsoever at the dependency-graph level.

It does, however, enforce something real, which the brief correctly predicted and which `internal/treadleengine/doc.go` (the "Geometry-blindness and fabric-blindness" section) already describes accurately: treadle is *told* its geometry and never derives it.
`Engine.Run` operates on a caller-supplied absolute `runDir`;
a block's `Profile` carries a caller-supplied `GateDir` (`perchengine` resolves it from its own `*lyxcwd.Location`).
Every `filepath.Join` in the package joins onto one of those told values — verified at `roundfiles.go:45-51`, `state.go:100,139,155,216`, and `run.go:98`.
Zero derivation anywhere.
Verdict is therefore keep-and-sharpen, exactly as the brief expected — the rule stands, its wording is what changes.

**lyxtest carries drift the brief did not anticipate.**
`CONSTRAINTS.md` stated "its import set is stdlib plus `internal/lyxcwd`, `internal/weftname`, and `internal/configengine`" — but `leaf_enforcement_test.go` is a **denylist** of nine banned paths.
That stated import set was unenforced prose sitting in a file whose entries are supposed to be rules.
Separately, the test's own header comment claimed lyxtest "imports only stdlib and internal/lyxcwd", stale on two counts (`weftname` and `configengine` are both imported and both legitimate).

**Modelspec and githubclient are genuinely leaf.**
`internal/lyxcwd` appears nowhere in either transitive closure.
`modelspec` has no `path/filepath` import in any production file — it is a pure config/spec parser that never touches worktree geometry.
`githubclient`'s `cacheDir()` (`cache.go:38-58`) resolves `%LOCALAPPDATA%\lyx` / `~/.config/lyx`, a machine-global location deliberately outside worktree/anchor geometry.
Both are zero-cost keeps, no wording change needed.

**Sweep for other now-invalid claims** (requested during discussion): three further sites restate a corrected rule and go stale — `internal/lyxtest/doc.go:7-9`, `internal/treadleengine/engine.go:6`, and `internal/modelspec/leaf_enforcement_test.go:4`.
All three are in scope above.

Two families that look like the same disease but are **not**, verified rather than assumed:

- `internal/perchengine/engine.go:9`, `internal/perchengine/doc.go:243`, `internal/burlerengine/doc.go:105` and `:196` all claim "never imports fabricengine and never constructs a `_lyx` path".
  `go list -deps` confirms `fabricengine` is absent from both packages' transitive closures.
  The claims hold. Not touched.
- `internal/treadleengine/seam_enforcement_test.go:4` and `:8` restate the treadle rule ("never `internal/lyxcwd` as a direct import";
  "a convenience lyxcwd import").
  Accurate as written — "as a direct import" already carries exactly the reading the `CONSTRAINTS.md` correction makes explicit, with no isolation claim to remove.
  Not touched.
- `internal/tokenvocab/doc.go:10` restates tokenvocab's rule accurately.
  `pattern`, `githubclient`, and `shuttleengine` package docs likewise. Not touched.

`internal/shuttleengine/seam_enforcement_test.go:22` was originally on this untouched list — it cites lyxtest's test as a *style* reference (the `go/parser` `ImportsOnly` idiom, to avoid false positives from doc-comment string literals), and that idiom survives the conversion unchanged.
It cites the test by **function name** ("`TestLeafInvariant`"), so the rename decision moves it into scope.
It is listed under **Scope → In** above, not here.

**Sweep method, so it can be re-derived and extended.**
The sweep was three `grep -rn` passes over `docs/`, `manifest/`, `internal/`, `cmd/`, `tools/`:

1. `grep -rn "treadle" --include=*.md --include=*.go … | grep -i "lyxcwd\|geometry\|leaf\|seam"` — treadle claims outside the package.
2. `grep -rn "lyxtest" --include=*.md --include=*.go … | grep -i "leaf\|banned\|allowlist\|denylist\|import set"` — lyxtest rule restatements.
3. `grep -rn "never imports\|does not import\|imports only" --include=*.go --include=*.md .` — the overstatement pattern repo-wide;
   this is the one that surfaced the `perchengine`/`burlerengine` false positives and is the pass to re-run when extending.

Transitive-closure evidence for every verdict came from `go list -deps ./internal/<pkg> | grep loomyard/internal/`, and direct imports from `go list -f '{{range .Imports}}{{.}}{{"\n"}}{{end}}' ./internal/<pkg>`.
Note that `go list` reports only build-tag-satisfied files, so a package with `//go:build` production files needs a raw import scan too — that is why `internal/lyxtest`'s four production files were checked directly for build constraints (there are none) before the allowlist was declared feasible.

## Work already landed

**Read this before planning — part of the task is already committed.**

Per the "CONSTRAINTS.md correction lands before the reviewers" decision, the two `CONSTRAINTS.md` edits were made during mill-start, in the commit that also adds this discussion file.
They are **done**;
the plan must not redo them.

1. **Treadle Runner-Seam Invariant** — the import-allowlist bullet gained two sentences after "Policed on direct imports only, not the transitive closure":
   that `lyxcwd` is reachable through both `logger` and `shuttleengine` so the exclusion buys no isolation, and that what it enforces is the told-never-derived posture with `runDir`/`GateDir` named.
2. **lyxtest Leaf Invariant** — the opening sentence was rewritten from denylist framing ("policed by a banned-imports list …, not an allowlist") to an allowlist statement: "production code imports only stdlib, `internal/lyxcwd`, `internal/weftname`, and `internal/configengine`", with `configreg` and the feature packages excluded by construction and the test-build-cycle reason stated in one clause.

**A third `CONSTRAINTS.md` edit is still outstanding and belongs to mill-go**, not to this list: the lyxtest section's "**Enforced by**" line gains the test-function name once the rename lands.
See the "lyxtest's test is renamed" decision.
So "do not redo the `CONSTRAINTS.md` work" means the two rule-text corrections above — it does not mean `CONSTRAINTS.md` is closed to further edits.

**Known transient inconsistency, do not "fix" it by reverting:** `CONSTRAINTS.md` now describes lyxtest's check as an allowlist, but `internal/lyxtest/leaf_enforcement_test.go` is still a denylist until this task's batch converts it.
The file is ahead of the test by design, for the window between the mill-start commit and the implementation commit on the same branch.
Closing that gap is scope item 5.

## Decisions

### No stored audit artifact

- Decision: the task produces **no** durable audit document — no file in `manifest/designs/`, no `docs/` page, no `_raddle/` note — and **no** per-invariant verdict lines inside `CONSTRAINTS.md`.
  The audit's findings live in this task-state `discussion.md` (which dies with the task) and in the implementation commit message.
  Only the corrections the audit found are durable.
- Rationale: three reasons, in order of weight.
  First, the justification for storing it does not survive contact — the claimed benefit was "so the next person doesn't redo the `go list` work", but that work is one command per package.
  The expensive part of the scout incident was that nobody thought to *ask*, and a document sitting unread in `manifest/designs/` does nothing to make a future reader think to ask.
  Second, it rots in the dangerous direction: "modelspec is genuinely leaf, `lyxcwd` not in the closure" is true today and becomes a confident false statement the moment someone adds an import — while the enforcement test, which carries the same guarantee, *fails in CI* instead of lying.
  A stored audit is an unmaintained second copy of what the tests already assert, with none of their ability to fail.
  Third, `CONSTRAINTS.md` line 5 states outright: "This file states rules only — no rationale, no incident narratives, no historical justification."
  A `KEEP`/`AUDITED` verdict line is precisely historical justification, so the brief's literal instruction to record a verdict in each section contradicts the file's own charter.
- Rejected: **a `manifest/designs/leaf-invariant-audit.md`** — rots into false confidence, duplicates the tests.
  **Inline verdict lines per section** — violates line 5 and grows `CONSTRAINTS.md` a history section on every future audit.
  **Amending line 5 to permit verdicts** — pays for a literal reading of the brief by weakening the file's charter.
  **Zero-diff pure audit, corrections deferred to a follow-up task** — leaves two known-wrong rules in the file reviewers follow slavishly, for no gain.
- Consequence for mill-plan: the implementation commit message must name all seven invariants and state the transitive evidence for treadle (both the `logger` and `shuttleengine` paths), so the reasoning is recoverable from `git log`.
  A commit message rots harmlessly — nobody mistakes one for current truth.

### Treadle's rule text states both the non-claim and the real claim

- Decision: the corrected bullet says *both* that excluding `lyxcwd` buys no isolation (naming both transitive paths) *and* what the exclusion does enforce (told-never-derived, naming `Engine.Run`'s `runDir` and `Profile`'s `GateDir`).
- Rationale: stating only the real claim leaves the next reader to re-discover the transitive reachability themselves — which is the exact failure that produced this task.
  Naming both paths inline costs one sentence and closes the question permanently.
  This is rule content (what the rule enforces), not history, so it stays inside line 5's boundary.
- Rejected: **state the real claim only** — shorter, but re-opens the discovery cost.
  **Drop `lyxcwd` from the exclusion entirely and enforce told-never-derived directly** — nothing would then stop a convenience `lyxcwd` import that *does* derive a path;
  the import ban is a cheap proxy that still has teeth even though its stated reason was wrong.

### lyxtest converts to an allowlist rather than having its prose weakened

- Decision: `internal/lyxtest/leaf_enforcement_test.go` becomes an allowlist of `internal/configengine`, `internal/lyxcwd`, `internal/weftname` (plus stdlib), following the shape of `internal/pattern/leaf_enforcement_test.go`.
  The `CONSTRAINTS.md` sentence stays a strong claim and becomes true, rather than being watered down to describe what the denylist happened to check.
- Rationale: there were two ways to remove the drift — enforce the stated set, or weaken the text to match the check.
  Enforcing is strictly better and verified feasible: `internal/lyxtest` has four production files (`doc.go`, `hermetic.go`, `lyxtest.go`, `reexecguard.go`), **no build constraints on any of them**, and exactly those three internal imports, so the allowlist holds today with zero source changes.
  It is also strictly stronger on the denylist's own purpose — the cycle-prevention ban on `configreg` and the feature packages is implied by "only these three" — so nothing is lost.
  It becomes self-maintaining: a future stray import fails without anyone editing a list.
  `lyxtest` is the package every test suite depends on, so a stray import there has the widest blast radius in the repo and is exactly the event that should require a human to notice.
- Rejected: **keep the denylist and weaken the `CONSTRAINTS.md` sentence** — preserves a weaker property for no benefit.
  **Record the gap without fixing it** — the original recommendation, rejected on the grounds that "this is an audit, not a refactor" is a procedural objection, not a technical one, and the fix is three lines.

### lyxtest's test is renamed, and that pulls two dependents into scope

- Decision: `TestLeafInvariant` is renamed to `TestLeafInvariant_AllowlistOnly`, matching every other allowlist enforcement test in the repo.
  This is settled here, not left to mill-plan.
  Two sites follow from it and are both in scope: `internal/shuttleengine/seam_enforcement_test.go:22`, which names the function in a style cross-reference, and `CONSTRAINTS.md`'s lyxtest "**Enforced by**" line, which names only the file today.
- Rationale: the rename was originally deferred as a judgement call, which was wrong — leaving it open made the sweep's "verified untouched" list conditional on an undecided question, so the list could not be trusted.
  Deciding it here makes the dependent set closed and knowable before planning starts.
  On the merits: the `_AllowlistOnly` suffix is what every sibling uses, and after the conversion the bare `TestLeafInvariant` name would be the only one not saying which mechanism it applies.
- Consequence for `CONSTRAINTS.md`: the lyxtest section's "**Enforced by** `internal/lyxtest/leaf_enforcement_test.go`." gains `(`TestLeafInvariant_AllowlistOnly`)`, matching the six siblings that already name their test function.
  This is a **third** `CONSTRAINTS.md` edit and it belongs to mill-go, not to the mill-start commit — the mill-start commit deliberately covered only the two rule-*text* corrections that reviewers act on, and an enforced-by line naming a function that does not exist yet would be a forward reference.
- Rejected: **skip the rename** — keeps `shuttleengine:22` and the enforced-by line untouched, at the cost of the one inconsistently-named enforcement test in the repo, in the file this task exists to correct.
  **Rename but leave the enforced-by line alone** — the line is already the only one of seven omitting its test name;
  renaming while leaving it is the worst of both.

### CONSTRAINTS.md correction lands before the reviewers

- Decision: the two `CONSTRAINTS.md` edits were made during mill-start, ahead of discussion review.
  Everything else — all Go doc comments and the test conversion — is left to mill-go as a single batch.
- Rationale: reviewers follow `CONSTRAINTS.md` slavishly, so a reviewer reading the uncorrected treadle rule would defend an isolation guarantee that does not exist and could push back on this task's own premise.
  Landing the correction first means every reviewer from discussion review onward reads corrected text *and* reviews the correction itself as part of the diff.
  The split stops at `CONSTRAINTS.md` because that is the file reviewers treat as authoritative;
  Go doc comments are not, so moving them early would collapse the task to nothing and leave mill-go without coherent work.
- Rejected: **everything in mill-start** — task collapses to one commit and skips plan/implementation review entirely.
  **Normal flow, everything in mill-go, with discussion.md flagging the wrong text loudly** — keeps mill's phase model clean but spends a review round on pushback risk.
- Note: landing early helps *this* task's reviewers only.
  Other worktrees get the corrected text when this branch merges — which is the argument for keeping the task to one batch and merging it promptly, not for widening it.

### Stale doc comments are fixed, not just recorded

- Decision: every comment site that restates a corrected rule is fixed in the same task — including sites that are stale *today*, independent of any change this task makes (`internal/lyxtest/leaf_enforcement_test.go`'s header, which claims lyxtest "imports only stdlib and internal/lyxcwd").
- Rationale: audit question 1 is "does the stated rationale still match what the check enforces?", and a package's own doc comment is where a developer actually reads the rationale — more often than `CONSTRAINTS.md`.
  Leaving a corrected rule contradicted by its own package doc reproduces the exact drift being fixed.
  These are comment-only edits with no behaviour change.
- Rejected: **`CONSTRAINTS.md`-only, per the brief's literal scope line** — leaves five known-wrong comments in the tree and guarantees a follow-up task.

### The sweep boundary is verified, not assumed

- Decision: `perchengine`/`burlerengine`'s "never imports fabricengine" claims and `shuttleengine`'s style cross-reference were each checked and found accurate, and are recorded as deliberately untouched rather than silently skipped.
- Rationale: a sweep that reports only what it changed cannot be distinguished from a sweep that missed things.
  These four sites pattern-match the treadle overstatement closely enough that a later reader will wonder;
  recording the negative result with its evidence closes that.
- Rejected: **widening scope to also audit the fabric-blindness claims** — they are a different invariant family, not among the seven, and they are true.

## Technical context

**The enforcement tests.** All are near-identical allowlists — `internal/modelspec`, `internal/tokenvocab`, `internal/pattern`, `internal/githubclient`, `internal/treadleengine` — except `internal/lyxtest`, which is the odd one out with a denylist.
`internal/scoutengine`'s is deliberately excluded from this comparison: `scout-seam-conversion` is changing it concurrently, so its shape is not stable at merge time and must not be used as the model to copy.
Use **`internal/pattern/leaf_enforcement_test.go` as the single shape reference.**
The shared allowlist shape is worth copying verbatim for the lyxtest conversion:

- `runtime.Caller(0)` + `filepath.Dir` to resolve the package directory independent of `go test`'s working directory.
- `filepath.WalkDir` skipping directories, `*_test.go`, and non-`.go` files.
- `go/parser.ParseFile(..., parser.ImportsOnly)` — deliberately AST-based so string literals in doc comments never produce false positives.
- The stdlib test: no `.` in the first path segment (`fmt`, `os`, `go/parser` pass;
  anything needing a registered TLD contains one).
- Accumulate `failures` as `relPath + ": " + importPath`, then one `t.Errorf` naming the allowed set in the message.

`internal/pattern/leaf_enforcement_test.go` is the smallest instance (single-entry allowlist) and the model to copy.
`internal/treadleengine/seam_enforcement_test.go` is the closest in allowlist size, if a three-entry example is more useful.

**The existing lyxtest test.** `TestLeafInvariant` (note: no `_AllowlistOnly` suffix, unlike its siblings) walks the same way but matches each import against a nine-entry `bannedImports` slice with a nested loop.
The conversion replaces that slice with an `allowedImports map[string]bool` and inverts the check.
The rename to `TestLeafInvariant_AllowlistOnly` is **decided**, not a judgement call — see the "lyxtest's test is renamed" decision, and note it drags `internal/shuttleengine/seam_enforcement_test.go:22` and the `CONSTRAINTS.md` enforced-by line along with it.

**`internal/lyxtest` production files** — `doc.go`, `hermetic.go`, `lyxtest.go`, `reexecguard.go`.
No `//go:build` constraints on any of them (the only constrained files are `bench_test.go` and `lyxtest_test.go`, which the walk skips).
Full non-stdlib import set across all four: `internal/configengine`, `internal/lyxcwd`, `internal/weftname`.
Stdlib used: `fmt`, `io`, `os`, `os/exec`, `path/filepath`, `strings`, `sync`, `testing`.

**`internal/treadleengine`'s two comment sites differ in severity.**
`doc.go:147` opens the "Geometry-blindness and fabric-blindness" section with "treadleengine never imports internal/lyxcwd and never constructs a `_lyx` path itself" and then describes the `runDir`/`GateDir` mechanism correctly in the following clause — so the section is right about the posture and misleading only in its opening framing.
`engine.go:6` carries a compressed version on the `Engine` type doc: "Engine is fabric-blind and geometry-blind: it never imports fabricengine/lyxcwd and never constructs a `_lyx` path itself".
Both need the isolation reading removed;
neither needs the full two-sentence `CONSTRAINTS.md` treatment, since the surrounding prose already carries the mechanism.

**`internal/modelspec/leaf_enforcement_test.go:4`** reads "Unlike lyxtest's leaf_enforcement_test.go (a banned-import denylist), this check is an ALLOWLIST".
After the conversion both are allowlists, so the contrast is simply deleted or reworded — the sentence's remaining purpose (explaining *why* an allowlist: no list maintenance, future stray dependencies caught automatically) is worth keeping.

**Markdown formatting.** `CLAUDE.md` mandates semantic line breaks in every `.md` file — one sentence per line, plus a break at internal independent-clause boundaries.
Never hard-wrap at a fixed column.
This applies to the `CONSTRAINTS.md` edits (already done) and to this file.

**Go comment style.** The `golang-comments` skill governs godoc and inline comment formatting;
the repo recently ran a semantic-line-break reflow over Go doc comments (commit `99fccc55`), so new comment text should match that convention.

## Constraints

From `CONSTRAINTS.md`, binding on this task:

- **The file states rules only** (line 5) — no rationale, no incident narratives, no historical justification.
  This is the constraint that rules out verdict lines and drove the "No stored audit artifact" decision.
  The treadle correction stays inside it because "what the exclusion enforces" is rule content, not history.
- **Cwd Resolution Invariant** — untouched by this task, but it is the invariant the treadle wording orbits.
  Note its own bullet: "`internal/lyxcwd`'s own imports are capped at stdlib plus `internal/gitexec` — this is what keeps `fabricengine` → `logger` → `lyxcwd` acyclic", which is the same `logger` → `lyxcwd` edge that defeats treadle's exclusion.
- **lyxtest Leaf Invariant** — being edited;
  the cycle it prevents (`lyxtest` → feature → `lyxtest`, closed because feature packages' internal tests import lyxtest) must survive the conversion intact, and does.
- **Treadle Runner-Seam Invariant** — being edited;
  the `burlerengine`/`*cli` half of the rule is untouched and still correct.
- **Fabric Vocabulary Invariant** — binds the treadle comment rewrites, and is machine-checked.
  `internal/treadleengine` is **not** in the owner set, and `TestEnforcement_FabricVocabulary` (`internal/lyxcwd/enforcement_test.go`) scans identifiers, string literals, **and comments** in production `.go` files under `internal/`.
  So the rewritten `doc.go`/`engine.go` comments must not introduce `weft` or `warp` as bare tokens, nor a fabric-sense `host` phrase (`host repo`, `host worktree`, `host branch`, …).
  The bare word `fabric` is unpoliced, so the existing "fabric-blind and geometry-blind" phrasing is safe to keep — the risk is only in reaching for fabric vocabulary while rewriting.
  `internal/lyxtest` *is* in the owner set, so its `doc.go` edit is unconstrained here.
- **Test Tier Purity Invariant** — all enforcement tests are untagged and must stay so;
  they parse ASTs and spawn nothing, so no tier concern arises as long as the conversion introduces no `exec.Command`, `gitexec.RunGit`, or `lyxtest.Copy*` call.
  It introduces none.
- **Documentation Lifecycle** — per `CLAUDE.md`, docs land in the same commit as the change.
  Here the affected docs *are* the change.
  No `manifest/designs/` module doc covers any of the seven packages, so none needs updating;
  `docs/overview.md` is unaffected (no module table or execution-stack change);
  `CONSTRAINTS.md` is handled;
  `manifest/roadmap.md` explicitly does not move.

From `CLAUDE.md`:

- **Worktree isolation** — all work stays in `wts/leaf-invariant-audit`.
  Never push to `main` from this worktree;
  the corrections reach other worktrees on merge, not before.
- **No new cross-cutting invariant** is introduced, so `CONSTRAINTS.md` needs no new section.

Discovered during discussion:

- **`scout-seam-conversion` runs in parallel and owns the Scoutengine sections.**
  The two tasks were designed to touch disjoint `CONSTRAINTS.md` line ranges.
  This task's two edits are at the lyxtest section (~34-40) and the treadle bullet (~55-56), both above the scoutengine section (~66-73) — no overlap, but a merge conflict in `CONSTRAINTS.md` is still possible if the other task reflows surrounding lines.
  Resolve by keeping both sides' section edits, never by reverting either.

## Testing

No new test *cases* are added.
The task changes one test's enforcement mechanism and otherwise edits comments.

**`internal/lyxtest/leaf_enforcement_test.go` — the one real change.**
This is the TDD candidate, in the narrow sense available for a guard test: the conversion is only correct if the new allowlist both passes on the current tree and would fail on a violation.

- It must pass unmodified against `internal/lyxtest`'s current four production files.
- It must still reject every path the old denylist rejected — `internal/configreg`, `boardengine`/`boardcli`, `ideengine`/`idecli`, `selfreportengine`/`selfreportcli`, `fabricengine`/`fabriccli` — which the allowlist gives by construction, since none is in the allowed set.
- It must additionally reject a path the denylist silently permitted (any other `internal/*` package, or a new third-party module).
  Confirming this is the *point* of the conversion.
  **How to confirm it is decided, not left to mill-plan:** add a throwaway import to a production file locally, watch the test fail, revert, and record in the commit message that this was done.
  Do **not** extract a table-driven matcher helper to test the check directly — no sibling enforcement test does that, and introducing the pattern in the one file this task exists to bring *into* line with its siblings would be self-defeating.

**Regression bar for the whole task** (agreed during discussion):

- `go test ./internal/lyxtest/... ./internal/modelspec/... ./internal/treadleengine/... ./internal/tokenvocab/... ./internal/pattern/... ./internal/shuttleengine/... ./internal/githubclient/...` — all seven audited packages, all green.
- `go build ./...` — comment-only edits cannot break it, but the treadle and lyxtest doc comments sit directly above `package` clauses where a malformed edit can.
- The full `go test ./...` untagged tier, since `internal/lyxtest` is imported by most test packages in the repo and a mistake in its allowlist would surface as a compile failure across many of them.

**No test is needed for the comment edits.**
Their correctness is a review obligation.
The reviewer's check is that no remaining comment in the tree asserts an isolation property that the import graph does not provide — the audit's sweep list in **Audit results** is the checklist.

## Q&A log

- **Q:** What is a "verdict" concretely, and why does the audit need storing at all? **A:** It doesn't.
  Storing it was rejected — the `go list` work it would save is one command, the doc rots into false confidence while the enforcement tests fail loudly instead, and `CONSTRAINTS.md` line 5 bans exactly that kind of content.
  Only the corrections the audit found are durable;
  the reasoning goes in the commit message.
- **Q:** Should the two wrong rule texts be fixed here, or reported as a follow-up? **A:** Fixed here.
  A wrong rule in the file reviewers follow slavishly costs more every day it stands than the fix does.
- **Q:** Should the stale Go doc comments be fixed too, or is this `CONSTRAINTS.md`-only per the brief's scope line? **A:** Fixed, including ones stale today independent of this task.
  A package doc is where developers actually read the rationale.
- **Q:** Should `lyxtest`'s denylist be converted to an allowlist, or should the gap just be recorded? **A:** Converted.
  The original "record only" recommendation was procedural ("this is an audit, not a refactor"), not technical.
  Verified feasible: four production files, no build constraints, exactly three internal imports, allowlist holds with zero source changes, and it is strictly stronger than the denylist on the denylist's own cycle-prevention purpose.
- **Q:** When does the `CONSTRAINTS.md` fix land, given reviewers follow that file slavishly? **A:** During mill-start, before discussion review — but only `CONSTRAINTS.md`.
  Reviewers then read corrected text *and* review the correction.
  Go doc comments and the test conversion stay in mill-go so the task retains a real implementation batch.
- **Q:** Beyond the two approved comment sites, are there other files whose claims are no longer valid? **A:** Three more — `internal/lyxtest/doc.go`, `internal/treadleengine/engine.go`, `internal/modelspec/leaf_enforcement_test.go`.
  Four further sites (`perchengine` ×2, `burlerengine` ×2) pattern-match but were verified true (`fabricengine` absent from both closures) and are recorded as deliberately untouched.
- **Q:** (discussion review r1) Is `TestLeafInvariant` renamed, and if so what does that pull into scope? **A:** Renamed to `TestLeafInvariant_AllowlistOnly`, decided here rather than deferred to mill-plan.
  It pulls in `internal/shuttleengine/seam_enforcement_test.go:22` (names the function) and a third `CONSTRAINTS.md` edit (the lyxtest "Enforced by" line, the only one of seven omitting its test name).
  Deferring it had made the sweep's verified-untouched list conditional on an open question, which is why it could not stay deferred.
- **Q:** (discussion review r1) Is `CONSTRAINTS.md` closed to further edits, given mill-start already landed two? **A:** No — closed only to redoing those two.
  The enforced-by line is a mill-go edit, deliberately not made during mill-start because it would forward-reference a function that does not exist yet.
- **Q:** (discussion review r2) Which machine-enforced invariant binds the treadle *comment* rewrites? **A:** The Fabric Vocabulary Invariant — `treadleengine` is not in its owner set, and the enforcement test scans comments, not just identifiers.
  Bare `fabric` is unpoliced so the existing "fabric-blind" wording is safe;
  the constraint is on not reaching for `weft`/`warp`/fabric-sense `host` while rewriting.
  Now listed under Constraints.
- **Q:** Is the treadle finding as the brief described it? **A:** Worse.
  The brief expected one transitive path via `logger`;
  the allowlist also permits `shuttleengine`, which imports `lyxcwd` directly.
  Verdict is unchanged (keep-and-sharpen), the evidence is stronger.
