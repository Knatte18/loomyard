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

- `internal/shedrecipe/entries_bouncer.go` — resolve `artifact_paths` against `env.AnchorPath` instead of `env.WorktreeRoot`; swap the corresponding `requireAbsRoot` guard; update `bouncerEntry`'s own doc comment, which names the old root at line 17.
- `internal/hubgeom/hubgeom.go` — `BurlerGeometry` fills `WorktreeRoot` from `l.AnchorPath()`, with a doc comment explaining the choice, mirroring the shipped `hubgeom.WebsterGeometry` precedent.
- `internal/burlerengine/geometry.go` — doc-only update to `Geometry.WorktreeRoot`'s field comment, recording that hub mode now tells it the anchor path while standalone still tells it the reviewed target directory.
- `internal/shedadapters/bouncer.go` — a clear-and-re-seed step at `Call` entry, triggered by an already-APPROVED verdict for the resolved round,
  plus the four doc comments in the same file that the change falsifies (see the second inventory below).
- `internal/shedadapters/round.go` (or a sibling in the same package) — the run-directory archive helper.
- `internal/burlercli/cli.go` and `internal/burlercli/wiring.go` — operator-facing text that asserts the superseded root (see the stale-assertion inventory below).
- `contracts/recipes/loom-recipe.yaml` — comment-only edits at the sites the two inventories below name, including `Plan-Revalidate`'s `on_stuck` comment.
- Tests in `internal/shedadapters`, `internal/shedrecipe`, and `internal/hubgeom`.
- Docs: `manifest/designs/loom.md` ("The gate"), `internal/shedadapters/doc.go` (the round-artifact convention), `manifest/roadmap.md` (Planned → Done).

**Out:**

- `shedrecipe.Env.WorktreeRoot` itself is **not** re-pointed or removed. It stays filled from `location.WorktreePath()` and stays read by `planValidateEntry` and `singleLLMEntry`. Its field doc is corrected (see the inventory), but its value is not.
- `internal/planparser`'s `Validate(plan, worktreeRoot)` root, and the `Plan-Validate`/`Plan-Revalidate` rows that feed it. A plan card's `paths:` entries name repo source, not `_lyx` content; whether that root is right is a separate question this task does not open.
- `singleLLMEntry`'s `output_files` root and its `worktree_root` fill token. No loom recipe row uses the `SingleLLM` engine directly (`Discussion-Write`/`Plan-Write` have their own entries), so nothing in the shipped recipe is affected.
- `burlerengine`'s own resolution logic and `standalonegeom.BurlerGeometry`. Standalone deliberately diverges (`WorktreeRoot` = reviewed target dir, `AnchorPath` = derived state dir) and must keep diverging.
- Renaming `burlerengine.Geometry.WorktreeRoot`.
- Re-pointing `Plan-Revalidate`'s `on_stuck` to `Plan-Bouncer`.
- Any new `CONSTRAINTS.md` invariant.
- The stale `round-<N>-focus.json` spelling in `internal/shedadapters/doc.go:90` (the code writes `round-<N>-focus.md`). Pre-existing, unrelated, left alone.

### Stale-assertion inventory — defect 1 (wrong root)

The enumeration method, so the set is reproducible rather than asserted: run `grep -rn "WorktreeRoot" --include=*.go --include=*.yaml internal/ contracts/` excluding `_test.go`, plus `grep -rn "worktree is already the target\|worktree itself is structurally the target" internal/`,
then keep only the hits that make a *claim about which root a review segment resolves against* — a bare `WorktreeRoot:` struct fill or a `shuttleengine.NewRunner(...)` argument asserts nothing and is left alone.
Every surviving hit gets an explicit disposition:

