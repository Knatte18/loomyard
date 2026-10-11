# Discussion bench: chair and advisors against the solo writer

Measurement report from the first round of the `discussion-multi-llm-bench` board note (2026-10-11).
It compares the seats producer (one chair with at most one advisor), the seats producer with no advisor (N=0), and the single producer, on one task.
Each configuration ran once, so read every difference below as one sample, not as a measured effect.

## Setup

- Task: `webster-stability`, its board body copied into one temporary board note per bench pair, on code base `800a0e892`.
- Reference: the run that wrote that task's discussion on 2026-10-10 with the single producer on `fable[high]`.
  Its first draft is records commit `82e8b0f9`, and its final discussion after three Discussion-Review rounds is `e77a1372`, both on the `archive/webster-stability/*` tag in the records repository.
  The final discussion is the best available reference, itself one model's output after review, not an answer key; where a bench draft departs from it, the departure is judged on its merits below.
- One bench pair per configuration (`dbench-<config>`), created with `lyx fabric add`, its code branch reset to the base, its models set in the pair's own `loom.yaml` (`discussion_producer`, `discussion`, `discussion_advisors`).
- Each pair was seeded with `lyx shed seed self --recipe loom --driver go --param parent=main` and stepped with `lyx loom step` through Discussion-Write only, with no driver session.
  The bench agent approved each pair's Discussion parent-review gate as soon as it opened, then copied the decision record, support log and advisor notes out.
- The DualDW run (`opus[high]` chair, `sonnet[high]` advisor) ran alone first and was watched; the other seven ran in parallel after it.
- Binary: `lyx` at `97d66bb00`, which includes `seats-chair-first` (`6df79b99d`).
- Confounders: the bench chairs read the board as it stood on 2026-10-11 (related entries such as `gate-test-hygiene` had moved on), and the stencils are newer than the reference run's.

## Prices

Costs are list prices in USD per million tokens, taken from `tools/tokencount/cost.go` and confirmed against the claude-api skill's model table (Claude Code 2.1.296, models cached 2026-10-06).

| Model | Input | Cache write (5 min) | Cache write (1 h) | Cache read | Output |
|---|---|---|---|---|---|
| `claude-fable-5-1` | 10 | 12.50 | 20 | 0.25 | 50 |
| `claude-opus-5-5` | 4 | 5 | 8 | 0.20 | 20 |
| `claude-sonnet-5-5` | 2 | 2.50 | 4 | 0.20 | 10 |

Tokens are counted once per API message id from each seat's Claude Code transcript (sub-agent transcripts included), and the cache writes are split into their 5-minute and 1-hour parts from the usage record.
Wall time is the seat's first to last API message; for an advisor configuration it is the chair's span, which covers the advisor's.

## Results

| Config | Chair (writer) | Advisor | Wall | Cost | Chair cost | Advisor cost | Batches | Reference findings repeated | Partly repeated |
|---|---|---|---|---|---|---|---|---|---|
| reference draft | single `fable[high]` | none | 12m11s | $5.58 | $5.58 | | | 7 of 7 round-1 findings | |
| `dbench-ohsh` | seats `opus[high]` | `sonnet[high]` | 9m06s | $4.46 | $2.36 | $2.10 | 2 | 5 | 1 |
| `dbench-ohom` | seats `opus[high]` | `opus[medium]` | 8m09s | $4.52 | $2.44 | $2.08 | 2 | 2 | 3 |
| `dbench-ohsm` | seats `opus[high]` | `sonnet[medium]` | 6m42s | $2.75 | $2.17 | $0.58 | 2 | 2 | 3 |
| `dbench-omsm` | seats `opus[medium]` | `sonnet[medium]` | 6m35s | $2.20 | $1.69 | $0.51 | 2 | 6 | 2 |
| `dbench-n0fm` | seats N=0 `fable[medium]` | none | 7m39s | $3.72 | $3.72 | | | 6 | 1 |
| `dbench-n0oh` | seats N=0 `opus[high]` | none | 6m07s | $2.16 | $2.16 | | | 0 | 0 |
| `dbench-n0om` | seats N=0 `opus[medium]` | none | 4m45s | $1.83 | $1.83 | | | 3 | 2 |
| `dbench-solo` | single `opus[medium]` | none | 4m21s | $1.71 | $1.71 | | | 4 | 2 |

