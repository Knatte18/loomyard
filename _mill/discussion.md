# Discussion: reed: born-as-strand for loom start's operator attach

```yaml
task: "reed: born-as-strand for loom start's operator attach"
slug: reed-born-as-strand
status: discussing
parent: main
```

## Problem

`lyx loom start` ends by handing the operator's terminal to a bare `tmux attach-session` (`internal/loomcli/start.go`, step 7).
Nothing about that handoff registers anything with reed.
The session the operator lands in contains the Selvage control pane, the `loom-status` strand, and whatever agent strands the detached driver spawns later — but nothing that belongs to the operator, and nothing reed's strand table knows the operator is sitting in.

Every other way into a worktree already does this correctly.
`lyx ide spawn`'s generated VS Code `folderOpen` chain (`internal/vscode/config.go`) runs `reed up` → `reed add --if-absent --cmd <claude> --name claude --focus` → `reed attach`, so the session running the operator's agent *is* a tracked Strand — which is exactly what the `/ly:ly-drive` skill's own self-check (`$TMUX_PANE` vs `lyx reed status`) relies on.
Every LLM agent lyx's Go code launches goes through `internal/shuttleengine.Runner.Start`, which calls `AddStrand` unconditionally.
`loom start`'s attach is the one remaining entry point with no `AddStrand` at all.

**Why now:** reed has grown features that only work for Strands.
The per-hub watchdog daemon's reconcile/reap loop treats untracked panes as debris and kills them once an alive Selvage authorizes the reap (`internal/reedengine/reconcile.go`, referenced from `spawn.go`'s `planPaneTarget` doc comment).
An operator who lands via `loom start` and splits themselves a pane by hand is therefore creating something the daemon is entitled to destroy — the M16 finding recorded in `planPaneTarget` is that same collision seen from the other side.
`reed: strand-based mailbox/addressing system` (`manifest/designs/reed-mailbox.md`) additionally depends on this for its receive side: nothing can be addressed before it exists as a Strand.
The design's own stated blocker — the header-pane/Selvage split (`manifest/designs/reed-header-selvage.md`) — has shipped; it sits in roadmap Done, so this item is unblocked.

## Scope

**In:**

- `internal/loomcli/start.go`: add an operator Strand immediately before the terminal-handover tail, then attach — the `reed add` + `reed attach` shape the VS Code chain already uses.
- `internal/loomcli/bootstrap.go`: the pure pieces the new step composes over (the pinned display-name constant and the command builder), beside the existing `statusStrandDisplayName` / `statusStrandCmd` pair.
- `internal/reedengine/lifecycle.go` (or wherever the existing accessors sit): one exported accessor, `func (e *Engine) Shell() string`, so `loomcli` can reuse the shell reed already launches Selvage with (`e.cfg.Shell`) instead of inventing its own.
- Gating: `--no-attach` skips the operator strand as well as the attach.
- Watchdog parity: `loom start` spawns the per-hub watchdog daemon, via `internal/reedcli/spawnwatchdog.go`'s logic moved to a new `internal/reedengine/spawnwatchdog.go` as an exported package-level function. `internal/reedcli`'s method becomes a one-line caller; `internal/loomcli` gains a `suppressWatchdogSpawn` field on `loomCLI`, set in both of its constructors.
- Docs in the same commit: `manifest/designs/loom.md`'s `start` step list (it enumerates steps 1–4 verbatim), `manifest/designs/reed-born-as-strand.md`, `manifest/roadmap.md` (Planned → Done, since this completes a planned item).

**Out:**

