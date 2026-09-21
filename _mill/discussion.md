# Discussion: Spawned agent panes resolve the spawning lyx binary

```yaml
task: Spawned agent panes resolve the spawning lyx binary
slug: lyx-bin-pane-path
status: discussing
parent: main
```

## Problem

Every Go-side spawn in lyx's execution chain re-execs `os.Executable()`:
`internal/battencli`'s wire spawns `lyx loom start` with it (`internal/battencli/wire.go:126`), `internal/loomcli` spawns the detached `lyx loom run` and the watchdog daemon with it (`internal/loomcli/start.go:144`, `internal/reedengine/spawnwatchdog.go:49`), and `internal/boardengine`, `internal/fabricengine` and `internal/ideengine` do the same.
So a dev build under `.dev-bin` drives itself all the way down the Go chain.
The LLM agents those rows spawn are the one exception.
Their prompts, stencils and skills say "run `lyx …`", and inside a reed pane that resolves through the pane shell's `PATH` — which on a developer machine is the production install (`~/go/bin/lyx` or `C:\Code\tools\bin\lyx.exe`), not `.dev-bin/lyx`.

**Why now:** observed live in crucible round `fable-high-r2` of batten.
batten and loom ran the dev binary throughout, while the Discussion/Plan/Webster agents inside the child worktree ran a different, newer build resolved from `PATH`.
That build reconciled the child's `reed.yaml`/`loom.yaml` to its own template and committed the rewrite on a webster weft commit, leaving the dev binary unable to load the child's reed config afterwards.
The dev build was only partially the binary under test, and nothing reported the mismatch.
The sandbox suite already solves the same problem for its own direct child (`tools/sandbox/resolve.go`'s `prependPath`), which is precisely why the gap only shows up in the crucible/manual flow, where nobody prepends anything.

## Scope

**In:**

- A new pane-environment prelude composed at reed's single strand pane-launch chokepoint (`internal/reedengine`), prepending `filepath.Dir(os.Executable())` to the pane's `PATH` and exporting `LYX_BIN` to the absolute `os.Executable()` path.
- Three new generic methods on the `internal/shell` `Shell` seam (POSIX + pwsh implementations): an `export KEY=VALUE` statement, a `PATH`-prepend statement, and a statement joiner.
- A new `CONSTRAINTS.md` clause recording the invariant, plus an enforcement test in `internal/reedengine` that keeps every pane-launch site routed through the chokepoint.
- Hermetic unit tests for both shell dialects and for the reed chokepoint composition.
- Docs in the same commit: `internal/reedengine/doc.go`, `internal/shell/shell.go`, `CONSTRAINTS.md`, `crucible/README.md`, `docs/sandbox-howto.md`, `tools/sandbox/SANDBOX-REED-SUITE.md` and `tools/sandbox/SANDBOX-SHUTTLE-SUITE.md`'s pre-condition sections — `SANDBOX-REED-WATCH-SUITE.md` is **out**: it carries the same boilerplate PATH pre-condition line, but the watch suite drives the resize watch loop and spawns no agent pane, so the new checks have nothing to exercise there.

**Out:**

- No stencil, skill, prompt or contract edits.
  The whole point of the `PATH` form is that no prompt has to remember `$LYX_BIN`.
- No change to `tools/sandbox/resolve.go`'s existing `prependPath` or to the `Dev/Prod Binary Separation` invariant — the sandbox launcher keeps prepending `.dev-bin` to its own child's environment, and the new reed mechanism agrees with it rather than replacing it.
- No `LYX_BIN` export added to the watchdog daemon spawn or the detached `lyx loom run` spawn (see the "Detached spawns stay untouched" Decision).
- No Selvage-pane or `new-session` first-pane prelude (see the "Chokepoint is the strand launch only" Decision).
- No new tmux subcommand, flag or capability requirement — `requiredSubcommands` (`internal/reedengine/probe.go`) is unchanged.
- **No change to how a pane's shell is started.** `launchStrandLocked` keeps splitting with no trailing command; `split-window`'s and `new-session`'s argv are untouched, and so is every pane's profile-sourcing and inherited `PATH` (see `pane-start-mode-stays-untouched`).
- No dialect selector, no `validateToldPaneShell`, no `e.cfg.Shell` involvement of any kind — `e.cfg.Shell` does not govern a strand pane, so it is not the prelude's dialect source.
- No change to the two `shell.ForGOOS()` launch-command builders (`internal/shuttleengine/claudeengine/claudeengine.go:102`, `internal/loomcli/sharedbootstrap.go:259`), and no dialect threading through `shuttleengine.ReedOps` or `Engine.Prepare` — the prelude simply uses the same selector they do.
- No fix for the pre-existing `default-shell`-vs-`ForGOOS()` assumption (a `fish` `$SHELL` already breaks the launch command today) — inherited, not widened; see `prelude-dialect-matches-the-launch-command`.
- No `manifest/roadmap.md` move: this is hardening of a shipped module, not a planned roadmap item.

## Decisions

### seam-is-reed-strand-launch

- Decision: compose the pane-environment prelude in `internal/reedengine`, at `launchStrandLocked` (`internal/reedengine/spawn.go:71`), the single chokepoint every strand-realizing path already funnels through — `addStrandLocked` (`strand.go:275`), `updateStrandLocked` (`strand.go:304`), the `--if-absent` relaunch (`strand.go:434`), and `Resume`'s replay (`lifecycle.go:632`).
  Put the composition in a new dedicated file, `internal/reedengine/panebin.go`, that owns the whole seam, and call into it from `launchStrandLocked`.
- Rationale: there are exactly four `AddStrand` callers in production — `internal/loomcli/start.go:413` (operator strand), `internal/loomcli/sharedbootstrap.go:265` (status strand), `internal/shuttleengine/run.go:270` (every LLM agent), and `internal/reedcli/add.go:87` (`lyx reed add`) — and all four reach a pane only through `launchStrandLocked`.
  One chokepoint makes "the binary that spawned a pane is the one `lyx` resolves to inside it" true by construction for every present and future caller, which is the structural property the task asks for.
  It also covers the `ResumeCmd` replay for free, which a caller-side fix would have to remember separately.
  A dedicated file matches this package's existing discipline (`selvagepane.go` owns the whole Selvage seam and is guarded by an enforcement test).
- Rejected: composing it in `internal/shuttleengine/claudeengine`'s `buildLaunchCmd`/`buildResumeCmd` — it would cover only claude agent panes, leaving the operator's own pane (`operatorStrandAddSpec`, the pane a human types `lyx …` into) and any future provider engine resolving `lyx` from prod `PATH`.
  Also rejected: composing it per-caller in `internal/loomcli` and `internal/shuttleengine`, which is the remembered-rather-than-structural shape the task explicitly argues against.

### mechanism-is-shell-prelude-not-tmux-e

- Decision: deliver the prelude as shell statements riding the line reed types into the pane via `send-keys`, built through the `internal/shell` seam — never via tmux's `split-window -e`.
- Rationale: three reasons.
  (a) `split-window -e` takes a literal `KEY=VALUE`, so the value could only ever be a Go-computed string based on the *lyx process's own* `PATH` — that replaces the pane's `PATH` rather than prepending to it, and diverges whenever the tmux server (a per-hub singleton other worktrees also attach to) was booted by a process with a different environment.
  The task's wording is explicitly "prepend … to the pane's `PATH`".
  (b) psmux's support for `split-window -e` is unverified, and this package already declines unverified psmux capabilities on exactly this reasoning (`internal/reedengine/reapply.go:55` declines tmux hooks for it).
  A `send-keys` line is dialect-level, so psmux needs no capability at all.
  (c) There is direct precedent: `internal/shuttleengine/claudeengine/command.go`'s `forkSubagentEnvKey` already rides the pane command via `sh.WithEnv`, with the comment "It must ride the pane command because the reed server env is scrubbed of `CLAUDE_CODE_*` at boot."
- Rejected: `split-window -e KEY=VAL` (above).
  Also rejected: `set-environment -t <session>` before the split — same literal-value problem, plus it is not in `requiredSubcommands` and would add a psmux capability requirement, and its granularity is the worktree session rather than the pane.
  Also rejected: setting `PATH`/`LYX_BIN` on the tmux server spawn's `cmd.Env` in `lifecycle.go` (where `CleanClaudeEnv` already runs) — the server is a long-lived per-hub singleton that later invocations reattach to, so whichever binary happened to boot it would win for every worktree on the hub, reproducing the original bug with a longer fuse.

### shell-seam-gets-three-generic-methods

- Decision: extend the `Shell` interface (`internal/shell/shell.go`) with three methods, implemented in `posix.go` and `pwsh.go`:
  - `ExportEnv(key, value string) string` — a standalone statement exporting `key` into the shell session.
  - `PrependPathEntry(dir string) string` — a standalone statement prepending `dir` to the live `PATH`.
  - `Chain(parts ...string) string` — joins statements into one `send-keys`-safe single line, separated by `"; "` in both dialects, dropping empty parts so an empty `Cmd` yields no trailing separator.
    The separator is `;`, never `&&`: the prelude is env decoration, and a shell that rejected it must still run the agent's own launch command rather than leaving a silently empty pane.
    Residual, stated plainly: a prelude the pane's shell rejects leaves that pane on the ambient `PATH`, and the agent's own command still runs.
    That is the deliberate trade — `&&` would convert a rejected prelude into a dead agent pane, which is strictly worse than a pane that merely resolves the wrong `lyx`.
    It is also bounded by `prelude-dialect-matches-the-launch-command`: a shell that rejects the prelude would reject the `ForGOOS()`-built launch command joined to it anyway, so the prelude adds no failure mode that pane did not already have.
    `&&` is additionally wrong for pwsh, where it is a 7.0+ pipeline-chain operator with different semantics from POSIX's.
- Rationale: the `Shell Mechanics Seam` invariant states pane-shell command strings are built ONLY via `internal/shell`, and every one of these emits raw pwsh/POSIX syntax.
  All three are provider- and lyx-agnostic, satisfying the seam's "implementations carry no provider-specific knowledge" rule — the `LYX_BIN` key name and the `.dev-bin` directory are reed's knowledge, passed in as arguments.
  `reedengine` already imports `internal/shell` (`reapply.go`, `windowsize.go`, `watchdog.go`), so no new dependency edge is created.
- Rejected: a single `WithPaneEnv(dir, binPath, cmd string)` method — it would bake `LYX_BIN` (lyx vocabulary) into the generic shell seam.
  Also rejected: reusing the existing `WithEnv` — it is command-scoped on POSIX (`KEY=value cmd`), so it cannot emit anything at all for a pane whose `Cmd` is empty, and its value is single-quoted, so it can never reference the live `$PATH`.

### prelude-is-session-scoped-in-both-dialects

- Decision: both dialects emit session-scoped statements (POSIX `export`, pwsh `$env:`), not POSIX's existing command-scoped `KEY=value cmd` form.
  A pane whose `Cmd` is empty receives the prelude alone, with no trailing command.
- Rationale: the operator's own strand (`operatorStrandAddSpec`, `internal/loomcli/bootstrap.go:54`) carries an empty `Cmd` — it is an interactive shell a human types `lyx …` into, and it is one of the panes most likely to run the wrong binary.
  A command-scoped POSIX assignment has nothing to attach to there.
  Session scope also makes the two dialects symmetric, which is the difference the existing `WithEnv` doc comment already flags as an asymmetry to live with rather than one to spread.
- Rejected: mirroring `WithEnv`'s command-scoped POSIX / session-scoped pwsh split — it would silently skip the empty-`Cmd` operator pane on POSIX only, a platform-dependent hole in a structural guarantee.

### prelude-dialect-matches-the-launch-command

- Decision: build the prelude with **`shell.ForGOOS()`** — the same selector that builds the launch command it is `;`-joined to (`internal/shuttleengine/claudeengine/claudeengine.go:102`, `internal/loomcli/sharedbootstrap.go:259`).
  The prelude and the command are one line typed into one shell, so the only property that matters is that the two **agree with each other**.
  Reed derives no dialect of its own, adds no config-driven selector, and does not change how a pane's shell is started.
- Rationale: `e.cfg.Shell` looked like the right source, and is not — it does not govern a strand pane.
  `launchStrandLocked` splits with no trailing command (`internal/reedengine/spawn.go:113`) and the package issues no `set-option default-shell`/`default-command` anywhere, so a strand pane's shell is tmux's own `default-shell`.
  `e.cfg.Shell` governs the `new-session` first pane (`lifecycle.go:337`) and Selvage (`selvagepane.go:235`), neither of which has anything typed into it.
  So the `LYX_REED_SHELL=bash`-on-Windows hazard never reached a strand pane at all: the pane is the ambient default shell either way, and the launch command typed into it has been `ForGOOS()`-built since long before this task.
  Matching `ForGOOS()` makes the prelude exactly as correct as the command it rides on — no more, and no less — which is the only guarantee this task can honestly make without changing how panes are started.
- Pre-existing limitation, recorded and explicitly **not** fixed here: the whole pane-command path assumes tmux's `default-shell` dialect matches `ForGOOS()`'s.
  On a machine whose `$SHELL` is `fish`, the `ForGOOS()`-built launch line is already rejected by that pane today, with nothing reporting it — independent of this task, and true before the prelude exists.
  The prelude inherits that assumption rather than widening it.
  Retiring it means threading a dialect from reed through `shuttleengine.ReedOps` and `Engine.Prepare` to the command builders, across the `Shuttle Provider-Seam Invariant`'s boundary; that is a separate task with its own scope, and nothing currently asks for it.
- Rejected: deriving the dialect from `e.cfg.Shell` — it does not govern the pane, so a config-derived dialect would be a guess dressed as a declaration.
  Making it govern the pane (passing `e.cfg.Shell` as `split-window`'s trailing command, as Selvage's split does) was designed in rounds 1–4 and is rejected outright — see `pane-start-mode-stays-untouched`.
  Also rejected: probing the pane's live shell at runtime (`display-message -p '#{pane_current_command}'`) — a tmux round trip per launch, racing the shell's own startup, answering with the foreground command rather than the shell.
  Also rejected: a dialect-agnostic prelude both shells accept — no such syntax exists for a `PATH` prepend.

### pane-start-mode-stays-untouched

- Decision: **do not change how a strand pane's shell is started.**
  `launchStrandLocked` keeps splitting with no trailing command, exactly as today.
  Reed adds no `validateToldPaneShell`, no config-derived dialect selector, and no `split-window` argv change — the only change at the seam is the prelude prepended to the string that was already being typed.
- Rationale: passing `e.cfg.Shell` as the split's trailing command does not merely rename the shell, it changes the **start mode**.
  A commandless `split-window` has tmux start `default-shell` as a login shell; a trailing shell-command is handed to `/bin/sh -c`, which then execs a non-login shell.
  Non-login skips `~/.profile` / `~/.bash_profile`, which is where a developer machine's `PATH` additions typically live — so the pane would inherit a **different `PATH`** than it does today.
  That is fatal in this pane specifically: `claude` is resolved **by bare name** from the pane's `PATH` (`claudeBinary`, `internal/shuttleengine/claudeengine/command.go:63`, defaulting to `"claude"` because `internal/shuttleengine/template.yaml:6`'s `claude:` key is empty by default).
  A task whose entire purpose is "the pane resolves the right binary" must not, as a side effect, make the pane stop resolving the agent binary at all.
