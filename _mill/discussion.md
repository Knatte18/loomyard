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
The non-determinism reported in the finding is fully explained by the mechanism analysis below — it is not a timing race in reed, it is one noise source (the stencil-seed WARNs) whose firing depends on build channel and on how far the board's stencils have drifted from the shipped defaults and the worktree source.

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
   One qualification, stated rather than glossed: removing the *interactive* shell removes the interactive/login RC paths (`~/.bashrc`, `~/.bash_profile`), which is where an `env`-sourcing line of this shape normally lives.
   It does **not** categorically remove every RC path — a non-interactive `bash -c` still sources whatever `$BASH_ENV` names.
   The captured lines are consistent with either, and this worktree cannot inspect the operator's environment to settle which.
   So the honest claim is: the interactive/login RC route is closed by construction, and a residual `BASH_ENV`-style write is covered by the `ED 3` backstop rather than by the shell change.

3. **Stencil-seed WARN lines on the keepalive's own stderr** — the five observed lines are one of **two** distinct sources, and the fix must account for both.
   All of them are emitted by the header keepalive process itself, before it prints anything, because `cmd/lyx/main.go`'s root `PersistentPreRunE` calls `seedStencils` for *every* command (`cmd/lyx/stencilseed.go:29`), including `reed header --blocking`.
   `internal/logger`'s default stderr threshold is Warn, so anything at Warn reaches the pane regardless of `-v`.
   `stencilstore.Reconcile` has two independent Warn emitters:
   - **The dev-refusal warn** (`internal/stencilstore/reconcile.go:106`) — one line per stencil that is `StateUntouched` with a body hash differing from the shipped default, **only** when `mode == ModeDev`.
     This is the one whose text the finding captured verbatim ("dev build does not refresh an untouched stencil ... producers will read the OLDER on-disk copy").
   - **The port-back drift warn** (`warnPortBackDrift`, called at `internal/stencilstore/reconcile.go:77-79`, body at 170-190) — one line per stencil whose board copy differs from the worktree's `contracts/stencils` copy, emitted whenever `sourceDir` is non-empty, in **either** mode.
     Its text is different ("board copy has drifted from worktree source; see lyx stencil promote"), so the two are distinguishable in a capture, but both land on the same stderr and both reach the same pane.
   Corrected non-determinism explanation: the dev-refusal warn depends on build channel *and* stencil staleness, so it comes and goes exactly as the finding describes.
   The drift warn does **not** depend on build channel at all — it fires from a production binary too, whenever the process runs inside a worktree that carries `contracts/stencils` and the board copy has drifted.
   So the earlier claim that the noise "stops firing ... [if] the binary is a production build" was wrong;
   what is true is that neither warn fires when the board, the worktree source, and the shipped defaults all agree.
   That is why the fix is "the header declines the seed pass", not "keep the stencils fresh" — freshness is not something a display pane can depend on.
   It is still not a timing window.

A separate exposure — independently discovered by reading the same pre-run code path — is that the header keepalive can reach `fabricengine.CommitSeededStencils` (`cmd/lyx/stencilseed.go:105`) at all and perform a **git commit in the hub** from a tmux pane process.
It is not the same event as the **dev-refusal** warn, and never co-occurs with it: `reconcileOne`'s `StateUntouched` dev-mode branch logs that warn and returns `wrote = false` (`internal/stencilstore/reconcile.go:100-108`), so a warned-about stencil is never added to `written`, and `seedStencilsAt` returns early when `written` is empty (`cmd/lyx/stencilseed.go:101-103`).
A commit fires only from the other classifications — a `StateAbsent` seed, a `StateReconciled` restamp, a `.gitattributes` seed, or a production-mode `StateUntouched` refresh — none of which produce the dev-refusal warn.
The **drift** warn carries no such exclusivity: `warnPortBackDrift` runs after the write loop and is independent of what was written, so a drift warn and a commit can occur in the same pass.
The commit exposure is undesirable on its own terms, in its own scenarios, and is a second reason to keep the header out of the seed pass;
it is not evidence for the noise, and this distinction must not be blurred when `internal/reedengine/doc.go` is updated.

Note also that `headerCmd`'s `--blocking` path already prints `\x1b[2J\x1b[H` before the text (`internal/reedcli/header.go:70`).
`ED 2` clears the visible screen only;
it does not touch scrollback, which is precisely why the noise survives where the operator eventually sees it.

