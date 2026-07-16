# mux — independent review + fix

You are a senior engineer doing a COMPLETE, adversarial, INDEPENDENT review of the `mux`
module in the loomyard repo, followed by FIXING what you find. Work in whichever worktree
this task was spawned into — check `git branch --show-current` and `_mill/status.md` in
your own worktree rather than assuming a fixed path; the module has moved worktrees/branches
several times as it evolved (most recently `mux-anchor-top-redesign`, done) and will again.

## Your two jobs, in order
1. REVIEW: form your own independent judgment of mux's scope and correctness. Hunt for bugs by
   reading the code AND by driving real tmux (native tmux on Linux/macOS, psmux on Windows —
   whichever this machine runs; this is where mux's defects hide). **Write your findings to
   `.scratch/mux-review-<yourtag>.md` — completely, on disk — BEFORE you touch any production
   or test file.** This is `CONSTRAINTS.md`'s Review Round Invariant (A-before-B), not a
   stylistic preference: if you find a bug mid-live-driving, WRITE IT DOWN as a finding and
   KEEP DRIVING — do not stop to fix it in the moment, even if the fix is obvious and one line.
   The review file existing on disk, with every finding you intend to act on already in it, is
   what makes Job B a response to an independent judgment instead of a post-hoc rationalization
   of edits you already made. A round that discovers a bug live and fixes it immediately, only
   writing the review report afterward, HAS VIOLATED this invariant even if the fix is correct
   and even if the finding was genuinely self-discovered — the written record is what proves
   the independence, not your memory of how it happened.
2. FIX: only after the review file above exists on disk with your complete findings list,
   implement the fixes, verify each against real tmux, keep the whole test suite green, and
   update the docs in the same change. Do NOT commit or push unless the user explicitly tells
   you to — leave the changes in the working tree and report them.

## Clean-room review constraint (do this part unprimed)
Form your OWN findings first. Do NOT read any prior review or review-dialogue files before you
have your own list. Specifically do not open anything under `.scratch/` (it is gitignored and
holds prior reviews: `mux-review-*.md`, `internalmux_*.md`, `orch_*.md`, `*review*.md`) or any
`_mill/reviews/` content. Reading the design SPEC and the module docs is expected and required
(those are not reviews). After you have formed and written your own independent findings, you
MAY consult the prior rounds' review material under `.scratch/` — ANY `mux-review-*.md` and its
`*-fixer-report.md`, regardless of which model produced it (rounds rotate across Opus / Fable /
Sonnet, so do NOT assume a single model's filename prefix — the most recent prior round is
whichever `mux-review-*` file is newest), EXCEPT your own `-<yourtag>` deliverables — to
(a) confirm previously-fixed behaviors have not regressed and (b) re-evaluate the deferred items
listed at the bottom of this prompt — but only AFTER your own pass, so the independent signal is
preserved.

## What to read
- Code: `internal/muxengine/**` (incl. `render/**`), `internal/muxcli/**`, and the `cmd/lyx`
  integration (`main.go`, sandbox/help/registration guard tests). **NEW this round** — the
  header-pane surface: `lifecycle.go`'s `ensureHeaderPaneLocked` + the `ValidateHeader`
  eager-validation call sites + the `HeaderPaneID`-clear-on-rebirth path; `spawn.go`'s
  `planPaneTarget` header-exclusion-from-adoption and header-as-last-resort-split-target
  logic; `reconcile.go`'s `exemptPaneIDs` (separate from `boundPaneIDs`); `render/rules.go`'s
  header-band splicing + divider-row budgeting; `render/height.go`'s `clampHeaderHeight`;
  `render/layout.go`'s `bandHeader`; `muxcli/header.go` (the config block + `headerLaunchCmd`);
  and `internal/tokenvocab` (new leaf dependency — the token registry + `Render` that fills
  the header template; read its own leaf-invariant test too).
- Docs: the `internal/muxengine` package documentation (the design doc this prompt originally
  pointed at was deleted per the documentation lifecycle once mux landed — it now also
  carries a package-level summary of the header-pane invariant and the divider-row
  behavioral assumption, added when `docs/modules/mux.md` was deleted a second time for
  recreating exactly the doc this lifecycle rule forbids), `internal/tokenvocab`'s package
  documentation, `docs/research/mux-exploration.md`,
  `docs/research/mux-hooks-exploration.md`, `docs/overview.md`, `docs/roadmap.md`,
  `CONSTRAINTS.md`, `README.md`.
- The dedicated live-driving suite you will RUN: `tools/sandbox/SANDBOX-MUX-SUITE.md`
  (scenarios M0–M18 as of this writing — M6 was retired when `anchor:top` was removed and
  replaced by M18's below-parent mother/child shrink scenario; confirm the current max
  scenario number yourself, the suite is expected to keep growing) plus `docs/sandbox-howto.md`
  for how the sandbox harness works. This suite is the maintained, structured vehicle for
  driving real tmux — see "What to TEST".
- Repo rules you MUST follow: `CLAUDE.md` (root + `~/.claude/CLAUDE.md`) and `CONSTRAINTS.md`
  (Hub Geometry Invariant, CLI/Cobra Invariant, lyxtest Leaf Invariant, Sandbox Suite Coverage,
  Documentation Lifecycle). A change that ships behaviour without updating the module doc /
  invariants in the same change is incomplete.
- Design intent (SPEC, not a review). `_mill/discussion.md` and `_mill/plan/*` were removed from
  this branch by a pre-merge cleanup commit; recover them from git history if needed.
  Use these as the authoritative source of intended v1 scope/behavior.

## Mission (assess on two axes, be adversarial)
1. Scope / omfang — is the module's scope right? Does the as-built code deliver what the design
   intended? Gaps, over-reach, silently-dropped requirements, deferred-that-should-ship-in-v1.
2. Correctness — bugs, races, error handling, edge cases. Pay special attention to the areas
   that are historically where mux breaks (see "High-yield focus" below).
Also assess docs accuracy (do the docs match the code?) and operability (could a user actually
run this?).

## High-yield focus — where mux's real bugs live (drive these, do not just read them)
The pure/unit-tested parts (render math, checksum, name templating, parsing, the op lock) are
solid and rarely wrong. The defects concentrate in the COMPOSED, LIVE-tmux behavior that the
hermetic tests and the single-strand smoke test never exercise. Treat every one of these as an
INVARIANT you must actively verify by driving real tmux — a green `go test` proves nothing here:

- LIVENESS DEFINITION. "present in `list-panes`" must NOT be conflated with "the strand's
  process is alive". A `pane_dead=1` pane is present-but-not-alive. Verify: `status` reports a
  crashed strand as NOT live; `resume` treats a dead-pane-bound strand as needing relaunch;
  render still counts dead panes (select-layout must enumerate them).
- CRASH / SERVER REBIRTH. After `kill-server`, a reborn session reuses pane ids (initial pane is
  `%1` again). Verify a stale binding is never mistaken for a live strand: `up` after a crash
  must clear stale bindings; `resume` after a crash must rebuild every non-hidden strand exactly
  once (adopt the initial pane, split the rest — no orphans, no double-launch).
- SOLE / ALL-DEAD PANES. This bullet's ORIGINAL claim — "tmux refuses to kill a session's last
  pane" — is now KNOWN WRONG on tmux (see `mux-remove-last-pane-error`, done): killing a
  session's true last pane destroys the session outright, corpsing nothing. What still needs
  verifying: reconcile keeps exactly one pane when every pane is dead-but-present (a `pane_dead=1`
  corpse is not the same as "no panes" — a session with N dead panes and zero live ones is a
  different state than a session actually reduced to zero panes by an explicit `kill-pane`), and
  that resume still rebuilds all strands in ONE pass (no "resumed:N but only 1 came back", no
  adopting a dead pane and silently swallowing the launch). Whether real tmux on Windows still
  behaves the old "refuses to kill the last pane" way the original bullet claimed is itself
  UNVERIFIED (no Windows box was available when this was found) — if you have Windows access,
  checking that directly would resolve a real open question, not just a documentation nit.
- CROSS-WORKTREE SCOPE. The tmux server is per-HUB, shared by sibling worktrees. Verify `down`
  in one worktree does NOT kill sibling worktrees' sessions/agents; verify two worktrees on one
  hub server coexist; watch for duplicate server processes spawned during down/up churn.
- REMOVE / LAYOUT REAPING. tmux (3.3.4) silently DESTROYS any pane not present in an applied
  `select-layout` string. Verify `remove` kills the removed strand's pane deterministically
  (not by accident of layout reaping), and think about what a manual operator-split pane suffers
  when the next mux verb re-applies the layout.
- MID-OP FAILURE. A launched pane must never become an untracked orphan if a later apply/persist
  step fails (i.e. persist the record before the fragile apply).
- SEND-KEYS HYGIENE. Opaque `cmd`/`resumeCmd` strings (shuttle builds arbitrary PowerShell
  chains) must be sent literally so an embedded `;` or a key-name-like token is not reinterpreted
  by tmux.
- REPORTING HONESTY. Result counts (`resumed`, `removed`) and `status.live` must reflect reality,
  not intent.
- ENV HYGIENE. `CleanClaudeEnv` must strip `CLAUDECODE` + `CLAUDE_CODE_*` on the server spawn.
- DEBUG LOGGING. With `debug_log: 1` (or `LYX_MUX_DEBUG=1`) set on the boot that spawns the
  shared per-hub server, that boot must leave a `tmux-server-*.log` file under the hub's
  `<hub>/.lyx/logs/`; old logs there are pruned to the newest 3 total (including the fresh
  one); an invalid `debug_log` value (anything other than `0`, `1`, or `2`) fails the boot
  loud instead of being silently ignored; `debug_log: 0` (the default) adds no extra tmux
  flags and leaves the invocation exactly as before. Verify: boot with each of `0`/`1`/`2`/an
  invalid value and check both the log file and the exact server-spawn argv (`-vv`/`-v`/none).
- DEAD-SERVER HINT. With persisted strands (a mux.json with ≥1 strand) but no live session,
  every verb that shares the `requireSessionLocked` chokepoint (status, add, remove, attach's
  pre-flight) must fail with an error pointing at `lyx mux resume` to rebuild the strands —
  not just `lyx mux up`. When zero strands are persisted (or no mux.json exists), the plain
  `no mux session; run "lyx mux up"` message is correct and must NOT mention `resume`. Verify
  both branches: kill the server with strands persisted (resume-hint expected) vs. a genuinely
  fresh hub with no persisted state (bare `up` hint expected).
- REMOVE EMPTIES THE SESSION. `lyx mux remove <guid>` on a session's true last live strand
  must return SUCCESS, not the old `"list panes: exit status 1: no server running"` error —
  killing that pane legitimately destroys the session (and, since it was the server's only
  session, the server itself exits); this is expected, not a failure. Verify: (a) removing the
  sole live strand returns `ok:true` with the removed strand named in the result; (b) `mux.json`
  afterward has zero strands (the persistence-gap regression — a resurrect-on-resume bug if this
  is broken); (c) `lyx mux resume` afterward does NOT try to resurrect the removed strand; (d) a
  genuinely unexpected reconcile/apply failure with the session still ALIVE (e.g. some other
  tmux/tmux error unrelated to session death) still surfaces as an error — the swallow must be
  specific to "session confirmed gone", not blanket-swallow every remove-time failure. Also
  verify the hidden-strand edge case: removing the last VISIBLE strand while an `anchor:hidden`
  strand remains still succeeds (hidden strands never hold a pane, so the session legitimately
  dying is still expected there too).
- MOUSE MODE DEFAULT. A fresh mux server boot must explicitly pin `mouse` via
  `set-option -g mouse <on|off>` to the configured value — default `off` (preserves native
  terminal text-selection/copy), operator-configurable via `mouse: on` in mux.yaml or
  `LYX_MUX_MOUSE=on`. Verify: (a) `show-options -g mouse` reports `off` under the default
  config, `on` when configured on — for BOTH values, not just the non-default one (the boot
  must pin the option in both directions, never skip the call when the value is `off`); (b) an
  invalid `mouse` value (anything but `on`/`off`, case-insensitive; including an explicitly-empty
  value) fails the boot loud before any tmux/tmux round-trip, mirroring `debug_log`'s validation
  placement; (c) toggling `mouse` in config on an ALREADY-RUNNING hub has NO effect until the
  mux server actually restarts — a live session with live panes hits the early-return boot path
  and never re-runs `set-option`, same live-change semantics as `debug_log`/`remain-on-exit`.
  Do not conflate this with a per-strand or per-pane setting — mouse is a server-global (`-g`)
  tmux concept with no finer-grained variant; there is deliberately no CLI flag for it.
- ANCHOR:TOP IS GONE — verify nothing still assumes it exists. `anchor:top`/`TopBandRows`/
  `top_band_rows` were removed entirely (see `mux-anchor-top-redesign`, done) in favor of
  `below-parent` + `ShrinkWhenWaitingOnChild` (already the default on every `lyx mux add`),
  which places a parent above its live descendants and collapses it to `collapsed_strip_rows`
  only once it actually has one. Verify: (a) `--anchor` only accepts `below-parent`/`hidden` —
  any other value (including the old `top`) is rejected with a clear "want below-parent|hidden"
  error, not a silent fallback; (b) a config file carrying a stale `top_band_rows` key from
  before the removal does not break `lyx config reconcile`/boot (should be silently droppable
  as an unrecognized key, per the existing "preserved unless reconciled" contract — confirm this
  is actually true rather than assumed); (c) a below-parent root ("mother") with no live
  descendant renders FULL HEIGHT (not collapsed) — this is intended, not a regression, and is
  exactly the behavior `mux-anchor-top-redesign`'s M18 sandbox scenario exercises; (d) once a
  child is added under it via `--parent`, the mother collapses to `collapsed_strip_rows` and a
  PLAIN status-line command stays legible there (no box-drawing-TUI corruption risk for a
  simple status line — that corruption class was specifically what the now-removed `TopBandRows`
  override existed to work around for TUI commands sharing the old fixed `anchor:top` band; a
  full TUI command, e.g. `claude`, should still generally be the below-parent CHILD, not the
  collapsing ancestor, precisely to avoid ever forcing a TUI into a collapsed strip).

- **NEW THIS ROUND — THE ALWAYS-ON HEADER PANE.** Every session now carries one extra,
  permanent pane beyond its strands (`MuxState.HeaderPaneID`), deliberately never a `Strand`.
  This is genuinely new stateful lifecycle + layout surface — treat it with the same
  adversarial weight as the bullets above, not as a light add-on:
  - NOT-A-STRAND ACCOUNTING. Verify the header never appears in any strand-keyed output
    (`status`'s per-strand loop, `UpResult`'s strand count, the no-session error's
    strand-count pointer) and is never itself adoptable, splittable-into, or reconcile-reaped
    as an ordinary strand would be.
  - BOOT / REBIRTH IDEMPOTENCY. `ensureHeaderPaneLocked` must be a no-op on a repeated
    `up`/`resume` when `HeaderPaneID` already names a live pane; after a `kill-server` crash
    (server rebirth, pane ids reused), `HeaderPaneID` must be cleared alongside every strand
    binding and the header rebuilt exactly once — verify no double-header, no stale-id
    misdetection as still-live.
  - EAGER VALIDATION. A bad/unresolvable header template must fail the boot loud (via
    `Engine.ValidateHeader`) BEFORE the header pane is ever created — on both a first `Up`
    and a crash-recovery `Resume`. Verify both paths, not just the happy one.
  - THE WHOLE POINT: HEADER SURVIVES LAST-STRAND REMOVAL. This is the feature's core promise
    — removing a session's true last strand must no longer destroy the session (tmux) or
    corpse its sole pane (psmux); the header pane keeps the session alive. Verify this live,
    then verify a subsequent `add` still works against the header-only session (the header
    becomes the split target as a last resort — confirm it survives that split too and its
    configured height is restored on the next render).
  - THE THREE EXCLUSION SEAMS, ADVERSARIALLY. (1) Adoption: on a fresh substrate with no
    strand pane binding, the header must never be adopted as if it were an orphaned strand
    pane. (2) Split-target selection: the tallest ALIVE NON-header pane is preferred; only
    when zero non-header panes exist does the header become the target. (3) Reconcile: the
    header pane id must be excluded from `boundPaneIDs` (which also gates
    `anyBoundPresent`) but included in the separate `exemptPaneIDs` set that guards the
    untracked-pane reap loop — construct a scenario with zero strands bound, the header
    alive, and a genuine untracked/foreign pane present, and verify the foreign pane is
    reaped while the header is not, and that `anyBoundPresent` does not spuriously flip true
    off the header's mere presence.
  - HEADER BAND LAYOUT + THE DIVIDER-ROW REGRESSION. `render.Rules` reserves one divider row
    between the header band and the strand stack (mirroring the inter-strand divider budget)
    and `clampHeaderHeight` never clamps the header below 1 row — both exist because a real
    tmux `select-layout` accepts a layout omitting either and still silently overflows the
    window by one row. A regression test
    (`contract_integration_test.go`'s `TestHeaderNeverGetsZeroHeightLayoutCell`) already pins
    this, but re-verify it live with your OWN pathological window/`height_rows` ratios beyond
    what that test constructs — this is exactly the class of bug a green `go test` can miss if
    the adversarial ratio isn't the one hardcoded in the test. Also verify the ordinary case:
    a normal-sized header + several strands lays out with no visual corruption and no
    off-by-one at the window's bottom edge.
  - `-b` SPLIT DIRECTION / PHYSICAL TOP POSITION. The header's own boot split uses `-b` so it
    lands physically above its split target and every later STRAND split targets a
    non-header pane and inserts below it. Verify the header never loses its physically
    topmost position across a realistic sequence of adds/removes — `render.Rules` always
    emits the header cell first and assumes it IS topmost; if physical position and layout
    emission order ever disagree, `select-layout` positionally misassigns cells (see the
    package documentation's "Multiplexer contract surface" for why this is load-bearing).
  - INTERACTION WITH EXISTING INVARIANTS. The header pane is new state layered onto every
    existing invariant above — do not review it in isolation. In particular: combine header
    presence with CRASH/SERVER REBIRTH, CROSS-WORKTREE SCOPE (does each sibling worktree's
    session get its own header pane correctly?), and REMOVE/LAYOUT REAPING (does a header
    pane ever get destroyed as reaping collateral when a `select-layout` string is
    misconstructed?).

BOOT-WINNER SEMANTICS (review lens): the tmux server is per-hub and shared by sibling
worktrees, so `debug_log` only matters on the boot that actually spawns that shared server —
a sibling worktree's `up`/`resume` that finds the server already running does not re-apply
its own `debug_log` value. If you are testing from a sibling worktree, either arm
`LYX_MUX_DEBUG=1` machine-wide before any worktree boots, or boot from the worktree whose
config carries `debug_log: 1`/`2` — do not conclude the feature is broken just because a
non-boot-winning worktree's `debug_log` had no effect.

## Hooks are OUT of scope for mux v1
Claude Code hooks (Stop/SessionStart/PreToolUse, marker/idle detection, resume-command
construction) belong to `shuttle`, not mux. Their absence is correct — do not flag it. mux is a
dumb carrier: it runs opaque command strings and its only liveness signal is generic `pane-died`.

## Round context — R2 (Opus), after R1 (Fable, self-tagged `sonnet-r1`) closed two real defects
mux already merged into `main` long ago (the `internal-mux` build-out and its R3–R6 review
rounds referenced in old `.scratch/mux-review-*` files are historical — that work is done and
should not be re-litigated by number). The immediate context for THIS campaign: four separate,
individually-reviewed changes (`mux-server-crash`, `mux-mouse-default`,
`mux-remove-last-pane-error`, `mux-anchor-top-redesign` — see the High-yield-focus bullets above
for what each touches) landed in quick succession, each scoped and tested on its own but never
exercised TOGETHER — that is what this hardening campaign as a whole exists to close.

CLOSED-AND-VERIFIED by R1 — do not re-litigate, but DO check for regressions (commits are on
this branch, `cluster-fork-spike`):
- **F1** (MEDIUM): `tmux-client-*.log` grew unbounded under `debug_log`. tmux's `-v`/`-vv` are
  GLOBAL flags on a boot invocation that is simultaneously a client and, once forked, the server
  — so a debug-armed boot writes BOTH `tmux-server-*.log` AND `tmux-client-*.log`, but pruning
  only matched the server prefix. Fixed in `internal/muxengine/lifecycle.go` (commit `0570b620`)
  — both filename shapes now pruned independently to the same newest-3 policy. Never caught by
  the original `mux-server-crash` batch because that work was developed/reviewed against psmux
  on Windows, which does not write a client-side log.
- **F2** (LOW): `TestSmokeRemoveLastStrandThenAddRunsTheNewCommand`'s corpse-pane-adoption
  premise is psmux-specific (its own doc comment already said so) but the test was not actually
  skipped on other backends and hard-failed on native tmux. Fixed with a `runtime.GOOS` skip
  guard (commit `ec5409c2`).
- The orchestrator independently re-verified both (not just R1's self-report): build/vet green,
  hermetic `-count=5` green, serial smoke — the only remaining failures (attach's pwsh-syntax
  assumption, claude-resume's nested-session transcript issue, two hardcoded-`pwsh.exe`
  reap-tree tests, and one sibling-worktree teardown test) were confirmed PRE-EXISTING on the
  pre-R1 baseline too (orchestrator stashed R1's diff and re-ran the identical suite to confirm)
  — R1 introduced zero regressions. 3× concurrent smoke clean, zero stray tmux throughout.

UNCONFIRMED — worth a second, harder attempt this round: mid-way through a long cross-worktree
churn sequence (mother/child stack → hidden strand → a recursive remove that emptied and
re-booted the session → single-pane kill+resume → a full `kill-server` crash-resume cycle → THEN
a sibling worktree boot), R1 hit ONE observation where `lyx mux down` on worktree A returned
`{"ok":true}` while an immediate follow-up `tmux -L <socket> has-session -t <A>` still reported
the session alive, with sibling worktree B correctly still alive. R1 carefully rebuilt the exact
same sequence afterward and could NOT reproduce it; it suspects transient background system load
(R1 had its own concurrent tool invocations running at the time, one of which was forcibly killed
mid-run by the permission system) rather than a real defect, and reported it PLAUSIBLE-but-
UNCONFIRMED per the CONFIRMED/PLAUSIBLE discipline rather than filing a speculative fix. Full
narrative: `.scratch/mux-review-sonnet-r1.md`'s "Investigated, not reproduced" section (read it
only AFTER your own independent pass, per the clean-room constraint above). If you can reproduce
this — especially under genuine system load with a similarly long operation chain, not a quiet
isolated attempt — that is a real, high-value finding: `down` reporting success without actually
killing the session is exactly the class of bug the REPORTING HONESTY invariant exists to catch.
If you cannot reproduce it either after a serious attempt, that is itself useful convergence
signal — say so explicitly rather than silently dropping it a second time.

PROCESS NOTE (do not repeat this — read it, then follow "Your two jobs" above exactly): R1
formed its two findings independently in the moment — both were self-discovered via its own
live-driving, not read from a prior report — but fixed each one immediately upon discovery,
writing the review report to disk only at the very end. That is a real violation of
`CONSTRAINTS.md`'s Review Round Invariant (A-before-B), which R1 caught in itself mid-round and
disclosed honestly rather than hiding. The operator accepted R1's result as-is — the fixes are
independently verified correct, and redoing the round would only fix the paperwork ordering, not
the outcome — but "Your two jobs" above has since been tightened with an explicit,
unambiguous instruction on exactly this point. Follow it to the letter this round: write EVERY
finding to `.scratch/mux-review-opus-r2.md` before touching any production or test file, full
stop, even if the fix is one line and you spotted the bug three tool-calls ago.

Non-blocking items carried forward from the ORIGINAL `internal-mux` build-out, never revisited
since (treat as unverified, not as settled — confirm or refute rather than assuming either way):
1. mux does not stamp the strand name into the pane title/identity (`pane_title` stays the
   hostname), so an attached operator cannot visually tell strands apart. Acceptable, or a cheap
   ergonomic win (pane title = strand name)?
2. The reap probe spawns a fresh `pwsh` + full `Get-CimInstance Win32_Process` per poll (Windows
   path) — costly and self-saturating under load; a cheaper probe would speed real
   single-instance `down` too. Worth doing now, or a documented follow-up?
3. Portability lens (mux targets Linux/tmux too; tmux is meant to be a faithful tmux clone): for
   each Windows-substrate workaround, note whether it is faithful-tmux (portable) or a tmux
   divergence (upstream candidate). Flag observations; do not implement a Linux engine here.

YOUR JOB this round:
- Do a genuinely INDEPENDENT clean-room pass (form + WRITE your own findings before reading prior
  `.scratch/mux-review-*` reports — including R1's). Adversarially live-drive the real
  multiplexer (native tmux on Linux/macOS, psmux on Windows — whichever this machine runs),
  continuing the INTERACTION-hunting mandate (debug_log × mouse × the remove-fix × the
  anchor:top removal, in every combination you can construct) plus a serious attempt at
  reproducing the UNCONFIRMED anomaly above under real system load.
- If you find a REAL defect, fix it with tests + doc updates in the same change (A-before-B —
  see the PROCESS NOTE above). If you do NOT, say so explicitly and give an honest hardening
  verdict — "no new defects, R1's fixes hold, the anomaly still does not reproduce" is a valid
  and valuable outcome, and may be the safety pass that closes this campaign. Do not invent work
  to look busy.
- VERIFY with the usual discipline: build/vet; hermetic `-count=5`; the integration suite
  (`-tags integration`, real tmux); full serial smoke; live hand-rolled driving (M0–M18 as a
  checklist, current numbering) on your own freshly rebuilt binary. Report a hardening verdict
  explicitly.

## What to TEST — do not just read, EXERCISE it
Report the exact commands you ran and what you observed.

Hermetic (must stay green throughout):
- `go build ./...`
- `go vet ./internal/muxengine/... ./internal/muxcli/...`
- `go test ./internal/muxengine/... ./internal/muxcli/... ./cmd/lyx/...` (stress the
  concurrency/timing tests with `-count=5` to catch flakiness)

Smoke (real tmux, behind a build tag):
- `go test -tags smoke ./internal/muxcli/... -run Smoke -v -count=1`
- tmux (or `psmux.exe` on Windows) must be on PATH; a shell (bash on POSIX, pwsh 7 on Windows)
  resolved via PATH. On Windows: use explicit paths to resolve WindowsApps ConPTY stubs
  correctly, or ensure PATH points to the real binary. On this machine (Linux), `which tmux`
  already resolves to a real tmux 3.6 — confirm that before assuming anything is missing.

Live tmux driving (PRIMARY — this is where the bugs surface). DO NOT invoke
`sandbox-mux-suite.cmd` / `go run ./tools/sandbox ... mux-suite` or any other suite launcher —
that machinery spawns a SEPARATE, context-free interactive `claude` session as a naive
black-box tester in a materialized sandbox Hub; it is built for a human operator dogfooding the
CLI, not for you. Spawning it from inside this round would just be paying for and waiting on
another agent's opaque session instead of doing the driving yourself, and you already have full
source knowledge plus your own tool calls (see `docs/reviews/README.md`'s "Driving the real
substrate" section for the full rationale — this is a hard rule, not a style preference).
Instead:
- Read `tools/sandbox/SANDBOX-MUX-SUITE.md` (scenarios M0–M18) as your scenario CHECKLIST only —
  for ideas on what to exercise — then run every scenario yourself with direct `lyx mux <verb>`
  CLI calls (foreground, waiting for each to return) against a throwaway git-repo fixture you
  create, exactly as described in "Deeper hand-rolled driving" below. The suite's black-box rule
  ("do not read the lyx source tree") binds the agent-under-test persona that launcher would
  spawn — it does NOT apply to you; you read the source AND drive the CLI directly.
- Build the binary yourself first: `go build -o <scratch>/lyx ./cmd/lyx` (see "Deeper
  hand-rolled driving" below) — re-run this after every source change, same footgun as any
  deploy step: a stale binary gives a false PASS/FAIL.
- The suite's own scenarios already map onto the "High-yield focus" invariants: M8 (kill one
  pane → resume recreates it), M9 (kill-server → crash-resume rebuilds all), M10 (recursive
  remove), M11 (down leaves no stray tmux). Walk every one via your own direct CLI calls and
  record OK/WARN/FAIL per the suite's verdict key.
- The `attach` scenario (M7) is operator-assisted (needs a TTY in a second terminal); flag it as
  not-headlessly-verifiable, as before.

Deeper hand-rolled driving (COMPLEMENTARY, and EXPECTED — the suite is a FLOOR, not a ceiling).
Running M0–M18 is the minimum, not the whole job. You are expected to devise and run MANY MORE
adversarial tests of your own beyond the suite — invent scenarios the suite does not cover, push
edge cases, combine verbs in orders the suite never tries, and chase anything the code makes you
suspicious of. In particular drive the paths M0–M18 do not cover: two worktrees on one hub
server, a dead-but-present `pane_dead=1` pane, stale-pane-id reuse after server rebirth,
mid-op-failure orphans, send-keys hygiene with embedded `;`/key-name tokens, rapid down→up→add
churn, non-leaf remove without `--recursive`, unknown-parent and `own-window` rejection paths.
Also drive the mitigations this task shipped:
- DEBUG LOGGING: boot with `LYX_MUX_DEBUG=1` (or `debug_log: 1` in mux.yaml) armed on the
  worktree that wins the boot, then inspect the hub's `<hub>/.lyx/logs/` directory for a fresh
  `tmux-server-*.log` and confirm stale logs beyond the newest 3 are gone; repeat with `2`
  (`-vv`) and with an invalid value (boot must fail loud) and with `0`/unset (no log, no extra
  flags).
- DEAD-SERVER HINT: with strands persisted, kill the server (`kill-server`) and run each verb
  (`status`, `add`, `remove`, `attach`) to read its error — it must point at `lyx mux resume`;
  then repeat from a hub with zero persisted strands and confirm the plain `lyx mux up` hint
  (no `resume` mention).
Report the exact commands and observations for these too. Build the binary
(`go build -o <scratch>/lyx ./cmd/lyx`), create throwaway git-repo fixtures with a
`_lyx/config/mux.yaml` (copy `internal/muxengine/template.yaml`), and drive `lyx mux <verb>`
while inspecting real tmux with `tmux -L <socket> list-panes -t <session> -F "#{pane_id}
#{pane_dead} #{pane_top} #{pane_height}"` and `... display-message -p -t <session>
"#{window_layout}"`. Use isolated `-L` sockets. Walk at minimum every scenario in "High-yield
focus" above, including: two worktrees under one hub; a parent+child stack; killing a strand's
process (`send-keys -t <pane> "exit" Enter`, repeat until `pane_dead=1`); `kill-server` to
simulate a crash; `up`/`resume`/`status`/`remove`/`down` in each resulting state; and
`--anchor below-parent|hidden` plus rejection paths (`own-window`, unknown parent, non-leaf
remove without `--recursive`). Use `-vv` to trace exact tmux invocations.

TEARDOWN DISCIPLINE (critical): if you start ANY tmux server/session, tear it down
(`tmux -L <socket> kill-server`, or `lyx mux down`). At the end, confirm with `pgrep -a tmux`
(POSIX; Windows: `tasklist | findstr /i tmux`) that ZERO tmux processes remain. Leave no stray
tmux state.

Be honest about what you could NOT verify: interactive `attach` cannot be driven headlessly (no
TTY); real `claude --resume` needs a live agent. Say so explicitly.

## How to judge each finding
For each code finding give: `file:line`, a concrete failure scenario (inputs/state → wrong
behavior), severity (BLOCKING / MEDIUM / LOW / NIT), suggested fix, and CONFIRMED
(reproduced/traced) vs PLAUSIBLE (looks wrong, unverified). For scope: plan-promised vs shipped;
flag deferred-that-should-be-v1 and shipped-beyond-scope.

## Deferred items from the prior round — RE-EVALUATE these (after your own pass)
These were consciously deferred last time; decide whether any now warrants fixing:
- Untracked panes destroyed by `select-layout` reaping (mux "owns" the session window — needs a
  documented policy for operator-split panes rather than silent death).
- A rare duplicate tmux server process spawned during rapid down→up→add churn (a boot-path
  race; needs a "server-down vs session-missing" distinction to fix safely).
- tmux normalizing applied layouts (band/strip heights come back off-by-one vs the config knob
  `collapsed_strip_rows` — cosmetic; maybe a code/doc note).
- `.lyx`/config anchored at `Cwd` not `WorktreeRoot` (running from a worktree SUBDIR gives a
  misleading "not initialized" error; a consistent fix belongs at the `hubgeometry` level).
- Dead/spec-inherited surfaces — **re-verify this whole bullet, it's stale**: `TmuxCmd.windowSize`
  and `TmuxCmd.paneIDsTopToBottom` no longer exist in the codebase at all (already removed at some
  point, this list was never updated), and `MuxState.StrippedEnv` is actively populated
  (`lifecycle.go` writes to it in the env-hygiene path) — the "always serialized null" claim is
  demonstrably false today. Confirm what (if anything) in this bullet still applies before acting
  on it; do not trust it as-is.

## Fixing — after the review
- Load the code-quality guidance (`/code-quality` skill or `mill:code-quality`) before editing.
- Prefer surgical edits; match existing style and the file-level doc-comment convention.
- For every bug you fix, add or extend a test that would have caught it. In particular, if you
  find a live-only defect, add a `//go:build smoke` test that walks the failing scenario against
  real tmux (the existing `internal/muxcli/smoke_test.go` shows the pattern, incl. a skip when
  tmux is absent). A hermetic unit test for the pure planning helper is good; a smoke test for
  the composed behavior is what actually protects the recovery paths.
- EXTEND THE MUX SANDBOX SUITE when it helps. If the review surfaces a live/visual behavior that
  M0–M18 do not cover — or you find yourself repeatedly hand-driving a scenario the suite should
  own — add it to `tools/sandbox/SANDBOX-MUX-SUITE.md` as a new `M19+` scenario (match the
  existing Goal/Watch/Verdict shape; note any controlled `tmux -L <socket>` exception; keep the
  black-box ethos for the agent-under-test persona). The suite is meant to grow with mux — this
  is encouraged, not scope-creep. If you touch the suite's scenario set, keep the coverage guard
  green (`go test ./cmd/lyx/...` — `sandbox_coverage_test.go` scans `tools/sandbox/*SUITE.md`
  for the `**Covers:** mux` tag) and honor the Documentation Lifecycle / Sandbox Suite Coverage
  invariant in `CONSTRAINTS.md` in the SAME change.
- MAKE SMOKE TESTS DETERMINISTIC. Timing-sensitive tmux operations are asynchronous: `kill-server`
  returns before the socket is released, and a freshly spawned server takes a variable time to
  accept commands (longer on a loaded machine). A smoke test that assumes a CLI verb is synchronous
  will pass on your quiet machine and FAIL intermittently on a loaded orchestrator box. Wait on the
  actual state transition (poll `has-session` until the server is genuinely down / up) with a
  deadline, rather than sleeping a fixed amount or assuming completion. Verify reliability by
  running the new smoke test many times in parallel under load (e.g. several `go test -tags smoke
  -run <name> -count=3` processes at once), not just once — a single PASS is not proof of
  determinism. If a fix touches a boot/reboot poll, prefer a deadline-based loop over a
  fixed-attempt count in the production code too.
- Keep `go build`/`go vet`/`go test` green after every change. Then REBUILD your own binary
  (`go build -o <scratch>/lyx ./cmd/lyx`) and re-run the smoke + hand-rolled live scenarios to
  confirm the fix holds and nothing regressed — rebuilding FIRST is mandatory: your live driving
  exercises whatever binary you built earlier, not your edited tree, so a stale binary gives a
  false PASS/FAIL.
- Update the `internal/muxengine` package documentation (and `docs/overview.md` / `CONSTRAINTS.md`
  if invariants or the module table move) IN THE SAME change — reconcile any prose the fix makes
  stale. Do NOT add
  bugfix/hardening notes to `docs/roadmap.md` (roadmap is for planned milestones only, per
  CLAUDE.md).
- Tear down all tmux state; confirm zero tmux processes.
- Do NOT commit or push unless the user explicitly asks. Report the changed files and how you
  verified each fix.

## Deliverables
1. A structured review report (Executive summary with top risks + merge-readiness opinion;
   Scope assessment plan-vs-shipped; Code findings severity-ranked with file:line + scenario +
   fix + CONFIRMED/PLAUSIBLE; Docs & operability findings; What-was-tested with exact commands
   and observed results, including what you could NOT verify and why). Write it to
   `.scratch/mux-review-<yourtag>.md`.
2. A fixer report: what you implemented, what you deliberately deferred (with reasons), the
   exact test commands run + results, and the changed files. Write it to
   `.scratch/mux-review-<yourtag>-fixer-report.md`.
3. In your final chat message: a concise summary (executive summary + counts by severity + the
   two report paths). Do not paste the whole reports.

Begin with the clean-room review (read the SPEC + code + docs, then drive real tmux), produce
your independent findings, then implement and verify the fixes.
