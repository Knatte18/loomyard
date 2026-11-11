# Coverage footprints as a content-keyed cache, filled per test in idle time

Proposal A (Fable 5.2), written 2026-11-11 against commit `4ae8978ec`.
Nothing here was run: every number is marked as read from code, config or a committed record, counted by `grep`, or my estimate.

## Summary

A coverage map stops being a per-package snapshot with a freshness date.
It becomes a cache of per-test footprints: each entry records what one test executed at one commit,
and it holds for every tree in which that code is unchanged.
No entry is ever stale in place; an entry matches a tree or it does not, and the consumer checks that itself, by content, when it reads.
Upkeep then has one job: measure, at the prime's HEAD, the tests that have no matching entry.
It measures one test at a time as the unit, several test processes at once,
inside a gate slot that it gives up within seconds when a gate asks for it.
The prime dobby starts it when a backlog exists and the pool is quiet.
The age limit, the landing count and the package cap go away.

## Diagnosis

### Where today's cost comes from

1. **The unit is the package, though the measurement is per test.**
   `testcover.Map` carries one `Commit`, one `Files` table and one `BuiltAt` for all its tests,
   so `testMapRun.mapPackage` (`internal/gatecli/testmappackage.go`) can only replace the whole record,
   and it runs every mapped test of the package again.
2. **Staleness is judged by two proxies.**
   `testMapRun.stale` (`internal/gatecli/testmap.go`) compares the package's own Go file hashes and the map's age.
   The file rule is too coarse: one edited line reruns every test of the package.
   The age rule is blind: it rebuilds packages in which nothing changed,
   and it is the only thing that ever picks up a changed dependency or non-Go file (`internal/gatecli/doc.go`, section "Coverage maps").
3. **The tests run one after another.**
   `mapPackage` loops over the mapped tests and calls `Toolchain.RunAlone` for each, all writing the same `cover.out` path.
   Each run is a process start of a large binary, a text profile of every block of the package written and parsed, and the test itself.
4. **The slot is held from the first build to the exit, and taken by a blocking acquire.**
   `testMapRun.holdSlot` calls `Pool.Acquire` once and `testMapRun.close` releases it,
   so a refresh of several packages holds one slot for its whole length.
   The template declares `slots: 2` (`internal/gateslot/template.yaml`), so a refresh takes half the hub's gate capacity while it runs.
5. **The trigger knows neither the load nor the backlog.**
   `daemon.testMapStep` (`internal/dobbyengine/testmap.go`) starts a refresh when `LandingsSinceTestMap` reaches `test_map_every`, whatever the pool is doing.
   `testcover.ExitRecord.Unbuilt` is written by `testMapRun.close`, and no production code reads it; note E12 states the consequence.
6. **Every machine builds its own set**, under `testcover.Dir`, which is the board's never-tracked `.lym`.

### The defaults cannot converge

This is arithmetic over configured and recorded numbers, not a measurement.

- The cap lets through `test_map_batch` (5) packages per `test_map_every` (5) landings: one package per landing.
- The T4 decision record counts 118 commits in seven days that touch non-test Go under `internal/` or `cmd/`, about 17 a day.
  Each makes at least one map stale by the own-files rule, so the inflow from landings alone is at least the outflow.
- The age limit of 7 days adds one rebuild a week per package.
  Note E12 counts about 170 packages, and `find` shows 202 directories holding a `_test.go` file,
  so age adds up to 24 to 29 package rebuilds a day; a package with no mapped test needs no build.
  That alone can exceed what the cap lets through at 17 landings a day.

So today's upkeep is not only large; at its own defaults it never reaches a full set and never stays there.
Raising the cap repairs the arithmetic by holding a slot longer, which is the cost the operator objects to.

### What T4 will add

