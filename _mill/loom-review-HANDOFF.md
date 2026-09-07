# `loom` crucible campaign — glyph-hardening — HANDOFF

> Refreshed after every round's verification, per `crucible/orchestrator-prompt.md`'s hygiene section. If this session's context resets, a fresh orchestrator should read this file first, then `_mill/loom-crucible-orchestrator-kickoff.md` for the campaign charter.

## Campaign identity

- Worktree: `/home/knatte/Code/loomyard/wts/crucible-loom-glyph-hardening`, branch `crucible-loom-glyph-hardening`, parent `main`.
- Mission: harden `loom` against the newly-landed quarry-glyph-plan-alphabet surface (PR #230) — never before exercised through loom's real phase machine, only through the landing task's own unit/integration suite.
- Model/effort rotation, pre-approved by the operator for up to four rounds unless it converges sooner: **Opus/high (r1) → Sonnet/xhigh (r2) → Fable/high (r3) → Opus/high (r4, final safety pass)**.

## Current state

**Round 1 CLOSED-AND-VERIFIED. Round 2 CLOSED-AND-VERIFIED. The `#004` mill-task gate has CLEARED — round 3 is spawning next, with an EXPANDED mission (see "Round 3 seed" below).** Working tree is clean, HEAD is `d7c52df6c` (the squash-merged `#004` fix, landed directly into THIS branch). Nothing pushed.

## `#004` mill-task gate — CLEARED (corrected understanding)

**`#004` `standalonegeom-webster-run-and-log-hygiene` completed the full mill flow (plan → go → holistic-approve → done) and squash-merged to its PARENT — which is THIS crucible branch (`crucible-loom-glyph-hardening`), not `main`.** This orchestrator initially misread a closed-but-unmerged GitHub PR (`#231` on `Knatte18/loomyard`) as the task having stalled; the operator corrected this: the standard loomyard mill procedure for this task was close the PR + `mill-merge` (squash to parent), and this crucible session was deliberately set as `#004`'s parent when it was spawned (since `#004` is itself a product of this crucible campaign's own findings, F16/F22 — it belongs to this campaign's tree, not to `main` directly). The fix landed here as commit `d7c52df6c` ("webster standalone mode: run refuses to start Master; logs write untracked into target repo") — 18 files, 879 insertions, including a new `CONSTRAINTS.md` invariant line: a `shuttleengine` runner whose anchor is deliberately outside its worktree root is now constructed only through the new `shuttleengine.NewDetachedRunner`, exclusively from standalone CLI wiring — `NewRunner`'s containment assertion (the shared safety invariant four other callers rely on, per round 1's investigation) is never relaxed. This is exactly the "separate constructor" resolution this orchestrator's own earlier investigation anticipated as one legitimate design option.

**Consequence: round 3's mission is now EXPANDED, per the operator's explicit instruction.** Since this fix landed inside THIS branch/session's own tree, it is now in-scope material this campaign must also harden before the branch merges to `main` — it has never itself been through a crucible round. F16/F22 are no longer "permanently out of loom's scope" as rounds 1/2 stated; that framing is now stale (see "Explicitly OUT of scope" correction needed in the next seed).

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

## Round 3 seed — TWO missions (both required, per the operator's explicit instruction)

**A) Review the newly-merged `#004` fix (`d7c52df6c`) for the first time — it has never been through a crucible round.** Read the full diff (`git show d7c52df6c`). Files: `internal/shuttleengine/run.go` (new `NewDetachedRunner`, containment assertion untouched for `NewRunner`), `internal/shuttleengine/wait.go`, `internal/standalonegeom/logsdir.go` (new — the `.lyx/logs/` fix for F22), `internal/standalonegeom/doc.go`, `internal/webstercli/wiring.go`, `internal/burlercli/wiring.go`, `internal/logger/sink.go`, `CONSTRAINTS.md` (new invariant line, quoted above). Be adversarial: this is unreviewed code from outside the original glyph-hardening scope, landed via a different task's own mill-go pipeline, not via this campaign's own discipline. Live-verify F16 and F22 are ACTUALLY fixed now: a real standalone `lyx webster run --target-dir <repo> --plan-dir <dir>` should now actually start Master (previously refused outright); standalone mode's `.lyx/logs/` should no longer appear as untracked in the target repo's `git status`. **This is a genuinely separate LLM-driving live scenario from loom's own hub-mode work** — budget for it the same way (one real session at a time, foreground, cost declaration applies) since starting Master standalone was exactly what was broken before.

**B) Continue hardening loom's own glyph integration surface**, informed by round 2's "What remains open" above: rounds 1+2 closed 27 findings (0 BLOCKING remaining), and the primary hub-mode-run mission already succeeded — so round 3 is shaped as a genuine safety pass over loom's own glyph surface, PLUS an attempt to close the one interesting remaining live gap if time allows: a real declared `Rename` card executing through a real Webster batch (seed a second live run from a worktree with a pre-existing symbol to rename, rather than a fresh `main`-derived one, so `Plan-Write` has a rename-shaped task to work with).

1. Rewrite `_mill/loom-review-prompt.md` for round 3 covering BOTH missions above. Correct the stale "F16/F22 permanently out of loom's scope" framing from rounds 1/2's seeds — that fix is now IN scope, having landed in this same branch.
2. List rounds 1 and 2's 27 fixes as CLOSED-AND-VERIFIED (this file has the evidence) so round 3 does not re-litigate them.
3. Spawn `crucible-reviewer-high`, `model: fable`, tag `fable-high-r3`.
4. Verify independently exactly as rounds 1 and 2 were verified (sabotage-proofs, cold gate re-runs, live-claim verification via real artifacts, not narrative) — do not relax the bar for either mission.
5. If round 3 converges clean on both missions, the pre-approved rotation calls for one more round (Opus/high, final safety pass, r4) — but per the README's own guidance, convergence (a safety pass + this orchestrator's gates + an operator-assisted check all agreeing) can close the campaign before using all four rounds. Surface merge-readiness and let the operator decide whether r4 is still wanted.