The reference run's three Discussion-Review rounds raised 13 findings: seven in round 1 against the draft, five in round 2 and one in round 3 against the fixed text.
"Repeated" counts the findings whose defect the bench draft also has; a finding that does not apply because the draft took another route counts as not repeated.

Token detail per seat (input / cache write / cache read / output):

| Config | Seat | Model | Messages | Input | Cache write | Cache read | Output |
|---|---|---|---|---|---|---|---|
| reference | writer | `claude-fable-5-1` | 20 | 520 | 191,993 | 2,360,200 | 51,704 |
| `dbench-ohsh` | chair | `claude-opus-5-5` | 55 | 116 | 135,632 | 4,559,094 | 38,362 |
| `dbench-ohsh` | advisor | `claude-sonnet-5-5` | 53 | 108 | 193,807 | 5,938,845 | 43,245 |
| `dbench-ohom` | chair | `claude-opus-5-5` | 56 | 116 | 138,956 | 5,022,759 | 37,067 |
| `dbench-ohom` | advisor | `claude-opus-5-5` | 55 | 112 | 128,175 | 4,431,015 | 27,620 |
| `dbench-ohsm` | chair | `claude-opus-5-5` | 55 | 114 | 123,101 | 4,492,503 | 32,916 |
| `dbench-ohsm` | advisor | `claude-sonnet-5-5` | 26 | 56 | 71,552 | 1,275,650 | 14,444 |
| `dbench-omsm` | chair | `claude-opus-5-5` | 49 | 102 | 100,658 | 3,295,487 | 26,579 |
| `dbench-omsm` | advisor | `claude-sonnet-5-5` | 22 | 48 | 67,540 | 1,038,029 | 13,181 |
| `dbench-n0fm` | chair | `claude-fable-5-1` | 21 | 552 | 130,274 | 1,708,728 | 33,081 |
| `dbench-n0oh` | chair | `claude-opus-5-5` | 50 | 100 | 125,834 | 4,185,593 | 34,666 |
| `dbench-n0om` | chair | `claude-opus-5-5` | 47 | 94 | 116,979 | 3,553,056 | 26,730 |
| `dbench-solo` | writer | `claude-opus-5-5` | 51 | 102 | 101,442 | 3,636,185 | 23,588 |

## Grading against the reference findings

Key: R repeated, P partly repeated, A avoided or fixed, n/a the draft took a route the finding does not apply to.

| Reference finding | ohsh | ohom | ohsm | omsm | n0fm | n0oh | n0om | solo |
|---|---|---|---|---|---|---|---|---|
| R1-F1 D1's deadline outcome stated as `OutcomeTimeout` | n/a | P | P | P | R | A | R | P |
| R1-F2 `unix` and GOOS implications missing from the environment set | R | A | A | R | R | A | A | R |
| R1-F3 recovery writes invisible to contract-file evidence | A | R | A | A | R | A | R | R |
| R1-F4 reset own-path widening without a stated bound or criterion | R | R | P | R | R | A | A | R |
| R1-F5 per-entry re-prompt cannot carry the findings path | A | A | A | R | A | A | A | A |
| R1-F6 Scope item carries probe history | A | A | A | A | A | A | A | A |
| R1-F7 no criterion for the Rename-new done-check or plan-gate member reads | P | P | P | P | P | A | P | P |
| R2-F1 Rename of a partitioned member undecided | A | P | A | R | R | A | P | R |
| R2-F2 base `build.Context` fixes built-in tags true | R | A | A | R | A | A | A | A |
| R2-F3 free-tag cap has no value | R | A | R | A | R | A | R | n/a |
| R2-F4 recovery-kept contract finding still says "a fork wrote" | R | n/a | R | R | n/a | A | n/a | n/a |
| R2-F5 cost claim mismatches `MatchFile` | A | A | A | A | A | A | A | A |
| R3-F1 partitioned Rename way forward names a per-file Create | n/a | n/a | n/a | n/a | n/a | A | n/a | n/a |

