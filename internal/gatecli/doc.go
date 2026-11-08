// Package gatecli provides the cobra command tree for `lym gate`.
// Its verb `test` is the sanctioned route for running the hub's test tiers.
// Its verb `status` shows who holds the hub's gate slots, and its verb `report` totals the run records per gate key.
// Its verb `card-verify` runs a plan card's `**Verify:**` line.
//
// The module is not standalone-capable and does not go through internal/cliwire.
// It has no target directory, state, plan or stencils to wire, and it resolves hub membership itself.
//
// # Verb
//
// The form is `lym gate test [-C <dir>] [--gate <key>] [--tier N]... [--upto N] [--card <id>]... [--main-packages] [--base <rev>] [--all] [--guards] [--wait] [<targets>...] [-- <args>]`.
// It runs the tiers the hub declares in gate.yaml in ascending tier number.
// It then runs the vet step and the guard step below when they are planned.
// A run that fails does not stop the call: every later tier, vet and guard run still runs.
// A failure the change under test did not cause, such as one present at the base, therefore hides no later check.
// When a run failed, the verb closes with one stderr line naming each failed run.
// The line follows the failure report the Failure report section describes.
// It names a vet run as `vet tierN`, a guard run as `guard tierN` and a lint step as `lint <name>`.
// Each name carries the run's exit code, or a lint step's finding count.
// When no run failed under the `--card` scope form, the verb closes with one stderr line.
// It names each declared tier that no listed tier is or covers, manual tiers included.
// The line says the pass is not green for them, so a card gate that runs tier0 is never read as fully green.
// A vet or guard run under a tier does not count as running it.
// Bound: the line is output only and changes no exit code or record.
// No other scope form prints it, since their tier lists are the coverage the hub chose for that site.
// A tier run that fails without timing out and whose output holds a resource-shortage marker is rerun once.
// The markers are `resource temporarily unavailable`, `failed to create new OS thread` and `cannot fork`.
// Matching ignores case.
// The rerun uses the same slots and starts after a ten-second delay, and its result is the tier's.
// A stderr line names the marker, and the rerun's log opens with it.
// A rerun that fails on a marker again keeps its exit code.
// The closing line names the run as a resource shortage, an environment error.
// It gives the way forward of re-running once the machine's process load drops.
// A tier run is also rerun once after the same delay when all three hold:
// it fails without timing out, it reports no failing test, and it exits 128 plus the number of a crash signal.
// The crash signals are SIGABRT, SIGILL, SIGTRAP, SIGBUS, SIGSEGV and SIGFPE.
// A shell reports that exit code when a runtime aborted and crashed its test runner, so the run judged no code.
// A command the shell replaced and the signal ended takes that same exit code.
// A run with no shortage marker is read for a crash.
// The stderr line names the exit code and the signal, and the rerun's log opens with it.
// The first run's record and log are written before either rerun starts.
// The first run's record carries a crash's signal as `runner_crash`.
// The rerun's record carries `rerun` and counts its slot wait as none, since the first run's record holds it.
// A rerun that crashes again keeps its exit code.
// The closing line names the run as a runner crash with no failing test.
// A call closes with `gateslot.RunnerCrashLine` after the line of failed runs when every failed run crashed so.
// It does so only when no failing test, failed build or lint finding stands beside them.
// A site thereby tells a runtime's crash from a failing test.
// Bound: one rerun per tier run, so a test whose own output holds a marker costs one extra run and keeps its verdict.
// A test that aborts its own runner, such as a .NET stack overflow, is reported as a crash, not as a failing test.
// The build step is not rerun.
// A refusal, a slot wait that does not end with the slots held and a terminating signal still end the call at once.
// `--gate <key>` runs the tiers and the scope the `gates` key of that name holds.
// `listedTiers` takes the key's tiers as the listed tiers.
// It goes with none of `--tier`, `--upto`, `--all` and `--base`, since the key's tiers and scope come from gate.yaml.
// Such a call is refused before hub resolution, naming the key and the flag to drop.
// Once the config is loaded, a key the config does not hold, an empty tier list and an undeclared tier are refused.
// The refusal takes its way forward from `gateslot.Config.Gate` and names `lym config gate`.
// A key of scope `card` takes the `--card` ids as the card form.
// It is refused when none is given, naming the key and the way forward of passing `--card <id>` per card.
// A key of scope `all` is the `--all` form, and a key of scope `diff` is the computed form.
// `--card` ids beside a key of another scope are ignored.
// Explicit `<targets>` beside `--gate` replace the key's scope.
// Each run record carries the key as `gate_key`, empty for a call with manual flags.
// `--gate` opens no flag combination the manual flags lack, bar the key's lint steps the Lint steps section describes.
// `--tier` names a tier and may repeat.
// The default is tier0 when neither `--tier` nor `--upto` is given.
// A tier another listed tier covers is dropped and named as covered on stderr.
// `--upto N` adds every declared tier numbered 0 through N to the `--tier` values, skipping a gap in the numbers.
// It also skips a tier marked `manual`, which it names on stderr unless `--tier` names it.
// A manual tier runs only when `--tier` names it, so a tier that spends tokens never starts from a range.
// A negative N is refused before hub resolution.
// `--tags` stays a registered flag only to be refused before any slot, naming `--tier`.
// A tier's own command sets its build tags.
// The words after `--` fill each tier's `{args}`.
//
// A tier is one shell command that may hold these placeholders:
// `{targets}`, `{test_targets}`, `{parallel}`, `{args}`, `{filter}` and `{test_timeout}`.
// `renderTierCommand` fills them, quoting each target and each arg for the platform shell through `verifyrun.QuoteArg`.
// `{test_timeout}` renders as the tier's `test_timeout_min` followed by `m`, such as `20m`.
// A hub's `go test -timeout {test_timeout}` thereby takes its per-binary bound from config.
// `{filter}` renders as `--filter ` and the quoted expression, or as nothing when there is no expression.
// With no class terms the expression is the tier's `filter`.
// With the terms of an invocation the dotnet scoper narrowed to one project it is `(<filter>)&(<term>|<term>...)`.
// It is the terms alone when the tier has no filter.
// Terms that push the expression past `greenCommandBudget` are dropped, and an Info log names the tier.
// The tier's `filter` then stands alone, so that project runs whole within itself.
// A command without `{filter}` renders no filter whatever terms it is given.
// A tier's `filter` applies at whole scope too, landing gates included.
// The executable, hub-file-only `tiers` key bounds it.
// `{test_targets}` stands where the targets reach `go test`, beside a `{targets}` the command's other steps take.
// It renders each invocation's targets narrowed to the packages that hold a test function the tier runs,
// so a pass compiles, links and starts no test binary that would find no test of its tier.
// `impactset.HoldsTierTest` judges each package over the tier and the tiers it `covers`.
// A test function opening with `tiergate.Require(t, N)` counts for tier N alone, and any other for every tier.
// A pattern target is listed with `go list` first, as the green record lists one, and an import path is kept.
// An invocation with no package left runs no command, its other steps included, and says so on stderr.
// The targets fill it unnarrowed when they cannot be listed or the narrowed list passes `greenCommandBudget`.
// A command that holds `{test_targets}` without `{targets}` takes a scope's targets as one holding `{targets}` does.
// A package gaining its first test of a tier thereby joins that tier's runs with no config edit.
// Bound: a tier whose `LYM_TEST_TIER` names a range runs the tests of tiers it does not `covers`,
// and a package holding only those is left out.
// The command runs through the shell `verifyrun.ShellFor` names, as a tied child in the run directory.
// It starts from this process's environment, less the strand-name and trace-id variables.
// An inherited `gateslot.PrebuiltLymEnv` is dropped too.
// The variables of the `env` config module (`internal/envconfig`) replace an inherited value.
// They are the hub's with this machine's override inside a hub, and the worktree's own outside every hub.
// `proc.BuildServerOffEnv` replaces an inherited or env module value, so the module never undoes a build-server switch.
// The tier's `env`, with `{parallel}` filled, replaces an env module value.
// The lease's `gateslot.InheritEnv` and the enclosing-gate pid are added last.
// The tier's `dir`, relative to the worktree root, is the run directory.
// A tier that sets no `dir` runs in the worktree's anchor.
// `gateslot.Config.WithDefaultDir` fills that once the config is loaded,
// so a hub anchored at a subdirectory need not spell it per tier.
// A tier with `prebuilt_lym` first builds `lym` at the tier's `parallel`, inside its slots and timeout.
// It builds only where the worktree root holds `cmd/lym`, and it exports `gateslot.PrebuiltLymEnv`.
// Inside a hub the binary is copied into the pool's store (`gateslot.Pool.PrebuiltDir`).
// The copy is named `lym-` and the first 16 hex digits of the binary's SHA-256.
// The build directory is removed, and the variable names the stored copy.
// Outside a hub the variable names the build directory's binary, which is removed after the tier.
// Go keys a cached test result on the variable's value.
// A path named by the binary's content therefore keeps a re-run on an unchanged tree cached.
// A stored binary already present is reused, with its modification time set to mark its last use.
// Each store prunes the other entries unused for two hours.
//
// # Scope
//
// A call takes one scope form:
// explicit `<targets>`, `--all`, `--card <id>`, `--base <rev>` or, with none of them, the computed form.
// Explicit targets replace each run's `{targets}` verbatim and run in the `-C` directory, else in the tier's `dir`.
// `--all` sets `{targets}` to the tier's `all_targets`, split at spaces, and runs in the tier's `dir`.
// A command holding neither `{targets}` nor `{test_targets}` runs once, whole.
// It does so under explicit targets and under a non-empty narrower scope.
// It says so on stderr.
// Flag-shape refusals need no config and run first, before hub resolution:
// `--tags`, naming `--tier`; a negative `--upto`; `-C` without explicit targets, naming them;
// `--gate` beside `--tier`, `--upto`, `--all` or `--base`; more than one scope form;
// and `--main-packages` without `--card`, naming `--card <id>`.
//
// # Scoper
//
// `computeScope` resolves the computed, `--base` and `--card` forms inside the pre-tier ceiling.
// `gate.yaml`'s `scoper` decides what they mean.
// The computed form takes its base per listed tier.
// A tier's base is the newest commit that is an ancestor of HEAD, read through `gitrepo`'s `IsAncestor`.
// The green record must hold that commit whole-green for the tier under its current fingerprint.
// The entry has empty args and the tier's module directory (`greenRecord.newestWholeAncestor`).
// A narrowed green never becomes a base.
// Neither does a whole entry recorded under `-- -run X` or under a since-changed tier command.
// Without such a commit the base is the merge base of HEAD with the parent branch.
// `parentMergeBase` reads that branch from the pair's fabric origin record.
// A tier with neither base, such as the prime on `main` or a standalone run with no green ancestor, runs whole.
// A dirty worktree reads no record, so it takes the merge base.
// Tiers that land on the same base share one `impactset.DeriveTargets` derivation.
// Each derivation prints `gate test: diff scope from <base> for <tiers>` on stderr.
// `--base` names one base for every tier.
// `--card`, repeatable, reads the plan at the anchor's plan directory through `planparser.ParsePlan`.
// It takes `planparser.CardPackages` of the named cards.
// An id is a card's number or its `N-slug`.
// An unknown id is refused before any slot, naming the plan's ids.
// A worktree with no plan is refused, naming `--all` and explicit targets.
// The plan read and its refusals, held by `readCards`, run first under every scoper.
//
// Under the `none` scoper each scoped form is whole, so a tier takes its `all_targets`.
// No guard step is planned, except under `inert_paths` below.
// Under the `dotnet` scoper `dotnetScope` derives the forms through `dotnetimpact`.
// A computed or `--base` scope groups the listed tiers by base as above.
// It runs `dotnetimpact.DeriveDiff` once per distinct base and tier `dir`.
// Each derivation prints `gate test: diff scope from <base> for <tiers>`.
// A tier with no base runs whole.
// A `--card` scope runs `dotnetimpact.DeriveCards` once per distinct tier `dir`.
// The seeds are `planparser.CardCSharpSeeds` of the cards.
// Both are told `gateslot.Config.Inert` as the inert predicate.
// A derivation that falls back gives its tiers a whole share.
// It prints `gate test: the scope falls back to whole: <reason>`.
// A narrowed share runs each selected test project once in the tier's `dir`, under one slot hold and one `timeout_min`.
// The project path is `{targets}`, and its class terms render into `{filter}`.
// A command holding neither `{targets}` nor `{test_targets}` runs once, whole, with its tier filter only.
// Two empty scopes tell apart.
// Nothing testable changed, such as an inert-only diff: that makes no invocation, as under `go`.
// Changes that no test class names run a tier with `shared_build` as no invocation.
// Any other tier runs whole and is named on stderr, so a compile break still fails the gate.
// The scope plans no guard step and no vet step.
// `--guards` is refused, naming the scoper and the way forward of dropping it.
// No config change gives a C# hub the guard tests.
// Green record: a narrowed project invocation is partial and writes no entry.
// That holds even when its target equals the tier's `all_targets`, so a filtered pass never stands in for a whole run.
// A whole `dotnet` pass writes its whole entry as under `none`, and only whole entries carry.
// The class closure skips every test class it does not select.
// It therefore misses only references that never spell a changed type's name.
// The gates that run whole, such as `parent-align` and `finalize`, catch those.
// Under the `go` scoper a computed or `--base` scope is `impactset.DeriveTargets` of the diff from its base.
// The diff runs to the working tree.
// The scope falls back to whole wherever the derivation does, naming the reason on stderr.
// A `--card` scope is the cards' packages.
// With `--main-packages` it also holds every `package main` package of tier0's module.
// `impactset.ModuleMainPackages` lists them.
// A main package a card already names is listed once, and a failed listing refuses the call naming the module.
// Under the `none` and `dotnet` scopers the flag adds nothing.
// A narrower scope runs each listed tier once per module, in that module's directory with its module-relative targets.
// One slot hold and one `timeout_min` cover every invocation.
// An empty scope, such as a diff of Markdown files only, runs no listed tier's command, bar a tier with `carry: false`:
// each other listed tier passes with zero discovered, is logged so and takes no slot, and only the guard step runs;
// a `shared_build` tier that only has the build to run takes its slots for it, as the Build step section says.
// A tier with `carry: false` runs whole over its `all_targets` in its `dir` under every scope form but explicit targets.
// That includes the empty scope, so a timing or capture tier never goes unrun on a computed scope.
// The import graph cannot see everything a narrower run skips, such as `go build ./...`.
// That is left to the whole runs at `final` and `landing`.
// Once the config is loaded, `planTierRuns` refuses these before any slot, naming `lym config gate`:
// words after `--` for a tier whose command lacks `{args}`; an undeclared tier; a hub with no `tier0`;
// a `covers` entry naming a tier the hub does not declare;
// and, for the guard step below, a guard group whose tier the hub does not declare.
// Every such refusal closes with a clause for a session that is not the prime:
// it takes none of the steps, reports status: FAILED and leaves them to the orch.
//
// # Inert paths
//
// A hub's `inert_paths` lists the patterns of files no test reads, and `gateslot.Config.Inert` is the one matcher.
// `allInertSince` reads the files that differ between a base and the working tree, untracked files included.
// It holds them inert when every one matches.
// An empty diff is not inert-only.
// A read error is logged at Warn and holds the diff not inert, so the scope stays as it would be.
// Under the `go` and `none` scopers a computed or `--base` scope checks each base group's diff before deriving it.
// A group that is inert-only gets the empty share.
// None of its tiers runs a command but a `carry: false` tier, which runs whole.
// Under `go` the derivation is skipped for that group.
// A call whose groups are all inert-only plans the guard step over a module scan of `tier0`'s dir, as a whole tier does.
// Under `none` the empty share is the only change, and every other group and tier stays whole.
// The scope plans no guard step.
// Under `dotnet` the derivation drops inert paths itself.
// A `--card` scope is not narrowed by `inert_paths`.
// A whole gate carries over inert-only history.
// A whole invocation of a tier that carries, with no entry on the current tree, is carried under one condition.
// The condition is that the newest whole entry of an ancestor has an inert-only diff to the working tree.
// The entry must be under the tier's fingerprint and the same module.
// A whole entry is then written for the current tree, so a later gate carries it directly.
// The carry runs only on a clean tree, and the inert list is part of the fingerprint, so a changed list voids it.
//
// Bound: a test that reads an inert file is skipped when only that file changes.
// A hub therefore lists only files no test reads.
//
// # Build step
//
// A hub's `build` declares a shell command, run once per call, that the tiers with `shared_build: true` depend on.
// The first such tier in plan order whose run takes slots runs `runBuild` inside its slots, before its own command.
// No later tier runs it again.
// A run takes slots when it has invocations left after the green record's subtraction or has only a build to run.
// A call whose opted-in tiers are all carried, or whose shares are empty, runs no build.
// A share is empty for an inert-only, all-inert-card or empty diff.
// A `carry: false` tier is the exception: it runs whole and so runs the build before its command.
// A `shared_build` tier whose share selected no test class has no invocation and takes its slots only to run the build.
// A compile break therefore still fails the gate, and the tier otherwise passes with nothing discovered.
// The command runs through the platform shell as a tied child in the build's `dir`.
// Its environment is the call's, the env config module's variables included.
// It adds the slot's inherited variable and the build's `env`, which replaces an env module value.
// The words after `--` never reach the command, and lym adds no switch to it.
// A build that sets no `dir` runs in the anchor, as a tier does.
// Its output passes to stdout and into its log.
// Its own `timeout_min`, counted from the hold, bounds it, so the tier's `timeout_min` starts after it.
// A timeout kills the tree, exits 1 and prints and logs `gateslot.BuildTimeoutLine`.
// A shell that cannot start is refused naming the build's `dir`, halts the call with exit 1 and writes no build record.
// The build writes a run record, `build` true and scope form `build`, for the tier it precedes.
// It prints `gateslot.BuildSummaryLine` on stderr.
// A failed or timed-out build is a failed run counted before that tier.
// The call exits with the first failed run's code in plan order.
// Every tier that opts into the build from there on runs no command.
// It writes a run record with outcome failed, the build's exit code and `blocked_by` the build's log.
// It prints `gate test: tierN did not run, since the build failed; see <log>` and no tier summary line.
// The closing line names it blocked by the build.
// Tiers that do not opt in still run.
// `lym gate report` totals build records apart, in `build_runs` and `build_seconds`.
//
// Bound: a `shared_build` tier whose command lacks `--no-build` builds again inside its own run.
// lym does not read the command.
//
// # Vet step
//
// Under the `go` scoper a `--card` scope also plans vet runs, after the listed tiers and before the guard step.
// Each declared tier whose build tag `tierN` brings a `.go` file into a card package (`tagTiers`) gets one vet run.
// The file is one that a build without tags leaves out.
// Vet runs go in ascending tier order.
// A listed tier and a tier a listed tier covers get none, since their own commands compile under that tag.
// So a card that breaks the compile under another tier's tag fails at its own gate.
// It does not wait for the batch gate's `go vet` steps.
// A helper a `tier0` file defines and a `tier1` file uses is such a break.
// A vet run is `go vet -tags tierN` over the card's packages, once per module in that module's directory.
// It uses the prebuild's go command, runs under the tier's `env`, `parallel` and `timeout_min`, and holds one slot.
// Its summary line and run record carry the scope form `vet`.
// It takes part in the green record under its own fingerprint, and a failed vet run stops no later run.
// Bound: this step does not vet a break visible only under a combination of tags.
// It does not vet one under a tag that adds no file to a card package either.
// The batch gate finds both.
//
// # Guard step
//
// The verb runs the guard step after the listed tiers.
// It runs with `--guards`, and under the `go` scoper after any computed, `--base` or `--card` scope.
// `guardStepRun` builds the step over the module's `//lym:guard` tests outside the scope.
// `impactset.DeriveTargets` names those packages for a narrower computed or `--base` scope.
// An `impactset.ModuleGuardTests` scan of the module at the run directory names them in the other cases:
// a `--card` scope, a scope fallen back to whole, and `--guards` over explicit targets.
// The run directory is `tier0`'s dir or the `-C` directory of explicit targets.
// The round and darn writer gates run `--base`.
// Every guard package counts as outside a tier that does not compile it, so those gates keep full guard coverage.
// The scan reads each guard test's tier from its file's `tierN` build tag, a file naming none counting as `tier0`.
// The step plans one guard run per tier holding guard tests, ascending.
// A guard run is one run of that tier's command with the group's guard packages as `{targets}`.
// The `{args}` are any words from `--` followed by `-run '^(<names>)$'`,
// so the guard's own pattern is the last `-run` and wins over one among those words.
// It acquires that tier's slots after the last listed tier and vet run released their own.
// It runs under that tier's `timeout_min` and `prebuilt_lym`, and it is logged as a guard step.
// A failed guard run, like a failed listed tier, stops no later run.
// A group is skipped when the call ran its tier, or a tier covering it, whole.
// A group whose tier the green record holds green carries.
// A group is refused before any slot when its tier is undeclared.
// It is refused likewise when its tier's command lacks `{args}`, or lacks both `{targets}` and `{test_targets}`.
// The refusal names `lym config gate`.
// A scan error refuses the call before any slot.
// The refusal names the file, the line when there is one and a way forward for its kind:
// move a misplaced marker directly above its `func Test…` line;
// move a marked test out of a `tier2` or `tier3` file into a `tier0` or `tier1` file;
// or make an unparsable or unreadable file readable and parseable.
// Any other scan error, such as an unreadable package directory, names the module directory.
// Its way forward is to make `go list ./...` succeed there.
// `--guards` under the `none` or `dotnet` scoper is refused naming the scoper, since the scan is Go's.
// A module with no marked test runs no guard step and logs so.
//
// # Watched tests
//
// A top-level test marked `//lym:watch <pattern>...` guards only the files its patterns name.
// An example is a cross-compile that matters only when a `_linux.go` file changes.
// Under the `go` scoper a narrowed computed or `--base` share and a `--card` scope skip such a test.
// They skip it when no changed file matches a pattern and its own file did not change.
// `impactset.DeriveTargets` names those tests for a computed or `--base` share.
// A `--card` scope reads its changes as the diff from the merge base with the parent branch.
// It matches the watched tests of its packages through `impactset.UnwatchedTests`.
// With no parent branch, or a diff that cannot be read, it skips none.
// A listed tier's run gets `-skip '^(<names>)$'` after the words from `--`, and a line on stderr names the tests.
// A call whose words after `--` carry a filter flag gets no skip, nor does a tier whose command holds no `{args}`.
// The guard step leaves out a watched guard test that the same changes leave alone.
// A whole scope and explicit targets skip nothing, so the whole gates run every watched test.
// A skipped test does not count toward the empty-tier check,
// and a whole pass with empty args covers a skipping run in the green record.
// A misplaced or malformed watch marker refuses the call, as a guard scan error does.
// Bound: `-skip` matches by name in every package of the run, so a test of the same name elsewhere is skipped too.
//
// # Hub resolution
//
// The directory is resolved with lymcwd.ResolveWorktree.
// It finds the containing worktree from any directory inside it, a nested module directory included.
// preflight.BoardLymPresent then reports whether the hub carries a board.
// It does not use the standalone-capable CLIs' mode resolution.
// Its anchor gate would refuse a nested module directory.
// Inside a hub the config is `gateslot.LoadConfig` over the hub, this machine's override included.
// An absent gate.yaml is refused with a way forward naming `lym fabric reconcile`.
// An invalid one is refused with an error that names each file in the stack and its way forward.
// A hub with no tier is refused naming `lym config gate`.
// Outside every hub, in a git worktree whose hub carries no board, the run is unslotted.
// The worktree's own gate.yaml is read through the same loader with only the anchor told.
// With none, the call is refused naming `lym config gate`.
// A directory in no git worktree has no worktree layer to read.
// It is refused naming `lym config gate` run in a git worktree.
//
// # Slots
//
// A tier first takes the slots it asks for through `gateslot.Pool.AcquireSlots`, with a holder that names the tier.
// It writes a wait record under the worktree's wait directory while it waits.
// It releases the slots before the next tier.
// The wait of a call that runs in this process has no cap.
// It ends when the slots free.
// An interrupt from the caller cancels the wait and exits 128 plus the signal number.
// Bound: a gate tier or the build step holds its slots for at most its `timeout_min`.
// A slot-held card-verify line holds them for at most the runner's per-line timeout.
// Any other slot taker of the pool, such as a drift step run or a plan-glyph type load, is bounded only by its own run.
// A capped call's front is bounded by the caller role's keep-warm cap instead, as the Capped call section says.
// A tier with `slots: all` holds every slot and waits behind the pool's drain.
// Outside a hub the tiers run unslotted.
//
// `lym gate status` reads the pool of the hub that contains the working directory and changes nothing.
// It answers an ok envelope with `in_hub`, the pool's `slots` count, `draining` and `held`.
// `draining` says whether a multi-slot acquire holds the drain, so new single-slot runs wait while a slot is free.
// `held` has one entry per held slot, oldest first.
// Each entry carries its `slot` number, `worktree`, `site`, `tier`, `pid` and `started` time.
// Outside every hub it answers `in_hub: false` and nothing held.
//
// # Inheritance
//
// The slot variable gateslot.InheritEnv may name a slot of the pool that is held.
// The run then goes inside that slot without acquiring, whatever slots its tiers ask for,
// so a verify command that calls `lym gate test` cannot deadlock.
// Bound: a forged variable naming any held slot runs unslotted inside that holder's slot.
// The standing agent deny refuses an agent command that names the variable.
// An operator shell or a script that hides the name is outside the bound.
//
// The prebuilt-binary variable is set or stripped on every path.
// A run that exports no fresh build removes an inherited value from the tier's environment.
// `gateslot.StripPrebuilt` does the removal.
// A forged value therefore never reaches a gated run, and can mislead only an ungated raw run.
//
// # Green record
//
// `lym gate test` carries a green tier forward over an unchanged tree, at every site and manual run alike.
// The record is one JSON file per worktree, `green.json` in the worktree's never-tracked gate directory.
// That directory is `gateslot.WaitDir` of the anchor.
// The file is read tolerantly: an unreadable or malformed file reads as empty.
// An entry holds the tier fingerprint, HEAD's tree and commit, the module directory and the args after `--`.
// It also holds either `whole` or the green package set, and its write time.
// `tierFingerprint` hashes the tier's declaration without `parallel`, `timeout_min`, `test_timeout_min` and `slots`.
// A changed command, env, targets, counter, `filter`, `shared_build` or `carry` therefore never carries.
// It also hashes the hub's `build` declaration for a tier with `shared_build`.
// It hashes the hub's `inert_paths` when it is non-empty.
// A changed build or inert list therefore stops the tiers it concerns from carrying.
// A key at its default value adds nothing to the hash.
// A tier that sets none of these therefore keeps its fingerprint and its green entries.
//
// Before the first tier the verb checks the worktree with `verifytree.DirtyPaths`.
// A dirty worktree neither reads nor writes the record.
// On a clean worktree it reads HEAD's tree and commit.
// Before acquiring slots each listed tier subtracts what `greenRecord.covered` holds green for the current tree:
// a whole entry covers every target;
// under the `go` scoper a package entry covers its packages, so a remainder runs as explicit packages;
// under the `none` scoper only a whole entry carries.
// The same holds under the `dotnet` scoper, since a pass narrowed to a project and its classes records nothing.
// An entry with empty args covers a request with empty args, the guard step's own `-run` filter and the watch skip;
// any other args need an exact match, so `-- -count=1` reruns.
// A remainder whose targets would exceed a shell-safe length runs whole instead.
// So does one whose packages `go list` cannot list.
// A tier with nothing left is carried: it takes no slot, runs no command and writes a run record with `carried` set.
// It prints its summary line with `carried` after the scope form and exit 0, so every reader of the line parses it.
//
// A pass is recorded only when HEAD's tree is unchanged and the worktree still clean at that point.
// A tree that changes during a run therefore records nothing.
// A tier writes a whole entry when its carried part and passed remainder cover every package of its `all_targets`.
// The packages are listed once with `go list` under the tags the tier's command names.
// Otherwise the tier writes the packages it ran.
// The guard step writes its entries under its own `-run` args.
// Such an entry never covers an empty-args request, and an exact repeat of the guard group carries.
// Writers serialize through a lock file beside the record.
// Each write keeps the current tree's entries and each fingerprint's 20 newest whole-green commits.
// An older ancestor therefore stays findable.
// Only a whole entry with empty args counts toward the 20 and outlives its tree.
// Only such an entry can serve as a base.
// The carry sits under `verifytree`'s command-and-tree skip, which every site keeps.
// The record only saves time, so a failure around it decides nothing.
// An unreadable record, a failed lock, or a failed status or HEAD read is logged at Warn, and every tier runs.
// A failed write is logged and leaves the tier's outcome and exit code unchanged.
//
// A tier with `carry: false` is never carried:
// `subtract` keeps every invocation of it and `recordPass` writes no entry for it,
// so it runs again on an unchanged tree.
// Bound: that is the record's rule alone.
// A verify site's verified-tree record still skips a command over a tree it passed on.
// Explicit targets still narrow such a tier, and a listed tier whose `covers` names it still drops it.
//
// Bound: a carried tier trusts a pass over the same tree, tier declaration and args on this worktree,
// so a flaky test that passed once is not rerun.
// A Go toolchain change is outside the fingerprint and the clean check.
// So are inherited environment variables outside the tier's `env`, and ignored files such as `.dev-bin`.
// The record never leaves the machine, so an adopted run tests afresh.
// A forged or hand-edited whole entry carries every later gate on that tree, Parent-Align and Finalize included.
// That is accepted: a session that can write the worktree can change the code and tests under review.
// The record guards only honest repeats, and no further deny is added, since `cp` and `tee` would still reach it.
//
// # Timeouts
//
// The tier's `timeout_min`, counted once its slots are held, bounds the prebuild and every invocation of its command.
// A tier that outlives it has its process tree killed through the tied child's context.
// It fails with exit code 1 and `gateslot.TierTimeoutLine` on stderr and in its log.
// The work before the first tier is bounded by a ceiling, 10 minutes in production.
// A step that outlives it fails naming the step and the way forward, which is to run the same command again;
// a transient hang such as a stalled `go list` passes on the rerun.
// The length of a `timeout_min` minute, the ceiling and the pipe wait delay are parameters of `newCommand`,
// so a test shortens them.
//
// # Counters, record and log
//
// A tier's output goes through its counter, `go-json` or `vstest`, which passes the text on and counts its tests.
// After each tier the verb prints `gateslot.TierSummaryLine` to stderr.
// It writes the run record and log through `gateslot.WriteRunRecord` under `gateslot.RunLogDir`:
// the board's gate directory inside a hub, so the records outlive the worktree;
// the worktree's own `.lym` gate directory outside every hub.
// The record's slug is the one the pair's fabric origin record names, empty for the prime and outside every hub.
// A write failure is logged and decides nothing.
// The `go-json` counter keeps, per package, the gap from its `start` event to its next event.
// It keeps the closing event's elapsed time too.
// The record carries both as `launches`; a cached package and one with no test files are left out,
// and a `vstest` run records none.
// It keeps, per package, the number of tests that ran in it, and a `vstest` run keeps none.
// For the failure report it keeps each failed test with its first printed lines.
// It keeps them beside `go test`'s status and summary lines, and leaves out a test whose subtest failed.
// It keeps each failed package that no failed test explains, with its own first lines and its build's.
// It keeps the first lines that were no JSON event.
// The record carries each failed test with its first line as `failures`, and a `vstest` run keeps none.
// A `go-json` tier under the `go` scoper fails when a package holding a test under the tier's tag ran none;
// the tier's command then lacks its tag.
// A package holds such a test when a file of it has a `Test` function,
// and that file carries a build constraint that holds under `tierN` on the host's GOOS and GOARCH.
// The failure exits 1.
// Its line names the tier, the package and `lym config gate`, and goes to stderr and the log before the summary line.
// A run whose words after `--` carry `-run`, `-skip`, `-list` or `-bench` is exempt, since its filter may match nothing;
// the guard step's own `-run` filter exempts nothing.
// A test the run's watch skip names does not count as a test the package holds.
// Otherwise a tier that discovers no tests passes with zero counted, as a package with only a `TestMain` does.
//
// # Lint steps
//
// A `--gate` key's `lints` run in the key's order after every tier, vet and guard run of an uncancelled call.
// Each runs over the worktree against HEAD, untracked files included:
// `comments` through `commentlint.Lint` and `pattern` through `patternlint.Lint`.
// A step that finds something, or cannot run, is a failed run labelled `lint <name>`.
// Its exit code is 1 when no earlier run failed.
// Its findings go into the failure report with the step's way forward.
// The template's `card` key lists both, so a card gate is one call, and a call with manual flags runs none.
// Bound: a step reads only the working tree against HEAD, so on a clean tree it finds nothing;
// the round and darn gates lint their own range through `loomshed`.
//
// # Failure report
//
// A call with a failed run, a halted call included, prints the failure report to stderr before its closing line.
// One read of the call's result then shows every failure.
// It opens with `gate test: failure report, every failure of this call:`.
// It then names each failed run in plan order, lint steps last.
// Each name carries the run's outcome and its log path, and for a lint step its way forward.
// Under a run each failure follows.
// A failed test shows as its package and test path.
// A package that failed outside any test, such as by a build error or a crash in `TestMain`, shows as its package.
// A lint finding shows as its own `file:line` line.
// Each carries up to `failureLines` lines it printed beside go test's own status and summary lines,
// so an error's got and want or a panic's first frames show.
// A bare file name a test printed is prefixed with its package's worktree-relative directory.
// A build error in a package that failed outside any test may print a path relative to the run directory.
// That path is prefixed with the directory's worktree-relative path.
// A run lists the verb's own cause line, such as a timeout, whether or not a failure was read.
// A run with no failure read lists up to `unparsedLines` output lines no test event carried, such as a `go vet` error;
// it may instead say its log holds the cause.
// A build step's run lists the last `unparsedLines` lines of its output.
// The report holds at most `failureReportLines` lines below its header, each cut at `failureReportLineBytes`.
// Past that a line names the lines each run lost.
// It points at the logs for a run with one and at the standalone lint verb for a lint step, which has no log.
// The report closes with the files the change touches, at most `touchedFilesListed` named, untracked files included.
// The change is the diff from the merge base with the pair's parent branch.
// Outside a hub or without a parent it is the diff from HEAD.
// A failed test's first line is also its run record's `failures` entry.
//
// # Trimpath
//
// A hub's tiers set `-trimpath` in their GOFLAGS, as the example below does,
// so a fresh worktree reuses the build cache.
// Under it `runtime.Caller` reports module-relative file paths,
// so a test that locates its repository from one fails, though a raw `go test` passes.
// The verb writes a line when a tier's last GOFLAGS entry, inherited or the tier's own, holds an enabled `-trimpath`.
// The line names the tier, that GOFLAGS value and the way forward:
// locate the repository from the working directory or `go.mod`.
// The line opens the tier's log, before its first command's output.
// It goes to stderr just before the tier's summary line, so it sits beside a failing run's verdict.
// A tier with an empty scope runs no command and gets no such line, and a refused tier prints only its JSON envelope.
//
// # Test result cache
//
// Every tier command runs with `gateslot.TestCacheOffFlag`, `-count=1`, first in its GOFLAGS.
// It comes ahead of the inherited or tier-set value, which the trimpath line names without it.
// `go test` therefore keeps no test result cache.
// It writes no test log into the build cache either.
// No hub config is needed.
// The green record carries a pass over an unchanged tree.
// A hub's own `-count` in GOFLAGS, or one in the words after `--`, still wins.
//
// # Child lifetime
//
// A tier's shell runs through `internal/proc`'s tied child.
// The tied child keeps the tree killable without signalling the shell that called the verb.
// It ends the tree when the verb dies, a hard kill included.
// On a terminating signal, including one sent to the verb's own group, the verb kills that tree.
// It then releases its slots and exits with 128 plus the signal number.
// Bound: the parent-death signal reaches the shell alone,
// so a command the shell forks rather than replaces outlives a hard kill of the verb;
// a tier command that must die with it starts with `exec`.
//
// A tier's or the build's shell that exits ends its run with its own exit code.
// This holds even while a process it started still holds its output pipe.
// An MSBuild worker node a crashed `dotnet` left is one such process.
// After the exit the verb reads the output for at most the pipe wait delay, 10 seconds in production.
// It then kills what is left of the shell's process group, on Windows by closing the child's kill-on-close job,
// so no leftover holds the pipe or runs on inside the slots.
// Bound: a leftover that left the shell's process group, such as one in a session of its own, is not killed.
// Only the delay bounds its hold on the pipe.
//
// # CPU watchdog
//
// While a tier's command runs, the verb samples its child tree through `proc.TreeCPU` once per period.
// The period is 5 seconds in production.
// A `.test` process that keeps at least 90 percent of one core busy for `busy_quit_sec` gets one SIGQUIT,
// so the Go runtime prints every goroutine's stack,
// and the test run reports the package failed long before the tier's own timeout.
// The window counts whole samples, and a pid's first sample is its baseline.
// Any sample below 90 percent restarts the count.
// The verb prints `gateslot.WatchdogLine` to its stderr after the quit, and a halt names `gateslot.BusyQuitWayForward`.
// Off Linux `proc.TreeCPU` reports `errors.ErrUnsupported`.
// The watchdog then logs that no watchdog applies and ends, and the tier runs as before.
// The watchdog is off when `busy_quit_sec` is 0.
// It is off when the words after `--` carry `-bench` or `-fuzz`, which keep a core busy by intent.
// It is off when the `LYM_GATE_TEST_PID` an outer `lym gate test` sets in the tier's environment names a live ancestor.
// That outer run's watchdog already sees this run's test binaries.
// An empty, unparsable or non-ancestor value is ignored.
// One example is a value a tmux server started by a test carried into its panes after leaving the outer run's tree.
// An inherited slot does not turn it off:
// a verify run holds a slot and passes it to a verify command whose `lym gate test` call is the outermost gate run.
// It watches `.test` processes only, so a `vstest` tier is bounded by its `timeout_min` alone.
//
// # Process cap
//
// While a tier's command runs, the verb also lists, once per period, every process carrying the run's entry.
// The entry is `LYM_GATE_TEST_PID` in the process's environment, and `proc.PIDsWithEnv` does the listing.
// A tmux server a test starts and its panes keep that entry after they leave the command's tree, so they count too.
// When the list passes `runProcessCap`, the verb kills every listed process with SIGKILL.
// It kills again at each later period until the command ends.
// The tier then fails with a line naming the cap and the way forward, and it is never rerun as a resource shortage.
// The cap guards against a test binary that a pinned tmux job or child runs again, re-running its own suite.
// A nested `lym gate test` sets its own entry, so its processes count toward its own cap.
// The nested run itself counts once toward the outer one.
// Off Linux `proc.PIDsWithEnv` reports `errors.ErrUnsupported`, and the cap logs that none applies and ends.
//
// # Example
//
// loomyard declares tiers and a go scoper; this block shows its `gate.yaml` tier section.
// Every tier's `env` puts the cap in GOFLAGS, so every go command in the tier, its build and vet included, takes it.
// Each tier's `LYM_TEST_TIER` picks its tests at run time, and `{test_targets}` keeps `go test` to their packages.
//
//	scoper: go
//	refuse_raw: ["go test"]
//	tiers:
//	  tier0:
//	    about: unit tests
//	    command: go test -json {test_targets} {args}
//	    all_targets: ./...
//	    env: {GOFLAGS: "-trimpath -p={parallel}", LYM_TEST_TIER: "0"}
//	    counter: go-json
//	    parallel: 4
//	    timeout_min: 10
//	  tier1:
//	    about: integration tests with build and vet first
//	    command: go build ./... && go vet {targets} && go test -json {test_targets} {args}
//	    all_targets: ./...
//	    env: {GOFLAGS: "-trimpath -p={parallel}", LYM_TEST_TIER: "1"}
//	    counter: go-json
//	    parallel: 4
//	    timeout_min: 30
//	    prebuilt_lym: true
//	  tier2:
//	    about: tmux tests
//	    command: go test -json {test_targets} {args}
//	    all_targets: ./...
//	    env: {GOFLAGS: "-trimpath -p={parallel}", LYM_TEST_TIER: "2"}
//	    counter: go-json
//	    parallel: 4
//	    timeout_min: 30
//	    prebuilt_lym: true
//	gates:
//	  card: {tiers: [0], scope: card}
//	  batch: {tiers: [0, 1], scope: card}
//	  webster: {tiers: [0, 1], scope: all}
//	  parent-align: {tiers: [0, 1], scope: all}
//	  finalize: {tiers: [0, 1], scope: all}
//	  adopt: {tiers: [0, 1], scope: all}
//	  webster-fix: {tiers: [0, 1], scope: diff}
//	  darn-write: {tiers: [0, 1], scope: diff}
//	  code-burler-fix: {tiers: [0, 1], scope: diff}
//	  pr-review: {tiers: [0], scope: diff}
//
// A tier is written whole with one flow mapping, for example:
// `lym config gate --set 'tiers.tier0={command: "go test -json {targets} {args}", all_targets: ./..., counter: go-json, timeout_min: 10}'`.
//
// # Detached run
//
// `lym gate run-detached <run-dir>` is a hidden internal-audience verb: it runs a gate call that outlasts its caller.
// A run lives in `gateslot.WaitDir(anchor)/detach/<key>/<run id>`, under the never-tracked gate directory.
// Its directory holds these files.
// `request.json` holds the verb, `test` or `card-verify`, the call's words after it and the directory it ran in.
// `lease.json` holds the deadline.
// `process.json` holds the runner's pid and start reading.
// `tree.json` holds HEAD's tree and the sorted dirty paths the front recorded.
// `phase.json` holds `starting`, `slots` or `running`, with the tier name or the card line.
// `line.json` holds the process tree of a card-verify run's line.
// `exit.json` holds the call's code and whether it was abandoned.
// The directory also holds `stdout`, `stderr`, `runner.log` and `delivered`.
// The lease is the latest deadline any call wrote, and only an explicit expiry moves it earlier.
//
// Before it runs anything or writes an exit file, the runner refuses:
// a directory with no live lease, a request that is missing or malformed, a verb it does not run, or a claimed run.
// The refusal names `lym gate test` and `lym gate card-verify` as the way forward.
// The claim is an exclusive create of `process.json`, so it is the one check for a claimed run,
// and a reader that finds the file not yet decodable reads it again.
// Bound: a direct call reaches only a directory a front made and refuses one its runner claimed,
// so a hand-written lease lets one run start, which ends at that lease.
//
// The runner runs the request in-process through a fresh gate tree, as a `--wait` call does.
// Its output goes to `stdout` and `stderr`, and it reports the tier it waits for or runs to `phase.json`.
// A `card-verify` request runs the card's line instead, as the Card verify section says.
// It reads the lease once a second from its start, the slot wait included.
// It cancels the call once the lease is read as lapsed, an unreadable lease being read again a second later.
// A terminating signal cancels the call too.
// A call that ended on its own writes its own code to `exit.json`.
// A lease that lapsed while the call ran appends an abandoned line to `stderr`, naming the request's verb.
// It then writes code 1 and the abandoned mark to `exit.json`.
// A lease that lapsed after the call returned abandons nothing.
// A signal writes 128 plus its number and the abandoned mark.
// The cancelled call kills its tiers and releases its slots as a signal does,
// and the records it writes carry the abandoned mark.
//
// # Capped call
//
// A `lym gate test` call without `--wait` from an agent strand is capped:
// it starts or joins a detached runner and waits for the run's result up to the caller role's keep-warm cap,
// so a run that outlasts the cap keeps the agent's prompt cache warm instead of blocking one long call.
// `frontDecision` makes the choice.
// A call is not capped when it gives `--wait` or when `agentname.StrandNameEnv` is empty.
// It is also uncapped when its directory is in no git worktree or when the resolver's cap is zero.
// A call inside a slot an enclosing gate run holds is uncapped too.
// The cap is the answer of `shuttleengines.KeepWarm` for the strand;
// a fallback answer is logged at Warn once per call and still caps, at its `DefaultAwaitCap`.
// A call that is not capped runs in this process.
// `RunGate` and every call a verify or card-verify line makes are among them and never reach the front.
// Bound: the strand is read from the caller's environment,
// so a forged or absent strand chooses only how long a call blocks, never what runs or its verdict.
// Bound: only a hand-set slot variable beside a strand reaches the inherited-slot branch.
// Every lym path that holds a slot runs its child without the strand.
// The branch stays so the choice is complete,
// and the standing agent deny on the variable keeps an agent from reaching it.
//
// A call's key is a hash of the verb, the resolved run directory and the call's words.
// The key directory `detach/<key>` under the gate directory holds the key's lock and one subdirectory per run.
// It also holds `current.json`, which names the current run id.
// A call from a fixer strand, a role `gatefixer.IsFixerRole` matches, adds its strand name to the hashed words,
// so a fixer's own gate call never joins the run waiting on it and two fixer strands never share one.
// The front parses the strand's role through `strandRole` and writes it to the request as `role`.
// The runner's environment carries no strand name;
// a request without a role reads as one from a strand that is no fixer.
// The front counts its cap from its own start and catches terminating signals.
// A signal ends the front alone with 128 plus the signal number while the runner goes on.
// It takes the key directory's lock, trying once a second only while at least the lock margin of its cap remains;
// past that it answers as at a passed cap in the phase `starting`.
// Under the lock it joins the key's current run when `joinable` holds, else it starts a fresh one.
// A run is joinable only while its lease holds and no front printed its result (`delivered` is absent).
// HEAD's tree and the sorted dirty paths must also equal the run's `tree.json`.
// The run must also have ended without the abandoned mark, or have a live runner.
// A runner is live when its pid is live with the start reading it recorded, so a reused pid is no runner.
// Joining writes the call's deadline plus the grace to the lease, which keeps the later one.
// A fresh run expires the superseded run's lease, so a live runner of it ends at its next lease read.
// The front writes `request.json`, `tree.json` and the lease before launching.
// It points `current.json` at the new id.
// It launches `lym gate run-detached <run dir>` detached, the strand name dropped.
// The front registers the runner under the pair's slug with `runproc.RoleGate` inside a hub,
// so the pair's teardown ends it;
// the prime and a worktree outside every hub register nothing.
// It holds the lock until the runner claimed the run, checking once a second for at most the claim bound.
// A launch that failed, or a runner that ended or did not claim by the bound, is a failed start.
// The front then claims the run itself so no runner can, expires the lease, and prints the runner's log to stderr.
// It exits 1 with an error whose way forward is to run the same command again.
// When that fails again, the way forward is to run it with `--wait`.
// It then waits for the run's `exit.json` for what remains of its cap.
// A result without the abandoned mark is printed: `stdout`, then `stderr`, then `delivered` is created.
// A `stdout` past `replayStdoutBytes` is named on stderr with its size instead of printed.
// Its last line is printed when it is a JSON envelope,
// so the answer reads as one tool result and the failure report on `stderr` carries the failures.
// A failure to copy the output leaves the run joinable.
// The call exits with the recorded code.
// A recorded 75 exits 1 after a stderr line saying the code was a test's own.
// A passed cap, and a result with the abandoned mark, answer ok with `done: false` and the phase from `phase.json`.
// The phase is `starting` when there is none.
// The answer carries a message to run the same command again in a turn that does nothing else.
// While the run waits for its slots it also carries `slot_free: false` with each holder's worktree, site and tier.
// The exit code is SlotBusyExit.
// A fresh run removes the key directory's other run directories older than 14 days whose runner is not live.
// A call with `--gate <key>` may wait one long call in place of many short ones.
// The resolver may give a break-even (`shuttle.yaml`'s `keep_warm_max_reruns`, 0 for none or for a fallback).
// With one, `chooseCap` reads this machine's run records and `longWait` weighs the latest five full calls of the key.
// A full call ran every tier it listed to an outcome and was neither carried, abandoned nor of an empty scope.
// Given at least three, a median wall time above the cap times the break-even sets the cap to the resolver's long cap.
// The long cap is 55 minutes from the claude engine, and the lease takes it too.
// A read error, no key and no break-even keep the short cap.
// Bound: an estimate from a few earlier calls can mis-pick, which costs cache money only, never a verdict.
// Bound: the join compares the tree and the dirty paths, not the dirty files' content,
// so an edit to an already-dirty file between two calls with the same words gets the earlier content's run,
// and a second call with the same words after another took the result starts a fresh run.
// Bound: the plan lives under `_lym`, which neither `headIdentity` nor `verifytree.DirtyPaths` reads.
// After a plan edit, a `--card` call with the same words and an unchanged code tree joins the run of the old plan.
// The join lasts while its lease holds.
// It costs at most the earlier plan's verdict for the remaining lease, after which the same command starts afresh.
//
// # Card fixer
//
// A red `lym gate test --gate <key>` agent call hands its failure to a cold fixer when the key's `fixer` is set.
// The call answers with the re-run's result.
// Config load allows `fixer` on the `card` key alone, so only the Webster card gate hands off;
// only a capped call reaches a runner;
// an operator's call, a `--wait` call and a call inside an enclosing slot never do.
// `handsOff` decides it from the request:
// the verb is `test`, the request's role is no fixer role, the call names a gate key whose `fixer` is non-empty,
// and the pair has a slug.
// `newCardFixer` builds the kit:
// a strand fixer over the worktree's reed geometry, registered under the slug with the gate fixer role.
// The kit carries the model choice the key's `fixer` names, the gate config's fixer timeout and the stencils directory.
// It also carries the hub's language skills.
// A call outside a hub, with no slug, or for a key or role `handsOff` turns down ends as it does without a fixer.
// A kit that cannot be built ends the same way, after the runner logs why.
// `runWithFixer` runs in place of `runRequest`.
// Before the gate runs it waits on a live fixer record of an earlier run in the key directory's `fixer.json`.
// When it cannot attach it stops the strand through `fixerStopper`.
// It marks the record dead only once the stop succeeded.
// When the stop failed it ends with exit 1 and the way forward.
// On a red verdict, once `runRequest` released its slots, it writes the phase `fixing`.
// It writes the stderr report to `fixer-report.md`.
// It spawns one fixer with the shared cold frame and the card body.
// It waits on the fixer, rewrites `tree.json` and runs `runRequest` again.
// It then marks the fixer's record terminal, so a restart during the re-run still finds it live.
// It appends the fixer's reply or the dead note to the re-run's stderr under `## Cold fixer reply`.
// A pass, an abandoned exit and a `card-verify` request end with the first run's result.
// So does a non-zero exit that left nothing on stderr, as a refusal does.
// So does a red verdict whose stderr holds `gateslot.RunnerCrashLine`.
// So does a spawn that failed, with a stderr line saying no fixer ran.
// A passed lease or a terminating signal during the phase stops the fixer.
// The run then ends abandoned, with the fixer's record marked dead once the stop succeeded.
// A run in the phase `fixing` is joinable under changed dirty paths,
// so a caller's repeated call joins whatever tree the fixer is changing;
// from the re-run's start the rewritten `tree.json` holds the rule again.
// Bound: one fixer per red call, which Go bounds;
// the number of red calls per card rests on the Webster fork prompt's self-fix cap.
// Bound: a fixer's own gate call never hands off, and its strand name keys it apart from the call waiting on it.
//
// # Report
//
// `lym gate report [--slug <slug>] [--since <YYYY-MM-DD>]` prints one row per slug and gate key.
// It reads this machine's run records through `gateslot.ReadRunRecords`.
// They come from the board's gate run directory inside a hub and the worktree's own outside every hub.
// A row holds the tier runs, the carried tiers and the summed wall time.
// It holds the summed slot wait, a record's start minus its wait start.
// It holds the launch time with its share.
// The launch time is the sum of the records' launch gaps, and its share is that sum over their summed package time.
// A record from before `--gate` has no key and groups under its tier list and scope form, such as `tiers 0,1 all`;
// a record without a launch list adds nothing to the launch time,
// and a group whose records hold none shows no launch time.
// A record marked abandoned is written after its call was cancelled by a terminating signal or a passed lease.
// It counts only in the row's `abandoned_runs`, `abandoned_wait_seconds` and `abandoned_wall_seconds`.
// It adds nothing to the tier runs, the carried tiers, the wait, the wall time, the build step or the launch totals.
// Every record of one call carries the call's id.
// A malformed `--since` is refused naming the expected form before hub resolution.
// The verb carries the operator audience annotation, takes no slot and changes nothing.
//
// # Card verify
//
// `lym gate card-verify <card id>` runs one plan card's `**Verify:**` line through `internal/cardverify`'s `Runner`.
// Webster's record-batch re-run shares that runner.
// It sits under `lym gate` because the fork-mode hook refuses every `lym webster` command from a fork.
// It carries the role audience annotation because the implementer fork's prompt names it.
// An id is a card's number or its `N-slug`, read from the plan at the anchor's plan directory;
// an unknown id is refused naming the plan's ids,
// and a worktree with no plan is refused naming the way forward:
// run it from the task worktree whose `_lym/plan` holds the card.
// A card with no `**Verify:**` line passes with a message saying so.
// The line's output passes through to the terminal and the verb exits with the line's own code;
// a line that timed out or did not start exits 1.
// Inside a hub a line that is not a `--wait` gate call takes one slot of the hub's pool;
// outside every hub it runs unslotted.
//
// The verb has no `--wait`, so `frontDecision` caps a call from an agent strand as it does a `lym gate test` call,
// and the line runs in a detached runner the call starts or joins.
// The key is the verb `card-verify`, the worktree as run directory and the matched card's `N-slug` id as its word.
// A call with `4` and a call with `4-<slug>` therefore share one run.
// The runner reads the plan again in the request's directory and matches the card;
// a refusal there, as when the plan was edited since the front's read, is written to `stderr` with exit 1.
// The front replays it.
// A card with no line in the runner's read answers the in-process message with exit 0.
// The line runs through a `cardverify.Runner` over the worktree with the hub's pool and wait directory.
// The runner's lease-cancelled context governs the line.
// `phase.json` names `slots` with the line while a slot-held line waits for its slot.
// It names `running` with the line once it runs, a `--wait` gate line included.
// A `--wait` gate line takes its own slots inside its child and answers no `slot_free`.
// The line's process tree goes to `line.json`.
// On Windows the runner first holds the kill-on-close job `lym-gate-<run id>`, which the line's tree inherits;
// on Linux the line's shell carries the parent-death SIGKILL of `verifyrun.RunTied`.
// A cancelled line's process group gets SIGTERM and then SIGKILL,
// and a `--wait` gate child cancels on that SIGTERM, killing its tiers and releasing its slots.
// A front that supersedes a `card-verify` run whose runner is not live first kills the group `line.json` records.
// That reaches the shell's descendants the parent-death signal misses.
// Bound: a job that cannot be held is logged and the line runs without it.
// On Windows the runner's death then leaves the line's tree to its own end, since `proc.KillTree` kills nothing there.
// Bound: on Linux a dead runner's line runs on until a front supersedes it, or to its own end when no front comes.
// Bound: the key is the card's id,
// so a call after an edit of the card's `**Verify:**` line joins the run of the old line while its lease holds.
//
// # In-process run
//
// `RunGate(ctx, dir, gate, out, stderr, observer)` runs the gate key `gate` in `dir`.
// It runs through the same `runTest` as `lym gate test --wait --gate <gate>`, with the CLI's defaults.
// `dir` is seeded as the context's working directory.
// The caller names only the key and the directory, never targets or tiers.
// It gets the call's exit code, the envelope on `out` and the summary lines on `stderr`.
// A `TierObserver` hears each tier of the run through `Started` and `Ended`.
// `Started` gets the tier's number once its slots are held, or at once for a run that takes none.
// `Ended` gets the tier's run record when the tier reports.
// A tier carried as already green ends with no start,
// and a tier blocked by a failed build ends with its blocked record.
// Only the tier whose slots ran the build is started.
// A tier rerun after a crash or a resource shortage ends once, with the rerun's record.
// A guard or vet run reports through the same calls with its record's own flags,
// and a refusal after a start ends with no end.
// A nil func is skipped, and every CLI call passes the zero observer.
//
// # Exit codes
//
// The exit code is the first failing run's own code, in plan order.
// For a test binary the watchdog quit it is the test run's failure code.
// For a runner crash it is 128 plus the signal's number.
// A tier that timed out gives 1.
// SlotBusyExit (75) comes only as a capped call's `done: false` answer.
// That answer is no failure and never the code of a call that runs in this process.
// A run that ended without a failure while the call was cancelled gives 1.
// A terminating signal gives 128 plus the signal number.
// A refusal gives 1, with a JSON error envelope on stdout.
package gatecli
