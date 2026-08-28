# Discussion: reed: header pane's boot sometimes leaves shell/log noise in its scrollback

```yaml
task: "reed: header pane's boot sometimes leaves shell/log noise in its scrollback"
slug: reed-header-pane-boot-noise
status: discussing
parent: main
```

## Problem

reed's operator-console session carries exactly one permanent non-strand pane — the header — whose entire job is to display one static line (` hub: <path>`).
Its scrollback is not clean.
Captured verbatim twice during ordinary suite testing, the pane's buffer contained, in order: a literal echoed shell command (`'/home/.../lyx' 'reed' 'header' '--blocking'`), two `-bash: .../env: No such file or directory` shell-RC errors, and five `stencilstore: dev build does not refresh an untouched stencil...` WARN lines — all *before* the header line itself.

Under normal operation the pane is clamped to `height_rows: 1`, so only the last-written line survives on screen and the noise is invisible.
It stops being invisible the moment anything reveals more of the buffer: the attach-time and resize sizing bugs tracked separately, or an operator entering tmux copy-mode and scrolling up.

**Why now:** the noise was captured live during the M19/M22 sandbox-suite investigation and recurred twice without any deliberate attempt to trigger it, so it is a real and reasonably common occurrence rather than a one-off.
It is worth closing independently of the sizing bugs: a single-purpose status pane should never accumulate unrelated output in the first place, regardless of whether the pane is ever resized.
The non-determinism reported in the finding is fully explained by the mechanism analysis below — it is not a timing race in reed, it is one noise source (the stencil WARN) that only fires under a dev-stamped binary with a stale on-disk stencil.

## Mechanism — where each of the three noise classes comes from

Established by reading the code, not inferred:

1. **The echoed command line.**
   `ensureHeaderPaneLocked` (`internal/reedengine/lifecycle.go:515-533`) creates the header pane with a bare `split-window` carrying *no* shell-command argument (`splitPaneAboveLocked`, `internal/reedengine/lifecycle.go:627`), so tmux starts an ordinary interactive shell in it.
   It then *types* the launch line into that shell with `send-keys -t <pane> -l <cmd>` followed by `send-keys -t <pane> Enter`.
   An interactive shell echoes what is typed into it, so the composed line from `headerLaunchCmd` (`internal/reedengine/headerpane.go:12`) lands in the pane's buffer verbatim — by construction, on every single boot, not intermittently.

2. **The two `-bash: .../env: No such file or directory` lines.**
   Same root cause: the pane hosts a real interactive `bash`, which reads the operator's shell RC files.
   Those errors are the operator's own RC failing;
   reed's only fault is having put an RC-reading interactive shell into a pane that needs no shell at all.

3. **The five `stencilstore: dev build does not refresh an untouched stencil` WARN lines.**
   These are emitted by the header keepalive process itself, on its own stderr, before it prints anything.
   `cmd/lyx/main.go`'s root `PersistentPreRunE` calls `seedStencils` for *every* command (`cmd/lyx/stencilseed.go:29`), including `reed header --blocking`.
   `stencilstore.Reconcile` logs one `logger.Warn` per stencil that is `StateUntouched` with a body hash differing from the shipped default, and only when `mode == ModeDev` (`internal/stencilstore/reconcile.go:106`).
   `internal/logger`'s default stderr threshold is Warn, so these always reach stderr regardless of `-v`.
   This explains the reported non-determinism exactly: the WARN fires only for a `deploy-dev`-stamped binary (`buildinfo.Channel == "dev"`) whose board stencils are stale relative to the working tree, and stops firing the moment those stencils are refreshed (`lyx stencil sync`) or the binary is a production build.
   It is not a timing window.

A fourth, related side effect falls out of the same reading: because the header keepalive runs the full root pre-run, it can also reach `fabricengine.CommitSeededStencils` (`cmd/lyx/stencilseed.go:105`) and perform a **git commit in the hub** from a tmux pane process.
That is undesirable independently of the noise.

Note also that `headerCmd`'s `--blocking` path already prints `\x1b[2J\x1b[H` before the text (`internal/reedcli/header.go:70`).
`ED 2` clears the visible screen only;
it does not touch scrollback, which is precisely why the noise survives where the operator eventually sees it.

