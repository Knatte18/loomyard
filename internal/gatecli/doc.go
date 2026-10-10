// Package gatecli provides the cobra command tree for `lyx gate`, whose one verb, `test`, is the sanctioned route for an agent's slotted `go test`.
//
// The module is not standalone-capable and does not go through internal/cliwire: it has no target directory, state, plan or stencils to wire, and it resolves hub membership itself.
//
// # Verb
//
// `lyx gate test [-C <dir>] [--tags <tags>] <packages...> [-- <go test flags>]` runs `go test -C <dir> -p <cap>`, then `-tags <tags>` when given, then the packages, then the flags after `--`.
// The -C directory is made absolute against the seam cwd, never the process cwd when -C is given, and must exist.
// At least one package is required; a module-wide pattern is accepted and slotted.
// The output streams through and the exit code is go test's own.
// With `--tags` non-empty the verb first builds `lyx` once, inside the held slot: `go build -C <root> -p <cap> -o <tmp>/lyx ./cmd/lyx`, where <root> is the worktree root inside a hub and the -C directory outside one, and <tmp> is a per-invocation temporary directory removed after go test ends.
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
// go test runs in its own process group, so the verb can kill the whole tree without signalling the shell that called it.
// On a terminating signal, including one sent to the verb's own group, it kills that group, releases its slot and exits with 128 plus the signal number.
// On Linux the child also carries a parent-death SIGKILL, set from a goroutine locked to its OS thread for the child's lifetime, so a SIGKILL of the verb ends go itself.
// On another Unix the group outlives an uncatchable kill.
// On Windows the child is assigned to a job object created with kill-on-job-close right after it starts, so any exit of the verb, a hard kill included, ends the tree.
//
// # Exit codes
//
// go test's own code; SlotBusyExit (75) for a busy hub; 128 plus the signal number after a terminating signal; 1 for a refusal, with a JSON error envelope on stdout.
package gatecli
