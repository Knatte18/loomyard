# Keeping the coverage maps fresh: three independent proposals, compared

Board task `testmap-upkeep` asked whether there is a smarter way to keep the per-test coverage maps fresh,
and asked for three brainstorms by agents that never see each other's answer.
This report holds the comparison; the three proposals stand beside it, unedited:

| Proposal | Agent | Title |
|---|---|---|
| [A](proposal-fable-a.md) | Fable 5.2, high effort | Coverage footprints as a content-keyed cache, filled per test in idle time |
| [B](proposal-fable-b.md) | Fable 5.2, high effort | Carry on landing, measure when idle: coverage maps that go stale one test at a time |
| [O](proposal-opus.md) | Opus 5.5, high effort | Roll the maps forward: the gate's own selector decides staleness, and a polite lane measures only what changed |

The report is the input for a later design task.
It decides no design and builds nothing.

## How the proposals were made

Each agent started fresh on 2026-11-11 at commit `4ae8978ec` with the same brief, which differed only in the file to write.
The brief named the board note, `internal/testcover`, `internal/gatecli/testmap.go` with `testmappackage.go`, and the prime dobby's refresh step in `internal/dobbyengine/testmap.go` as required reading.
It let the agent read the rest of the repository and the board entries around the maps: `T4`, `T14` and `T15` (gate scoping by coverage map), `E12` (`testmap-backlog-drain`) and `E4` (`redundancy-sweep-existing`).
It fixed the section headings, so the proposals compare section by section.
No agent ran a test, a build or a further agent, so every duration in a proposal is an estimate or a figure read from an earlier record.

No Codex agent took part: Codex does not run on OSL-1046.

## Terms the proposals share

All three lean on the decisions of the Sketch `gate-scope-by-coverage-map`, which the board holds for `T4`, `T14` and `T15`:

- **D1**: a `coverage` gate scope narrows `diff`; the `finalize` landing gate keeps scope `all` and runs every test before a landing.
- **D2**: a map records each test's blocks across every in-module package its binary links, with the file hashes of each linked package.
- **D3**: a package the diff touches runs whole; a package that only imports a changed package runs the tests its map selects, every test its map does not record, and every test marked out-of-process or unresolved.
- **D4**: a test is selected when it covered a block of a function whose declaration differs between the map's own `Commit` and the working tree.
- **D5**: a *miss* runs the importing package whole: a missing or unreadable map, a change to the package's own files, and a *non-body* change to a linked file, such as a changed const, var, type, import, `//go:` line, method set or non-Go file.
- **D8**: the gate only reads maps and never starts a build.

The Sketch left the staleness rule to this task:
its D2 keeps testmap's present rule, and one acceptance criterion says a map stays fresh when only a linked package's file changed at HEAD.
Each proposal replaces that rule, so each changes that criterion.

## Where the three agree

The agents worked apart, so an agreement here is a finding rather than an echo.

1. **A process per test and a coverage build per package stay.**
   A and O both weigh a shared process and reject it as unsound, not only imprecise:
   code that runs once per process, such as a `sync.Once` body, would be recorded for the first test alone,
   and a later change there would not select the other tests that need it.
2. **The test, not the package, becomes the unit of staleness.**
   Today one changed file reruns every test of its package.
   In all three a test is measured again only when its own text changed or a function it reached changed.
3. **The grain is the function, not the block.**
   All three reject the note's block-level rule (starting idea 1) and take D4's function-level one.
   B gives the sharpest reason: a block inserted into a function can call code the test never reached,
   while the blocks the test covered keep their keys, so a block-level rule would keep an entry that is now wrong.
4. **One classifier serves the gate and upkeep.**
   Upkeep calls `T14`'s selector with the prime's HEAD as the target where the gate calls it with the working tree.
   A wrongly kept map entry and a wrongly skipped test are then the same error, caught by the same backstop.
5. **A lagging map only widens a gate.**
   D3 runs a test the map does not record, D4 compares from the map's own `Commit`, and `finalize` runs everything.
   So upkeep may lag, be interrupted and lose work without a wrong answer.
6. **The age rebuild goes.**
   It stood in for three things, and each gets its own check:
   dependency drift is compared directly once D2's linked hashes exist,
   toolchain and module drift by a recorded environment stamp,
   and what no rule models by an audit that measures a few kept entries again and counts the mismatches.
   B and O add a check that costs nothing: whenever a package is built, every kept entry's block keys must appear in the fresh block table.
