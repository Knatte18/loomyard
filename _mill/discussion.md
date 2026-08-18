# Discussion: pattern told-geometry

```yaml
task: pattern told-geometry
slug: pattern-told-geometry
status: discussing
parent: standalone-producers
```

## Problem

`internal/pattern` is a leaf package whose whole job is to answer "is PATTERN active in this worktree, and what directive text should this agent role be handed?".
It needs exactly one path to do that: the directory that `_lyx/PATTERN.md` sits under.
Today it takes a `*lyxcwd.Location` instead and derives that directory itself, via `FileHere(l)` → `File(filepath.Join(l.WorktreePath(), l.AnchorRel))`.
That derivation is the package's only reason to import `internal/lyxcwd` at all, and `internal/lyxcwd` is the hub-geometry resolver — a package that, by construction, cannot exist in a standalone (non-hub, possibly non-git) run.

**Why now.** This is T4 of [`manifest/designs/producers-standalone.md`](../manifest/designs/producers-standalone.md), the first task of wave 2.
Wave 1 (commit `b98ee2ba`) already converted `shuttleengine`, `reedengine` and `tokenvocab` to told geometry on exactly this rule, so the precedent, the review vocabulary and the test-relocation pattern all already exist in-tree.
`pattern` blocks wave 3: T6 (`burlerengine`+`perchengine`) and T7 (`websterengine`+`webstercli`) both list T4 as a dependency, because `pattern.Directive` is called from `burlerengine/engine.go` and `websterengine/render.go`, whose own signatures those tasks change.
The design doc also records that `pattern` was never a standalone *blocker* — `Directive` already returns `("", nil)` for a nil `Location` — so this is a signature and dependency cleanup, not a behaviour fix.

## Scope

**In:**

- `internal/pattern/pattern.go` — `Directive(l *lyxcwd.Location, stencilsDir string, role Role)` becomes `Directive(anchorPath, stencilsDir string, role Role)`; `isActive(l *lyxcwd.Location)` becomes `isActive(anchorPath string)`; `FileHere` is deleted. Two doc comments in the same file go stale with the signatures and are rewritten with them: `Directive`'s at 84–85 ("for a nil `l`") and `isActive`'s at 120 ("an absent `FileHere(l)`").
- `internal/pattern/doc.go` — the package godoc's references to `FileHere` (13–14), to the nil-layout early return (69, "no read is attempted on a nil layout"), and the "Why the pointer stays relative" section's "an interpolated absolute path built from a `Location` field" (55), which after this change should name a caller-supplied path rather than a `Location` field.
- `internal/pattern/pattern_test.go` — the `layoutAt` helper and **both** sites that pass a literal `nil`: `TestDirective_NilLayout` (182) and `TestDirective_LazyRead`'s "nil layout, stencilsDir does not exist" sub-test (383).
- `internal/pattern/patternpath_test.go` — the two `FileHere`-only tests, the `newTestLocation` helper, and the file-header comment's "the `File/FileHere` constructors" phrasing (line 1).
- `internal/pattern/leaf_enforcement_test.go` — the allowlist map, the file-header comment and the failure message.
- Call sites, argument only: `internal/burlerengine/engine.go` (103), `internal/websterengine/render.go` (174, 237), `internal/loomengine/plan.go` (71).
- `internal/loomengine/plan_test.go` — gains one **new** test carrying the relocated PATTERN-anchoring proof; the existing `TestPlanSpec_AnchoredUnderAnchorPathNotWorktreePath` is not modified.
- `cmd/lyx/constructoranchoring_test.go` — the two `pattern.FileHere` rows (88, 144).
- `internal/websterengine/template_test.go` — the stale `PatternFileHere()` / `layoutAt` comment references (194–195); comments only, no test-logic change.
- `CONSTRAINTS.md` — the Pattern Leaf Invariant's allowlist sentence.

**Out:**

- **Engine and CLI signatures.** `burlerengine.New`, `websterengine.Render*`, `loomengine.PlanSpec` keep their `*lyxcwd.Location` parameters; each call site just computes `l.AnchorPath()` inline at the `Directive` call.
  T6 and T7 own those signature changes and this task must not pre-empt them.
