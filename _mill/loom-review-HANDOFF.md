# `loom` crucible campaign — glyph-hardening — HANDOFF

> Refreshed after every round's verification, per `crucible/orchestrator-prompt.md`'s hygiene section. If this session's context resets, a fresh orchestrator should read this file first, then `_mill/loom-crucible-orchestrator-kickoff.md` for the campaign charter.

## Campaign identity

- Worktree: `/home/knatte/Code/loomyard/wts/crucible-loom-glyph-hardening`, branch `crucible-loom-glyph-hardening`, parent `main`.
- Mission: harden `loom` against the newly-landed quarry-glyph-plan-alphabet surface (PR #230) — never before exercised through loom's real phase machine, only through the landing task's own unit/integration suite.
- Model/effort rotation, pre-approved by the operator for up to four rounds unless it converges sooner: **Opus/high (r1) → Sonnet/xhigh (r2) → Fable/high (r3) → Opus/high (r4, final safety pass)**.

## Current state

**Round 1 CLOSED-AND-VERIFIED. Round 2 CLOSED-AND-VERIFIED. Round 3 is BLOCKED on an operator sequencing gate — see "Blocking gate before round 3" below, not a technical residual.** Working tree is clean, HEAD is `fb29b6b22` (round 2's fixer-report commit). Nothing pushed.

## Blocking gate before round 3 (do not spawn round 3 until this clears)

**Operator's explicit instruction: do NOT spawn round 3 (Fable/high) until the `#004` `standalonegeom-webster-run-and-log-hygiene` mill task has LANDED (merged/finalized, not just past discussion).** That task lives in its own worktree (`/home/knatte/Code/loomyard/wts/standalonegeom-webster-run-and-log-hygiene`), spawned from this campaign's own findings (F16/F22, see round 1 below); as of this handoff it is mid `mill-start --orch` (an `orch-review.md` was written and consumed for discussion-review round 1; current phase unknown to this session — check its own `_mill/status.md` before assuming anything about its progress).
Reasoning given: round 2 ran on Sonnet, judged less capable than Opus/Fable, so before committing to round 3 the operator wants that external task's own outcome in hand first. **This gate is independent of round 2's own convergence** — round 2 finished clean (see below) and that does NOT clear this gate. Check the mill task's wiki entry / `status.md` for a landed state before spawning round 3; if genuinely unsure whether "landed" is satisfied, ask rather than assume.

## Round 1 (opus-high-r1) — CLOSED-AND-VERIFIED

**Timing:** 78.4 min wall-clock, 364 tool calls, ~564k tokens. Job 1 (review): ~31 min. Job 2 (fix): ~34 min. Post-fix re-verification + reports: ~12 min.

**Findings:** 22 total (13 BLOCKING, 5 MEDIUM, 3 LOW, 1 NIT). 20 fixed across 14 commits (`c2b64e001`..`9726310b6`). 2 recorded, deliberately NOT fixed: **F16** (standalone `lyx webster run` can't start Master — `shuttleengine.NewRunner`'s anchor/worktree containment check structurally refuses `standalonegeom`'s deliberately-divergent geometry) and **F22** (standalone mode writes untracked `.lyx/logs/` into the target repo) — both out of loom's own scope (loom never constructs `standalonegeom`, confirmed by code, not just asserted).

**Headline:** the glyph mechanics themselves tested fine in isolation, but the integration surface was broken hard — no multi-batch plan with a `Create`/`Delete`/`Rename` card could ever complete through Webster (root cause: whole-plan re-resolution against the post-change tree on every batch dispatch, F5/F6/F7/F21). Post-fix, a three-batch hand-authored plan with a handle-`Create` and a declared `Rename` ran end-to-end for the first time ever, via webster's zero-LLM-cost standalone probe harness (round 1 could not reach a real hub-mode `lyx loom run` — three environment gaps, two standalone-only).

**Independent verification (this orchestrator):** 9/9 sabotage-proofs passed (every BLOCKING finding's commit, plus one MEDIUM/LOW spot-check). All gates green cold. The one design-shaped change (`c6ee9eb16`, two new `internal/planglyph` exported functions `ValidateDispatch`/`PendingPlan`) confirmed correctly scoped — called only from the two sites it fixes, Gate Self-Check Parity Invariant's named functions provably unchanged. F16/F22 confirmed genuinely unreachable from loom by grep (`internal/loomcli`/`loomengine`/`loomshed`/`loomrecipe` never construct `standalonegeom`). All three doc fixes (F13/F15/F19) confirmed landed correctly. Teardown clean. **Nothing failed.**

**Mill-wiki task opened for F16/F22** (Hard Rule 5): `#004` `standalonegeom-webster-run-and-log-hygiene`, via the sanctioned `.millhouse/millpy-add` wrapper. Investigated on the operator's direct question ("are these really large enough for their own task?") — confirmed yes for F16 specifically: `shuttleengine.NewRunner`'s containment check is a shared safety invariant with FOUR callers outside webster (`burlercli`, `webstercli`, `shuttlecli`, `loomcli`), so relaxing it for standalone geometry is a genuine cross-module design decision, not an inline patch. Spawned as its own worktree; currently in progress (see "Blocking gate" above).

**Full detail:** `_mill/loom-review-opus5-high-r1.md`, `_mill/loom-review-opus5-high-r1-fixer-report.md`.

## Round 2 (sonnet-xhigh-r2) — CLOSED-AND-VERIFIED

**Mission:** round 1 proved every glyph mechanism through hand-authored plans and direct bracket-verb calls; round 2's job was to prove the same mechanics survive contact with a REAL orchestrator — a real `lyx loom run`, hub mode, real LLM sessions, a plan authored by a real `Plan-Write` session, not hand-edited.

**Result — this succeeded spectacularly.** The round set up a real disposable sandbox hub (`/home/knatte/Code/lyx-test-HUB`, warp repo `github.com/Knatte18/lyx-test`), fixed the stale-PATH-`lyx` hazard round 1 flagged (reinstalled from current HEAD to both `go/bin` and `.local/bin`), and drove a genuine `lyx loom run` all the way from `Preflight` through `Webster-Review` (APPROVED) with **zero glyph-surface defects blocking it anywhere** — the first time loom's real phase machine has ever carried a glyph-bearing plan this far — then reached `Publish`, which opened a real GitHub PR and correctly halted at that human-review gate (loom's own designed boundary, not a wedge).

**Findings:** 7 total, **0 BLOCKING**, 2 MEDIUM, 2 LOW, 3 NIT — all 7 fixed (`06977e1ca`..`0fcbfee98`, plus fixer-report commit `fb29b6b22`). All residual doc/stencil drift (stale mechanical-check counts, a terminology slip, a missing canonical check-ID list that nearly caused a live review round to mis-fire) plus one real Master-prompt gap (no scripted response to a round-1-introduced refusal shape) and one narrow format-checker widening. Two findings are code-shaped (F-parse1, F-webster1) with new tests; five are pure doc/stencil text.

**Independent verification (this orchestrator) — the real-PR/real-hub claim was the priority, per "prove the scenario reached the code":**
- `gh pr view 1 --repo Knatte18/lyx-test` confirmed a REAL PR: exact title/branch match, body is real Master-written content, `commits` field lists exactly the two SHAs the round claimed (`f235810`, `ed1263a`).
- The sandbox hub and its `glyph-demo-greet` worktree still exist on disk; both commits are real, on top of the real pre-existing HEAD, and their content matches the plan cards exactly (`FormatGreeting` function + table-driven test in one commit, `main()` wiring in the next).
- **One discrepancy found, now resolved:** `gh pr view` showed the PR as **CLOSED (unmerged)**, contradicting both reports' "remains open" claim. **Operator confirmed they closed it themselves** (deliberately not merged — the sandbox hub is disposable test infrastructure). Just a stale fact in both reports (closed mid-round, after the review-notes commit but before the final fixer-report commit, which never re-checked) — not a code defect, not a process failure, nothing further to investigate.
- 2/2 sabotage-proofs passed (F-parse1, F-webster1). All gates green cold (`go build`/`vet`/`test -count=5` on the 11 in-scope sets, `-tags integration` on 5 sets, whole-repo `go test` — 81 packages, 0 FAIL, `TestEnforcement_MarkdownLinks` green). All doc-fix text confirmed landed correctly, quoted. Teardown clean.

**Full detail:** `_mill/loom-review-sonnet5-xhigh-r2.md`, `_mill/loom-review-sonnet5-xhigh-r2-fixer-report.md`.

## What remains open / unproven after round 2

- A declared `Rename` card executing through a real Webster batch, live — round 2's real `Plan-Write` session correctly determined the given task couldn't be expressed as same-plan Create-then-Rename (now documented, F-plan1) and produced a Create-only plan instead. The Rename mechanic remains proven only at round 1's code/standalone-rig level plus round 2's live `Plan-Review` judge independently re-confirming the same constraint against source. A second live run seeded from a worktree with a pre-existing symbol to rename would close this gap (~20-40 min real wall-clock) — not attempted, judged non-essential given the triangulated evidence.
- `Finalize` — never reached (blocked on the human-gated PR).
- `DetectDrift`'s exact-tier auto-repair path specifically, live through a real Webster fork — not triggered this round (no out-of-band rename occurred); remains verified only at round 1's level.
- PR #1's actual disposition (see discrepancy above) — unresolved as of this handoff.

## Next action (exact instruction for whoever picks this up)

1. **Do not spawn round 3 yet — check the "Blocking gate before round 3" section above first.** If it has cleared (the `#004` mill task has landed), proceed to step 2. If not, wait and re-check periodically; this is not something to work around.
2. When clear: re-seed `_mill/loom-review-prompt.md` for round 3 (Fable/high). Given round 2 found 0 BLOCKING and the primary mission (real hub-mode run) already succeeded, round 3's seed should likely be shaped as a genuine safety pass PLUS an attempt to close the one remaining live gap named above (a real Rename card executing through a real Webster batch) if time allows — the orchestrator's own judgment call at seed time, informed by whatever the mill task's own outcome was.
3. List rounds 1 and 2's fixes as CLOSED-AND-VERIFIED (this file has the evidence) so round 3 does not re-litigate them.
4. Spawn `crucible-reviewer-high`, `model: fable`, tag `fable-high-r3`.
5. Verify independently exactly as rounds 1 and 2 were verified — do not relax the bar.
6. If round 3 converges clean, the pre-approved rotation calls for one more round (Opus/high, final safety pass, r4) — but per the README's own guidance, convergence (a safety pass + this orchestrator's gates + an operator-assisted check all agreeing) can close the campaign before using all four rounds. Surface merge-readiness and let the operator decide whether r4 is still wanted.
