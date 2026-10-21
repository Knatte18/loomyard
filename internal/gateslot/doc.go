// Package gateslot owns the hub-wide pool of gate slots that bounds how many gate builds and test runs the hub executes at once.
//
// The package is told every directory it works on and derives none beyond the Dir, WaitDir and Pool.PrebuiltDir joins, which spell the ephemeral directory name only through internal/lyxdirs.
//
// # Slots
//
// A Pool holds one OS file lock per slot at `<Dir>/slot-<n>.lock`, n from 1 to the current Limits.Slots.
// Acquire takes the first free slot and writes its Holder record (worktree, site, pid, start time) as YAML at `<Dir>/slot-<n>.yaml`.
// While every slot is held it logs the wait start, polls every Pool.Poll and logs the wait end on acquiring.
// Limits is a closure read at every attempt, so a lowered count takes effect on the next acquire while a running holder of a higher slot finishes.
// Acquire imposes no deadline: a caller wanting a bound passes a deadline context.
// Polling gives no FIFO order among waiters.
//
// AcquireSlots takes a tier's slots: a count of 1 is Acquire, and a count of 0 (`slots: all`) or above the current Limits.Slots is clamped to every slot.
// A multi-slot acquire first takes the drain lock at `<Dir>/drain.lock`, an OS file lock, then takes slots one at a time in ascending slot order, holding each it has while it waits for the next, and releases the drain lock once it holds them all.
// While another process holds the drain lock Acquire waits, polling as it does for a held slot, so new single-slot acquires queue behind a waiting multi-slot tier and cannot starve it.
// Ascending acquisition means two multi-slot waiters never deadlock: the drain admits one at a time, and every single-slot holder finishes.
// A cancelled context releases every slot and the drain lock already taken.
// The Holder record names the tier run that holds a slot, so a status reader can show it.
//
// An OS lock is released by process death, so a crashed holder never leaks a slot; its holder record stays on disk but Holders trusts a record only while its lock is held.
// Holders reads every slot record in the directory, so a holder of a slot above a lowered count still shows.
//
// # Inheritance
//
// Lease.Env returns the environment for a child that runs inside the slot: GOFLAGS gets `-p=<GoParallel> -trimpath` appended and InheritEnv names the held slot's lock path.
// `-trimpath` makes builds from two checkouts byte-identical and their cache keys independent of the checkout's directory,
// so a fresh worktree hits the build and test cache and a host scanner that holds each unseen executable sees known hashes.
// It also makes `runtime.Caller` report module-relative file paths, so no code a gate run builds locates the repo root from them alone.
// Lease.InheritEnv returns the environment of a tier run: only InheritEnv is set, naming the lease's first slot's lock path, and no GOFLAGS is appended, since a tier's command and env set its own parallelism.
// HolderEnv returns the environment of a non-tier slot holder's child: GOFLAGS gets `-trimpath` and, when the holder passes a tier0 parallel above 0, `-p=<n>`, and a non-nil lease sets InheritEnv.
// Limits.Tier0Parallel carries `tier0`'s effective parallel, this machine's override included, and is 0 when the hub declares no `tier0`, so a holder reads it from the pool's Limits at its acquire.
// A nested gate run checks Pool.Inherited on that variable and runs inside its parent's slot without acquiring, whatever slots its own tier asks for.
//
// # Prebuilt store
//
// Pool.PrebuiltDir is `<Dir>/prebuilt`, where a tagged gate run keeps the `lyx` it built under a name derived from the binary's content, so PrebuiltLyxEnv holds the same value for every run on an unchanged tree.
// The gate run that writes the store also prunes it.
//
// # Config
//
// The hub-wide `gate` config module keeps `slots`, `go_parallel`, `busy_quit_sec` and the tier keys below at `<BoardDir>/_lyx/config/gate.yaml`, seeded from template.yaml by the hub-wide reconcile.
// `slots`, `go_parallel` and `tier_parallel` take a machine-local override at `<BoardDir>/.lyx/config/gate.machine.yaml`, never reconciled and read at every acquire; every other key stays hub-file only.
// `cli_wait_sec` is a retired key: every load drops it and reconcile prunes it, since the slot wait is bounded by the keep-cache-warm cap with no knob.
// LoadConfig reads it strictly and refuses any value below 1 naming its key, except `busy_quit_sec`, which accepts 0 and is refused below it; Config.Limits converts it to the Limits a Pool reads.
// An absent hub file is ErrConfigAbsent with the way forward `lyx fabric reconcile`; outside every hub, where the dirs tell no board, the file is the worktree's own and the way forward is `lyx config gate`, after creating the worktree's `_lyx` directory.
// TemplateConfig decodes the template alone, for a run outside every hub.
//
// The tier keys are `tiers`, `gates`, `scoper`, `guard_tier`, `refuse_raw` and `tier_parallel`.
// `tiers` is an open map keyed `tier0`, `tier1`, and so on, each a Tier written whole with one `lyx config gate --set 'tiers.tierN={...}'` flow mapping;
// `gates` is an open map from a gate name to its list of tier numbers, and `tier_parallel` an open map from a tier name to this machine's parallel for it.
// `tiers` is an executable key, since a tier command runs any shell command in a task worktree, and `gates`, `scoper`, `guard_tier` and `refuse_raw` are guard keys, so all of them are hub-file only.
// The machine override is a sibling map rather than a path under `tiers`, because an open map is one unit in the layer merge, so a machine layer holding part of `tiers` would replace the hub's tiers whole.
// A declared tier's effective Parallel is its `tier_parallel` entry when one is set, from whichever layer sets it, and 1 when neither sets one; a standalone worktree has no machine layer and sets the key in its own file.
//
// LoadConfig refuses at load, naming the key and `lyx config gate`: a tier lacking `command`, `timeout_min` or `counter`, or `all_targets` when its command holds `{targets}`;
// `parallel` or `timeout_min` below 1, `slots` neither `all` nor at least 1, a counter other than `go-json` or `vstest`, a scoper other than `go` or `none`;
// a `covers` entry not lower than its tier's number, a tier key not spelled `tier<N>`, a `refuse_raw` entry that is not words of letters, digits, `.`, `_` and `-` separated by single spaces, and a `tier_parallel` entry that is no integer of at least 1.
// A `tier_parallel` entry for a tier the config does not declare is dropped with a logged warning, so a tier the hub removes leaves no entry that fails the load.
// Cross-references are checked at use, never at load, so the template and a hub whose gate lists name an undeclared tier both load.
//
// At use, Config.TierList refuses a hub with no `tier0`, a gate name the config holds no list for, an empty list for any gate but `publish`, and a listed tier the hub does not declare; Config.Tier refuses an undeclared number.
// Each refusal names `lyx config gate` and closes with the clause that a session that is not the prime reports status: FAILED and the orch takes the steps.
// Config.GateCall renders the one `lyx gate test --wait --tier <n>...` call a site runs, in the list's order and followed by the scope words, and "" for an empty `publish` list.
// Config.TiersValue renders the tier lines and the Go-read gate lists a prompt takes, or `none` when no tier is declared.
//
// # Watchdog and timeout lines
//
// The line the gate test's CPU watchdog prints after a quit is rendered by WatchdogLine and found by FindWatchdogQuit.
// BusyQuitWayForward is the way forward after one.
// The line the gate prints when a tier outlives its `timeout_min` is rendered by TierTimeoutLine and found by FindTierTimeout;
// it names the tier, the timeout and the whole-tier `lyx config gate --set` rewrite that raises it.
//
// # Run log
//
// RunLogDir is `<gateDir>/runs`, under the board's never-tracked `.lyx` inside a hub and under the worktree's own outside one, so the records outlive the worktree.
// WriteRunRecord writes one RunRecord per tier run as `<stamp>-tier<N>-<pid>.json` with the run's full output beside it as a `.log`, and prunes files older than 14 days first.
// The record is a domain log that no run reads to proceed.
// TierSummaryLine renders the one stderr line the gate prints after each tier run, and FailedTier finds the last failing one in a log, so a site that keeps the gate's output can name the failing tier.
//
// # Wait records
//
// A waiter that has no verifytree marker keeps a Wait record, one uniquely named YAML file in WaitDir under its worktree's anchor root.
// WriteWait creates it and the writer removes it; ReadWaits returns only the records whose process is alive.
package gateslot