- **`internal/hubgeom`.** T3 added `ReedGeometry` there because reed needed a multi-field struct converted from a `Location`. `pattern` needs one string, so no helper and no new `hubgeom` export.
- **`pattern.File`, `PathspecFile`, `PathspecDir`.** Unchanged in signature, value and semantics. `File` is already exported and already correct.
- **The active-check semantics.** Pure existence, directory-means-inactive, non-`IsNotExist` stat error means active — all three edge cases and their tests survive verbatim.
- **`docs/overview.md`.** Its two `internal/pattern` lines (241, 315) describe what the package computes, not how it is parameterised, so they stay accurate as written.
- **`manifest/roadmap.md`.** The Planned entry this task belongs to ("producers standalone: mid-layer") covers *two* tasks, T4 and T5. T4 alone does not complete it, so the roadmap does not move in this commit.
- **Nested-hub / anchoring behaviour.** `FileHere(l)` is `File(l.AnchorPath())` for every `Location` — an equality currently pinned by `TestFileHere_EqualsFileOfAnchorPath` — so passing `l.AnchorPath()` at each call site is byte-identical to today's resolution in every **populated** geometry, root-anchored and subpath-anchored alike.
  The one input where the change is *not* byte-identical is a zero-value or partially-zero `*lyxcwd.Location`: `AnchorPath()` on one is `filepath.Join("", "", "")` = `""`, so today `FileHere` hands `os.Stat` the cwd-relative `"_lyx/PATTERN.md"`, whereas after this change the empty-string guard returns inactive with no stat at all. That is a deliberate improvement, not a regression — see the *empty-string-guard-placement* decision — but it means the equivalence claim is scoped to fully-populated Locations and must not be stated unqualified.

## Decisions

### empty-string-guard-placement

- **Decision.** `Directive` keeps an explicit early return for `anchorPath == ""`, positioned exactly where today's `if l == nil` return sits, before the `isActive` call. `isActive` is never handed the empty string.
- **Rationale.** This is a correctness guard, not a stylistic one. `File("")` is `filepath.Join("", "_lyx", "PATTERN.md")` = `"_lyx/PATTERN.md"` — a *relative* path. Handing that to `os.Stat` resolves it against the process working directory, so an empty anchor path in a repo that happens to have an `_lyx/PATTERN.md` beside the cwd would report PATTERN **active** and read a stencil, instead of the documented inactive no-op. Guarding in `Directive` also preserves the existing control-flow shape one-for-one, which keeps the diff reviewable against the "preserving the existing `("", nil)` behaviour exactly" wording in T4's brief.
  Note that this guard is strictly *better* than what it replaces, not merely equivalent to it: today a zero-value `*lyxcwd.Location` — non-nil, so it slips past the `l == nil` branch — makes `FileHere` produce that same cwd-relative path and stat it for real. The empty-string guard closes that path. The `("", nil)` return is preserved exactly; the stray stat is not preserved, and is not meant to be.
- **Rejected.** Putting the guard inside `isActive` — equally safe, and arguably one fewer branch in `Directive`, but it loads a sentinel meaning onto a predicate whose whole documented job is "stat this path", and it moves the `("", nil)`-without-a-read contract out of the function whose doc comment states it.

### told-value-is-anchorpath

- **Decision.** Every one of the four call sites passes `l.AnchorPath()`. No `hubgeom` helper, no new exported constructor, no stored field.
- **Rationale.** T4's brief derives it directly: `isActive(l)` → `FileHere(l)` → `File(Join(l.WorktreePath(), l.AnchorRel))`, which *is* `File(l.AnchorPath())`. The equality is currently test-pinned. A single string parameter has nothing for a geometry struct to carry, and `hubgeom` exists for the multi-field conversions (T3's `ReedGeometry`, T6/T7's `BurlerGeometry`/`PerchGeometry`/`WebsterGeometry`), not for one accessor call.
- **Rejected.** A `hubgeom.PatternAnchor(l)` wrapper — pure indirection over `l.AnchorPath()`, and it would put a `hubgeom` import into three packages that do not otherwise need one.

### burlerengine-computes-inline

