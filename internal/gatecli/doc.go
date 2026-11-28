// Package gatecli provides the cobra command tree for `lym gate`.
// Its verb `test` is the sanctioned route for running the hub's test tiers.
// Its verb `status` shows who holds the hub's gate slots, and its verb `report` totals the run records per gate key.
// Its verb `card-verify` runs a plan card's `**Verify:**` line.
// Its verb `red-first` runs the tests the worktree's diff adds or changes on the base's code.
// It reports whether any of them failed there.
//
// The module is not standalone-capable and does not go through internal/cliwire.
// It has no target directory, state, plan or stencils to wire, and it resolves hub membership itself.
//
// # Verb
//
// The form is `lym gate test [-C <dir>] [--gate <key>] [--tier N]... [--upto N] [--card <id>]... [--main-packages] [--base <rev>] [--all] [--guards] [--stop-on-fail] [--no-carry] [--copies N] [--wait] [<targets>...] [-- <args>]`.
// It runs the tiers the hub declares in gate.yaml in ascending tier number.
// It then runs the vet step and the guard step below when they are planned.
// A run that fails does not stop the call: every later tier, vet and guard run still runs,
// so a failure the change under test did not cause hides no later check.
// `--stop-on-fail` ends the call at its first failed run instead, naming the failed run and every run it skipped.
// A run that crashed its test runner in its run and its rerun judged no code, so it stops nothing.
// Nor does a run the memory guard stopped in its run and its rerun for a memory limit other load filled.
// `--no-carry` treats every tier of the call as a `carry: false` tier.
// A call over an unchanged tree then still runs its tests.
// When a run failed, the call prints the failure report, then closes with one stderr line naming each failed run.
// The line names a vet run as `vet tierN`, a guard run as `guard tierN` and a lint step as `lint <name>`.
//
// `--tier` names a tier and may repeat; the default is tier0 when neither `--tier` nor `--upto` is given.
// A tier another listed tier covers is dropped and named as covered on stderr.
// `--upto N` adds every declared tier numbered 0 through N, skipping a tier marked `manual` unless `--tier` names it.
// A manual tier runs only when `--tier` names it, so a tier that spends tokens never starts from a range.
// `--tags` stays a registered flag only to be refused, naming `--tier`; a tier's own command sets its build tags.
// The words after `--` fill each tier's `{args}`.
//
// `--gate <key>` runs the tiers and the scope the `gate.gates` key of that name holds.
// It goes with none of `--tier`, `--upto`, `--all` and `--base`.
// A key the config does not hold, an empty tier list and an undeclared tier are refused, naming `lym config gate`.
// In a hub that declares no tier at all, a `--gate` call runs the key's lints and no tier.
// A key of scope `card` takes the `--card` ids and is refused without one.
// A key of scope `all` is the `--all` form, and `diff` is the computed form.
// A key of scope `marked` runs only the tests marked for it, as Held tests below says.
// Explicit `<targets>` beside `--gate` replace the key's scope, bar a key of scope `marked`, whose call they narrow.
// Each run record carries the key as `gate_key`.
// `--gate` opens no flag combination the manual flags lack, except the key's lint steps.
//
// `--copies N` runs N copies of one tier side by side, each the tier's own command over the same explicit targets.
// It needs `--gate` and explicit targets, the key must name one tier below `gateslot.BilledTier`,
// and it is refused inside a slot an enclosing gate run holds.
// The call takes one lease of N times the tier's slots, and a count above the pool's slots runs one copy per share held.
// Each copy runs under the tier's own counter, timeout and guards.
// A failed copy is labelled `copy <i>` in the report.
// A copy is a tier run of its own, never a raw test binary, and the green record carries none.
//
// Every tier run prints one started line, built by `gateslot.FormatStartedLine`;
// `gateslot.ParseStartedLines` is its sole parser.
//
// Under the `--card` scope form, when no run failed, the verb closes with a stderr line.
// The line names each declared tier that no listed tier is or covers, manual tiers included;
// a card gate that runs tier0 is therefore never read as fully green.
// A vet or guard run does not count as running a tier.
// The line is output only and changes no exit code or record.
//
// A tier run that fails without timing out is rerun once after a delay in three cases.
// The first is an output that holds a resource-shortage marker.
// The markers are `resource temporarily unavailable`, `failed to create new OS thread` and `cannot fork`.
// The second is a run that reports no failing test and exits 128 plus a crash signal number.
// That code is what a shell reports when a runtime aborted and crashed its test runner.
// The third is a run whose only stops were the memory guard's slice-pressure kills.
// Such a run has no failing test and no failed package but those whose test binary was killed.
// The rerun's result is the tier's.
// The first run's record carries `runner_crash` for a crash; the rerun's record carries `rerun`.
// The record of a run the guard stopped so, the first or the rerun, carries `memory_pressure`.
// A call closes with `gateslot.RunnerCrashLine` when every failed run of it crashed so.
// It closes with `gateslot.MemoryPressureLine` instead when the guard stopped one of them so twice,
// and every other failed run crashed or was stopped so too.
// Either needs no failing test, failed build or lint finding beside those runs;
// the line lets a site tell a runtime's crash or a full memory limit from a failing test.
// Bound: one rerun per tier run, and a test that aborts its own runner is reported as a crash, not a failing test.
// The build step is not rerun.
//
// A tier is one shell command that may hold these placeholders:
// `{targets}`, `{test_targets}`, `{parallel}`, `{args}`, `{filter}` and `{test_timeout}`.
// Each target and arg is quoted for the platform shell.
// `{filter}` renders as `--filter ` and the tier's filter expression, or as nothing when there is none.
// A narrowed dotnet invocation combines the tier's filter with the class terms the scoper chose.
// A `--filter` among the words after `--` leaves `{args}` and is ANDed into `{filter}` too.
// The reason is that `dotnet test` refuses a second `--filter`.
// A command without `{filter}` renders no filter whatever terms it is given, and keeps a `--filter` in its `{args}`.
// Planning refuses a `--filter` after `--` for a command that holds a `--filter` of its own.
// That refusal names the tier's `gate.tiers.<tier>.filter` key.
// It also refuses words after `--` that end in `--filter`, which has no expression to take.
// `{test_targets}` stands where the targets reach `go test`.
// It renders each invocation's targets narrowed to the packages that hold a test function the tier runs,
// so a pass starts no test binary that would find no test of its tier.
// A test function opening with `tiergate.Require(t, N)` counts for tier N alone, and any other for every tier.
// An invocation with no package left runs no command and says so on stderr.
// Bound: a tier whose `LYM_TEST_TIER` names a range runs the tests of tiers it does not `covers`,
// and a package holding only those is left out.
//
// The command runs through a tied child in the run directory, the tier's `dir` or the worktree's anchor.
// The environment is this process's, less the strand-name and trace-id variables.
// An inherited `gateslot.PrebuiltLymEnv` is dropped too.
// The `env` config module (`internal/envconfig`) then replaces an inherited value.
// `proc.BuildServerOffEnv` replaces an env module value, and the tier's `env` replaces an env module value.
// A tier run first prints and logs one line naming each `env` variable the session sets to another value.
// It names the tier's value only, never the session's, which may be a secret.
// A tier with `prebuilt_lym` first builds `lym` inside its slots and timeout, where the worktree root holds `cmd/lym`.
// It then exports `gateslot.PrebuiltLymEnv`.
// It builds only when a test it runs may use the binary.
// A command that takes no targets runs whole, so it always builds.
// For one that takes targets, that is so when a targeted package's test file imports `internal/testkit/lymbin`.
// It is also so when the kit's own directory is targeted, or when a target is a `...` pattern or no directory.
// The whole set's `./...` therefore builds it, and an invocation with no target does too.
// The Testkit Invariant keeps every other file from importing the kit,
// so a test reaches the binary through no other route.
// Inside a hub the binary is copied into the pool's store under a name from its content hash.
// Go therefore keeps a re-run on an unchanged tree cached.
//
// # Scope
//
// A call takes one scope form:
// explicit `<targets>`, `--all`, `--card <id>`, `--base <rev>` or, with none of them, the computed form.
// Explicit targets replace each run's `{targets}` verbatim and run in the `-C` directory, else in the tier's `dir`.
// `--all` sets `{targets}` to the tier's `all_targets`.
// A command holding neither `{targets}` nor `{test_targets}` runs once, whole, under any scope, and says so on stderr.
// Explicit targets that no listed tier's command takes are refused before any slot.
// Beside a `--gate` key of scope `marked`, explicit targets instead narrow its guard step, as Held tests below says.
// Flag-shape refusals need no config and run before hub resolution:
// `--tags`; a negative `--upto`; `-C` without explicit targets; `--gate` beside `--tier`, `--upto`, `--all` or `--base`;
// more than one scope form; and `--main-packages` without `--card`.
// A run agent is a call whose environment names a strand through `agentname.StrandNameEnv`.
// Its call is refused too when `--tier` or a reaching `--upto` lists `gateslot.TmuxTier`,
// and the call names no explicit target free of `...`; a filter after `--` alone does not open the call.
// The refusal names `lym gate test --tier <n> <package> -- <filter>` as the way forward.
// A `--gate` key that lists the tier is not refused, since `gate.yaml` sets its scope.
//
// # Scoper
//
// `gate.scoper` decides what the computed, `--base` and `--card` forms mean.
// The computed form takes its base per listed tier:
// the newest ancestor of HEAD the green record holds whole-green for the tier under its current fingerprint;
// else the merge base of HEAD with the parent branch.
// A narrowed green never becomes a base.
// A tier with neither base, such as the prime on `main`, runs whole.
// A dirty worktree reads no record, so it takes the merge base.
// `--base` names one base for every tier.
// `--card`, repeatable, reads the plan and takes the packages of the named cards.
// An id is a card's number or its `N-slug`; an unknown id and a worktree with no plan are refused.
//
// Under the `none` scoper each scoped form is whole, so a tier takes its `all_targets`, and no guard step is planned.
// Under the `go` scoper a computed or `--base` scope is `impactset.DeriveTargets` of the base-to-working-tree diff.
// It falls back to whole wherever the derivation does, naming the reason on stderr.
// A `--card` scope is the cards' packages and, per module, every package whose build or tests reach one of them.
// So a test outside the cards that asserts on what they change runs in the same call;
// a module `go list` cannot list keeps the cards' packages alone, which the call names on stderr.
// With `--main-packages` it also holds every `package main` package of tier0's module.
// Under `--gate card` every listed tier runs the cards' packages alone, and the call plans no guard step.
// It says so on stderr, since a `--gate batch` call over the cards runs the dependent packages and the guard step.
// Under `--gate batch` only tier0, and a listed tier that covers it, run that scope.
// Each other listed tier runs the cards' packages alone, with the main packages when asked,
// and the call names it on stderr.
// A tier that runs whole is neither narrowed nor named there.
// The gates that run the whole tree run the slower tests of the dependent packages once per plan.
// A narrower scope runs each listed tier once per module, in that module's directory.
// One slot hold and one `gate.tiers.<tier>.timeout_min` cover every invocation.
// An empty scope, such as a diff of Markdown files only, runs no listed tier's command, bar a tier with `carry: false`:
// each other listed tier passes with zero discovered and takes no slot, and only the guard step runs.
// A tier with `carry: false` runs whole over its `all_targets` under every scope form but explicit targets.
// The import graph cannot see everything a narrower run skips, such as `go build ./...`;
// the whole runs at `final` and `landing` cover that.
//
// Under the `dotnet` scoper the forms are derived through `dotnetimpact` over test projects and their classes.
// A narrowed share runs each selected test project once in the tier's `dir`.
// The project path is `{targets}` and its class terms render into `{filter}`.
// A derivation that falls back gives its tiers a whole share and prints the reason.
// When no run of the call reaches a selected project, the call closes with a stderr line per project.
// The line says the pass is not green for that project; it is output only and changes no exit code or record.
// An empty scope is no invocation when nothing testable changed,
// and a tier with `shared_build` runs only its build when no test class is named.
// Any other tier runs whole, so a compile break still fails the gate.
// The scope plans no guard step and no vet step, and `--guards` is refused, naming the scoper.
// A narrowed project invocation is partial and writes no green entry.
// The class closure misses only references that never spell a changed type's name;
// the gates that run whole, such as `parent-align` and `finalize`, catch those.
//
// Once the config is loaded, planning refuses these before any slot, naming `lym config gate`:
// words after `--` for a tier whose command lacks `{args}`;
// explicit targets no listed tier's command takes, bar beside a `--gate` key of scope `marked`;
// an undeclared tier; a hub with no `tier0`, naming `gate.tiers.tier0` in full;
// a `--filter` after `--` for a command that holds a `--filter` of its own and no `{filter}`;
// a `covers` entry naming an undeclared tier; and a guard group whose tier the hub does not declare.
// Every such refusal closes with a clause for a session that is not the prime:
// it takes none of the steps, reports status: FAILED and leaves them to the orch.
//
// # Inert paths
//
// A hub's `gate.inert_paths` lists the patterns of files no test reads, and `gateslot.Config.Inert` is the one matcher.
// Under the `go` and `none` scopers a computed or `--base` scope gets the empty scope when its diff is inert-only.
// A diff is inert-only when it matches only inert paths, untracked files included;
// an empty diff is not inert-only, and a read error holds the diff not inert.
// Under `dotnet` the derivation drops inert paths itself.
// Under every scoper a `--gate` key of scope `all` checks the diff from the merge base with the parent branch,
// so the Finalize gate of an inert-only change runs no tier.
// A `--card` scope is not narrowed by `gate.inert_paths`.
// A whole invocation of a tier that carries is carried over inert-only history.
// This holds when the newest whole entry of an ancestor has an inert-only diff to the working tree;
// the entry is taken under the tier's fingerprint and module, and a whole entry is written for the current tree.
// The carry runs only on a clean tree, and the inert list is part of the fingerprint.
// Bound: a test that reads an inert file is skipped when only that file changes, so list only files no test reads.
//
// # Build step
//
// A hub's `gate.build` declares a shell command, run once per call, that the tiers with `shared_build: true` depend on.
// The first such tier in plan order whose run takes slots runs it inside its slots, before its own command.
// A call whose opted-in tiers are all carried or empty runs no build; a `carry: false` tier runs it before its command.
// A `shared_build` tier with no invocation takes its slots only to run the build,
// so a compile break still fails the gate.
// The command runs in the build's `dir` with the call's environment plus the build's `env`.
// The words after `--` never reach it.
// The build's own `gate.build.timeout_min` counts from the hold.
// A timeout kills the tree, exits 1 and prints `gateslot.BuildTimeoutLine`.
// The build writes a run record with `build` true and scope form `build`, and prints `gateslot.BuildSummaryLine`.
// A failed build is a failed run counted before its tier, and every tier that opted in from there on runs no command:
// its record has outcome failed, the build's exit code and `blocked_by` the build's log.
// Tiers that do not opt in still run.
// `lym gate report` totals build records apart, in `build_runs` and `build_seconds`.
// Bound: a `shared_build` tier whose command lacks `--no-build` builds again inside its own run; lym does not read it.
//
// # Vet step
//
// Under the `go` scoper a `--card` scope also plans vet runs, after the listed tiers and before the guard step.
// Each declared tier whose tag `tierN` adds a `.go` file to a card-scope package gets a `go vet -tags tierN` run.
// The run covers the card scope's packages, once per module.
// A listed tier and a tier a listed tier covers get none.
// So a card that breaks the compile under another tier's tag fails at its own gate.
// The run holds one slot, takes part in the green record under its own fingerprint, and stops no later run.
// Bound: it does not vet a break visible only under a combination of tags.
// Nor does it vet one visible only under a tag that adds no file to a package of the card scope;
// the batch gate finds both.
//
// # Guard step
//
// The verb runs the guard step after the listed tiers when given `--guards`.
// Under the `go` scoper it also runs after any computed, `--base` or `--card` scope, bar a `--gate card` call's.
// It covers the module's `//lym:guard` tests outside the scope, so a narrower gate keeps full guard coverage.
// It also covers a scope package's guard test whose tier no listed tier is or covers, which the listed runs skip:
// under `--card` for the cards' scope, and under a computed or `--base` scope for each base's impacted set and tiers.
// Under `--card` the step also takes a dependent package's guard test.
// It does so when the listed run of the test's tier holds the cards' packages alone.
// A guard test's tier comes from its file's `tierN` build tag, a file naming none counting as `tier0`.
// The step plans one run per tier holding guard tests, ascending.
// Each run is the tier's command with the group's packages as `{targets}`;
// `-run '^(<names>)$'` follows any words from `--`, so the guard's own pattern wins.
// It acquires its slots after the last listed tier and vet run released theirs, and a failed run stops no later run.
// A group is skipped when the call ran its tier, or a tier covering it, whole.
// A group whose tier the green record holds green carries.
// A group is refused before any slot when its tier is undeclared.
// It is also refused when its command lacks `{args}`, or lacks both `{targets}` and `{test_targets}`.
// A scan error refuses the call before any slot, naming the file, the line when there is one and a way forward.
// `--guards` under the `none` or `dotnet` scoper is refused, since the scan is Go's.
// A module with no marked test runs no guard step and logs so.
//
// # Watched tests
//
// A top-level test marked `//lym:watch <pattern>...` guards only the files its patterns name.
// An example is a cross-compile that matters only when a `_linux.go` file changes.
// Under the `go` scoper a narrowed computed or `--base` share and a `--card` scope skip such a test;
// it is skipped when no changed file matches a pattern and its own file did not change.
// A `--card` scope reads its changes as the diff from the merge base with the parent branch, and skips none without one.
// A listed tier's run gets `-skip '^(<names>)$'` after the words from `--`, and the guard step leaves such a test out.
// A call whose words after `--` carry a filter flag, and a tier whose command holds no `{args}`, get no skip.
// A whole scope and explicit targets skip nothing.
// A misplaced or malformed watch marker refuses the call, as a guard scan error does.
// Bound: `-skip` matches by name in every package of the run, so a test of the same name elsewhere is skipped too.
//
// # Held tests
//
// A top-level test marked `//lym:only <key>...` runs only in a `--gate` call whose key the marker names,
// so a slow whole-repository check runs at the gate key that needs it and leaves the others.
// Under the `go` scoper every `--gate` call scans the module of its guard step for the markers.
// A call holds back each marked test whose marker does not name its key, under every scope form.
// Each listed tier's run gets it in its `-skip` after the words from `--`, beside the watched tests.
// A line names it, and the guard step leaves it out.
// A call whose words after `--` carry a filter flag, and a tier whose command holds no `{args}`, hold nothing back.
// A call without `--gate` holds nothing back, so a hand run reaches every test.
// A marker that names a key `gate.gates` does not hold refuses the call, naming the test and the key.
// A directory with no `go.mod` holds nothing back.
// A module whose packages `go list` cannot list holds nothing back and says so on stderr, so its tiers report the break.
//
// A key of scope `marked` runs no package in its listed tiers, under any flag or tier setting.
// Its guard step runs every test marked for the key under its file's tier, the guard step's way.
// The key's tiers play no part in that, so no hub sets them to reach its marked tests, except for a manual tier:
// its marked tests run only when the key lists that tier or one that covers it, and any other call is refused.
// A marked test of a tier the hub does not declare refuses the call, as a guard test of such a tier does.
// Explicit targets narrow the call to the marked tests in the packages they name, each still under its file's tier.
// A target names a package by its path relative to the scanned module.
// A target ending in `/...` names that package and the packages below it.
// Targets that name no package holding a test marked for the key refuse the call.
// The refusal names the call that runs a test the key does not reach:
// `lym gate test --tier <its tier> <its package> -- -run '^<its name>$'`.
// A narrowed call names on stderr the tests it keeps and each target that names no package holding one.
// The guard step's `-run` replaces a `-run` among the words after `--`,
// and that line then says so and names the same call.
// `--copies` beside such a key is refused, since the guard step runs no copies.
// In a hub that declares tiers, a call that would run no test is refused too:
// no test is marked for the key, or the scanned directory holds no `go.mod`.
// The scope needs the `go` scoper, and a listing failure refuses the call, since the scope would run nothing.
// The template's `gateslot.UpkeepGateKey` is such a key.
//
// # Hub resolution
//
// The directory is resolved with lymcwd.ResolveWorktree from any directory inside a worktree, a nested module included.
// Every verb acts on code, so a directory in a records worktree resolves to the code worktree it pairs with.
// Inside a hub the config is `gateslot.LoadConfig` over the hub, this machine's override included.
// An absent gate.yaml, an invalid one and a hub with no tier are refused with a way forward.
// Outside every hub, in a git worktree whose hub carries no board, the run is unslotted.
// It reads the worktree's own gate.yaml.
// With none, or in no git worktree, the call is refused naming `lym config gate`.
//
// # Slots
//
// A tier first takes the slots it asks for through `gateslot.Pool.AcquireSlots`, with a holder that names the tier.
// It releases them before the next tier.
// The wait of a call that runs in this process has no cap and ends when the slots free.
// An interrupt cancels the wait and exits 128 plus the signal number.
// A tier with `slots: all` holds every slot and waits behind the pool's drain.
// The holder carries the call's slot priority, read once before the first tier.
// A call whose worktree's run holds a live parent lock of the hub waits first.
// A `card` or `batch` call waits next, and every other call last.
// A parent lock store that cannot be read is logged and reads as no lock held.
// Bound: a gate tier holds its slots for at most its `gate.tiers.<tier>.timeout_min`.
// The build step holds its slots for at most `gate.build.timeout_min`.
// Outside a hub the tiers run unslotted.
//
// `lym gate status` reads the pool of the hub that contains the working directory and changes nothing.
// It answers an ok envelope with `in_hub`, the pool's `slots` count, `draining` and `held`.
// `held` has one entry per held slot, oldest first.
// Outside every hub it answers `in_hub: false` and nothing held.
//
// # Inheritance
//
// The slot variable gateslot.InheritEnv may name a slot of the pool that is held.
// The run then goes inside that slot without acquiring,
// so a verify command that calls `lym gate test` cannot deadlock.
// Bound: a forged variable naming any held slot runs unslotted inside that holder's slot.
// The standing agent deny refuses an agent command that names the variable.
// An operator shell or a script that hides the name is outside the bound.
//
// The prebuilt-binary variable is set or stripped on every path,
// so a forged value never reaches a gated run and can mislead only an ungated raw run.
//
// # Green record
//
// `lym gate test` carries a green tier forward over an unchanged tree, at every site and manual run alike.
// The record is `green.json` in the worktree's never-tracked gate directory, and an unreadable file reads as empty.
// A tier's fingerprint hashes its declaration without `gate.tiers.<tier>.parallel` and `gate.tiers.<tier>.slots`.
// It leaves out `gate.tiers.<tier>.timeout_min` and `gate.tiers.<tier>.test_timeout_min` too.
// It adds the hub's `gate.build` declaration for a `shared_build` tier.
// It adds the hub's `gate.inert_paths` when non-empty.
// A changed command, env, targets, counter, filter, build or inert list therefore never carries.
//
// A dirty worktree, judged by `verifytree.DirtyPaths`, neither reads nor writes the record.
// On a clean worktree each listed tier subtracts what the record holds green for the current tree:
// a whole entry covers every target, and under the `go` scoper a package entry covers its packages.
// Under the `none` and `dotnet` scopers only a whole entry carries.
// An entry with empty args covers a request with empty args, the guard step's own `-run` filter and the watch skip;
// any other args need an exact match, so `-- -count=1` reruns.
// An entry records the held tests apart from its args.
// It covers a request, or serves as its base, only when that request holds back each test the entry held back.
// A tier with nothing left is carried: it takes no slot, runs no command and writes a run record with `carried` set.
// Each entry names the log of the run that passed, and a carried record lists those logs as `reused_from`;
// a line before the tier's summary line names them too.
// A tier with `carry: false` is never carried and writes no entry.
// Neither `gateslot.BilledTier` nor a manual tier is carried, since such a tier runs only when a call names it.
// Nor is any tier of a `--gate` call whose key sets `gate.gates.<key>.reuse` to false;
// that call still writes its passes, so a later call of another key can carry them.
//
// A pass is recorded only when HEAD's tree is unchanged and the worktree still clean at that point.
// A tier writes a whole entry when its carried part and passed remainder cover every package of its `all_targets`;
// else the packages it ran.
// Each write keeps the current tree's entries and each fingerprint's 20 newest whole-green commits;
// only a whole entry with empty args outlives its tree and can serve as a base.
// The record only saves time, so a failure around it is logged and decides nothing: every tier runs.
//
// Bound: a carried tier trusts a pass over the same tree, tier declaration and args on this worktree,
// so a flaky test that passed once is not rerun.
// A Go toolchain change, inherited environment variables outside the tier's `env` and ignored files are outside it.
// The record never leaves the machine, so an adopted run tests afresh.
// A forged or hand-edited whole entry carries every later gate on that tree.
// That is accepted: a session that can write the worktree can change the code and tests under review.
//
// # Timeouts
//
// The tier's `gate.tiers.<tier>.timeout_min` bounds the prebuild and every invocation of its command.
// It counts once the tier's slots are held.
// A tier that outlives it has its process tree killed, fails with exit code 1 and prints `gateslot.TierTimeoutLine`.
// The tree is the shell's process group and every process that carries the shell's marker in its environment.
// The bound counts only the time the host was awake.
// A tier the host suspended or hibernated through therefore keeps the time left on it.
// The time passed is read at short polls,
// and a reading that ran more than one poll past the wait before it spans a suspend and counts only that wait;
// the build's `gate.build.timeout_min` counts the same way.
// The work before the first tier is bounded by a ceiling;
// a step that outlives it fails naming the step, and the way forward is to run the same command again.
//
// # Counters, record and log
//
// A tier's output goes through its counter, `go-json` or `vstest`, which passes the text on and counts its tests.
// The `vstest` counter reads the summary of every console logger verbosity:
// the minimal one's `Passed!` or `Failed!` line, and the normal and detailed ones' `Total tests:` block.
// After each tier the verb prints `gateslot.TierSummaryLine` to stderr.
// In that line `wait` is the tier's wait for its gate slots, on the line of a rerun too.
// Work done inside the held slots before the tier's command, such as the shared build, is no part of it.
// Its `wall` is the run's own time without that wait.
// The `lym` prebuild of a `--copies` run is in neither figure, while a run without copies counts it in `wall`.
// The verb then writes the run record and log through `gateslot.WriteRunRecord` under `gateslot.RunLogDir`:
// the board's gate directory inside a hub, so the records outlive the worktree;
// else the worktree's `.lym` gate directory.
// A write failure is logged and decides nothing.
// The record carries each failed test with its first line as `failures`, and `launches` for a `go-json` run.
//
// A `vstest` tier that sets `results: trx` gets its results from TRX files the gate asks for.
// The gate owns the results-directory and logger words, so a call whose own words hold them is refused.
// The TRX files replace the console counts and failed tests.
// A run-level error message, such as a test host crash, is listed among the causes.
// An invocation that wrote no readable TRX file keeps the console counts.
// A `vstest` tier that sets no `results` keeps the console counts and logs no crash cause.
//
// A `go-json` tier under the `go` scoper fails with exit 1 when a package holding a test under the tier's tag ran none;
// the tier's command then lacks its tag.
// The line names the tier, the package and `lym config gate`.
// A run whose words after `--` carry `-run`, `-skip`, `-list` or `-bench` is exempt.
// A watch-skipped test does not count as a test the package holds.
// A passing `vstest` invocation whose output reported no test still passes and prints a line to stderr and the log.
// The line is there because a test project can skip all tests unless an MSBuild property is passed.
// The line cannot see a disabled project beside a project that ran, or name the property behind a skip.
// When the words after `--` carry a test filter, the line says instead that no test of the project matched it.
// It names the declared tiers the call does not run, and `--tier` as the way to run the test through its own tier.
//
// # Stray files
//
// A tier run lists the worktree's untracked, non-ignored files before and after it.
// A file the second listing holds and the first lacks fails the run with exit 1, a failing run keeping its own code.
// Such a file is one a test wrote outside its own temp directory.
// A stderr and log line names it, with the way forward of moving the write under the test's temp directory.
// A `--copies` run is listed once around all its copies.
// A worktree git cannot list is not checked.
// Bound: a file another process makes in the worktree while the tier runs fails the run as well.
//
// # Lint steps
//
// A `--gate` key's `lints` run in the key's order after every tier, vet and guard run of an uncancelled call.
// Each runs over the worktree against HEAD, untracked files included:
// `comments` through `commentlint.Lint` and `pattern` through `patternlint.Lint`.
// A step that finds something, or cannot run, is a failed run labelled `lint <name>`.
// Its exit code is 1 when no earlier run failed, and its findings go into the failure report.
// A call with manual flags runs none.
// Bound: a step reads only the working tree against HEAD, so on a clean tree it finds nothing;
// the round and darn gates lint their own range through `loomshed`.
//
// # Red-first verb
//
// `lym gate red-first --gate <key> [--base <rev>]` proves that a bug fix's test was red before the fix.
// A gate on a bug fix's writer row runs it as a subprocess and reads its exit code.
// The base is `--base`, else the merge base with the parent branch.
// A worktree with no parent branch and no `--base`, the prime's case, and a `--base` that names no commit are refused.
// `--gate` is required, and its refusal comes before hub resolution.
// A diff with no added or changed test is refused with exit `gateslot.RedFirstNoTestExit`,
// whose way forward is a recorded decision whose title opens `gateslot.NoTestSeamTitle`.
// A hub whose scoper is neither `go` nor `dotnet` exits `gateslot.RedFirstUnjudgedExit`, which a red-first gate passes.
//
// Under the go scoper the verb puts the base's production code back with a Go build overlay.
// It runs the key's tiers once in this process, once per package that holds a changed test.
// The `{args}` are `-overlay=<file>` and `-run=^(<names>)$`.
// The overlay maps each changed production `.go` file of the module to its text at the base.
// It maps a file the base lacks to the empty path.
// It maps each changed non-Go file a base production file embeds to its base text.
// Test files and every other changed non-Go file keep their working-tree text.
// The key's lints and scope never run, and each tier's command must hold `{args}`.
//
// Under the dotnet scoper the changed tests are the test methods `dotnetimpact.ChangedTests` names in the changed files.
// Each belongs to the `.csproj` of the nearest directory above its file.
// The verb writes the base's tree into a temp checkout through fabric.
// Every changed file whose nearest `.csproj` is one of those test projects keeps its working-tree text there.
// It runs the key's tiers that hold `{targets}` once per test project, with the request's checkout root on the checkout.
// So the shared build and each tier build and run the base's production code with the changed tests.
// The one target is the project's absolute path in the checkout.
// Each test's `FullyQualifiedName~<Class>.<Method>` term fills `{filter}`.
// A failed test is red when the failure report names its method, bare, with arguments or fully qualified.
// With none named, a C# compile error in a file of the test project makes every test of the project red.
// A test that reads a member only the change adds does not build on the base.
// The envelope then lists the error under `build_errors`.
// The verb exits `gateslot.RedFirstUnjudgedExit` when no changed test is a method of a non-abstract class.
// It exits so too when a test file's directory chain holds no `.csproj` or its nearest holds two.
// It exits so too when no tier of the key holds `{targets}`.
// It exits so too when a project's tier run discovered no test:
// its filter selected none, or a restore or build failure ended the run before any test ran.
//
// The exit code is the contract:
//   - 0 when a changed test failed on the base.
//     The envelope lists the red tests as `<package>.<Name>` under the go scoper.
//     Under the dotnet scoper it lists them as `<Class>.<Method>`.
//   - `gateslot.RedFirstGreenExit` when every changed test passed on the base, which does not show the bug.
//   - `gateslot.RedFirstNoTestExit` when the diff adds or changes no test.
//   - `gateslot.RedFirstUnjudgedExit` when the hub's scoper is neither `go` nor `dotnet`,
//     or when a C# change's tests cannot be selected or run.
//   - 1 for a refusal, and for a run that ran tests and judged none of them, such as a timeout or a crash.
//   - 128 plus the signal number after a terminating signal.
//
// The green exit's way forward names a second path, for a change that fixes no bug.
// That path is a recorded decision whose title opens `gateslot.NoBugFixedTitle`.
//
// A test is red when `gateslot.NamedFailedTests` names its package and name in the output,
// or `gateslot.PackagesFailedOutsideTests` names its package, so a test that does not build on the base counts as red.
// Bound: `go.mod` and `go.sum` are not put back.
// Bound: a changed non-Go file that the code reads at run time, rather than embeds, keeps its working-tree text.
// Bound: a test that every tier of the key skips counts as passing.
// Bound: a `lym` binary a tier prebuilds is built from the working tree, not from the base.
//
// # Failure report
//
// A call with a failed run, a halted call included, prints the failure report to stderr before its closing line,
// so one read of the call's result shows every failure.
// It opens with `gate test: failure report, every failure of this call:`.
// It names each failed run in plan order, lint steps last, with its outcome and log path.
// Under a run each failure follows: a failed test as its package and test path, a package that failed outside any test,
// and a lint finding as its `file:line`, each with the first lines it printed.
// The report holds a bounded number of lines and names the lines each run lost.
// It points at the logs, or at the standalone lint verb for a lint step.
// It closes with the files the change touches.
// They are the diff from the merge base with the pair's parent branch;
// outside a hub or without a parent they are the diff from HEAD.
//
// # Trimpath
//
// A hub's tiers set `-trimpath` in their GOFLAGS so a fresh worktree reuses the build cache.
// Under it `runtime.Caller` reports module-relative file paths,
// so a test that locates its repository from one fails, though a raw `go test` passes.
// When a tier's last GOFLAGS entry enables it, the verb prints a line naming the tier and its GOFLAGS value.
// The line gives the way forward, locating the repository from the working directory or `go.mod`.
// The line opens the tier's log and goes to stderr just before the tier's summary line.
//
// # Test result cache
//
// Every tier command runs with `gateslot.TestCacheOffFlag`, `-count=1`, first in its GOFLAGS,
// so `go test` keeps no test result cache and the green record carries a pass over an unchanged tree.
// A hub's own `-count` in GOFLAGS, or one in the words after `--`, still wins.
//
// # Child lifetime
//
// A tier's shell runs through `internal/proc`'s tied child, which ends the tree when the verb dies, a hard kill included.
// On a terminating signal, including one sent to the verb's own group, the verb kills that tree.
// It releases its slots and exits with 128 plus the signal number.
// Bound: the parent-death signal reaches the shell alone,
// so a command the shell forks rather than replaces outlives a hard kill of the verb;
// a tier command that must die with it starts with `exec`.
//
// A tier's or the build's shell that exits ends its run with its own exit code.
// This holds even while a leftover process holds its output pipe.
// One example is an MSBuild worker node a crashed `dotnet` left.
// After a bounded delay the verb kills what is left of the shell's process group.
// Every shell gets an environment entry of its own, `LYM_GATE_CHILD_<pid>_<n>=1`, which each process it starts inherits.
// The verb then also kills every process carrying it.
// A leftover in a session of its own, such as a test host, therefore dies too.
// Bound: a process that cleared its environment, or runs where no environment is readable by pid, escapes that kill;
// on Windows the shell's kill-on-close job ends its tree instead.
//
// # CPU watchdog
//
// While a tier's command runs, the verb samples its child tree through `proc.TreeSample`.
// A `.test` process that keeps at least 90 percent of one core busy for `gate.busy_quit_sec` gets one SIGQUIT,
// so the Go runtime prints every goroutine's stack and the run reports the package failed long before the tier's timeout.
// The verb prints `gateslot.WatchdogLine` after the quit, and a halt names `gateslot.BusyQuitWayForward`.
// The watchdog is off when `gate.busy_quit_sec` is 0 or when the words after `--` carry `-bench` or `-fuzz`.
// It is also off when `LYM_GATE_TEST_PID` in the tier's environment names a live ancestor.
// That ancestor's own watchdog sees these binaries.
// It is off where `proc.TreeSample` is unsupported.
// It watches `.test` processes only, so a `vstest` tier is bounded by its `gate.tiers.<tier>.timeout_min` alone.
//
// # Memory guard
//
// While a tier's command runs, a memory guard samples its child tree through `proc.TreeSample`.
// A `.test` process whose tree holds more than `gate.memory_quit_mb` resident is killed whole with SIGKILL;
// a tree that holds another tree over the cap is spared, and the inner binary is killed.
// When no tree passes the cap and the memory limit the verb sits under is nearly spent,
// the largest tree of 1024 MB or more is killed whole.
// That kill is a slice-pressure kill when the gate call's own process tree holds less than half the limit's use:
// other load filled it.
// The tree is the whole call's, so sibling `--copies` copies count as the call's own.
// Its line names the largest other holders of the limit's cgroup, which `proc.ResidentInCgroup` lists.
// It calls the kill an infrastructure stop, whose way forward is to wait for that load to end.
// Each kill prints a line naming the binary, its package directory and the reason;
// the tier's log and failure report carry it.
// A tier whose only stops were slice-pressure kills, and that failed nothing else, is rerun once;
// after any other guard kill the tier is never rerun.
// The guard is off when `gate.memory_quit_mb` is 0 or when `LYM_GATE_TEST_PID` names a live ancestor.
// It is also off where `proc.TreeSample` is unsupported.
//
// A tier's `memory_mb` runs its command under `proc.RunMemoryCappedVia`.
// One cap holds its builds and test binaries together.
// A kill at that cap fails the run with a line naming the tier and its cap, and the tier is never rerun.
// On a host whose user manager cannot cap a scope, the cap limits each process's data segment instead,
// and an allocation past it fails in that process with no cap line.
// A command that cannot be started under the cap is a refusal naming the cap and its way forward.
// This cap holds every process of the command, a `vstest` tier's included.
//
// # Process cap
//
// While a tier's command runs, the verb also lists every process carrying the run's `LYM_GATE_TEST_PID` entry.
// An example is a tmux server a test starts, which leaves the command's tree.
// When the list passes `runProcessCap`, the verb kills every listed process with SIGKILL at each period.
// It does so until the command ends.
// The tier then fails with a line naming the cap and the way forward, and is never rerun as a resource shortage.
// The cap guards against a test binary that a pinned tmux job or child runs again, re-running its own suite.
// A nested `lym gate test` sets its own entry and counts once toward the outer cap.
// It is unsupported where `proc.PIDsWithEnv` is, and the cap then ends.
//
// # Detached run
//
// `lym gate run-detached <run-dir>` is a hidden internal-audience verb: it runs a gate call that outlasts its caller.
// A run lives in `gateslot.WaitDir(anchor)/detach/<key>/<run id>`, under the never-tracked gate directory,
// and its directory holds the request, lease, process claim, tree identity, phase, line, exit and output files.
// The lease is the latest deadline any call wrote, and only an explicit expiry moves it earlier.
//
// Before it runs anything, the runner refuses a directory with no live lease or a missing or malformed request.
// It also refuses a verb it does not run and a claimed run.
// Each refusal names `lym gate test` and `lym gate card-verify` as the way forward.
// The claim is an exclusive create of `process.json`.
// Bound: a direct call reaches only a directory a front made and refuses one its runner claimed,
// so a hand-written lease lets one run start, which ends at that lease.
//
// The runner runs the request in-process through a fresh gate tree, as a `--wait` call does,
// and reports the tier it waits for or runs to `phase.json`.
// It cancels the call once the lease lapses or a terminating signal arrives.
// It kills its tiers and releases its slots as a signal does, and its records carry the abandoned mark.
// A call that ended on its own writes its own code to `exit.json`.
// A lease that lapsed while the call ran appends an abandoned line to `stderr` and writes code 1 and the abandoned mark.
// A lease that lapsed after the call returned abandons nothing.
//
// # Capped call
//
// A `lym gate test` call without `--wait` from an agent strand is capped:
// it starts or joins a detached runner and waits for the run's result up to the caller role's keep-warm cap,
// so a run that outlasts the cap keeps the agent's prompt cache warm instead of blocking one long call.
// A call is not capped when it gives `--wait` or when `agentname.StrandNameEnv` is empty.
// A call is also not capped when its directory is in no git worktree or the resolver's cap is zero.
// A call inside a slot an enclosing gate run holds is not capped either.
// An uncapped call runs in this process; `RunGate` and every call a verify or card-verify line makes are among them.
// The cap is the answer of `shuttleengines.KeepWarm` for the strand.
// Bound: the strand is read from the caller's environment,
// so a forged or absent strand chooses only how long a call blocks, never what runs or its verdict.
//
// A call's key is a hash of the verb, the resolved run directory and the call's words.
// A call from a fixer strand, a role `gatefixer.IsFixerRole` matches, adds its strand name to the hashed words,
// so a fixer's own gate call never joins the run waiting on it and two fixer strands never share one.
// Under the key directory's lock the front joins the key's current run when `joinable` holds, else starts a fresh one.
// A run is joinable only while its lease holds and no front printed its result;
// HEAD's tree and the sorted dirty paths equal the run's `tree.json`,
// and the run ended without the abandoned mark or has a live runner.
// A fresh run expires the superseded run's lease, so a live runner of it ends at its next lease read.
// The front launches `lym gate run-detached <run dir>` detached with the strand name dropped.
// Inside a hub it registers the runner with `runproc.RoleGate` so the pair's teardown ends it.
// A launch that fails, or a runner that ends or does not claim in time, is a failed start:
// the front exits 1 with a way forward of running the same command again, then with `--wait`.
// A signal ends the front alone with 128 plus the signal number while the runner goes on.
//
// A result without the abandoned mark is printed: `stdout`, then `stderr`.
// A `stdout` past `replayStdoutBytes` is named on stderr with its size.
// Its last line is printed when it is a JSON envelope, so the answer reads as one tool result.
// The call exits with the recorded code; a recorded 75 exits 1 after a stderr line saying the code was a test's own.
// A passed cap, and a result with the abandoned mark, answer ok with `done: false` and the phase from `phase.json`.
// The answer carries a message to run the same command again in a turn that does nothing else.
// While the run waits for its slots it also carries `slot_free: false` with each holder's worktree, site and tier.
// The exit code is SlotBusyExit.
//
// A call with `--gate <key>` may wait one long call in place of many short ones:
// with the resolver's break-even, `chooseCap` raises the cap to the resolver's long cap;
// it does so when the median wall time of the key's latest full calls exceeds the cap times the break-even.
// Bound: an estimate from a few earlier calls can mis-pick, which costs cache money only, never a verdict.
// Bound: the join compares the tree and the dirty paths, not the dirty files' content,
// so an edit to an already-dirty file between two calls with the same words gets the earlier content's run.
// Bound: the plan lives under `_lym`, which neither the join nor `verifytree.DirtyPaths` reads.
// So after a plan edit a `--card` call with the same words joins the run of the old plan while its lease holds.
//
// # Card fixer
//
// A red `lym gate test --gate <key>` agent call hands its failure to a cold fixer when the key's `fixer` is set.
// The call answers with the re-run's result.
// Config load allows `fixer` on the `card` key alone, so only the Webster card gate hands off;
// only a capped call reaches a runner;
// an operator's call, a `--wait` call and a call inside an enclosing slot never do.
// A call outside a hub, with no slug, or for a key or role that does not hand off ends as it does without a fixer.
// A kit that cannot be built ends the same way, after the runner logs why.
//
// On a red verdict, once the slots are released, the runner writes the phase `fixing`.
// It writes the stderr report to `fixer-report.md` and spawns one fixer with the shared cold frame and the card body.
// It then runs the gate again.
// It appends the fixer's reply or the dead note to the re-run's stderr under `## Cold fixer reply`.
// A pass, an abandoned exit, a `card-verify` request and a refusal end with the first run's result.
// So do a red verdict holding `gateslot.RunnerCrashLine` or `gateslot.MemoryPressureLine` and a failed spawn.
// A passed lease or a terminating signal during the phase stops the fixer and the run ends abandoned.
// A run in the phase `fixing` is joinable under changed dirty paths,
// so a caller's repeated call joins whatever tree the fixer is changing.
// Bound: one fixer per red call, which Go bounds;
// the number of red calls per card rests on the Webster fork prompt's self-fix cap.
// Bound: a fixer's own gate call never hands off, and its strand name keys it apart from the call waiting on it.
//
// # Report
//
// `lym gate report [--slug <slug>] [--since <YYYY-MM-DD>]` prints one row per slug and gate key.
// It reads this machine's run records through `gateslot.ReadRunRecords`.
// A row holds the tier runs, the carried tiers, the summed wall time and slot wait,
// and the launch time with its share.
// A record marked abandoned counts only in the row's `abandoned_*` fields and adds nothing to the other totals.
// A malformed `--since` is refused naming the expected form before hub resolution.
// The verb carries the operator audience annotation, takes no slot and changes nothing.
//
// # Card verify
//
// `lym gate card-verify <card id>` runs one plan card's `**Verify:**` line through `internal/cardverify`'s `Runner`.
// It sits under `lym gate` because the fork-mode hook refuses every `lym webster` command from a fork.
// It carries the role audience annotation because the implementer fork's prompt names it.
// An id is a card's number or its `N-slug`, read from the plan at the anchor's plan directory;
// an unknown id and a worktree with no plan are refused.
// A card with no `**Verify:**` line passes with a message saying so.
// The line's output passes through and the verb exits with the line's own code.
// A line that timed out or did not start exits 1.
// Inside a hub a line that is not a `--wait` gate call takes one slot of the hub's pool.
//
// The verb has no `--wait`, so a call from an agent strand is capped as a `lym gate test` call is,
// and the line runs in a detached runner the call starts or joins.
// The key is the verb, the worktree and the matched card's `N-slug` id, so `4` and `4-<slug>` share one run.
// A cancelled line's process group gets SIGTERM and then SIGKILL,
// and a front that supersedes a run whose runner is not live first kills the group `line.json` records.
// Bound: a job that cannot be held on Windows is logged and the line runs without it,
// so the runner's death leaves the line's tree to its own end.
// Bound: on Linux a dead runner's line runs on until a front supersedes it, or to its own end when no front comes.
// Bound: the key is the card's id,
// so a call after an edit of the card's `**Verify:**` line joins the run of the old line while its lease holds.
//
// # In-process run
//
// `RunGate(ctx, dir, gate, out, stderr, observer)` runs the gate key `gate` in `dir`.
// It runs through the same path as `lym gate test --wait --gate <gate>`, with the CLI's defaults.
// The caller names only the key and the directory, never targets or tiers.
// It gets the call's exit code, the envelope on `out` and the summary lines on `stderr`.
// A `TierObserver` hears each tier of the run through `Started` and `Ended`, and a nil func is skipped.
// A tier carried as already green ends with no start, and a tier blocked by a failed build ends with its blocked record.
// A rerun tier ends once, with the rerun's record, and a refusal after a start ends with no end.
//
// # Exit codes
//
// The exit code is the first failing run's own code, in plan order.
// For a test binary the watchdog quit it is the test run's failure code, and for a runner crash 128 plus the signal.
// A tier that timed out gives 1, a run that ended without a failure while the call was cancelled gives 1,
// and a refusal gives 1 with a JSON error envelope on stdout.
// A terminating signal gives 128 plus the signal number.
// SlotBusyExit (75) comes only as a capped call's `done: false` answer.
// It is never the code of a call that runs in this process.
package gatecli
