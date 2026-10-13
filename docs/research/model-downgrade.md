# Model downgrade candidates

Analysis of where lyx can run a role on a cheaper model, from the model-spec resolution, the config templates, the hub config in `_board/_lyx/config/` and the 6-run report in `docs/research/token-usage-by-role.md`.
Cost shares are of the $237 role cost of those 6 runs (the orch adds $34 on top, 12.5% of $271).

## Findings that shape the answer

- Price table (`tools/tokencount/cost.go`): Opus 5.5 is $4 input, $5 cache write, $0.20 cache read, $20 output per million tokens.
- Sonnet 5.5 is $2, $2.5, $0.20 and $10, so Sonnet halves cache writes and output but its cache read costs the same as Opus.
- Roles are dominated by cache reads (long agentic sessions), so Opus to Sonnet saves about 30-40% of a role's cost, not 50%.
- Haiku 5.5 is $0.10, $0.125, $0.01 and $0.50 for prompts up to 100k tokens, and $0.50, $0.625, $0.05 and $2.50 above; it is 4x to 20x cheaper than Sonnet, so Haiku is where the large savings are.
- The driver already runs on Haiku in the hub (`driver: haiku[medium]`, $0.20 for 6 runs), so the "log readers on Haiku" idea is already applied where it fits; it is under 0.1% of spend.
- The embedded default `internal/loomengine/template.yaml:32` is still `driver: sonnet[medium]`, so a fresh hub pays Sonnet for it; the saving is negligible but the default has drifted from the hub.
- Most spend is not log reading: it is code and plan writing, review and Webster implementation.
- Forks cannot take a model: the Agent tool's `fork` type always runs on the parent's model, so every fork role is hardwired to its parent's setting.

## Inventory and proposals

Hub config is `/home/knatte/Code/loomyard-LYXHUB/_board/_lyx/config/` (called hub below); template defaults are under `internal/*/template.yaml`.
"Settable" means a config key sets that role's model alone.

| role | today's model/effort | where set | settable | share of cost | proposal | risk |
|---|---|---|---|---|---|---|
| Plan-Write producer (`plan`) | opus[medium] | hub loom.yaml:6; template loom:6 | yes | 24.2% | keep | design and card writing; a bad plan costs a whole run |
| Discussion-Write (`discussion`) | opus[medium] | hub loom.yaml:1; template loom:1 | yes | 6.6% | keep | design judgment, interview |
| Discussion advisors (seats, unused: producer is twill) | sonnet[high] | hub loom.yaml:5 | yes (list) | 0 | none | not in use |
| Plan/Discussion lens forks (twill) | inherits writer: opus | `contracts/stencils/twill/twill-directive.md:11` (Agent `subagent_type: "fork"`) | no, hardwired | 8.1% (plan+sub 5.2%, discussion+sub 2.9%) | keep; reduce lens count or context instead | forks share the writer's cached prefix; a cold Sonnet subagent would lose that and read the context again |
| Discussion-Review reviewer (`discussion_review`) | opus[medium] | hub loom.yaml:11; template loom:11 empty, falls to `review` loom:9 | yes | part of review chair 22.6% | Sonnet[high] | reviewer catches BLOCKING design findings (round 2 of fork-wait-noise raised 2); Sonnet misses some, a later parent review catches them |
| Plan-Review reviewer (`plan_review`) | opus[medium] | hub loom.yaml:13; template loom:13 | yes | part of review chair (Plan advisor is 26% of all Burler and Bouncer spend) | Sonnet[high], or Sonnet round 2 onward via list | highest-value review; a missed plan flaw reaches Webster; try round 2+ first |
| Webster-Review reviewer (`webster_review`) | sonnet[medium] | hub loom.yaml:15 | yes | in review chair | keep (already Sonnet) | none |
| Fan reviewer and its forks (`fan_review`) | sonnet[high] | hub loom.yaml:19; template loom:19 | yes | small | keep | none |
| Discussion/Plan fixer (`discussion_fix`, `plan_fix`, `fix`) | opus[medium] | hub loom.yaml:10,12,14 | yes | burler-fix-chair 6.9% | Sonnet[medium] | fixer applied all findings in 31 of 31 archived rounds (0 disputed); editing a plan or discussion is mostly mechanical, risk is a sloppy edit that reopens a key |
| Webster fixer (`webster_fix`) | sonnet[medium] | hub loom.yaml:16 | yes | in fix chair | keep | none |
| Bouncer judge (`judge` and `*_judge`) | sonnet[medium] | hub loom.yaml:20; template loom:20 | yes | 2.4% | Haiku[medium] | it is a sit-rep reader: it reads a Go-rendered facts file, the latest review and the ledger, never the artifacts (`bouncer-template-judge.md`); the risk is a false CONVERGED, which ends review early, or a false CONTINUE, which buys a $1.5-4 round |
| Bouncer seed (`seed` and `*_seed`) | sonnet[medium] (follows judge) | hub loom.yaml:21; template loom:21 | yes | 1.3% | Haiku[medium] | writes focus lists from the review format; format errors are rejected by Go and retried |
| Webster Master and implementer forks | sonnet[medium] | hub webster.yaml `master` (line 1); template websterengine:1 | yes for Master; forks inherit it | 21% (webster+sub 19.7%, webster 1.3%) | pilot Haiku[high] on a low-risk run | writes the code; forks run at 50-410k context, so on Haiku most reads bill at the long-prompt card ($0.05), still 4x cheaper than Sonnet; risk is more stuck batches, which go to the Opus recovery strand and the Webster review |
| Webster recovery strand | opus[high] | hub webster.yaml `recovery` (line 2) | yes | in webster | keep | rare, cold, hard cases |
| Driver (llm-driven loom driver) and its repair fork | haiku[medium] hub; sonnet[medium] template | hub loom.yaml:32; template loom:32 | yes | 0.1% | keep; set template default to haiku[medium] | negligible; the repair fork inherits it and reads traces, which is the log-reading case |
| Arbiter | opus[high] | hub loom.yaml:33 | yes | 1.1% | keep | settles a stop the driver cannot; a wrong verdict wastes a run |
| Alignment agent | opus[high] | hub loom.yaml:36 | yes | 2.8% | keep; consider effort medium | approves PRs for landing with no orch turn |
| Merge-conflict resolver | opus[high] | hub landing.yaml `conflict`; template landingshed:3 | yes | 0.9% | Sonnet[high] | most conflicts are trivial, but a wrong resolution lands in main; saving is under $1 per 6 runs |
| Describe | sonnet[medium] | hub landing.yaml `describe`; template landingshed:5 | yes | 0.6% | keep, or Haiku | text is the PR title, body and landing commit message; saving $1 per 6 runs is not worth a weaker summary |
| Friction reflection | sonnet[medium] | hub loom.yaml:30 | yes | 0.1% | Haiku[medium] | files notes from the friction log; a weak note costs little; saving cents |
| PR-Rework | takes `plan` (opus[medium]) | template loom:28 (`rework`) | yes | 0.2% | keep | code and plan fixes after review |
| Darn writer | opus[medium] | `internal/darnengine/template.yaml:1` | yes | not in report | keep | writes code |
| Orch session and its parent-review forks | provider default (Opus) | hub orch.yaml `model`, `effort` (empty) | yes (orch only) | 12.5% of all cost (orch $20.56, orch+sub $13.41) | keep for now | the orch fixes bugs and makes calls; the parent-review and Bouncer-escalation forks inherit its model and cannot be set apart |
| Parent-review reviewer fork, Bouncer escalation fork | inherits the orch/parent | `loom-template-parent-review-brief.md`, `bouncer-template-escalation.md` | no, hardwired | in orch+sub | keep | judgment calls on design |
| Preflight, Batchifier, Loom-Preflight, Publish, Finalize, PR-Gate verify | no model (Go only) | n/a | n/a | 0 | n/a | n/a |

