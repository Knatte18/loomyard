# Batch: operator-strand-and-loom-wiring

```yaml
task: "reed: born-as-strand for loom start's operator attach"
batch: "operator-strand-and-loom-wiring"
number: 2
cards: 8
verify: go test ./internal/loomcli/... ./cmd/lyx/... && go test -tags smoke ./internal/loomcli/...
depends-on: [1]
```

## Batch Scope

This batch closes the gap the task exists for: `lyx loom start`'s terminal handover adds an operator-owned Strand before attaching, and the same verb spawns the per-hub watchdog daemon its session has been running without.
The pure pieces — a pinned display-name constant and an `AddSpec` builder — go in `internal/loomcli/bootstrap.go` beside `statusStrandDisplayName` and `statusStrandCmd`, which is what keeps `start.go`'s verb body assembly over judgment that is already under test.
The watchdog call reaches batch 1's `reedengine.SpawnWatchdog` through an injected func field on `loomCLI`, because under `go test` the real call is suppressed and leaves nothing to assert against otherwise.
User-visible help and design docs are updated by the same cards that make their new wording true.

**Consumes from batch 1:** `reedengine.SpawnWatchdog(hubPath, tmuxPath string, suppress bool)`.

Batch-local decision: the operator strand's add and the watchdog spawn are gated differently on purpose, and nothing here may "tidy" them into one gate.
The strand add sits inside `start.go`'s `mustAttach` gate — no operator means no pane to give them;
the watchdog spawn sits outside it, beside the substrate step, because the daemon reconciles a session that exists on every invocation including `--no-attach`, where the detached driver still spawns agent strands.

## Cards

### Card 8: The operator strand's pinned name and `AddSpec` builder

- **Context:**
  - `internal/reedengine/strand.go`
  - `internal/reedengine/render/types.go`
- **Edits:**
  - `internal/loomcli/bootstrap.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add two things to `internal/loomcli/bootstrap.go`, beside the existing `statusStrandDisplayName` constant. First, `const operatorStrandDisplayName = "loom-operator"`. Its doc comment must state that the literal is pinned for three reasons: it is the `IfAbsent` match key, it is what an operator sees in `lyx reed status`, and it must stay byte-stable across versions or a re-run stacks a second pane instead of matching the first — reed's add has no upsert semantics, the same reason `statusStrandDisplayName` is pinned. Second, a pure builder `func operatorStrandAddSpec() reedengine.AddSpec` taking no parameters and returning `reedengine.AddSpec{NameOverride: operatorStrandDisplayName, IfAbsent: true, Display: render.Display{Anchor: render.AnchorBelowParent, Focus: false, ShrinkWhenWaitingOnChild: false}}`, leaving `Cmd` at its zero value. Write out each of the four non-default choices in the builder's doc comment. `Cmd` is empty because a Strand's `Cmd` is typed into an already-running shell via `send-keys`, not passed as a trailing `split-window` argument, so naming a shell there would nest one shell inside another and make `lyx reed resume` stack a third; the pane runs whatever shell tmux gives a freshly split pane. `IfAbsent` is true because `lyx loom start` is explicitly re-entrant and the engine already implements the needed no-op / relaunch-dead / fall-through-to-add behaviour, so this must not repeat `resolveStatusStrandAction`'s older manual keep/replace/add dance. `Focus` is false because `Display.Focus` is persisted on the strand and re-evaluated on every subsequent `AddStrand`, so a true value would re-capture focus on every agent-pane spawn for the rest of the run — the operator still lands in their own pane at cold bootstrap for free, via the bottom-most default. `ShrinkWhenWaitingOnChild` is false as a declaration of intent rather than a rendering change: the flag is inert for a parentless, childless strand, and setting it true would accidentally encode that the operator's own pane may collapse to a one-row strip. Take no command parameter and add no `Shell` accessor call — nothing in `loomcli` reads reed's configured shell. Add the `render` import if `bootstrap.go` does not already carry it.
- **Commit:** `loom: add the operator strand's pinned name and AddSpec builder`