- **Decision.** `internal/burlerengine/engine.go:103` becomes `pattern.Directive(e.layout.AnchorPath(), e.stencilsDir, pattern.RoleReviewFix)`. No `anchorRoot` field is added to `Engine`, and `New`'s signature is untouched.
- **Rationale.** T6 is the task that converts `burlerengine.New(shuttle, layout, cfg, stencilsDir)` into `New(shuttle, worktreeRoot, anchorRoot string, cfg, stencilsDir)`. Adding a field here would either duplicate that work or conflict with it, and T4 and T6 are in different waves precisely so the file contention on `engine.go` is serialised, not merged.
- **Rejected.** Threading an `anchorRoot` field now, to "save a step later" — that is T6's step, and doing it early lands an unused-by-anything-else field.

### nil-guard-value-moves-to-the-call-sites

- **Decision.** The defensive nil-`Location` guard does not get re-created at any call site. It disappears with the signature.
- **Rationale.** `TestDirective_NilLayout`'s comment justifies the guard as protection for "several Deps structs assembled field-by-field by CLI callers that could leave Layout unset". That protection is already dead at all four call sites today: `burlerengine/engine.go` dereferences `e.layout.WorktreePath()` at line 97, six lines *before* the `Directive` call; `websterengine/render.go` dereferences `l.HubPath` at 173 and 236, one line before each call; `loomengine/plan.go` dereferences `layout` at 67–68, three lines before. A nil `Location` panics in every one of those functions regardless of what `pattern` does. Re-creating the guard would add a branch that changes no observable outcome.
- **Rejected.** Adding `if l != nil` wrappers at each call site — cargo-culting a guard whose stated risk is already unmitigated by the surrounding code.

### anchoring-proof-relocates-to-the-call-site

- **Decision.** `cmd/lyx/constructoranchoring_test.go`'s two `pattern.FileHere` rows (88, 144) are rewritten as `pattern.File(l.AnchorPath())` and folded into the tautology-comment block the `planparser` rows immediately above them already carry. That block is **amended, not duplicated**: it currently opens "The two planparser rows below…" and points at `internal/loomengine/plan_test.go` and `internal/webstercli/verbs_test.go`, so copying it verbatim onto a third row would leave both the count and the pointer list wrong. Widen the count to cover the `pattern.File` row and add this task's new `plan_test.go` case to the pointer list. The real proof is added to `internal/loomengine/plan_test.go` as a **new sibling test** — working name `TestPlanSpec_PatternDirectiveAnchoredUnderAnchorPath` — built on a real `t.TempDir()`. The existing `TestPlanSpec_AnchoredUnderAnchorPathNotWorktreePath` (line 284) is **not** modified.
- **Rationale.** This is verbatim the pattern T3 established for `planparser.PlanDir`/`PlanOverview` in the same file. A row that passes `l.AnchorPath()` in and compares against an `anchor`-derived expectation can no longer catch a production call site that passes the wrong root — it only pins join arithmetic and `_lyx`-vs-`.lyx` group placement. Keeping the row preserves the group-membership signal; adding the call-site case restores the anchoring signal that the rewrite would otherwise silently drop.
  The proof must be a *new* test rather than an extension of the existing one because that test builds its layout from `filepath.Join("home", "user", "repo")` — a synthetic, **relative**, never-created path, chosen deliberately (its own comment at lines 281–283 says so) so the case stays pure string arithmetic that touches no filesystem. Writing a real `_lyx/PATTERN.md` "under the anchor subpath" of that layout would create `home/user/repo/backend/_lyx/` **relative to the test's working directory**, i.e. inside `internal/loomengine/` in the source tree, and the directive would then be read out of a polluted repo path. A PATTERN fixture needs a real root; that root has to be `t.TempDir()`.
- **Rejected.** (a) Deleting rows 88/144 outright — `TestFile_Free` already covers `File`'s join, but the constructor-anchoring table's value is that it is an exhaustive roster of worktree-level constructors and their group; a hole in the roster is what lets a future constructor be added to the wrong group unnoticed. (b) Rewriting the rows with no relocated proof — that is the silent coverage loss T3 explicitly refused. (c) Re-rooting the existing `TestPlanSpec_AnchoredUnderAnchorPathNotWorktreePath` on `t.TempDir()` — mechanically viable (every assertion in it derives from `layout.AnchorPath()`/`WorktreePath()` rather than a path literal, so none would need rewriting), but it converts a deliberately filesystem-free arithmetic case into a fixture-writing one and invalidates the comment that explains its construction. (d) Putting the proof in `internal/burlerengine`, whose smoke tests already build real temp trees — that is T6's file, and T4/T6 are in separate waves specifically to serialise contention on it.

