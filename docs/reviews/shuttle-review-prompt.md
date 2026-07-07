# shuttle — independent review + fix

You are a senior engineer doing a COMPLETE, adversarial, INDEPENDENT review of the `shuttle`
module in the loomyard repo, followed by FIXING what you find. Work in the worktree at
`C:\Code\loomyard\wts\internal-shuttle` (branch `internal-shuttle`). Adjust that path/branch if
the task lives elsewhere now. Note: PR #54 (https://github.com/Knatte18/loomyard/pull/54) is open
against `main` for this branch but not yet merged — you are hardening it further before merge,
exactly like the mux campaign hardened `internal-mux` before its own merge.

## Your two jobs, in order
1. REVIEW: form your own independent judgment of shuttle's scope and correctness. Hunt for bugs by
   reading the code AND by driving a real psmux session with a real, logged-in `claude` (this is
   where shuttle's defects hide — it drives an actual interactive TUI over a file contract).
2. FIX: after you have a findings list, implement the fixes ONE AT A TIME, verify each against the
   real substrate, keep the whole test suite green, and update the docs in the same change as the
   fix they document. COMMIT after each individual fix lands green (see "Commit per fix" below). Do
   NOT push unless the user explicitly tells you to.

## Commit per fix (BLOCKING — do not batch fixes into one uncommitted diff)
As soon as one finding's fix is implemented, green, and its doc update (if any) is included, COMMIT
it — on the current branch, no push — before starting the next finding. Commit message format:
`shuttle: fix <finding-id> — <one-line what/why>` (e.g. `shuttle: fix M1 — assert redirected file
content, not Wait outcome, after interrupt+send`), so the finding ID matches exactly what your
review report calls it. Do not commit `.scratch/` (gitignored). This exists because round 2 of this
exact loop was killed mid-fix by a terminal corruption issue outside the method's control, leaving
several real fixes sitting as one uncommitted diff with no fixer report — the orchestrator had to
reverse-engineer, finding by finding, which fixes were actually complete before it could safely
continue. Small per-finding commits make that recovery trivial instead.

## Sequencing rule (BLOCKING — do not skip, do not interleave)
Job 1 must be COMPLETE — and its full review report SAVED to
`.scratch/shuttle-review-<yourtag>.md` on disk — before you touch (edit, create, or delete) a
single production or test file. Do not fix findings as you go, even ones that look small and
obviously right. A review written or finished after code has already changed is no longer an
independent judgment — it is a post-hoc rationalization of edits you already made, and it silently
destroys the one property this whole method depends on. If you catch yourself wanting to patch
something the moment you spot it: don't. Write it down as a finding, keep reading, finish the
review, save the file, THEN start Job 2. (This rule exists because round 1 of this exact loop
interleaved review and fix — it had modified `internal/shuttleengine/run.go`, `wait.go`,
`fakes_test.go`, `run_test.go`, and `internal/shuttlecli/smoke_interrupt_test.go` before writing a
single line of its review report. Do not repeat that.)

## Clean-room review constraint (do this part unprimed)
Form your OWN findings first. Do NOT read any prior review or review-dialogue files before you
have your own list. Specifically do not open anything under `.scratch/` (gitignored; holds prior
reviews `shuttle-review-*.md` and `*-fixer-report.md`) or any `_mill/` content (there is none left
on this branch — it was removed by the pre-merge cleanup commit; see "Design intent" below for how
to recover it). Reading the design SPEC and the module docs is expected and required (those are
not reviews). AFTER you have written your own independent findings, you MAY consult prior rounds'
`.scratch/shuttle-review-*` material — regardless of which model produced it (rounds rotate across
Opus / Fable / Sonnet; the most recent prior round is whichever `shuttle-review-*` file is newest),
EXCEPT your own `-<yourtag>` deliverables — to (a) confirm previously-fixed behaviors have not
regressed and (b) re-evaluate the deferred items at the bottom.

## What to read
- Code: `internal/shuttleengine/**` (incl. `internal/shuttleengine/claudeengine/**`),
  `internal/shuttlecli/**`, and the `cmd/lyx` integration (`main.go`, sandbox/help/registration
  guard tests). Also skim `internal/muxengine/**` far enough to understand the `Strand`/`Status`/
  `CapturePane`/`SendKey` contract shuttle drives — shuttle is a thin caller on top of mux, not a
  reimplementation of it.
- Docs: the `internal/shuttleengine` and `internal/shuttleengine/claudeengine` package
  documentation (`doc.go` in each — the module docs were deleted per the Documentation Lifecycle
  once shuttle landed, exactly like mux), `docs/overview.md`, `docs/roadmap.md` (milestone 10),
  `CONSTRAINTS.md` (esp. the Shuttle Provider-Seam Invariant), `README.md`.
- The dedicated live-driving suite you will RUN: `tools/sandbox/SANDBOX-SHUTTLE-SUITE.md`
  (scenarios S1–S3) plus [`docs/sandbox-howto.md`](../sandbox-howto.md) for how the sandbox harness
  works.
- Repo rules you MUST follow: `CLAUDE.md` (root + `~/.claude/CLAUDE.md`) and `CONSTRAINTS.md` (Hub
  Geometry, CLI/Cobra, lyxtest Leaf, Shuttle Provider-Seam, Sandbox Suite Coverage, Documentation
  Lifecycle). A change that ships behaviour without updating the package doc / invariants in the
  SAME change is incomplete.
- Design intent (SPEC, not a review). `_mill/discussion.md` and `_mill/plan/*` were removed from
  this branch by the pre-merge cleanup commit; recover them from git history — the last commit that
  still had them is `6e66afb` ("mill-go: done internal-shuttle"):
    - `git show 6e66afb:_mill/discussion.md`
    - `git show 6e66afb:_mill/plan/00-overview.md` and the per-batch cards
      `git show 6e66afb:_mill/plan/NN-*.md` (01..07)
  Use these as the authoritative source of intended v1 scope/behavior. Also useful for prose
  context: `git log --oneline` shows the batch-by-batch build order and the two holistic-review
  rounds that already ran during implementation (round 1 found a real gap — `shuttle-suite` wiring
  — and fixed it; round 2 approved clean). That was a *design-time* review inside mill-go's own
  pipeline; this loop is a *deeper, adversarial, live-substrate* pass on top of it, the same
  relationship the mux campaign had to mux's own mill-go review.

## Mission (assess on two axes, be adversarial)
1. Scope / omfang — is the module's scope right? Does the as-built code deliver what the design
   intended? Gaps, over-reach, silently-dropped requirements, deferred-that-should-ship-in-v1.
2. Correctness — bugs, races, error handling, edge cases; concentrate on the historically-fragile
   areas below (shuttle is architecturally similar to mux — a live TUI driven via psmux — so the
   same *class* of live/composed bugs is the highest-yield place to look). Also assess docs
   accuracy (do the docs match the code?) and operability.

## High-yield focus — where shuttle's real bugs live (drive these, do not just read them)
The pure/unit-tested parts (Spec validation, run-dir bookkeeping, event parsing, the posix path
helper) are solid and rarely wrong. The defects concentrate in the COMPOSED, LIVE behavior the
hermetic tests never exercise — a real claude TUI running in a real psmux pane. Treat every one of
these as an INVARIANT you must actively verify by driving the real substrate — a green `go test`
proves nothing here:

- **DONE/ASKING RACE.** `wait.go`'s `pollEventsTick` classifies outcome from the LAST `StopEvent` in
  a newly-read batch, checking `allOutputFilesExist` at that instant. Verify: an interrupted turn
  immediately followed by a resumed one (two Stop events land in the same poll tick) is classified
  by the most recent turn only, and every consumed byte still counts (none of the earlier events in
  that batch gets reprocessed on the next tick). Also chase the inverse race: the agent writes its
  output file(s) an instant AFTER the Stop event line is flushed vs. an instant BEFORE — does a slow
  filesystem write ever cause a real `done` run to be misclassified as `asking` (or vice versa)?
- **ORPHAN-SWEEP RACE.** `rundir.go`'s `createRunDir` runs BEFORE `mux.AddStrand` — a run directory
  and its `run.json` can exist for a strand that mux does not know about yet. `sweepOrphans`'s
  `minAge` guard is the only thing protecting a starting-up run from being swept as an orphan.
  Verify live: start two shuttle runs back-to-back (or concurrently) so a second run's orphan sweep
  fires while the first run is still inside that window, and confirm neither run's directory is
  ever deleted prematurely.
- **STARTUP HEURISTIC FRAGILITY.** `claudeengine.Startup` classifies a pane's capture by substring
  match (`"trust"`+`"folder"` for the trust dialog; `"❯"` or `"shortcuts"` for ready). Drive a REAL
  claude launch (first-run trust dialog if you can trigger a fresh worktree, and the normal case)
  and confirm the classification is correct against what actually renders — including whatever
  claude version is installed locally, which may differ from what the heuristic was proven against
  (`docs/research/mux-hooks-exploration.md`). Watch for false positives (e.g. a prompt the AGENT
  ITSELF writes that happens to contain "trust" or "folder").
- **DIED vs. DONE RACE.** `checkLivenessTick` only samples `mux.Status` every
  `LivenessEveryNPolls`-th tick. If the claude process crashes right after writing all output files
  but before its Stop hook ever appends an event line, does the run correctly report `done` (files
  exist, so a subsequent events-based check would still classify done) or does a liveness tick that
  lands first wrongly report `died` even though the file contract was actually satisfied? Drive an
  actual crash (kill the claude process inside the pane) at each of these timings and observe.
- **CLEANUP HONESTY / KeepPane.** `finalize` only removes the strand and run dir for
  `OutcomeDone && !KeepPane`; every other outcome (`asking`, `died`, `timeout`) must leave BOTH
  alive for diagnosis/attach. Verify live that an `asking` run really does survive attach+continue
  (S2), and that a `died`/`timeout` run's pane and run dir are still inspectable afterward — and
  that a LATER run's orphan sweep does not prematurely delete an still-diagnosable `died`/`timeout`
  run's directory before an operator gets to look at it (the sweep only keys off `strandGUIDs`
  presence, not outcome).