T4 records, per package, each test's coverage across every in-module package its binary links.
A change to a widely linked package then concerns every importer's map.
Under a whole-package unit that is a near-full rebuild per landing to such a package,
and the same record counts 15 of 26 weekly commits to `internal/fabricengine` as non-body changes that a map cannot reason about.
The upkeep design has to be sized for that case, not for today's own-package maps.

### How large a full build is

Unknown, and the split matters more than the total.

- Counted by `grep` at this commit: 4424 `tiergate.Require(t, 0)` and 903 `tiergate.Require(t, 1)` call sites, so about 5300 process starts for a full set.
- Read from `docs/benchmarks/test-redundancy.md` (generation date and machine not stated in the file; its last content predates 2026-11-01):
  3506 tests in 116 packages whose test times sum to 335 s.
- So the test bodies are a small part.
  My estimate for the rest: 5 to 20 s of coverage build and link per package, and 0.1 to 0.5 s per test for process start, profile write and profile parse.
  That puts a full serial build at 30 to 120 minutes of slot time, with the builds and the process starts as the two large terms.
- `internal/gateslot/doc.go` notes a host scanner that holds each unseen executable.
  Every coverage build is a new executable,
  and board note R3 records a large unexplained machine factor on OSL-1046,
  so the estimate may be low there.

### Essential and chosen

Essential to per-test coverage:

- **A process per test.**
  Coverage counters are process-wide (`internal/testcover/doc.go`).
  The Go toolchain's `runtime/coverage` does offer `WriteCounters` and `ClearCounters` (read from its `coverage.go`; `ClearCounters` needs atomic mode),
  and every test opens with `tiergate.Require`, which could host a snapshot hook, so one process per package is technically reachable.
  I reject it as the source of truth: code that runs once per process, such as a `sync.Once` body, a lazy cache or a shared test fixture,
  would be attributed to the first test alone.
  A gate that scopes by the map would then skip the other tests that depend on that code.
  A process per test attributes it to every test that needs it.
- **One coverage build per package touched**, because Go builds one test binary per package.
- **Measuring a test again when code it executed changed.**

Choices that could be made differently:

- the package as the unit of validity and of rebuild;
- staleness by file hash and by age;
- running the test processes one at a time;
- holding the slot throughout, and waiting for it in line with gates;
- starting on a landing count;
- one map set per machine.

## Design

### An entry is a cache record matched by content

Each `MapTest` entry carries its own commit and measurement time, where today the map carries them once.
An entry says: this test, with this function text, run alone at this commit, executed these functions and blocks.
An entry **matches** a tree when nothing it depends on differs between its commit and that tree.
The table gives the rule; the comparison is the one T14's D4 and D5 already define, taken per entry.

