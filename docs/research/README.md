# Research

Measurement studies of LoomYard's own runs and tools, grouped by topic.
A study is a single report file,
or a folder that holds `report.md` beside the scripts and data behind it.
A folder inside a topic groups a series of near-identical studies.
Each report records its own runs at a point in time and is not rewritten when the code changes.
A study's script finds the repository root with `git rev-parse --show-toplevel`, so it runs from any folder depth.

## burler — review rounds, lenses and fixers

- [burler-fan-tuning-bench-2026-10-16](burler/burler-fan-tuning-bench-2026-10-16/report.md) — whether a tuned lens fan broadens a Burler review.
- [burler-fork-review-audit-2026-10-16](burler/burler-fork-review-audit-2026-10-16/report.md) — a reviewer master that forks its review, fixes, and reviews its own fix.
- [concrete-review-findings-2026-11-03](burler/concrete-review-findings-2026-11-03/report.md) — whether Plan and Discussion reviewers already write concrete findings.
- [fixers-on-sonnet-2026-11-03](burler/fixers-on-sonnet-2026-11-03/report.md) — Plan and Discussion fixers on Sonnet against Opus.
- [review-setup-audit-2026-10-28](burler/review-setup-audit-2026-10-28/report.md) — what each part of the review setup is worth, read from landed loom runs.
- [session-fork-spike.md](burler/session-fork-spike.md) — whether a review forked over lenses from one session finds more than one reviewer.

### lens-fan — the cost of the lens fan

- [lens-fan-cost-analysis.md](burler/lens-fan/lens-fan-cost-analysis.md) — the cost of the Burler lens fan on one run's reviews.
- [lens-fan-lab-phase0.md](burler/lens-fan/lens-fan-lab-phase0.md) — what the fan's forks read, and variants of the master's instructions.
- [lens-fan-lab-phase1.md](burler/lens-fan/lens-fan-lab-phase1.md) — measured runs of the master-instruction variants.

## codex — the Codex engine

- [codex-feasibility.md](codex/codex-feasibility.md) — which Claude couplings stand between lym and the Codex CLI.
- [codex-spike-2026-10-24](codex/codex-spike-2026-10-24/report.md) — a hands-on spike of the Codex CLI: its pane, hooks and sessions.
- [codex-twill-spike-2026-10-24](codex/codex-twill-spike-2026-10-24/report.md) — a fan of Codex forks with inherited context.
- [codex-hook-delivery-2026-10-28](codex/codex-hook-delivery-2026-10-28/report.md) — whether Codex can take creel posts through hooks.
- [stencil-engine-audit-2026-10-24](codex/stencil-engine-audit-2026-10-24/report.md) — the Claude-specific mechanics in the stencils, and what a second engine needs.

### engine-trial — real Codex launches through lym

- [codex-engine-trial-home-sandbox-2026-11-03.md](codex/engine-trial/codex-engine-trial-home-sandbox-2026-11-03.md) — home, trust, sandbox and refusals.
- [codex-engine-trial-hooks-guards-skills-2026-11-03.md](codex/engine-trial/codex-engine-trial-hooks-guards-skills-2026-11-03.md) — hooks, shell guards and skills.
- [codex-engine-trial-pane-load-2026-11-03.md](codex/engine-trial/codex-engine-trial-pane-load-2026-11-03.md) — pane, sends, creel by the pane path, and load.
- [codex-engine-trial-darn-e2e-2026-11-05.md](codex/engine-trial/codex-engine-trial-darn-e2e-2026-11-05.md) — a darn run with a Codex writer.
- [codex-creel-trial-2026-11-05.md](codex/engine-trial/codex-creel-trial-2026-11-05.md) — creel mail reaching Codex sessions through the new hooks.

### astra — Codex Astra as a plan reviewer

