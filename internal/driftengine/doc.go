// Package driftengine owns regression around a task: the hub-wide `drift` config module, the drift paths, and the records and runs the Drift rows read and write.
// Regression is no test tier: it records a profile of the suite's behavior on the task's branch point and compares the task's code against it, and `lyx gate test` never runs it.
//
// `drift.yaml` is a hub-wide config module with these keys: `enabled`, which switches the Drift rows on through their entry gate; `capture` and `compare`, each a command step written whole as `{command, dir, env, timeout_min}`; `baseline_model` and `verify_model`, the model-specs of the two agents; and `verify_max_runs`, the run budget of one Drift-Verify agent session.
// A declared command holds the placeholder `{baseline_dir}`, the directory the capture writes its profile files to and the compare reads them from, and may hold `{skip_file}`, a file naming one skipped test per line.
// `capture` and `compare` are executable keys, so only the template and the hub's board default layer set them, with `lyx config drift --set 'capture={command: ..., timeout_min: N}'`.
// LoadConfig loads the file strictly and refuses a declared step without `{baseline_dir}` or with a `timeout_min` below 1, a `verify_max_runs` below 1 and a model-spec that fails its grammar, each naming the key.
// Config.Step returns a declared step and refuses an undeclared one, so every row and verb that runs a command goes through it.
//
// Paths is the one declarer of every drift path.
// The durable directory `_lyx/drift/` holds the scope record, the two agents' reports and the logs of every run; the ephemeral directory `.lyx/drift/` holds the capture record, the capture lock, the compare marker, the baseline and side-capture directories, the skip file and the run counter.
//
// Every record is JSON written through `internal/state`, so an unknown key survives a rewrite.
// The scope record (`scope.json`) holds the recorded scope (`off`, `partial` or `full`) and its skipped tests, when the baseline was awaited or deferred, a waiver, the accepted and withdrawn tests, the unruled tests the last decision halt named and the time of the last gate halt.
// The capture record (`capture.json`) holds the state of the latest main capture (`running`, `passed` or `failed`) with its pid, HEAD, times, exit code, reason and log.
// CountedCapture decides whether a capture counts as the baseline: a passed record, and under a recipe with an await row also an `awaited_at` in the scope record.
// The run counter (`runs.json`) holds how many compare or side-capture runs the current Drift-Verify agent session took, the commit its attempt started at and whether the agent-session mark is open.
// TakeRun refuses once the budget is spent, ResetRuns starts a fresh attempt with the whole budget, and CloseAgentSession closes the mark while keeping the count and the attempt's head.
// A verb reads the mark with AgentSessionOpen to tell the agent session from the row's own mechanical work.
//
// # Capture
//
// RunCapture is the main capture: it takes the capture lock with a bounded non-blocking retry, so a reader's probe never makes it refuse, runs the caller's precondition under the lock and writes a `running` capture record naming the HEAD that precondition checked.
// It moves a non-empty baseline directory to the first free numbered backup beside it, writes the skip file, waits for one gate slot as a non-tier holder with no deadline, and runs the declared command in its shell with `{baseline_dir}` and `{skip_file}` rendered as quoted paths and the command's stdout and stderr the open log file.
// A timeout, counted from the command's start, kills the command's whole tree.
// The record ends `passed` for exit 0 with at least one file under the baseline directory, else `failed` with the reason, and then the slot and the lock are released.
// RunCapture holds no job object of its own, since it also runs in a row's step process; the detached child holds the capture's named job (CaptureJobName) around it.
// RunSideCapture takes no lock and writes no record: it runs the same command into the next free numbered directory under the side directory.
// CheckMainCapture is the main capture's precondition: the run is at Drift-Baseline, or blocked at Drift-Await, HEAD equals the branch point and the tracked tree is clean; each refusal names its way forward.
// StartCapture spawns `drift capture-child` detached with breakaway, registers it under the run's processes with role `capture` and returns its pid only once the capture record carries that pid, in any state, since a capture can fail between two reads.
// A start Windows refuses for breakaway is retried plain-detached when the caller sets FallbackDetach, and is ErrBreakawayRefused otherwise.
// ProbeLock reads whether an OS lock is held without creating a directory, and Active reads the capture lock and the compare marker.
//
// # Compare
//
// RunCompare is the mechanical compare, on the runner the capture uses.
// It takes the compare marker, an OS lock, before it waits for its gate slot and holds it until it ends, so a compare queued for a slot reads as activity and one a dead compare left reads as none; a marker still held after a bounded retry, which rides out a reader's probe, is refused.
// It writes the skip file, holds one slot as a non-tier holder, renders `{baseline_dir}` as the baseline directory and `{skip_file}` as the skip file, and kills the command's tree at its timeout.
// The result carries the exit code, the log and whether the compare passed, which is exit 0 within the timeout; the compare writes no record and counts no run, since TakeRun is the caller's.
// SkipList is the scope's skipped tests followed by every accepted test, without duplicates; a withdrawn test is off the accepted list and so is compared again.
//
// # Verbs
//
// The `lyx drift` verbs' rules are functions over a VerbStatus: the run status's row, state and error, whether the recipe has a Drift-Await row, whether the capture lock is held and whether the agent-session mark is open.
// The agent session is a running status at Drift-Verify with the mark open; a caller at a halt is a blocked status, and a decision halt is one whose error opens with shedadapters.DecisionHaltPrefix.
// A verb run at a row it does not admit refuses naming that row and `lyx loom status`, and a refused verb writes nothing.
// RecordScope writes the scope record once, at Drift-Baseline, and refuses a skip list outside scope partial and a partial scope without one.
// Accept admits the agent session and a caller at a halt; a running status with the mark closed is the row's own work and refuses, and any other state refuses naming `lyx loom resume`.
// Under scope full the agent's acceptances are unruled and a caller's are ruled; a withdrawn test, and under scope full any test, is accepted by a caller only at a decision halt, and never by the agent session.
// Withdraw moves an accepted test to the withdrawn list, and RunCount reads the agent session's run count.
// Defer sets awaited_at with deferred while blocked at Drift-Await or at a Drift-Baseline capture halt, and refuses while the capture lock is held.
// Waive records a waiver at a blocked Drift-Verify only when no counted capture exists.
// CheckVerifyRow is the row check `lyx drift compare` and `lyx drift capture --side` run first, before they take a run of the agent session's budget.
//
// # Agents
//
// The Drift-Baseline and Drift-Verify agents are each one non-interactive lead seat, built by BaselineTable and VerifyTable over the stencils `drift-template-baseline` and `drift-template-verify`, which the seat engine reads at call time and renders with the parent and edit directives.
// The baseline seat's only output is `baseline.md` and the verify seat's `verify-report.md`: the scope record is Go-written, and a fresh spawn archives its outputs.
// The task's subject is read from the decision record in a loom run (SubjectDecisionRecord) and from the board entry in a darn run (SubjectBoardEntry), the record's `Added after Discussion` entries winning in both.
// The seats take the `plan` and `review` segment colors, their models from `baseline_model` and `verify_model`, and session timeouts that are package constants, since drift.yaml declares no timeout key.
// The verify seat also carries the hub's language skills and the PATTERN implementer directive, since its agent changes code.
// Neither table carries a gate; the Drift-Verify row sets it.
//
// # Drift-Baseline
//
// Deps holds the seams the three Drift producers share, and a producer fails loudly naming the field when a seam it reads is nil.
// Row holds a row's portable config: the subject its agent reads the task from and WaitCapture, darn's `capture: wait`.
// DriftBaseline has the baseline agent record the scope and write baseline.md when either is missing, the agent spawned with a note that the scope is already recorded when it is, and a Done agent with no scope record is a Stuck.
// Scope off, a set awaited_at and a passed capture take no capture; a capture holding the lock is waited on under WaitCapture, by awaitCaptureEnd from one second doubling to thirty, and left running otherwise.
// Otherwise it captures again: an undeclared capture command is a Stuck naming `lyx config drift`, and a refusal of the capture's precondition is a Stuck whose reason opens with CaptureHaltPrefix and names `lyx drift defer` with `lyx loom resume`, and on a dirty tree restoring or stashing the listed paths.
// Without WaitCapture the capture is the detached child, and the pass returns Done once the child's record exists, so the plan is written meanwhile;
// with WaitCapture, and when Windows refuses the detached start breakaway, the capture runs in the step's own process, so its command dies with the step.
// A failed capture is a Stuck opening with CaptureHaltPrefix and naming the log, `lyx drift capture` with `lyx loom resume`, and `lyx drift defer` with `lyx loom resume`; the row carries no on_stuck, so each Stuck halts the run.
// Every pass commits through Deps.Commit on its way out: the scope record and baseline.md while a detached capture may still write its logs, and all of the durable drift directory once a capture has ended in this process.
//
// # Drift-Await
//
// DriftAwait is the mechanical row that holds the run until the baseline exists; it spawns nothing and commits all of the durable drift directory, the capture log included, on every way out.
// It is Done with no scope record, with scope off, and with awaited_at set whatever HEAD is, so a later entry after the code changed passes.
// A capture holding the lock is waited on by awaitCaptureEnd with no bound of its own, and the capture record is read only after the lock reads free, so a capture that just ended is read as ended.
// A passed capture taken at HEAD is Done after the row writes awaited_at into the scope record, which makes the capture the counted one.
// A missing record, a failed one, and a running one whose lock is free are Stuck, naming the log when there is one, as is a passed record taken at another commit than HEAD.
// While HEAD is the branch point a Stuck names `lyx drift capture` followed by `lyx loom resume`, which captures again and re-enters the row, and `lyx drift defer` followed by `lyx loom resume`;
// once HEAD moved it names only `lyx drift defer` followed by `lyx loom resume`, since no capture may run on changed code.
// No Stuck of the row carries the decision halt prefix, so the arbiter may settle one with the way forward it names.
// An adopting machine has no `.lyx/drift/`, so before awaited_at the row halts on the missing record and `lyx drift capture` captures there.
//
// # Drift-Verify
//
// DriftVerify runs the mechanical compare and, when it shows drift, one strong agent that accepts the intended drift and fixes the rest.
// It commits all of the durable drift directory on every way out, and runs these steps in order:
// no scope record, scope off and a waived scope are Done with no compare;
// an open agent-session mark means an interrupted entry may have left a live agent, which a probe of the agent's table decides, and a live chair goes straight to the agent step to be resumed while a closed mark or an empty probe closes the mark;
// under scope full the acceptances the parent stood by are ruled, an unruled acceptance is a decision halt, and a missing counted capture is a decision halt under every scope that names `lyx drift waive`;
// an undeclared compare is a Stuck naming `lyx config drift`;
// a dirty tracked tree is a Stuck, and a resume from a gate halt runs the told verify entry once over the agent's commits;
// then the compare runs, and a pass is Done.
// A failing compare resets the run counter at the current HEAD and spawns one agent whose table gates, in order, on a clean tree, on the verify entry, which runs only once the session committed code, and on a final compare that ends the session when drift remains.
// The agent's end closes the mark; a Done under scope full is checked again for unruled acceptances.
//
// A Stuck opens with shedadapters.DecisionHaltPrefix, so the stop chain hands it to the parent, under scope full, for the missing counted capture, and under scope partial for a failing final compare while a test is withdrawn.
// Under scope full a decision halt names the unruled acceptances and the parent's ways forward for them, `lyx loom resume` to stand by them or `lyx drift withdraw <test>` to reject one, and records the names and the time in the scope record; the parent's resume then rules them.
// The entry that follows a halt it recorded recognizes it from the run's history: the last entry is a Stuck of this row whose latest time is at or after the recorded time, compared at second precision.
// gate_halt_at is written by each dirty-tree and test-gate halt, only when unset, and survives the halts before the compare, so a resume that halts at one of them runs the verify gate once more; an entry that is no resume from it clears the mark first.
// The row carries no on_stuck, the agent is no perch, and no review follows its fixes.
package driftengine