- This retracts the `strand-panes-run-the-configured-shell` Decision that rounds 1–4 built up, together with everything that hung off it: the `validateToldPaneShell` op-boundary refusal, its five-verb binding and insertion points, its recovery path, and the trailing-argument quoting question.
  All of it existed to make `e.cfg.Shell` govern the strand pane; none of it is needed once the pane is left alone.
  The retraction is recorded rather than silently dropped because five review rounds are on the record discussing that machinery — see the superseded Q&A entries.
- Rejected: keeping the trailing command but preserving login semantics (`bash -l`, `zsh -l`) — the flag is dialect-specific, pwsh has no equivalent, and it buys nothing the commandless split does not already give.
  Also rejected: accepting the start-mode change with a note — a `claude` that cannot be resolved is a hard failure, not a documented quirk.

### unset-path-idiom

- Decision: emit `PATH`-prepend idioms that are correct when `PATH` is unset or empty, so no trailing empty entry (which POSIX shells read as the current directory) is ever produced.
  Target shapes, exact spelling left to implementation:
  - POSIX: `export PATH=<quoted dir>"${PATH:+:$PATH}"`
  - pwsh: `$env:PATH = <quoted dir> + $(if ($env:PATH) { [IO.Path]::PathSeparator + $env:PATH })`