- [codex-astra-solo-bench-2026-10-25](codex/astra/codex-astra-solo-bench-2026-10-25/report.md) — Codex Astra against solo Opus as a plan reviewer.
- [codex-astra-solo-high-bench-2026-10-27](codex/astra/codex-astra-solo-high-bench-2026-10-27/report.md) — Astra at high effort against medium effort, solo, on one plan.
- [codex-astra-fan-bench-2026-10-27](codex/astra/codex-astra-fan-bench-2026-10-27/report.md) — an Astra lens fan against the Claude fan on one plan.
- [codex-astra-fork-fan-bench-2026-10-27](codex/astra/codex-astra-fork-fan-bench-2026-10-27/report.md) — an Astra fan with real forks against the Claude fan.
- [codex-astra-context-parity-bench-2026-10-27](codex/astra/codex-astra-context-parity-bench-2026-10-27/report.md) — the Astra fork fan given the Claude reviewers' context.
- [codex-astra-tuned-prompt-bench-2026-10-27](codex/astra/codex-astra-tuned-prompt-bench-2026-10-27/report.md) — the Astra forked fan with a tuned reviewer prompt against the Claude fan.
- [codex-astra-tuned-p1-high-bench-2026-10-27](codex/astra/codex-astra-tuned-p1-high-bench-2026-10-27/report.md) — the tuned prompt on plan P1 at high effort.
- [codex-astra-tuned-p1-med-bench-2026-10-27](codex/astra/codex-astra-tuned-p1-med-bench-2026-10-27/report.md) — the tuned prompt on plan P1 at medium effort.
- [codex-astra-tuned-p4-med-bench-2026-10-27](codex/astra/codex-astra-tuned-p4-med-bench-2026-10-27/report.md) — the tuned prompt on plan P4 at medium effort.

### v2 — the Astra forked fan with reading block v2

- [codex-v2-p1-bench-2026-10-27](codex/v2/codex-v2-p1-bench-2026-10-27/report.md) — on plan P1: one high run against two medium runs.
- [codex-v2-p2-bench-2026-10-27](codex/v2/codex-v2-p2-bench-2026-10-27/report.md) — with a 20-call fork cap on plan P2: one high run against two medium runs.
- [codex-v2-p4-bench-2026-10-27](codex/v2/codex-v2-p4-bench-2026-10-27/report.md) — with a 20-call fork cap on plan P4: one high run against two medium runs.
- [codex-p2-gap-texts-bench-2026-10-27](codex/v2/codex-p2-gap-texts-bench-2026-10-27/report.md) — on plan P2 with three texts from the gap study: two medium runs.

### review — Codex reviews against Claude reviews

- [codex-review-bench-2026-10-25](codex/review/codex-review-bench-2026-10-25/report.md) — Codex as a Burler reviewer against Claude reviews.
- [claude-review-gaps-2026-10-27](codex/review/claude-review-gaps-2026-10-27/report.md) — what the Claude lens fan misses that Codex Astra finds.
- [codex-code-review-bench-2026-10-28](codex/review/codex-code-review-bench-2026-10-28/report.md) — the Astra forked fan as Code reviewer on two darn diffs.
- [codex-review-yardstick-2026-10-28](codex/review/codex-review-yardstick-2026-10-28/report.md) — Codex code review with the decision record as yardstick.

## creel — mail between sessions

- [creel-hook-delivery-2026-10-25](creel/creel-hook-delivery-2026-10-25/report.md) — creel mail delivery through a tool-boundary hook.
- [creel-hook-channels-2026-10-28](creel/creel-hook-channels-2026-10-28/report.md) — which hook channels deliver a creel post, and which the operator sees.
- [creel-midturn-trust-2026-10-28](creel/creel-midturn-trust-2026-10-28/report.md) — whether a mid-turn creel post gets acted on, and whether the operator sees it.

### audit — creel stage 1

- [creel-audit-2026-10-15](creel/audit/creel-audit-2026-10-15/report.md) — the stage 1 audit.
- [creel-audit-2026-10-16](creel/audit/creel-audit-2026-10-16/report.md) — the first re-test.
- [creel-audit-2026-10-17](creel/audit/creel-audit-2026-10-17/report.md) — the second re-test.

## drift — the Drift stages

- [drift-whole-review-2026-11-04.md](drift/drift-whole-review-2026-11-04.md) — Drift as a whole: a map, a fault analysis and a reworked design.

### csharp-trial — Drift on the C# sandbox hub

- [drift-csharp-trial-2026-11-03.md](drift/csharp-trial/drift-csharp-trial-2026-11-03.md) — the first trial.
- [drift-csharp-trial-2-2026-11-03.md](drift/csharp-trial/drift-csharp-trial-2-2026-11-03.md) — the second trial.

## glyph — what Webster forks read

