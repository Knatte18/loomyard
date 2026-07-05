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
2. FIX: after you have a findings list, implement the fixes, verify each against the real
   substrate, keep the whole test suite green, and update the docs in the same change. Do NOT
   commit or push unless the user explicitly tells you to — leave the changes in the working tree
   and report them.

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
not that one exists; their absence is correct, do not flag it. The `review` module (the generic
gate engine that will drive Handler/fixer/cluster-reviewer loops on top of shuttle) and `loom` are
separate, not-yet-built modules — do not review them or expect shuttle to already behave like their
future caller. Multi-agent cluster reviews (N parallel shuttle strands under one review round) are
explicitly future work per `docs/roadmap.md` milestone 10's notes — shuttle only needs to run ONE
agent well.

## Round context seeded from prior-round verification
**Round 1 — no prior round exists yet.** This is the FIRST clean-room review+fix round for
`shuttle` in this hardening loop (mirroring the mux campaign's R3, its first independently-verified
round). There is no CLOSED-AND-VERIFIED history and no deferred-items list to carry forward — find
and fix whatever you find. State the **merge bar** so you calibrate: correctness in the NORMAL
single-instance flow (one shuttle run, one strand, one claude session) is the gate; an N×-concurrent
stress suite (if you choose to run one) is a diagnostic amplifier, not a merge blocker.

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

## Deferred items from the prior round — RE-EVALUATE these (after your own pass)
None yet — this is round 1. (Future rounds: the orchestrator fills this in from what this round
consciously defers.)

## Fixing — after the review
- Load the code-quality guidance (`/code-quality` skill or `mill:code-quality`) before editing.
  Prefer surgical edits; match existing style and the file-level doc-comment convention.
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
  directories. Do NOT commit or push unless the user explicitly asks. Report the changed files and
  how you verified each fix.

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
