# Conflict Resolution Brief

Your sole job is to resolve git conflict markers in the listed files, stage each resolved file, and report success.
Do NOT commit.
Do NOT run `git merge --continue` — the SKILL does that after receiving `{"status":"success"}`.

## Task intent

These excerpts describe what THIS branch is trying to accomplish.
When the merge introduces a parent-side change that conflicts with this branch's intent, the resolution preserves THIS branch's intent.
In particular: if a file appears under a batch's `Deletes:` list and the merge introduces a modified version of that file from the parent, the resolution is to delete the file (your branch's intent overrides).
Stage the deletion with `git -C /home/knatte/Code/loomyard/wts/loom-discussion-burler-fix-scope rm <file>`.

### From discussion.md

# Discussion: loom: Discussion-Burler Fabric Git Invariant fix

```yaml
task: 'loom: Discussion-Burler Fabric Git Invariant fix'
slug: loom-discussion-burler-fix-scope
status: discussing
parent: loom-webster-review-producer
```

## Problem

The shipped `Discussion-Burler` row in `contracts/recipes/loom-recipe.yaml` carries `fix-scope: source`.
A `source` fixer round is told, in its own composed prompt (`internal/burlerengine/prompt.go`'s `fixScopeRules`), to write into the working tree and to **git-commit each fix individually**.
But that row's fix target is `_lyx/discussion/decision-record.md` and `_lyx/discussion/support-log.md` — weft content reached through the junction, not warp source.
The Fabric Git Invariant reserves every weft commit to Go calling `internal/fabricengine` in-process at a boundary the loop owner controls, and states explicitly that an agent commits its own code to the **warp** repo only, "the weft, never".
The Review Round Invariant scopes the same discipline the other way round: "commit-per-fix on warp source".
So the shipped row instructs an LLM agent to do exactly what both invariants forbid.

Why now: the fix used to require new machinery, and no longer does.
The parent task (`loom-webster-review-producer`, this worktree's parent branch) shipped the `Bouncer` `Commit` closure and the recipe-level `commit_seam` key that resolves it (`internal/shedrecipe/entries_bouncer.go`), plus the `Env.CommitDiscussion` closure that key can resolve to (`internal/loomcli/wiring.go`).
`Plan-Bouncer`/`Plan-Burler` already ship the corrected shape — `fix-scope: overlay` on the Burler, `commit_seam: plan` on the Bouncer — and both rows carry comments naming the Discussion row's `source` value as a known, deliberately-deferred violation.
The correction is now a two-line recipe change; what makes it its own task is that flipping the row changes shipped runtime behaviour and the tests and comments that pin it.

## Scope

**In:**

- `contracts/recipes/loom-recipe.yaml`: `Discussion-Burler`'s `fix-scope: source` → `overlay`; `Discussion-Bouncer` gains `commit_seam: discussion`.
- The four recipe comment sites whose text goes stale the moment that flip lands (`Discussion-Burler`, `Plan-Burler`, `Webster-Burler`, `Webster-Bouncer`), plus a rationale comment on the new `Discussion-Bouncer` key mirroring `Plan-Bouncer`'s.
- `internal/loomrecipe/sequence_test.go`: `commitDiscussionCalls` expectation `1` → `2`.
- `internal/shedadapters/bouncer_commit_test.go`: `TestBouncer_Commit_NilIsNotAnError`'s doc comment, which names `Discussion-Bouncer` as the shipped nil-`Commit` row it pins.
- `internal/loomcli/wiring.go`: `CommitDiscussion`'s doc comment, which now has two callers rather than one.
- A new regression-guard test in `internal/loomrecipe` over the parsed recipe (see Decisions).
- Docs in the same commit: `manifest/designs/loom.md` (the gate section gains the per-segment fix-scope/commit-seam record **and** has its "only the review *profile* … differs per phase" sentence corrected), `manifest/roadmap.md` (this Planned item moves to Shipped).

**Out:**

- `internal/burlerengine` — no engine change at all.
  `fix-scope` is a pure prompt-composition switch (`fixScopeRules`); both values already exist, are validated (`Profile.validate`), and are covered by `prompt_test.go`'s `TestComposePrompt_FixScope`.
  No enforcement code, no new `FixScope` value, no write-surface sandbox.
- `internal/shedrecipe` — `commit_seam` resolution (including the `"discussion"` case and its `requireSeam` guard) already ships and is already covered by `entries_bouncer_test.go`'s `TestBouncerEntry_CommitSeam`.
  No new key, no new registry entry, no runtime guard added here.
- `internal/shedadapters` — `Bouncer`'s `Commit` closure and its commit-on-approved-settle behaviour ship unchanged; only a test doc comment moves.
- `internal/loomcli/wiring.go`'s `CommitDiscussion` **closure body** — it already commits the whole discussion directory idempotently and is already reached by `Discussion-Write`.
  Only its doc comment changes.
- `Webster-Burler`'s `fix-scope: source` — correct and stays.
  Its target is the repo's own warp source files, which is the one agent commit the Fabric Git Invariant permits.
- `Plan-Burler` / `Plan-Bouncer` config values — already correct; only `Plan-Burler`'s comment text changes.
- `contracts/stencils/loom/loom-rubric-discussion-review.md` — carries no git or commit language at all; nothing to change.
- `tool-use: true` on `Discussion-Burler` — stays; the round still reads the repo to judge the discussion against it.
- `CONSTRAINTS.md` — no new invariant.
  This task removes a violation of an existing one and adds a machine check for it; the Fabric Git Invariant's own "Enforced by" text is not currently structured as a per-test list for this class, so no edit is required there.
- Any other recipe row, any `.lyx`/`_lyx` layout change, any change to what `Discussion-Write` or `Discussion-Validate` do.

## Decisions

### flip-the-two-recipe-lines

- Decision: In `contracts/recipes/loom-recipe.yaml`, change `Discussion-Burler`'s `profile.fix-scope` from `source` to `overlay`, and add `commit_seam: discussion` to `Discussion-Bouncer`'s `config` block.
  Nothing else about either row changes — `run_subdir: discussion` stays shared, `artifact_paths` stay, `rubric_stencil` stays, `tool-use: true` stays, both `max_bounces: 5` and the mutual `on_stuck` edges stay.
- Rationale: This makes the Discussion segment structurally identical to the already-correct Plan segment.
  Under `overlay`, `fixScopeRules` tells the fixer its write surface is exactly the target paths plus the two round output files and that it runs **no git at all**, with the loop owner committing at the round boundary.
  The target paths are already exactly the two discussion files, so the overlay write surface is precisely right with no other edit.
  The `commit_seam: discussion` key is then what makes the loop owner actually commit: `bouncerEntry` resolves it to `env.CommitDiscussion` and fills `BouncerConfig.Commit`, which `Bouncer` invokes on an approved settle, and `CommitDiscussion` is a `fabricengine.CommitAnchoredPaths` call — Go, in-process, at a loop-owner-controlled boundary, which is what the invariant requires.
  Without the seam key the flip alone would be a regression: an overlay round runs no git and nothing else in the segment commits, so every approved fix would sit uncommitted in the weft working tree.
- Rejected: Leaving `fix-scope: source` and instead narrowing the prompt's git instructions — the violation is the instruction to commit weft at all, not its wording, and it would leave the two review segments structurally divergent for no reason.
  Also rejected: adding a third `FixScope` value ("overlay-plus-commit" or similar) — the existing two values already express exactly this split, and `commit_seam` is the shipped mechanism for the commit half.

### bouncer-commits-only-on-approval-is-accepted-parity

- Decision: Accept that `Bouncer.Commit` fires only on an approved settle, so if the Discussion segment exhausts its bounce budget and goes stuck, the fixer's overlay writes remain uncommitted in the weft working tree.
  Do not add a commit-on-stuck path.
- Rationale: This is exactly the shipped `Plan-Review` behaviour, and parity with the neighbouring segment is worth more than an ad-hoc divergence.
  A stuck segment is a human-escalation boundary, and the human wants to see the uncommitted diff.
  Nothing is lost: loom's resume model reads files on disk, not git history, so an uncommitted overlay fix survives a crash and a `lyx run` resume identically to a committed one.
- Rejected: Committing on stuck as well — it would make the two segments diverge, and it would commit content that no gate approved.

### add-a-parse-level-regression-guard-in-loomrecipe

- Decision: Add a new test in `internal/loomrecipe` that parses the embedded recipe via `shedbuild.Parse(recipes.LoomRecipe)` and asserts, for every row whose `engine` is `BurlerRound`: if any entry in the row's `config.profile.target.paths` resolves under the lyx overlay directory, then that row's `config.profile.fix-scope` must be `overlay`, **and** the `Bouncer` row sharing its `segment` must carry a `commit_seam` whose value is the overlay subdirectory those target paths live under.
  The seam half is a value check, not a presence check: `_lyx/discussion/…` targets require `commit_seam: discussion`, `_lyx/plan` targets require `commit_seam: plan`.
  Three shapes fail loudly rather than being skipped: an overlay-targeting Burler whose `segment` has no `Bouncer` row at all; one whose partner `Bouncer` carries no `commit_seam`; and one whose target paths do not all share a single overlay subdirectory, since a row straddling two overlay trees has no single correct seam and the guard must not guess one.
- Rationale: This task exists because a `fix-scope: source` row over `_lyx` content shipped and survived review; a comment recording the violation is what kept it from being forgotten, which is a review obligation doing a machine's job.
  The rule is mechanically checkable straight off the parsed recipe with no new production code and no new machinery, and it catches all three variants of the bug — the wrong fix-scope, a correct fix-scope left without a seam to commit through (the silent, uncommitted-forever variant), and a seam pointing at the wrong tree (which commits the other segment's directory and leaves this one's fixes behind).
  Pairing the assertions in one test is deliberate: any one alone is a partial guard.
- Rejected: Enforcing this inside `internal/shedrecipe`'s `burlerEntry` as a construction error — `shedrecipe` is generic `Shed` machinery shared by reference with a future product's producer list, and it has no business knowing that `_lyx` means "weft content the Fabric Git Invariant governs".
  The Shed Recipe Registry Invariant's told-geometry half also forbids that package deriving anything about the overlay tree.
  Also rejected: no guard at all, relying on the recipe comments — that is the status quo that produced this task.
- Rejected: `Webster-Burler` needing an exemption entry — it needs none and must not get one.
  Its `target` block deliberately carries `instructions` and no `paths` (a diff has no file on disk), so the guard's precondition is simply never met for that row, and its correct `fix-scope: source` passes untouched.
  A guard that needed a hand-maintained allowlist would be the wrong guard.

### behaviour-change-is-proved-by-the-existing-sequence-scenario

- Decision: Prove the runtime behaviour change by updating `internal/loomrecipe/sequence_test.go`'s existing assertion from `commitDiscussionCalls != 1` to `!= 2`, with a comment mirroring the one already on the `commitPlanCalls != 2` assertion immediately below it.
  Do not add a separate dedicated test for the seam.
- Rationale: That assertion is already the scenario proof that a `Done` from the Discussion segment reaches the Fabric-commit seam through a real `Shed` run over the whole seventeen-row graph.
  After this change the count is `Discussion-Write`'s own commit plus `Discussion-Bouncer`'s approval commit — the identical two-caller shape the Plan assertion below it already pins, so the two read as one pattern rather than two mechanisms.
  A new standalone test would re-prove what `TestBouncerEntry_CommitSeam` already covers at the unit level.
- Rejected: Adding a focused `loomrecipe` test asserting only the Discussion seam — duplicated coverage, and it would not exercise the full run the way the sequence scenario does.

### rewrite-every-stale-cross-reference-in-the-same-commit

- Decision: Update, in this same commit, every comment whose text asserts something about the Discussion row that stops being true:
  1. `Discussion-Burler`'s own profile block — the flipped `fix-scope` gets a rationale comment naming the Fabric Git Invariant and the `commit_seam` partner, in the shape `Plan-Burler`'s already has.
  2. `Discussion-Bouncer`'s new `commit_seam: discussion` key — a rationale comment mirroring `Plan-Bouncer`'s "required rather than optional here" reasoning.
  3. `Plan-Burler`'s `fix-scope: overlay` comment — currently reads "deliberately NOT the shipped Discussion-Burler row's source … recorded and left to its own roadmap item"; the two rows now match, so it must say so instead.
  4. `Webster-Burler`'s `fix-scope: source` comment — currently reads "matching the shipped Discussion-Burler row rather than Plan-Burler's overlay"; it is now the sole `source` row, and the comment must explain why it alone is legitimate (warp source, the one permitted agent commit) rather than pointing at a row that no longer matches it.
  5. `Webster-Bouncer`'s "No commit_seam key, deliberately unlike Plan-Bouncer" comment — now unlike both other Bouncers.
  Plus two comments outside the recipe: `internal/shedadapters/bouncer_commit_test.go`'s `TestBouncer_Commit_NilIsNotAnError` doc comment, which currently justifies itself as pinning "the shipped Discussion-Bouncer row's behaviour as unchanged, since that row's `BouncerConfig` never sets `Commit`" (the test stays valid and unchanged — `Webster-Bouncer` is now the shipped nil-`Commit` row it should name); and `internal/loomcli/wiring.go`'s `CommitDiscussion` doc comment, which should gain the "this idempotence now covers two callers rather than one" note that `CommitPlan`'s comment already carries for the identical reason.
- Rationale: These comments are load-bearing — each one is a recorded design decision explaining a deliberate divergence, and three of them cite the Discussion row's `source` value as their reason.
  Leaving them would leave the file asserting, in four places, a fact the same file contradicts two lines away, which is worse than no comment.
- Rejected: Updating only the recipe and leaving the two Go comments for a later sweep — a stale test doc comment that names the wrong row is exactly what makes a future reader trust the wrong invariant.

### docs-land-in-the-same-commit

- Decision: Update `manifest/designs/loom.md`'s "The gate" section with a short, explicit record of which review segment fixes what and commits how — Discussion and Plan fix overlay content and commit through the loop owner's `commit_seam`, Webster fixes warp source and commits per fix itself — citing the Fabric Git Invariant as the reason the split exists.
  That section also carries a sentence that must be **rewritten**, not merely supplemented: "only the review *profile* (rubric + fasit) differs per phase" (`manifest/designs/loom.md`, in the black-box paragraph of "The gate").
  It is already inaccurate today — fix-scope and `commit_seam` differ per phase too — and the flip makes the divergence sharper, so the sentence must be corrected to name fix-scope and the commit seam as the other per-phase axes rather than having the new record appended beside a false claim.
  Move this item in `manifest/roadmap.md` from Planned to Shipped.
- Rationale: The project's task-completion rule requires the module doc to move with a change to observable behaviour, and this changes what the Discussion review segment does at runtime.
  "The gate" is the section that describes the review-segment black box, and it does not merely omit the commit split — it currently asserts the profile is the *only* per-phase difference, which is precisely the claim a future row author would rely on and precisely the gap that let this bug ship.
  The roadmap moves because this completes a Planned item (not a bugfix or polish pass, despite its size).
- Rejected: Adding the record as a new `CONSTRAINTS.md` invariant — the governing invariant already exists (Fabric Git); a second one restating it per-segment would be a duplicate to keep in step.
- Rejected: Recipe comments alone — they are the right place for the per-row rationale but not for the cross-segment picture, and `loom.md` is where a reader looks for the segment model.

## Technical context

**The recipe.** `contracts/recipes/loom-recipe.yaml` is embedded into the binary by `contracts/recipes/recipes.go` (`//go:embed`, `var LoomRecipe []byte`) — there is no on-disk runtime copy.
`internal/loomrecipe`'s `New` parses it with `shedbuild.Parse` and builds it with `shedbuild.Build` against a caller-supplied `shedrecipe.Env`.
`shedbuild.Recipe`/`shedbuild.Row` (`internal/shedbuild/recipe.go`, `parse.go`) are what a parse-level guard test reads; `Build` is not needed for the guard.

**The rows.** `Discussion-Bouncer` (engine `Bouncer`) and `Discussion-Burler` (engine `BurlerRound`) share `segment: Discussion-Review` and `run_subdir: discussion`, and route to each other by mutual `on_stuck` — `internal/shedengine`'s validator rejects an `OnStuck` naming a producer in a different `Segment`, which is why the shared label exists.
Row names are pinned against `internal/loomshed`'s `Name*` constants by `internal/loomrecipe/coverage_guard_test.go`; this task renames nothing, so that guard is untouched.

**`commit_seam` (already shipped).** `internal/shedrecipe/entries_bouncer.go` reads the optional `commit_seam` string, accepts `""` (nil `Commit`, a legitimate configuration), `"plan"` → `env.CommitPlan`, `"discussion"` → `env.CommitDiscussion`, and errors on anything else.
Each non-empty case is guarded by `requireSeam` on the named `Env` field, so a nil closure is a construction error rather than a silent nil `Commit`.
`commit_seam` is already in `configRejectUnknown`'s accepted-key list.

**`Env.CommitDiscussion` (already shipped and already wired).** `internal/loomcli/wiring.go` fills it with a closure calling `fabricengine.CommitAnchoredPaths(fabricengine.NewMutations(""), location, []string{loomengine.DiscussionDirRel()}, "loom: discussion artifacts for <slug>", fabricengine.EnvSyncOptions())`.
The pathspec is the whole discussion directory deliberately, so `archiveStaleOutputs`' timestamped siblings get committed rather than left as untracked dirt.
It is idempotent: `CommitAnchoredPaths` reports `committed == false` for an already-clean, already-tracked path and the closure discards that result, returning only the error — which is exactly why a second caller (the Bouncer's approval commit, after `Discussion-Write`'s own) is safe.
This is the same property `CommitPlan`'s comment already documents for its own two callers.

**`Bouncer.Commit` (already shipped).** `shedadapters.BouncerConfig.Commit` is invoked on an approved settle only.
`internal/shedadapters/bouncer_commit_test.go` already pins three behaviours: not called on a BLOCKING verdict, a nil `Commit` is not an error and commits nothing, and a failing `Commit` surfaces as an error rather than degrading to `Stuck`.
None of those tests change.

**`fix-scope` is prompt-only.** `internal/burlerengine/prompt.go`'s `fixScopeRules` switches on `Profile.FixScope` and returns one of two prose blocks, interpolated as the `fix_scope_rules` marker into `contracts/stencils/burler/burler-step-3-fix.md`.
`FixScopeSource` yields "the working tree … commit each fix individually … Never push"; `FixScopeOverlay` yields "exactly the target paths plus the two output files … you run no git commands at all; the loop owner commits these files at the round boundary".
There is no runtime write-surface enforcement in either case — the switch is the whole mechanism.
`internal/shedrecipe/entries_burler.go` reads the recipe's `fix-scope` string straight into `burlerengine.FixScope`; `Profile.validate` rejects any value other than the two legal ones.

**Tests that encode the current value.** Only one asserts loom's shipped behaviour: `internal/loomrecipe/sequence_test.go`'s `commitDiscussionCalls != 1`, backed by `fixture_test.go`'s `fakeLoomShuttle` counters and the `Env.CommitDiscussion`/`Env.CommitPlan` counting closures in `buildSequenceFixture`.
Every other `fix-scope: source` / `FixScopeSource` occurrence in the tree is either a synthetic test fixture or a warp-source round, and none of them is loom's recipe: `internal/burlerengine`'s own tests (`prompt_test.go`, `profile_test.go`, `engine_test.go`, `template_test.go`) and `internal/burlercli` (`run.go`'s usage example and `cli_test.go`) exercise the engine's two values directly, `internal/shedrecipe/entries_burler_test.go` carries it in synthetic recipe-config maps, and `manifest/designs/hardener.md` names `fix-scope: source` for the future Tenter round agent — correct there, since that round fixes warp source and commits per fix.
None of those change.
`internal/loomrecipe/shape_test.go` fills both commit closures non-nil, so the new `requireSeam("Bouncer", "CommitDiscussion", …)` path it now reaches is already satisfied there.

**Guard-test mechanics.** `internal/loomrecipe/seam_enforcement_test.go`'s import allowlist binds **production** files only (it skips `*_test.go`), so a new test file may import `contracts/recipes`, `internal/shedbuild`, and `internal/lyxdirs` freely.
Use `lyxdirs.LyxDirName` for the overlay-directory prefix rather than the `_lyx` string literal — the Lyxdirs Single-Declarer Invariant governs production path construction, and matching the existing test convention (`fixture_test.go` already uses `lyxdirs.LyxDirName`) keeps the guard from being the one place the literal is written down.
The guard is a pure in-memory parse with no process spawn, so it stays untagged under the Test Tier Purity Invariant.
Reading `config.profile.target.paths`, `config.profile.fix-scope`, and the partner `Bouncer`'s `config.commit_seam` out of a parsed `Row` means walking `map[string]any` by hand; failing loudly on an unexpected shape is better than silently skipping a row, since a silently-skipped row is a guard that does not guard.
Factor the rule as a function over a parsed `shedbuild.Recipe` so the shipped-recipe assertions and the negative cases both call it: the negative cases feed it small synthetic recipe YAML through the same `shedbuild.Parse`, never a mutated copy of the shipped bytes, which keeps the shipped-recipe half a straight read of the real file.

## Constraints

- **Fabric Git Invariant (`CONSTRAINTS.md`)** — the invariant this task restores compliance with.
  Every git operation LYX's own code performs on the weft goes through `internal/fabricengine`, in Go, in-process, never raw git and never an LLM agent; the weft commit happens at a round/phase boundary the loop owner controls.
  An agent commits to the **warp** repo (commit-per-fix) and to the weft never.
- **Review Round Invariant (`CONSTRAINTS.md`)** — "commit-per-fix on warp source, never push", the same rule stated from the round's side.
  It is what makes `Webster-Burler`'s `fix-scope: source` correct and `Discussion-Burler`'s incorrect.
- **Shed Recipe Registry Invariant (`CONSTRAINTS.md`)** — `internal/shedrecipe` takes every absolute path from its caller and derives nothing; the registry is one central map reached only through `Lookup`/`Names`.
  This is why the new guard belongs in `internal/loomrecipe` and not in `shedrecipe`.
- **Lyxdirs Single-Declarer Invariant (`CONSTRAINTS.md`)** — the `_lyx` literal is not hand-built in path-construction context; use `lyxdirs.LyxDirName`.
- **Test Tier Purity Invariant (`CONSTRAINTS.md`)** — the new test file is untagged and must spawn nothing (no `exec.Command`, no `gitexec`, no fixture-tree copies).
  A recipe-parse guard trivially satisfies this.
- **Documentation Lifecycle + the project's task-completion rule (`CLAUDE.md`)** — a change to observable behaviour updates its module doc in the same commit; `manifest/roadmap.md` moves because this completes a Planned item.
- **Recipe row names are durable identities** — the recipe header comment states that renaming a row without a matching `loomshed.Name*` constant rename breaks resume for in-flight tasks.
  This task renames no row.
- **Markdown: semantic line breaks (`CLAUDE.md`)** — one sentence per line, break at internal independent-clause boundaries, never a fixed-column hard wrap.
  Applies to `loom.md` and `roadmap.md` edits.
- **Producer Pointer-Rule Invariant** — `loom.md`'s gate section is a doc *about* the producers for a human reader, not an instruction file, so recording the commit split there duplicates no format contract.

## Testing

**`internal/loomrecipe` — the new regression guard (TDD candidate).**
Write it first, against the unmodified recipe, and watch it fail on `Discussion-Burler` before the recipe flip lands; that failure is the proof the guard actually detects the bug this task fixes rather than merely passing afterwards.
Scenarios it must cover:

- `Discussion-Burler` (overlay target paths) requires `fix-scope: overlay` — fails before the flip, passes after.
- `Discussion-Bouncer`, as the segment partner of an overlay Burler whose targets live under `_lyx/discussion/`, requires `commit_seam: discussion` — fails before the key is added, passes after.
- `Plan-Burler`/`Plan-Bouncer` pass unchanged, both before and after — proof the guard describes a general rule and is not a one-row assertion in disguise.
- The seam half rejects a wrong *value*, not only a missing key: a `Bouncer` partnered with `_lyx/discussion/` targets but carrying `commit_seam: plan` must fail.
  Cover this with a synthetic recipe rather than by mutating the shipped one, so the shipped-recipe assertions stay a straight read of the real file.
- The three loud-failure shapes each fail rather than being skipped: an overlay-targeting Burler whose `segment` names no `Bouncer` row, one whose partner `Bouncer` omits `commit_seam` entirely, and one whose `target.paths` straddle two overlay subdirectories.
- `Webster-Burler` passes unchanged with `fix-scope: source` and `Webster-Bouncer` passes with no `commit_seam`, purely because the row declares no `target.paths` — proof the exemption is structural rather than an allowlist entry.

**`internal/loomrecipe/sequence_test.go` — the behaviour change.**
The full-graph clean run must record exactly two `CommitDiscussion` invocations (`Discussion-Write`'s own commit, then `Discussion-Bouncer`'s approval commit) and, unchanged, exactly two `CommitPlan` invocations, three `fakeLoomBurler` calls, and three bouncer-judge spawns.
The failure message should name both callers, matching the `commitPlanCalls` message immediately below it.

**Existing coverage that must keep passing untouched** — do not modify any of these; a change here means the fix went wrong somewhere:

- `internal/shedrecipe/entries_bouncer_test.go`'s `TestBouncerEntry_CommitSeam` (all subtests, including the `"discussion"` → `CommitDiscussion` resolution and the nil-`CommitDiscussion` guard).
- `internal/shedadapters/bouncer_commit_test.go`'s three cases — only the doc comment on `TestBouncer_Commit_NilIsNotAnError` changes, never its body or its expectations.
- `internal/burlerengine`'s `TestComposePrompt_FixScope` and `Profile.validate` coverage — no engine behaviour changes.
- `internal/loomrecipe/coverage_guard_test.go`, `shape_test.go`, `resume_test.go`, `revalidate_test.go`, `recipe_test.go`, and `seam_enforcement_test.go`.

**Verify command.** `go test ./... && go test -tags integration ./...` — the same shape as the configured done gate.
The changed packages are `internal/loomrecipe`, `internal/shedadapters`, and `internal/loomcli`, but the recipe is embedded and consumed repo-wide, so the sweep is warranted rather than a scoped run.

**Not needed.** No integration- or smoke-tagged test: the change is a recipe value plus a commit-seam wiring that the untagged fixture already drives end-to-end through a real `Shed` run with fake closures, and driving a genuine `fabricengine` weft commit needs a two-worktree git pair that `internal/loomrecipe`'s own tier deliberately excludes.

## Q&A log

- **Q:** Should this task add a machine check preventing the same class of violation from shipping again, or is the fix just the two recipe lines? **A:** [auto-pick] Add a parse-level guard test in `internal/loomrecipe` over the embedded recipe. **Why:** the violation shipped once and survived review with only a comment recording it, which is a review obligation doing a machine's job; the rule is checkable straight off `shedbuild.Parse` with no production code and it catches the silent uncommitted-forever variant too.
- **Q:** Where should the guard live — `internal/loomrecipe` (test), `internal/shedrecipe` (runtime construction error), or nowhere? **A:** [auto-pick] `internal/loomrecipe`, as a test. **Why:** `shedrecipe` is generic `Shed` machinery shared by reference with future products and, per the Shed Recipe Registry Invariant, derives nothing about geometry; it has no business knowing `_lyx` means weft content the Fabric Git Invariant governs.
- **Q:** How is the runtime behaviour change proved — update the existing sequence assertion, or add a dedicated test? **A:** [auto-pick] Update `sequence_test.go`'s `commitDiscussionCalls` from 1 to 2. **Why:** that assertion is already the full-graph scenario proof that the segment's `Done` reaches the Fabric-commit seam, and the resulting two-caller count mirrors the `commitPlanCalls != 2` assertion beside it, so the two segments read as one pattern.
- **Q:** Which comments go stale, and do they all move in this commit? **A:** [auto-pick] All five — `Discussion-Burler`, the new `Discussion-Bouncer` key, `Plan-Burler`, `Webster-Burler`, `Webster-Bouncer` — plus `bouncer_commit_test.go`'s `TestBouncer_Commit_NilIsNotAnError` doc comment and `wiring.go`'s `CommitDiscussion` doc comment, all in the same commit. **Why:** three of them cite the Discussion row's `source` value as their own reason to exist, and a comment asserting what the same file contradicts two lines away is worse than no comment.
- **Q:** Does `manifest/designs/loom.md` need updating, and does the roadmap item move? **A:** [auto-pick] Yes to both — the gate section records the per-segment fix-scope/commit split, and the item moves Planned → Shipped. **Why:** the project rule requires the module doc to move with observable behaviour change, "The gate" is where the segment model is described and currently omits the commit split, and this completes a Planned item rather than being a bugfix or polish pass.
- **Q:** Should a stuck (bounce-budget-exhausted) Discussion segment also commit the fixer's overlay writes? **A:** [auto-pick] No — accept parity with `Plan-Review`. **Why:** a stuck segment is a human-escalation boundary where the operator wants the uncommitted diff visible, nothing is lost because loom resumes from files on disk rather than git history, and a divergence from the neighbouring segment would need its own justification it does not have.
- **Q:** Does `Discussion-Burler` keep `tool-use: true` under overlay? **A:** [auto-pick] Yes, unchanged. **Why:** `tool-use` governs job A's evidence gathering (reading the repo to judge the discussion against it) and is orthogonal to the fix-scope write surface; `Plan-Burler` is `overlay` + `tool-use: true` for the same reason.
- **Q:** Does `CONSTRAINTS.md` need a new invariant for the commit split? **A:** [auto-pick] No. **Why:** the Fabric Git Invariant and the Review Round Invariant already state both halves of the rule; a per-segment restatement would be a duplicate needing to be kept in step, and this task removes a violation rather than introducing a new cross-cutting rule.


### From _mill/plan/00-overview.md


```yaml
task: 'loom: Discussion-Burler Fabric Git Invariant fix'
slug: 'loom-discussion-burler-fix-scope'
approved: true
started: '20260825-140240'
parent: 'loom-webster-review-producer'
root: ""
verify: null
skip_checks: ["verify-full-suite"]
```

### From _mill/plan/01-fabric-git-invariant-fix.md


```yaml
task: 'loom: Discussion-Burler Fabric Git Invariant fix'
batch: 'fabric-git-invariant-fix'
number: 1
cards: 3
verify: go test ./... && go test -tags integration ./...
depends-on: []
```



- **Edits:**
  - `contracts/recipes/loom-recipe.yaml`
  - `internal/loomrecipe/sequence_test.go`
- **Creates:**
  - `internal/loomrecipe/overlay_seam_guard_test.go`
- **Deletes:** none
- **Edits:**
  - `internal/loomcli/wiring.go`
  - `internal/shedadapters/bouncer_commit_test.go`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `manifest/designs/loom.md`
  - `manifest/roadmap.md`
- **Creates:** none
- **Deletes:** none

## Conflicting files

- `contracts/recipes/loom-recipe.yaml`
- `manifest/roadmap.md`

## Instructions

For each file listed above:

1. Read the file and locate every conflict block (`<<<<<<<`, `=======`, `>>>>>>>`).
2. Understand both sides of the conflict — what each branch intended.
3. Write a resolution that preserves the intent of both sides.
   When both sides modify **different, non-overlapping parts** of the same conflict region — for example, different columns of one table row, different keys of one object, or disjoint lines of a prose block — **combine both edits** into a single resolved structure.
   Do NOT pick one side wholesale just because the region overlaps syntactically;
   picking one side wholesale is correct only when the two changes are genuinely mutually exclusive (e.g. the same key is renamed to two different values).
   Worked example: if `ours` changes column A and `theirs` changes column B of the same table row, the resolution keeps both column changes in a single row — it does not discard either.
4. Before keeping content from either side inside a conflict hunk, search the rest of the file (outside the hunk) for that same content.
   This judgment call is scoped narrowly — it applies only when a hunk's content might be a moved duplicate of content living elsewhere in the file;
   it does NOT apply to every ordinary step-3 disjoint-region combine (e.g. the column-A/column-B worked example above), which remains today's silent, high-confidence success path.
   Two branches:
   - **Confident case:** if the content clearly already exists elsewhere and the surrounding context makes it unambiguous that this is the same item having been moved (not two independent, separately-intended copies) — do not re-add it in the hunk;
     keep only the other side's unrelated edit.
     Worked example: one side moves a roadmap item from `## Planned` to `## Done`, while the other side makes an unrelated edit elsewhere in the file.
     The resolution keeps the item only under `## Done`;
     it is not re-added under `## Planned`.
   - **Ambiguous case:** if you cannot confidently tell whether this is the same moved content or a legitimate independent duplication — fall back to step 3's default (keep both) rather than guessing, and report the ambiguity via the `discarded` field (see Report section) with the description `"kept both sides of a conflict, ambiguous move-vs-duplicate"`.
     Worked example: a similarly-worded item appears in two different sections and you cannot tell whether it is the same item moved or a legitimate second, independently-added item.
     The resolution keeps both occurrences and reports the ambiguity via `discarded`.
5. Run `git -C /home/knatte/Code/loomyard/wts/loom-discussion-burler-fix-scope add <file>` to stage the resolved file.
6. For modify/delete (DU) conflicts: if Task intent above lists this file under a batch's `Deletes:`, run `git -C /home/knatte/Code/loomyard/wts/loom-discussion-burler-fix-scope rm <file>` instead of editing;
   that stages the intentional deletion.
7. For UD conflicts — files this branch **modified** that the parent branch **deleted**: do not silently keep the modification.
   Instead: a. Run `git log --diff-filter=D --oneline MERGE_HEAD -- <file>` to find the deletion commit on the parent. b. Run `git show <deletion-commit>` to inspect context. c. If the deletion commit message mentions a replacement file (e.g. "replaced by", "moved to", "consolidated into"),
   or the commit also adds a file in the same directory with overlapping content: stage the deletion — `git -C /home/knatte/Code/loomyard/wts/loom-discussion-burler-fix-scope rm <file>`. d. If detection is inconclusive: report `{"status":"stuck","stuck_type":"logic","reason":"modify/delete conflict on <file>: cannot determine if parent deletion is a replacement -- operator must decide"}` and halt.
   Do NOT silently keep the modification.
8. Before reporting `{"status":"success"}` (with or without `discarded`), re-read each file listed in Conflicting files in full and explicitly verify no contradictory losing-side claims survive the resolution — e.g. a stale value from one side of the conflict left alongside the correct value from the other side, or a claim that only made sense before the other side's edit was applied.
   If you find a contradiction you missed, fix it before reporting.
   If you find a contradiction you cannot confidently resolve, report `{"status":"stuck","stuck_type":"logic","reason":"self-verification found an unresolved contradiction in <file>: <description>"}` instead of `{"status":"success"}`.

Never use `git checkout --ours` or `git checkout --theirs` — they silently discard one side of the conflict.

## Report

Your last output line MUST be a bare JSON object (no code fence, no backticks):

On success (nothing discarded):

{"status":"success"}

On success with discarded content — if you had to drop content from one side (e.g. two sides made mutually exclusive changes and only one could survive), list each dropped item:

{"status":"success","discarded":["<short description of what was dropped from which side>"]}

An empty or absent `discarded` field means nothing was lost.
If anything was discarded, you MUST list it;
an empty list when content was actually dropped is a protocol violation. `discarded` also carries the step 4 ambiguous-case entry `"kept both sides of a conflict, ambiguous move-vs-duplicate"` — even though nothing was technically dropped in that case, the field's purpose is to surface anything the operator should double-check before `git merge --continue`, which covers both a genuine drop and a kept-both ambiguity.
The `mill-merge-in` frontend reads this field and surfaces any losses (or ambiguities) to the operator before continuing, rather than silently running `git merge --continue`.

If you cannot resolve one or more conflicts:

{"status":"stuck","stuck_type":"logic","reason":"<one-line description of what you could not resolve>"}

Anything other than this JSON object on the last line is a protocol violation;
the merge-in dispatcher treats that as stuck_type: logic with reason "no structured report" — your work is lost.
Do not wrap the JSON in a code fence;
do not add commentary after it.

## Tools

Available: Read, Edit, Write, Bash, Grep, Glob.
Use `git -C /home/knatte/Code/loomyard/wts/loom-discussion-burler-fix-scope` for any git commands;
do not `cd`.
Worktree cwd is `/home/knatte/Code/loomyard/wts/loom-discussion-burler-fix-scope`.
