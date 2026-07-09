# perch — independent review + fix

You are a senior engineer doing a COMPLETE, adversarial, INDEPENDENT review of the `perch`
module in the loomyard repo, followed by FIXING what you find. Work in the worktree at
`C:\Code\loomyard\wts\internal-perch` (branch `internal-perch`). Adjust that path/branch if
the task lives elsewhere now. Note: PR #58 (https://github.com/Knatte18/loomyard/pull/58) is
open against `main` for this branch but not yet merged — you are hardening it further before
merge, exactly like the mux, shuttle, and burler campaigns hardened their branches before
their own merges.

## Your two jobs, in order
1. REVIEW: form your own independent judgment of perch's scope and correctness. Hunt for bugs
   by reading the code AND by driving real perch blocks — `lyx perch run` spawning real
   `burlerengine` rounds (each a real psmux pane + a real, logged-in `claude` doing an actual
   review+fix pass), plus real progress-judge/asking-triage calls (a real ephemeral Haiku via
   shuttle). This is where perch's defects hide — it is a deterministic Go loop wrapped around
   several live, asynchronous LLM calls, and its real behavior only shows when a live block
   actually runs to convergence, to a milestone gate, to STUCK, or is paused mid-flight.
2. FIX: after you have a findings list, implement the fixes ONE AT A TIME, verify each against
   the real substrate, keep the whole test suite green, and update the docs in the same change
   as the fix they document. COMMIT after each individual fix lands green (see "Commit per fix"
   below). Do NOT push unless the user explicitly tells you to.

## Commit per fix (BLOCKING — do not batch fixes into one uncommitted diff)
As soon as one finding's fix is implemented, green, and its doc update (if any) is included,
COMMIT it — on the current branch, no push — before starting the next finding. Commit message
format: `perch: fix <finding-id> — <one-line what/why>` (e.g. `perch: fix P1 — clear the pause
flag before validating profile hash on resume`), so the finding ID matches exactly what your
review report calls it. Do not commit `.scratch/` (gitignored). This exists because a round
agent's session can be killed mid-fix by something entirely outside the method's control (a
corrupted terminal, a lost connection) — round 2 of the shuttle loop hit exactly this, leaving
several real fixes sitting as one uncommitted diff with no fixer report, forcing the
orchestrator to reverse-engineer, finding by finding, which fixes were actually complete before
it could safely continue. Small per-finding commits make that recovery trivial instead.

## Sequencing rule (BLOCKING — do not skip, do not interleave)
Job 1 must be COMPLETE — and its full review report SAVED to
`.scratch/perch-review-<yourtag>.md` on disk — before you touch (edit, create, or delete) a
single production or test file. Do not fix findings as you go, even ones that look small and
obviously right. A review written or finished after code has already changed is no longer an
independent judgment — it is a post-hoc rationalization of edits you already made, and it
silently destroys the one property this whole method depends on. If you catch yourself wanting
to patch something the moment you spot it: don't. Write it down as a finding, keep reading,
finish the review, save the file, THEN start Job 2. (This is precisely the discipline perch
itself exists to enforce mechanically one layer down — the Review Round Invariant in
`CONSTRAINTS.md` — you reviewing perch are held to the same rule perch holds every burler round
to.)

## Clean-room review constraint (do this part unprimed)
Form your OWN findings first. Do NOT read any prior review or review-dialogue files before you
have your own list. Specifically do not open anything under `.scratch/` (gitignored; holds
prior reviews `perch-review-*.md` and `*-fixer-report.md`) or any `_mill/` content (there is
none left on this branch — it was removed by the pre-merge cleanup commit; see "Design intent"
below for how to recover it). Reading the design SPEC and the module docs is expected and
required (those are not reviews). AFTER you have written your own independent findings, you MAY
consult prior rounds' `.scratch/perch-review-*` material — regardless of which model produced it
(rounds rotate across Opus / Fable / Sonnet; the most recent prior round is whichever
`perch-review-*` file is newest), EXCEPT your own `-<yourtag>` deliverables — to (a) confirm
previously-fixed behaviors have not regressed and (b) re-evaluate the deferred items at the
bottom.

