# Claude usage outside the counted runs, W45 to W47 (2026-11-18)

Every Claude Code session on this machine from 2026-11-03 to 2026-11-18 about 14:00 local, read from `~/.claude/projects` and priced as `tools/tokencount` prices.
No code or config changed.
Codex usage is out of scope.

- **Tokencount counts 77.5% of the Claude usage; the rest is $1,121.66 of $4,975.37.**
  The counted share is the $3,853.71 of the 361 finished tapestry and darn runs.
  The uncounted kinds, largest first: motif and raddle runs $484.92, the hub orch $349.62, unfinished and bench runs $166.00,
  agents in the prime other than the orch $37.47, other hubs $50.75, beam orchs $24.17, sessions outside LoomYard $8.55.
- **The hub orch is already counted when tokencount runs from the prime, and silently dropped when it runs from a task worktree.**
  From the prime, `go run ./tools/tokencount -since 2026-11-03` reports orch $344.59 + unpriced on top of the runs' $3,823.94 + unpriced,
  and a tapestry run at $62.58, the figure the board entry quotes.
  From a task worktree the same call reports 0 orch sessions and $0.00 in every Orch row.
- **The rise from W46 to W47 is $237.5 to $474.7 per active day; counted runs give $168.6 of it and usage outside them $68.6.**
  The uncounted kinds cost $70.7 per active day in W45, $70.3 in W46 and $138.9 in W47, 16.3%, 29.6% and 29.3% of the week.
  In W47 they rose through unfinished runs, $3.5 to $42.8 per active day, motif runs, $44.5 to $54.9, and beam orchs, $0.3 to $6.9.
  The counted runs' rise is mostly tapestry: $109.69 in W46, $511.08 in the first three days of W47.
- **Motif runs are the largest uncounted kind and are Opus.**
  A motif worktree costs about $19, $416 of its $485 being Discussion, and Opus is $430.71 of the $484.92.

## Method

Each assistant message of every transcript under `~/.claude/projects` dated 2026-11-03 or later is priced with `claudeengine`'s price table,
the one `tools/tokencount` uses.
A message is counted once per message id, as tokencount counts it; of the entries repeating an id, the one with the most tokens stands.
Main transcripts leave out sidechain lines, and the transcripts under `<session>/subagents/` count as forks.
A message falls in the ISO week of its local date.
W45 starts 2026-11-03, the scope's start, and W47 ends with the messages written up to the report.

The sessions are classified by the project directory they ran in and the role in their name, the part after the last colon:

| kind | what it holds |
|---|---|
| counted runs | every session in the worktree of a run `tokencount -since 2026-11-03` counted, 361 finished tapestry and darn runs |
| hub orch | the `ly:orch` sessions and their forks in the prime, 4 sessions |
| motif and raddle runs | sessions in the 26 uncounted worktrees that hold a raddle session or no Plan, Webster, Darn or Code session, bar orchs |
| unfinished and bench runs | sessions in the other uncounted worktrees: runs in flight or stopped before Finalize, and the Plan bench pairs `cm-*`, `cmb-*` and `pwc-*` |
| beam orchs | `ly:<slug>:orch` sessions in uncounted worktrees: `adopt-build`, `adopt-build-resplit`, `codex-engine` |
| prime agents | the prime's sessions that are not the orch: `board-tidy`, `tackler`, `arbiter`, and the `cs-trial` sessions that trialled the cs hub |
| other hubs | every session of the `cs`, `lyt`, `odd` and `scribe` hubs, orch and runs |
| outside LoomYard | sessions in the home directory and `~/Code` outside any hub |
| test fixtures in /tmp | the `-tmp-` project directories of LoomYard's own smoke tests |

Dobby retro sessions are runs of the darn recipe and sit in counted runs.
No session titled as a board agent other than `board-tidy` ran in the period.

The sweep matches tokencount on the shared population.
The counted runs' worktrees cost $3,853.71 in the sweep against tokencount's $3,823.94 + unpriced;
the $29.77 gap is sessions of a reused slug path from before tokencount's archive-tag cutoff.
The hub orch costs $349.62 against tokencount's $344.59 + unpriced, a gap of messages outside tokencount's window.
Tokencount has no weekly view, so the weekly run spend is the counted runs' sessions priced by message date.

