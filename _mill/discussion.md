# Discussion: loom: Discussion-Write producer

```yaml
task: 'loom: Discussion-Write producer'
slug: loom-discussion-write-producer
status: discussing
parent: main
```

## Problem

Loom's producer graph (`contracts/recipes/loom-recipe.yaml`) has thirteen rows, and row 3 — `Discussion-Write`, the row that produces `_lyx/discussion/decision-record.md` and `support-log.md` — is still backed by `loomshed.NewStub`, a placeholder that reports `Done` unconditionally without spawning anything.
A real `lyx loom run` therefore walks straight past the phase that is supposed to produce the Plan producer's sole input, and `Discussion-Validate` (row 4, real) immediately bounces back on the missing files until the bounce budget is exhausted.

Two Wave 1 items landed the prerequisites this row was waiting on, so it is now unblocked.
`loom: redesign the Discussion format` produced `manifest/designs/loom-format-discussion.md`, which bounds what `Discussion-Write` may explore and explicitly says its own content folds into the stencil and the doc is deleted when this task lands.
`loom: code-writing skills — comments, build, testing` shipped the `scribe` plugin, whose `plugins/scribe/skills/INDEX.md` names this task, by name, as the one that adds an explicit "Load these skills: …" line to a lyx-generated producer prompt.
Both are waiting on this task and neither has any other consumer.

## Scope

**In:**

- Rewrite `contracts/stencils/loom/loom-template-discussion.md`: fold in `loom-format-discussion.md`'s Fix 1 exploration bound (Step 2 and Step 3), replace Step 3's `Architecture` interview category with a bounded coarse-level one, add a Step 0 that loads `scribe:prose` and `scribe:conversation`, and rewrite the leading HTML comment to describe the real call path.
- Add a `DiscussionWrite` entry to `internal/shedrecipe`'s registry, constructing `shedadapters.NewSingleLLMProducer` over an injected Spec closure.
- Add `DiscussionSpec shedadapters.SpecSource` to `shedrecipe.Env`, filled in `internal/loomcli/wiring.go` from the already-in-scope `location`, `loomCfg`, `registry`, `runner`, and `websterGeom.StencilsDir`; fill the so-far-empty `Env.Shuttle` with `runner`.
- Flip `contracts/recipes/loom-recipe.yaml`'s row 3 from `engine: Stub` to `engine: DiscussionWrite`, routing unchanged.
- Delete `manifest/designs/loom-format-discussion.md` and retarget its five inbound markdown links.
- Update `manifest/designs/loom.md` (row 3 of the producer table), `manifest/roadmap.md` (this item Planned → Done, plus a new Planned item for interactive mode), and one sentence of `CONSTRAINTS.md`.
- Tests at the five sites listed under Testing below.

**Out:**

- **Interactive Discussion-Write.** The producer runs autonomously only. Interactive mode is deferred to a new Planned roadmap item this task writes (see Decisions → `autonomous-only`).
- **`Discussion-Review`, `Plan-Write`, `Plan-Review`, `Webster-Review`** — each is its own roadmap item and each stays a `Stub`. In particular, no `Bouncer`/`Burler` perch is wired here.
- **`planparser`'s Card-format migration** — the sibling Wave 2 item; this task is independent of it and touches no `planparser` or `websterengine` code.
- **`loomengine.PlanSpec` and `Env.PlanSpec`** — Wave 3's `Plan-Write` adds the symmetric field when it needs it, not before.
- **`docs/overview.md`** — untouched. `CLAUDE.md`'s docs rule triggers on module-table or execution-stack changes; no module is added or removed here, only an engine key flipped on one recipe row.
- **`SingleLLMProducer` itself** — no change to `internal/shedadapters`. Its archive-then-respawn behaviour on a stale output file is used as-is.
- **The generic `SingleLLM` registry entry** — left in place and untouched; it stays available for a future recipe row whose Spec really is static recipe config.
- **`internal/discussionparser`** — its mechanical checks are unaffected; this task changes no heading names in either output file.