## Scope

**In:**

- `internal/reedengine`: replace the header pane's split-then-`send-keys` boot with a `split-window` that carries the launch command as its own shell-command argument, so no interactive shell ever exists in that pane.
- `cmd/lyx`: give the root pre-run a way to skip the stencil-seed pass for commands that opt out, and opt `reed header` out.
- `internal/reedcli`: extend the `--blocking` clear sequence to clear scrollback as well as the visible screen, as a backstop against any future stray write.
- Tests: hermetic unit coverage for the new launch composition and the seed-skip gate, plus one smoke test that reproduces the noise deterministically against a real reed session and a real dev-stamped `lyx` binary.
- Docs: `internal/reedengine/doc.go`'s header-pane section, and `internal/reedcli/header.go`'s `Long`/file header where they describe the boot mechanics.

**Out:**

- The header-pane sizing bugs (`reed-attach-header-height-bug`, `reed-watchdog-daemon`, the resize/attach-height gaps).
  This task makes the buffer clean;
  it does not change how many rows of it are shown.
- The `stencilstore` dev-mode refusal policy and its WARN text.
  The WARN is correct and stays exactly as it is for every other command;
  this task only stops the header keepalive from being a command that runs the seed pass at all.
- The operator's broken shell RC.
  Not reed's to fix — it stops mattering because the pane stops hosting a shell.
- `internal/logger` levels, sinks, or the durable trace file.
  No logger change of any kind.
- Strand panes.
  They keep the shell + `send-keys` launch (`launchStrandLocked`, `internal/reedengine/spawn.go:155-175`) — an agent pane genuinely needs an interactive shell.
- Windows/psmux live verification.
  The change is written to be psmux-safe (see Decisions), but this worktree cannot execute a Windows run;
  flag it, do not attempt it.

## Decisions

### header-pane-runs-its-own-command

- Decision: `ensureHeaderPaneLocked` creates the header pane by passing the composed launch line to `split-window` as its trailing shell-command argument, instead of splitting a bare shell and typing the command into it with `send-keys`.
  The `send-keys -l` + `Enter` pair at `internal/reedengine/lifecycle.go:527-533` goes away for the header only.
- Rationale: this removes noise classes 1 and 2 at the source rather than papering over them.
  With no interactive shell there is nothing to echo the command and nothing to read the operator's RC files.
  It also removes an entire class of silent failure the current design carries: `send-keys` into a pane exits 0 whether or not anything runs.
- Rejected: keeping `send-keys` and prefixing the typed line with `clear` — the echo has already landed in scrollback by the time `clear` runs, and `clear` does not clear scrollback either.
  Also rejected: keeping `send-keys` and making the launch line `exec ... 2>/dev/null` — it hides genuine errors, needs a new shell-mechanics primitive per platform, and still leaves the typed line echoed.

### single-string-shell-command-form

- Decision: pass the launch line as **one** trailing argument to `split-window`, keeping the existing `shell.Shell`-composed string from `headerLaunchCmd` unchanged (`'<exe>' 'reed' 'header' '--blocking'` on POSIX, `& '<exe>' 'reed' 'header' '--blocking'` on pwsh).
  Do not switch to a multi-argument `argv` form.
- Rationale: a single shell-command argument is the form tmux has always accepted and the form `new-session ... e.cfg.Shell` (`internal/reedengine/lifecycle.go:314`) already relies on in this codebase, so it is the shape most likely to be supported identically by psmux, the Windows tmux port reed targets.
  tmux runs a single-argument shell-command through a **non-interactive** shell, which reads no RC files — so noise class 2 is gone even though a shell is technically still in the chain.
  Keeping the `shell.Shell` composition also means `headerLaunchCmd`/`headerLaunchLine` and their existing table tests (`internal/reedengine/headerpane_test.go`) survive intact;
  only the call site changes.
- Rejected: multi-argument `argv` form (`split-window ... <exe> reed header --blocking`), which tmux 3.x execs directly with no shell at all.
  Marginally cleaner, but it discards the `shell.Shell` seam, diverges from the one command-passing form already proven against psmux in this repo, and buys nothing once the shell is non-interactive.
  Also rejected: `exec`-prefixing the command so the pane's process is `lyx` rather than the shell — `exec` has no pwsh equivalent, so it would need a new per-platform `shell.Shell` method for no behavioural gain here (the pane still dies when `lyx` dies either way).

