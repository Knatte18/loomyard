# Discussion: shed: the LLM driver as a generic stepper and mender

```yaml
task: 'shed: the LLM driver as a generic stepper and mender'
slug: shed-llm-driver
status: discussing
parent_branch: main
```

## Problem

Every crucible campaign against a live-substrate module ends with more crash-window findings than it started with:
a process killed mid-`git push`, a branch stranded on the remote, a remedy text that fails when followed verbatim.
Go code has to be perfect against these today, because nothing downstream of a `lyx` command can see what it did or repair what it left behind.
The user has changed course: `lyx` stops trying to be perfect, and an LLM outer loop repairs instead — the property that has made millhouse useful.

The design is `manifest/designs/shed-llm-driver.md` (read it first; this file settles its `## Open` list and fixes the build shape).
Three things change:
`ly-drive` becomes a recipe-blind driver of `lyx shed step <run-id>`;
every step leaves a durable trace complete enough to repair from, and the step envelope names it;
the driver repairs failures from that trace instead of handing every failure back.
An orchestrator session drives runs by forking one `ly-drive` loop per run.

Why now: the `#019 crucible-batten-followup` campaign ran four rounds without a safety pass, and its findings moved outward into fabric's crash windows.
This task absorbs the former `fabric-pair-state-after-crash` and `remedy-texts-followed-verbatim` tasks;
the other crash-window findings stay parked as GitHub issues #269, #270, #271 and are out of scope.

## Scope

**In:**

- `plugins/ly/skills/ly-drive/SKILL.md` rewritten recipe-blind, with the mender loop, the repair rules, and the orchestrator-fork usage.
- `plugins/ly/skills/INDEX.md` entry updated to match.
- `internal/logger`: two new exported accessors (`TraceFile`, `TraceDir`) and an amended level policy.
- `internal/fabricengine/mutation.go`: every appended entry written to the durable sink at `Info` as it is appended.
- `internal/shedverbs`: the step body logs its own boundaries at `Info`, and the step envelope (success and every five-kind error) gains `trace_file`, `friction_dir`, `run_dir`; the status envelope gains `trace_dir`.
- `internal/shedverbs/spec.go`: two new told `Spec` fields, `RunDir` and `FrictionDir`.
- `internal/loomcli` and `internal/battencli` fill `RunDir` at their one `shedverbs.Spec` construction site each (`arm.go`), so `lyx shed step`, `lyx loom step` and `lyx batten step` all carry it; `internal/loomcli` also fills `FrictionDir`, batten leaves it empty.
- `internal/landingshed`: its GitHub write calls log at `Info`.
- The loom driver launch: the step cap is dropped (`AutonomousDriveStepCap`, the prompt sentence, `cmd/lyx/drivercap_test.go`).
- Docs: `CONSTRAINTS.md` (Shed Verb-Set Invariant allowlist and envelope clause), `docs/overview.md`, `internal/battenshed/doc.go`'s llm-driver paragraph, `internal/shedverbs/step.go`'s one-retry comment, the logger level-policy doc comment.
- On completion, `manifest/designs/shed-llm-driver.md` is deleted and its `manifest/roadmap.md` Planned item removed, per the rule that `manifest/` holds only unbuilt work.

**Out:**

- GitHub issues #269, #270, #271 and every other crash-window fix in Go — the whole point is not to harden these.
- Rewriting remedy texts or making fabric resume partial states itself — both absorbed tasks shrink to the trace requirements above.
- Letting batten be seeded `--driver llm` (see Decision `bootstrap-gate-stays`).
- Any Go orchestrator, fork scheduler, or reed work — forking is the orchestrator session's own `Agent` tool, described in the skill.
- Moving loom's friction directory or running loom's friction reflection under `step`.
- The `#023 loom-done-after-friction` bug.
- `manifest/HANDOFF.md` — never edited by a task.

## Decisions

### bootstrap-gate-stays

