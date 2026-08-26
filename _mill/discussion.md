# Discussion: Fix Bouncer anchor-path and run-dir clearing

```yaml
task: Fix Bouncer anchor-path and run-dir clearing
slug: loom-bouncer-anchor-rundir-fix
status: discussing
parent: main
```

## Problem

Two defects sit in the same shared `Bouncer`/`BurlerRound` code the loom recipe's three review segments are hand-wired out of.
They are folded into one task because splitting them would have two tasks editing the same rows and the same functions.

**Defect 1 — wrong root.**
A review segment's `_lyx` paths resolve against `Env.WorktreeRoot` (`location.WorktreePath()`), while the commit closures that commit those very artifacts anchor at `location.AnchorPath()`.
Two sites carry it: `shedrecipe.bouncerEntry` resolves the row's `artifact_paths` against `env.WorktreeRoot` (`internal/shedrecipe/entries_bouncer.go:119`),
and `hubgeom.BurlerGeometry` fills `burlerengine.Geometry.WorktreeRoot` from `l.WorktreePath()` (`internal/hubgeom/hubgeom.go:38`), which is the root `burlerengine`'s `(*Profile).validate` resolves every `target.paths`/`fasit.paths` entry against (`internal/burlerengine/engine.go:98`).
Both are latent today because `AnchorRel` is `"."` by default and the two roots coincide;
re-pointing the shared anchor would silently change every review segment at once, and the judge and the fixer would read a different tree than the one the commit seam commits.
The `Plan-Bouncer` row's own yaml comment already records the divergence as knowingly deferred to this task.

**Defect 2 — stale run directory on re-entry.**
Neither segment clears its Bouncer run directory when a downstream row bounces back past the writer and control flows through the segment a second time.
`Bouncer.Call` resolves the round with `ResolveRound` (highest N whose `round-<N>-review.md` exists), sees `judged(N)` still true from the previous generation, and `settle` replays the already-settled verdict.
An APPROVED replay returns `Done` immediately, so a rewritten artifact passes the gate without ever being judged;
`Plan-Revalidate`'s `on_stuck` is currently pinned to `Plan-Write` rather than `Plan-Bouncer` purely to dodge the live-lock this produces, which its yaml comment states outright.

**Why now.**
Both defects are confirmed present in the shipped `Discussion-Validate` → `Discussion-Write` → `Discussion-Bouncer` path and are shared, unchanged, by the newly-landed `Plan-Revalidate` → `Plan-Write` → `Plan-Bouncer` path.
The second segment landing is what turned a single latent defect into a duplicated one, and the third (`Webster-Review`) is wired out of the same two adapters.

## Scope

**In:**

- `internal/shedrecipe/entries_bouncer.go` — resolve `artifact_paths` against `env.AnchorPath` instead of `env.WorktreeRoot`; swap the corresponding `requireAbsRoot` guard.
- `internal/hubgeom/hubgeom.go` — `BurlerGeometry` fills `WorktreeRoot` from `l.AnchorPath()`, with a doc comment explaining the choice, mirroring the shipped `hubgeom.WebsterGeometry` precedent.
- `internal/burlerengine/geometry.go` — doc-only update to `Geometry.WorktreeRoot`'s field comment, recording that hub mode now tells it the anchor path while standalone still tells it the reviewed target directory.
- `internal/shedadapters/bouncer.go` — a durable settled-marker written on an APPROVED settle, and a clear-and-re-seed step at `Call` entry when that marker is found.
- `internal/shedadapters/round.go` (or a sibling in the same package) — the marker's path helper and the run-directory archive helper.
- `contracts/recipes/loom-recipe.yaml` — comment-only edits: delete the two comments that document the wrong-root defect as deferred, and rewrite `Plan-Revalidate`'s `on_stuck` comment so its rationale no longer rests on a defect that has been fixed.
- Tests in `internal/shedadapters`, `internal/shedrecipe`, and `internal/hubgeom`.
- Docs: `manifest/designs/loom.md` ("The gate"), `internal/shedadapters/doc.go` (the round-artifact convention), `manifest/roadmap.md` (Planned → Done).