### Card 9: Tier 1 coverage for the builder and the name

- **Context:**
  - `internal/loomcli/bootstrap.go`
  - `internal/reedengine/render/types.go`
- **Edits:**
  - `internal/loomcli/bootstrap_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add untagged tests to `internal/loomcli/bootstrap_test.go` pinning the builder's whole output shape and the name's distinctness, following that file's existing table-driven style. Assert on the value `operatorStrandAddSpec()` returns: `NameOverride` equals `operatorStrandDisplayName`, `IfAbsent` is true, `Cmd` is the empty string, `Display.Anchor` is `render.AnchorBelowParent`, `Display.Focus` is false, and `Display.ShrinkWhenWaitingOnChild` is false. Assert separately that `operatorStrandDisplayName` and `statusStrandDisplayName` differ — a one-line test guarding the exact failure `resolveStatusStrandAction`'s own doc comment describes, where a name collision appends a second pane rather than replacing the first. Write the doc comments so they say why `Focus` and `Cmd` in particular are pinned: both are values an implementer would plausibly "fix" to something else, and both would be wrong in ways nothing else in the suite would catch. These tests construct no engine and spawn nothing, so they stay untagged.
- **Commit:** `loom: pin the operator strand's AddSpec shape and name`

### Card 10: Add the operator strand before the attach

- **Context:**
  - `internal/loomcli/bootstrap.go`
  - `internal/loomcli/sharedbootstrap.go`
  - `internal/loomcli/cli.go`
- **Edits:**
  - `internal/loomcli/start.go`
  - `manifest/designs/loom.md`
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/loomcli/start.go`'s `startCmd` `RunE`, add the operator strand at the top of the step-7 block: after `_ = bootstrapLock.Release()`, after the `!mustAttach(noAttachFlag)` early return, after the existing `c.reed.Status()` pre-flight call, and before the `term.GetSize` call. Call `c.reed.AddStrand(operatorStrandAddSpec())` and, on error, report it on the envelope through `clihelp.SetExit(ctx, output.Err(out, err.Error()))` and return — matching every other pre-flight failure in this `RunE` exactly. The position is load-bearing in three ways, and the code comment must say so. It is after the bootstrap lock's release because that release point is already documented in-file as deliberate. It is inside the `mustAttach` gate because an invocation that hands no terminal over has no operator to give a pane to, and a tracked idle shell in every CI worktree is debris the watchdog would then keep alive. It is before `term.GetSize` and the `c.reed.AttachArgv(cols, rows)` call because that argv chains a `select-layout` computed for the current pane count, so adding the strand afterwards would compute a layout for a pane count about to change. The failing-add path must report on the envelope and not attach: a failed `AddStrand` is an ordinary pre-flight error, and swallowing it into the handover would widen the CLI/Cobra Invariant's deliberately narrow interactive-handoff exception. Add no new flag and no new refusal — `lyx loom start` still does not check whether it is itself running inside a reed pane. Then update the three places that enumerate this verb's steps to the operator, in this same commit. In `start.go`'s `startCmd` `Long`: step 4 becomes adding the operator's own strand and then handing the terminal to the tmux session, and the `--no-attach` sentence must say that skipping the handover skips the operator strand with it. In `manifest/designs/loom.md`'s fenced `lyx loom start:` step list, add the operator strand to step 4 alongside the attach, naming the display settings the way step 2 names the status strand's, and note that the operator pane takes a permanent full share of window rows exactly as the status strand does under that file's own `childless-full-height-is-acceptable` Decision. In `docs/overview.md`, extend the sentence enumerating the four steps so its fourth clause names the operator strand alongside the terminal handover.
- **Commit:** `loom: add the operator's own strand before start's terminal handover`

### Card 11: The injected watchdog seam and a single `loomCLI` factory

- **Context:**
  - `internal/reedengine/spawnwatchdog.go`
  - `internal/reedcli/cli.go`
  - `internal/loomcli/bootstrap.go`
- **Edits:**
  - `internal/loomcli/cli.go`
  - `internal/loomcli/start.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add two fields to the `loomCLI` struct in `internal/loomcli/cli.go`. First, `suppressWatchdogSpawn bool`, whose doc comment states it is initialised from `testing.Testing()` and exists to enforce the Live-Substrate Spawn Observability invariant's "never re-exec `os.Executable()` under `go test`" clause — without it, every test reaching the bootstrap would re-exec the test binary and run the whole suite recursively. Second, `spawnWatchdog func(hubPath, tmuxPath string, suppress bool)`, whose doc comment states it defaults to `reedengine.SpawnWatchdog` and exists as an injection point so a test can assert the call site's arguments and its position relative to the `mustAttach` gate — under `go test` the real call returns immediately and leaves no process, no log line, and nothing else to assert against. Name `awaitRunLock`'s four injected seams as the existing precedent for this shape, and this file's own doc comment principle that the verb body is assembly over judgment already under test. Do not add a package-level function variable and do not add an interface — one function, one implementation, a func field is the lighter form and matches `awaitRunLock`'s existing shape. Then add an unexported package-level factory `func newLoomCLI() *loomCLI` in `internal/loomcli/cli.go`, returning a receiver with `suppressWatchdogSpawn` set to `testing.Testing()` and `spawnWatchdog` set to `reedengine.SpawnWatchdog`, and make it the only place in production code that builds a `loomCLI` value: replace the bare `&loomCLI{}` literal in `Command()` and the second, independent bare `&loomCLI{}` literal in `StartAliasCommand()` with calls to it. The factory exists because this package has two constructors and a nil `spawnWatchdog` on the alias would panic rather than degrade, so a field set in one and forgotten in the other is the exact defect to design out rather than merely test for — the alias builds its own receiver precisely so it carries no seam functions of its own, which is also what makes it easy to miss. `internal/reedcli` already imports `testing` in production code for this same guard, so the import is precedented, not novel. This card adds no call site; card 12 adds it.
