# Batch: ly-drive-recipe-blind

```yaml
task: 'shed: the LLM driver as a generic stepper and mender'
batch: ly-drive-recipe-blind
number: 5
cards: 4
verify: go build ./... && go test ./cmd/lyx/ -run 'TestLyDriveSkill|TestHelpTree|TestDriftGuard' && go test ./internal/loomcli/ -run 'TestDriverPrompt|TestStartLLMDriverArm' && go test -tags integration ./internal/loomcli/ -run 'TestIntegrationDriverBootstrap' && go test -tags smoke ./internal/loomcli/ -run 'TestSmokeStatusAndPause_OnNeverBootstrappedPairNameTheRemedy' && go test ./internal/shedadapters/ -run 'TestBouncer_ReBounceProbesForALiveSeed'
depends-on: [3, 4]
```

## Batch Scope

Rewrites `ly-drive` as a recipe-blind driver and mender of `lyx shed step`, drops the loom step cap and its pin test, moves the loom-only launch knowledge into `lyx loom start`'s own help, replaces the cap pin with a recipe-blindness tripwire, and closes the task's documentation lifecycle (CONSTRAINTS, overview, design doc, roadmap).
It consumes batch 3's envelope keys and batch 2's trace vocabulary and `refDetail` grammar (overview decisions `trace-record-message-vocabulary` and `ref-detail-format`).
One batch because the skill rewrite, the deleted cap pin, and the tripwire replacing it must land together or `cmd/lyx`'s tests fail.
The last card deletes the design doc and moves the roadmap item, per the Documentation Lifecycle, so it must stay the batch's and the task's final commit.

## Cards

### Card 12: drop the autonomous step cap

- **Context:**
  - `internal/loomcli/start.go`
- **Edits:**
  - `internal/loomcli/driverprompt.go`
  - `internal/loomcli/driverprompt_test.go`
- **Creates:** none
- **Deletes:**
  - `cmd/lyx/drivercap_test.go`
- **Moves:** none
- **Requirements:**
  - In `internal/loomcli/driverprompt.go`, delete `AutonomousDriveStepCap` and its doc comment, and remove the cap sentence and the "step cap reached" stop condition from `driverPrompt`'s format string; the prompt keeps the run-id, the autonomous statement and the report path, with the stop conditions reading "task done, or a failure you cannot repair".
    Update the file header comment and `driverPrompt`'s doc comment so neither mentions a cap.
  - In `internal/loomcli/driverprompt_test.go`, drop the `AutonomousDriveStepCap` assertion and the `strconv` import, rename `TestDriverPrompt_NamesRunIDReportPathAndAutonomousMode`'s doc comment accordingly, and add an assertion that the prompt does not contain the substring `"step cap"`; keep `TestDriverPrompt_StaysWellUnderLaunchPromptCap` unchanged.
  - Delete `cmd/lyx/drivercap_test.go`; card 13 adds the tripwire that replaces it.
- **Commit:** `refactor(loomcli): drop the autonomous ly-drive step cap`

### Card 13: rewrite ly-drive recipe-blind, with the mender loop and a tripwire

- **Context:**
  - `_mill/discussion.md`
  - `internal/shedverbs/step.go`
  - `internal/shedverbs/status.go`
  - `internal/shedrun/seed.go`
  - `internal/fabricengine/mutation.go`
  - `internal/fabriccli/fabric.go`
  - `internal/logger/sink.go`
  - `cmd/lyx/sandbox_coverage_test.go`
- **Edits:**
  - `plugins/ly/skills/ly-drive/SKILL.md`
  - `plugins/ly/skills/INDEX.md`
