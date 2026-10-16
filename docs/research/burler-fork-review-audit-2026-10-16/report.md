```yaml
date: 2026-10-16
author: Claude (audit agent, Opus 5.5), for Henrik Anfinsen
subject: Burler reviewer-master that forks (board note burler-fixer-fork, operator proposal 2026-10-16)
sources:
  - PATTERN.md, pattern/PATTERN-review-round.md
  - internal/burlerengine/doc.go, internal/burlerengine/template.yaml
  - internal/shedadapters/doc.go
  - contracts/recipes/loom-recipe.yaml (Discussion/Plan/Webster/Merge Burler rows)
  - contracts/stencils/burler/*, contracts/stencils/twill/*
  - internal/twilldirective/doc.go, internal/loomengine/twill.go, internal/loomengine/config.go
  - contracts/stencils/webster/webster-prefix-fork.md, internal/websterengine/doc.go
  - internal/shuttleengine/doc.go, requires.go, claudeengine/pricing.go
  - _board/_lyx/config/loom.yaml (hub)
  - docs/research/session-fork-spike.md, twill-fixer-bench.md, twill-fork-timing-bench.md, twill-fork-timing-plan-bench.md, twill-fork0-bench.md, twill-discussion-fork0-bench.md, twill-rationale-bench.md, lens-fan-cost-analysis.md, codex-feasibility.md
  - board notes burler-fixer-fork, perch-review-analysis, fan-prompt-tuning (read only)
  - git log 572ab84e8, edb4e6984, b2e4cf913
  - tools/tokencount/tokencount.sh -last 10 -> .scratch/token-usage-by-role.md (generated 2026-10-16 12:38)
  - reviewer and fixer transcripts of 6 of those runs (per-turn context sizes, via jq)
  - fixer reports of the 60 newest archive tags in loomyard-weft (dispositions)
```

# Audit: a Burler reviewer master that forks its review, fixes, and self-reviews the fix

## Verdict up front

- The proposal's claimed gain does not survive the numbers.
  A fresh fixer is cheap: about $0.40 and about 1 minute per session, and all fixers together are 7.3 % of run cost.
  Its "re-read" is 15-35k tokens on a 20.5k startup base.
  Fixing at the reviewer's 90-140k context instead pays most of that back in cache reads, so stages 1-2 save about $0 on Discussion and lose about $0.15 on Plan per round, and save under a minute per round.