- [glyph-audit-2026-10-12](glyph/glyph-audit-2026-10-12/report.md) — the Webster kick-start pack.
- [glyph-audit-2026-10-13](glyph/glyph-audit-2026-10-13/report.md) — read-all-first forks against earlier Webster forks.
- [glyph-audit-2026-10-15](glyph/glyph-audit-2026-10-15/report.md) — the pack-file layout against read-all-first forks.
- [glyph-audit-2026-10-17](glyph/glyph-audit-2026-10-17/report.md) — what Webster forks read.

## loom — planning and agent instructions

- [claude-md-rule-map-2026-10-29.md](loom/claude-md-rule-map-2026-10-29.md) — each rule of `CLAUDE.md` mapped to where lym's agents already get it.
- [plan-writer-audit-2026-10-24](loom/plan-writer-audit-2026-10-24/report.md) — the Plan writer's cost, rounds, wall time and `relevant.md` coverage.
- [writer-cost-remeasure-2026-11-08](loom/writer-cost-remeasure-2026-11-08/report.md) — writer lookups and Plan phase cost, re-measured on the tapestry runs after the Discussion lookup change.
- [darn-writer-anatomy-2026-11-08](loom/darn-writer-anatomy-2026-11-08/report.md) — the darn writer's cost by token kind and activity, gate-wait cache rewrites, and what a writer that leaves gate failures to a fixer would save.

### plan-split — splitting a plan into parts

- [plan-split-bench-2026-11-01](loom/plan-split/plan-split-bench-2026-11-01/report.md) — Parts-Write and Parts-Plan-Write against Plan-Write.
- [plan-split-tune-2026-11-02](loom/plan-split/plan-split-tune-2026-11-02/report.md) — round 1 changes to the plan-split writers, and their replay.

## platform — where lym runs

- [linux-portability-survey.md](platform/linux-portability-survey.md) — what broke when the Go test suite first ran on Linux.
- [sandboxing-options-2026-10-30.md](platform/sandboxing-options-2026-10-30.md) — sandboxing options for a closed area for lym.

## quarry — code search

- [tsa-quarry-2026-10-24](quarry/tsa-quarry-2026-10-24/report.md) — ideas from tree-sitter-analyzer for quarry.

## reed — panes and Claude Code hooks

- [psmux-tui-behavior.md](reed/psmux-tui-behavior.md) — the claude TUI under psmux on Windows.
- [reed-hooks-exploration.md](reed/reed-hooks-exploration.md) — Claude Code hooks and `claude agents --json`.

## speed — test and start time

- [batten-start-2026-10-18](speed/batten-start-2026-10-18/report.md) — the time of a batten start before the fast-start plan.
- [gate-fewer-launches-2026-10-31](speed/gate-fewer-launches-2026-10-31/report.md) — new test binaries and launches per gate tier.
- [gate-fewer-binaries-trial-2026-11-01](speed/gate-fewer-binaries-trial-2026-11-01/report.md) — trials of fewer new test binaries per gate run.
- [fabric-test-speed-survey-2026-11-03.md](speed/fabric-test-speed-survey-2026-11-03.md) — where `internal/fabricengine` test time goes, and how to cut it.
- [task-runtime-before-after-2026-11-06](speed/task-runtime-before-after-2026-11-06/report.md) — whether darn and tapestry runs take less active time since turn-savings, and how much of it P1 can claim.
- [run-slowness-causes-2026-11-06](speed/run-slowness-causes-2026-11-06/report.md) — where a run's wall time goes, what made darn runs slower, and the changes that would cut it, ranked.
- [tier1-test-revision-2026-11-06](speed/tier1-test-revision-2026-11-06/report.md) — where a tier 1 pass goes, which levers of the fabricengine survey still stand, which tests can fold, and the changes that would cut a pass, ranked.
- [testmap-upkeep-2026-11-11](speed/testmap-upkeep-2026-11-11/report.md) — three independent proposals, two by Fable and one by Opus, for keeping the per-test coverage maps fresh, compared side by side.

## tokens — token use, cost and models