## Scope

**In:**

- `internal/reedengine`: replace the header pane's split-then-`send-keys` boot with a `split-window` that carries the launch command as its own shell-command argument, so no interactive shell ever exists in that pane.
- `cmd/lyx`: give the root pre-run a way to skip the stencil-seed pass for commands that opt out, and opt `reed header` out.
- `internal/reedcli`: extend the `--blocking` clear sequence to clear scrollback as well as the visible screen, as a backstop against any future stray write, and extract the written bytes into a pure helper so that sequence is assertable without entering the blocking path.
- Tests: hermetic unit coverage for the new launch composition, the seed-skip gate, and the payload helper, plus smoke coverage that pins the seed-skip effect directly (real dev-stamped binary, stderr assertion) and one composite scrollback backstop against a real reed session.
- Docs: `internal/reedengine/doc.go`'s header-pane section, and `internal/reedcli/header.go`'s `Long`/file header where they describe the boot mechanics.
- `CONSTRAINTS.md`: amend the Stencil Ownership Invariant's seed/refresh bullet to record the annotation opt-out (exact substance decided under Constraints below).

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
  One precision, recorded because the cited precedent is not an exact parallel: `new-session` is handed `e.cfg.Shell` *as* the command, whereas a `split-window` shell-command argument is interpreted by tmux's own `default-shell`, which reed does not set and which need not equal `cfg.Shell`.
  The composed line still comes from `shell.ForGOOS()`.
  That mismatch is accepted and explicitly **out of scope**: it is unchanged from today, because the line typed by `send-keys` already lands in a pane running tmux's default shell rather than `cfg.Shell`.
  This task neither introduces nor worsens the gap;
  making `cfg.Shell` actually govern pane interpretation is a separate change.
  tmux runs a single-argument shell-command through a **non-interactive** shell, which reads no interactive or login RC files — so noise class 2's observed shape is closed even though a shell is technically still in the chain (with the `BASH_ENV` qualification recorded in Mechanism §2).
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
- Both header modes opt out, deliberately.
  A cobra annotation is per-command, not per-flag, so plain `lyx reed header` — the ordinary JSON-envelope preview verb — declines the seed pass too, not just `--blocking`.
  That is accepted rather than worked around: the two modes render the identical `HeaderText` output and neither reads a stencil, so neither has any reason to seed one.
  The preview verb is also the command an operator runs repeatedly while editing `header.template`, and making a preview command silently commit to the hub's git history is exactly the surprise this decision removes.
  Anyone who wants the seed pass runs any other lyx command, which is every other command there is.
  A per-flag gate was considered and rejected as strictly worse: cobra annotations are the declarative mechanism, a flag-conditional gate would have to live in imperative code inside the root pre-run, and there is no scenario in which the preview mode wants the pass while the keepalive does not.
- **Only the header opts out.**
  Strand panes also host lyx processes running through the same root pre-run, and they are deliberately **not** annotated.
  A strand pane runs real work — producers that read stencils — so the seed pass is doing its job there, its warnings are information the operator wants, and a seeded/committed stencil is the intended outcome rather than a surprise.
  The header is singled out because it is the one pane whose entire purpose is to display a fixed line, reads no stencils, and is expected to stay silent forever.
  The generic phrasing "a git commit from a display pane" means exactly that: display pane, not every pane.
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
- Consequence that must be planned around: `ED 3` runs **after** anything the shell or the pre-run wrote, so once it is in place the pane's scrollback comes up clean whether or not the source fixes exist.
  It therefore masks the source fixes from any end-to-end scrollback assertion.
  That is not a reason to drop it — it is exactly the property that makes it a backstop — but it means the scrollback assertion cannot be the regression pin for the source fixes.
  See `verification-per-fix-not-per-symptom` for how each fix is pinned independently, and `ordering-lands-source-fixes-before-the-backstop` for the landing order that keeps the pre-fix failure demonstrable.
- Rejected: issuing `tmux clear-history -t <pane>` from reed after launching — it introduces a new tmux verb into reed's surface (`internal/reedengine/probe.go`'s verb list and `doc.go`'s enumerated command set both track this), it races the keepalive's own first write, and it would have to be re-issued on every heal.
  Also rejected: relying on `ED 3` *alone* and leaving the source noise in place — the task's own framing is that this pane should never accumulate unrelated output in the first place.