**Out:**

- `shedrecipe.Env.WorktreeRoot` itself is **not** re-pointed or removed. It stays filled from `location.WorktreePath()` and stays read by `planValidateEntry` and `singleLLMEntry`.
- `internal/planparser`'s `Validate(plan, worktreeRoot)` root, and the `Plan-Validate`/`Plan-Revalidate` rows that feed it. A plan card's `paths:` entries name repo source, not `_lyx` content; whether that root is right is a separate question this task does not open.
- `singleLLMEntry`'s `output_files` root and its `worktree_root` fill token. No loom recipe row uses the `SingleLLM` engine directly (`Discussion-Write`/`Plan-Write` have their own entries), so nothing in the shipped recipe is affected.
- `burlerengine`'s own resolution logic and `standalonegeom.BurlerGeometry`. Standalone deliberately diverges (`WorktreeRoot` = reviewed target dir, `AnchorPath` = derived state dir) and must keep diverging.
- Renaming `burlerengine.Geometry.WorktreeRoot`.
- Re-pointing `Plan-Revalidate`'s `on_stuck` to `Plan-Bouncer`.
- Any new `CONSTRAINTS.md` invariant.
- The stale `round-<N>-focus.json` spelling in `internal/shedadapters/doc.go:90` (the code writes `round-<N>-focus.md`). Pre-existing, unrelated, left alone.

## Decisions

### anchor-fix-lands-at-two-precise-sites

- Decision: fix the root at exactly two production sites — `bouncerEntry`'s `artifact_paths` resolution (→ `env.AnchorPath`) and `hubgeom.BurlerGeometry`'s `WorktreeRoot` fill (→ `l.AnchorPath()`).
- Rationale: `_lyx` is a module's own durable-storage subdirectory, which the Cwd Resolution Invariant states is joined onto `AnchorPath()` directly.
  Both sites are the only places a review segment turns a recipe-authored `_lyx/...` string into an absolute path, and `hubgeom.WebsterGeometry` already establishes the precedent of a hub-mode geometry builder filling a `WorktreeRoot` field from `l.AnchorPath()` with a doc comment saying why.
- Rejected: re-pointing `shedrecipe.Env.WorktreeRoot` to `anchorPath` in `loomcli.wire` — one line, but it silently changes `planValidateEntry`'s root too, which is a separate question with a separate answer.
  Adding a new `Env.ArtifactRoot` field — new surface for a value `Env.AnchorPath` already carries.

### all-three-segments-fix-together

- Decision: the fix applies to `Discussion-Review`, `Plan-Review`, and `Webster-Review` alike, because all three are wired out of the same `bouncerEntry`/`burlerRoundEntry` code and the same `hubgeom.BurlerGeometry`.
- Rationale: there is no per-row seam to special-case on, and `Webster-Bouncer`/`Webster-Burler` name `_lyx/plan` in `artifact_paths` and `fasit.paths` exactly as the other two segments name their own `_lyx` content.
  A `Webster` carve-out would preserve the defect in one segment for no stated reason.
- Rejected: scoping the change to the two segments the roadmap item names.

### burlercli-hub-mode-changes-too

- Decision: `lyx burler`'s hub mode changes with it, because it shares `hubgeom.BurlerGeometry` (`internal/burlercli/wiring.go:99`).
- Rationale: `wireHub`'s own comment already states the intent — "Both configs anchor at `loc.AnchorPath()` — the worktree the operator is actually standing in, never `WorktreeRoot` or any fabric sibling."
  The geometry builder was the one line that did not follow it. A hub-mode operator standing at the anchor expects a profile's relative paths to resolve there.
- Rejected: giving loom its own geometry builder — duplicating a two-field struct to preserve the same defect in a second caller.

### burlerengine-geometry-field-keeps-its-name