### lyxcwd-leaves-the-package-entirely-including-tests

- **Decision.** `internal/lyxcwd` is removed from every file in `internal/pattern`, test files included. Concretely: `patternpath_test.go` loses `TestLocation_PatternAccessors`, `TestFileHere_EqualsFileOfAnchorPath` and the now-unused `newTestLocation`; `pattern_test.go`'s `layoutAt` helper is replaced by passing the temp-dir root string directly.
- **Rationale.** `TestLeafInvariant_AllowlistOnly` skips `_test.go` files, so leaving `lyxcwd` in the tests would pass green while quietly keeping the dependency alive in `go list` terms and re-seeding it for the next person editing the file. Both deleted tests exist *only* to pin `FileHere`, which no longer exists — `TestLocation_PatternAccessors` asserts `FileHere(l) == File(Join(WorktreePath(), AnchorRel))`, and `TestFileHere_EqualsFileOfAnchorPath` asserts `FileHere(l) == File(l.AnchorPath())`. Both are tautologies once the caller passes `l.AnchorPath()` itself.
- **Rejected.** Keeping the two tests rewritten onto `File(l.AnchorPath())` — that asserts `File(x) == File(x)`.

### empty-anchorpath-test-asserts-no-stat

- **Decision.** `TestDirective_NilLayout` is replaced by `TestDirective_EmptyAnchorPath`, which keeps the `("", nil)` assertion **and** additionally asserts, through the package's existing `statFile` seam, that no stat call was attempted.
- **Rationale.** The `("", nil)` assertion alone would pass even if the guard were removed, because an inactive PATTERN also returns `("", nil)` — and in a test process whose cwd has no `_lyx/PATTERN.md`, statting the relative `"_lyx/PATTERN.md"` returns `IsNotExist` and yields the same result. Only asserting the *absence* of the stat distinguishes the guard from its accidental cwd-dependent lookalike, and it is the same assertion that pins `Directive`'s documented "no read attempted" contract. `statFile` is already a package-level `var` declared for exactly this kind of test substitution, so no new seam is introduced.
- **Rejected.** Asserting the `("", nil)` return only — passes green with the guard deleted, i.e. tests nothing this decision cares about. Chdir-ing the test process into a fixture containing `_lyx/PATTERN.md` — `os.Chdir` is process-global and unsafe against Go's parallel test execution.

### constraints-and-docs-in-the-same-commit

- **Decision.** The commit updates exactly five things: `CONSTRAINTS.md` **line 157 only** (the Pattern Leaf Invariant's allowlist sentence — the `- **Enforced by**` bullet at line 163 names the test file and nothing else, carries no allowlist, and stays byte-identical, exactly as T3 left `tokenvocab`'s); `internal/pattern/doc.go`; `internal/pattern/leaf_enforcement_test.go`'s file-header comment and its failure message; and the stale `PatternFileHere()`/`layoutAt` references in `internal/websterengine/template_test.go` (194–195). No `docs/overview.md` change and no `manifest/roadmap.md` move.
  Nothing under `manifest/designs/` is touched either. Two files there hold references that this task makes stale — `pattern-directive-stencils.md:7` still writes the older `internal/pattern.Directive(l, role)` spelling, and `producers-standalone.md:337` still carries T4's pre-`b98ee2ba` line numbers — and both are left alone on purpose: design docs are historical records of what was decided, not living API documentation, and rewriting a shipped design doc to match the tree destroys the record of what the task was actually planned against.
- **Rationale.** This is the repo's documented Task-completion rule: a task changing cross-cutting infrastructure updates its docs in the same commit. The invariant text is the *narrowing* this task delivers, so it cannot lag. `docs/overview.md` describes `pattern` behaviourally and stays true. The roadmap's "producers standalone: mid-layer" entry spans T4 **and** T5; moving it on T4 alone would mark unshipped work as done.
- **Rejected.** Deferring the `CONSTRAINTS.md` narrowing to T10 (the invariants-and-docs consolidation task) — T10 lands the *new* three-tier told-geometry rule; leaving a stale allowlist standing for three waves in the meantime is exactly the silent-rot the Documentation Lifecycle exists to prevent.

