// Package gatecli provides the cobra command tree for `lym gate`, whose verb `test` is the sanctioned route for running the hub's test tiers, whose verb `status` shows who holds the hub's gate slots, whose verb `report` totals the run records per gate key and whose verb `card-verify` runs a plan card's `**Verify:**` line.
//
// The module is not standalone-capable and does not go through internal/cliwire: it has no target directory, state, plan or stencils to wire, and it resolves hub membership itself.
//
// # Verb
//
// `lym gate test [-C <dir>] [--gate <key>] [--tier N]... [--upto N] [--card <id>]... [--base <rev>] [--all] [--guards] [--wait] [<targets>...] [-- <args>]` runs the tiers the hub declares in gate.yaml, in ascending tier number, and then the vet step and the guard step below when they are planned.
// A run that fails does not stop the call: every later tier, vet and guard run still runs, so a failure the change under test did not cause, such as one present at the base, hides no later check.
// When a run failed, the verb closes with one stderr line naming each failed run, a vet run as `vet tierN`, a guard run as `guard tierN` and a lint step as `lint <name>`, with its exit code or its finding count, after the failure report the Failure report section describes.
// When no run failed under the `--card` scope form, the verb closes with one stderr line naming each declared tier that no listed tier is or covers, manual tiers included, and saying the pass is not green for them, so a card gate that runs tier0 is never read as fully green;
// a vet or guard run under a tier does not count as running it.
// Bound: the line is output only and changes no exit code or record, and no other scope form prints it, since their tier lists are the coverage the hub chose for that site.
// A tier run that fails without timing out and whose output holds a resource-shortage marker, `resource temporarily unavailable`, `failed to create new OS thread` or `cannot fork` in any case, runs once more inside the same slots after a ten-second delay, and the rerun's result is the tier's;
// a stderr line names the marker, and the rerun's log opens with it.
// A rerun that fails on a marker again keeps its exit code, and the closing line names the run as a resource shortage, an environment error, with the way forward of re-running once the machine's process load drops.
// A tier run that fails without timing out, reports no failing test and exits 128 plus the number of a crash signal, SIGABRT, SIGILL, SIGTRAP, SIGBUS, SIGSEGV or SIGFPE, as a shell reports a runtime that aborted, crashed its test runner and judged no code, so it also runs once more after the same delay;
// a command the shell replaced and the signal ended takes that same exit code.
// A run with no shortage marker is read for a crash, the stderr line names the exit code and the signal, and the rerun's log opens with it.
// Before either rerun starts the first run's record and log are written, its record carrying a crash's signal as `runner_crash`, and the rerun's record carries `rerun` and counts its slot wait as none, since the first run's record holds it.
// A rerun that crashes again keeps its exit code, and the closing line names the run as a runner crash with no failing test.
// When every failed run of a call crashed so, with no failing test, failed build or lint finding beside them, the call closes with `gateslot.RunnerCrashLine` after the line of failed runs, so a site tells a runtime's crash from a failing test.
// Bound: one rerun per tier run, so a test whose own output holds a marker costs one extra run and keeps its verdict, and a test that aborts its own runner, such as a .NET stack overflow, is reported as a crash rather than as a failing test; the build step is not rerun.
// A refusal, a slot wait that does not end with the slots held and a terminating signal still end the call at once.
// `--gate <key>` runs the tiers and the scope the `gates` key of that name holds, and `listedTiers` takes the key's tiers as the listed tiers.
// It goes with none of `--tier`, `--upto`, `--all` and `--base`, which is refused before hub resolution naming the key and the flag to drop, since the key's tiers and scope come from gate.yaml.
// Once the config is loaded, a key the config does not hold, an empty tier list and an undeclared tier are refused with `gateslot.Config.Gate`'s way forward naming `lym config gate`.
// A key of scope `card` takes the `--card` ids as the card form and is refused, naming the key and the way forward of passing `--card <id>` per card, when none is given;
// a key of scope `all` is the `--all` form, and a key of scope `diff` is the computed form.
// `--card` ids beside a key of another scope are ignored, and explicit `<targets>` beside `--gate` replace the key's scope.
// Each run record carries the key as `gate_key`, empty for a call with manual flags.
// `--gate` opens no flag combination the manual flags lack, bar the key's lint steps the Lint steps section describes.
// `--tier` names a tier and may repeat; the default is tier0 when neither `--tier` nor `--upto` is given, and a tier another listed tier covers is dropped and named as covered on stderr.
// `--upto N` adds every declared tier numbered 0 through N to the `--tier` values, skipping a gap in the numbers and a tier marked `manual`, which it names on stderr unless `--tier` names it.
// A manual tier runs only when `--tier` names it, so a tier that spends tokens never starts from a range; a negative N is refused before hub resolution.
// `--tags` stays a registered flag only to be refused before any slot, naming `--tier`: a tier's own command sets its build tags.
// The words after `--` fill each tier's `{args}`.
//
// A tier is one shell command with the placeholders `{targets}`, `{parallel}`, `{args}`, `{filter}` and `{test_timeout}`, which `renderTierCommand` fills with each target and each arg quoted for the platform shell by `verifyrun.QuoteArg`.
// `{test_timeout}` renders as the tier's `test_timeout_min` followed by `m`, such as `20m`, so a hub's `go test -timeout {test_timeout}` takes its per-binary bound from config.
// `{filter}` renders as `--filter ` and the quoted expression, or as nothing when there is no expression.
// With no class terms the expression is the tier's `filter`;
// with the terms of an invocation the dotnet scoper narrowed to one project it is `(<filter>)&(<term>|<term>...)`, or the terms alone when the tier has no filter.
// Terms that push the expression past `greenCommandBudget` are dropped and the tier's `filter` stands alone, so that project runs whole within itself, and an Info log names the tier.
// A command without `{filter}` renders no filter whatever terms it is given.
// A tier's `filter` applies at whole scope too, landing gates included, bounded by the executable, hub-file-only `tiers` key.
// The command runs through the shell `verifyrun.ShellFor` names, as a tied child in the run directory, with this process's environment less the strand-name and trace-id variables, less an inherited `gateslot.PrebuiltLymEnv`, plus the variables of the `env` config module (`internal/envconfig`), the hub's with this machine's override inside a hub and the worktree's own outside every hub, replacing an inherited value, plus `proc.BuildServerOffEnv`, replacing an inherited or env module value, so the module never undoes a build-server switch, plus the tier's `env` with `{parallel}` filled, replacing an env module value, plus the lease's `gateslot.InheritEnv` and the enclosing-gate pid.
// The tier's `dir`, relative to the worktree root, is the run directory, and a tier that sets no `dir` runs in the worktree's anchor, filled by `gateslot.Config.WithDefaultDir` once the config is loaded, so a hub anchored at a subdirectory need not spell it per tier.
// A tier with `prebuilt_lym` first builds `lym` at the tier's `parallel`, inside its slots and timeout, only where the worktree root holds `cmd/lym`, and exports `gateslot.PrebuiltLymEnv`.
// Inside a hub the binary is copied into the pool's store (`gateslot.Pool.PrebuiltDir`) as `lym-` and the first 16 hex digits of its SHA-256, the build directory is removed, and the variable names the stored copy;
// outside a hub the variable names the build directory's binary, which is removed after the tier.
// Go keys a cached test result on the variable's value, so a path named by the binary's content keeps a re-run on an unchanged tree cached, and a stored binary already present is reused with its modification time set, marking its last use.
// Each store prunes the other entries unused for two hours.
//
// # Scope
//
// There are five scope forms, and a call takes one: explicit `<targets>`, `--all`, `--card <id>`, `--base <rev>` and, with none of them, the computed form.
// Explicit targets replace each run's `{targets}` verbatim and run in the `-C` directory, else in the tier's `dir`.
// `--all` sets `{targets}` to the tier's `all_targets`, split at spaces, and runs in the tier's `dir`.
// A command without `{targets}` runs once, whole, under explicit targets and under a non-empty narrower scope, and says so on stderr.
// Flag-shape refusals need no config and run first, before hub resolution: `--tags`, naming `--tier`; a negative `--upto`; `-C` without explicit targets, naming them; `--gate` beside `--tier`, `--upto`, `--all` or `--base`; and more than one scope form.
//
// # Scoper
//
// `computeScope` resolves the computed, `--base` and `--card` forms inside the pre-tier ceiling, and `gate.yaml`'s `scoper` decides what they mean.
// The computed form takes its base per listed tier.
// A tier's base is the newest commit that is an ancestor of HEAD, read through `gitrepo`'s `IsAncestor`, and that the green record holds whole-green for the tier under its current fingerprint, with empty args and the tier's module directory (`greenRecord.newestWholeAncestor`).
// A narrowed green never becomes a base, and neither does a whole entry recorded under `-- -run X` or under a since-changed tier command.
// Without such a commit the base is the merge base of HEAD with the parent branch the pair's fabric origin record names, read through `parentMergeBase`; a tier with neither, such as the prime on `main` or a standalone run with no green ancestor, runs whole.
// A dirty worktree reads no record, so it takes the merge base.
// Tiers that land on the same base share one `impactset.DeriveTargets` derivation, and each derivation prints `gate test: diff scope from <base> for <tiers>` on stderr.
// `--base` names one base for every tier.
// `--card`, repeatable, reads the plan at the anchor's plan directory through `planparser.ParsePlan` and takes `planparser.CardPackages` of the named cards; an id is a card's number or its `N-slug`, an unknown id is refused before any slot naming the plan's ids, and a worktree with no plan is refused naming `--all` and explicit targets.
// The plan read and its two refusals, held by `readCards`, run first under every scoper.
//
// Under the `none` scoper each of the three forms is whole, so a tier takes its `all_targets`, and no guard step is planned, except under `inert_paths` below.
// Under the `dotnet` scoper `dotnetScope` derives the forms through `dotnetimpact`.
// A computed or `--base` scope groups the listed tiers by base as above and runs `dotnetimpact.DeriveDiff` once per distinct base and tier `dir`, printing `gate test: diff scope from <base> for <tiers>` per derivation, and a tier with no base runs whole.
// A `--card` scope runs `dotnetimpact.DeriveCards` once per distinct tier `dir` over `planparser.CardCSharpSeeds` of the cards.
// Both are told `gateslot.Config.Inert` as the inert predicate.
// A derivation that falls back gives its tiers a whole share and prints `gate test: the scope falls back to whole: <reason>`.
// A narrowed share runs each selected test project once, in the tier's `dir`, with the project path as `{targets}` and its class terms rendered into `{filter}`, under one slot hold and one `timeout_min`.
// A command without `{targets}` runs once, whole, with its tier filter only.
// Two scopes are empty and tell apart: nothing testable changed, such as an inert-only diff, which makes no invocation as under `go`, and changes that no test class names, which runs a tier with `shared_build` as no invocation and any other tier whole, naming it on stderr, so a compile break still fails the gate.
// The scope plans no guard step and no vet step, and `--guards` is refused naming the scoper and the way forward of dropping it, since no config change gives a C# hub the guard tests.
// Green record: a narrowed project invocation is partial and writes no entry, even when its target equals the tier's `all_targets`, so a filtered pass never stands in for a whole run; a whole `dotnet` pass writes its whole entry as under `none`, and only whole entries carry.
// The class closure skips every test class it does not select, so it misses only references that never spell a changed type's name, and the gates that run whole, such as `parent-align` and `finalize`, catch those.
// Under the `go` scoper a computed or `--base` scope is `impactset.DeriveTargets` of the diff from its base to the working tree, and falls back to whole wherever it does, naming the reason on stderr;
// a `--card` scope is the cards' packages.
// A narrower scope runs each listed tier once per module, in that module's directory with its module-relative targets, under one slot hold and one `timeout_min` covering every invocation.
// An empty scope, such as a diff of Markdown files only, runs no listed tier's command, bar a tier with `carry: false`: each other listed tier passes with zero discovered, is logged so, takes no slot, and only the guard step runs; a `shared_build` tier that only has the build to run takes its slots for it, as the Build step section says.
// A tier with `carry: false` runs whole over its `all_targets` in its `dir` under every scope form but explicit targets, the empty scope included, so a timing or capture tier never goes unrun on a computed scope.
// The import graph cannot see everything a narrower run skips, such as `go build ./...`, so that is left to the whole runs at `final` and `landing`.
// Once the config is loaded, `planTierRuns` refuses, before any slot and naming `lym config gate`: words after `--` for a tier whose command lacks `{args}`, an undeclared tier, a hub with no `tier0`, a `covers` entry naming a tier the hub does not declare, and, for the guard step below, a guard group whose tier the hub does not declare.
// Every such refusal closes with the clause that a session that is not the prime takes none of the steps, reports status: FAILED and leaves them to the orch.
//
// # Inert paths
//
// A hub's `inert_paths` lists the patterns of files no test reads; `gateslot.Config.Inert` is the one matcher, and `allInertSince` reads the files that differ between a base and the working tree, untracked files included, and holds them inert when every one matches.
// An empty diff is not inert-only, and a read error is logged at Warn and holds the diff not inert, so the scope stays as it would be.
// Under the `go` and `none` scopers a computed or `--base` scope checks each base group's diff before deriving it, and a group that is inert-only gets the empty share, so none of its tiers runs a command but a `carry: false` tier, which runs whole.
// Under `go` the derivation is skipped for that group, and a call whose groups are all inert-only plans the guard step over a module scan of `tier0`'s dir, as a whole tier does.
// Under `none` the empty share is the only change, every other group and tier staying whole, and the scope plans no guard step.
// Under `dotnet` the derivation drops inert paths itself.
// A `--card` scope is not narrowed by `inert_paths`.
// A whole gate carries over inert-only history: a whole invocation of a tier that carries, with no entry on the current tree, is carried when the newest whole entry of an ancestor under the tier's fingerprint and the same module has an inert-only diff to the working tree, and a whole entry is then written for the current tree so a later gate carries it directly.
// The carry runs only on a clean tree, and the inert list is part of the fingerprint, so a changed list voids it.
//
// Bound: a test that reads an inert file is skipped when only that file changes, so a hub lists only files no test reads.
//
// # Build step
//
// A hub's `build` declares a shell command, run once per call, that the tiers with `shared_build: true` depend on.
// The first such tier in plan order whose run takes slots, one with invocations left after the green record's subtraction or one with only a build to run, runs `runBuild` inside its slots, before its own command, and no later tier runs it again.
// A call whose opted-in tiers are all carried, or whose shares are empty, such as an inert-only, all-inert-card or empty diff, runs no build, bar a `carry: false` tier, which runs whole and so runs the build before its command.
// A `shared_build` tier whose share selected no test class has no invocation and takes its slots only to run the build, so a compile break still fails the gate; it then passes with nothing discovered.
// The command runs through the platform shell as a tied child in the build's `dir`, with the call's environment, the env config module's variables included, the slot's inherited variable and the build's `env`, which replaces an env module value; the words after `--` never reach it, and lym adds no switch to the command.
// A build that sets no `dir` runs in the anchor, as a tier does.
// Its output passes to stdout and into its log, and it is bounded by its own `timeout_min` counted from the hold, so the tier's `timeout_min` starts after it.
// A timeout kills the tree, exits 1 and prints and logs `gateslot.BuildTimeoutLine`.
// A shell that cannot start is refused naming the build's `dir`, halts the call with exit 1 and writes no build record.
// The build writes a run record, `build` true and scope form `build`, for the tier it precedes, and prints `gateslot.BuildSummaryLine` on stderr.
// A failed or timed-out build is a failed run counted before that tier, and the call exits with the first failed run's code in plan order.
// Every tier that opts into the build from there on runs no command: it writes a run record with outcome failed, the build's exit code and `blocked_by` the build's log, prints `gate test: tierN did not run, since the build failed; see <log>` and no tier summary line, and the closing line names it blocked by the build.
// Tiers that do not opt in still run.
// `lym gate report` totals build records apart, in `build_runs` and `build_seconds`.
//
// Bound: a `shared_build` tier whose command lacks `--no-build` builds again inside its own run, since lym does not read the command.
//
// # Vet step
//
// Under the `go` scoper a `--card` scope also plans, after the listed tiers and before the guard step, one vet run per declared tier whose build tag `tierN` brings a `.go` file into a card package that a build without tags leaves out (`tagTiers`), ascending, leaving out a listed tier and a tier a listed tier covers, since their own commands compile under that tag.
// So a card that breaks the compile under another tier's tag, such as a helper a `tier0` file defines and a `tier1` file uses, fails at its own gate instead of at the batch gate's `go vet` steps.
// A vet run is `go vet -tags tierN` over the card's packages, once per module in that module's directory, with the go command the prebuild uses, under the tier's `env`, `parallel` and `timeout_min` and holding one slot.
// Its summary line and run record carry the scope form `vet`, it takes part in the green record under its own fingerprint, and a failed vet run stops no later run.
// Bound: a break visible only under a combination of tags, or under a tag that adds no file to a card package, is not vetted here and stays the batch gate's to find.
//
// # Guard step
//
// With `--guards`, and under the `go` scoper after any computed, `--base` or `--card` scope, the verb runs, after the listed tiers, the step `guardStepRun` builds over the module's `//lym:guard` tests outside the scope.
// `impactset.DeriveTargets` names those packages for a narrower computed or `--base` scope, and an `impactset.ModuleGuardTests` scan of the module at the run directory, which is `tier0`'s dir or the `-C` directory of explicit targets, names them for a `--card` scope, a scope fallen back to whole, and `--guards` over explicit targets.
// So the round and darn writer gates, which run `--base`, keep the guard coverage they have had, where every guard package counts as outside a tier that does not compile it.
// The scan reads each guard test's tier from its file's `tierN` build tag, a file naming none counting as `tier0`, and the step plans one guard run per tier holding guard tests, ascending.
// A guard run is one run of that tier's command with the group's guard packages as `{targets}` and `-run '^(<names>)$'` after any words from `--` as `{args}`, so the guard's own pattern is the last `-run` and wins over one among those words.
// It acquires that tier's slots after the last listed tier and vet run released their own, runs under that tier's `timeout_min` and `prebuilt_lym`, and is logged as a guard step; a failed guard run, like a failed listed tier, stops no later run.
// A group is skipped when the call ran its tier, or a tier covering it, whole, and a group whose tier the green record holds green carries.
// A group whose tier is undeclared, or whose command lacks `{targets}` or `{args}`, is refused before any slot, naming `lym config gate`.
// A scan error refuses the call before any slot with the file, the line when there is one and a way forward for its kind:
// move a misplaced marker directly above its `func Test…` line, move a marked test out of a `tier2` or `tier3` file into a `tier0` or `tier1` file, or make an unparsable or unreadable file readable and parseable.
// Any other scan error, such as an unreadable package directory, names the module directory and the way forward to make `go list ./...` succeed there.
// `--guards` under the `none` or `dotnet` scoper is refused naming the scoper, since the scan is Go's.
// A module with no marked test runs no guard step and logs so.
//
// # Hub resolution
//
// The directory is resolved with lymcwd.ResolveWorktree, which finds the containing worktree from any directory inside it, a nested module directory included, then preflight.BoardLymPresent.
// It does not use the standalone-capable CLIs' mode resolution, whose anchor gate would refuse a nested module directory.
// Inside a hub the config is `gateslot.LoadConfig` over the hub, this machine's override included; an absent gate.yaml is refused with a way forward naming `lym fabric reconcile`, an invalid one with an error that names each file in the stack and its way forward, and a hub with no tier is refused naming `lym config gate`.
// Outside every hub, in a git worktree whose hub carries no board, the worktree's own gate.yaml is read through the same loader with only the anchor told, and the run is unslotted; with none, the call is refused naming `lym config gate`.
// A directory in no git worktree has no worktree layer to read, so it is refused naming `lym config gate` run in a git worktree.
//
// # Slots
//
// A tier first takes the slots it asks for through `gateslot.Pool.AcquireSlots`, with a holder that names the tier, writes a wait record under the worktree's wait directory while it waits, and releases them before the next tier.
// The wait of a call that runs in this process has no cap: it ends when the slots free, or when the caller interrupts the call, which cancels the wait and exits 128 plus the signal number.
// Bound: a holder is bounded as before, a gate tier or the build step by its `timeout_min` and a slot-held card-verify line by the runner's per-line timeout, and any other slot taker of the pool, such as a drift step run or a plan-glyph type load, only by its own run.
// A capped call's front is bounded by the caller role's keep-warm cap instead, as the next section says.
// A tier with `slots: all` holds every slot and waits behind the pool's drain.
// Outside a hub the tiers run unslotted.
//
// `lym gate status` reads the pool of the hub that contains the working directory and changes nothing.
// It answers an ok envelope with `in_hub`, the pool's `slots` count, `draining`, whether a multi-slot acquire holds the drain so new single-slot runs wait while a slot is free, and `held`, one entry per held slot, oldest first, with its `slot` number, `worktree`, `site`, `tier`, `pid` and `started` time.
// Outside every hub it answers `in_hub: false` and nothing held.
//
// # Inheritance
//
// When the slot variable gateslot.InheritEnv names a slot of the pool that is held, the run goes inside that slot without acquiring, whatever slots its tiers ask for, so a verify command that calls `lym gate test` cannot deadlock.
// Bound: a forged variable naming any held slot runs unslotted inside that holder's slot.
// The standing agent deny refuses an agent command that names the variable, and an operator shell or a script that hides the name is outside the bound.
//
// The prebuilt-binary variable is set or stripped on every path: a run that exports no fresh build removes an inherited value from the tier's environment through `gateslot.StripPrebuilt`.
// A forged value therefore never reaches a gated run, and can mislead only an ungated raw run.
//
// # Green record
//
// `lym gate test` carries a green tier forward over an unchanged tree, at every site and manual run alike.
// The record is one JSON file per worktree, `green.json` in the worktree's never-tracked gate directory, `gateslot.WaitDir` of the anchor, read tolerantly:
// an unreadable or malformed file reads as empty.
// An entry holds the tier fingerprint, HEAD's tree and commit, the module directory, the args after `--`, and either `whole` or the green package set, plus its write time.
// `tierFingerprint` hashes the tier's declaration without `parallel`, `timeout_min`, `test_timeout_min` and `slots`, so a changed command, env, targets, counter, `filter`, `shared_build` or `carry` never carries.
// It also hashes the hub's `build` declaration for a tier with `shared_build` and the hub's `inert_paths` when it is non-empty, so a changed build or inert list stops the tiers it concerns from carrying.
// A key at its default value adds nothing to the hash, so a tier that sets none of these keeps its fingerprint and its green entries.
//
// Before the first tier the verb checks the worktree with `verifytree.DirtyPaths`: a dirty worktree neither reads nor writes the record.
// On a clean one it reads HEAD's tree and commit, and before acquiring slots each listed tier subtracts what `greenRecord.covered` holds green for the current tree:
// a whole entry covers every target, and under the `go` scoper a package entry covers its packages, so a remainder runs as explicit packages; under the `none` scoper only a whole entry carries, and so under the `dotnet` scoper, whose pass narrowed to a project and its classes records nothing.
// An entry with empty args covers a request with empty args and the guard step's own `-run` filter; any other args need an exact match, so `-- -count=1` reruns.
// A remainder whose targets would exceed a shell-safe length, or whose packages `go list` cannot list, runs whole instead.
// A tier with nothing left is carried: it takes no slot, runs no command, writes a run record with `carried` set, and prints its summary line with `carried` after the scope form and exit 0, so every reader of the line still parses it.
//
// A pass is recorded only when HEAD's tree is unchanged and the worktree still clean at that point, so a tree that changes during a run records nothing.
// A tier whose carried part and passed remainder together cover every package its `all_targets` expand to, listed once with `go list` under the tags the tier's command names, writes a whole entry; otherwise it writes the packages it ran.
// The guard step writes its entries under its own `-run` args, so such an entry never covers an empty-args request and an exact repeat of the guard group carries.
// Writers serialize through a lock file beside the record, and each write keeps the current tree's entries and each fingerprint's 20 newest whole-green commits, so an older ancestor stays findable.
// Only a whole entry with empty args counts toward the 20 and outlives its tree, since only such an entry can serve as a base.
// The carry sits under `verifytree`'s command-and-tree skip, which every site keeps.
// The record only saves time, so a failure around it decides nothing: an unreadable record, a failed lock, a failed status or HEAD read is logged at Warn and every tier runs, and a failed write is logged and leaves the tier's outcome and exit code unchanged.
//
// A tier with `carry: false` is never carried: `subtract` keeps every invocation of it and `recordPass` writes no entry for it, so it runs again on an unchanged tree.
// Bound: that is the record's rule alone; a verify site's verified-tree record still skips a command over a tree it passed on, explicit targets still narrow such a tier, and a listed tier whose `covers` names it still drops it.
//
// Bound: a carried tier trusts a pass over the same tree, tier declaration and args on this worktree, so a flaky test that passed once is not rerun.
// A Go toolchain change, inherited environment variables outside the tier's `env` and ignored files such as `.dev-bin` are outside the fingerprint and the clean check.
// The record never leaves the machine, so an adopted run tests afresh.
// A forged or hand-edited whole entry carries every later gate on that tree, Parent-Align and Finalize included, which is accepted because a session that can write the worktree can change the code and tests under review, so the record guards only honest repeats and no further deny is added, since `cp` and `tee` would still reach it.
//
// # Timeouts
//
// The tier's `timeout_min`, counted once its slots are held, bounds the prebuild and every invocation of the tier's command.
// A tier that outlives it has its process tree killed through the tied child's context and fails with exit code 1 and `gateslot.TierTimeoutLine` on stderr and in its log.
// The work before the first tier is bounded by a ceiling, 10 minutes in production, and fails naming the step and the way forward to run the same command again, which a transient hang such as a stalled `go list` passes.
// The length of a `timeout_min` minute, the ceiling and the pipe wait delay the Child lifetime section names are parameters of `newCommand`, so a test shortens them.
//
// # Counters, record and log
//
// A tier's output goes through its counter, `go-json` or `vstest`, which passes the text on and counts the tests it reports.
// After each tier the verb prints `gateslot.TierSummaryLine` to stderr and writes the run record and log through `gateslot.WriteRunRecord` under `gateslot.RunLogDir`:
// the board's gate directory inside a hub, so the records outlive the worktree, and the worktree's own `.lym` gate directory outside every hub.
// The record's slug is the one the pair's fabric origin record names, empty for the prime and outside every hub.
// A write failure is logged and decides nothing.
// The `go-json` counter also keeps, per package, the gap from its `start` event to its next event and its closing event's elapsed time, and the record carries them as `launches`; a cached package and one with no test files are left out, and a `vstest` run records none.
// The `go-json` counter also keeps, per package, the number of tests that ran in it, and a `vstest` run keeps none.
// The `go-json` counter also keeps each failed test with the first lines it printed beside `go test`'s status and summary lines, leaving out a test whose subtest failed, each failed package no failed test of it explains with its own and its build's first lines, and the first lines that were no JSON event, for the failure report; the record carries each failed test with its first line as `failures`, and a `vstest` run keeps none.
// A `go-json` tier under the `go` scoper fails when a package holding a test under the tier's tag ran none, since the tier's command then lacks its tag: a package with a `Test` function in a file whose build constraint holds under `tierN` on the host's GOOS and GOARCH.
// The failure exits 1 and its line, naming the tier, the package and `lym config gate`, goes to stderr and the log before the summary line.
// A run whose words after `--` carry `-run`, `-skip`, `-list` or `-bench` is exempt, since its filter may match nothing; the guard step's own `-run` filter exempts nothing.
// Otherwise a tier that discovers no tests passes with zero counted, as a package with only a `TestMain` does.
//
// # Lint steps
//
// A `--gate` key's `lints` run after every tier, vet and guard run of a call that was not cancelled, in the key's order, each over the worktree against HEAD, untracked files included: `comments` through `commentlint.Lint` and `pattern` through `patternlint.Lint`.
// A step that finds something, or cannot run, is a failed run labelled `lint <name>` with exit code 1 when no earlier run failed, and its findings go into the failure report with the step's way forward.
// The template's `card` key lists both, so a card gate is one call, and a call with manual flags runs none.
// Bound: a step reads only the working tree against HEAD, so on a clean tree it finds nothing; the round and darn gates lint their own range through `loomshed`.
//
// # Failure report
//
// A call with a failed run, a halted call included, prints the failure report to stderr before its closing line, so one read of the call's result shows every failure.
// It opens with `gate test: failure report, every failure of this call:`, then names each failed run in plan order, lint steps last, with its outcome, the way forward of a lint step and its log path.
// Under a run each failure follows: a failed test as its package and test path, a package that failed outside any test, such as a build error or a crash in `TestMain`, as its package, and a lint finding as its own `file:line` line.
// Each carries up to `failureLines` lines it printed beside go test's own status and summary lines, so an error's got and want or a panic's first frames show, and a bare file name a test printed is prefixed with its package's worktree-relative directory.
// A path a build error prints relative to the run directory, in a package that failed outside any test, is prefixed with that directory's worktree-relative path.
// A run lists the verb's own cause line, such as a timeout, whether or not a failure was read; a run with no failure read also lists up to `unparsedLines` lines of its output that no test event carried, such as a `go vet` or `go build` error, or says its log holds the cause; a build step's run lists the last `unparsedLines` lines of its output.
// The report holds at most `failureReportLines` lines below its header, each cut at `failureReportLineBytes`; past that a line names the lines each run lost, points at the logs for a run with one and at the standalone lint verb for a lint step, which has no log, and the report closes with the files the change touches, the diff from the merge base with the pair's parent branch, or from HEAD outside a hub or without a parent, untracked files included, at most `touchedFilesListed` named.
// A failed test's first line is also its run record's `failures` entry.
//
// # Trimpath
//
// A hub's tiers set `-trimpath` in their GOFLAGS on purpose, as the example below does, so a fresh worktree reuses the build and test cache.
// Under it `runtime.Caller` reports module-relative file paths, so a test that locates its repository from one fails, though a raw `go test` passes.
// When the last GOFLAGS entry of a tier's environment, the inherited one or the tier's own, holds an enabled `-trimpath`, the verb writes a line naming the tier, that GOFLAGS value and the way forward of locating the repository from the working directory or `go.mod`.
// The line opens the tier's log, before its first command's output, and goes to stderr just before the tier's summary line, so it sits beside a failing run's verdict.
// A tier with an empty scope runs no command and gets no such line, and a refused tier prints only its JSON envelope.
//
// # Child lifetime
//
// A tier's shell runs through `internal/proc`'s tied child, which keeps its tree killable without signalling the shell that called the verb and ends it when the verb dies, a hard kill included.
// On a terminating signal, including one sent to the verb's own group, the verb kills that tree, releases its slots and exits with 128 plus the signal number.
// Bound: the parent-death signal reaches the shell alone, so a command the shell forks rather than replaces outlives a hard kill of the verb; a tier command that must die with it starts with `exec`.
//
// A tier's or the build's shell that exits ends its run with its own exit code, even while a process it started still holds its output pipe, such as an MSBuild worker node a crashed `dotnet` left.
// The verb reads the output for at most the pipe wait delay after the exit, 10 seconds in production, and then kills what is left of the shell's process group, on Windows by closing the child's kill-on-close job, so no leftover holds the pipe or runs on inside the slots.
// Bound: a leftover that left the shell's process group, such as one that started a session of its own, is not killed, and only the delay bounds its hold on the pipe.
//
// # CPU watchdog
//
// While a tier's command runs, the verb samples its child tree once per period, 5 seconds in production, through `proc.TreeCPU`.
// A process whose path ends in `.test` that keeps at least 90 percent of one core busy for `busy_quit_sec` gets one SIGQUIT, so the Go runtime prints every goroutine's stack and the test run reports the package failed long before the tier's own timeout.
// The window counts whole samples, a pid's first sample is its baseline, and any sample below 90 percent restarts the count.
// The verb prints `gateslot.WatchdogLine` to its stderr after the quit, and a halt names `gateslot.BusyQuitWayForward`.
// Off Linux `proc.TreeCPU` reports `errors.ErrUnsupported`, the watchdog logs that no watchdog applies and ends, and the tier runs as before.
// The watchdog is off when `busy_quit_sec` is 0, when the words after `--` carry `-bench` or `-fuzz`, which keep a core busy by intent, and when `LYM_GATE_TEST_PID` names a live ancestor, the pid an outer `lym gate test` sets in the tier's environment, since that run's watchdog already sees this run's test binaries.
// An empty, unparsable or non-ancestor value is ignored, such as one a tmux server a test started carried into its panes after leaving the outer run's tree.
// An inherited slot does not turn it off: a verify run holds a slot and passes it to a verify command that calls `lym gate test`, which is the outermost gate run there.
// It watches `.test` processes only, so a `vstest` tier is bounded by its `timeout_min` alone.
//
// # Example
//
// loomyard declares four tiers and a go scoper; this block is its `gate.yaml` tier section.
// Every tier's `env` puts the cap in GOFLAGS, so every go command in the tier, its build and vet included, takes it.
//
//	scoper: go
//	refuse_raw: ["go test"]
//	tiers:
//	  tier0:
//	    about: offline tests
//	    command: go test -json -tags tier0 {targets} {args}
//	    all_targets: ./...
//	    env: {GOFLAGS: "-trimpath -p={parallel}"}
//	    counter: go-json
//	    parallel: 4
//	    timeout_min: 10
//	  tier1:
//	    about: integration tests, with build and vet under each tag first
//	    command: go build ./... && go vet -tags tier1 {targets} && go vet -tags tier2 {targets} && go vet -tags tier3 {targets} && go test -json -tags tier1 {targets} {args}
//	    all_targets: ./...
//	    env: {GOFLAGS: "-trimpath -p={parallel}"}
//	    counter: go-json
//	    parallel: 4
//	    timeout_min: 30
//	    covers: [0]
//	    prebuilt_lym: true
//	  tier2:
//	    about: tmux tests
//	    command: go test -json -tags tier2 {targets} {args}
//	    all_targets: ./...
//	    env: {GOFLAGS: "-trimpath -p={parallel}"}
//	    counter: go-json
//	    parallel: 4
//	    timeout_min: 30
//	    covers: [0]
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
//	  drift-verify: {tiers: [0, 1], scope: diff}
//	  pr-review: {tiers: [0], scope: diff}
//
// A tier is written whole with one flow mapping, for example `lym config gate --set 'tiers.tier0={command: "go test -json {targets} {args}", all_targets: ./..., counter: go-json, timeout_min: 10}'`.
//
// # Detached run
//
// `lym gate run-detached <run-dir>` is a hidden internal-audience verb: the runner of a gate call that outlasts its caller.
// A run lives in `gateslot.WaitDir(anchor)/detach/<key>/<run id>`, under the never-tracked gate directory.
// Its directory holds `request.json` (the verb, `test` or `card-verify`, the call's words after it and the directory it ran in), `lease.json` (the deadline), `process.json` (the runner's pid and start reading), `tree.json` (HEAD's tree and the sorted dirty paths the front recorded), `phase.json` (`starting`, `slots` or `running`, with the tier name or the card line), `line.json` (the process tree of a card-verify run's line), `stdout`, `stderr`, `runner.log`, `exit.json` (the call's code and whether it was abandoned) and `delivered`.
// The lease is the latest deadline any call wrote, and only an explicit expiry moves it earlier.
//
// The runner refuses, before it runs anything or writes an exit file, a directory with no live lease, a request that is missing or malformed, a verb it does not run and a run another runner already claimed, naming `lym gate test` and `lym gate card-verify` as the way forward.
// The claim is an exclusive create of `process.json`, so it is the one check for a claimed run, and a reader that finds the file not yet decodable reads it again.
// Bound: a direct call reaches only a directory a front made and refuses one its runner claimed, so a hand-written lease lets one run start, which ends at that lease.
//
// The runner runs the request in-process through a fresh gate tree exactly as a `--wait` call, its output in `stdout` and `stderr`, and reports the tier it waits for or runs to `phase.json`.
// A `card-verify` request runs the card's line instead, as the Card verify section says.
// It reads the lease once a second from its start, the slot wait included, and cancels the call once the lease is read as lapsed, an unreadable lease being read again a second later; a terminating signal cancels it too.
// A call that ended on its own writes its own code to `exit.json`; a lease that lapsed while the call ran writes code 1 and the abandoned mark after appending an abandoned line, naming the request's verb, to `stderr`, and one that lapsed after the call returned abandons nothing; a signal writes 128 plus its number and the abandoned mark.
// The cancelled call kills its tiers and releases its slots as a signal does, and the records it writes carry the abandoned mark.
//
// # Capped call
//
// A `lym gate test` call without `--wait` from an agent strand is capped: it starts or joins a detached runner and waits for the run's result up to the caller role's keep-warm cap, so a run that outlasts the cap keeps the agent's prompt cache warm instead of blocking one long call.
// `frontDecision` makes the choice.
// A call is not capped when it gives `--wait`, when `agentname.StrandNameEnv` is empty, when its directory is in no git worktree, when it runs inside a slot an enclosing gate run holds, and when the resolver's cap is zero.
// The cap is the answer of `shuttleengines.KeepWarm` for the strand; a fallback answer is logged at Warn once per call and still caps, at its `DefaultAwaitCap`.
// A call that is not capped runs in this process, `RunGate` and every call a verify or card-verify line makes among them, which never reach the front.
// Bound: the strand is read from the caller's environment, so a forged or absent strand chooses only how long a call blocks, never what runs or its verdict.
// Bound: the inherited-slot branch is reached only by a hand-set slot variable beside a strand, since every lym path that holds a slot runs its child without the strand; it stays so the choice is complete, and the standing agent deny on the variable keeps an agent from reaching it.
//
// A call's key is a hash of the verb, the resolved run directory and the call's words, and the key directory `detach/<key>` under the worktree's gate directory holds the key's lock, its `current.json` naming the current run id and one subdirectory per run.
// A call from a fixer strand, a role `gatefixer.IsFixerRole` matches, adds its strand name to the hashed words, so a fixer's own gate call never joins the run waiting on it and two fixer strands never share one.
// The front parses the strand's role through `strandRole` and writes it to the request as `role`, since the runner's environment carries no strand name; a request without a role reads as one from a strand that is no fixer.
// The front counts its cap from its own start and catches terminating signals, which end the front alone with 128 plus the signal number while the runner goes on.
// It takes the key directory's lock, trying once a second only while at least the lock margin of its cap remains; past that it answers as at a passed cap in the phase `starting`.
// Under the lock it joins the key's current run when `joinable` holds, else it starts a fresh one.
// A run is joinable only while its lease holds, when no front printed its result (`delivered` is absent), when HEAD's tree and the sorted dirty paths equal the run's `tree.json`, and when the run has ended without the abandoned mark or its runner's pid is live with the start reading the runner recorded, so a reused pid is no runner.
// Joining writes the call's deadline plus the grace to the lease, which keeps the later one.
// A fresh run expires the superseded run's lease, so a live runner of it ends at its next lease read, writes `request.json`, `tree.json` and the lease before launching, points `current.json` at the new id and launches `lym gate run-detached <run dir>` detached with the strand name dropped.
// The front registers the runner under the pair's slug with `runproc.RoleGate` inside a hub, so the pair's teardown ends it; the prime and a worktree outside every hub register nothing.
// It holds the lock until the runner claimed the run, checking once a second for at most the claim bound.
// A launch that failed, or a runner that ended or did not claim by the bound, is a failed start: the front claims the run itself so no runner can, expires the lease, prints the runner's log to stderr and exits 1 with an error whose way forward is to run the same command again and, when that fails again, to run it with `--wait`.
// It then waits for the run's `exit.json` for what remains of its cap.
// A result without the abandoned mark is printed: `stdout`, then `stderr`, then `delivered` is created;
// a `stdout` past `replayStdoutBytes` is not printed but named on stderr with its size, its last line printed when it is a JSON envelope, so the answer reads as one tool result and the failure report on `stderr` carries the failures, unless copying the output failed, which leaves the run joinable, and the call exits with the recorded code, except that a recorded 75 exits 1 after a stderr line saying the code was a test's own.
// A passed cap, and a result with the abandoned mark, answer ok with `done: false`, the phase from `phase.json` (`starting` when there is none), a message to run the same command again in a turn that does nothing else, and, while the run waits for its slots, `slot_free: false` with each holder's worktree, site and tier; the exit code is SlotBusyExit.
// A fresh run removes the key directory's other run directories older than 14 days whose runner is not live.
// A call with `--gate <key>` may wait one long call in place of many short ones.
// When the resolver gave a break-even (`shuttle.yaml`'s `keep_warm_max_reruns`, 0 for none or for a fallback), `chooseCap` reads this machine's run records and `longWait` weighs the latest five full calls of the key, a call that ran every tier it listed to an outcome and was neither carried, abandoned nor of an empty scope;
// when at least three exist and their median wall time exceeds the cap times the break-even, the front's cap is the resolver's long cap, 55 minutes from the claude engine, and so is its lease.
// A read error, no key and no break-even keep the short cap.
// Bound: an estimate from a few earlier calls can mis-pick, which costs cache money only, never a verdict.
// Bound: the join compares the tree and the dirty paths, not the dirty files' content, so an edit to an already-dirty file between two calls with the same words gets the earlier content's run, and a second call with the same words after another took the result starts a fresh run.
// Bound: the plan lives under `_lym`, which neither `headIdentity` nor `verifytree.DirtyPaths` reads, so a `--card` call made after an edit of the plan, with the same words and an unchanged code tree, joins the run of the plan as it was while its lease holds; the edit costs at most the earlier plan's verdict for the remaining lease, after which the same command starts afresh.
//
// # Card fixer
//
// A red `lym gate test --gate <key>` agent call hands its failure to a cold fixer when the key's `fixer` is set, and answers with the re-run's result.
// Config load allows `fixer` on the `card` key alone, so only the Webster card gate hands off; only a capped call reaches a runner, so an operator's call, a `--wait` call and a call inside an enclosing slot never do.
// `handsOff` decides it from the request: the verb is `test`, the request's role is no fixer role, the call names a gate key whose `fixer` is non-empty, and the pair has a slug.
// `newCardFixer` builds the kit: a strand fixer over the worktree's reed geometry, registered under the slug with the gate fixer role, the model choice the key's `fixer` names, the gate config's fixer timeout, the stencils directory and the hub's language skills.
// A call outside a hub, with no slug, or for a key or role `handsOff` turns down ends as it does without a fixer, and so does a kit that cannot be built, after the runner logs why.
// `runWithFixer` runs in place of `runRequest`:
// before the gate runs it waits on a live fixer record of an earlier run in the key directory's `fixer.json`, and when it cannot attach it stops the strand through `fixerStopper` and marks the record dead only once the stop succeeded, ending with exit 1 and the way forward when the stop failed;
// on a red verdict, after `runRequest` released its slots, it writes the phase `fixing`, the stderr report to `fixer-report.md`, spawns one fixer with the shared cold frame and the card body, waits on it, rewrites `tree.json` and runs `runRequest` again, then marks the fixer's record terminal, so a restart during the re-run still finds it live, and appends the fixer's reply or the dead note to the re-run's stderr under `## Cold fixer reply`.
// A pass, an abandoned exit, a non-zero exit that left nothing on stderr, as a refusal does, a red verdict whose stderr holds `gateslot.RunnerCrashLine`, a `card-verify` request and a spawn that failed end with the first run's result, the last with a stderr line saying no fixer ran.
// A passed lease or a terminating signal during the phase stops the fixer, marks its record dead once the stop succeeded and ends the run abandoned.
// A run in the phase `fixing` is joinable under changed dirty paths, so a caller's repeated call joins whatever tree the fixer is changing; from the re-run's start the rewritten `tree.json` holds the rule again.
// Bound: one fixer per red call, which Go bounds; the number of red calls per card rests on the Webster fork prompt's self-fix cap.
// Bound: a fixer's own gate call never hands off, and its strand name keys it apart from the call waiting on it.
//
// # Report
//
// `lym gate report [--slug <slug>] [--since <YYYY-MM-DD>]` reads this machine's run records through `gateslot.ReadRunRecords`, from the board's gate run directory inside a hub and the worktree's own outside every hub, and prints one row per slug and gate key.
// A row holds the tier runs, the carried tiers, the summed slot wait (a record's start minus its wait start), the summed wall time, and the launch time with its share, the launch time being the sum of the records' launch gaps and its share that sum over their summed package time.
// A record from before `--gate` has no key and groups under its tier list and scope form, such as `tiers 0,1 all`; a record without a launch list adds nothing to the launch time, and a group whose records hold none shows no launch time.
// A record marked abandoned, written after its call was cancelled by a terminating signal or a passed lease, counts apart in the row's `abandoned_runs`, `abandoned_wait_seconds` and `abandoned_wall_seconds`, and adds nothing to the tier runs, the carried tiers, the wait, the wall time, the build step or the launch totals.
// Every record of one call carries the call's id.
// A malformed `--since` is refused naming the expected form before hub resolution.
// The verb carries the operator audience annotation, takes no slot and changes nothing.
//
// # Card verify
//
// `lym gate card-verify <card id>` runs one plan card's `**Verify:**` line through `internal/cardverify`'s `Runner`, the runner webster's record-batch re-run shares.
// It sits under `lym gate` rather than `lym webster`, since the fork-mode hook refuses every `lym webster` command from a fork, and it carries the role audience annotation because the implementer fork's prompt names it.
// An id is a card's number or its `N-slug`, read from the plan at the anchor's plan directory; an unknown id is refused naming the plan's ids, and a worktree with no plan is refused naming the way forward to run it from the task worktree whose `_lym/plan` holds the card.
// A card with no `**Verify:**` line passes with a message saying so.
// The line's output passes through to the terminal and the verb exits with the line's own code; a line that timed out or did not start exits 1.
// Inside a hub a line that is not a `--wait` gate call takes one slot of the hub's pool; outside every hub it runs unslotted.
//
// The verb has no `--wait`, so `frontDecision` caps a call from an agent strand as it does a `lym gate test` call, and the line runs in a detached runner the call starts or joins.
// The key is the verb `card-verify`, the worktree as run directory and the matched card's `N-slug` id as the only word, so a call with `4` and a call with `4-<slug>` share one run.
// The runner reads the plan again in the request's directory and matches the card; a refusal there, as when the plan was edited since the front's read, is written to `stderr` with exit 1 and replayed by the front.
// A card with no line in the runner's read answers the in-process message with exit 0.
// The line runs through a `cardverify.Runner` over the worktree with the hub's pool and wait directory, under the runner's lease-cancelled context.
// `phase.json` names `slots` with the line while a slot-held line waits for its slot, and `running` with the line once it runs, a `--wait` gate line included, since it takes its own slots inside its child and answers no `slot_free`.
// The line's process tree goes to `line.json`.
// On Windows the runner first holds the kill-on-close job `lym-gate-<run id>`, which the line's tree inherits; on Linux the line's shell carries the parent-death SIGKILL of `verifyrun.RunTied`.
// A cancelled line's process group gets SIGTERM and then SIGKILL, and a `--wait` gate child cancels on that SIGTERM, killing its tiers and releasing its slots.
// A front that supersedes a `card-verify` run whose runner is not live kills the group `line.json` records, which reaches the shell's descendants the parent-death signal misses, before it starts the fresh runner.
// Bound: a job that cannot be held is logged and the line runs without it, so on Windows the runner's death then leaves the line's tree to its own end, since `proc.KillTree` kills nothing there.
// Bound: on Linux a dead runner's line runs on until a front supersedes it, or to its own end when no front comes.
// Bound: the key is the card's id, so a call after an edit of the card's `**Verify:**` line joins the run of the old line while its lease holds.
//
// # In-process run
//
// `RunGate(ctx, dir, gate, out, stderr, observer)` runs the gate key `gate` in `dir` through the same `runTest` as `lym gate test --wait --gate <gate>`, with the CLI's defaults and `dir` seeded as the context's working directory.
// The caller names only the key and the directory, never targets or tiers, and gets the call's exit code, the envelope on `out` and the summary lines on `stderr`.
// A `TierObserver` hears each tier of the run: `Started` with the tier's number once its slots are held, or at once for a run that takes none, and `Ended` with the tier's run record when the tier reports.
// A tier carried as already green ends with no start, and a tier blocked by a failed build ends with its blocked record, started only for the tier whose slots ran the build.
// A tier rerun after a crash or a resource shortage ends once, with the rerun's record.
// A guard or vet run reports through the same calls with its record's own flags, and a refusal after a start ends with no end.
// A nil func is skipped, and every CLI call passes the zero observer.
//
// # Exit codes
//
// The first failing run's own code, in plan order, which for a test binary the watchdog quit is the test run's failure code and for a runner crash 128 plus the signal's number; 1 for a tier that timed out; SlotBusyExit (75) only as a capped call's `done: false` answer, which is no failure and never the code of a call that runs in this process; 1 for a run that ended without a failure while the call was cancelled; 128 plus the signal number after a terminating signal; 1 for a refusal, with a JSON error envelope on stdout.
package gatecli