- Rationale: the naive `<dir>:"$PATH"` form yields `<dir>:` against an unset `PATH`, and a trailing empty `PATH` entry means "search the cwd" — an unnecessary behaviour change in a pane that runs agents with `--dangerously-skip-permissions`.
  Both idioms are single-line and typed verbatim through `send-keys`, so neither needs a continuation.
- Rejected: accepting the trailing separator on the grounds that `PATH` is never unset in a real pane shell — it costs one conditional to be exactly right, and the pane's shell is not reed's to assume about.

### chokepoint-is-the-strand-launch-only

- Decision: only strand panes created by `launchStrandLocked` get the prelude.
  The Selvage pane (`splitPaneBelowLocked`, `internal/reedengine/selvagepane.go:342`) and the session's first pane (`new-session`, `lifecycle.go`) do not.
- Rationale: Selvage is a one-row status band running `e.cfg.Shell` with no operator input and no `lyx` invocation; the `new-session` first pane is untracked and is reaped by reed's own untracked-pane reap before any strand is allocated.
  Adding the prelude to either means either a second composition site (defeating the chokepoint) or moving the composition below `splitPaneBelowLocked`, where the Selvage split's `launchCmd` is passed as a *trailing `split-window` argument* rather than typed — a different mechanism with different quoting rules.
  YAGNI: no pane outside the strand set resolves `lyx`.
- Rejected: prefixing every pane reed creates — more surface, two mechanisms, no benefit.

### detached-spawns-stay-untouched

- Decision: neither the per-hub watchdog daemon spawn (`internal/reedengine/spawnwatchdog.go`) nor the detached `lyx loom run` spawn (`internal/loomcli/start.go`) gains a `LYX_BIN` export or a `PATH` prepend.
- Rationale: this closes the brief's second open question.
  Both are already spawned from `os.Executable()`, so each child's *own* `os.Executable()` is already the right binary, and reed computes the prelude from `os.Executable()` at pane-launch time — so a detached `lyx loom run` that later spawns panes already hands those panes its own (correct) binary.
  Neither process resolves `lyx` from `PATH`, and neither reads `LYX_BIN`.
  Exporting a variable nothing consumes is dead code.
- Rejected: exporting `LYX_BIN` in both for symmetry — symmetry with what?
  The property being guaranteed is about panes, and these two spawn no shell that resolves `lyx` by name.

### lyx-bin-is-the-binary-path

- Decision: `LYX_BIN` carries the absolute path of the binary itself (`os.Executable()`), not its directory.
- Rationale: matches the task brief verbatim, and is directly invocable (`"$LYX_BIN" reed status`) by a script or prompt that wants to be explicit.
  The directory is already recoverable from it, and is separately on `PATH`.
- Rejected: exporting the directory — it would force every consumer to re-append the platform-specific binary name (`lyx` vs `lyx.exe`).

### executable-error-warns-and-degrades

- Decision: when `os.Executable()` returns an error, log `logger.Warn` naming the strand and the cause, and launch the pane with no prelude.
  Do not fail the strand launch.
- Rationale: `os.Executable()` failing is near-unreachable on Linux and Windows.
  Failing the launch would take down a whole loom run over a decoration that, on any machine where the error could plausibly occur, would have resolved to the production binary anyway — the exact `PATH` the pane already had.
  A `Warn` is not the silent fallback the brief rejects: the brief's "no silent fallback" argument is about the env-token-only design, where a *forgotten* export leaves no trace at all.
  Here the degradation is logged, named, and attributable.
  This matches `SpawnWatchdog`'s own `os.Executable()` error handling in the same package.
- Rejected: failing the launch loudly — maximal cost, zero benefit on the only machines that can reach the branch.
  Also rejected: silently skipping the prelude, which would reproduce the original "nothing reported the mismatch" failure.

### unconditional-prepend-no-dedup-guard

- Decision: prepend unconditionally.
  Do not guard against the directory already being on the pane's `PATH`.
- Rationale: every strand pane is a *fresh* pane with a *fresh* shell, so there is no accumulation across relaunches or resumes.
  The one nesting case — an agent inside a pane running `lyx loom start`, whose nested panes get the directory prepended a second time — produces a duplicate `PATH` entry that resolves identically and costs nothing.
  A shell-side dedup conditional would be a long, dialect-divergent one-liner defending against a cosmetic outcome.
- Rejected: a `case ":$PATH:" in *":$dir:"*)`-style POSIX guard plus its pwsh twin — far more shell syntax in the seam, for no behavioural difference.