## Technical context

**The package as it stands** (`internal/pattern/pattern.go`, 134 lines):

- `File(baseDir string) string` → `filepath.Join(baseDir, lyxdirs.LyxDirName, patternFileName)`. Exported, keeps its signature.
- `FileHere(l *lyxcwd.Location) string` → `File(filepath.Join(l.WorktreePath(), l.AnchorRel))`. **Deleted.**
- `Directive(l, stencilsDir, role) (string, error)` — nil check, `isActive` check, a `Role`→stencil-name switch with a documented `default` no-op, then `stencilstore.Read` + `stencil.StripLeadingComment`.
- `isActive(l) bool` — `statFile(FileHere(l))`; `IsNotExist` → inactive, any other error → **active** (deliberate: fail loud in the agent's own read rather than silently disabling constraints), a directory → inactive.
- `var statFile = os.Stat` — an existing test seam, documented as never reassigned in production.
- `PathspecFile` / `PathspecDir` — forward-slashed git-pathspec constants consumed by `internal/fabricengine`. Untouched.

**The four call sites, with their current pre-existing dereference of `l`:**

| File:line | Current arg | New arg | `l` already dereferenced at |
|---|---|---|---|
| `internal/burlerengine/engine.go:103` | `e.layout` | `e.layout.AnchorPath()` | line 97 (`e.layout.WorktreePath()`) |
| `internal/websterengine/render.go:174` | `l` | `l.AnchorPath()` | line 173 (`fabricengine.StencilsDir(l.HubPath)`) |
| `internal/websterengine/render.go:237` | `l` | `l.AnchorPath()` | line 236 (same) |
| `internal/loomengine/plan.go:71` | `layout` | `layout.AnchorPath()` | lines 67–68 |

The last column is the evidence for the *nil-guard-value-moves-to-the-call-sites* decision above.
Note that `render.go:183` and `plan.go:68-69` already call `l.AnchorPath()` in the same function, so two of the four sites are literally re-using a value already on screen.

**`internal/loomengine/plan_test.go`** already has a subpath-anchored `PlanSpec` case, `TestPlanSpec_AnchoredUnderAnchorPathNotWorktreePath` (284), built with `AnchorRel: "backend"` and deliberately not sharing the file's other tests' zero-value `AnchorRel`. It is the right *shape* to copy but the wrong body to extend: its root is `filepath.Join("home", "user", "repo")` — relative and never created on disk — and its own comment (281–283) records that being pure, filesystem-free arithmetic is the point. The relocated PATTERN proof therefore lands as a new sibling test on a `t.TempDir()` root; see the *anchoring-proof-relocates-to-the-call-site* decision and the Testing section for the fixture. `newTestStencilsDir(t)` is the existing stencils-seeding helper both cases use — declared in the **same package but a different file**, `internal/loomengine/prompt_test.go:21`, not in `plan_test.go`.

**`internal/websterengine/template_test.go`** has two fixtures whose comments reference `pattern` internals: `testLayout` (line 184, whose comment explains that its worktree subdirectory is never created so PATTERN always resolves inactive) and `patternActiveLayout` (line 198, whose comment at 194–195 names `pattern.isActive`'s "`PatternFileHere()` check" and `pattern_test.go`'s `writePatternFile`/`layoutAt` fixtures). Both fixtures keep working unchanged — they build `*lyxcwd.Location` values for `Render*`, which still take one. Only the comment text needs correcting, and note that `PatternFileHere()` is *already* a stale name today (the accessor has been `FileHere` since it moved into this package).

**T3's precedent to mirror** (commit `b98ee2ba`): the `CONSTRAINTS.md` narrowing of the Tokenvocab Leaf Invariant is the exact shape of edit this task makes to the Pattern Leaf Invariant — one sentence, `internal/lyxcwd` struck from the allowlist, the enforcement bullet left intact.

**Enumeration obligation.** The design doc requires re-running the caller enumeration rather than trusting its `Files` list. Done: `grep -rn "pattern.Directive\|FileHere"` across `*.go` returns exactly the files listed under Scope. `FileHere` has no callers outside `internal/pattern` and `cmd/lyx/constructoranchoring_test.go`.

## Constraints

From [`CONSTRAINTS.md`](../CONSTRAINTS.md):

- **Pattern Leaf Invariant** (line 155). Today: stdlib + `lyxcwd` + `lyxdirs` + `stencilstore` + `stencil`. This task narrows it to stdlib + `lyxdirs` + `stencilstore` + `stencil`. Enforced by `internal/pattern/leaf_enforcement_test.go` (`TestLeafInvariant_AllowlistOnly`), which is an allowlist walk over non-test `.go` files — so tightening the map is what makes the narrowing machine-checked. The invariant text, the test's header comment, its `allowedImports` map and its failure message must all be narrowed together or they diverge.
- **Cwd Resolution Invariant.** `internal/lyxcwd` alone resolves cwd; each module owns its own relative subpath and is *told* its root. This task is a direct application: `pattern` stops knowing about hub geometry and is told its anchor directory.
- **Documentation Lifecycle.** Docs land in the same commit as the change — see the *constraints-and-docs-in-the-same-commit* decision for the exact file set.
- **Fabric Vocabulary walk.** `internal/lyxcwd/enforcement_test.go`'s `TestEnforcement_GeometryLiterals` polices bare geometry tokens by whole-token equality. `pattern`'s `doc.go` and `pattern.go` already comply by building every path from `lyxdirs.LyxDirName` rather than a `"_lyx"` literal, and `doc.go` records that this is a **review obligation, not a mechanically-enforced one** for this package (the invariant's test cannot see `"_lyx/PATTERN.md"` as a token). Any doc-comment rewrite in this task must preserve that property and must not introduce a bare `_lyx` token into Go source.
- **CLI / Cobra Invariant.** Not engaged — this task adds no flag, no command and no observable CLI behaviour.

Discovered during exploration:

- **`filepath.Join("", …)` yields a relative path.** The empty-string sentinel is only safe because `Directive` returns before it reaches `File`. See the *empty-string-guard-placement* decision.
- **`TestLeafInvariant_AllowlistOnly` does not scan `_test.go` files.** Tightening the allowlist alone would not catch a `lyxcwd` import surviving in this package's tests.
- **The new signature loses a compile-time transposition check.** Today `Directive(l *lyxcwd.Location, stencilsDir string, role Role)` makes swapping the first two arguments a type error. After the change both leading parameters are `string`, so `Directive(stencilsDir, anchorPath, role)` compiles cleanly at any of the four call sites and fails only at runtime — silently, as an inactive PATTERN, since the stencils directory almost never contains an `_lyx/PATTERN.md`. This is inherent to told geometry and is accepted rather than designed around (a named `type AnchorPath string` would be new machinery T4's brief does not ask for, and would break the plain-string contract every other told-geometry package in waves 1–3 uses). The mitigation is behavioural: `websterengine/template_test.go`'s `patternActiveLayout` fixture and the new `internal/loomengine` PATTERN case both assert a **non-empty** directive, so a transposed call site turns them red. Name that reliance explicitly in review — it is the only detector.
- **T4 and T5 are parallel-safe but share the wave.** T5 touches `internal/loomengine` (lifting the orchestrator preflight out of it). T4 touches `internal/loomengine/plan.go` and `plan_test.go`. These are different files from T5's preflight extraction, but a rebase conflict in the package is plausible — worth a `git pull --rebase` before the final commit rather than a design change.

## Testing

**`internal/pattern` — the package's own suite.** TDD candidate: `TestDirective_EmptyAnchorPath` is written first, against the new signature, before `Directive`'s guard is converted. It is the one test in this task that pins behaviour a naive port would silently break.

- `TestDirective_EmptyAnchorPath` — replaces `TestDirective_NilLayout` (182). Asserts `("", nil)` **and**, via the `statFile` seam, that no stat was attempted. Restore the seam with `t.Cleanup`.
- `TestDirective_LazyRead`'s second sub-test, "nil layout, stencilsDir does not exist" (383), is the package's **other** literal-`nil` site. It is **kept and converted to an empty-string sub-test**, renamed accordingly — not deleted as subsumed. `TestDirective_EmptyAnchorPath` and this sub-test prove different things: the former asserts no *stat* happens against a real stencils directory, the latter asserts no *stencil read* happens against a `stencilsDir` that does not exist, which is the property `TestDirective_LazyRead` exists to pin for all three inactive paths at once. Its sibling sub-test ("PATTERN inactive, stencilsDir does not exist") converts by fixture plumbing alone.
- Every other existing `Directive` case — the three roles, the active/inactive matrix, empty `PATTERN.md`, `PATTERN.md`-as-directory, the non-`IsNotExist` stat error, the banner strip, the unknown/zero role — survives with its assertions unchanged; only the fixture plumbing changes, from `layoutAt(root, ".")` to `root`. These are regression anchors: any assertion that has to be *weakened* to make the port compile is a signal the port is wrong.
- Nested-anchor coverage: today `layoutAt(root, relPath)` carries the nesting. With a told string the caller simply passes the nested directory, so keep at least one case whose anchor path is a subdirectory of the temp-dir root, to prove `Directive` joins onto whatever it is told rather than onto a root it re-derives.
- `patternpath_test.go` — `TestFile_Free`, `TestPathspec_ExactStrings` and `TestPathspec_ForwardSlashedOnEveryPlatform` are untouched. The two `FileHere` tests and `newTestLocation` are deleted; the file's header comment loses its `File/FileHere` phrasing.
- `leaf_enforcement_test.go` — the tightened `allowedImports` map is itself the test. It should fail before the `lyxcwd` import is removed from `pattern.go` and pass after; running it in that order is the cheap proof the narrowing is real rather than decorative.

**`internal/loomengine`.** The relocated anchoring proof is a **new** test — working name `TestPlanSpec_PatternDirectiveAnchoredUnderAnchorPath` — placed beside `TestPlanSpec_AnchoredUnderAnchorPathNotWorktreePath`, which it does not modify. Fixture:

- `HubPath: t.TempDir()`, a `WorktreeName`, and `AnchorRel: "backend"`. A real temp root is mandatory, not a preference: the neighbouring test's `filepath.Join("home", "user", "repo")` root is relative and never created, so any file the fixture writes would land in the source tree under `internal/loomengine/`.
- Create the worktree and anchor directories, write `_lyx/PATTERN.md` under `layout.AnchorPath()` **only** — never under `layout.WorktreePath()`. Seed the pattern-directive stencils into the `stencilsDir` passed to `PlanSpec`. `newTestStencilsDir(t)` is the existing helper, declared in the same package at `internal/loomengine/prompt_test.go:21` — confirm it seeds the three `pattern-directive-*` stencils, and extend it or seed them alongside it if it does not.
- Assert `PlanSpec`'s prompt contains the directive text. The case fails if `PlanSpec` passes `WorktreePath()` instead of `AnchorPath()`, because at the worktree root there is no PATTERN file, so PATTERN resolves inactive and the directive comes back empty. That asymmetry — file present at exactly one of the two candidate roots — *is* the proof; a non-`"."` `AnchorRel` is what makes the two roots distinguishable.
- Pair it with the negative direction in the same test or a sub-test: with `_lyx/PATTERN.md` written at the worktree root instead, the directive must be empty. Without that half, a `PlanSpec` that ignored anchoring entirely and always read the worktree root would still be caught, but a `PlanSpec` that read *both* would not.

**`internal/burlerengine` and `internal/websterengine`.** No new tests. Their existing suites are the regression check that a mechanical argument change compiles and behaves: `websterengine/template_test.go` in particular already covers both an inactive-PATTERN fixture and an active-PATTERN fixture across the `Render*` paths, and both must stay green with byte-identical expectations. `burlerengine`'s smoke tests likewise.

**`cmd/lyx`.** `constructoranchoring_test.go` keeps both rewritten `pattern.File` rows and stays green in both its root-anchored and subpath-anchored tables.

**Verify.**

- Baseline: `go test ./...` from the worktree root.
- The design's named set, for a focused loop: `go test ./internal/pattern/... ./internal/burlerengine/... ./internal/websterengine/... ./internal/loomengine/... ./cmd/lyx/...`.
- Explicit dependency proof, as T4 requires: `grep -rn lyxcwd internal/pattern/` returns nothing at all — production **and** test files.
- Confirm by reading the diff that `leaf_enforcement_test.go`'s allowlist was **tightened**, not left permissive; a green `TestLeafInvariant_AllowlistOnly` over an unchanged map proves nothing.

## Q&A log

- **Q:** Where does the empty-string "inactive" guard live — in `Directive` before `isActive`, or inside `isActive`? **A:** [auto-pick] In `Directive`, before `isActive`. **Why:** `File("")` produces the relative path `_lyx/PATTERN.md`, which `os.Stat` would resolve against the process cwd; guarding in `Directive` also preserves today's `l == nil` control flow one-for-one.
- **Q:** What replaces `TestDirective_NilLayout`? **A:** [auto-pick] `TestDirective_EmptyAnchorPath`, asserting `("", nil)` **and** that no stat was attempted via the `statFile` seam. **Why:** the return-value assertion alone passes green with the guard deleted, because an inactive PATTERN returns the same pair.
- **Q:** What happens to `cmd/lyx/constructoranchoring_test.go`'s two `pattern.FileHere` rows? **A:** [auto-pick] Rewrite onto `pattern.File(l.AnchorPath())` with the tautology comment, and relocate the real anchoring proof into `internal/loomengine/plan_test.go`. **Why:** verbatim the precedent T3 set for the `planparser` rows two lines above them in the same file. **~~Superseded in part by the `[review r1 gap]` entry below~~:** this answer originally said the proof would extend the *existing* subpath-anchored `PlanSpec` case. It does not — that case's root is relative and never created on disk, so it cannot host a fixture. The proof is a new sibling test on `t.TempDir()`. Everything else in this answer stands.
- **Q:** Do the `FileHere`-only tests in `patternpath_test.go` get rewritten or deleted, and does `lyxcwd` leave the test files too? **A:** [auto-pick] Deleted, along with `newTestLocation` and `pattern_test.go`'s `layoutAt`; `lyxcwd` leaves every file in the package. **Why:** both tests become `File(x) == File(x)` tautologies, and the allowlist test skips `_test.go` files so a surviving test import would never be caught.
- **Q:** How far does the leaf-allowlist tightening reach? **A:** [auto-pick] The `allowedImports` map, the test's file-header comment, its failure message, `CONSTRAINTS.md` line 157, and `doc.go`. **Why:** the invariant text and its enforcement must narrow together or they diverge silently.
- **Q:** What value do the call sites pass, and is a `hubgeom` helper introduced? **A:** [auto-pick] `l.AnchorPath()` inline at all four sites; no helper. **Why:** `FileHere(l) == File(l.AnchorPath())` is a currently test-pinned equality, and `hubgeom` exists for multi-field geometry conversions, not for one accessor call.
- **Q:** Does `burlerengine.Engine` gain an `anchorRoot` field now? **A:** [auto-pick] No — compute `e.layout.AnchorPath()` inline at `engine.go:103`. **Why:** T6 owns `burlerengine.New`'s conversion to told strings; T4 and T6 are in separate waves precisely to serialise contention on that file.
- **Q:** Is the nil-`Location` defensive guard re-created at any call site? **A:** [auto-pick] No. **Why:** all four call sites already dereference `l` one to six lines before the `Directive` call, so the guard is dead code today and re-creating it changes no observable outcome.
- **Q:** [review r1 gap] The relocated PATTERN-anchoring proof cannot write a fixture into `TestPlanSpec_AnchoredUnderAnchorPathNotWorktreePath`, whose root is a relative, never-created path — where does the proof go? **A:** [auto-pick] A new sibling test on `t.TempDir()`, leaving the existing test untouched. **Why:** that test's own comment records its filesystem-free arithmetic as deliberate; re-rooting it would work mechanically but change what it is, and `burlerengine` is T6's file.
- **Q:** Which docs move in this commit? **A:** [auto-pick] `doc.go`, `CONSTRAINTS.md`, the leaf-test comments, and the stale `PatternFileHere()` references in `websterengine/template_test.go` — not `docs/overview.md`, not `manifest/roadmap.md`. **Why:** overview describes the package behaviourally and stays true; the roadmap's Planned entry spans T4 **and** T5, so moving it on T4 alone would mark unshipped work done.
