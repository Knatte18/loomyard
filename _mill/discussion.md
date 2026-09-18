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
- Tests: `internal/vscode/config_test.go` (generated-task assertions), `internal/reedcli` (the new flag's CLI surface), `internal/reedengine` (the `--if-absent` decision table).
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
- Rejected: a skill-doc-only change (leaves the convention unenforced and the ad hoc terminal as the path of least resistance);
  a new `lyx` verb wrapping the whole chain (a fourth spelling of three verbs that already compose, and the roadmap explicitly frames this as convention, not new surface).

### sequenced-tasks-not-shell-operators

- Decision: express the chain as three separate VS Code tasks plus a `Start Claude` task that names them via `dependsOn` with `"dependsOrder": "sequence"`. No `&&`, `;`, or `windows.command` override.
- Rationale: a `"type": "shell"` task's command string is interpreted by the platform default shell — bash on POSIX, PowerShell on Windows — and the two disagree on operator semantics (`;` does not short-circuit in bash, and older PowerShell has no `&&`). loomyard is a cross-OS repo (`internal/fslink`, the `launch_linux.go`/`launch_windows.go` split), so a portability trap here would be a real bug, not a theoretical one. `dependsOrder: sequence` gives ordering and failure-stops-the-chain semantics from VS Code itself, shell-independent.
- Rejected: a single shell task with `&&` (breaks or silently mis-sequences on Windows PowerShell);
  parallel `windows.command`/`linux.command` overrides (two spellings of one chain to keep in sync).

### add-if-absent-flag

- Decision: `lyx reed add` gains `--if-absent`. With it set, `add` resolves the strand name exactly as today (`--name` wins, else the template from `--role`/`--round`, else a guid), and then: if a strand of that resolved name exists and is live, it is a no-op that prints the existing strand's guid and name on the ordinary success envelope; if it exists but is not live, its pane is relaunched through the same `launchStrandLocked` path `resume` uses; if no such name exists, it behaves exactly as `add` does today. Without the flag, `add` is byte-for-byte unchanged.
- Rationale: the `folderOpen` task re-runs on every VS Code window open, and `AddStrand` (`internal/reedengine/strand.go:288`) performs no name deduplication — so without this, reopening a worktree stacks a second, third, fourth orchestrator Claude in the same session. `lyx reed up` is documented as substrate-only and never relaunches a strand command; `lyx reed resume` replays only strands that already exist, so it cannot cover the first open. `--if-absent` is the smallest addition that makes the chain genuinely idempotent across both the "session still live" and "machine rebooted, session gone" reopens.
- Rejected: accepting duplicate strands (turns a normal reopen into a resource leak and a confusing pane stack);
  a new `lyx reed ensure` verb (a second verb for one flag's worth of behaviour, and it would duplicate `add`'s whole flag surface);
  making bare `add` reject duplicate names (turns a harmless reopen into a red error task and breaks every legitimate multi-strand add that reuses a role/round name).

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

- Decision: the skill's precondition check is: read `$TMUX_PANE` from the environment, run `lyx reed status`, and confirm that pane id appears among the tracked strands. If `$TMUX_PANE` is unset, or is set but not tracked, the skill tells the operator this session was not launched as a strand, names the launch chain, and offers the numbered choice of relaunching that way or proceeding unsupervised-by-reed.
- Rationale: reed sets no `LYX_REED_*` marker in a strand's pane environment, but tmux itself sets `TMUX_PANE`, and `lyx reed status` is already the read-only cross-reference of tracked strands against live panes (`internal/reedcli/status.go`) that reports each strand's pane id. The check therefore needs no new code at all — it composes two facts that already exist.
- Rejected: adding a `LYX_REED_STRAND` env var to launched panes (new engine surface for a check that already composes;
  reed's env hygiene is deliberate and worth not perturbing for a doc-level precondition);
  no check at all (the whole point of the convention is that it holds, and a precondition nobody verifies decays).

### no-migration-of-existing-worktrees

- Decision: `WriteConfig`'s never-clobber contract is untouched. Existing worktrees keep their bare-`claude` task. The manual upgrade — delete `.vscode/tasks.json` and re-run `lyx ide spawn`, or edit it by hand — is documented in the `ly-supervise` skill and in the `docs/overview.md` **ide** bullet.
- Rationale: `.vscode/` is gitignored and operator-editable by design; `WriteConfig` writes each file only when absent specifically so operator edits survive. A migration path that rewrites it would have to distinguish "still the generated default" from "the operator changed it", which is exactly the kind of adoption seam `internal/reedengine/spawn.go` records two live bugs against in a different context.
- Rejected: a `--force` reconcile flag on `ide spawn` (new surface, and it would clobber deliberate operator edits);
  clobbering unconditionally when the file's content matches the previous generated default byte-for-byte (a content fingerprint that must be maintained forever, for a one-time upgrade a single operator can do in ten seconds).

### fail-loud-on-no-hub

- Decision: if `lyx reed up` fails, the task chain fails and no Claude starts. No fallback to a bare `claude` invocation.
- Rationale: `lyx ide spawn` derives the worktree path through `fabricengine.WorktreePath`, so it only ever writes this config inside a hub worktree where `reed up` is expected to work. A silent fallback would mask a broken hub as "Claude just didn't become a strand today", which is precisely the invisible-session state this task exists to eliminate. reed's JSON error envelope names its own remedy.
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

**`internal/vscode`** — TDD candidate, and the clearest one in this task. `config_test.go`'s `TestWriteVSCodeConfigCreatesFilesWhenAbsent` currently asserts the first task's label is `Start Claude`; extend it to assert the full generated shape: that the launch chain exists as distinct tasks, that the entry task sequences them (`dependsOn` plus `dependsOrder: sequence`), that the add step carries `--if-absent` and the `--name` the convention fixes, and that `folderOpen` still triggers the entry task.
`TestWriteVSCodeConfigDoesNotClobber` must keep passing untouched — it is the regression guard for the no-migration decision.

**`internal/reedengine`** — the `--if-absent` decision table, as untagged unit tests over engine state with no live tmux: name absent → ordinary add; name present and live → no-op returning the existing guid and name; name present and not live → relaunch through the shared helper; and `--if-absent` off → today's behaviour, including the duplicate-name add that must still be permitted.
Cover that the resolved name is what matches, not the raw `--name` (an `--if-absent` add driven by `--role`/`--round` must match a strand of the same templated name).

**`internal/reedcli`** — the flag exists, defaults off, maps onto the spec, and the success envelope shape is unchanged in the no-op case (an operator script reading `guid` must not have to special-case it).

**Smoke/integration** (tagged) — the real reopen scenario against a live server: `up`, `add --if-absent`, `add --if-absent` again, assert exactly one strand and one pane; then kill the pane and `add --if-absent` a third time, assert the strand is live again and no second strand appeared.

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
