# Roll the maps forward: the gate's own selector decides staleness, and a polite lane measures only what changed

## Summary

Today a map is either fresh or rebuilt whole, and "fresh" is judged by a rule (own Go file hashes, age) that neither matches what the consumer needs nor bounds the work.
The consumer the maps exist for (`T14`'s D4) already compares the working tree with the map's own `Commit`, so a map at an older commit is never wrong, only less precise or in a D5 miss.
I propose that one function, the selector `T14` builds, decides both what the gate runs and what upkeep must re-measure: upkeep runs it against main's HEAD instead of the working tree.
Its answer per map is one of three: nothing the map's tests covered changed (re-stamp the map to HEAD with no build and no test), a set of affected tests (re-measure only those and carry the rest), or a miss (rebuild the package whole).
The measuring work runs from a pinned clean checkout at a main commit, in a background worker that takes a gate slot per package job only while another slot stays free, yields at test boundaries when a multi-slot tier waits, and orders its queue by what each map saves the gate.
The age rebuild goes, replaced by a toolchain and module fingerprint and a sampled audit; sharing across machines comes last, once map size is measured.

## Diagnosis

### Where the cost of today's upkeep comes from

Each item cites the code it reads from.

1. **A whole package per stale map.**
   `testMapRun.mapPackage` (`internal/gatecli/testmappackage.go`) builds the package's coverage binary and runs every tiered tier0 and tier1 test of the package, whatever made the map stale.
2. **Serial runs, one process per test.**
   `testMapRun.runTest` runs each test through `Toolchain.RunAlone` with one shared profile path, `filepath.Join(r.tmp, "cover.out")`, so the runs are serial by construction.
3. **Every mapped test is measured, even those no consumer can use.**
   `mapPackage` filters only by tier; `testcover.ScanSpawns` is called by the redundancy step (`internal/gatecli/redundancy.go`, `scanPackage`) and never by testmap.
   A test the spawn scan marks out-of-process or unresolved is never judged (`testcover.notJudgedReason`), and `T15`'s D3 runs it in every coverage-scoped gate anyway, so its measured blocks serve nobody.
4. **A staleness rule far wider than what changes a test's coverage.**
   `testMapRun.stale` (`internal/gatecli/testmap.go`) rebuilds when `maps.Equal(m.Files, files)` fails, and `fileHashes` hashes every `.go` file of the directory, test files included.
   A comment edit, a reformatted file or one changed test therefore rebuilds the whole package.
5. **An age rule that rebuilds everything.**
   `stale` also rebuilds any map older than `test_map_max_age_days` (7, read from `internal/gateslot/template.yaml`), whatever changed.
6. **Work thrown away.**
   `mapPackages` skips a package whose Go file is dirty in the prime (`dirtyGoFile`).
   After the last test, `changedFiles` rereads HEAD, and a package whose files moved under its tests gets no map: the whole package's runs are discarded.
   A failed build overwrites the previous map with an empty one (`writeFailed` sets `m.Blocks, m.Tests = nil, nil`), so a map that was usable becomes a miss.
7. **A gate slot held for the whole call.**
   `holdSlot` takes one slot at the first build and `close` releases it at the end, across up to `--max` packages.
   The hub declares `slots: 3` (read from `_bolt/_lym/config/gate.yaml`; the template says 2), so a refresh removes a third to a half of the gate capacity for its whole run.
8. **One machine's maps.**
   `testcover.Dir` places the maps under the hub's `_bolt/.lym/testcover`, which no sync carries.

### Demand outruns capacity at today's defaults

The numbers below are read from `git log` and config unless marked as estimates.

- Main took 173 first-parent commits from 2026-11-04 to 2026-11-11 (read from `git log --first-parent main`), about 25 landings a day.
- The prime dobby starts a refresh every `test_map_every` (5) landings, each building at most `test_map_batch` (5) packages (`dobbyengine.daemon.testMapStep`, `internal/dobbyengine/template.yaml`): about 25 package builds a day (estimate from the two).
- The distinct directories holding a changed Go file were 88, 48, 206, 50 and 49 on the days 2026-11-05 to 2026-11-09 (read from `git log`); about 50 maps a day go stale by the own-file rule alone.
- The age rule adds about 28 maps a day for the module's roughly 195 test packages (estimate: 195 / 7).

Demand of about 78 rebuilds a day meets capacity of about 25, so the map set never converges, even after the first fill `E12` describes.
Raising the batch only moves the cost into longer slot holds.
The fix has to cut demand, not only add capacity.

### What is essential and what is a choice

Essential to per-test coverage:

- **One process per test.**
  Coverage counters are process-wide (`internal/testcover/doc.go`), so attribution needs it.
  It is also needed for soundness, not only attribution: in a shared process a test that relies on a `sync.Once` or a lazily filled package variable another test filled would record no blocks for that initialization, and a later change there would not select it.
  For the same reason a gate run, which runs a package's tests together, cannot feed the maps.
- **A coverage-instrumented test binary per package and commit.**
- **A passing run.** A failed or timed-out test records nothing, and the consumer then runs it always (`T15` D3), which is safe.

Choices that can be made differently:

- the whole package as the unit of re-measurement;
- serial runs within a package, when most tests already declare themselves safe to run concurrently;
- measuring tests no consumer can use;
- a staleness rule that differs from the consumer's own notion of a usable map;
- the age rebuild;
- building inside the live prime worktree, which brings the dirty skips and the discards;
- holding a slot across a whole batch;
- equal effort for every package, whatever its map saves;
- per-machine storage.

### The fact the design rests on

`T14`'s D4 compares each linked package's files in the working tree with the map's recorded hashes, reads changed files at the map's own `Commit` through `fabricengine.Fabric.CodeFileAt`, and selects every test that covered a block of a changed function.
It states that a change on main since the map was built counts as a change, which can only add tests.
So for the consumer a map's age changes precision and the miss rate, never correctness.
"Fresh" should therefore mean "the selector, run against main's HEAD, finds nothing to re-measure", not "the hashes equal HEAD's and the map is less than a week old".

## Design

### One predicate with two callers

Build the selector once, in `internal/testcover`, as `T14` already plans, with one signature both callers use:
given a map and a target tree, return either the affected tests or a miss reason.

- The coverage gate scope (`T15`) calls it with the working tree as target, as `T14` designs.
- Upkeep calls it with main's HEAD as target.

Its rules are `T14`'s D4 and D5, with three additions that upkeep needs and that I recommend the gate adopt too, since each only narrows a miss into a selection by the same argument as D4:

1. **Own non-test files are compared like a linked package's.**
   D5 makes any change to the importing package's own Go files a miss.
   Its non-test files are instrumented exactly like a linked package's, so the function-level comparison of D4 holds for them unchanged.
2. **Test files are compared per test function.**
   A `_test.go` file is not instrumented, so coverage cannot see a helper a test calls.
   A test is affected when its `Test` function's text changed (`MapTest.FuncHash` against `testcover.ParseTestFuncs`) or it is new.
   Any other change to a test file (a helper, `TestMain`, an `init`, a package-level declaration) is a miss for that package.
   A test file is compared with every top-level `Test` function's text removed, so the two rules never overlap.
3. **An environment fingerprint.**
   The map records the Go toolchain version, the hashes of `go.mod` and `go.sum`, the mapped tiers' rendered env (`renderTierEnv`), and the map format marker `T4`'s D2 adds.
   A difference is a miss: a toolchain bump can move the instrumenter's block boundaries, and an external module upgrade changes behaviour with no in-module file changed.
   Today nothing but the age limit catches either.

Non-Go files follow D5 as `T4`'s D2 records them: a changed file of a linked package's recorded set that `inert_paths` does not exempt is a miss for every map that links the package.
Dependency changes need no rule of their own: with D2's cross-package maps a dependency is a linked package, and the predicate compares it like any other.

### The three outcomes and what upkeep does with each

Let `C` be the map's `Commit` and `H` main's HEAD.

**Current: no affected test, no miss.**
Upkeep re-stamps the map: `Commit` becomes `H`, the recorded file hashes become `H`'s, every test's block set is carried verbatim, and the block table keeps only the keys some test covers.
No build, no test run and no slot.
This is the common case after most landings for most maps: a map that links a changed package needs work only if one of its tests covered a changed function.

**Affected tests.**
Upkeep builds the package's coverage binary at `H`, runs the affected tests and the new ones alone, keys their blocks against `H`'s profile, carries every other test, drops deleted tests, and writes the map at `H`.

**Miss.**
Upkeep rebuilds the package whole, as today.

### Why carrying a test is correct

Take a test `T` not affected between `C` and `H`, and assume `T` is deterministic for fixed code, data and environment.

- With no miss, the only differences between `C` and `H` that `T`'s process can observe are function declarations that differ.
  Non-function declarations, compiler directives, method sets, `init` counts, imports, non-Go files and the environment are misses under D5 and the fingerprint.
- `T`'s execution at `H` follows its execution at `C` step for step until it first executes code that differs.
  That code lies in a changed function, and the step that reaches it executes a block of it.
  `T` would then have covered that block at `C`, which is exactly what makes it affected.
  So an unaffected `T` executes the same blocks at `H`.
- Those blocks lie in unchanged functions, and a block key is the file, the enclosing function, the hash of the block's text and an ordinal counted among blocks equal in the first three parts (`testcover.BlockKey`).
  An unchanged function therefore keeps every block key.
  Functions that share a key name, such as several `init` functions of one file, count as changed together (D4), and a function moved to another file changes its key's file and counts as removed, so neither breaks the argument.

The block keys `T` carries are therefore its keys at `H`.
The same argument justifies the re-stamp, which is the case with no affected test at all.

Two checks guard the predicate in practice:

- **Self-check on every measured job.**
  A profile lists every block of the instrumented packages, covered or not (`fillMapTests` relies on that).
  When upkeep builds at `H` for a package, every carried key must appear in `H`'s block table.
  A carried key that does not makes upkeep re-measure that test and log the event; a non-zero rate of such events is a predicate bug.
- **Sampled audit.**
  On idle time, upkeep re-measures a few carried tests at random and compares.
  A fresh run that covers a block the carried set lacks is evidence of a nondeterministic test or a predicate gap; upkeep logs it and rebuilds that package whole.
  The audit replaces the age rebuild with a measured error rate.

### The unit of work

The unit is a package job at one commit: one coverage build and the runs of the tests the predicate names, or every test on a miss.
Within a job the runs are independent processes, so the job can pause at any test boundary and resume, keeping its binary and finished profiles in its scratch directory.
The map is written once, atomically, when the job ends (`testcover.WriteMap`), so a reader never sees a half-rolled map.

A job measures only tests a consumer can use:

- A test the spawn scan marks out-of-process or unresolved is recorded with its verdict, as `T4`'s D2 already requires, and is never run.
- A test whose top-level body opens with `t.Parallel()` runs concurrently with the other such tests of its package, up to the slot's tier0 parallel, each in its own process with its own profile path.
  The test declares itself safe to run beside its package's other parallel tests in one process, so separate processes are no riskier.
  Other tests run serially, as today.

A failed build at `H` leaves the map at `C` as it is and records the failure beside it, instead of `writeFailed`'s empty map.
The map at `C` stays valid for the selector, so a broken commit costs no precision.

### Where the measuring runs

Jobs run in a checkout of their own, detached at the main commit `H` the plan chose, which only the worker writes to.
A pinned clean tree removes the prime's dirty skip, makes `changedFiles`' discard impossible, and lets the prime's operator work undisturbed.
The checkout is created and moved through `internal/fabricengine`, per PATTERN-fabric-boundary.
Every gate build keeps `-trimpath` (PATTERN-gate-slot), so the checkout shares the build cache with every worktree.
`T4`'s two fresh dirty reads stay as assertions; in a checkout nothing else writes to, they always pass.

### Who starts the work, when, and what it holds

- **Trigger.**
  The prime dobby's `testMapStep` starts the worker when main's HEAD moved since the worker's last plan, or when the last worker's exit record reports queued jobs, and no worker is live (`testMapRunning`, unchanged).
  This replaces the landing count and subsumes `E12`: a backlog drains on its own.
- **Plan.**
  The worker first runs the predicate for every map at `H`, holding no slot.
  It re-stamps every current map at once and queues the rest.
  The plan reads files at `C` and `H` through `CodeFileAt` and parses only changed files; I estimate it at seconds per landing.
  To keep it cheap once `T4`'s maps grow, each map can store, beside its tests, the set of functions its tests cover per linked package, so the plan intersects function sets and never loads block tables.
- **Order.**
  Each job's priority is the gate time its map saves per day divided by its cost.
  The saving comes from the gate run records (`gateslot.RunRecord`'s `PackageLaunch` elapsed per package and tier, already written), and later from `T15`'s per-package scope report (D7), whose `miss:` lines name exactly the packages a missing map cost a gate.
  The cost is the recorded `Seconds` of the tests to run plus a build.
  The records show where the value lies: in the full tier1 run recorded at `20261102T160144Z`, the ten slowest packages carry 74 percent of tier1's package time and the thirty slowest 91 percent, while 143 of 186 packages take under 3 seconds (read from the run record).
  `internal/fabricengine` alone took 533 of 1571 seconds there.
  A package whose whole tiers take less than a selected invocation's launch gains nothing from a map and gets no upkeep job for scoping; the gate runs it whole, which a missing map already does.
- **Slot.**
  The worker takes one slot per package job and releases it when the job ends.
  It takes a slot only while at least one other slot stays free, read from `Pool.Holders` and `Limits.Slots`, so an arriving single-slot gate never waits on upkeep.
  Between two test runs it checks `Pool.Draining`; while a multi-slot tier waits on the drain lock, it releases its slot, pauses the job, and resumes when the condition holds again.
  Its child processes run at the lowest CPU and IO priority.
  On a machine where no slot has been held for a while, it also runs the audit and the whole rebuilds the queue ranks low.
- **Locks.**
  The per-package build lock (`testMapBuildLockPath`) and the singleton worker record (`runproc.RoleTestMap`) stay as they are.

`lym gate testmap <pkg>` by hand keeps working: it queues the named packages at the front and forces a whole rebuild.

### Where the maps live, and sharing

The maps stay under `_bolt/.lym/testcover` on each machine for now.

They are shareable in principle: a map is a statement about one commit, valid for any tree that holds that commit, on any machine of the same platform and toolchain the fingerprint records.
When sharing is worth it, I propose a single writer, the machine that owns the prime (PATTERN-owner-machine), publishing one file per package to a dedicated ref on origin, newest commit winning per package.
Other machines fetch it at `lym adopt` and when their store is empty.
Not the Bolt: `pullBolt` runs on every dobby tick, and multi-megabyte map files would slow the board for every reader.
Not the records (weft) branch: it is per pair, and the maps are per hub.

`T4`'s D2 grows every map by the blocks of every linked package.
If size blocks sharing, the block tables can be stored once per file content hash, since a file's block keys depend only on its content and the toolchain, and each map then holds per file hash the indexes its tests cover.

### What a consumer does with a missing or stale map

- **Missing, unreadable, failed or fingerprint-mismatched map:** a miss; the package runs whole for the tier (`T14` D5).
- **Map at an older commit:** the selector compares the working tree with the map's own `Commit`, so changes on main since then select more tests or miss, never fewer.

Why that is safe:

- The carry argument above applies unchanged to the gate's comparison, so a map at `C` selects soundly for any tree descended from `C`; age costs precision, not soundness.
- The residual gaps, a nondeterministic test path and a child process the spawn scan does not see (`T15` D3's Bound), exist the same for a map built a minute ago, so a fresh map is no safer than a rolled one.
- The `finalize` landing gate runs scope `all` before every landing (`T15` D1's Bound), so a test a map wrongly leaves out is found before the change lands, only later than it could have been.

A lagging map costs gate time and never lets a broken change land, so upkeep may run lazily and in value order.
The redundancy report (`internal/gatecli/redundancy.go`) is advisory and already carries only the map tests whose `FuncHash` still matches (`judgeRuns`), so a rolled map serves it the same.

## The six starting ideas

1. **Update a map in part: adopt, with a change of grain.**
   The affected set is the selector's own (function level, D4) rather than the note's block level.
   A changed function can change control flow in text no block holds, such as a signature, a named result or a `switch` case expression, and the function level is the grain the gate selects by, so the two never disagree.
   Test files need the per-function rule above, since helpers are invisible to coverage.
   Beyond the note, a map with no affected test is re-stamped with no build at all, and that case, not the partial re-run, is where most of the saving lies.
2. **A stale map gives a slower gate, never a wrong answer: adopt.**
   It holds because D4 compares from the map's own `Commit` and `finalize` runs everything; the argument is under "What a consumer does".
3. **Build when idle: adapt.**
   At about 25 landings a day an idle machine may never come during working hours, so waiting for idleness alone starves the queue.
   The worker runs whenever one slot stays free beside it, yields to a waiting multi-slot tier at test boundaries, and keeps idle time for the audit and the low-ranked whole rebuilds.
4. **Drop the weekly age rebuild: adopt, and replace it.**
   The age rule stood in for three things: dependency drift, which the predicate now compares directly; environment drift, which the fingerprint catches; and nondeterministic drift, which the sampled audit measures.
   Until the audit shows a low rate, keep a long age backstop (I suggest 30 days, my estimate) that queues a whole rebuild at the lowest rank, never a forced one.
5. **Share maps across machines: adapt, and defer.**
   Sharing is sound for one platform and toolchain, but the Bolt is the wrong carrier.
   A single writer and a dedicated ref fit, and the decision should wait for two measurements: map size under D2, and how often work moves between machines.
6. **Only gate scoping needs fresh maps: adopt, and push it further.**
   The redundancy sweep (`E4`) asks for whole rebuilds of the packages it is about to sweep, through `lym gate testmap <pkg>`, at sweep time.
   Within gate scoping, freshness should also follow value: maps of packages whose whole tiers cost almost nothing need no upkeep.

## Cost and gain

### What gets cheaper, by reasoning

- **Comment, formatting and import-order edits:** today a whole rebuild of the package; proposed nothing, since D4 compares declarations ignoring comments and formatting.
- **An edit to one test:** today a whole rebuild; proposed that test alone.
- **A body change to a function `f` of package `L`:** today a whole rebuild of `L` only, while every importer's dependency drift waits for the age limit; proposed a re-stamp of every map whose tests did not cover `f`, and re-runs of only the tests that did.
- **Out-of-process and unresolved tests:** never measured.
  I counted 31 tier1 test functions in the files that import `internal/testkit/lymbin` (read by `grep`); tests reaching a spawn through a testkit add to that, so the share is real but modest (estimate).
- **Wall time of a package job:** divided by up to the tier0 parallel for the tests that open with `t.Parallel()`, which PATTERN-test-speed makes the norm.
  CPU time stays the same.
- **The age rule:** about 28 whole rebuilds a day removed (estimate above).
- **Discarded and skipped work:** none, from the pinned checkout.
- **Slot pressure:** from one slot held across up to five whole packages to one slot per job, taken only while another stays free and given back to a waiting multi-slot tier at the next test boundary.

The whole-set cost stays for the first fill and for misses: about 5300 tiered tier0 and tier1 test functions (4424 and 903 `tiergate.Require` calls, read by `grep`), each one process.

### What I could not know without measuring

1. **How often a landing misses rather than rolls.**
   The Sketch's Constraints read from main's history that at least 95 of 118 commits touching non-test Go held a D5 non-body change, and that 15 of 26 commits touching `internal/fabricengine` still miss after D5's exemptions.
   A non-body change misses only the maps that link the changed package, but `internal/fabricengine` alone is imported directly by 63 directories (read by `grep`).
   If misses dominate, whole rebuilds dominate upkeep, and the largest lever is the selector narrowing misses (see Risks).
   **Measurement:** build one full map set at a commit `C0`, then replay main's first-parent commits from `C0` with the predicate alone, no builds.
   Per commit and map it reports current, affected tests with their recorded seconds, or miss with its reason.
   This gives the upkeep workload in test-seconds per day, split by outcome.
2. **Coverage build cost under D2.**
   With `-coverpkg` naming every linked package, I do not know whether an instrumented package compiled for one test binary is reused from the build cache for another.
   If not, every roll-forward job recompiles its linked set.
   **Measurement:** time `BuildBinary` for a few packages with D2's coverpkg, cold and then warm, and once more after a one-function change to a widely linked package.
   If reuse fails, a module-wide coverpkg for every package may make the instrumented compiles identical and cacheable; the same timing settles it.
3. **Per-process overhead of a coverage binary.**
   In the full tier0 run recorded at `20261111T122418Z`, package launch time was close to package elapsed time for the slowest packages (read from the run record), so launch dominates tier0.
   Part of that is the host scanner holding each unseen binary (`gateslot.PackageLaunch` doc), which a re-exec of the same binary may not pay again.
   **Measurement:** time the second and later `RunAlone` of a trivial passing test under one binary.
4. **Carry error rate.** **Measurement:** the sampled audit, plus one whole rebuild of a sample of rolled maps compared key for key.
5. **Map size under D2.** **Measurement:** the file sizes of a full set of D2 maps.

## Risks and open questions

- **Misses may dominate.**
  By the Sketch's count, most commits carry a non-body change.
  The predicate can narrow many of them into selections: treat a changed non-function declaration as changing every function and declaration whose text names it (qualified from other packages, by bare name inside its own package and through a dot import), followed transitively.
  A const, a type, a struct field or a plain var then selects the tests that covered code naming it.
  A value of a changed type is always made by code that names the type or a type that contains it, so a test that holds one covered that code; when it is made at init, every test covers the init blocks.
  The misses that stay are a var with an initializer, a method set change, a `//go:` line, an added `init`, an import change and a non-Go file.
  That argument needs its own review against reflection by name and `unsafe`, and it changes `T14`'s design, which is still a draft waiting on this research.
- **Nondeterministic tests.**
  A test whose path depends on timing, scheduling or map order may cover a function in one run and not in another, so a carried set can miss a path.
  A full rebuild has the same weakness, since one run sees one path.
  The audit measures the rate.
  If it is not negligible, a re-measured test can keep the union of its fresh blocks and its old blocks in unchanged functions, which over-selects and never under-selects.
- **A predicate bug is silent.**
  Upkeep and the gate would share it.
  The self-check on every measured job and the audit are the only detectors; the design should treat a non-zero self-check rate as an alarm, not a log line.
- **Starvation.**
  On a hub that always holds all but one slot, the polite rule never admits upkeep.
  Value order puts the important maps first, and the idle lane catches up overnight, but a hub with `slots: 2` (the template) admits upkeep only when both slots are free.
  An open question is whether such hubs should allow upkeep the second slot outside working hours.
- **The pinned checkout.**
  It costs a second checkout's disk and a fabric-boundary verb to create and move it, and `testMapLocation`'s rule that testmap runs only in the prime has to become "runs at a main commit".
- **Shared code with `T14`.**
  The design depends on one predicate serving both callers.
  If `T14` lands its selector with D5's strict own-file miss first, upkeep can still roll a package's own body changes forward, because advancing the map clears the gate's own-file miss; the gate then gains nothing from the finer rule until it adopts it.
- **Re-stamp churn.**
  Every landing rewrites many map files.
  The writes are atomic and small next to a rebuild, but a gate that reads a map mid-landing may see either commit, which is safe either way.
- **Map platform.**
  A map built on another platform may record paths a test takes only there.
  The fingerprint should hold `GOOS` and `GOARCH`, and a reader on another platform should treat the map as a miss until sharing is decided.

## Build order

Each step is useful alone; later steps assume the earlier ones.

1. **Polite, drainable upkeep.**
   Take the slot per package rather than per call, only while another slot stays free, and release it to a waiting multi-slot tier between test runs (`Pool.Draining`).
   The dobby restarts the worker while the exit record reports packages left unbuilt (`E12`).
   This answers the slot complaint with no change to what a map holds.
2. **Measure only usable tests, and run parallel-safe ones concurrently.**
   Record the spawn verdict per test (part of `T4`'s D2) and skip out-of-process and unresolved tests.
   Give each run its own profile path and run the tests that open with `t.Parallel()` up to the tier0 parallel.
3. **Keep the old map on a failed build.**
   Record the failure beside the map instead of replacing it.
4. **A pinned checkout at a main commit** for every testmap build, through `internal/fabricengine`.
5. **The shared predicate** in `internal/testcover`: `T14`'s D4 and D5 plus the own-package function rule, the test-file rule and the environment fingerprint, written over `T4`'s linked-package map shape.
   Staleness becomes the predicate's verdict at main's HEAD.
   The age limit drops to a long, low-ranked backstop.
6. **Re-stamp** maps with no affected test, with no build.
7. **Roll forward** maps with affected tests, with the self-check.
8. **Value order** from the gate run records, later from `T15`'s scope report; skip upkeep for maps that save nothing.
9. **Measure:** the history replay, the build-cache timing, the re-exec overhead, the map sizes.
   Start the sampled audit on idle time, and set or remove the age backstop from its rate.
10. **Narrow the misses** with the declaration-reference closure, if the replay shows misses dominate.
11. **Share maps** through a dedicated ref with a single writer, if the measured size allows it and machine moves are frequent enough to pay for it.