- **Commit:** `loom: add the injected watchdog seam and a single loomCLI factory`

### Card 12: Spawn the watchdog from `loom start`

- **Context:**
  - `internal/loomcli/cli.go`
  - `internal/loomcli/sharedbootstrap.go`
  - `internal/reedengine/spawnwatchdog.go`
- **Edits:**
  - `internal/loomcli/start.go`
  - `manifest/designs/loom.md`
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/loomcli/start.go`'s `startCmd` `RunE`, call `c.spawnWatchdog(c.location.HubPath, c.reed.TmuxPath(), c.suppressWatchdogSpawn)` immediately after the existing `c.ensureStatusStrand()` call returns without error, and before the run-lock probe. Call the field, never the package function directly — the field is what makes the call site assertable. Three placement facts must appear as a code comment. It is outside the `mustAttach` gate, unlike the operator strand: the daemon is per-hub and reconciles a session that exists on every invocation, `--no-attach` included, where the detached driver still spawns agent strands that need reconciling. It is at this `RunE` rather than inside `ensureStatusStrand`, because that helper lives in `internal/loomcli/sharedbootstrap.go` and `lyx loom step` calls it too, and `step` is out of this task's scope — putting the call there would silently widen that scope line. It stays inside the region where the bootstrap lock is still held, deliberately: the spawn is a `MkdirAll`, an `os.Executable()` and a detached `Start` with no `Wait`, so it is bounded and cannot extend the hold the way a wait could, while releasing the lock early to place the call outside it would mean releasing before the driver-spawn and handshake steps the lock exists to serialise. The call returns nothing and must not be error-checked or reported on the envelope: every failure path inside the seam logs and returns, and `up`, `attach` and `resume` already treat it as best-effort. Then update the same three step enumerations in this commit. In `start.go`'s `startCmd` `Long`: state that the substrate step also spawns the per-hub watchdog daemon, best-effort, and that `--no-attach` still performs it. In `manifest/designs/loom.md`'s fenced `lyx loom start:` step list: add the watchdog spawn to step 1, noting it is best-effort and per-hub. In `docs/overview.md`: extend the same four-step sentence so its second clause names the watchdog spawn alongside the session and status strand.
- **Commit:** `loom: spawn the per-hub watchdog daemon from start`

### Card 13: Tier 1 coverage for both constructors and the call site

- **Context:**
  - `internal/loomcli/cli.go`
  - `internal/loomcli/start.go`
  - `internal/loomcli/bootstrap.go`
  - `internal/burlercli/wiring_test.go`
- **Edits:**
  - `internal/loomcli/cli_test.go`
- **Creates:**
  - `internal/loomcli/start_watchdog_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Two untagged additions. In `internal/loomcli/cli_test.go`, pin the factory and its sole-use property in two parts, because neither `Command()` nor `StartAliasCommand()` exposes the receiver it constructs and neither may grow an accessor purely for a test. Part one: assert directly that `newLoomCLI()` returns a receiver with a non-nil `spawnWatchdog` field and a `suppressWatchdogSpawn` field equal to `testing.Testing()`. Part two: assert that no production file in this package builds a `loomCLI` composite literal outside that factory, by scanning this package's own non-`_test.go` `*.go` files for the `&loomCLI{` token and allowing it only in the factory's own file — the same source-scan shape `internal/burlercli/wiring_test.go` already uses for a boundary property that has no other static form. Together these are what would have caught the alias gotcha the factory now designs out: part one proves the fields are set, part two proves both constructors go through the place that sets them. Neither needs a process. In a new `internal/loomcli/start_watchdog_test.go`, substitute a recording stub into `loomCLI.spawnWatchdog` and assert two things about the call site: that it receives the hub path and the tmux path the receiver carries, and that it fires on a `--no-attach` invocation — which is what proves the call sits outside the `mustAttach` gate, the single thing a later edit is most likely to get wrong. Leave `suppressWatchdogSpawn` at its `testing.Testing()` value so the real seam is never reached; the stub replaces the call entirely. Drive the verb without spawning anything: no `exec.Command`, no `gitexec`, no `hubforge.NewHub`, and no live tmux, per the Test Tier Purity Invariant. If driving the full `RunE` cannot be done offline in this package, assert over the smallest seam that still observes the call's arguments and its gate position, and say in the file doc comment which half the smoke tier covers instead.
