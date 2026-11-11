// Package gateslot owns the hub-wide pool of gate slots.
// The pool bounds how many gate builds and test runs the hub executes at once.
//
// The package is told every directory it works on.
// Only the Dir, WaitDir and Pool.PrebuiltDir joins derive a directory.
// They spell the ephemeral directory name only through internal/lymdirs.
//
// # Slots
//
// A Pool holds one OS file lock per slot at `<Dir>/slot-<n>.lock`, n from 1 to the current Limits.Slots.
// Acquire takes the first free slot.
// It writes the slot's Holder record (worktree, site, pid, start time) as YAML at `<Dir>/slot-<n>.yaml`.
// While every slot is held it logs the wait start, polls every Pool.Poll and logs the wait end on acquiring.
// Limits is a closure read at every attempt.
// A lowered count takes effect on the next acquire, while a running holder of a higher slot finishes.
// Acquire imposes no deadline: a caller wanting a bound passes a deadline context.
// Polling gives no FIFO order among waiters.
//
// AcquireSlots takes a tier's slots: a count of 1 is Acquire,
// and a count of 0 (`slots: all`) or above the current Limits.Slots is clamped to every slot.
// A multi-slot acquire first takes the drain lock, an OS file lock at `<Dir>/drain.lock`.
// It then takes slots one at a time in ascending order, holding each it has while it waits for the next.
// It releases the drain lock once it holds them all.
// While another process holds the drain lock, Acquire waits, polling as it does for a held slot.
// New single-slot acquires therefore queue behind a waiting multi-slot tier and cannot starve it.
// Ascending acquisition means two multi-slot waiters never deadlock:
// the drain admits one at a time, and every single-slot holder finishes.
// A cancelled context releases every slot and the drain lock already taken.
// The Holder record names the tier run that holds a slot, so a status reader can show it.
//
// An OS lock is released by process death, so a crashed holder never leaks a slot;
// its holder record stays on disk, but Holders trusts a record only while its lock is held.
// Holders reads every slot record in the directory, so a holder of a slot above a lowered count still shows.
// It fills each Holder's slot number from its record's file name, which the record itself never carries.
// Pool.Draining reports whether the drain lock is held, and creates no file in a pool that never had one.
//
// # Inheritance
//
// A slot's environment is Lease.InheritEnv for a tier run and HolderEnv for a non-tier holder.
// `-trimpath` makes builds from two checkouts byte-identical and their cache keys independent of the checkout path.
// A fresh worktree therefore hits the build cache.
// A host scanner that holds each unseen executable then sees known hashes.
// It also makes `runtime.Caller` report module-relative file paths.
// No code a gate run builds can therefore locate the repo root from them alone.
// It does not reach the cgo-generated files the cache keeps.
// They name their source by the absolute path of the checkout that first built them,
// so no package of the module imports "C", and `go vet` never opens another worktree's files.
// Lease.InheritEnv returns the environment of a tier run.
// Only InheritEnv is set, naming the lease's first slot's lock path.
// No GOFLAGS is appended, since a tier's command and env set its own parallelism.
// HolderEnv returns the environment of a non-tier slot holder's child.
// GOFLAGS gets `-trimpath` and, when the holder passes a tier0 parallel above 0, `-p=<n>`.
// It opens with TestCacheOffFlag, `-count=1`, which WithTestCacheOff also puts first in a tier run's GOFLAGS.
// `go test` then keeps no test result cache, and so writes none of the per-test logs of touched files it keys one by;
// the gate's own green record carries a pass over an unchanged tree instead.
// A non-nil lease sets InheritEnv.
// Limits.Tier0Parallel carries `tier0`'s effective parallel, this machine's override included.
// It is 0 when the hub declares no `tier0`, and a holder reads it from the pool's Limits at its acquire.
// A nested gate run checks Pool.Inherited on that variable.
// It runs inside its parent's slot without acquiring, whatever slots its own tier asks for.
//
// The non-tier holders hold one slot and set their own Go flags through HolderEnv.
// Each passes the `-p` value it reads from the pool's Limits at its acquire,
// so a hub that declares `tier0` bounds them by its parallel and a hub that declares none adds no `-p`.
// `lym gate testmap` is one: it holds one slot from its first build to its end.
// The redundancy report step of `lym gate test` is another: it takes one slot after the call's tiers released theirs.
//
// # Prebuilt store
//
// Pool.PrebuiltDir is `<Dir>/prebuilt`.
// A tagged gate run keeps the `lym` it built there under a name derived from the binary's content.
// PrebuiltLymEnv therefore holds the same value for every run on an unchanged tree.
// The gate run that writes the store also prunes it.
//
// # Config
//
// The hub-wide `gate` config module keeps `slots`, `busy_quit_sec` and the tier keys below.
// They live at `<BoardDir>/_lym/config/gate.yaml`, seeded from template.yaml by the hub-wide reconcile.
// `slots` and `tier_parallel` take a machine-local override at `<BoardDir>/.lym/config/gate.machine.yaml`.
// The override is never reconciled and is read at every acquire; every other key stays hub-file only.
// `cli_wait_sec`, `go_parallel`, `guard_tier` and `test_map_max_age_days` are retired keys:
// every load drops them, in the hub and machine layers alike, and reconcile prunes them.
// `lym gate testmap` rebuilds no map for its age, so `test_map_max_age_days` bounds nothing.
// The slot wait of a call that runs in this process has no cap,
// and a capped call's front is bounded by the caller role's keep-warm cap.
// `tier_parallel` replaces `go_parallel`.
// The guard step runs each guard test under the tier its file belongs to, so no key names one tier for them all.
// LoadConfig reads the file strictly and refuses any value below 1 naming its key.
// `busy_quit_sec` is the exception: it accepts 0 and is refused below it.
// Config.Limits converts the config to the Limits a Pool reads.
// An absent hub file is ErrConfigAbsent with the way forward `lym fabric reconcile`.
// Outside every hub, where the dirs tell no board, the file is the worktree's own.
// The way forward is then `lym config gate`, after creating the worktree's `_lym` directory.
// TemplateConfig decodes the template alone, for a run outside every hub.
//
// The tier keys are `tiers`, `gates`, `scoper`, `refuse_raw`, `tier_parallel`, `build` and `inert_paths`.
// `tiers` is an open map keyed `tier0`, `tier1`, and so on, each a Tier.
// A tier is written whole with one `lym config gate --set 'tiers.tierN={...}'` flow mapping.
// It may also be written one key at a time, as `--set tiers.tier0.env.LYM_TEST_TIER=0` or `--set build.timeout_min=10`.
// A key no tier or build step holds is refused.
// `gates` is a per-name map from a gate key to a GateKey written `{tiers: [<n>...], scope: card|diff|coverage|all}`,
// so a hub file that sets one key keeps the template's value for every other.
// ScopeCoverage is the `diff` scope with each importing package narrowed to the tests its coverage map selects.
// The template gives it to `webster-fix`, `darn-write` and `code-burler-fix`, and keeps `pr-review` on `diff`.
// It is a new value, so a hub file holding `diff` keeps its meaning,
// and a binary that predates the value refuses a hub file holding it until it updates.
// `tier_parallel` is an open map from a tier name to this machine's parallel for it.
// A GateKey may also hold `lints`, the lint steps its call runs after its tiers, each LintComments or LintPattern.
// The template's `card` key lists both.
// A GateKey may also hold `reports`, the advisory steps its call runs after its lints, each ReportRedundancy.
// The template lists it on `webster`, `parent-align`, `darn-write` and `code-burler-fix`.
// Config load fills a template key a tuned `gates` entry lacks into that entry,
// so a hub base file that tunes one of those keys without `reports` still runs the step, and `reports: []` turns it off.
// `tiers` and `build` are executable keys, since a tier or build command runs any shell command in a task worktree.
// `gates`, `scoper`, `refuse_raw` and `inert_paths` are guard keys.
// All of them are hub-file only.
// `build` is an open map, so its sub-keys travel whole through the layer stack and reconcile never prunes a hub's build.
// A tier may also hold `filter`, its own test-filter expression.
// It may hold `shared_build: true`, which makes the tier depend on the build step.
// It may hold `carry: false`, which keeps a green run from being carried over an unchanged tree.
// Tier.NoCarry is the negative form, so a zero Tier carries.
// A tier whose command holds TestTimeoutPlaceholder may hold `test_timeout_min`.
// It sets the minutes the placeholder renders for the test runner's per-binary bound.
// Unset, it is DefaultTestTimeoutMin.
// A hub thereby raises `go test`'s `-timeout` default from config instead of a literal in the command.
// A tier's command may hold TestTargetsPlaceholder where its targets reach `go test`.
// A run fills it with the targets narrowed to the packages that hold a test of the tier.
// HoldsTargets counts it as taking targets, so such a tier needs `all_targets` too.
// `build` holds `about`, `command`, `env`, `timeout_min` and `dir`.
// An unset `dir` is the worktree's anchor, filled by `Config.WithDefaultDir`.
// The template's `build: {}` is no build step, and a declared step has a non-empty Config.Build.Command.
// ResolveCommandDir is the base of a `dir` for the drift steps and the hub-wide dir checks:
// relative to the worktree root, absolute as written, and the worktree's anchor when unset.
// The gate's run path reaches the same directory for a tier's and the build's `dir`.
// It fills an unset dir through Config.WithDefaultDir and joins the dir onto the worktree root.
// StatCommandDir refuses a `dir` that resolves to no existing directory, naming the resolved path.
// CheckDirs applies it to every tier and the build, which a hub-wide `lym config gate` write runs against the prime.
// `inert_paths` lists glob patterns of files no test reads.
// Config.Inert matches a worktree-relative slash path through `path.Match`:
// a pattern without a slash against the path's base name, and one with a slash against the whole path.
// A pattern `path.Match` rejects matches nothing.
// Config.AllInert is true only when the list and the paths are both non-empty and every path is inert.
// The `dotnet` scoper derives the impacted C# test projects and classes from the type-name closure over test classes.
// The machine override is a sibling map, not a path under `tiers`.
// An open map is one unit in the layer merge.
// A machine layer holding part of `tiers` would therefore replace all of the hub's tiers.
// A tier's `manual: true` keeps it out of a `lym gate test --upto` range, so it runs only when named by number.
// A declared tier's effective Parallel is its `tier_parallel` entry from whichever layer sets one, else 1.
// A standalone worktree has no machine layer and sets the key in its own file.
//
// LoadConfig refuses at load, naming the key and `lym config gate`:
// a tier lacking `command`, `timeout_min` or `counter`;
// a tier lacking `all_targets` when its command holds `{targets}` or `{test_targets}`;
// `manual: true` on `tier0`, the tier a call with no tier flag runs;
// `parallel` or `timeout_min` below 1, `slots` neither `all` nor at least 1;
// a counter other than `go-json` or `vstest`, a scoper other than `go`, `none` or `dotnet`;
// a tier `filter` whose command holds no `{filter}`;
// a `test_timeout_min` below 1 or on a command that holds no `{test_timeout}`;
// a non-empty `build` lacking `command` or `timeout_min`, a build `timeout_min` below 1;
// a tier with `shared_build: true` while the hub declares no build step;
// a `covers` entry not lower than its tier's number, a tier key not spelled `tier<N>`;
// a `refuse_raw` entry that is not words of letters, digits, `.`, `_` and `-` separated by single spaces;
// a `tier_parallel` entry that is no integer of at least 1;
// a gate key whose scope is not `card`, `diff`, `coverage` or `all`;
// a `card` scope on any key but `card` and `batch`; a lint other than `comments` or `pattern`;
// a report other than ReportRedundancy, `redundancy`.
// A GateKey may also hold `fixer`, a model-spec naming the cold fixer an agent's red call hands its failures to.
// Only the `card` key may hold one; the template sets `sonnet[medium]` there, and an empty value turns the fixer off.
// `fixer_timeout_min` bounds how long the callers of a cold gate fixer wait for it.
// LoadConfig refuses a value below 1, and Config.FixerTimeout returns it as a duration.
// `lym gate testmap` builds coverage maps under two bounds.
// `redundancy_build_timeout_min` bounds each build in minutes.
// `redundancy_test_timeout_sec` bounds each test in seconds.
// The redundancy report step of `lym gate test` takes the two redundancy bounds too, and two of its own.
// `redundancy_max_tests` caps the tests it judges in one call.
// `redundancy_budget_min` bounds, in minutes, its builds and tests once it holds its slot.
// They are cross-pair keys, since every worktree of the hub reads the maps, and LoadConfig refuses a value below 1.
// LoadConfig refuses a `fixer` on any key but `card`, and a `fixer` that `modelspec.Parse` rejects.
// Each refusal names the key and its way forward.
// A `gates` entry still in the retired list shape, such as `card: [0]`, is refused naming `lym fabric reconcile`.
// The reconcile runs MigrateConfig, which migrates it.
// A `tier_parallel` entry for a tier the config does not declare is dropped with a logged warning.
// A tier the hub removes therefore leaves no entry that fails the load.
// Cross-references are checked at use, never at load.
// The template and a hub whose gate keys name an undeclared tier therefore both load.
//
// At use, Config.Gate returns a key's tiers and scope.
// It refuses a hub with no `tier0`; a key the config does not hold;
// an empty tier list; and a listed tier the hub does not declare.
// Config.TierList returns its tiers, and Config.Tier refuses an undeclared number.
// Each refusal names `lym config gate`.
// It closes with the clause that a session that is not the prime reports status: FAILED and the orch runs the steps.
// Config.GateCall renders the `lym gate test --wait --gate <key>` call, with one `--card <id>` per card given.
// Its `--wait` form is for a call Go runs.
// Config.AgentGateCall renders the same call without `--wait`, the form for a call an agent runs.
// Config.AgentGateCallWithMainPackages renders the agent call with `--main-packages` last.
// StopOnFailCall adds `--stop-on-fail` to a call, which ends it at its first failed run.
// FailedTestsCall renders the command that runs a failure report's failed tests alone:
// one `--wait` call per tier, joined by `&&`, each over the tier's packages with a `-run` pattern of its test names.
// An agent strand's capped front answers it with `done: false` and exit 75 when its keep-warm cap passes.
// MigrateConfig prunes the retired `gates.publish` list and `guard_tier` key from a gate.yaml document.
// It also migrates the retired lists, so reconcile rewrites the hub file:
// `landing` becomes `parent-align` and `finalize` with scope `all`;
// it also becomes `darn-write` and `code-burler-fix` with scope `diff`;
// `final` becomes `webster` with scope `all`; `round` becomes `webster-fix` with scope `diff`;
// a `card` or `batch` list becomes a mapping with scope `card`.
// A `merge-in` key is renamed to `parent-align` with its value, and dropped beside an existing `parent-align`.
// A key the file already holds in the new shape keeps its value, and a second pass changes nothing.
// SeedAdoptGate runs over the hub base file alone, through the registry's hub-base step.
// When a hub tunes `finalize` and holds no `adopt`, it adds `adopt` with a copy of `finalize`'s value,
// so the adopt check runs the tiers the hub tuned for the landing.
// While `finalize` is tuned, the template's own `adopt` value cannot be pinned:
// the reconcile drops a template-equal entry, and the next pass seeds it again from `finalize`.
// MigrateConfig never seeds `adopt`: it also runs over a local-layer file, a seed and every run-settings gate object.
// Only the hub base file takes the seed.
// Config.TiersValue renders the tier lines and the Go-read gate keys a prompt takes, or `none` when no tier is declared.
//
// # Tier plan
//
// Each shipped recipe row that runs a test gate reads one key, whose template scope is its default:
//
//	Recipe                         Row                                         Key              Scope
//	loom                           Webster, per card                           card             card
//	loom                           Webster, per batch                          batch            card
//	loom                           Webster, verify fixers                      webster-fix      coverage
//	loom                           Webster, end gate                           webster          all
//	loom                           Parent-Align                                parent-align     all
//	tapestry, darn, sampler        Code-Burler-Fix                             code-burler-fix  coverage
//	tapestry, darn, pawl, sampler  PR-Review                                   pr-review        diff
//	tapestry, darn, sampler        Finalize                                    finalize         all
//	adopt                          lym adopt check <subject>, in its worktree  adopt            all
//	darn                           Darn-Write                                  darn-write       coverage
//	darn                           Darn-Write, Code-Burler-Fix, Finalize       the row's key    the touched trees' targets
//
// The last line holds for a diff wholly under darn's `tree_scope`:
// the row runs its own key's tiers over the targets of the trees the diff touches, in place of the key's scope.
// Darn and tapestry share the keys of the rows they share.
// Their writers' gates differ: Webster's end gate runs whole, and darn's writer runs the diff.
// A recipe row's `gate_key` setting names the key it reads in place of its lower-cased row name,
// and the merge fixer runs the key of the row that spawned it.
//
// # Watchdog and timeout lines
//
// The line the gate test's CPU watchdog prints after a quit is rendered by WatchdogLine and found by FindWatchdogQuit.
// BusyQuitWayForward is the way forward after one.
// The line printed when a tier outlives its `timeout_min` is rendered by TierTimeoutLine and found by FindTierTimeout;
// it names the tier, the timeout and the whole-tier `lym config gate --set` rewrite that raises it.
// BuildTimeoutLine renders the build step's timeout line, naming `lym config gate --set 'build={...}'` as the rewrite.
// FindTierTimeout finds it too and returns `build` as the tier.
// RunnerCrashLine renders the line a gate call closes with when every failed run crashed its test runner.
// That covers the run and its one rerun.
// FindRunnerCrash finds the line and returns the runs it names, so a site tells a runtime's crash from a failing test.
// FailureReportHeader opens the failure report a failing call prints.
// FailedTests reads the report's failed tests, each with the tier of the run that failed it.
// It reports false unless every failure of the report is a named test of a tier or guard run.
// TierRunFailures reads every failure of the report, each opened by its run's label.
// It reports false unless every run the report lists is a tier or guard run.
// It also reports false for a package that failed outside any test, such as by a compile error, which a rerun repeats.
//
// # Run log
//
// RunLogDir is `<gateDir>/runs`.
// Its `.lym` is the board's never-tracked one inside a hub and the worktree's own outside one.
// The records therefore outlive the worktree.
// WriteRunRecord writes one RunRecord per tier run as `<stamp>-tier<N>-<pid>.json`.
// The build step's record, whose `build` field is set, is written as `<stamp>-build-<pid>.json`.
// The run's full output sits beside each record as a `.log`; WriteRunRecord prunes files older than 14 days first.
// ReadRunRecords reads a directory's records for the reports that total them.
// It skips a file that does not decode.
// The record is a domain log that no run reads to proceed.
// Every record of one gate call carries the call's id.
// A record written after the call was cancelled is marked abandoned; a record without either reads with them empty.
// A tier the gate carried forward over an unchanged tree writes its record with `carried` set.
// A tier run that crashed its test runner carries the signal as `runner_crash`.
// The one rerun after such a crash or a resource shortage writes its own record, with `rerun` set.
// A go-json tier's record lists a PackageLaunch per package whose test binary ran;
// a record written without the list reads with none.
// A tier the build step blocked by failing writes its record with `blocked_by` set to the failed build's log path.
// TierSummaryLine renders the one stderr line the gate prints after each tier run.
// It marks a carried tier with `carried` after the scope form.
// BuildSummaryLine renders the build step's line, `gate test: build before tier<N> wall <s>s exit <code> log <path>`.
// No tier summary reader matches it.
// FailedTier finds the first failing tier or build line in a log, with `build` true for a build line.
// A site that keeps the gate's output can thereby name the failing tier and tell a compile break from a test failure.
//
// # Wait records
//
// A waiter that has no verifytree marker keeps a Wait record.
// It is one uniquely named YAML file in WaitDir under its worktree's anchor root.
// WriteWait creates it and the writer removes it; ReadWaits returns only the records whose process is alive.
package gateslot
