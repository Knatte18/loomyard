# Conflict Resolution Brief

Your sole job is to resolve git conflict markers in the listed files, stage each resolved file, and report success.
Do NOT commit.
Do NOT run `git merge --continue` — the SKILL does that after receiving `{"status":"success"}`.

## Task intent

These excerpts describe what THIS branch is trying to accomplish.
When the merge introduces a parent-side change that conflicts with this branch's intent, the resolution preserves THIS branch's intent.
In particular: if a file appears under a batch's `Deletes:` list and the merge introduces a modified version of that file from the parent, the resolution is to delete the file (your branch's intent overrides).
Stage the deletion with `git -C /home/knatte/Code/loomyard/wts/ly-supervise-reed-add rm <file>`.

### From discussion.md

# Discussion: Launch ly-supervise and orchestrator via lyx reed add

```yaml
task: Launch ly-supervise and orchestrator via lyx reed add
slug: ly-supervise-reed-add
status: discussing
parent: main
```

## Problem

Every Claude session that works a loomyard worktree today is launched ad hoc: `internal/vscode/config.go` generates a `.vscode/tasks.json` whose single "Start Claude" task runs the bare `claude` binary in a VS Code integrated terminal on `folderOpen`, and the `/ly:ly-supervise` skill tells the operator to open `lyx reed attach` in a *separate* side terminal to watch the agents loom spawns.
The result is two disconnected views of one task: the orchestrator/supervisor session lives in a VS Code terminal that reed knows nothing about, while everything loom spawns lives in reed's tmux session.
The session driving the work is invisible to `lyx reed status`, cannot be a strand's parent, cannot be addressed later, and dies with the VS Code window rather than surviving in the session the rest of the work runs in.

Why now: reed is implemented and `lyx reed up; lyx reed add --cmd claude --name claude --focus; lyx reed attach` already works today, unchanged — the gap is purely that nothing instructs or automates that chain.
`manifest/roadmap.md`'s Planned item frames this as a convention change rather than a design task, and calls it the fastest of its cluster to land;
landing it also satisfies the watchdog and orchestrator halves of the Someday `reed: born-as-strand` item, leaving only that item's `loom run` operator-attach half as a real code gap.

## Scope

**In:**

- `internal/vscode/config.go` — the generated `tasks.json` becomes the reed launch chain instead of a bare `claude` invocation, expressed as sequenced tasks rather than a shell operator chain.
- `internal/reedcli/add.go` + `internal/reedengine` — a new `--if-absent` flag on `lyx reed add`, making the chain safe to re-run on every `folderOpen`.
- `plugins/ly/skills/ly-supervise/SKILL.md` — the Preconditions section gains the launch convention (this session must itself be a reed strand) and a `$TMUX_PANE`-based self-check;
  the "open `lyx reed attach` in a side terminal" instruction is rewritten, since the operator is now already attached to the session the supervisor runs in.