- Decision: keep `burlerengine.Geometry.WorktreeRoot` as a field; update only its doc comment.
- Rationale: the field is genuinely distinct from `AnchorPath` in standalone mode, where `standalonegeom.BurlerGeometry` fills `WorktreeRoot` with the reviewed target directory and `AnchorPath` with the derived state directory — collapsing them would push a hidden `.lyx` tree into the reviewed folder.
  What changes is only what hub mode *tells* it.
- Rejected: renaming to `ProfileRoot` — clearer, but a cross-package rename well past this task's brief.
  Dropping the field and resolving profile paths against `AnchorPath` — breaks standalone outright.

### clearing-trigger-is-a-durable-settled-marker

- Decision: on an APPROVED settle, after `Commit` (when configured) returns successfully and before returning `Done`, the Bouncer writes a durable marker file into its run directory.
  At `Call` entry — before `ResolveRound` — a Bouncer that finds the marker archives the run directory, recreates it empty, and proceeds, which makes the very next `ResolveRound` return 0 and the call a seed call.
- Rationale: the marker fires exactly on segment-exit-followed-by-re-entry.
  It does not fire on in-segment bouncing (`Bouncer` ↔ `Burler` never settles APPROVED mid-loop), it does not fire on a mid-segment resume, and it does not fire when the bounce budget is exhausted and a human resumes a BLOCKING segment — all three of which must continue the existing round sequence, not restart it.
  Durability is what makes it work across a process boundary: the `Plan-Revalidate` → `Plan-Write` → `Plan-Bouncer` bounce can be interrupted and resumed in a fresh process.
- Rejected: an in-memory `settled` flag on the `Bouncer` value — free, but lost on resume, so it fixes only the same-process half of the bug.
  A digest-keyed marker recording the artifact content at approval and clearing only when the digest differs — more precise, and it would avoid one redundant review round in the narrow crash window between `Commit` returning and `shedengine`'s `persist`, but it adds a content-hashing helper (with a directory walk for `_lyx/plan`) plus its own failure modes for an unreadable or absent path.
  The accepted cost is that a crash in that window re-reviews an already-approved artifact from round 1;
  `shedengine`'s own `run.go` documents at-least-once producer calls as the accepted semantics for exactly this window.

### clearing-archives-by-renaming-the-run-directory-aside

- Decision: clearing is `os.Rename(runDir, <runDir>-<UTC-stamp>)` followed by `os.MkdirAll(runDir, 0o755)`, with the archive target chosen through the package's existing `firstFreeArchivePath` same-second collision helper and the existing `archiveTimestampFormat` stamp.
- Rationale: one rename moves every round artifact, every focus file, every prior archive sibling, and the marker itself, atomically, with no per-entry failure mode.
  Archive rather than delete matches the package's established archive-never-refuse posture (`archiveStaleOutputs`, `ensureFocus`, `seedCall`) — the archived generation is the only record of why a round was re-run.
  The archived directory lands beside its live sibling under `Env.RunRoot`, which holds one directory per segment (`discussion`, `plan`, `webster`), so a `plan-20260826T120000Z` sibling collides with nothing.
  The whole tree is already ephemeral (`LoomReviewsDir` is `.lyx`-anchored), so nothing archived here is ever committed.
- Rejected: moving each `round-*` entry into a nested `<runDir>/archive-<stamp>/` — tidier `RunRoot`, but a per-entry loop with per-entry failure modes and a name-matching predicate to maintain.
  `os.RemoveAll(runDir)` — cheapest, but destroys the audit trail.

### failure-posture-splits-marker-write-from-clear

- Decision: a marker-write failure is logged via `logger.Warn` and swallowed — `settle` still returns `Done`.
  A clear failure (the rename or the recreate) goes through `b.degrade`, yielding `Stuck` with an empty pointer and a nil error.
- Rationale: these mirror the two postures the package already establishes.
  `ensureFocus` swallows its own write failures because "a failure to write the next round's targeting hint must not retract a verdict `Call` has already committed to returning" — the same reasoning applies to a marker written after a commit has already happened.
  `seedCall` already degrades on a stale-focus archive failure, and a run directory that could not be cleared is the same class of problem: the segment cannot proceed correctly, and `Stuck` routes it to the fixer row and ultimately to a human.
