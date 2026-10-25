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
// Holders reads every slot record in the directory, so a holder of a slot above a lowered count still shows, and fills each Holder's slot number from its record's file name, which the record itself never carries.
// Pool.Draining reports whether the drain lock is held, and creates no file in a pool that never had one.
//
// # Inheritance
//
// A slot's environment is Lease.InheritEnv for a tier run and HolderEnv for a non-tier holder.
// `-trimpath` makes builds from two checkouts byte-identical and their cache keys independent of the checkout's directory,
// so a fresh worktree hits the build and test cache and a host scanner that holds each unseen executable sees known hashes.
// It also makes `runtime.Caller` report module-relative file paths, so no code a gate run builds locates the repo root from them alone.
// Lease.InheritEnv returns the environment of a tier run: only InheritEnv is set, naming the lease's first slot's lock path, and no GOFLAGS is appended, since a tier's command and env set its own parallelism.
// HolderEnv returns the environment of a non-tier slot holder's child: GOFLAGS gets `-trimpath` and, when the holder passes a tier0 parallel above 0, `-p=<n>`, and a non-nil lease sets InheritEnv.
// Limits.Tier0Parallel carries `tier0`'s effective parallel, this machine's override included, and is 0 when the hub declares no `tier0`, so a holder reads it from the pool's Limits at its acquire.
// A nested gate run checks Pool.Inherited on that variable and runs inside its parent's slot without acquiring, whatever slots its own tier asks for.
//
// The non-tier holders hold one slot and set their own Go flags through HolderEnv:
// the card `**Verify:**` runner, the plan gate's type load and `cmd/testtiming`, which holds one slot for its whole run in every mode.
// Each passes the `-p` value it reads from the pool's Limits at its acquire, so a hub that declares `tier0` bounds them by its parallel and a hub that declares none adds no `-p`.
//
// # Prebuilt store
//
// Pool.PrebuiltDir is `<Dir>/prebuilt`, where a tagged gate run keeps the `lyx` it built under a name derived from the binary's content, so PrebuiltLyxEnv holds the same value for every run on an unchanged tree.
// The gate run that writes the store also prunes it.
//
// # Config
//
// The hub-wide `gate` config module keeps `slots`, `busy_quit_sec` and the tier keys below at `<BoardDir>/_lyx/config/gate.yaml`, seeded from template.yaml by the hub-wide reconcile.
// `slots` and `tier_parallel` take a machine-local override at `<BoardDir>/.lyx/config/gate.machine.yaml`, never reconciled and read at every acquire; every other key stays hub-file only.
// `cli_wait_sec` is a retired key: every load drops it and reconcile prunes it, since the slot wait is bounded by the keep-cache-warm cap with no knob.
// `go_parallel` is retired in the hub and machine layers alike, in favor of `tier_parallel`, with the same load and prune behavior.
// `guard_tier` is retired too: the guard step runs each guard test under the tier its file belongs to, so no key names one tier for them all, and the same load and prune behavior applies.
// LoadConfig reads it strictly and refuses any value below 1 naming its key, except `busy_quit_sec`, which accepts 0 and is refused below it; Config.Limits converts it to the Limits a Pool reads.
// An absent hub file is ErrConfigAbsent with the way forward `lyx fabric reconcile`; outside every hub, where the dirs tell no board, the file is the worktree's own and the way forward is `lyx config gate`, after creating the worktree's `_lyx` directory.
// TemplateConfig decodes the template alone, for a run outside every hub.
//
// The tier keys are `tiers`, `gates`, `scoper`, `refuse_raw`, `tier_parallel`, `build` and `inert_paths`.
// `tiers` is an open map keyed `tier0`, `tier1`, and so on, each a Tier written whole with one `lyx config gate --set 'tiers.tierN={...}'` flow mapping;
// `gates` is a per-name map from a gate key to a GateKey written `{tiers: [<n>...], scope: card|diff|all}`, so a hub file that sets one key keeps the template's value for every other, and `tier_parallel` an open map from a tier name to this machine's parallel for it.
// The keys lyx reads are `card` and `batch` (scope `card`), `webster`, `parent-align` and `finalize` (scope `all`), and `webster-fix`, `darn-write` and `code-burler-fix` (scope `diff`).
// `tiers` and `build` are executable keys, since a tier or build command runs any shell command in a task worktree, and `gates`, `scoper`, `refuse_raw` and `inert_paths` are guard keys, so all of them are hub-file only.
// `build` is an open map, so its sub-keys travel whole through the layer stack and reconcile never prunes a hub's build.
// A tier may also hold `filter`, its own test-filter expression, `shared_build: true`, which makes the tier depend on the build step, and `carry: false`, which keeps a green run from being carried over an unchanged tree; Tier.NoCarry is the negative form, so a zero Tier carries.
// `build` holds `about`, `command`, `dir` (default `.`), `env` and `timeout_min`; the template's `build: {}` is no build step, and a declared step has a non-empty Config.Build.Command.
// `inert_paths` lists glob patterns of files no test reads: Config.Inert matches a worktree-relative slash path through `path.Match`, a pattern without a slash against the path's base name and one with a slash against the whole path, and a pattern `path.Match` rejects matches nothing.
// Config.AllInert is true only when the list and the paths are both non-empty and every path is inert.
// The `dotnet` scoper derives the impacted C# test projects and classes from the type-name closure over test classes.
// The machine override is a sibling map rather than a path under `tiers`, because an open map is one unit in the layer merge, so a machine layer holding part of `tiers` would replace the hub's tiers whole.
// A tier's `manual: true` keeps it out of a `lyx gate test --upto` range, so it runs only when named by number.
// A declared tier's effective Parallel is its `tier_parallel` entry when one is set, from whichever layer sets it, and 1 when neither sets one; a standalone worktree has no machine layer and sets the key in its own file.
//
// LoadConfig refuses at load, naming the key and `lyx config gate`: a tier lacking `command`, `timeout_min` or `counter`, or `all_targets` when its command holds `{targets}`;
// `manual: true` on `tier0`, the tier a call with no tier flag runs; `parallel` or `timeout_min` below 1, `slots` neither `all` nor at least 1, a counter other than `go-json` or `vstest`, a scoper other than `go`, `none` or `dotnet`;
// a tier `filter` whose command holds no `{filter}`, a non-empty `build` lacking `command` or `timeout_min`, a build `timeout_min` below 1, and a tier with `shared_build: true` while the hub declares no build step;
// a `covers` entry not lower than its tier's number, a tier key not spelled `tier<N>`, a `refuse_raw` entry that is not words of letters, digits, `.`, `_` and `-` separated by single spaces, and a `tier_parallel` entry that is no integer of at least 1;
// a gate key whose scope is not `card`, `diff` or `all`, and a `card` scope on any key but `card` and `batch`.
// A `gates` entry still in the retired list shape, such as `card: [0]`, is refused naming `lyx fabric reconcile` as the way forward, since the reconcile runs MigrateConfig.
// A `tier_parallel` entry for a tier the config does not declare is dropped with a logged warning, so a tier the hub removes leaves no entry that fails the load.
// Cross-references are checked at use, never at load, so the template and a hub whose gate keys name an undeclared tier both load.
//
// At use, Config.Gate returns a key's tiers and scope and refuses a hub with no `tier0`, a key the config does not hold, an empty tier list, and a listed tier the hub does not declare; Config.TierList returns its tiers; Config.Tier refuses an undeclared number.
// Each refusal names `lyx config gate` and closes with the clause that a session that is not the prime reports status: FAILED and the orch takes the steps.
// Config.GateCall renders the one `lyx gate test --wait --gate <key>` call a site runs, followed by one `--card <id>` per card given.
// MigrateConfig prunes the retired `gates.publish` list and the retired `guard_tier` key from a gate.yaml document and migrates the retired lists, so reconcile rewrites the hub file:
// `landing` becomes `parent-align` and `finalize` with scope `all` and `darn-write` and `code-burler-fix` with scope `diff`, `final` becomes `webster` with scope `all`, `round` becomes `webster-fix` with scope `diff`, and a `card` or `batch` list becomes a mapping with scope `card`.
// A `merge-in` key is renamed to `parent-align` with its value, and dropped beside an existing `parent-align`.
// A key the file already holds in the new shape keeps its value, and a second pass changes nothing.
// Config.TiersValue renders the tier lines and the Go-read gate keys a prompt takes, or `none` when no tier is declared.
//
// # Tier plan
//
// Each shipped recipe row that runs a test gate reads one key, whose template scope is its default:
//
//	Recipe      Row                                     Key              Scope
//	loom        Webster, per card                       card             card
//	loom        Webster, per batch                      batch            card
//	loom        Webster, verify fixers                  webster-fix      diff
//	loom        Webster, end gate                       webster          all
//	loom        Parent-Align                            parent-align     all
//	loom, darn  Code-Burler-Fix                         code-burler-fix  diff
//	loom, darn  Finalize                                finalize         all
//	darn        Darn-Write                              darn-write       diff
//	darn        Darn-Write, Code-Burler-Fix, Finalize   the row's key    the touched trees' targets
//
// The last line holds for a diff wholly under darn's `tree_scope`: the row runs its own key's tiers over the targets of the trees the diff touches, in place of the key's scope.
// Darn and loom share the keys of the rows they share.
// Their writers' gates differ by operator decision: Webster's end gate runs whole, and darn's writer runs the diff.
// A recipe row's `gate_key` setting names the key it reads in place of its lower-cased row name, and the merge fixer runs the key of the row that spawned it.
//
// # Watchdog and timeout lines
//
// The line the gate test's CPU watchdog prints after a quit is rendered by WatchdogLine and found by FindWatchdogQuit.
// BusyQuitWayForward is the way forward after one.
// The line the gate prints when a tier outlives its `timeout_min` is rendered by TierTimeoutLine and found by FindTierTimeout;
// it names the tier, the timeout and the whole-tier `lyx config gate --set` rewrite that raises it.
// The build step's timeout line is rendered by BuildTimeoutLine, names `lyx config gate --set 'build={...}'` as the rewrite, and FindTierTimeout finds it too and returns `build` as the tier.
//
// # Run log
//
// RunLogDir is `<gateDir>/runs`, under the board's never-tracked `.lyx` inside a hub and under the worktree's own outside one, so the records outlive the worktree.
// WriteRunRecord writes one RunRecord per tier run as `<stamp>-tier<N>-<pid>.json`, and the build step's record, whose `build` field is set, as `<stamp>-build-<pid>.json`, with the run's full output beside it as a `.log`, and prunes files older than 14 days first.
// ReadRunRecords reads a directory's records tolerantly, skipping a file that does not decode, for the reports that total them.
// The record is a domain log that no run reads to proceed.
// A tier the gate carried forward over an unchanged tree writes its record with `carried` set.
// A go-json tier's record lists a PackageLaunch per package whose test binary ran; a record written without the list reads with none.
// A tier the build step blocked by failing writes its record with `blocked_by` set to the failed build's log path.
// TierSummaryLine renders the one stderr line the gate prints after each tier run, marking a carried tier with `carried` after the scope form, and BuildSummaryLine renders the build step's line, `gate test: build before tier<N> wall <s>s exit <code> log <path>`, which no tier summary reader matches.
// FailedTier finds the first failing tier or build line in a log, with `build` true for a build line, so a site that keeps the gate's output can name the failing tier and tell a compile break from a test failure.
//
// # Wait records
//
// A waiter that has no verifytree marker keeps a Wait record, one uniquely named YAML file in WaitDir under its worktree's anchor root.
// WriteWait creates it and the writer removes it; ReadWaits returns only the records whose process is alive.
package gateslot