7. **The trigger is "work remains", not a landing count.**
   The dobby starts a refresh when the prime's HEAD moved or a backlog is recorded.
   The landing count goes, with `test_map_every` and `test_map_batch` as A and B name them, and note `E12` is answered by construction.
8. **Upkeep never makes a gate wait.**
   It takes a slot without blocking and gives it back; the proposals differ on how (see below).
9. **Tests run several at a time inside the slot**, each in its own process with its own profile path.
10. **The maps stay machine-local for now.**
    Sharing comes last, is safe because a map is validated by content, and never goes through tracked files in the Bolt checkout.
11. **The redundancy sweep (`E4`) refreshes its own packages** with a hand call when it runs, so it sets no upkeep cadence.
12. **None of this makes a non-body change cheap.**
    The Sketch counts a non-body change in at least 95 of 118 code commits of one week.
    After D2 each one empties the map of every package that links the changed file, in all three designs.
    The proposals answer with placement: the refill runs in idle time, in parallel and most valuable first.
    The lever that would avoid the work is a narrower D5, which belongs to the selector, not to upkeep.

A and O also show by arithmetic that today's defaults never converge:
the cap admits about one package per landing, while landings and the age rule together make more maps stale than that.

## Side by side

| | A | B | O |
|---|---|---|---|
| Core idea | An entry is a cache record that matches a tree or does not; nothing is stale in place | Carry moves a map to HEAD with no build; measure fills what is absent | The selector's verdict at HEAD is the staleness rule: re-stamp, roll forward or rebuild |
| Record shape | Each test entry carries its own commit and time; up to two entries per test | One `Commit` per map; an entry is present or absent; failed and skipped tests get an outcome entry | One `Commit` per map; the map is rewritten whole per package job |
| How a map reaches HEAD | It does not have to: the consumer matches each entry by content when it reads | Carry rewrites it on each call, dropping entries, holding no slot | A plan step re-stamps maps with no affected test, holding no slot |
| A changed function of the package's own non-test files | Entries that covered it stop matching | Entries that reached it are dropped | Tests that covered it are run again |
| A changed helper in a test file | Every entry of the package | Every entry of the package | Miss: whole rebuild |
| A non-body change, or a changed non-Go file | Every entry of the package | Every entry of the map | Miss: whole rebuild |
| Toolchain, `go.mod`, tier env | Environment mark; every entry | Build stamp; every entry | Fingerprint; miss |
| Age rule | None | 30 days until `T4` lands, then none | 30 days as a low-ranked backstop until the audit shows a low rate |
| Audit | Oldest entries first, in leftover idle time; a drifting test is marked unstable and always runs | A few oldest entries per pass; a mismatch is logged and replaced | A random sample in idle time; a mismatch rebuilds the package whole |
| Unit of measurement | One test | One test, written per batch | A package job; pauses at a test boundary |
| Tests measured | Every mapped test | Every mapped test | Not those the spawn scan marks out-of-process or unresolved |
| Tests run at once | All, up to the slot's parallel, at low priority | All, up to the slot's parallel; a failure is retried alone | Only those that open with `t.Parallel()`; the rest serially |
| Where it measures | The prime worktree | The prime worktree | A checkout of its own, pinned to a main commit |
| When it may take a slot | Pool quiet, no drain, without waiting | Another slot stays free, without waiting | Another slot stays free |
| How it gives the slot back | A waiting gate leaves a request file; upkeep kills its tests and releases | It sees every slot held, or a drain; kills its tests and releases | Between two tests, to a waiting multi-slot tier; kills nothing |
| Change to `gateslot` | A yielding holder mark and a request file | The non-blocking take exported | None named; it reads `Holders` and `Draining` |
| Order of work | Named, then what a gate missed, then cheapest first, then the audit | By gate seconds per package from the run records, largest first | By gate time saved per unit of cost; packages too cheap to gain get no job |
| A failed build | Recorded once per HEAD, as today | Not addressed | The old map is kept and the failure recorded beside it |
| Sharing | Export and import through a history-free ref of the records repository | `--export` and `--import` as a file copy | The owner machine publishes to a dedicated ref on origin; fetched at `lym adopt` |
| First build step | Yielding slot and idle trigger, package unit unchanged | Parallel tests and a write per batch | Slot per package, drain yield and the `E12` trigger |

## Where they differ

### Where the staleness verdict lives