- Any form of pane **adoption**. `loom start` never claims an already-running pane. This is a hard constraint, not a preference — see the Decision below.
- Refactoring the existing `loom-status` strand's keep/replace/add dance (`resolveStatusStrandAction`) onto `AddSpec.IfAbsent`. It is a genuine simplification and a genuine separate task; touching it here doubles the blast radius of a change whose whole risk surface is the bootstrap's attach tail.
- Making the detached driver (`lyx loom run`) a pane. It is deliberately detached with file-redirected output (`manifest/designs/loom.md`: "loom goes to the background and the tmux session takes the window") and needs no TTY.
- Launching an agent (`claude` or any other) from `loomcli`. Provider specifics belong to `internal/shuttleengine/claudeengine` per the Shuttle Provider-Seam Invariant; `loomcli` names no agent binary.
- `lyx loom step` and `lyx loom run`. Neither hands a terminal over, so neither has this gap.
- The Selvage pane's own identity. It stays what it is today — tracked as `ReedState.SelvagePaneID`, not as a Strand. Converting it is out of scope and out of this design's intent.

## Decisions

### operator-strand-is-the-thing-born

- Decision: `loom start` adds one new Strand of its own — the operator's working pane — and attaches with that pane focused. It does not turn any existing pane, process, or client into a Strand.
- Rationale: the design's sentence "spawn the pane as a Strand first, then attach" has exactly one shape that is both buildable and consistent with the rest of lyx: the VS Code chain's add-then-attach. An attach *client* cannot be a Strand (a Strand is a pane with a command), and the driver is deliberately paneless. What is genuinely missing is a tracked pane belonging to the operator, so that the watchdog will not reap what they work in and the future mailbox has something to address.
- Rejected: **(a)** focus the existing Selvage pane instead and add nothing — cheapest, but it satisfies none of the stated motivation: Selvage is not a Strand, is pinned to a one-row band at the bottom, and is not addressable by anything that reasons over the strand table. **(b)** make the detached driver a pane — contradicts `loom.md`'s explicit design and gains nothing, since the driver is already fully observable through its log and the status strand. **(c)** register the attach client itself — not representable in reed's model.

### never-adopt