- **Creates:**
  - `cmd/lyx/lydriveskill_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - Rewrite `plugins/ly/skills/ly-drive/SKILL.md` in full.
    Read the discussion's `## Decisions` first; every rule below is stated there, and the skill states each once.
    The skill names no recipe, no row, no recipe file path, no `.lyx/` path, and neither `lyx loom` nor `lyx batten`; it branches only on envelope fields, the policy words and the five kinds.
    Semantic line breaks throughout.
  - Frontmatter: keep `name: ly-drive`; remove `disable-model-invocation: true`; `argument-hint: "[run-id]"`; a `description` saying it drives one seeded run through `lyx shed step [<run-id>]`, repairing failures from each step's trace, and runs only when an operator, a launch prompt or an orchestrator's fork prompt names it.
  - Sections, in this order (headings may be worded differently):
    1. **What it does** — drives one addressed run (default `self`) by repeated `lyx shed step`, reads each envelope, repairs what it can from the step's trace, escalates what it cannot; which recipe runs is a property of the seed alone.
    2. **Drive directory** — the directory the run was seeded from; the fork prompt names it, otherwise the session's own cwd.
       Every `lyx` call runs as `(cd <drive-dir> && lyx …)` in a subshell so the session's own shell cwd never changes; a wrong directory surfaces as the recipe's own refusal text, which the driver reports verbatim.
    3. **How to invoke a step** — mint a fresh 16-lowercase-hex `LYX_TRACE_ID` per invocation and export it into the subshell; launch in the background (a step can block for a whole agent run, longer than any foreground shell call allows) with stdout redirected to `.scratch/ly-drive/<run-id>/step-<n>.json` under the driving session's own cwd, resolved to an absolute path before the subshell `cd`; wait for exit and read the envelope from the file.
    4. **Baseline** — one `lyx shed status [<run-id>]` read before the first step: record `current_producer`/`history_length`; an envelope with `found: false` (success or error) is an empty baseline `("", 0)` and the driver proceeds to the first step; any other status error is handed back.
    5. **The loop** — no step cap; continue while `continue` is true, reading the artifact `output` names when non-empty; after every step read its `trace_file` right away (traces are swept by retention, see section 9).
    6. **Stopping on a non-running state** — `continue: false`: `state: done` stops; `blocked`/`paused` are handed back with `reason`, never repaired.
    7. **Error envelopes** — the disposition table from Decision `repair-scope`: `producer`/`bootstrap`/`unseeded` go down the repair path under the repair cap; `busy` is handed back (the lock holder may be a live driver or a sibling fork, and the driver never pauses, kills or unlocks another driver); `ownership` is handed back; a kind-less refusal (unseeded run-id, unsupported verb) always escalates with its text verbatim, after reading its trace for the report.
    8. **Interrupted invocations** — no parseable envelope: one status read and the three sub-cases (run advanced → continue; unchanged and `interrupt_policy: reinvoke` → re-invoke, counting toward the repair cap; unchanged and `handback` or `""` → hand back unconditionally, since a live agent may still be running).
       The trace of an interrupted step is every file in the status envelope's `trace_dir` whose name carries that step's `LYX_TRACE_ID`, read in the order of the UTC timestamp in the name (`trace-<UTC>-<traceid>-<pid>.log`); a child spawned into another worktree, or a process in a reed strand pane, is reached only through paths the parent trace names, never by id.
    9. **Repairing** — read `trace_file` and the child traces it names; identify the half-finished mutation from the `fabric: mutation` records and the `shed: step` / `shed: step done` / `shed: step refused` boundary records; restore a state the next step can proceed from, then step again.
       Repairs act through `lyx`'s own verbs (`lyx fabric …`, and `lyx reed …` only for strands the trace names as the failed step's own) plus read-only git for diagnosis.
       Never: `lyx shed pause` as a repair, hand-editing a status file or `seed.json`, re-seeding, force-pushing, or deleting a branch or worktree the trace does not name as created by the failed step.
       When no `lyx` verb can do it, escalate — with the one exception in section 10.
       The coverage list from Decision `repair-scope` (partial `lyx fabric add`, partial `lyx fabric remove`, stranded remote weft branch, stranded remote warp branch, drifted weft side), each naming its verb.
       Before writing each verb and flag into the skill, confirm it against `internal/fabriccli/fabric.go` (`remove [--force] [--remote] <slug>`, `prune [--apply] [--force]`, `cleanup [--apply] [--force] [--remote]`, `reconcile` at planning time); a crash window outside the list escalates by design.
       Retention: traces can be swept before a later read, so each repair record copies the trace lines it acted on.
    10. **Stranded-branch exception** — the local- and remote-delete rules exactly as Decision `repair-scope` states them: warp branches only; a `branch_created` entry for exactly that branch (and, for a remote delete, also a `branch_pushed` entry) in the failed step's trace or a child trace sharing its id, with no later `branch_deleted` (local) or `remote_branch_deleted` (remote) entry; repository and remote taken from the entry's `detail` (`side=warp repo=<abs> [remote=<name>]`), run as `git -C <repo> branch -D <branch>` and `git -C <repo> push <remote> --delete <branch>`; a missing `detail`, a `side` other than `warp`, a missing `remote=` for a remote delete, or a repository path that no longer exists fails the rule and escalates; a `branch_pushed` entry alone never qualifies; never delete a branch carrying commits the trace does not attribute to the failed step.
       State that the Fabric Git Invariant binds `lyx`'s own code and this skill makes no commits, citing `CONSTRAINTS.md`.
    11. **Repair cap** — at most two repairs of the same row without the run advancing, keyed by `(current_producer, history_length)` from a status read taken before the first repair (an error envelope carries neither); a different pair resets the count; the third failure escalates; the `reinvoke` sub-case counts toward it; `("", 0)` is a valid key for a run with no status file.
    12. **Repair records and the stop report** — one record per repair under `<scratch_dir>/repairs/` (the failure, the trace lines acted on, the action taken, the outcome); the stop report goes under `scratch_dir` too (or to the path a launch prompt names); at every stop it lists `friction_dir` and `<scratch_dir>/repairs/`.
    13. **Self-report** — after the loop stops, read the `friction_dir` notes and the repair records; operator-driven mode may draft one `lyx selfreport create` call (body via `-b -` on stdin; `--label enhancement` for non-defects) fired only on explicit operator approval, at most once per supervised run; autonomous mode files nothing and puts the draft in the report; every repair record is itself friction data.
    14. **Operator choices** — numbered text list, `1) Label — description`, never a mouse prompt.
    15. **Autonomous mode** — when the launch prompt says so: no operator choices (each becomes a report line), the report goes to the path the prompt names, and every other rule stays absolute.
    16. **Driven from an orchestrator** — the orchestrator forks one `Agent` per run with a prompt naming this skill, the run-id and the drive directory; the fork runs the loop and returns the stop report path plus a short summary; recipe-specific directory knowledge lives in the orchestrator's own prompt, never here; a fork's lifetime is the orchestrator session's, stated as a limit.
  - Rewrite the `ly-drive` row and the closing sentence of `plugins/ly/skills/INDEX.md` to match the new description: runs only when an operator, a launch prompt or a fork prompt names it (drop "explicit-invocation-only and is never started by a model").
  - Create `cmd/lyx/lydriveskill_test.go` (package `main`, untagged, spawns nothing), resolving the repo root through `runtime.Caller` the way `cmd/lyx/sandbox_coverage_test.go` does, with `TestLyDriveSkill_IsRecipeBlind`: reads `plugins/ly/skills/ly-drive/SKILL.md` and fails for each of `shedrun.RecipeNames()` that appears as a whole word, case-insensitively (a `regexp` with `(?i)\b<name>\b`, so `loomyard` does not match), and for each literal substring `.lyx/`, `lyx loom`, `lyx batten`; each failure names the offending token and its line number.
    Its file doc comment says it replaces the retired step-cap pin and why: the design's claim that the recipe is a property of the seed alone.
