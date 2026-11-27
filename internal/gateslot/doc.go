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
// Acquire takes the first free slot and writes its Holder record as YAML at `<Dir>/slot-<n>.yaml`.
// A tier run's record also carries the environment marker of the processes the run starts.
// Limits is a closure read at every attempt.
// A lowered count takes effect on the next acquire, while a running holder of a higher slot finishes.
// Acquire imposes no deadline: a caller wanting a bound passes a deadline context.
// Polling gives no FIFO order among waiters.
//
// AcquireSlots takes a tier's slots: a count of 1 is Acquire,
// and a count of 0 (`slots: all`) or above the current Limits.Slots is clamped to every slot.
// A multi-slot acquire first takes the drain lock, so new single-slot acquires queue behind it and cannot starve it.
// It then takes slots in ascending order, so two multi-slot waiters never deadlock.
// A cancelled context releases every slot and the drain lock already taken.
// A `lym gate test --copies` call takes one lease of its copies' slots together through AcquireSlots.
// It never runs BilledTier, the tier that drives a billed LLM session.
// Every tier run prints a started line built by FormatStartedLine, which ParseStartedLines alone reads back.
//
// An OS lock is released by process death, so a crashed holder never leaks a slot.
// Its holder record stays on disk, but Holders trusts a record only while its lock is held.
//
// # Inheritance
//
// A slot's environment is Lease.InheritEnv for a tier run and HolderEnv for a non-tier holder.
// GOFLAGS gets `-trimpath`, which makes builds from two checkouts byte-identical,
// so a fresh worktree hits the build cache and a host scanner sees known hashes.
// `-trimpath` also makes `runtime.Caller` report module-relative paths,
// so no code a gate run builds can locate the repo root from them alone.
// It does not reach the cgo-generated files the cache keeps, which name the absolute path of the first checkout;
// no package of the module imports "C", and `go vet` never opens another worktree's files.
// GOFLAGS also opens with TestCacheOffFlag, `-count=1`, so `go test` keeps no result cache;
// the gate's own green record carries a pass over an unchanged tree instead.
// A nested gate run checks Pool.Inherited and runs inside its parent's slot without acquiring.
//
// The non-tier holders hold one slot and set their own Go flags through HolderEnv.
// Each passes the `-p` value it reads from the pool's Limits.Tier0Parallel at its acquire,
// so a hub that declares `tier0` bounds them by its parallel and a hub that declares none adds no `-p`.
// They are a card's `**Verify:**` run, a drift command, the plan lint's Go type load and every mode of `cmd/testtiming`.
//
// # Prebuilt store
//
// Pool.PrebuiltDir is `<Dir>/prebuilt`.
// A tagged gate run keeps the `lym` it built there under a name derived from the binary's content,
// so PrebuiltLymEnv holds the same value for every run on an unchanged tree.
// The gate run that writes the store also prunes it.
//
// # Config
//
// The hub-wide `gate` config module lives at `<BoardDir>/_lym/config/gate.yaml`, seeded from template.yaml by reconcile.
// `slots` and `tier_parallel` take a machine-local override at `<BoardDir>/.lym/config/gate.machine.yaml`.
// The override is never reconciled and is read at every acquire; every other key stays hub-file only.
// The machine override is a sibling map and not a path under `tiers`.
// An open map is one unit in the layer merge, so a partial `tiers` layer would replace all of the hub's tiers.
// A standalone worktree has no machine layer and sets the key in its own file.
// LoadConfig reads the file strictly and refuses a retired key naming it.
// It also refuses an invalid or inconsistent tier, build, gate key or bound naming the key and `lym config gate`.
// An absent hub file is ErrConfigAbsent with the way forward `lym fabric reconcile`.
// LoadRefuseRaw is the session launch's read: it needs `refuse_raw` alone,
// so a failure in any other key is logged as a warning and the list is read from the merged stack.
//
// `tiers` and `build` are executable keys, since a tier or build command runs any shell command in a task worktree.
// `gates`, `scoper`, `refuse_raw` and `inert_paths` are guard keys.
// All of them are hub-file only.
// `tiers` is an open map keyed `tier0`, `tier1`, and so on, each a Tier.
// `gates` is a per-name map from a gate key to a GateKey,
// so a hub file that sets one key keeps the template's value for every other.
// A GateKey's scope is `card`, `diff`, `all` or ScopeMarked, and only the `card` and `batch` keys take `card`.
// UpkeepGateKey names the key of the upkeep run, whose template scope is ScopeMarked.
// A GateKey may hold `lints`, which run after its tiers.
// It may also hold a `fixer` on the `card` key only.
// Config load fills a template key a tuned `gates` entry lacks into that entry.
// It ignores a key the GateKey does not hold, such as the `reports` list of an older template.
// A Tier's `manual: true` keeps it out of a `lym gate test --upto` range.
// A Tier's `carry: false` keeps a green run from being carried over an unchanged tree.
// A Tier's `memory_mb` caps the memory its command holds with every process it starts.
// A tier's command may hold TestTimeoutPlaceholder, rendered from `test_timeout_min`.
// It may also hold TestTargetsPlaceholder, which HoldsTargets counts as taking targets.
// `inert_paths` lists glob patterns of files no test reads; a pattern `path.Match` rejects matches nothing.
// A `tier_parallel` entry for a tier the config does not declare is dropped with a logged warning.
//
// Cross-references are checked at use, never at load.
// The template and a hub whose gate keys name an undeclared tier therefore both load.
// Config.Gate refuses a key the config does not hold and an empty tier list.
// It also refuses a hub that declares tiers but no `tier0`, and a listed tier the hub does not declare.
// A hub that declares no tier at all gets the key with no tiers and no refusal,
// so a fresh hub with no tests yet runs the key's lints and no tier.
// Each refusal names `lym config gate`.
// The call renderers, such as Config.GateCall and Config.AgentGateCall, render only a form the hub's config can run.
//
// A recipe row's `gate_key` setting names the gate key it reads in place of its lower-cased row name.
// The shipped recipes and template.yaml hold which row reads which key.
//
// # Watchdog and timeout lines
//
// The gate prints a line after a CPU-watchdog quit, a tier or build timeout, a runner crash or a memory pressure stop.
// Each such line has one renderer.
// It also has one finder.
// A site therefore tells a runtime's crash, or a memory limit other load filled, from a failing test.
//
// # Red-first call
//
// `lym gate red-first --gate <key>` runs the tests a diff adds or changes on its merge base.
// A parent's decision that names a bug no test can reach lifts the guard.
// Exit 0 means a test went red on the base;
// any other code is a refusal, a call that cannot judge the tests, or a run that ran tests and judged none of them.
//
// # Run log
//
// RunLogDir is `<gateDir>/runs`, inside the board's never-tracked `.lym` in a hub and the worktree's own outside one.
// The records therefore outlive the worktree.
// WriteRunRecord writes one RunRecord per tier run, and the build step's record has its `build` field set.
// ReadRunRecords skips a file that does not decode.
// The record is a domain log that no run reads to proceed.
//
// # Wait records
//
// A waiter that has no verifytree marker keeps a Wait record.
// It is one uniquely named YAML file in WaitDir under its worktree's anchor root.
// WriteWait creates it and the writer removes it; ReadWaits returns only the records whose process is alive.
package gateslot
