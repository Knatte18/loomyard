// Package gatecli provides the cobra command tree for `lyx gate`, whose verb `test` is the sanctioned route for running the hub's test tiers and whose verb `card-verify` runs a plan card's `**Verify:**` line.
//
// The module is not standalone-capable and does not go through internal/cliwire: it has no target directory, state, plan or stencils to wire, and it resolves hub membership itself.
//
// # Verb
//
// `lyx gate test [-C <dir>] [--tier N]... [--card <id>]... [--base <rev>] [--all] [--guards] [--wait] [<targets>...] [-- <args>]` runs the tiers the hub declares in gate.yaml, in ascending tier number, and stops at the first tier that fails.
// `--tier` names a tier and may repeat; the default is tier0, and a tier another listed tier covers is dropped and named as covered on stderr.
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
// Flag-shape refusals need no config and run first, before hub resolution: `--tags`, naming `--tier`; `-C` without explicit targets, naming them; and more than one scope form.
//
// # Scoper
//
// `computeScope` resolves the computed, `--base` and `--card` forms inside the pre-tier ceiling, and `gate.yaml`'s `scoper` decides what they mean.
// The computed form's base is the merge base of HEAD with the parent branch the pair's fabric origin record names, read through `parentMergeBase`; a worktree with no origin record or no parent, such as the prime on `main` or a standalone run, runs whole.
// `--base` names that base itself.
// `--card`, repeatable, reads the plan at the anchor's plan directory through `planparser.ParsePlan` and takes `planparser.CardPackages` of the named cards; an id is a card's number or its `N-slug`, an unknown id is refused before any slot naming the plan's ids, and a worktree with no plan is refused naming `--all` and explicit targets.
// The plan read and its two refusals run first under either scoper.
//
// Under the `none` scoper each of the three forms is whole, so a tier takes its `all_targets`, and no guard step is planned.
// Under the `go` scoper a computed or `--base` scope is `impactset.DeriveTargets` of the diff from its base to the working tree, and falls back to whole wherever it does, naming the reason on stderr;
// a `--card` scope is the cards' packages.
// A narrower scope runs each listed tier once per module, in that module's directory with its module-relative targets, under one slot hold and one `timeout_min` covering every invocation.
// An empty scope, such as a diff of Markdown files only, runs no listed tier's command: each listed tier passes with zero discovered, is logged so, takes no slot, and only the guard step runs.
// The import graph cannot see everything a narrower run skips, such as `go build ./...`, so that is left to the whole runs at `final` and `landing`.
// Once the config is loaded, `planTierRuns` refuses, before any slot and naming `lyx config gate`: words after `--` for a tier whose command lacks `{args}`, an undeclared tier, a hub with no `tier0`, and a `covers` entry or a `guard_tier` naming a tier the hub does not declare.
// Every such refusal closes with the clause that a session that is not the prime takes none of the steps, reports status: FAILED and leaves them to the orch.
//
// # Guard step
//
// With `--guards`, and under the `go` scoper after any computed, `--base` or `--card` scope, the verb runs, after the listed tiers pass, `runGuardStep` over the module's `//lyx:guard` tests outside the scope.
// `impactset.DeriveTargets` names those packages for a narrower computed or `--base` scope, and an `impactset.ModuleGuardTests` scan of the module at the run directory names them for a `--card` scope, a scope fallen back to whole, and `--guards` over explicit targets.
// So the round and darn writer gates, which run `--base`, keep the guard coverage they have had, where every guard package counts as outside a tier that does not compile it.
// The step is one run of `guard_tier`'s command with the guard packages as `{targets}` and `-run '^(<names>)$'` after any words from `--` as `{args}`, so the guard's own pattern is the last `-run` and wins over one among those words.
// It acquires `guard_tier`'s slots after the last listed tier released its own, runs under `guard_tier`'s `timeout_min` and `prebuilt_lyx`, and is logged as the guard step.
// It is skipped when the call ran `guard_tier`, or a tier covering it, whole.
// A step that is not skipped refuses a `guard_tier` command lacking `{targets}` or `{args}`, naming `lyx config gate`.
// A scan error refuses the call before any slot with the file, the line when there is one and a way forward for its kind:
// move a misplaced marker directly above its `func Test…` line, compile the marked test's file under `integration`, or make an unparsable or unreadable file readable and parseable.
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
// # Inheritance
//
// When the slot variable gateslot.InheritEnv names a slot of the pool that is held, the run goes inside that slot without acquiring, whatever slots its tiers ask for, so a verify command that calls `lyx gate test` cannot deadlock.
// Bound: a forged variable naming any held slot runs unslotted inside that holder's slot.
// The standing agent deny refuses an agent command that names the variable, and an operator shell or a script that hides the name is outside the bound.
//
// The prebuilt-binary variable is set or stripped on every path: a run that exports no fresh build removes an inherited value from the tier's environment through `gateslot.StripPrebuilt`.
// A forged value therefore never reaches a gated run, and can mislead only an ungated raw run.
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
// A tier's output goes through its counter, `go-json` or `vstest`, which passes the text on and counts the tests it reports; the counts decide nothing about the outcome.
// After each tier the verb prints `gateslot.TierSummaryLine` to stderr and writes the run record and log through `gateslot.WriteRunRecord` under `gateslot.RunLogDir`:
// the board's gate directory inside a hub, so the records outlive the worktree, and the worktree's own `.lyx` gate directory outside every hub.
// The record's slug is the one the pair's fabric origin record names, empty for the prime and outside every hub.
// A write failure is logged and decides nothing.
// A tier that discovers no tests passes with zero counted, as a package with only a `TestMain` does.
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
//	guard_tier: 1
//	refuse_raw: ["go test"]
//	tiers:
//	  tier0:
//	    about: untagged tests
//	    command: go test -json {targets} {args}
//	    all_targets: ./...
//	    env: {GOFLAGS: "-trimpath -p={parallel}"}
//	    counter: go-json
//	    parallel: 4
//	    timeout_min: 10
//	  tier1:
//	    about: integration tests, with build and vet under each tag first
//	    command: go build ./... && go vet -tags integration {targets} && go vet -tags tmux {targets} && go vet -tags llm {targets} && go test -json -tags integration {targets} {args}
//	    all_targets: ./...
//	    env: {GOFLAGS: "-trimpath -p={parallel}"}
//	    counter: go-json
//	    parallel: 4
//	    timeout_min: 30
//	    covers: [0]
//	    prebuilt_lyx: true
//	  tier2:
//	    about: tmux tests
//	    command: go test -json -tags tmux {targets} {args}
//	    all_targets: ./...
//	    env: {GOFLAGS: "-trimpath -p={parallel}"}
//	    counter: go-json
//	    parallel: 4
//	    timeout_min: 30
//	    covers: [0]
//	    prebuilt_lyx: true
//	gates:
//	  card: [0]
//	  batch: [0, 1]
//	  round: [0, 1]
//	  final: [0, 1]
//	  landing: [0, 1]
//	  publish: [2]
//
// A tier is written whole with one flow mapping, for example `lyx config gate --set 'tiers.tier0={command: "go test -json {targets} {args}", all_targets: ./..., counter: go-json, timeout_min: 10}'`.
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
// The failing tier's own code, which for a test binary the watchdog quit is the test run's failure code; 1 for a tier that timed out; SlotBusyExit (75) for a busy hub; 128 plus the signal number after a terminating signal; 1 for a refusal, with a JSON error envelope on stdout.
package gatecli