### header-opts-out-of-the-stencil-seed-pass

- Decision: introduce a cobra-annotation opt-out consumed by `cmd/lyx`'s root `PersistentPreRunE` — a named annotation key declared once (alongside the other CLI-wide helpers in `internal/clihelp`) and checked by `seedStencils` before it does anything else.
  Annotate the `reed header` command with it.
- Rationale: kills noise class 3 at the source, and simultaneously stops a long-lived tmux pane process from ever running `fabricengine.CommitSeededStencils` against the hub — a git commit from a display pane is a side effect nobody asked for.
  The header keepalive reads no stencils (`HeaderText` renders the embedded `console-header.md` through `tokenvocab`, deliberately outside the stencil mechanism — see `internal/reedengine/headertemplate.go`), so skipping the pass costs it nothing.
  An annotation rather than a name check keeps the gate declarative and greppable, and gives any future quiet/long-lived command the same opt-out without touching `cmd/lyx` again.
  The pass still runs for `lyx reed up`, `lyx reed resume`, and every other command, so nothing about when stencils get seeded in practice changes.
- Rejected: hard-coding a `cmd.Name() == "header"` check in `seedStencils` — brittle (two commands could share a name in different subtrees) and not discoverable from the command definition.
  Also rejected: suppressing the logger's stderr sink from inside the header's `RunE` — the root `PersistentPreRunE` runs *before* any module hook or `RunE`, so the WARNs are already on stderr by then;
  it cannot work.
  Also rejected: launching the header with `LYX_LOG_FILE` set via `split-window -e` — depends on a tmux 3.0+ flag of unknown psmux support, and redirects *all* of the process's log output rather than declining work it should not be doing.

### scrollback-clearing-backstop

- Decision: `headerCmd`'s `--blocking` path prints `\x1b[2J\x1b[3J\x1b[H` instead of `\x1b[2J\x1b[H` — `ED 3` clears the scrollback buffer in addition to `ED 2`'s visible screen.
- Rationale: defence in depth.
  The three source fixes above address the three observed noise classes;
  `ED 3` guarantees the pane is clean at the moment the header renders regardless of what any future code path, shell, or terminal wrote before it.
  It costs one escape byte sequence, is emitted by the same single `Fprint` that already exists, and needs no new tmux verb.
- Rejected: issuing `tmux clear-history -t <pane>` from reed after launching — it introduces a new tmux verb into reed's surface (`internal/reedengine/probe.go`'s verb list and `doc.go`'s enumerated command set both track this), it races the keepalive's own first write, and it would have to be re-issued on every heal.
  Also rejected: relying on `ED 3` *alone* and leaving the source noise in place — the task's own framing is that this pane should never accumulate unrelated output in the first place.

### dead-header-detection-improves-deliberately

- Decision: accept — and document — that the header pane now becomes `pane_dead=1` when the keepalive process exits, where previously the surviving interactive shell kept the pane alive.
- Rationale: this is a latent-bug fix falling out of the main change, not a regression.
  `ensureHeaderPaneLocked` decides header health by pane aliveness (`aliveIDSet(live)[st.HeaderPaneID]`, `internal/reedengine/lifecycle.go:463`), and `doc.go:48` documents the intended behaviour as "a header whose keepalive process dies (pane_dead=1) is deliberately kept as an enumerable corpse ... and healed on the next Up/Resume".
  Under the current shell + `send-keys` design that never actually happens: `bash` returns to its prompt and the pane stays alive, so a dead header is silently mistaken for a working one and never healed.
  With the command as the pane's own process, `remain-on-exit on` (set globally at boot) corpses the pane exactly as the documented design intends, and the existing heal path starts working as written.
- Rejected: preserving the old masking behaviour by wrapping the command in a lingering shell — it would deliberately re-break the documented heal path.

### under-go-test-behaviour-unchanged