- Decision: keep `BootstrapVerb` and `shedcli`'s `--driver llm` validator unchanged.
  The skill drives any seeded run through `lyx shed step <run-id>` whatever its recorded driver;
  `driver: llm` keeps its present, narrower meaning — the recipe's bootstrap verb spawns an `ly-drive` strand.
  Loom has that spawn (`lyx loom start`); batten has none, so a batten run is driven by an orchestrator fork running in the hub's prime worktree, with no seed change.
- Rationale: stepping never reads the driver field (Driver Choice Single-Site Invariant), so "every recipe drivable" already holds for the skill.
  Letting batten record `llm` with no spawn behind it would give the recorded value no effect at all, which is the silent divergence that invariant exists to prevent.
- Rejected: making every recipe seedable with `llm` — it needs a bootstrap verb batten does not have and adds nothing the fork does not already give.

### trace-in-envelope

- Decision: every envelope the generic `step` body emits — the success envelope and each of the five-kind error envelopes — carries `trace_file`: the absolute path of this process's durable trace file, or `""` when no sink is armed.
  `internal/logger` gains `TraceFile() string`, which forces the lazy sink open (the same `ensureDurableSink` path `NotifyExit` uses) and returns its path, `""` when the sink cannot arm.
  `StepEnvelope`'s closed key set grows from ten to thirteen (`trace_file`, `friction_dir`, `run_dir`), and both key-set tests (`internal/shedverbs/step_test.go`, `internal/loomcli/step_test.go`) move with it.
  The five-value refusal-kind vocabulary does not change.
- Rationale: the driver has to read exactly what the step did without searching; the error envelopes are where it needs the trace most.
  The design's worry that this touches the Shed Verb-Set Invariant is narrower than it looked: that invariant pins the refusal kinds, not the envelope's success keys — the key set is closed by doc comment and test only.