## Decisions

### autonomous-only

- Decision: `Discussion-Write` runs with `Interactive: false`. The wiring site passes `autonomous: true` to `loomengine.DiscussionSpec` unconditionally. Interactive mode becomes a new Planned roadmap item in the `loom: real LLM producers` group.
- Rationale: interactive interviewing does not survive resume with today's machinery, and making it survive is a separate, well-understood piece of work. `shuttleengine`'s `Wait` classifies any turn end without all output files present as `OutcomeAsking` (`internal/shuttleengine/wait.go`, `pollEventsTick`); `SingleLLMProducer` maps `Asking` to `Stuck` (`internal/shedadapters/singlellm.go`); `Discussion-Write` has no `on_stuck`, so `shedengine` persists `blocked` and exits (`internal/shedengine/run.go`, the `def.OnStuck == ""` branch). That much is loom's documented "stops cleanly at the Discussion input boundary" behaviour, and the agent's pane survives it, because `wait.go` only cleans a pane on `OutcomeDone`. The break is on resume: `shedengine` re-calls the same `current_producer`, and `SingleLLMProducer.Call` archives both freshly-written files (`internal/shedadapters/archive.go`) and spawns a new agent that knows nothing of the interview. `SingleLLMProducer`'s own package doc states it never reattaches to a live shuttle session. A resume-on-output-files pre-check ("both files exist → report `Done` without spawning") would fix that case but breaks the other one: `Discussion-Validate`'s `on_stuck: Discussion-Write` bounce also re-enters this row with both files present, and the pre-check cannot distinguish "present and valid" from "present and just rejected by the validator" — it would report `Done` instantly and ping-pong until the bounce budget is exhausted. Resolving that ambiguity is real design work and does not belong inside an otherwise bounded wiring task.
- Rejected: shipping interactive mode with the pre-check (the ambiguity above); shipping both modes behind a `lyx loom run --auto` flag (no such flag exists today, and the interactive branch inherits the same defect).

### spec-closure-in-env

