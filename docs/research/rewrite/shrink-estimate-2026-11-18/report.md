# How much smaller would a clean rewrite be? — shrink estimate, 2026-11-18

An estimate, made before any rewrite starts, of the code and test lines a clean-room rewrite of the seven most patched modules would save.
Step 1 is measured for all seven modules.
Steps 2 and 3 are done for `landingshed` alone and are a judgement from its spec, not a measurement.
Step 4 is an extrapolation to the other six modules and carries no measurement of those modules beyond step 1.

Files in this folder:

- `report.md` — this report.
- `landingshed-spec.md` — what `landingshed` must do and why, reusable as step 1 of a pilot.
- `measure.sh` and `union.sh` — the scripts behind the step 1 table.

## Result in one paragraph

Duplicate code is not where the size is: at a 50-token threshold `dupl` finds 0.3 to 1.1 per cent of production lines and 1.1 to 3.6 per cent of test lines duplicated in every module.
The size is in comments (20 to 31 per cent of production lines) and in tests (1.3 to 2.7 times the production code lines).
For `landingshed` a clean version needs about 4,400 code lines against 5,418 today (a range of 4,000 to 4,750), about 950 comment lines against 2,568, and about 4,000 to 6,000 test lines against 12,585.
Extrapolated, the seven modules shrink from about 319,000 lines to about 142,000 to 197,000, which is 38 to 55 per cent.
Most of that comes from tests and comments; the production code itself shrinks 12 to 26 per cent.

## Step 1 — measured, all seven modules

Tools: `go run ./tools/codestats -root internal/<module> -json` for lines, and `github.com/mibk/dupl@v1.0.0` built into the scratch directory with `go install` (nothing installed globally), run with `-t 50 -plumbing -files`, once over the module's non-test `.go` files and once over its `_test.go` files.
A module's directory includes its sub-packages (for example `shuttleengine/claudeengine`).
"Production" in the codestats columns is every non-test file of a language with a comment syntax, so a small embedded `yaml` template counts too.
Duplicated lines are the union of line ranges that `dupl` reports as a clone, so a range in several clones counts once.
A duplicated-line percentage is taken against the production or test lines of the module.

| Module | Prod code | Prod comment | Prod blank | Prod total | Comment share | Test code | Test comment | Test blank | Test total | Test code / prod code |
|---|---|---|---|---|---|---|---|---|---|---|
| `loomcli` | 14,293 | 4,429 | 1,292 | 20,014 | 22.1% | 27,771 | 2,955 | 2,273 | 32,999 | 1.94 |
| `fabricengine` | 22,833 | 10,574 | 2,839 | 36,246 | 29.1% | 43,006 | 6,251 | 6,022 | 55,279 | 1.88 |
| `landingshed` | 5,418 | 2,568 | 564 | 8,550 | 30.0% | 11,003 | 702 | 880 | 12,585 | 2.03 |
| `shuttleengine` | 17,129 | 8,619 | 1,955 | 27,703 | 31.1% | 35,048 | 3,747 | 2,596 | 41,391 | 2.04 |
| `battencli` | 9,204 | 2,463 | 728 | 12,395 | 19.8% | 15,053 | 1,268 | 1,281 | 17,602 | 1.63 |
| `websterengine` | 7,834 | 3,828 | 950 | 12,612 | 30.3% | 20,785 | 1,886 | 1,610 | 24,281 | 2.65 |
| `driftengine` | 6,023 | 2,097 | 605 | 8,725 | 24.0% | 7,780 | 330 | 621 | 8,731 | 1.29 |
| Seven modules | 82,734 | 34,578 | 8,933 | 126,245 | 27.4% | 160,446 | 17,139 | 15,283 | 192,868 | 1.94 |

Duplicate code (`dupl -t 50`):

| Module | Prod duplicated lines | % of prod | Largest prod clone (lines) | Test duplicated lines | % of test | Largest test clone (lines) |
|---|---|---|---|---|---|---|
| `loomcli` | 122 | 0.6 | 45, within `decision.go` | 583 | 1.7 | 20, within `wiring_test.go` |
| `fabricengine` | 132 | 0.4 | 22, within `mergelifecycle.go` | 679 | 1.2 | 18, within `destroy_test.go` |
| `landingshed` | 36 | 0.4 | 12, `parentalign.go` and `finalize.go` | 147 | 1.2 | 14, within `beamdescribe_test.go` |
| `shuttleengine` | 262 | 0.9 | 58, `codexengine/events.go` and `claudeengine/signals.go` | 531 | 1.3 | 15, within `wait_test.go` |
| `battencli` | 119 | 1.0 | 15, three places in `wire.go` | 191 | 1.1 | 15, within `cancelcmd_test.go` |
| `websterengine` | 141 | 1.1 | 23, `outcome.go` and `archive.go` | 475 | 2.0 | 16, in three test files |
| `driftengine` | 38 | 0.4 | 14, `seal.go` and `diff.go` | 313 | 3.6 | 18, within `judgesnapshot_test.go` |