### blocking-payload-extracted-to-a-pure-helper

- Decision: extract the `--blocking` mode's written bytes into a pure, unexported helper in `internal/reedcli` — a function taking the rendered header text and returning the exact string `RunE` prints (clear sequence + trimmed text).
  `RunE` becomes `fmt.Fprint(out, headerBlockingPayload(text))` followed by `blockForever()`;
  the untagged test asserts on the helper's return value, never on `RunE`.
- Rationale: the `--blocking` path calls `blockForever()` immediately after its single `fmt.Fprint` (`internal/reedcli/header.go:64-71`), and `internal/reedcli/header_test.go` records in its own file header that it "never invokes the `--blocking` path, since that path blocks forever".
  Any untagged test that drives `RunCLI` with `--blocking` hangs the suite forever;
  the writer seam does not help, because the process never returns to the test.
  A pure helper is the same shape `internal/reedengine/headerpane.go` already uses for exactly this reason (composition split from the blocking/side-effecting call site so the composition stays host-testable), so it is the established idiom in this area rather than a new pattern.
- Rejected: asserting the clear sequence only through the smoke path — it would leave the byte sequence untested in the fast suite and make a one-character escape-sequence typo a smoke-only failure.
  Also rejected: running `RunCLI --blocking` in a goroutine with a timeout — a deliberately leaked goroutine per test run, and a timing-dependent assertion for something that is pure string composition.

### dead-header-detection-improves-deliberately

- Decision: accept — and document — that the header pane now becomes `pane_dead=1` when the keepalive process exits, where previously the surviving interactive shell kept the pane alive.
- Rationale: this is a latent-bug fix falling out of the main change, not a regression.
  `ensureHeaderPaneLocked` decides header health by pane aliveness (`aliveIDSet(live)[st.HeaderPaneID]`, `internal/reedengine/lifecycle.go:463`), and `doc.go:48` documents the intended behaviour as "a header whose keepalive process dies (pane_dead=1) is deliberately kept as an enumerable corpse ... and healed on the next Up/Resume".
  Under the current shell + `send-keys` design that never actually happens: `bash` returns to its prompt and the pane stays alive, so a dead header is silently mistaken for a working one and never healed.
  With the command as the pane's own process, `remain-on-exit on` (set globally at boot) corpses the pane exactly as the documented design intends, and the existing heal path starts working as written.
- Rejected: preserving the old masking behaviour by wrapping the command in a lingering shell — it would deliberately re-break the documented heal path.

### under-go-test-behaviour-unchanged-but-overridable

- Decision: the **default** under `go test` is unchanged — the header pane is still created by a bare `split-window` with no command argument, a bare shell, exactly as today.
  What changes is that the suppression stops being a hard-wired `testing.Testing()` call at the boot site and becomes engine state a test in the same package can override: the `Engine` carries the suppression decision (an unexported field, or an unexported launch-line composer func), initialised from `testing.Testing()` at construction and settable by an in-package test helper.
  No exported API changes;
  nothing outside `internal/reedengine` can reach it.
- Rationale: two requirements collide, and only a seam satisfies both.
  The suppression must stay on by default, because re-exec'ing `os.Executable()` from a test binary would recursively run the whole suite (`internal/reedengine/lifecycle.go:517-522`) — that reasoning is untouched.
  But P1 has to observe the **real** launch path, and today `lifecycle.go:515` calls `headerLaunchLine(shell.ForGOOS(), exe, testing.Testing())` with no injection point, so under `go test` `launchCmd` is always `""` and the boot site takes the branch that issues neither the command argument nor any `send-keys` — an untagged test can therefore neither see the new behaviour nor fail on the old one.
  Overriding the flag makes the non-suppressed path drivable while the fake tmux records argv and nothing is ever executed: no process spawn, so the Test Tier Purity Invariant is untouched.
  Every existing untagged `reedengine`/`reedcli` test that asserts header-pane geometry, id recording, or up/resume idempotence continues to see the same bare-shell pane, because they do not set the override.