## What to read
- Code: `internal/perchengine/**` (`config.go`, `profile.go`, `state.go`, `result.go`,
  `roundfiles.go`, `engine.go`, `run.go`, `gate.go`, `judge.go`, `judgeverdict.go`, `template.go`
  + the embedded templates `judge-circling-template.md`, `judge-milestone-template.md`,
  `triage-template.md`, `template.yaml`, and every `_test.go` incl. `smoke_judge_test.go`),
  `internal/perchcli/**` (`cli.go`, `run.go`, `pause.go` + tests), and the `cmd/lyx`
  integration (`main.go`, helptree/registration/longlist/sandbox-coverage guard tests). Also
  skim `internal/burlerengine/**` far enough to understand the `Profile`/`Result`/outcome
  contract perch drives (perch is a loop caller on top of burler, exactly as burler is a thin
  caller on top of shuttle — not a reimplementation), and `internal/shuttleengine` for the
  judge/triage transport (`shuttleengine.Spec`/`Runner`) perch composes directly.
- Docs: the `internal/perchengine` package documentation (`doc.go` — the module doc was folded
  into the package doc per the Documentation Lifecycle, exactly like mux/shuttle/burler),
  `docs/overview.md`, `docs/roadmap.md`, `CONSTRAINTS.md` (esp. the Review Round Invariant,
  Weft Git Invariant, Hub Geometry, CLI/Cobra, Shuttle Provider-Seam, Sandbox Suite Coverage),
  `README.md`, and `docs/reviews/README.md` (the method perch is the mechanized, automated form
  of — perch's own round loop IS this loop, minus the human, moved into Go).
- The dedicated live-driving suite you will RUN: `tools/sandbox/SANDBOX-BURLER-SUITE.md`'s **S4**
  scenario (the file holds both burler S1-S3 and perch S4 — perch does not get its own suite
  file; add new perch scenarios to S4 or as new `S`-numbered scenarios in the SAME file) plus
  [`docs/sandbox-howto.md`](../sandbox-howto.md) for how the harness works, including its
  attached-terminal pre-condition (see "What to TEST" below).
- Repo rules you MUST follow: `CLAUDE.md` (root + `~/.claude/CLAUDE.md`) and `CONSTRAINTS.md`. A
  change that ships behaviour without updating the package doc / invariants in the SAME change
  is incomplete.
- Design intent (SPEC, not a review). `_mill/discussion.md` and `_mill/plan/*` were removed from
  this branch by the pre-merge cleanup commit; recover them from git history — the last commit
  that still had them is `dcd4034` ("mill-go: done internal-perch"):
    - `git show dcd4034:_mill/discussion.md`
    - `git show dcd4034:_mill/plan/00-overview.md` and the per-batch cards
      `git show dcd4034:_mill/plan/NN-*.md` (01..05)
  Use these as the authoritative source of intended v1 scope/behavior. Also useful for prose
  context: `git log --oneline main..internal-perch` shows the batch-by-batch build order
  (foundations → profile-state → judge-triage → gate-loop → cli-docs) and the holistic-review
  round that already ran during implementation (2 rounds, both design-time code review, not an
  adversarial live-substrate campaign — see `5fae6b0`..HEAD for the review/fix commits). That was
  a *design-time* review inside mill-go's own pipeline; this loop is a *deeper, adversarial,
  live-substrate* pass on top of it, the same relationship the mux, shuttle, and burler
  campaigns had to their own mill-go reviews.

## Mission (assess on two axes, be adversarial)
1. Scope / omfang — is the module's scope right? Does the as-built code deliver what the design
   intended? Gaps, over-reach, silently-dropped requirements, deferred-that-should-ship-in-v1.
2. Correctness — bugs, races, error handling, edge cases; concentrate on the historically-fragile
   areas below (perch's hermetic layer is a fake-burler + fake-judge unit suite — thorough, but
   by construction it never sees what a REAL burler round or a REAL judge call actually does,
   which is where the whole module's value and risk live). Also assess docs accuracy (do the
   docs match the code?) and operability.

## High-yield focus — where perch's real bugs live (drive these, do not just read them)
The pure/unit-tested parts (round-caps validation, gate-mode/argv-mismatch validation, profile
strict decode, the deterministic loop transitions under a FAKE burler/judge) are solid and
rarely wrong. The defects concentrate in the COMPOSED, LIVE behavior the fake-substrate tests
never exercise — real burler rounds, a real ephemeral judge, real crash/pause timing. Treat every
one of these as an INVARIANT you must actively verify by driving the real substrate — a green
`go test` proves nothing here:

- **MILESTONE LADDER UNDER REAL BLOCKING ROUNDS.** Drive a profile whose fixture keeps failing
  (a rubric a real burler round cannot satisfy, or one it satisfies only after several rounds)
  through a short `round-caps` ladder (e.g. `[2, 3]`). Confirm: the milestone rung (round 2)
  really triggers a judge call INSTEAD OF the per-round circling check (not both, not neither);
  the final rung (round 3) is an unconditional hard cap with NO judge call even though still
  BLOCKING; `StuckReason` is exactly `hard-cap`. Then separately force a milestone `STOP` (a
  fixture/rubric combination a real judge is likely to call non-progressing) and confirm
  `StuckReason` is `milestone-stop`, not `hard-cap` or `circling`.
- **PER-ROUND CIRCLING CHECK — REAL JUDGE, REAL VERDICT FILE.** Drive at least one real circling
  judge call (a BLOCKING round with a prior round to compare against, mid-window, not at a
  milestone rung) and inspect the actual verdict file it writes: does it parse under
  `judgeverdict.go`'s real parser, does the frontmatter carry the fields the doc promises
  (verdict + rationale), does the prose actually carry a human-facing themes overview? Confirm
  round 1 and any round immediately after an APPROVED round never spawn a circling judge at all
  (read `run.go`'s call site, then confirm it live by checking no judge artifact exists for
  those rounds).
- **JUDGE FAIL-SAFE — MUST WARN, MUST NEVER BE AN ERROR OR STUCK.** Force a judge infrastructure
  failure live if you can (a judge-model that fails to spawn, e.g. a bogus `judge-model` string,
  or kill the judge's psmux pane mid-call) and confirm: the loop continues as
  progressing/CONTINUE, `internal/logger`'s Warn actually fires with round + cause (check
  stderr/log output, not just that the code path exists), and the block never returns STUCK or
  an error from this alone. Cross-check the doc.go claim precisely: does Warn actually fire on
  every judge infra failure, or does some real failure mode (e.g. a validly-parsed UNCERTAIN)
  slip through without one — the doc is explicit that UNCERTAIN itself logs no Warn (it is a
  normal outcome) while infra failure does; verify the code matches that exact split, not a
  blurred version of it.
- **GATE MODES — command AND both, live.** `llm-verdict` is the easy path (already implied by
  the milestone/circling driving above). Separately drive `gate: {mode: command, command:
  [argv...]}` with a command that deliberately fails after B's fix: confirm the burler verdict
  is NOT what decides convergence in this mode (a BLOCKING-findings-free review does NOT
  converge the block if the gate command still fails), `round-N-gate.md` is written with the
  actual combined stdout+stderr, and the NEXT round's burler hydration actually includes that
  gate file's content (inspect the composed prompt/hydration, not just that the file exists on
  disk). Then drive `both` and confirm BOTH signals are required (flip each independently and
  watch the outcome track). Confirm the gate command runs with cwd = the WORKTREE ROOT (not the
  run dir, not the shell cwd) and is actually killed at `gate.timeout` (a deliberately
  long-running command should be observed to die at the timeout, not hang the block).
