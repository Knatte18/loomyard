# `loom` crucible campaign — glyph-hardening — HANDOFF

> Refreshed after every round's verification, per `crucible/orchestrator-prompt.md`'s hygiene section. If this session's context resets, a fresh orchestrator should read this file first, then `_mill/loom-crucible-orchestrator-kickoff.md` for the campaign charter.

## Campaign identity

- Worktree: `/home/knatte/Code/loomyard/wts/crucible-loom-glyph-hardening`, branch `crucible-loom-glyph-hardening`, parent `main`.
- Mission: harden `loom` against the newly-landed quarry-glyph-plan-alphabet surface (PR #230) — never before exercised through loom's real phase machine, only through the landing task's own unit/integration suite.
- Model/effort rotation, pre-approved by the operator for up to four rounds unless it converges sooner: **Opus/high → Sonnet/xhigh → Fable/high → Opus/high** (final safety pass).

## Current state

**Round 1 is CLOSED-AND-VERIFIED as of independent verification completing 2026-09-06.** Working tree is clean, HEAD is `aba1558e5` (this handoff's own commit, sitting on top of round 1's last commit `93b0d3a95`). Round 2 is being seeded next (Sonnet/xhigh, per the pre-approved rotation) — see "Next action" below for exactly what it's being asked to do.

## Independent verification results (this orchestrator, not the round's own claim)

A verification fork ran 9 sabotage-proof cycles (revert the production hunk → confirm the regression test fails at the intended assertion → restore → confirm empty diff) covering every one of the 13 BLOCKING findings' commits plus one MEDIUM/LOW spot-check, re-ran every hermetic/integration/named-smoke gate cold, judged the one design-shaped change against Hard Rule 5's size line, and checked F16/F22's out-of-scope claims against the actual code rather than trusting the fixer report.

**All 9/9 sabotage-proofs passed.** All gates green cold (`go build`, `go vet`, `go test -count=5` on the 11 in-scope package sets + `./cmd/lyx/...`, whole-repo `go test`, `-tags integration` on the 5 in-scope packages, one named `-tags smoke` test). `c6ee9eb16` (the two new `internal/planglyph` exported functions, `ValidateDispatch`/`PendingPlan`) is confirmed correctly scoped — grepped repo-wide, called only from the two dispatch-boundary sites it fixes, `ValidateFormat`/`Validate` (the Gate Self-Check Parity Invariant's named functions) provably unchanged in behavior. F16 confirmed genuinely unreachable from loom (grepped `internal/loomcli`/`loomengine`/`loomshed`/`loomrecipe` for `standalonegeom`/`standalonestate`/`wireStandalone` — zero hits; loom exclusively uses `hubgeom`). F22 confirmed the same way (`.lyx` exclusion machinery is hub-mode-only, in `internal/fabricengine`). All three doc fixes (F13, F15, F19) confirmed landed with correct text, quoted in the verification report. Teardown clean, no stray processes, no working-tree contamination from the verification pass itself.

**Nothing failed. Round 1's 20 fixes are genuinely closed, not just self-reported.**

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

Round 1 verified clean — decision made: round 2 is seeded as **Sonnet/xhigh**, NOT a generic safety pass, but a targeted mission to close the one gap round 1 itself could not reach (see round 1's fixer report, "residual risk worth naming for the next round"): round 1 drove every glyph mechanism through hand-authored plans and direct bracket-verb calls (a real git repo, real quarry, zero LLM cost — the "standalone probe harness"), because three environment gaps blocked a real `lyx loom run`. Two of those three gaps are **standalone-mode-only** (F16's Master-start refusal; no local Go-backed hub) and do NOT block **hub mode**, which is the only mode loom itself ever uses — so round 2 CAN and SHOULD attempt what round 1 could not: a real `lyx loom run`, hub mode, real LLM sessions, plan authored by a real `Plan-Write` session, carrying the glyph scenarios through Webster for real. The third gap (stale `lyx` on PATH predating the glyph landing) is an ordinary environment hazard round 2 must check for itself before trusting any live result (`which lyx`, confirm its build reflects current HEAD, re-deploy if not).

1. Round 2's seed (`_mill/loom-review-prompt.md`, already rewritten and committed — see below) lists round 1's 20 fixes as CLOSED-AND-VERIFIED with this handoff's verification evidence, so round 2 does not re-litigate them, and states the residual as: prove the now-fixed glyph mechanics survive contact with a REAL planning session and a REAL Webster hub run, not just a hand-crafted plan.
2. Spawn `crucible-reviewer-xhigh`, `model: sonnet`, tag `sonnet-xhigh-r2`.
3. Verify independently exactly as round 1 was verified (sabotage-proof every new regression test, cold gate re-runs, check any new design-shaped change against the size line) — do not relax the bar because round 1 held up.
4. **F16/F22 mill-wiki task: OPENED.** `#004` `standalonegeom-webster-run-and-log-hygiene` in Home.md (via the sanctioned `.millhouse/millpy-add` wrapper — the wiki daemon client, never a hand edit), unclaimed/backlog. The operator asked directly whether F16 is genuinely large enough to warrant its own task rather than an inline fix; investigation confirmed yes for F16 specifically: `shuttleengine.NewRunner`'s anchor/worktree containment check (`run.go`'s `validateToldPaths`) is a deliberate swap-detector every one of its FOUR callers relies on (`internal/burlercli`, `internal/webstercli`, `internal/shuttlecli`, `internal/loomcli`), and `standalonegeom`'s `AnchorPath`/`WorktreeRoot` divergence (documented as deliberate in `burlergeom.go`/`reedgeom.go`/`webstergeom.go`) structurally violates that check by design — so fixing F16 means either relaxing a shared safety invariant in a module outside webster/loom entirely, or redesigning standalone geometry to satisfy it, either of which is a real cross-module design call, not a scoped bugfix. F22 is smaller (missing `.git/info/exclude` seeding, standalone-mode-only) but was folded into the same task since it's the same code path and the same non-loom owner.
5. Rounds remaining under the operator's pre-approved rotation after round 1: Sonnet/xhigh (round 2, in flight), Fable/high, Opus/high (final safety pass) — max four total unless it converges sooner. If round 2 converges clean and an operator-assisted check agrees, the campaign can close before using all four.
