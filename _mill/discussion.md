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
- A dialect selector in `internal/reedengine` mapping reed's configured pane shell (`e.cfg.Shell`) to a `shell.Shell`.
- A `validateToldPaneShell` refusal rejecting an empty, unmodelled, or cross-dialect `e.cfg.Shell`, at the op boundary of the pane-creating verbs only (`AddStrand`, `UpdateStrand`, `Resume`) — never in `withOpLock`.
- Passing `e.cfg.Shell` verbatim as `split-window`'s trailing command in `launchStrandLocked`, so a strand pane's shell is reed's declared config rather than tmux's ambient `default-shell`.
- A new `CONSTRAINTS.md` clause recording the invariant, plus an enforcement test in `internal/reedengine` that keeps every pane-launch site routed through the chokepoint.
- Hermetic unit tests for both shell dialects and for the reed chokepoint composition.
- Docs in the same commit: `internal/reedengine/doc.go`, `internal/shell/shell.go`, `CONSTRAINTS.md`, `crucible/README.md`, `docs/sandbox-howto.md`, the affected `tools/sandbox/SANDBOX-*.md` pre-condition sections, and the `shell` key comment in `internal/reedengine/template_posix.yaml` / `template_windows.yaml` (which now also declares the strand pane's shell and the prelude dialect).

**Out:**

- No stencil, skill, prompt or contract edits.
  The whole point of the `PATH` form is that no prompt has to remember `$LYX_BIN`.
- No change to `tools/sandbox/resolve.go`'s existing `prependPath` or to the `Dev/Prod Binary Separation` invariant — the sandbox launcher keeps prepending `.dev-bin` to its own child's environment, and the new reed mechanism agrees with it rather than replacing it.
- No `LYX_BIN` export added to the watchdog daemon spawn or the detached `lyx loom run` spawn (see the "Detached spawns stay untouched" Decision).
- No Selvage-pane or `new-session` first-pane prelude (see the "Chokepoint is the strand launch only" Decision).
- No new tmux subcommand, flag or capability requirement — `requiredSubcommands` (`internal/reedengine/probe.go`) is unchanged.
- No change to the two `shell.ForGOOS()` launch-command builders (`internal/shuttleengine/claudeengine/claudeengine.go:102`, `internal/loomcli/sharedbootstrap.go:259`), and no dialect threading through `shuttleengine.ReedOps` or `Engine.Prepare` — the op-boundary refusal makes them same-dialect by construction (see the `one-dialect-per-pane-enforced-at-the-op-boundary` Decision).
- No quoting of the `split-window` / `new-session` trailing shell argument at any site — the multiplexer, not the pane dialect, parses it, and psmux's parsing is unverified; the spaced-path limitation stays uniform across all three sites (see the `strand-panes-run-the-configured-shell` Decision's Known limitation).
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
    The `prelude-dialect-comes-from-cfg-shell` Rationale argues from exactly these fail-open semantics.
    The residual — a failed prelude leaves the pane on the ambient `PATH` — is bounded by the `one-dialect-per-pane-enforced-at-the-op-boundary` refusal, which removes the only realistic way for the prelude to be rejected.
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

### prelude-dialect-comes-from-cfg-shell

- Decision: select the `shell.Shell` implementation from **reed's configured pane shell** (`e.cfg.Shell`), never from `shell.ForGOOS()`.
  Match on the configured value's basename with any executable extension stripped, case-insensitively: `bash`, `sh`, `zsh`, `dash`, `ash`, `ksh` → `shell.Posix()`; `pwsh`, `powershell` → `shell.Pwsh()`.
  The dialect selector lives in `internal/reedengine/panebin.go` alongside the rest of the seam, and is a pure function of the config string so it is host-agnostically testable.
  A configured shell that matches neither set, or that is empty, is **refused at reed's op boundary** — never degraded, never sanitized.
  See the `one-dialect-per-pane-enforced-at-the-op-boundary` Decision below for the refusal and for why the prelude never has an unrecognized-shell path to degrade down.