- Tests: `internal/vscode/config_test.go` (generated-task assertions), `internal/reedcli` (the new flag's CLI surface), `internal/reedengine` (the `--if-absent` decision table), plus a new `smoke`-tagged file `internal/reedcli/smoke_ifabsent_test.go` for the live-server reopen scenario. A new file rather than an addition to `smoke_lifecycle_test.go` or `smoke_resume_test.go`: the scenario is one flag's behaviour end to end, and that package's existing convention is one smoke file per concern.
- `tools/sandbox/SANDBOX-REED-SUITE.md` — a scenario for the repeated `--if-absent` case, per the Sandbox Suite Coverage invariant.
- `internal/ideengine/spawn.go` — resolves the `lyx` binary path (`os.Executable()`) and the `claude` binary path (`exec.LookPath`) and passes both into `WriteConfig`, whose signature gains those parameters.
- Docs in the same commit: `manifest/designs/reed-header-selvage.md` if the Selvage section's `lyx reed add` example needs to stay consistent, `docs/overview.md`'s **ide** bullet, and `manifest/roadmap.md` (the Planned item moves to Done).

**Out:**

- `loom run`'s bare `tmux attach-session` — the operator-attach half of the Someday `born-as-strand` item. Explicitly left as the one remaining code gap.
- The strand-based mailbox/addressing system (Someday) — this task makes the orchestrator addressable *in principle* by making it a strand; it delivers no addressing mechanism.
- Renaming `ly-supervise`, or renaming loom's `run`/`drive`/`step` verbs (Someday).
- Generalizing `ly-supervise` into a Shed-generic watchdog (Someday).
- The header-pane/Selvage split (a separate Planned item) — this task must not depend on Selvage existing.
- Migrating existing worktrees' already-written `.vscode/tasks.json`. `WriteConfig` never clobbers, and that contract stands.
- Any change to `lyx reed up`, `attach`, or `resume` semantics.

## Decisions

### vscode-task-is-the-launch-surface

- Decision: the generated `.vscode/tasks.json` written by `vscode.WriteConfig` is where the launch convention is mechanized. Its `Start Claude` task stops running `claude` directly and instead runs, in order, `lyx reed up`, `lyx reed add --if-absent --cmd claude --name claude --focus`, `lyx reed attach`.
- Rationale: `lyx ide spawn` (`internal/ideengine/spawn.go`) is the one place that materializes a worktree's editor configuration, and its `folderOpen` task is already the de facto "how a session starts here" declaration. Changing it converts every future worktree without asking the operator to memorize a chain.
- **How `lyx` is spelled in the generated file:** the absolute path of the running binary, from `os.Executable()`, resolved once by `ide spawn` and passed into `WriteConfig` as a new parameter — not the bare name `lyx`. If `os.Executable()` fails, fall back to the bare name and carry on; a wrong-but-plausible spelling is better than refusing to write the config.
  **`WriteConfig` owns the fallback.** It substitutes the bare name for any empty path it is handed, and that is the single authoritative rule — `ide spawn` just passes through whatever resolution produced, empty string included, rather than substituting first. One owner, in the layer that writes the file, so the assertion has one obvious home. `ide spawn` may still log a resolution failure; it must not paper over it with its own substitution.
  Reason: today's task runs bare `claude`, one PATH dependency the operator already satisfies. The chain would replace it with three bare `lyx` invocations, and a `folderOpen` task's shell does not inherit a login shell's PATH on macOS or Windows — so an operator who starts VS Code from a desktop icon rather than a terminal would get three red tasks and no Claude, on a machine where `lyx` works fine in every terminal they own. `.vscode/tasks.json` is gitignored and machine-local (`gitignore.Ensure`), and `ide spawn` is itself run by the very binary being stamped, so an absolute path is both correct to bake in and trivially regenerated if the binary moves. Stamping a string is not an `os.Executable()` re-exec and does not touch the CONSTRAINTS rule barring that under `go test`.
  Rejected: the bare name plus a documented PATH prerequisite (moves a platform-dependent failure onto the operator, and it fails at the least debuggable moment — window open, three red tasks, no output anyone reads);
  resolving `lyx` through a login shell in the task itself (a third cross-shell spelling to keep working on both platforms, which is the trap the sequenced-tasks decision exists to avoid).
- **`claude` in `--cmd` is stamped the same way, for the same reason.** `ide spawn` resolves it with `exec.LookPath` and writes the absolute path into the `--cmd` argument, falling back to the bare name `claude` when lookup fails — the identical rule the `lyx` stamp uses, deliberately not a different one.
  Reason: the PATH argument that justifies stamping `lyx` applies verbatim to `claude`, and slightly worse. `reed up` boots the per-hub tmux server from the calling process's environment (routed through `CleanClaudeEnv`, which strips only `CLAUDECODE`/`CLAUDE_CODE_*` and leaves PATH exactly as the task shell had it), the server is long-lived per hub, so that PATH is frozen at first boot for every pane it will ever spawn — and `Cmd` is persisted into `reed.json`, so a bare string that failed once is replayed verbatim by every later `resume`. Stamping both means the chain carries no PATH assumption at all.
- **Concurrent windows on one worktree are accepted, not guarded.** Two VS Code windows open on the same worktree each run the chain; `--if-absent` no-ops on the second, and both then `attach`, producing two tmux clients whose differing terminal sizes clamp the layout to the smaller. That is tmux's ordinary multi-client behaviour, no worse than today's editor terminal plus a side `reed attach`, and the operator resolves it by closing one. No guard, no detection, no new state — a plan writer should not invent one.
- Rejected: a skill-doc-only change (leaves the convention unenforced and the ad hoc terminal as the path of least resistance);
  a new `lyx` verb wrapping the whole chain (a fourth spelling of three verbs that already compose, and the roadmap explicitly frames this as convention, not new surface).

### sequenced-tasks-not-shell-operators

- Decision: express the chain as three separate VS Code tasks plus a `Start Claude` task that names them via `dependsOn` with `"dependsOrder": "sequence"`. No `&&`, `;`, or `windows.command` override.
- Per-task presentation: today's single task carries one `presentation` block (`echo: true`, `reveal: always`, `panel: new`, `internal/vscode/config.go`), and splitting into four means deciding each one's. The `up` and `add` steps are quiet — `reveal: silent`, `panel: shared`, so a successful reopen does not stack panels the operator has to close (`reveal: silent` still surfaces the panel on failure, which is what makes the fail-loud decision below visible). The `attach` step keeps `reveal: always` with `panel: new` **and `focus: true`**: it is the terminal the operator actually works in and types into, and `presentation.focus` defaults to `false` in VS Code, so revealing the panel without it would leave the keyboard in the editor. `up` and `add` carry an explicit `focus: false` rather than relying on the default, so the intent is readable in the generated file and assertable in the test.
- Entry task shape: the `Start Claude` task carries exactly `label`, `dependsOn` (the three steps, in order), `dependsOrder: "sequence"`, and `runOptions.runOn: "folderOpen"`. It has no `type`, no `command`, and no `presentation` of its own — a dependency-only task needs none of them, and today's literal's `"type": "shell"` belongs on the three steps that actually run something.
- Rationale: a `"type": "shell"` task's command string is interpreted by the platform default shell — bash on POSIX, PowerShell on Windows — and the two disagree on operator semantics (`;` does not short-circuit in bash, and older PowerShell has no `&&`). loomyard is a cross-OS repo (`internal/fslink`, the `launch_linux.go`/`launch_windows.go` split), so a portability trap here would be a real bug, not a theoretical one. `dependsOrder: sequence` is relied on for **ordering only**, shell-independent.
Do not assume it aborts the chain on a non-zero exit: VS Code's task runner has historically run the next dependent task regardless of the previous one's exit code, and whether it still does is a claim to verify at implementation time rather than design against.
The chain is safe either way without that guarantee — see the `fail-loud-on-no-hub` decision below for where the actual safety comes from.
- Rejected: a single shell task with `&&` (breaks or silently mis-sequences on Windows PowerShell);
  parallel `windows.command`/`linux.command` overrides (two spellings of one chain to keep in sync).

### add-if-absent-flag

- Decision: `lyx reed add` gains `--if-absent`. With it set, `add` matches `--name` against this worktree's persisted strand table and takes one of the branches below. Without the flag, `add` is byte-for-byte unchanged.
- **`--if-absent` requires `--name`.** Given without it, the command fails up front with a JSON error envelope naming the requirement, before any state is loaded. `--cmd` stays required too, exactly as today (`MarkFlagRequired`, `internal/reedcli/add.go`), even though both matched branches ignore the value it carries — the flag is what the absent-name branch launches, and relaxing it would make the common first-open case fail instead.
  Reason: `resolveStrandName` falls back to `guid[:8]` when `--name` and `--role` are both absent, and the shipped default template is `<ROLE>:<ROUND>:<SHORT_GUID>` (`internal/reedengine/template_posix.yaml`, `template_windows.yaml`), whose guid is minted fresh per invocation. A templated or guid-derived name therefore resolves to something that can never match an existing strand, so `--if-absent` would silently add a duplicate on every reopen — the exact failure the flag exists to prevent, delivered under a flag name promising the opposite. Failing loudly is the only honest option.
  Rejected: permitting a template that happens to contain no `<SHORT_GUID>` (makes the flag's behaviour depend on a config value the caller cannot see from the call site, so the same command silently deduplicates on one machine and stacks duplicates on another);
  accepting the guid case and documenting it as always-adds (a flag that silently does the opposite of its name for the default configuration).
- **Two sets, named once so the branches below cannot disagree.** `matched` is every persisted strand carrying the name, hidden ones included. `candidates` is `matched` minus the hidden ones, in persisted order. The four branches are keyed on both, and are mutually exclusive and exhaustive:

| `matched` | `candidates` | branch |
| --- | --- | --- |
| empty | empty | ordinary add |
| non-empty | non-empty, at least one alive | no-op on the alive one |
| non-empty | non-empty, none alive | relaunch the first |
| non-empty | empty (every match hidden) | no-op, nothing added |

  The last row is the one worth stating explicitly: an empty `candidates` alone does not mean "add", or every reopen of a worktree holding a hidden `claude` strand would stack a fresh duplicate.
- **What each row prints.** Every row emits today's `output.Ok` envelope with a non-empty `guid` and `name`, so no caller has to special-case a branch it cannot see: the add row reports the newly created strand, the alive row the alive candidate, the relaunch row the relaunched candidate, and the hidden-only row the **first matched hidden strand in persisted order**. An empty-`Strand` return (`{"guid":"","name":""}`) is specifically not the contract for any row — the hidden strand is a real, addressable strand, and reporting it is what lets the operator find the thing that suppressed the add.
- Branch **`matched` empty**: behaves exactly as `add` does today, including every display and spec flag.
- Branch **a candidate is alive**: a no-op that prints that strand's guid and name on the ordinary `output.Ok` envelope, with the same key shape today's add emits. It mutates nothing — not `Cmd`, not `ResumeCmd`, not `Parent`, and specifically not `Display`, so the `--focus` the launch chain passes on a reopen never re-focuses or re-arranges a running layout. Nothing is persisted and no layout apply runs.
- Branch **candidates exist but none is alive**: the strand's pane is relaunched through `launchStrandLocked`, the same helper `Resume` uses, passing the **stored** `ResumeCmd`-else-`Cmd` from the persisted strand — exactly `Resume`'s own fallback (`internal/reedengine/lifecycle.go:761-765`), never the `--cmd`/`--resume-cmd` string this invocation happened to supply. The matched strand's persisted spec fields (`Cmd`, `ResumeCmd`, `Parent`, `Display`) are never rewritten from the new flags.
  This branch does persist and does apply, unlike the alive no-op: the new `PaneID` binding is saved immediately after the launch succeeds and before the layout apply, then the layout is re-applied — the same order `AddStrand` and `Resume` both use, and for the same reason both give, that a pane whose binding is not yet persisted would be reaped as untracked if the apply then failed.
- Branch **`matched` non-empty, `candidates` empty** — every strand of that name is hidden (`Display.Anchor == render.AnchorHidden`): a no-op, and specifically not an add. A hidden strand never owns a pane and is dropped before placement (`internal/reedengine/render/types.go`), so treating it as a relaunch candidate would materialize a pane for a strand the operator deliberately hid. `planResumeLaunches` skips hidden strands for exactly this reason (`internal/reedengine/lifecycle.go:152`), and `--if-absent`'s whole claim is to do what `resume` would have done. Surfacing a hidden strand stays `UpdateStrand`'s job, engine-API-only in v1. Not an error either: `--if-absent` asks whether a strand of this name exists, and a hidden one does.
- **Which candidate, when there is more than one:** bare `add` permits duplicate names and `--if-absent` does not change that, so the candidate set can hold two or more strands. The match is the **first alive** candidate in persisted order, else the **first** candidate in persisted order. Persisted order is `st.Strands`' own order, which is append order — so the oldest strand of a name wins, and a repeated `--if-absent` keeps converging on the same one instead of drifting between duplicates.
  Rejected: last match (a stray duplicate created by some other tool would capture the name from the orchestrator that has held it since the worktree opened);
  erroring on ambiguity (the one invocation that must never fail is the `folderOpen` task, and a duplicate name is a state bare `add` is explicitly still allowed to create).
- **Liveness predicate:** a candidate is alive when `PaneID != "" && aliveIDSet(live)[PaneID]` — the same two-part guard `planResumeLaunches` uses (`internal/reedengine/lifecycle.go:148`), not `aliveIDSet` membership alone. The empty-`PaneID` half is not a corner case but the commonest relaunch path there is: `Up` calls `clearAllPaneBindings` on a freshly booted server (`lifecycle.go:675-679`), so after a machine restart the persisted orchestrator arrives with no binding at all, and a predicate that only consulted the id set would index on `""` and reach the wrong branch.
  The set half is `aliveIDSet`, not `liveIDSet` (`internal/reedengine/apply.go`). The two are genuinely different sets — `liveIDSet` is "present in the window, dead panes included", `aliveIDSet` is "present AND not dead" — and `planResumeLaunches` deliberately takes `aliveIDSet` so a strand bound to a dead-but-present pane (the sole dead pane tmux keeps) is relaunched rather than mistaken for live (`lifecycle.go:748-750`). `--if-absent` matches that choice exactly: it is the same question `Resume` asks, and answering it differently would make a reopen silently skip a dead orchestrator.
- Rationale: the `folderOpen` task re-runs on every VS Code window open, and `AddStrand` (`internal/reedengine/strand.go:288`) performs no name deduplication — so without this, reopening a worktree stacks a second, third, fourth orchestrator Claude in the same session. `lyx reed up` is documented as substrate-only and never relaunches a strand command; `lyx reed resume` replays only strands that already exist, so it cannot cover the first open. `--if-absent` is the smallest addition that makes the chain genuinely idempotent across the "session still live", "pane died", and "machine rebooted, session gone" reopens.
  Reusing the stored command rather than the fresh one keeps a name-matched relaunch identical to what `resume` would have done for the same strand, so the two entry points can never diverge into two different ideas of what that strand runs.
- Rejected: accepting duplicate strands (turns a normal reopen into a resource leak and a confusing pane stack);
  a new `lyx reed ensure` verb (a second verb for one flag's worth of behaviour, and it would duplicate `add`'s whole flag surface);
  making bare `add` reject duplicate names (turns a harmless reopen into a red error task and breaks every legitimate multi-strand add that reuses a role/round name);
  having the relaunch branch adopt the invocation's fresh `--cmd` and overwrite the persisted spec (an `add` that silently rewrites an existing strand's recorded command is an update verb wearing `add`'s name, and it would let a stale task file quietly redefine a strand the operator configured by hand — `UpdateStrand` is where a deliberate mutation belongs, and it is engine-API-only in v1);
  classifying with `liveIDSet` (a dead-but-present pane would read as live, so the one case most worth recovering — the orchestrator's pane died while the session stayed up — would be the one case `--if-absent` refused to fix).

### orchestrator-strand-name

- Decision: the orchestrator strand is named `claude`, per the roadmap item's literal command.
- Rationale: one worktree hosts exactly one orchestrator session; the name is what `--if-absent` keys on, so it must be stable and predictable rather than derived from the slug or a guid. A short fixed word also reads well in reed's rendered layout.
- Rejected: naming it after the worktree slug (already visible in the tmux session name and the VS Code title bar — redundant, and it makes `--if-absent` depend on a derivation the operator would have to reproduce by hand);
  `orchestrator` (longer, and the roadmap's Someday `born-as-strand` item records that "repo-orchestrator" is not yet a defined concept — adopting the word here would presume that definition).

### supervisor-is-the-orchestrator-strand

- Decision: `/ly:ly-supervise` spawns nothing of its own. The session running the skill *is* the orchestrator strand created by the launch chain. The skill's Preconditions gain a check that this is so, and the instruction to open `lyx reed attach` in a side terminal is replaced by the statement that the operator is already inside the session — the supervisor's pane and every pane loom spawns are siblings in it.
- Rationale: the roadmap groups "ly-supervise + orchestrator" as one item precisely because they are one session in practice: the supervisor loop runs in the orchestrator's own Claude. A second strand for the supervisor would be a second session to talk to with nothing to say to it.
- Rejected: the skill issuing its own `lyx reed add` for a dedicated supervisor strand (a Claude session cannot usefully spawn the session it is already running in);
  keeping the side-terminal advice unchanged (it is now wrong — the side terminal and the supervisor would be two attaches to the same session).

### strand-self-check-via-tmux-pane

- Decision: the skill's precondition check is: read `$TMUX_PANE` from the environment, run `lyx reed status`, and confirm that pane id appears among the tracked strands. It has **three** outcomes, not two.
  - `$TMUX_PANE` set and tracked → confirmed a strand; proceed silently.
  - `$TMUX_PANE` set but not tracked → this session is in a pane reed does not track; tell the operator it was not launched as a strand, name the launch chain, and offer the numbered choice of relaunching that way or proceeding unsupervised-by-reed.
  - `$TMUX_PANE` unset → **unconfirmed, not failed.** Say exactly that: the check could not run, the session may or may not be a strand, and proceeding is fine. Do not tell the operator to relaunch.
- Rationale: reed sets no `LYX_REED_*` marker in a strand's pane environment, but tmux sets `TMUX_PANE`, and `lyx reed status` is already the read-only cross-reference of tracked strands against live panes (`internal/reedcli/status.go`) that reports each strand's pane id. The check therefore needs no new code at all — it composes two facts that already exist.
- **Why the third outcome exists:** on Windows reed drives **psmux**, a tmux-compatible port that identifies itself as tmux (`internal/reedengine/doc.go`, `template_windows.go`), and nothing in this repo verifies that psmux exports `TMUX_PANE` into a pane's environment. A two-outcome check would therefore tell every correctly-launched Windows operator to relaunch — a precondition that misfires on an entire platform is worse than no precondition. The unset branch is the honest answer to "I cannot tell", and it is also correct for any future substrate that declines to export the variable.
  If an implementer confirms psmux does export it, nothing here changes: the check simply reaches the exact branch on both platforms, and the unset branch stays as the correct answer for the case where it is genuinely absent. Confirming it is a one-line check, worth doing, but the design must not depend on the answer.
- Rejected: adding a `LYX_REED_STRAND` env var to launched panes (new engine surface for a check that already composes;
  reed's env hygiene is deliberate and worth not perturbing for a doc-level precondition);
  no check at all (the whole point of the convention is that it holds, and a precondition nobody verifies decays).

### no-migration-of-existing-worktrees

- Decision: `WriteConfig`'s never-clobber contract is untouched. Existing worktrees keep their bare-`claude` task. The manual upgrade — delete `.vscode/tasks.json` and re-run `lyx ide spawn`, or edit it by hand — is documented in the `ly-supervise` skill and in the `docs/overview.md` **ide** bullet.
- Rationale: `.vscode/` is gitignored and operator-editable by design; `WriteConfig` writes each file only when absent specifically so operator edits survive. A migration path that rewrites it would have to distinguish "still the generated default" from "the operator changed it", which is exactly the kind of adoption seam `internal/reedengine/spawn.go` records two live bugs against in a different context.
- Rejected: a `--force` reconcile flag on `ide spawn` (new surface, and it would clobber deliberate operator edits);
  clobbering unconditionally when the file's content matches the previous generated default byte-for-byte (a content fingerprint that must be maintained forever, for a one-time upgrade a single operator can do in ten seconds).

### fail-loud-on-no-hub

- Decision: if `lyx reed up` fails, no Claude starts. No fallback to a bare `claude` invocation.
- Rationale: `lyx ide spawn` derives the worktree path through `fabricengine.WorktreePath`, so it only ever writes this config inside a hub worktree where `reed up` is expected to work. A silent fallback would mask a broken hub as "Claude just didn't become a strand today", which is precisely the invisible-session state this task exists to eliminate. reed's JSON error envelope names its own remedy.
- A missing binary is a distinct failure from a failing `reed up`, and it fails the same way: if the stamped path no longer resolves, every step of the chain fails with the shell's own not-found error and no Claude starts. There is still no fallback — a bare `claude` rescue here would recreate the untracked session for the one operator whose lyx install is broken, which is the last person who should get a silently different setup. The remedy is two steps, not one: delete `.vscode/tasks.json`, then re-run `lyx ide spawn` — `WriteConfig` never clobbers an existing file (`internal/vscode/config.go`), so a bare re-run restamps nothing. This is the same manual step the no-migration decision documents, for the same reason.
- **A stale stamped `claude` path already inside `reed.json` is not fixed by any restamp, and that is accepted.** The stamp reaches `Cmd` when the strand is first added, `Cmd` is persisted, and the relaunch branch deliberately never rewrites it — so a `claude` binary that has since moved is replayed verbatim by every later `--if-absent` relaunch and by `resume`. The operator action is `lyx reed remove <guid>` followed by a fresh `add` (or `lyx reed down`, which clears the worktree's strand state wholesale). No detection, no self-healing rewrite: a command that silently rewrote a persisted strand's `Cmd` is exactly what the relaunch branch's own rejected-alternative rules out.
- The safety here comes from each downstream command's own guard, not from VS Code aborting the sequence. `AddStrand` pre-flights `requireSessionLocked` and fails with reed's friendly `no reed session; run "lyx reed up"` envelope, and `attach` pre-flights `c.eng.Status()` on the envelope before any terminal handover — so even if the runner does fire the later tasks after a failed `up`, the end state is the intended one: no strand registered, no pane, and no bare `claude` anywhere. The implementation should not add a guard of its own to compensate for the runner's behaviour either way.
- Rejected: a fallback task that runs bare `claude` when the chain fails (reintroduces the untracked session as a hidden default and makes the convention unenforceable).

## Technical context

**`internal/vscode/config.go`** — `WriteConfig(worktreeDir, relpath, slug, color string) error` builds both `settings.json` and `tasks.json` as Go `map[string]any` literals and `json.MarshalIndent`s them, writing each only when absent, then calls `gitignore.Ensure(dir, ".vscode/")`.
The current tasks literal is a single `Start Claude` shell task with `"command": "claude"`, `runOptions.runOn: "folderOpen"`, a `presentation` block (`echo`/`reveal: always`/`panel: new`) and `isBackground: false`.
Its only caller is `internal/ideengine/spawn.go`'s `Spawn`, which picks a colour, writes the config, then launches VS Code through the injectable `CodeLauncher` seam.

**`internal/reedcli/add.go`** — a thin flag-to-spec mapper over `reedengine.AddStrand`. It validates only the closed `--anchor` vocabulary (`below-parent`/`hidden`, with `own-window` rejected as deferred) and builds a `reedengine.AddSpec`; guid generation, worktree stamping and name resolution all belong to the engine. `--cmd` is `MarkFlagRequired`. The success envelope is `{"guid": …, "name": …}` via `output.Ok`.

**`internal/reedengine/strand.go`** — `AddStrand` (line 288) takes the op lock, calls `requireSessionLocked` (so `up` must precede `add`), `loadOrInitStateLocked`, `addStrandLocked`, persists immediately after launch succeeds and before the layout apply (deliberately, so a failed apply leaves a tracked strand rather than an untracked orphan pane), then `reconcileApplyPersistLocked`.
`resolveStrandName` (line 123) is the name resolution `--if-absent` must reuse verbatim: `NameOverride` wins, else the template from role/round, else a short guid.
There is no name uniqueness check anywhere in this path.

**`internal/reedengine/spawn.go`** — `launchStrandLocked` is the shared pane-launch helper every strand-realizing path composes (`AddStrand`, `UpdateStrand`'s hidden→visible surface, and `Resume`'s replay). The `--if-absent` relaunch branch must go through it, not build its own pane.
This file's `planPaneTarget` doc comment records the two live findings (R4-F5 and M16) that killed pane *adoption* — relevant as prior art: `--if-absent` must key on reed's own strand table, never on inspecting a pane to guess what it is.

**`internal/reedengine/lifecycle.go`** — `Resume` (line 702) calls `ensureServerAndSessionLocked` itself, clears all pane bindings on a server rebirth (reborn sessions reuse pane ids, so a stale binding would look live), rebuilds the header pane, then reconciles and replays. `Resume` therefore subsumes `up`; `add` does not, which is why the task chain keeps `lyx reed up` first.

**`internal/reedengine/apply.go`** — holds the two liveness predicates `--if-absent` must choose between. `liveIDSet` is every pane id present in the window, dead-but-remain-on-exit included, and exists because `select-layout` must enumerate every pane tmux still holds. `aliveIDSet` is present-and-not-dead, and is what resume-planning and `Status` use so a strand bound to a dead pane is not mistaken for a live one. `--if-absent` uses `aliveIDSet`.

**`internal/reedcli/status.go`** — `status` is read-only: it cross-references the live pane set and reports guid, name, pane id and liveness per tracked strand, without reconciling. This is what the skill's `$TMUX_PANE` self-check reads.

**`plugins/ly/skills/ly-supervise/SKILL.md`** — the Preconditions section currently states that cwd must be the task worktree root (because `lyx loom step` derives everything from cwd and `lyxcwd.Resolve` requires a worktree root) and tells the operator to open `lyx reed attach` in a side terminal as a second, independent watching layer. The skill's "Operator choices" section already mandates numbered text lists for every choice — the new precondition's fallback prompt must follow it.

**`manifest/designs/reed-header-selvage.md`** — its Selvage section already cites `lyx reed add` as the thing you would type in the control terminal to spawn a new Claude. Check for consistency; do not build on Selvage, which is a separate Planned item.

## Constraints

From `CONSTRAINTS.md`:

- **CLI / Cobra Invariant** — `--if-absent` adds a flag, not a command, so no new `Command()`/`RunCLI` seam is involved. The existing rules still bind the changed code: non-empty `Short` (unchanged), errors as JSON one object per line via `internal/output`, and `RunE` checking `clihelp.ShouldAbort` first. `reedcli` imports `reedengine`; the engine never imports cli or cobra, so the `--if-absent` decision logic belongs in `reedengine`, with `add.go` mapping the flag onto `AddSpec` only.
- **Told-Geometry Invariant** — `reedengine` is a bound package: it is handed its paths and derives none. Nothing in this change may reach for `lyxcwd`.
- **Live-Substrate Spawn Observability** — the `--if-absent` relaunch branch starts a real OS process, so it logs its spawn via `internal/logger` at `Info`. The no-op branch starts nothing and logs nothing at `Info`.
- **Test Tier Purity Invariant** — untagged test files may not call `exec.Command`/`gitexec` or spawn. The `--if-absent` decision table belongs in untagged unit tests against the engine's state; anything that needs a live tmux server goes in `smoke`- or `integration`-tagged files.
- **Sandbox Suite Coverage** — `tools/sandbox/SANDBOX-REED-SUITE.md` already covers `reed add` behaviours by scenario; a new flag that changes what a repeated `add` does warrants a scenario there.
- **Documentation Lifecycle** — per `CLAUDE.md`, a change to observable CLI behaviour updates its docs in the same commit, and the roadmap moves only on completing a planned item, which this is.
- **Markdown Link Integrity** — any new cross-reference between the skill, the design docs and the roadmap must resolve.

Discovered during exploration:

- `AddStrand` requires a live session (`requireSessionLocked`), so `--if-absent` cannot be used to bootstrap a session — `lyx reed up` stays first in the chain.
- `.vscode/` is gitignored via `gitignore.Ensure`, so the generated task file is machine-local: nothing about this change propagates to another clone except through `WriteConfig` running there too.

## Testing

**`internal/vscode`** — TDD candidate, and the clearest one in this task. `config_test.go`'s `TestWriteVSCodeConfigCreatesFilesWhenAbsent` currently asserts the first task's label is `Start Claude`; extend it to assert the full generated shape: that the launch chain exists as distinct tasks, that the entry task sequences them (`dependsOn` plus `dependsOrder: sequence`), that the add step carries `--if-absent` and the `--name` the convention fixes, that each task's `presentation` matches the split above (quiet and `focus: false` for `up`/`add`, `reveal: always` + `panel: new` + `focus: true` for `attach`), that the entry task carries exactly its four keys with no `type`/`command`/`presentation`, that each step's command is the stamped `lyx` path the caller passed rather than the bare name, that the add step's `--cmd` carries the stamped `claude` path, and that `folderOpen` still triggers the entry task.
Cover both fallbacks against `WriteConfig` itself, which owns the rule: an empty `lyx` path and an empty `claude` path each fall back to their bare name and still write a valid file.
`TestWriteVSCodeConfigDoesNotClobber` must keep passing untouched — it is the regression guard for the no-migration decision.

**`internal/reedengine`** — the `--if-absent` decision table, as untagged unit tests over engine state with no live tmux: one case per row of the branch table — `matched` empty → ordinary add; a candidate is alive → no-op returning that strand's guid and name; candidates exist but none alive → relaunch through the shared helper; `matched` non-empty with `candidates` empty → no-op with nothing added and no pane created; and `--if-absent` off → today's behaviour, including the duplicate-name add that must still be permitted.
Cover the empty-`PaneID` case explicitly: a persisted candidate whose binding was cleared (what `Up` leaves behind after a server reboot) classifies as not alive and relaunches — the commonest real path, and the one a set-membership-only predicate gets wrong.
Cover the candidate-selection rule as its own case: two candidates where the second is the alive one selects the alive one; two where neither is alive selects the first in persisted order; and a hidden strand sharing the name with a not-alive non-hidden one relaunches the non-hidden one rather than no-opping — the case the candidate-set definition exists to settle.
Cover that `--if-absent` without `--name` is rejected before any state is loaded, and that the rejection names the requirement rather than surfacing a generic error.
Cover the predicate directly: a strand bound to a pane that is present but `Dead` classifies as not-alive and relaunches — this is the case that distinguishes `aliveIDSet` from `liveIDSet`, and the one a wrong predicate would silently skip.
Cover the no-mutation guarantees on both matched branches: the no-op leaves `Display` (focus and anchor included) untouched even when the invocation passes `--focus`, and the relaunch passes the stored `ResumeCmd`-else-`Cmd` while leaving `Cmd`/`ResumeCmd`/`Parent`/`Display` as persisted, even when the invocation supplies different ones.

**`internal/reedcli`** — the flag exists, defaults off, maps onto the spec, and the success envelope shape is unchanged in the no-op case (an operator script reading `guid` must not have to special-case it).
Also: `--cmd` is still rejected as missing under `--if-absent`, and `--if-absent` without `--name` produces the JSON error envelope on the same `output.Err` path every other CLI-level rejection uses.
Assert the envelope carries a non-empty `guid` and `name` on every branch, the hidden-only row included — that row is the one where an empty-`Strand` return would slip through unnoticed.

**Smoke/integration** (tagged) — the real reopen scenario against a live server: `up`, `add --if-absent`, `add --if-absent` again, assert exactly one strand and one pane; then kill the strand's pane and `add --if-absent` a third time, assert the same strand is alive again and no second strand appeared.
Note for whoever writes this: tmux keeps a session's sole pane on the screen after its process dies, so the third call must be asserted against `aliveIDSet`'s question ("present and not dead"), not against the pane merely still existing — a test that checks only for a pane would pass without the relaunch ever happening.

**Sandbox** — a scenario in `tools/sandbox/SANDBOX-REED-SUITE.md` for the repeated-`--if-absent` case, written in that file's existing **Watch:** style.

**Not tested mechanically** — the skill's `$TMUX_PANE` precondition and the VS Code `folderOpen` trigger are operator-facing behaviour with no test harness in this repo. Both are exercised by opening a worktree and reading `lyx reed status`.

## Q&A log

- **Q:** Where is the launch convention mechanized — skill doc only, the generated VS Code task, or a new wrapper verb? **A:** [auto-pick] The generated VS Code task in `internal/vscode/config.go`, plus the skill documenting it. **Why:** `lyx ide spawn`'s `folderOpen` task is already the de facto declaration of how a session starts in a worktree; a doc-only change leaves the ad hoc terminal as the easier path, and a wrapper verb is a fourth spelling of three verbs that already compose.
- **Q:** How does the chain survive the `folderOpen` task re-running on every window open, given `AddStrand` does not deduplicate names? **A:** [auto-pick] Add an `--if-absent` flag to `lyx reed add`. **Why:** `up` is substrate-only and never relaunches a command, and `resume` cannot cover the first-ever open; `--if-absent` is the smallest change that makes the chain idempotent for both the live-session and rebooted-machine reopens.
- **Q:** How is the chain expressed in `tasks.json` — one shell task with operators, or sequenced tasks? **A:** [auto-pick] Three tasks plus an entry task using `dependsOn` with `dependsOrder: sequence`. **Why:** a shell task's command string is interpreted by bash on POSIX and PowerShell on Windows, which disagree on `;` and `&&`; loomyard is cross-OS, so the sequencing must come from VS Code, not the shell.
- **Q:** Does the orchestrator task keep `runOn: folderOpen`? **A:** [auto-pick] Yes. **Why:** it is the existing convention and the reason the worktree is usable the moment it opens; `--if-absent` is what makes keeping it safe.
- **Q:** What name does the orchestrator strand get? **A:** [auto-pick] `claude`, per the roadmap item's literal command. **Why:** `--if-absent` keys on the name, so it must be fixed and predictable; the slug is already in the session name and title bar, and `orchestrator` would presume a concept the roadmap records as not yet defined.
- **Q:** Does `/ly:ly-supervise` spawn its own strand, or is the supervising session the orchestrator strand? **A:** [auto-pick] It is the orchestrator strand; the skill spawns nothing. **Why:** the supervisor loop runs inside the orchestrator's own Claude — a separate strand would be a second session with nothing to say to it.
- **Q:** How does the skill verify it is running as a strand? **A:** [auto-pick] Read `$TMUX_PANE` and cross-reference it against `lyx reed status`. **Why:** tmux already sets the variable and `status` already reports each strand's pane id, so the check needs no new code; adding a `LYX_REED_*` marker would be new engine surface for a doc-level precondition.
- **Q:** What happens to worktrees that already have a bare-`claude` `tasks.json`? **A:** [auto-pick] Nothing automatic — `WriteConfig`'s never-clobber contract stands and the manual upgrade is documented. **Why:** `.vscode/` is operator-editable by design, and any auto-rewrite would have to guess whether the file is still the generated default or something the operator changed.
- **Q:** Should the task chain fall back to a bare `claude` when `lyx reed up` fails? **A:** [auto-pick] No — fail loud with reed's own error envelope. **Why:** `ide spawn` only writes this config inside a hub worktree where `reed up` is expected to work, and a fallback would silently restore the untracked session this task exists to eliminate.
- **Q:** Does the `loom run` attach half of the Someday `born-as-strand` item come along? **A:** [auto-pick] No, it stays out of scope. **Why:** the roadmap explicitly scopes this item to the watchdog and orchestrator halves and names the operator-attach half as the one remaining code gap afterwards; pulling it in would also entangle the undecided "repo-orchestrator" definition.


### From _mill/plan/00-overview.md


```yaml
task: "Launch ly-supervise and orchestrator via lyx reed add"
slug: "ly-supervise-reed-add"
approved: true
started: "20260918-171126"
parent: "main"
root: ""
verify: null
discussion_sha: 0f65cdd1555b2c978b976d96e49e10541acae4a6
```

### From _mill/plan/01-reed-if-absent.md


```yaml
task: "Launch ly-supervise and orchestrator via lyx reed add"
batch: "reed-if-absent"
number: 1
cards: 5
verify: go test ./internal/reedengine/... ./internal/reedcli/... && go test -tags integration ./internal/reedcli/... && go vet -tags smoke ./internal/reedcli/
depends-on: []
```



- **Edits:**
  - `internal/reedengine/strand.go`
  - `internal/reedengine/strand_test.go`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `internal/reedengine/strand.go`
  - `internal/reedengine/strand_test.go`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `internal/reedcli/add.go`
  - `tools/sandbox/SANDBOX-REED-SUITE.md`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `internal/reedcli/cli_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/reedcli/smoke_ifabsent_test.go`
- **Deletes:** none

### From _mill/plan/02-vscode-launch-chain.md


```yaml
task: "Launch ly-supervise and orchestrator via lyx reed add"
batch: "vscode-launch-chain"
number: 2
cards: 2
verify: go test ./internal/vscode/... ./internal/ideengine/...
depends-on: []
```



- **Edits:**
  - `internal/vscode/config.go`
  - `internal/vscode/config_test.go`
  - `internal/ideengine/spawn.go`
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `internal/ideengine/spawn_test.go`
- **Creates:** none
- **Deletes:** none

### From _mill/plan/03-skill-and-roadmap.md


```yaml
task: "Launch ly-supervise and orchestrator via lyx reed add"
batch: "skill-and-roadmap"
number: 3
cards: 2
verify: go test ./internal/lyxcwd/ -run TestEnforcement_MarkdownLinks
depends-on: [1, 2]
```



- **Edits:**
  - `plugins/ly/skills/ly-supervise/SKILL.md`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `manifest/roadmap.md`
  - `manifest/designs/reed-header-selvage.md`
- **Creates:** none
- **Deletes:** none

## Conflicting files

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
5. Run `git -C /home/knatte/Code/loomyard/wts/ly-supervise-reed-add add <file>` to stage the resolved file.
6. For modify/delete (DU) conflicts: if Task intent above lists this file under a batch's `Deletes:`, run `git -C /home/knatte/Code/loomyard/wts/ly-supervise-reed-add rm <file>` instead of editing;
   that stages the intentional deletion.
7. For UD conflicts — files this branch **modified** that the parent branch **deleted**: do not silently keep the modification.
   Instead: a. Run `git log --diff-filter=D --oneline MERGE_HEAD -- <file>` to find the deletion commit on the parent. b. Run `git show <deletion-commit>` to inspect context. c. If the deletion commit message mentions a replacement file (e.g. "replaced by", "moved to", "consolidated into"),
   or the commit also adds a file in the same directory with overlapping content: stage the deletion — `git -C /home/knatte/Code/loomyard/wts/ly-supervise-reed-add rm <file>`. d. If detection is inconclusive: report `{"status":"stuck","stuck_type":"logic","reason":"modify/delete conflict on <file>: cannot determine if parent deletion is a replacement -- operator must decide"}` and halt.
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
Use `git -C /home/knatte/Code/loomyard/wts/ly-supervise-reed-add` for any git commands;
do not `cd`.
Worktree cwd is `/home/knatte/Code/loomyard/wts/ly-supervise-reed-add`.