| Site | Claim today | Disposition |
| --- | --- | --- |
| `contracts/recipes/loom-recipe.yaml:118-121` (`Plan-Bouncer`) | `artifact_paths` "resolves against `Env.WorktreeRoot`, which is knowingly not the `AnchorPath()` root `Env.CommitPlan` anchors at … the fix is filed as its own roadmap item rather than made here" | **Delete.** The deferral it records is what this task closes; nothing in it survives the fix. |
| `contracts/recipes/loom-recipe.yaml:201-203` (`Webster-Bouncer`) | "every entry resolves to an absolute path under `Env.WorktreeRoot`" | **Reword**, `Env.WorktreeRoot` → `Env.AnchorPath`. The surrounding rationale (why a directory entry is the least-bad `artifact_paths` value for a diff) is unrelated and must survive. |
| `internal/loomcli/wiring.go:87-91` | "`BurlerGeometry`, not `WebsterGeometry`: `BurlerGeometry` fills `WorktreeRoot` from `location.WorktreePath()`, while `WebsterGeometry` fills its own from `location.AnchorPath()` … the two geometries are not interchangeable here" | **Rewrite.** Both builders now fill the field from the anchor, so the stated reason the two are not interchangeable evaporates. The line stays `hubgeom.BurlerGeometry(location)`; the comment must say why that is still the right builder (it is burler's geometry, carrying burler's `AnchorPath` semantics) rather than citing a divergence that no longer exists. |
| `internal/shedrecipe/recipe.go:37-42` | `AnchorPath` is "read by `Batchifier`, `PlanValidate`, `Webster`, and `SingleLLM`'s `anchor_path` token"; `WorktreeRoot` is "read by `PlanValidate` and the root every worktree-relative Config path resolves against" | **Reword both field docs.** `AnchorPath` gains `Bouncer` as a reader; `WorktreeRoot` loses the "every worktree-relative Config path" universal (only `SingleLLM`'s `output_files` still resolves there) and names its two remaining readers. |
| `internal/shedrecipe/entries_bouncer.go:17` | "resolves `artifact_paths` against `env.WorktreeRoot`" | **Reword** to `env.AnchorPath`. |
| `internal/burlercli/wiring.go:67` | `--target-dir` refusal: "the worktree is already the target" | **Reword** to name the anchor path. Once `AnchorRel` is not `"."`, the worktree root is not what burler reviews. |
| `internal/burlercli/wiring.go:71` | "Both configs anchor at `loc.AnchorPath()` … never `WorktreeRoot` or any fabric sibling" | **Leave.** It was already correct and is now also true of the geometry beside it; optionally extend to say so. |
| `internal/burlercli/cli.go:107` | `Long` text: `--target-dir` is "refused in hub mode, where the worktree itself is structurally the target" | **Reword** to the anchor path. |
| `internal/burlercli/cli.go:124` | `--target-dir` flag usage: "refused in hub mode, where the worktree is already the target" | **Reword** to the anchor path. |

`internal/shedrecipe/entries_singlellm.go` and `internal/shedrecipe/entries_simple.go` survive the grep but keep their assertions unchanged: both genuinely still resolve against `env.WorktreeRoot`, and both are explicitly Out.

### Stale-assertion inventory — defect 2 (run-directory clearing)

Defect 2 falsifies a different class of claim, so it needs its own enumeration method: the grep above finds nothing here.
Read every doc comment and inline comment in `internal/shedadapters/bouncer.go` and `contracts/recipes/loom-recipe.yaml` that asserts something about *the replay path, the four-mode branch, or the Bouncer's episode/budget behaviour*,
and give each an explicit disposition. The set is small and file-local because the replay path is entirely private to `Bouncer.Call`.

| Site | Claim today | Disposition |
| --- | --- | --- |
| `internal/shedadapters/bouncer.go:75-79` (`NewBouncer`, budget rule) | "The seed call's unconditional `Stuck` permanently consumes one unit of that budget because the Bouncer's only `Done` exits the segment and its episode therefore never resets." | **Rewrite.** `shedengine.episodeStuckCount` returns at the first history entry whose `Producer` is this row and whose `Outcome` is `Done`, so once a segment can be re-entered after settling, its episode *does* reset and the second generation starts on a fresh budget. That is the wanted behaviour — a fresh generation is a fresh review — but the stated reason for the one-unit offset must be restated as "within one generation" rather than "never resets". |
| `internal/shedadapters/bouncer.go:145-153` (`Call` doc) | "branch into one of four modes -- seed, re-bounce, judge, or replay"; "`shedengine.Done` and a BLOCKING `shedengine.Stuck` are reachable only through harvest or replay" | **Rewrite.** There is a fifth entry action (clear-and-re-seed) ahead of the branch, and after the change `Done` is reachable through harvest only — the replay path yields `Stuck` (BLOCKING) or the clear, never `Done`. The pointer rule itself is unchanged and must survive the rewrite. |
| `internal/shedadapters/bouncer.go:267-281` (`settle` doc) | a commit failure routed through `degrade` "would bounce a judge-approved artifact into a findings-free fixer round that re-approves and re-attempts the commit every pass until the bounce budget is spent, since `judged(n)` stays true on re-entry" | **Rewrite.** The trailing clause is the defect being removed. The decision it justifies — a `Commit` error is `settle`'s own error, never routed through `degrade` — stays correct and must keep a stated reason; the reason becomes that `degrade` only ever returns `Stuck`, which would silently convert an approval into a rejection. |
| `internal/shedadapters/bouncer.go:308-310` (inline, `settle`'s BLOCKING arm) | "an APPROVED replay is not a warning condition" | **Rewrite or delete.** An APPROVED replay no longer reaches `settle` at all — it is intercepted at `Call` entry and clears. The surviving half (why a BLOCKING replay *is* warned about) stays. |
| `contracts/recipes/loom-recipe.yaml:178-181` (`Plan-Revalidate`) | "`on_stuck` is `Plan-Write`, not `Plan-Bouncer`: bouncing back into the segment live-locks, because `judged(n)` is still true for the already-APPROVED round, so settle returns `Done` immediately and the two rows ping-pong forever." | **Rewrite.** The live-lock is what this task removes, so the stated reason rots even though the routing is unchanged. See the `plan-revalidate-on-stuck-stays-plan-write` Decision for the replacement reason. |

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
  Because that is an observable behaviour change, the CLI's own operator-facing text changes in the same commit: the `--target-dir` refusal message (`wiring.go:67`), the `Long` description (`cli.go:107`), and the flag usage string (`cli.go:124`) all currently tell the operator that "the worktree" is the target, and all three are reworded to name the anchor path.
- Rationale: `wireHub`'s own comment already states the intent — "Both configs anchor at `loc.AnchorPath()` — the worktree the operator is actually standing in, never `WorktreeRoot` or any fabric sibling."
  The geometry builder was the one line that did not follow it. A hub-mode operator standing at the anchor expects a profile's relative paths to resolve there.
  The CLI/Cobra Invariant makes help-text accuracy a review obligation on any observable-behaviour change, so leaving the three strings asserting the old root would ship a help tree that contradicts the code.
- Rejected: giving loom its own geometry builder — duplicating a two-field struct to preserve the same defect in a second caller.
  Changing the behaviour but leaving the strings — the operator's only description of where `--target-dir` is refused, and why, would be wrong.

### burlerengine-geometry-field-keeps-its-name

- Decision: keep `burlerengine.Geometry.WorktreeRoot` as a field; update only its doc comment.
- Rationale: the field is genuinely distinct from `AnchorPath` in standalone mode, where `standalonegeom.BurlerGeometry` fills `WorktreeRoot` with the reviewed target directory and `AnchorPath` with the derived state directory — collapsing them would push a hidden `.lyx` tree into the reviewed folder.
  What changes is only what hub mode *tells* it.
- Rejected: renaming to `ProfileRoot` — clearer, but a cross-package rename well past this task's brief.
  Dropping the field and resolving profile paths against `AnchorPath` — breaks standalone outright.

### clearing-trigger-is-the-approved-verdict-already-on-disk

- Decision: the trigger is state the Bouncer already writes. At `Call` entry, after `ResolveRound` returns `n > 0`, a Bouncer whose round `n` is `judged(n)` **and** whose round-`n` verdict parses as `verdictApproved` archives the run directory, recreates it empty, and falls through with the round re-resolved to 0 — which makes the same call a seed call.
  No new file, no new write, no new failure mode.
  `settle(n, spawned: false)` is thereby reachable only with a BLOCKING verdict; the APPROVED replay path disappears, which is the defect.
  The harvest path is untouched: `judgeCall` calls `settle(n, spawned: true)` within the same `Call` that produced the verdict, so an approval still returns `Done` normally on the call that earns it.
- Rationale: `judged(n)` already requires round `n`'s verdict *and* ledger to exist and parse, and `settle` already parses the verdict to decide `Done` versus `Stuck`.
  An APPROVED verdict sitting on disk at `Call` entry *is* the durable record that a previous `Call` settled the segment — it is written before `Done` is returned, it survives a process boundary, and it is exactly the file the buggy replay reads.
  A separate marker would encode the identical fact a second time, and its fire/non-fire set is identical: it does not fire during in-segment BLOCKING bouncing (the latest verdict is BLOCKING), nor on a mid-segment resume with an unjudged round (`judged(n)` is false), nor on a re-bounce with round-1 focus seeded and no report (`n == 0`), nor on a human-resumed budget-exhausted BLOCKING segment (verdict is BLOCKING).
  It does fire in one case beyond the in-run re-entry the task is named for, and that is intended rather than tolerated: `LoomReviewsDir` is per-worktree and is never cleaned between `lyx loom run` invocations,
  so a later run that reaches the segment from the top — a fresh run in a worktree whose previous run left the segment APPROVED, as opposed to a *resume*, which restarts at the persisted `CurrentProducer` and never re-enters from the top — also clears and re-reviews.
  That is the correct answer for the same reason the in-run case is: a new run means the artifact was written again, and a verdict from a previous run's artifact must not gate this run's.
- Rejected: a durable `settled.md` marker written on an APPROVED settle. Two reasons, either sufficient.
  It duplicates state the verdict file already carries, so it adds a write step and a filename that becomes a durable on-disk contract for no new information.
  And it needs a failure posture that has no good answer: hard-erroring a marker write would retract a verdict whose artifact is already committed, while swallowing it (the posture that mirrors `ensureFocus`) leaves a `Done`-returning segment with no marker — silently restoring the exact replay defect this task exists to fix, with a `logger.Warn` as the only evidence.
  The verdict-based trigger has no such hole because it writes nothing.
  Also rejected: an in-memory `settled` flag on the `Bouncer` value — free, but lost on resume, so it fixes only the same-process half of the bug.
  A digest-keyed marker recording the artifact content at approval and clearing only when the digest differs — more precise, and it would close the crash window described below, but it adds a content-hashing helper (with a directory walk for `_lyx/plan`) plus its own failure modes for an unreadable or absent path.

### the-accepted-crash-window-cost

- Decision: accept that a crash in the window between `settle`'s `Commit` returning and `shedengine`'s `persist` writing the next producer causes the segment to be re-reviewed from round 1, and that the re-review is a *fresh, non-deterministic judgement* which may return BLOCKING on an artifact that was already approved and already committed.
- Rationale: `shedengine`'s `run.go` documents at-least-once producer calls as the accepted semantics for exactly this window, and the window is narrow.
  The cost is stated plainly rather than minimised: today that window replays a settled APPROVED verdict and returns `Done` deterministically, so this is a real behaviour change, not merely a wasted round.
  A BLOCKING outcome there is not a corruption — it routes to the fixer round and, if unresolved, to a human — but it is a strictly worse outcome than today's replay, and it is the price of removing the replay that lets a *rewritten* artifact through unjudged.
  The same cost attaches to every rejected alternative except the digest-keyed marker, which was rejected on machinery grounds.
- Rejected: preserving a replay path for "the same artifact, unchanged" — that is the digest-keyed marker under another name, and it carries the same directory-walk machinery.

### clearing-archives-by-renaming-the-run-directory-aside

- Decision: clearing is `os.Rename(runDir, <runDir>-<UTC-stamp>)` followed by `os.MkdirAll(runDir, 0o755)`, with the archive target chosen through the package's existing `firstFreeArchivePath` same-second collision helper and the existing `archiveTimestampFormat` stamp.
- Rationale: one rename moves every round artifact, every focus file, and every prior archive sibling, atomically, with no per-entry failure mode.
  Archive rather than delete matches the package's established archive-never-refuse posture (`archiveStaleOutputs`, `ensureFocus`, `seedCall`) — the archived generation is the only record of why a round was re-run.
  The archived directory lands beside its live sibling under `Env.RunRoot`, which holds one directory per segment (`discussion`, `plan`, `webster`), so a `plan-20260826T120000Z` sibling collides with nothing.
  The whole tree is already ephemeral (`LoomReviewsDir` is `.lyx`-anchored), so nothing archived here is ever committed.
- Rejected: moving each `round-*` entry into a nested `<runDir>/archive-<stamp>/` — tidier `RunRoot`, but a per-entry loop with per-entry failure modes and a name-matching predicate to maintain.
  `os.RemoveAll(runDir)` — cheapest, but destroys the audit trail.

### clear-failure-degrades-to-stuck

- Decision: a clear failure — either the rename or the recreate — goes through `b.degrade`, yielding `Stuck` with an empty pointer and a nil error.
- Rationale: `seedCall` already degrades on a stale-focus archive failure, and a run directory that could not be cleared is the same class of problem.
  Proceeding instead would replay the stale verdict, which is the bug;
  hard-erroring would fail the whole run for a condition the fixer row and ultimately a human can act on.
  This is the only failure posture the decision needs, because the verdict-based trigger writes nothing.
- Rejected: swallowing the failure and continuing — the Bouncer would then replay the stale verdict.
  Returning a hard error — `Stuck` is the routing this producer already uses for every recoverable failure.

### plan-revalidate-on-stuck-stays-plan-write

- Decision: `Plan-Revalidate`'s `on_stuck` stays `Plan-Write`. Only its yaml comment changes.
- Rationale: the comment's current rationale ("bouncing back into the segment live-locks, because `judged(n)` is still true") stops being true once this task lands, and a comment that justifies a routing choice with a fixed defect rots.
  The routing itself is still right for an unchanged reason: `Plan-Revalidate` reports mechanical format findings, and the fixer round is rubric-forbidden from re-deriving those, so the row that can actually repair them is the writer.
- Rejected: re-pointing to `Plan-Bouncer` — a behaviour change beyond this task's brief, sending mechanical findings to a round that will not act on them.

### no-new-constraints-invariant

- Decision: record nothing new in `CONSTRAINTS.md`.
- Rationale: the segment re-entry rule is one adapter's internal round-artifact contract, whose durable home is already `internal/shedadapters/doc.go`'s "round-artifact convention" section — a section that exists precisely so this contract survives the roadmap entry's deletion.
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
`judged(n)` → `settle(n, spawned: false)`; otherwise → `judgeCall(n)`.
`judged(n)` requires round N's verdict *and* ledger files to both exist and parse.
`settle` maps APPROVED → call `Commit` if non-nil, return `Done` with the ledger pointer;
BLOCKING → `ensureFocus(n+1)`, return `Stuck` with the ledger pointer, committing nothing.

The clear step slots between `ResolveRound` and the four-mode branch, so the branch sees a fresh directory and re-resolves to the seed path.
Note `ResolveRound` hard-errors if `RunDir` is absent — the recreate is not optional.

**Reading the verdict without reading it twice.**
`judged(n)` today reads and parses both files and discards the parsed values, returning only a `bool`;
`settle` then re-reads and re-parses the verdict, and its own comment notes the second read can only fail if the file vanished in between.
The clear trigger needs the parsed verdict at `Call` entry, so the natural shape is to have `judged` (or a sibling built beside it) return the parsed `bouncerVerdict` alongside its boolean, letting `Call` branch on `verdictApproved` directly.
Whether `settle` then takes the already-parsed verdict as a parameter or keeps its defensive re-read is an implementation choice;
keeping the re-read preserves `settle`'s documented "vanished between judged and settle" degrade path and is the smaller change.

**The Burler side of the shared directory.**
`BurlerProducer.Call` `MkdirAll`s the run dir, then `highestCompleteRound` reads the directory and takes the highest N for which both `round-<N>-review.md` and `round-<N>-fixer-report.md` exist, writing round `highest+1`;
`hydrationPaths` feeds every prior complete round's pair back in as context.
Both scan only the run directory's top level, which is why an archived sibling directory (or, had that route been chosen, a nested archive subdirectory) is invisible to them.
This is also why clearing must remove *both* rows' artifacts, not just the Bouncer's — the Burler would otherwise resume at round N+1 with hydration from a dead generation.

**Existing helpers to reuse, not re-implement.**

- `archiveTimestampFormat` (`"20060102T150405Z"`) and `firstFreeArchivePath(candidate func(suffix string) string)` in `internal/shedadapters/archive.go`. `firstFreeArchivePath` is name-agnostic — it takes a candidate-builder closure — so it composes with a directory name as readily as with a file name.
- `b.cfg.Now` is the injectable clock already on `BouncerConfig`, filled explicitly with `time.Now` in `loomcli.wire` specifically so a test can inject a fixed clock for archive naming.
- `b.degrade(ctx, msg, args...)` for the `Stuck`-with-warning posture.
- `verdictPath`/`ledgerPath`/`focusPath` in `round.go` are the package's single place a round number becomes a path. The archive-target helper belongs there or in `archive.go`, not inline at the call site.

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
- **CLI/Cobra Invariant.** Every command carries a `Short`, and the help tree is test-guarded.
  An observable-behaviour change carries a review obligation on the affected `Long`/flag-usage text, which is why the three `burlercli` strings are In rather than left to drift.
- **Lyxdirs Single-Declarer Invariant.** No production file outside `internal/lyxdirs` may name the `_lyx` or `.lyx` literal in path-construction context. Enforced by `internal/lyxcwd/enforcement_test.go`'s `TestEnforcement_GeometryLiterals`.
  Recipe-yaml `_lyx/...` strings are recipe data, not Go path construction, and are unaffected.
- **Durable-vs-Ephemeral State Invariant.** The run tree is `.lyx`-anchored and never tracked; the archived generation directory inherits that and must stay under the same root.
- **Documentation Lifecycle** and the project's task-completion rule: the module doc, `docs/overview.md` (only if the module table or execution stack changes — it does not here), and `manifest/roadmap.md` move in the same commit.
- **Markdown style.** Semantic line breaks — one sentence per line, no fixed-column hard-wrap — in every `.md` file touched.

Discovered during exploration:

- `shedengine.ProducerDef.Segment` is a validation-only label with no runtime effect, and producers are handed no run history, so a producer cannot ask the engine whether it is being re-entered. The trigger has to be the Bouncer's own on-disk state — which the verdict file already is.
- Producers are constructed once, at `loomrecipe.New`, so `RunDir` is fixed for the whole run. A generation-numbered run directory resolved at construction time cannot work; the clearing has to happen at `Call` time against a fixed path.
- `shedengine.run.go` persists the next producer *after* `Call` returns, and documents that a crash before that persist re-calls the producer. That is the window the accepted cost lives in.

## Testing

TDD candidates — write the test first for each of these:

**`internal/shedadapters` (the substance of the task).**

- `Call` on a run directory whose highest round is judged APPROVED renames the directory aside, recreates it empty, and takes the seed path in the same call — asserting the archived sibling exists and carries the old artifacts, the fresh directory contains only what `seedCall` writes, and the outcome is the seed `Stuck` with an empty pointer.
- The full generation is moved, not just the Bouncer's files: the archived sibling carries `round-<N>-review.md` and `round-<N>-fixer-report.md` (the Burler's pair) as well as the verdict, ledger, and focus files.
- Archive naming: with an injected fixed `Now`, the sibling is `<runDir>-<stamp>`; a second clear in the same second lands on the `-1` suffix via `firstFreeArchivePath`.
- Non-triggering cases, each asserting the run directory is byte-for-byte untouched and the existing outcome is unchanged: an in-segment BLOCKING replay (`settle` returns `Stuck` with the ledger pointer and logs the no-new-spawn warning), a mid-segment resume with an unjudged round N (routes to `judgeCall`), a re-bounce with round-1 focus seeded and no report (`n == 0`), and a run directory whose round N has a verdict but no parsable ledger (`judged(n)` false).
- The harvest path is unaffected: a `judgeCall` whose spawn produces an APPROVED verdict and ledger returns `Done` with the round's ledger pointer in that same call, and does **not** clear the directory.
- A rename failure degrades to `Stuck` with an empty pointer and a nil error; a recreate failure does the same.
- The cross-invocation case fires too, and is asserted as intended rather than left implicit: a Bouncer constructed fresh over a run directory left APPROVED by a previous process clears and re-seeds on its first `Call`, exactly as the in-run re-entry does.
  At package level the two are the same test shape — a fresh `Bouncer` value over an already-APPROVED directory — which is precisely why the behaviour is uniform.
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
- `internal/burlercli` — confirm its hub-mode tests still pass under the changed geometry, that no test asserted the old `WorktreePath()` fill, and that the CLI/Cobra help-tree tests accept the three reworded strings.

Not doing: a new LLM-driven or fake-shuttle end-to-end run at the `loomrecipe` level.
The package-level tests above cover every branch, and a recipe-level run would duplicate them at far higher cost.

## Q&A log

- **Q:** Where does the artifact-path anchor fix land? **A:** [auto-pick] `bouncerEntry` resolves `artifact_paths` against `env.AnchorPath`, and `hubgeom.BurlerGeometry` fills `WorktreeRoot` from `l.AnchorPath()`. **Why:** two precise sites covering both halves of the divergence, with `hubgeom.WebsterGeometry` as shipped precedent; re-pointing `Env.WorktreeRoot` wholesale would silently change `PlanValidate`'s root too.
- **Q:** Does the fix extend to `Webster-Bouncer`/`Webster-Burler`? **A:** [auto-pick] Yes, unavoidably. **Why:** same `bouncerEntry` and same `BurlerGeometry`; there is no per-row seam to carve out on, and the Webster rows name `_lyx/plan` just as the others name their own `_lyx` content.
- **Q:** Does `burlercli` hub mode change with it? **A:** [auto-pick] Yes, and its operator-facing text changes with it. **Why:** it shares `hubgeom.BurlerGeometry`, and its own `wireHub` comment already declares that configs anchor at `AnchorPath()`; leaving the `--target-dir` refusal message and the two help strings asserting "the worktree is the target" would ship a help tree contradicting the code, which the CLI/Cobra Invariant does not permit.
- **Q:** Rename `burlerengine.Geometry.WorktreeRoot`? **A:** [auto-pick] No — keep the field, update its doc. **Why:** standalone genuinely fills it with the reviewed target directory, distinct from `AnchorPath` (the derived state dir); collapsing or renaming reaches well past this task.
- **Q:** What triggers the run-directory clear? **A:** [auto-pick] The already-APPROVED verdict for the resolved round, read at `Call` entry. **Why:** `judged(n)` plus a parsed `verdictApproved` is durable, already-written evidence that a previous `Call` settled the segment, with the identical fire/non-fire set a marker would have and no write step to fail.
- **Q:** Why not a durable `settled.md` marker written on an APPROVED settle? **A:** [auto-pick] Rejected. **Why:** it duplicates state the verdict file already carries, and its failure posture has no good answer — hard-erroring retracts a verdict whose artifact is already committed, while swallowing the write failure leaves a `Done`-returning segment unmarked and silently restores the replay defect this task exists to fix.
- **Q:** Digest-keyed marker instead, to avoid re-reviewing in the crash window? **A:** [auto-pick] No. **Why:** it would add a content-hashing helper with a directory walk and its own failure modes to close a window `shedengine` already documents as accepted at-least-once semantics.
- **Q:** What exactly is that accepted cost? **A:** [auto-pick] A fresh, non-deterministic re-review from round 1, which may return BLOCKING on an already-approved, already-committed artifact. **Why:** stating it as "a wasted round" would understate it — today the same window replays a settled APPROVED verdict and returns `Done` deterministically, so this is a real behaviour change, not just a cost.
- **Q:** What does "clear" do? **A:** [auto-pick] Rename the run directory aside to a timestamped sibling, then recreate it empty. **Why:** one atomic rename moves every artifact from both rows, with no per-entry failure mode, and it preserves the audit trail the package's archive-never-refuse posture expects.
- **Q:** Failure posture on a clear failure? **A:** [auto-pick] `degrade` to `Stuck`. **Why:** mirrors `seedCall`'s stale-focus archive failure; proceeding would replay the stale verdict, and hard-erroring would fail a run for a condition the fixer row can act on. The verdict-based trigger writes nothing, so this is the only posture the decision needs.
- **Q:** Re-point `Plan-Revalidate`'s `on_stuck` to `Plan-Bouncer` now the live-lock is gone? **A:** [auto-pick] No — keep `Plan-Write`, rewrite the comment. **Why:** the routing is still right for an unchanged reason (the fixer round is rubric-forbidden from re-deriving mechanical findings), but its stated rationale would otherwise cite a defect that no longer exists.
- **Q:** Which comments become false, and what happens to each? **A:** [auto-pick] Enumerated in two Scope inventories — one per defect — each with a stated enumeration method and a reword-vs-delete-vs-leave disposition per site. **Why:** "the two comments" named no identifiable pair, and two of defect 1's nine sites (`loomcli/wiring.go:87-91`, `shedrecipe/recipe.go:37-42`) were outside Scope entirely.
- **Q:** Does defect 2's fix falsify any comment? **A:** [auto-pick] Yes — five sites, given their own inventory. **Why:** defect 1's grep keys on root claims and finds none of them; the replay path's removal falsifies `NewBouncer`'s budget-rule paragraph (a re-entered segment's episode *does* reset, because `episodeStuckCount` returns at the first `Done`), `Call`'s four-mode doc, `settle`'s commit-failure rationale, the "an APPROVED replay is not a warning condition" inline, and `Plan-Revalidate`'s live-lock comment.
- **Q:** A later `lyx loom run` over a run directory a previous run left APPROVED also fires the clear — intended? **A:** [auto-pick] Yes, intended. **Why:** `LoomReviewsDir` is never cleaned between invocations, and a new run means the artifact was written again, so a previous run's verdict must not gate this one; a *resume* is unaffected, since it restarts at the persisted `CurrentProducer` rather than re-entering from the top.
- **Q:** Test approach? **A:** [auto-pick] Package-level unit tests in `shedadapters`, `shedrecipe`, and `hubgeom`, with the divergent-roots case made explicit. **Why:** every branch is reachable at package level, and an assertion made with `AnchorRel` at its `"."` default would prove nothing.
- **Q:** New `CONSTRAINTS.md` invariant? **A:** [auto-pick] No. **Why:** the segment re-entry rule is one adapter's internal round-artifact contract, whose durable home is already `internal/shedadapters/doc.go`; the anchor fix applies an existing invariant rather than adding one.
