# `shuttle` — independent review + fix (round 2)

> Filled instance of `crucible/review-prompt-template.md` for shuttle's campaign, rewritten fresh for round 2 (round 1's version preserved in git history at commit `33a14555`).
> See `crucible/README.md` for the loop this prompt runs inside, and `_mill/reed-shuttle-HANDOFF.md` for campaign-wide state.

You are a senior engineer doing a COMPLETE, adversarial, INDEPENDENT review of the `shuttle` module (`internal/shuttleengine` + `internal/shuttleengine/claudeengine` + `internal/shuttlecli`) in the loomyard repo, followed by FIXING what you find.
Work in the worktree at `/home/knatte/Code/loomyard/wts/reed-shuttle-crucible-hardening` (branch `reed-shuttle-crucible-hardening`).

## Your two jobs, in order
1. REVIEW: form your own independent judgment of shuttle's scope and correctness.
   Hunt for bugs by reading the code AND by driving the real substrate (real tmux + a real, logged-in `claude` CLI, both resolved on `PATH`) — this is where the defects hide.
2. FIX: after you have a findings list, implement the fixes one at a time, verify each against the real substrate, keep the whole test suite green, and update the docs in the same change as the fix they document.
   COMMIT after each individual fix lands green (see "Commit per fix" below).
   Do NOT push unless the user explicitly tells you to.

## Commit per fix (BLOCKING — do not batch fixes into one uncommitted diff)
As soon as one finding's fix is implemented, green (`go build`/`vet`/hermetic test, plus the live smoke check if the finding needed one), and its doc update (if any) is included, COMMIT it — on the current branch, no push — before starting the next finding.
Commit message format: `shuttle: fix <finding-id> — <one-line what/why>`.
Also commit `_mill/shuttle-review-r2.md` and `_mill/shuttle-review-r2-fixer-report.md` as you write or update them — they are NOT gitignored scratch, they are the campaign's durable record.

## A LARGE finding is not fixed inline (BLOCKING — new this round, read carefully)
If a finding's fix is LARGE — a genuine subsystem/feature addition, a cross-cutting refactor reaching outside this module, anything that would benefit from its own design/plan step rather than a scoped bugfix — do NOT cram it into Job 2's commit-per-fix loop.
Record it fully in the review report exactly like any other finding (severity, scenario, suggested fix), but mark it explicitly **NOT-FIXED-THIS-ROUND** with the reason ("too large for an inline crucible fix — needs its own mill-wiki task").
The orchestrator will open a proper mill-wiki task for it afterward.
This is a SIZE line, not a severity line — a NIT is still small and still gets fixed inline; a large finding does not get fixed inline no matter its severity label. See `crucible/orchestrator-prompt.md`'s Hard Rule 5 for the full rationale.
When genuinely unsure whether something crosses this line, say so explicitly in the finding rather than guessing either way — record your uncertainty, don't silently pick the easier path.

## Sequencing rule (BLOCKING — do not skip, do not interleave)
Job 1 must be COMPLETE — and its full review report SAVED to `_mill/shuttle-review-r2.md` and committed — before you touch (edit, create, or delete) a single production or test file.
Do not fix findings as you go, even ones that look small and obviously right.
Write it down as a finding, keep reading, finish the review, save the file, THEN start Job 2.
(Shuttle's own round 1 history — see "Round context" below — includes both a round that interleaved review and fix, and a round killed mid-fix with an uncommitted diff. Both are exactly what this rule and "Commit per fix" exist to prevent.)

## Log as you go during Job 1 (BLOCKING)
Append your observations to `_mill/shuttle-review-r2.md`'s "What was tested" section immediately after each command/scenario returns.
Jot each finding into the file's findings section provisionally as you spot it.
COMMIT each meaningful append (`shuttle: review notes — <what you just appended>`).

## Clean-room review constraint (do this part unprimed)
Form your OWN findings first.
Do NOT read any prior review or review-dialogue files before you have your own list.
Specifically do not open anything under `_mill/` matching `shuttle-review-*` — this is a FILENAME PATTERN, not a content judgment, so it covers round 1's review report and fixer report (`shuttle-review-r1.md`, `-r1-fixer-report.md`), AND the orchestrator's own running handoff note (`_mill/reed-shuttle-HANDOFF.md`) even though its name doesn't match the pattern literally — treat it as equally off-limits until your own findings list is complete. Do not open any of these out of curiosity, and do not act on anything you might glimpse in them even by accident.
Reading the design SPEC and the module docs is expected and required (those are not reviews). Reading reed's own current code/`doc.go`/`CONSTRAINTS.md` entries (as opposed to reed's *review history* under `_mill/reed-review-*`, which stays off-limits the same way) is expected and required too — shuttle is built directly on reed's API.
AFTER your own findings are written, you MAY consult round 1's material (10 findings, all CLOSED-AND-VERIFIED — see "Round context" below for exact commit shas) to confirm previously-fixed behaviors have not regressed, and to re-evaluate the one open item it left named (see below).