A stores no verdict at all.
An entry carries its own commit, and every reader, gate and upkeep alike, checks it against the tree it cares about.
Upkeep then has nothing to rewrite after a landing, and two entries per test serve a task worktree that lags the prime as well as one that does not.
The price is the largest change to the record and to `T14`: the selector must take entries grouped by commit.

B and O keep one `Commit` per map and move it.
B's carry drops entries and leaves gaps, which D3 already fills by running unrecorded tests, so a map is usable at every moment.
O keeps a map complete: a job writes it once at its end, and until then the old map stands at its old commit.
B's maps are fresher in pieces; O's are never partial, and a reader never has to reason about a gap.

### How polite the slot use is

A adds a protocol: a waiting gate signals, upkeep kills its tests in flight and leaves within a few polls.
It works with any slot count, at the price of new state in `gateslot`.

B and O add no protocol and take a slot only while another stays free.
B then leaves as soon as every slot is held, killing its tests.
O leaves only for a waiting multi-slot tier, between two tests, and kills nothing.

The hub's slot count decides how much this differs.
A and B reason from the template's `slots: 2`, where "another slot stays free" means no gate runs at all.
This hub's `gate.yaml` holds `slots: 3`, which O used: upkeep may then run beside one gate.

### Idle only, or whenever a slot is free

A and B start upkeep only after the pool has been quiet for a while.
O rejects idle-only: at this hub's landing rate a quiet pool may not come during working hours, and the queue would starve.
O runs whenever a slot stays free and keeps true idle time for the audit and the low-ranked rebuilds.
A names the same starvation as a risk and holds a non-yielding floor in reserve.
How much of the day the pool is idle is one of the measurements below; it settles this.

### The prime worktree or a pinned checkout

Today a dirty Go file skips a package, and a landing under a package's tests discards its runs.
A and B keep measuring in the prime and shrink the loss: B writes per batch, A per test.
O removes both cases with a checkout of its own at a fixed main commit, at the price of a second checkout and a new fabric verb.
A names the same checkout as an option under its risks.

### Which tests to measure, and which to run side by side

Only O skips the tests no consumer can use: D3 runs an out-of-process or unresolved test in every scoped gate whatever its map says.
Only O limits parallel runs to tests that declare `t.Parallel()`.
A and B run every test beside the others; B retries a failure alone, A relies on the repository's rule that tests tolerate parallel neighbours.

### What comes first

A orders by demand and cheapness, B by gate seconds, O by saving per cost.
A's demand signal has the gate append its misses to a record, which sits close to D8's "the gate only reads maps"; B keeps D8 untouched by reading the run records the gate already writes.
O alone says some packages should get no upkeep:
a package whose whole tiers cost less than a selected invocation's launch gains nothing from a map,
and by one tier 1 run record it read, 143 of 186 packages take under 3 seconds.

### Ideas only one proposal holds

- **A**: more than one entry per test; a demand record written by the gate; a simple function-hash comparison that needs nothing from `T14` and is stricter than D4, as a first form.
- **B**: outcome entries, so a failing test is not tried again on every pass; keeping what the redundancy step already measures; `finalize` as a producer of maps, left out until a pass is timed.
- **O**: the pinned checkout; skipping unusable tests; keeping the old map on a failed build; storing block tables once per file content hash; a sketch of how to narrow D5 by following the names a changed declaration is referenced by.

## The six starting ideas

| Idea | A | B | O |
|---|---|---|---|
| 1. Update a map in part | Adopt, with a wider rule at function grain | Adopt, with a corrected rule at function grain | Adopt, at function grain; the re-stamp with no build is where most of the saving lies |
| 2. A stale map is only slower | Adopt, on condition the consumer can detect the mismatch from the map | Adopt, as the fact everything else rests on | Adopt |
| 3. Build when idle | Adopt; idle is a quiet pool, enforced by yielding | Adopt, and add yielding | Adapt: run whenever a slot stays free, since idle alone starves |
| 4. Drop or lengthen the age rebuild | Drop | Drop once `T4` lands; 30 days until then | Drop, with a 30-day low-ranked backstop until the audit is trusted |
| 5. Share across machines | Adapt, last: a history-free ref | Adapt: file export and import, no synced storage | Adapt and defer: single writer, dedicated ref |
| 6. Only gate scoping needs freshness | Adopt as the ordering principle | Adopt; costly packages first | Adopt, and skip packages a map cannot help |

## What each proposal asks of the gate-scope tasks

`T4`, `T14` and `T15` wait on this research, so these are still cheap to take up.