| Difference between the entry's commit and the tree | Entries that stop matching |
|---|---|
| The test function's text (`FuncHash`) | that test's |
| A test file of the package, outside its top-level Test functions | every entry of the package |
| A function of the package's own non-test files: receiver, signature or body | entries that covered a block of that function |
| A function of a linked package (needs T4's footprint) | entries that covered it |
| A non-body change D5 does not exempt, in a Go file of the package or of a linked package | every entry of the package |
| A non-Go file under the package's directory or a linked package's, D5's `inert_paths` rule applied | every entry of the package |
| The environment mark: Go toolchain, `go.mod` and `go.sum`, the tier's `env`, GOOS and GOARCH, the map format | every entry |
| Nothing above | none, whatever the entry's age |

Three consequences:

- **A dependency change is handled exactly**, once T4 records linked footprints: an importer's entries stop matching only where they reached the changed function.
- **A non-Go file change is handled exactly but coarsely**: no entry records which test reads which `testdata` file,
  so the whole package's entries stop matching.
- **Age is not a rule.**
  An old entry whose inputs are unchanged is as good as a new one.

A map may keep more than one entry per test, the newest `G` (I propose 2).
The gate runs in task worktrees whose base lags the prime, while upkeep measures at the prime's HEAD.
With one entry per test, each non-body landing to a linked package makes the new entries useless to every worktree that does not hold that landing yet,
and the old entries useless to every worktree that does.
Keeping the previous entry serves both.
Content keys make this safe: `BlockKey` (`internal/testcover/blockkey.go`) is designed so that an unchanged block keeps its key across commits.

Before T4 lands, entries hold own-package blocks only, as today, and the rows that name a linked package see only the package's own directory.
A dependency change is then invisible to the match, exactly as it is to today's file rule;
the audit below bounds it.

### The unit of rebuild is one test, and why that stays correct

Upkeep measures a test when the package declares it at HEAD and no entry of it matches HEAD.
That covers a new test, a changed test, a test whose covered code changed, and every test of a package with no map.

A partial update is correct for three reasons.

1. **Entries are independent measurements.**
   Each comes from its own process (`Toolchain.RunAlone`), so an entry's content never depended on its siblings or on when they ran.
   The shared `Commit` is an artefact of the record's shape, not of the measurement.
2. **An unmeasured test is trusted by the same rule the gate already uses.**
   Leaving an entry in place across a landing claims that the landing cannot change what the test executes.
   Gate scoping (T14, T15) makes that same claim about a task's diff when it skips the test.
   The design asks T14's selector to take entries grouped by their commit and to cover the package's own files too;
   upkeep then calls that selector with HEAD's tree and the gate calls it with the working tree.
   One function, two callers, one safety argument.
3. **Block keys stay comparable.**
   The doc's bound on ordinals (`internal/testcover/doc.go`) concerns identical blocks inside one function.
   A change there changes the function, so every entry that covered it stops matching and is never compared under a shifted ordinal.

`Map.Blocks` becomes the union of the keys its entries reference.
`fillMapTests` today stores every block of the first profile, covered or not, and no reader uses the uncovered ones.

A simple first form of the comparison needs nothing from T14.
Hash each function declaration's text, as `ParseTestFuncs` already does for tests, and hash the file with its function declarations cut out.
An unequal function hash is a changed function, and an unequal remainder is a non-body change.
That is stricter than D4 and D5, so it only measures more, and T14's finer comparison replaces it later.

### The backlog and its order

A **plan** step computes the backlog without running anything and without a slot.
For each distinct entry commit it reads the paths that differ from HEAD, once,
and only maps whose package or linked packages hold such a path are compared further.
Today `testMapRun.mapPackages` hashes every Go file of every package at HEAD on every call.
The plan writes an index record beside the maps, replacing `testcover.ExitRecord`: the planned HEAD, and per package the unmatched tests with the sum of their recorded `Seconds`.
The backlog is therefore a known quantity in seconds before any slot is taken.

The **measure** step works through it in this order:

1. packages named on the command line;
2. packages a gate asked for and could not use: T15's scope report already names each `miss`,
   and the gate appends the ones upkeep can fix to a demand record;
3. the remaining packages, cheapest first, so that the most maps become usable per idle minute;
4. when nothing is unmatched, the **audit**: measure the entries with the oldest measurement time again.

The audit replaces the age limit.
It spends only idle time that has no other use,
and it is also a check: an entry whose new footprint differs although its inputs match is unexplained drift.
The step logs it and marks the test unstable, and an unstable test never matches, so every consumer runs it.
That catches nondeterministic footprints, dependency drift before T4, and anything the matching rule does not model.

### Who starts the work, when, and what it holds

**The prime dobby starts it**, as today, through `startTestMap` (`internal/beamcli/dobbydeps.go`) and the `runproc.RoleTestMap` record.
`testMapStep` starts a session when all of these hold:

- no refresh is live;
- the index's planned HEAD differs from the prime's HEAD, or the index lists unmatched tests;
- the pool has a free slot and no drain, and has been quiet for a short configured time.

`test_map_every`, `test_map_batch`, `State.LandingsSinceTestMap` and `test_map_max_age_days` are retired.
Note E12 dissolves: a backlog is itself the trigger, so it drains without further landings.

**The session holds one gate slot as a yielding holder.**
This needs a small addition to `internal/gateslot`:

- `Holder` gains a `yielding` mark, written in the slot's record.
- A waiter in `Pool.Acquire`, `Pool.takeSlotsInOrder` or `Pool.acquireDrain` that finds no free slot leaves a request file beside each slot whose holder is yielding.
- The yielding holder polls for the request and for `Pool.Draining`.
  On either it cancels its context, which kills the test processes in flight through `Toolchain.Ctx`, writes the entries it has, and releases.
- The session takes its slot only without waiting, and never while a drain is held, so it never stands in line with gates.

A gate then waits behind upkeep for a few polls, where today it waits for the rest of the refresh.
`defaultPoll` is 2 s, so I estimate under ten seconds.
Interrupting costs at most the tests in flight, because finished entries are already written.
A hand call without the idle flag keeps today's behavior: it waits for a slot and does not yield.
The pattern rule that a gate build or test runs only inside a held slot (`pattern/PATTERN-gate-slot.md`) stands.

**Inside the slot the session runs several test processes at once.**
Isolation needs a process per test, not one process at a time.
The count is `Limits.Tier0Parallel`, which `holdSlot` already reads for the `-p` of its builds, so the session uses what one slot grants and no more.
Each process gets its own profile path.
Running beside other test processes is safe by the existing rule that tests tolerate parallel neighbours (`pattern/PATTERN-test-speed.md`, "Tests run in parallel"),
and process-global state is per process.
The child processes run at low CPU priority, so a gate in the other slot keeps the cores.

**Locks.**
The per-package build lock (`testMapBuildLockPath`) stays, so a hand call and a session never measure one package twice.
Map and index writes go through `internal/state` as today.

**A moving or dirty prime.**
The session reads HEAD before a package's build and after the last test of its batch, as `mapPackage` and `changedFiles` do today.
If HEAD moved and the difference touches the package or a linked package, the batch's entries are discarded and the tests stay in the backlog.
A package with a modified or untracked file is skipped, as `dirtyGoFile` does today, extended to linked packages by T4.

**Failures.**
A test that fails, skips or times out alone gets no entry, as today, so consumers run it.
A failed coverage build is recorded once per HEAD, as `BuildFailure` is today.

### Where the maps live, and sharing

The maps stay under `testcover.Dir`, shared by every worktree of the hub on the machine.
Sharing across machines becomes possible because an entry is validated by content, never by who wrote it:
an imported entry with an equal environment mark and a readable commit is exactly as trustworthy as a local one.
I propose export and import, not tracked files:

- The owning machine publishes the map set as one parentless commit on a dedicated ref of the records repository, force-updated, so no history accumulates.
- Another machine imports it into `testcover.Dir`, merging per test and keeping the newest `G` entries.
- Git access goes through `fabricengine`, as PATTERN-fabric-boundary requires.

Tracked files in the Bolt checkout would put megabytes of rewritten JSON into the board's history on every session.
I rank sharing last: without it a new machine starts with an empty cache, which costs one full build of idle time and no wrong answer.

### What a consumer does with a missing or unmatched entry

- **Gate scoping (T15).**
  A test of an importing package is left out only when one of its entries matches the working tree.
  A test with no entry, no matching entry or an unstable mark runs; a package with no readable map, or a different environment mark, runs whole.
  This is D3's union and D5's miss list, unchanged in effect.
  It is safe because leaving a test out needs positive proof, checked by the gate against file content at read time.
  The gate trusts no freshness flag, no timestamp and no bookkeeping of the refresher, since none exists.
  A late, interrupted or broken refresh can only make more tests run.
  The `finalize` gate with scope `all` remains the backstop for what the matching rule itself gets wrong, as D1's bound states.
- **The redundancy report step** (`redundancyRun.judgeRuns`).
  Only entries that match the working tree count as covers; today it checks the function hash alone and lists the drift.
  Fewer covers make a verdict lean to "unique", so an outdated entry can let a redundant test through and never condemns one.
- **The redundancy sweep (E4).**
  A cut task calls `lym gate testmap <package>` first and judges only when the package's backlog at HEAD is empty.
  It needs no standing upkeep.

## The six starting ideas

1. **Update a map in part: adopt, with a wider rule.**
   The note's rule, a changed test or a changed covered block, is incomplete.
   A test also depends on test-file helpers, which coverage does not instrument, on declarations outside functions, on non-Go files and on other packages.
   The table above is the complete rule, at function granularity so that the plan needs no coverage build.
2. **A stale map is never wrong, only slower: adopt, with its condition stated.**
   It holds only when the consumer can detect the mismatch from the map's own content.
   Own-package maps cannot show a changed dependency, and no hash shows a nondeterministic footprint; T4's footprints and the audit close those.
   For the redundancy report the price is weaker advice, not a slower gate.
3. **Build when idle: adopt.**
   "Idle" is defined by the gate pool, the one load signal every party can read,
   and it is enforced by the yielding slot, not by a guess at start time.
4. **Drop or lengthen the age rebuild: drop it as a rule.**
   Dependency changes are matched exactly after T4.
   The oldest-first audit in leftover idle time bounds everything else and costs no gate time.
5. **Share across machines: adapt, and build it last.**
   Export and import over a history-free ref; safe because consumers validate by content.
6. **Only gate scoping needs fresh maps: adopt as the ordering principle.**
   Gate demand orders the backlog, and the sweep refreshes its own package when it runs.

## Cost and gain

By reasoning; every figure here is an estimate unless it names its source.

| What | Today | Proposed |
|---|---|---|
| A gate's wait behind upkeep | the rest of a refresh of up to `test_map_batch` packages | a few slot polls, under ten seconds |
| Work after a body-only change | every test of the package; after T4, every test of every importer | the tests that covered the changed functions, plus one coverage build per package holding one |
| Work after a non-body change to a linked package | the same as above | every test of every importer, unchanged in size, but in idle time, resumable and demand-ordered |
| Mandatory age rebuilds | every package weekly, 24 to 29 packages a day | none |
| Wall time of the run phase | one process at a time | divided by the slot's parallel, less contention; I expect 3 to 6 times |
| Finding the backlog | every Go file of every package hashed per call | one path diff per distinct entry commit |
| First full set | never reached at the defaults | one full build spread over idle time |
| A new machine | a full build | a full build in idle time, or an import |

The per-test unit does not by itself make upkeep small.
The T4 record counts at least 95 of 118 weekly commits as holding a non-body change somewhere,
so most landings still leave some importing packages without matching entries.
Its per-package figures are better (`internal/lymdirs` 0 of 9, `internal/configengine` 1 of 6, `internal/configreg` 3 of 12), and worst for the most linked package.
What makes the remainder affordable is that it no longer costs gate time, runs several processes wide, and is spent first where a gate asked.

There is also a ceiling on what upkeep is worth.
The same record sums the `darn-write` tier 0 and tier 1 runs to 707 minutes in a week, about 100 minutes a day,
and scoping can save only the importers' share of that.
Upkeep that needed comparable slot time would be a loss; in idle time it competes with nothing.

What I could not know, and the measurement that settles each:

1. **The split of a full build** between coverage builds, process starts, profile handling and test bodies.
   Add build seconds, run seconds and test count per package to the verb's report and run one full set.
   If builds dominate, parallel test processes gain little and the floor of one build per touched package is the term to attack.
2. **How many tests a changed function unmatches.**
   Once a full set exists, replay the plan over a week of first-parent commits;
   it runs no test and gives unmatched tests and their seconds per landing.
3. **How much idle time the pool has.**
   Read slot occupancy from the gate run records under `gateslot.RunLogDir`.
4. **Whether parallel processes change a footprint.**
   Measure one large tier 1 package serially and in parallel and compare the entries; they should be equal.
5. **How stable footprints are.**
   Measure a sample of tests twice at one commit.
6. **Whether instrumented packages are shared in the build cache across packages' coverage binaries under T4's `coverpkg` lists.**
   I expect so, because each list is closed under dependencies; time a second importer's build after a first.
7. **The size of a map under T4**, measured on one widely importing package.
8. **How much a low-priority session slows a gate in the other slot.**
   That decides whether the session should also yield whenever any other holder appears.

## Risks and open questions

- **The matching rule now carries upkeep as well as scoping.**
  Today a whole rebuild flushes any error weekly; here a wrongly kept entry lives until its inputs change or the audit reaches it.
  The audit's rate is the mitigation, and its drift count is the measure of whether the rule is sound.
- **Per-entry commits change T14's input.**
  Its selector is specified against one map commit; it must take entries grouped by commit and handle own-package files.
  T4, T14 and T15 wait on this research, so the change is still cheap.
- **Map size.**
  T4 records linked blocks; D4 consumes only their file and enclosing function.
  Recording linked coverage at function granularity would shrink the maps and the work of keying them;
  that is T4's decision, and sharing and `G` depend on it.
- **Idle starvation.**
  On a day with no quiet pool the backlog grows and gates run whole, which keeps the pool busy.
  A bounded non-yielding floor per hour would break the loop; I would add it only if measurement 3 shows the need.
- **Killing tests in flight.**
  A yield kills tier 1 tests that spawn git and subprocesses.
  The process-group kill of `Toolchain.command` covers the processes, but leftover temp directories need a check.
- **Timeouts under low priority.**
  A test slowed by a busy machine may pass `redundancy_test_timeout_sec`.
  It then has no entry and is tried again, so a persistently slow test never gets one.
- **A dirty prime blocks its packages**, more widely once T4 checks linked packages.
  Measuring in a private checkout pinned to a commit would remove the dirty and moving-HEAD cases at the price of a second checkout.
- **Footprints that differ by machine.**
  An imported entry can describe a branch the importing host does not take; the environment mark covers platform and toolchain only.
- **Old binaries.**
  A binary that does not know per-entry commits must not read the new shape as today's; T4's format marker is the place to carry that.
- **The largest remaining cost is D5's non-body miss.**
  Narrowing it to the tests whose covered functions reference the changed declaration needs type-aware analysis and its own soundness review.
  It would help upkeep and scoping alike, and it is research, not part of this design.
- **How many entries per test to keep.**
  Two is a guess; the right number depends on how far task worktrees lag the prime.

## Build order

Each step is useful alone, and the first three need nothing from T4, T14 or T15.

1. **Yielding slot and idle trigger.**
   `gateslot` gets the yielding holder and the request file.
   `lym gate testmap` gets an idle mode that takes a slot without waiting, yields on request and writes each finished package.
   `testMapStep` starts a session on a backlog and a quiet pool; the landing count, the package cap and their keys retire.
   The unit is still the package.
   This alone ends the gate waits, drains the first full set and closes E12.
2. **Report timings.**
   The verb's report gains build seconds, run seconds and test count per package: measurement 1.
3. **Parallel test processes** inside the slot, with one profile path per test: measurement 4 guards it.
4. **Per-entry commits and the plan step.**
   The simple function-hash comparison over the package's own directory, the index record with the backlog in seconds,
   measurement of unmatched tests only, and the oldest-first audit in place of `test_map_max_age_days`.
5. **Linked footprints and the shared selector**, with T4 and T14.
   Matching extends to linked packages, T14's comparison replaces the simple one, and upkeep and the gate call the same function.
6. **Gate demand** orders the backlog, with T15's scope report.
7. **More than one entry per test**, once measurement 7 gives the size.
8. **Export and import** across machines.
9. **Research:** narrowing D5's non-body miss.