- **Commit:** `feat(ly-drive): drive any seeded run recipe-blind and repair from the step trace`

### Card 14: move the launch convention into lyx loom start's help and update step's help

- **Context:**
  - `internal/loomcli/driverprompt.go`
  - `internal/shedverbs/spec.go`
  - `internal/shedengine/run.go`
- **Edits:**
  - `internal/loomcli/start.go`
  - `internal/loomcli/cli.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - In the `start` command's `Long` help in `internal/loomcli/start.go`, add a paragraph after the numbered steps that carries what the old skill held about the llm driver's launch: a worktree opened through `lyx ide spawn`'s generated VS Code task starts `lyx reed up`, `lyx reed add --if-absent --cmd claude --name claude --focus`, then `lyx reed attach`, so the operator's own session is the strand named `claude` and the panes the run spawns are its siblings; the `$TMUX_PANE` self-check (compare `$TMUX_PANE` against `lyx reed status`'s tracked strands — tracked: fine; set but untracked: relaunch through that chain or proceed without reed supervision; unset: unconfirmed, not failed, since psmux on Windows may not export it); and that a worktree whose `.vscode/tasks.json` predates the convention is upgraded by deleting that file and re-running `lyx ide spawn`.
    Keep it in plain help-text prose (no markdown), wrapped like the surrounding `Long` text; `Short` is unchanged.
  - In the `Step` `VerbText`'s `Long` in `internal/loomcli/cli.go`, replace the paragraph saying the supervisor reads only `continue` and `next_interrupt_policy`: the envelope also names `trace_file` (the durable trace this invocation wrote), `friction_dir` and `scratch_dir`, and every error envelope carries the same three keys beside `kind`, so a supervisor can read what the step did and repair from it.
    `Short` is unchanged.
- **Commit:** `docs(loomcli): move the llm driver launch convention into start's help`