- Decision: `headerLaunchLine(sh, exe, testing.Testing())` keeps returning `""` under `go test`, and in that case the header pane is still created by a bare `split-window` with no command argument — a bare shell, exactly as today.
- Rationale: the existing suppression exists because re-exec'ing `os.Executable()` from a test binary would recursively run the whole suite (`internal/reedengine/lifecycle.go:517-522`).
  That reasoning is untouched by this change.
  Every untagged `reedengine`/`reedcli` test that asserts header-pane geometry, id recording, or up/resume idempotence continues to see the same bare-shell pane it sees today.
- Rejected: nothing — this is a no-change decision recorded so mill-plan does not "helpfully" unify the two paths.

### verification-is-a-real-deterministic-smoke-test

- Decision: the primary verification is a new `//go:build smoke` test in `internal/reedcli` that builds a **dev-stamped** `lyx` binary, plants a stale-but-untouched stencil in a real hub fixture so the dev WARN fires with certainty, boots a real reed session, and asserts the header pane's **full scrollback** — `capture-pane -p -S - -t <headerPaneID>` — contains the header line and nothing else.
- Rationale: the finding's own note is that the bug is non-deterministic from the outside and "verification needs several repeat cycles".
  It is only non-deterministic because noise class 3 depends on build channel and stencil staleness (see Mechanism).
  Controlling both makes it fire every time, which turns "run it a few more times and hope" into a test that fails before the fix and passes after it.
  Classes 1 and 2 are deterministic already — the echo lands on every boot, and the RC errors land whenever the operator's RC is broken (the test does not depend on the operator's RC;
  the echoed line alone is enough to fail the pre-fix assertion).
- Rejected: unit tests only — they cannot observe a tmux pane's scrollback, which is the whole defect.
  Also rejected: manual repeat cycles as the verification of record — unrepeatable, and the finding already shows the bug hiding for several cycles in a row.

## Technical context

Everything mill-plan needs, with the exploration already done:

- **Header boot site:** `internal/reedengine/lifecycle.go`, `ensureHeaderPaneLocked` (line 448) — pane-health check at 463, corpse handling 471-492, `os.Executable()` at 494, `splitHeaderPaneAtTopLocked` at 499, launch line at 515, `send-keys` pair at 527-533, `st.HeaderPaneID` persisted at 536.
- **The split helper:** `splitPaneAboveLocked` (`internal/reedengine/lifecycle.go:626`) issues `split-window -b -t <target> -c <PaneCwd> -P -F '#{pane_id}'` and runs the shared `validateSplitCreatedNewPane` guard.
  A trailing shell-command argument appends after `-F '#{pane_id}'`.
  Note this helper is also reached by `splitHeaderPaneAtTopLocked`'s one-shot re-tile retry (`internal/reedengine/lifecycle.go` around 613-636);
  the command argument must be threaded through **both** attempts, not just the first, or a retried header boots commandless.
  Whether to thread the command through `splitPaneAboveLocked` as a new parameter or to keep it header-local is mill-plan's call;
  the guard behaviour must not change either way.
- **`validateSplitCreatedNewPane`** (`internal/reedengine/spawn.go:110`) exists because psmux's `split-window` on a too-small pane exits 0 and prints an *existing* pane's id.
  Every split site must keep running it — this change must not bypass or duplicate it.
- **Launch-line composition:** `internal/reedengine/headerpane.go` (`headerLaunchCmd`, `headerLaunchLine`) — pure, host-testable, already covered by `headerpane_test.go` against both `shell.Posix()` and `shell.Pwsh()`.
  Expect these to survive unchanged.
- **Strand launch, for contrast:** `launchStrandLocked` (`internal/reedengine/spawn.go:130-175`) keeps its split-then-`send-keys` flow.
  `sendKeysLiteralArg` (`internal/reedengine/spawn.go:120`) stays — it is still used by strands and by `io.go:95`.
- **Root pre-run and the seed pass:** `cmd/lyx/main.go:78-88` (`PersistentPreRunE`, with `cobra.EnableTraverseRunHooks = true`), `cmd/lyx/stencilseed.go` (`seedStencils` at 29, `stencilSeedTarget` at 60, `seedStencilsAt` at 84).
  `seedStencils` already returns early under `testing.Testing()`;
  the annotation gate belongs alongside that early return, and must be checked **before** `stencilSeedTarget` so no `git rev-parse` is spawned for an opted-out command either.
