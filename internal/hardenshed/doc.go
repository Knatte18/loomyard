// Package hardenshed holds the Hardener campaign's engine.
// The campaign is the autonomous behaviour review that drives a live-substrate module against throwaway sandbox hubs.
// The package takes absolute paths and told seams.
// It imports neither `internal/fabriccli`, `internal/configreg` nor `internal/lymcwd`,
// so `configreg` can register its config unit without a cycle; the CLI layer tells it what needs either.
//
// # Config
//
// The hub-wide `hardener` config module is `hardener.yaml`, whose keys Config mirrors with the defaults of template.yaml.
// ConfigSpec classifies `auto_accept`, `reviewer_fan`, `fixer_fan` and `tree_scope` as guard keys.
// Only the hub file sets a guard key, and a task's `run_settings` setting one is refused at seed with that way forward.
// Every other key is open to a task's `run_settings`.
// `tree_scope` is an open map, which ConfigOpenMaps names.
// LoadConfig loads the unit strictly through `configengine`, then runs CheckValues over the resolved document.
// It normalizes `tree_scope` through the one tree-key rule darn's `tree_scope` takes.
// CheckValues refuses a `concurrency`, `live_attempts`, `verify_attempts` or `hard_max_bounces` below 1.
// It refuses a negative `max_bounces`.
// It refuses a `reviewer`, `fixer`, `judge` or `seed` entry whose model-spec does not parse.
// A refusal of LoadConfig names the write for the layer the value comes from, then `lym loom resume`.
//
// # Campaign check
//
// CheckCampaign is Preflight's check of a campaign, and returns the first refusal in a fixed order.
// Each refusal names its way forward and ends with `lym loom resume`.
// It refuses a campaign with no package or a package the worktree lacks, and a live gate key the hub did not set.
// It refuses a live key naming the billed tier, and a copies key naming more than one tier while copies run.
// It refuses a package holding no test of a tier a live key runs.
// It refuses a reviewer list that repeats a model in neighbouring rounds, and a soft round cap above the hard one.
// Copies run when the live tests drive no LLM session and the concurrency is above 1.
// Rotate expands a per-round model list cyclically, so a round past the list's length never clamps to its last entry.
//
// # Live gate
//
// NewLiveGate is Go's independent check of each fix round, run through `lym gate test` calls that take their own slots.
// Each call runs the `harden-live` key once.
// It then runs the `harden-live-copies` key with the configured copies.
// That second run is made only when the live tests drive no LLM session and the concurrency is above 1.
// The gate names keys only, never a tier.
// It writes the round's live report, `harden-live-<round>.json` in the run directory.
// ReadLiveReport is the report's sole reader.
// The report's mark is red when the `harden-live` run failed or started no test binary, and green otherwise.
// A copies run never changes the mark: a red copies run, one that started none and a refused one are recorded beside it.
// A red report lists the `harden-live` run's failing lines as deferred findings.
// The report also names the packages whose test files hold no `tmuxkit.Main(` call, as outside the stray-state cover.
// A refused `harden-live` call, or a call that could not run, is returned as an error and writes no report.
//
// # Sandbox hubs
//
// NewSandbox makes a throwaway hub for the live tests, writing only under the run's sandbox root.
// SandboxRoot derives that root from the worktree root, so every process of a run names the same directory.
// The package names the hub's two bares by role, the code bare and the records bare, and never by side name.
// A hub is made in a container of its own: the bares are seeded, the hub is cloned, and the sandbox config is committed.
// The `.dev-bin` lym is rebuilt only when its revision is not the worktree HEAD's or it is marked modified.
// The root's hub list, `hubs.json`, records every hub made, and ListSandbox reads it alone.
// A failure after the container is made removes the container before the error returns.
//
// # Sandbox reaper
//
// Reap ends what the sandbox hubs left running, then empties the sandbox root, at each ReapPoint a campaign names.
// It ends each recorded hub's registered processes through the run-process owner's end sequence, sparing the caller.
// It stops each recorded hub's reed server whether or not the hub's directory is still there.
// It prunes the hub's list entry once the server is stopped or none was live.
// Only a process that names the root and that no record names is ended as an orphan, never the caller or its ancestors.
// A process names the root through its working directory, executable or command line,
// and the root counts both as written and with its links resolved, the form the kernel reports a working directory in.
// It returns one StrayEntry per server and process it found.
// A ReapStart entry is marked as left by an interrupted row.
// Off Linux the process scan is skipped and only the servers are stopped.
// Every entry under the root but the hub list is removed, so no bare repository is left for a cleanup to refuse.
//
// # Round rows
//
// WrapRow decorates a Harden round row, a RowReview or a RowFix, with a reap at its start and at its end.
// It fixes the row's round once, at the start of the call, so a resumed row keeps the round it began.
// A review row writes the round's models file, `harden-models-<round>.json`.
// A fix row records the head it first started at in the round's head file, `harden-head-<round>.json`.
// A call resumed after a crash reuses that head, so the commits file lists every commit the round's calls made.
// A fix row writes the round's commits file, `harden-commits-<round>.json`, when its call returned no error.
// Every reap adds its strays to the round's stray report, `harden-strays-<round>.json`.
// A fix row whose call returned no error then archives the round through ArchiveRound.
// ArchiveRound copies the round's files and the latest Bouncer ledger into `_lym/harden/round-<round>/` and commits them.
// It replaces the directory, so a second archive of one round is harmless.
// ArchiveFinalLedger copies the latest ledger into `_lym/harden/`.
//
// # Method
//
// A campaign hardens a live-substrate module before it merges, by running it rather than reading it.
// Such a module drives real processes, sockets or a tmux server,
// and its defects hide in composed, stateful, timing-sensitive behaviour that a green hermetic test does not prove.
// Text review reads an artifact and judges it; behaviour review drives the substrate and judges what it does.
// The tell that a module needs a campaign is that its unit tests pass but nobody trusts it under load.
// A crash or a concurrency defect is the usual cause.
//
// Each round is independent.
// A fresh reviewer forms its own findings, then a fixer works them, and neither grades its own work.
// A fixer that has just fixed something is motivated to declare it fixed.
// So a round's own "merge-ready" verdict never gates the campaign.
// Go gates each round instead, by re-running the live gate on the committed tree and believing only what it observes.
// Rounds rotate models, since different models miss different things.
// Agreement across models is stronger evidence than repeated passes by one.
// A round's assignment follows from the previous round's residue, never from "review it again".
// When findings repeat a shape, the round enumerates that shape.
// When the countable classes are closed, it drives the regions no round has reached.
//
// The merge bar is one green live run with zero stray state.
// A concurrent run of the suite is a diagnostic amplifier that drives out teardown races and leaked state.
// It is never a merge gate, because a timeout under an artificial load is not a defect.
//
// A clean run proves only that its scenario reached the code.
// A green scenario that never entered the mechanism it claims to exercise is indistinguishable from a fix,
// so a regression test is proven by sabotaging the mechanism and watching the test fail.
//
// A converged campaign proves what its rounds covered and no more.
// A round with no finding in an enumerated class shows that class clean, not the module.
// A scenario that cannot be driven on the host, such as another platform's path behaviour, stays an unproven gap.
// A convergence verdict names those limits, so the next campaign does not learn to claim more than it proved.
package hardenshed