- Rationale: `shell.ForGOOS()` keys on `runtime.GOOS`, but the pane shell is operator-configurable.
  `reed.yaml`'s `shell` key defaults to `${env:LYX_REED_SHELL:-bash}` on POSIX and `${env:LYX_REED_SHELL:-pwsh}` on Windows (`internal/reedengine/template_posix.yaml:2`, `template_windows.yaml:2`), and both template comments actively invite pinning an explicit path.
  With `LYX_REED_SHELL=bash` on Windows, `ForGOOS()` would emit `$env:PATH = …` into bash; because the prelude is `;`-joined ahead of the launch command, the pane would receive a syntax error followed by the agent's own command, and the `PATH` guarantee would fail while the strand still looked launched — the exact silent-mismatch failure class this task exists to eliminate.
  Deriving from the declared value rather than from `ForGOOS()` also keeps the selector meaningful if the cross-dialect refusal is ever relaxed in favour of threading the dialect to the command builders (see the same Decision's Rejected list).
- Rejected: `shell.ForGOOS()` — correct only when nobody has overridden `LYX_REED_SHELL`, i.e. exactly the "works until someone forgets" property the brief rejects.
  Also rejected: probing the pane's live shell at runtime (e.g. `display-message -p '#{pane_current_command}'`) — it costs a tmux round trip per launch, races the shell's own startup, and answers with the *foreground* command rather than the shell.
  Also rejected: emitting a dialect-agnostic prelude that both shells accept — no such syntax exists for a `PATH` prepend.
  Also rejected: warn-and-degrade on an unrecognized shell — see the next Decision; it would drop only the prelude half while `strand-panes-run-the-configured-shell` still put the pane on that same un-modelled shell.

### one-dialect-per-pane-enforced-at-the-op-boundary

- Decision: **one dialect governs a pane, it is declared by `reed.yaml`'s `shell` key, and reed refuses to run when that declaration cannot be honoured.**
  A new `validateToldPaneShell(cfg)` in `internal/reedengine/panebin.go` refuses three cases with a message naming `LYX_REED_SHELL`, the offending value, and the supported set:
  1. `e.cfg.Shell` is empty — note `ShellPath()` (`internal/reedengine/lock.go:66`) returns it verbatim and defaults nothing, so an empty value would otherwise reach `split-window` as an empty trailing argument.
  2. Its basename matches no known dialect (e.g. `fish`, `nu`, `cmd.exe`).
  3. Its dialect disagrees with `shell.ForGOOS()`'s dialect for this host (e.g. `LYX_REED_SHELL=bash` on Windows).

  **Where it fires — the pane-creating verbs only, never `withOpLock`.**
  Call it at the op boundary of `AddStrand`, `UpdateStrand` and `Resume`, beside the `validateAnchor` / `validateIfAbsent` calls `strand.go` already makes "at the op boundary, before any pane is launched or state persisted".
  Do **not** put it in `withOpLock`/`withTryOpLock` (`internal/reedengine/lock.go:88`, `:154`) alongside the three told-geometry validators.
  Those three refuse on every op because a bad tmux identity, anchor, or worktree root makes *every* verb meaningless.
  A bad pane shell does not: `Down`, `Status`, `AttachArgv`, and the `io.go` transport verbs are all still perfectly meaningful, and they are exactly the verbs an operator needs to recover.
  Putting the check in `withOpLock` would refuse `lyx reed down` and `lyx reed status` too, so an operator whose `reed.yaml` has already materialized a `fish` or empty `shell` value — reconcile is key-based and never rewrites a materialized value — could not tear the session down or inspect it, with hand-editing the file the only escape.
- Recovery path for an already-materialized bad value, stated because the refusal is deliberately narrow: `lyx reed down`, `status`, `attach` and the capture/send verbs keep working, and the operator fixes `reed.yaml`'s `shell` key or `LYX_REED_SHELL` and retries the add.
  Nothing is left half-created — the refusal fires before any state is loaded, any pane is reaped, or any strand record is appended.
- Rationale: this is the coherence rule the whole design turns on, and case 3 is what makes it hold end to end.
  A strand's `launchCmd` is **not** built by reed — it is built with `shell.ForGOOS()` at `internal/shuttleengine/claudeengine/claudeengine.go:102` and `internal/loomcli/sharedbootstrap.go:259`.
  Without case 3, `strand-panes-run-the-configured-shell` would put a pane deterministically on bash while the `;`-joined agent command typed into it stayed pwsh syntax, which is strictly worse than today's accident, where tmux's ambient `default-shell` usually happens to agree with `ForGOOS()`.
  With case 3, the configured shell, the pane's actual shell, the prelude, and every `ForGOOS()`-built launch command are the same dialect by construction, so those two call sites need no change and no dialect threading.
  Cases 1 and 2 remove the half-degraded pane entirely: there is no unrecognized-shell pane to type a `ForGOOS()`-built line into, because reed never gets that far.
  Refusal rather than sanitization or degradation is this package's established answer to an unhonourable told value — `validateToldTmuxIdentity`'s own doc comment argues it at length for the session name, and refuses at every op boundary before any tmux round trip.
  It is also the loud form of a breakage that already exists silently: a `fish` pane today receives a `ForGOOS()`-built POSIX launch line that fish does not accept, and nothing reports it.
- Deliberate asymmetry with `executable-error-warns-and-degrades`, stated so the two are not read as inconsistent: that Decision degrades because `os.Executable()` failing is a runtime failure of an unrelated primitive on a machine where the fallback `PATH` is the one the pane already had.
  This one refuses because the pane shell is **configuration**, it is wrong before any pane exists, and every downstream string depends on it.
- Operator-visible restriction, recorded rather than buried: `LYX_REED_SHELL` may pin a path, but not a cross-dialect or unmodelled shell.
  A `fish` or `nu` operator gets a refusal naming the supported set instead of a silently broken agent pane.
- Rejected: threading reed's dialect into the command builders — `shuttleengine.ReedOps` would grow a `PaneShell()` method, `Engine.Prepare`'s signature would grow a `shell.Shell` parameter across the provider seam, and `loomcli.statusStrandCmd` would take it too.
  That is the right shape *if* cross-dialect pinning must be supported, and this Decision is written so it can be adopted later by relaxing case 3 alone.
  It is rejected now because nothing asks for a cross-dialect pin, it widens this task across three modules and the `Shuttle Provider-Seam Invariant`'s boundary, and YAGNI applies.
  Also rejected: narrowing `strand-panes-run-the-configured-shell` back to a commandless split — that returns the pane's shell to ambient `default-shell` and leaves the prelude dialect derived from a value that does not govern the pane.
  Also rejected: validating in `LoadConfig` — `reedengine` is a **degrading** `LoadOrTemplate` consumer under the `Config Strictness Invariant`, so its config load must keep answering a missing file with the embedded template; the op boundary is where this package already refuses told values it cannot honour.

### strand-panes-run-the-configured-shell

- Decision: pass `e.cfg.Shell` as `split-window`'s trailing command argument in `launchStrandLocked`, exactly as `splitPaneBelowLocked` (`internal/reedengine/selvagepane.go:352`) already does for Selvage and as `new-session` (`lifecycle.go:337`) already does for the session's first pane.
- Rationale: this is what makes the `prelude-dialect-comes-from-cfg-shell` Decision *true* rather than merely declared.
  Today `launchStrandLocked` splits with **no** trailing command (`internal/reedengine/spawn.go:113`), so a strand pane runs tmux's own `default-shell` — and reed never issues `set-option default-shell` or `default-command` anywhere in the package, so that value is ambient (`$SHELL`, or tmux's compiled-in default), not reed's.
  A dialect derived from `e.cfg.Shell` while the pane actually runs something else would reintroduce the same mismatch one level down.
  Strand panes are the only panes reed creates whose shell is undeclared; this closes the inconsistency rather than adding a rule.
