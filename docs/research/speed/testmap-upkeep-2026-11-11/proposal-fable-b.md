# Carry on landing, measure when idle: coverage maps that go stale one test at a time

## Summary

Today a map is one unit: any change to its package's Go files, or its seventh day, throws all of it away and reruns every test.
This proposal makes the test entry the unit and splits upkeep into two phases.
**Carry** moves a map to the prime's HEAD without a build, a test or a gate slot:
it keeps each entry whose test reached no changed function and drops the rest.
**Measure** runs only the tests a map lacks, each alone, several at a time, in a gate slot it takes only while the pool is idle and gives back the moment a gate needs it.
A map with entries missing is a valid map, because a consumer runs a test its map does not record.
The rule that decides which entries carry is the rule the coverage gate scope will use to decide which tests to skip (D4 and D5 of the Sketch `gate-scope-by-coverage-map`),
so carry adds no new way to be wrong.
The age rebuild goes; a build stamp and a small audit sample replace it.

## Diagnosis

### What is essential

Three costs follow from per-test coverage itself.

- Coverage counters are process-wide (`internal/testcover/doc.go`), so a test's profile is its own only when no other test ran in the process.
  `Toolchain.RunAlone` in `internal/testcover/runner.go` therefore starts the coverage binary once per test.
- A package needs one coverage binary, built by `Toolchain.BuildBinary` with `go test -c -cover`.
- Each measured test costs its standalone run time, which includes fixtures a suite pays once.
  The fabricengine report measured 236 s for 590 tier 1 tests run alone, against 132 s of `go test` package time for the same tier in a gate
  (read from `docs/research/speed/fabric-test-redundancy-2026-11-08/report.md` and `docs/research/speed/fabricengine-per-test-times-2026-11-07.md`).

Nothing else in today's upkeep is forced.

### What is a choice

1. **The package is the unit of staleness and of rebuild.**
   `testMapRun.stale` in `internal/gatecli/testmap.go` compares the map's `Files` with HEAD's hashes, and one differing file reruns every test in `mapPackage`.
   A landing that edits one test of `internal/fabricengine` reruns all of its mapped tests.
2. **Tests run one at a time.**
   `mapPackage` in `internal/gatecli/testmappackage.go` loops `for _, fn := range mapped` and `runTest` reuses one profile path, `cover.out`.
   Separate processes have separate counters, so the serial order buys no correctness.
3. **A package's work is all or nothing.**
   `mapPackage` writes only after the last test.
   A terminating signal returns before the write, and `changedFiles` discards the whole map when a landing touched the package while its tests ran.
   The Sketch's constraints count 26 commits to `internal/fabricengine` in one week, the package whose tests take longest to map.
4. **The slot is held from the first build to the end of the call.**
   `testMapRun.holdSlot` calls the blocking `Pool.Acquire` and `close` releases it, so a gate arriving mid-call waits out up to `--max` packages.
5. **The trigger counts landings and the call is capped.**
   `daemon.testMapStep` in `internal/dobbyengine/testmap.go` starts a refresh after `test_map_every` landings (5, read from `internal/dobbyengine/template.yaml`) with `--max` `test_map_batch` (5, same file).
   A backlog waits for more landings, which note `testmap-backlog-drain` (E12) reports as about 170 landings for a first set.
6. **Age rebuilds everything weekly.**
   `stale` also returns true past `test_map_max_age_days` (7, read from `internal/gateslot/template.yaml`), whatever changed.
   It is the only bound on a dependency change or a non-Go file change, and it pays for that bound with a full pass.
7. **Every package has the same priority.**
   `mapPackages` walks the packages in path order, so a package no gate spends time on is built before one that dominates the gate.
8. **The maps are per machine.**
   `testcover.Dir` puts them under the board directory's `.lym`, which git excludes.
9. **Measurements are thrown away.**
   The redundancy step (`redundancyRun.runTest` in `internal/gatecli/redundancy.go`) already runs each new or changed test alone under a coverage binary and keys its blocks, then keeps only the verdict.

### The cost that is coming

