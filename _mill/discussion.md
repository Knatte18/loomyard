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

- Rewrite `contracts/stencils/loom/loom-template-discussion.md`: fold in `loom-format-discussion.md`'s Fix 1 exploration bound (Step 2 and Step 3), replace Step 3's `Architecture` interview category with a bounded coarse-level one, add a best-effort Step 0 that loads `scribe:prose` and `scribe:conversation`, and rewrite the leading HTML comment to describe the real call path.
- Add a `DiscussionWrite` entry to `internal/shedrecipe`'s registry, constructing `shedadapters.NewSingleLLMProducer` over an injected Spec closure and wrapping it in the commit decorator below.
- Add `internal/loomshed/discussionwrite.go`: a thin decorator producer that delegates to the wrapped `SingleLLMProducer` and, on `Done`, commits the two produced artifacts into the weft.
- Add two fields to `shedrecipe.Env`: `DiscussionSpec shedadapters.SpecSource` and `CommitDiscussion func() error`, both filled in `internal/loomcli/wiring.go`; fill the so-far-empty `Env.Shuttle` with `runner`.
- Add `loomengine.DiscussionDirRel()`, the anchor-relative counterpart of `DiscussionDir`, for the commit pathspec.
- Correct `internal/loomengine/prompt.go`'s autonomous `modeRules` text, which currently names a `--auto` flag that does not exist.
- Flip `contracts/recipes/loom-recipe.yaml`'s row 3 from `engine: Stub` to `engine: DiscussionWrite`, routing unchanged.
- Delete `manifest/designs/loom-format-discussion.md` and retarget its five inbound markdown links.
- Update `manifest/designs/loom.md` (producer-table row 3 and the module-decomposition table's stale "built but not yet wired into `Shed`" claim), `internal/loomengine/discussion.go`'s stale header comment, `internal/loomshed/stub.go`'s doc comment, `manifest/roadmap.md` (this item Planned → Done, plus a new Planned item for interactive mode), and one sentence of `CONSTRAINTS.md`.
- Tests at the sites listed under Testing below.

**Out:**

- **Interactive Discussion-Write.** The producer runs autonomously only. Interactive mode is deferred to a new Planned roadmap item this task writes (see Decisions → `autonomous-only`).
- **`Discussion-Review`, `Plan-Write`, `Plan-Review`, `Webster-Review`** — each is its own roadmap item and each stays a `Stub`. In particular, no `Bouncer`/`Burler` perch is wired here.
- **`planparser`'s Card-format migration** — the sibling Wave 2 item; this task is independent of it and touches no `planparser` or `websterengine` code.
- **`loomengine.PlanSpec` and `Env.PlanSpec`** — Wave 3's `Plan-Write` adds the symmetric fields when it needs them, not before.
- **`docs/overview.md`** — untouched. `CLAUDE.md`'s docs rule triggers on module-table or execution-stack changes; no module is added or removed here, only an engine key flipped on one recipe row.
- **`SingleLLMProducer` itself** — no change to `internal/shedadapters`. Its archive-then-respawn behaviour on a stale output file is used as-is.
- **`internal/loomshed/discussionvalidate.go`** — unchanged. It keeps discarding `discussionparser.Validate`'s findings (see Decisions → `blind-revalidate-bounce`).
- **The generic `SingleLLM` registry entry** — left in place and untouched; it stays available for a future recipe row whose Spec really is static recipe config.
- **`internal/discussionparser`** — its mechanical checks are unaffected; this task changes no heading names in either output file.
- **Plugin installation machinery** — nothing in the tree installs or verifies Claude Code plugins, and this task adds none.

## Decisions

### autonomous-only

- Decision: `Discussion-Write` runs with `Interactive: false`. The wiring site passes `autonomous: true` to `loomengine.DiscussionSpec` unconditionally. Interactive mode becomes a new Planned roadmap item in the `loom: real LLM producers` group.
- Rationale: interactive interviewing does not survive resume with today's machinery, and making it survive is a separate, well-understood piece of work. `shuttleengine`'s `Wait` classifies any turn end without all output files present as `OutcomeAsking` (`internal/shuttleengine/wait.go`, `pollEventsTick`); `SingleLLMProducer` maps `Asking` to `Stuck` (`internal/shedadapters/singlellm.go`); `Discussion-Write` has no `on_stuck`, so `shedengine` persists `blocked` and exits (`internal/shedengine/run.go`, the `def.OnStuck == ""` branch). That much is loom's documented "stops cleanly at the Discussion input boundary" behaviour, and the agent's pane survives it, because `wait.go` only cleans a pane on `OutcomeDone`. The break is on resume: `shedengine` re-calls the same `current_producer`, and `SingleLLMProducer.Call` archives both freshly-written files (`internal/shedadapters/archive.go`) and spawns a new agent that knows nothing of the interview. `SingleLLMProducer`'s own package doc states it never reattaches to a live shuttle session. A resume-on-output-files pre-check ("both files exist → report `Done` without spawning") would fix that case but breaks the other one: `Discussion-Validate`'s `on_stuck: Discussion-Write` bounce also re-enters this row with both files present, and the pre-check cannot distinguish "present and valid" from "present and just rejected by the validator" — it would report `Done` instantly and ping-pong until the bounce budget is exhausted. Resolving that ambiguity is real design work and does not belong inside an otherwise bounded wiring task.
- Rejected: shipping interactive mode with the pre-check (the ambiguity above); shipping both modes behind a `lyx loom run --auto` flag (no such flag exists today, and the interactive branch inherits the same defect).

### spec-closure-in-env

- Decision: `shedrecipe.Env` gains `DiscussionSpec shedadapters.SpecSource`. `internal/loomcli/wiring.go` fills it with a closure over `loomengine.DiscussionSpec`. The new `discussionWriteEntry` validates `Env.DiscussionSpec`, `Env.CommitDiscussion`, and `Env.Shuttle` non-nil, and returns `shedadapters.NewSingleLLMProducer(name, env.DiscussionSpec, env.Shuttle, env.Now)` wrapped per `commit-produced-artifacts` below.
- Rationale: `loomengine.DiscussionSpec` takes a `*lyxcwd.Location` as its first parameter, and the Shed Recipe Registry Invariant bars `internal/shedrecipe` from a direct `internal/lyxcwd` production import — passing that type through `Env` would breach it. `Env` already solves exactly this shape twice with whole-value passthrough fields, `WebsterDeps websterengine.RunDeps` and `Landing landingshed.Deps`, and it already carries per-producer named fields (`DecisionRecordPath`, `SupportLogPath`) rather than a generic bag. `wire()` is where `location`, `loomCfg`, `registry`, `runner`, and `websterGeom.StencilsDir` are all already in scope, so the closure costs no new plumbing. Evaluating the closure per `Call` (which is what `SpecSource` means) also keeps the stencil read at call time, as the Stencil Ownership Invariant requires.
- Rejected: refactoring `DiscussionSpec` to take told strings and having the entry call it directly (pulls `loomengine` and `modelspec` into `shedrecipe` and churns a tested signature); a generic `Env.SpecSources map[string]SpecSource` keyed by row name (speculative generality for one call site — Wave 3 adds `Env.PlanSpec` symmetrically when it needs it); reusing the generic `SingleLLM` entry (its `Config.tokens` are static recipe strings, but `{{.slug}}` and `{{.mode_rules}}` are per-run, and its `model`/`effort` config would bypass the `discussion` role's model-spec resolution and `discussion_timeout_min` entirely).

### commit-produced-artifacts

- Decision: this task commits `decision-record.md` and `support-log.md` into the weft as soon as the producer reports `Done`. A new `internal/loomshed/discussionwrite.go` holds a thin decorator — `NewDiscussionWrite(name string, inner shedengine.ShedProducer, commit func() error) shedengine.ShedProducer` — that delegates `Call` to the wrapped `SingleLLMProducer` and, on a `Done` outcome with a nil error, invokes `commit`. A commit failure returns an error rather than `Stuck`: a git fault is not something re-writing the discussion can fix, exactly the reasoning `discussionvalidate.go` already applies to a non-not-exist read failure. `shedrecipe.Env` gains `CommitDiscussion func() error`, filled in `wire()` with a closure over `fabricengine.CommitAnchoredPaths(fabricengine.NewMutations(""), location, []string{loomengine.DiscussionDirRel()}, msg, fabricengine.EnvSyncOptions())` — the same call shape `internal/loomcli/run.go` already uses for the seed commit. `loomengine.DiscussionDirRel()` is added as the anchor-relative counterpart of `DiscussionDir`, since the Cwd Resolution Invariant makes `loomengine` the sole declarer of that path.
- Rationale: `DiscussionDir` anchors at `lyxdirs.LyxDirName` (`_lyx`, durable), while `fabricengine`'s weft exclude list (`seedWeftArtifactExcludes`) covers only `lyxdirs.DotLyxDirName` (`.lyx`, ephemeral) — so the two produced files are genuinely tracked-able weft content that nothing currently commits. `run.go:118` commits only the status file and the origin record, and no `loomshed`/`landingshed` row touches these two. Leaving them untracked leaves the weft dirty for `Finalize`'s merge guard and for any subsequent fresh `Preflight`, whose fabric scan includes untracked files. The Fabric Write-Side Containment and Fabric Git invariants put committing agent-written content on the Go side, not the agent's, so the stencil must not be told to run git.
- **Committing before validation is intentional, not an oversight.** The decorator fires on `Discussion-Write`'s `Done`, which is before `Discussion-Validate` has judged anything, so a bounce round genuinely commits output the validator is about to reject, and the following round commits the corrected version as a second commit. That is the right trade: the commit exists to keep the weft clean and the artifact durable, not to certify it. Moving it behind validation (a decorator on `Discussion-Validate` instead) would leave the files untracked for the whole write-to-validate window — exactly the dirt this decision exists to eliminate — and would leave them uncommitted entirely whenever a run blocks at validation. Weft is a state repo; an honest intermediate commit followed by its fix is better history than a gap.
- **Pathspec is the whole `_lyx/discussion/` directory, and that deliberately includes the archive siblings.** `shedadapters.archiveStaleOutputs` renames a stale output to a timestamped sibling *in the same directory*, so each bounce round leaves a `decision-record-<stamp>.md` beside the live files. A two-file pathspec would leave those as untracked weft dirt, re-creating the problem; the directory pathspec commits them instead. That is also useful — the archived draft is the only surviving record of what the validator rejected — and it is bounded by the per-producer bounce budget over two small markdown files.
- **Commit message** is `fmt.Sprintf("loom: discussion artifacts for %s", slug)`, mirroring `run.go:120`'s `"loom: seed session bootstrap for %s"` shape. The slug is already in scope at the wiring site, and this text is durable weft history, so it is pinned here rather than left to the plan writer.
- Rejected: declaring it out of scope and recording the dirty-weft consequence (ships a pipeline with a known break); deferring to `Publish`/`Finalize` (`landingshed` is shared by reference with the Someday `Hardener`, so teaching it about `_lyx/discussion/` puts loom-specific knowledge in a generic producer); having the discussion agent run `git` itself (barred by the Fabric Write-Side Containment Invariant); decorating `Discussion-Validate` instead so only validated output is committed (leaves the write-to-validate window dirty, and commits nothing at all when a run blocks at validation); a two-file pathspec with the archive siblings deleted after a successful commit (throws away the rejected draft and adds delete logic for no gain).

### blind-revalidate-bounce

- Decision: the `Discussion-Validate` → `Discussion-Write` bounce stays context-free. The respawned agent receives the identical prompt and no findings, and `discussionvalidate.go` keeps discarding them. This is recorded as an accepted, reasoned behaviour rather than left as an unexamined default.
- Rationale: `discussionparser.Validate` is exhaustively defined by two mechanical checks — both files exist, and `decision-record.md` carries all seven required H2 headings — with no judgment component. The stencil states those seven headings, in order, in its own Step 5. A bounce therefore means the previous agent failed to follow an explicit, enumerated instruction, which is precisely the failure a fresh agent has an independent chance of not repeating, and the per-producer bounce budget bounds the retries. Threading findings through would buy little for a heading-presence check while costing a new stencil marker, a place to persist findings between `Call`s, and a change to a shipped producer's contract.
- Rejected: threading `discussionparser.Validate`'s findings into the respawned prompt (real scope growth into an already-shipped producer, for a two-check mechanical validator); removing `on_stuck` from `Discussion-Validate` so a validation failure escalates to a human instead (changes a shipped row's routing and gives up automatic recovery).

### scribe-best-effort

- Decision: Step 0 instructs the agent to load `scribe:prose` and then `scribe:conversation` **if they are available**, and to continue without them if they are not. The manual `/plugin install scribe@loomyard` step is recorded as an operator prerequisite in `manifest/roadmap.md`'s Done entry for this item.
- Rationale: `manifest/roadmap.md` records that install as a manual step not yet done, and nothing in `shuttleengine`/`claudeengine` installs or verifies plugins. A missing plugin therefore degrades prose quality rather than breaking a run, which is the right failure mode for a writing-style aid. Naming the availability condition explicitly also stops a spawned agent from treating an unresolvable skill name as a hard error and stalling.
- Rejected: a hard precondition in `Loom-Preflight` or the registry entry (nothing in the tree knows about plugins; a whole new surface in a shipped producer); dropping Step 0 until the plugin is installed (abandons the wiring `plugins/scribe/skills/INDEX.md` names this task as the owner of).

### keep-mode-rules-both-branches

- Decision: `loomengine.DiscussionSpec` keeps its `autonomous bool` parameter, and `prompt.go`'s `modeRules` keeps both its autonomous and interactive branches, along with their existing tests. Only the wiring site pins the value. The autonomous branch's text is corrected, though: it currently reads "autonomous (`--auto`) mode", naming a `lyx loom run` flag that does not exist, and becomes a plain statement that the session is autonomous and no operator will answer questions.
- Rationale: the interactive follow-up item then flips one argument instead of re-authoring prose that is already written and tested. The unreached branch is a few lines of string data, not logic. The `--auto` phrase is a separate matter: it ships in the prompt this task pins on, so it would be a live inaccuracy in a shipped artifact, and the follow-up item that introduces a real flag can name it then.
- Rejected: deleting the parameter and the interactive branch (cleaner today, but throws away exactly what the committed follow-up item needs back); leaving the `--auto` phrase for the follow-up item (ships a prompt citing a nonexistent flag).

### no-on-stuck

- Decision: `Discussion-Write` keeps no `on_stuck` in the recipe. A `Stuck` outcome escalates to a human.
- Rationale: under autonomous mode, `Stuck` means the agent ended a turn without writing both files — it gave up, or died mid-write. A self-bounce would archive the partial work and respawn a fresh agent carrying no information about why the previous one stopped, which is blind guessing that burns the bounce budget. `Discussion-Validate` already owns the one legitimate bounce path into this row, and that path at least carries the signal that the *files* were the problem.
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
- Rationale: nothing cross-cutting is introduced — the existing registry, told-geometry, and fabric invariants already cover the new entry and the commit decorator — and the count is a machine-checked fact that must not go stale.
- Rejected: adding an invariant about per-producer `Env` closure fields.

## Technical context

**The stub and the row.**
`internal/loomshed/stub.go` backs five of loom's thirteen rows; its doc comment names all five, so removing `Discussion-Write` from that list is part of this task.
`NewStub` itself stays — four rows still use it.
`contracts/recipes/loom-recipe.yaml` is embedded via `contracts/recipes/recipes.go` and parsed by `internal/loomrecipe.New` through `shedbuild.Parse` + `shedbuild.Build`; there is no on-disk runtime copy.

**What already exists and is currently dead.**
`internal/loomengine/discussion.go`'s `DiscussionSpec` and `internal/loomengine/prompt.go`'s `composePrompt` are complete and unit-tested but have no non-test caller.
`DiscussionSpec` resolves the `discussion` role's model-spec through `modelspec.Parse` + `Registry.Resolve`, names both output paths via `loomengine.DiscussionDecisionRecord` / `DiscussionSupportLog`, composes the prompt, and returns a `shuttleengine.Spec` with `Timeout` from `cfg.DiscussionTimeoutMin`.
It errors on an empty slug.
This task's job is to call it, not to rewrite it — apart from the `--auto` wording correction in `modeRules`.

**Two doc claims that go stale the moment this lands.**
`internal/loomengine/discussion.go`'s header comment says "the future loom phase machine drives the returned Spec through shuttle.Run", and `manifest/designs/loom.md`'s module-decomposition table says `DiscussionSpec`/`PlanSpec` are "both ✅ **built** but not yet wired into `Shed`".
Both must be corrected in the same commit; `PlanSpec` is still genuinely unwired, so the latter becomes a per-Spec statement rather than a joint one.

**The registry.**
`internal/shedrecipe/registry.go` holds one central `map[string]Constructor` literal reached only through `Lookup` and `Names` — no `init()` self-registration, no runtime `Register`.
`internal/shedrecipe/entries_simple.go` is the home for entries taking an empty `Config`; `discussionWriteEntry` takes an empty `Config` (everything it needs is on `Env`) and belongs there or in its own file.
`configRejectUnknown(cfg)` with no allowed keys is how an empty-`Config` entry rejects stray recipe config.
`requireSeam` is the nil-check helper, and it detects a typed-nil interface, which matters for both new `Env` fields since each is a func type.

**The wiring site.**
`internal/loomcli/wiring.go`'s `wire()` builds everything: `loomCfg` (`loomengine.LoadConfig`), `registry` (`modelspec.LoadRegistry`), `runner` (`shuttleengine.NewRunner`), and `websterGeom` (`hubgeom.WebsterGeometry(location)`, whose `StencilsDir` field is what `internal/loomcli/landingdeps.go` already uses).
The slug comes from `seedSlug(c.location.WorktreeName)` — `seedSlug` lives in `internal/loomcli/seedinput.go` and `internal/loomcli/run.go` already calls it that way.
The `c.env` literal carries a comment reading "StencilsDir, RunRoot, Shuttle, Burler, and Now are left zero — only SingleLLM, Bouncer, and BurlerRound read them, and no row in loom's recipe uses those engines yet"; that comment is now wrong about `Shuttle` and must be corrected.
`Env.StencilsDir` stays unfilled, because the closure captures the stencils directory directly.
`Env.Now` stays nil, which `NewSingleLLMProducer` defaults to `time.Now`.

**The weft-tracking fact behind the commit decorator.**
`lyxdirs.LyxDirName` is `_lyx` and `lyxdirs.DotLyxDirName` is `.lyx`.
`fabricengine`'s `seedWeftArtifactExcludes` (`internal/fabricengine/weftgit.go`) writes `.lyx/` into the weft's `.git/info/exclude`, not `_lyx/`, so `_lyx/discussion/`'s contents are ordinary trackable weft files.
`internal/loomcli/run.go`'s seed commit shows the exact call shape to reuse, and its own comment records why an uncommitted `_lyx` path matters: "the phase machine's very first precondition row scans the fabric including untracked files".

**Outcome mapping, for anyone reasoning about behaviour.**
`SingleLLMProducer.Call` archives stale outputs, runs the shuttle seam once, and maps `OutcomeDone` → `Done` with an `OutputPointer` naming `spec.OutputFiles[0]`; `OutcomeAsking` → `Stuck`; `OutcomeDied`/`OutcomeTimeout` → error.
It rejects a non-absolute `OutputFiles` entry outright — `DiscussionSpec`'s paths are `AnchorPath`-anchored and therefore absolute.
The decorator must preserve every one of these mappings untouched and add only the commit on `Done`.

**The bounce path this task makes live.**
`internal/loomshed/discussionvalidate.go`'s `Call` maps a non-empty findings slice to bare `Stuck` with an empty pointer, "the findings themselves discarded here" (its own doc comment).
`shedengine` then routes via `Discussion-Validate`'s `on_stuck: Discussion-Write`, and `SingleLLMProducer.Call` archives both files and respawns with an identical prompt.
See Decisions → `blind-revalidate-bounce` for why that is accepted.

**The five inbound links to the doc being deleted** (Markdown Link Integrity is machine-enforced by `internal/lyxcwd/docslink_test.go`):
`manifest/designs/loom.md:35` (producer-table row 3) and `:115` (the relocation-rubric subsection's companion-doc pointer), `manifest/roadmap.md:14` (the card-format group intro) and `:168` (the `## Done` entry for `loom: redesign the Discussion format`), and `manifest/designs/plan-card-format.md:3` (the status blockquote).
The deleted doc's Lifecycle section states the natural retarget for each is the stencil, and that the `## Done` entry may instead keep a historical, non-link reference — that call is this task's.

**The stencil's four markers.**
`stencil.Fill` requires all four of `{{.slug}}`, `{{.mode_rules}}`, `{{.decision_record_path}}`, `{{.support_log_path}}` to be non-empty, and the file must contain no `{{if}}`/`{{range}}` conditionals — a required marker inside a conditional branch renders silently blank.
The rewrite must keep all four markers and add none.

## Constraints

From `CONSTRAINTS.md`:

- **Shed Recipe Registry Invariant** — every registry value constructs a `shedengine.ShedProducer` and nothing else; the registry stays one central map literal reached only through `Lookup`/`Names`; `internal/shedrecipe` takes every absolute path from its caller and has no direct production import of `internal/lyxcwd`. This is what forces both new `Env` fields to be closures rather than a `*lyxcwd.Location`. Enforced by `internal/shedrecipe/seam_enforcement_test.go`, `internal/loomrecipe/coverage_guard_test.go`, and `internal/shedrecipe/registry_test.go`.
- **Cwd Resolution Invariant** — `internal/lyxcwd` owns cwd resolution alone and each module owns its own relative subpath, which is why `DiscussionDirRel` is added to `loomengine` rather than assembled at the commit call site.
- **Fabric Write-Side Containment Invariant** and **Fabric Git Invariant (warp + weft)** — committing agent-written weft content is Go's job through `internal/fabricengine`, never the agent's and never a raw `git` call; the commit is a positive-pathspec `CommitAnchoredPaths`, never a stage-all.
- **Mutation Record Invariant** — the commit passes a `fabricengine.NewMutations("")` record, matching `run.go`'s existing seed-commit call.
- **Stencil Ownership Invariant** — the prompt is read at call time from a told absolute stencils directory, never from embedded bytes; `//go:embed` in `contracts/stencils` is seed-only. The per-`Call` closure preserves this. The stencil is already registered in `contracts/stencils/stencils.go`, so editing its content needs no registry change. **Operator step:** `stencilstore.reconcileOne`'s `StateUntouched` branch returns without writing under `ModeDev` (it warns instead), so an already-seeded hub running a `-dev`-stamped binary keeps the *old* stencil text after this lands. `lyx stencil sync` forces the refresh — it routes through `stencilstore.ForceRefresh`, which reconciles in `ModeProduction` regardless of the build channel. Note it alongside the `scribe` install as a manual step.
- **Producer Pointer-Rule Invariant** — an instruction file must never duplicate or paraphrase another producer's format-contract content, only point at it. **The seven required H2 headings stay enumerated in the stencil's Step 5** — that is the output-shape carve-out (the stencil pinning its *own* two output files' shape), not a restatement of `Discussion-Validate`'s checklist, and `blind-revalidate-bounce`'s rationale depends on the enumeration being there. Do not delete it in the name of this rule. What the stencil must not carry is `Discussion-Validate`'s checklist framing or `Discussion-Review`'s rubric.
- **Markdown Link Integrity** — every inline link under `manifest/` and `docs/` must resolve, file part and `#anchor` alike. Deleting a design doc without retargeting its five inbound links fails `TestEnforcement_MarkdownLinks`.
- **Test Tier Purity Invariant** — keeps the new tests hermetic and tier 1; no test may spawn a real agent or resolve a real cwd. The commit decorator is testable with an injected `commit` closure, so no test needs a real git repo.
- **CLI/Cobra Invariant** — not engaged; no command, flag, or `Short` changes here.

From `CLAUDE.md`:

- Docs land in the same commit as the code: `manifest/designs/loom.md`, `manifest/roadmap.md`, and the `CONSTRAINTS.md` sentence.
- `manifest/roadmap.md` moves for this item because it completes a planned item and adds one.
- Markdown uses semantic line breaks, one sentence per line, in every `.md` file this task touches.
- Worktree isolation: everything happens in this worktree; nothing is pushed to `main` from here.

## Testing

Tier 1, hermetic, no smoke suite.
An end-to-end test driving a real autonomous agent would be slow and non-deterministic, and `internal/loomcli/smoke_test.go`'s own doc comment already leans on the durable status file rather than on live producers.

- **`internal/loomshed`** — TDD candidate, the one genuinely new behaviour. Table test for the `DiscussionWrite` decorator against a fake inner `ShedProducer`: `Done` invokes `commit` exactly once and passes the inner `OutputPointer` through unchanged; `Stuck` and an inner error each leave `commit` uninvoked; a `commit` that returns an error surfaces as a returned error with an empty outcome, never as `Stuck`; and the context entry-check still short-circuits before the inner call.
- **`internal/shedrecipe`** — TDD candidate. Table test for `discussionWriteEntry`: rejects a nil `Env.DiscussionSpec`, a nil `Env.CommitDiscussion` (both including the typed-nil func case), and a nil `Env.Shuttle`, each with a message naming the entry and the field; rejects an unknown `Config` key; and on the happy path returns a non-nil `shedengine.ShedProducer` whose `Call` drives the injected `SpecSource`, a fake `Shuttle`, and the injected commit closure. Mirror `entries_singlellm_test.go`'s existing shape.
- **`internal/shedrecipe/registry_test.go`** — update the exact-names pin from twelve to thirteen entries, including the test's own name and doc comment, and confirm `TestNames`' sortedness assertion still holds with `DiscussionWrite` inserted.
- **`internal/loomrecipe`** — `coverage_guard_test.go` drives loom's real row list against the registry and its doc comment enumerates which rows are still stubs; `shape_test.go`, `resume_test.go`, and `sequence_test.go` carry fixtures and comments that assume `Discussion-Write` is a `Stub` reporting `Done` unconditionally. Each needs its expectation moved to the new engine; the fixture `Env` now has to supply both new closures for the row to construct at all.
- **`internal/loomcli/wiring_test.go`** — assert `wire()` fills `Env.DiscussionSpec`, `Env.CommitDiscussion`, and `Env.Shuttle`, and that evaluating the Spec closure yields a `shuttleengine.Spec` with both `_lyx/discussion/` output files absolute, `Interactive: false`, `Role: "discussion"`, the model resolved from the `discussion` role's model-spec, and the timeout from `discussion_timeout_min`.
- **`internal/loomengine`** — `prompt_test.go` and `discussion_test.go` already cover marker filling and both `modeRules` branches; extend them to prove the rewritten stencil still fills all four markers non-empty, that the corrected autonomous text no longer names `--auto`, and that the Step 0 skill-load line and the exploration bound survive the fill. Add a case pinning `DiscussionDirRel`'s value against `DiscussionDir` so the two cannot drift.

Scenarios that must be covered, stated as behaviour rather than assertion shape: a row whose `Env.DiscussionSpec` or `Env.CommitDiscussion` is absent fails at *construction* with a message naming the entry and the field, not at first `Call`; the recipe still parses and builds end-to-end with row 3 on the new engine; `Discussion-Write` still routes `on_done: Discussion-Validate` with no `on_stuck`; and a second `Done` over already-committed artifacts is a harmless no-op rather than an error.

## Q&A log

- **Q:** Interactive or autonomous `Discussion-Write`? **A:** Autonomous only; interactive becomes its own Planned roadmap item, because a resume-on-output-files pre-check cannot distinguish an interrupted interview from a `Discussion-Validate` bounce.
- **Q:** Generic `SingleLLM` recipe row or a loom-specific registry entry? **A:** Loom-specific `DiscussionWrite` entry — `{{.slug}}` and `{{.mode_rules}}` are per-run values a static recipe `tokens` map cannot carry, and a generic row would move model/effort selection off the `discussion` role's config and away from its timeout handling.
- **Q:** Which `scribe` skills does the stencil load? **A:** `scribe:prose` and `scribe:conversation` only; this producer writes no code.
- **Q:** Delete `manifest/designs/loom-format-discussion.md`? **A:** Yes — exactly what that doc's own Lifecycle section instructs, with all inbound links retargeted.
- **Q:** What replaces Step 3's `Architecture` interview category? **A:** A bounded coarse-level category keeping the positive/negative pair (MAY ask about module boundary and pattern conflict; MUST NOT enumerate signatures, `file:line`, interface shapes, dependency lists, or do exhaustive pattern research), rather than deleting the category and risking overcorrection.
- **Q:** How does the Spec reach the registry entry? **A:** `Env.DiscussionSpec shedadapters.SpecSource`, filled in `wire()`; the generic `SpecSources` map was rejected as speculative generality for one call site.
- **Q:** Keep `DiscussionSpec`'s `autonomous` parameter and the interactive `modeRules` branch? **A:** Keep both; the follow-up item flips one argument instead of re-authoring tested prose.
- **Q:** Adjust `discussion_timeout_min`? **A:** Keep `480`, fix only the now-inaccurate comment.
- **Q:** Test scope? **A:** Tier 1 across six sites; no `//go:build smoke` end-to-end test — the project's own smoke doc already advises against that shape.
- **Q:** Does `Discussion-Write` gain an `on_stuck`? **A:** No. A self-bounce with no context about why the previous agent stopped is blind guessing that burns the bounce budget, and `Discussion-Validate` already owns the one legitimate bounce path into this row.
- **Q:** Keep the stencil's trailing `AskUserQuestion` prohibition? **A:** Yes — cheap, robust to a shuttle config change, and correct again when interactive mode lands.
- **Q:** Any new `CONSTRAINTS.md` invariant? **A:** No; only the Shed Recipe Registry Invariant's twelve-names sentence changes to thirteen.
- **Q:** Does `docs/overview.md` change? **A:** No. `CLAUDE.md`'s rule triggers on module-table or execution-stack changes; no module is added or removed, only an engine key flipped on one recipe row.
- **Q:** What happens when `Discussion-Validate` bounces back into `Discussion-Write`? **A:** The blind re-write is accepted and documented — the validator's two checks are purely mechanical heading/existence checks the stencil already enumerates, so a fresh agent converges within the bounce budget; threading findings through was rejected as scope growth into a shipped producer.
- **Q:** The `scribe` plugin is not installed — what happens at Step 0? **A:** Best-effort wording ("if available, load … ; otherwise continue"), with the manual install recorded as an operator prerequisite in the roadmap Done entry. A hard precondition was rejected because nothing in the tree knows about plugins.
- **Q:** Does anything commit the two produced files? **A:** This task does, via a thin `loomshed` decorator that calls an injected `Env.CommitDiscussion` closure on `Done`. `_lyx/` is not on the weft exclude list (only `.lyx/` is), so the files are real weft dirt otherwise; deferring to `Publish`/`Finalize` was rejected because `landingshed` is shared with the Someday `Hardener`.
- **Q:** The commit fires before `Discussion-Validate` has judged the output — is that intended? **A:** Yes. The commit keeps the weft clean and the artifact durable; it does not certify it. A bounce round commits the rejected draft and the next round commits its fix. Decorating `Discussion-Validate` instead was rejected: it leaves the write-to-validate window dirty and commits nothing when a run blocks at validation.
- **Q:** What happens to `archiveStaleOutputs`' timestamped siblings? **A:** They are committed too — the pathspec is the whole `_lyx/discussion/` directory precisely so they are not left as untracked dirt, and the archived draft is the only record of what the validator rejected.
- **Q:** What is the commit message? **A:** `"loom: discussion artifacts for %s"` with the slug, mirroring `run.go:120`'s seed-commit shape.
- **Q:** Will an existing hub pick up the rewritten stencil? **A:** Not on a `-dev` binary — `stencilstore` seeds but never refreshes an untouched stencil in `ModeDev`. `lyx stencil sync` forces it, and that is recorded as a manual operator step beside the `scribe` install.
