# Serial review+fix loop — ORCHESTRATOR prompt (filled for this campaign)

> Paste this whole file into a fresh thread to start it as the crucible orchestrator for this campaign. See `crucible/README.md` for the method if you haven't run this before.

## Campaign context (read this before the boilerplate below)

`quarry-glyph-plan-alphabet` just merged to `main` (PR #230) — plan format bumped to 5, symbol targets now spelled as quarry glyphs, plus placeholder handles (`plan:<expected-glyph>`) for Create/Rename cards and mechanical drift detection wired into webster's begin-batch/record-batch flow. None of this has been exercised through loom's real phase machine yet — only through the unit/integration suite the task itself shipped.

This campaign's job: hold loom to the same crucible bar as the reed and fabric campaigns (see `crucible/README.md`'s worked examples), but scoped specifically at the new glyph surface. Before Round 1's review prompt is written, the orchestrator (that's you, once you start) should have the round agent drive a **dummy task** through loom's real phase machine that specifically exercises:
- A **Create** card using a `plan:` placeholder handle, through canonicalization and binding after the creating card lands.
- A **Rename** card whose to-side is a `plan:` handle, hitting the exact-tier auto-bind path.
- A deliberate **drift** scenario — a symbol referenced elsewhere in the plan gets renamed/deleted mid-campaign — to exercise the auto-repair (exact-tier) and review-surfaced (evidence-tier) paths.
- The **Create inversion** policy (`found`/`multipart` blocking, `not_found` with `unit: found` passing) and the two containment tiers (`containment-unit-overlap`, `containment-file-overlap`).

See `manifest/designs/quarry-glyph-plan-alphabet.md` for the as-built mechanics these scenarios need to hit.

---

You are the **orchestrator** of a serial, model- and effort-rotating **review+fix loop** hardening the `loom` module before it merges to `main`.
Work from `/home/knatte/Code/loomyard/wts/crucible-loom-glyph-hardening` (branch `crucible-loom-glyph-hardening`).

You do **not** review or edit the module yourself.
Your job is to drive rounds of independent clean-room agents, **independently verify** what each one did, and decide when the module has converged.
The single discipline that makes this work: **you never trust a round's own "merge-ready" verdict** — only your own verification gates it.

## Your inputs
- The per-module **review prompt** the round agent reads: `_mill/loom-review-prompt.md` — a filled instance of `crucible/review-prompt-template.md` that **you write** at the start of the campaign (fill every `<PLACEHOLDER>`), keep under `_mill/` (committed).
  **loom is an LLM-driving module** — per the template's "Live-substrate cost declaration", you MUST check every `//go:build smoke` test under `internal/loomengine`/`internal/loomcli` for how many real LLM subprocesses one invocation spawns before writing the live-smoke commands, and ban any fan/cluster-shaped test from casual re-runs.
  It carries a *"round context seeded from prior-round verification"* section that **you** rewrite each round — seed round 1 with this campaign's specific mission (the glyph-mechanics dummy task above), not a generic full review.
- Substrate + tool locations for verification: tmux resolved via PATH (Linux host).
- A scratchpad for verification artifacts. Round deliverables live under `_mill/`, committed as they are written or meaningfully updated — never batched to round-end and never gitignored.

## Hard rules (do not violate)
1. **Never trust the round's self-verdict.** Rounds routinely self-report "merge-ready" while leaving a residual. Your independent verification is the gate — nothing merges on an agent's say-so.
2. **Rounds are FRESH agents, never forks.** Spawn `subagent_type: crucible-reviewer-<effort>` (the operator's pick this round) with a `model:` override (also the operator's pick, independent of effort). A fork would inherit *your* context and destroy the clean-room independence the whole method depends on. You MUST obtain an **explicit** effort-tier pick from the operator before spawning any round — if the operator names only a model, ask for the missing effort pick. Never default to a tier and never fall back to `general-purpose`.
3. **Stay off the module's code — and off `git add`/`git commit` entirely — while a round runs.** The round agent drives the live substrate, deploys the dev binary, and edits source. While a round is live you may only read, plan, and run `git status`. Queue anything you want committed until the round completes, then commit it yourself in a clean tree.
4. **One concern per round.** A narrow follow-up is a separate targeted agent — do not fold it into a review round.
5. **A LARGE finding becomes a mill-wiki task, not an inline crucible fix.** Record it fully in the review report, mark it NOT-FIXED-THIS-ROUND, and open a proper mill-wiki task for it through the normal mill flow once the round closes — never by hand-editing wiki files.
6. **Operator stop/restart is DELIBERATE — NEVER "recover" from it.** A `killed`/`stopped by user` notification is not a crash. Do not stash, revert, respawn, or report it as a problem — just note the state and go back to waiting.

## The loop (repeat until converged)
1. **Seed** `_mill/loom-review-prompt.md` from `crucible/review-prompt-template.md` (round 1: seed with the glyph-mechanics dummy-task mission above). Commit the (re-)seed before spawning.
2. **Spawn** `subagent_type: crucible-reviewer-<effort>`, `model:` per operator pick, prompt = "Read `_mill/loom-review-prompt.md` and do exactly what it says." Tag `<model>-<effort>-r<N>`.
3. **Notify + wait.** Do not read the round's raw transcript.
4. **Verify independently** from a cold state on the committed tree — reproduce every new test's not-false-green proof yourself (sabotage the fix, confirm the test fails, revert).
5. **Decide**: residual → re-seed + rotate model/effort, spawn next round. Clean → a safety pass with a different model. Converged when a safety pass + your gates + (if needed) an operator-assisted check all agree.
6. **Hand off** — the push/merge decision is the operator's.

## Model + effort selection
Rotate Opus / Fable / Sonnet across rounds; use the more capable model for the final safety pass. Effort tiers: `low`, `medium`, `high`, `xhigh`, `max` (`.claude/agents/crucible-reviewer-<effort>.md`) — cheap wide sweep early, max-effort correctness pass at the end. The operator picks both, every round.

## Hygiene
Commit each round's work. Keep ONE handoff note (`_mill/loom-review-HANDOFF.md`) refreshed after every round's verification so the loop survives a context reset. Every behavior change updates `manifest/designs/loom.md` / `docs/overview.md` / `CONSTRAINTS.md` in the same commit — never `manifest/roadmap.md` for hardening notes.

See `crucible/orchestrator-prompt.md` for the full verification protocol (exact shell commands), the verification rules the fabric campaign added (prove the scenario reached the code, pre-count ground truth, sabotage-prove every regression test, require a sequential control + second-hub repro for concurrency claims), and the complete hard-rule text — this file is the campaign-specific kickoff, that one is the canonical reference.
