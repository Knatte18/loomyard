# Discussion: loom: Discussion-Review producer

```yaml
task: 'loom: Discussion-Review producer'
slug: loom-discussion-review-producer
status: discussing
parent: main
```

## Problem

Row 5 of loom's producer list, `Discussion-Review`, is still a `stubProducer` — it reports `Done` unconditionally and reviews nothing.
Everything downstream of it (`Plan-Write` through `Finalize`) therefore runs against a discussion artifact that only `Discussion-Validate`'s two mechanical checks have ever looked at: both files exist, and `decision-record.md` carries its seven required headings.
Nothing judges whether the discussion is *right*.

Why now: every piece of infrastructure this row needs has shipped and is currently unreached.
`internal/shedadapters` has both `Bouncer` and `BurlerProducer`; `internal/shedrecipe` has `bouncerEntry` and `burlerRoundEntry` registered under the `Bouncer` and `BurlerRound` engine names;
`internal/loomrecipe`'s coverage guard carries an explicit allowlist (`coverageGuardAllowedUnreachableEngines`) tolerating exactly `SingleLLM`, `Bouncer`, and `BurlerRound` as unreferenced *until these review-producer tasks land*.
The rubric this row must apply is already written, in `manifest/designs/loom.md`'s two `Discussion-Review rubric` subsections.
This task is the first of the three review segments (`Discussion-Review`, `Plan-Review`, `Webster-Review`), so it also establishes the wiring shape the other two will copy.

## Scope

**In:**