- Rejected: leaving `testing.Testing()` hard-wired and moving P1 to the smoke tier — it puts the one assertion that pins the core change into the slowest tier, and a fake-tmux argv assertion is exactly the kind of thing the hermetic tier exists for.
  Also rejected: an exported setter or a package-level global — an unexported field on the engine (or a composer func supplied at construction, matching the "told, never derived" style the package already follows) keeps the seam invisible outside the package.

### verification-per-fix-not-per-symptom

- Decision: each source fix gets its own regression pin that observes **that fix's own mechanism**, and the end-to-end scrollback assertion is explicitly demoted to a backstop check that pins no individual change.
  Three pins, one backstop:
  - **P1 — the pane runs the command, not a shell** (noise classes 1 and 2).
    Untagged, hermetic, `internal/reedengine`, **driven with the launch-suppression override off** (see `under-go-test-behaviour-unchanged-but-overridable` — without that seam this test is impossible, since the suppressed path emits neither a command argument nor any `send-keys`): the recorded fake-tmux argv shows `split-window` carrying the launch command, and **zero** `send-keys` calls are issued against the header pane.
    The argv + zero-`send-keys` assertion **is** P1's pin;
    nothing else is required for it.
    Its red condition needs stating precisely, because the seam and the fix are separate changes: on unmodified `main` the test cannot even be compiled, since the override does not exist.
    The demonstrable pre-fix state is therefore the intermediate one — **seam landed, launch change not yet applied** — where the same test drives the real path and fails on both halves (no command on the split, two `send-keys` calls recorded).
    That intermediate observation is the evidence of record for P1;
    "red on unmodified `main`" is not achievable for this pin and must not be claimed.
    A `#{pane_current_command}` assertion was considered as a smoke-tier reinforcement and is **not** adopted as a pin: whether the pane's process ends up being lyx or the interpreting shell depends on that shell's last-command `exec` optimisation, which is shell-dependent — the same fact this discussion cites when rejecting an explicit `exec` prefix.
    If a plan wants it at all, it belongs as a best-effort observation that is allowed to see either value, never as an assertion that the value is the lyx binary.
  - **P2 — the header declines the stencil-seed pass** (noise class 3).
    Its own test, and deliberately **not** routed through tmux: run the binary as `lyx reed header` (non-blocking) against a hub arranged so the seed pass warns, capture the process's **stderr**, and assert it is empty.
    The pre-fix expectation is stated as "stderr is non-empty", not as a specific line count or a specific message: either WARN source is sufficient to fail it, and the test must not be written so that it only detects the dev-refusal one.
    Arranging **both** warns is the preferred shape, and it takes one extra fixture step that must be stated rather than assumed: a `hubforge` fixture worktree is a synthetic template (README, `backend/`, `nested/`, `wts/some-task/`) with **no `contracts/` directory at all**, and `seedStencilsAt` sets `sourceDir = ""` when `<worktree>/contracts/stencils` is absent (`cmd/lyx/stencilseed.go:87-92`), so `warnPortBackDrift` cannot fire in an untouched fixture.
    The test must therefore **materialise a `contracts/stencils` tree inside the fixture worktree** — writing the registry's shipped defaults there with at least one body deliberately altered — before running the binary.
    With that in place, dev-stamping the build trips the dev-refusal warn and the planted drift trips the port-back warn, in the same pass.
    If materialising that tree proves awkward in practice, dev-refusal-only is an acceptable fallback: it alone makes stderr non-empty pre-fix, which is all the assertion requires.
    What is **not** acceptable is silently ending up dev-refusal-only while the discussion claims both — hence this paragraph.
    Post-fix, stderr is silent because the pass does not run at all, which is the property being pinned;
    the test asserts that, not the absence of any particular sentence.
    No pane, no scrollback, no escape sequences — so `ED 3` is structurally incapable of masking it.
    Pair it with the untagged `cmd/lyx` gate tests (annotation present on the command;
    predicate returns skip) so the wiring is pinned in the fast suite and the observable effect is pinned in the slow one.
  - **P3 — the clear sequence itself.**
    Untagged assertion on the pure payload helper's return value (see `blocking-payload-extracted-to-a-pure-helper`).
  - **B — the backstop check.**
    The `capture-pane -p -S - -t <headerPaneID>` scrollback assertion stays, as the one test that answers the operator-facing question "is the pane actually clean end to end".
    It is documented in its own file header as pinning the *composite* outcome and none of the individual fixes, precisely because `ED 3` runs last and would keep it green if a source fix regressed.