- **The command being annotated:** `internal/reedcli/header.go`, `headerCmd()` (line 28).
  Annotations go on the `&cobra.Command{...}` literal.
- **Where a shared annotation key belongs:** `internal/clihelp` — it already owns the CLI-wide seams (`ShouldAbort`, `SetExit`, `RunRoot`, `InstallJSONHelp`, `GroupRunE`) that both `cmd/lyx` and every `*cli` package import, so a key constant there creates no new dependency edge in either direction.
- **The WARN's exact trigger:** `internal/stencilstore/reconcile.go:94-108` — `StateUntouched` (on-disk stamp matches on-disk body) **and** shipped body hash differs from on-disk body hash **and** `mode == ModeDev`.
  `mode` comes from `stencilstore.ModeFor(buildinfo.IsDev())` (`cmd/lyx/stencilseed.go:94`).
  `buildinfo.Channel` is `""` for a plain `go build`, so an unstamped test binary is **production** mode and never warns — the smoke test must build with `-ldflags "-X github.com/Knatte18/loomyard/internal/buildinfo.Channel=dev"` to reproduce.
- **Logger defaults:** `internal/logger/logger.go:148-153` — stderr sink defaults to `os.Stderr` at Warn threshold, so `logger.Warn` always reaches the pane.
  No logger change is in scope;
  this is context for why the WARNs are visible without `-v`.
- **Header render path:** `internal/reedcli/header.go:70` is the single `fmt.Fprint` carrying the clear sequence.
  `Engine.HeaderText` (`internal/reedengine/header.go:8`) renders `console-header.md` through `tokenvocab.Render`;
  the template is deliberately outside the stencil mechanism (`internal/reedengine/headertemplate.go`).
- **Smoke-test harness already available in `internal/reedcli`:** `buildLyxBinary` (`smoke_test.go:692`), `capturePane` (`smoke_test.go:652`), `pollPaneContains` (672), `paneEventuallyContains` (708).
  `smoke_lifecycle_test.go:306-345` is the closest existing pattern: build a real binary, run `lyx reed up` as a real subprocess against a real hub, then drive tmux directly.
  `buildLyxBinary` currently hard-codes its `go build` argv;
  the dev-stamped build needs either a variant helper or an ldflags parameter — mill-plan's call, but do not change the existing helper's behaviour for its current callers.
  `capturePane` passes no `-S`, so it captures the visible viewport only;
  this task needs a scrollback-inclusive capture (`-S -`), which is a new helper, not an edit to `capturePane`.
- **Hub fixtures** are built by `internal/hubforge` through `fabriccli.CloneAndWire` — never hand-assembled (hubforge Fabric-Fixture Invariant).
  The smoke test must obtain its hub that way, like its neighbours do.
- **Tmux verb surface:** `internal/reedengine/probe.go:30-45` and `internal/reedengine/doc.go:105-115` both enumerate the tmux commands reed issues.
  This change issues no new verb (it adds an argument to an existing `split-window`), but `doc.go`'s prose about the header launch mechanics is now wrong and must be updated.

## Constraints

From `CONSTRAINTS.md`, the ones this task can actually trip:

- **CLI / Cobra Invariant.**
  Every command keeps a non-empty `Short`;
  errors stay JSON via `internal/output`;
  every `RunE` checks `clihelp.ShouldAbort` first.
  `reed header --blocking` is one of the explicitly named interactive-handoff exceptions to the JSON-envelope rule and stays that way.
  The `Command()`/`RunCLI`/`RunCLIIn` seam shape must not change.
  Help-tree tests (`cmd/lyx/helptree_test.go`) must still pass — adding an annotation must not disturb help output.
- **Told-Geometry Invariant.**
  `reedengine` is a bound package: it is handed absolute paths and derives none.
  The header launch keeps using the told `e.geom.PaneCwd` and the `os.Executable()` value resolved at the boot site — no new path derivation, and no direct `internal/lyxcwd` import.