Today's maps record own-package blocks, so only choices 1 to 7 hurt.
`testmap-cross-package` (T4) makes a map record every linked in-module package, and its record passed the dependency staleness rule to this note.
If that rule were "a linked file's hash differs, rebuild the package", a landing in a widely linked package would rebuild most of the module.
The Sketch measured that at least 95 of 118 code commits in one week hold a change outside function bodies,
and that `internal/fabricengine` still misses about twice a day (read from the constraints and open risks of T4's record).
Under D5 such a change in a linked package is a miss for every importing package until its map is rebuilt.
So the upkeep that T4 to T15 need is "bring every importer's map to HEAD after most landings", which whole-package rebuilds cannot deliver.

### What the gain is worth

The Sketch sums the `darn-write` tier runs of one week to 191 min of tier 0 and 516 min of tier 1 over about 245 runs each (read from T15's record).
That is about 100 min a day, and it is the ceiling of what coverage scoping can save on that key.
An upkeep that holds a gate slot for a comparable time while runs work cancels the saving it exists for.
Upkeep therefore has to cost no gate latency at all, and its compute has to go where gates spend their time.

## Design

### An entry, not a map, is stale

A map stays one JSON record per package at `testcover.MapPath`, with one `Commit` for the whole record.
Each test entry is either present or absent.
Carry decides, for a map at commit M and the prime's HEAD C, which entries stay.

An entry of test T is dropped when, between M and C:

1. T's function text changed (`MapTest.FuncHash`, which the map already carries) or T is gone.
2. A function T reached changed or was removed: its declaration differs between M and C, compared as D4 compares it.
   Today that covers the package's own files; after T4 it covers every linked package the map records.
3. A change that no function carries touched a file the map records.
   Every entry of the map is then dropped.
   The cases are D5's: a changed const, var, type, import, `//go:` line, method set or `init` count,
   a changed non-Go file of the recorded set (`go:embed` targets and `testdata` included, `inert_paths` exempt as D5 exempts them),
   and a file that does not parse.
4. A non-Test declaration of the package's own test files changed, such as a helper or `TestMain`.
   Test files are not instrumented, so the map cannot tell which tests use a helper, and every entry is dropped.
   An added test file or an added helper under a new name drops nothing, as D5's "added declaration no unchanged code can reach" drops nothing.
5. The map's **build stamp** differs from the current one.
   The stamp holds the Go toolchain version, GOOS and GOARCH, the hash of `go.mod` and `go.sum`, a digest of the mapped tiers' `command` and `env`, and the map format.
   `impactset` already runs a gate whole when `go.mod` or `go.sum` changed since the base (`internal/impactset/doc.go`),
   but a change that landed before the run's base is in no run's diff, so the map has to record it.

A test the package declares that has no entry is not stale, it is unmeasured.
A test that failed, timed out, skipped or covered no block gets an entry that records that outcome and no blocks.
Carry keeps such an entry only through a rebase that changed no recorded file of the package, so the test is tried again after any relevant landing and not on every pass.

There is no age rule.
Rule 2 and rule 3 bound a dependency change and a non-Go file change exactly, which is what the age limit only approximated.

### Why carry stays correct

Assume a test's execution depends only on the code it executes and on the inputs the map records.
If no function T executed changed, and no change outside functions touched a recorded file, T executes the same statements at C as at M.
It therefore covers the same blocks, and those blocks keep their keys:
a key is file, enclosing function, content hash and an ordinal counted among equal triples (`BlockKey` in `internal/testcover/blockkey.go`),
all of which are fixed by the text of an unchanged function.
The one ordinal that spans functions is `PackageLevelFunc`'s, and a change to a package-level initializer falls under rule 3.

This is the assumption the coverage scope already makes.
D4 skips T for a diff when T reached no changed function, which is the claim that T behaves the same.
Whenever carry would wrongly keep an entry, the selector would already have wrongly skipped the test for the same change.
Both fail the same way and both are caught by the same backstop, the `finalize` gate at scope `all`.
So carry and the selector call one classifier in `internal/testcover`: carry applies it to M against HEAD, the gate to the map's `Commit` against the working tree.

Carry also checks itself when measure next builds the package.
Measure reads a fresh block table from the first profile, as `fillMapTests` does today, and re-indexes every carried entry by key.
A carried entry with a key the fresh table lacks is dropped and measured again.

### The audit replaces the age rule

Each entry records the commit and time at which its test last ran.
Every measure pass re-runs a few carried entries, oldest first, and compares the result with the carried blocks.
A mismatch is logged with the test, both commits and the differing functions, and the entry is replaced.
The age rule rebuilt everything on suspicion and reported nothing.
The audit spends a fixed share of idle time, re-measures every entry eventually, and yields the number the whole design rests on:
how often an entry the classifier kept was wrong.
That number also audits the gate's selector, which nothing else does before a landing gate fails.

### Measure: only what is absent, several at a time

For one package, measure builds the coverage binary once and runs each absent test alone, as today, with these changes.

- Up to the slot's parallel (`Limits.Tier0Parallel`, which `holdSlot` already reads) test processes run at once, each with its own profile path.
  A test that fails while others run is tried once more with nothing else running before its outcome is recorded.
- The map is written after each batch of finished tests, not once at the end.
  A written map lists only tests that ran against an unchanged HEAD: the call reads HEAD before and after each batch and discards a batch that straddles a change.
- A package with no absent test and no audit due gets no build.

The unit of rebuild is the test, and the unit of writing is a batch of tests.
A package as large as `internal/fabricengine` can be measured across several idle periods.

### Who starts it, when, and what it holds

`lym gate testmap` stays the one writer and stays in the prime worktree.
One call does carry for every map, then measure for as long as it may.

**Carry** holds no gate slot.
It runs no go command: it reads file content at two commits through `Fabric.CodeFileAt`, as `fileHashes` does today, and parses the changed files.
It holds the package's build lock (`testMapBuildLockPath`) while it rewrites a map.

**Measure** holds one gate slot under two rules that keep it out of every gate's way.

- It takes a slot without waiting, and only when another slot stays free.
  `Pool.tryAcquireAny` in `internal/gateslot/pool.go` already is the non-blocking take; the pool needs it exported, with the drain lock respected.
- While it holds the slot it checks the pool at each test start and at each `Pool.Poll` interval.
  When every slot is held (`Pool.Holders`) or a multi-slot acquire drains (`Pool.Draining`), it kills its tests in flight, writes what finished, releases and exits.
  A killed measurement is lost work of one test and nothing else.

With the template's `slots: 2` this means measure runs only while no gate runs.
A gate that takes the last free slot shares the CPU with measure for at most one poll (two seconds by `Pool.Poll`'s default),
and the gate after it waits at most that long for a slot.
A hub with one slot gets no background measure; a call by hand still works there.

**The trigger** is work, not a landing count.
The exit record (`testcover.ExitRecord`) gains the HEAD the call carried to and the count of absent entries, replacing `Unbuilt`.
The prime dobby's step starts a call when none is live, and the prime's HEAD differs from the recorded one or the count is above zero,
and the pool has been idle for a short quiet period, so a run's back-to-back gate calls do not start and stop it.
`test_map_every`, `test_map_batch` and `State.LandingsSinceTestMap` retire.
Note `testmap-backlog-drain` is answered by construction: a backlog is work, so the next idle tick continues it.

**The order** of measure is by where gates spend time.
Every tier run already records each package's `ElapsedSeconds` in `RunRecord.Launches` (`internal/gateslot/runlog.go`).
Measure reads the retained run records and takes packages by their summed seconds, largest first, then every other package.
The order excludes nothing: idle time that the costly packages do not need goes to the rest, so every package gets a map.
No gate writes anything new, so D8 ("the gate only reads maps") stands.

**A call by hand** naming packages takes its slot with the blocking `Pool.Acquire`, as today, and measures every absent entry of those packages before it returns.
A `--remeasure` flag drops their entries first.
That is the path for a consumer that needs complete maps at a known time.

### Where the maps live, and other machines

The maps stay under `<BoltDir>/.lym/testcover`, machine-local and untracked.
They are a cache whose miss costs time and never correctness, and tracked storage would cost more than it returns:
a map rewritten after most landings is steady churn in the Bolt checkout or on a records branch, and two machines writing one file need a merge rule.

The entry design makes a map portable all the same.
A map from another machine is admissible when its build stamp matches and its `Commit` is readable in the local object store.
Carry then brings it to the local HEAD by the ordinary rule, however old it is.
So the transfer is a plain copy with no trust beyond the stamp:
`lym gate testmap --export <file>` and `--import <file>` over the maps directory, which `lym adopt` can name in its way forward.
A machine that imports nothing starts from no maps and converges in idle time, costliest packages first.
Whether that is fast enough to make even the copy unnecessary is a measurement (see Cost and gain).

### What a consumer does with a missing or stale map

- **The coverage scope (T14, T15)** changes nothing in D3 to D8.
  A missing or unreadable map, or one whose stamp differs, is a D5 miss and runs the package whole.
  A test with no entry, or with an outcome entry, is in D3's union ("every test the package declares that its map does not record") and runs.
  A map that carry has not yet moved compares from its own `Commit`, which D4 says can only add tests.
  Every lag in upkeep therefore widens the gate and never narrows it, which is why upkeep may wait for idle time.
- **The redundancy step** reads the entries that are present.
  An absent test is no cover (`judgeRuns` builds `kept` from the map's tests), so a gap can only turn a "contained" verdict into "unique".
  The step is advisory, and its documented bound already covers a let-through.
- **The sweep of existing redundant tests (E4)** needs every entry of its packages measured, not carried from long ago.
  It calls `lym gate testmap --remeasure <packages>` first and judges only a package whose map then has an entry for every declared mapped test.

## The six starting ideas

1. **Update a map in part: adopt, with a corrected rule.**
   The note's rule reruns a test whose recorded blocks include a changed or removed block.
   That misses added blocks.
   An `if` inserted into a function adds blocks, and can add a call into a function the test never reached, while a block after it keeps its text and so its key.
   A test that covered only the unchanged blocks would keep an entry that lacks the new callee, and a later change there would not select it.
   The function is the right grain: an entry is dropped when any function it reached changed, which is D4's rule and needs no block boundaries.
2. **A lagging map is harmless: adopt, and make it the load-bearing fact.**
   It holds for the scope because D4 compares from the map's own `Commit` and D3 runs unrecorded tests.
   It is what permits idle-only measure, partial maps and killing a measurement.
3. **Build when idle: adopt, and add yielding.**
   Starting only when idle is not enough for a call that can run for minutes; the call also has to leave when a gate arrives.
4. **Drop or lengthen the age rebuild: drop it once T4 lands.**
   Linked file hashes with carry bound a dependency change exactly, the build stamp covers the toolchain and `go.mod`, and the audit covers what the classifier cannot see.
   Before T4 a map records no dependency, so the age key stays as that one bound, lengthened to 30 days, since the only consumer until then is advisory.
5. **Share across machines: adapt.**
   Reject tracked or synced storage; keep the maps a local cache; make them portable by stamp and `Commit` and move them by export and import.
6. **Only scoping needs freshness: adopt, and sharpen.**
   Scoping needs entries for the packages gates spend time on, which the run-record order delivers first.
   The sweep needs complete, recently measured maps once, which a blocking call by hand delivers.
   Neither needs every map fresh at every HEAD.

## Cost and gain

### What gets cheaper, by reasoning

- **Gate latency caused by upkeep drops to about zero.**
  Today a gate can wait behind up to five package builds; with yielding it waits at most one poll.
  The prime's anchor move today waits out a whole refresh (`internal/dobbyengine/doc.go`, the test-map refresh bounds);
  it could end the call instead and wait for one write, since ending it loses only the tests in flight.
- **The test phase divides by the slot's parallel.**
  The build phase does not.
- **A landing costs the entries it invalidated.**
  A landing that edits tests alone costs those tests plus one build per touched package: for `internal/fabricengine`, one test in place of 590 tier 1 tests and their 236 s (read from the report above).
  A landing that changes only function bodies costs the tests that reached them.
  A landing with a change outside function bodies costs every test of each map that records the file, which is today's cost for an own-package map.
- **The weekly full pass disappears.**
  The audit replaces it at a rate chosen in idle time.
- **A first full set takes one pass, not about 170 landings.**
  My estimate of that pass is 13 to 27 minutes of serial work:
  about 170 packages (E12) at an estimated 2 to 5 s of coverage build and link each;
  4424 tier 0 call sites of `tiergate.Require` at an estimated 20 to 100 ms of process cost each;
  and tier 1 at the measured 236 s for fabricengine's 590 tests plus an estimated 1 to 3 minutes for the other 313 call sites.
  The call-site counts are mine, taken with `git ls-files` and `grep` over the tracked test files on 2026-11-11; every duration but the 236 s is an estimate.
- **No work is lost.**
  A signal, a yield or a landing mid-package keeps every finished test.

### What the design does not make cheap

Carry saves nothing on a change outside function bodies, and the Sketch measured such a change in at least 95 of 118 code commits.
After T4 each of those empties the maps of every importer of the touched package.
The design answers with speed and placement, not with avoidance: the refill runs in parallel, in idle time, costliest packages first, and a gate in the gap runs whole as it does today.
The lever that would avoid the work is the classifier's precision, which belongs to T14's D5 and to the options under Risks.

### What I could not know without measuring

| Unknown | Measurement that settles it |
|---|---|
| Per-test process cost and per-package build cost, which decide the length of a pass | One `lym gate testmap` call over ten packages with a log line per phase; `PackageLaunch.LaunchSeconds` in the existing run records already gives exec, init and `TestMain` per package |
| The share of landings, per package, where carry keeps entries | After one full build, replay main's first-parent history through the classifier and count kept and dropped entries per landing; the replay runs no test |
| How much of the day the pool is idle | The `Start` and `End` of the retained `RunRecord`s give the time with no slot held |
| How often a test fails only because others run beside it | One pass at parallel 1 and one at the slot's parallel over the same HEAD, comparing outcome entries |
| How often a carried entry is wrong | The audit's mismatch count, from the first week of running |
| The cost of a profile that lists every linked package's blocks after T4 | Time `RunAlone` on one widely linking package under T4's coverpkg; `ParseProfile` runs `fmt.Sscanf` per line (`internal/testcover/profile.go`), which I expect to matter at that size |
| The size of the maps directory, which decides whether export is a file copy or a burden | `du` after the first full set, before and after T4 |

## Risks and open questions

- **The assumption can fail.**
  A test whose path depends on time, randomness, the environment or a file outside the recorded set covers different code from run to run.
  That already makes any map wrong for it; the audit is the first thing that would show it.
  A test the audit catches twice could be recorded as always run.
- **Tests alone but side by side may collide** on a fixed path or port.
  The retry with nothing else running contains it, and a test that still fails is recorded as such and always runs in a scoped gate, which is safe and slow.
  The standalone `Seconds` of a test measured beside others is inflated; `JudgeAgainst` uses it only to order the greedy pass.
- **Killing a measurement** relies on the process-group kill `Toolchain.command` already configures; a test that leaves files outside its temporary directory leaves them on a kill as on a timeout.
- **Refill after changes outside function bodies may dominate.**
  Two options would cut it, and both need a decision this proposal does not make.
  One is a finer D5, where a changed declaration stales only the functions that reference it, which needs type information.
  The other is a confirmation run: one ordinary coverage run of the package, and when the union of covered functions is unchanged the entries are kept.
  That is cheap and not sound, since two tests could swap paths inside an unchanged union; only the landing gate would catch it.
- **Changed helpers in test files drop a whole package.**
  The spawn scan (`spawnScanner.analyze` in `internal/testcover/spawnscan.go`) already follows same-package calls from a test, so a per-test helper closure is a possible later refinement.
- **A machine that runs gates but whose prime dobby does not run** gets no background upkeep.
  I did not read whether such a machine exists under the owner-machine rule; if it does, import is its only source of maps.
- **T4's map shape decides carry's cost and the export's size.**
  D4 matches on functions, and the redundancy step judges own-package blocks only.
  Recording another package's coverage as functions, not blocks, would shrink what every carry rewrites; T4's record chose block keys with an import path.
- **The landing gate as a producer.**
  `finalize` runs every test on the tree that becomes HEAD.
  Running it through the measuring runner would refresh every map with no second run of any test.
  It would also put process-per-test cost and a new failure mode on the landing path, so I leave it out until a pass has been timed.
- **The quiet period and the audit share** are two new numbers.
  Fixed constants are enough to start; they become config only if the measurements above ask for it.

## Build order

1. **Run a package's tests several at a time and write per batch.**
   A profile path per test, the slot's parallel as the bound, the retry alone, HEAD read around each batch.
   Maps and staleness stay as they are; a signal or a landing mid-package stops costing the package.
2. **Trigger on work, not on landings.**
   The exit record gains the carried HEAD and the absent count; the dobby step starts a call while either says work remains.
   This closes `testmap-backlog-drain` and retires the two beam keys.
3. **Take the slot without waiting and yield.**
   Export the pool's non-blocking take, add the yield check, add the dobby's idle check and quiet period.
   Named packages keep the blocking take.
4. **Entries and carry for the package's own files.**
   Outcome entries, the per-entry measured commit, the build stamp, and a first classifier: function declarations compared as text, any other difference dropping the map.
   `stale`'s file comparison and the age rule give way to carry; the age key stays at 30 days until step 7.
5. **Order by gate seconds.**
   Read the run records, sum `ElapsedSeconds` per package, measure in that order.
6. **The audit sample**, with its log line and a count in the call's output.
7. **Carry across linked packages**, with T4: the same rule over every recorded package, and the age key retires.
   With T14 the first classifier is replaced by the D4 and D5 one, so the gate and carry share it.
8. **Export and import**, and `--remeasure` for the sweep.
9. **Optional: keep what the redundancy step measures.**
   Its runs are alone and keyed already; an entry it produced on a tree whose recorded files equal HEAD's can fill an absent entry without a second run.