- Rationale: the round-2 review is right that a single end-to-end scrollback assertion, in the presence of an `ED 3` that runs after everything else, is a test that cannot fail for the reasons this task cares about.
  Splitting the verification by mechanism rather than by symptom gives each change a test that goes red when *that* change is reverted, which is the property "verification of record" was supposed to mean.
  The bug's reported non-determinism is still removed the same way — dev-stamped build plus deliberately stale stencils — it is just applied to P2, where it belongs, instead of to a composite assertion.
- Evidence that counts as a pre-fix failure, stated so it cannot be quietly skipped, and stated separately for the two pins because their baselines differ:
  P2 must be observed **red on unmodified `main`** and green once the seed-skip lands.
  P1 must be observed red on the **seam-landed, launch-change-not-yet-applied** intermediate state and green once the launch change lands — it cannot run on unmodified `main` at all, because the override it needs does not exist there.
  A green full run at the end of the task is not that evidence, and neither is B going green.
- Rejected: unit tests only — they cannot observe a real pane or a real process's stderr, and P2's whole value is that it runs the real binary.
  Also rejected: keeping the composite scrollback test as the sole verification (the round-2 BLOCKING finding).
  Also rejected: dropping `ED 3` so the composite test regains its diagnostic power — that trades a real runtime guarantee for test convenience, which is backwards.

### ordering-lands-source-fixes-before-the-backstop

- Decision: the source fixes and their pins (P1, P2) land **before** the `ED 3` backstop and B in the batch order, and B is written only once `ED 3` exists.
- Rationale: this keeps the pre-fix demonstration honest and cheap.
  While `ED 3` is not yet in the tree, the composite scrollback state is still observable, so the task can record the actual pre-fix pane content once, at the start, as the artifact matching the original finding.
  Landing `ED 3` first would erase that observation before it was ever made.
- Rejected: landing everything in one batch — it makes "did this specific fix work" unanswerable, which is precisely the failure mode the round-2 review caught.

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
  This is **not** a body-only edit: today's signature is `seedStencils(ctx context.Context)` and the call site is `seedStencils(cmd.Context())` (`cmd/lyx/main.go:86`), so the function never sees the `*cobra.Command` and cannot read its annotations.
  The plan must thread the command in — either `seedStencils(cmd *cobra.Command)` deriving the context itself, or an added parameter carrying the annotation map — and the choice is mill-plan's, but the signature change itself is not optional.
  Extract the predicate as its own directly-assertable function regardless, for the same reason `stencilSeedTarget` was extracted (`cmd/lyx/stencilseed.go:47-59`): `seedStencils`' `testing.Testing()` early return makes the gate unobservable through it.
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
- **Stencil Ownership Invariant — `CONSTRAINTS.md` IS edited in this task's commit.**
  Decided here rather than punted: the invariant's current bullet reads "Seed/refresh runs once per process pre-run, never lazily inside `Read`", which is unconditional, and this task makes it conditional on an annotation.
  Leaving it unamended would make `CONSTRAINTS.md` describe behaviour the code no longer has, which the Documentation Lifecycle rule forbids.
  The amended bullet reads:
  "Seed/refresh runs once per process pre-run, never lazily inside `Read`.
  A command that reads no stencils may decline the pass entirely by carrying the skip annotation;
  declining is all-or-nothing per command and never defers seeding to a later or lazier point."
  The second sentence is the whole change — the first is preserved verbatim, because the property that matters (no lazy seeding inside `Read`) is untouched.
  Exact final wording is mill-plan's to place in the file, but the substance above is decided, not open.
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
  These assertions must turn the launch-suppression override **off** — with the default on, the boot site emits neither the command nor any `send-keys`, so both halves would pass vacuously.
