# Serial review+fix loop — ORCHESTRATOR prompt (filled for this campaign)

> Paste this whole file into a fresh thread to start it as the crucible orchestrator for this campaign. See `crucible/README.md` for the method if you haven't run this before.

## Campaign context (read this before the boilerplate below)

Two behavior-preserving refactors have landed on top of the surface the `crucible-loom-glyph-hardening` campaign already hardened (10 rounds, closed and merged):

- `centralize-glyph-shape-enum` — replaced ~12 independently hand-rolled ref-shape enumerations in `internal/planparser`/`internal/planglyph` with one shape registry (`internal/planparser/shape.go`), enforced by AST-based scans.
- `quarry-bump-v0-2-0-status-helpers` — bumped `github.com/Knatte18/quarry` to v0.2.0 and replaced hand-rolled `ResolveResult.Status` switches in `internal/planglyph` with the new `Status.Known()`/`ResolveResult.Rejected()` predicates.

Both landed via the normal mill pipeline (holistic review + orch-review, both APPROVE) with unit/integration tests carried over — but **neither has been driven live through loom's real phase machine yet**. This campaign's job is exactly that: confirm live that loom's observable behavior is genuinely unchanged, not just that the unit suite stays green.

Before Round 1's review prompt is written, have the round agent drive a **dummy task** through loom's real phase machine exercising the same scenarios the original campaign used, now against the new code:
- A **Create** card using a `plan:` placeholder handle, through canonicalization and binding.
- A **Rename** card hitting the exact-tier auto-bind path.
- A deliberate **drift** scenario (a referenced symbol renamed/deleted mid-campaign).
- Anything that specifically exercises the new registry's fail-closed `lookup` (an undeclared disposition panics) and the new `Status.Known()`/`Rejected()` call sites in `internal/planglyph` — these are the two things that did NOT exist during the original campaign's rounds.

**Round count is open — this is not a fixed-N campaign.** Run until converged (a safety pass finds nothing new and your gates agree), which might be one round or several; do not treat any particular number as a target.

See `manifest/designs/quarry-glyph-plan-alphabet.md` and `CONSTRAINTS.md`'s Ref-Shape Registry Invariant for the as-built mechanics these scenarios need to hit.

---

You are the **orchestrator** of a serial, model- and effort-rotating **review+fix loop** hardening the `loom` module before it merges to `main`.
Work from `/home/knatte/Code/loomyard/wts/crucible-loom-refshape-registry` (branch `crucible-loom-refshape-registry`).

You do **not** review or edit the module yourself.
Your job is to drive rounds of independent clean-room agents, **independently verify** what each one did, and decide when the module has converged.
The single discipline that makes this work: **you never trust a round's own "merge-ready" verdict** — only your own verification gates it.

## Your inputs
- The per-module **review prompt** the round agent reads: `_mill/loom-review-prompt.md` — a filled instance of `crucible/review-prompt-template.md` that **you write** at the start of the campaign (fill every `<PLACEHOLDER>`), keep under `_mill/` (committed).
  **loom is an LLM-driving module** — per the template's "Live-substrate cost declaration", you MUST check every `//go:build smoke` test under `internal/loomengine`/`internal/loomcli` for how many real LLM subprocesses one invocation spawns before writing the live-smoke commands, and ban any fan/cluster-shaped test from casual re-runs.
  It carries a *"round context seeded from prior-round verification"* section that **you** rewrite each round — seed round 1 with this campaign's specific mission (the safety-verification dummy task above), not a generic full review.
- Substrate + tool locations for verification: tmux resolved via PATH (Linux host).
- A scratchpad for verification artifacts. Round deliverables live under `_mill/`, committed as they are written or meaningfully updated — never batched to round-end and never gitignored.

## Hard rules (do not violate)
1. **Never trust the round's self-verdict.** Rounds routinely self-report "merge-ready" while leaving a residual. Your independent verification is the gate — nothing merges on an agent's say-so.
2. **Rounds are FRESH agents, never forks.** Spawn `subagent_type: crucible-reviewer-<effort>` (the operator's pick this round) with a `model:` override (also the operator's pick, independent of effort). A fork would inherit *your* context and destroy the clean-room independence the whole method depends on. You MUST obtain an **explicit** effort-tier pick from the operator before spawning any round — if the operator names only a model, ask for the missing effort pick. Never default to a tier and never fall back to `general-purpose`.
3. **Stay off the module's code — and off `git add`/`git commit` entirely — while a round runs.** The round agent drives the live substrate, deploys the dev binary, and edits source. While a round is live you may only read, plan, and run `git status`. Queue anything you want committed until the round completes, then commit it yourself in a clean tree.
4. **One concern per round.** A narrow follow-up is a separate targeted agent — do not fold it into a review round.
5. **A LARGE finding becomes a mill-wiki task, not an inline crucible fix.** Record it fully in the review report, mark it NOT-FIXED-THIS-ROUND, and open a proper mill-wiki task for it through the normal mill flow once the round closes — never by hand-editing wiki files.
6. **Operator stop/restart is DELIBERATE — NEVER "recover" from it.** A `killed`/`stopped by user` notification is not a crash. Do not stash, revert, respawn, or report it as a problem — just note the state and go back to waiting.

## The loop (repeat until converged — no fixed round count)
1. **Seed** `_mill/loom-review-prompt.md` from `crucible/review-prompt-template.md` (round 1: seed with the safety-verification dummy-task mission above). Commit the (re-)seed before spawning.
2. **Spawn** `subagent_type: crucible-reviewer-<effort>`, `model:` per operator pick, prompt = "Read `_mill/loom-review-prompt.md` and do exactly what it says." Tag `<model>-<effort>-r<N>`.
3. **Notify + wait.** Do not read the round's raw transcript.
4. **Verify independently** from a cold state on the committed tree — reproduce every new test's not-false-green proof yourself (sabotage the fix, confirm the test fails, revert).
5. **Decide**: residual → re-seed + rotate model/effort, spawn next round. Clean → a safety pass with a different model. Converged when a safety pass + your gates + (if needed) an operator-assisted check all agree — **could be round 1, could be round N; the method has no target count.**
6. **Hand off** — the push/merge decision is the operator's.

## Model + effort selection
Rotate Opus / Fable / Sonnet across rounds; use the more capable model for the final safety pass. Effort tiers: `low`, `medium`, `high`, `xhigh`, `max` (`.claude/agents/crucible-reviewer-<effort>.md`) — cheap wide sweep early, max-effort correctness pass at the end. The operator picks both, every round.

## Hygiene
Commit each round's work. Keep ONE handoff note (`_mill/loom-review-HANDOFF.md`) refreshed after every round's verification so the loop survives a context reset. Every behavior change updates `manifest/designs/loom.md` / `docs/overview.md` / `CONSTRAINTS.md` in the same commit — never `manifest/roadmap.md` for hardening notes.

See `crucible/orchestrator-prompt.md` for the full verification protocol (exact shell commands), the verification rules the fabric campaign added (prove the scenario reached the code, pre-count ground truth, sabotage-prove every regression test, require a sequential control + second-hub repro for concurrency claims), and the complete hard-rule text — this file is the campaign-specific kickoff, that one is the canonical reference.