- **NON-DONE BURLER OUTCOMES — retry-then-error, and triage.** If you can force it (e.g. a
  `--timeout` on the underlying shuttle spec, or killing the burler's claude pane mid-round),
  drive at least one `died`/`timeout` outcome and confirm: the round is retried ONCE with a
  fresh burler over the SAME hydration (round number NOT advanced — check the state.json round
  count before/after), and a SECOND consecutive non-done for the same round is a hard ERROR
  (never STUCK) whose message names the round, the shuttle SessionID, and the kept run dir.
  Separately force an `asking` outcome (a profile/rubric that plausibly makes a real agent stop
  to ask a question) and confirm the triage call actually reads `LastAssistantMessage` and
  returns RETRY (retried once, then the second-consecutive rule) or GIVE_UP (hard ERROR
  surfacing the agent's question) — and that triage infra failure itself fails safe to RETRY,
  never an error.
- **RESUME + RUN IDENTITY — profile-hash mismatch, terminal-state, partial-round restart.** Run
  a block to a non-terminal state (or interrupt one), then: (a) resume with the SAME profile and
  confirm it continues at the recorded round using the existing run dir, not from scratch; (b)
  edit the profile (even a whitespace-insignificant change that changes its hash) and resume —
  confirm perch fails loud naming the run-id and instructing a fresh `--run-id`, rather than
  silently continuing under a stale identity; (c) resume a run dir whose `state.json` is already
  terminal (APPROVED/STUCK) and confirm a fail-loud "already finished" error, not a silent
  no-op or a restart; (d) kill a round mid-flight (before its burler round reaches `done`) and
  resume — confirm the round is restarted from scratch (round number not advanced) and that any
  stale partial output files at the SAME round's paths are moved aside first, not left to
  collide with shuttle's pre-existing-output-file rejection.
- **PAUSE — boundary-only, flag-clearing on resume.** Start a block whose fixture needs several
  rounds, and from a SECOND terminal run `lyx perch pause --run-id <id>` WHILE a burler round is
  actively mid-flight. Confirm the running block does NOT pause mid-round — it finishes the
  in-flight round first and only then returns `PAUSED` at the next round boundary (never
  mid-round, never mid-aggregation per the doc's explicit claim). Confirm `StuckReason` is unset
  on a PAUSED result (callers must branch on Outcome before reading it — verify this is actually
  true of the Result Go value, not just documented). Then resume with `lyx perch run` again and
  confirm the pause flag is cleared BEFORE the resumed run re-checks `PauseRequested` — a stale
  flag must never cause an instant re-pause on the very request that was just satisfied.
- **WEFT COMMIT — once, at block exit, CLI-owned, never mid-round.** Confirm across every one of
  the terminal outcomes you drive above (APPROVED, STUCK via any reason, PAUSED) that exactly one
  weft commit lands at block exit (`git -C <weft worktree> log` on the host's weft sibling), that
  `perchengine` itself never touches weft (grep for `weftengine`/`warpengine` imports in
  `internal/perchengine/**` — must be absent, per Weft-blindness in doc.go), and that no round
  boundary in between produces its own commit (the design explicitly rejected per-round commits).
- **PROFILE SURFACE — the one user-authored input.** Strict decode (unknown keys rejected, not
  ignored) across BOTH the burler-content keys and perch's own keys in the same file. Probe
  `round-caps` edge cases live through the CLI, not just the unit-tested validator: empty array,
  single non-positive entry, non-increasing entries, a huge cap that would make a live run
  impractical (confirm it is rejected fast, before any round spawns, not discovered mid-run).
  Probe `gate` mode/argv mismatches the same way (`command`/`both` with empty argv;
  `llm-verdict` with a non-empty argv). Confirm CLI flags `--model`/`--effort`/`--timeout`
  actually override the profile's own values in a live round (not just that the flag parses).
- **run-id DERIVATION AND COLLISION.** Confirm the default run-id derivation (profile path +
  content hash) is actually stable across two separate invocations of the identical profile
  file (same run-id both times → the second one resumes/continues rather than colliding), and
  that editing the profile changes the derived run-id (or is caught by the hash-mismatch check
  in the RESUME case above — clarify which of these two mechanisms actually fires for an
  unmodified `--run-id` vs. a freshly-derived one, since both exist in the codebase and a
  reviewer should nail down exactly which layer catches a given edit).

## Explicitly OUT of scope for perch v1
`loom` (the phase machine) is a separate, not-yet-built module — do not review it or expect
perch to already behave like its future caller; perch only exposes the engine API loom will
call, and profile-driven invocation (not a phase name) is the correct v1 shape. Cluster fan-out
(`cluster-n > 0`) is burler's rejection, inherited unchanged — not a perch gap. Session-level
resume of a crashed mid-round burler (`claude --resume` re-entry) is explicitly deferred;
round-level restart is the v1 contract — do not flag it as missing. Per-rung model/effort
escalation schedules are explicitly v-later; v1's uniform profile `model`/`effort` for every
round is correct, not an omission. LLM-triage of `died`/`timeout` outcomes is explicitly
deferred (v1 handles those deterministically) — do not flag the deterministic-only handling as
a gap. Non-Claude engines are shuttle's seam concern, not perch's. `hardener` (behavior-based
review) is a separate, later module that only shares the round discipline — do not expect perch
to run live substrates itself; it runs burler, which runs shuttle, which runs claude — perch
itself never imports claudeengine or muxengine directly except through its package-local
Shuttle/Burler seams.

## Round context seeded from prior-round verification
**Round 4 (`opus-r4`) — evaluation-checkpoint round over rounds 1-3's fixes.** This is the
4th round in the fixed Fable→Opus→Fable→Opus rotation. Per operator directive: after this round
completes, if the loop has STILL not converged, the orchestrator stops and evaluates with the
operator before spawning any further round — so give this pass everything you have.

Round 1 (`fable-r1`): 16 findings, all fixed (`3bf7284`..`75281da`). Round 2 (`opus-r2`): found
one real defect (O1) introduced by round 1's own P6 fix — a resume landing past the hard cap
could spawn burler/judge rounds unbounded — plus closed a seeded test-coverage gap (O2) and a NIT
(O3), all fixed (`351ebe1`, `f39f1ac`, `b128abb`). Round 3 (`fable-r3`) then did a genuinely fresh
adversarial pass, drove SIX real live blocks (including, for the first time in this campaign, a
genuinely forced `died`→deterministic-retry), and found **two more real MEDIUM defects**, both
introduced independently of rounds 1-2's changes (found by fresh exploration, not by re-checking
prior fixes):

- **F1 (MEDIUM, CONFIRMED by experiment)** — `execGateCommand` had no `cmd.WaitDelay`: a gate
  command that exits fine but leaves a child holding the combined-output pipe (any real test
  runner, build tool, or server-starting fix routinely does this) stalled the gate call for the
  child's whole lifetime — observed 29.4s on a 2s timeout, unbounded for a daemon child —
  defeating `Gate.Timeout` and the block's documented guaranteed termination in command/both
  modes. Fixed (`40cbe12`) with `cmd.WaitDelay = 10s` + `exec.ErrWaitDelay` classified from
  `cmd.ProcessState.Success()`. **The orchestrator independently reproduced this fix's
  not-false-green proof**: removed `cmd.WaitDelay`, ran `TestExecGateCommand_LingeringChildDoesNotHangPastWaitDelay`,
  confirmed BOTH subtests fail at ~19.3s ("want within the ~10s pipe-abandon grace"), restored,
  confirmed `git diff --stat` empty. Solidly covered.
- **F2 (MEDIUM, CONFIRMED by code-trace)** — `perchcli`'s run-dir base was anchored at
  `layout.WorktreeRoot`, while every OTHER `_lyx` anchor (`Layout.LyxDir`, the config loads, the
  weft pathspec) anchors at `layout.Cwd`. `lyx init` supports initializing from any directory
  (RelPath != "."), so a nested-initialized repo would silently write perch run dirs into an
  un-junctioned `_lyx` the weft commit's pathspec never reaches — every block artifact stranded,
  never weft-synced. Fixed (`42b6fc7`) to anchor at `layout.Cwd`. **Independently reproduced**:
  reverted to `layout.WorktreeRoot`, confirmed `TestRunCLI_Pause_NestedInitAnchorsRunDirsAtCwd`
  fails with `{"error":"perch: no such run \"nestedrun\""}`, restored, confirmed clean diff.
- **F3-F5 (LOW)**: lock files (`run.lock`, `state.json.lock`) were being weft-committed into
  durable history (now pathspec-excluded, `45a5c75`); `pause` on an already-terminal block
  falsely reported `ok:true` for a pause nothing could ever honor (now refused loud, `2c31ff9`);
  a fail-fast busy-lock invocation was weft-committing+pushing the OTHER (winning, mid-round)
  invocation's in-flight state under a misleading `... ERROR` label (now skipped via a new
  `ErrBlockBusy` sentinel, `b879b6a`).
- **F6-F7 (NIT)**: a test now pins that the O1 past-cap guard correctly wins over a pending pause
  (`364464b`); a stale header comment fixed (`3ed4564`, `6f3b12f`).

All fixed and verified: `go build`/`vet` clean, `go test -count=5` on perchengine/perchcli/cmd/lyx
all green ×5, live smoke judge test PASS, zero stray psmux processes, worktree clean.

**Notice the pattern across rounds 2 and 3: each fresh, genuinely independent pass has found real
MEDIUM correctness defects that every previous round — including live-driving rounds — missed.**
Do not let this checkpoint round be the one that rubber-stamps. Re-derive your own view of
merge-readiness completely from scratch: read doc.go, the discussion, and all three prior rounds'
review/fixer reports (you MAY read these), then independently drive the substrate again — real
blocks, not just the fake-substrate unit suite — focusing especially on:

(a) **Any other place besides `execGateCommand` where a subprocess/pipe/child-process lifetime
    could silently exceed an intended bound** — burler/shuttle call sites, weft git operations,
    anywhere `exec.Command`/`CommandContext` is used without `WaitDelay` or explicit process-group
    handling. F1's exact failure shape (successful exit, lingering child, unbounded Wait) may
    recur elsewhere.
(b) **Any other `_lyx`/geometry anchor that might disagree with `layout.Cwd`** the way F2's
    `runDirBase` did — grep for every place perch (or any module this review doesn't normally
    cover, if you have budget) resolves a path from `layout.WorktreeRoot` instead of `layout.Cwd`,
    and check each one against the RelPath-mirrored weft pathspec the same way F2 was traced.
(c) Whether F3/F4/F5's fixes compose correctly together and with rounds 1-2's fixes under stress
    — e.g. does the `ErrBlockBusy`-skips-weft-sync path (F5) still correctly weft-sync a REAL
    hard error (P10, round 1) rather than over-broadly skipping the sync for every non-nil error?
(d) Anything ALL THREE prior rounds explicitly could NOT verify live: a real `asking`/triage
    call, a real milestone/circling STUCK verdict inside a full multi-round live block, live
    judge-input pollution (P2), and the full interactive SANDBOX-BURLER-SUITE (S4/S5) walk via
    its actual launcher (all three rounds lacked an attached console). If you have any way to
    drive these that no prior round found, do so — otherwise state plainly why not.

State the **merge bar** so you calibrate: correctness in the NORMAL single-block flow (one
profile, one run, real burler rounds, real judge calls where applicable) is the gate. Perch has
no equivalent of mux/burler's N×-concurrent-suite stress amplifier in this method (its own
concurrency story is bounded by a single sequential round loop, not parallel sessions) — do not
invent an artificial concurrency stress pass for perch itself; concentrate your live-driving
budget on the high-yield list above instead.

## What to TEST — do not just read, EXERCISE it
Report the exact commands you ran and what you observed.

Hermetic (must stay green throughout):
- `go build ./...`
- `go vet ./internal/perchengine/... ./internal/perchcli/...`
- `go test -count=5 ./internal/perchengine/... ./internal/perchcli/... ./cmd/lyx/...`

Live smoke (real substrate, behind the `smoke` build tag):
- `go test -tags smoke ./internal/perchengine/... -run TestSmokeJudge -v -count=1` — one real
  per-round circling judge call over two tiny fixture review files; skips when no claude
  resolves. Note this is DELIBERATELY narrow (proves the judge spawn machinery + file contract
  + verdict parse, never judge quality) — it is not a substitute for driving full live blocks
  below.
- psmux is installed at `C:\Code\tools\bin\psmux.exe` (also on PATH as `psmux`); pwsh 7 at
  `C:\Code\tools\powershell7\pwsh.exe`. A logged-in `claude` must be on PATH for the real-agent
  scenarios — launch tools with EXPLICIT absolute paths where the codebase does (a bare `pwsh`
  resolves to a 0-byte WindowsApps ConPTY stub that renders nothing).

Live driving via the SANDBOX SUITE (PRIMARY — where the bugs surface):
- Deploy the current source as the binary under test: `deploy.cmd`. **FOOTGUN:** the suite runs
  the DEPLOYED snapshot, not your working tree — re-run `deploy.cmd` after EVERY source change or
  you validate a stale binary. Deploy first, always.
- Materialize the sandbox hub: `sandbox-build.cmd` (or `-reset` for a clean start), run `lyx init`
  in the hub host repo, then launch the interactive suite session: `sandbox-burler-suite.cmd`.
  **The launcher MUST run in a real, attached terminal — never backgrounded, detached, or with
  stdio redirected** (a detached driver session can end early and silently abandon scenarios).
  Follow `SANDBOX-BURLER-SUITE.md`'s own Pre-conditions + "How to run a scenario" sections. Walk
  **S4** (the perch gate loop — convergence, run-dir inspection, weft commit, then pause/resume)
  and record OK/WARN/FAIL per its verdict key. After the session, pull findings back with
  `sandbox-fetch.cmd`.
- The suite (S4) is a FLOOR, not a ceiling. It proves one straightforward convergence and one
  pause/resume — it does NOT exercise milestone STOP, hard-cap, circling STUCK, gate-command
  mode, non-done burler outcomes, or a profile-hash-mismatch resume. Devise and hand-drive MANY
  more adversarial live blocks of your own beyond S4 — the "High-yield focus" list above is your
  checklist for what to construct profiles/fixtures for. Real blocks cost real burler rounds
  (each a real claude session) — sequence them deliberately, but do not substitute reading for
  driving; this module's entire value proposition is unproven until you have watched it drive a
  real STUCK, a real milestone gate, and a real PAUSE/resume with your own eyes.

TEARDOWN DISCIPLINE (critical): if you start any psmux server/session, tear it down
(`lyx mux down`, or `psmux -L <socket> kill-server`). The burler-suite launcher runs `lyx mux
down` itself after the session — verify it actually did. At the end, confirm with `tasklist |
grep -i psmux` that ZERO psmux processes remain, and that no shuttle/burler/perch run
directories are left under the hub host repo's `_lyx/` from a run you started (perch's own run
dirs live under a perch-runs area alongside shuttle's — check both). Leave no stray state. Be
honest about what you could NOT verify and why (e.g. anything needing a claude account state you
don't have, or a non-done outcome you could not reliably force).

## How to judge each finding
For each code finding give: `file:line`, a concrete failure scenario (inputs/state → wrong
behavior), severity (BLOCKING / MEDIUM / LOW / NIT), suggested fix, and CONFIRMED
(reproduced/traced) vs PLAUSIBLE (looks wrong, unverified). For scope: plan-promised vs shipped;
flag deferred-that-should-be-v1 and shipped-beyond-scope.

**Severity affects how you REPORT a finding, not whether you fix it.** ALL findings you record
get fixed in Job 2 — including every NIT — not just BLOCKING/MEDIUM ones. A finding you write
down but leave unfixed as "low priority" is not actually a reported finding; it is a dropped one
that will either silently vanish or re-surface and loop across future rounds instead of closing
(this is a known failure mode from an earlier review setup this method is descended from — and
it is the very rule perch's own gate loop enforces mechanically on burler rounds; hold yourself
to it). The only legitimate reason to leave a finding unfixed is that fixing it genuinely
requires something you cannot do alone this round — an operator decision on a real design
tradeoff, or a live capability you don't have (e.g. reliably forcing a specific non-done
outcome). Even then you must say so explicitly, with the specific reason, in the fixer report's
deferred section — never bucket something as "deferred, low priority" just because it felt
small. Small and low-severity findings are usually the CHEAPEST to fix, not a reason to skip
them.

## Deferred items from the prior round — RE-EVALUATE these (after your own pass)
(Empty — this is round 1; nothing has been deferred yet.)

## Fixing — after the review
- Fix EVERY finding from your review, all severities including NIT (see "How to judge each
  finding" above for the full rationale) — not just BLOCKING/MEDIUM ones.
- Load the code-quality guidance (`/code-quality` skill or `mill:code-quality`) AND the
  language-specific skills for this codebase (`mill:golang-build`, `mill:golang-testing`,
  `mill:golang-comments`) before editing — all of them, not just code-quality. Prefer surgical
  edits; match existing style and the file-level doc-comment convention.
- For every bug you fix, add or extend a test that would have caught it. For a live-only defect,
  add a `//go:build smoke` test that walks the failing scenario against the real substrate
  (`internal/perchengine/smoke_judge_test.go` shows the pattern, incl. the skip when the
  substrate is absent and the self-contained-helpers convention) OR extend the deterministic
  fake-burler/fake-judge loop tests when the defect is reproducible without a real agent. A
  hermetic unit test for the pure helper is good; a smoke test for the composed behavior is what
  protects the live-substrate paths this method exists to check.
- MAKE SMOKE TESTS DETERMINISTIC. A live round's or judge call's timing is the agent's own — a
  test that assumes turn timing passes on a quiet machine and FLAKES on a loaded one. Wait on the
  actual state transition (poll with a deadline), never sleep a fixed amount. Prove determinism
  by running the new test repeatedly, not once.
- EXTEND THE SANDBOX SUITE (S4, or a new S-numbered scenario in `SANDBOX-BURLER-SUITE.md`) when
  a review surfaces a live/visual behavior it doesn't cover (match the existing scenario
  Goal/Watch/Verdict shape; keep the coverage guard green in the SAME change — `go test
  ./cmd/lyx/...` includes `sandbox_coverage_test.go`).
- Keep `go build`/`vet`/`test` green after every change. Then RE-DEPLOY (`deploy.cmd`) and
  re-run the smoke + suite scenarios — re-deploying FIRST is mandatory (the suite tests the
  deployed binary, not your working tree).
- Update the `internal/perchengine` package documentation (and `docs/overview.md` /
  `CONSTRAINTS.md` if invariants or the module table move) IN THE SAME change. Do NOT add
  bugfix/hardening notes to `docs/roadmap.md` (roadmap is planned milestones only, per
  `CLAUDE.md`).
- Tear down all substrate state; confirm zero stray psmux processes and no leftover
  shuttle/burler/perch run directories. COMMIT each fix as you finish it (see "Commit per fix"
  above) — do NOT push unless the user explicitly asks. Report the changed files and how you
  verified each fix.

## Deliverables
1. A structured review report (Executive summary with top risks + merge-readiness opinion; Scope
   assessment plan-vs-shipped; Code findings severity-ranked with file:line + scenario + fix +
   CONFIRMED/PLAUSIBLE; Docs & operability findings; What-was-tested with exact commands and
   observed results, including what you could NOT verify and why). Write it to
   `.scratch/perch-review-<yourtag>.md`.
2. A fixer report: what you implemented, what you deliberately deferred (with reasons), the exact
   test commands run + results, and the changed files. Write it to
   `.scratch/perch-review-<yourtag>-fixer-report.md`.
3. In your final chat message: a concise summary (executive summary + counts by severity + the two
   report paths + an explicit merge-readiness verdict). Do not paste the whole reports.

Begin with the clean-room review (read the SPEC + code + docs, then drive real perch blocks and
real judge calls), produce your independent findings, then implement and verify the fixes.