- **Stencil Ownership Invariant.**
  "Seed/refresh runs once per process pre-run, never lazily inside `Read`."
  The annotation opt-out does not violate this: it makes one command decline the pass entirely, it does not move the pass elsewhere or make it lazy.
  Record the opt-out in the invariant's wording if review judges it a change to that invariant's meaning;
  the discussion's position is that it is not, because no seeding is deferred — it simply does not happen for a command that reads no stencils.
- **Test Tier Purity Invariant.**
  Untagged `reedcli`/`reedengine` tests must not spawn processes.
  The new scrollback test is `//go:build smoke`, like every other real-substrate test in the package.
  The annotation-gate unit test in `cmd/lyx` must not cause a `git rev-parse` spawn — which is precisely why the gate is checked before `stencilSeedTarget`, and why it should be extracted as a directly-assertable predicate the way `stencilSeedTarget` itself was (`cmd/lyx/stencilseed.go:47-59` records that rationale).
- **Shell Mechanics Seam** (`internal/shell`).
  No provider or platform specifics leak outside it;
  the existing `Quote`/`Invoke` composition is reused as-is and no new method is added.
- **Documentation Lifecycle.**
  Docs land in the same commit.
  `internal/reedengine/doc.go`'s header-pane paragraphs and `internal/reedcli/header.go`'s file header and `Long` text describe the current boot mechanics and become wrong with this change.
  There is no `manifest/designs/reed.md`, so `doc.go` *is* reed's module doc.
  `docs/overview.md` needs no change (no module table or execution-stack change).
  `manifest/roadmap.md` does **not** move — this is a bugfix, not a planned-item completion.
- **Worktree isolation.**
  All work stays in this worktree;
  no pushes to `main` from here.

Discovered during discussion, not from `CONSTRAINTS.md`:

- **psmux parity is asserted, not verified.**
  This worktree is Linux;
  the Windows path cannot be executed here.
  The design deliberately picks the most conservative command-passing form for that reason (see `single-string-shell-command-form`).
  Any residual doubt is a Windows-verification follow-up, not a blocker for this task.
- **`remain-on-exit on` is set globally at boot**, which is what makes the improved dead-header detection work rather than making the pane vanish and take the session with it.

## Testing

**`internal/reedengine` (untagged, hermetic — fake tmux, no live substrate).**

- TDD candidate: the header split now issues `split-window` **with** the launch command as its trailing argument.
  Extend the existing fake-tmux-driven header tests (`lifecycle_test.go:273` `TestEnsureHeaderPaneLocked_SplitsWithPaneCwdNotAnchorPath` is the closest pattern) to assert the recorded argv carries the command, and that **no** `send-keys` call is issued for the header pane.
  The negative half is the one that pins the fix.