- Operator-visible change, stated rather than buried: on a POSIX machine whose `$SHELL` is `zsh`, strand panes currently come up as zsh and will come up as `bash` (the `reed.yaml` default) after this change, unless `LYX_REED_SHELL` is set.
  That is the point — the pane shell becomes declared config rather than ambient environment — and it is recorded in `internal/reedengine/doc.go` and in the `reed.yaml` `shell` key's own comment.
- Spaced shell paths: the trailing argument is passed **verbatim, unquoted**, exactly as Selvage's split (`splitPaneBelowLocked`, `internal/reedengine/selvagepane.go:352`) and `new-session` (`lifecycle.go:337`) already pass the same value.
  The quoting authority here is the **multiplexer's own argv parsing**, not the pane shell's dialect — tmux or psmux parses this argument before any pane shell exists.
  That distinction is easy to miss because the two coincide on POSIX: native tmux hands a single trailing shell-command to `/bin/sh -c`, so the correct quoting there happens to be POSIX quoting, which is also what the POSIX pane dialect emits.
  On Windows they do not coincide, and psmux's parsing of a trailing shell-command is **unverified anywhere in this package** — the same unverified-until-proven standard this discussion already invokes to reject `split-window -e`.
  Quoting would emit `pwshShell.Quote`'s single-quoting (`internal/shell/pwsh.go:12`) into psmux, replacing a shipped default that demonstrably works unquoted (`shell: pwsh`, `template_windows.yaml:2`) with an untested one.
  So: no quoting, no `runtime.GOOS` branch, and behaviour identical to the two existing sites.