- Rejected: hard-erroring the marker write — a cosmetic write failure would fail a run whose artifact is already committed.
  Swallowing the clear failure — the Bouncer would then replay the stale verdict, which is the exact bug.

### plan-revalidate-on-stuck-stays-plan-write

- Decision: `Plan-Revalidate`'s `on_stuck` stays `Plan-Write`. Only its yaml comment changes.
- Rationale: the comment's current rationale ("bouncing back into the segment live-locks, because `judged(n)` is still true") stops being true once this task lands, and a comment that justifies a routing choice with a fixed defect rots.
  The routing itself is still right for an unchanged reason: `Plan-Revalidate` reports mechanical format findings, and the fixer round is rubric-forbidden from re-deriving those, so the row that can actually repair them is the writer.
- Rejected: re-pointing to `Plan-Bouncer` — a behaviour change beyond this task's brief, sending mechanical findings to a round that will not act on them.

### no-new-constraints-invariant

- Decision: record nothing new in `CONSTRAINTS.md`.
- Rationale: the settled-marker lifecycle is one adapter's internal round-artifact contract, whose durable home is already `internal/shedadapters/doc.go`'s "round-artifact convention" section — a section that exists precisely so this contract survives the roadmap entry's deletion.
  The anchor fix is an application of the existing Cwd Resolution Invariant, not a new rule.
- Rejected: adding a segment-run-directory-lifecycle invariant — premature for a contract with one implementation.

## Technical context

**The segment shape.**
A review segment is a `Bouncer` row plus a `BurlerRound` row sharing one `run_subdir`, mutually pointed at each other via `on_stuck` (`shedengine.validate` rejects an `OnStuck` naming a producer in a different `Segment`).
`manifest/designs/loom.md`'s "The gate" section is the design home;
`internal/shedadapters/doc.go` is the code-side contract.

**Path plumbing, end to end.**

- `internal/loomcli/wiring.go` `wire()` fills `shedrecipe.Env`: `AnchorPath: location.AnchorPath()`, `WorktreeRoot: location.WorktreePath()`, `RunRoot: loomengine.LoomReviewsDir(location)`, plus the `CommitDiscussion`/`CommitPlan` closures, which call `fabricengine.CommitAnchoredPaths(..., location, []string{loomengine.DiscussionDirRel()}, ...)` / `planparser.PlanDirRel()` — anchor-anchored, which is the other half of the divergence.
- `internal/shedrecipe/entries_bouncer.go` resolves `run_subdir` under `env.RunRoot` (and `os.MkdirAll`s it — the entry creates the run dir, not the producer) and `artifact_paths` under `env.WorktreeRoot`.
  Both go through `resolveUnderRoot`, which rejects an empty value, rejects an absolute value, and rejects a value escaping the root.
- `internal/shedrecipe/entries_burler.go` deliberately passes `profile.target.paths`/`profile.fasit.paths` through relative and unjoined (documented at `entries_burler.go:207`), because `burlerengine`'s own `(*Profile).validate` resolves and stats them against its told root.
  That told root is `Geometry.WorktreeRoot`, filled by `hubgeom.BurlerGeometry`.
- `internal/loomengine/config.go`'s `LoomReviewsDir` is `AnchorPath`-anchored and lives under `.lyx` — the run tree is ephemeral by construction and is never committed by any seam.

**Bouncer control flow (`internal/shedadapters/bouncer.go`).**
`Call` entry-checks the context, then `ResolveRound(RunDir, ReportName)` scans upward from 1 for `round-<N>-review.md`, returning the highest N present (0 when round 1 is absent).
Four modes follow: `n == 0` and round-1 focus not seeded → `seedCall`; `n == 0` and focus seeded → re-bounce (`Stuck`, spawn nothing);
`judged(n)` → `settle(n, spawned=false)`; otherwise → `judgeCall(n)`.
`judged(n)` requires round N's verdict *and* ledger files to both exist and parse.
`settle` maps APPROVED → call `Commit` if non-nil, return `Done` with the ledger pointer;
BLOCKING → `ensureFocus(n+1)`, return `Stuck` with the ledger pointer, committing nothing.
The clear step belongs at the very top of `Call`, after `entryErr` and before `ResolveRound`, so the whole four-mode branch sees a fresh directory.
Note `ResolveRound` hard-errors if `RunDir` is absent — the recreate is not optional.