- **All three**: the selector of `T14` takes its target tree as an argument, so upkeep can call it with HEAD;
  the map records an environment stamp;
  `T4`'s criterion that a linked change leaves a map fresh is replaced.
- **A and O**: the selector compares the package's own non-test files by function, as it does a linked package's, where D5 makes any own-file change a miss.
  All three add a rule for test files: a changed `Test` function concerns that test, any other change the whole package.
- **A**: the map holds a commit per entry, and the selector takes entries grouped by commit.
- **B**: the map holds outcome entries and a measured commit per entry; `gateslot` exports its non-blocking take.
- **O**: testmap reads the spawn verdict that `T4` records and does not run those tests; a failed build keeps the old map.
- **A and B** both ask whether `T4` should record another package's coverage as functions rather than blocks, since D4 matches on functions and the maps would shrink.

## Claims checked against the code

Checked at commit `4ae8978ec` while writing this report:

| Claim | By | Result |
|---|---|---|
| `mapPackage` reruns every mapped test, serially, through one profile path, and writes once after the last test | all | Holds (`internal/gatecli/testmappackage.go`) |
| The slot is held from the first build to the end of the call | all | Holds (`holdSlot`, `close`) |
| `ExitRecord.Unbuilt` is written and never read by production code | A | Holds |
| `writeFailed` replaces a usable map with an empty one | O | Holds |
| testmap never calls `ScanSpawns`; only the redundancy step does | O | Holds |
| `Pool.tryAcquireAny` exists unexported; `Holders` and `Draining` are exported | B | Holds (`internal/gateslot/pool.go`) |
| `judgeRuns` leaves out a map test whose `FuncHash` differs | A, O | Holds (`internal/gatecli/redundancy.go`) |
| `tiergate.Require` call sites: 4424 for tier 0 and 903 for tier 1 | all | Holds, by `grep` |
| Slots: 2 | A, B | The template's value; this hub's `gate.yaml` holds 3 |
| First-parent commits on main from 2026-11-04 to 2026-11-11: 173 | O | 172 by `git log` |
| Packages: about 170 (B, from `E12`), about 195 (O), 202 directories with a test file (A) | all | 202 directories hold a tracked `_test.go` file, 216 a tracked Go file |

Not checked: the run-record figures O cites, the standalone and gate times of `internal/fabricengine` B cites, and the Sketch's weekly sums and non-body counts that all three cite.

The estimates of a first full set differ by a factor of about four: A gives 30 to 120 minutes of serial slot time, B 13 to 27.
They agree on the test count and differ in the guessed cost per coverage build (5 to 20 s against 2 to 5 s) and per test process (0.1 to 0.5 s against 20 to 100 ms).
O gives no total.
Nothing in the repository settles it; the first measurement below does.

## Measurements the proposals ask for

The same few measurements recur, and none needs the new design to exist:

1. **The split of a full build** into coverage build, process start, profile handling and test body, per package (A, B, O).
   It decides whether parallel test processes help or the build per package is the floor.
2. **A history replay of the classifier** (A, B, O): after one full map set, run the selector alone over a week of main's first-parent commits.
   It runs no test and gives, per landing and map, kept entries, affected tests with their recorded seconds, and misses with their reason.
   This is the upkeep workload, and it shows whether misses dominate.
3. **How much of the day the pool is idle**, from the start and end of the retained gate run records (A, B).
4. **Whether a footprint is the same measured serially and side by side** (A, B), and how stable it is across two runs at one commit (A, O through the audit).
5. **Whether an instrumented package is reused from the build cache** across the coverage binaries of several importers under D2's coverpkg lists (A, O).
6. **Map size under D2** (A, B, O), which decides the sharing carrier and A's second entry per test.

## What the design task has to decide

- Whether a map carries one commit (B, O) or each entry its own (A), and whether a map may hold gaps (A, B) or is always complete (O).
- How upkeep leaves a slot: a request protocol (A), leaving when the pool fills (B) or yielding only to a drain at a test boundary (O);
  and whether it waits for a quiet pool or runs whenever a slot stays free.
- Whether to measure in the prime or in a pinned checkout.
- Whether to skip tests the spawn scan marks, and whether every test or only `t.Parallel()` tests run side by side.
- The order of work, and whether some packages get no upkeep.
- Whether an age backstop stays, and for how long.
- What changes in `T4`, `T14` and `T15` before they are built, above all the selector's own-package rule and its target argument.
- Whether narrowing D5 is taken up, since every proposal names it as the one lever on the remaining cost.
- Which of the measurements run before the design, and which the first build step can carry.