- Known limitation, recorded rather than half-fixed: a spaced absolute path in `shell` is word-split by the multiplexer at **all three** sites — the strand split, Selvage's split, and `new-session`.
  This task neither introduces nor retires it; it keeps the three uniform.
  Retiring it is a follow-up that must fix all three together and needs psmux's trailing-argument parsing verified first, which is a live-substrate question no hermetic test can answer.
  A sandbox pre-condition line covers the live case (`LYX_REED_SHELL` pinned to a spaced absolute path), and is what would surface it.
- Rejected: leaving the split commandless and deriving the dialect from `$SHELL` or from a `pane_current_command` probe — it would make reed's own `shell` config key a lie for the majority of its panes, and keeps the dialect ambient.
  Also rejected: deriving the dialect from `e.cfg.Shell` while leaving the actual pane shell ambient — a knowingly-unsound premise.

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
  The Selvage pane (`splitPaneBelowLocked`, `internal/reedengine/selvagepane.go:352`) and the session's first pane (`new-session`, `lifecycle.go`) do not.
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
`ForGOOS()` is the wrong selector here and must not be used — see the `prelude-dialect-comes-from-cfg-shell` Decision.

**The pane shell is ambient today, and this task makes it declared.**
`launchStrandLocked` splits with no trailing command (`internal/reedengine/spawn.go:113`), and the package issues no `set-option default-shell` or `default-command` anywhere — every `set-option` call in `lifecycle.go`, `windowsize.go` and `statusline.go` targets `remain-on-exit`, `mouse`, `status*`, `window-size` or `window-status-format`.
So a strand pane's shell is tmux's ambient default, while `e.cfg.Shell` governs only the `new-session` first pane (`lifecycle.go:337`) and Selvage (`selvagepane.go:235`).
`e.cfg.Shell` itself resolves from `reed.yaml`'s `shell` key, `${env:LYX_REED_SHELL:-bash}` on POSIX and `${env:LYX_REED_SHELL:-pwsh}` on Windows, so it can be any absolute path an operator pins.
Note `internal/reedengine/lock.go:66`'s `ShellPath()` accessor: it returns `e.cfg.Shell` verbatim, validating and defaulting nothing — an empty configured shell comes back as the empty string, which is why `validateToldPaneShell` refuses it outright rather than degrading or guessing (see the `one-dialect-per-pane-enforced-at-the-op-boundary` Decision; there is no degrade path).

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
- **Documentation Lifecycle** — docs land in the same commit: `internal/reedengine/doc.go`, `internal/shell/shell.go`, `CONSTRAINTS.md`, `crucible/README.md`, `docs/sandbox-howto.md`, the affected `tools/sandbox/SANDBOX-*.md`, and the `shell` key comment in both `reed.yaml` templates.
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
- Dialect selection, as a table test over configured-shell strings: `bash`, `/usr/bin/bash`, `/bin/sh`, `zsh`, `dash`, `ash`, `ksh` → POSIX; `pwsh`, `pwsh.exe`, `PWSH.EXE`, `C:\Program Files\PowerShell\7\pwsh.exe`, `powershell` → pwsh; `fish`, `cmd.exe`, `nu`, and the empty string → unrecognized.
  An explicit test asserts the selector never reads `runtime.GOOS`, so it is host-independent.