**The Burler side of the shared directory.**
`BurlerProducer.Call` `MkdirAll`s the run dir, then `highestCompleteRound` reads the directory and takes the highest N for which both `round-<N>-review.md` and `round-<N>-fixer-report.md` exist, writing round `highest+1`;
`hydrationPaths` feeds every prior complete round's pair back in as context.
Both scan only the run directory's top level, which is why an archived sibling directory (or, had that route been chosen, a nested archive subdirectory) is invisible to them.
This is also why clearing must remove *both* rows' artifacts, not just the Bouncer's — the Burler would otherwise resume at round N+1 with hydration from a dead generation.

**Existing helpers to reuse, not re-implement.**

- `archiveTimestampFormat` (`"20060102T150405Z"`) and `firstFreeArchivePath(candidate func(suffix string) string)` in `internal/shedadapters/archive.go`. `firstFreeArchivePath` is name-agnostic — it takes a candidate-builder closure — so it composes with a directory name as readily as with a file name.
- `b.cfg.Now` is the injectable clock already on `BouncerConfig`, filled explicitly with `time.Now` in `loomcli.wire` specifically so a test can inject a fixed clock for archive naming.
- `b.degrade(ctx, msg, args...)` for the `Stuck`-with-warning posture; `logger.Warn` for the swallow posture.
- `verdictPath`/`ledgerPath`/`focusPath` in `round.go` are the package's single place a round number becomes a path. The marker path helper belongs there too, beside them.

**Naming caution.**
The marker file must not collide with the `round-<N>-*` namespace `parseRoundReviewName` and `ResolveRound` scan, and it must not be mistaken for a round artifact by `highestCompleteRound`'s directory walk.
A `settled.md`-style name at the run-directory root satisfies both.

**Precedent for the geometry doc comment.**
`internal/hubgeom/webstergeom.go`'s doc comment is the model: it states plainly that `WorktreeRoot` is `l.AnchorPath()` and not `l.WorktreePath()`, names the call sites that make it correct, and warns that converging the two would silently change behaviour.
`BurlerGeometry`'s new comment should do the same, and should additionally name `standalonegeom.BurlerGeometry` as the mode where the two fields still diverge.

## Constraints

From `CONSTRAINTS.md`:

- **Cwd Resolution Invariant.** `root` names a git worktree root and `cwd` names a working directory — never swapped.
  A module's own durable-storage subdirectory (`_lyx/plan`, `_lyx/discussion`) is that module's private relative-path constant joined onto `AnchorPath()` directly, never a `lyxcwd` call.
  Geometry is structural, never config-overridable. `internal/loomengine`'s `LoomReviewsDir` doc states no other package may construct that path.
- **Told-Geometry Invariant.** `internal/shedadapters`, `internal/shedrecipe`, `internal/loomrecipe`, and `internal/loomshed` take their absolute paths from their caller and must not gain a direct production import of `internal/lyxcwd`;
  `shedrecipe`, `loomrecipe`, and `loomshed` are machine-enforced by `seam_enforcement_test.go`'s `TestToldGeometryInvariant_AllowlistOnly`.
  `internal/hubgeom` is the adapter that legitimately imports `lyxcwd` — the anchor fix therefore lands there, never inside `burlerengine` or `shedadapters`.
- **Lyxdirs Single-Declarer Invariant.** No production file outside `internal/lyxdirs` may name the `_lyx` or `.lyx` literal in path-construction context. Enforced by `internal/lyxcwd/enforcement_test.go`'s `TestEnforcement_GeometryLiterals`.
  Recipe-yaml `_lyx/...` strings are recipe data, not Go path construction, and are unaffected.