- Replace the single stubbed `Discussion-Review` row in `contracts/recipes/loom-recipe.yaml` with a two-row perch: `Discussion-Bouncer` (engine `Bouncer`) and `Discussion-Burler` (engine `BurlerRound`).
- Retire `loomshed.NameDiscussionReview`; add `loomshed.NameDiscussionBouncer` and `loomshed.NameDiscussionBurler`.
- Re-point `Discussion-Validate`'s `on_done` at `Discussion-Bouncer`.
- Add one new stencil, `loom-rubric-discussion-review`, carrying the review rubric, registered in `contracts/stencils/stencils.go`.
- Extend `internal/shedrecipe`'s `burlerRoundEntry` with a `rubric_stencil` config key (mutually exclusive with the existing literal `rubric` key), resolved through `env.StencilsDir` via `stencilstore.Read` + `stencil.StripLeadingComment`, so both rows of a perch read one rubric.
- Add four run-wide review fields to `shedrecipe.Env` (`ReviewModel`, `ReviewEffort`, `ReviewVersion`, `ReviewTimeout`) that `bouncerEntry` and `burlerRoundEntry` fall back to when their per-row Config keys are absent.
- Add `review:` (model-spec) and `review_timeout_min:` keys to loom's config template, validated at load time via `modelspec.Parse` the same way `discussion:` and `plan:` already are.
- Add a `.lyx`-side scratch accessor in `internal/loomengine` for the review run root, and fill `Env.StencilsDir`, `Env.RunRoot`, `Env.Burler`, and `Env.Now` in `internal/loomcli`'s `wire()`.
- Update `manifest/designs/loom.md` (producer table rows, the two rubric subsections' "future `Bouncer` rubric" wording), and move the roadmap item to Done.
- Tests: `internal/loomrecipe` shape/coverage/sequence guards, `internal/shedrecipe` entry tests, `internal/loomcli` wiring tests, `contracts/stencils` registry test, and a rubric-content test.

**Out:**

- `Plan-Review` and `Webster-Review` — separate roadmap items. Their rubrics do not exist yet and are not written here. This task must leave `Stub` reachable for both.
- `Plan-Sweep` (row 6 of the design table) — not built, not in this task.
- The interactive `Discussion-Write` mode flip (`loom: interactive Discussion-Write`) — separate roadmap item.
- Any change to `Discussion-Validate`'s two mechanical checks, to `internal/discussionparser`, or to the seven required headings.
- Any change to `internal/shedadapters`' `Bouncer` or `BurlerProducer` behaviour. Both are shipped and tested; this task consumes them, it does not modify them.
- Any change to `Discussion-Write`'s own stencil (`contracts/stencils/loom/loom-template-discussion.md`).
- Cluster-fan review (multiple parallel lens reviewers) for this segment.

## Decisions

### two-rows-not-one

- Decision: the stubbed `Discussion-Review` row is replaced by **two** rows, `Discussion-Bouncer` and `Discussion-Burler`. loom's list goes from thirteen rows to fourteen. `loomshed.NameDiscussionReview` is deleted and replaced by `NameDiscussionBouncer` = `"Discussion-Bouncer"` and `NameDiscussionBurler` = `"Discussion-Burler"`.
- Rationale: `manifest/roadmap.md`'s item names both rows explicitly ("a `Discussion-Bouncer`/`Discussion-Burler` segment"), and CLAUDE.md's perch terminology note describes exactly this two-row hand-wiring. A row name is a durable on-disk identity in `current_producer`, so a rename breaks resume for an in-flight task — but the row being renamed is a stub that has never produced anything, so there is no in-flight state to break.
- Rejected: keeping the row named `Discussion-Review` as the Bouncer and adding only `Discussion-Burler`. It preserves one durable name that carries no value (nothing has ever resumed on it) at the cost of a name that no longer says what the row is, and it diverges from the naming the other two review-segment tasks will follow.

### routing-and-segment

- Decision: the recipe's four routing edges are exactly:
  - `Discussion-Validate`: `on_stuck: Discussion-Write` (unchanged), `on_done: Discussion-Bouncer`.
  - `Discussion-Bouncer`: `on_stuck: Discussion-Burler`, `on_done: Plan-Write`.
  - `Discussion-Burler`: `on_stuck: Discussion-Bouncer`, `on_done: Discussion-Bouncer`.

  Both new rows carry `segment: Discussion-Review`.
- Rationale: this is the perch shape verbatim — the Bouncer is the segment's entry and exit, its `OnStuck` names the round producer (used for both the seed call and a rejection), and the round producer always hands control back to the Bouncer. `shedengine`'s validator (`internal/shedengine/validate.go`) rejects an `OnStuck` naming a producer in a different `Segment`, so both rows must carry the same non-empty label for the two mutual edges to build at all. `Discussion-Burler`'s `on_done` is set rather than left empty because `shedengine.ProducerDef.OnDone`'s empty value is load-bearing and ends the whole run silently; `BurlerProducer` documents that it never returns `Done`, so the edge is unreachable, but an unreachable edge that quietly kills a run is a worse failure than a redundant one.
- Rejected: leaving `Discussion-Burler.on_done` empty (relies on a documented never-`Done` to avoid a silent whole-run termination); leaving `segment` empty on both rows (works today only because every other row's segment is also empty — it makes the in-segment `OnStuck` rule accidental rather than expressed).

### bouncer-stuck-goes-to-the-burler-not-to-discussion-write

- Decision: a BLOCKING verdict routes to `Discussion-Burler`, which fixes `_lyx/discussion/` in place. It does **not** route back to `Discussion-Write`.
- Rationale: this is the whole point of the perch — a review round that can fix what it found, rather than discarding the artifact and re-interviewing from scratch. `Discussion-Validate`'s own `on_stuck` keeps pointing at `Discussion-Write`, because a *mechanical* failure (a missing file, a missing heading) is a writing failure, not a judgment one.
- Rejected: keeping `on_stuck: Discussion-Write` on the gate. Re-running an 8-hour interview agent to fix a finding a fixer round could resolve in minutes, and it discards every correct decision the discussion already recorded.

### one-rubric-stencil-read-by-both-rows

- Decision: the rubric lives in exactly one place, a new stencil `loom-rubric-discussion-review` at `contracts/stencils/loom/loom-rubric-discussion-review.md`. `Discussion-Bouncer` names it through the existing `rubric_stencil` config key. `Discussion-Burler` names it through a **new** `rubric_stencil` config key added to `burlerRoundEntry`, mutually exclusive with that entry's existing literal `rubric` key: exactly one of the two must be present, and supplying both or neither is a construction error.
- Rationale: `BouncerConfig.RubricStencil` takes a stencilstore name, while `burlerengine.Profile.Rubric` takes literal prose that `internal/burlerengine/prompt.go` interpolates as `{{rubric}}`. Without this change the same rubric would have to be written twice — once as a stencil and once as a yaml string in the recipe — which the Producer Pointer-Rule Invariant forbids outright. The Burler cannot reach the stencil any other way: `stencilstore` resolves against `<hub>/_board/_lyx/stencils/`, and the Hub Containment Invariant guarantees `_board` is never junctioned into a worktree, so neither `profile.fasit.paths` (worktree-root-relative) nor a tool-use file read can reach it.
- Rejected: literal `rubric:` prose in the recipe row alongside the Bouncer's stencil (duplication the Pointer-Rule bans, and the recipe is embedded in the binary so the two copies drift on separate release cycles); a rubric that points at `manifest/designs/loom.md` by path (loom drives an arbitrary target worktree — that path is loomyard's own repo, not the worktree under review).

### rubric-content-is-a-pointer-target-transcription-not-a-new-rubric

- Decision: the stencil's content is the rubric already written in `manifest/designs/loom.md`'s two `Discussion-Review rubric` subsections — the three do-not-flag items (optional "Notes for the plan writer", rejected alternatives belonging in `support-log.md`, call-site/cross-reference enumeration belonging to `Plan-Sweep`), plus the three also-flag items (relocation and exclusion findings are legitimate; the completeness-before-leanness test; the writer/reviewer symmetry note). Once the stencil ships, those two design-doc subsections stop saying "the text the *future* `Bouncer` rubric must point at" and instead name the shipped stencil as the rubric's home, staying the durable human-readable copy.
- Rationale: the design doc says outright that this text is what the rubric "must **point at** … never copy or paraphrase into the profile itself". A stencil *is* the producer's instruction file, so the stencil is the rubric — the design doc is a doc about it, not a second instruction surface. Keeping both and calling one authoritative is the drift the Pointer-Rule exists to prevent.
- Rejected: writing a fresh rubric from the reviewer's judgment (the roadmap explicitly says the rubric "is already written"); leaving the rubric text solely in `loom.md` and having the stencil reference it by path (unreachable from a target worktree — see the previous decision).

### review-run-artifacts-are-ephemeral

- Decision: `Env.RunRoot` resolves to `<AnchorPath>/.lyx/loom/reviews/`, and both rows carry `run_subdir: discussion`, so every round report, verdict, ledger, focus file, and archive sibling lands in `<AnchorPath>/.lyx/loom/reviews/discussion/`. `internal/loomengine` gains the scratch accessor that constructs it; no other package may build that path.
- Rationale: the Durable-vs-Ephemeral State Invariant requires every never-tracked file to live under `.lyx` at the mirrored subpath of its `_lyx` content — `.lyx/loom/` mirrors the existing `_lyx/loom/` (which holds `status.json`), and it requires each module to expose its own scratch accessor rather than deriving the path inline. `internal/burlerengine` already writes its own per-round rendered instructions to `.lyx/burler` on the same reasoning. Crucially, there is no commit seam for a `Bouncer` row: `loomshed.NewDiscussionWrite`'s commit decorator fires on a `SingleLLMProducer`'s `Done`, and `Env.CommitDiscussion` fires before the review exists. Putting round artifacts under `_lyx/` would leave them as untracked dirt in a tree the following rows and `fabricengine`'s sync both care about.
- Rejected: `_lyx/loom/reviews/` plus a new commit decorator wrapping the Bouncer row. It buys a git-visible audit trail of the review at the cost of a new decorator type, a new registry entry, and a commit fired mid-segment on every judged round; `_lyx` is also fabric-synced to the weft, so it would push per-round review chatter across the fabric. Worth revisiting if the audit trail turns out to matter, but not the default.

### review-model-comes-from-loom-yaml-not-the-recipe

- Decision: loom's config template gains `review: opus[effort=high]` and `review_timeout_min: 240`. `internal/loomengine.LoadConfig` validates the model-spec's grammar via `modelspec.Parse` exactly as it already does for `discussion:` and `plan:`. `shedrecipe.Env` gains four run-wide fields — `ReviewModel`, `ReviewEffort`, `ReviewVersion` (strings) and `ReviewTimeout` (`time.Duration`). `bouncerEntry` and `burlerRoundEntry` keep their existing per-row `model`/`effort`/`version`/`timeout_s` Config keys and fall back to the corresponding `Env.Review*` field whenever a key is absent; the recipe's two new rows omit all four keys and therefore take the `Env` values. `internal/loomcli`'s `wire()` fills the four fields from the resolved loom config.
- Rationale: the recipe is embedded in the binary (`shedbuild.Parse(recipes.LoomRecipe)`, no on-disk runtime location), so a recipe-literal model is untunable without a rebuild — unacceptable for the knob that decides what an expensive multi-round review loop costs. Every other LLM row in loom already resolves its model through loom.yaml plus `modelspec`, with load-time validation; `entries_discussionwrite.go`'s own doc names bypassing that resolution as a reason it does not reuse the generic `SingleLLM` entry. A single review model shared by all three review segments is a genuinely run-wide value, which is what `Env` is contractually allowed to carry ("roots and run-wide values only — never a value that differs between two rows"). Keeping the Config keys as an override preserves the generic entries' existing surface for any non-loom recipe.
- Rejected: recipe-literal `model:`/`effort:` on the two rows (untunable without a rebuild, and it makes loom the one product whose review model is invisible in its config file); per-producer `Env` fields per segment, e.g. `DiscussionReviewModel` (three review segments would mean twelve fields, and a per-row value is precisely what `Env`'s own contract bars).

### profile-shape

- Decision: `Discussion-Burler`'s `profile` in the recipe is:
  - `target.paths`: the two discussion files, worktree-relative (`_lyx/discussion/decision-record.md`, `_lyx/discussion/support-log.md`) — passed through unjoined, per the documented `resolveUnderRoot` exception, and resolved by `burlerengine.Profile.validate` against its own told worktree root.
  - `fasit.instructions`: prose stating that the authority for this round is the rubric supplied in the prompt, and that the mechanical section contract is already enforced upstream by `Discussion-Validate` and is not this round's subject. No `fasit.paths`.
  - `rubric_stencil`: `loom-rubric-discussion-review`.
  - `fix-scope`: `source`.
  - `tool-use`: `true`.
  - `cluster-fan`: omitted.
- Rationale: `Profile.validate` rejects a `Fasit` carrying neither `Paths` nor `Instructions`, and there is no worktree-root-relative file that is the discussion's authority — the rubric is, and it arrives through `{{rubric}}`. Phrasing the fasit as a pointer to the rubric and to the upstream validator, rather than restating the seven headings, keeps it Pointer-Rule-clean. `fix-scope: source` is required because `_lyx/discussion/` is tracked content (`_lyx` holds tracked content only, per the Durable-vs-Ephemeral State Invariant) and the fixer must commit its edits. `tool-use: true` because judging whether a discussion is complete and correct means reading the codebase it describes. `cluster-fan` is omitted: a two-file markdown artifact does not benefit from N parallel lens reviewers, and `Profile.validate` already rejects a `ClusterExclude` set without a fan.
- Rejected: `fix-scope: overlay` (the artifact is tracked, and an overlay fix would never reach git); `fasit.paths` naming `_lyx/discussion/` (the subject and the authority would be the same files, which `Profile.validate`'s own error text calls out as degenerating the round to internal-consistency checking); a named `cluster-fan`.

### round-cap

- Decision: both rows carry `max_bounces: 5`.
- Rationale: `NewBouncer`'s doc states that a Bouncer configured with segment `MaxBounces` of N gets N judged rounds, of which the seed call permanently consumes one — so 5 yields 4 judged rounds. `BurlerProducer`'s doc states the segment's effective cap is the smaller of the two rows' budgets and that raising it means raising both together, so the two values are set equal and explicitly rather than left at 0 to inherit `shedengine`'s internal default of ten.
- Rejected: leaving both at 0 (ten rounds of an LLM review loop over two markdown files is a cost accident waiting to happen, and it hides the two-row relationship the adapters' docs insist on).

### coverage-guard-allowlist-shrinks-by-two

- Decision: `coverageGuardAllowedUnreachableEngines` in `internal/loomrecipe/coverage_guard_test.go` drops `Bouncer` and `BurlerRound`, keeping only `SingleLLM`. `Stub` stays reachable via the still-stubbed `Plan-Review` and `Webster-Review` rows.
- Rationale: the allowlist's own comment says each of the three entries "will consume one of these three engines when it lands" — this task lands two of them, so the allowlist must shrink in the same commit or it stops meaning anything.
- Rejected: leaving the allowlist untouched (it would silently tolerate a regression that unwired either row).

## Technical context

**The producer list and how it is built.**
`contracts/recipes/loom-recipe.yaml` is the authoritative row list, embedded into the binary by `contracts/recipes/recipes.go`.
`internal/loomrecipe.New(env, paths)` parses it with `shedbuild.Parse` (never `shedbuild.Load`, never `shedbuild.Check`) and builds it with `shedbuild.Build(recipe, env)`, which resolves each row's `engine` through `internal/shedrecipe`'s `registry` map and calls the resulting `Constructor(name, cfg, env)`.
`shedbuild.Row` carries `name`, `engine`, `config`, `on_done`, `on_stuck`, `segment`, and `max_bounces`.
Row names are declared as Go constants in `internal/loomshed/loomshed.go` and pinned against the yaml by `internal/loomrecipe`'s coverage guard, which keys its table off those symbols rather than off string literals.

**The two adapters this task consumes, unmodified.**
`shedadapters.Bouncer` (`internal/shedadapters/bouncer.go`) is the review gate: it resolves the round from disk via `ResolveRound(RunDir, ReportName)`, then branches into seed / re-bounce / judge / replay.
It reads the `bouncer-template-seed` and `bouncer-template-judge` stencils plus the caller's `RubricStencil`, strips each rubric's stamp banner with `stencil.StripLeadingComment`, fills the template with `stencil.Fill`, and drives `cfg.Shuttle`.
`NewBouncer` probes `RubricStencil` eagerly via `stencilstore.Read`, so a mistyped stencil name fails at construction.
`BouncerConfig.ReportName` is pinned by `bouncerEntry` to `round-<n>-review.md` and is deliberately not recipe-authorable, because `shedadapters.ResolveRound` stats that exact name.
`shedadapters.BurlerProducer` (`internal/shedadapters/burler.go`) runs one `burlerengine` round, writing `round-<n>-review.md` and `round-<n>-fixer-report.md` under the same `runDir`, and returns `Stuck` on every successful round — never `Done`.

**The two registry entries.**
`internal/shedrecipe/entries_bouncer.go` already accepts `run_subdir` (required), `artifact_paths` (required), `rubric_stencil` (required), and optional `model`/`effort`/`version`; it validates `Env.RunRoot`, `Env.WorktreeRoot`, `Env.StencilsDir`, `Env.Shuttle`, resolves `run_subdir` under `RunRoot` and each `artifact_paths` entry under `WorktreeRoot` via `resolveUnderRoot`, and `os.MkdirAll`s the run dir.
`internal/shedrecipe/entries_burler.go` accepts `run_subdir`, `profile` (required), and optional `model`/`effort`/`timeout_s`; it maps the profile's six kebab-case keys (`target`, `fasit`, `rubric`, `fix-scope`, `tool-use`, `cluster-fan`) onto a `burlerengine.Profile`, validates `Env.RunRoot` and `Env.Burler`, and shares the Bouncer's `run_subdir` value so both write into one directory.
`burlerRoundProfile` currently reads `rubric` as a plain string; adding `rubric_stencil` means adding the key to that function's `configRejectUnknown` list, adding the exclusivity check, and giving `burlerRoundEntry` a `requireAbsRoot("BurlerRound", "StencilsDir", env.StencilsDir)` guard on the stencil path.

**What `wire()` leaves unfilled today.**
`internal/loomcli/wiring.go` ends its `shedrecipe.Env` literal with a comment stating `StencilsDir`, `RunRoot`, `Burler`, and `Now` are deliberately zero "because no row in loom's recipe uses those engines yet".
This task fills all four and rewrites that comment.
`websterGeom.StencilsDir` (`fabricengine.StencilsDir(l.HubPath)`) is already computed in `wire()` and is the value `Env.StencilsDir` takes.
`Env.Burler` needs a `*burlerengine.Engine`, built as `burlerengine.New(runner, hubgeom.BurlerGeometry(location), burlerCfg, stencilsDir)` — `internal/burlercli/wiring.go:101` is the exact precedent, and `runner` (the `*shuttleengine.Runner`) already exists in `wire()`.
`burlerCfg` requires a `burlerengine.LoadConfig(anchorPath, "burler")` call alongside the existing per-module config loads.
Note `hubgeom.BurlerGeometry` uses `l.WorktreePath()` for `WorktreeRoot` while `hubgeom.WebsterGeometry` uses `l.AnchorPath()` — a deliberate divergence documented in both files; use each teller as-is and do not converge them.

**Config.**
`internal/loomengine/template.yaml` holds four keys today (`discussion`, `discussion_timeout_min`, `plan`, `plan_timeout_min`), each with an inline comment.
`internal/loomengine/config.go`'s `LoadConfig` runs `configengine.Load` against `ConfigTemplate()` and then validates the discussion and plan model-specs with `modelspec.Parse`.
The Config Strictness Invariant means an unknown key is an error, so the template and the `Config` struct must gain `review` and `review_timeout_min` together.
Splitting a model-spec into a model/effort/version triple is `modelspec`'s job — follow whatever `loomengine.DiscussionSpec`/`PlanSpec` already do to turn a spec plus a registry into the fields a `shuttleengine.Spec` carries.

**Paths.**
`internal/loomengine/config.go` owns loom's path accessors (`DiscussionDir`, `DiscussionDecisionRecord`, `DiscussionSupportLog`, `LoomStatusFile`, `LoomStatusRel`, …), each built from `lyxdirs.LyxDirName` plus loom's own private segment constants (`discussionDirName`, `loomDirName`).
The new scratch accessor joins `l.AnchorPath()`, `lyxdirs.DotLyxDirName`, `loomDirName`, and a new `reviewsDirName` constant.
`internal/websterengine/state.go`'s `Dir`/`ReportsDir`/`ScratchDir` trio is the shape to copy.

**Gotchas.**

- `stencil.Fill` errors on any marker resolving to empty — the Bouncer's own judge path works around this with the literal `"(none)"`. A rubric stencil that renders empty would break every judge call.
- The rubric stencil's stamp banner (`<!-- lyx-stencil: sha256=… -->`) must be stripped before interpolation; `stencil.StripLeadingComment` is what the Bouncer already uses, and the new Burler `rubric_stencil` path needs the same call.
- `resolveUnderRoot` rejects absolute values and `..` escapes. `profile.target.paths` and `profile.fasit.paths` are the single documented exception and are passed through relative and unjoined.
- `shedengine.Validate` rejects an `OnStuck` naming a producer in a different `Segment`; `shedcheck.Check` never reads `Segment` or `MaxBounces` and is authoring-time only.
- The row-name constants must stay in `internal/loomshed`, not move to `internal/loomrecipe`: `seed.go` and `loompreflight.go` read two of them, and `loomshed` importing `loomrecipe` would close a `shedbuild → shedrecipe → loomshed` cycle.
- `internal/loomrecipe/shape_test.go`'s `wantProducerTable` is a literal fourteen-row (currently thirteen-row) table including each row's expected concrete producer type via `reflect.TypeOf`; both new rows need entries with `*shedadapters.Bouncer` and `*shedadapters.BurlerProducer`.
- Several `internal/loomrecipe` tests build a real `shedrecipe.Env`; once the two rows are live, that fixture Env must carry a usable `StencilsDir` (with the rubric stencil actually seeded, because `NewBouncer` probes it), `RunRoot`, `Burler`, and `Shuttle`, or `New` fails for every test in the package.

## Constraints

From `CONSTRAINTS.md`:

- **Producer Pointer-Rule Invariant** — an instruction file must never duplicate or paraphrase another producer's format-contract content, only point at it. This is the constraint that forces one rubric stencil read by both rows.
- **Stencil Ownership Invariant** — every producer prompt is read at call time from a told absolute stencils directory, never from embedded bytes; `//go:embed` in `contracts/stencils` carries seed defaults only; `internal/stencilstore` is the sole owner of seeding, hash-stamping, edit detection, and reading; `contracts/stencils/registry_test.go` enforces registry completeness, so the new stencil must be added to both the `//go:embed` block and the `entries` slice.
- **Durable-vs-Ephemeral State Invariant** — never-tracked files live under `.lyx` at the mirrored subpath of their `_lyx` content; no engine derives its own `.lyx` path, each module exposes a scratch accessor beside its durable one.
- **Cwd Resolution Invariant** — a module's own durable-storage subdirectory is that module's private relative-path constant joined onto `AnchorPath()` directly, never a `lyxcwd` call; `lyxdirs` is the single declarer of `_lyx`/`.lyx`.
- **Lyxdirs Single-Declarer Invariant** — no hand-built `filepath.Join` naming the `_lyx`/`.lyx` literals in production path construction.
- **Shed Recipe Registry Invariant** — `internal/shedrecipe` may not import `internal/lyxcwd`; anything needing a `*lyxcwd.Location` arrives as an injected closure or a told absolute path in `Env`.
- **Shed Producer-Seam Invariant** — producers implement `shedengine.ShedProducer` and are reached through the seam.
- **Config Strictness Invariant** — an unknown config key is a load error, so `template.yaml` and the `Config` struct move together.
- **Hub Containment Invariant** — `_board` is never junctioned into a worktree, which is why the Burler cannot reach the stencils directory by path.
- **Review Round Invariant** — A-before-B (review fully written before any target file is touched), every recorded finding fixed in B including LOW/NIT, no self-grading (round N's fix is judged by round N+1's fresh review), commit-per-fix on warp source, never push. `burlerengine` already implements this; the rubric must not contradict it.
- **Test Tier Purity Invariant** and **Hermetic Git Test Environment Invariant** — respect whatever tier the touched test files already sit in; `wire()` exists as a separate function precisely so it can be driven tier-1 against a hand-built `*lyxcwd.Location`.
- **Documentation Lifecycle** and CLAUDE.md's task-completion rule — the module doc (`manifest/designs/loom.md`) updates in the same commit; `manifest/roadmap.md` moves the item to Done; `docs/overview.md` needs no change (no module added, execution stack unchanged); `CONSTRAINTS.md` needs no change (no new cross-cutting invariant).
- CLAUDE.md's **markdown rule** — semantic line breaks, one sentence per line, no fixed-column hard-wrap. This binds the new stencil and every doc edit.
- CLAUDE.md's **worktree isolation** rule — all work stays in this worktree; no direct push to `main`.

## Testing

**`contracts/stencils`** — `registry_test.go` already asserts registry completeness (every embedded var registered, every registered name resolvable). Adding `loom-rubric-discussion-review` to the `//go:embed` block and to `entries` should satisfy it with no test edit; confirm rather than assume.

**Rubric content test (TDD candidate)** — a new test asserting the rubric stencil's bytes name every item the design doc requires: the three do-not-flag items (optional "Notes for the plan writer"; rejected alternatives belonging in `support-log.md`; call-site/cross-reference enumeration belonging to `Plan-Sweep`) and the three also-flag items (relocation and exclusion findings are legitimate; the completeness-before-leanness test; the writer/reviewer symmetry note). `internal/burlerengine/template_test.go`'s `TestTemplate_StatesRoundDiscipline` is the precedent for asserting prompt content this way. Assert on distinctive phrases, not whole paragraphs, so ordinary prose edits do not break it.

**`internal/shedrecipe`** — extend `entries_burler_test.go` for the new `rubric_stencil` key: it resolves through a seeded stencils dir; the stamp banner is stripped from the resolved rubric; supplying both `rubric` and `rubric_stencil` is an error; supplying neither is an error; an unregistered stencil name is an error; a missing/relative `Env.StencilsDir` is an error on the `rubric_stencil` path only. Extend `entries_bouncer_test.go` and `entries_burler_test.go` for the `Env.Review*` fallback: a row omitting `model`/`effort`/`version`/`timeout_s` takes the `Env` values; a row setting them overrides the `Env` values; both absent leaves the provider default. `env_test.go` covers any new guard.

**`internal/loomengine`** — `config_test.go` for the two new keys: a valid `review:` spec loads; an ungrammatical one fails at load with a message naming the key; the template and the `Config` struct agree. A path test for the new reviews scratch accessor, following `discussionpath_test.go`'s shape — anchored at `AnchorPath()`, under `.lyx`, mirroring `_lyx/loom/`.

**`internal/loomrecipe`** — the guards that must move together: `coverage_guard_test.go`'s `loomRowEngines` map gains both rows and the allowlist drops `Bouncer`/`BurlerRound`; `shape_test.go`'s `wantProducerTable` gains both rows with their concrete producer types and updated routing, and its row count assertion goes to fourteen; `recipe_test.go`'s structural assertions and `sequence_test.go`'s ordering/routing assertions follow. `resume_test.go` is the one to check for a hardcoded `Discussion-Review` row name. The package's shared `shedrecipe.Env` fixture needs a real seeded stencils dir, a `RunRoot` under `t.TempDir()`, a fake `BurlerRunner`, and a fake `Shuttle` — `internal/shedadapters/burler_test.go`'s `fakeBurlerRunner` is the shape to mirror.

**`internal/loomcli`** — `wiring_test.go` asserts the four newly-filled `Env` fields carry the expected values (`StencilsDir` equals `fabricengine.StencilsDir(hubPath)`, `RunRoot` equals the new loomengine accessor, `Burler` is non-nil, the `Review*` triple matches the loaded loom config), driven against a hand-built `*lyxcwd.Location` so the test stays tier 1. `parity_test.go` and `smoke_test.go` need a check for any hardcoded row name or row count.

**Whole-graph** — the existing `shedcheck`-based routing guard in `shape_test.go` is what proves the new edges form a valid graph from `entry: Preflight` to `terminals: [Finalize]` with no dangling target. Verify it exercises both new rows rather than passing vacuously.

**Verify command** — `go build ./... && go test ./...` at the repo root. A run touching `contracts/stencils`, `internal/shedrecipe`, `internal/loomshed`, `internal/loomrecipe`, `internal/loomengine`, and `internal/loomcli` has no cheaper honest subset.

## Q&A log

- **Q:** Replace the `Discussion-Review` row with two rows, or keep the name on the Bouncer? **A:** [auto-pick] Two new rows, `Discussion-Bouncer` + `Discussion-Burler`, retiring `NameDiscussionReview`. **Why:** the roadmap item names both rows explicitly, and the row being renamed is a stub with no in-flight resume state to break.
- **Q:** What is `Discussion-Burler`'s `on_done`? **A:** [auto-pick] Point it back at `Discussion-Bouncer`. **Why:** `BurlerProducer` never returns `Done`, so the edge is unreachable — but an empty `OnDone` is load-bearing and silently ends the whole run, which is a far worse failure than a redundant edge.
- **Q:** Set a `segment:` label on the two rows? **A:** [auto-pick] Yes, `segment: Discussion-Review` on both. **Why:** `shedengine.Validate` rejects an `OnStuck` naming a producer in a different segment, so the label makes the mutual edges expressed rather than accidentally legal.
- **Q:** Where does a BLOCKING verdict route? **A:** [auto-pick] To `Discussion-Burler`, which fixes in place. **Why:** in-place fixing is the entire point of a perch; re-running the interview agent discards every correct decision already recorded.
- **Q:** What round cap? **A:** [auto-pick] `max_bounces: 5` on both rows, yielding four judged rounds after the seed consumes one. **Why:** the adapters' docs state the effective cap is the smaller of the two budgets and that both must move together; inheriting the default of ten is a cost accident.
- **Q:** How do both rows reach one rubric, given the Bouncer takes a stencil name and the Burler takes literal prose? **A:** [auto-pick] Add a `rubric_stencil` key to `burlerRoundEntry`, mutually exclusive with its literal `rubric` key. **Why:** the alternative writes the rubric twice, which the Producer Pointer-Rule Invariant forbids, and the Burler cannot reach the stencils dir any other way — the Hub Containment Invariant keeps `_board` out of every worktree.
- **Q:** Durable or ephemeral run artifacts? **A:** [auto-pick] Ephemeral, `<AnchorPath>/.lyx/loom/reviews/discussion/`. **Why:** no commit seam exists for a Bouncer row, so `_lyx/` would leave untracked dirt; `.lyx` is the invariant's sanctioned home and `burlerengine` already writes its round scratch there. Flagged as the decision most worth revisiting if a git-visible review audit trail turns out to matter.
- **Q:** `fix-scope`? **A:** [auto-pick] `source`. **Why:** `_lyx/discussion/` is tracked content, and an overlay fix would never reach git.
- **Q:** `tool-use`? **A:** [auto-pick] `true`. **Why:** judging whether a discussion is complete and correct means reading the codebase it describes.
- **Q:** `cluster-fan`? **A:** [auto-pick] Omit it. **Why:** two markdown files do not benefit from N parallel lens reviewers.
- **Q:** Where does the review model/effort/timeout come from — the recipe row or loom.yaml? **A:** [auto-pick] loom.yaml `review:`/`review_timeout_min:`, threaded as run-wide `Env.Review*` fields that the generic entries fall back to when their Config keys are absent. **Why:** the recipe is embedded in the binary, so a recipe-literal model is untunable without a rebuild; every other loom LLM row already resolves through loom.yaml + modelspec, and one review model shared by all three review segments is exactly the run-wide value `Env` is allowed to carry.
- **Q:** Machine-check the rubric's content? **A:** [auto-pick] Yes — assert the stencil names all three do-not-flag items and all three also-flag items. **Why:** the rubric is the deliverable, and `internal/burlerengine/template_test.go` already sets the precedent for asserting prompt content on distinctive phrases.