Notes on the grades:

- R1-F1 is graded P where the draft bounds the hold by the deadline but never says which outcome the deadline yields; it is R where the draft says the run then ends `timeout` (n0om says "never `done`", n0fm records a "timeout" end), which `PATTERN-completion-signal` forbids.
- R1-F3 is R in ohom because it decides that contract-file evidence "keeps reading `RunWrites.Master` alone", the same false premise the reference draft had; in n0om and n0fm the recovery writes land in a slice the evidence never reads; in solo they are audited "like a Master session", so a recovery's later write would clear the finding as Master's.
- n/a under R2-F4 means the draft never routes recovery writes onto the fork side, so its defect is the R1-F3 one instead.
- The solo draft decides disjointness by satisfiability over free booleans with one-hot GOOS and GOARCH, so it has no tag enumeration and no cap to name (R2-F3 n/a) and holds no built-in tag true (R2-F2 A), but a free `unix` tag reads `unix` beside `_windows.go` as overlapping (R1-F2 R).

## Departures from the reference, judged on the merits

- Item 1, the gate shell.
  The reference holds every gated run's turn end on any outstanding task, in shuttle.
  n0oh reaches the same decision, with the same deadline outcome, and argues it more broadly: every agent is told to run `lyx gate test` in the background, so any gated producer can hit the fault.
  n0fm holds on a `lyx gate` label shuttle-wide; ohom, ohsm, omsm, n0om and solo add a `lyx gate` prefix to Master's awaited shells only, which a `cd x &&` command defeats (ohsm alone matches after `&&`, `||`, `;` and `|`).
  ohsh departs: Master never runs tests (a stencil rule), the ended-shell record says the result was discarded, and shuttle's wait is unchanged, on the ground that the verify gate re-runs every tier.
  The reference is better here: ohsh's rule binds Master only, while the at-once count it leaves in shuttle still lets any other gated producer read `done` across an unread gate shell.
- Rename of a partitioned member.
  The reference decides that a Rename's old side stays Found-only and names a way forward the plan format accepts; n0oh decides the same with the same two routes, ohsh and ohsm keep the old side refused without naming a way forward, and the rest leave it open.
- `lyx quarry resolve`.
  ohsh rewrites a partitioned `ambiguous` to `multipart` inside the exported `Resolve`, so the CLI answers differently; ohom keeps the CLI raw because quarrycli's help promises never to re-shape an answer.
  ohom's reading is the better one, and the reference does not touch the CLI either.