- **Commit:** `loom: pin the watchdog seam's constructors and call site`

### Card 14: Live-substrate coverage for the operator strand

- **Context:**
  - `internal/loomcli/smoke_test.go`
  - `internal/loomcli/smoke_bootstrapwiring_test.go`
  - `internal/loomcli/bootstrap.go`
  - `internal/reedengine/strand.go`
- **Edits:** none
- **Creates:**
  - `internal/loomcli/smoke_operatorstrand_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add a `//go:build smoke`-tagged test file in `package loomcli` covering the operator strand against a real wired hub and a real tmux session, reusing this package's existing smoke fixtures — the real built binary helper, the wired-pair fixture, the bootstrap teardown registration, the reed engine probe, the strand-count helper, and the tmux-binary skip — rather than building a second rig. Four cases. One: after a `lyx loom start` bootstrap, the strand table contains a strand under `operatorStrandDisplayName` and it is `Live`. Two: a second `lyx loom start` in the same worktree leaves exactly one such strand, which is the `IfAbsent` no-op path and the regression the status strand's own history says will otherwise happen. Three: a worktree whose reed server was killed and re-booted — the dead-entry case `resolveStatusStrandAction`'s doc comment describes — gets the operator strand relaunched, present and `Live` again, neither duplicated nor permanently lost. Four: a `lyx loom start --no-attach` bootstrap adds no strand under that name at all. Add a fifth sanity assertion to whichever case is cheapest: the operator strand is not the Selvage pane and does not become it, checked by asserting reed's persisted Selvage pane id is unchanged across the bootstrap. Every case drives the real built binary as a subprocess rather than `RunCLI` in-process, for the reason this package's smoke suite doc comment already gives: `lyx loom start` spawns its detached driver via `os.Executable()`, which an in-process call would resolve to the test binary.