- **Durable-vs-Ephemeral State Invariant.** The run tree is `.lyx`-anchored and never tracked; the archived generation directory inherits that and must stay under the same root.
- **Documentation Lifecycle** and the project's task-completion rule: the module doc, `docs/overview.md` (only if the module table or execution stack changes — it does not here), and `manifest/roadmap.md` move in the same commit.
- **Markdown style.** Semantic line breaks — one sentence per line, no fixed-column hard-wrap — in every `.md` file touched.

Discovered during exploration:

- `shedengine.ProducerDef.Segment` is a validation-only label with no runtime effect, and producers are handed no run history, so a producer cannot ask the engine whether it is being re-entered. The marker has to be the Bouncer's own durable state.
- Producers are constructed once, at `loomrecipe.New`, so `RunDir` is fixed for the whole run. A generation-numbered run directory resolved at construction time cannot work; the clearing has to happen at `Call` time against a fixed path.
- `shedengine.run.go` persists the next producer *after* `Call` returns, and documents that a crash before that persist re-calls the producer. That is the window the accepted at-least-once cost lives in.

## Testing

TDD candidates — write the test first for each of these:

**`internal/shedadapters` (the substance of the task).**

- APPROVED settle writes the marker into the run directory, and still returns `Done` with the round's ledger pointer.
- APPROVED settle with a configured `Commit` writes the marker only after `Commit` succeeds; a `Commit` returning an error leaves no marker and surfaces as `settle`'s own error (not `degrade`).
- A marker-write failure (run directory made unwritable, or an equivalent seam) logs and still returns `Done` — the verdict is not retracted.
- BLOCKING settle writes no marker.
- `Call` on a run directory carrying the marker plus a full generation of artifacts renames the directory aside, recreates it empty, and takes the seed path — asserting the archived sibling exists, carries the old artifacts and the marker, and that the fresh directory is empty.
- Archive naming: with an injected fixed `Now`, the sibling is `<runDir>-<stamp>`; a second clear in the same second lands on the `-1` suffix via `firstFreeArchivePath`.
- Non-triggering cases, each asserting the run directory is byte-for-byte untouched: an in-segment BLOCKING replay, a mid-segment resume with an unjudged round N, and a re-bounce with round-1 focus seeded but no report.
- A rename failure degrades to `Stuck` with an empty pointer and a nil error.
- End-to-end within the package: seed → judge BLOCKING → judge APPROVED → `Done` → re-enter → the next `Call` is a seed call writing `round-1-focus.md` into a fresh directory, with the prior generation preserved beside it.

**`internal/shedrecipe`.**

- `bouncerEntry` resolves `artifact_paths` under `env.AnchorPath`, proven with an `Env` whose `AnchorPath` and `WorktreeRoot` are *different* absolute directories — the assertion is worthless if they coincide.
- `bouncerEntry` errors naming `AnchorPath` when `env.AnchorPath` is blank or relative, and no longer requires `env.WorktreeRoot` (an `Env` with a blank `WorktreeRoot` builds a Bouncer row successfully).
- `resolveUnderRoot`'s existing escape and absolute-path guards still fire, now against the anchor root.

**`internal/hubgeom`.**

- `BurlerGeometry` fills both `WorktreeRoot` and `AnchorPath` from `l.AnchorPath()`, with a table row whose `AnchorRel` is deliberately not `"."` so `WorktreePath()` and `AnchorPath()` diverge. `hubgeom_test.go` already parameterises `AnchorRel` and `webstergeom_test.go` already carries exactly this shape — follow it.

**Regression sweep.**

- `go test ./...`. Expect fallout in `internal/shedrecipe`'s existing `entries_bouncer_test.go` (its fixture `Env` likely fills both roots), `internal/hubgeom`'s existing `BurlerGeometry` assertions, and any `loomrecipe`/`loomshed` fixture asserting a resolved artifact path.
- `internal/loomrecipe`'s `coverage_guard_test.go` and `shape_test.go` pin the recipe's row names and shape; comment-only yaml edits must not disturb them, and the guard is the check that they haven't.
- `internal/burlercli` — confirm its hub-mode tests still pass under the changed geometry, and that no test asserted the old `WorktreePath()` fill.

