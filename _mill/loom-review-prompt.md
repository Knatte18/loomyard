# `loom` — independent review + fix (prompt template) — ROUND 7 (glyph-hardening campaign) — SAFETY PASS + FIRST REVIEW OF THE CLIWIRE MERGE

> Filled instance of `crucible/review-prompt-template.md` for round 7 of the campaign scoped to the quarry-glyph-plan-alphabet surface (GitHub PR #230), the standalone-webster fix (`#004`) it spun off, AND the `unify-webster-burler-wiring` consolidation (commit `7e54cc280`) that landed into this same branch after round 6 closed. See [../../crucible/README.md](../../crucible/README.md) for the loop, [../../_mill/loom-crucible-orchestrator-kickoff.md](loom-crucible-orchestrator-kickoff.md) for the campaign charter, and [loom-review-HANDOFF.md](loom-review-HANDOFF.md) for the campaign's full running state (read the handoff only AFTER you have your own independent findings list — see "Clean-room review constraint" below).
>
> **This round exists for two reasons.** First: round 6 was NOT a clean safety pass. It found and fixed 2 more genuine BLOCKING bugs (R6-1, R6-3) — one of them (R6-1) in the exact code round 5 shipped to close ITS OWN two BLOCKING findings. Rounds 4, 5, and 6 now form an unbroken run of three consecutive rounds each finding real BLOCKING material — the campaign's own stated bar for convergence ("a safety pass finding nothing severe") has not been met once in the last three rounds. Second: after round 6 closed, a SEPARATE mill task (`unify-webster-burler-wiring`) squash-merged into THIS branch's own tree as commit `7e54cc280` — a new package, `internal/cliwire`, that absorbs and rewrites the exact wiring logic six of round 6's own findings (R6-7, R6-8, R6-9, R6-15, R6-16, R6-17) diagnosed as chronically drift-prone. **That merge has never been through a crucible round.** Same precedent as `#004` in round 3: code that lands inside this branch's own tree is this campaign's material to harden before merge, regardless of which mill task produced it.
>
> **Effort note:** this round runs on **Fable/high** — the operator's own explicit, deliberate choice, this time for MODEL diversity rather than effort diversity. Opus has now run four times in this campaign (r1/high, r4/high, r5/medium, r6/medium); the fresh-model-angle argument raised after round 5 ("Sonnet or Fable, higher effort") was noted in the handoff but not taken up until now. Round 3 was this campaign's only prior Fable deployment (fable-high-r3) and it found 2 real BLOCKING defects in territory two prior rounds had already "closed" — treat that as this round's own precedent for what a fresh model can still find in reviewed ground.

You are a senior engineer doing a COMPLETE, adversarial, INDEPENDENT review of the `loom` module (including the standalone-webster material `#004` and the `unify-webster-burler-wiring` consolidation, both merged into this same branch) in the loomyard repo, followed by FIXING what you find.

## Where to work — determine this yourself, do not assume a path
Work in the current git worktree. Confirm it yourself at the start (`git rev-parse --show-toplevel`, `git branch --show-current`) rather than trusting any hardcoded path in this file or in a prior round's report — this campaign has already run across multiple host/user environments. The branch is `crucible-loom-glyph-hardening`. If a live-driving fixture (a sandbox hub) is needed, do not assume one from a prior round still exists on disk where its own report says it does — check first (`gh repo view Knatte18/lyx-test`, then look for a reusable branch).

**Durable method lesson from round 5, BLOCKING for any live scenario you drive this round:** a fixture whose repository path has EVER hosted an interactive `claude` session (including one you yourself drove earlier in this same round, or any prior round's `r4-*`/`r5sandbox*` fixture) is permanently immunized against the whole class of defect round 5 found — claude's one-time startup gates simply do not render there again. If you attempt ANY live scenario that touches Master/fork startup, you MUST build it from a repository path claude has never seen (a brand-new `lyx fabric clone` + `lyx fabric add`, or a brand-new standalone target directory) — reusing `r4-crash-hub`, `r4-drift-hub`, `r5sandbox`, or `r5sandbox2` for that purpose proves nothing.

## Your two jobs, in order
1. REVIEW: form your own independent judgment of the whole surface's correctness — not just the residuals prior rounds named, but a genuine fresh pass over everything, including territory prior rounds already "closed" AND the brand-new `internal/cliwire` package.
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
Do NOT read any prior review or review-dialogue files before you have your own list — specifically do not open anything under `_mill/` matching `loom-review-*` in THIS worktree, including all six prior rounds' review/fixer reports AND this campaign's running `loom-review-HANDOFF.md`. This is a FILENAME PATTERN, not a content judgment.
AFTER you have your own independent findings, you MAY (and should) consult the prior rounds' material and the handoff to (a) confirm the prior fixes have not regressed and (b) understand this round's specific mission below.
Reading the design SPEC and the module docs is expected and required (those are not reviews).

## What to read
- Code — loom's own machinery: `internal/loomengine/**`, `internal/loomcli/**`, `internal/loomrecipe/**`, `internal/loomshed/**`, `internal/shedengine/**`, `internal/shedadapters/**`, `internal/shedrecipe/**`, `internal/shedbuild/**`, `internal/hubgeom/**`, `contracts/recipes/loom-recipe.yaml`, `cmd/lyx`'s loom integration.
- The glyph surface: `internal/planparser/**`, `internal/planglyph/**` (six rounds of fixes now — read the CURRENT state, not any prior round's description of it), `internal/websterengine/**` (`beginbatch.go`, `recordbatch.go`, `fingerprint.go`, `render.go`, `runlevel.go` all changed across rounds).
- The standalone-webster material (`#004`, hardened further by rounds 3 and 4): `internal/shuttleengine/run.go` (`NewRunner`/`NewDetachedRunner`), `internal/standalonegeom/**`, `internal/standalonestate/**`, `internal/logger/sink.go`.
- **Brand-new, never-reviewed territory — `unify-webster-burler-wiring` (`7e54cc280`), read this adversarially, it has never been through a crucible round:** `internal/cliwire/module.go`, `internal/cliwire/paths.go`, `internal/cliwire/standalone.go`, `internal/cliwire/doc.go`, `internal/cliwire/cliwire_test.go`, `internal/cliwire/bannedecl_enforcement_test.go`, `internal/cliwire/callerset_enforcement_test.go`, plus the now-much-thinner `internal/webstercli/wiring.go` and `internal/burlercli/wiring.go` (both now delegate into `cliwire.Module`/`ResolveStandalone` instead of hand-duplicating the logic). Read `CONSTRAINTS.md`'s new "Cliwire Sole-Wiring Invariant" (quoted in full below) and confirm the code actually satisfies it, not just that a test exists claiming it does. This package's whole reason for existing is to close six round-6 findings (R6-7, R6-8, R6-9, R6-15, R6-16, R6-17) — all six were instances of the SAME shape ("a rule correct for the case it was written against, wrong in general") duplicated between `webstercli` and `burlercli`. Verify the consolidation actually preserves every one of those six guarantees for BOTH callers, not just one:
  - R6-7: `--target-dir` is stat'd and refused if absent/non-directory.
  - R6-8: hub mode refuses a moved `--plan-dir` the same way standalone does.
  - R6-9: the missing-plan refusal's recourse is verb-aware (default location for `run`, `--plan-dir` for read-only verbs).
  - R6-15: the nested-geometry guard and the plan-dir override check both compare NORMALIZED paths (symlinks resolved), not raw strings.
  - R6-16: `ShouldAbort` is checked before any wiring-specific flag validation.
  - R6-17: `--profile` (burler) resolves through the seam cwd, not the process cwd.

  `CONSTRAINTS.md`'s "Cliwire Sole-Wiring Invariant" (current text, for reference — read the live file, this may have drifted):
  ```
  internal/cliwire is the sole owner of standalone/hub CLI wiring resolution for the standalone-capable CLIs.
  - A <module>cli never re-implements --target-dir resolution, the repository-root lift, mode-derived
    state/plan/stencils resolution, the nested-geometry guard, or the durable-sink redirect; it declares
    its own cliwire.Module descriptor and calls in.
  - internal/cliwire is the only production caller of standalonestate.Derive, while test files may call it
    to build fixtures and to assert the real derivation.
  - Both halves are enforced by tests in internal/cliwire.
  ```
  Adversarial angle: does `bannedecl_enforcement_test.go` actually catch a `webstercli`/`burlercli` function that reintroduces the banned duplication, or does it only check for specific old function NAMES (i.e. trivially defeated by a rename)? Does `callerset_enforcement_test.go` actually enumerate every production caller of `standalonestate.Derive`, or could a new caller slip past it (e.g. one added inside a test-helper file that the check misclassifies as non-production)? Is `webstercli`'s webster-specific plan-directory layout (explicitly carried as "a function value" per `cliwire/doc.go`'s design rationale, since `internal/planparser` is deliberately excluded from `cliwire`) actually wired correctly for BOTH the hub and standalone paths, or did something get lost translating six inline call sites into one shared entry point?
- **The provider-startup seam, opened by round 5, re-hardened by round 6 — read it adversarially again, it has now produced BLOCKING findings twice in a row (R5-2/R5-7, then R6-1):** `internal/shuttleengine/claudeengine/startup.go` (`Startup`, `TrustDismissSequence`, `locateGateLines`, `startupGateNeedles`, `gateAcceptNeedles`), `internal/shuttleengine/claudeengine/doc.go`, `internal/shuttleengine/wait.go`, `internal/shuttleengine/engine.go`. Round 6's fix requires POSITIVE evidence a dialog is rendered (a locatable accepting-option line, or claude's own gate footer) before classifying a gate at all — is THAT rule itself still correct in general, or does it have its own "correct for the case it was written against" blind spot? What happens if claude's gate footer text changes, or if an accepting-option line legitimately appears in ordinary agent prose (not just the gate-needle phrases R6-1 covered)?
- Docs: `manifest/designs/quarry-glyph-plan-alphabet.md`, `manifest/designs/loom.md`, `manifest/designs/shed.md`, `contracts/specs/loom-plan-spec.md`, `contracts/stencils/loom/**`, `contracts/stencils/webster/webster-template-master.md`, `docs/overview.md`, `manifest/roadmap.md`, `CONSTRAINTS.md`, `README.md`.
- All six prior rounds' material (read AFTER your own findings list): `_mill/loom-review-opus5-high-r1.md`/`-fixer-report.md`, `_mill/loom-review-sonnet5-xhigh-r2.md`/`-fixer-report.md`, `_mill/loom-review-fable5-high-r3.md`/`-fixer-report.md`, `_mill/loom-review-opus5-high-r4.md`/`-fixer-report.md`, `_mill/loom-review-opus-medium-r5.md`/`-fixer-report.md`, `_mill/loom-review-opus-medium-r6.md`/`-fixer-report.md`, `_mill/loom-review-HANDOFF.md` (the full campaign record).
- Repo rules: `CLAUDE.md` (root + `~/.claude/CLAUDE.md`) and `CONSTRAINTS.md` in full.

## Mission (be genuinely adversarial — try to find nothing, and earn that)

Two axes, applied to the WHOLE surface:

1. **Scope/integration** — does everything actually work as designed, end to end, under conditions no prior round has tried?
2. **Correctness** — bugs, races, error handling, edge cases — including in code six prior rounds already "fixed," AND in the brand-new `cliwire` consolidation that has never been reviewed at all. A fix that passed sabotage-proofing is proven correct for the SPECIFIC scenario it was tested against; it is not proven correct in general. Read fixed code as skeptically as new code.

## High-yield focus

- **1. First-ever adversarial review of the `unify-webster-burler-wiring` merge (`7e54cc280`).** This is genuinely unreviewed code, landed via a different mill task's own pipeline, not via this campaign's own discipline — same category as `#004` in round 3. See "What to read" above for the specific six guarantees to verify survived consolidation, plus the two adversarial angles on the enforcement tests themselves. Drive it live if you can: a standalone `lyx webster run`/`lyx burler run` with a typo'd `--target-dir`, a moved `--plan-dir` in BOTH hub and standalone mode, a symlinked state home — confirm each still refuses/behaves exactly as R6-7/8/9/15/16/17 established, now through the new shared path.
- **2. General adversarial sweep, genuinely trying to find nothing, over everything else.** Read the whole surface fresh. If, after genuinely trying, you find nothing — that is a valid and valuable outcome (see the campaign README's own guidance) — but earn it; don't skip to it.
- **3. Close the two coverage gaps this orchestrator's own round-6 verification surfaced (not correctness bugs, but real gaps — see "Deferred items" below for the specifics).**
- **4. Adversarially re-examine the provider-startup seam once more (`claudeengine/startup.go`, `wait.go`, `engine.go`).** It has produced BLOCKING findings in TWO consecutive rounds now (round 5, then round 6 in the round-5 fix itself). Does round 6's own fix have an analogous "correct for the case it was written against" gap? See "What to read" above for the specific angle. If you drive this live, it MUST be on a fixture claude has never seen — see "Where to work" above.
- **5. Whatever a seventh pass, on this campaign's SECOND deployment of Fable (after round 3's own 2-BLOCKING-finding result), turns up that six prior passes (Opus×4, Sonnet, Fable×1) didn't.** Don't force it. If the answer is genuinely "nothing new," that is this round's most valuable possible outcome — say so plainly and back it with the same rigor as a findings list.

## Explicitly OUT of scope for this round
- Windows path behavior — unreachable from this Linux host across all seven rounds; do not reason about it as if driven.
- `quarry`'s own resolve/delta engine correctness — treat its answers as ground truth.
- Loom's general pre-glyph pipeline mechanics from the two PRE-glyph crucible campaigns — don't re-verify from scratch; DO flag if your live driving happens to expose a regression. (This does NOT cover `internal/cliwire` — that package is explicitly IN scope this round, see High-yield focus item 1, even though some of the code it replaces predates the glyph campaign.)
- The ~45-item residue from round 6's sweep over loom's pre-glyph pipeline machinery (doc drift, non-blocking correctness items unrelated to the glyph/cliwire surface) — this orchestrator's job to spin into its own mill-wiki task, not this round's to fix.
- Full `--plan-dir` override propagation into Master's in-pane verbs (standalone mode) and `lyx reed` standalone support — both explicitly deferred by round 3 as their own future module tasks. Flag only if genuinely broken beyond what's already recorded.
- `burlercli`'s standalone reed bring-up — wired but still not live-verified by any round (no standalone burler scenario has been in scope yet); fair game if you want an extra live scenario, not required.
- Any new feature or roadmap work.

## Round context seeded from prior-round verification

**Rounds 1 through 6 are ALL CLOSED-AND-VERIFIED** — independently confirmed by this orchestrator, not self-reported. Full detail and verification evidence: `_mill/loom-review-HANDOFF.md`.

- **Round 1:** 22 findings (13 BLOCKING), 20 fixed. Root cause: whole-plan re-resolution against the post-change tree wedged every multi-batch plan with a Create/Delete/Rename card.
- **Round 2:** 7 findings (0 BLOCKING), all fixed. Proved a real hub-mode `lyx loom run` carries a glyph-bearing, `Plan-Write`-authored plan cleanly through `Webster-Review`.
- **Round 3:** 12 findings (5 BLOCKING), all fixed. Closed the Rename-through-Webster gap live to `Finalize → done`; found and fixed 3 more layers of the standalone-webster fix (`#004`); proved genuine crash resilience via a real `kill -9` mid-standalone-batch.
- **Round 4:** 38 findings (2 BLOCKING), 36 fixed. Fixed the last 2 fingerprint-restamp wedge bugs (root-caused in round 1, missed by rounds 1–3); achieved the first-ever live `DetectDrift` exact-tier auto-repair through a real Webster fork; ran real `kill -9` hub-mode crash tests.
- **Round 5:** 7 findings (2 BLOCKING), all fixed. `claudeengine.TrustDismissSequence` confirmed claude's trust gate's REFUSING option, killing every agent spawned in a not-yet-trusted directory; claude's Bypass-Permissions modal wasn't a recognized startup gate and silently disabled the startup deadline. Both on the module's single hottest live path.
- **Round 6:** 28 findings (2 BLOCKING), all fixed. **R6-1**: round 5's own fix, read from the other side — the needle set matched against the WHOLE pane capture with no requirement a gate actually be rendered, so a healthy pane whose transcript happened to contain gate-like phrasing got keys pressed into it mid-turn. **R6-3**: `containment-file-overlap` indexed read-only `Uses:` refs alongside `Targets`, so two cards that merely READ overlapping things blocked `lyx webster run` outright. Plus 9 MEDIUM, 10 LOW, 6 NIT, including the six wiring-duplication findings (R6-7/8/9/15/16/17) that motivated the `unify-webster-burler-wiring` consolidation this round now reviews for the first time.

**Since round 6 closed:** `unify-webster-burler-wiring` squash-merged as `7e54cc280` — new package `internal/cliwire`, `webstercli`/`burlercli` wiring rewritten to delegate into it, new `CONSTRAINTS.md` invariant with mechanical enforcement. THIS IS THIS ROUND'S PRIMARY NEW MATERIAL — see High-yield focus item 1.

**Convergence status: NOT MET.** Three consecutive rounds (4, 5, 6) have each found genuine BLOCKING material — the campaign's own bar for convergence (a safety pass finding nothing severe) has not been hit once in that span. This round is not being seeded as a formality safety pass; it is a genuine adversarial pass over both old and brand-new territory, on a different model than the last three rounds.

State the **merge bar**: correctness in the NORMAL single-instance flow, across every scenario above, is the gate. Do not chase artificial concurrency stress.

## Live-substrate cost declaration (loom IS an LLM-driving module)

**`LLM-DRIVING: yes, conditionally.`** This round's high-yield-focus items 1 and 4 may or may not need live driving depending on what you find reading the code adversarially first — a genuine defect found by reading may not need a live reproduction to fix and test. If your sweep turns up something needing live confirmation:

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
- `CGO_ENABLED=1 go test -tags integration ./internal/planglyph/... ./internal/planparser/... ./internal/websterengine/... ./internal/loomcli/... ./internal/loomshed/... ./internal/webstercli/... ./internal/burlercli/... ./internal/cliwire/...`
- `CGO_ENABLED=1 go test ./...` (whole repo)

Live driving — YOU drive it directly, for every high-yield-focus scenario that turns out to need it:
- Deploy: `CGO_ENABLED=1 go run ./tools/deploy` before every source change you want live-reflected; confirm PATH agreement.
- Set up whatever real hub/standalone fixtures each scenario needs — ALWAYS a fresh, never-hand-driven one if the scenario touches Master/fork startup (see "Where to work" above).
- **Report exact `lyx reed status`/`lyx reed attach` commands every time you start a session.**
- "Headless" means "no human required" — NOT "no time/token cost to you." Forbidden reasons to skip a scenario: "operator-assisted", "cost-bearing", "long-running", "impractical". If your session's own permission classifier genuinely refuses `tmux`/`reed`/`deploy` outright (this happened to round 6), say so PLAINLY and explicitly, exactly as round 6 did — do not silently skip live driving and do not claim it as done.

TEARDOWN DISCIPLINE (critical): confirm ZERO stray substrate processes at the end of every scenario and again at the very end (`ps aux | grep -iE 'tmux|lyx|claude'`, scoped to what YOU started). Be honest about what you could NOT verify and why.

## How to judge each finding
`file:line`, concrete failure scenario, severity (BLOCKING/MEDIUM/LOW/NIT), suggested fix, CONFIRMED vs PLAUSIBLE. Severity affects reporting, not whether you fix it — fix everything, all severities, including NIT. A genuinely LARGE fix gets marked NOT-FIXED-THIS-ROUND with full reasoning; the orchestrator spins it into its own mill-wiki task.

## Deferred items from the prior round — RE-EVALUATE these (after your own pass)
- **R6-6's regression test doesn't cover its own call site.** `TestIsLyxWorktree_GatesTheCwdAnchoredFallback` (`internal/logger/sink_test.go`) exercises the `isLyxWorktree` helper directly, never the actual `armDurableSinkLocked` call site the fix lives in. The fix itself was confirmed correct by direct code reading, but this is a real coverage gap — worth closing now, especially since the `cliwire` merge touches adjacent wiring territory (the durable-sink redirect is one of the five things `cliwire.ResolveStandalone` now owns per its own doc comment).
- **R6-27's `loomcli` twin has no dedicated regression test.** `planFindingsHaveBlocking` (`internal/loomcli/validate.go`) got the identical fail-closed fix as `loomshed`'s `hasBlockingFinding`, verified only by reading that the two are identical — no test pins `loomcli`'s half the way `TestHasBlockingFinding_UnrecognizedSeverityFailsClosed` pins `loomshed`'s.
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
1. A structured review report: executive summary with an EXPLICIT convergence verdict (not just merge-readiness — does the campaign as a whole appear converged, per the README's own bar: a safety pass + this orchestrator's gates + an operator-assisted check all agreeing); which high-yield-focus items were attempted and how far each got; findings severity-ranked with file:line/scenario/fix/CONFIRMED-PLAUSIBLE; what-was-tested with exact commands. Write to `_mill/loom-review-<yourtag>.md`, commit incrementally.
2. A fixer report: implemented/deferred/tests/changed-files, table complete and accurate. Write to `_mill/loom-review-<yourtag>-fixer-report.md`.
3. Final chat message: concise summary + severity counts + report paths + explicit merge-readiness AND convergence verdict + per-high-yield-focus-item yes/no on what was achieved. Also state, explicitly, this campaign's own honest limits (per the README's "state the limits" guidance) — e.g. Windows never reachable, anything this round still didn't manage to drive.

Begin with the clean-room review, produce your independent findings, then implement and verify the fixes.