Three limits apply:

- **Unpriced Fable.** `claude-fable-5-2` has no row in the price table, so its 108 messages and 19.6M tokens are in no dollar figure.
  Tokencount lists it as unpriced as well, 129 messages in its own count.
  `claude-fable-5-1` is priced: 105 messages, $40.55, all in W45 runs.
- **List price is not the quota.** The figures rank kinds by API list price; the weekly quota's own weights are not known here.
- **Missing days.** The transcripts hold no message dated 2026-11-10, 2026-11-13 or 2026-11-14.
  A week's active days are W45 six, W46 four and W47 three, the last cut at the report's time.

## Weekly usage by session kind

Cost is the sum of priced messages; tokens are input, cache write, cache read and output together.
The model column gives each model's cost and message count, with `<synthetic>` and unpriced messages left out except for Fable.

### W45, 2026-11-03 to 2026-11-08

| session kind | cost | share | tokens | models: cost (messages) |
|---|---|---|---|---|
| counted runs | $2,177.57 | 83.7% | 5,429M | sonnet-5-5 $1,174.94 (27,588), opus-5-5 $884.34 (10,453), haiku-5-5 $77.74 (21,706), fable-5-1 $40.55 (105) |
| hub orch | $177.25 | 6.8% | 410M | opus-5-5 $177.10 (3,453), sonnet-5-5 $0.14 (7), haiku-5-5 $0.01 (2) |
| motif and raddle runs | $142.28 | 5.5% | 177M | opus-5-5 $125.51 (1,638), sonnet-5-5 $16.67 (551), haiku-5-5 $0.10 (11) |
| other hub cs | $29.82 | 1.1% | 55M | opus-5-5 $16.33 (299), sonnet-5-5 $13.45 (429), haiku-5-5 $0.04 (20) |
| prime agents | $29.39 | 1.1% | 79M | opus-5-5 $17.33 (348), sonnet-5-5 $12.01 (543), haiku-5-5 $0.05 (40) |
| unfinished and bench runs | $23.77 | 0.9% | 43M | opus-5-5 $18.66 (261), sonnet-5-5 $5.11 (129) |
| other hub lyt | $7.96 | 0.3% | 10M | sonnet-5-5 $4.59 (107), opus-5-5 $3.34 (29), haiku-5-5 $0.03 (14) |
| other hub scribe | $5.89 | 0.2% | 11M | sonnet-5-5 $3.76 (130), opus-5-5 $2.12 (45), haiku-5-5 $0.01 (4) |
| outside LoomYard | $5.47 | 0.2% | 5M | opus-5-5 $5.47 (52) |
| beam orchs | $2.18 | 0.1% | 4M | opus-5-5 $2.18 (55) |
| **total** | **$2,601.58** | | 6,222M | |

### W46, 2026-11-09 to 2026-11-15

| session kind | cost | share | tokens | models: cost (messages) |
|---|---|---|---|---|
| counted runs | $668.79 | 70.4% | 1,321M | sonnet-5-5 $342.79 (9,547), opus-5-5 $326.00 (3,742), fable-5-2 unpriced (37) |
| motif and raddle runs | $177.80 | 18.7% | 213M | opus-5-5 $158.62 (1,994), sonnet-5-5 $19.17 (689), haiku-5-5 $0.01 (8) |
| hub orch | $82.77 | 8.7% | 188M | opus-5-5 $56.63 (1,069), sonnet-5-5 $26.14 (635), fable-5-2 unpriced (7) |
| unfinished and bench runs | $13.93 | 1.5% | 29M | opus-5-5 $7.38 (86), sonnet-5-5 $6.55 (161), fable-5-2 unpriced (24) |
| prime agents | $3.75 | 0.4% | 7M | sonnet-5-5 $3.74 (225), haiku-5-5 $0.01 (14) |
| outside LoomYard | $1.53 | 0.2% | 3M | sonnet-5-5 $1.53 (34) |
| beam orchs | $1.25 | 0.1% | 1M | opus-5-5 $1.25 (17) |
| **total** | **$949.82** | | 1,762M | |