- `validateToldPaneShell`, as a table test over its three refusal cases plus the accepting case, asserting each refusal's message names `LYX_REED_SHELL`, the offending value, and the supported set.
  Case 3 (cross-dialect) is the one that needs a host-independent shape: the comparison target is `shell.ForGOOS()`'s dialect for the running host, so the test must derive the expectation the same way rather than hardcoding pwsh or POSIX, and must assert both that a same-dialect value is accepted and that the opposite-dialect value is refused.
  Also assert **where** it fires: `AddStrand`, `UpdateStrand` and `Resume` refuse before any tmux round trip, any state load, or any strand-record append; and `Down`, `Status`, `AttachArgv` and the `io.go` transport verbs still succeed with the same bad configured shell — the recovery path the Decision promises.
  That second half is the one a `withOpLock` placement would break, so it needs its own explicit test rather than being implied by the first.
- `split-window` argv now carries `e.cfg.Shell` verbatim as its trailing argument: assert it on the fake tmux recorder, including that a spaced absolute path is passed through **unmodified** (no quoting, no escaping), and assert Selvage's own split argv (`splitPaneBelowLocked`) and `new-session`'s argv are both unchanged — all three sites stay uniform.

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
- Pin `LYX_REED_SHELL` to a **spaced absolute path** (e.g. `C:\Program Files\PowerShell\7\pwsh.exe`) and confirm the strand pane still comes up — this is the live half of the quoted-trailing-argument fix, which no hermetic test can prove.
- Pin `LYX_REED_SHELL` to an unmodelled shell (`fish`) and confirm reed refuses with a message naming the supported set, rather than launching a broken pane.
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
- **Q:** [discussion-review r1 gap, BLOCKING:design] Which `shell.Shell` composes the prelude, and what happens when `LYX_REED_SHELL` pins a shell that contradicts `runtime.GOOS`? **A:** [auto-pick] derive the dialect from `e.cfg.Shell` by basename match, never from `shell.ForGOOS()`. **Superseded in r2:** the "unrecognized or empty shell emits no prelude and logs a `Warn`" half was replaced by a hard refusal at the pane-creating verbs' op boundary — see the r2 entry below and the `one-dialect-per-pane-enforced-at-the-op-boundary` Decision. **Why:** `ForGOOS()` is correct only until someone overrides `LYX_REED_SHELL`, and a `$env:PATH = …` statement `;`-joined ahead of an agent's command in bash is a syntax error that fails the `PATH` guarantee while the strand still looks launched.
- **Q:** [discussion-review r1 gap, follow-on] `e.cfg.Shell` governs only the `new-session` first pane and Selvage today — a strand pane runs tmux's ambient `default-shell`. Derive the dialect from config anyway, or make config true? **A:** [auto-pick] make it true — pass `e.cfg.Shell` as `split-window`'s trailing command in `launchStrandLocked`, as Selvage's own split already does. **Why:** deriving a dialect from a value that does not govern the pane would reintroduce the same mismatch one level down; strand panes are the only panes reed creates whose shell is undeclared.
- **Q:** [discussion-review r2 gap, BLOCKING:design] A strand's `launchCmd` is built with `shell.ForGOOS()` at two call sites, so pinning the pane shell to `e.cfg.Shell` makes the pane deterministically bash while the typed command stays pwsh. Thread the dialect to the command builders, or narrow the pane-shell Decision? **A:** [auto-pick] neither — refuse a cross-dialect `LYX_REED_SHELL` at reed's op boundary, which makes the two `ForGOOS()` sites same-dialect by construction and leaves them unchanged. **Why:** threading would grow `ReedOps` and `Engine.Prepare` across the `Shuttle Provider-Seam Invariant`'s boundary for a configuration nobody asks for; the refusal is one function and can be relaxed later if a cross-dialect pin is ever wanted.
- **Q:** [discussion-review r2 gap, BLOCKING:design] The unrecognized-shell degrade drops the prelude but still passes that shell to `split-window`, so only half degrades. What is the trailing argument then? **A:** [auto-pick] there is no degrade — an empty or unmodelled `e.cfg.Shell` is refused at the op boundary, so no pane is ever created on an un-modelled shell. **Why:** refusal rather than sanitization is this package's established answer to an unhonourable told value (`validateToldTmuxIdentity`), and it is the loud form of a breakage that already exists silently — a `fish` pane today gets a POSIX launch line fish does not accept, with nothing reporting it.
- **Q:** [discussion-review r2, NIT:design] Is `Chain`'s separator `;` or `&&`? **A:** [auto-pick] `"; "` in both dialects, fail-open, dropping empty parts. **Why:** the prelude is env decoration and must not be able to suppress the agent's launch; `&&` is additionally a pwsh 7.0+ pipeline-chain operator with different semantics from POSIX's.
- **Q:** [discussion-review r2, NIT:design] A spaced configured-shell path becomes `split-window`'s trailing shell-command, which tmux word-splits. **A:** [auto-pick] quote it through the dialect's `Quote` at the strand split, leave Selvage's and `new-session`'s unquoted sites as pre-existing and out of scope, and add a sandbox pre-condition line for a spaced `LYX_REED_SHELL`. **Why:** this task must not widen a known hazard to a third site, but the other two are a separate change — tmux treats `new-session`'s trailing args as command-plus-arguments rather than one `sh -c` string, so it is not even the same fix.
- **Q:** [discussion-review r3 gap, BLOCKING:design] `validateToldTmuxIdentity` fires from `withOpLock`, which every public op passes — placing the pane-shell refusal there would refuse `down` and `status` too, wedging an operator whose `reed.yaml` already carries a bad `shell` value. Which ops does the refusal bind? **A:** [auto-pick] the pane-creating verbs only — `AddStrand`, `UpdateStrand`, `Resume` — beside the `validateAnchor`/`validateIfAbsent` op-boundary calls `strand.go` already makes; never `withOpLock`. **Why:** a bad tmux identity makes every verb meaningless, but a bad pane shell does not — `down`, `status`, `attach` and the transport verbs stay meaningful and are exactly the recovery path, so refusing them would leave hand-editing `reed.yaml` as the only escape.
- **Q:** [discussion-review r3 gap, BLOCKING:design] The `split-window` trailing argument is parsed by tmux/psmux before any pane shell exists, but the r2 fix quoted it with the pane dialect. What is the quoting authority, and what does Windows emit? **A:** [auto-pick] the multiplexer's argv parsing, not the pane dialect — and pass the value verbatim at all three sites, quoting nowhere. **Why:** the two coincide only on POSIX (`sh -c`), which is what hid the category error; on Windows, quoting would emit pwsh single-quoting into psmux, whose trailing-argument parsing is unverified, replacing a shipped default that demonstrably works unquoted. The spaced-path limitation stays uniform across all three sites and is recorded as a follow-up that must fix them together.
- **Q:** [discussion-review r3, NIT:consistency] Technical context still told a plan writer to degrade on an empty shell, and the r1 Q&A entry still described the retired `Warn` path. **A:** [auto-pick] rewrite both — Technical context now states the refusal, and the r1 entry carries an explicit **Superseded in r2** marker. **Why:** a plan writer reading Technical context alone would have implemented exactly the degrade the Decisions forbid.
- **Q:** [discussion-review r3, NIT:consistency] The out-of-scope rationale claimed `new-session` takes command-plus-arguments rather than one `sh -c` string, but `lifecycle.go:330-338` passes exactly one trailing argument. **A:** [auto-pick] drop the claim; the exclusion now rests on the multiplexer-parsing argument and on keeping all three sites uniform. **Why:** the claim was factually wrong, and the remaining rationale stands without it.
