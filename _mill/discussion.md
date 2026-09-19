# Discussion: Shed-generic watchdog for ly-drive and loom's CLI verbs

```yaml
task: Shed-generic watchdog for ly-drive and loom's CLI verbs
slug: shed-generic-watchdog
status: discussing
parent: main
```

## Problem

`shedengine`, `shedbuild` and `shedrecipe` are already fully generic — an engine is handed the absolute paths it operates on and derives none of its own (the Told-Geometry Invariant), the recipe file format is data, and the engine registry resolves every row's `engine` name without knowing which product asked.
Everything above that line is not.
`internal/loomcli`'s `start.go`/`run.go`/`step.go` hardcode loom's own recipe, paths, envelope shape, and interrupt-policy table, and `internal/lifecyclecli` — the second `shedrecipe` consumer — reimplements the same status decode, the same `ErrShedBusy` refusal, and the same run envelope a second time in its own `run.go`/`status.go`.
`internal/loomrecipe` and `internal/lifecyclerecipe` are near-literal copies of each other: the same five-field `ShedPaths` struct and the same `Parse(embedded bytes) → Build(env) → &shedengine.Shed{…}` body, differing only in which embedded recipe they name and in loomrecipe's extra Env/paths coherence guard.
The `ly-drive` skill, which already claims to carry no phase knowledge and to branch only on envelope fields, still names `lyx loom step` in every instruction.

**Why now:** the design doc (`manifest/designs/shed-generic-watchdog.md`) held this speculative pending "a second `shedrecipe` consumer beyond loom … to validate the generalization against".
That consumer now exists and has shipped as `lifecyclerecipe`/`lifecyclecli`, which is what promoted the item from Someday to Planned in `manifest/roadmap.md`.
The third consumer is already drafted (`manifest/designs/hardener.md`: `Hardener` is `Shed` wrapping `Tenter`), and it would otherwise arrive needing a third hand-written copy of the same verb bodies.

## Scope

**In:**

- A new `internal/shedcli` package owning the generic verb bodies — `run`, `step`, `status` (including `--watch`), and `pause` — built over a told `*shedengine.Shed` plus a told arming spec, with optional injected hooks for module-specific work.
- A new `lyx shed` cobra subtree exposing those four verbs, armed by a `--recipe <name>` flag resolved against a named-recipe table.
- `loomcli`'s `run`, `step`, `status`, `pause` reimplemented as arming over `shedcli`, preserving their present CLI surface and envelopes byte-for-byte.
- `lifecyclecli`'s `run` and `status` reimplemented the same way; `lifecyclecli` gains `pause` as a consequence of the shared verb set.
- `shedbuild` gains the deduplicated `ShedPaths` type and a `NewShed(recipe []byte, env, paths)` assembler; `loomrecipe.New` and `lifecyclerecipe.New` shrink to a delegation plus their own package-specific guards, and both packages' own `ShedPaths` types are removed in favour of the hoisted one.
- The lifecycle recipe's inner-run producer neutralized: the registry engine name `LoomRun`, the `Env.LoomRun` field, the `lifecycleshed.LoomRunDeps` type, `NewLoomRun`, the entry's own identifiers, and the producer's log and stuck-reason strings all stop naming loom.
  The seam only — no second lifecycle recipe and no Hardener innards (see the `inner-run-engine-goes-product-neutral` Decision).
- The `ly-drive` skill generalized to drive any recipe through `lyx shed step --recipe <name>`, with its loom-specific sections gated on the recipe name.
- A new **Shed Verb-Set Invariant** in `CONSTRAINTS.md`, with a seam-enforcement scan and a help-tree test enforcing it.
- Doc updates in the same commit: `manifest/designs/shed-generic-watchdog.md` — including a note recording Hardener as a future consumer of both the verb set and the neutralized inner-run engine — plus `docs/overview.md`'s module table and execution-stack entries, `manifest/roadmap.md`'s Planned item, and the package headers of every touched package.

**Out:**

- `lyx loom start` and its `lyx start` alias.
  `start` does not generalize (see the `start-stays-loom-specific` Decision); its file, its bootstrap helpers (`seedAndCommitBootstrap`, `ensureStatusStrand`, `awaitRunLock`, `mustSpawnDriver`, `spawnWatchdog`, `recordStepHandoff`), and its tests are untouched apart from any signature change forced by the hoisted `ShedPaths` type.
- `lyx loom validate-discussion` / `validate-plan`.
  These are standalone format self-checks, not Shed drivers, and stay on `loomcli`'s lightweight wiring path unchanged.
- Any change to `shedengine`, `shedadapters`, `shedcheck`, or the recipe file format.
- Any change to a *durable* recipe identity: the row names in either shipped recipe YAML, their `entry`/`terminals` values, or the `Name*` constants' string values that pin them.
  The one recipe-YAML edit in scope is the `Loom-Run` row's `engine:` value, which changes with the registry rename and is resolved at build time, never persisted.
  The registry stays at seventeen keys — this is a rename, not an addition.
- A second lifecycle recipe, a Hardener recipe, a Hardener arming function, or any Hardener innards.
- Any change to loom's own producer graph, its row names, its interrupt-policy table's *contents*, or `loomshed`/`lifecycleshed` producer constructors.
- Building a `Hardener` recipe or arming function.
  The generalization must make the third consumer cheap; it does not write it.
- Deleting `loomrecipe`/`lifecyclerecipe`.
  Both keep their graph-shape, sequence, resume and coverage-guard tests, which are product-specific and have no home in a generic package.
- Any new on-disk location for recipe files (barred by the Recipe-Format Sole-Parser Invariant) and any `shed.yaml`-style runtime recipe discovery.

## Decisions

### shedcli-owns-the-verb-bodies

- Decision: create `internal/shedcli`, a `<module>cli`-shaped package whose exported surface is an arming `Spec` type, a `Hooks` type, a `Verbs(spec) []*cobra.Command` (or equivalent) constructor returning the four generic subcommands, plus the module's own `Command()`/`RunCLI`/`RunCLIIn` seams for the `lyx shed` subtree.
  `loomcli` and `lifecyclecli` build their own subtrees from the same constructor, so there is exactly one body per verb in the tree.
- Rationale: the duplication is already two-deep and the third consumer is drafted.
  A shared package is what makes the verb/engine symmetry the design doc settled (`run` → `Shed.Run`, `step` → `Shed.Step`) literal rather than a coincidence maintained by hand in each `<module>cli`.