### W47, 2026-11-16 to 2026-11-18

| session kind | cost | share | tokens | models: cost (messages) |
|---|---|---|---|---|
| counted runs | $1,007.35 | 70.7% | 2,108M | opus-5-5 $543.06 (6,332), sonnet-5-5 $464.22 (10,556), haiku-5-5 $0.07 (27), fable-5-2 unpriced (25) |
| motif and raddle runs | $164.84 | 11.6% | 207M | opus-5-5 $146.58 (2,080), sonnet-5-5 $18.25 (675), haiku-5-5 $0.01 (10) |
| unfinished and bench runs | $128.30 | 9.0% | 242M | opus-5-5 $98.98 (1,066), sonnet-5-5 $29.32 (764) |
| hub orch | $89.60 | 6.3% | 169M | opus-5-5 $89.60 (1,561), fable-5-2 unpriced (15), haiku-5-5 $0.00 (2) |
| beam orchs | $20.74 | 1.5% | 28M | opus-5-5 $20.74 (331) |
| other hub odd | $7.08 | 0.5% | 13M | opus-5-5 $6.68 (129), sonnet-5-5 $0.40 (7) |
| prime agents | $4.33 | 0.3% | 10M | sonnet-5-5 $2.45 (154), opus-5-5 $1.69 (49), haiku-5-5 $0.19 (155) |
| outside LoomYard | $1.55 | 0.1% | 2M | sonnet-5 $1.23 (10), sonnet-5-5 $0.32 (3) |
| test fixtures in /tmp | $0.18 | 0.0% | 0M | sonnet-5-5 $0.18 (4) |
| **total** | **$1,423.97** | | 2,779M | |

### The three weeks together

| session kind | cost | share | tokens |
|---|---|---|---|
| counted runs | $3,853.71 | 77.5% | 8,859M |
| motif and raddle runs | $484.92 | 9.7% | 597M |
| hub orch | $349.62 | 7.0% | 767M |
| unfinished and bench runs | $166.00 | 3.3% | 314M |
| prime agents | $37.47 | 0.8% | 96M |
| other hub cs | $29.82 | 0.6% | 55M |
| beam orchs | $24.17 | 0.5% | 33M |
| outside LoomYard | $8.55 | 0.2% | 10M |
| other hub lyt | $7.96 | 0.2% | 10M |
| other hub odd | $7.08 | 0.1% | 13M |
| other hub scribe | $5.89 | 0.1% | 11M |
| test fixtures in /tmp | $0.18 | 0.0% | 0M |
| **total** | **$4,975.37** | | 10,765M |

## Beside tokencount's run spend

Tokencount's own totals for the same period, `go run ./tools/tokencount -since 2026-11-03` from the prime:

| figure | value |
|---|---|
| runs counted | 361: 340 darn at $8.29 per run and 21 tapestry at $62.58, orch charge included |
| run spend | $3,823.94 + unpriced |
| orch charged to runs and unattributed | $344.59 + unpriced, 8.3% of the $4,168.53 + unpriced total |

Per active day, in dollars:

| session kind | W45 (6 days) | W46 (4 days) | W47 (3 days) |
|---|---|---|---|
| counted runs | 362.9 | 167.2 | 335.8 |
| hub orch | 29.5 | 20.7 | 29.9 |
| motif and raddle runs | 23.7 | 44.5 | 54.9 |
| unfinished and bench runs | 4.0 | 3.5 | 42.8 |
| prime agents | 4.9 | 0.9 | 1.4 |
| beam orchs | 0.4 | 0.3 | 6.9 |
| other hubs, outside LoomYard and fixtures | 8.2 | 0.4 | 2.9 |
| **all** | **433.6** | **237.5** | **474.7** |

The counted runs by recipe, by message date:

| week | darn | tapestry |
|---|---|---|
| W45 | $1,558.81, 193 slugs active | $618.77, 9 slugs active |
| W46 | $559.10, 87 slugs active | $109.69, 3 slugs active |
| W47 | $496.26, 61 slugs active | $511.08, 9 slugs active |

## What grew, and why

