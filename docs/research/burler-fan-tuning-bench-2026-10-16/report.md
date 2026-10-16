# Burler lens-fan tuning bench: does a tuned lens fan broaden a review?

2026-10-16, prime `/home/knatte/Code/loomyard-LYXHUB/loomyard`, board notes `fan-prompt-tuning` and `perch-review-analysis`.
The operator cut the bench to a proof of concept partway through: the solo baseline plus one lens-fan run, with more iterations only if that run was inconclusive.
It was not inconclusive, so one iteration ran.

## Verdict

The tuned 4-lens fan found more distinct real defects than a single general reviewer: 8 at MEDIUM or above against 5 and 6 for two solo runs, and 16 distinct real defects in all against 11 and 9.
It found the task's late BLOCKING defect K3, first raised in round 4 of the real run, which neither solo run found and real rounds 1-3 missed.
It cost $2.89 review side against $1.30-1.77 for a solo round, about 1.9x the solo mean, and took about as long: 347 s against 320-447 s.
Overlap is still large: 36 % of the forks' distinct finding ids repeat another fork's.
This rests on one fan run and two solo runs over one fixture.

## Reference

- Task `reed-view`, Discussion-Review, round 1: the decision record and support log in weft `f078e6da6`, plus round 1's focus file.
- It suits the bench because the real run took five rounds and 39 findings.
  Its round-4 BLOCKING (reed's own apply and resize paths drop the zoom) was in the round-1 record and rounds 1-3 missed it, a class-B late finding.
- Code: pair `twb-fan-rv`, reset to `fd5b0f58e`, which was main when the reed-view pair was created.
- Key (`key/key.md`): a grader session graded all 39 original findings against the record and the code: 35 real, 0 wrong, 4 noise, with severities re-rated.
  That gives 30 distinct real defects.
  22 were already in the round-1 record and form the key, K1-K22: 3 BLOCKING, 9 MEDIUM, 7 LOW, 3 NIT.
  The other 8 were introduced by fixers.
  7 key items were first raised in round 3 or later, among them K3 (BLOCKING) and K8 (MEDIUM).

## Arms

Every arm ran `lyx burler run` standalone in the pair.
The reviewer was `opus[medium]`, the same model as the hub's round-1 Discussion reviewer; the fixer was `haiku[low]` and its output was ignored.
All arms got the same profile: the round-1 target, the board entry as a fasit file, today's Discussion-Review rubric inlined, and round 1's focus directive in the explore step.
Stencils came from a per-arm copy of the board stencils passed through `--stencils-dir`, and lenses from a pair-layer `burler.yaml`.
No code, hub config or shared stencil changed.

- **s1, s2 (baseline):** today's single general reviewer, `cluster-fan: ""`, stock stencils plus the focus directive.
- **f1 (fan, iteration 1):** `cluster-fan: fan-bench` with four lenses, plus two stencil additions: fan-lab variant D's pre-read manifest in the explore step, and fork rules after `{{.cluster_rules}}` in the review step.

### Lens set, iteration 1

Each lens is based on a class of late BLOCKING finding in `perch-review-analysis`.
Fan-lab found that topic lenses overlap, so each lens here is a procedure with its own required list.
The plan's `enumerate-sites` lens became `existing-paths`, because the Discussion-Review rubric forbids findings about incomplete call-site enumeration.
No lens is `generic`: the handler's holistic review is the general pass.

- **`i1-interactions`.** Evidence: reed-view D1 vs D7, loom-bouncer-rounds D4 vs D3's seam budget, engine-model-choice D6 vs D7.
  Text: "Own the interactions between the record's own decisions. List every pair of decisions that act on the same state, file, key, command, ordering or invariant, including a pair where one adds a mechanism and the other adds another mechanism over the same state. For each pair, run the first decision's paths, its failure and restart paths included, with the second in force, and say whether both still hold; then check each acceptance criterion and constraint against every decision it touches. A pair where one decision silently undoes, outruns or contradicts the other is a finding. Your list: the pairs you checked, one line each, held or broken."
- **`i1-existing-paths`.** Evidence: reed-view zoom vs reed's own resize and apply paths, config-layers clone tests, loom-bouncer-rounds sibling refusal routes.
  Text: "Own what the record's changes meet in the code that already runs. For each state, setting, file or invariant a decision introduces or changes, find every existing code path that also writes, resets, reapplies or reads it, such as other verbs, hooks, resize or apply steps, restart and resume paths and background processes, starting from the definitions your context holds. For each path, say whether the decision's result survives it. A path that undoes or breaks a decision while the record is silent about it is a design finding about that decision, never a missing call-site list. Your list: the paths you traced, per decision, survives or breaks."
- **`i1-bound-breaker`.** Evidence: webster-replan-state's D8 quoting bypass, fan-lab F1's hop to `SpawnDetachedPush`, logging-cleanup D17, shuttle-state's false write bound.
  Text: "Own the record's claims and bounds. For every bound, guard, guarantee, ordering promise and claim about existing code the record states, try to build a concrete input, interleaving or path that breaks it: a concurrent caller, a process the code spawns, another entry point that reaches the same verb, a state that already exists or was left by an older version, a file in an unexpected shape. Go one hop past what the record cites. A claim you break is a finding with its counterexample. Your list: each claim you attacked, held or broken."
- **`i1-way-forward`.** Evidence: webster-dead-ends reset, loom-bouncer-rounds refusals with no retry clause, logging-cleanup unreachable guard rows.
  Text: "Own the failure and recovery ends. For each step a decision adds, state what happens when it fails, is interrupted, or meets a state it did not create, such as a stale, partial or foreign artifact, a missing file or a session that is already gone, and what the operator or the next run does then. A failure whose outcome the record leaves unstated, a refusal with no way forward, a retry that cannot make progress and a state no path leaves are findings. Your list: the end states per decision, stated or unstated."

### Reviewer prompt, iteration 1

- **Explore step** (`setup/iter1/burler-step-1-explore.md`):
  - the same focus directive the solo arms got;
  - variant D's "Reading for the fork reviewers" section, with the same-turn fix ("the manifest never ends your turn");
  - one new checklist line: the existing code paths that also write, reset or reapply each state a decision changes;
  - the board-entry line dropped, since the board entry is a fasit file here.
- **Review step** (`setup/iter1/burler-step-review.md`): a "Fork rules for this round" section after Go's `clusterRulesBlock`, which overrides that block where they differ.
  - The forks spawn in the same turn as the manifest.
  - A fork's prompt is the fixed boilerplate plus `Your lens: <name>` and the lens text; the master adds no direction of its own.
  - The boilerplate tells a fork that it owns one lens, starts with its lens's list, and reports off-lens findings only if BLOCKING.
    It gets at most 6 tool calls, only for what the manifest lacks, each listed with a reason; it is read-only and gives file-and-line evidence.
  - The handler owns what no lens owns: the record against its board entry, the leanness and relocation rules, architecture and PATTERN-fit.
  - The consolidation paragraph is unchanged.
- **Confound:** Go's own fork boilerplate ("after exploring the target fully", "fetch only what your lens needs") still renders above the override.

## Grading

One grader session (`opus[high]`) graded s1, s2 and f1 under neutral labels, plus f1's raw fork messages, against the key and the code.
It used the twill scale: real/wrong/noise, re-rated severity, and a mapping to a key id (K) or a pool id (N, real but not in the key).
Blindness was partial, because the raw fork file reveals the fan arm.

## Results

| Arm | Findings | Real | Wrong | Noise | Distinct real | Distinct real MEDIUM+ | Key hit | Late key ids | Review fresh | Review $ | Review wall |
|---|---|---|---|---|---|---|---|---|---|---|---|
| s1 solo | 9 | 8 | 0 | 1 | 11 | 5 (K4, K9, K10, K11, K12) | 7 | K15 (LOW) | 154k | 1.77 | 447 s |
| s2 solo | 10 | 8 | 0 | 2 | 9 | 6 (K2, K4, K9, K10, K11, K12) | 7 | none | 120k | 1.30 | 320 s |
| s1 ∪ s2 | | | | | 15 | 6 | 9 | K15 | 274k | 3.07 | |
| f1 fan | 14 | 11 | 0 | 3 | 16 | 8 (K1, K3, K4, K5, K9, K10, K11, K12) | 10 | K3 (BLOCKING) | 227k (master 160k, forks 67k) | 2.89 (master 1.81, forks 1.08) | 347 s |

- **What the fan found that the solo runs did not:**
  - the round-1 BLOCKING K1 (D7's row budget), which neither solo run found;
  - the round-4 BLOCKING K3, through `i1-existing-paths` only;
  - K5, first raised in round 2.
- **What it missed:** K2 (BLOCKING), which s2 found, and the MEDIUM key items K6, K7 and K8.
- **Against two solo runs together:** at about the same price, the fan had 16 distinct real defects against 15 and 8 at MEDIUM or above against 6, and both BLOCKINGs the union lacks; the union has K2.
- **Consolidation:** every real raw fork finding reached the review.
  The review kept 3 noise findings: one from the handler and two from `i1-way-forward`.
- **Fork behaviour:**
  - each fork made 2-5 tool calls, inherited about 96k of context at the spawn, and used 12-20k fresh tokens;
  - the master pre-read code and judged nothing before the spawn;
  - the master spawned in the same turn as the manifest, so the same-turn fix held.

### Per lens

| Fork | Raw | Real | Distinct ids | Ids no other fork raised | Ids shared with another fork |
|---|---|---|---|---|---|
| `i1-interactions` | 7 | 7 | 10 | K10, K13, N4, N7 | K1, K9, K11, K12, N1, N3 |
| `i1-existing-paths` | 2 | 2 | 2 | K3 | K1 |
| `i1-bound-breaker` | 9 | 9 | 8 | K4, K5 | K1, K9, K12, N1, N3, N6 |
| `i1-way-forward` | 9 | 5 | 5 | N5, K18 | N1, N6, K11 |

- Duplicate share: 9 of the 25 per-fork distinct ids repeat another fork's (36 %).
- Three forks raised K1, one of them as "outside my lens" through the off-lens BLOCKING escape.
- The record's obvious defects drew several lenses whatever their procedure: D7's row budget, D15's JSONC insertion and the pre-change pane's color.
- `i1-existing-paths` had the fewest findings but the most valuable ones.
- `i1-way-forward` was the noisiest: 4 of its 9 raw findings were noise, and the ids only it raised are LOW.

## What it shows

- A lens fan can broaden a review on this fixture when each lens is a procedure aimed at a class of defect that reviews find late.
- The decisive lens sends a fork from each decision into the code paths that already touch its state: the hop the solo runs and real rounds 1-3 skipped.
- With the pre-read manifest, `opus[medium]` forks are cheap, about $0.27 each.
  The extra cost over solo is the master's larger pre-read plus about $1.08 of forks.
- Overlap is reduced, not solved.

## Next changes if the fan is pursued (not run)

- Replace `i1-way-forward` with a walkthrough lens: execute each decision's mechanism step by step on a concrete case, with the data each step is actually handed, against the decision's own stated purpose (aimed at K2 and K6).
- Widen `i1-existing-paths` to the readers of a changed state under the new mode (aimed at K7 and K8).
- Drop the off-lens BLOCKING escape: a fork reports only what its procedure produced.
- Use real runs as the reference from here, as the operator decided.

## Cost

| Item | $ |
|---|---|
| Key grader | 2.04 |
| s1, with its fixer | 1.80 |
| s2, with its fixer | 1.33 |
| f1, with its fixer | 2.92 |
| Grader | 1.57 |
| **Total (API list prices, cap $45)** | **9.66** |

## Deviations

- One iteration instead of up to three, after the operator's scope change.
  The run also fell short of the stop rule's targets: 1.45x against 1.5x, and 36 % duplicates against 25 %.
- The planned `enumerate-sites` lens became `existing-paths`, because of the rubric's ban on enumeration findings.
- The arms used today's rubric and stencils, with the board entry as a fasit file, because `reed-view` is no longer on the board.
- Grading was only partly blind.
- The fan ran once, so its single-run noise is unmeasured.

## Files

- `setup/`: profiles, the iteration-1 stencils and `burler.yaml`, diffs against the board stencils, grader prompts, `cost.jq` and `commands.md`.
- `key/key.md`: the graded key of the original run's findings.
- `grading/grades.md`: the iteration-1 grades.
- The runs' reviews, fixer reports and raw fork messages stayed in the bench's scratch folder and are not kept.