- Keep one test that exercises the **default** (suppressed) path and asserts the split is commandless and still records the pane id, so the `go test` behaviour this discussion deliberately preserves is itself pinned.
- The re-tile retry path (`lifecycle_test.go:371`'s one-row-top-pane recovery test) must assert the *retried* split also carries the command, likewise with the override off.
- `headerLaunchCmd`/`headerLaunchLine` coverage in `headerpane_test.go` stays as-is;
  the `underTest=true` branch must still yield `""`, and `""` must still produce a commandless split.
- Existing header geometry, corpse-heal, and idempotence tests must pass unchanged.

**`cmd/lyx` (untagged, must not spawn git).**

- TDD candidate: the seed-skip predicate.
  Assert it returns "skip" for a command carrying the annotation and "proceed" for one without it, driven directly rather than through `seedStencils`' `testing.Testing()` early return — mirroring how `stencilSeedTarget` was extracted to be assertable.
- Assert the **ordering** inside the extracted predicate's call site — that the annotation check precedes the `stencilSeedTarget` call — as a pure, in-process assertion.
  Do **not** write a test claiming to observe that "no `git rev-parse` is spawned": `seedStencils` returns unconditionally under `testing.Testing()` (`cmd/lyx/stencilseed.go:36-38`), so such a test passes whatever the ordering is, and is the same unfalsifiable shape the earlier review rounds rejected elsewhere.
  The package's existing Test Tier Purity guard is what actually catches an accidental spawn.
- A registration-level test that `reed header` actually carries the annotation — the gate is worthless if the annotation is silently dropped in a later refactor.
- `helptree_test.go` and the other existing `cmd/lyx` guards must pass unchanged.

**`internal/reedcli` (untagged) — P3.**

- TDD candidate: assert the pure payload helper's return value is exactly the `ED 2` + `ED 3` + cursor-home sequence followed by the trimmed header text (see `blocking-payload-extracted-to-a-pure-helper`).
  Assert on the **helper**, never by driving `RunCLI` with `--blocking`: that path calls `blockForever()` right after its single `fmt.Fprint`, so a test that reaches it never returns and hangs the untagged suite — `internal/reedcli/header_test.go`'s own file header already records why the existing tests avoid it.
- The non-blocking JSON-envelope path is **not** asserted in the untagged tier.
  Driving it through `RunCLI` reaches reed's `PersistentPreRunE` and therefore `lyxcwd.Resolve`, which spawns `git rev-parse` — banned here by the Test Tier Purity Invariant, and the reason `internal/reedcli/header_test.go` states in its own header that it never runs `RunE`/`PreRunE`.
  Existing `RunCLI`-driven reedcli coverage lives in `cli_integration_test.go` (`//go:build integration`);
  if the envelope path needs a regression assertion in this task, it belongs there or in the smoke tier, not untagged.
  This task changes nothing about that path's output, so adding one is optional.

**`internal/reedcli` (`//go:build smoke`) — P2 and B.**

- **P2 (pins the seed-skip fix, no tmux involved):** build a dev-stamped `lyx` (`-ldflags "-X github.com/Knatte18/loomyard/internal/buildinfo.Channel=dev"`), forge a hub, plant a stale-but-untouched board stencil so the dev-refusal WARN would fire, materialise a `contracts/stencils` tree in the fixture worktree with one drifted body so the port-back WARN would fire too (a `hubforge` fixture has no `contracts/` of its own, and without it `sourceDir` is empty and that WARN cannot fire at all), then run `lyx reed header` (non-blocking) as a real subprocess and assert its **stderr is empty**.
  Pre-fix stderr is non-empty;
  post-fix it is silent.
  Assert emptiness, never a line count or a particular message.
  This is the only test that observes noise class 3's suppression directly, and it is deliberately immune to `ED 3` because no terminal, pane, or escape sequence is in the picture.
- **B (composite backstop):** build the same dev-stamped `lyx`, forge a hub with the same stale stencil, `lyx reed up`, resolve the header pane id, poll until the header line appears, then capture the pane's **entire** scrollback with `capture-pane -p -S -` and assert it holds the header line and no other non-empty line.
  Its file header must state plainly that it pins the composite end-to-end outcome and **not** any individual fix, because `ED 3` runs after everything else and would keep this green if a source fix regressed.
- Both assertions should name what they found when they fail — a future regression must be diagnosable from the failure output alone, since the original bug was only ever caught by an operator eyeballing a pane.
- Scenarios worth covering in the same file if cheap: the header survives a `reed resume` with its scrollback still clean, and a healed header (corpse killed, fresh pane split) is equally clean.
  The heal path is the one most likely to regress, because it re-runs the same launch code from a different entry point.
- Do **not** add a smoke assertion that `#{pane_current_command}` for the header pane is the lyx binary — the value is shell-dependent (last-command `exec` optimisation).
  P1 is pinned by the untagged argv + zero-`send-keys` assertions alone.

**Evidence discipline (applies to the whole task).**

- P2 must be observed **red on unmodified `main`**, green after the seed-skip fix.
- P1 must be observed red on the **seam-landed, launch-change-not-yet-applied** intermediate state, green after the launch change — never claimed as red on unmodified `main`, which is impossible for it.
  Record that observation;
  it is the pre-fix failure evidence this task is accountable for.
- Run the `smoke` tag explicitly at the end, in addition to the untagged suite.
  A green untagged run is not sufficient evidence for this task, and neither is B going green — B cannot fail for the reasons this task cares about once `ED 3` exists.

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
- **Q:** [round-2 gap] `ED 3` runs after everything else, so an end-to-end scrollback assertion stays green even if the source fixes regress — how is each fix pinned? **A:** [auto-pick] Split verification by mechanism: P1 pins the pane-command change on the recorded tmux argv plus a zero-`send-keys` assertion, P2 pins the seed-skip on the real binary's stderr with no tmux in the picture, P3 pins the escape sequence on a pure helper, and the scrollback test is demoted to a composite backstop that pins nothing individually. **Why:** a test that cannot go red for the reasons the task cares about is not verification;
  observing each fix's own mechanism restores that property without giving up the runtime guarantee `ED 3` provides.
- **Q:** [round-2 gap] The proposed untagged assertion on `--blocking` output would hang the suite, since that path blocks forever. Which seam makes the clear sequence assertable? **A:** [auto-pick] Extract the written bytes into a pure helper in `internal/reedcli` and assert on the helper;
  `RunE` prints its result and then blocks as before. **Why:** it is the same composition-split-from-side-effecting-call-site idiom `internal/reedengine/headerpane.go` already uses for exactly this reason, and it keeps the byte sequence covered in the fast suite instead of smoke-only.
- **Q:** [round-2] Is `CONSTRAINTS.md`'s Stencil Ownership Invariant amended in this task's commit, or left to a later judgement? **A:** [auto-pick] Amended in the same commit, with the existing sentence preserved verbatim and one sentence added for the all-or-nothing per-command opt-out. **Why:** the invariant currently reads unconditionally and this task makes it conditional;
  the Documentation Lifecycle rule does not allow that gap to persist, and punting the decision to an unnamed reviewer is not a decision.
- **Q:** [round-3 gap] `stencilstore.Reconcile` has a second Warn emitter (`warnPortBackDrift`) that fires in either build mode — does that change the fix or only the explanation? **A:** [auto-pick] Only the explanation and P2's expectation;
  the fix (decline the pass) already covers both emitters. **Why:** the header declines the whole pass, so every Warn the pass can emit is suppressed by construction — but the discussion had to stop claiming a production build is quiet, and P2 must assert "stderr non-empty pre-fix" rather than counting dev-refusal lines.
- **Q:** [round-4 gap] P1 cannot observe the fix untagged, because `testing.Testing()` suppresses the whole launch path with no injection point — add a seam, or move the pin to a slower tier? **A:** [auto-pick] Add an in-package override seam on the engine (suppression state initialised from `testing.Testing()`, settable by an in-package test helper) and keep P1 hermetic. **Why:** the fake-tmux argv assertion is exactly what the hermetic tier is for, the default behaviour every existing test depends on is unchanged, and nothing is exported;
  the cost is that P1's red baseline becomes the seam-landed intermediate state rather than unmodified `main`, which is now stated explicitly.
- **Q:** [round-4 gap] A `hubforge` fixture worktree has no `contracts/` at all, so `sourceDir` is empty and the port-back drift WARN cannot fire — how is P2's "both WARNs" arrangement achieved? **A:** [auto-pick] The test materialises a `contracts/stencils` tree inside the fixture worktree with one drifted body, in addition to dev-stamping the build;
  dev-refusal-only is named as an acceptable fallback. **Why:** the assertion only needs stderr to be non-empty pre-fix, so either arrangement is sound — what was unacceptable was claiming both while the fixture could only ever produce one.