- **Commit:** `loom: cover the operator strand against a real session`

### Card 15: Reap the daemon the smoke tier now spawns, and land the design docs

- **Context:**
  - `internal/loomcli/start.go`
  - `internal/reedengine/spawnwatchdog.go`
  - `internal/loomcli/smoke_operatorstrand_test.go`
- **Edits:**
  - `internal/loomcli/smoke_test.go`
  - `manifest/designs/reed-born-as-strand.md`
  - `manifest/roadmap.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Two halves, one commit. First, extend `registerBootstrapTeardown` in `internal/loomcli/smoke_test.go` so its cleanup also reaps the per-hub watchdog daemon that this package's `loom start` invocations now spawn. This is unconditional inventory, not a branch: out of process the smoke tier builds and runs a real binary, so `testing.Testing()` is false there and the spawn genuinely fires against a throwaway fixture hub the test then tears down, and the `LYX_REED_WATCHDOG` environment key does not suppress it — that key reaches the worktree-level watch goroutine, never the process start. Find the daemon the way `findDriverPIDs` already finds the detached driver: a `/proc`-native scan for a live process, matched on an argv signature specific to the watchdog verb rather than on a lone verb word, Linux-only and returning nothing on any other GOOS. Reuse `findDriverPIDs`' own structure and the same find-then-`proc.KillPID` pattern the cleanup already applies to the driver pids, rather than writing a second scanner, and say in the doc comment why a lone argv word is not a sufficient discriminator. Second, update the two design docs. Rewrite `manifest/designs/reed-born-as-strand.md` to describe what shipped rather than what was proposed: the operator-owned pane added before the attach under its pinned name, the never-adopt constraint restated as the hard constraint it is, the empty command leaving the pane's own shell, the three accepted focus outcomes, the accepted permanent full row share, and `--no-attach` skipping the strand with the handover. Resolve its Open items section: the header-pane/Selvage blocker has cleared, and the undefined "repo-orchestrator" concept is not load-bearing for this change and stays open. In `manifest/roadmap.md`, move the `reed: born-as-strand` item out of Planned into Done, rewriting its entry in that section's past-tense shipped-summary style and keeping its design-doc link, and remove the now-stale held-back clause from the Next Up entry that names this item as its blocker. Follow the numbered-list maintenance rules that file documents, and leave every relative link resolving per the Markdown Link Integrity invariant.
- **Commit:** `loom: reap the smoke tier's watchdog and land born-as-strand's docs`

## Batch Tests

`verify:` runs two chained commands.
The untagged half, `go test ./internal/loomcli/... ./cmd/lyx/...`, covers the Tier 1 files this batch edits or creates — `internal/loomcli/bootstrap_test.go` (card 9), `internal/loomcli/cli_test.go` and `internal/loomcli/start_watchdog_test.go` (card 13) — and re-runs this package's whole existing untagged suite, which is what catches a regression in `startCmd`'s assembly from cards 10 and 12.
`./cmd/lyx/...` is included because the help-tree tests there execute the real root command, so a broken `Long` or a panicking alias constructor surfaces immediately;
those tests assert supersets rather than exact help text, so the `Long` rewrites in cards 10 and 12 do not themselves break them.

The tagged half, `go test -tags smoke ./internal/loomcli/...`, is chained with `&&` rather than folded into the untagged invocation because cards 14 and 15 edit build-tagged files that would otherwise never compile, and it is a separate invocation rather than a comma-joined `-tags` value so it makes no assumption about whether this repo gives its tagged suites mutually exclusive semantics.
This half builds the real `cmd/lyx` binary and drives a real tmux server;
it skips rather than fails when tmux is absent, matching this package's existing smoke tier.

No unbounded repo-wide run is used: the hub's `pipeline.done_gate` covers cross-package regressions once, at Handoff.