### Card 15: close the documentation lifecycle

- **Context:**
  - `_mill/discussion.md`
- **Edits:**
  - `CONSTRAINTS.md`
  - `docs/overview.md`
  - `internal/battenshed/doc.go`
  - `internal/shedadapters/bouncer_seed_test.go`
  - `internal/loomcli/smoke_bootstrapwiring_test.go`
  - `manifest/roadmap.md`
- **Creates:** none
- **Deletes:**
  - `manifest/designs/shed-llm-driver.md`
- **Moves:** none
- **Requirements:**
  - In `CONSTRAINTS.md`'s `## Fabric Git Invariant (warp + weft)`, add one bullet recording the reading from Decision `repair-scope`: the invariant binds `lyx`'s own code, and the `ly-drive` skill is not `lyx` code;
    its one raw-git mutation is the stranded-branch exception — deleting a warp branch the failed step's trace proves it created (and, remotely, pushed) — which makes no commit, so the agent-commit clause does not reach it;
    every other repair goes through `lyx` verbs and so through `fabricengine`'s gates.
  - In `docs/overview.md`'s `shed` bullet, add sentences after the `internal/shedcli` sentence: the generic `step` envelope names the durable `trace_file` the invocation wrote plus the run's `friction_dir` and `scratch_dir` (on success and every error), `status` names `trace_dir`, fabric writes every recorded mutation to that trace, and the `ly-drive` skill drives any seeded run through `lyx shed step` recipe-blind, repairing failures from the trace and escalating what it cannot, with an orchestrator session forking one `ly-drive` loop per run.
    Keep the loom and bootstrap bullets' existing `ly-drive` mentions; they stay true.
  - In `internal/battenshed/doc.go`, reword the llm-driver clause "an llm driver handing back on a refusal it may not retry, exhausting its step cap, or finding its skill unavailable" to "an llm driver escalating a failure it cannot repair or finding its skill unavailable"; the rest of the paragraph stays.
  - In `internal/shedadapters/bouncer_seed_test.go`, reword `TestBouncer_ReBounceProbesForALiveSeed`'s doc comment so it no longer quotes the old skill's "there is no orphan" claim as current text: say the crucible-round-1 reproduction contradicted what the pre-rewrite `ly-drive` skill told operators, and that the rewritten skill makes no such claim; the reproduction facts stay.
  - In `internal/loomcli/smoke_bootstrapwiring_test.go`, reword `TestSmokeStatusAndPause_OnNeverBootstrappedPairNameTheRemedy`'s doc comment sentence that quotes the skill's "literal first instruction": the `ly-drive` skill's first move is a status read taken as its baseline, and on a brand-new task that read must reach the verb's own remedy rather than a lock-directory error; do not quote a `lyx` command the rewritten skill does not contain.
    Comment-only; no test logic changes.
  - In `manifest/roadmap.md`, move the Planned item **shed: the LLM driver as a generic stepper and mender** to the top of `## Done`, per the file's own Maintenance rules: one or two sentences, and replace the design-doc link with a pointer to the `ly-drive` skill and the `internal/shedverbs` package documentation (card 13 rewrites `plugins/ly/skills/ly-drive/SKILL.md`, the skill's path to link).
    Leave `## Planned`'s intro sentence with no items.
  - Delete `manifest/designs/shed-llm-driver.md` in the same commit; after deleting, grep `manifest/` and `docs/` for `shed-llm-driver.md` and confirm no link to it remains (Markdown Link Integrity).
  - This card is the task's last commit (Documentation Lifecycle).
- **Commit:** `docs: record the ly-drive repair exception and retire the shed-llm-driver design`

## Batch Tests

`go build ./...` confirms nothing else referenced `AutonomousDriveStepCap`.
`TestLyDriveSkill_IsRecipeBlind` is the new tripwire over the rewritten skill;
`TestHelpTree*`/`TestDriftGuard*` in `cmd/lyx` confirm the edited `Long` help keeps every command's `Short` and the help tree intact;
`TestDriverPrompt*` covers the cap removal and the prompt length bound;
`TestStartLLMDriverArm*` confirm the llm arm still composes its prompt;
the tagged `TestIntegrationDriverBootstrap*` runs the real llm-driver bootstrap that sends the reworded prompt.
The smoke case and `TestBouncer_ReBounceProbesForALiveSeed` compile and run the two test files card 15 re-comments.
The Markdown Link Integrity test runs in the done gate's full suite and covers card 15's roadmap and design-doc edits.