- The part of the proposal with a real payoff is the Twill-style check of the fix (stage 3 / the smaller first step), and it is cheaper in today's lean fixer than in a master, because its forks inherit a 40-65k context instead of 100-250k.
- Recommendation: build the smaller first step only (Twill self-review inside today's fixer, Discussion and Plan first), and measure it before touching the reviewer/fixer split.

## 1. How a Burler round works today

### Rows and wiring

- Each review perch (Discussion-Review, Plan-Review, Webster-Review, Merge-Review) is a three-row ring in `contracts/recipes/loom-recipe.yaml`: `<P>-Bouncer` → `<P>-Burler-Review` → `<P>-Burler-Fix` → `<P>-Bouncer`, all `on_stuck`, all with one shared `segment` label, because Burler rows never return Done (a Done ends the bounce episode).
  That is the "perch": the Bouncer row and its Burler rows, wired per segment; there is no perch type.
- `BurlerReviewProducer` and `BurlerFixProducer` (`internal/shedadapters/burlerreview.go`, `burlerfix.go`) each wrap one half of a `burlerengine` round as a one-seat `seatengine` table, one fresh shuttle session each, strands `<short>:<slug>:<prefix>-burler-review` and `-burler-fix`.
- The split into two rows (commit `b2e4cf913`, 2026-10-11) made the fixer lean on purpose: it holds the review, the target paths and the fix rules, and none of the rubric, fasit, PATTERN overview, focus file or prior rounds; every finding carries a `fix` key so the fixer can act from the review alone; an empty review spawns no fixer.
- Review-before-fix is enforced by sequencing: the review row ends, Go accepts the review and writes `round-<N>-acceptance.md` with its SHA-256 digest, and only then does the fix row spawn; the fixer is refused if the review bytes changed (`ErrReviewChanged`).
  A restart with an acceptance record resumes at the fix row without re-running the reviewer.
- Models are per half and per round (`loom.yaml` `review`/`fix` lists and the six per-segment keys).
  The hub uses this: `discussion_review: [opus, opus, sonnet[high]]` and `plan_review: [opus, sonnet[high]]`, with `fix: opus[medium]` throughout; Webster runs `sonnet[medium]` on both.

### Why the split exists (from the records)

- Commit `572ab84e8` (2026-10-07), which first split one agent into reviewer and fixer, gives the reasons as model per half ("a cheap reviewer and a stronger fixer can differ") and overlapping the fixer's orientation with the review.
  Before it, one agent did both jobs.
- Independence is recorded in two other places, not as the reason for that split:
  - `burlerengine/doc.go` "Review before fix": fixing on sight "turns the review into a post-hoc rationalization of edits already made"; this is about ordering within the round, and a single session that writes and Go-accepts its review before editing still meets it.
  - `PATTERN-review-round`: "The reviewer and the fixer are separate sessions", and "the fixer holds neither the fasit nor the rubric, so neither is a dispute ground".
    This is a PATTERN invariant, so any master design must change PATTERN in the same commit.
- How much the independent fixer actually pushes back: across the 60 newest archived runs in `loomyard-weft`, about 184 fixer reports and about 630 findings, there is **one** dispute (a darn round) and about 11 "fixed differently" (about 2 %); the rest are "fixed as proposed".
  So today's fixer's independence is mostly theoretical, which cuts both ways (see Risks).

### Fan-out and lens machinery that exists

- `burler.yaml` (`internal/burlerengine/template.yaml`) holds the lens library and fans (`standard`, `full`, `discussion`, `plan`, `twill-discussion`, `twill-plan`); `Profile.ClusterFan` turns a cluster round on; `ClusterExclude`/`ClusterExcludeHeld` trim it; the Bouncer's judge can request `exclude_lenses`; `round-<N>-usage.yaml` records lenses, forks included.
- A cluster round is already the proposal's first half: one reviewer session ("handler") explores, spawns N unnamed in-session forks (Agent tool, `subagent_type: "fork"`) in one message, does its own holistic review meanwhile, and consolidates every fork's findings into the one review file with `origin:` keys.
  It is enforced by a PreToolUse(Agent) hook (unnamed forks only) and `auditClusterRound` over `shuttleengine.ForkAudit` (exactly N forks, zero Write/Agent calls, no mutating git).
- The fan is off in the hub (`discussion_fan: ""`, `plan_fan: ""`; Webster has no key because read-only forks may not run git).
  It is parked (`fan-prompt-tuning`): fanned rounds cost 301-638k fresh against 86-121k solo on Discussion and 901k against 202-227k on Plan, found more MEDIUM/LOW but not more BLOCKING, and about half of raw fork findings were duplicates; the root cause was a master that forked before reading code (`lens-fan-cost-analysis.md`).
  Variant D (pre-read manifest, 6-call fork budget) brought a round to 150-200k, still 1.5-2x solo, and at `sonnet[high]` it found the BLOCKING defect but ran dearer and slower than solo `opus[medium]`.

### Twill and forks elsewhere

- `internal/twilldirective` renders a host-neutral lens-review block: the writer reads its rubric, spawns one read-only fork per lens after its files pass the self-check, and judges the findings with either the plain judge or the critical judge (`twill-judge-critical.md`, a copy of Burler's fixer dispositions); an optional writer-fork part (fork0) has one fork write the files so the master reviews them without the writer's reasoning.
  Plan-Write uses fork0 + critical judge (`twill-plan`), Discussion-Write the after-spawn plain judge (`twill-discussion`).
- Webster's Master forks write-capable implementers in-session (commits per card, own fork audit in `websterengine/audit.go`), so a fork that edits and commits is already done in this repo; Burler's cluster audit is the one that forbids writes.
- Fork mechanics (`session-fork-spike.md`, re-verified on CLI 2.1.228):
  - In-session fork subagents: 98-99.9 % cache hit on the first fork request, about 17k marginal compute per reviewer; same model as the parent, cannot nest, run inside the parent's pane.
  - CLI `--resume <id> --fork-session`: zero parent-specific cache reuse (the fork rebuilt 36.9k of history as cache writes), unchanged by `--exclude-dynamic-system-prompt-sections`; it is not a usable mechanism for this design.
  - Plain `--resume` with the same session id and matching flags: a full cache hit; known resume cache-busters exist (skill-listing scatter #43657/#67497, `--effort` dropped on resume #66005).

## 2. Token and time evidence

### Last 10 full runs (`tokencount.sh -last 10`)

| role | sessions | est. cost | cost/session | active time | time/session |
|---|---|---|---|---|---|
| discussion-burler-review | 22 | $20.74 | $0.94 | 60m37s | 165 s |
| plan-burler-review | 14 | $24.42 | $1.74 | 70m01s | 300 s |
| webster-burler-review | 12 | $6.08 | $0.51 | 19m01s | 95 s |
| discussion-burler-fix | 20 | $8.10 | $0.40 | 23m02s | 69 s |
| plan-burler-fix | 13 | $4.93 | $0.38 | 11m14s | 52 s |
| webster-burler-fix | 5 | $1.59 | $0.32 | 7m13s | 87 s |
| merge-burler-fix | 2 | $0.41 | $0.21 | 1m33s | 47 s |

- Run cost over the 10 runs (orch included) sums to about $205; all fixers are $15.03 (7.3 %), all reviewers $52.85 (26 %).
- Fixers average 4 sessions and 4.3 minutes of active time per run.
- The Shed hand-off between review row and fix row costs about 14-15 s of idle per entry (Discussion-Burler-Fix elapsed 28m03s against 23m02s active over 22 entries).

### Context sizes from the transcripts (6 of the 10 runs, distinct context per turn)

| session | turns | context at first turn | context at end |
|---|---|---|---|
| discussion reviewer (n=16) | 11-33 | 20.5-21k | 66-125k, median about 89k |
| plan reviewer (n=10) | 24-64 | 20.5-21k | 81-201k, median about 140k |
| webster reviewer (n=8) | 12-44 | 20.7-21k | 61-172k, median about 78k |
| discussion fixer (n=14) | 9-19 | 20.5k | 42-65k |
| plan fixer (n=9) | 8-21 | 20.5k | 37-70k |
| webster fixer (n=4) | 9-48 | 20.7-21k | 38-91k |

- A fixer's first two turns are the shared startup (about 20.5k, then about 6k of skills); its "re-read" of the review and the files is the growth to its end context, 15-35k tokens on Discussion and Plan.
- A Discussion fixer session is on average 6.4k output, 38.2k cache write and 433k cache read, which prices at Opus 5.5 (`pricing.go`: write $5, read $0.20, output $20 per MTok) to $0.13 + $0.19 + $0.09 = $0.40, matching the report.
  The whole possible saving from never paying a fixer's own context is therefore its cache write, about $0.19 a session, less what a forked or continuing fixer writes itself (its edits and tool results, about 10k, $0.05).

### What a fork or master fixer costs instead

- A fixer that runs on the reviewer's context pays each of its about 12 turns at the reviewer's size instead of today's 26-60k (average about 40k).
  - Discussion: about +50k per turn × 12 = +0.6M cache read = +$0.12.
  - Plan: about +100k per turn × 12 = +1.2M cache read = +$0.24.
- Net per round, against the fixer's $0.14 net cache-write saving:
  - Discussion: about −$0.02 (break-even).
  - Plan: about +$0.10 (dearer).
  - The subscription weight metric tokencount uses (output ×5, write ×1.25, read ×0.1) gives the same picture: 47k weight saved against 60k (Discussion) or 120k (Plan) added.
- Break-even rule: a forked or master fixer saves tokens only while the reviewer's context at the fork point stays under about 60-70k more than a fresh fixer would build, which Discussion medians barely meet and Plan medians do not.
- Time: a master fixer skips the row hand-off (about 15 s), the spawn and skill-load turns (about 10-15 s) and the reading turns (about 20-30 s): about 45-60 s per round, about 3 minutes per run, roughly 6 % of a run's 52 minutes of active producer time.

## 3. Stage by stage

### Stage 0, the smaller first step: Twill self-review inside today's fixer

What it is: the fixer fixes, then spawns K read-only forks of itself, each checking one aspect of the fix, and folds their findings in before it writes its fixer-report.
This is `perch-review-analysis` proposal 5 (lenses: consistency of siblings, claims against the code, way forward can be carried out).

Feasible with what exists; to build:

- `ForkSubagents` on the fix half's spec (today set only for a fanned reviewer) and the Agent hook, which already exists.
- The twill directive rendered into `burler-step-fix.md` through a new optional marker, with fixer values: reviewed files = the target paths (overlay) or the changed files (source); governing input = the review and the code; critical judge part; the final self-check = the row's gate (`validate-discussion`, `validate-plan`, `verify`).
  `twilldirective.Values.RubricPath` is required, and the fixer holds no rubric by design, so it needs a short fix-check stencil to point at, not the review rubric.
- New lenses and a fan in `burler.yaml` (for example `fix-check: [consistency, claims, way-forward]`), and a `loom.yaml` key per perch, closed to the local layer like `plan_lenses`.
- A fork audit on the fix half: exactly K forks, no Write, no Agent, no mutating git (reuse `auditClusterRound`'s rules on the fix half's `ForkAudit`).
- A recorded lens review in the fixer-report and a gate that checks it, as `validate-plan --lens-review` does, and the twill relaunch rules for a resumed fixer (`ProbeFix`/`ResumeFix`).
- Docs: `burlerengine/doc.go`, `PATTERN-review-round` (the round now grades its fix once, inside the round, which "A single burler round never grades its own fix" in `doc.go` currently forbids in spirit: that sentence must be rewritten, since a cross-round fresh reviewer stays the independent gate).

Expected cost and time per fixer session (Opus, Discussion/Plan):

- Forks inherit the fixer's end context, 42-65k.
  Scaling the twill after-arm forks (`twill-fork-timing-bench.md`: $0.18-0.25 a fork at writer contexts of about 120-190k, 5.7k cache write a fork) to that context gives about $0.13-0.18 a fork, so $0.40-0.55 for three.
- Folding the findings: 4-6 more fixer turns at 60-70k context, about $0.13-0.20.
- Total about +$0.55-0.75 a fixer session, more than doubling a $0.40 fixer; about +$1.8-2.5 a run on Discussion and Plan (3.3 such sessions a run), +9-12 % of the mean $20.5 run.
- Time: twill after-arm forks ran 1m08s-1m21s wall, plus about 0.5-1 minute of folding: about +2 minutes a fixer session, +6-7 minutes a run.
- Break-even: one avoided review round costs a review ($0.94 Discussion, $1.74 Plan), a fix ($0.40) and a judge (about $0.13), plus 3-6 minutes, so the self-review pays if it removes one round per 2.5-4 self-reviewed fixes.
  It also covers a gap no cost metric shows: in most long runs the closing round's fix was never reviewed again (`perch-review-analysis`), and Discussion's late BLOCKINGs were mostly fix-introduced (class A: 4 of 7).

Evidence on quality: `twill-fixer-bench.md` found the critical fixer safe (every dispute and own fix right, no real finding turned down) but no better on the final plans; `twill-discussion-fork0-bench.md` found the critical judge brought in key defects twice at Discussion where lenses raised no wrong findings.
So the value has to come from checking the fix after it is made, not from more critique before it, which is exactly this stage's shape.

### Stages 1-2: the reviewer master forks N review forks, then fixes

What it is: the reviewer explores, forks N lens reviewers (N = 1, generic, for now), waits, and fixes the findings itself.

Feasible in mechanics, but it is mostly a rebuild of the parked cluster round plus a fix phase:

- The fork-and-consolidate half exists (cluster round); with N = 1 the fork does today's review and the master only relays it, so the fork adds overhead and no coverage.
  Its one purpose would be fork0-style distance: a master that did not reason the review judging it critically.
  Today's fixers dispute about 0 of 630 findings, and the critical-fixer benches found no quality gain, so that distance has no measured value yet.
- If the fork returns findings as a message and the master writes the review file, the review's output is paid twice (3-8k tokens, $0.06-0.16, and 1-2 minutes of output time); letting the fork write the review file avoids that but needs a fork audit exception for one path.
- Go must accept the review on disk before the master's first edit.
  Today acceptance happens between two rows; one session needs a mid-session acceptance: the master ends its turn with the review written, the review gate passes, Go records the digest and types the fix step into the same live session (reed's guarded paste-submit, as gates already re-prompt a live session).
  The two rows either merge into one producer (undoing `b2e4cf913` and its resume identities) or the fix row continues the review row's session.
- Model per half is lost: forks and the master run one model.
  The hub's later rounds pair a `sonnet[high]` reviewer with an `opus[medium]` fixer; under a master one of them changes, and `fan_review`'s rule "replacing that segment's review list while its fixer list is kept" stops making sense.
- PATTERN changes: "reviewer and fixer are separate sessions" goes, and "the fixer holds neither the fasit nor the rubric" goes, so the dispute grounds must say that rubric and fasit are still not dispute grounds even though the master holds them.
- Webster-Review: the master can run the read-only git the subject needs and forks inherit it, so the "forks cannot run git" reason for keeping Webster solo weakens; the master would run the `verify` gate and the per-fix commits.

Expected effect per round: tokens about level on Discussion (−$0.02 from the fixer, +$0.06-0.16 if the review is relayed) and about +$0.10-0.25 on Plan; time about −45-60 s per round from the skipped hand-off and re-read, minus any relay time.
This does not pay for the code, PATTERN and recipe change.

A cheaper way to test the "no re-read" claim alone, if it is ever wanted: let the fix row resume the reviewer's own session (plain `--resume`, same session id, full cache hit per the spike) with the fix prompt.
It keeps both rows and the acceptance between them, needs no fork machinery, and its cost is the same arithmetic as above; it loses independence and per-half model the same way.

### Stage 3: the master forks again for a Twill review of its fix

Same mechanism as stage 0, but the forks inherit the master's context: exploration plus review plus fix, 100-250k.

- At twill's measured $0.18-0.25 a fork for those context sizes, three forks cost $0.55-0.75, and folding at 150-250k costs about $0.25-0.35: about +$0.8-1.1 a round, about $0.3 more than stage 0 for the same check.
- Upside over stage 0: the forks also inherit the full exploration, so a claims or consistency fork needs fewer reads to check a sibling decision the fixer never opened.
  Downside: they also inherit the master's framing, the same reading that produced the findings.

### Stage 4: the fix itself in a fork

- Feasible: Webster already runs write-and-commit forks with their own audit; Burler needs a per-fork-role audit (the fix fork may write the target and commit under source scope; review and check forks stay read-only), and a fork cannot spawn forks, so the master spawns the check forks after the fix fork returns.
- Cost: the fix fork pays the master's context per turn exactly as a master fix does, and the master must then read the changed text in full to review it blind (+10-40k cache write, +$0.05-0.20), as twill fork0's step d does.
- Evidence: fork0 was level in cost and sharper in lens findings at Plan (`twill-fork0-bench.md`, three benches in that direction) but did not pay at Discussion (22 % dearer, more key items repeated, `twill-discussion-fork0-bench.md`); the alive-fork0 variant cost $0.75-1.00 more per plan with no quality gain.
  For a fix, which is short and local, the case is weaker than for writing a plan.

## 4. Risks

- **Context bloat.** A Plan master ends its review at a median 140k and up to 201k; adding a fix (+20-40k), returned fork reports (10-20k) and folding (10-20k) puts it at 200-280k.
  Every later turn and every fork pays that size, and on a model with a 200k window it would hit compaction; the stage 0 fixer stays at 40-80k.
- **Independence.** A master that fixes findings formed from its own exploration shares that exploration's misreadings; the class-A findings in `perch-review-analysis` include two where the fixer wrote the reviewer's false premise into the record.
  Against that, today's fresh fixer already accepts about 98 % of findings as proposed, so the split buys little independence in practice; the cross-round fresh reviewer is the gate that matters, and every stage keeps it.
- **Fork cache behaviour.** Only in-session fork subagents share the cache; CLI `--fork-session` rebuilds the whole history as cache writes and would make stages 1-4 dearer than a fresh fixer.
  Fork cost scales with fork turns (about 3.5-4k fresh per turn, `lens-fan-cost-analysis.md`), so a fork that re-reads, as before-arm twill forks did, loses the inheritance; the fork boilerplate's 6-call budget is required.
  The fork flag and the unnamed-fork rule are Claude Code version facts (`burlerengine/doc.go`, version pinning).
- **Engine neutrality.** Every fork stage requires of an engine: in-session forks that inherit the conversation and share its prompt cache; a fork audit it can produce (`ForkAudit`, write and git facts per fork); usage reading with forks included; and, for any one-session variant, a mid-session continuation that keeps the cache.
  `shuttleengine` already refuses a `ForkSubagents` spec on an engine without fork support; Codex has subagents and `/fork`, but whether they inherit history is undocumented (`codex-feasibility.md`, class c).
  So the two-session split must stay as the fallback path for any engine without forks, and a master design means keeping two round shapes alive.
- **Interaction with fan-out plans.** Stages 1-2 are the cluster round with a fix added, so they inherit its open problems (master pre-read, lens overlap, about half duplicate findings) and its parked status; building them on N = 1 ships the overhead before `fan-prompt-tuning` shows a fan that pays.
  Where it does pay off later: `perch-review-analysis` proposal 2 (two independent reviewers in round 1) costs about $0.2-0.35 per extra fork on a shared exploration against $0.94-1.74 for a second cold reviewer, though forks share the master's blind spots (every twill bench found lens blind spots no arm reached).
  Stage 0 does not touch the reviewer, so it composes with any later fan decision.
- **Resume.** Today a dead fixer is re-spawned from the accepted review without re-running the reviewer; a dead master loses the exploration, and its recovery is today's fresh fixer, so the fallback path must stay built anyway.

## 5. Recommendation

1. Build stage 0 only: Twill self-review in today's fixer, with the three fix-check lenses and the critical judge, on Discussion-Review and Plan-Review first (overlay scope, read-only forks, no git in forks), behind a per-perch `loom.yaml` key that defaults off.
   It needs `ForkSubagents` on the fix half, a fix-check stencil for `RubricPath`, a fix-half fork audit, a lens-review gate on the fixer-report, relaunch rules, and the `PATTERN-review-round` and `doc.go` wording.
2. Do not build stages 1-4 on the token and time argument: the fresh fixer's re-read is 15-35k tokens and about a minute, and fixing at the reviewer's context pays it back in cache reads.
   Revisit only if a fan comes back on (where the master already holds the forks' context) or if the per-half model is given up for other reasons.
3. Before going further, a measurement over 15-25 runs with the key on, using the `perch-review-analysis` scripts and `tokencount.sh`, should show:
   - added cost and time per self-reviewed fixer session close to the estimate (+$0.55-0.75, +2 minutes); a figure above $1 means the forks are re-reading;
   - fewer fix-introduced (class A) findings in the next round's review, and in the closing round's self-review, findings a later round would otherwise never see;
   - mean rounds per perch and the round-2 BLOCKING share falling enough that at least one round is saved per 2.5-4 self-reviewed fixes (Discussion mean today 2.4-2.5 rounds, Plan 1.6-1.8);
   - the share of fork findings the fixer acted on that were real, and no rise in key defects brought in while fixing (the risk `twill-discussion-fork0-bench.md` saw with the critical judge at Discussion).
   If rounds do not fall, keep the key only for the closing round, where no later review exists.
