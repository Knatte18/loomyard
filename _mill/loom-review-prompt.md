# `loom` — independent review + fix (prompt template) — ROUND 8 (glyph-hardening campaign) — SAFETY PASS, PROVIDER-STARTUP SEAM PRIORITY

> Filled instance of `crucible/review-prompt-template.md` for round 8 of the campaign scoped to the quarry-glyph-plan-alphabet surface (GitHub PR #230), the standalone-webster fix (`#004`) it spun off, and the `unify-webster-burler-wiring` consolidation (`7e54cc280`, `internal/cliwire`) round 7 gave its first-ever review. See [../../crucible/README.md](../../crucible/README.md) for the loop, [../../_mill/loom-crucible-orchestrator-kickoff.md](loom-crucible-orchestrator-kickoff.md) for the campaign charter, and [loom-review-HANDOFF.md](loom-review-HANDOFF.md) for the campaign's full running state (read the handoff only AFTER you have your own independent findings list — see "Clean-room review constraint" below).
>
> **This round exists because round 7 was NOT a clean safety pass either.** Round 7 (Fable/high) found and fixed 1 more genuine BLOCKING bug (F1) — inside the exact code round 6 shipped to fix ITS OWN BLOCKING finding (R6-1), which was itself inside the exact code round 5 shipped to fix ITS OWN two BLOCKING findings (R5-2/R5-7). **Rounds 4, 5, 6, and 7 now form an unbroken run of FOUR consecutive rounds each finding real BLOCKING material** — the campaign's own stated bar for convergence ("a safety pass finding nothing severe") has not been met once in the last four rounds, and three of those four (5, 6, 7) are the SAME seam, each fix narrowing but not closing the hole its predecessor left. Round 7's own read, worth taking seriously rather than either dismissing or over-weighting: each iteration IS a strict narrowing (R6-1's whole-capture matching → R7-F1's missing-adjacency requirement), and for the first time in this four-round run, everything OUTSIDE that one seam — including the brand-new `cliwire` merge round 7 reviewed for the first time — came back clean under genuine adversarial pressure from an independent model.
>
> **Effort note:** this round runs on **Sonnet/xhigh** — the operator's own explicit, deliberate choice. This is the campaign's FIRST use of the `xhigh` effort tier on Sonnet (round 2, `sonnet-xhigh-r2`, was Sonnet at xhigh too — its only prior deployment, and it found 0 BLOCKING, which the operator explicitly flagged afterward as weaker convergence evidence than the same result from a more capable model, since round 2's own territory had never been re-examined adversarially by a stronger model at the time). Rounds 3–7 have since re-covered round 2's territory repeatedly without incident, so that specific concern is largely addressed by now — this round's value is a genuinely fresh model's eyes on the CURRENT state of the provider-startup seam specifically, which no Sonnet pass has ever looked at (round 2 predates the seam's very existence — it was opened by round 5). **Given that framing, and that F1's fix was never confirmed against a real live claude pane by round 7 or by this orchestrator's verification of it, this round should make a genuine, serious attempt at live driving the startup seam on a fresh fixture** — not as one option among several, but as this round's single highest-priority scenario, budget and attempt it before anything else in "What to TEST."

You are a senior engineer doing a COMPLETE, adversarial, INDEPENDENT review of the `loom` module (including the standalone-webster material `#004` and the `unify-webster-burler-wiring` consolidation, both merged into this same branch) in the loomyard repo, followed by FIXING what you find.

## Where to work — determine this yourself, do not assume a path
Work in the current git worktree. Confirm it yourself at the start (`git rev-parse --show-toplevel`, `git branch --show-current`) rather than trusting any hardcoded path in this file or in a prior round's report — this campaign has already run across multiple host/user environments. The branch is `crucible-loom-glyph-hardening`. If a live-driving fixture (a sandbox hub) is needed, do not assume one from a prior round still exists on disk where its own report says it does — check first (`gh repo view Knatte18/lyx-test`, then look for a reusable branch).

**Durable method lesson from round 5, BLOCKING for any live scenario you drive this round:** a fixture whose repository path has EVER hosted an interactive `claude` session (including one you yourself drove earlier in this same round, or any prior round's `r4-*`/`r5sandbox*`/`r7-*` fixture) is permanently immunized against the whole class of defect round 5 found — claude's one-time startup gates simply do not render there again. If you attempt ANY live scenario that touches Master/fork startup, you MUST build it from a repository path claude has never seen (a brand-new `lyx fabric clone` + `lyx fabric add`, or a brand-new standalone target directory) — reusing any prior round's fixture for that purpose proves nothing. This is doubly important this round since the startup seam IS the priority scenario.

## Your two jobs, in order
1. REVIEW: form your own independent judgment of the whole surface's correctness — not just the residuals prior rounds named, but a genuine fresh pass over everything, including territory prior rounds already "closed."
   Hunt for bugs by reading the code AND by driving the real substrate (real tmux via `reed`, real interactive `claude` sessions via `shuttle`/`burler`/`webster`).
2. FIX: after you have a findings list, implement the fixes one at a time, verify each against the real substrate, keep the whole test suite green, and update the docs in the same change as the fix they document.
   COMMIT after each individual fix lands green (see "Commit per fix" below).
   Do NOT push unless the user explicitly tells you to.

## Commit per fix (BLOCKING — do not batch fixes into one uncommitted diff)
As soon as one finding's fix is implemented, green (`go build`/`vet`/hermetic test, plus the live smoke/suite check if the finding needed one),
and its doc update (if any) is included, COMMIT it — on the current branch, no push — before starting the next finding.
Commit message format: `loom: fix <finding-id> — <one-line what/why>`.
Also commit `_mill/loom-review-<yourtag>.md` and `_mill/loom-review-<yourtag>-fixer-report.md` as you write or update them, and **fill in every row of the fixer report's own table before you consider Job 2 done** — round 4's table shipped with 7 real, committed fixes missing from it, caught only by the orchestrator's own independent audit. Don't repeat that.

## Sequencing rule (BLOCKING — do not skip, do not interleave)
Job 1 must be COMPLETE — and its full review report SAVED to `_mill/loom-review-<yourtag>.md` and committed — before you touch (edit, create, or delete) a single production or test file.
Do not fix findings as you go, even ones that look small and obviously right.

## Log as you go during Job 1 (BLOCKING — crash-resilience, do not batch it all to the end)
As you work through "What to TEST" below, APPEND your observations to `_mill/loom-review-<yourtag>.md`'s "What was tested" section immediately after each command/scenario returns, and jot findings provisionally as you spot them.
**COMMIT each append.** This round has a priority real-LLM live-driving scenario (see "High-yield focus" below) — incremental commits matter as much as ever.

## Clean-room review constraint (do this part unprimed)
Form your OWN findings first.
Do NOT read any prior review or review-dialogue files before you have your own list — specifically do not open anything under `_mill/` matching `loom-review-*` in THIS worktree, including all seven prior rounds' review/fixer reports AND this campaign's running `loom-review-HANDOFF.md`. This is a FILENAME PATTERN, not a content judgment.
AFTER you have your own independent findings, you MAY (and should) consult the prior rounds' material and the handoff to (a) confirm the prior fixes have not regressed and (b) understand this round's specific mission below.
Reading the design SPEC and the module docs is expected and required (those are not reviews).

## What to read
- Code — loom's own machinery: `internal/loomengine/**`, `internal/loomcli/**`, `internal/loomrecipe/**`, `internal/loomshed/**`, `internal/shedengine/**`, `internal/shedadapters/**`, `internal/shedrecipe/**`, `internal/shedbuild/**`, `internal/hubgeom/**`, `contracts/recipes/loom-recipe.yaml`, `cmd/lyx`'s loom integration.
- The glyph surface: `internal/planparser/**`, `internal/planglyph/**` (seven rounds of fixes now — read the CURRENT state, not any prior round's description of it), `internal/websterengine/**` (`beginbatch.go`, `recordbatch.go`, `fingerprint.go`, `render.go`, `runlevel.go` all changed across rounds).
- The standalone-webster material (`#004`, hardened further by rounds 3 and 4): `internal/shuttleengine/run.go` (`NewRunner`/`NewDetachedRunner`), `internal/standalonegeom/**`, `internal/standalonestate/**`, `internal/logger/sink.go`.
- **The `unify-webster-burler-wiring` consolidation (`7e54cc280`, `internal/cliwire`) — round 7 gave it its first review and found it sound plus 2 real gaps (F2, plus 2 enforcement-test blind spots F3/F4), all fixed. This round is its SECOND review, by a different model — read it adversarially again, do not assume round 7 caught everything just because it was thorough:** `internal/cliwire/module.go`, `internal/cliwire/paths.go`, `internal/cliwire/standalone.go`, `internal/cliwire/doc.go`, `internal/cliwire/cliwire_test.go`, `internal/cliwire/bannedecl_enforcement_test.go`, `internal/cliwire/callerset_enforcement_test.go`, plus `internal/webstercli/wiring.go` and `internal/burlercli/wiring.go` (both delegate into `cliwire.Module`/`ResolveStandalone`). `CONSTRAINTS.md`'s "Cliwire Sole-Wiring Invariant" states the contract; read the live file for its current text.
- **The provider-startup seam — THIS ROUND'S TOP PRIORITY. Three consecutive rounds (5, 6, 7) have found BLOCKING material here, each time inside the exact code the PRIOR round shipped as the fix. Read it as if you expect a fourth:** `internal/shuttleengine/claudeengine/startup.go` (`Startup`, `TrustDismissSequence`, `gateIsRendered`, `isGateAcceptOptionLine`, `locateGateLines`, `acceptLineIsGateOption`, `startupGateNeedles`, `gateAcceptNeedles`), `internal/shuttleengine/claudeengine/doc.go`, `internal/shuttleengine/wait.go`, `internal/shuttleengine/engine.go`. The current rule (post round-7 fix): a gate needs a needle PLUS an evidence line (accepting-option line or gate footer) that is ADJACENT to the last caret (accept line ≤1 line away, footer ≤4 lines below) — or, if no caret exists yet, the evidence alone (booting pane). Round 7's own report names the residual explicitly: "one rendering-quirk wide, not one prose line wide." Find that quirk, or genuinely convince yourself it is not there. Concrete angles to try: can a caret exist somewhere IRRELEVANT (e.g. inside a code block claude is composing, a quoted transcript in its own output) within the adjacency window of a real accept-phrase line, without an actual gate being rendered? Does the footer-based path (no caret required at all) have its own weaker bar — could "press Enter to confirm" appear within 4 lines of nothing in particular and still fire? Is the adjacency window (1 line / 4 lines) calibrated only against the two specific live-transcribed gate captures rounds 5/6 happened to obtain, or does it hold for gate renderings shaped differently (a longer multi-line option, extra blank-line padding from a terminal resize, a wrapped narrow-terminal capture)?
- Docs: `manifest/designs/quarry-glyph-plan-alphabet.md`, `manifest/designs/loom.md`, `manifest/designs/shed.md`, `contracts/specs/loom-plan-spec.md`, `contracts/stencils/loom/**`, `contracts/stencils/webster/webster-template-master.md`, `docs/overview.md`, `manifest/roadmap.md`, `CONSTRAINTS.md`, `README.md`.
- All seven prior rounds' material (read AFTER your own findings list): `_mill/loom-review-opus5-high-r1.md`/`-fixer-report.md`, `_mill/loom-review-sonnet5-xhigh-r2.md`/`-fixer-report.md`, `_mill/loom-review-fable5-high-r3.md`/`-fixer-report.md`, `_mill/loom-review-opus5-high-r4.md`/`-fixer-report.md`, `_mill/loom-review-opus-medium-r5.md`/`-fixer-report.md`, `_mill/loom-review-opus-medium-r6.md`/`-fixer-report.md`, `_mill/loom-review-fable-high-r7.md`/`-fixer-report.md`, `_mill/loom-review-HANDOFF.md` (the full campaign record).
- Repo rules: `CLAUDE.md` (root + `~/.claude/CLAUDE.md`) and `CONSTRAINTS.md` in full.

## Mission (be genuinely adversarial — try to find nothing, and earn that)

Two axes, applied to the WHOLE surface:

1. **Scope/integration** — does everything actually work as designed, end to end, under conditions no prior round has tried?
2. **Correctness** — bugs, races, error handling, edge cases — including in code seven prior rounds already "fixed." A fix that passed sabotage-proofing is proven correct for the SPECIFIC scenario it was tested against; it is not proven correct in general. Read fixed code as skeptically as new code, and the provider-startup seam most skeptically of all.

## High-yield focus

- **1. TOP PRIORITY — attempt genuine live driving of the provider-startup seam, on a fresh fixture, before anything else.** This is the single largest honest limit left standing after round 7: F1's fix has been sabotage-proofed (the reverted code was shown to produce the exact hazardous keypress sequence) but has NEVER been confirmed against a real, live claude pane — not by round 7, not by the orchestrator's independent verification of round 7. Build a brand-new fixture (a repository path claude has never seen — see "Where to work"), start a real Master/fork session, and — if you can construct or naturally trigger a transcript state resembling F1's failure scenarios (an accept-phrase-shaped list item, or "press Enter to confirm" phrasing, appearing near the input-box caret in ordinary agent output) — confirm live that no spurious keys are pressed. If you cannot naturally trigger it, at minimum confirm a real trust-gate dismissal and a real bypass-modal dismissal both still work end-to-end on a fresh fixture (closing the same live-confirmation gap for the SEAM as a whole, even if F1's specific prose-collision scenario itself proves hard to trigger live). Report exactly what you attempted and what you could/couldn't trigger.
- **2. Adversarially re-examine the provider-startup seam by READING, independent of whether item 1's live attempt succeeds.** See "What to read" above for the specific angles (irrelevant carets within the adjacency window, the footer path's weaker bar, calibration-only-against-two-captures). Three consecutive rounds have found real material here; do not assume the well is dry.
- **3. Second-model adversarial review of the `cliwire` consolidation.** Round 7 (a different model) reviewed it once and found it sound plus 3 fixed gaps. Convergence across DIFFERENT models is stronger evidence than repeated passes from one model — bring genuinely fresh eyes, don't just confirm round 7's conclusions.
- **4. General adversarial sweep, genuinely trying to find nothing, over everything else.** Read the whole surface fresh. If, after genuinely trying, you find nothing — that is a valid and valuable outcome (see the campaign README's own guidance) — but earn it; don't skip to it.
- **5. Whatever an eighth pass, on this campaign's FIRST xhigh-effort deployment and second-ever Sonnet deployment, turns up that seven prior passes (Opus×4, Sonnet×1/xhigh-but-untested-on-this-seam, Fable×2) didn't.** Don't force it. If the answer is genuinely "nothing new outside what's already being tracked," that is a valuable outcome — say so plainly and back it with the same rigor as a findings list.

## Explicitly OUT of scope for this round
- Windows path behavior — unreachable from this Linux host across all eight rounds; do not reason about it as if driven.
- `quarry`'s own resolve/delta engine correctness — treat its answers as ground truth.
- Loom's general pre-glyph pipeline mechanics from the two PRE-glyph crucible campaigns — don't re-verify from scratch; DO flag if your live driving happens to expose a regression. (This does NOT cover `internal/cliwire`, which is explicitly IN scope, see High-yield focus item 3.)
- The ~45-item residue from round 6's sweep over loom's pre-glyph pipeline machinery (doc drift, non-blocking correctness items unrelated to the glyph/cliwire/startup surface) — this orchestrator's job to spin into its own mill-wiki task, not this round's to fix.
- Full `--plan-dir` override propagation into Master's in-pane verbs (standalone mode) and `lyx reed` standalone support — both explicitly deferred by round 3 as their own future module tasks. Flag only if genuinely broken beyond what's already recorded.
- `burlercli`'s standalone reed bring-up — wired but still not live-verified by any round (no standalone burler scenario has been in scope yet); fair game if you want an extra live scenario, not required.
- Any new feature or roadmap work.

## Round context seeded from prior-round verification

**Rounds 1 through 7 are ALL CLOSED-AND-VERIFIED** — independently confirmed by this orchestrator, not self-reported. Full detail and verification evidence: `_mill/loom-review-HANDOFF.md`.

- **Round 1:** 22 findings (13 BLOCKING), 20 fixed. Root cause: whole-plan re-resolution against the post-change tree wedged every multi-batch plan with a Create/Delete/Rename card.
- **Round 2:** 7 findings (0 BLOCKING), all fixed. Proved a real hub-mode `lyx loom run` carries a glyph-bearing, `Plan-Write`-authored plan cleanly through `Webster-Review`. This campaign's only OTHER Sonnet deployment (xhigh) — predates the provider-startup seam entirely (opened by round 5).
- **Round 3:** 12 findings (5 BLOCKING), all fixed. Closed the Rename-through-Webster gap live to `Finalize → done`; found and fixed 3 more layers of the standalone-webster fix (`#004`); proved genuine crash resilience via a real `kill -9` mid-standalone-batch.
- **Round 4:** 38 findings (2 BLOCKING), 36 fixed. Fixed the last 2 fingerprint-restamp wedge bugs (root-caused in round 1, missed by rounds 1–3); achieved the first-ever live `DetectDrift` exact-tier auto-repair through a real Webster fork; ran real `kill -9` hub-mode crash tests.
- **Round 5:** 7 findings (2 BLOCKING), all fixed. `claudeengine.TrustDismissSequence` confirmed claude's trust gate's REFUSING option, killing every agent spawned in a not-yet-trusted directory; claude's Bypass-Permissions modal wasn't a recognized startup gate and silently disabled the startup deadline. Both on the module's single hottest live path. **Opened the provider-startup seam.**
- **Round 6:** 28 findings (2 BLOCKING), all fixed. **R6-1**: round 5's own fix, read from the other side — the needle set matched against the WHOLE pane capture with no requirement a gate actually be rendered, so a healthy pane whose transcript happened to contain gate-like phrasing got keys pressed into it mid-turn. **R6-3**: `containment-file-overlap` indexed read-only `Uses:` refs alongside `Targets`, blocking `lyx webster run` on cards that only READ overlapping things. Plus 9 MEDIUM, 10 LOW, 6 NIT, including six wiring-duplication findings (R6-7/8/9/15/16/17) that motivated the `unify-webster-burler-wiring` consolidation.
- **Round 7:** 6 findings (1 BLOCKING), all fixed. **F1**: round 6's own R6-1 fix, narrowed but not closed — the positive-evidence rule never required the evidence lines to sit near each other, so a single prose line (an accept-phrase list item, or a "press Enter to confirm" mention) still classified a healthy pane as a gate and got keys pressed into it. Fixed with an adjacency requirement. First-ever review of the `cliwire` merge — found sound, plus F2 (`--stencils-dir` never stat'd at the wiring boundary), F3/F4 (enforcement-test blind spots), D1/D2 (closed the two round-6 coverage gaps). Independently verified 6/6 sabotage-proofs PASS, including a real constructed-fixture proof for F3.

**Convergence status: NOT MET.** Four consecutive rounds (4, 5, 6, 7) have each found genuine BLOCKING material — the campaign's own bar for convergence (a safety pass finding nothing severe) has not been hit once in that span, and three of the four (5, 6, 7) are the same seam, each fix a narrowing of its predecessor's hole. This round is not a formality safety pass; it is a genuine adversarial pass, on a model that has never examined the current provider-startup seam, with an explicit mandate to attempt real live confirmation where round 7 could not.

State the **merge bar**: correctness in the NORMAL single-instance flow, across every scenario above, is the gate. Do not chase artificial concurrency stress.

## Live-substrate cost declaration (loom IS an LLM-driving module)

**`LLM-DRIVING: yes.`** This round's item 1 is not conditional — make a genuine attempt at it (see High-yield focus item 1 for exactly what "genuine attempt" means and what counts as a valid outcome if the specific F1 scenario proves hard to trigger naturally).

- Each real hub-mode or standalone `lyx loom run`/`lyx webster run`/`lyx burler run` spawns roughly one real `claude` subprocess at a time, strictly sequential — budget real wall-clock minutes per scenario.
- **Check your PATH's `lyx` before ANY live driving** — every prior round found stale installed binaries at least once; confirm `which lyx`/`lyx --version` reflects current HEAD, redeploy (`CGO_ENABLED=1 go run ./tools/deploy`) if not.
- **Run at most one full-pipeline attempt at a time**, foreground, waited on to completion. Never start two live runs concurrently.
- **Any scenario touching Master/fork startup MUST use a fixture claude has never seen** — see "Where to work" above. This is the specific lesson round 5 leaves behind; do not skip it for convenience.
- For a crash-kill scenario specifically: use a REAL `kill -9`, not a graceful stop, and confirm via `pgrep`/process inspection that the target was genuinely alive before the kill and genuinely dead after. Confirm the absence of the run's own terminal artifact (e.g. `outcome.yaml`) as evidence the death was unclean.
- **Report the exact `lyx reed status`/`lyx reed attach` commands every time you start a live run.**

**Named smoke tests.** Bare `-run Smoke` is BANNED. See prior rounds' seeds (recoverable from git history) for the full named-test list if you want to run any hermetic smoke tests; check each one's own subprocess cost before running regardless.

**EXECUTION BAN**: `internal/burlerengine/smoke_cluster_test.go`'s cluster-fan tests — 2 real subprocesses each, no cluster-fan configured anywhere in this campaign's scope.

- Never run more than one live-substrate invocation at a time, in parallel, or backgrounded.
- **The generic "N× CONCURRENT full smoke suites" gate does NOT apply, full stop.**

## What to TEST — do not just read, EXERCISE it

Hermetic (must stay green throughout):
- `CGO_ENABLED=1 go build ./...`
- `CGO_ENABLED=1 go vet ./internal/loomengine/... ./internal/loomcli/... ./internal/loomrecipe/... ./internal/loomshed/... ./internal/shedengine/... ./internal/shedadapters/... ./internal/shedrecipe/... ./internal/shedbuild/... ./internal/hubgeom/... ./internal/planparser/... ./internal/planglyph/... ./internal/shuttleengine/... ./internal/shuttleengine/claudeengine/... ./internal/standalonegeom/... ./internal/standalonestate/... ./internal/cliwire/... ./internal/webstercli/... ./internal/burlercli/... ./internal/logger/...`
- `CGO_ENABLED=1 go test -count=5` over the same set + `./cmd/lyx/...`
- `CGO_ENABLED=1 go test -tags integration ./internal/planglyph/... ./internal/planparser/... ./internal/websterengine/... ./internal/loomcli/... ./internal/loomshed/... ./internal/webstercli/... ./internal/burlercli/... ./internal/cliwire/... ./internal/logger/...`
- `CGO_ENABLED=1 go test ./...` (whole repo)

Live driving — YOU drive it directly, START WITH THIS ROUND'S TOP-PRIORITY SCENARIO (item 1 above) before the rest:
- Deploy: `CGO_ENABLED=1 go run ./tools/deploy` before every source change you want live-reflected; confirm PATH agreement.
- Set up a brand-new hub or standalone fixture claude has never seen — see "Where to work" above.
- **Report exact `lyx reed status`/`lyx reed attach` commands every time you start a session.**
- "Headless" means "no human required" — NOT "no time/token cost to you." Forbidden reasons to skip a scenario: "operator-assisted", "cost-bearing", "long-running", "impractical". If your session's own permission classifier genuinely refuses `tmux`/`reed`/`deploy` outright (this happened to round 6), say so PLAINLY and explicitly, exactly as round 6 did — do not silently skip live driving and do not claim it as done.

TEARDOWN DISCIPLINE (critical): confirm ZERO stray substrate processes at the end of every scenario and again at the very end (`ps aux | grep -iE 'tmux|lyx|claude'`, scoped to what YOU started). Be honest about what you could NOT verify and why.

## How to judge each finding
`file:line`, concrete failure scenario, severity (BLOCKING/MEDIUM/LOW/NIT), suggested fix, CONFIRMED vs PLAUSIBLE. Severity affects reporting, not whether you fix it — fix everything, all severities, including NIT. A genuinely LARGE fix gets marked NOT-FIXED-THIS-ROUND with full reasoning; the orchestrator spins it into its own mill-wiki task.

## Deferred items from the prior round — RE-EVALUATE these (after your own pass)
- None carried forward as open coverage gaps — round 7 closed both of round 6's (R6-6's call-site test, R6-27's `loomcli`-twin test).
- Round 5's bypass-gate needle (`yes,iaccept`) watch item and round 6's own residual pre-glyph-machinery sweep (~45 items) remain out of scope for this round exactly as stated above — not unresolved items this round needs to revisit.

## Fixing — after the review
- Fix EVERY finding, all severities including NIT.
- Load `/code-quality` and `mill:golang-build`/`mill:golang-testing`/`mill:golang-comments` before editing.
- For every bug you fix, add or extend a test that would have caught it; for a live-only defect, a `//go:build smoke` test walking the real scenario.
- MAKE SMOKE TESTS DETERMINISTIC — poll with a deadline, never sleep a fixed amount.
- Update the relevant docs in the SAME change as the fix. Do NOT add bugfix/hardening notes to `manifest/roadmap.md`.
- Keep gates green after every change; redeploy and re-verify live scenarios.
- Tear down all substrate state; confirm zero stray processes. Commit each fix — do NOT push.
- **Fill in the fixer report's table completely before finishing** — every finding you fixed gets a row, written before you move to the next finding, not reconstructed from memory at the end.

## Deliverables
1. A structured review report: executive summary with an EXPLICIT convergence verdict (not just merge-readiness — does the campaign as a whole appear converged, per the README's own bar: a safety pass + this orchestrator's gates + an operator-assisted check all agreeing); which high-yield-focus items were attempted and how far each got — ESPECIALLY item 1 (live driving attempt), report exactly what was tried and what happened even if it did not trigger the specific hazard; findings severity-ranked with file:line/scenario/fix/CONFIRMED-PLAUSIBLE; what-was-tested with exact commands. Write to `_mill/loom-review-<yourtag>.md`, commit incrementally.
2. A fixer report: implemented/deferred/tests/changed-files, table complete and accurate. Write to `_mill/loom-review-<yourtag>-fixer-report.md`.
3. Final chat message: concise summary + severity counts + report paths + explicit merge-readiness AND convergence verdict + per-high-yield-focus-item yes/no on what was achieved. Also state, explicitly, this campaign's own honest limits (per the README's "state the limits" guidance) — e.g. Windows never reachable, anything this round still didn't manage to drive.

Begin with the clean-room review, produce your independent findings, then implement and verify the fixes.
