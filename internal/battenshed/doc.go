// Package battenshed owns the task-worktree batten producers.
// The producers create the task worktree and seed its own inner run.
// They run that run inside the worktree to a terminal state, land it into the prime, and tear the pair down.
// Any producer list may name them by reference.
// The package also declares batten's config module, batten.yaml (config.go).
//
// Told-geometry tier: this package takes every absolute path it operates on from its caller,
// and it has no direct production import of internal/lymcwd, per PATTERN-cwd-resolution.
// Its seam_enforcement_test.go enforces that membership mechanically.
//
// This package describes one repository throughout.
// It is not in the Fabric Vocabulary Invariant's owner set.
// None of its identifiers, string literals, or comments may name either fabric-internal side;
// write "the task worktree" and "the pair" instead.
//
// A batten run is one pass after another:
// "lym batten run <slug>" runs one pass and exits.
// The run's parent worktree's dobby starts it, or an operator by hand in a terminal.
// A second batten for the slug is refused by the run's own driver lock inside the pass.
// The lifecycle steps through the Go driver,
// so an unexpected stop meets the driver's rule table and a cold tackler before the orch.
//
// Worktree-Create claims the entry and skips a complete pair.
// A beam member's Worktree-Create refuses while its beam's run holds the member's entry,
// and the refused member keeps its claim, so it stays an open member that stops the beam's landing.
//
// A resume, "lym batten run" over an existing status file, first runs the origin reconcile of the slug's run scope.
// It refuses an entry origin ended, a stopped entry, and an entry another machine owns.
// Each refusal names the way forward (a raddle run standing at Raddle-File is the exception and resumes).
// An active entry resumes, and so does an unreachable origin, with a Warn;
// then the resume reads the local board, refuses a task it shows owned elsewhere, and writes no owner.
// A resume at Worktree-Teardown whose task worktree is gone runs no reconcile.
// A fresh start runs no reconcile: it pulls the board, which needs origin.
// It refuses a task another machine owns before anything is seeded or claimed.
//
// Between Seed-Child and Run-Shed, the Command rows run one shell command each:
// Open-Window, Seed-IDE and Open-IDE.
// Each runs only when its setting (batten.open_window, batten.seed_ide, batten.open_ide) is on, and is Done either way;
// a failed command only warns.
// The settings are read from the pair's own config stack, so a task's run_settings sets them per task.
// The batten.open_window_command, batten.seed_ide_command and batten.open_ide_command keys are read from two layers only:
// the template and the board default layer.
// Another layer's value is ignored with a warning.
// A command may hold the placeholders {lym}, {slug}, {task_dir} and {title}.
// A run resumed past Open-Window runs that command again when batten.open_window is on and no client is attached.
//
// The InnerRun row checks its child once per Call and never waits inside one:
// a Call is one pass, which the scheduling dobby runs again and again.
// A running child is checked without a time limit.
// It is bounded by "lym batten pause" (honoured at the next pass), cancellation and the notices.
// A running child whose driver is dead and whose status file does not change yields on every pass;
// the dobby's driver-dead notice is the only signal.
// A running child whose spawn this batten process has not confirmed is spawned again.
// It is not respawned when its reed state holds a live driver strand or its run lock or Go driver lock is held.
// The InnerRun row also reads the hub's pause record through the Pause seam.
// While it covers the slug, the row spawns, resumes and revives nothing.
// RespawnCapReached reads the respawn count for a reader outside the row.
//
// The InnerRun row notifies through the injected Notify seam (notice.go).
// A notice is one line, sent once per episode and informational: a delivery failure is only warned about.
// A halt, an awaiting hand-off and an owner change each send one.
// The dobby that drives the run, never the row, sends a running child's notices.
// A done child sends none.
// A parked stop's notice waits for the driver's stop report.
// That way the orch is not told "report none yet" for a stop the arbiter or tackler may settle.
// Every wait is bounded, no delay loses a notice, and a batten restart never repeats a posted one.
//
// A child that halts (blocked, paused or failed) is a budget-exempt wait, not a failure of the Run-Shed row:
// batten never spawns or resumes a halted child, bar the binary re-step below.
// The reason names the child's state, error and producer and gives the resume command ("lym loom resume").
// A halted or awaiting child whose driver strand is dead has its pair's strands revived once per halt,
// and the operator's resume then wakes the driver.
// A run halted at Parent-Align or Finalize gets the same notice and no goto: the recipe's own rows fix those failures.
// A halted go child is also re-stepped once per halt and per lym build (innerrun_binaryrestep.go),
// so a fix deployed after a Go driver handed back lands without a parent turn.
// A landing halt marked by `InnerRunDeps.HandBack`, and an escalate halt the arbiter judges first, are not re-stepped.
//
// Seed-Child writes the task worktree's local config layer from the board task's run settings.
// The settings are laid key by key over the profile block of the recipe the row resolved (see `internal/runprofile`).
// It rewrites the whole layer from the current board entry on every Call,
// so a hand edit to the pair's local layer before a resume is lost.
// An unknown profile, a profile with no block for the recipe, or a key that fails validation stands the row Stuck.
// The reason names the way forward: correct the task's `profile` or `run_settings`, then "lym batten run <slug>".
//
// Seed-Child and the InnerRun row also keep the batten-watched marker of the task worktree's run (MarkWatched).
// It holds this batten's pid while its notices have a destination that is also the child driver's parent.
// A driver launched watched keeps the silent rule.
// The run's stops stay unannounced until a batten runs again and sends the notices not yet sent.
//
// With deps.ParentLock wired, every probe pass releases the parent lock record of the child's run.
// It does so once every agent of its owner has been idle for the idle bound.
// Batten releases only a record naming its own child's run.
//
// With deps.OwnerCheck wired, the check also re-checks the run's owner (innerrun_owner.go).
// A verdict of another machine records that owner and releases the parent lock record.
// It also sends one notice naming the owner and "lym adopt <subject>", and holds every spawn while the owner stands.
// A Finalize already past its parent-side merge keeps its record and finishes its landing;
// FinalizePastMerge is that decision, for a caller outside the wait.
// A spawn on an existing run reads a ChildOwnedElsewhereError as that verdict.
//
// A driver that finishes normally leaves its strand and its run directory behind, by design.
// Only the whole-worktree teardown row cleans up, by removing the worktree they live in.
// The done arm waits for the driver strand up to the driver-exit grace window;
// a driver alive past it is ended by the teardown's session shutdown and recorded as an abandoned session.
//
// Worktree-Teardown has two halves, session shutdown and worktree removal,
// which call the pair-teardown composite (internal/pairteardown) as its EndSession and RemovePair phases.
// The push first removes the bench pairs a docs bench's darn writer made, so none outlives the run.
// That removal holds no prime lock.
// A pair of which nothing remains is the done post-condition for both halves.
// The row pushes nothing itself: between two holds of the prime lock it hands the pair's push on, with no lock held.
// The second hold removes the pair if it reads level once, and otherwise ends the row Done with the pair standing,
// so the row waits on no network and never loops.
// With git or pushes off nothing of the pair is pushed, and the pair is removed without that read.
// The bench pairs' archive tags are pushed whatever those switches say.
//
// Every producer that holds the hub's prime lock waits for a contended lock instead of halting on the first try.
// It returns Stuck only when the bound is spent (primelockwait.go).
// Worktree-Create hands its create closure a HoldPrimeLock.
// Runs creating different pairs thereby overlap on everything but these steps:
// those that read the prime's tree or write the hub's worktree registry.
// Worktree-Create halts blocked at its own row when the create closure returns a PreconditionError.
// No bounce budget is spent, since no tackler can settle the precondition,
// and the Go driver hands such a halt back.
// battenshed never imports fabric, so the closure alone builds the error.
//
// The Land row lands a child whose Run-Shed returned Done.
// Each step is idempotent, so a resume re-runs the row from the top.
// It takes the hub-level land lock for its whole body and pulls the prime.
// It runs the hub-wide `landing.deploy` command for a parent branch in `landing.deploy_branches`.
// It then prunes the board.
// Batten never pushes the prime.
// A clean landing posts no notice; a stop on the pull's records side posts one.
// A failed step is Stuck naming it.
// A pause seen during the land-lock or dirty-prime wait halts the row blocked, with no notice;
// the way back is `lym batten goto <slug> --to Land`, which clears the pause flag, then `lym batten run <slug>`.
//
// A Land or Worktree-Teardown stop the driver gave back sends one notice per stop (rownotice.go),
// so the orch is told only what the rule table, the tackler and the arbiter did not settle.
// Every marker that must last one batten run is named in runMarkerSuffixes.
// ClearRunMarkers clears them where batten seeds a fresh status.
//
// The package declares its own unexported entryErr and cancelErr helpers (ctx.go).
// It declares its own reportStuck carrier too (stuck.go).
// Each producer-owning package carries its own copy of them,
// so that no package depends on another only to share them.
package battenshed
