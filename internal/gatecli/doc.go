// Package gatecli provides the cobra command tree for `lyx gate`, whose verb `test` is the sanctioned route for running the hub's test tiers, whose verb `status` shows who holds the hub's gate slots, whose verb `report` totals the run records per gate key and whose verb `card-verify` runs a plan card's `**Verify:**` line.
//
// The module is not standalone-capable and does not go through internal/cliwire: it has no target directory, state, plan or stencils to wire, and it resolves hub membership itself.
//
// # Verb
//
// `lyx gate test [-C <dir>] [--gate <key>] [--tier N]... [--upto N] [--card <id>]... [--base <rev>] [--all] [--guards] [--wait] [<targets>...] [-- <args>]` runs the tiers the hub declares in gate.yaml, in ascending tier number, and then the vet step and the guard step below when they are planned.
// A run that fails does not stop the call: every later tier, vet and guard run still runs, so a failure the change under test did not cause, such as one present at the base, hides no later check.
// When a run failed, the verb closes with one stderr line naming each failed run, a vet run as `vet tierN` and a guard run as `guard tierN`, with its exit code.
// A refusal, a slot wait that does not end with the slots held and a terminating signal still end the call at once.
// `--gate <key>` runs the tiers and the scope the `gates` key of that name holds, and `listedTiers` takes the key's tiers as the listed tiers.
// It goes with none of `--tier`, `--upto`, `--all` and `--base`, which is refused before hub resolution naming the key and the flag to drop, since the key's tiers and scope come from gate.yaml.
// Once the config is loaded, a key the config does not hold, an empty tier list and an undeclared tier are refused with `gateslot.Config.Gate`'s way forward naming `lyx config gate`.
// A key of scope `card` takes the `--card` ids as the card form and is refused, naming the key and the way forward of passing `--card <id>` per card, when none is given;
// a key of scope `all` is the `--all` form, and a key of scope `diff` is the computed form.
// `--card` ids beside a key of another scope are ignored, and explicit `<targets>` beside `--gate` replace the key's scope.
// Each run record carries the key as `gate_key`, empty for a call with manual flags.
// `--gate` opens no flag combination the manual flags lack.
// `--tier` names a tier and may repeat; the default is tier0 when neither `--tier` nor `--upto` is given, and a tier another listed tier covers is dropped and named as covered on stderr.
// `--upto N` adds every declared tier numbered 0 through N to the `--tier` values, skipping a gap in the numbers and a tier marked `manual`, which it names on stderr unless `--tier` names it.
// A manual tier runs only when `--tier` names it, so a tier that spends tokens never starts from a range; a negative N is refused before hub resolution.
// `--tags` stays a registered flag only to be refused before any slot, naming `--tier`: a tier's own command sets its build tags.
// The words after `--` fill each tier's `{args}`.
//
// A tier is one shell command with the placeholders `{targets}`, `{parallel}` and `{args}`, which `renderTierCommand` fills with each target and each arg quoted for the platform shell by `verifyrun.QuoteArg`.
// The command runs through the shell `verifyrun.ShellFor` names, as a tied child in the run directory, with this process's environment less the strand-name and trace-id variables, less an inherited `gateslot.PrebuiltLyxEnv`, plus the tier's `env` with `{parallel}` filled, plus the lease's `gateslot.InheritEnv` and the enclosing-gate pid.
// The tier's `dir`, relative to the worktree root, is the run directory.
// A tier with `prebuilt_lyx` first builds `lyx` at the tier's `parallel`, inside its slots and timeout, only where the worktree root holds `cmd/lyx`, and exports `gateslot.PrebuiltLyxEnv`.
// Inside a hub the binary is copied into the pool's store (`gateslot.Pool.PrebuiltDir`) as `lyx-` and the first 16 hex digits of its SHA-256, the build directory is removed, and the variable names the stored copy;
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
// The plan read and its two refusals run first under either scoper.
//
// Under the `none` scoper each of the three forms is whole, so a tier takes its `all_targets`, and no guard step is planned.
// Under the `go` scoper a computed or `--base` scope is `impactset.DeriveTargets` of the diff from its base to the working tree, and falls back to whole wherever it does, naming the reason on stderr;
// a `--card` scope is the cards' packages.
// A narrower scope runs each listed tier once per module, in that module's directory with its module-relative targets, under one slot hold and one `timeout_min` covering every invocation.
// An empty scope, such as a diff of Markdown files only, runs no listed tier's command: each listed tier passes with zero discovered, is logged so, takes no slot, and only the guard step runs.
// The import graph cannot see everything a narrower run skips, such as `go build ./...`, so that is left to the whole runs at `final` and `landing`.
// Once the config is loaded, `planTierRuns` refuses, before any slot and naming `lyx config gate`: words after `--` for a tier whose command lacks `{args}`, an undeclared tier, a hub with no `tier0`, a `covers` entry naming a tier the hub does not declare, and, for the guard step below, a guard group whose tier the hub does not declare.
// Every such refusal closes with the clause that a session that is not the prime takes none of the steps, reports status: FAILED and leaves them to the orch.
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
// With `--guards`, and under the `go` scoper after any computed, `--base` or `--card` scope, the verb runs, after the listed tiers, the step `guardStepRun` builds over the module's `//lyx:guard` tests outside the scope.
// `impactset.DeriveTargets` names those packages for a narrower computed or `--base` scope, and an `impactset.ModuleGuardTests` scan of the module at the run directory, which is `tier0`'s dir or the `-C` directory of explicit targets, names them for a `--card` scope, a scope fallen back to whole, and `--guards` over explicit targets.
// So the round and darn writer gates, which run `--base`, keep the guard coverage they have had, where every guard package counts as outside a tier that does not compile it.
// The scan reads each guard test's tier from its file's `tierN` build tag, a file naming none counting as `tier0`, and the step plans one guard run per tier holding guard tests, ascending.
// A guard run is one run of that tier's command with the group's guard packages as `{targets}` and `-run '^(<names>)$'` after any words from `--` as `{args}`, so the guard's own pattern is the last `-run` and wins over one among those words.
// It acquires that tier's slots after the last listed tier and vet run released their own, runs under that tier's `timeout_min` and `prebuilt_lyx`, and is logged as a guard step; a failed guard run, like a failed listed tier, stops no later run.
// A group is skipped when the call ran its tier, or a tier covering it, whole, and a group whose tier the green record holds green carries.
// A group whose tier is undeclared, or whose command lacks `{targets}` or `{args}`, is refused before any slot, naming `lyx config gate`.
// A scan error refuses the call before any slot with the file, the line when there is one and a way forward for its kind:
// move a misplaced marker directly above its `func Test…` line, move a marked test out of a `tier2` or `tier3` file into a `tier0` or `tier1` file, or make an unparsable or unreadable file readable and parseable.
// Any other scan error, such as an unreadable package directory, names the module directory and the way forward to make `go list ./...` succeed there.
// `--guards` under the `none` scoper is refused naming the scoper, since the scan is Go's.
// A module with no marked test runs no guard step and logs so.
//
// # Hub resolution
//
// The directory is resolved with lyxcwd.ResolveWorktree, which finds the containing worktree from any directory inside it, a nested module directory included, then preflight.BoardLyxPresent.
// It does not use the standalone-capable CLIs' mode resolution, whose anchor gate would refuse a nested module directory.
// Inside a hub the config is `gateslot.LoadConfig` over the hub, this machine's override included; an absent gate.yaml is refused with a way forward naming `lyx fabric reconcile`, an invalid one with an error that names each file in the stack and its way forward, and a hub with no tier is refused naming `lyx config gate`.
// Outside every hub, in a git worktree whose hub carries no board, the worktree's own gate.yaml is read through the same loader with only the anchor told, and the run is unslotted; with none, the call is refused naming `lyx config gate`.
// A directory in no git worktree has no worktree layer to read, so it is refused naming `lyx config gate` run in a git worktree.
//
// # Slots
//
// A tier first takes the slots it asks for through `gateslot.Pool.AcquireSlots`, with a holder that names the tier, writes a wait record under the worktree's wait directory while it waits, and releases them before the next tier.
// Without `--wait` the wait keeps the keep-cache-warm cap, `shuttleengine.DefaultAwaitCap`, and a cap that passes with the slots held is no failure:
// the verb prints an ok envelope with `slot_free: false`, each holder's worktree, site and tier, and a message to run the same command again in a turn that does nothing else, then exits with SlotBusyExit.
// `--wait` lifts the cap, so the call blocks until its slots free, which costs only that caller's prompt cache.
// The cap keeps one blocked Bash call under the prompt-cache lifetime, so an agent that re-runs the verb keeps its cache warm.
// A tier with `slots: all` holds every slot and waits behind the pool's drain.
// Outside a hub the tiers run unslotted.
//
// `lyx gate status` reads the pool of the hub that contains the working directory and changes nothing.
// It answers an ok envelope with `in_hub`, the pool's `slots` count, `draining`, whether a multi-slot acquire holds the drain so new single-slot runs wait while a slot is free, and `held`, one entry per held slot, oldest first, with its `slot` number, `worktree`, `site`, `tier`, `pid` and `started` time.
// Outside every hub it answers `in_hub: false` and nothing held.
//
// # Inheritance
//
// When the slot variable gateslot.InheritEnv names a slot of the pool that is held, the run goes inside that slot without acquiring, whatever slots its tiers ask for, so a verify command that calls `lyx gate test` cannot deadlock.
// Bound: a forged variable naming any held slot runs unslotted inside that holder's slot.
// The standing agent deny refuses an agent command that names the variable, and an operator shell or a script that hides the name is outside the bound.
//
// The prebuilt-binary variable is set or stripped on every path: a run that exports no fresh build removes an inherited value from the tier's environment through `gateslot.StripPrebuilt`.
// A forged value therefore never reaches a gated run, and can mislead only an ungated raw run.
//
// # Green record
//
// `lyx gate test` carries a green tier forward over an unchanged tree, at every site and manual run alike.
// The record is one JSON file per worktree, `green.json` in the worktree's never-tracked gate directory, `gateslot.WaitDir` of the anchor, read tolerantly:
// an unreadable or malformed file reads as empty.
// An entry holds the tier fingerprint, HEAD's tree and commit, the module directory, the args after `--`, and either `whole` or the green package set, plus its write time.
// `tierFingerprint` hashes the tier's declaration without `parallel`, `timeout_min` and `slots`, so a changed command, env, targets or counter never carries.
//
// Before the first tier the verb checks the worktree with `verifytree.DirtyPaths`: a dirty worktree neither reads nor writes the record.
// On a clean one it reads HEAD's tree and commit, and before acquiring slots each listed tier subtracts what `greenRecord.covered` holds green for the current tree:
// a whole entry covers every target, and under the `go` scoper a package entry covers its packages, so a remainder runs as explicit packages; under the `none` scoper only a whole entry carries.
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
// Bound: a carried tier trusts a pass over the same tree, tier declaration and args on this worktree, so a flaky test that passed once is not rerun.
// A Go toolchain change, inherited environment variables outside the tier's `env` and ignored files such as `.dev-bin` are outside the fingerprint and the clean check.
// The record never leaves the machine, so an adopted run tests afresh.
// A forged or hand-edited whole entry carries every later gate on that tree, Merge-In and Finalize included, which is accepted because a session that can write the worktree can change the code and tests under review, so the record guards only honest repeats and no further deny is added, since `cp` and `tee` would still reach it.
//
// # Timeouts
//
// The tier's `timeout_min`, counted once its slots are held, bounds the prebuild and every invocation of the tier's command.
// A tier that outlives it has its process tree killed through the tied child's context and fails with exit code 1 and `gateslot.TierTimeoutLine` on stderr and in its log.
// The work before the first tier is bounded by a ceiling, 10 minutes in production, and fails naming the step and the way forward to run the same command again, which a transient hang such as a stalled `go list` passes.
// The length of a `timeout_min` minute and the ceiling are parameters of `newCommand`, so a test shortens them.
//
// # Counters, record and log
//
// A tier's output goes through its counter, `go-json` or `vstest`, which passes the text on and counts the tests it reports.
// After each tier the verb prints `gateslot.TierSummaryLine` to stderr and writes the run record and log through `gateslot.WriteRunRecord` under `gateslot.RunLogDir`:
// the board's gate directory inside a hub, so the records outlive the worktree, and the worktree's own `.lyx` gate directory outside every hub.
// The record's slug is the one the pair's fabric origin record names, empty for the prime and outside every hub.
// A write failure is logged and decides nothing.
// The `go-json` counter also keeps, per package, the gap from its `start` event to its next event and its closing event's elapsed time, and the record carries them as `launches`; a cached package and one with no test files are left out, and a `vstest` run records none.
// The `go-json` counter also keeps, per package, the number of tests that ran in it, and a `vstest` run keeps none.
// A `go-json` tier under the `go` scoper fails when a package holding a test under the tier's tag ran none, since the tier's command then lacks its tag: a package with a `Test` function in a file whose build constraint holds under `tierN` on the host's GOOS and GOARCH.
// The failure exits 1 and its line, naming the tier, the package and `lyx config gate`, goes to stderr and the log before the summary line.
// A run whose words after `--` carry `-run`, `-skip`, `-list` or `-bench` is exempt, since its filter may match nothing; the guard step's own `-run` filter exempts nothing.
// Otherwise a tier that discovers no tests passes with zero counted, as a package with only a `TestMain` does.
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
// # CPU watchdog
//
// While a tier's command runs, the verb samples its child tree once per period, 5 seconds in production, through `proc.TreeCPU`.
// A process whose path ends in `.test` that keeps at least 90 percent of one core busy for `busy_quit_sec` gets one SIGQUIT, so the Go runtime prints every goroutine's stack and the test run reports the package failed long before the tier's own timeout.
// The window counts whole samples, a pid's first sample is its baseline, and any sample below 90 percent restarts the count.
// The verb prints `gateslot.WatchdogLine` to its stderr after the quit, and a halt names `gateslot.BusyQuitWayForward`.
// Off Linux `proc.TreeCPU` reports `errors.ErrUnsupported`, the watchdog logs that no watchdog applies and ends, and the tier runs as before.
// The watchdog is off when `busy_quit_sec` is 0, when the words after `--` carry `-bench` or `-fuzz`, which keep a core busy by intent, and when `LYX_GATE_TEST_PID` names a live ancestor, the pid an outer `lyx gate test` sets in the tier's environment, since that run's watchdog already sees this run's test binaries.
// An empty, unparsable or non-ancestor value is ignored, such as one a tmux server a test started carried into its panes after leaving the outer run's tree.
// An inherited slot does not turn it off: a verify run holds a slot and passes it to a verify command that calls `lyx gate test`, which is the outermost gate run there.
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
//	    prebuilt_lyx: true
//	  tier2:
//	    about: tmux tests
//	    command: go test -json -tags tier2 {targets} {args}
//	    all_targets: ./...
//	    env: {GOFLAGS: "-trimpath -p={parallel}"}
//	    counter: go-json
//	    parallel: 4
//	    timeout_min: 30
//	    covers: [0]
//	    prebuilt_lyx: true
//	gates:
//	  card: {tiers: [0], scope: card}
//	  batch: {tiers: [0, 1], scope: card}
//	  webster: {tiers: [0, 1], scope: all}
//	  merge-in: {tiers: [0, 1], scope: all}
//	  finalize: {tiers: [0, 1], scope: all}
//	  webster-fix: {tiers: [0, 1], scope: diff}
//	  darn-write: {tiers: [0, 1], scope: diff}
//	  code-burler-fix: {tiers: [0, 1], scope: diff}
//
// A tier is written whole with one flow mapping, for example `lyx config gate --set 'tiers.tier0={command: "go test -json {targets} {args}", all_targets: ./..., counter: go-json, timeout_min: 10}'`.
//
// # Report
//
// `lyx gate report [--slug <slug>] [--since <YYYY-MM-DD>]` reads this machine's run records through `gateslot.ReadRunRecords`, from the board's gate run directory inside a hub and the worktree's own outside every hub, and prints one row per slug and gate key.
// A row holds the tier runs, the carried tiers, the summed slot wait (a record's start minus its wait start), the summed wall time, and the launch time with its share, the launch time being the sum of the records' launch gaps and its share that sum over their summed package time.
// A record from before `--gate` has no key and groups under its tier list and scope form, such as `tiers 0,1 all`; a record without a launch list adds nothing to the launch time, and a group whose records hold none shows no launch time.
// A malformed `--since` is refused naming the expected form before hub resolution.
// The verb carries the operator audience annotation, takes no slot and changes nothing.
//
// # Card verify
//
// `lyx gate card-verify <card id>` runs one plan card's `**Verify:**` line through `internal/cardverify`'s `Runner`, the runner webster's record-batch re-run shares.
// It sits under `lyx gate` rather than `lyx webster`, since the fork-mode hook refuses every `lyx webster` command from a fork, and it carries the role audience annotation because the implementer fork's prompt names it.
// An id is a card's number or its `N-slug`, read from the plan at the anchor's plan directory; an unknown id is refused naming the plan's ids, and a worktree with no plan is refused naming the way forward to run it from the task worktree whose `_lyx/plan` holds the card.
// A card with no `**Verify:**` line passes with a message saying so.
// The line's output passes through to the terminal and the verb exits with the line's own code; a line that timed out or did not start exits 1.
// Inside a hub a line that is not a `--wait` gate call takes one slot of the hub's pool; outside every hub it runs unslotted.
//
// # Exit codes
//
// The first failing run's own code, in plan order, which for a test binary the watchdog quit is the test run's failure code; 1 for a tier that timed out; SlotBusyExit (75) for a busy hub when no earlier run failed, and the first failed run's code when one did; 1 for a run that ended without a failure while the call was cancelled; 128 plus the signal number after a terminating signal; 1 for a refusal, with a JSON error envelope on stdout.
package gatecli