- **INTERRUPT / SEND CROSS-TERMINAL (S3).** Interrupt an in-progress turn from a second terminal,
  confirm the pane and session survive (`live: true`), then `send` — verify multiline text is
  REJECTED outright (not silently truncated or mis-submitted across multiple Enter presses), that a
  single-line send correctly becomes the agent's next turn, and that sending/interrupting a guid
  whose run has ALREADY reached a terminal state produces a sane error rather than corrupting a
  dead or nonexistent pane.
- **PROVIDER-SEAM LEAKAGE.** Per the Shuttle Provider-Seam Invariant, no Claude-specific marker
  strings, hook payload shapes, or TUI grammar should leak outside `claudeengine`. Grep
  `internal/shuttleengine` (excluding `claudeengine`) and `internal/shuttlecli` for anything
  Claude-flavored that should have stayed inside the adapter.
- **ENV / HOOK HYGIENE.** Check `command.go`/`settings.go` for how the launch command and
  `settings.json` hook schema are constructed — is there any equivalent of mux's `CleanClaudeEnv`
  concern (stray `CLAUDECODE`/`CLAUDE_CODE_*` env vars leaking into a nested shuttle-launched
  claude), and does `posix.go`'s Windows-to-POSIX path conversion hold for hook commands with
  spaces/unusual characters in the worktree path?
