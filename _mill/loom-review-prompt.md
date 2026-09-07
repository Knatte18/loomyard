# `loom` — independent review + fix (prompt template) — ROUND 4 (glyph-hardening campaign) — FINAL SAFETY PASS

> Filled instance of `crucible/review-prompt-template.md` for round 4 — the LAST round in this campaign's pre-approved four-round budget — of the campaign scoped to the quarry-glyph-plan-alphabet surface (GitHub PR #230) plus the standalone-webster fix (`#004`) it spun off. See [../../crucible/README.md](../../crucible/README.md) for the loop, [../../_mill/loom-crucible-orchestrator-kickoff.md](loom-crucible-orchestrator-kickoff.md) for the campaign charter, and [loom-review-HANDOFF.md](loom-review-HANDOFF.md) for the campaign's full running state (read the handoff only AFTER you have your own independent findings list — see "Clean-room review constraint" below).
>
> **This round is explicitly NOT a formality.** Three prior rounds (Opus/high, Sonnet/xhigh, Fable/high) closed 41 findings and independently verified every fix — but round 2 (Sonnet) reported 0 BLOCKING, and Sonnet's own clean result is weaker evidence of convergence than the same result from a more capable model would be, which is exactly why crucible rotates models and puts the strongest one last. Round 3 already found 2 real BLOCKING defects in territory round 2 never drove — which doesn't prove round 2 missed anything WITHIN what it actually tested, but leaves that question genuinely open. **Your job is to find out, adversarially, not to confirm merge-readiness by default.**

You are a senior engineer doing a COMPLETE, adversarial, INDEPENDENT review of the `loom` module (including the standalone-webster material `#004` merged into this same branch) in the loomyard repo, followed by FIXING what you find.
Work in the worktree at `/home/knatte/Code/loomyard/wts/crucible-loom-glyph-hardening` (branch `crucible-loom-glyph-hardening`).
Adjust that path/branch if the task lives elsewhere now.

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
Also commit `_mill/loom-review-<yourtag>.md` and `_mill/loom-review-<yourtag>-fixer-report.md` as you write or update them.

## Sequencing rule (BLOCKING — do not skip, do not interleave)
Job 1 must be COMPLETE — and its full review report SAVED to `_mill/loom-review-<yourtag>.md` and committed — before you touch (edit, create, or delete) a single production or test file.
Do not fix findings as you go, even ones that look small and obviously right.

## Log as you go during Job 1 (BLOCKING — crash-resilience, do not batch it all to the end)
As you work through "What to TEST" below, APPEND your observations to `_mill/loom-review-<yourtag>.md`'s "What was tested" section immediately after each command/scenario returns, and jot findings provisionally as you spot them.
**COMMIT each append.** This round has SEVERAL real-LLM live-driving scenarios (see "High-yield focus" below) — incremental commits matter as much as ever.

## Clean-room review constraint (do this part unprimed)
Form your OWN findings first.
Do NOT read any prior review or review-dialogue files before you have your own list — specifically do not open anything under `_mill/` matching `loom-review-*` in THIS worktree, including all three prior rounds' review/fixer reports AND this campaign's running `loom-review-HANDOFF.md`. This is a FILENAME PATTERN, not a content judgment.
AFTER you have your own independent findings, you MAY (and should) consult the prior rounds' material and the handoff to (a) confirm the 41 prior fixes have not regressed and (b) understand this round's specific mission below.
Reading the design SPEC and the module docs is expected and required (those are not reviews).