- Decision: the new strand is always a fresh split via `AddStrand`. No code path inspects live panes and claims one.
- Rationale: adoption was built and deliberately removed. `internal/reedengine/spawn.go`'s `planPaneTarget` doc comment records two live findings — **R4-F5** (adoption picked the previous header pane, still running `lyx reed header --blocking`; the strand's command was typed onto its screen, never ran, and status reported `live:true` with no such process) and **M16** (adoption claimed an operator's own manually-split pane). The seam adoption needs — telling reed's own idle pane from a foreign one — could not be made safe.
- Rejected: adopting the session's initial pane to avoid a "needless" extra split. `planPaneTarget` already answers this: once the untracked reap is authorized by an alive Selvage, the initial pane is disposed of before any split is planned, so a fresh split is idle by construction and costs one `kill-pane` plus one `split-window`.

### shell-not-agent

- Decision: the operator strand's command is reed's own configured shell (`ReedConfig.Shell`, the same string `ensureSelvagePaneLocked` passes to `splitSelvagePaneAtBottomLocked`), reached through a new exported `Engine.Shell()` accessor.
- Rationale: it is the shell reed already commits to for an interactive pane on this platform, so behaviour matches Selvage exactly and no second notion of "the operator's shell" enters the codebase. From that pane the operator runs `claude`, `lyx reed add`, git, anything. `loomcli` names no agent binary, which keeps the Shuttle Provider-Seam Invariant intact, and it stays on the right side of the Shell Mechanics Seam.
- Rejected: **(a)** hardcode `claude`, mirroring the VS Code chain literally — puts a provider name in `loomcli` and makes a terminal bootstrap unusable without that binary. **(b)** compose a command via `shell.ForGOOS()` the way `statusStrandCmd` does — that seam builds a *command line* to invoke a known binary with arguments, which is not what launching a bare interactive shell needs.

### idempotent-via-ifabsent

- Decision: the add uses `AddSpec{NameOverride: <pinned constant>, IfAbsent: true}`. The name is a package-level constant in `bootstrap.go`, beside `statusStrandDisplayName`.
- Rationale: `loom start` is explicitly re-entrant — a second invocation while a driver is alive ensures substrate and attaches rather than spawning a second driver (`mustSpawnDriver`). The operator strand must be re-entrant on the same terms. `AddSpec.IfAbsent` already implements exactly the needed three-way behaviour in the engine: a live match is returned unchanged, a dead match is relaunched under its own guid, an unmatched name falls through to an ordinary add. `validateIfAbsent` requires a non-empty `NameOverride`, which the pinned constant supplies.
- Rejected: repeating the status strand's manual `resolveStatusStrandAction` keep/replace/add dance. It predates `IfAbsent` and is the strictly worse of the two mechanisms — it needs a `RemoveStrand` round-trip to handle a dead entry, and its failure path degrades to `statusStrandKeep`. Writing a second copy of it would entrench the older pattern in the same file that should eventually shed it.

### display-below-parent-focused-no-shrink

- Decision: `Display{Anchor: render.AnchorBelowParent, Focus: true, ShrinkWhenWaitingOnChild: false}`.
- Rationale: `Focus: true` is the whole point — the operator must *land* in their own pane, which is what the VS Code chain's `--focus` achieves. `AnchorBelowParent` is the only non-hidden anchor supported in v1 (`validateAnchor` rejects `own-window` as deferred). `ShrinkWhenWaitingOnChild` is false as a deliberate declaration of intent, not because it changes today's rendering: `stackHeights` (`internal/reedengine/render/height.go`) marks a strand a strip only when `isAncestor(s, stack) && s.Display.ShrinkWhenWaitingOnChild`, and the operator strand is parentless and childless, so the flag is inert for it either way. Setting it false says what should happen if a child is ever parented onto this strand — the operator's own pane must not collapse to a one-row strip while a child runs, because unlike the status strand it is a pane someone is typing in. Setting it true would encode the opposite intent by accident.
- Rejected: `AnchorHidden` (a pane the operator cannot see is not a working pane); `ShrinkWhenWaitingOnChild: true` copied from the status strand and `reed add`'s own default without thinking about what it means here.

### operator-pane-takes-a-permanent-full-share

- Decision: accept that the operator pane consumes a full share of window rows for the rest of the run, exactly as the `loom-status` strand already does. No mitigation is built.
- Rationale: `stackHeights` splits `usable - stripDemand` rows evenly across every non-strip pane, and per the decision above the operator strand is never a strip. So it is a permanent full-share consumer, and every agent pane the driver spawns gets a smaller share than it does today: a run showing Selvage (its own fixed band) plus `loom-status` plus one agent pane divides the remainder three ways instead of two. This is the identical question the repo has already answered once — `manifest/designs/loom.md` records the Decision `childless-full-height-is-acceptable` for the status strand, in the same words ("a childless status strand rendering full-height is intended, not a bug to re-file"). Deciding it differently for the operator's own pane would be inconsistent, and the pane the operator types in has the strongest claim of any pane to real height.
- Rejected: **(a)** `ShrinkWhenWaitingOnChild: true` as a mitigation — inert, per the decision above, so it would be a comment pretending to be a fix. **(b)** a fixed-height pin for the operator pane — `render.Pin`/`fixedHeightPins` exists, but pinning is how Selvage gets its band, and a pinned operator pane is a worse Selvage rather than a working terminal. **(c)** teaching `stackHeights` to deprioritise parentless strands — a render-policy change affecting every strand in every session, far outside this task, and it would silently re-height the status strand too.
- Operator escape if the layout is genuinely too tight on a small terminal: `lyx reed remove <guid>` drops the pane, or bootstrap with `--no-attach` and attach separately with `lyx reed attach`. Neither is a code path this task adds; both already exist.

### no-attach-skips-the-strand

- Decision: `mustAttach(noAttachFlag) == false` skips the operator strand too. The strand add is inside the same gate as the handover, not before it.
- Rationale: `--no-attach` exists for CI and debugging — "perform every bootstrap step and return". An invocation that never hands a terminal over has no operator to give a pane to, and adding one would leave an idle shell pane in every CI worktree, which the watchdog would then dutifully keep alive because it is tracked. The design's "running `loom start` outside reed remains a valid escape hatch for debugging/CI" is preserved precisely by this gating.
- Rejected: a separate `--no-strand` flag. Two flags for one situation; no caller wants the attach without the pane or the pane without the attach.

### watchdog-parity

- Decision: `loom start` also spawns the per-hub watchdog daemon, best-effort. The existing `internal/reedcli/spawnwatchdog.go` logic moves to **`internal/reedengine`** as an exported package-level function, and `reedcli`'s `ensureWatchdogSpawned` method becomes a one-line caller. This is unconditionally in scope for this task, not a reviewer's call.
- Decision (gating): the watchdog spawn sits **outside** the `mustAttach` gate, beside the substrate step (`ensureStatusStrand`), not in the handover tail. The operator strand's placement inside the gate is unchanged.
- Rationale: `ensureWatchdogSpawned` is today called from exactly three places — `reedcli`'s `up`, `attach` and `resume` — so an operator who enters a worktree via `lyx loom start` gets no watchdog at all. That is not incidental to this task: the daemon's reconcile/reap loop is the first of the two Strand-only features the design names as the reason for doing this work. A Strand born into a session nothing reconciles delivers half the value the item was scheduled for.
- Rationale (gating): the daemon is per-hub and reconciles every worktree's session, and `reedcli`'s `up` spawns it with no attach involved. `loom start`'s substrate step is the `up` analogue — it brings the session up and adds the status strand on every invocation, `--no-attach` included, and the detached driver then spawns agent strands into that session whether or not a human is watching. Those strands need reconciling either way, so tying the daemon to the handover would leave every `--no-attach` bootstrap unsupervised. This is the one place the two additions are deliberately gated differently: a pane is for an operator who is there; a daemon is for a session that exists.
- Rejected: **(a)** leaving it out as scope creep — defensible on size, but it means shipping the item and still not having the stated benefit on the `loom start` path. **(b)** copying the function body into `loomcli` — it re-execs `os.Executable()` with a hub path and a tmux path, holds the `suppressWatchdogSpawn` test guard, and has Windows-specific `cmd.Dir` reasoning; a second copy would drift. **(c)** putting the seam anywhere `loomcli` cannot reach — see the seam-ownership decision below.

### watchdog-seam-lives-in-reedengine

- Decision: the extracted function lives in `internal/reedengine` (a new `spawnwatchdog.go`, mirroring the filename it comes from), exported, taking its inputs explicitly rather than reading a receiver: the hub path, the tmux path, and a `suppress bool`. `internal/reedcli` and `internal/loomcli` both call it.
- Rationale: `loomcli` already imports `reedengine` (`internal/loomcli/sharedbootstrap.go`) and imports no `*cli` package anywhere outside `cmd/lyx` wiring and smoke tests, so `reedengine` is the only home that needs no new import direction. It also satisfies the CLI / Cobra Invariant's package-naming rule directly: `<module>cli` imports `<module>engine`, engine imports no cli package, and `loomcli → reedengine` is an already-shipped edge rather than a new deviation to record. The function's inputs are all things `reedengine` already owns or is handed (`Engine.TmuxPath()`, `fabricengine.HubScratchDir(hubPath)`).
- Rejected: **(a)** a new shared package for one function — `internal/reedcli` → `internal/reedengine` is a documented edge and a third package buys nothing. **(b)** leaving it in `reedcli` and having `loomcli` import it — reverses the invariant's stated direction and makes `loomcli` depend on another module's cobra layer. **(c)** an `Engine` method rather than a package function — tempting since `Engine` holds the tmux path, but the daemon is per-*hub* and deliberately outlives any one worktree's engine (`cmd.Dir` is pinned to the hub for exactly that reason); a method would imply an ownership that does not exist.

### watchdog-suppression-is-passed-in-not-inferred

- Decision: the `suppress` argument is supplied by each caller, not computed inside the seam. `reedcli` passes its existing `c.suppressWatchdogSpawn` field unchanged. `loomcli` gains the same field on `loomCLI`, initialised from `testing.Testing()` the way `reedcli.Command()` already does (`internal/reedcli/cli.go`: `c := &reedCLI{suppressWatchdogSpawn: testing.Testing()}`).
- Rationale: this is what enforces the Live-Substrate Spawn Observability invariant's "never re-exec `os.Executable()` under `go test`" — without it, every `loomcli` test that reaches the bootstrap would re-exec the test binary and run the whole suite recursively. Passing it explicitly keeps the guard visible at both call sites instead of hiding a `testing` import inside an engine package that has none today.
- Gotcha for the plan writer: `loomcli` has **two** constructors. `Command()` builds `&loomCLI{}`, and `StartAliasCommand()` (`internal/loomcli/start.go`) builds a second, independent `&loomCLI{}` for the root-level `lyx start` alias. Both must set the field, or the alias re-execs under test while the subtree verb does not.
- Rejected: calling `testing.Testing()` inside the seam itself. It would work — `reedcli` already imports `testing` in production code — but it moves a test-only concern into `reedengine`, which currently has none, and hides from each call site that the guard exists at all.

### escape-hatch-unchanged

- Decision: no new refusal. `loom start` does not check whether it is already running inside a reed pane, and does not refuse or warn when it is not.
- Rationale: the design states running `loom start` outside reed stays a valid escape hatch, mirroring `loom run`'s relationship to `loom start`. The handover tail is the CLI/Cobra Invariant's narrow interactive-handoff exception precisely because everything fallible has already been reported by then; adding a new refusal there would widen an exception the invariant deliberately keeps narrow.
- Rejected: a `$TMUX_PANE`-based self-check like `/ly:ly-drive`'s. That check exists for an *agent session* that cannot see its own launch chain; `loom start` is the launch chain.

## Technical context

**The file that changes.** `internal/loomcli/start.go`'s `startCmd` RunE runs seven steps. Steps 1–6 are pre-flight on the JSON envelope; step 7 is the terminal handover and reports nothing.
The new strand add belongs at the top of step 7's block, after `_ = bootstrapLock.Release()` and inside the `mustAttach` gate, alongside the existing `c.reed.Status()` pre-flight call and before `term.GetSize`.
Placing it there, not earlier, matters for two reasons: the bootstrap lock must not be held across it (the lock's release point is load-bearing and documented in-file), and it must be inside the `mustAttach` gate per the `no-attach-skips-the-strand` decision.

**The engine surface already available.** `c.reed` is a concrete `*reedengine.Engine` (`internal/loomcli/cli.go`), so nothing needs a new interface.
`AddStrand(AddSpec) (Strand, error)`, `Status() (StatusResult, error)`, `Up() (UpResult, error)`, `RemoveStrand(guid, bool)`, `TmuxPath()` and `AttachArgv(cols, rows)` are all already used from `loomcli`.
The only missing piece is the configured shell: `ReedConfig.Shell` (`internal/reedengine/config.go:20`) has no exported accessor. Add one in the style of the existing `TmuxPath()` / `Socket()` / `SessionName()`.

**The pattern to copy.** `internal/loomcli/sharedbootstrap.go`'s `ensureStatusStrand` is the nearest existing example of `loomcli` adding a strand, and `internal/vscode/config.go`'s `reed add claude` task is the nearest example of the add-then-attach shape.
`internal/reedcli/add.go` shows the exact `AddSpec` assembly including `Display.Focus`.

**Where `AddSpec.IfAbsent` is implemented.** `internal/reedengine/strand.go` — `validateIfAbsent` (requires `NameOverride`) plus `classifyIfAbsent`, which is what produces the no-op / relaunch-dead / fall-through-to-add behaviour this task depends on rather than reimplementing.

**Watchdog extraction.** `internal/reedcli/spawnwatchdog.go` holds `ensureWatchdogSpawned` as a `*reedCLI` method reading `c.suppressWatchdogSpawn`, `c.hubPath` and `c.eng.TmuxPath()`.
The body moves verbatim to a new `internal/reedengine/spawnwatchdog.go` as an exported package-level function over those three values passed in explicitly (hub path, tmux path, suppress flag), leaving the `reedCLI` method as a one-line caller so `up`/`attach`/`resume` keep behaving identically.
Everything it does — `fabricengine.HubScratchDir`, `os.Executable`, `proc.Detach`, the two `logger` calls, the hub-pinned `cmd.Dir` — is already available inside `reedengine`; the move adds no new import to that package except `os/exec`, `proc` and `fabricengine`, none of which creates a cycle (`fabricengine` does not import `reedengine`).
`loomcli` needs a hub path to call it: it has `c.location`, and `fabricengine.HubScratchDir` is what the function derives its scratch dir from, so the hub path is reachable without new geometry resolution.
The `suppress` value's source and the two-constructor gotcha are in the `watchdog-suppression-is-passed-in-not-inferred` decision above.

**Layout arithmetic, for the row-budget decision.** `internal/reedengine/render/height.go`'s `stackHeights` computes `usable = box.H - (n-1)` dividers, marks a strand a strip only when `isAncestor(s, stack) && s.Display.ShrinkWhenWaitingOnChild`, then splits the remaining rows evenly across the non-strip panes with the remainder going to the active (deepest, last) pane.
`orderStack` sorts by chain depth, so a parentless operator strand is never the deepest once any agent strand exists.
Both facts together are why the operator pane is a permanent full-share consumer and why its `ShrinkWhenWaitingOnChild` value is inert — see the two decisions on this above.

**Gotcha — the `StartAliasCommand` twin.** `lyx start` is the same `startCmd` registered a second time as a root child with `resolvePersistentPreRun` attached manually.
It takes `startCmd` unchanged, so it inherits this change for free — but any help-tree test pinned on the command's flags or `Short` covers both registrations.

**Gotcha — what `AttachArgv` already does.** The attach is not a bare `attach-session` any more; `AttachArgv(cols, rows)` chains a `select-layout` computed for the operator's terminal size, degrading to the bare argv on a non-positive size.
The new strand is added *before* that argv is built, so the layout the attach applies already accounts for the new pane. Adding the strand after `term.GetSize` would compute a layout for a pane count that is about to change.

**Gotcha — `planPaneTarget` and Selvage.** The new pane is split from the tallest alive non-Selvage pane. On a freshly-bootstrapped worktree that is the `loom-status` strand, which is the intended parent-ish neighbour; the layout engine then places both under `AnchorBelowParent` rules. Nothing here needs a `Parent` guid — the status strand is not this strand's parent in the strand-tree sense, and setting one would give the operator's pane the status strand's lifecycle.

## Constraints

From `CONSTRAINTS.md`:

- **CLI / Cobra Invariant.** The interactive-handoff exception is enumerated per command and already lists `lyx loom start` / `lyx start`. This change must not widen it: no new fallible step may report on the envelope *after* the handover begins, and the strand add must sit before `attach.Run()`. A failed `AddStrand` before the handover is an ordinary pre-flight error on the envelope; it must not be silently swallowed. `Short` stays non-empty; no new flag is added.
- **Live-Substrate Spawn Observability.** The strand add starts a real OS process (a shell in a new pane) — reed's own `launchStrandLocked` already logs it. The watchdog spawn is a detached `Start` with no `Wait`, so it logs the spawn alone; the extracted seam must keep `spawnwatchdog.go`'s existing `logger.Info`/`logger.Warn` calls. **Never re-exec `os.Executable()` under `go test`** — this is why `suppressWatchdogSpawn` exists, and the extraction must preserve that guard for every caller, `loomcli` included.
- **Test Tier Purity Invariant.** No `exec.Command`, no `gitexec`, no `hubforge.NewHub` in untagged test files. Everything touching a real tmux session belongs in a `smoke`-tagged file.
- **Shell Mechanics Seam.** Shell quoting/invocation belongs to `internal/shell`. Reusing reed's configured `Shell` string verbatim stays inside that boundary — it is a command, not a composed command line, so it needs no `Quote`/`Invoke` treatment.
- **Shuttle Provider-Seam Invariant.** No provider name (`claude`, a model id, an engine name) may appear in `loomcli` as a result of this change.
- **Markdown Link Integrity.** The doc updates must not leave a broken relative link; `manifest/roadmap.md`'s numbered-list maintenance rules apply when the item moves to Done.
- **Documentation Lifecycle** (project `CLAUDE.md`). This changes observable CLI behaviour and completes a planned roadmap item, so `manifest/designs/loom.md`, `manifest/designs/reed-born-as-strand.md` and `manifest/roadmap.md` update in the same commit as the code.
- **cgo build prerequisite.** `CGO_ENABLED=1` with a C compiler on `PATH` — `go build`/`go test` do not work without it in this repo.

## Testing

**Tier 1, untagged, in `internal/loomcli`.** The new pure pieces go in `bootstrap.go` beside `mustAttach` / `statusStrandCmd`, which is what makes them testable with no process and no lock — the same split `bootstrap_test.go` already exercises.

- The operator strand's `AddSpec` builder: assert the pinned name constant, `IfAbsent: true`, `Display.Anchor == render.AnchorBelowParent`, `Display.Focus == true`, `Display.ShrinkWhenWaitingOnChild == false`, and that `Cmd` is the shell string handed in rather than a composed command line. **TDD candidate** — it is a pure function with a closed output shape.
- The name constant is distinct from `statusStrandDisplayName`. A one-line test, but it guards the exact failure the status strand's own doc comment describes (reed has no upsert; a name collision appends a second pane).
- Gating: the operator strand is added only when `mustAttach` is true. If this is expressed as a predicate rather than an inline `if`, it is a **TDD candidate** on the same terms as `mustAttach`/`mustSpawnDriver`.

**Tier 1, untagged, in `internal/reedengine`.** `Engine.Shell()` returns the configured shell — a trivial accessor test in the style of the existing `TmuxPath()`/`SessionName()` coverage, worth having only because it pins the accessor as part of the exported surface.

**Tier 1, untagged, in `internal/reedengine` and `internal/reedcli`.** The extracted watchdog seam keeps `up`/`attach`/`resume` behaviour identical: assert the `suppress` and empty-hub-path early returns still no-op, driven through the exported `reedengine` function directly.
`internal/reedcli`'s existing `spawnwatchdog_test.go` coverage moves or narrows to match — whatever survives there should assert the method still forwards `c.suppressWatchdogSpawn`, `c.hubPath` and `c.eng.TmuxPath()`, not re-test the body.
Separately, assert **both** `loomcli` constructors set `suppressWatchdogSpawn` — `Command()` and `StartAliasCommand()`. This is the one test that would have caught the alias gotcha, and it needs no process at all.

**Smoke-tagged, real tmux.** `internal/loomcli` already carries `smoke_attachprobe_test.go`, which brings up a real reed engine and inspects `Status().Strands` — that is the right home.

- After a `loom start` bootstrap against a real session, the strand table contains a strand under the pinned operator name, and it is `Live`.
- A second `loom start` in the same worktree leaves exactly one such strand (the `IfAbsent` no-op path). This is the regression the status strand's history says will actually happen if the mechanism is wrong.
- A worktree whose reed server was killed and re-booted — the dead-entry case `resolveStatusStrandAction`'s doc comment describes — gets the operator strand relaunched, not duplicated and not permanently lost.
- `--no-attach` adds no operator strand, but still spawns the watchdog — the two are gated differently on purpose, so one test asserting both halves pins the distinction.
- Sanity: the operator strand is not the Selvage pane and does not become it; `ReedState.SelvagePaneID` is unchanged across the bootstrap.

**Scenarios that must be covered by something, wherever it lands best.** A failing `AddStrand` reports on the JSON envelope and does not attach (the CLI/Cobra Invariant's line — the failure must be legible, not swallowed into a handover). And the layout the attach chains accounts for the new pane, i.e. the add precedes `term.GetSize`/`AttachArgv`; `attachgeometry_integration_test.go` is the existing precedent for driving a real attach through a PTY if this is worth pinning mechanically.

**Not worth testing.** That tmux actually focused the pane — that is reed's `Display.Focus` contract, already covered in `reedengine`, and re-asserting it here tests tmux rather than this change.

## Q&A log

- **Q:** What concretely "becomes a Strand" — the attach client, the detached driver, an existing pane, or a new pane? **A:** [auto-pick] A new operator-owned pane, added before the attach. **Why:** an attach client is not representable as a Strand and the driver is deliberately paneless; the add-then-attach shape is what the VS Code `folderOpen` chain already does, and a tracked pane for the operator is what the watchdog-reap and mailbox motivations both need. Flagged as the highest-uncertainty call in this document — the design doc does not name the pane, and option (a) below is the cheap alternative if this reading is wrong.
- **Q:** May the implementation adopt an already-running pane instead of splitting a new one? **A:** [auto-pick] No, never. **Why:** adoption was built, produced R4-F5 and M16, and was removed; `planPaneTarget`'s doc comment is the written record and the design doc restates it as a hard constraint.
- **Q:** What command does the operator strand run — an agent, a composed `lyx` invocation, or a plain shell? **A:** [auto-pick] Reed's own configured `Shell`, via a new `Engine.Shell()` accessor. **Why:** matches Selvage exactly, introduces no second notion of "the operator's shell", and keeps every provider name out of `loomcli`.
- **Q:** How is the repeated-`loom start` case handled — a manual keep/replace/add dance like the status strand, or `AddSpec.IfAbsent`? **A:** [auto-pick] `IfAbsent: true` with a pinned name constant. **Why:** the engine already implements no-op / relaunch-dead / add, and `loom start` is explicitly re-entrant.
- **Q:** Does `--no-attach` still get an operator strand? **A:** [auto-pick] No — the add sits inside the `mustAttach` gate. **Why:** no operator means no pane to give them, and a tracked idle shell in every CI worktree is debris the watchdog would keep alive.
- **Q:** Is the missing per-hub watchdog spawn on the `loom start` path in scope? **A:** [auto-pick] Yes, unconditionally, by moving the existing `reedcli` seam into `internal/reedengine`. **Why:** the watchdog is the first of the two Strand-only features the design names as the reason for this work; shipping the Strand without it delivers half the item. Round 2 review (NIT:scope) correctly rejected the earlier "separable, reviewer decides" framing — a conditional work inventory is not a decision, so it is settled here instead.
- **Q:** Does the watchdog spawn sit inside the `mustAttach` gate, like the operator strand? **A:** [auto-pick] No — outside it, beside the substrate step. **Why:** the daemon is per-hub and reconciles a session that exists on every invocation including `--no-attach`, where the detached driver still spawns agent strands; `reedcli`'s `up` already spawns it with no attach involved.
- **Q:** Which package owns the extracted watchdog seam, and where does its test-suppression flag come from? **A:** [auto-pick] `internal/reedengine`, as an exported package function taking hub path, tmux path and `suppress` explicitly; each caller supplies `suppress` from its own `testing.Testing()`-initialised field. **Why:** `loomcli → reedengine` is an already-shipped import edge and the CLI/Cobra Invariant's stated direction; passing the flag in keeps the "never re-exec under `go test`" guard visible at both call sites rather than importing `testing` into an engine package.
- **Q:** Is it acceptable that a permanent operator pane takes a full share of window rows away from every agent pane? **A:** [auto-pick] Yes, accepted with no mitigation. **Why:** `stackHeights` gives every non-strip pane an equal share and a parentless, childless strand is never a strip, so this is the identical situation `loom.md`'s `childless-full-height-is-acceptable` Decision already accepted for the `loom-status` strand. Deciding it differently for the pane the operator actually types in would be inconsistent.
- **Q:** Should `loom start` refuse or warn when it is not itself running inside reed? **A:** [auto-pick] No. **Why:** the design preserves running `loom start` outside reed as an escape hatch, and a new refusal in the handover tail would widen the CLI/Cobra Invariant's deliberately narrow interactive-handoff exception.
- **Q:** Is the design doc's stated blocker — reed's pane lifecycle being solid enough, pending the header-pane split — cleared? **A:** [auto-pick] Yes, proceed. **Why:** `reed: replace the header pane with a native tmux status-line, a permanent "Selvage" pane, and a detached per-hub watchdog` sits in roadmap Done. The design's second open item ("what 'repo-orchestrator' means") is not load-bearing for this change and is left undefined.
