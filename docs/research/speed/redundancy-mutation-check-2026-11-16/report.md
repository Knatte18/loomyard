# Confirming redundancy candidates by mutation: tool choice and cost, 2026-11-16

Board task `redundancy-judge-asserts` fixes `lym gate redundant`, which listed a test as redundant when other tests ran all its blocks.
The verb now mutates the candidates' blocks and keeps a candidate only when its covering tests kill every mutant it kills.
This report settles how the mutants are made and measures the check on the five packages whose candidates
`redundancy-candidates-survey` read (`docs/research/speed/redundancy-candidates-survey-2026-11-16/report.md`).
One machine with 12 CPUs, one run per package, with the hub's other runs live beside it.

## Answer in brief

- lym writes its own mutator over `go/ast`, in `internal/testcover`, and builds every mutant of a package into one test binary.
- Every candidate the survey sampled is dropped: three are refuted by a mutant their covering tests miss,
  two are source scans no mutant can judge, and one holds no mutant in its blocks.
- Of the five packages' 120 candidates, 37 stay listed; 65 are refuted, and 18 are not judged.
- The check took 13.3 minutes of wall over the five packages, from 3 s for `internal/gitkit` to 8.6 minutes for `internal/loomcli`.
  It builds three binaries per package; its cost is the test runs, a few hundred to two thousand per package.

## Own mutator or an existing tool

The check needs a kill matrix: for each mutant, which of the candidate and its covering tests fail with it on.
The Go mutation tools in use, such as go-mutesting and gremlins, build each mutant and run the package's tests against it,
reporting whether any test killed it.
Using one here would still need a run per mutant and test to tell which test killed what,
plus a build per mutant and a dependency the module does not carry.

The own mutator works as a mutant schema:

- it places the blocks of a coverage profile in the package's files and takes the sites of the candidates' blocks;
- it changes each site by one operator: it negates an if or for condition or a comparison,
  drops a call, assignment or increment statement, or changes an integer, rune or string constant;
- it wraps every site in a helper that changes its value only while `LYM_TESTCOVER_MUTANT` names the mutant,
  and hands the changed files to `go test -c -overlay`, so one build holds every mutant and the worktree stays untouched;
- it inserts no line, so a compiler error names the mutant's own line.
  A constant a typed context needs, such as `5 * time.Second`, does not compile behind the helper;
  the build drops the mutants on the lines the compiler names and builds again.

The dropped mutants were 23 of 1,151 over the five packages.

A candidate's covering tests are the set the coverage verdict picks for it from every other judged test,
largest overlap first, leaving out tests whose check reads source files.
The pick does not depend on which tests the coverage pass's greedy removal keeps, so it does not vary with the tests' seconds.
A pool of every test that shares a block is wider than the board entry's "its covering tests":
measured that way, `TestEnsure_PairDirectoryDeletedIsSkippedAsRemoved` and `TestStart_NeverAttaches` came back confirmed,
since some test outside their covering set kills each of their mutants.

## Cost

Measured with `/usr/bin/time` around `lym gate redundant <package>`, after `lym gate testmap` built each map.
Build seconds run from the slot's hold to a built mutant binary: the coverage build, the block table run and the mutant builds.
Raw output: [data/](data), one `redundant-<package>.json` per package and [time.txt](data/time.txt).

| Package | Candidates | Mutants | Dropped | Runs | Build s | Run s | Wall s | CPU s |
|---|---|---|---|---|---|---|---|---|
| `internal/hubreconcile` | 10 | 143 | 7 | 1,209 | 1.3 | 240.9 | 242.3 | 703.2 |
| `internal/loomrecipe` | 51 | 87 | 0 | 1,760 | 1.7 | 21.1 | 22.8 | 109.0 |
| `internal/orchcli` | 9 | 152 | 3 | 724 | 2.0 | 9.1 | 11.1 | 39.6 |
| `internal/gitkit` | 4 | 25 | 0 | 71 | 0.5 | 2.7 | 3.2 | 5.2 |
| `internal/loomcli` | 46 | 721 | 13 | 2,075 | 8.9 | 508.8 | 517.9 | 293.4 |
| All five | 120 | 1,128 | 23 | 5,839 | 14.4 | 782.6 | 797.3 | 1,150.3 |

