# `loom` — independent review + fix (prompt template) — ROUND 6 (glyph-hardening campaign) — SAFETY PASS

> Filled instance of `crucible/review-prompt-template.md` for round 6 of the campaign scoped to the quarry-glyph-plan-alphabet surface (GitHub PR #230) plus the standalone-webster fix (`#004`) it spun off. See [../../crucible/README.md](../../crucible/README.md) for the loop, [../../_mill/loom-crucible-orchestrator-kickoff.md](loom-crucible-orchestrator-kickoff.md) for the campaign charter, and [loom-review-HANDOFF.md](loom-review-HANDOFF.md) for the campaign's full running state (read the handoff only AFTER you have your own independent findings list — see "Clean-room review constraint" below).
>
> **This round exists because round 5 was NOT a clean safety pass either.** Round 5 (Opus/medium) found and fixed 2 MORE genuine BLOCKING bugs — this time on the single hottest live path in the entire module (claude's own startup-gate sequence: every agent lyx spawns goes through it). All FIVE prior rounds, across five different model/effort combinations, missed them, for a specific, structural reason: both defects are invisible on any fixture the `claude` CLI has ever been driven in by hand, and every prior round reused or inherited such a fixture. Round 5's own second finding (R5-7) was reachable ONLY after its first finding (R5-2) was fixed — a defect hidden behind another defect on the same path. Round 5 explicitly declined to claim convergence: "two rounds in a row have now found BLOCKING material... is not evidence that the path is now clear." **Your job is to try, genuinely, to find nothing** — and to earn that conclusion by actually driving the surface adversarially, not by assuming five prior passes already covered it.
>
> **Effort note:** this round runs at Opus/**medium** again — the operator's explicit, deliberate choice (the third Opus deployment in the rotation, after r1/high and r4/high; the operator judged that method/angle diversity, not model diversity, is what has been finding real material — round 5's own "genuinely fresh fixture" angle was itself the thing that exposed its findings, not a model-capability difference). Spend your budget on the two things below that most need YOUR independent eyes rather than re-treading everything five prior rounds already drove.

You are a senior engineer doing a COMPLETE, adversarial, INDEPENDENT review of the `loom` module (including the standalone-webster material `#004` merged into this same branch) in the loomyard repo, followed by FIXING what you find.

## Where to work — determine this yourself, do not assume a path
Work in the current git worktree. Confirm it yourself at the start (`git rev-parse --show-toplevel`, `git branch --show-current`) rather than trusting any hardcoded path in this file or in a prior round's report — this campaign has already run across multiple host/user environments. The branch is `crucible-loom-glyph-hardening`. If a live-driving fixture (a sandbox hub) is needed, do not assume one from a prior round still exists on disk where its own report says it does — check first (`gh repo view Knatte18/lyx-test`, then look for a reusable branch).

**Durable method lesson from round 5, BLOCKING for any live scenario you drive this round:** a fixture whose repository path has EVER hosted an interactive `claude` session (including one you yourself drove earlier in this same round, or any prior round's `r4-*`/`r5sandbox*` fixture) is permanently immunized against the whole class of defect round 5 found — claude's one-time startup gates simply do not render there again. If you attempt ANY live scenario that touches Master/fork startup, you MUST build it from a repository path claude has never seen (a brand-new `lyx fabric clone` + `lyx fabric add`, or a brand-new standalone target directory) — reusing `r4-crash-hub`, `r4-drift-hub`, `r5sandbox`, or `r5sandbox2` for that purpose proves nothing.

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
**COMMIT each append.** This round may have real-LLM live-driving scenarios (see "High-yield focus" below) — incremental commits matter as much as ever.

## Clean-room review constraint (do this part unprimed)
Form your OWN findings first.
Do NOT read any prior review or review-dialogue files before you have your own list — specifically do not open anything under `_mill/` matching `loom-review-*` in THIS worktree, including all five prior rounds' review/fixer reports AND this campaign's running `loom-review-HANDOFF.md`. This is a FILENAME PATTERN, not a content judgment.
AFTER you have your own independent findings, you MAY (and should) consult the prior rounds' material and the handoff to (a) confirm the prior fixes have not regressed and (b) understand this round's specific mission below.
Reading the design SPEC and the module docs is expected and required (those are not reviews).

## What to read
- Code — loom's own machinery: `internal/loomengine/**`, `internal/loomcli/**`, `internal/loomrecipe/**`, `internal/loomshed/**`, `internal/shedengine/**`, `internal/shedadapters/**`, `internal/shedrecipe/**`, `internal/shedbuild/**`, `internal/hubgeom/**`, `contracts/recipes/loom-recipe.yaml`, `cmd/lyx`'s loom integration.
- The glyph surface: `internal/planparser/**`, `internal/planglyph/**` (five rounds of fixes now — read the CURRENT state, not any prior round's description of it), `internal/websterengine/**` (`beginbatch.go`, `recordbatch.go`, `fingerprint.go`, `render.go`, `runlevel.go` all changed across rounds).
- The standalone-webster material (`#004`, hardened further by rounds 3 and 4): `internal/shuttleengine/run.go` (`NewRunner`/`NewDetachedRunner`), `internal/standalonegeom/**`, `internal/standalonestate/**`, `internal/webstercli/**` (especially `wiring.go` — three rounds of fixes to path resolution alone: relative flags, nested-state refusal, repository-root normalization), `internal/burlercli/**`, `internal/logger/sink.go`.
- **The provider-startup seam, new territory round 5 opened — read it adversarially, it is exactly the shape that just yielded two BLOCKING findings:** `internal/shuttleengine/claudeengine/startup.go` (`Startup`, `TrustDismissSequence`, `startupGateNeedles`, `gateAcceptNeedles`), `internal/shuttleengine/claudeengine/doc.go`, `internal/shuttleengine/wait.go` (the startup-window classification loop), `internal/shuttleengine/engine.go` (the `Engine` seam `TrustDismissSequence` now takes a capture argument through). Round 5's fix is capture-driven and fails safe (presses nothing when it cannot locate the gate it's looking for) — is that actually true of every code path that calls into it? Are there other claude UI states (a rate-limit banner, a model-unavailable notice, an update prompt) that could hit the same "carries the ready caret glyph but isn't actually ready" trap R5-7 found? Is `ComposeSend`/`ModelSwitchSequence`/`InterruptSequence` — the other fixed key choreographies in this same file — subject to any analogous provider-string assumption?
- Docs: `manifest/designs/quarry-glyph-plan-alphabet.md`, `manifest/designs/loom.md`, `manifest/designs/shed.md`, `contracts/specs/loom-plan-spec.md`, `contracts/stencils/loom/**`, `contracts/stencils/webster/webster-template-master.md`, `docs/overview.md`, `manifest/roadmap.md`, `CONSTRAINTS.md`, `README.md`.
- All five prior rounds' material (read AFTER your own findings list): `_mill/loom-review-opus5-high-r1.md`/`-fixer-report.md`, `_mill/loom-review-sonnet5-xhigh-r2.md`/`-fixer-report.md`, `_mill/loom-review-fable5-high-r3.md`/`-fixer-report.md`, `_mill/loom-review-opus5-high-r4.md`/`-fixer-report.md`, `_mill/loom-review-opus-medium-r5.md`/`-fixer-report.md`, `_mill/loom-review-HANDOFF.md` (the full campaign record).
- Repo rules: `CLAUDE.md` (root + `~/.claude/CLAUDE.md`) and `CONSTRAINTS.md` in full.

## Mission (be genuinely adversarial — try to find nothing, and earn that)

Two axes, applied to the WHOLE surface:

1. **Scope/integration** — does everything actually work as designed, end to end, under conditions no prior round has tried?
2. **Correctness** — bugs, races, error handling, edge cases — including in code five prior rounds already "fixed." A fix that passed sabotage-proofing is proven correct for the SPECIFIC scenario it was tested against; it is not proven correct in general. Read fixed code as skeptically as new code.

## High-yield focus

- **1. General adversarial sweep, genuinely trying to find nothing.** Read the whole surface fresh. If, after genuinely trying, you find nothing — that is a valid and valuable outcome (see the campaign README's own guidance) — but earn it; don't skip to it.
- **2. Adversarially re-examine the provider-startup seam round 5 just opened (`claudeengine/startup.go`, `wait.go`, `engine.go`).** Round 5 found two BLOCKING defects there in one pass; a single round's own fix is proven correct for the SPECIFIC scenarios it tested (a fresh trust gate, a fresh bypass-permissions modal) but this is exactly the kind of provider-owned, string-keyed surface that has already changed once under lyx (the caret's default position) and is a standing watch item per round 5's own "honest limits" section. Look for: any other claude UI state that could be misclassified the same way (READY-by-caret-coincidence, or an unrecognized one-time dialog); whether the capture-driven caret walk is robust to option orderings beyond the two live-transcribed cases; whether `wait.go`'s consumption of the capture argument is complete everywhere `TrustDismissSequence` is called, not just the one site round 5 touched. If you drive this live, it MUST be on a fixture claude has never seen — see "Where to work" above.
- **3. Whatever a sixth pass, on a THIRD deployment of Opus in this rotation, turns up that five prior passes (Opus×2, Sonnet, Fable, Opus/medium) didn't.** Don't force it. If the answer is genuinely "nothing new," that is this round's most valuable possible outcome — say so plainly and back it with the same rigor as a findings list.

## Explicitly OUT of scope for this round
- Windows path behavior — unreachable from this Linux host across all six rounds; do not reason about it as if driven.
- `quarry`'s own resolve/delta engine correctness — treat its answers as ground truth.
- Loom's general pre-glyph pipeline mechanics from the two PRE-glyph crucible campaigns — don't re-verify from scratch; DO flag if your live driving happens to expose a regression.
- Full `--plan-dir` override propagation into Master's in-pane verbs (standalone mode) and `lyx reed` standalone support — both explicitly deferred by round 3 as their own future module tasks. Flag only if genuinely broken beyond what's already recorded.
- `burlercli`'s standalone reed bring-up — wired but still not live-verified by any round (no standalone burler scenario has been in scope yet); fair game if you want an extra live scenario, not required.
- Any new feature or roadmap work.

## Round context seeded from prior-round verification

**Rounds 1 (opus-high-r1), 2 (sonnet-xhigh-r2), 3 (fable-high-r3), 4 (opus-high-r4), and 5 (opus-medium-r5) are ALL CLOSED-AND-VERIFIED** — independently confirmed by this orchestrator, not self-reported. Full detail and verification evidence: `_mill/loom-review-HANDOFF.md`.

- **Round 1:** 22 findings (13 BLOCKING), 20 fixed. Root cause: whole-plan re-resolution against the post-change tree wedged every multi-batch plan with a Create/Delete/Rename card.
- **Round 2:** 7 findings (0 BLOCKING), all fixed. Proved a real hub-mode `lyx loom run` carries a glyph-bearing, `Plan-Write`-authored plan cleanly through `Webster-Review`.
- **Round 3:** 12 findings (5 BLOCKING), all fixed. Closed the Rename-through-Webster gap live to `Finalize → done`; found and fixed 3 more layers of the standalone-webster fix (`#004`); proved genuine crash resilience via a real `kill -9` mid-standalone-batch.
- **Round 4:** 38 findings (2 BLOCKING), 36 fixed (1 withdrawn as a false positive after implementation, 1 recorded but not reproduced/out of scope). Fixed the last 2 fingerprint-restamp wedge bugs (root-caused in round 1, missed by rounds 1–3); achieved the first-ever live `DetectDrift` exact-tier auto-repair through a real Webster fork; ran real `kill -9` hub-mode crash tests on both a Bouncer/Burler review segment and a Webster batch. Independently verified via 9/9 sabotage-proofs and real-GitHub-artifact confirmation; process-level crash mechanics from that round were not independently re-verifiable from a later host.
- **Round 5:** 7 findings (2 BLOCKING), all fixed. Clean-room review found `claudeengine.TrustDismissSequence` confirmed claude's trust gate's REFUSING option (caret defaults there on claude 2.1.263), killing every agent spawned in a not-yet-trusted directory — i.e. every fresh fabric worktree, the module's own primary entry path. The second finding, reachable only after the first was fixed, was that claude's Bypass-Permissions modal (raised on every `--dangerously-skip-permissions` launch) wasn't a recognized startup gate and its own caret glyph made the pane misclassify as READY, silently disabling the startup deadline for up to the full `master_timeout_min`. Also fixed: a leaked tmux server in `webstercli`'s own integration test (R5-1), a done-check that silently passed on an uncovered resolve target instead of erroring (R5-6), a dropped error when a fingerprint re-baseline failed alongside a validation error (R5-3), and two doc NITs. Independently verified: 5/7 findings sabotage-proofed firsthand (both BLOCKING plus R5-1/R5-3/R5-6), all cold gates green, and — for the first time in this campaign — the live crash-kill/resume claim verified ENTIRELY firsthand from real on-disk artifacts on the same host as the round itself (zero substitution gap). Round 5 explicitly declined to claim convergence.

State the **merge bar**: correctness in the NORMAL single-instance flow, across every scenario above, is the gate. Do not chase artificial concurrency stress.

## Live-substrate cost declaration (loom IS an LLM-driving module)

**`LLM-DRIVING: yes, conditionally.`** This round's item 2 may or may not need live driving depending on what you find reading `claudeengine/startup.go` adversarially — a genuine defect found by reading may not need a live reproduction to fix and test (round 5's own R5-1/R5-3/R5-4/R5-5/R5-6 needed no live driving at all; only R5-2/R5-7 did, and those were found BY live driving on a fresh fixture). If your sweep turns up something needing live confirmation:

- Each real hub-mode or standalone `lyx loom run`/`lyx webster run` spawns roughly one real `claude` subprocess at a time, strictly sequential — budget real wall-clock minutes per scenario.
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
- `CGO_ENABLED=1 go vet ./internal/loomengine/... ./internal/loomcli/... ./internal/loomrecipe/... ./internal/loomshed/... ./internal/shedengine/... ./internal/shedadapters/... ./internal/shedrecipe/... ./internal/shedbuild/... ./internal/hubgeom/... ./internal/planparser/... ./internal/planglyph/... ./internal/shuttleengine/... ./internal/shuttleengine/claudeengine/... ./internal/standalonegeom/... ./internal/standalonestate/... ./internal/webstercli/... ./internal/burlercli/... ./internal/logger/...`
- `CGO_ENABLED=1 go test -count=5` over the same set + `./cmd/lyx/...`
- `CGO_ENABLED=1 go test -tags integration ./internal/planglyph/... ./internal/planparser/... ./internal/websterengine/... ./internal/loomcli/... ./internal/loomshed/... ./internal/webstercli/... ./internal/burlercli/...`
- `CGO_ENABLED=1 go test ./...` (whole repo)

Live driving — YOU drive it directly, for every high-yield-focus scenario that turns out to need it:
- Deploy: `CGO_ENABLED=1 go run ./tools/deploy` before every source change you want live-reflected; confirm PATH agreement.
- Set up whatever real hub/standalone fixtures each scenario needs — ALWAYS a fresh, never-hand-driven one if the scenario touches Master/fork startup (see "Where to work" above).
- **Report exact `lyx reed status`/`lyx reed attach` commands every time you start a session.**
- "Headless" means "no human required" — NOT "no time/token cost to you." Forbidden reasons to skip a scenario: "operator-assisted", "cost-bearing", "long-running", "impractical".

TEARDOWN DISCIPLINE (critical): confirm ZERO stray substrate processes at the end of every scenario and again at the very end (`ps aux | grep -iE 'tmux|lyx|claude'`, scoped to what YOU started). Be honest about what you could NOT verify and why.

## How to judge each finding
`file:line`, concrete failure scenario, severity (BLOCKING/MEDIUM/LOW/NIT), suggested fix, CONFIRMED vs PLAUSIBLE. Severity affects reporting, not whether you fix it — fix everything, all severities, including NIT. A genuinely LARGE fix gets marked NOT-FIXED-THIS-ROUND with full reasoning; the orchestrator spins it into its own mill-wiki task.

## Deferred items from the prior round — RE-EVALUATE these (after your own pass)
- Round 5's "honest limits" flagged the bypass-gate needle (`yes,iaccept`) as keyed on a provider-owned string that "fails safe... but is not 'keeps working'... a standing watch item, not a solved problem." If your reading of `claudeengine` turns up a more robust classification signal than string-matching an option label, that is fair game — but do not treat "it might change again" alone as a finding without a concrete, better mechanism to propose; round 5 already made the fails-safe tradeoff deliberately.
- Everything else prior rounds deferred is a deliberate, recorded future-task deferral (see "Explicitly OUT of scope" above), not an unresolved item this round needs to revisit.

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
1. A structured review report: executive summary with an EXPLICIT convergence verdict (not just merge-readiness — does the campaign as a whole appear converged, per the README's own bar: a safety pass + this orchestrator's gates + an operator-assisted check all agreeing); which high-yield-focus items were attempted and how far each got; findings severity-ranked with file:line/scenario/fix/CONFIRMED-PLAUSIBLE; what-was-tested with exact commands. Write to `_mill/loom-review-<yourtag>.md`, commit incrementally.
2. A fixer report: implemented/deferred/tests/changed-files, table complete and accurate. Write to `_mill/loom-review-<yourtag>-fixer-report.md`.
3. Final chat message: concise summary + severity counts + report paths + explicit merge-readiness AND convergence verdict + per-high-yield-focus-item yes/no on what was achieved. Also state, explicitly, this campaign's own honest limits (per the README's "state the limits" guidance) — e.g. Windows never reachable, anything this round still didn't manage to drive.

Begin with the clean-room review, produce your independent findings, then implement and verify the fixes.