Total duplicated: 850 production lines (0.7%) and 2,919 test lines (1.5%).
The one cross-package clone worth a rewrite note is `shuttleengine`'s: the Claude and Codex engines carry a 58-line and a 39-line pair of near-identical blocks (`events.go` against `signals.go`, and the two `manifest.go` files).
Every other clone is 23 lines or fewer.
A tool run at a lower threshold would find more, but a clone this small is not what makes a module large.

## Step 2 — the `landingshed` spec

`landingshed-spec.md` states the job, eleven invariants and a rule table per row, each rule with the commit whose incident or review created it.
The package doc was used as a statement of behaviour; its wording of how the code does it was not.
A reason without a commit in the spec is an inference from the rule, not from history.

## Step 3 — clean `landingshed`, from the spec

### What a reader of the spec can already see

The spec has about 55 rule rows and 11 invariants.
They fall into the same units the code has: Finalize, PR-Review, the fixer, shared gate and lock plumbing, Parent-Align, Describe, config.
No unit disappears, so the clean design is the same architecture with less around it.
One read of the production code found where the surplus sits (every file read once; counts below are that read's, and its judgement, not a tool's).

### Parts of a clean version

| Part | Today (code lines) | Clean estimate | What carries the reduction |
|---|---|---|---|
| Finalize (lock pass, GitHub landing, owner checks, landing texts) | 1,616 | 1,300 | one result type instead of a four-value return at 60 call sites; one read of "parent holds the work" per pass; one retry loop; PR helpers shared with PR-Review |
| PR-Review (decide, open, wait, gate head, send-back, session, resolver) | 1,146 | 1,000 | shared GitHub failure and open-or-find helpers; one wiring check |
| Fixer (turn loop, reports, caps, attempts, review, acceptance) | 881 | 700 | one report parser; one seat-table builder |
| Gate and lock plumbing | 643 | 520 | one failure-record path; one capped-list helper; fewer test-only function fields |
| Parent-Align | 411 | 330 | shared resolver construction and fixer judgement |
| Describe and beam describe | 281 | 230 | one table builder |
| Config and `Deps` | 292 | 240 | migration of retired keys removed |
| Merge record and hand-back | 127 | 110 | retired `Merge-In` row name removed |
| Total code | 5,418 | about 4,430 | range 4,000 to 4,750 |

The low end of the range assumes the lock-release-during-fixer layer (about 90 lines of requeue, rebase and reset bookkeeping) is designed away by running the fixer outside the locked pass, and that a change to the result type saves about 150 lines.
The high end keeps both.
The estimate keeps every feature in the spec, the flaky rerun, the adopter-first repair note and the beam landing record included.
Those are about 340 lines a product owner could remove; they are not in the estimate.

Comments: today 2,568 lines, of which `doc.go` is 678 and restates the function docs.
A clean version needs a package overview of about 60 lines, one contract comment per exported symbol and a "why" comment where a spec rule is non-obvious, which is about 900 to 1,400 lines.
Blank lines follow code at about 10 per cent.

| `landingshed` production | Today | Clean | Change |
|---|---|---|---|
| Code | 5,418 | 4,000 to 4,750 | 12% to 26% fewer |
| Comment | 2,568 | 900 to 1,400 | 45% to 65% fewer |
| Blank | 564 | 400 to 500 | |
| Total | 8,550 | 5,300 to 6,650 | 22% to 38% fewer |

### What today's code has that a clean version drops

Sizes are the production-code reader's estimates.

| Category | Lines | Confidence |
|---|---|---|
| Duplicated logic: stop-result wrappers, report parsers, table builders, PR helpers repeated across Finalize and PR-Review, repeated error wraps | about 340 | medium to high |
| Single-caller or test-only seams and adapters, a `parentPuller` interface never used in production, a 52-line wiring check | about 100 to 190 | low to medium |
| Workaround layers: the retired `Merge-In` name, a scratch-to-records attempt migration, key-rename migration, token checks that compensate for a wrapped HTTP error | about 100 | medium |
| Lock-freed, requeue and rebase layer around the fixer | about 90 | medium to low |
| Over-long comments: `doc.go` (about 600 of 678 lines), seven "Bound:" disclaimers, one sentence repeated about eight times in `deps.go`, step-number narration inside `Finalize.Call` | about 1,400 to 1,800 | medium |