- The re-tile retry path (`lifecycle_test.go:371`'s one-row-top-pane recovery test) must assert the *retried* split also carries the command.
- `headerLaunchCmd`/`headerLaunchLine` coverage in `headerpane_test.go` stays as-is;
  if the call site is refactored, the `underTest=""` branch must still produce a commandless split.
- Existing header geometry, corpse-heal, and idempotence tests must pass unchanged.

**`cmd/lyx` (untagged, must not spawn git).**

- TDD candidate: the seed-skip predicate.
  Assert it returns "skip" for a command carrying the annotation and "proceed" for one without it, driven directly rather than through `seedStencils`' `testing.Testing()` early return — mirroring how `stencilSeedTarget` was extracted to be assertable.
- Assert the gate is evaluated before any geometry resolution, so an opted-out command spawns no `git rev-parse` (the Test Tier Purity guard in this package will catch a regression here, but an explicit test states the intent).
- A registration-level test that `reed header` actually carries the annotation — the gate is worthless if the annotation is silently dropped in a later refactor.
- `helptree_test.go` and the other existing `cmd/lyx` guards must pass unchanged.

**`internal/reedcli` (untagged).**

- Assert the `--blocking` output begins with the `ED 2` + `ED 3` + cursor-home sequence and then the trimmed header text.
  A plain string assertion on the written buffer;
  `RunCLI` already exposes the writer seam.
- Assert non-blocking mode still returns the JSON envelope unchanged.

**`internal/reedcli` (`//go:build smoke`, the verification of record).**

- One test: build a dev-stamped `lyx`, forge a hub, plant a stale-untouched stencil so the dev WARN is guaranteed, `lyx reed up`, resolve the header pane id, poll until the header line appears, then capture the pane's **entire** scrollback with `-S -` and assert it contains the header line and no other non-empty line.
- The assertion should name what it found when it fails — the whole point is that a future regression is diagnosable from the failure output alone, since the original bug was only ever caught by an operator eyeballing a pane.
- Scenarios worth covering in the same file if cheap: the header survives a `reed resume` with its scrollback still clean, and a healed header (corpse killed, fresh pane split) is equally clean.
  The heal path is the one most likely to regress, because it re-runs the same launch code from a different entry point.
- Run the smoke tag explicitly at the end of the task;
  a green untagged run is not sufficient evidence for this task, and neither is a single smoke run that never asserted the pre-fix failure.
  Confirm the new smoke test **fails on the pre-fix code** before accepting it as passing on the post-fix code.

## Q&A log

- **Q:** Fix the noise at its sources, or just wipe the pane's scrollback when the header renders? **A:** [auto-pick] Fix at source, with a scrollback wipe as an additional backstop. **Why:** the task's own framing is that a single-purpose pane should never accumulate unrelated output in the first place;
  a wipe alone leaves the design defect and depends on the wipe always winning the race with whatever wrote before it.
- **Q:** How should the header pane's command be handed to tmux — as a single shell-command string, or as a multi-argument argv tmux execs directly? **A:** [auto-pick] Single shell-command string, reusing the existing `shell.Shell` composition. **Why:** it is the one command-passing form already proven against psmux in this repo (`new-session ... e.cfg.Shell`), it keeps `headerLaunchCmd`/`headerLaunchLine` and their tests intact, and a non-interactive shell already removes the RC-file noise that motivated the change.
- **Q:** How should the `stencilstore` WARN lines be kept out of the pane — skip the seed pass for the header command, redirect the pane's stderr, or rely on the scrollback wipe? **A:** [auto-pick] Skip the seed pass, via a cobra annotation the root pre-run honours. **Why:** the header keepalive reads no stencils, so the pass is pure waste for it;
  skipping also stops a display pane from performing a hub git commit via `CommitSeededStencils`.
  Redirecting stderr would hide genuine errors and needs per-platform redirection syntax.
- **Q:** Name-check or annotation for the seed-skip gate? **A:** [auto-pick] Annotation, with the key declared once in `internal/clihelp`. **Why:** declarative, greppable, visible at the command definition, and reusable by any future long-lived command without editing `cmd/lyx` again.
- **Q:** Which scrollback-clearing mechanism — `ED 3` in the header's own output, or `tmux clear-history` issued by reed? **A:** [auto-pick] `ED 3` in the header's own `Fprint`. **Why:** no new tmux verb enters reed's surface, it cannot race the keepalive's own first write, and it re-applies for free on every heal and re-render.
- **Q:** The direct pane command means the pane now dies when the keepalive exits, where bash previously kept it alive. Accept, or preserve the old behaviour? **A:** [auto-pick] Accept and document as an intended improvement. **Why:** `doc.go:48` already documents corpse-and-heal as the intended design, and the surviving shell is what has been silently defeating it;
  the change makes the documented heal path start working.
- **Q:** Keep the `testing.Testing()` bare-shell header pane? **A:** [auto-pick] Keep it, unchanged. **Why:** re-exec'ing the test binary would recursively run the whole suite;
  that rationale is untouched, and every existing header test depends on the current bare-shell shape.
- **Q:** How is the fix verified, given the finding calls the bug non-deterministic? **A:** [auto-pick] A smoke test that removes the non-determinism by controlling both of its causes — a dev-stamped binary and a deliberately stale stencil — then asserts the header pane's full scrollback. **Why:** the non-determinism is fully explained (dev channel + stencil staleness), so it can be forced;
  a test that fails pre-fix and passes post-fix is worth more than any number of manual repeat cycles.
- **Q:** Is Windows/psmux behaviour verified as part of this task? **A:** [auto-pick] No — the design picks the most conservative, already-proven command form, and live Windows verification is a follow-up. **Why:** this worktree is Linux and cannot execute a Windows run;
  claiming verification that did not happen would be worse than flagging the gap.