- Rejected: a new hook that lets a recipe add step-envelope keys (like `PostRun`'s map) — the trace is generic, so the generic body owns it.

### shedverbs-imports-logger

- Decision: admit `internal/logger` into `internal/shedverbs`'s import allowlist (`seam_enforcement_test.go`) and say so in the Shed Verb-Set Invariant.
  The step body logs `Info` at entry (run's `status_file`) and after `shed.Step` returns (producer, outcome, state, next, reason), and `Warn` on each error envelope with its kind.
- Rationale: one generic site gives every recipe a step boundary in its trace, which is what "every mutating step logs at `Info`" needs — `shedengine` cannot log (Shed Producer-Seam Invariant), and a per-recipe hook would have every recipe repeat the same two lines.
  The no-resolver clause still holds: `shedverbs` calls no resolver itself; the logger's own sink resolution is the logger's concern, the same reasoning that admitted `logger` into `internal/friction`.
- Rejected: a `Spec.TraceFile func() string` plus `PreStep`/`PostStep` logging per recipe — keeps the allowlist unchanged but duplicates the boundary logging per recipe and leaves the next recipe free to forget it.

### mutations-logged-on-append

- Decision: `Mutations.Append` and `Mutations.AppendRef` each emit one `logger.Info("fabric: mutation", "kind", …, "target", …, "detail", …)` after recording the entry.
  `Extend` logs nothing, since its entries were logged when first appended.
  A nil receiver still records and logs nothing.
- Rationale: a SIGKILL then loses nothing up to the last completed mutation.
  Logging in the recorder, not in every executor, covers all current and future call sites, including the shed-path callers that discard the record today.
- Rejected: logging an "about to" intent line before each primitive — the step-boundary `Info` line already names the producer in flight, and the executor-order rule ("append only after an observable change") stays the single meaning of an entry.

### level-policy-amended

- Decision: amend `internal/logger/logger.go`'s level-policy doc comment so `Info` covers three things: a real OS-process spawn or teardown (unchanged), one recorded durable state mutation, and a shed step boundary.
  The Live-Substrate Spawn Observability constraint is unchanged — it requires `Info` for spawns, it does not reserve `Info` for them.
- Rationale: the durable sink records `Info` and above only, and an unrecorded mutation cannot be repaired from.
- Rejected: logging mutations at `Warn` — mislabels every successful mutation as a notable failure.

### other-mutations

- Decision: `internal/landingshed`'s GitHub write calls (PR create, merge, and any other API write) log at `Info` after success, naming the PR number and action.
  Status-file writes are not logged — the status file already records them.
  Seed writes go through `CommitAnchoredPaths`, so they are covered by `mutations-logged-on-append`.
- Rationale: outside fabric, GitHub is the one external state a crash can strand with no local record.
- Rejected: a blanket rule to log every `os.WriteFile` — noise the driver cannot act on.

### trace-dir-and-trace-id

- Decision: the generic `status` envelope gains `trace_dir` (`logger.TraceDir()`, the directory the sink writes to, resolved the same way as the sink, `""` when unarmed).
  The skill mints a fresh 16-hex `LYX_TRACE_ID` for each step invocation (the root pre-run already adopts a valid inherited id through `MintOrAdoptAndExport`), so the trace of an interrupted step — one that wrote no envelope — is the set of files in `trace_dir` whose names carry that id.
  Child `lyx` processes sharing the anchor write their own files with the same id (`trace-<UTC>-<traceid>-<pid>.log`) and the step's pid is unknown after an interrupt, so the driver reads all of them, ordered by the UTC timestamp in the name.
  The same lookup covers the kind-less refusals `internal/shedcli`'s pre-run emits (unseeded run-id, unsupported verb): those envelopes gain no key, and `shedcli`'s pre-run is not touched.
  Child `lyx` processes a step spawns (for example batten's `loom start` in a task worktree) inherit the same id and write their own files, possibly under another worktree's `.lyx/logs`; the skill follows paths the parent trace names.
- Rationale: an interrupted invocation is the crash-window case this whole design targets, and it is the one case with no envelope to name a trace.
- Rejected: finding the latest file in the logs directory by timestamp — races any concurrent `lyx` process.

### friction-and-run-dir

- Decision: two told `Spec` fields surface on every step envelope.
  `FrictionDir` is the recipe's own agent friction-note directory, `""` when the recipe has none or friction is off: loom fills it from its existing `loomengine.LoomFrictionDir` when `loom.yaml` enables friction (the same condition as `internal/loomcli/wiring.go`), batten leaves it empty.
  `RunDir` is the run's shed scratch directory, `shedrun.ScratchDir(location, runID)`, the sanctioned constructor `internal/loomcli/driverreport.go` already calls.
  It is filled at each module's single `shedverbs.Spec` construction site (`internal/loomcli/arm.go` ~161, `internal/battencli/arm.go` ~312), which both the module's own subtree (`internal/loomcli/cli.go` ~334, `internal/battencli/cli.go` ~172) and its `ArmAt` reach, so all three step entry points carry it.
  Filling it in `internal/shedcli` alone was rejected: the module-local `lyx loom step`/`lyx batten step` would then emit `run_dir: ""`.
  The driver writes its own records under `run_dir`: one repair record per repair under `repairs/`, and its stop report.
  At every stop it lists `friction_dir` and `run_dir/repairs/` in the report.
- Rationale: the design moves the friction location out of the skill and onto the envelope.
  Keeping loom's directory where it is avoids moving `frictionengine`'s input; the driver's own repair records are driver artifacts and belong beside its report, which already lands in `shedrun.ScratchDir` (`internal/loomcli/driverreport.go`).
- Rejected: one shed-level friction directory every recipe shares — it would move loom's friction directory, its lock, archive prefix and first-seed clearing, for no gain the driver needs.

### repair-scope

- Decision: the driver repairs on error envelopes (all five kinds and the kind-less shed pre-run refusals) and on interrupted invocations.
  An envelope with `continue: false` is not repaired: `done` stops, and a `blocked` or `paused` state is handed back with its `reason`, because a Go gate concluded a human is needed.
  A repair reads `trace_file` (and the child traces it names), identifies the half-finished mutation, and restores a state the next step can proceed from.
  Repairs act through `lyx`'s own verbs (`lyx fabric …`, `lyx reed …`, `lyx shed pause`) and read-only git for diagnosis;
  the driver never edits a status file or `seed.json` by hand, never re-seeds, never force-pushes, and never deletes a branch or worktree the trace does not name as created by the failed step.
  When no `lyx` verb can perform the repair, it escalates, with one narrow exception below.
- Stranded-branch exception: the driver may run `git branch -D <branch>` (local) or `git push <remote> --delete <branch>` (remote) on a branch only when all three hold:
  the failed step's trace (or a child trace sharing its id) carries a `fabric: mutation` entry of kind `branch_created` or `branch_pushed` for exactly that branch;
  no later entry in those traces records it deleted (`branch_deleted`, `remote_branch_deleted`);
  and no `lyx fabric` verb removes it.
  Each such deletion is a repair record like any other.
  The Fabric Git Invariant binds `lyx`'s own code; the driver is a skill, and the only git clause that names agents ("an agent commits its own code to warp only") governs commits, which this exception never makes.
  This task records that reading in the invariant's text, same commit.
- Coverage of the absorbed crash windows (the plan verifies each verb's behaviour against its `--help` and code before the skill names it):
  - Partial `lyx fabric add` (worktrees created, a later step such as the push failed): `lyx fabric remove <slug>` removes the pair and the weft branch; the warp branch `rollbackAdd` leaves behind (GitHub #269) falls under the stranded-branch exception.
  - Partial `lyx fabric remove`: re-run `lyx fabric remove [--force]`, or `lyx fabric prune --apply` for an orphaned half-pair.
  - Stranded remote weft branch: `lyx fabric cleanup --apply --remote`.
  - Stranded remote warp branch pushed by the failed step: the stranded-branch exception.
  - Drifted or broken weft side of a pair: `lyx fabric reconcile`.
  A crash window outside this list escalates by design; that is accepted, and its repair record or escalation still surfaces it as data.
- Rationale: the Fabric Git Invariant keeps mutating git inside `fabricengine` for `lyx`'s own code; routing repairs through `lyx` verbs keeps the driver inside the same destructive gates.
  Without the exception, the headline crash window of the absorbed `fabric-pair-state-after-crash` work (#269) would escalate by construction.
  A `blocked` state is a verdict, not a crash.
- Rejected: repairing every non-running envelope — overrides Go gates that exist to stop for a human;
  unrestricted mutating git — bypasses fabric's destruction chokepoint;
  no exception at all — leaves the absorbed stranded-branch window unrepairable.

### repair-cap

- Decision: the one-retry rule for `kind: producer` is replaced by the driver's judgment, bounded by one cap: at most **two** repairs of the same row without the run advancing (same `producer` and `history_length` as the envelope before the first repair).
  On the third failure the driver escalates.
  The interrupted-invocation branch keeps its existing two-consecutive re-invoke cap and counts toward the same limit.
  `internal/shedverbs/step.go`'s kind comment drops the one-retry wording and points at the skill.
- Rationale: two attempts separate a transient crash window from a systematic defect; shed's bounce budgets already bound normal progress.
- Rejected: no cap — a repair that re-breaks the same state loops until the step cap that this task removes.

### step-cap-dropped

- Decision: no step cap in the skill, operator-driven or autonomous.
  Delete `loomcli.AutonomousDriveStepCap`, the cap sentence in `driverPrompt`, `cmd/lyx/drivercap_test.go`, and the cap assertion in `internal/loomcli/driverprompt_test.go`.
  The prompt keeps run-id, autonomy and report path.
- Rationale: the design drops it — shed's bounce budgets bound the graph and `repair-cap` bounds repairs; the cap's derivation was loom arithmetic a recipe-blind skill cannot carry.
- Rejected: a generic large cap — an arbitrary number with no derivation.
- Merge ordering: wiki task #23 `loom-done-after-friction` adds a fifteenth loom row and updates row-count assertions, which may touch the cap files.
  Whichever of the two lands second reconciles them; if #23 lands first and edits the cap files, this task still deletes them.
  #23's friction handling (reflection skipped under `step`, notes left in `loomengine.LoomFrictionDir`) is what `friction_dir` names, so the two do not otherwise conflict.

### recipe-blind-skill

- Decision: the rewritten `SKILL.md` names no recipe, row, or recipe file path.
  It branches only on the envelope's fields, the policy words, and the five kinds.
  What moves out, per the design's table: the loom step arithmetic, `.lyx/loom/friction/` and loom's self-report paragraph, the cwd paragraph and `lyx loom start` as bootstrap remedy (the recipe's own refusal text now carries it, which the driver reports verbatim), the `$TMUX_PANE` self-check and loom launch convention (moved to `lyx loom start`'s own `Long` help), and any gloss on what `handback` means for a recipe.
  Step output files go to `.scratch/ly-drive/<run-id>/step-<n>.json`, relative to the driving session's cwd.
  A new Go tripwire test replaces `drivercap_test.go`: `SKILL.md` contains none of the shipped recipe names (`shedrun.RecipeNames()`) as whole words, case-insensitively (so `loomyard` does not trip it), nor `.lyx/`, `lyx loom` or `lyx batten`.
- Rationale: the design's core claim is that which recipe runs is a property of the seed alone; a tripwire keeps a later edit from re-teaching the skill a recipe.
- Rejected: leaving the loom sections as "recipe-specific notes" — the pattern the current skill already shows, which the design removes.

### orchestrator-fork

- Decision: the skill gains a section on being driven from an orchestrator: the orchestrator session forks one `Agent` per run with a prompt naming the skill and the run-id, and the fork runs the loop and returns the stop report path plus a short summary.
  The fork runs from the directory the run's recipe requires (for batten, the hub's prime worktree).
  Frontmatter `disable-model-invocation: true` is removed, since a fork and the loom driver strand both invoke the skill as the model; the description says it runs only when an operator or a launch/fork prompt names it, and `INDEX.md` line 7 is reworded to match.
  A fork's lifetime is the orchestrator session's; this is stated as a limit, not solved.
- Rationale: the design puts the fork in the orchestrator's own `Agent` tool, which keeps the interactive-tmux rule without new Go.
- Rejected: a Go-side orchestrator or a `lyx` verb that spawns forks — YAGNI.

### self-report

- Decision: the self-report section stays, recipe-blind: after the loop stops, the driver reads `friction_dir` notes and its own repair records, and in operator-driven mode may draft one `lyx selfreport create` call, fired only on explicit operator approval.
  In autonomous mode it files nothing; the draft goes into the report.
  Every repair record is itself friction data, so a healed crash window still surfaces.
- Rationale: filing a public issue is outward-facing and hard to reverse.
- Rejected: autonomous filing.

## Technical context

- Step verb: `internal/shedverbs/step.go` — `StepEnvelope` (ten keys today), the five kinds and `StepKinds`, and the `stepCmd` body (PreStep → BuildShed → `shed.Step` → PostStep → envelope).
  Error envelopes go through `output.ErrFields(out, msg, map[string]any{"kind": …})`; the new keys go into that map.
- `Spec` and `Hooks`: `internal/shedverbs/spec.go`; `Hooks.StatusExtras` exists for the status verb (`status.go` ~169), but `trace_dir` is generic and belongs in the status body itself.
- `shedverbs`'s allowlist: `internal/shedverbs/seam_enforcement_test.go` (lines ~26-32); `lyxcwd` is denied explicitly and stays denied.
- Recipe arming: `internal/shedcli/table.go` (`recipes`, `entry.Arm`), resolved once in `resolvePersistentPreRun` (`internal/shedcli/cli.go`), which already holds the `*lyxcwd.Location` and run-id `RunDir` needs.
  Kind-less refusals (unseeded run-id, unsupported verb) are emitted in `shedcli`'s pre-run, above the step body; they stay unchanged, and the skill finds their trace through `trace_dir` and the per-step `LYX_TRACE_ID` (Decision `trace-dir-and-trace-id`).
- Loom arm: `loomcli.ArmAt`; friction condition at `internal/loomcli/wiring.go` ~265-272; directory at `internal/loomengine/config.go` (`LoomFrictionDir`, which says no other package may construct it — `loomcli` calls it, it does not construct it).
- Logger: `internal/logger/sink.go` — `sinkPath` (unexported, ~line 82), `ensureDurableSink` (~99), `armDurableSinkLocked`, file name `trace-<UTC>-<traceid>-<pid>.log` (~150), `NotifyExit` (~303); `durableHandler.Enabled` passes `Info`+ only (`logger.go` ~301); the level policy comment is at `logger.go` ~120-133.
  The sink is off under `go test` unless `LYX_TRACE=1` or a directory override is set (`SetDurableSinkDir`); tests of `TraceFile`/`TraceDir` and of mutation logging use the override.
  `MintOrAdoptAndExport` (`trace.go` ~83) adopts a valid 16-hex `LYX_TRACE_ID`.
- Mutations: `internal/fabricengine/mutation.go` (`Append` ~133, `Extend` ~166, `AppendRef` ~177), nil-safe pointer receivers; `fabricengine` already imports `logger`.
  The Fabric Destruction Chokepoint Invariant's "`rec *Mutations` threaded into `destroy.go` only" is untouched — this changes what the recorder does, not where it is threaded.
- Loom driver launch: `internal/loomcli/driverprompt.go` (`AutonomousDriveStepCap`, `driverPrompt`), `driverprompt_test.go`, `driverreport.go` (report under `shedrun.ScratchDir`), `start.go` (`startLLMDriverArm`, the `Long` help that receives the moved launch-convention text).
- Pins to update: `cmd/lyx/drivercap_test.go` (deleted), `internal/shedverbs/step_test.go` (`TestStepEnvelope_KeySetIsExactlyTen`), `internal/loomcli/step_test.go` (~39-64 key list), `internal/loomcli/driverprompt_test.go`.
  Comments naming ly-drive's retry rule or orphan claim: `internal/battencli/step_test.go:27`, `internal/shedadapters/bouncer_seed_test.go:475`, `internal/loomcli/smoke_bootstrapwiring_test.go:150`, `internal/battenshed/doc.go:15-29` (mentions the step cap).
- Docs naming the old behaviour: `docs/overview.md` (~335, ~385, ~444), `internal/loomcli/cli.go` step `Long` help (~233-240), `CONSTRAINTS.md` Shed Verb-Set Invariant.
- The current `SKILL.md` wrongly says loom's friction directory is under the worktree root; it is under `AnchorPath`. The rewrite drops the sentence.
- GitHub writes: `internal/landingshed/publish.go` and `finalize.go` through `githubclient`.

## Constraints

- **Shed Verb-Set Invariant** — the five kinds stay closed; this task amends the allowlist clause to admit `internal/logger` and records that the step envelope carries `trace_file`/`friction_dir`/`run_dir`. Same commit.
- **Shed Producer-Seam Invariant** — `shedengine` still imports only stdlib, `state`, `lock`; no logging there.
- **Shed Run-Directory Invariant** — `run_dir` comes from `shedrun.ScratchDir`; nothing else names the `shed` segment.
- **Driver Choice Single-Site Invariant** — nothing new reads the recorded driver field.
- **Told-Geometry Invariant** — `RunDir`/`FrictionDir` are told to `shedverbs`; no engine derives them.
- **Durable-vs-Ephemeral State Invariant** — traces and repair records stay under `.lyx`.
- **Mutation Record Invariant** / **Fabric Destruction Chokepoint Invariant** — entry semantics and threading unchanged.
- **Fabric Git Invariant** — the driver's repairs mutate git through `lyx` verbs, except the narrow stranded-branch exception in `repair-scope`, whose reading this task records in the invariant's text.
- **Live-Substrate Spawn Observability** — unchanged; the level-policy amendment widens `Info`, it does not narrow spawn logging.
- **CLI / Cobra Invariant** — no new command; changed `Long` help keeps `Short` non-empty.
- **Test Tier Purity** — the new logger and mutation tests are untagged and spawn nothing.
- **Markdown** — semantic line breaks in every edited `.md`; `Markdown Link Integrity` for `docs/` and `manifest/` edits.
- **Documentation Lifecycle** — design doc deleted and roadmap item removed at completion, same commit as the last change.

## Testing

- `internal/logger` (TDD): `TraceFile` returns `""` with no sink, forces the file open and returns its path with a directory override, and returns the same path on repeat calls; `TraceDir` matches the override.
- `internal/fabricengine` (TDD): with a sink override, `Append` and `AppendRef` each write one `fabric: mutation` record carrying kind, target, detail; `Extend` writes none; a nil `*Mutations` writes none.
- `internal/shedverbs` (TDD): success envelope key set is exactly thirteen; each five-kind error envelope carries `trace_file`, `friction_dir`, `run_dir`; `run_dir`/`friction_dir` echo the `Spec` fields; step writes an entry and an outcome `Info` record; status envelope carries `trace_dir`; the seam test admits `logger` and still denies `lyxcwd` and `*cli`.
- `internal/loomcli`: step key-list test moved to thirteen; `FrictionDir` set when friction is enabled and empty when not; `driverPrompt` no longer mentions a cap and stays under its length bound.
- `internal/loomcli` and `internal/battencli`: `RunDir` equals `shedrun.ScratchDir` for the addressed run on every step entry point — `lyx shed step`, `lyx loom step`, `lyx batten step` — asserted on the emitted envelope, not only on the `Spec`.
- `internal/landingshed`: a successful PR write logs one `Info` record (through the existing fake GitHub seam).
- `cmd/lyx`: the recipe-blindness tripwire on `SKILL.md`.
- Integration: an existing integration test that drives a real `lyx shed step` gets one assertion that `trace_file` names an existing file containing the step's boundary records.

## Q&A log

- **Q:** Does a generic driver need every recipe seedable with `--driver llm` (the `BootstrapVerb` gate)? **A:** [auto-pick] Keep the gate; drive batten through an orchestrator fork. **Why:** stepping never reads the driver field, and an `llm` value with no spawn behind it would be an inert recorded value.
- **Q:** How does the trace path reach the envelope when `shedverbs` cannot import `logger`? **A:** [auto-pick] Admit `logger` into `shedverbs` and log step boundaries there. **Why:** one generic site covers every recipe; a hook would be repeated per recipe.
- **Q:** Does adding envelope keys breach the Shed Verb-Set Invariant? **A:** [auto-pick] No — it pins the refusal kinds; the key set moves to thirteen with both tests. **Why:** the kinds are unchanged.
- **Q:** How is an interrupted step's trace found? **A:** [auto-pick] The driver mints `LYX_TRACE_ID` per step; `status` carries `trace_dir`. **Why:** a direct lookup, no timestamp race.
- **Q:** Shared shed-level friction, or recipe-named? **A:** [auto-pick] Recipe-named `friction_dir` plus shed-owned `run_dir` for the driver's repair records. **Why:** no move of loom's friction machinery.
- **Q:** Which non-running envelopes does the driver repair? **A:** [auto-pick] Error envelopes and interrupted invocations; `blocked`/`paused` are handed back. **Why:** a gate verdict is not a crash.
- **Q:** What may a repair touch? **A:** [auto-pick] `lyx` verbs plus read-only git, with one narrow exception for deleting a branch the failed step's trace shows it created; never status/seed by hand, no force-push. **Why:** Fabric Git Invariant, while keeping the absorbed stranded-branch window (#269) repairable.
- **Q:** Cap on repeated repairs of the same row? **A:** [auto-pick] Two repairs without the run advancing, then escalate. **Why:** separates a transient crash window from a systematic defect.
- **Q:** Keep a step cap? **A:** [auto-pick] Drop it and its pin test. **Why:** the design drops it; bounce budgets and the repair cap bound the loop.
- **Q:** Log mutations at which level, and where? **A:** [auto-pick] `Info`, inside `Mutations.Append`/`AppendRef`; amend the level-policy comment. **Why:** only `Info`+ reaches the durable sink.
- **Q:** How does a fork load a skill marked `disable-model-invocation`? **A:** [auto-pick] Remove the flag; keep explicit-invocation by description. **Why:** forks and the loom driver strand invoke as the model.