### new-constraints-clause-plus-enforcement-test

- Decision: add a new `CONSTRAINTS.md` clause — working title **Pane Binary Resolution** — placed immediately after `## Live-Substrate Spawn Observability`, since it is the same cross-cutting area the brief names.
  Back it with an enforcement test in `internal/reedengine` asserting that every `split-window` pane-creation site in the package either routes through the prelude chokepoint or is on a named allowlist (Selvage's own split, per the chokepoint Decision above).
- Rationale: `CLAUDE.md` requires any new cross-cutting invariant be recorded in `CONSTRAINTS.md` in the same commit, and this repo's house style is that a structural rule gets a guard test rather than review discipline alone — `internal/reedengine/selvagepane_enforcement_test.go`, `internal/cliwire/bannedecl_enforcement_test.go`, `internal/gitkit/callerset_enforcement_test.go` and `tools/sandbox/pathresolve_guard_test.go` are the precedents.
  Without the test, a future fifth pane-creation path silently drops back to prod `PATH`, which is exactly the class of regression this task exists to make impossible.
- Rejected: clause only, no guard test — the invariant would then be enforced by nothing but memory, which is the failure mode being fixed.

### docs-describe-the-landed-mechanism

- Decision: `crucible/README.md` and the sandbox-suite pre-conditions describe the landed mechanism ("panes resolve the spawning binary; no PATH setup needed"), not the brief's interim "state the prerequisite until the mechanism lands" warning.
- Rationale: the mechanism lands in this task, so an interim warning would be stale on arrival.
  `tools/sandbox/SANDBOX-SHUTTLE-SUITE.md:15` already uses exactly this "no PATH setup needed" phrasing for the launcher's own prepend; the reed/crucible wording should match it.
- Rejected: adding a temporary prerequisite note — it would need removing in the same commit that adds it.

## Technical context

**The chokepoint.**
`internal/reedengine/spawn.go:71`, `launchStrandLocked(st *ReedState, s *Strand, launchCmd string)`.
It reconciles, splits a fresh pane with `split-window … -c e.geom.PaneCwd -P -F '#{pane_id}'`, validates the pane is genuinely new via `validateSplitCreatedNewPane` (psmux's too-small-to-split failure prints an existing pane's id with exit 0), then sends the command in two steps: `send-keys -t <pane> -l <sendKeysLiteralArg(launchCmd)>` followed by `send-keys -t <pane> Enter`.
`sendKeysLiteralArg` prefixes a dash-leading string with a space so tmux does not parse it as a flag — the composed prelude+command string must go through it unchanged.
Everything is one line; `send-keys` submits a line at a time, so the prelude must be `;`-joined onto the command, never newline-separated.

**The four production `AddStrand` callers** and what their `Cmd` looks like:

| Caller | `Cmd` | Notes |
| --- | --- | --- |
| `internal/shuttleengine/run.go:270` | `launch.Cmd` from `claudeengine.Prepare` | every LLM agent; also supplies `ResumeCmd` |
| `internal/loomcli/sharedbootstrap.go:265` | `statusStrandCmd(shell.ForGOOS(), exe)` | already resolves `exe` explicitly via `os.Executable()` |
| `internal/loomcli/start.go:413` | empty (`operatorStrandAddSpec`) | interactive operator shell — the empty-`Cmd` case |
| `internal/reedcli/add.go:87` | operator-supplied | `lyx reed add` |

**The shell seam.**
`internal/shell/shell.go` declares `Shell` with `Quote`, `Invoke`, `ReadFile`, `WithEnv`, `Touch`; `ForGOOS()` returns `Pwsh()` on Windows and `Posix()` elsewhere.
`posix.go` and `pwsh.go` are both plain untagged Go and host-testable on either platform — `posix.go`'s header comment states this explicitly, so both dialects' new methods must be unit-tested on whatever host CI runs.
`WithEnv`'s existing asymmetry (POSIX command-scoped, pwsh session-wide) is documented on the interface method and must not be disturbed; the new methods are additions, not changes.
`shell.ForGOOS()` is the selector to use — see `prelude-dialect-matches-the-launch-command`.

**A strand pane's shell is tmux's `default-shell`, and `e.cfg.Shell` is not it.**
This is the single fact the design turns on, and it is easy to get backwards.
`launchStrandLocked` splits with no trailing command (`internal/reedengine/spawn.go:113`), and the package issues no `set-option default-shell` or `default-command` anywhere — every `set-option` call in `lifecycle.go`, `windowsize.go` and `statusline.go` targets `remain-on-exit`, `mouse`, `status*`, `window-size` or `window-status-format`.
So a strand pane runs tmux's own ambient `default-shell`, started as a login shell.
`e.cfg.Shell` governs the `new-session` first pane (`lifecycle.go:337`) and Selvage (`selvagepane.go:235`) — neither of which has anything typed into it — and it resolves from `reed.yaml`'s `shell` key (`${env:LYX_REED_SHELL:-bash}` on POSIX, `${env:LYX_REED_SHELL:-pwsh}` on Windows), so it can be any absolute path an operator pins.

`e.cfg.Shell` also has two consumers outside pane creation, named here so a plan writer does not read it as a pane-only value:
`internal/reedengine/proctree_windows.go:42` and `:71` exec it directly as a pwsh interpreter (`-NoProfile -NonInteractive -Command`, degrading to `roots`/`nil` on error), and `spawnwatchdog.go:55` propagates it to the watchdog daemon as `--shell`.
`internal/reedengine/lock.go:66`'s `ShellPath()` returns it verbatim, validating and defaulting nothing.
This task reads none of them and changes none of them.

**Why the pane's start mode is load-bearing.**
A commandless `split-window` has tmux start `default-shell` as a login shell; a trailing shell-command is handed to `/bin/sh -c`, which execs a non-login shell that skips `~/.profile` / `~/.bash_profile`.
That changes the pane's inherited `PATH` — and `claude` is resolved **by bare name** from the pane's `PATH` (`claudeBinary`, `internal/shuttleengine/claudeengine/command.go:63`, defaulting to `"claude"` because `internal/shuttleengine/template.yaml:6`'s `claude:` key is empty by default).
Hence `pane-start-mode-stays-untouched`: the only thing this task changes at the seam is the string typed into a pane that is started exactly as it is today.

**`os.Executable()` inside `reedengine` is established practice**, not a new precedent: `spawnwatchdog.go:49` already calls it, `Warn`s on error, and degrades.
Note the `CONSTRAINTS.md` `Live-Substrate Spawn Observability` clause "Never re-exec `os.Executable()` under `go test`" — that bans *re-exec*, not *reading the path*.
Composing a string from it is not a spawn.
Under `go test` the value is the test binary's path, so hermetic tests of the composition must inject the path rather than assert against the live `os.Executable()`; a small package-local seam (e.g. a `var executablePath = os.Executable` in `panebin.go`) is the cheapest way to do that and matches `tools/sandbox/resolve.go`'s own `var devBinPath = devbin.BinPath` seam.

**Geometry is not the right carrier.**
`reedengine.Geometry` (`internal/reedengine/geometry.go`) is built by `hubgeom.ReedGeometry` and `standalonegeom.ReedGeometry` from a resolved `*lyxcwd.Location`, which carries no binary information.
Threading the binary path through would force those pure path-mappers to call `os.Executable()`, which is outside what the `Told-Geometry Invariant` means by told coordinates.
The binary path is process identity, not worktree geometry.

**No tmux capability change.**
`requiredSubcommands` (`internal/reedengine/probe.go`) stays as-is: the prelude adds no verb and no flag.
Minimum versions (`internal/reedengine/version.go`: tmux 3.3.0, psmux 3.3.3) are untouched.

**Windows/pwsh profile question, answered structurally.**
The brief asks whether a `PATH` prepend survives the pane shell's own profile.
It does, because the prelude is typed into an *already-running* pane shell via `send-keys` — `new-session`/`split-window` started the shell and its profile ran before anything is typed.
A profile cannot clobber a statement executed after it.
This is a further argument for the `send-keys` mechanism over `split-window -e`, where the profile *would* run afterwards and could overwrite the value.

**Related, deliberately untouched:** `tools/sandbox/resolve.go`'s `prependPath` does the same job for the sandbox launcher's direct child process, guarded by `tools/sandbox/pathresolve_guard_test.go` and the `Dev/Prod Binary Separation` invariant.
The two mechanisms are complementary and must not be merged: one composes a Go `exec.Cmd` environment, the other composes a pane shell statement.

## Constraints

From `CONSTRAINTS.md`:

- **Shell Mechanics Seam** — pane-shell command strings are built ONLY via `internal/shell` (`Quote`/`Invoke`/`ReadFile`, stdlib-only).
  Every new shell token this task emits goes in `internal/shell`; `internal/reedengine` composes but never spells raw pwsh/POSIX syntax.
- **Live-Substrate Spawn Observability** — the new clause is recorded adjacent to it.
  Its "Never re-exec `os.Executable()` under `go test`" sub-clause is respected: this task reads the path, it does not re-exec.
- **Told-Geometry Invariant** — `reedengine` must not grow a direct `internal/lyxcwd` import.
  The binary path comes from `os.Executable()` inside the package, not from `Geometry`.
- **Dev/Prod Binary Separation** — unchanged; `tools/sandbox/resolve.go` remains the sole `.dev-bin`-first resolution site for sandbox tooling, and this task adds no bare-PATH `lyx` lookup anywhere.
- **CLI / Cobra Invariant** — no CLI surface changes, so no `Short`/help-tree work.
- **Test Tier Purity Invariant** — all new tests are hermetic Tier-1 (string composition, AST walk); none spawns a process.
- **Documentation Lifecycle** — docs land in the same commit: `internal/reedengine/doc.go`, `internal/shell/shell.go`, `CONSTRAINTS.md`, `crucible/README.md`, `docs/sandbox-howto.md`, `tools/sandbox/SANDBOX-REED-SUITE.md` and `SANDBOX-SHUTTLE-SUITE.md` (not the reed-watch suite — see Scope).
  `docs/overview.md` is untouched — no module is added and the execution stack is unchanged.
  `manifest/roadmap.md` is untouched — this is hardening, not a planned item.

From `CLAUDE.md`:

- **Markdown: semantic line breaks** — one sentence per line, break at internal independent-clause boundaries, plain newline only.
  Applies to every `.md` file touched.
- **Build prerequisite: cgo** — `CGO_ENABLED=1` and a C compiler on `PATH` for `go build`/`go test`.

Discovered during exploration:

- The composed line must stay a **single line** with no embedded newline: `send-keys` submits a line at a time.
- The composed line must survive `sendKeysLiteralArg`: if the prelude ever begins with `-`, the existing space-prefix guard handles it, but nothing else may assume the string starts with the command.
- psmux capability surface is treated as unverified-until-proven in this package; the design must not require a psmux feature that is not already exercised.

## Testing

**`internal/shell` — TDD candidate, pure string transforms, host-agnostic.**
Both `Posix()` and `Pwsh()` are directly constructible, so both dialects are tested on whichever host CI runs (`shell_test.go` already does this for the existing methods).
Scenarios per dialect:

- `ExportEnv` with an ordinary key/value; with a value containing the dialect's quote character (POSIX `'` → `'\''`, pwsh `'` → `''`); with a value containing spaces and a path separator.
- `PrependPathEntry` with an ordinary directory; with a directory containing a space and a quote; asserting the emitted idiom references the live `PATH` variable rather than a baked-in literal, and that the unset-`PATH` guard is present.
- `Chain` with zero, one and several parts, asserting a single-line result with no embedded newline.
- An interface-completeness check that both implementations satisfy `Shell` (the existing compile-time pattern suffices).

**`internal/reedengine` — TDD candidate for the composition, hermetic.**

- Prelude composition against an injected executable path: asserts the result contains the `PATH` prepend for the executable's directory, the `LYX_BIN` export carrying the full binary path, and the strand's own command, in that order, on one line.
- Empty `Cmd`: asserts the prelude is emitted alone, with no trailing separator or empty command fragment.
- `os.Executable()` error via the injected seam: asserts the launch still proceeds and the command is passed through unchanged, and that a `Warn` is logged — this package already has `logcapture_test.go` for log assertions.
- A dash-leading composed line still round-trips through `sendKeysLiteralArg`.
- **`split-window` argv is unchanged**: assert on the fake tmux recorder that the strand split still carries no trailing shell-command, and that Selvage's split (`splitPaneBelowLocked`) and `new-session`'s argv are untouched.
  This is the regression guard for `pane-start-mode-stays-untouched` — the one thing in this task that could silently change a pane's inherited `PATH`, and therefore whether bare-name `claude` still resolves.
- The prelude is built with `shell.ForGOOS()`, the same selector as the launch command: assert the composed line's dialect matches what `ForGOOS()` produces on the running host, so prelude and command can never diverge.

**Existing tests this change invalidates — update, never replace:**

- `internal/reedengine/spawn_test.go` and `lifecycle_test.go` — `send-keys` payload expectations and the `split-window` argv; the fake tmux recorder they already use is the assertion surface.
- `internal/reedengine/emptycmd_integration_test.go` (build tag `integration`) — its doc comment and `TestAddStrand_EmptyCmdLeavesALivePane` currently pin the premise "`launchStrandLocked` issues `send-keys -t <pane> -l \"\"` followed by Enter for an empty command, and `sendKeysLiteralArg(\"\")` returns the empty string".
  The `prelude-is-session-scoped-in-both-dialects` Decision makes that false: the empty-`Cmd` operator pane now receives the prelude alone.
  Move both the doc comment and the assertion from "empty payload" to "prelude-only payload, with no trailing separator and no empty command fragment", so the test documents the behaviour that exists rather than passing while describing behaviour that does not.
  It also references a prior task's `_mill/discussion.md` "empty-cmd-leaves-the-panes-own-shell decision" by name; repoint that citation at this task's `prelude-is-session-scoped-in-both-dialects` Decision.

**`internal/reedengine` — enforcement test, AST-based, hermetic.**
Modelled on `selvagepane_enforcement_test.go`'s `runtime.Caller(0)` root resolution and allowlist shape: parse every non-`_test.go` file in the package and fail if a `split-window` pane-creation site exists outside the chokepoint and outside the named allowlist (Selvage's `splitPaneBelowLocked`).
Record the residual honestly in the file's doc comment, as that package's other enforcement test does.

**Not covered by `go test`, stated as sandbox-suite pre-conditions:**
the live property "a pane spawned by `.dev-bin/lyx` resolves `lyx` to `.dev-bin/lyx`" is a live-substrate fact.
Add these check lines to the reed and shuttle sandbox suite docs (`tools/sandbox/SANDBOX-REED-SUITE.md`, `tools/sandbox/SANDBOX-SHUTTLE-SUITE.md`):

- Run `where lyx` / `command -v lyx` inside a spawned pane and confirm it names the spawning binary.
- Confirm `LYX_BIN` is set in the pane and names the same binary.
- Confirm `claude` still resolves inside a spawned agent pane — the bare-name lookup `pane-start-mode-stays-untouched` exists to protect, and the one property no hermetic test can prove.
No new Go integration test: `contract_integration_test.go` asserts the tmux wire contract, and this change adds no tmux verb to that contract.

## Q&A log

- **Q:** Which seam composes the pane environment? **A:** [auto-pick] `reedengine.launchStrandLocked`, a single chokepoint for every strand pane and every caller. **Why:** all four production `AddStrand` callers funnel through it, including `Resume`'s replay, so the property holds by construction rather than by each call site remembering.
- **Q:** What mechanism delivers `PATH`/`LYX_BIN` into the pane? **A:** [auto-pick] shell statements riding the `send-keys` launch line, built through the `internal/shell` seam. **Why:** it prepends onto the pane's live `PATH` rather than replacing it with a Go-computed literal, needs no unverified psmux capability, and follows the existing `forkSubagentEnvKey` precedent.
- **Q:** Session-scoped export, or POSIX's existing command-scoped `WithEnv` shape? **A:** [auto-pick] session-scoped in both dialects. **Why:** the operator strand carries an empty `Cmd`, and a command-scoped assignment has nothing to attach to there — it would leave a platform-dependent hole on POSIX only.
- **Q:** One `WithPaneEnv(dir, binPath, cmd)` method, or generic primitives? **A:** [auto-pick] three generic methods (`ExportEnv`, `PrependPathEntry`, `Chain`). **Why:** the Shell Mechanics Seam requires implementations carry no caller-specific knowledge; `LYX_BIN` is reed vocabulary and must stay an argument.
- **Q:** Guard the prepend against a duplicate `PATH` entry? **A:** [auto-pick] no, prepend unconditionally. **Why:** every strand pane is a fresh shell, so nothing accumulates; the only duplicate case is nested spawns, where the entry resolves identically and costs nothing.
- **Q:** Which panes get the prelude — strand panes only, or also Selvage and the `new-session` first pane? **A:** [auto-pick] strand panes only. **Why:** Selvage is a one-row status band running a bare shell, the first pane is reaped before any strand allocates, and covering them would require a second composition site with different quoting rules.
- **Q:** Does the `PATH` prepend survive the pane shell's own profile on Windows/pwsh? **A:** [auto-pick] yes, structurally — the prelude is typed into an already-running shell whose profile has already loaded. **Why:** this is a further argument against `split-window -e`, where the profile would run afterwards and could overwrite the value; a sandbox pre-condition line records the live check.
- **Q:** Should the watchdog daemon and the detached `lyx loom run` also export `LYX_BIN`? **A:** [auto-pick] no. **Why:** both are already spawned from `os.Executable()`, so each child's own `os.Executable()` is already correct and reed derives the prelude from it; neither reads `LYX_BIN` or resolves `lyx` from `PATH`, so the export would be dead code.
- **Q:** `LYX_BIN` carries the binary path or its directory? **A:** [auto-pick] the absolute binary path. **Why:** matches the brief, is directly invocable, and avoids forcing consumers to re-append `lyx` vs `lyx.exe`.
- **Q:** What happens when `os.Executable()` errors? **A:** [auto-pick] `logger.Warn` and launch without the prelude. **Why:** failing a whole loom run over a near-unreachable branch costs everything and gains nothing, and a logged, named degradation is not the silent fallback the brief rejects.
- **Q:** Should the `PATH` idiom guard against an unset `PATH`? **A:** [auto-pick] yes. **Why:** the naive form leaves a trailing empty entry, which POSIX shells read as the current directory — an unnecessary behaviour change in a pane running agents with `--dangerously-skip-permissions`.
- **Q:** New `CONSTRAINTS.md` clause with or without an enforcement test? **A:** [auto-pick] with — an AST guard in `internal/reedengine` allowlisting Selvage's own split. **Why:** without it the invariant is enforced by memory alone, which is the exact failure mode this task fixes; the package already has the precedent.
- **Q:** Do `crucible/README.md` and the sandbox pre-conditions carry the brief's interim "until the mechanism lands" warning? **A:** [auto-pick] no, they describe the landed mechanism. **Why:** the mechanism lands in this task, so the warning would be stale in the commit that adds it; `SANDBOX-SHUTTLE-SUITE.md` already uses the matching "no PATH setup needed" phrasing.
- **Q:** Where does the composition live inside `reedengine`? **A:** [auto-pick] a new dedicated `panebin.go` owning the whole seam, called from `launchStrandLocked`. **Why:** matches the package's existing discipline, where `selvagepane.go` owns the whole Selvage seam and is guarded by its own enforcement test.
- **Q:** [discussion-review r1 gap, BLOCKING:design] Which `shell.Shell` composes the prelude, and what happens when `LYX_REED_SHELL` pins a shell that contradicts `runtime.GOOS`? **A:** [auto-pick] derive the dialect from `e.cfg.Shell` by basename match, never from `shell.ForGOOS()`. **Superseded in r2:** the "unrecognized or empty shell emits no prelude and logs a `Warn`" half was replaced by a hard refusal at the pane-creating verbs' op boundary — see the r2 entry below and the `one-dialect-per-pane-enforced-at-the-op-boundary` Decision. **Why:** `ForGOOS()` is correct only until someone overrides `LYX_REED_SHELL`, and a `$env:PATH = …` statement `;`-joined ahead of an agent's command in bash is a syntax error that fails the `PATH` guarantee while the strand still looks launched. **Retracted in r5:** this whole line of design rested on making `e.cfg.Shell` govern the strand pane, which changes the pane's start mode and therefore its inherited `PATH` — fatal for the bare-name `claude` lookup. The pane is left exactly as it is today and the prelude uses `shell.ForGOOS()`, the same selector as the launch command it rides on. See `prelude-dialect-matches-the-launch-command` and `pane-start-mode-stays-untouched`.
- **Q:** [discussion-review r1 gap, follow-on] `e.cfg.Shell` governs only the `new-session` first pane and Selvage today — a strand pane runs tmux's ambient `default-shell`. Derive the dialect from config anyway, or make config true? **A:** [auto-pick] make it true — pass `e.cfg.Shell` as `split-window`'s trailing command in `launchStrandLocked`, as Selvage's own split already does. **Why:** deriving a dialect from a value that does not govern the pane would reintroduce the same mismatch one level down; strand panes are the only panes reed creates whose shell is undeclared. **Retracted in r5:** this whole line of design rested on making `e.cfg.Shell` govern the strand pane, which changes the pane's start mode and therefore its inherited `PATH` — fatal for the bare-name `claude` lookup. The pane is left exactly as it is today and the prelude uses `shell.ForGOOS()`, the same selector as the launch command it rides on. See `prelude-dialect-matches-the-launch-command` and `pane-start-mode-stays-untouched`.
- **Q:** [discussion-review r2 gap, BLOCKING:design] A strand's `launchCmd` is built with `shell.ForGOOS()` at two call sites, so pinning the pane shell to `e.cfg.Shell` makes the pane deterministically bash while the typed command stays pwsh. Thread the dialect to the command builders, or narrow the pane-shell Decision? **A:** [auto-pick] neither — refuse a cross-dialect `LYX_REED_SHELL` at reed's op boundary, which makes the two `ForGOOS()` sites same-dialect by construction and leaves them unchanged. **Why:** threading would grow `ReedOps` and `Engine.Prepare` across the `Shuttle Provider-Seam Invariant`'s boundary for a configuration nobody asks for; the refusal is one function and can be relaxed later if a cross-dialect pin is ever wanted. **Retracted in r5:** this whole line of design rested on making `e.cfg.Shell` govern the strand pane, which changes the pane's start mode and therefore its inherited `PATH` — fatal for the bare-name `claude` lookup. The pane is left exactly as it is today and the prelude uses `shell.ForGOOS()`, the same selector as the launch command it rides on. See `prelude-dialect-matches-the-launch-command` and `pane-start-mode-stays-untouched`.
- **Q:** [discussion-review r2 gap, BLOCKING:design] The unrecognized-shell degrade drops the prelude but still passes that shell to `split-window`, so only half degrades. What is the trailing argument then? **A:** [auto-pick] there is no degrade — an empty or unmodelled `e.cfg.Shell` is refused at the op boundary, so no pane is ever created on an un-modelled shell. **Why:** refusal rather than sanitization is this package's established answer to an unhonourable told value (`validateToldTmuxIdentity`), and it is the loud form of a breakage that already exists silently — a `fish` pane today gets a POSIX launch line fish does not accept, with nothing reporting it. **Retracted in r5:** this whole line of design rested on making `e.cfg.Shell` govern the strand pane, which changes the pane's start mode and therefore its inherited `PATH` — fatal for the bare-name `claude` lookup. The pane is left exactly as it is today and the prelude uses `shell.ForGOOS()`, the same selector as the launch command it rides on. See `prelude-dialect-matches-the-launch-command` and `pane-start-mode-stays-untouched`.
- **Q:** [discussion-review r2, NIT:design] Is `Chain`'s separator `;` or `&&`? **A:** [auto-pick] `"; "` in both dialects, fail-open, dropping empty parts. **Why:** the prelude is env decoration and must not be able to suppress the agent's launch; `&&` is additionally a pwsh 7.0+ pipeline-chain operator with different semantics from POSIX's.
- **Q:** [discussion-review r2, NIT:design] A spaced configured-shell path becomes `split-window`'s trailing shell-command, which tmux word-splits. **A:** [auto-pick] quote it through the dialect's `Quote` at the strand split. **Superseded in r3:** quoting was retired entirely — the multiplexer, not the pane dialect, parses that argument, so the value is now passed verbatim at all three sites and the spaced-path limitation is recorded as uniform and pre-existing. The original rationale also rested on a factually wrong claim (that `new-session` takes command-plus-arguments rather than one `sh -c` string; `lifecycle.go:337` passes exactly one trailing argument), which the r3 entries retract. See the r3 entries below and the `strand-panes-run-the-configured-shell` Decision. **Retracted in r5:** this whole line of design rested on making `e.cfg.Shell` govern the strand pane, which changes the pane's start mode and therefore its inherited `PATH` — fatal for the bare-name `claude` lookup. The pane is left exactly as it is today and the prelude uses `shell.ForGOOS()`, the same selector as the launch command it rides on. See `prelude-dialect-matches-the-launch-command` and `pane-start-mode-stays-untouched`.
- **Q:** [discussion-review r3 gap, BLOCKING:design] `validateToldTmuxIdentity` fires from `withOpLock`, which every public op passes — placing the pane-shell refusal there would refuse `down` and `status` too, wedging an operator whose `reed.yaml` already carries a bad `shell` value. Which ops does the refusal bind? **A:** [auto-pick] the pane-creating verbs only — `AddStrand`, `UpdateStrand`, `Resume` — beside the `validateAnchor`/`validateIfAbsent` op-boundary calls `strand.go` already makes; never `withOpLock`. **Why:** a bad tmux identity makes every verb meaningless, but a bad pane shell does not — `down`, `status`, `attach` and the transport verbs stay meaningful and are exactly the recovery path, so refusing them would leave hand-editing `reed.yaml` as the only escape. **Retracted in r5:** this whole line of design rested on making `e.cfg.Shell` govern the strand pane, which changes the pane's start mode and therefore its inherited `PATH` — fatal for the bare-name `claude` lookup. The pane is left exactly as it is today and the prelude uses `shell.ForGOOS()`, the same selector as the launch command it rides on. See `prelude-dialect-matches-the-launch-command` and `pane-start-mode-stays-untouched`.
- **Q:** [discussion-review r3 gap, BLOCKING:design] The `split-window` trailing argument is parsed by tmux/psmux before any pane shell exists, but the r2 fix quoted it with the pane dialect. What is the quoting authority, and what does Windows emit? **A:** [auto-pick] the multiplexer's argv parsing, not the pane dialect — and pass the value verbatim at all three sites, quoting nowhere. **Why:** the two coincide only on POSIX (`sh -c`), which is what hid the category error; on Windows, quoting would emit pwsh single-quoting into psmux, whose trailing-argument parsing is unverified, replacing a shipped default that demonstrably works unquoted. The spaced-path limitation stays uniform across all three sites and is recorded as a follow-up that must fix them together. **Retracted in r5:** this whole line of design rested on making `e.cfg.Shell` govern the strand pane, which changes the pane's start mode and therefore its inherited `PATH` — fatal for the bare-name `claude` lookup. The pane is left exactly as it is today and the prelude uses `shell.ForGOOS()`, the same selector as the launch command it rides on. See `prelude-dialect-matches-the-launch-command` and `pane-start-mode-stays-untouched`.
- **Q:** [discussion-review r3, NIT:consistency] Technical context still told a plan writer to degrade on an empty shell, and the r1 Q&A entry still described the retired `Warn` path. **A:** [auto-pick] rewrite both — Technical context now states the refusal, and the r1 entry carries an explicit **Superseded in r2** marker. **Why:** a plan writer reading Technical context alone would have implemented exactly the degrade the Decisions forbid.
- **Q:** [discussion-review r3, NIT:consistency] The out-of-scope rationale claimed `new-session` takes command-plus-arguments rather than one `sh -c` string, but `lifecycle.go:330-338` passes exactly one trailing argument. **A:** [auto-pick] drop the claim; the exclusion now rests on the multiplexer-parsing argument and on keeping all three sites uniform. **Why:** the claim was factually wrong, and the remaining rationale stands without it. **Retracted in r5:** this whole line of design rested on making `e.cfg.Shell` govern the strand pane, which changes the pane's start mode and therefore its inherited `PATH` — fatal for the bare-name `claude` lookup. The pane is left exactly as it is today and the prelude uses `shell.ForGOOS()`, the same selector as the launch command it rides on. See `prelude-dialect-matches-the-launch-command` and `pane-start-mode-stays-untouched`.
- **Q:** [discussion-review r4 gap, BLOCKING:design] `Up` and `EnsureSession` also create panes on `e.cfg.Shell` (`new-session`, Selvage's split), but the r3 refusal bound only the three strand verbs — so "no pane is ever created on an un-modelled shell" was false as written. Do they refuse or are they exempt? **A:** [auto-pick] they refuse — the rule is "this verb can create a pane", binding all five (`Up`, `EnsureSession`, `Resume`, `AddStrand`, `UpdateStrand`) and nothing else. **Why:** binding only the strand verbs would leave `Up` booting a `fish` first pane and a `fish` Selvage, a weaker and harder-to-state rule for no benefit; `Up` is also the earliest verb that can tell the operator, so failing there beats failing minutes into a loom run. The exempt set (`Down`, `Status`, `StatusLineText`, `AttachArgv`, transport, the resize watch loop) is unchanged and still the recovery path. **Retracted in r5:** this whole line of design rested on making `e.cfg.Shell` govern the strand pane, which changes the pane's start mode and therefore its inherited `PATH` — fatal for the bare-name `claude` lookup. The pane is left exactly as it is today and the prelude uses `shell.ForGOOS()`, the same selector as the launch command it rides on. See `prelude-dialect-matches-the-launch-command` and `pane-start-mode-stays-untouched`.
- **Q:** [discussion-review r4 gap, BLOCKING:design] "Beside `validateAnchor`/`validateIfAbsent`" names two different places — `validateIfAbsent` is at `AddStrand`'s op boundary, `validateAnchor` is inside `addStrandLocked` after a session boot and state load, and `UpdateStrand` has no op-boundary validator at all. Where exactly does the check go? **A:** [auto-pick] the first statement inside each verb's `withOpLock` closure, before `ensureSessionLocked`/`requireSessionLocked`/`ensureServerAndSessionLocked` and before `validateIfAbsent`. **Why:** only that placement makes the "nothing half-created" promise implementable — the same reason `validateIfAbsent` already precedes `AddStrand`'s session pre-flight, so a pure config error never deposits a spawned tmux server as residue. **Retracted in r5:** this whole line of design rested on making `e.cfg.Shell` govern the strand pane, which changes the pane's start mode and therefore its inherited `PATH` — fatal for the bare-name `claude` lookup. The pane is left exactly as it is today and the prelude uses `shell.ForGOOS()`, the same selector as the launch command it rides on. See `prelude-dialect-matches-the-launch-command` and `pane-start-mode-stays-untouched`.
- **Q:** [discussion-review r4, NIT:consistency] The r2 Q&A entry still answered "quote it through the dialect's `Quote`" and still carried the retracted `new-session` claim, while the r1 entry had a supersession marker. **A:** [auto-pick] add the same explicit **Superseded in r3** marker and retract the wrong claim in place. **Why:** a reader of the Q&A log alone would otherwise implement quoting the Decisions now forbid. **Retracted in r5:** this whole line of design rested on making `e.cfg.Shell` govern the strand pane, which changes the pane's start mode and therefore its inherited `PATH` — fatal for the bare-name `claude` lookup. The pane is left exactly as it is today and the prelude uses `shell.ForGOOS()`, the same selector as the launch command it rides on. See `prelude-dialect-matches-the-launch-command` and `pane-start-mode-stays-untouched`.
- **Q:** [discussion-review r4, NIT:consistency] `splitPaneBelowLocked` is declared at `selvagepane.go:342`, not `:352`. **A:** [auto-pick] repoint all three citations at `:342`. **Why:** `:352` is inside the function's error return, so the cite pointed a plan writer at the wrong statement. **Retracted in r5:** this whole line of design rested on making `e.cfg.Shell` govern the strand pane, which changes the pane's start mode and therefore its inherited `PATH` — fatal for the bare-name `claude` lookup. The pane is left exactly as it is today and the prelude uses `shell.ForGOOS()`, the same selector as the launch command it rides on. See `prelude-dialect-matches-the-launch-command` and `pane-start-mode-stays-untouched`.
- **Q:** [discussion-review r5 gap, BLOCKING:design] Passing `e.cfg.Shell` as `split-window`'s trailing command does not just rename the pane's shell — it stops being tmux's own login-shell launch and becomes a `/bin/sh -c` non-login shell, so the pane inherits a different `PATH`. That is the one pane where `claude` is resolved by bare name. Accept it, or preserve profile sourcing? **A:** [auto-pick] neither — retract the pane-shell change entirely and leave the pane started exactly as it is today. **Why:** a task whose purpose is "the pane resolves the right binary" must not, as a side effect, make the pane stop resolving the agent binary at all; and once the pane is left alone, nothing needs `e.cfg.Shell` as a dialect source, so the dialect selector, the `validateToldPaneShell` refusal, its verb set and insertion points, and the trailing-argument quoting question all fall away with it.
- **Q:** [discussion-review r5, follow-on] If not `e.cfg.Shell`, what is the prelude's dialect source? **A:** [auto-pick] `shell.ForGOOS()`, the same selector that builds the launch command the prelude is `;`-joined to. **Why:** prelude and command are one line typed into one shell, so the only property that matters is that they agree with each other. The round-1 hazard (`LYX_REED_SHELL=bash` on Windows) never reached a strand pane in the first place — `e.cfg.Shell` governs the `new-session` first pane and Selvage, neither of which has anything typed into it. The residual `default-shell`-vs-`ForGOOS()` assumption is pre-existing, already breaks the launch command on a `fish` `$SHELL` today, and is inherited rather than widened.
- **Q:** [discussion-review r5, NIT:consistency] Technical context described `e.cfg.Shell` as governing only two panes, omitting its non-pane consumers. **A:** [auto-pick] name both — `proctree_windows.go:42`/`:71` exec it as a pwsh interpreter, `spawnwatchdog.go:55` propagates it as `--shell` — and state that this task reads and changes none of them. **Why:** a plan writer reading it as pane-only could mistake it for a value this task is free to reinterpret.
- **Q:** [discussion-review r5, NIT:scope] Scope said "the affected `SANDBOX-*.md`" while Testing named two files, leaving `SANDBOX-REED-WATCH-SUITE.md` ambiguous. **A:** [auto-pick] enumerate the same two in both places and mark the reed-watch suite explicitly out. **Why:** it carries the same boilerplate PATH pre-condition line, but drives the resize watch loop and spawns no agent pane, so the new checks have nothing to exercise there.
