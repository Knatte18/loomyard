# `shuttle` — independent review + fix (round 3 — reed×shuttle joint testing)

> Filled instance of `crucible/review-prompt-template.md` for shuttle's campaign, rewritten fresh for round 3 (round 1's version preserved in git history at commit `33a14555`; round 2's at `fa93d9f5`).
> See `crucible/README.md` for the loop this prompt runs inside, and `_mill/reed-shuttle-HANDOFF.md` for campaign-wide state.

You are a senior engineer doing a COMPLETE, adversarial, INDEPENDENT review of the `shuttle` module (`internal/shuttleengine` + `internal/shuttleengine/claudeengine` + `internal/shuttlecli`) in the loomyard repo, followed by FIXING what you find.
Work in the worktree at `/home/knatte/Code/loomyard/wts/reed-shuttle-crucible-hardening` (branch `reed-shuttle-crucible-hardening`).

**This round has a DIFFERENT primary focus than rounds 1-2, per an explicit operator course correction.** The mill-wiki task that created this whole campaign instructed reed and shuttle to be tested AGAINST EACH OTHER — cross-module joint adversarial testing — not each module reviewed in isolation with the other treated as an already-trusted black box. Rounds 1-2 under-weighted this: round 1 had exactly one composition scenario (a reed foreign-session refusal propagating through shuttle's wrapping), round 2 had it as one item among eleven. **This round's mandate is to make the reed↔shuttle boundary the PRIMARY test surface**, not an afterthought — see "High-yield focus" below, which is now organized entirely around it.

## Your two jobs, in order
1. REVIEW: form your own independent judgment of shuttle's correctness AT THE REED BOUNDARY specifically (plus anything else you notice).
   Hunt for bugs by reading the code AND by driving the real substrate (real tmux + a real, logged-in `claude` CLI, both resolved on `PATH`) — this is where the defects hide.
2. FIX: after you have a findings list, implement the fixes one at a time, verify each against the real substrate, keep the whole test suite green, and update the docs in the same change as the fix they document.
   COMMIT after each individual fix lands green (see "Commit per fix" below).
   Do NOT push unless the user explicitly tells you to.

## Commit per fix (BLOCKING — do not batch fixes into one uncommitted diff)
As soon as one finding's fix is implemented, green (`go build`/`vet`/hermetic test, plus the live smoke check if the finding needed one), and its doc update (if any) is included, COMMIT it — on the current branch, no push — before starting the next finding.
Commit message format: `shuttle: fix <finding-id> — <one-line what/why>`.
Also commit `_mill/shuttle-review-r3.md` and `_mill/shuttle-review-r3-fixer-report.md` as you write or update them.

## A LARGE finding is not fixed inline (BLOCKING)
If a finding's fix is LARGE — a genuine subsystem/feature addition, a cross-cutting refactor reaching outside this module, anything that would benefit from its own design/plan step rather than a scoped bugfix — do NOT cram it into Job 2's commit-per-fix loop.
Record it fully in the review report exactly like any other finding (severity, scenario, suggested fix), but mark it explicitly **NOT-FIXED-THIS-ROUND** with the reason.
The orchestrator opens a proper mill-wiki task for it afterward. This is a SIZE line, not a severity line — see `crucible/orchestrator-prompt.md`'s Hard Rule 5. Round 2 already applied this once (R2-F11, a `sendVerified` viewport-scroll defect) — that finding is real, named, and out of THIS round's scope (see "Round context" below); do not re-litigate it, but do not hesitate to name a second one if you find one.

## Sequencing rule (BLOCKING — do not skip, do not interleave)
Job 1 must be COMPLETE — and its full review report SAVED to `_mill/shuttle-review-r3.md` and committed — before you touch (edit, create, or delete) a single production or test file.
Do not fix findings as you go, even ones that look small and obviously right.
(Shuttle's own history includes a round that interleaved review and fix, and a round killed mid-fix with an uncommitted diff — both are exactly what this rule and "Commit per fix" exist to prevent.)

## Log as you go during Job 1 (BLOCKING)
Append your observations to `_mill/shuttle-review-r3.md`'s "What was tested" section immediately after each command/scenario returns.
Jot each finding into the file's findings section provisionally as you spot it.
COMMIT each meaningful append (`shuttle: review notes — <what you just appended>`).

## Clean-room review constraint (do this part unprimed)
Form your OWN findings first.
Do NOT read any prior review or review-dialogue files before you have your own list.
Specifically do not open anything under `_mill/` matching `shuttle-review-*` (rounds 1-2's reports), nor `_mill/reed-shuttle-HANDOFF.md`, until your own findings list is complete.
Reading the design SPEC and the module docs is expected and required. Reading reed's own CURRENT code/`doc.go`/`CONSTRAINTS.md` entries (as opposed to reed's *review history* under `_mill/reed-review-*`, which stays off-limits) is expected and required too — this round is specifically ABOUT the reed↔shuttle boundary, so you must understand reed's current, hardened contract deeply, not just shuttle's.
AFTER your own findings are written, you MAY consult rounds 1-2's material (21 findings, all CLOSED-AND-VERIFIED, plus R2-F11 named-but-not-fixed — see "Round context" below).

## What to read
- Shuttle code: `internal/shuttleengine/**`, `internal/shuttleengine/claudeengine/**`, `internal/shuttlecli/**` — you should already know this well from the two prior rounds' worth of fixes; read the CURRENT state, especially `wait.go` (the `Wait` loop, `checkLivenessTick`, `classifyStartupWindow`), `run.go` (`Start`/`Interrupt`/`Send`/`Inject`, `sweepOrphansOpportunistic`), and `rundir.go`.
- **Reed's CURRENT contract, in depth — this is the new material this round is about.** Read `internal/reedengine/doc.go` in full, plus the specific mechanisms shuttle must compose correctly with:
  - `validateToldTmuxIdentity`/`validateToldAnchorPath` (`server.go`) — the pre-flight refusals reed's own campaign built (rounds 2-4), refusing a told identity before any tmux round trip.
  - The `PaneGeneration` mechanism (`generation.go`) — reed's rounds 5-6 built this to detect stale/foreign bindings across a tmux server rebirth or a worktree rename; `refuseLiveForeignSessionLocked`, `refuseRecordedForeignSessionBeforeBootLocked`.
  - `Down`'s `AbandonedSession` field (reed round 6) — reports rather than kills a live orphan session.
  - `LoadState`'s corrupt/`null`-rejecting behavior and `unreadableStateError` (reed round 5).
  - The header-rebuild retry (`ensureHeaderPaneLocked`, reed round 4) — recovers from a lost `.lyx/reed.json` while a session stays up.
  - `safeReapRoot`/pane-child reaping (reed rounds 1-2) — what happens to a shuttle-launched `claude` process tree specifically when reed reaps it.
- Docs: `docs/overview.md`'s shuttle bullet, `manifest/roadmap.md`, `CONSTRAINTS.md`, `README.md`.
- `tools/sandbox/SANDBOX-SHUTTLE-SUITE.md` — for scenario ideas only.

## Mission (assess on two axes, be adversarial)
1. **Composition correctness — the PRIMARY axis this round.** Does shuttle behave correctly, and report honestly, when reed's OWN (already-correct, already-hardened) defensive behaviors activate WHILE a shuttle run is live — not just before one starts, and not just via one scenario, but systematically across reed's major hardened mechanisms?
2. Correctness elsewhere — anything else you notice, at the usual bar. Also assess docs accuracy and operability.

## High-yield focus — reed×shuttle joint adversarial testing (THIS ROUND'S PRIMARY MANDATE)

The shape every scenario below shares: **start a real shuttle run (`--model haiku`), let it reach a live, in-progress state (not yet finished), THEN deliberately trigger a reed-side event that reed itself now handles correctly (per its own six-round campaign) — and observe what shuttle does with the consequence.** This is different from rounds 1-2's pattern of triggering the reed-side event BEFORE or largely independent of a live run. The live run is the point: these are the conditions under which shuttle's own operator (or an unattended pipeline) actually experiences reed's hardening, mid-flight, not as a pre-flight check.

For each scenario: does `Wait` classify a sane outcome (not hang forever, not silently misclassify)? Does the caller get an actionable error/result, not a bare wrapped Go error? Is the run's identity (guid/sessionId/runDir) preserved per R1-F2's fix? Does nothing get silently destroyed (a live pane, a running process) the way reed's OWN campaign found and fixed within reed — is shuttle now accidentally reintroducing a similar failure at its own layer, one level up?

1. **Worktree rename DURING a live run.** Start a shuttle run. While `Wait` is polling, rename the worktree directory out from under it (same shape as reed's own R5-F5/R6-F1: the old session keeps running, reed's refusal fires on the NEXT reed op against the new name). What does the in-flight `Wait` loop do? What does a concurrent `Interrupt`/`Send` against the same guid, issued from the renamed path, do?
2. **`reed.json` corrupted or truncated DURING a live run** (not before `Start`, as round 2 tested, but while the agent is actively working and shuttle's own polling/orphan-sweep machinery is reading reed state concurrently). Does `Wait`'s liveness polling (which calls into reed) degrade the same way `Interrupt`/`Send` did in round 2's testing, or does the continuous polling loop behave differently under a state that goes bad mid-poll rather than being bad from the start?
3. **The tmux SERVER killed (not just a pane) DURING a live run** — reed's crash/rebirth handling (hardened since reed round 1) activates on the next reed op. Does shuttle's `Wait` loop detect this promptly and classify sanely, or does it hang against a dead server, or misclassify a rebuilt-but-different session as still the same run?
4. **Cross-worktree contamination under REAL concurrent live agent load** — two worktrees under one hub, EACH running a real shuttle agent simultaneously (2 real `claude` processes — within this round's live-substrate budget, see the cost declaration below), then corrupt/copy one's `.lyx` state to point at the other's session (reed's own R5-F4/R6 shape). Does shuttle's layer add any NEW cross-contamination risk beyond what reed itself already correctly refuses, or does it correctly inherit reed's protection? Does an operator watching worktree A see any bleed from worktree B's real agent?
5. **A worktree rename that reed's refusal catches, WHILE `Interrupt`/`Send` (not `Wait`) is the one making the reed call.** Rounds 1-2 mostly drove this through `run`'s own polling; round 3 should specifically test the out-of-process verbs (`interrupt`, `send`, invoked as separate CLI processes against an already-in-flight run) against a reed that is mid-refusal.
6. **`lyx reed down` + `lyx reed up` (a full session rebuild cycle) WHILE a shuttle run believes its pane still exists.** Not a crash — an ordinary operator action reed handles cleanly by design. Does shuttle's in-flight `Wait`/`Interrupt`/`Send` correctly detect the pane identity changed, or does it silently address a wrong/nonexistent pane?
7. **Anything from rounds 1-2's own "what could not be verified" lists that is now newly practical BECAUSE this round's scenarios naturally construct a live run under stress** — e.g. round 2 could not force R2-F9's stuck-trust-prompt or capture-failure paths; if this round's joint-testing scenarios happen to produce a real pane in a state that exercises them, drive it.

Everything you find here should be judged the same way as any other finding — severity, CONFIRMED/PLAUSIBLE, small vs. large. If you find shuttle correctly composing with reed (no defect), that is a valuable, explicit result to record too — do not manufacture a finding to have something to report.

**If a joint scenario reveals what looks like a genuine REED defect** (not a shuttle-side handling gap), name it explicitly as an OUT-OF-CAMPAIGN finding — reed's campaign is closed, you do not fix reed here — but do not suppress it; it may warrant reopening reed at the operator's discretion.

## Secondary focus — anything else, lower priority than the above
- Round 2 named two things it could not verify live: subpath-anchored geometry (this worktree has `AnchorRel = "."`), and R2-F9's two failure paths. If your joint-testing scenarios happen to construct a subpath-anchored fixture or a stuck-startup pane naturally, verify these too; do not go out of your way if it would cost a dedicated separate scenario unrelated to the primary mandate.
- Anything you notice while reading the current code that isn't reed-boundary-related but is a real, small finding — record and fix it, same as always.

## Explicitly OUT of scope for this round
- `internal/reedengine`/`internal/reedcli`/`internal/hubgeom` code changes — CLOSED campaign, do not fix reed. You WILL be deliberately driving reed into its hardened failure/refusal states as the whole point of this round — that is reading and exercising reed, not reviewing or changing it.
- `internal/burlerengine`/`internal/burlercli`, `internal/perchengine`, `internal/loomengine` — built on shuttle, out of scope.
- Windows-specific live driving — this host is Linux.
- **R2-F11** (the `sendVerified` viewport-scroll defect) — already named, NOT-FIXED-THIS-ROUND by round 2, awaiting an operator decision on a separate mill-wiki task. Do not re-review or fix it this round; if your joint-testing scenarios happen to exercise `sendVerified`, note whether you observe its known limitation but do not treat it as new.

## Round context seeded from prior-round verification

**Round 1 (`opus-medium-r1`) and round 2 (`opus-high-r2`) — CLOSED AND INDEPENDENTLY VERIFIED (2026-08-18).** 21 findings total (10 + 11), no BLOCKING across either round, everything small fixed inline except R2-F11 (large, named, not fixed). Full detail in `_mill/shuttle-review-r{1,2}.md` / `-r{1,2}-fixer-report.md`, readable only after your own findings list is complete.

The orchestrator independently re-verified both rounds from a cold state: all gates green matching each round's claims; the large majority of each round's fixes sabotage-proved (reverted, confirmed failure at the intended assertion, restored to empty diff); headline fixes independently live-driven on scenarios distinct from each round's own reproductions; docs spot-checked accurate; teardown clean against the correct process-count baseline each time.

**CLOSED-AND-VERIFIED from round 1, do NOT re-litigate:**
- R1-F1 (MEDIUM) — all smoke tests pin `--model haiku`. Commit `f16eaba7`.
- R1-F2 (MEDIUM) — `Wait`'s mechanism-failure exits preserve the run's identity (guid/sessionId/runDir) instead of discarding it. Commit `342482ed`. **Directly relevant to this round**: every joint-testing scenario above should re-confirm this holds under ITS specific trigger, not just round 1's `lyx reed down` case.
- R1-F3 (MEDIUM) — `NewRunner`'s told anchor/worktree pair is validated (`validateToldPaths`), refusing a swapped/relative/empty pair. Commit `b68cfd21`.
- R1-F4-F10 (LOW/NIT) — fixture fix, message clarity, resume flag threading, stale docs, teardown logging. Commits `e0f5107e`/`fc1dc45f`/`e30f2786`/`7951c86a`/`7d3cd491`/`7fa47ef8`/`6e1cccf3`.

**CLOSED-AND-VERIFIED from round 2, do NOT re-litigate:**
- R2-F9 (MEDIUM) — the startup deadline now binds every not-yet-started exit path, not just one. Commit `e6ea9d05`.
- R2-F3 (MEDIUM) — closed round 1's own regression-test gap for the model pin. Commit `934abdb9`.
- R2-F1/F2/F5/F6/F8 (LOW) — double JSON envelope on abort+bad-flag, fork-audit outcome loss, transcript streaming, viewport-scroll re-baseline (partial — see R2-F11), 5 log sites migrated to `logger.Warn`. Commits `bbe5636b`/`41a70c3d`/`354946c1`/`cdb22bf8`/`ec205fee`.
- R2-F4/F7/F10 (NIT/NIT/LOW) — doc drift on run_dir's anchor, the Agent-deny's real conditions, `asking`'s real meaning. Commit `22461927`.
- **R2-F11 (LOW severity, LARGE size) — NAMED, NOT FIXED, NOT this round's to touch.** A `sendVerified` viewport-scroll edge case that a count alone cannot distinguish from non-delivery. The orchestrator independently confirmed this size judgment is sound (a position-aware check is technically buildable without a reed change, but replacing a live-proven detector with something unvalidated is a real risk warranting its own task, not a rushed inline swap). Awaiting an operator decision on opening it as a separate mill-wiki task — not this round's concern.

**Attribution note:** none of rounds 1-2's findings trace to a wave-1 commit (`b98ee2ba`) in a way that implicates the told-string refactor beyond R1-F3 itself (already fixed). Not a reason to narrow this round's scope.

**Your mission this round:** third round, Opus / Medium (operator's explicit pick — not the originally planned Fable/High rotation slot; the operator chose to prioritize a fresh joint-testing mandate over continuing the model-diversity rotation strictly). This round's job is fundamentally different in KIND from rounds 1-2, not just another lens on the same material — it is the first round to systematically test what this whole campaign was originally commissioned to test. Weight this accordingly: even a modest number of findings here, if they are genuinely new joint-composition defects, matter more than a larger number of round-1/round-2-shaped unhappy-path findings would have. If every joint scenario comes back clean — shuttle correctly inherits reed's hardening with no added risk at its own layer — that is a strong, valuable, and explicitly stateable convergence signal for THIS axis specifically, distinct from whether shuttle-alone correctness has converged.

State the **merge bar**: correctness in the NORMAL single-instance flow is the gate. Scenario 4 (two real concurrent agents) is a deliberate, budgeted exception to shuttle's usual one-agent-at-a-time driving discipline — see the cost declaration below for the exact budget.

## Live-substrate cost declaration (BLOCKING)
`LLM-DRIVING: YES.` This round's joint-testing scenarios need MORE real `claude` processes than rounds 1-2, because several scenarios require a run to be genuinely mid-flight (not just started and immediately probed) when the reed-side event fires.

- **You MUST pass `--model haiku` (or `Spec.Model: "haiku"`) for every real `claude` process — no exceptions, same standing operator instruction as every round.**
- The four existing smoke tests remain one-real-`claude`-process-per-invocation; run each by exact name if you touch them.
- **New budget for this round's joint scenarios**: up to ONE real `claude` process per scenario in the "High-yield focus" list above (7 scenarios named, so a ceiling of roughly 7-8 additional real processes across the whole review, not counting smoke-test re-runs), EXCEPT scenario 4, which is explicitly authorized for exactly TWO simultaneous real `claude` processes (one per worktree) — this is the one deliberate exception to "never run more than one live-substrate command at a time" in this campaign, and it is capped at 2, not open-ended. Do not run more than 2 simultaneous real `claude` processes under any circumstance this round.
- Prefer a long-running prompt (e.g. `sleep 60` then write a file) over a fast one for scenarios that need a genuine mid-flight window — you need real wall-clock time for the reed-side trigger to land while the run is still polling, not already finished.
- Still no N×-concurrent SMOKE-SUITE sweep — that is a different thing from this round's deliberately small, budgeted joint-scenario concurrency.

## What to TEST — do not just read, EXERCISE it
Report the exact commands you ran and what you observed.

Hermetic (must stay green throughout):
- `go build ./...`
- `go vet ./internal/shuttleengine/... ./internal/shuttleengine/claudeengine/... ./internal/shuttlecli/...`
- `go vet -tags smoke ./internal/shuttlecli/...`
- `go test -count=5 ./internal/shuttleengine/... ./internal/shuttleengine/claudeengine/... ./internal/shuttlecli/... ./cmd/lyx/...`

Live smoke: `go test -tags smoke ./internal/shuttlecli/... -run <ExactTestName> -v -count=1` — one at a time, by exact name, only if you touch smoke-tested code.

Live driving — YOU drive it directly (PRIMARY — this round's actual work):
- Deploy first: `./deploy-dev`. Re-run after EVERY source change.
- Do NOT invoke `sandbox-shuttle-suite.cmd`. Run real `lyx shuttle`/`lyx reed` commands yourself, foreground, interleaved as each scenario requires (a background `lyx shuttle run` you then act against with a second-process `lyx reed <verb>` or `lyx shuttle interrupt/send`, per the scenarios above).
- Record baseline `claude`/tmux process counts BEFORE driving anything, same discipline as round 2 — your teardown check is judged against that baseline, not zero.

TEARDOWN DISCIPLINE (critical): confirm ZERO NEW stray tmux processes (`ps -eo comm | grep -cx 'tmux: server'` = 0 — NOT `pgrep -x tmux` alone) AND zero new stray `claude` processes beyond your recorded baseline, after EVERY scenario and again at the end. This round deliberately runs more real processes than usual — be more careful about teardown, not less.

## How to judge each finding
`file:line`, concrete failure scenario, severity (BLOCKING/MEDIUM/LOW/NIT), suggested fix, CONFIRMED/PLAUSIBLE, and SIZE (small = fix inline; large = name it, mark NOT-FIXED-THIS-ROUND).
For scope: plan-promised vs shipped.

**Severity affects how you report; SIZE affects whether you fix inline this round.** Every small finding, all severities including NIT, gets fixed in Job 2.

## Deferred items — RE-EVALUATE
None deferred from rounds 1-2 (R2-F11 is named-not-fixed, not deferred — it stays out of this round's scope per above).

## Fixing — after the review
- Fix EVERY small finding, all severities including NIT. Name, don't fix, anything genuinely LARGE.
- Load `/code-quality` AND `golang:golang-build`/`golang:golang-testing`/`golang:golang-comments` before editing.
- For every bug you fix, add or extend a test. For a live-only defect, add a `//go:build smoke` test if practical — several of this round's scenarios may be hard to make deterministic/hermetic; say so explicitly if a finding's regression test has to stay a documented manual repro rather than an automated one, and explain why.
- MAKE SMOKE TESTS DETERMINISTIC — poll on actual state transitions, never sleep a fixed amount.
- Extend `tools/sandbox/SANDBOX-SHUTTLE-SUITE.md` if a review surfaces a live/visual behavior it doesn't cover (keep `sandbox_coverage_test.go` green).
- Keep `go build`/`vet`/`test` green after every change. RE-DEPLOY and re-run every live scenario yourself.
- Update `docs/overview.md` / `internal/shuttleengine/doc.go` / `CONSTRAINTS.md` IN THE SAME change if invariants or scope move. Do NOT add hardening notes to `manifest/roadmap.md`.
- Tear down all tmux/claude state; confirm zero new stray processes. COMMIT each fix as you finish it. Do NOT push.

## Deliverables
1. Structured review report → `_mill/shuttle-review-r3.md`, committed incrementally.
2. Fixer report → `_mill/shuttle-review-r3-fixer-report.md`, committed.
3. Final chat message: concise executive summary + counts by severity + any NOT-FIXED-THIS-ROUND large findings + the two report paths + explicit merge-readiness verdict + your own convergence assessment, distinguishing (if relevant) "shuttle-alone correctness" convergence from "reed×shuttle joint-composition" convergence — this round is specifically evidence for the second axis. Do not paste the whole reports.

Begin with the clean-room review (read the SPEC + shuttle code + reed's current contract in depth, then drive the real substrate with the joint scenarios as your primary work), produce your independent findings, then implement and verify the fixes.
