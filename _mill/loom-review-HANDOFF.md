# `loom` crucible — round 2 handoff

## What is running right now

Nothing. Round 2 (`opus-high-r1`, Opus/high, tag `opus-high-r1`) finished and was independently
verified by the orchestrator. No round is currently spawned.

## CLOSED-AND-VERIFIED (commit sha, orchestrator-verified, not just self-reported)

Round 2 found 15 findings (2 BLOCKING, 5 MEDIUM, 7 LOW, 1 NIT), fixed 12, deferred 3 (plus three
half-findings inside F1/F2/F12). All committed on `loom-crucible-hardening-round2`, nothing pushed.

- **F12 (BLOCKING)** `b8fa9239` — `Finalize`/`Publish` now commit loom's own status file
  (`landingshed.Deps.CommitStatus`) before merging, closing the defect that made the last row of
  *every* loom run refuse deterministically after every session in the run had been paid for.
  Orchestrator-verified: read the diff, reverted the `Finalize` commit-call, confirmed
  `TestFinalize_CommitStatus_RunsBeforeTheMergeIn`/`_FailureIsAnErrorAndNeverMerges` fail at the
  intended assertion, restored, confirmed green + empty diff.
- **F0 (BLOCKING)** `f1b30b51` — `BurlerProducer` and both `Bouncer` spawn paths now probe
  `shuttleengine.Attach` before archiving/respawning, closing a duplicate-live-agent defect on a
  driver crash mid review-segment (reproduced on `Discussion-Burler` and `Webster-Burler`, the
  latter with two agents holding commit authority over one branch).
  Orchestrator-verified: reverted the `BurlerProducer.Call` probe call, confirmed
  `TestBurlerProducer_AttachesToLiveRoundInsteadOfRespawning` fails at all four intended
  assertions, restored, confirmed green.
- F10, F1 (narrow half), F2 (message half), F14, F4, F8, F3, F6, F7, F9, F5 — fixed, one commit
  each; read but not individually sabotage-proven by the orchestrator (see "What this pass did NOT
  do" below).
- **Independently re-verified from a cold state by the orchestrator** (not trusting the round's own
  report): `go build ./...` clean; `go vet ./...` clean; `go test -count=5` on the ten module
  packages (incl. `landingshed`) + `cmd/lyx` all green; `go test ./...` whole repo, 78 packages
  `ok`, 0 `FAIL`; two named smoke tests re-run directly (`TestSmokeBurlerRound_...Respawning`,
  `TestSmokeBootstrap_DiedDriverProceedsToHandoverAndLogsWhy`), both green; zero stray tmux/claude
  processes before and after (provider count at the host's own baseline of 2, both pre-existing
  unrelated sessions).
- **The primary mission — a full live 17-row `lyx loom run`, Preflight through Finalize —
  independently confirmed real**, not just self-reported: read the actual git history of the
  isolated fixture hub the round left at `/home/knatte/Code/loomyard/live-r2/` (outside this repo,
  no contamination risk — confirmed the orchestrator's own worktree and `main` are untouched).
  Saw for real: `ac5e2d2 loom: discussion artifacts`, `3bd8cb1 loom: plan artifacts`,
  `e47278e webster: begin-batch 01-greet-trim` / `b222e11 ...done`, a real Webster-internal
  review/fix pass (`1b09ea7`/`5e4fa3c` fixing two of Webster's own review findings), `79a3df4 loom:
  status checkpoint` (the exact F12 fix commit named in the fixer report), and the final merge
  (`bea6c9a`/`8ba6e70`). The merge's own conflict marker on `_lyx/loom/status.json` is the live,
  independent confirmation that F13 (below) is a real, currently-open residual, not a
  hedge in the report.

## RESIDUAL — what round 3 (or a spun-off mill task) should pick up

Per crucible Hard Rule 5, none of these belong in a crucible round's inline fix loop — each is a
durable design decision reaching outside `loom` alone. **Orchestrator's job next: open one or more
proper mill-wiki tasks for these**, not spawn a bugfix-flavored crucible round:

1. **F13 (MEDIUM)** — loom's own status file is a merge subject on the landing merge, and now that
   F12 makes it get committed, it can genuinely conflict (confirmed live above). Interacts with
   F12's fix; read both together before scoping a fix (`merge=ours` driver, drop from git entirely,
   or exclude via pathspec — see the review report's own options).
2. **F11 (LOW)** — the mandated card shape is forced onto `Custom`; a real fix is a plan-format
   contract change reaching `planparser`, the plan stencil, the Plan-Review rubric, and Webster's
   consumption of `Targets`.
3. **F1's second half** — `lyx config reconcile --apply` truncates a list longer than the
   template's; fixing it changes the sequence merge model for every module's config.
4. **F2's second half** — a spawned agent should reach the binary that spawned it; belongs in the
   shuttle/reed spawn layer. Note for whoever takes it: prepending `.dev-bin` to PATH is *not*
   sufficient — measured live, the agent's shell re-sources the profile and reorders PATH back.
5. **F12's second half** — continuous status durability (a commit per producer transition) is what
   the documented cross-machine-resume feature actually needs; today only the seed and the new
   landing-row checkpoint are ever committed.

## What this verification pass did NOT do (state the limits, per the method's own discipline)

- Only the two BLOCKING fixes (F12, F0) were personally sabotage-proven by the orchestrator
  (revert-hunk → confirm intended test failure → restore). The ten MEDIUM/LOW/NIT fixes were read
  and judged plausible, and pass in aggregate under `go test ./...`, but were not each individually
  neutered and re-proven.
- Only 2 of the 12 named smoke tests were independently re-run by the orchestrator; the rest were
  trusted from the round's own reported table (cross-checked against exact matching timings, e.g.
  the F0 smoke test's reported 3.38s matched the orchestrator's own re-run to the hundredth).
- The live fixture hub at `/home/knatte/Code/loomyard/live-r2/` was read for evidence but not
  exhaustively audited end to end; the specific commits cited above were checked, not every commit
  in its history.
- No second model has yet run an independent pass over the newly-reachable late-pipeline territory
  (Plan-Write onward, Webster, Finalize) that no round before this one had ever exercised live —
  round 1 never reached it, and only Opus has now looked at it once.

## Suggested next action

Two independent threads, either or both:
- **Spin F13/F11/F1-half/F2-half/F12-half into mill-wiki task(s)** via the normal mill flow — not
  another crucible round.
- **Consider a round 3 safety pass with a rotated model** (Fable or Sonnet, operator's effort pick)
  specifically because this round covered genuinely new territory (the full live pipeline) for the
  first time ever with only one model's eyes on it — convergence across different models is
  materially stronger evidence than round 2 alone, per the method's own rationale.

The push/merge decision on round 2's own fixes (12 commits, `b8fa9239`..`e9bde678` plus the fixer
report commit `c3dfeb75`) is the operator's, not the orchestrator's.
