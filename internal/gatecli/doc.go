// Package gatecli provides the cobra command tree for `lyx gate`, whose one verb, `test`, is the sanctioned route for an agent's slotted `go test`.
//
// The module is not standalone-capable and does not go through internal/cliwire: it has no target directory, state, plan or stencils to wire, and it resolves hub membership itself.
//
// # Verb
//
// `lyx gate test [-C <dir>] [--tags <tags>] <packages...> [-- <go test flags>]` runs `go test -C <dir> -p <cap>`, then `-tags <tags>` when given, then the packages, then the flags after `--`.
// The -C directory is made absolute against the seam cwd, never the process cwd when -C is given, and must exist.
// At least one package is required; a module-wide pattern is accepted and slotted.
// A package argument spelled relative (`.`, `..`, `./…` or `../…`), not ending in `.go` and with a trailing `/...` removed must name a directory under the -C directory, or the verb refuses before hub resolution, any slot and the prebuild; for an external test package it names the package directory to pass instead.
// Import paths, absolute paths, module-wide patterns whose stem exists and `.go` file arguments are not read and reach go test unchanged.
// The output streams through and the exit code is go test's own.
// With `--tags` non-empty the verb first builds `lyx` once, inside the held slot: `go build -C <root> -p <cap> -o <tmp>/lyx ./cmd/lyx`, where <root> is the worktree root inside a hub and the -C directory outside one, and <tmp> is a per-invocation directory in the process run directory of `internal/lyxtemp`, removed after go test ends.
// go test's environment then carries `gateslot.PrebuiltLyxEnv` naming that binary, so every package of the run shares it instead of building its own.
// A root without `cmd/lyx` builds nothing, and a nested module inside a hub still builds the worktree's `lyx`, which that module's tests may leave unused.
// A build failure is a JSON error with the build's output and exit code 1, before go test runs.
//
// # Hub resolution
//
// The directory is resolved with lyxcwd.ResolveWorktree, which finds the containing worktree from any directory inside it, a nested module directory included, then preflight.BoardLyxPresent.
// It does not use the standalone-capable CLIs' mode resolution, whose anchor gate would refuse a nested module directory.
// A directory in no git worktree, or in a hub with no board, is outside every hub.
//
// # Slot
//
// Inside a hub the pool comes from hubgeom.GateSlots and its limits are read from the board's gate.yaml.
// The run writes a wait record under the worktree's wait directory, acquires a slot under a context bounded by `cli_wait_sec`, removes the record once the acquire returns, and runs go test with the lease's environment.
// Every branch, slotted, inherited and unslotted, runs go test without the strand-name variable (agentname.StrandNameEnv), so a test that drives `lyx fabric` never meets the sandboxed-role guard.
// Bound: a test is code the fork writes, which could clear the variable itself.
// A bound that passes with every slot held exits with SlotBusyExit and a JSON error naming each holder's worktree and site; the way forward is to re-run the same command.
// An absent gate.yaml is refused with a way forward naming `lyx fabric reconcile`, which writes it from the template.
// An unreadable or invalid gate.yaml is refused with a way forward naming `lyx config gate` from the prime.
//
// # Inheritance
//
// When the slot variable gateslot.InheritEnv names a slot of the pool that is held, the run goes inside that slot without acquiring, so a verify command that calls `lyx gate test` cannot deadlock.
// Bound: a forged variable naming any held slot runs unslotted inside that holder's slot.
// The standing agent deny refuses an agent command that names the variable, and an operator shell or a script that hides the name is outside the bound.
//
// The prebuilt-binary variable is set or stripped on every path: a run that exports no fresh build, untagged or tagged with the build skipped, removes an inherited value from go test's environment through `gateslot.StripPrebuilt`.
// A forged value therefore never reaches a gated run, and can mislead only an ungated raw `go test`.
//
// # Outside every hub
//
// The run goes unslotted under the template config's `-p` cap and logs that no hub bound applies.
// Bound: that path lets through only a target outside every hub, capped by the template's `-p` alone.
//
// # Child lifetime
//
// go test runs through `internal/proc`'s tied child, which keeps its tree killable without signalling the shell that called the verb and ends it when the verb dies, a hard kill included.
// On a terminating signal, including one sent to the verb's own group, the verb kills that tree, releases its slot and exits with 128 plus the signal number.
//
// # CPU watchdog
//
// While go test runs, the verb samples its child tree once per period, 5 seconds in production, through `proc.TreeCPU`.
// A process whose path ends in `.test` that keeps at least 90 percent of one core busy for `busy_quit_sec` gets one SIGQUIT, so the Go runtime prints every goroutine's stack and go test reports the package failed long before its own `-timeout`.
// The window counts whole samples, a pid's first sample is its baseline, and any sample below 90 percent restarts the count.
// The verb prints `gateslot.WatchdogLine` to its stderr after the quit, and a halt names `gateslot.BusyQuitWayForward`.
// Off Linux `proc.TreeCPU` reports `errors.ErrUnsupported`, the watchdog logs that no watchdog applies and ends, and go test runs as before.
// The watchdog is off when `busy_quit_sec` is 0, when the go test flags after `--` carry `-bench` or `-fuzz`, which keep a core busy by intent, and when `LYX_GATE_TEST_PID` names a live ancestor, the pid an outer `lyx gate test` sets in go test's environment, since that run's watchdog already sees this run's test binaries.
// An empty, unparsable or non-ancestor value is ignored, such as one a tmux server a test started carried into its panes after leaving the outer run's tree.
// An inherited slot does not turn it off: `verifytree.Verify` holds a slot and passes it to a verify command that calls `lyx gate test`, which is the outermost gate run there.
// Bound: it signals only `.test` processes in the verb's own child tree, once each, and covers only verify commands that run through `lyx gate test`.
// A CPU-bound package that is healthy but busy past the window is quit and fails, and the operator raises the key.
// A run carrying `-bench` or `-fuzz`, a run under a live enclosing `lyx gate test`, a run whose inherited variable names a live ancestor, and a non-Go hub's verify are bounded only by go test's own `-timeout` or the verify timeout.
//
// # Exit codes
//
// go test's own code, which for a test binary the watchdog quit is go test's failure code; SlotBusyExit (75) for a busy hub; 128 plus the signal number after a terminating signal; 1 for a refusal, with a JSON error envelope on stdout.
package gatecli
