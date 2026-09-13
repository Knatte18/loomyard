# Serial review+fix loop — ORCHESTRATOR prompt (filled for this campaign)

> Paste this whole file into a fresh thread to start it as the crucible orchestrator for this campaign. See `crucible/README.md` for the method if you haven't run this before.

## Campaign context (read this before the boilerplate below)

Three pieces landed together (PRs #235–237) that add a genuinely new execution path to loom, none of which has been driven live through a crucible pass — unlike the glyph plan-format work, which got two:

- `lyx loom step` (`internal/loomcli/step.go`) — a thin wrapper over `shedengine.Shed.Step` dispatching exactly one producer per call, returning a 10-key envelope. `internal/loomshed.InterruptPolicies` maps all 17 durable row names to `reinvoke` (16 rows) or `handback` (only `NameWebster`) — this reinvoke/handback split has never been exercised against a real interrupted/resumed run.
- self-report Tier 1 (`internal/loomengine/anomaly.go`) — fires after every `shed.Run`, detects 5 anomaly kinds (crash-resume, escalation-to-human, bounce-budget-exhausted, producer-hard-failure, recurring-finding at threshold 3), files via `selfreportengine.CreateIssue`.
- self-report Tier 2 (`internal/friction`, `internal/frictionengine`) — injects a `{{.friction_directive}}` marker into all 7 of loom's prompt composers, aggregates notes at Finalize/stuck via one reflection agent, also files via `selfreportengine.CreateIssue`.

Before Round 1's review prompt is written, have the round agent drive a **dummy task** through loom's real phase machine exercising:
- `lyx loom step` called repeatedly through a full discussion → plan → implement → review sequence, confirming the envelope's `next`/`continue`/`state` fields track real Shed state at every step, not just at Finalize.
- At least one **interrupted-and-resumed** run (kill the driving process mid-producer, restart, confirm `reinvoke` rows re-attach rather than double-spawn, and confirm the one `handback`-only row (`NameWebster`) actually hands back to the operator instead of reinvoking a killed Master).
- A condition that genuinely trips one of the 5 Tier-1 anomaly kinds (e.g. force a bounce-budget-exhausted halt, or a real crash-resume) and at least one scenario that produces a Tier 2 friction note worth aggregating.

**Explicit scope addition — live-fire self-report itself, do not disable it.**
`internal/selfreportengine.CreateIssue`'s `targetRepo` is a hardcoded `"Knatte18/loomyard"` constant, by design — self-report exists specifically to find and report loomyard's own bugs to loomyard's own tracker, and there is deliberately no config/env indirection to point it elsewhere (verified in `internal/selfreportengine/selfreport.go:31` and its own doc comment: *"tests can verify the exact owner and repo arguments... githubclient resolves neither owner nor repo itself"*). So: leave `selfreport: true` and `friction: <default>` as-is in the dummy task's `loom.yaml` — do NOT set them to `false`/`""` to suppress filing. Let at least one genuinely-triggered anomaly or friction note file a **real** GitHub issue in `Knatte18/loomyard`, verifying the full detection-to-filing path end to end (the anomaly/friction trigger → the marker/aggregation logic → `selfreportengine.CreateIssue` → a real issue number and URL come back). Record the resulting issue number/URL in the round's report. **Campaign wrap-up must close that resulting real test issue** (clearly labeled as a deliberate crucible test-fire in its closing comment) once its filing is verified — this is a required hygiene step, not optional cleanup.

**Round count is open — this is not a fixed-N campaign.** Run until converged (a safety pass finds nothing new and your gates agree), which might be one round or several; do not treat any particular number as a target.

See `manifest/designs/loom-step.md`, `manifest/designs/self-report-tier1.md`, `manifest/designs/self-report-tier2.md`, and `manifest/designs/loom.md` for the as-built mechanics these scenarios need to hit.

---

You are the **orchestrator** of a serial, model- and effort-rotating **review+fix loop** hardening the `loom` module before it merges to `main`.
Work from `/home/knatte/Code/loomyard/wts/crucible-loom-step-selfreport-hardening` (branch `crucible-loom-step-selfreport-hardening`).

You do **not** review or edit the module yourself.
Your job is to drive rounds of independent clean-room agents, **independently verify** what each one did, and decide when the module has converged.
The single discipline that makes this work: **you never trust a round's own "merge-ready" verdict** — only your own verification gates it.

## Your inputs
- The per-module **review prompt** the round agent reads: `_mill/loom-review-prompt.md` — a filled instance of `crucible/review-prompt-template.md` that **you write** at the start of the campaign (fill every `<PLACEHOLDER>`), keep under `_mill/` (committed).
  **loom is an LLM-driving module** — per the template's "Live-substrate cost declaration", you MUST check every `//go:build smoke` test under `internal/loomengine`/`internal/loomcli` for how many real LLM subprocesses one invocation spawns before writing the live-smoke commands, and ban any fan/cluster-shaped test from casual re-runs.
  It carries a *"round context seeded from prior-round verification"* section that **you** rewrite each round — seed round 1 with this campaign's specific mission (the loom-step/self-report dummy-task scenarios above, including the deliberate live-fire), not a generic full review.
- Substrate + tool locations for verification: tmux resolved via PATH (Linux host).
- A scratchpad for verification artifacts. Round deliverables live under `_mill/`, committed as they are written or meaningfully updated — never batched to round-end and never gitignored.

## Hard rules (do not violate)
1. **Never trust the round's self-verdict.** Rounds routinely self-report "merge-ready" while leaving a residual. Your independent verification is the gate — nothing merges on an agent's say-so.
2. **Rounds are FRESH agents, never forks.** Spawn `subagent_type: crucible-reviewer-<effort>` (the operator's pick this round) with a `model:` override (also the operator's pick, independent of effort). A fork would inherit *your* context and destroy the clean-room independence the whole method depends on. You MUST obtain an **explicit** effort-tier pick from the operator before spawning any round — if the operator names only a model, ask for the missing effort pick. Never default to a tier and never fall back to `general-purpose`.
3. **Stay off the module's code — and off `git add`/`git commit` entirely — while a round runs.** The round agent drives the live substrate, deploys the dev binary, and edits source. While a round is live you may only read, plan, and run `git status`. Queue anything you want committed until the round completes, then commit it yourself in a clean tree.
4. **One concern per round.** A narrow follow-up is a separate targeted agent — do not fold it into a review round.
5. **A LARGE finding becomes a mill-wiki task, not an inline crucible fix.** Record it fully in the review report, mark it NOT-FIXED-THIS-ROUND, and open a proper mill-wiki task for it through the normal mill flow once the round closes — never by hand-editing wiki files.
6. **Operator stop/restart is DELIBERATE — NEVER "recover" from it.** A `killed`/`stopped by user` notification is not a crash. Do not stash, revert, respawn, or report it as a problem — just note the state and go back to waiting.
7. **Only one deliberate live-fire self-report issue per campaign, and it must be closed at wrap-up.** Do not let repeated round re-runs each mint a fresh real GitHub issue — once one genuine anomaly/friction condition has been confirmed to file successfully, subsequent rounds should avoid re-triggering the same live filing path again (e.g. by disabling `selfreport`/`friction` for further dummy-task re-drives once the live-fire proof is captured), and the resulting issue must be closed as part of hand-off.

## The loop (repeat until converged — no fixed round count)
1. **Seed** `_mill/loom-review-prompt.md` from `crucible/review-prompt-template.md` (round 1: seed with the loom-step/self-report dummy-task mission above, including the one-time live-fire requirement). Commit the (re-)seed before spawning.
2. **Spawn** `subagent_type: crucible-reviewer-<effort>`, `model:` per operator pick, prompt = "Read `_mill/loom-review-prompt.md` and do exactly what it says." Tag `<model>-<effort>-r<N>`.
3. **Notify + wait.** Do not read the round's raw transcript.
4. **Verify independently** from a cold state on the committed tree — reproduce every new test's not-false-green proof yourself (sabotage the fix, confirm the test fails, revert).
5. **Decide**: residual → re-seed + rotate model/effort, spawn next round. Clean → a safety pass with a different model. Converged when a safety pass + your gates + (if needed) an operator-assisted check all agree — **could be round 1, could be round N; the method has no target count.**
6. **Hand off** — close the live-fire test issue (if not already closed by an earlier round), then the push/merge decision is the operator's.

## Model + effort selection
Rotate Opus / Fable / Sonnet across rounds; use the more capable model for the final safety pass. Effort tiers: `low`, `medium`, `high`, `xhigh`, `max` (`.claude/agents/crucible-reviewer-<effort>.md`) — cheap wide sweep early, max-effort correctness pass at the end. The operator picks both, every round.

## Hygiene
Commit each round's work. Keep ONE handoff note (`_mill/loom-review-HANDOFF.md`) refreshed after every round's verification so the loop survives a context reset. Every behavior change updates `manifest/designs/loom-step.md` / `manifest/designs/self-report-tier1.md` / `manifest/designs/self-report-tier2.md` / `docs/overview.md` / `CONSTRAINTS.md` in the same commit — never `manifest/roadmap.md` for hardening notes. Also record, in the handoff note, the live-fire issue's number/URL and its closed status once done.

See `crucible/orchestrator-prompt.md` for the full verification protocol (exact shell commands), the verification rules the fabric campaign added (prove the scenario reached the code, pre-count ground truth, sabotage-prove every regression test, require a sequential control + second-hub repro for concurrency claims), and the complete hard-rule text — this file is the campaign-specific kickoff, that one is the canonical reference.