- **MID-OP FAILURE ORPHANS.** If `createRunDir`+`saveRunState` succeed but the subsequent
  `mux.AddStrand` (or the claude launch itself) fails, is the run directory left behind as a
  reportable failure, or does it become an untracked, un-swept orphan (no strand ever gets bound,
  so `sweepOrphans`'s `strandGUIDs` check would eventually catch it, but confirm the caller surfaces
  the failure honestly rather than silently returning nothing)?

## Explicitly OUT of scope for shuttle v1
Non-Claude engines (Gemini etc.) — the whole point of the Engine seam is that they are *possible*,
not that one exists; their absence is correct, do not flag it. The `perch` + `burler` modules (the
generic gate loop and its review+fix round worker that will drive handler/fixer/cluster-reviewer loops
on top of shuttle) and `loom` are separate, not-yet-built modules — do not review them or expect
shuttle to already behave like their future caller. Multi-agent cluster reviews (N parallel shuttle strands under one review round) are
explicitly future work per `docs/roadmap.md` milestone 10's notes — shuttle only needs to run ONE
agent well.

## Round context seeded from prior-round verification
**Safety pass (round 5 — a clean-check).** There is NO known residual. Rounds 1-4 each found real
bugs (though round 4's were much smaller: 1 LOW + 1 NIT, zero BLOCKING/MEDIUM — a sharp drop from
round 3's 10 findings including a BLOCKING regression), fixed them, and every fix was independently
verified by the orchestrator (hermetic gates + live smoke reruns, not the round's own self-report)
— do NOT re-open or re-litigate any of these, they are CLOSED AND VERIFIED:
- F1 (round 1): a `saveRunState` failure during `Start` left a live, untracked strand with no
  recovery path (`internal/shuttleengine/run.go`) — fixed by removing the strand and run dir on
  that failure path.
- F2 (round 1): a pane dying right after satisfying the file contract but before its Stop hook
  fired was misclassified as `died` instead of `done` (`internal/shuttleengine/wait.go`) — fixed by
  checking output files first.
- F4 (round 1): the Stop hook's shell command did not escape single quotes in the run's events
  path (`internal/shuttleengine/claudeengine/settings.go`) — fixed with the standard POSIX quote
  idiom.
- docs (round 1): `docs/overview.md` self-contradicted its own module table, claiming shuttle has
  no CLI — fixed.
- M1 (round 2): `TestSmokeInterruptSendContinues` asserted the racy `Wait == done` property —
  rewritten to assert the deterministic redirected-file-content property instead.
- M2 (round 2): the `Interrupt` doc comment overpromised a deterministic `done` after interrupt+send;
  calibrated to describe what live probing actually established.
- N1 (round 2): `run_timeout_min: 0` silently means "times out immediately," not "unlimited";
  documented in `config.go`, `spec.go`, `template.yaml`.
- **H1 (round 3, BLOCKING — a regression IN round 2's own L1 fix)**: the real claude trust dialog
  (`❯ 1. Yes, I trust this folder`) contains the "❯" ready marker itself, so round 2's
  ready-before-trust ordering hung EVERY fresh-directory run at the trust gate until timeout.
  Reordered to check trust FIRST with tight whitespace-stripped whole-phrase needles
  (`trustthisfolder`, `filesinthisfolder`), which still preserves the original L1 concern (a
  coincidentally-worded ready pane still classifies Ready). This is exactly why a round that finds
  something doesn't count as a safety pass — even a fix independently verified two rounds ago can
  regress against a real substrate detail no one had captured yet.
- M1 (round 3): a pre-existing `OutputFiles` entry silently classified an asking run as done on the
  first Stop — now rejected at spec validation.
- M2 (round 3): `Interrupt`/`Send` against a dead/untracked strand silently no-opped (`ok:true`) —
  now checks `mux.Status` liveness first and refuses with a clear error (closes what round 2 had
  deferred as its own L2).
- H2 (round 3): `Send`'s Escape+text choreography could silently coalesce and vanish in the TUI's
  input parser — now verifies delivery via pane-capture polling before returning success.
- L1 (round 3): a `muxengine.LoadState` error (corrupt/unreadable `mux.json`) degraded to "sweep
  with an empty live set," which could delete kept `died`/`asking`/`timeout` run dirs over an
  unrelated I/O problem — now skips the sweep entirely on a read error.
- L2 (round 3): `pollEventsTick` advanced its read offset before `ParseEvents` succeeded, so a
  transient parse error would silently discard the unconsumed batch forever — offset now only
  advances after a successful parse.
- L3 (round 3): `--model` was interpolated unquoted into the pwsh launch line — now single-quoted
  like every other argument.
- N1 (round 3): the steer-text init guard only rejected an embedded single quote, not `"` or `\` —
  now rejects all three.
- N2 (round 3): `startup_timeout_s: 0` silently fast-fails startup AND zeroes the orphan-sweep age
  guard — documented.
- N3 (round 3): `Runner.Interrupt`/`Send` masked `FindRun`'s underlying error behind a generic "not
  a shuttle strand" message — now wraps it (`%w`) so a real I/O error isn't misreported as a bad guid.

Round 3 also **examined and judged genuinely defensible, not a bug — do NOT treat as new work**: a
turn-end `Stop` with `background_tasks` still running classifies `asking` (the file contract is
genuinely unmet; no action needed for v1).

- **R4-1 (round 4, LOW)**: `Send` accepted empty/whitespace-only text, which made `sendVerified`'s
  pane-capture check vacuous (an empty needle matches any capture), falsely claiming a verified
  delivery while still playing a stray empty turn — fixed by rejecting empty/whitespace text in a
  shared `validateSendText` guard.
- **R4-2 (round 4, NIT)**: the session id was the one argument still interpolated unquoted into the
  pwsh launch/resume lines (inconsistent with round 3's L3 `--model` quoting); safe today (always a
  UUID) but now quoted uniformly as defense-in-depth.
- **R5-1 (round 5, MEDIUM)**: prompts over ~30000 bytes broke the pane launch against the Windows
  command-line ceiling, surfacing as an opaque `died` ~90s later — now rejected instantly at
  `Prepare` with a self-describing error steering to the file-pointer pattern.
- **R5-2 (round 5, MEDIUM)**: `Interrupt`/`Send` against a strand whose provider crashed at launch
  but whose pane's shell survived would play keys into that shell — proven live, sent text got
  EXECUTED as a pwsh command. Now all four entry points require the engine to classify the pane
  `StartupReady` (the actual provider TUI on screen) before playing any keys.
- **R5-3 (round 5, LOW)**: `SANDBOX-SHUTTLE-SUITE.md`'s S3 scenario text oversold a deterministic
  `done` outcome that round 2 already proved isn't guaranteed — recalibrated to accept `asking` with
  the redirected file as the real pass criterion.
- **R5-4 (round 5, LOW)**: `OutcomeDied`/`OutcomeTimeout` docs overclaimed "process ended" — a
  mid-run crash behind a surviving shell is invisible to liveness checks and classifies `timeout`;
  docs calibrated to the real pane-death boundary.
- **R5-5 (round 5, LOW)**: `sendVerified` was vacuous when the sent text already pre-existed on
  screen (a retry, or text quoting visible agent output) — now requires the occurrence COUNT to
  rise, not mere presence.
- **R5-6 (round 5, NIT)**: the trust-dismissal Enter keypress was hardcoded in the run loop instead
  of living behind the Engine seam — moved to `Engine.TrustDismissSequence()`.
- **R5-7 (round 5, NIT)**: `poll_interval_ms: 0` (or negative) silently busy-spun instead of failing
  fast like the timeout keys — now floored to the template default.
- **R5-8 (round 5, NIT)**: `run_dir` sharing across worktrees would let one worktree's orphan sweep
  delete another's kept run dirs — documented as worktree-local-only.
- **R5-9 (round 5, NIT)**: `--output-file`'s relative-path resolution base (the worktree root) was
  undocumented in the CLI help — now stated.
- **R6-1 (a dedicated targeted fix, not a full round)**: `TestSmokeInterruptSendContinues`'s
  mid-turn detection (matching "≥3 visible numeric lines") could only ever match AFTER a turn had
  already ended — the provider TUI renders no streamed text mid-turn, flushing the whole response in
  ONE frame at turn end (proven live with 250ms-interval captures). Reproduced live: a hard 4/4-miss
  failure (855s) and a 3rd-attempt near-miss pass (561s), minutes apart, in isolation (not resource
  contention). Fixed by detecting the pane capture CHANGING 3 times across StartupReady-classified
  polls (the spinner's own per-second repaint is a reliable turn-ongoing heartbeat, independent of
  when the response content itself appears). Verified: 13/13 consecutive live passes across two
  independent verification sessions.

**Your job this round: a genuinely independent clean-room safety pass — this is explicitly ANOTHER
"clean-check" round**, after round 5's clean-check itself turned up 9 real findings (2 MEDIUM) —
proof that "severity trending down" is not evidence of safety. Do not assume there is nothing left
just because a lot has already been fixed — round 3 found a BLOCKING regression in a fix that had
already been verified clean two rounds prior, and round 5 (itself seeded as a clean-check) found
MORE and WORSE than round 4 did. Equally, do not invent work to justify the round: "no new defects,
ship it" is the expected, valuable outcome of a genuine clean-check, but only if it is actually true
after a genuinely adversarial look, not a rubber stamp. Fix EVERY finding you record, all severities
including NIT — severity affects how you report a finding, not whether you fix it (see "How to
judge each finding" below). Read the full prior findings above and the deferred list below only
AFTER forming your own
independent judgment (per the Clean-room review constraint above).

State the **merge bar** so you calibrate: correctness in the NORMAL single-instance flow (one
shuttle run, one strand, one claude session) is the gate; an N×-concurrent stress suite (if you
choose to run one) is a diagnostic amplifier, not a merge blocker.

## What to TEST — do not just read, EXERCISE it
Report the exact commands you ran and what you observed.

Hermetic (must stay green throughout):
- `go build ./...`
- `go vet ./internal/shuttleengine/... ./internal/shuttleengine/claudeengine/... ./internal/shuttlecli/...`
- `go test -count=5 ./internal/shuttleengine/... ./internal/shuttleengine/claudeengine/... ./internal/shuttlecli/... ./cmd/lyx/...`

Live smoke (real substrate, behind the `smoke` build tag):
- `go test -tags smoke ./internal/shuttlecli/... -run Smoke -v -count=1`
- psmux is installed at `C:\Code\tools\bin\psmux.exe` (also on PATH as `psmux`); pwsh 7 at
  `C:\Code\tools\powershell7\pwsh.exe`. A logged-in `claude` must be on PATH for the real-agent
  scenarios — launch tools with EXPLICIT absolute paths where the codebase does (a bare `pwsh`
  resolves to a 0-byte WindowsApps ConPTY stub that renders nothing).

Live driving via the SANDBOX SUITE (PRIMARY — where the bugs surface):
- Deploy the current source as the binary under test: `deploy.cmd`. **FOOTGUN:** the suite runs the
  DEPLOYED snapshot, not your working tree — re-run `deploy.cmd` after EVERY source change or you
  validate a stale binary. Deploy first, always.
- Materialize the sandbox hub: `sandbox-build.cmd` (or `-reset` for a clean start), then launch the
  interactive suite session: `sandbox-shuttle-suite.cmd`, which runs
  `go run ./tools/sandbox -parent C:\Code shuttle-suite` and copies `SANDBOX-SHUTTLE-SUITE.md`
  (with a binary-fingerprint header) into the sandbox Hub host repo. Follow that file's own
  Pre-conditions + "How to run a scenario" sections. Walk S1 (autonomous happy path), S2 (asking +
  operator-assisted attach/answer), and S3 (interrupt + send) and record OK/WARN/FAIL per its
  verdict key. After the session, pull findings back with `sandbox-fetch.cmd`.
- The suite is a FLOOR, not a ceiling. Devise and run MANY more adversarial scenarios of your own
  beyond S1–S3 — the "High-yield focus" list above is your checklist for what to hand-drive that the
  three scripted scenarios don't isolate on their own (concurrent runs, mid-startup crash, interrupt/
  send against a terminal-state guid, env hygiene, etc).
- S2's operator-assisted attach step needs a real TTY in a second terminal — flag it as
  not-headlessly-verifiable if you cannot drive it yourself, rather than skipping silently.

TEARDOWN DISCIPLINE (critical): if you start any psmux server/session, tear it down
(`psmux -L <socket> kill-server`, or `lyx mux down`). At the end, confirm with
`tasklist | grep -i psmux` that ZERO psmux processes remain, and that no shuttle run directories
are left under `_lyx/shuttle/` from a run you started. Leave no stray state. Be honest about what
you could NOT verify and why (e.g. S2's live attach step, or anything needing a claude account
state you don't have).

## How to judge each finding
For each code finding give: `file:line`, a concrete failure scenario (inputs/state → wrong
behavior), severity (BLOCKING / MEDIUM / LOW / NIT), suggested fix, and CONFIRMED
(reproduced/traced) vs PLAUSIBLE (looks wrong, unverified). For scope: plan-promised vs shipped;
flag deferred-that-should-be-v1 and shipped-beyond-scope.

**Severity affects how you REPORT a finding, not whether you fix it.** ALL findings you record get
fixed in Job 2 — including every NIT — not just BLOCKING/MEDIUM ones. A finding you write down but
leave unfixed as "low priority" is not actually a reported finding; it is a dropped one that will
either silently vanish or re-surface and loop across future rounds instead of closing (this is a
known failure mode from an earlier review setup this method is descended from: NITs left unfixed
"because they're just NITs" kept escalating and going in circles instead of ever getting closed).
The only legitimate reason to leave a finding unfixed is that fixing it genuinely requires
something you cannot do alone this round — an operator decision on a real design tradeoff, or a
live capability you don't have (e.g. a second real TTY). Even then you must say so explicitly, with
the specific reason, in the fixer report's deferred section — never bucket something as "deferred,
low priority" just because it felt small. Small and low-severity findings are usually the CHEAPEST
to fix, not a reason to skip them.

## Deferred items from the prior round — RE-EVALUATE these (after your own pass)
- **S2's operator-assisted attach step** (needs a real TTY in a second terminal): still not driven
  live by either round 1 or round 2 — both honestly flagged it as not-headlessly-verifiable. Still
  open; drive it yourself if you can get a real second terminal, otherwise flag it again.
- **Concurrent-run orphan-sweep race**: round 1 only reasoned about this; round 2 went further and
  **traced it to safe** (not live-driven, but by code inspection): mux never deletes a strand record
  on pane death (`reconcile.go` clears `PaneID` only — "only RemoveStrand deletes one"), so a
  `died`/`asking`/`timeout` run's GUID persists in `mux.json` and `sweepOrphans` skips it; the
  `minAge = 2×startup_timeout` guard covers the pre-`AddStrand` window. Re-evaluate whether this
  tracing is convincing enough, or whether it's worth actually driving live for extra confidence in
  a safety pass.
- **Manual second-process CLI invocation of `interrupt`/`send`**: round 1 hadn't exercised this;
  round 2 **verified it live** (cross-process CLI interrupt/send against a blocking run in the
  sandbox hub — keys reached the pane, the agent rewrote the output file). Consider this CLOSED
  unless your own pass finds a reason to doubt it.

## Fixing — after the review
- Fix EVERY finding from your review, all severities including NIT (see "How to judge each
  finding" above for the full rationale) — not just BLOCKING/MEDIUM ones.
- Load the code-quality guidance (`/code-quality` skill or `mill:code-quality`) AND the
  language-specific skills for this codebase (`mill:golang-build`, `mill:golang-testing`,
  `mill:golang-comments`) before editing — all of them, not just code-quality. (This is called out
  explicitly because round 2 of this exact loop loaded code-quality only and skipped the golang
  skills when it reached this step; the operator caught it live and had to stop the round to
  redirect it.) Prefer surgical edits; match existing style and the file-level doc-comment
  convention.
- For every bug you fix, add or extend a test that would have caught it. For a live-only defect,
  add a `//go:build smoke` test that walks the failing scenario against the real substrate (the
  existing `internal/shuttlecli` smoke test files show the pattern, incl. a skip when the substrate
  is absent). A hermetic unit test for the pure helper is good; a smoke test for the composed
  behavior is what protects the recovery paths.
- MAKE SMOKE TESTS DETERMINISTIC. Substrate operations are asynchronous (claude's own turn timing,
  psmux pane state) — a test that assumes a verb is synchronous passes on a quiet machine and FLAKES
  on a loaded one. Wait on the actual state transition (poll with a deadline), never sleep a fixed
  amount. Prove determinism by running the new test many times in parallel under load, not once.
- EXTEND THE SANDBOX SUITE when a review surfaces a live/visual behavior it doesn't cover (match the
  existing S1–S3 Goal/Watch/Verdict shape; keep the coverage guard green in the SAME change —
  `go test ./cmd/lyx/...` includes `sandbox_coverage_test.go`).
- Keep `go build`/`vet`/`test` green after every change. Then RE-DEPLOY (`deploy.cmd`) and re-run
  the smoke + suite scenarios — re-deploying FIRST is mandatory (the suite tests the deployed
  binary, not your working tree).
- Update the `internal/shuttleengine`/`internal/shuttleengine/claudeengine` package documentation
  (and `docs/overview.md` / `CONSTRAINTS.md` if invariants or the module table move) IN THE SAME
  change. Do NOT add bugfix/hardening notes to `docs/roadmap.md` (roadmap is planned milestones
  only, per `CLAUDE.md`).
- Tear down all substrate state; confirm zero stray psmux processes and no leftover shuttle run
  directories. COMMIT each fix as you finish it (see "Commit per fix" above) — do NOT push unless
  the user explicitly asks. Report the changed files and how you verified each fix.

## Deliverables
1. A structured review report (Executive summary with top risks + merge-readiness opinion; Scope
   assessment plan-vs-shipped; Code findings severity-ranked with file:line + scenario + fix +
   CONFIRMED/PLAUSIBLE; Docs & operability findings; What-was-tested with exact commands and
   observed results, including what you could NOT verify and why). Write it to
   `.scratch/shuttle-review-<yourtag>.md`.
2. A fixer report: what you implemented, what you deliberately deferred (with reasons), the exact
   test commands run + results, and the changed files. Write it to
   `.scratch/shuttle-review-<yourtag>-fixer-report.md`.
3. In your final chat message: a concise summary (executive summary + counts by severity + the two
   report paths + an explicit merge-readiness verdict). Do not paste the whole reports.

Begin with the clean-room review (read the SPEC + code + docs, then drive the real substrate),
produce your independent findings, then implement and verify the fixes.