## What to read
- Code — loom's own machinery: `internal/loomengine/**`, `internal/loomcli/**`, `internal/loomrecipe/**`, `internal/loomshed/**`, `internal/shedengine/**`, `internal/shedadapters/**`, `internal/shedrecipe/**`, `internal/shedbuild/**`, `internal/hubgeom/**`, `contracts/recipes/loom-recipe.yaml`, `cmd/lyx`'s loom integration.
- The glyph surface: `internal/planparser/**`, `internal/planglyph/**` (three rounds of fixes now — read the CURRENT state, not any prior round's description of it), `internal/websterengine/**` (`beginbatch.go`, `recordbatch.go`, `fingerprint.go`, `render.go`, `runlevel.go` all changed across rounds).
- The standalone-webster material (`#004`, hardened further by round 3): `internal/shuttleengine/run.go` (`NewRunner`/`NewDetachedRunner`), `internal/standalonegeom/**`, `internal/webstercli/**`, `internal/burlercli/**`, `internal/logger/sink.go`.
- Docs: `manifest/designs/quarry-glyph-plan-alphabet.md`, `manifest/designs/loom.md`, `contracts/specs/loom-plan-spec.md`, `contracts/stencils/loom/**`, `contracts/stencils/webster/webster-template-master.md`, `docs/overview.md`, `manifest/roadmap.md`, `CONSTRAINTS.md`, `README.md`.
- All three prior rounds' material (read AFTER your own findings list): `_mill/loom-review-opus5-high-r1.md`/`-fixer-report.md`, `_mill/loom-review-sonnet5-xhigh-r2.md`/`-fixer-report.md`, `_mill/loom-review-fable5-high-r3.md`/`-fixer-report.md`, `_mill/loom-review-HANDOFF.md` (the full campaign record — read this closely, it names exactly what remains genuinely untested: `DetectDrift`'s exact-tier auto-repair live through a real fork, a real hub-mode crash-kill test, and round 2's own hub-mode territory never having had an adversarial second look).
- Repo rules: `CLAUDE.md` (root + `~/.claude/CLAUDE.md`) and `CONSTRAINTS.md` in full — by now you've touched most of the relevant invariants across three rounds' worth of fixes; read the whole file once fresh rather than assuming you remember which ones apply.

## Mission (be genuinely adversarial — this is the last round)

Two axes, applied to the WHOLE surface, not just what's changed since round 3:

1. **Scope/integration** — does everything actually work as designed, end to end, under conditions no prior round has tried?
2. **Correctness** — bugs, races, error handling, edge cases — including in code three prior rounds already "fixed." A fix that passed sabotage-proofing is proven correct for the SPECIFIC scenario it was tested against; it is not proven correct in general. Read fixed code as skeptically as new code.

## High-yield focus — what genuinely remains untested after three rounds

- **1. A genuinely skeptical re-look at round 2's own territory.** Round 2 (Sonnet) drove a real hub-mode `lyx loom run` through Discussion-Write, Plan-Write, Plan-Review, Webster, Webster-Review with 0 BLOCKING findings. No round since has gone back and read THOSE code paths (handle canonicalization as `Plan-Write`/`Plan-Validate`/`Plan-Revalidate` actually exercise it end-to-end, `Plan-Review`'s rubric and judge behavior, `Webster-Review`'s own per-card mechanical checks) with fresh, adversarial eyes the way round 3 did for `#004`'s supposedly-already-reviewed fix. Do that here. Don't re-run round 2's exact scenario — read the code paths it exercised and hunt for what a less thorough pass might have missed.
- **2. `DetectDrift`'s exact-tier auto-repair path, live, through a real Webster fork.** Proven only at round 1's standalone-rig level (a hand-simulated out-of-band rename). No round has yet driven a REAL Webster fork that performs a symbol rename mid-plan (not as its own card's declared outcome, but as an incidental/out-of-band change during its turn) and watched `DetectDrift` auto-repair the rest of the plan live, through the real orchestrator. Seed a real hub-mode task where one card's fork is naturally likely to touch a symbol another pending card references, and observe.
- **3. A real hub-mode crash-kill test.** Round 3's crash-resilience proof (`kill -9` mid-batch, confirmed no wedge on resume) was standalone-mode only. Do the equivalent in HUB mode: a real `lyx loom run`, kill the driver process (and/or the tmux session hosting a live Master/fork) mid-Webster-batch on a glyph-bearing plan, confirm resume via `lyx loom run` continues cleanly rather than wedging. This directly extends the operator's own crash-resilience concern from round 3 into the mode loom itself actually uses.
- **4. A crash during a review segment (Bouncer/Burler round) on a glyph-bearing plan.** Never tested by any round. Kill the driver while a `Plan-Bouncer`/`Plan-Burler` or `Webster-Bouncer`/`Webster-Burler` round is live, confirm resume re-attaches or correctly respawns per `loom.md`'s documented crash-recovery ladder ("attach if live, else respawn — never both").
- **5. General adversarial sweep.** Anything a fourth pass across a third model and two effort tiers turns up that Opus(high)/Sonnet(xhigh)/Fable(high) didn't. Don't force new findings if there genuinely are none past the above — an honest "no new defects, ship it" is a valid and valuable outcome of a safety pass (see the campaign README's own guidance on this) — but earn that conclusion by actually trying the above, not by skipping to it.

## Explicitly OUT of scope for this round
- Windows path behavior — unreachable from this Linux host across all four rounds; do not reason about it as if driven.
- `quarry`'s own resolve/delta engine correctness — treat its answers as ground truth.
- Loom's general pre-glyph pipeline mechanics from the two PRE-glyph crucible campaigns — don't re-verify from scratch; DO flag if your live driving happens to expose a regression.
- Full `--plan-dir` override propagation into Master's in-pane verbs (standalone mode) and `lyx reed` standalone support — both explicitly deferred by round 3 as their own future module tasks, not this campaign's job to build. Flag only if genuinely broken beyond what's already recorded, don't attempt to build the deferred feature.
- `burlercli`'s standalone reed bring-up — wired but not live-verified by round 3 (no standalone burler scenario was in its scope); fair game for this round if you want a fifth live scenario, but not required.
- Any new feature or roadmap work.

## Round context seeded from prior-round verification

**Rounds 1 (opus-high-r1), 2 (sonnet-xhigh-r2), and 3 (fable-high-r3) are ALL CLOSED-AND-VERIFIED** — independently confirmed by this orchestrator, not self-reported. Full detail and verification evidence: `_mill/loom-review-HANDOFF.md`.

- **Round 1:** 22 findings (13 BLOCKING), 20 fixed. Root cause: whole-plan re-resolution against the post-change tree wedged every multi-batch plan with a Create/Delete/Rename card. 9/9 sabotage-proofs passed.
- **Round 2:** 7 findings (0 BLOCKING), all fixed. Proved a real hub-mode `lyx loom run` carries a glyph-bearing, `Plan-Write`-authored plan cleanly through `Webster-Review`. 2/2 sabotage-proofs passed. **This is the territory High-yield-focus item 1 above asks you to re-examine adversarially.**
- **Round 3 (two missions):** 12 findings (5 BLOCKING), all fixed. Closed the Rename-through-Webster gap live to `Finalize → done`; found and fixed 3 more layers of the standalone-webster fix (`#004`) that its own separate mill-go review missed; proved genuine crash resilience via a real `kill -9` mid-standalone-batch. 10/10 sabotage-proofs passed, including an independently-constructed crash test stronger than the round's own proof.

**RESIDUAL — this round's actual mission, see "High-yield focus" above:** items 1–4 are the specific untested territory; item 5 is the open-ended adversarial floor.

State the **merge bar**: correctness in the NORMAL single-instance flow, across every scenario above, is the gate. Do not chase artificial concurrency stress.

## Live-substrate cost declaration (loom IS an LLM-driving module)

**`LLM-DRIVING: yes.`** This round likely needs MULTIPLE real live-driving scenarios (items 1–4 above) — budget generously; this is the last round in the pre-approved rotation, so thoroughness here matters more than in any prior round.

- Each real hub-mode or standalone `lyx loom run`/`lyx webster run` spawns roughly one real `claude` subprocess at a time, strictly sequential (no cluster-fan configured anywhere in this campaign's scope) — 4–8 real sessions in series per full pipeline pass, 20–45 real minutes.
- **Check your PATH's `lyx` before ANY live driving** — prior rounds repeatedly found stale installed binaries; confirm `which lyx`/`lyx --version` reflects current HEAD, redeploy (`CGO_ENABLED=1 go run ./tools/deploy`) if not.
- **Run at most one full-pipeline attempt at a time**, foreground, waited on to completion. Never start two live runs concurrently.
- For the crash-kill scenarios (items 3–4): use a REAL `kill -9`, not a graceful stop, and confirm via `pgrep`/process inspection that the target was genuinely alive before the kill and genuinely dead after — a kill that races a process already finishing proves nothing. Confirm the absence of the run's own terminal artifact (e.g. `outcome.yaml`, a completion marker) as evidence the death was unclean, exactly as round 3's own verification did.
- **Report the exact `lyx reed status`/`lyx reed attach` commands every time you start a live run.**

**Named smoke tests.** Bare `-run Smoke` is BANNED. See prior rounds' seeds (recoverable from git history, e.g. `git show fa6f18167:_mill/loom-review-prompt.md`) for the full named-test list if you want to run any hermetic smoke tests; check each one's own subprocess cost before running regardless.

**EXECUTION BAN**: `internal/burlerengine/smoke_cluster_test.go`'s cluster-fan tests — 2 real subprocesses each, no cluster-fan configured anywhere in this campaign's scope.

- Never run more than one live-substrate invocation at a time, in parallel, or backgrounded.
- **The generic "N× CONCURRENT full smoke suites" gate does NOT apply, full stop.**

## What to TEST — do not just read, EXERCISE it

Hermetic (must stay green throughout):
- `CGO_ENABLED=1 go build ./...`
- `CGO_ENABLED=1 go vet ./internal/loomengine/... ./internal/loomcli/... ./internal/loomrecipe/... ./internal/loomshed/... ./internal/shedengine/... ./internal/shedadapters/... ./internal/shedrecipe/... ./internal/shedbuild/... ./internal/hubgeom/... ./internal/planparser/... ./internal/planglyph/... ./internal/shuttleengine/... ./internal/standalonegeom/... ./internal/webstercli/... ./internal/burlercli/... ./internal/logger/...`
- `CGO_ENABLED=1 go test -count=5` over the same set + `./cmd/lyx/...`
- `CGO_ENABLED=1 go test -tags integration ./internal/planglyph/... ./internal/planparser/... ./internal/websterengine/... ./internal/loomcli/... ./internal/loomshed/... ./internal/webstercli/... ./internal/burlercli/...`
- `CGO_ENABLED=1 go test ./...` (whole repo)

Live driving — YOU drive it directly, for every high-yield-focus scenario you attempt:
- Deploy: `CGO_ENABLED=1 go run ./tools/deploy` before every source change you want live-reflected; confirm PATH agreement.
- Set up whatever real hub/standalone fixtures each scenario needs (the sandbox hub from rounds 2/3, `/home/knatte/Code/lyx-test-HUB`, may still exist in a reusable state — check first, but a fresh fixture is also fine).
- **Report exact `lyx reed status`/`lyx reed attach` commands every time you start a session.**
- "Headless" means "no human required" — NOT "no time/token cost to you." Forbidden reasons to skip a scenario: "operator-assisted", "cost-bearing", "long-running", "impractical".

TEARDOWN DISCIPLINE (critical): confirm ZERO stray substrate processes at the end of every scenario and again at the very end (`ps aux | grep -iE 'tmux|lyx|claude'`, scoped to what YOU started). Be honest about what you could NOT verify and why.

## How to judge each finding
`file:line`, concrete failure scenario, severity (BLOCKING/MEDIUM/LOW/NIT), suggested fix, CONFIRMED vs PLAUSIBLE. Severity affects reporting, not whether you fix it — fix everything, all severities, including NIT. A genuinely LARGE fix gets marked NOT-FIXED-THIS-ROUND with full reasoning; the orchestrator spins it into its own mill-wiki task.

## Deferred items from the prior round — RE-EVALUATE these (after your own pass)
None requiring re-evaluation as "still open questions" — everything round 3 deferred is a deliberate, recorded future-task deferral (see "Explicitly OUT of scope" above), not an unresolved item this round needs to revisit.

## Fixing — after the review
- Fix EVERY finding, all severities including NIT.
- Load `/code-quality` and `mill:golang-build`/`mill:golang-testing`/`mill:golang-comments` before editing.
- For every bug you fix, add or extend a test that would have caught it; for a live-only defect, a `//go:build smoke` test walking the real scenario.
- MAKE SMOKE TESTS DETERMINISTIC — poll with a deadline, never sleep a fixed amount.
- Update the relevant docs in the SAME change as the fix. Do NOT add bugfix/hardening notes to `manifest/roadmap.md`.
- Keep gates green after every change; redeploy and re-verify live scenarios.
- Tear down all substrate state; confirm zero stray processes. Commit each fix — do NOT push.

## Deliverables
1. A structured review report: executive summary with an EXPLICIT convergence verdict (not just merge-readiness — does the campaign as a whole appear converged, per the README's own bar: a safety pass + this orchestrator's gates + an operator-assisted check all agreeing); which of the 5 high-yield-focus items were attempted and how far each got; findings severity-ranked with file:line/scenario/fix/CONFIRMED-PLAUSIBLE; what-was-tested with exact commands. Write to `_mill/loom-review-<yourtag>.md`, commit incrementally.
2. A fixer report: implemented/deferred/tests/changed-files. Write to `_mill/loom-review-<yourtag>-fixer-report.md`.
3. Final chat message: concise summary + severity counts + report paths + explicit merge-readiness AND convergence verdict + per-high-yield-focus-item yes/no on what was achieved. Also state, explicitly, this campaign's own honest limits (per the README's "state the limits" guidance) — e.g. Windows never reachable, anything the 5 focus items still didn't manage to drive even this round.

Begin with the clean-room review, produce your independent findings, then implement and verify the fixes.
