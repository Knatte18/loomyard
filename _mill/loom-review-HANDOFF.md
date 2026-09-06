# `loom` crucible campaign — glyph-hardening — HANDOFF

> Refreshed after every round's verification, per `crucible/orchestrator-prompt.md`'s hygiene section. If this session's context resets, a fresh orchestrator should read this file first, then `_mill/loom-crucible-orchestrator-kickoff.md` for the campaign charter.

## Campaign identity

- Worktree: `/home/knatte/Code/loomyard/wts/crucible-loom-glyph-hardening`, branch `crucible-loom-glyph-hardening`, parent `main`.
- Mission: harden `loom` against the newly-landed quarry-glyph-plan-alphabet surface (PR #230) — never before exercised through loom's real phase machine, only through the landing task's own unit/integration suite.
- Model/effort rotation, pre-approved by the operator for up to four rounds unless it converges sooner: **Opus/high → Sonnet/xhigh → Fable/high → Opus/high** (final safety pass).

## Current state

**Round running right now: NONE.** Round 1 (`opus-high-r1`) completed and reported its own "MERGE-READY" verdict. **That self-verdict is NOT yet trusted** — independent verification is IN PROGRESS (a verification fork launched by this orchestrator; agent id not durable, identify by: it's sabotage-proving the round's regression tests one at a time, restoring after each).

**Do not treat round 1 as closed until this handoff is updated with a verification outcome below.**

- HEAD as of round 1's own last commit: `93b0d3a95` ("loom: fixer report and post-fix live re-verification for round 1"), 2026-09-06 19:18:38 +0200.
- **Working tree is NOT currently clean** — `internal/planglyph/handle.go` shows as modified. This is expected: the verification fork is mid-sabotage-cycle (revert a production hunk → confirm the regression test fails → restore → confirm empty diff) for finding F9 (`renameSignature`/`draftHandleIdentifier`, commit `221dad17a`). **Do not `git add -A`, `git commit`, `git stash`, or otherwise touch the working tree while this is in flight** — the fork will restore it itself. If you find this file still dirty long after the fork should have finished, that's itself a problem worth investigating (a crashed sabotage cycle that never restored), not something to silently clean up — diff it against `git show 221dad17a` to see exactly what's reverted before deciding what to do.

## Round 1 (opus-high-r1) — self-reported, pending independent confirmation

**Timing:** 78.4 min wall-clock, 364 tool calls, ~564k tokens. Job 1 (review): ~31 min (18:02–18:33). Job 2 (fix): ~34 min (18:33–19:06). Post-fix re-verification + reports: ~12 min (19:06–19:19).

**Findings:** 22 total (13 BLOCKING, 5 MEDIUM, 3 LOW, 1 NIT). 20 fixed across 14 commits (one commit per finding-cluster, `c2b64e001`..`9726310b6`, plus doc-only `9726310b6`). 2 recorded, deliberately NOT fixed: **F16** (standalone `lyx webster run` can't start Master — anchor-outside-worktree-root refusal in `standalonegeom`) and **F22** (standalone mode writes untracked `.lyx/logs/` into the target repo) — both claimed out of loom's own module scope (loom always runs hub mode, never standalone), recommended as their own webster/standalonegeom-scoped task.

**Headline:** the integration surface (not the glyph mechanics themselves, which tested fine in isolation) was broken hard — no multi-batch plan with a `Create`/`Delete`/`Rename` card could ever complete through Webster, for two independent reasons (F3/F4) plus a structural gap with three manifestations (F5/F6/F7, later found to also cover F21). Post-fix, a three-batch plan with a handle-`Create` and a declared `Rename` ran end-to-end for the first time ever (review report's "Run 8").

**One design-shaped change flagged by the round itself:** commit `c6ee9eb16` (fixes F5/F6/F7/F21) adds two new exported `internal/planglyph` functions (`ValidateDispatch`, `PendingPlan`) and re-scopes the dispatch-boundary re-resolution to pending cards only. Not a surgical one-liner — the round called this out explicitly per Hard Rule 5's "size line" and argued it's still a scoped bugfix, not a subsystem addition. **My own judgment on whether this crosses the size line is pending the verification fork's read of the diff** — see "Next action" below.

**Full detail:** `_mill/loom-review-opus5-high-r1.md` (review report — findings, live-scenario transcripts, per-scenario verdict table) and `_mill/loom-review-opus5-high-r1-fixer-report.md` (fixer report — what was implemented, tests added, changed files).

**What the round could NOT verify, honestly stated by the round itself (not yet independently checked by me):** a full `lyx loom run` with real LLM sessions through the whole 17-row recipe, blocked by three compounding environment gaps on this host (standalone Master won't start — F16; no local hub has a Go warp repo; the `lyx` on PATH predates the glyph landing). Mitigation claimed: Plan-Validate/Plan-Revalidate and `lyx webster validate` call the identical `planglyph` functions the round drove directly, per the Gate Self-Check Parity Invariant — so the rows *above* Webster (a real `Plan-Write` session actually emitting a legal handle declaration, `Plan-Review` judging one) remain proven only at the mechanical layer.

## Verification IN PROGRESS — what it's checking

A fork (launched this session, not yet returned) is:
1. Sabotage-proving the regression tests behind all 13 BLOCKING findings' commits (8 sabotage cycles covering `c6ee9eb16`, `cb28973b2`, `5bf342d87`, `92cf21c93`, `c2b64e001`, `482079234`, `221dad17a`, `129329717`, plus one MEDIUM/LOW spot-check on `ceeaf38ba`).
2. Re-running every hermetic/integration gate cold, plus exactly one named smoke test (`TestSmokeBootstrap_CleanlinessOrderingAfterSeedCommit` — cheap, zero real subprocesses).
3. Giving an independent judgment on whether `c6ee9eb16` crosses Hard Rule 5's size line.
4. Checking F16/F22's "out of loom's scope" claims against the actual `standalonegeom`/`fabricengine`/`loomcli` code, not just trusting the fixer report's assertion.
5. Confirming the doc fixes (F13/F15/F19 — `loom-template-plan.md`, `loom-plan-spec.md`, `loom.md`) actually landed with correct text, and independently assessing whether `c6ee9eb16`'s new functions need a `CONSTRAINTS.md` invariant the round claims they don't.
6. A teardown check (stray tmux/claude/lyx processes) as of right now.

## Next action (exact instruction for whoever picks this up)

1. Wait for the verification fork's report. Do NOT re-read its raw transcript — read only its final structured report.
2. If every sabotage cycle came back PASS (test failed correctly under sabotage, restored clean), the gates are all green cold, `c6ee9eb16` is judged in-bounds, F16/F22's scope claims hold up, docs are confirmed correct, and teardown is clean: round 1 is CLOSED-AND-VERIFIED. Re-seed `_mill/loom-review-prompt.md` for round 2 as a **safety pass** (Sonnet/xhigh per the pre-approved rotation) — no known residual, confirm merge-readiness or find what round 1 missed — and list round 1's 20 fixes as CLOSED-AND-VERIFIED so round 2 doesn't re-litigate them. Also decide whether F16/F22 warrant their own mill-wiki task now (per Hard Rule 5, this orchestrator opens it through normal mill flow, never by hand-editing wiki files) — likely yes, they're real shipped defects, just not loom's.
3. If ANY sabotage cycle came back FAIL or INCONCLUSIVE, or a gate is red, or `c6ee9eb16` is judged oversized, or F16/F22's scope claim doesn't hold: do NOT advance to a safety pass. Re-seed round 2 with the SPECIFIC residual the verification found (file/scenario + fix-the-right-layer instruction), still rotating to Sonnet/xhigh per the pre-approved model list.
4. Either way: commit this handoff's next refresh, and commit the re-seeded `_mill/loom-review-prompt.md` BEFORE spawning round 2 (never after).
5. Rounds remaining under the operator's pre-approved rotation after round 1: Sonnet/xhigh, Fable/high, Opus/high (final safety pass) — max four total unless it converges sooner. If round 2 (whichever seed) converges clean and an operator-assisted check agrees, the campaign can close before using all four.
