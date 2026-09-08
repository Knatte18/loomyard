# `loom` — independent review + fix (prompt template) — ROUND 5 (glyph-hardening campaign) — SAFETY PASS

> Filled instance of `crucible/review-prompt-template.md` for round 5 of the campaign scoped to the quarry-glyph-plan-alphabet surface (GitHub PR #230) plus the standalone-webster fix (`#004`) it spun off. See [../../crucible/README.md](../../crucible/README.md) for the loop, [../../_mill/loom-crucible-orchestrator-kickoff.md](loom-crucible-orchestrator-kickoff.md) for the campaign charter, and [loom-review-HANDOFF.md](loom-review-HANDOFF.md) for the campaign's full running state (read the handoff only AFTER you have your own independent findings list — see "Clean-room review constraint" below).
>
> **This round exists because round 4 was NOT a clean safety pass.** Round 4 (Opus/high) found and fixed 2 more genuine BLOCKING bugs that THREE prior rounds — Opus, Sonnet, Fable, three different models — all missed. That is real evidence this surface may still have more to give up. The original campaign budget was four rounds; this round runs under fresh, explicit operator authorization beyond that budget. **Your job is to try, genuinely, to find nothing** — and to earn that conclusion by actually driving the surface adversarially, not by assuming four prior passes already covered it.
>
> **Effort note:** this round runs at Opus/**medium** (the operator's explicit choice, lower than round 4's `high`) — narrower and more targeted than a from-scratch deep dive. Spend your budget on the two things below that most need YOUR independent eyes rather than re-treading everything four prior rounds already drove.

You are a senior engineer doing a COMPLETE, adversarial, INDEPENDENT review of the `loom` module (including the standalone-webster material `#004` merged into this same branch) in the loomyard repo, followed by FIXING what you find.

## Where to work — determine this yourself, do not assume a path
Work in the current git worktree. Confirm it yourself at the start (`git rev-parse --show-toplevel`, `git branch --show-current`) rather than trusting any hardcoded path in this file or in a prior round's report — this campaign has already run across at least two different host/user environments (prior rounds' own artifacts reference `/home/knatte/...`, which does not exist on every host this campaign has run from since). The branch is `crucible-loom-glyph-hardening`. If a live-driving fixture (a sandbox hub) is needed, do not assume one from a prior round still exists on disk where its own report says it does — check first (`gh repo view Knatte18/lyx-test`, then look for a reusable branch), and if reusing one, `git pull`/`git fetch` it fresh rather than trusting a local clone that may be stale or absent. See "High-yield focus" item 2 below for why a **fresh, independent** fixture is actually preferred here over reusing `r4-crash-hub`/`r4-drift-hub`.

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
**COMMIT each append.** This round has real-LLM live-driving scenarios (see "High-yield focus" below) — incremental commits matter as much as ever.

## Clean-room review constraint (do this part unprimed)
Form your OWN findings first.
Do NOT read any prior review or review-dialogue files before you have your own list — specifically do not open anything under `_mill/` matching `loom-review-*` in THIS worktree, including all four prior rounds' review/fixer reports AND this campaign's running `loom-review-HANDOFF.md`. This is a FILENAME PATTERN, not a content judgment.
AFTER you have your own independent findings, you MAY (and should) consult the prior rounds' material and the handoff to (a) confirm the prior fixes have not regressed and (b) understand this round's specific mission below.
Reading the design SPEC and the module docs is expected and required (those are not reviews).

## What to read
- Code — loom's own machinery: `internal/loomengine/**`, `internal/loomcli/**`, `internal/loomrecipe/**`, `internal/loomshed/**`, `internal/shedengine/**`, `internal/shedadapters/**`, `internal/shedrecipe/**`, `internal/shedbuild/**`, `internal/hubgeom/**`, `contracts/recipes/loom-recipe.yaml`, `cmd/lyx`'s loom integration.
- The glyph surface: `internal/planparser/**`, `internal/planglyph/**` (four rounds of fixes now — read the CURRENT state, not any prior round's description of it), `internal/websterengine/**` (`beginbatch.go`, `recordbatch.go`, `fingerprint.go`, `render.go`, `runlevel.go` all changed across rounds).
- The standalone-webster material (`#004`, hardened further by rounds 3 and 4): `internal/shuttleengine/run.go` (`NewRunner`/`NewDetachedRunner`), `internal/standalonegeom/**`, `internal/standalonestate/**`, `internal/webstercli/**` (especially `wiring.go` — three rounds of fixes to path resolution alone: relative flags, nested-state refusal, repository-root normalization), `internal/burlercli/**`, `internal/logger/sink.go`.
- Docs: `manifest/designs/quarry-glyph-plan-alphabet.md`, `manifest/designs/loom.md`, `manifest/designs/shed.md`, `contracts/specs/loom-plan-spec.md`, `contracts/stencils/loom/**`, `contracts/stencils/webster/webster-template-master.md`, `docs/overview.md`, `manifest/roadmap.md`, `CONSTRAINTS.md`, `README.md`.
- All four prior rounds' material (read AFTER your own findings list): `_mill/loom-review-opus5-high-r1.md`/`-fixer-report.md`, `_mill/loom-review-sonnet5-xhigh-r2.md`/`-fixer-report.md`, `_mill/loom-review-fable5-high-r3.md`/`-fixer-report.md`, `_mill/loom-review-opus5-high-r4.md`/`-fixer-report.md`, `_mill/loom-review-HANDOFF.md` (the full campaign record — read this closely, it names exactly what this orchestrator's own verification could and could not confirm about round 4).
- Repo rules: `CLAUDE.md` (root + `~/.claude/CLAUDE.md`) and `CONSTRAINTS.md` in full.

## Mission (be genuinely adversarial — try to find nothing, and earn that)

Two axes, applied to the WHOLE surface:

1. **Scope/integration** — does everything actually work as designed, end to end, under conditions no prior round has tried?
2. **Correctness** — bugs, races, error handling, edge cases — including in code four prior rounds already "fixed." A fix that passed sabotage-proofing is proven correct for the SPECIFIC scenario it was tested against; it is not proven correct in general. Read fixed code as skeptically as new code.

## High-yield focus

- **1. General adversarial sweep, genuinely trying to find nothing.** Read the whole surface fresh. If, after genuinely trying, you find nothing — that is a valid and valuable outcome (see the campaign README's own guidance) — but earn it; don't skip to it.
- **2. An independent, SECOND hub-mode crash-kill reproduction — on a FRESH fixture, not `r4-crash-hub`/`r4-drift-hub`.** Round 4's own report describes real `kill -9` crash-resilience tests in hub mode (a Bouncer/Burler review-segment kill, and a Webster-batch kill) with real evidence (absent terminal artifacts proving an unclean death, `pgrep`-confirmed process state, `tmux list-panes`/`capture-pane` confirming orphan survival). This orchestrator independently confirmed the COMMIT-level artifacts those runs produced are real (cloned `github.com/Knatte18/lyx-test`, diffed the actual commits — they match round 4's report exactly, including the exact-tier drift-repair rename). What could NOT be independently re-confirmed is the PROCESS-level mechanics themselves (PIDs, tmux pane state) — those are inherently ephemeral and were only ever inspectable live, on the host round 4 ran on. Per the fabric campaign's own lesson ("a reproduction on a second independent hub is what turns an anecdote into a finding"), reproduce a real hub-mode `kill -9` mid-Webster-batch crash-kill yourself, on a fresh fixture, with the same rigor round 3/4 used (confirm alive via `pgrep` before the kill, confirm dead after, confirm the terminal artifact is genuinely absent, confirm resume completes cleanly). This is the one thing this round most needs to deliver that four prior rounds' own record cannot substitute for.
- **3. Whatever a fifth pass, on a fourth deployment of the strongest model in the rotation, at a lighter effort tier, turns up that the other four (spanning Opus/high, Sonnet/xhigh, Fable/high, Opus/high) didn't.** Don't force it.

## Explicitly OUT of scope for this round
- Windows path behavior — unreachable from this Linux host across all five rounds; do not reason about it as if driven.
- `quarry`'s own resolve/delta engine correctness — treat its answers as ground truth.
- Loom's general pre-glyph pipeline mechanics from the two PRE-glyph crucible campaigns — don't re-verify from scratch; DO flag if your live driving happens to expose a regression.
- Full `--plan-dir` override propagation into Master's in-pane verbs (standalone mode) and `lyx reed` standalone support — both explicitly deferred by round 3 as their own future module tasks. Flag only if genuinely broken beyond what's already recorded.
- `burlercli`'s standalone reed bring-up — wired but still not live-verified by any round (no standalone burler scenario has been in scope yet); fair game if you want an extra live scenario, not required.
- Any new feature or roadmap work.

## Round context seeded from prior-round verification

**Rounds 1 (opus-high-r1), 2 (sonnet-xhigh-r2), 3 (fable-high-r3), and 4 (opus-high-r4) are ALL CLOSED-AND-VERIFIED** — independently confirmed by this orchestrator, not self-reported. Full detail and verification evidence: `_mill/loom-review-HANDOFF.md`.

- **Round 1:** 22 findings (13 BLOCKING), 20 fixed. Root cause: whole-plan re-resolution against the post-change tree wedged every multi-batch plan with a Create/Delete/Rename card.
- **Round 2:** 7 findings (0 BLOCKING), all fixed. Proved a real hub-mode `lyx loom run` carries a glyph-bearing, `Plan-Write`-authored plan cleanly through `Webster-Review`.
- **Round 3:** 12 findings (5 BLOCKING), all fixed. Closed the Rename-through-Webster gap live to `Finalize → done`; found and fixed 3 more layers of the standalone-webster fix (`#004`); proved genuine crash resilience via a real `kill -9` mid-standalone-batch.
- **Round 4:** 38 findings (2 BLOCKING), 36 fixed (1 withdrawn as a false positive after implementation, 1 recorded but not reproduced/out of scope). Fixed the last 2 fingerprint-restamp wedge bugs (root-caused in round 1, missed by rounds 1–3); achieved the first-ever live `DetectDrift` exact-tier auto-repair through a real Webster fork; ran real `kill -9` hub-mode crash tests on both a Bouncer/Burler review segment and a Webster batch. This orchestrator independently confirmed 9/9 sabotage-proofs (both BLOCKING findings plus 7 findings whose commits had landed but were missing from the round's own fixer-report table) and confirmed the live claims' commit-level artifacts are genuine via the real `github.com/Knatte18/lyx-test` GitHub history — but could not re-verify the process-level crash mechanics from this session's host. **That gap is this round's item 2 above.**

State the **merge bar**: correctness in the NORMAL single-instance flow, across every scenario above, is the gate. Do not chase artificial concurrency stress.

## Live-substrate cost declaration (loom IS an LLM-driving module)

**`LLM-DRIVING: yes.`** This round likely needs at least one real hub-mode live-driving scenario (item 2 above) plus whatever item 1's sweep turns up.

- Each real hub-mode or standalone `lyx loom run`/`lyx webster run` spawns roughly one real `claude` subprocess at a time, strictly sequential — budget real wall-clock minutes per scenario.
- **Check your PATH's `lyx` before ANY live driving** — every prior round found stale installed binaries at least once; confirm `which lyx`/`lyx --version` reflects current HEAD, redeploy (`CGO_ENABLED=1 go run ./tools/deploy`) if not.
- **Run at most one full-pipeline attempt at a time**, foreground, waited on to completion. Never start two live runs concurrently.
- For the crash-kill scenario: use a REAL `kill -9`, not a graceful stop, and confirm via `pgrep`/process inspection that the target was genuinely alive before the kill and genuinely dead after — a kill that races a process already finishing proves nothing. Confirm the absence of the run's own terminal artifact (e.g. `outcome.yaml`, a completion marker) as evidence the death was unclean.
- **Report the exact `lyx reed status`/`lyx reed attach` commands every time you start a live run.**

**Named smoke tests.** Bare `-run Smoke` is BANNED. See prior rounds' seeds (recoverable from git history) for the full named-test list if you want to run any hermetic smoke tests; check each one's own subprocess cost before running regardless.

**EXECUTION BAN**: `internal/burlerengine/smoke_cluster_test.go`'s cluster-fan tests — 2 real subprocesses each, no cluster-fan configured anywhere in this campaign's scope.

- Never run more than one live-substrate invocation at a time, in parallel, or backgrounded.
- **The generic "N× CONCURRENT full smoke suites" gate does NOT apply, full stop.**

## What to TEST — do not just read, EXERCISE it

Hermetic (must stay green throughout):
- `CGO_ENABLED=1 go build ./...`
- `CGO_ENABLED=1 go vet ./internal/loomengine/... ./internal/loomcli/... ./internal/loomrecipe/... ./internal/loomshed/... ./internal/shedengine/... ./internal/shedadapters/... ./internal/shedrecipe/... ./internal/shedbuild/... ./internal/hubgeom/... ./internal/planparser/... ./internal/planglyph/... ./internal/shuttleengine/... ./internal/standalonegeom/... ./internal/standalonestate/... ./internal/webstercli/... ./internal/burlercli/... ./internal/logger/...`
- `CGO_ENABLED=1 go test -count=5` over the same set + `./cmd/lyx/...`
- `CGO_ENABLED=1 go test -tags integration ./internal/planglyph/... ./internal/planparser/... ./internal/websterengine/... ./internal/loomcli/... ./internal/loomshed/... ./internal/webstercli/... ./internal/burlercli/...`
- `CGO_ENABLED=1 go test ./...` (whole repo)

Live driving — YOU drive it directly, for every high-yield-focus scenario you attempt:
- Deploy: `CGO_ENABLED=1 go run ./tools/deploy` before every source change you want live-reflected; confirm PATH agreement.
- Set up whatever real hub/standalone fixtures each scenario needs. For item 2 specifically, prefer a FRESH fixture over reusing a prior round's — see "Where to work" above for why and how to check what's reusable.
- **Report exact `lyx reed status`/`lyx reed attach` commands every time you start a session.**
- "Headless" means "no human required" — NOT "no time/token cost to you." Forbidden reasons to skip a scenario: "operator-assisted", "cost-bearing", "long-running", "impractical".

TEARDOWN DISCIPLINE (critical): confirm ZERO stray substrate processes at the end of every scenario and again at the very end (`ps aux | grep -iE 'tmux|lyx|claude'`, scoped to what YOU started). Be honest about what you could NOT verify and why.

## How to judge each finding
`file:line`, concrete failure scenario, severity (BLOCKING/MEDIUM/LOW/NIT), suggested fix, CONFIRMED vs PLAUSIBLE. Severity affects reporting, not whether you fix it — fix everything, all severities, including NIT. A genuinely LARGE fix gets marked NOT-FIXED-THIS-ROUND with full reasoning; the orchestrator spins it into its own mill-wiki task.

## Deferred items from the prior round — RE-EVALUATE these (after your own pass)
None requiring re-evaluation as "still open questions" — everything prior rounds deferred is a deliberate, recorded future-task deferral (see "Explicitly OUT of scope" above), not an unresolved item this round needs to revisit.

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