- Rejected: a shared-helpers-only refactor with each module keeping its own `RunE` (leaves the envelope key sets and the busy-refusal wording free to drift, which is the exact failure the parity tests would then have to police forever); a `lyx shed` subtree with `loomcli`/`lifecyclecli` left untouched (ships a third copy instead of removing two).

### named-recipe-table-arming

- Decision: `lyx shed <verb> --recipe <name>` resolves `name` against a table mapping a recipe name to an arming function.
  The arming function supplies everything told: the embedded recipe bytes, the filled `shedrecipe.Env`, the `shedbuild.ShedPaths`, the hooks, and the status-line label.
  The table ships with `loom` and `lifecycle`; a third consumer adds one entry and one arming function.
- Rationale: a recipe alone cannot arm a Shed — `Env` carries injected seams (`Shuttle`, `Burler`, `WebsterRun`) and told roots that no file can name.
  Binding the name to an arming function rather than to a file path is what keeps the flag honest about that.
- Rejected: `--recipe <path>` over an arbitrary recipe file (cannot supply the seams; would also imply a runtime on-disk location for recipe files, which `internal/shedbuild`'s own doc and the Recipe-Format Sole-Parser Invariant both forbid); a per-worktree `shed.yaml` (same problem, plus a new config surface the Config Strictness Invariant would have to cover).

### start-stays-loom-specific

- Decision: `start` gets no generic counterpart and stays in `loomcli` exactly as it is.
- Rationale: the design doc already records the reason and this task settles it — there is no `Shed.Start`.
  `start` seeds the status file, commits that seed into the fabric, ensures the reed substrate and its status strand, spawns the per-hub watchdog, spawns the detached driver, waits for the run-lock handshake, and hands the terminal over.
  Every one of those sits above the engine, and the second consumer has an analogue for none of them: `lifecyclecli`'s `run` seeds inline in three lines and never touches reed, tmux, or a detached driver.
  Generalizing on a sample of one would be inventing an abstraction, not extracting one.
- Rejected: generalizing `start` with injected bootstrap/attach hooks (the hook set would have exactly one filler and would encode loom's seven-step ordering as the generic contract); moving the bootstrap into recipe rows (it must run before the Shed reads the status file it seeds, so it cannot be a producer).

### existing-subtrees-keep-their-surface

- Decision: `lyx loom run|step|status|pause` and `lyx lifecycle run|status` keep their present names, flags, `Short`/`Long` text, exit codes, and envelopes.
  They become arming over `shedcli` with no user-visible change.
  `lyx shed run --recipe loom` and `lyx loom run` are the same body armed the same way.
- Rationale: the existing behavioural suites (`loomcli`'s `smoke_test.go`, `step_test.go`, `parity_test.go`, `status_test.go`, `wiring_test.go`; `lifecyclecli`'s `run_test.go`, `lifecycle_integration_test.go`) are the real proof the extraction is behaviour-preserving, and they only prove it if they keep passing unchanged.
- Rejected: removing the per-module subtrees in favour of `lyx shed --recipe` alone (breaks every documented invocation, the `ly-drive` skill, `.vscode/tasks.json`-generated launch chains, and `start`'s own `exec.Command(exe, "loom", "run")` driver spawn); registering the per-module verbs as aliases owned by `shedcli` (the CLI/Cobra Invariant allows an alias with no seam of its own, but these verbs each need their own module's `PersistentPreRunE` resolution, which an alias cannot supply generically).

### shedpaths-and-newshed-hoist-to-shedbuild

- Decision: `shedbuild` gains `ShedPaths` (the same five fields: `StatusPath`, `LockPath`, `StatusLockPath`, `MaxBounces`, `CommitStatus`) and `NewShed(recipe []byte, env shedrecipe.Env, paths ShedPaths) (*shedengine.Shed, error)`, which parses, builds, and assembles.
  `loomrecipe.New` and `lifecyclerecipe.New` keep their signatures' shape but take `shedbuild.ShedPaths`, run their own package-specific guards first, and then delegate.
  `loomrecipe.ShedPaths` and `lifecyclerecipe.ShedPaths` are removed.
- Rationale: the two `New` bodies are the same eight lines and the two structs are field-identical including their doc comments.
  `shedbuild` already owns `Parse` and `Build` and already imports `shedengine` and `shedrecipe`, so the assembly step is adjacent to what it owns rather than a new responsibility.
- Rejected: leaving the duplication and generalizing only the CLI verbs (the two copies would then diverge under a `shedengine.Shed` field addition, which is precisely the silent-divergence class `loomrecipe.New`'s own coherence guard exists to catch); putting `NewShed` in `shedcli` (would make a cobra-importing package the assembler, and would leave any non-CLI consumer without one).

### loomrecipe-keeps-its-coherence-guard

- Decision: `loomrecipe.New`'s `env.StatusPath != paths.StatusPath` and `env.StatusLockPath != paths.StatusLockPath` checks stay in `loomrecipe`, ahead of the delegation, and are not hoisted into `shedbuild.NewShed`.
- Rationale: the guard exists because `loomPreflightEntry` reads `Env.StatusPath` while `Shed` reads `paths.StatusPath`, so the two copies can disagree silently and break resume.
  That coupling is loom's — `lifecyclerecipe` has no `LoomPreflight` row and deliberately carries no such check.
  Hoisting it would impose loom's constraint on every future recipe, including ones with no status-reading producer at all.
- Rejected: hoisting the guard into `NewShed` unconditionally; dropping it (it is the only place in the tree where both copies are visible at once).

### optional-hooks-for-module-specific-work

- Decision: `shedcli.Hooks` is a struct of nil-by-default function fields, each skipped when nil:
  - `PreRun func(ctx) (extraEnvelope map[string]any, err error)` — runs before `Shed.Run`; a non-nil error is reported on the error envelope and the run does not start.
  - `PostRun func(ctx, result shedengine.RunResult, runErr error) map[string]any` — runs unconditionally after `Shed.Run` returns, including when `runErr` is non-nil, and before the error envelope is written; its returned map is merged into the success envelope.
  - `PreStep func(ctx) (kind string, err error)` — runs before `Shed.Step`; a non-nil error is reported with the returned refusal kind on the envelope's `kind` field.
  - `InterruptPolicyFor func(row string) string` — supplies `step`'s `next_interrupt_policy`; the empty string when nil.
  - `StatusExtras func(st shedengine.Status) (map[string]any, error)` — supplies the product-specific half of `status`'s envelope, merged onto the generic core; a non-nil error is reported on the error envelope.
  `loomcli` fills all five.
  `lifecyclecli` fills `PreRun` — its status decode, its `StateDone` refusal naming the per-slug directory, its unrecognised-state refusal, and its seed-when-absent `state.UpdateJSON` — plus `PostRun` for its `abandonedSession` field and `StatusExtras` for its `found`/`status_path`/`history` keys, leaving `PreStep` and `InterruptPolicyFor` nil.
  The spec carries a Shed *constructor*, `BuildShed func() (*shedengine.Shed, error)`, never a pre-built `*shedengine.Shed`, and the generic `run` and `step` bodies call it only after `PreRun`/`PreStep` has returned.
  This is not a stylistic preference: `loomcli`'s pre-flight assigns `c.env.Landing = landingDeps(…)` on the way through, immediately before `loomrecipe.New`, so a Shed built at arming time would carry a nil `Landing` and the landing rows would fail deep in the run.
- Rationale: the loom-specific work is real and ordered, and the ordering is load-bearing in ways the existing comments already record — the entry observation must be taken before the reed substrate comes up, and `detectAndFileAnomalies` must run *above* `run`'s early error return because the hard-error arm would otherwise drop a whole failure class and lose the in-memory crash observation permanently.
  `PostRun` receiving `runErr` and running unconditionally is what preserves that exact property through the extraction.
- Rejected: a pre-built Shed on the spec (nil `Landing`, as above); a single combined `Hooks.Around` closure (hides the unconditional-`PostRun` ordering, which is the one property most easily lost); moving the loom extras into recipe rows (friction reflection and anomaly filing both run after `Shed.Run` has already returned and after `RunDone` merged and published, so no producer row can host them — `shedengine.Run` returns immediately on `RunBlocked` without calling a further producer, so a row could structurally never cover the blocked half); `loomcli` keeping its own `RunE` wrapper around a thin shared core (puts the envelope assembly back in two places).

### flags-and-args-belong-to-the-arming-module

- Decision: `shedcli.Verbs(spec)` returns the four `*cobra.Command` values with only the flags the generic bodies themselves read — `status`'s `--watch` and its `--interval`, the latter read by the watch body's own sleep and defaulting to one second.
  A module that needs more decorates the returned command itself, before adding it to its own subtree: `loomcli` registers `--parent` on the `step` command it got back, and `lifecyclecli` sets `Args: cobra.ExactArgs(1)` on `run` and `status`.
  Hooks reach those values by closure over the module's own receiver, never through a `shedcli` parameter — the same way `loomcli`'s `parentFlag` and `lifecyclecli`'s `c.slug` are already captured today.
  For the `lyx shed` subtree, each recipe's table entry declares its own positional-arg contract, which `shed`'s `PersistentPreRunE` applies after resolving `--recipe`: `loom` takes no positional argument, `lifecycle` takes exactly one slug.
  `--recipe` itself is a persistent flag on the `shed` parent, so it is never confused with a recipe's own arguments.
- Rationale: the flag and argument surfaces are genuinely per-module and there is no generic shape to give them — `--parent` writes a fabric provenance record that only loom's bootstrap has, and the slug is what `lifecyclecli`'s `wire` needs before it can build `Env` or `ShedPaths` at all, so it must be read in the pre-run, ahead of any arming.
  Decorating a returned command is ordinary cobra and keeps `shedcli` free of a flag-registration DSL it would otherwise need.
  It also keeps each module's existing `PersistentPreRunE` as the sole reader of its own arguments, which is what the `generic-package-resolves-nothing` Decision already requires.
- Rejected: a flag/arg declaration list on the arming spec (a DSL reimplementing what cobra already does, and one `shedcli` would have to keep in sync with cobra's own validators); passing parsed values into hooks as a `map[string]any` (loses typing and hides which verb reads what); letting `shedcli` own `--parent` and `--slug` generically (puts two product-specific flags on every recipe's verbs, including recipes that have neither).
- Consequence for the parity tests: `lyx shed run --recipe lifecycle <slug>` is the positional form the parity test compares against `lyx lifecycle run <slug>`; a missing or extra positional argument must be refused identically by both, and that refusal is part of what the parity test asserts.

### busy-refusal-comes-from-the-arming-spec

- Decision: the arming spec carries the told `ErrShedBusy` treatment — the message text and, for `step`, the refusal kind — so the generic bodies keep the three existing behaviours distinguishable without branching on which module armed them.
  `loomcli`'s `run` keeps reporting it as an ordinary error envelope, `lifecyclecli`'s `run` keeps its lock-path-naming message, and `step` keeps mapping it to `kind: busy` with its `lyx loom pause` remedy text.
- Rationale: `ErrShedBusy` is one sentinel with three shipped user-facing treatments, and the wording is what an operator acts on — `lifecyclecli`'s message names the lock path precisely so the refusal is legible rather than a raw lock error.
  Telling the message rather than deriving it is the same told-not-derived discipline the rest of the arming follows.
- Rejected: collapsing the three to one wording (a user-visible regression in two of the three, and the `existing-subtrees-keep-their-surface` Decision bars it); a fourth hook for the busy case alone (a hook is for work, and this is a string); branching inside `shedcli` on the recipe name (makes the generic body know its callers, which is exactly what this task removes).

### envelope-contracts-move-with-the-verbs

- Decision: `step`'s envelope keeps loom's exact ten keys — `producer`, `outcome`, `output`, `next`, `state`, `reason`, `continue`, `history_length`, `next_interrupt_policy`, `status_file` — with `continue` still derived as `res.State == shedengine.StateRunning` and `next_interrupt_policy` supplied by the `InterruptPolicyFor` hook.
  The five refusal kinds (`busy`, `unseeded`, `ownership`, `bootstrap`, `producer`) move to `shedcli` as the closed vocabulary, with the closure test moving with them.
  `run`'s envelope carries `outcome`, `halted_producer`, `reason`, `history_length`, plus the `PostRun` extras map.
- Rationale: the ten-key set is closed by an existing test and is the contract `ly-drive` reads; a key added or renamed in the move is a silent break of the skill.
  Deriving `continue` in Go, not in the skill, is the stated reason a thin supervisor never carries its own copy of the `State` vocabulary — that reason is now stronger, not weaker, since the skill must serve several recipes.
- Rejected: adding a `recipe` field to every envelope (widens a closed, tested key set for information the caller already passed in on the flag); letting each module keep its own envelope builder (re-creates the drift the extraction exists to remove).
- `status`'s envelope is built in two halves.
  The generic core is the four keys both shipped verbs already share — `current_producer`, `state`, `error`, `activity` — and everything else comes from the `StatusExtras` hook: `loomcli` adds `pause_requested`, `history_length`, `slug` and `parent` (from a `json.Unmarshal` of `st.Product` into `loomengine.Status`) and `interrupt_policy` (a plain `loomshed.InterruptPolicyFor(st.CurrentProducer)` map read); `lifecyclecli` adds `found`, `status_path` and `history`.
  Both verbs therefore keep their present key sets exactly.
  The absent-file disposition is a told field on the arming spec, not a hook and not a generic default, because the two shipped verbs disagree on it deliberately: loom refuses with `no status file at <path>; run "lyx loom start" first to bootstrap this task`, since only `start` may seed; lifecycle reports `found: false` with `status_path` on the success envelope, since nothing failed and a slug simply has not been run on this machine.
  Neither loom extra needs a config load or a built Shed, which is what keeps `status` on the lightweight path (see `per-verb-arming-preserves-lightweight-wiring`).
- Consequence to state plainly: `lyx lifecycle run`'s envelope gains `history_length`, which it does not carry today.
  This is a deliberate widening — additive, and the field is free from `result.History` — not an accident of the move.
  `lifecyclecli`'s own doc comment explaining why the run envelope carries neither a mutations array nor a `partial` bool stays true and must be preserved.

### pause-and-watch-generalize

- Decision: `pause` and `status --watch` join the generic verb set.
  `pause` is a `state.UpdateJSON` setting `PauseRequested` over a told status path, identical for any Shed.
  `status --watch`'s rendered line takes its literal prefix from a told label on the arming spec (`loom` for the loom recipe), keeping `loom <state> | now <now> | last <last> | wait <wait>` byte-identical for loom.
- Rationale: `shedengine` honours `PauseRequested` at producer granularity for every Shed, so a Shed that can be run and stepped but not paused is an arbitrary gap.
  `lifecyclecli` gaining `lyx lifecycle pause` is the intended consequence, not a side effect to suppress.
- Rejected: keeping either verb loom-only (would leave `shedcli` owning two of four verbs and `loomcli` still hosting verb bodies, defeating the invariant below); hardcoding the `loom` prefix in the generic renderer.
- Note for the plan: `loomcli`'s `verbUsesLightweightWiring` set (`status`, `pause`, `validate-discussion`, `validate-plan`) must keep working.
  `status` and `pause` still need only the two status-file paths and must not acquire a dependency on the full `wire()` through this change — `shedcli`'s `status` and `pause` bodies must therefore take their paths told and never require a built `*shedengine.Shed`.

### inner-run-engine-goes-product-neutral

- Decision: the lifecycle recipe's middle producer stops naming loom.
  The registry key `LoomRun` becomes a product-neutral name, `shedrecipe.Env.LoomRun` and `lifecycleshed.LoomRunDeps`/`NewLoomRun`/`loomRunProducer` and the entry's `loomRunEntry`/`defaultLoomRun*` identifiers follow, and the producer's `logger.Info` lines and stuck reasons stop saying "loom session".
  What does *not* change: the `Loom-Run` row name in `contracts/recipes/lifecycle-recipe.yaml`, `lifecyclerecipe.NameLoomRun`'s string value `"Loom-Run"`, the recipe's `entry`/`terminals`, the `poll_interval_s`/`poll_attempts` Config keys and their defaults, and the producer's own logic.
  No second lifecycle recipe is written and no Hardener artefact is created; the design doc gains a note recording Hardener as the future consumer of this seam.
- Rationale: the producer is already generic in substance — its poll loop and its verdict table branch on `shedengine.Status.State`, which is the Shed contract, not loom's, and the two genuinely product-specific parts (`Spawn`, `ResolveStatus`) are already injected seams on a Deps struct.
  Only the names claim otherwise.
  Neutralizing them is the same axis as the rest of this task — getting loom out of the layers above the engine — and is cheap now and dearer later, once a second caller exists.
  The split between what is renamed and what is not follows directly from what is durable: the status file persists `CurrentProducer`, which is the *row* name, so renaming a row breaks resume for an in-flight run (the recipe YAML's own header and `names.go`'s own comment both say so); an `engine:` value is resolved by `shedbuild.Build` at construction time and is persisted nowhere, so renaming it is safe.
- Rejected: adding a second lifecycle recipe or a parameterized inner-product Config key now (validates the seam against an empty consumer — the trap this project explicitly avoided by holding `shedrecipe`'s own generalization until `lifecyclerecipe` existed as a real second consumer; `manifest/designs/hardener.md` also carries a standing DRAFT banner saying not to implement from it yet, so any recipe written now would be guesswork to be rewritten); renaming the `Loom-Run` row for symmetry (breaks resume, for cosmetics); leaving the whole thing alone (throws away a mapping already made, and leaves the next consumer renaming a registry key under an in-flight run instead of ahead of one).
- Note for the plan: `lifecyclerecipe.RecipeEngines()` derives the engine set from the parsed recipe rather than a literal, so it needs no edit, but `shedrecipe`'s cross-consumer coverage guard and `lifecyclerecipe`'s own coverage guard both assert against the renamed key and must move with it in the same commit.

### per-verb-arming-preserves-lightweight-wiring

- Decision: the `lyx shed` table maps a recipe name to an arming function that takes the *verb* as a parameter and returns that verb's spec, rather than one verb-blind spec per recipe.
  `loom`'s arming calls `wireLightweight` for `status` and `pause` and the full `wire` for `run` and `step`, using `loomcli.verbUsesLightweightWiring` itself as the single authority — exported if the shed table needs to read it, so the predicate is never copied.
  A lightweight spec fills the told paths, the status label, the absent-file disposition and `StatusExtras`, and leaves `BuildShed` nil; the generic `status` and `pause` bodies never call `BuildShed`, so a nil one is legal for exactly those two verbs and an error for `run`/`step`.
- Rationale: `verbUsesLightweightWiring` exists because `status` and `pause` read only the two status-file paths, and forcing them through `wire()` would make an unrelated module's broken config break loom's own read-only status verbs — the hazard `wireLightweight`'s own history records.
  A verb-blind arming keyed on recipe name alone would reintroduce that hazard on `lyx shed status --recipe loom` and make it diverge from `lyx loom status`, which the `existing-subtrees-keep-their-surface` Decision forbids.
- Rejected: one spec per recipe with the heavy wiring always applied (reintroduces the broken-config hazard); `shedcli` deciding which verbs are lightweight (it would then hold a policy that is `loomcli`'s, and `lifecyclecli` has no such split at all); duplicating the verb-name predicate in the shed table (two authorities for one set, guaranteed to drift).

### generic-package-resolves-nothing

- Decision: `shedcli` derives no path, imports no resolver, and never calls `os.Getwd` or `lyxcwd`.
  Every path reaches it told, through the arming spec.
  Resolution stays where it is: `loomcli`'s `resolvePersistentPreRun` (cwd → `lyxcwd.Resolve`, hub-only, then `wire`/`wireLightweight`), and `lifecyclecli`'s (cwd → `lyxcwd.Resolve` → `fabricengine.PrimeName` → non-prime refusal → slug from args → `wire`).
  `lyx shed`'s own `PersistentPreRunE` reads `--recipe`, looks the name up, and delegates to that recipe's arming function, so each recipe keeps its own resolution and its own refusals.
- Rationale: required by the Cwd Resolution Invariant and the Told-Geometry Invariant.
  It is also the only way `lyx shed run --recipe lifecycle` can keep the Lifecycle Bookend Invariant's non-prime refusal, which is inseparable from lifecycle's own arming.
- Rejected: `shedcli` resolving cwd once for all recipes (would centralise a resolution that legitimately differs per recipe and would silently drop lifecycle's prime refusal).

### ly-drive-drives-any-recipe

- Decision: generalize the existing `ly-drive` skill rather than adding a second one.
  It takes a recipe name as its argument, defaults to `loom`, and invokes `lyx shed step --recipe <name>` in the background-to-file pattern it already mandates.
  Its recipe-agnostic content — the 40-step cap, the `continue` branch, the five error kinds, the one-retry rule for `producer`, the interrupted-invocation branch on `current_producer`/`history_length`/`interrupt_policy`, the never-clean-up rule — stays as written.
  Its loom-specific sections — the reed-strand `$TMUX_PANE` self-check, the friction directory at `.lyx/loom/friction/`, the `lyx selfreport create` gate, and the "loom's list is seventeen rows" arithmetic behind the cap — become explicitly gated on the `loom` recipe.
- Rationale: the skill already claims to carry no phase knowledge and to branch only on policy words and envelope fields; that claim is true of its loop and false only of its preamble.
  Splitting into two skills would duplicate the loop, which is the part worth having once.
- Rejected: a separate `shed-drive` skill (duplicates the loop); leaving `ly-drive` loom-only (the design doc pins the skill's end-state role as looping the generic `step`); gating the loom-specific sections on new envelope fields instead of the recipe name (would widen the closed ten-key envelope set, which the `envelope-contracts-move-with-the-verbs` Decision bars).

### new-shed-verb-set-invariant

- Decision: add a **Shed Verb-Set Invariant** to `CONSTRAINTS.md` in the same commit, stating: `internal/shedcli` owns the generic `run`/`step`/`status`/`pause` verb bodies and no `<module>cli` reimplements one; `shedcli` derives no path and imports no resolver (no `lyxcwd`, no `os.Getwd`, no `git rev-parse`); every name in the `lyx shed` recipe table is armed by exactly one arming function; and the `step` refusal-kind vocabulary stays closed at its five values.
- Rationale: the CLI/Cobra Invariant's package-naming clause already needs an entry for `shedcli` (`<module>cli` imports `<module>engine` — `shedcli` → `internal/shedengine`), and the extraction's whole value evaporates if a later module quietly grows its own `run` body again.
  CONSTRAINTS.md's own preamble requires a new cross-cutting invariant to land in the same commit.
- Rejected: relying on review discipline alone (two of the three clauses have a static shape a scan can see, so there is no reason to leave them unenforced).

## Technical context

**The generic layer, already done and not to be touched.**
`internal/shedengine` walks one flat producer list and imports only stdlib, `state`, and `lock`; `StatusPath`/`LockPath`/`StatusLockPath` are caller-supplied.
`internal/shedrecipe` is the engine registry — one `map[string]Constructor` at seventeen keys, reached only through `Lookup`/`Names`, no `init()` self-registration and no runtime `Register`.
`internal/shedbuild` is the sole parser of the recipe file format (`Parse` on bytes, `Load` on a told path, `Build` against a `shedrecipe.Env`) and declares no on-disk location for recipe files.
`shedrecipe.Env` carries roots and run-wide values only, never anything per-row; `shedbuild.Row` carries the per-row `Config` map each registry entry validates for itself.

**The two recipes and their near-duplicate packages.**
`contracts/recipes/recipes.go` embeds `loom-recipe.yaml` as `recipes.LoomRecipe` and `lifecycle-recipe.yaml` as `recipes.LifecycleRecipe`; `//go:embed` reaches only at-or-below its own directory, so any third recipe's byte var goes in that same file.
`internal/loomrecipe/loomrecipe.go` and `internal/lifecyclerecipe/lifecyclerecipe.go` each declare a five-field `ShedPaths` and a `New(env, paths)` that parses the embedded bytes, builds against `env`, and returns `&shedengine.Shed{…}`.
`loomrecipe.New` additionally runs the two coherence checks described above, as its first act, ahead of `Parse`.
Neither calls `shedbuild.Check`: `Check` is authoring-time only, because a resumed run legitimately starts mid-graph and reachability-from-entry is the wrong production question.
Both packages keep substantial product-specific tests (`recipe_test.go`, `shape_test.go`, `sequence_test.go`, `resume_test.go`, `coverage_guard_test.go`, `seam_enforcement_test.go`, `interruptpolicy_meta_test.go`, `approveseam_test.go`, `overlay_seam_guard_test.go`) that stay where they are.

**`internal/loomcli/step.go`** — the closest thing to the generic body already.
It declares the five refusal kinds and `stepKinds`, maps a `bootstrapStage` onto them via `stepKindForBootstrapStage`, builds the ten-key envelope in `stepEnvelope(res, nextPolicy, statusFile)`, and in `RunE`: `MkdirAll(filepath.Dir(LockPath))` (part of the probe, not incidental — the run lock lives in the ephemeral tree and `internal/lock` opens `O_CREATE` without creating a parent), a non-blocking run-lock probe released immediately when free, `seedAndCommitBootstrap`, the bootstrap lock around `ensureStatusStrand` released *before* the producer call, `buildLoomShed`, `shed.Step`, `recordStepHandoff`, then the envelope.
Everything from `MkdirAll` through `ensureStatusStrand` is loom's `PreStep`; `shed.Step` onward is the generic body.
The comment recording that the early probe is an optimisation and `shedengine.Step`'s own acquisition is the authority must survive the move.

**`internal/loomcli/run.go`** — the hook ordering to preserve exactly.
Pre-flight: status-file existence refusal naming `lyx loom start`, `loomengine.VerifySeedOwnership`, `observeEntry` (guarded on the `selfreport` knob so a disabled run pays for no lock probe and no extra status decode), `c.reed.Up()`, `fabricengine.Open`/`CurrentBranch`/`OriginURL`/`ReadOrigin`/`resolveLandingParent`, `c.env.Landing = landingDeps(…)`, `friction.EnsureDir`.
Then `loomrecipe.New` and `shed.Run`.
Then, unconditionally and deliberately above the early error return, `detectAndFileAnomalies(…)`.
Then the error envelope, or `shouldReflectFriction(frictionDir, result.Outcome)` → `c.reflectFriction()` and the success envelope with its `friction` field.
Note `run` treats `ErrShedBusy` as an ordinary error envelope while `lifecyclecli`'s `run` special-cases it with a lock-path-naming message and `step` maps it to `kind: busy` — three different treatments of one sentinel, resolved by the `busy-refusal-comes-from-the-arming-spec` Decision.

**`internal/lifecyclecli`** — the second consumer, and the shape that proves the abstraction.
`run.go` reads the status with `state.ReadJSONStrict[shedengine.Status]`, refuses `StateDone` naming the per-slug directory to delete, resumes silently on `StateRunning`/`StateBlocked`/`StateFailed`/`StatePaused`, refuses an unrecognised state, and seeds inline via `state.UpdateJSON` with an idempotent mutate closure when the file is absent.
That seed-when-absent behaviour is lifecycle's `PreRun` and must not leak into the generic body — loom refuses in exactly the situation lifecycle seeds, because only `lyx loom start` may seed loom's status file (it owns the commit-before-precondition ordering).
`status.go` is already recipe-agnostic apart from its `lifecyclecli:` error prefix and is the natural template for the generic `status`.
`paths.go` anchors everything on prime's `AnchorPath()` under `lyxdirs.DotLyxDirName` — ephemeral, never durable, per the Durable-vs-Ephemeral State Invariant — and its path-derivation tests are one of the two mechanical proxies for the Lifecycle Bookend Invariant.

**`internal/lifecycleshed/loomrun.go` — the inner-run producer to neutralize.**
`loomRunProducer.Call` resolves the status path through `deps.ResolveStatus` (evaluated on `Call`, never at wiring time, because the task worktree does not exist until `WorktreeCreate` has run), logs the spawn, calls `deps.Spawn` and blocks on it, logs the completed wait — both log lines required by the Live-Substrate Spawn Observability invariant, since this producer waits for its child rather than detaching — and then polls `deps.ReadStatus` up to `pollAttempts` times at `pollInterval`.
The verdict table is exhaustive over `shedengine.Status.State`: `StateDone` → `Done`; `StateBlocked`/`StatePaused`/`StateFailed` → `Stuck` naming the state, `Error` and `CurrentProducer`; `StateRunning` consumes an attempt; anything else is a hard error.
A `ResolveStatus` or `ReadStatus` error is a hard error rather than a verdict; a `Spawn` error and `found == false` are both `Stuck`.
`Now`/`Sleep` are nil-resolved to `time.Now`/`time.Sleep` once in the constructor so only a test substitutes them.
None of that logic changes — the neutralization is names and strings only.
The corresponding registry entry is `loomRunEntry` in `internal/shedrecipe/entries_lifecycle.go`, which validates `Env.Slug`, `Env.ScratchDir` and `Env.LoomRun.Spawn`/`ResolveStatus`/`ReadStatus` (and deliberately not `Now`/`Sleep`, whose nil values are legitimate), and whose `requireSeam`/`requireNonEmpty`/`requireAbsRoot` entry-name strings and error texts all carry the old name.
The recipe row's `on_stuck` is empty and load-bearing: a stuck verdict there escalates to a human with the task worktree fully intact, which is what keeps the destructive `Worktree-Teardown` row unreachable from any failure path.
That must survive untouched.

**Registration and the help tree.**
`cmd/lyx/main.go` assembles every module's `Command()` under one root (`loomcli.Command()`, `lifecyclecli.Command()`, and `loomcli.StartAliasCommand()` as a sibling).
`cmd/lyx/helptree_test.go` asserts the root help names every subtree, `cmd/lyx/drift_test.go` fails CI on any command with a blank `Short`, `cmd/lyx/longlist_test.go` keeps `--help` prose from drifting from the live tree, and `cmd/lyx/jsonhelp_test.go` asserts the `--json` help schema at several levels.
A new `shedcli.Command()` must be added to the root and will be picked up by all four.

**`internal/loomshed/interruptpolicy.go`** — `InterruptPolicies` maps loom's seventeen row names to `reinvoke`/`handback` (every row `reinvoke` except `NameWebster`), and `InterruptPolicyFor` returns the empty string for an unknown name, which is the caller's "no entry" signal and never a third policy word.
This table is loom's and stays in `loomshed`; it reaches the generic `step` only through the `InterruptPolicyFor` hook.
A recipe with no policy table yields an empty `next_interrupt_policy`, which `ly-drive`'s interrupted-invocation branch must tolerate — today it branches on `reinvoke` vs `handback` with no third arm.
Treat an empty policy as `handback` in the skill (hand back rather than re-invoke), because the conservative arm is the one that never restarts in-flight work.

## Constraints

From `CONSTRAINTS.md`, the ones this task must satisfy:

- **Cwd Resolution Invariant** — `internal/lyxcwd` owns cwd resolution alone; a module's own durable subdirectory is its own constant joined onto `AnchorPath()`, never a `lyxcwd` call.
  `shedcli` must import no resolver and derive no path.
- **Told-Geometry Invariant** — an engine is handed the absolute paths it operates on and derives none of its own, with no direct `internal/lyxcwd` import.
  The bound-packages list already names `shedengine`, `shedrecipe`, `shedbuild`, `loomrecipe`, `lifecycleshed`, `lifecyclerecipe`; the list must be reviewed for whether `shedcli` belongs on it (it is a CLI, not an engine, but its no-derived-paths obligation is the same — state the decision explicitly rather than leaving it implied).
- **Shed Producer-Seam Invariant** — `shedengine` imports only stdlib, `state`, `lock`; `StatusPath`/`LockPath`/`StatusLockPath` stay caller-supplied.
  Nothing in this task may add an import to `shedengine`.
- **Shed Recipe Registry Invariant** — one `map[string]Constructor` reached only through `Lookup`/`Names`, no `init()` self-registration, no runtime `Register`.
  The new `lyx shed` recipe table is a *different* table (recipe names, not engine names) and must not be confused with, or merged into, the engine registry — and it must not reintroduce `init()`-style self-registration by the back door.
- **Recipe-Format Sole-Parser Invariant** — `internal/shedbuild` is the sole parser of the recipe file format and declares no on-disk location for recipe files.
  `--recipe` takes a name, never a path.
- **CLI / Cobra Invariant** — every module exposes `Command()` and `RunCLI(out, args) int`, most also `RunCLIIn(cwd, out, args) int`; non-empty `Short` on every command; errors are JSON via `internal/output`, one object per line; every `RunE` checks `clihelp.ShouldAbort` first; `<module>cli` imports `<module>engine` and the engine never imports cli/cobra.
  The invariant's "twelve of thirteen" count needs updating for `shedcli`; its deviations list does not, since `shedcli` → `internal/shedengine` conforms to the `<module>cli` naming rule.
  The interactive-handoff exception list *does* change: generalizing `status --watch` creates two new never-exiting commands, `lyx shed status --watch` and `lyx lifecycle status --watch`, and both must be named alongside the existing `lyx loom status --watch` entry.
- **Lifecycle Bookend Invariant** — the lifecycle Shed is driven from the hub's prime worktree; `lifecycleshed`'s seam-enforcement scan bars a direct resolver import and `lifecyclecli`'s path-derivation tests pin the status and lock paths to prime's anchor.
  The non-prime refusal must survive reaching lifecycle through `lyx shed --recipe lifecycle` as well as through `lyx lifecycle`.
- **Durable-vs-Ephemeral State Invariant** — every never-tracked file lives under `.lyx`; loom's status file is durable under `_lyx` and fabric-synced, lifecycle's whole tree is ephemeral under `.lyx`.
  The generic verbs must stay agnostic about which, since both arrive told.
- **Config Strictness Invariant** and **Lyxdirs Single-Declarer Invariant** — no new config surface and no new `.lyx`/`_lyx` literal introduced by this task.
- **Documentation Lifecycle** — the module doc, `docs/overview.md`, `CONSTRAINTS.md` and `manifest/roadmap.md` all move in the same commit (see Scope).
- **Never Force-Add Invariant**, **Test Tier Purity Invariant**, **Hermetic Git Test Environment Invariant** — unchanged obligations that the new tests must respect.

Build prerequisite: `CGO_ENABLED=1` and a C compiler on `PATH` (quarry's tree-sitter grammars), already the default on a developer machine with a compiler installed.

## Testing

**`internal/shedcli` — the TDD candidate, and the only genuinely new test surface.**
Drive every verb body against a fake `*shedengine.Shed` (or a recipe of `Stub` rows, which the registry already provides) so no LLM, no tmux, and no git is involved:

- `run`: success, `ErrShedBusy`, producer hard error, each hook nil, each hook filled, and specifically that `PostRun` runs on the error path *before* the error envelope is written.
  That last one is the property most easily lost in the extraction and deserves a named test.
- `step`: the ten-key envelope's key set asserted closed; `continue` true only for `StateRunning`; `next_interrupt_policy` empty when the hook is nil and threaded when filled; the five refusal kinds asserted closed, mirroring the existing `stepKinds` test; `PreStep`'s returned kind reaching the envelope's `kind` field.
- `status`: both told absent-file dispositions driven — the refusal form (loom's) and the `found: false` success form (lifecycle's) — plus `StatusExtras` merging onto the core and a `StatusExtras` error reaching the error envelope; the `--watch` tail's change-only printing driven through a finite `polls` count with no wall-clock wait, exactly as `printStatusLinesOnChange` and `awaitRunLock` are driven today; the told label appearing in the rendered line.
- `pause`: sets `PauseRequested`; refuses with the told message when the status file is absent.
- A seam-enforcement scan asserting `shedcli` imports no resolver (`lyxcwd`, `os.Getwd`, `git rev-parse`), in the style of `lifecycleshed`'s and `loomrecipe`'s existing scans.

**Parity — the proof the extraction preserved behaviour.**
Assert that `lyx loom step` and `lyx shed step --recipe loom` produce byte-identical envelopes from the same fixture, and likewise for `run`, `status`, and `pause`, and for `lyx lifecycle run` against `lyx shed run --recipe lifecycle`.
`internal/loomcli/parity_test.go` already exists and is the precedent for the shape.

**Regression — the suites that must pass unchanged.**
`internal/loomcli`'s `step_test.go`, `status_test.go`, `smoke_test.go`, `wiring_test.go`, `stephandoff_test.go`, `start_watchdog_test.go`, `friction_test.go`, `selfreport_test.go`; `internal/lifecyclecli`'s `run_test.go`, `status`/`paths`/`refusal`/`wire` tests and `lifecycle_integration_test.go`; `internal/loomrecipe`'s and `internal/lifecyclerecipe`'s full suites.
A change to any of these assertions is a signal the extraction changed behaviour and must be justified in the plan, not silently absorbed — the surface changes agreed here are: `lifecyclecli`'s run envelope gains `history_length`; `lifecyclecli` gains a `pause` verb; and `lyx lifecycle status` gains `--watch` and `--interval`, which it has neither of today.
Each is additive, and no existing key, flag or refusal is removed or reworded by this task.

**Inner-run neutralization** — a pure rename, so its proof is that nothing moved: `internal/lifecycleshed`'s `loomrun_test.go` and `internal/shedrecipe`'s `entries_lifecycle_test.go` pass with identifiers renamed and no assertion weakened, and both coverage guards (`shedrecipe`'s cross-consumer one and `lifecyclerecipe`'s own) assert the new engine key with the registry still at seventeen.
Add one assertion that `lifecyclerecipe.NameLoomRun`'s *value* is still `"Loom-Run"` and that the recipe's row names are unchanged, so a later symmetry-minded rename of the durable identity fails loudly rather than silently breaking resume.

**`shedbuild`** — a test that `NewShed` returns the same `*shedengine.Shed` fields the two old `New` bodies did, and that an empty-producer recipe still errors.

**CLI tree** — `cmd/lyx`'s `helptree_test.go`, `drift_test.go`, `longlist_test.go` and `jsonhelp_test.go` all cover the new subtree automatically once it is registered; confirm each passes rather than assuming it.

**Markdown Link Integrity** — the design-doc and `docs/overview.md` edits add links; the existing link checker must stay green.

## Q&A log

- **Q:** Should the generalization be a shared library, a `lyx shed` subtree, or both? **A:** [auto-pick] Both — a new `internal/shedcli` owning the generic verb bodies, consumed by `loomcli`/`lifecyclecli` and registered as its own `lyx shed` subtree. **Why:** the duplication is already two-deep with a third consumer drafted; a library alone leaves the verb set unreachable for a new recipe, a subtree alone ships a third copy instead of removing two.
- **Q:** How is the generic verb set armed with its recipe? **A:** [auto-pick] A named-recipe table mapping `--recipe <name>` to an arming function supplying recipe bytes, `Env`, paths, hooks, and label. **Why:** a recipe file cannot carry `Env`'s injected seams, and a path-taking flag would imply a runtime on-disk recipe location the Recipe-Format Sole-Parser Invariant forbids.
- **Q:** Does `start` generalize? **A:** [auto-pick] No — it stays loom-specific. **Why:** there is no `Shed.Start`; every one of its seven steps sits above the engine and the second consumer has an analogue for none of them.
- **Q:** What happens to the existing `lyx loom` and `lyx lifecycle` verbs? **A:** [auto-pick] Kept with their surface unchanged, reimplemented as arming over `shedcli`. **Why:** their existing behavioural suites are the proof the extraction is behaviour-preserving, and they only prove it if they pass unchanged.
- **Q:** lifecycle should be able to wrap something other than loom — Hardener eventually. How much of that belongs here? **A:** The seam only: neutralize the inner-run engine's names and strings, leave the recipe's durable identities and any Hardener artefact alone. **Why:** neutralizing the engine is the same axis as the rest of the task and is cheap now; a second recipe would validate against an empty consumer, which is the trap this project avoided by waiting for `lifecyclerecipe` to exist before generalizing `shedrecipe`, and `hardener.md` is still an explicit DRAFT.
- **Q:** Where does the deduplicated `ShedPaths`/`New` live? **A:** [auto-pick] Hoisted into `shedbuild` as `ShedPaths` + `NewShed`; the two recipe packages shrink to delegation plus their own guards. **Why:** `shedbuild` already owns `Parse` and `Build` and already imports both `shedengine` and `shedrecipe`.
- **Q:** Does `loomrecipe`'s Env/paths coherence guard hoist too? **A:** [auto-pick] No — it stays in `loomrecipe`. **Why:** it exists for `loomPreflightEntry`'s `Env.StatusPath` read, which is loom's coupling; `lifecyclerecipe` deliberately has no such check.
- **Q:** How do loom's run/step extras stay out of the generic body? **A:** [auto-pick] Four nil-by-default hooks — `PreRun`, `PostRun`, `PreStep`, `InterruptPolicyFor`. **Why:** `PostRun` taking `runErr` and running unconditionally is what preserves `detectAndFileAnomalies`'s deliberate placement above `run`'s early error return.
- **Q:** Where do per-module flags and positional arguments live, given `shedcli` owns the verb bodies? **A:** The arming module decorates the returned commands with its own `--parent`/`Args` and hooks capture them by closure; each `lyx shed` recipe entry declares its own positional-arg contract. **Why:** there is no generic shape for a flag that writes a fabric provenance record or an argument `wire` needs before `Env` exists, and a declaration DSL would reimplement cobra.
- **Q:** How is `ErrShedBusy`'s three-way divergence resolved? **A:** The arming spec carries the told message and, for `step`, the refusal kind. **Why:** the wording is what an operator acts on and two of the three would regress if collapsed; a told string is not hook-shaped work.
- **Q:** What is the generic `status` envelope, given loom's and lifecycle's disagree on both keys and the absent-file case? **A:** A four-key core (`current_producer`, `state`, `error`, `activity`) plus a `StatusExtras` hook for each product's own keys, with the absent-file disposition a told field on the arming spec. **Why:** the disagreement is deliberate — only `lyx loom start` may seed, so loom must refuse where lifecycle legitimately reports `found: false`.
- **Q:** Is the Shed built at arming time? **A:** No — the spec carries a `BuildShed` constructor called after `PreRun`/`PreStep` returns. **Why:** loom's pre-flight assigns `env.Landing` on the way through, so a Shed built earlier would carry a nil `Landing`.
- **Q:** How does `lyx shed status --recipe loom` keep loom's lightweight wiring? **A:** The arming function takes the verb, and loom's arming routes `status`/`pause` through `wireLightweight` using `verbUsesLightweightWiring` as the single shared authority; a lightweight spec leaves `BuildShed` nil. **Why:** a verb-blind arming would run the full `wire()` and reintroduce the broken-config hazard that path exists to avoid.
- **Q:** Does the envelope contract change? **A:** [auto-pick] No for `step` — the ten keys and five refusal kinds stay closed and move with the verb. **Why:** the key set is what `ly-drive` reads; a rename in the move is a silent break of the skill.
- **Q:** Do `pause` and `status --watch` generalize too? **A:** [auto-pick] Yes, with the watch line's `loom` prefix becoming a told label. **Why:** `shedengine` honours `PauseRequested` for every Shed, so a pausable-loom/unpausable-lifecycle split is arbitrary; `lyx lifecycle pause` appearing is intended.
- **Q:** Where does cwd resolution live? **A:** [auto-pick] Unchanged, in each module's own `PersistentPreRunE`; `shedcli` resolves nothing. **Why:** required by the Cwd Resolution and Told-Geometry invariants, and it is the only way `lyx shed --recipe lifecycle` keeps lifecycle's non-prime refusal.
- **Q:** One skill or two for driving a generic step? **A:** [auto-pick] One — generalize `ly-drive`, gating its loom-specific sections on the recipe name. **Why:** its loop already branches only on envelope fields; two skills would duplicate the one part worth having once.
- **Q:** Does this need a new invariant? **A:** [auto-pick] Yes — a Shed Verb-Set Invariant, in the same commit. **Why:** the extraction's value evaporates if a later module grows its own `run` body again, and two of the three clauses have a static shape a scan can enforce.
- **Q:** What test surface is genuinely new? **A:** [auto-pick] `internal/shedcli`'s own fake-Shed table tests plus cross-subtree envelope parity; everything else is regression. **Why:** the existing suites already cover the behaviour, so the new tests only need to cover the seam and prove nothing moved.