`dupl` found 36 duplicated production lines; the reader found about 340 of duplicated logic.
The two agree in direction and not in size, because `dupl` sees repeated token runs and the reader saw near-identical helpers with different names and types.

### Tests

Today: 12,585 lines, 89 test functions, 197 subtests, 35 files that require a tier.
Largest files: two of about 1,200 lines, one of 944.
A sampled file (`finalize_test.go`) opens with a 58-line fake whose fields script each call's return, and each table row repeats long literals; a row is often one line of 150 characters or more.

A suite written by behaviour, one test per spec rule and the rule's outcomes as rows, needs:

- about 55 rule rows at an average of 60 lines (a rule with several outcomes is a table of 5 to 10 rows), about 3,300 lines;
- shared fakes and fixtures for the seams, about 600 lines;
- invariant tests, about 100 lines.

That gives about 4,000 lines.
The high end, 6,000, covers rules that turn out to have more outcomes than the spec lists.
Against 12,585 lines today that is 52% to 68% fewer.
This is the least certain figure in the report: it counts behaviours, not tests, and no pilot has yet shown how many rows a rule needs.

## Step 4 — extrapolation to the other six modules

This is an extrapolation.
It uses step 1's numbers for each module and `landingshed`'s ratios, and assumes the other six behave like `landingshed`; none has been read for what a clean version would drop.

Rules applied to each module's production code lines `c`:

- clean code = 0.74 to 0.88 of `c`, the `landingshed` range (4,000 to 4,750 of 5,418);
- clean production total = clean code × (1 + comment share 0.22 to 0.30 + blank 0.10), the `landingshed` range;
- clean test total = 1.0 to 1.3 times clean code lines, which `landingshed`'s 4,000 to 6,000 gives;
- neither total is allowed above today's value.

Step 1 says where the rules are least safe.
`loomcli` and `battencli` carry 22% and 20% comments, so less comment saving than `landingshed` is likely, and the range's high end is the better guess for them.
`driftengine` has a test ratio of 1.29 and `battencli` of 1.63, against `landingshed`'s 2.03, so their test saving is smaller.
The cap on today's value triggers for no module.

| Module | Prod today | Prod clean | Test today | Test clean |
|---|---|---|---|---|
| `loomcli` | 20,014 | 13,964 to 17,608 | 32,999 | 10,576 to 16,351 |
| `fabricengine` | 36,246 | 22,307 to 28,130 | 55,279 | 16,896 to 26,120 |
| `landingshed` (estimated from the spec) | 8,550 | 5,293 to 6,674 | 12,585 | 4,009 to 6,198 |
| `shuttleengine` | 27,703 | 16,735 to 21,102 | 41,391 | 12,675 to 19,595 |
| `battencli` | 12,395 | 8,992 to 11,339 | 17,602 | 6,810 to 10,529 |
| `websterengine` | 12,612 | 7,653 to 9,651 | 24,281 | 5,797 to 8,962 |
| `driftengine` | 8,725 | 5,884 to 7,420 | 8,731 | 4,457 to 6,890 |
| Six modules (extrapolated) | 117,695 | 75,535 to 95,250 | 180,283 | 57,211 to 88,447 |
| Seven modules | 126,245 | 80,828 to 101,924 | 192,868 | 61,220 to 94,645 |

All seven together: 319,113 lines today, 142,048 to 196,569 after, so 38% to 55% fewer.
Of the lines removed, tests account for 74% to 80%; production code shrinks 12% to 26% and production comments 45% to 65%.

## Limits

- The clean sizes are estimates from one reader's judgement; nothing was rewritten.
- The `landingshed` test figure counts behaviours; a pilot (note `clean-room-module-rewrite`) is what would turn it into a measurement.
- The extrapolation transfers one module's ratios to six others.
  Ranges from different modules do not add as independent errors: if `landingshed` is unrepresentative, all six move together.
- `dupl` at 50 tokens misses near-duplicates that differ in names or types, which the production reader found; a lower threshold or a structural detector would report more but was outside this study.
- The spec's per-rule commit citations come from commit subjects and bodies, not from reading each diff.