## Ranked changes by expected saving (6-run totals)

1. Webster Master to Haiku[high], as a pilot: saves about $30 (13% of role cost) if it holds; config-only (`webster.yaml master`).
   It is the biggest lever and the biggest quality risk, so run one low-risk task first and compare stuck-batch count, recovery spawns and Webster-Review findings.
2. Plan-Review and Discussion-Review reviewers to Sonnet[high]: saves about $10 (4-5%); config-only (`plan_review`, `discussion_review`).
   Start with a per-round list (Opus round 1, Sonnet from round 2) to keep the first, highest-yield review on Opus.
3. Bouncer judge and seed to Haiku[medium]: saves about $7.5 (3%); config-only (`judge`, `seed`).
   Both read prepared facts, which is the log-reading case the operator describes.
   Watch false CONVERGED against archived rounds before adopting.
4. Discussion and Plan fixers to Sonnet[medium]: saves about $4 (1.7%); config-only (`discussion_fix`, `plan_fix`).
5. Fork the fixer from the reviewer: part of the $1.9 per run in `.scratch/burler-cost-breakdown.md`; needs code in `internal/burlerengine`.
   Skipping the fixer on an empty review is already built (`internal/burlerengine/fix.go:3`, no findings and passing gates spawn no fixer).
6. Small items, each under $1: conflict resolver to Sonnet, friction to Haiku, driver template default to haiku, describe to Haiku; config-only except the template default, which is a one-line edit of `internal/loomengine/template.yaml:32` (plus its test fixtures).

## Needs code, not config

- A per-role model for twill lens forks, parent-review forks and Bouncer escalation forks: the fork type cannot take a model, so it would need a separate non-fork subagent with an explicit model, which loses the shared cache prefix.
- Fixer forked from the reviewer: the code change in item 5.
- Webster per-batch model (cheap model for mechanical cards, Sonnet for the rest): the Master holds one model for the run today, so this needs code in `internal/websterengine`.

## Not recommended

- Moving plan, discussion, arbiter, alignment or recovery off Opus: low share of cost per decision, high cost of a wrong one.
- Treating "reads logs" as a broad Haiku class: only the driver, the judge, the seed and friction fit, together about 3.9% of spend, and the driver is already Haiku.