## What to read
- Code: `internal/shuttleengine/**` (`run.go`'s `Runner`/`Start`/`Run`/`Interrupt`/`Send`/`Inject` — now also `validateToldPaths` and `(*Run).identity()`, both new this campaign — `rundir.go`'s run-directory lifecycle + orphan sweep, `wait.go`'s completion-classification loop + fork audit + teardown logging, `spec.go`'s validation, `config.go`, `forkaudit.go`'s value types, `posix.go`'s Windows-to-POSIX path helper), `internal/shuttleengine/claudeengine/**` (`command.go`'s `buildLaunchCmd`/`buildResumeCmd`, `settings.go`'s guardrail hooks), `internal/shuttlecli/**` (`cli.go`'s wiring, `run.go`/`interrupt.go`/`send.go`), `cmd/lyx` integration.
- Round 1's fixes, to understand the CURRENT shape before hunting for what it missed: `git show 33a14555..HEAD -- internal/shuttleengine internal/shuttlecli` (everything since round 1's seed) — read it in full.
- Docs: `docs/overview.md`'s shuttle bullet (~line 282), `manifest/roadmap.md`, `CONSTRAINTS.md` — especially the **Shuttle Provider-Seam Invariant**, **Durable-vs-Ephemeral State Invariant**, and **Live-Substrate Spawn Observability** (round 1's R1-F10 made shuttle comply with this — confirm it stays honored by anything you touch), `README.md`.
- Reed's CURRENT contract (`internal/reedengine/doc.go`) — shuttle drives reed through `ReedOps`, and round 1 confirmed composition with reed's hardened refusal/error shapes is currently correct (L12 in round 1's review). Re-confirm this hasn't drifted if you touch anything on that path.
- Repo rules you MUST follow: `CLAUDE.md` (root + `~/.claude/CLAUDE.md`) and `CONSTRAINTS.md`.
- `tools/sandbox/SANDBOX-SHUTTLE-SUITE.md` — for SCENARIO IDEAS only.

## Mission (assess on two axes, be adversarial)
1. Scope — anything round 1 missed in confirming shipped-vs-plan-promised?
2. Correctness — bugs, races, error handling, edge cases; concentrate on the areas below (a second, fresh lens on the same module, plus the specific residual round 1 named). Also assess docs accuracy and operability.

## High-yield focus — a second independent lens on shuttle (drive these, do not just read them)
Round 1 found real, fixed defects in the told-string construction seam and in error/identity handling under a reed failure. This round should try, with fresh eyes, to find what round 1's OWN method could not reach.

- **Close R1-F1's regression-test gap, if you agree it's real.** Round 1 pinned all 4 smoke tests to `haiku` via a shared `smokeClaudeModel` constant, but added NO test that would catch a future call site regressing back to the account default — the fix is enforced only by the constant existing and being referenced, confirmed by grep, not by a failing test. Decide independently whether this is worth a hermetic test (e.g. asserting each `RunCLI`/`Spec` construction site actually threads the constant, without needing to spawn a real process to prove it) and close it if so.
- **`validateToldPaths`'s edges, one round of scrutiny later.** Round 1's swap detector is `anchorPath` equal-to-or-inside `worktreeRoot`. Is there a real hub-geometry shape where this is too strict (a legitimate configuration it would now wrongly refuse) or too loose (a swap or misconfiguration it would NOT catch)? Drive it against every `hubgeom.ReedGeometry` shape you can construct, not just the ones round 1's own tests cover.
- **The `Wait`/reed-failure boundary, one layer deeper.** Round 1 fixed identity-loss on reed's mechanism failures but explicitly left "Wait cannot distinguish 'reed is gone' from 'reed refuses'" as a named, accepted residual (blocked on a reed change, out of campaign). Is there anything shuttle itself CAN do with today's reed API to narrow this without needing reed to grow a sentinel — e.g. a distinguishing retry/probe pattern, or is round 1's assessment that this is genuinely unclosable from shuttle's side correct? If you find shuttle-side room to improve it, that's a finding; if you confirm round 1's assessment, say so explicitly rather than silently re-agreeing.
- **Interrupt/Send/Inject races.** Round 1 drove `Interrupt`-then-`Send` as a sequential scenario. Try genuine races: `Send` and `Interrupt` issued near-simultaneously from two processes against the same guid; `Inject` racing a `Wait` goroutine's own poll; a `Start` whose `AddStrand` succeeds but whose `SaveRunState` fails concurrently with an `Interrupt` attempt on the not-yet-fully-persisted run.
- **Orphan sweep edge cases beyond round 1's.** Round 1 confirmed the sweep correctly ages out a genuinely orphaned dir and leaves fresh ones alone, and separately confirmed (as an observation, not a finding — it's inherent to the sweep's contract) that a foreign/corrupted `reed.json` can cause it to sweep kept diagnosis dirs prematurely. Are there other edge cases: a `run.json` for a strand reed is still tracking but as a DIFFERENT run (guid reuse across a crash/restart), a sweep racing a fresh `Start` for the same guid, the age threshold's behavior right at its boundary?
- **`ForkAudit`/`ForkReport` correctness under a genuinely large or malformed transcript.** Round 1 didn't drive this live in depth. If `ForkSubagents: true` and the resulting transcript is unusually large, malformed, or the session ended abnormally, does `AuditForks` degrade gracefully or does it error/hang/miscount?
- **Everything round 1 fixed, re-verified once more under YOUR OWN fresh scenarios** — not round 1's exact reproductions. Confirm no regression on: the model pin (R1-F1), identity-on-failure (R1-F2), told-path validation (R1-F3), the unreadable-run.json message (R1-F5), the untracked-strand message (R1-F6), resume flag threading (R1-F7), and teardown logging (R1-F10).

## Explicitly OUT of scope for this round
- `internal/reedengine`/`internal/reedcli`/`internal/hubgeom` — CLOSED campaign. Read reed's current contract as context; do not re-review or fix reed itself. A genuine reed defect found while reviewing shuttle gets named as an out-of-campaign finding, not fixed here.
- `internal/burlerengine`/`internal/burlercli`, `internal/perchengine`, `internal/loomengine` — built on shuttle, out of scope.
- Windows-specific live driving — this host is Linux; `posix.go`'s Windows gap is named, not driven (round 1 already read it for correctness).
- Non-Claude engines.

## Round context seeded from prior-round verification

**Round 1 (`opus-medium-r1`) — CLOSED AND INDEPENDENTLY VERIFIED (2026-08-18).** 10 findings (3 MEDIUM, 5 LOW, 2 NIT), all fixed, none deferred, none large enough to need a separate mill-wiki task. The orchestrator independently re-verified from a cold state: all gates green; 9 of 10 fixes sabotage-proved (each reverted, each failed at its intended assertion, restored to empty diff); R1-F2 and R1-F3 independently live-driven on a fresh scenario (different prompt/timing than round 1's own) — both held; docs spot-checked accurate; teardown clean (zero tmux, zero stray `claude`).

**CLOSED-AND-VERIFIED, do NOT re-litigate:**
- R1-F1 (MEDIUM) — all 4 smoke tests now pin `--model haiku`/`Spec.Model: "haiku"` via a shared `smokeClaudeModel` constant. Commit `f16eaba7`. **Exception: this is the one item with NO automated regression test** (confirmed by the orchestrator's verification — only grep-confirmable wiring). Named above as this round's first high-yield item; close it if you agree it's worth a test, or explain why not.
- R1-F2 (MEDIUM) — `Wait`'s three mechanism-failure exits now return the run's identity (guid/sessionId/runDir) alongside the error. Commit `342482ed`. Independently live-driven, holds.
- R1-F3 (MEDIUM) — `NewRunner`'s told `anchorPath`/`worktreeRoot` pair is now validated (`validateToldPaths`): non-empty, absolute, anchor inside-or-equal worktree root. Commit `b68cfd21`. Independently live-driven (production wiring) + sabotage-proved (refusal half), holds.
- R1-F4 (LOW) — a test fixture that defeated the swap-detection guarantee now uses a real subpath anchor. Commit `e0f5107e`.
- R1-F5 (LOW) — an unreadable `run.json` now names itself in the not-found error rather than reading as "never existed". Commit `fc1dc45f`.
- R1-F6 (LOW) — untracked-strand message states the fact and names alternatives instead of asserting one cause. Commit `e30f2786`.
- R1-F7 (LOW, PLAUSIBLE) — `buildResumeCmd` now threads `model`/`effort`/`interactive`. Commit `7951c86a`. Not live-driven by round 1 (would need a real resume-triggering scenario) — if you have time, consider whether a live repro is practical this round.
- R1-F10 (LOW) — teardown now logs through `internal/logger` (spawn AND teardown, per CONSTRAINTS.md's Live-Substrate Spawn Observability). Commit `6e1cccf3`.
- R1-F8, R1-F9 (NIT) — stale package doc, incorrect fork-hook comments, both corrected. Commits `7d3cd491`, `7fa47ef8`.

**Named residual, explicitly NOT a finding to re-litigate as a defect, but worth probing per this round's high-yield list:** `Wait` cannot distinguish "reed is gone" from "reed refuses" — blocked on reed exposing a typed sentinel it does not have today, which is a reed change and stays out of this campaign. Round 1 assessed this as unclosable from shuttle's side; this round is asked to independently confirm or refute that assessment.

**Your mission this round:** second round of the campaign's planned rotation — Opus / High (higher effort than round 1, a fresh independent lens). Given shuttle's post-wave-1 surface proved shallower than reed's in round 1 (no BLOCKING, no defect class remotely like reed's tmux-identity or state-loss families), this round may plausibly find little — that is a valuable, expected outcome if genuinely earned through real driving, not something to manufacture findings to avoid. If you find nothing new, or only trivial issues, say so plainly and give your own honest convergence assessment. If you find something real, fix it with the same rigor as round 1 (or name it explicitly NOT-FIXED-THIS-ROUND per the large-finding rule above, if it's genuinely too large for an inline fix).

State the **merge bar**: correctness in the NORMAL single-instance flow is the gate. No N×-concurrent sweep against this module (real `claude` subprocess cost, no operator sign-off).

## Live-substrate cost declaration (BLOCKING)
`LLM-DRIVING: YES.` Every `//go:build smoke` test in `internal/shuttlecli` spawns a real `claude` subprocess — unavoidable and expected.

The four smoke test functions, each spawning exactly ONE real `claude` process per invocation (no fan/cluster shape exists in this module):
- `TestSmokeShuttleRunWritesOutputAndCleans` (`smoke_run_test.go`)
- `TestSmokeInterruptSendContinues` (`smoke_interrupt_test.go`)
- `TestSmokeGuardrailDeniesAgentTool` (`smoke_guardrail_test.go`)
- `TestSmokeGuardrailAskingSurfacesQuestion` (`smoke_guardrail_test.go`)

If you add a new smoke test, it must keep this one-real-`claude`-process-per-invocation shape unless a specific finding genuinely requires more (name the exception explicitly and get the orchestrator's sign-off before running it more than once).

- **You MUST pass `--model haiku` (or `Spec.Model: "haiku"`) for every real `claude` process — every smoke test's own launch AND any ad-hoc `lyx shuttle run` you drive yourself — this is an explicit operator instruction for this whole campaign, not a suggestion.** All four smoke tests already do this (round 1's R1-F1 fix) — confirm this stays true for anything you add or modify.
- Run each smoke test by exact name (`-run <ExactTestName>`), never a bare `-run Smoke` sweep.
- Never run more than one live-substrate (`-tags smoke`) invocation at a time, in parallel, or backgrounded.
- Do NOT run an N×-concurrent sweep against this module.

## What to TEST — do not just read, EXERCISE it
Report the exact commands you ran and what you observed.

Hermetic (must stay green throughout):
- `go build ./...`
- `go vet ./internal/shuttleengine/... ./internal/shuttleengine/claudeengine/... ./internal/shuttlecli/...`
- `go vet -tags smoke ./internal/shuttlecli/...` (the smoke files aren't covered by untagged vet)
- `go test -count=5 ./internal/shuttleengine/... ./internal/shuttleengine/claudeengine/... ./internal/shuttlecli/... ./cmd/lyx/...`

Live smoke (real substrate, behind the `smoke` build tag):
- `go test -tags smoke ./internal/shuttlecli/... -run <ExactTestName> -v -count=1` — one at a time, by exact name.
- Confirm substrate present FIRST: real `claude` (logged in) and real tmux on `PATH`.

Live driving — YOU drive it directly, no launcher (PRIMARY — where the bugs surface):
- Deploy the current source as the dev binary: `./deploy-dev` (POSIX script at repo root). **FOOTGUN:** re-run after EVERY source change.
- Do NOT invoke `sandbox-shuttle-suite.cmd`. Run the real `lyx shuttle run|interrupt|send` commands yourself, directly, foreground, waiting for each to return.
- The list above is a FLOOR — devise more adversarial scenarios, especially races and edge cases round 1 didn't have time for.
- "Headless" means "no human required", not "no time/token cost to me." A real `claude` scenario takes real wall-clock minutes — expected and budgeted for. Only a genuine environment gap (no `claude` on PATH, not logged in — check FIRST) is a legitimate skip.

TEARDOWN DISCIPLINE (critical): confirm ZERO stray tmux processes (`ps -eo comm | grep -cx 'tmux: server'` = 0 — NOT `pgrep -x tmux` alone, which falsely reads clean while a server runs) AND zero stray `claude` processes at the end. Leave no stray state. Be honest about what you could NOT verify and why.

## How to judge each finding
For each code finding give: `file:line`, a concrete failure scenario (inputs/state → wrong behavior), severity (BLOCKING / MEDIUM / LOW / NIT), suggested fix, and CONFIRMED (reproduced/traced) vs PLAUSIBLE (looks wrong, unverified). Also judge SIZE per the new rule above: small (fix inline) vs large (name it, mark NOT-FIXED-THIS-ROUND, let the orchestrator spin up a mill-wiki task).
For scope: plan-promised vs shipped; flag deferred-that-should-be-v1 and shipped-beyond-scope.

**Severity affects how you REPORT a finding, not whether you fix it — but SIZE can.** Every small finding, all severities including NIT, gets fixed in Job 2. A genuinely LARGE finding gets named and left unfixed this round, per the rule above — that is not the same as "deferred because low priority," which remains disallowed.

## Deferred items from the prior round — RE-EVALUATE these
None recorded as deferred from round 1 (it deferred nothing). The one NAMED residual to independently assess is the `Wait` reed-sentinel gap described above — not a deferred finding, a named observation round 1 asks this round to check.

## Fixing — after the review
- Fix EVERY small finding from your review, all severities including NIT. Name, don't fix, anything genuinely LARGE (see the rule above).
- Load the code-quality guidance (`/code-quality` skill) AND `golang:golang-build`/`golang:golang-testing`/`golang:golang-comments` before editing — ALL of them, not code-quality alone.
- For every bug you fix, add or extend a test that would have caught it. For a live-only defect, add a `//go:build smoke` test, keeping the one-real-`claude`-process-per-invocation discipline.
- MAKE SMOKE TESTS DETERMINISTIC — poll on actual state transitions with a deadline, never sleep a fixed amount.
- If `tools/sandbox/SANDBOX-SHUTTLE-SUITE.md` needs extending, extend it (keep `sandbox_coverage_test.go` green). Otherwise note the new scenario in your fixer report.
- Keep `go build`/`vet`/`test` green after every change. Then RE-DEPLOY and re-run every live scenario yourself.
- Update `docs/overview.md` / `internal/shuttleengine/doc.go` / `CONSTRAINTS.md` IN THE SAME change if invariants or scope move. Do NOT add bugfix/hardening notes to `manifest/roadmap.md`.
- Tear down all tmux/claude state; confirm zero stray processes. COMMIT each fix as you finish it. Do NOT push.
- Report the changed files and how you verified each fix.

## Deliverables
1. Structured review report → `_mill/shuttle-review-r2.md`, committed incrementally per "Log as you go" above.
2. Fixer report → `_mill/shuttle-review-r2-fixer-report.md`, committed (folding into a fix commit is fine).
3. Final chat message: concise executive summary + counts by severity + any NOT-FIXED-THIS-ROUND large findings called out explicitly + the two report paths + an explicit merge-readiness verdict + your own convergence assessment. Do not paste the whole reports.

Begin with the clean-room review (read the SPEC + code + docs + round 1's diff, then drive the real substrate), produce your independent findings, then implement and verify the fixes.