- Delete-then-create of one name (#507's first option).
  The reference keeps it out of scope, since per-constraint Edit and Delete replace the workaround; omsm builds a waiver for `create-already-exists`.
  The reference is better: the waiver adds an ordering rule to the plan gate for a case the partitioned-member decision already removes.
- Recovery exit code.
  ohsm and solo keep exit 0 for `dead` with `recovery_retry: true`; the reference and the others exit non-zero for every non-done terminal digest.
  Either is defensible, since Master reads the digest fields; the reference's single rule is simpler to state.
- Smaller additions the reference lacks, each sound: omsm and solo add a Master stencil line never to run `lyx gate`; ohsm matches awaited prefixes after shell separators; ohom and n0fm name an `x`/`x_test` same-name pair as never partitioned.

## How the chairs used their advisors

Every chair with an advisor started first, read the task and code, then sent two question batches and took the answers into the decisions.

- ohsh: the advisor corrected the chair's cause for item 1 (Master started the shells before any gate message), proposed routing recovery writes to the fork side, and proposed the single `resolveTargets` chokepoint, all adopted.
- ohom: the advisor required the environment set to be a superset of real contexts with `unix` fallbacks on both sides, read the recovery session id at spawn through `FindRun`, and kept `lyx quarry resolve` raw, all adopted.
- ohsm: the advisor added the `unix`, `android` and `ios` implications and the separator-aware prefix match, both adopted.
- omsm: the advisor disagreed with the chair's independent-boolean evaluation, since it can never read `_linux.go` beside `_windows.go` as disjoint, and the chair switched to `MatchFile` over enumerated contexts.

## What the round shows

- Every bench configuration finished Discussion-Write faster and cheaper than the reference draft (12m11s, $5.58), and every bench draft repeated fewer of the reference's round-1 findings than the reference draft's seven.
  Part of that gap is the newer stencils and board, not the producer.
- An advisor did not buy a better draft in this round.
  The best draft came from seats N=0 on `opus[high]` (n0oh, $2.16, 6m07s), which repeats none of the 13 findings and matches the reference final discussion's decisions nearly one for one.
  The same chair with an advisor drafted worse with `sonnet[high]` (ohsh, 5 repeated, $4.46) and with `opus[medium]` (ohom, 2 repeated, $4.52), and about as well with `sonnet[medium]` (ohsm, 2 repeated, $2.75).
  With one sample per configuration this does not show advisors hurt; it shows the chair's own model and effort moved the result more than the advisor did.
- An advisor at `high` effort, or on Opus, roughly doubles the cost of a run; one at `sonnet[medium]` adds about a quarter.
  With `opus[high]` as chair, an advisor added from 35 seconds (`sonnet[medium]`) to three minutes (`sonnet[high]`) of wall time over N=0, partly because the advisor's up line lands only once the chair's first turn has ended.
- Seats N=0 matches the single producer on the same model: `opus[medium]` cost $1.83 against $1.71 and took 4m45s against 4m21s, with drafts of similar quality (3 repeated and 2 partly repeated against 4 and 2).
  This supports parking the single producer behind seats with an empty advisor list, as the board note proposes, though one pair of runs is a thin base.
- `fable[medium]` at N=0 drafted worse than `opus[high]` at N=0 and cost more ($3.72 against $2.16).

## Bugs and friction found

- `lyx loom step --help` contradicts itself: its first sentence says step "bootstraps this worktree's loom task exactly as \"lyx loom start\" does" (`internal/loomcli/cli.go:313`), and the same text says it "refuses when no seed already exists ... since it never writes one itself" (`internal/loomcli/cli.go:329-330`).
  The refusal is what happens: `lyx loom step` on a fresh pair answers "no seed found for run \"self\"; ... run \"lyx loom start\" first", which spawns a driver; `lyx shed seed self --recipe loom --driver go` is the driverless route and the help does not name it.
- A seats chair's planned waits notify the parent as stalled runs.
  The chair stencil tells the chair to end its turn until each advisor's up line arrives, and it ends turns again while waiting for answers, but `internal/seatengine/prompt.go:160` sets `QuietHold` only for advisors (`QuietHold: !isChair`), so each of those turn ends goes through `notifyHeld` (`internal/shuttleengine/hold.go:53`) as a "turn end held, output files missing" notice to `ly:orch`.
  Each advisor run logged three or four such held turn ends; N=0 runs logged none.
- The advisor's up line lands only once the chair is idle, so in the watched run the advisor waited from 10:44:32 until the chair's first turn ended at 10:48:01, and the line landed at 10:48:05; the up line cannot reach a chair mid-turn.
- Parallel bench pairs saw "status push failed, next transition will catch up" with "push rejected (remote diverged)" on their records side (`internal/statuscommit/statuscommit.go:68`); the next transition caught up as the warning says, and no run was harmed.
- Method friction, not a lyx fault: each bench pair's parent-review brief goes to `ly:orch`, the parent `hubgeom` derives from the pair's origin record, and there is no per-pair switch to address it elsewhere.
- Not a lyx fault: the first attempt at the DualDW run paused before Discussion-Write because the earlier bench agent ran a bare `lyx loom pause` when the run reached that row, intending a stop after it; `lyx loom pause --after Discussion-Write` is the stop it meant.

## Next round

The board note's second round (two advisors, Burler rounds to convergence, total time and cost to the final discussion) should repeat each configuration at least twice, since the spread between single runs here is as large as the spread between configurations.