Not doing: a new LLM-driven or fake-shuttle end-to-end run at the `loomrecipe` level.
The package-level tests above cover every branch, and a recipe-level run would duplicate them at far higher cost.

## Q&A log

- **Q:** Where does the artifact-path anchor fix land? **A:** [auto-pick] `bouncerEntry` resolves `artifact_paths` against `env.AnchorPath`, and `hubgeom.BurlerGeometry` fills `WorktreeRoot` from `l.AnchorPath()`. **Why:** two precise sites covering both halves of the divergence, with `hubgeom.WebsterGeometry` as shipped precedent; re-pointing `Env.WorktreeRoot` wholesale would silently change `PlanValidate`'s root too.
- **Q:** Does the fix extend to `Webster-Bouncer`/`Webster-Burler`? **A:** [auto-pick] Yes, unavoidably. **Why:** same `bouncerEntry` and same `BurlerGeometry`; there is no per-row seam to carve out on, and the Webster rows name `_lyx/plan` just as the others name their own `_lyx` content.
- **Q:** Does `burlercli` hub mode change with it? **A:** [auto-pick] Yes. **Why:** it shares `hubgeom.BurlerGeometry`, and its own `wireHub` comment already declares that configs anchor at `AnchorPath()`, never `WorktreeRoot` — the geometry builder was the line that didn't follow it.
- **Q:** Rename `burlerengine.Geometry.WorktreeRoot`? **A:** [auto-pick] No — keep the field, update its doc. **Why:** standalone genuinely fills it with the reviewed target directory, distinct from `AnchorPath` (the derived state dir); collapsing or renaming reaches well past this task.
- **Q:** What triggers the run-directory clear? **A:** [auto-pick] A durable settled marker written on an APPROVED settle and checked at `Call` entry. **Why:** it fires exactly on segment exit followed by re-entry, and never on in-segment bouncing, a mid-segment resume, or a human-resumed BLOCKING segment; durability is what carries it across a process boundary.
- **Q:** Digest-keyed marker instead, to avoid a redundant round in the crash window? **A:** [auto-pick] No. **Why:** it would add a content-hashing helper with a directory walk and its own failure modes to close a window `shedengine` already documents as accepted at-least-once semantics.
- **Q:** What does "clear" do? **A:** [auto-pick] Rename the run directory aside to a timestamped sibling, then recreate it empty. **Why:** one atomic rename moves every artifact from both rows plus the marker, with no per-entry failure mode, and it preserves the audit trail the package's archive-never-refuse posture expects.
- **Q:** Failure posture? **A:** [auto-pick] Swallow-and-warn on a marker-write failure; `degrade` to `Stuck` on a clear failure. **Why:** mirrors `ensureFocus` (never retract a committed verdict) and `seedCall` (a directory that cannot be prepared is a genuine `Stuck`) respectively.
- **Q:** Re-point `Plan-Revalidate`'s `on_stuck` to `Plan-Bouncer` now the live-lock is gone? **A:** [auto-pick] No — keep `Plan-Write`, rewrite the comment. **Why:** the routing is still right for an unchanged reason (the fixer round is rubric-forbidden from re-deriving mechanical findings), but its stated rationale would otherwise cite a defect that no longer exists.
- **Q:** Test approach? **A:** [auto-pick] Package-level unit tests in `shedadapters`, `shedrecipe`, and `hubgeom`, with the divergent-roots case made explicit. **Why:** every branch is reachable at package level, and an assertion made with `AnchorRel` at its `"."` default would prove nothing.
- **Q:** New `CONSTRAINTS.md` invariant? **A:** [auto-pick] No. **Why:** the marker lifecycle is one adapter's internal round-artifact contract, whose durable home is already `internal/shedadapters/doc.go`; the anchor fix applies an existing invariant rather than adding one.