- **Tapestry runs, W46 to W47.**
  Nine tapestry slugs were active in W47's three days against three in W46's four.
  Darn spend fell from $559 to $496 and tapestry rose from $110 to $511.
  The board entry's reading holds: the rise in run spend is more tapestry runs.
- **Unfinished runs, $3.5 to $42.8 per active day.**
  W47's $128 is the six `cm-*` Plan bench pairs of 2026-11-16, $57 with only Plan sessions,
  and two tapestry runs in flight on 2026-11-18, `claude-accounts-per-worktree` and `review-result-checks`, $69 together over all weeks.
  Tokencount counts a run once it holds an archive tag and a Finalize entry,
  so the in-flight spend moves into counted runs as the runs finish.
  W45's $24 is mostly `pawl-checklist-hidden-in-beam`, a run of 2026-11-03 that holds no Finalize.
- **Motif runs, $23.7 to $54.9 per active day.**
  The weekly total is flat, $142, $178 and $165, but the motif runs began 6, 9 and 8 raddle worktrees, so a day of W47 holds more of them.
  A motif worktree costs about $19.
  Of the roughly $420 of Discussion in the uncounted worktrees, the Burler review takes $178, the fix $123 and the writer $110;
  the Burler seats and the writer run on Opus, which is $430.71 of the motif kind's $484.92.
  Raddle itself is $46 and the arbiters $14.
- **Beam orchs, $0.3 to $6.9 per active day.**
  The kind first appears on 2026-11-15 with `adopt-build`;
  `adopt-build-resplit` followed on 2026-11-16 and 2026-11-17 for $12.95 of the kind's $24.17.
  All of it is Opus.
- **The hub orch is flat per active day**, $29.5, $20.7 and $29.9.
  It is four long sessions of 550 to 3,400 messages each, every message re-reading about 100k to 130k tokens of cached context.
  On the Opus main sessions cache reads are 650M of 678M tokens and cost $130 of $304, cache writes $125 and output $49.
  The session of 2026-11-11 and 2026-11-12 ran on Sonnet at $0.042 to $0.044 per message against $0.041 to $0.076 on Opus;
  W46 holds it, and W46's orch is the cheapest week.
- **Prime agents shrank**, $4.9 to $0.9 and $1.4 per active day.
  W45 holds the two `cs-trial` sessions of 2026-11-03 ($19.04); `board-tidy` ran on every active day for $16.49 in all.
- **Other hubs and outside LoomYard** hold single days: cs, lyt and scribe in W45 and odd in W47.
  The hubs' orch sessions are $18.92 of their $50.75, and the outside sessions are a home-directory session ($7.32) and a keyboard-setup session ($1.23).

## Where tokencount could count a kind with a small change

No code changed here; each item names the line to change.

| kind | spend | what stops tokencount, and the small change |
|---|---|---|
| hub orch | $349.62 | `tools/tokencount/main.go` passes the current directory to `CountOrch`, so a run from a task worktree reads no orch sessions.<br>Passing the prime, resolved through `lymcwd`, counts it from any worktree. |
| prime agents | $37.47 | `tools/tokencount/orch.go` keeps a session only when its role is `orch`.<br>Widening the test to every session of the prime, charged by time as the orch's are, counts `board-tidy`, `tackler`, `arbiter` and the trial sessions. |
| motif and raddle runs | $484.92 | `countedRecipes` in `tools/tokencount/shed.go` names tapestry and darn, and `finishedRun` requires a Finalize entry that neither motif nor raddle has.<br>Adding both recipes and testing a recipe's last producer instead counts them; their roles already fall under the Discussion segment and Other parts. |
| beam orchs | $24.17 | A beam orch has no finished run to select; naming its slug on the command line counts it today, as an Other part.<br>A default selection needs a rule for beam slugs. |
| unfinished runs | $166.00 | A slug named on the command line counts, finished or not.<br>The default stops at `finishedRun`; a flag that keeps runs in flight would count them, their cutoff being the newest archive tag. |
| other hubs | $50.75 | `-hub` and `-weft` reach another hub's run worktrees, but `CountOrch` reads the current directory's project only. |
| outside LoomYard | $8.55 | No hub holds them; out of tokencount's reach. |