- [token-audit-2026-10-22](tokens/token-audit-2026-10-22/report.md) — where Claude tokens went over two weeks of runs.
- [token-usage-by-role.md](tokens/token-usage-by-role.md) — token usage and time per role.
- [haiku-judge.md](tokens/haiku-judge.md) — Haiku as the Bouncer judge, per row.
- [startup-context-2026-11-05](tokens/startup-context-2026-11-05/report.md) — what the orch and a spawned role load at start, by source, and how to leave out what they do not need.
- [session-scope-launch-2026-11-06](tokens/session-scope-launch-2026-11-06/report.md) — which session-scope settings work in a real Claude Code launch, and the start load with and without them.
- [session-scope-fix-launch-2026-11-06](tokens/session-scope-fix-launch-2026-11-06/report.md) — the session-scope fix in a real launch: first-request tokens of a spawned role and an orch, the `Workflow` tool gone, preloaded and plugin skills kept.
- [role-tool-schemas-2026-11-07](tokens/role-tool-schemas-2026-11-07/report.md) — each tool's schema tokens in a role's and the orch's first request, the tools each role called, and `--disallowedTools` against `permissions.deny`.

### model-downgrade — cheaper models per role

- [model-downgrade.md](tokens/model-downgrade/model-downgrade.md) — the roles that could run on a cheaper model.
- [model-downgrade-results.md](tokens/model-downgrade/model-downgrade-results.md) — what was tried per role, and what was decided.

## turns — agent turns and tool calls

- [turn-audit-2026-10-28](turns/turn-audit-2026-10-28/report.md) — where the turns of an agent session go, and which a merge would remove.
- [review-turn-audit-2026-11-01](turns/review-turn-audit-2026-11-01/report.md) — the turns, context and cost of the Discussion and Plan review segment.
- [batching-effect-audit-2026-11-01](turns/batching-effect-audit-2026-11-01/report.md) — the Plan and Discussion writers before and after the batching sentences.
- [hook-first-effect.md](turns/hook-first-effect.md) — the effect of reading a session's state from hook lines before the pane.
- [turn-savings-measure-2026-11-04](turns/turn-savings-measure-2026-11-04/report.md) — turn savings measured on landed runs.
- [writer-lookups-measure-2026-11-04](turns/writer-lookups-measure-2026-11-04/report.md) — how the Plan and Discussion writers explore the code and use lookup subagents.
- [batching-hook-remeasure-2026-11-05](turns/batching-hook-remeasure-2026-11-05/report.md) — the batching hook and the edit-round rule, re-measured once the hook reached forks.

## twill — fork-based writers and fixers

- [discussion-bench.md](twill/discussion-bench.md) — a chair and advisors against the solo Discussion writer.
- [twill-discussion-bench](twill/twill-discussion-bench/report.md) — twill on Discussion-Write, round 1.
- [twill-discussion-fork0-bench.md](twill/twill-discussion-fork0-bench.md) — Discussion-Write: today's twill, the critical fixer, and fork0 with the critical fixer.
- [twill-fixer-bench.md](twill/twill-fixer-bench.md) — Plan-Write: after and fork0 against today's judge and a critical fixer.
- [twill-fork0-bench.md](twill/twill-fork0-bench.md) — Plan-Write: fork0 against after, both with the critical fixer.
- [twill-rationale-bench.md](twill/twill-rationale-bench.md) — Plan-Write: fork0 with a rationale file against fork0.
- [twill-fork-timing-bench.md](twill/twill-fork-timing-bench.md) — spawning the forks before writing against after writing.
- [twill-fork-timing-plan-bench.md](twill/twill-fork-timing-plan-bench.md) — Plan-Write: spawning after writing against a writer fork.
- [burler-twill-fixer-bench-2026-10-16](twill/burler-twill-fixer-bench-2026-10-16/report.md) — today's fixer against a fixer that checks its own fix with forks.

### plan-bench — twill on Plan-Write

- [twill-plan-bench](twill/plan-bench/twill-plan-bench/report.md) — the first bench and its later rounds.
- [twill-plan-bench-sonnet](twill/plan-bench/twill-plan-bench-sonnet/report.md) — the Sonnet arm.
- [twill-plan-bench-sonnet-medium-fable](twill/plan-bench/twill-plan-bench-sonnet-medium-fable/report.md) — twill on Sonnet at medium effort, and a single Fable writer.

## webster — implementation batches

- [parallel-batches-2026-10-17](webster/parallel-batches-2026-10-17/report.md) — how often cards and batches could have run in parallel worktrees.

### batch-budget — the batcher's budget profiles

- [batch-budget-trial-2026-10-24](webster/batch-budget/batch-budget-trial-2026-10-24/report.md) — the cautious profile against the wide one.
- [batch-budget-trial-2026-11-01](webster/batch-budget/batch-budget-trial-2026-11-01/report.md) — a recheck on later runs.