- Decision: `shedrecipe.Env` gains one field, `DiscussionSpec shedadapters.SpecSource`. `internal/loomcli/wiring.go` fills it with a closure over `loomengine.DiscussionSpec`. The new `discussionWriteEntry` validates `Env.DiscussionSpec` and `Env.Shuttle` non-nil and returns `shedadapters.NewSingleLLMProducer(name, env.DiscussionSpec, env.Shuttle, env.Now)`.
- Rationale: `loomengine.DiscussionSpec` takes a `*lyxcwd.Location` as its first parameter, and the Shed Recipe Registry Invariant bars `internal/shedrecipe` from a direct `internal/lyxcwd` production import — passing that type through `Env` would breach it. `Env` already solves exactly this shape twice with whole-value passthrough fields, `WebsterDeps websterengine.RunDeps` and `Landing landingshed.Deps`, and it already carries per-producer named fields (`DecisionRecordPath`, `SupportLogPath`) rather than a generic bag. `wire()` is where `location`, `loomCfg`, `registry`, `runner`, and `websterGeom.StencilsDir` are all already in scope, so the closure costs no new plumbing. Evaluating the closure per `Call` (which is what `SpecSource` means) also keeps the stencil read at call time, as the Stencil Ownership Invariant requires.
- Rejected: refactoring `DiscussionSpec` to take told strings and having the entry call it directly (pulls `loomengine` and `modelspec` into `shedrecipe` and churns a tested signature); a generic `Env.SpecSources map[string]SpecSource` keyed by row name (speculative generality for one call site — Wave 3 adds `Env.PlanSpec` symmetrically when it needs it); reusing the generic `SingleLLM` entry (its `Config.tokens` are static recipe strings, but `{{.slug}}` and `{{.mode_rules}}` are per-run, and its `model`/`effort` config would bypass the `discussion` role's model-spec resolution and `discussion_timeout_min` entirely).

### keep-mode-rules-both-branches

- Decision: `loomengine.DiscussionSpec` keeps its `autonomous bool` parameter, and `prompt.go`'s `modeRules` keeps both its autonomous and interactive branches, along with their existing tests. Only the wiring site pins the value.
- Rationale: the interactive follow-up item then flips one argument instead of re-authoring prose that is already written and tested. The unreached branch is a few lines of string data, not logic.
- Rejected: deleting the parameter and the interactive branch — cleaner today, but throws away exactly what the committed follow-up item needs back.

### no-on-stuck

- Decision: `Discussion-Write` keeps no `on_stuck` in the recipe. A `Stuck` outcome escalates to a human.
- Rationale: under autonomous mode, `Stuck` means the agent ended a turn without writing both files — it gave up, or died mid-write. A self-bounce would archive the partial work and respawn a fresh agent carrying no information about why the previous one stopped, which is blind guessing that burns the bounce budget. `Discussion-Validate` already owns the one legitimate bounce path into this row.
- Rejected: `on_stuck: Discussion-Write` for a budgeted self-retry.

### skills-load-at-step-0

- Decision: the stencil gains a Step 0, ahead of the board read, instructing the agent to load `scribe:prose` then `scribe:conversation`. Those two only — not `code-quality`, `testing`, or the `golang-*` skills.
- Rationale: skills that govern how the agent writes must load before it writes anything, and `scribe:conversation` builds on `scribe:prose`, so load order is stated explicitly. `Discussion-Write` produces two markdown files and conducts an interview; it edits no code, so the code-facing skills have nothing to bite on. This is the wiring `plugins/scribe/skills/INDEX.md` describes as "a separate, later roadmap item (`loom: Discussion-Write producer`) — not yet built".
- Rejected: folding the line into Step 5 (later than the interview text it also governs); loading all seven non-`handoff` skills (irrelevant instructions in a prompt that is trying to stay narrow).

### keep-askuserquestion-prohibition

- Decision: the stencil keeps its trailing "Never use `AskUserQuestion`" section.
- Rationale: it costs three lines, survives a shuttle config flipping `claude_deny_ask_user_question` off, and stays correct when the interactive follow-up lands — in interactive mode `claudeengine` does not install the deny at all (see `shuttleengine.Spec.Interactive`'s own field doc).
- Rejected: deleting it as redundant with the `PreToolUse` deny.

### timeout-comment-only

- Decision: `discussion_timeout_min` stays at `480` in `internal/loomengine/template.yaml`; only its trailing comment changes, since it currently reads "interactive interviews run long".
- Rationale: the value is a deadline, not a wait, so a generous ceiling costs nothing, and an autonomous agent that explores a codebase and writes two files can legitimately run long. Lowering it would in any case only reach newly-seeded configs — an existing `loom.yaml` keeps whatever it already has.
- Rejected: lowering to `120` to match `plan_timeout_min`.

### delete-format-doc-per-its-own-lifecycle

- Decision: `manifest/designs/loom-format-discussion.md` is deleted, its Fix 1 content folded into the stencil, and all five inbound links retargeted.
- Rationale: the doc's own Lifecycle section instructs exactly this and names this task as the deleter. Fix 2's content survives the deletion because `manifest/designs/loom.md` already holds the durable copy.
- Rejected: keeping the doc alongside the rewritten stencil, which would leave a stale draft duplicating it.

### constraints-count-sentence

- Decision: the only `CONSTRAINTS.md` edit is the Shed Recipe Registry Invariant's sentence naming `TestRegistry_ShipsTwelveEntries` and "the registry's exact twelve names". No new invariant is recorded.
- Rationale: nothing cross-cutting is introduced — the existing registry and told-geometry invariants already cover the new entry, and the count is a machine-checked fact that must not go stale.
- Rejected: adding an invariant about per-producer `Env` Spec-closure fields.

## Technical context

**The stub and the row.**
`internal/loomshed/stub.go` backs five of loom's thirteen rows; its doc comment names all five, so removing `Discussion-Write` from that list is part of this task.
`NewStub` itself stays — four rows still use it.
`contracts/recipes/loom-recipe.yaml` is embedded via `contracts/recipes/recipes.go` and parsed by `internal/loomrecipe.New` through `shedbuild.Parse` + `shedbuild.Build`; there is no on-disk runtime copy.

**What already exists and is currently dead.**
`internal/loomengine/discussion.go`'s `DiscussionSpec` and `internal/loomengine/prompt.go`'s `composePrompt` are complete and unit-tested but have no non-test caller.
`DiscussionSpec` resolves the `discussion` role's model-spec through `modelspec.Parse` + `Registry.Resolve`, names both output paths via `loomengine.DiscussionDecisionRecord` / `DiscussionSupportLog`, composes the prompt, and returns a `shuttleengine.Spec` with `Timeout` from `cfg.DiscussionTimeoutMin`.
It errors on an empty slug.
This task's job is to call it, not to rewrite it.

**The registry.**
`internal/shedrecipe/registry.go` holds one central `map[string]Constructor` literal reached only through `Lookup` and `Names` — no `init()` self-registration, no runtime `Register`.
`internal/shedrecipe/entries_simple.go` is the home for entries taking an empty `Config`; `discussionWriteEntry` belongs there or in its own file, and takes an empty `Config` (everything it needs is on `Env`).
`configRejectUnknown(cfg)` with no allowed keys is how an empty-`Config` entry rejects stray recipe config.
`requireSeam` is the nil-check helper, and it detects a typed-nil interface, which matters for `Env.DiscussionSpec` since a `SpecSource` is a func type.

**The wiring site.**
`internal/loomcli/wiring.go`'s `wire()` builds everything: `loomCfg` (`loomengine.LoadConfig`), `registry` (`modelspec.LoadRegistry`), `runner` (`shuttleengine.NewRunner`), and `websterGeom` (`hubgeom.WebsterGeometry(location)`, whose `StencilsDir` field is what `internal/loomcli/landingdeps.go` already uses).
The slug comes from `seedSlug(c.location.WorktreeName)` — `seedSlug` lives in `internal/loomcli/seedinput.go` and `internal/loomcli/run.go` already calls it that way.
The `c.env` literal carries a comment reading "StencilsDir, RunRoot, Shuttle, Burler, and Now are left zero — only SingleLLM, Bouncer, and BurlerRound read them, and no row in loom's recipe uses those engines yet"; that comment is now wrong about `Shuttle` and must be corrected.
`Env.StencilsDir` stays unfilled, because the closure captures the stencils directory directly.
`Env.Now` stays nil, which `NewSingleLLMProducer` defaults to `time.Now`.

**Outcome mapping, for anyone reasoning about behaviour.**
`SingleLLMProducer.Call` archives stale outputs, runs the shuttle seam once, and maps `OutcomeDone` → `Done` with an `OutputPointer` naming `spec.OutputFiles[0]`; `OutcomeAsking` → `Stuck`; `OutcomeDied`/`OutcomeTimeout` → error.
It rejects a non-absolute `OutputFiles` entry outright — `DiscussionSpec`'s paths are `AnchorPath`-anchored and therefore absolute.

**The five inbound links to the doc being deleted** (Markdown Link Integrity is machine-enforced by `internal/lyxcwd/docslink_test.go`):
`manifest/designs/loom.md:35` (producer-table row 3) and `:115` (the relocation-rubric subsection's companion-doc pointer), `manifest/roadmap.md:14` (the card-format group intro) and `:168` (this item's own `## Done` entry for `loom: redesign the Discussion format`), and `manifest/designs/plan-card-format.md:3` (the status blockquote).
The deleted doc's Lifecycle section states the natural retarget for each is the stencil, and that the `## Done` entry may instead keep a historical, non-link reference — that call is this task's.

**The stencil's four markers.**
`stencil.Fill` requires all four of `{{.slug}}`, `{{.mode_rules}}`, `{{.decision_record_path}}`, `{{.support_log_path}}` to be non-empty, and the file must contain no `{{if}}`/`{{range}}` conditionals — a required marker inside a conditional branch renders silently blank.
The rewrite must keep all four markers and add none.

## Constraints

From `CONSTRAINTS.md`:

- **Shed Recipe Registry Invariant** — every registry value constructs a `shedengine.ShedProducer` and nothing else; the registry stays one central map literal reached only through `Lookup`/`Names`; `internal/shedrecipe` takes every absolute path from its caller and has no direct production import of `internal/lyxcwd`. This is what forces the `SpecSource`-closure design over passing a `*lyxcwd.Location`. Enforced by `internal/shedrecipe/seam_enforcement_test.go`, `internal/loomrecipe/coverage_guard_test.go`, and `internal/shedrecipe/registry_test.go`.
- **Stencil Ownership Invariant** — the prompt is read at call time from a told absolute stencils directory, never from embedded bytes; `//go:embed` in `contracts/stencils` is seed-only. The per-`Call` closure preserves this. The stencil is already registered in `contracts/stencils/stencils.go`, so editing its content needs no registry change.
- **Producer Pointer-Rule Invariant** — an instruction file must never duplicate or paraphrase another producer's format-contract content, only point at it. The stencil may pin its *own* two output files' shape (it already does); it must not restate `Discussion-Validate`'s checklist or `Discussion-Review`'s rubric.
- **Markdown Link Integrity** — every inline link under `manifest/` and `docs/` must resolve, file part and `#anchor` alike. Deleting a design doc without retargeting its five inbound links fails `TestEnforcement_MarkdownLinks`.
- **Test Tier Purity Invariant** — keeps the new tests hermetic and tier 1; no test may spawn a real agent or resolve a real cwd.
- **CLI/Cobra Invariant** — not engaged; no command, flag, or `Short` changes here.

From `CLAUDE.md`:

- Docs land in the same commit as the code: `manifest/designs/loom.md`, `manifest/roadmap.md`, and the `CONSTRAINTS.md` sentence.
- `manifest/roadmap.md` moves for this item because it completes a planned item and adds one.
- Markdown uses semantic line breaks, one sentence per line, in every `.md` file this task touches.
- Worktree isolation: everything happens in this worktree; nothing is pushed to `main` from here.

## Testing

Tier 1, hermetic, no smoke suite.
An end-to-end test driving a real autonomous agent would be slow and non-deterministic, and `internal/loomcli/smoke_test.go`'s own doc comment already leans on the durable status file rather than on live producers.

- **`internal/shedrecipe`** — TDD candidate. Table test for `discussionWriteEntry`: rejects a nil `Env.DiscussionSpec` (including a typed-nil `SpecSource`), rejects a nil `Env.Shuttle`, rejects an unknown `Config` key, and on the happy path returns a non-nil `shedengine.ShedProducer` whose `Call` drives the injected `SpecSource` and a fake `Shuttle`. Mirror `entries_singlellm_test.go`'s existing shape.
- **`internal/shedrecipe/registry_test.go`** — update the exact-names pin from twelve to thirteen entries, including the test's own name and doc comment, and confirm `TestNames`' sortedness assertion still holds with `DiscussionWrite` inserted.
- **`internal/loomrecipe`** — `coverage_guard_test.go` drives loom's real row list against the registry and its doc comment enumerates which rows are still stubs; `shape_test.go`, `resume_test.go`, and `sequence_test.go` carry fixtures and comments that assume `Discussion-Write` is a `Stub` reporting `Done` unconditionally. Each needs its expectation moved to the new engine; the fixture `Env` now has to supply a `DiscussionSpec` for the row to construct at all.
- **`internal/loomcli/wiring_test.go`** — assert `wire()` fills `Env.DiscussionSpec` and `Env.Shuttle`, and that evaluating the closure yields a `shuttleengine.Spec` with both `_lyx/discussion/` output files absolute, `Interactive: false`, `Role: "discussion"`, the model resolved from the `discussion` role's model-spec, and the timeout from `discussion_timeout_min`.
- **`internal/loomengine`** — `prompt_test.go` and `discussion_test.go` already cover marker filling and both `modeRules` branches; extend them to prove the rewritten stencil still fills all four markers non-empty and that the Step 0 skill-load line and the exploration bound survive the fill.

Scenarios that must be covered, stated as behaviour rather than assertion shape: a row whose `Env.DiscussionSpec` is absent fails at *construction* with a message naming the entry and the field, not at first `Call`; the recipe still parses and builds end-to-end with row 3 on the new engine; and `Discussion-Write` still routes `on_done: Discussion-Validate` with no `on_stuck`.

## Q&A log

- **Q:** Interactive or autonomous `Discussion-Write`? **A:** Autonomous only; interactive becomes its own Planned roadmap item, because a resume-on-output-files pre-check cannot distinguish an interrupted interview from a `Discussion-Validate` bounce.
- **Q:** Generic `SingleLLM` recipe row or a loom-specific registry entry? **A:** Loom-specific `DiscussionWrite` entry — `{{.slug}}` and `{{.mode_rules}}` are per-run values a static recipe `tokens` map cannot carry, and a generic row would move model/effort selection off the `discussion` role's config and away from its timeout handling.
- **Q:** Which `scribe` skills does the stencil load? **A:** `scribe:prose` and `scribe:conversation` only; this producer writes no code.
- **Q:** Delete `manifest/designs/loom-format-discussion.md`? **A:** Yes — exactly what that doc's own Lifecycle section instructs, with all inbound links retargeted.
- **Q:** What replaces Step 3's `Architecture` interview category? **A:** A bounded coarse-level category keeping the positive/negative pair (MAY ask about module boundary and pattern conflict; MUST NOT enumerate signatures, `file:line`, interface shapes, dependency lists, or do exhaustive pattern research), rather than deleting the category and risking overcorrection.
- **Q:** How does the Spec reach the registry entry? **A:** `Env.DiscussionSpec shedadapters.SpecSource`, filled in `wire()`; the generic `SpecSources` map was rejected as speculative generality for one call site.
- **Q:** Keep `DiscussionSpec`'s `autonomous` parameter and the interactive `modeRules` branch? **A:** Keep both; the follow-up item flips one argument instead of re-authoring tested prose.
- **Q:** Adjust `discussion_timeout_min`? **A:** Keep `480`, fix only the now-inaccurate comment.
- **Q:** Test scope? **A:** Tier 1 across five sites; no `//go:build smoke` end-to-end test — the project's own smoke doc already advises against that shape.
- **Q:** Does `Discussion-Write` gain an `on_stuck`? **A:** No. A self-bounce with no context about why the previous agent stopped is blind guessing that burns the bounce budget, and `Discussion-Validate` already owns the one legitimate bounce path into this row.
- **Q:** Keep the stencil's trailing `AskUserQuestion` prohibition? **A:** Yes — cheap, robust to a shuttle config change, and correct again when interactive mode lands.
- **Q:** Any new `CONSTRAINTS.md` invariant? **A:** No; only the Shed Recipe Registry Invariant's twelve-names sentence changes to thirteen.
- **Q:** Does `docs/overview.md` change? **A:** No. `CLAUDE.md`'s rule triggers on module-table or execution-stack changes; no module is added or removed, only an engine key flipped on one recipe row.