The cost follows the runs and what each run costs, not the package's size:

- `internal/hubreconcile` holds tier-1 tests that build a hub each, so its 1,209 runs cost 0.58 CPU-seconds apiece.
- `internal/loomcli` spent 518 s of wall on 293 CPU-seconds, so its runs mostly waited.
  The likeliest cause is mutants that hang a test until its bound, ten times its unmutated seconds and at least 10 s;
  this run did not record which runs timed out.
- `internal/loomrecipe` ran 1,760 tier-0 runs in 21 s.

The check runs on command, per package, inside one gate slot, as the map build does.
At these costs a package costs seconds to minutes, so a sweep runs it per package before deleting tests, not on every gate.

## The yardstick

| Package | Sampled candidate | Verdict |
|---|---|---|
| `internal/hubreconcile` | `TestEnsure_PairDirectoryDeletedIsSkippedAsRemoved` | refuted by 4 mutants |
| `internal/loomrecipe` | `TestZeroBudget_BouncerSettlesAtOnce` | refuted by 3 mutants |
| `internal/orchcli` | `TestStart_NeverAttaches` | refuted by 5 mutants |
| `internal/orchcli` | `TestUnderAdoptWatch_CoversThePrimeWhileARootAdoptIsUnfinished` | not judged: its blocks hold no mutant |
| `internal/gitkit` | `TestCopyRepoCallerSet_LymcwdOnly` | not judged: it checks source files |
| `internal/loomcli` | `TestDriverChoiceSingleSiteInvariant_OnlyLoomcliReadsTheSeedDriverField` | not judged: it checks source files |

The two scans first came back confirmed.
Their blocks are the shared setup their covering tests run too, and every mutant there breaks both;
their own check reads repository files, which no mutant of the package's code changes.
The verb therefore never judges a test whose check reads source files:
one marked `//lym:guard`, or one that reaches `internal/testkit/scankit` directly or through a same-package call.

## What the verb now lists

| Package | Candidates | Confirmed | Removable | Refuted | Source checks | Kills no mutant | No mutant in its blocks |
|---|---|---|---|---|---|---|---|
| `internal/hubreconcile` | 10 | 2 | 2 | 8 | 0 | 0 | 0 |
| `internal/loomrecipe` | 51 | 16 | 16 | 35 | 0 | 0 | 0 |
| `internal/orchcli` | 9 | 4 | 4 | 2 | 0 | 2 | 1 |
| `internal/gitkit` | 4 | 0 | 0 | 2 | 2 | 0 | 0 |
| `internal/loomcli` | 46 | 15 | 12 | 18 | 1 | 4 | 8 |
| All five | 120 | 37 | 34 | 65 | 3 | 6 | 9 |

A confirmed candidate is evidence, not a verdict: each one killed only mutants a covering test killed,
but the operators cannot reach every check, such as one on another package's result.
Most confirmed candidates are in `internal/loomrecipe`, whose tests build the same recipes and assert the same rows;
the survey already read them as rows to fold into a table test.

## Limits

- One run per package on a shared machine; wall and CPU carry the load of the hub's other runs.
- The maps were built at `194650f0`, and `internal/loomcli`'s at `02a4f92d`; no package's own files changed since.
  The verb was measured with the covering pick uncommitted beside them, which no test binary of theirs links.
- A candidate's kill beside other runs is not rerun alone, so a run another one disturbs can refute it falsely.
  A covering test's kill is rerun alone, and that run stands.
- The yardstick is the six candidates the survey named; the survey read about a dozen test bodies.
