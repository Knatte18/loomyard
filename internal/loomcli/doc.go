// Package loomcli is the `lym loom` command:
// it bootstraps and drives one task's phase machine per worktree on the generic shed engine.
// It carries the verbs through which the operator and the run's parent answer its gates.
//
// The run, step, status, pause and goto bodies are the shared `internal/shedverbs` verbs.
// They are mounted here with loom's own text.
// Friction-Reflect, the last row, runs the Tier 2 friction reflection under run and under step alike.
// It runs before the run records done.
// A `blocked` or `failed` halt, and an interrupted stop, write their Go halt note and spawn no reflection,
// so no stop holds the driver lock for a friction session and `lym loom resume` never waits on one;
// the note waits for the run's Friction-Reflect row, which reflects every note in the friction directory.
// An interrupted stop's note carries the `interrupted-stop` anomaly kind, never `producer-hard-failure`.
// Its reason is the stop's cause.
// It names the in-flight step's trace file, or its captured stderr, where the stop knows it.
// It records the state the status file holds after the stop.
// The state stays `paused` for a step that died before its resume write over a paused run.
// It also records an error the status file holds that the stop did not write.
// Such an error, the step's own producer failure for one, goes on a `status error:` line of its reason.
//
// A loom run's shed and routing come from one recipe-keyed builder in `internal/loomrecipe`.
// Its entry points are `loomrecipe.New` and `loomrecipe.Routing`; both take the run's recipe name, pawl's included.
// The row names a run parked under an old recipe may still carry are that recipe's retired-row data.
// They are read through `loomrecipe.RetiredRows`:
// arming renames them in the status file with no branch on the recipe name,
// and a recipe with none renames nothing.
// A row the recipe dropped is declared apart, in the recipe's `retired` map.
// It is read through `loomrecipe.RetiredRowNames`:
// arming renames nothing for it,
// and the shed moves a run parked at it to the row the map names and pauses it.
//
// The wiring branches on the recipe's rows and keys, never its name.
// It reads them through the `loomrecipe` accessors on the run's recipe, and an empty recipe reads as tapestry.
// The verify source follows the recipe's `verify_source` key.
// The tree-scope verify seams follow `verify_source: tree_scope`.
// The Darn-Write seams are filled for a recipe with a Darn-Write row.
// The perch-off reading follows the Code-Review rows' entry gate as well as the budget.
// The arras recipe's split perch, the Raddle-Review segment, reads loom.yaml's `split_*` keys as perch `split`.
// The merge fixer's plan directory and failure tracer follow a Plan-Write row.
// A recipe with a SamplerSession row loads the hub's `sampler.yaml` strictly in `wire`.
// `wire` refuses a malformed file before any row builds, and it fills the row's seams from `samplerDeps`.
// The row's entry stamp counts the times the run entered it, from the status file and the rows that route into it.
// `wireLanding` is passed through by both the `run` and `step` building paths,
// and `status` and `pause` never pass through it.
// It fills the session's start and stop, its liveness and its process registration as for PR-Review's session.
// It also fills the watch on the row's record that wakes the wait.
// A run's decision record follows its recipe's rows through `runRecordOf`:
// a recipe with a Beam-Await row keeps the beam's project record under the beam's slug,
// and every other recipe keeps the discussion record.
// A task run's review rows are told its record as the PATTERN declaration path.
// A beam's project record declares none.
// A loom recipe holds a landing's escalate halt for the arbiter first.
// Discussion-Bouncer's approve seam stores the approved decision record on the run's board entry.
// It does so through `storeDecisionRecord`.
// It logs and passes a done entry, and it fails the settle on any other store failure.
// The Motif block's undiscussed gate seam handles an entry flagged discussed.
// It writes the entry's stored record as the run's decision record.
// It writes a support log saying so when none exists, and it commits the discussion directory.
// The gate then passes over Discussion.
// `decision add` and `decision replace` copy the committed record onto the board entry after their commit.
// They copy it once the run is past that approval.
// On a recipe with no approval row they copy it when the entry carries a stored record.
// A beam's project record is never copied.
// They report a failed copy as `record_store_error` and never fail the verb.
// `decision store` makes the same copy by hand on the same runs.
// It admits the unnamed shell, the run's arbiter and its parent orch.
// Each status transition writes `<state> · <producer>` on the run's board entry, the run's start write included.
// The entry thus names the first row while it runs.
// A finished run of a recipe with a Finalize row writes nothing, its landing having marked the entry done.
// A finished run of a recipe with a RaddleFile row, a raddle or arras run, writes nothing either.
// Its Raddle-File row ended the entry done instead of as a Sketch, or a family filing turned the entry into its project.
// Such a filing writes the run's filing mark among the slug's run records in the prime.
// From then on, no status write of the run reaches the entry.
// A finished run of a recipe with neither a Finalize row nor a RaddleFile row, a motif run, makes its entry a Sketch.
// A Sketch is a note of the built-in type `sketch`.
// It leaves its run lock, never runs and waits for the operator's triage with its record stored.
// When the board refuses that, as while another entry depends on the entry:
// the run warns, leaves the entry draft and mails its parent the refusal with the ways forward.
// Any other failure of the write is retried at the run's next finished transition.
//
// `wire` fills `Env.GateFixer` for every recipe through `gateFixerSeam`.
// The seam wraps the gate of a Darn-Write, Plan-Write or Discussion-Write row whose `fixer` names a model-spec,
// so a failure at the writer's idle boundary goes to a cold fixer before the writer hears of it.
// The seam returns the spec unchanged on the prime and outside a hub, where no pair owns a fixer.
// The tables of the rows it wraps read that condition through `writerFixerOn`,
// so a writer whose gate has a fixer is told to run neither its validator nor, on Darn-Write, `lym gate test`.
// A fixer's records live in the run's scratch directory under `gatefix`.
// `rowVisitDir` keys each record directory by the row and the status history's length and newest repeat count,
// so a crash re-step finds the fixer it left, and a goto or a fresh entry starts clean.
// `fixerSpec` renders the prompt from the shared cold frame and one body stencil per gate entry:
// `gatefix-body-darn`, `gatefix-body-plan` or `gatefix-body-discussion`.
//
// The `lym loom` verbs serve loom runs only.
// `arm`, the subtree's entry, sets `loomPath` before `armAt`.
// `armAt` then refuses a recipe that does not declare the loom contract.
// It refuses right after the recipe is resolved and before any wiring:
// an embedded non-loom recipe is refused naming the `lym shed` verbs,
// and a seed naming no embedded recipe is refused naming the re-seed.
// It then refuses these verbs on a recipe with no Plan-Write row:
// `validate-plan`, `insert-card`, `remove-card` and `restore-renumber`.
// Such a recipe has no plan.
// It refuses `validate-parts` on a recipe with no Parts-Write row, which writes no parts file.
// It refuses `join-parts` on a recipe with no Parts-Plan-Write row, which has no parts to join.
// The target resolver the `circling` and `decision` groups share judges the target's own seed recipe the same way.
// `ArmAt`, which `lym shed` shares, leaves `loomPath` false and never refuses a recipe.
// `lym loom checklist` is exempt, being a read-only pane the pre-run never arms.
//
// A run seeded with the recipe `pawl` is a beam's life, batten's Seed-Child seeds it in the beam's pair.
// Its shed reads Preflight, Record-Write, Beam-Up, Beam-Await, Finalize and Beam-Down.
// It has no plan, no review round and no reroute edge.
// The recipe's `retired` map sends a run parked at Parent-Align, Beam-Describe or PR-Review back to Beam-Await.
// Its Record-Write writes the project record and its support log from the project entry's body,
// and it commits them under the beam's slug.
// wire fills `Env.Pawl` for it through `pawlDeps`:
// the run's seed id is the seed's start time;
// the status is written on the beam's project entry through the board;
// Beam-Up's one step is `orchcli.BeamUp` with no prepare, since the run already judged origin;
// the warnings its report carries are logged and posted to the beam's orch as one `PAWL` notice;
// no warning halts the row;
// `Deps.RetiredRows` is the recipe's retired row names.
// wire also sets `ShedPaths.RetiredMove` to `pawlRetiredMove` for a beam run,
// so a run moved off a retired row gets the landing's side effects,
// and it sets `ShedPaths.RetiredWayForward` to `lym loom resume` for every loom run:
// `pawlRetiredMove` ends the PR-Review session through the run's `RoleSession` records.
// It reads the scratch pull request's state through the pair's origin.
// It commits the landing directory in the beam's worktree.
// The beam's dobby is stopped through `runproc.EndRole` by its `RoleDobby` record.
// It is restarted through `orchcli.BeamUpWorktree` naming the beam, which brings the beam's orch up with it.
// Beam-Await's re-check reads `battencli.ReadBeamLandBlockers` and `battencli.BeamLandRefusal`.
// They take the beam's owner, its parent's owner and the beam's ready record.
// The re-check words each failure as `pawlRecheckLines` does.
// A failure names `lym adopt <beam>` or the parent's adopt subject, with `root` for the prime.
// A beam not flagged ready gets `lym beam ready <slug>`.
// A running shed also watches the land request's directory, so a request wakes Beam-Await at once.
// Its Finalize reads the hub's `finalize` gate key over the whole merged tree.
// It halts for the orch on a fixer that is spent or over its caps.
// A land check backstops the re-check:
// Finalize halts, behind its own prefix, while an open member or a pair forked from the beam still works.
// It reads `pawlLandBlockers` and names the beam's parent worktree as where a cancel runs.
//
// A member's Discussion-Write lead and darn writer get the project record directive.
// The directive is `loomengine.ProjectRecordDirective` over `projectRecordPaths`:
// the record of each beam above the pair, read in that beam's own worktree at each spawn, nearest first.
// `decision add <beam-slug>` resolves the beam's project record.
// It takes the beam's own orch as the caller its rule admits.
// It posts its notice to the live runs of the beam and of every member pair `memberPairs` reads under it.
//
// `decisionCallerRefusal` is the caller rule of `decision add` and `decision replace`.
// It admits a named session that is the run's parent orch;
// it admits the run's own arbiter strand with `--by arbiter`;
// it admits the strand a sampler run's Sampler-Session row has recorded as its session.
// `samplerStrandReader` admits that strand by the recipe's rows.
// It admits it only while the run's current row is the session row, whether or not the run is halted there.
// It admits it only with `--by operator`; any other label from it is refused naming `--by operator`.
// A strand it does not admit meets the parent rule.
//
// `decision add`, `replace` and `store` refuse a raddle run through `raddleFilingRefusal`.
// A run in perch mode, whose recipe rows hold the split perch, is refused at its Raddle-File row before the filing.
// That refusal names the goto to Raddle-Write.
// Once the filing is done, in any mode and at any row, the refusal names the filed parts.
// Each part is recorded on once released.
// It also names `lym board upsert --record-file` for a call on the family's project.
// A filing the raddle store has not marked done is seen from the entry's kind or from the board.
// Its refusal names `lym loom resume` first.
//
// `decision replace` writes a whole record from a file and commits it with a `Lym-Record-Replaced` trailer.
// It holds the run's decision lock, `holdDecisionLock`, which `add`, `store` and `writeCarryOver` take too,
// so its base check and its write are one step against every other Go writer of the record.
// `recordHistory` reads the record's versions from the newest such trailer's cut onward,
// so the dropped-entry check no longer holds the record to an entry a replace dropped.
//
// `parent-review answer` records the parent orch's answer to the run's latest Discussion-Review ask.
// It admits the operator's unnamed shell, the run's parent orch resolved at call time, and the strand the ask went to.
// A refused answer names `decision add` as the way forward, run by the run's parent orch or the operator's shell.
// An older ask while the latest awaits an answer is instead sent to review the record again under the latest ask.
// `parent-review await` is the chair's capped wait on the same ask.
// It reads the record once a second up to the caller's keep-warm cap.
// It sets an ask past its deadline timeout and writes nothing else.
// The wiring gives the Discussion-Review segment its parent seat, `discussionParentSeat`.
// On each call the seat reads `discussion_review_ask_parent` through the wiring's setting lookup.
// An unreadable value reads as false.
// It also uses the capped wait, the latest Discussion-Write call in the status history, and `NotifyParent`'s routing.
// It posts as `<shortname>:LOOM`, tagged with the pair's run.
//
// # Start
//
// `lym loom start` works in order.
// It resolves the recorded parent branch and seeds the status file when absent.
// It commits the seed into the fabric before anything else touches it.
// Every start or step commits whatever the friction directory holds along with the seed.
// A first seed clears the friction directory first, so the clear is committed with the seed.
// Notes the fabric carried in, such as a beam pawl run's halt note, never reach the task's reflection.
// They do not leave Preflight a dirty worktree.
// An unseeded run is seeded with the driver `shed.yaml`'s `driver_kind` and its layers resolve to, `llm` by default;
// a seeded run keeps its recorded driver whatever the key says later.
// It brings up the worktree's tmux session;
// it ensures the status strand on either driver, and it spawns the per-hub watchdog daemon best-effort.
// The status strand runs the hidden `lym loom checklist` verb in an overview window named for the run's recipe.
// The window is opened with the worktree's console title, as in `T4 [ly:P3] tapestry`.
// The recipe stands where the slug does.
// A part the hub or board does not give is left out.
// In a beam's session it is placed right after the runs strand's overview window.
// The verb draws the checklist frame: the recipe's rows in order, each entry numbered after its box.
// A review segment takes one number.
// Each row has its box.
// A row the run passed on a false entry gate and never visited is boxed `[-]`, apart from one not reached.
// Under a row come the time spent in it, its visit count and its visit lines, each carrying no output path.
// A visit that recorded gate spans gets a gate line per gate entry.
// The line opens with the UTC clock time its gate entry started.
// The visit after a gate row's refusal is marked as rework for it, and its time is added to the gate row's line.
// A review segment's rounds each carry their total.
// Every closed time is drawn to the second, and every time that includes the open visit is drawn in whole minutes.
// The frame ends with the run's state line and the wait line.
// The state line carries the note of what a running run waits on, the same note `lym loom status` reports.
// The window's title is read when the strand is added or replaced,
// so a live strand of the running build keeps its title,
// and a handle the board assigns later shows from the strand's next replacement.
// A Stuck visit whose status entry records where Shed routed it reads `bounced`,
// and an awaiting one reads `escalated`, in this window only.
// A block row's Done visit reads `entered`,
// and a row inlined from a block is named `<row> in <block path>` on its line and its visit lines.
// The verb reads the status file and the seed without arming a run, and reports no content size to reed.
// Right after ensuring the status strand, every start entry turns the session's follow state on.
// It also selects the run's relevant window.
// The start entries are a fresh run, a re-entrant start on a live run and a resume of a parked run.
// A failure there is logged and never fails the start.
// `lym loom resume`, which never brings reed up, leaves the follow state as it is.
// Unless a driver is already alive, it spawns the driver the seed records, which is never chosen by a flag;
// a second invocation while a driver runs ensures the substrate instead.
//
// A live loom driver parked at a hand-back is resumed by typing one line into its pane.
// Start refuses after a bounded wait when that pane is not ready.
// The driver may be busy, having resumed on its own.
// A pane holding a draft, an active operator or another busy reading is refused with the retryable kind `driver_busy`.
// The park marker is kept, and the message names the draft and the way forward;
// a resume line that never appeared stays a non-retryable refusal.
// The stop-report path in the resume line is picked once per park.
// It is kept in a resume-report file beside the park marker, keyed to the marker's content,
// so every rerun against one park types the identical line;
// a landed resume or a fresh spawn removes the file.
// A run halted at a hand-back (awaiting, blocked, paused or failed) has a live loom driver.
// Until that driver has written its park marker it is still writing its stop report.
// Start refuses with the kind `driver_not_parked` meanwhile;
// a live driver over a running run is left working.
// Before anything else, start and resume refuse a run whose status file exists and that another machine owns.
// The refusal has the kind `owned_elsewhere` and carries the owner's name.
// It names `lym adopt <subject>`, the beam or `root` the owner verdict carries.
// The owner is judged by a judge-only run-scope origin reconcile from the hub's prime.
// When origin is unreachable, the local board judges it instead.
// Nothing is written.
// Before spawning or resuming a driver, start refuses with the kind `merge_in_progress` over an unfinished merge.
// The refusal names the conflicted paths and the remedy:
// the fabric verbs for a fabric merge, and git for one fabric did not start.
// The exception is a run whose current producer is Parent-Align or Finalize.
// It goes through over a parked fabric merge-in of its own parent branch, since that row aborts and redoes it.
// A loom driver strand reed marked retiring is never adopted: start removes it and spawns a fresh driver.
//
// start returns once the driver's readiness signal confirms it is up.
// For a parked loom driver it returns once delivery of the resume line is verified.
// For the Go driver, `lym loom drive`, the signal is the driver lock being taken.
// The verb takes it first and holds it for life, while the run lock is free between steps;
// for a loom driver it is the provider TUI coming up ready within shuttle's `startup_timeout_s`.
// Any one-time startup gate its provider requires is dismissed along the way.
// A readiness refusal removes the driver strand, so the next start spawns a fresh one.
// The signal is checked only for a driver this invocation spawns.
// A later start returns over a strand left live without re-checking it.
// Shuttle leaves a strand live when it got no liveness answer from reed or could not remove the strand.
//
// A held driver lock with no live strand reads as a live Go driver:
// a run halted at a hand-back is refused with the kind `driver_not_parked` while its Go driver is still settling,
// and a running run is left working.
// The detached Go driver writes its stdout and stderr to the driver log in the ephemeral tree.
// It never writes to start's own output;
// a loom driver strand writes no such log, since its pane holds its output.
// A loom driver strand launches from the driver stencil, read from the stencils directory at start time.
//
// An operator session started with `lym reed add --cmd claude --name claude` is the strand named `claude`,
// and the run's panes are its siblings.
// To self-check, compare `$TMUX_PANE` against the tracked strands `lym reed status` reports: tracked is fine;
// set but untracked means relaunching through `lym reed add` or proceeding without reed supervision;
// unset is unconfirmed rather than failed, since psmux on Windows may not export it.
//
// # Resume
//
// `lym loom resume` takes the bootstrap lock start takes, so the two never run at once, and decides by the run's state:
//
//   - no status file: refused, naming `lym loom start`.
//   - done: a no-op success.
//   - awaiting at a review segment's Bouncer row with a circling decision recorded:
//     handled as a halted run, so a live, parked driver is woken and its step acts on the decision.
//   - awaiting at a Bouncer row with no decision: refused, naming the circling verbs and then resume.
//   - awaiting at a Bouncer row with an unreadable decision:
//     refused naming the file, to fix it or delete it and record the decision again.
//   - awaiting at any other row:
//     refused, naming the verb the run's waiting note names.
//     A batten run watching the child then resumes it, or `lym batten run <slug>` from the prime when none does.
//   - running:
//     a no-op success when the driver is live or holds the run lock or the driver lock;
//     otherwise refused as a halted run.
//   - halted with the run lock or the driver lock held, or with a live driver and no park marker:
//     refused with the kind `driver_not_parked`, to retry shortly;
//     a held driver lock means the Go driver is still settling the stop.
//   - halted with a live driver and a park marker:
//     resumed;
//     refused with `merge_in_progress` over an unfinished merge under start's Publish and Finalize exception;
//     refused with `driver_busy` when the driver's pane would not take the resume line.
//   - halted with no live driver and a seed recording the Go driver:
//     refused over an unfinished merge with `merge_in_progress`;
//     otherwise a Go driver is spawned, and the success envelope names its pid.
//     The driver re-steps the run.
//     At a blocked Finalize stop under a dirty parent code checkout it hands back and steps nothing instead.
//   - halted with no live driver otherwise:
//     refused over an unfinished merge with `merge_in_progress`;
//     otherwise refused naming `lym batten run <slug>` from the prime and then resume for a dead strand,
//     and `lym loom start` for a retiring strand or none.
//
// The driver strand is read from reed's directory, which still lists a session that died with the machine.
// It is never read from the run's seed;
// the seed is read only to tell a go-seeded run that has no driver, which gets a Go driver.
// `lym loom circling continue` refuses at or past the perch's hard cap.
// Its way forward is `circling accept` or raising the cap.
// A resume line that cannot be delivered is refused, naming `lym loom status` and a re-run of resume.
//
// # Plan card verbs
//
// These verbs are the sanctioned way to add or drop one card of the plan and renumber the cards after it:
// `lym loom insert-card <position> <slug> --intent <intent>`;
// `lym loom remove-card <position>`;
// `lym loom restore-renumber`.
// A renumber moves each later card's file name prefix, heading number, explicit `Commit:` prefix and Card Index number.
// It writes a heading-only stub for an inserted card, which the agent fills with Edit or Write.
// A card id naming a moved card is rewritten to the card's new number in the overview and every card file.
// The id is `<N>-<slug>` or its zero-padded file form `<NN>-<slug>`, and the slug pins which card it means.
// A bare card reference is read against the numbering before the call and rewritten the same way.
// It is `card <N>`, a list such as `cards <N>, <M> and <K>` or a range `cards <N>–<M>`.
// A bare reference naming the removed card has no faithful rewrite and is left as is.
// So is a range the inserted or removed card falls inside.
// The success envelope carries `plan_dir`, `moved`, `created` or `removed`, `rewritten_refs` and `prose_mentions`.
// `rewritten_refs` holds one `file`, `line`, `from` and `to` per rewritten card id or bare reference.
// `prose_mentions` holds the lines left holding a bare reference with no faithful rewrite.
// The agent fixes them by hand.
// `restore-renumber` reports `plan_dir`, `journal`, `restored` and `removed`.
//
// Each verb takes webster's state-mutation lease first and releases it before it prints,
// so no begin-batch records a batch between the bound check and the renumber.
// `insert-card` and `remove-card` refuse a position at or before the last card of any batch webster has begun.
// A batch counts whether it is terminal or not, since a begun card is frozen.
// They touch only the current generation's card files and the Card Index of the overview.
// They never touch an archive, a prior generation, the overview's frontmatter or any other section.
// They never commit, approve or set `approved:`.
//
// Every call writes a journal under the plan's never-tracked mirror before it changes the plan directory,
// and it removes the journal as its last step.
// A failure undoes the call; one whose undo fails keeps the journal, and `restore-renumber` completes the undo from it.
// The restore writes back whole pre-call files, the overview included.
// It removes every created name whatever it holds by then,
// so it discards any edit made to the journaled files since the interrupted call.
// A journal naming anything but the overview or a card file at the top level of the plan directory is refused.
// No file is written first.
//
// The plan card verbs read only the resolved anchor,
// so they join the lightweight wiring and are not held up by an unrelated module's config.
// `renumberRefusal` words every refusal with its way forward.
//
// # Format verb
//
// `lym loom fmt <file>...` rewrites each named `.go` file in place to its `go/format` form.
// It runs in-process with no `gofmt` binary,
// so an agent fixes the formatting `cmd/lym/gofmt_test.go` fails the way its edit directive names.
// It reads only the worktree root and the caller's cwd, so it joins the lightweight wiring,
// and its arguments are files, never a run-id.
// `planGoFormat` checks and formats every file before any write.
// A path that is missing or outside the worktree refuses the whole call and writes nothing.
// A symlink resolving outside the worktree counts as outside.
// So does a path that does not end in `.go`, cannot be read or does not parse.
// The success envelope's `changed` lists the files rewritten as named;
// a file already formatted is left untouched and unreported.
// A write that fails stops the call, naming `written` and `failed`.
//
// # Validate plan card checks
//
// Every `lym loom validate-plan` mode but `--require-approved` also runs these card checks of the plan gates.
// They run from the context `validatePlanCmd` builds beside the rename tools and refused raw commands.
// `test-tier-unstated` reads the tier keys of the hub's `gate.yaml`.
// `seam-test-unlisted` reads the `//lym:seam` markers of the wired worktree through `loomshed.SeamTestReader`.
// `doc-go-unlisted` reads the package `doc.go` files there.
// An absent `gate.yaml` skips the tier check, as `loomCLI.wire` does for the gates it builds.
//
// # Validate discussion
//
// `lym loom validate-discussion` on a tapestry run also runs the citation check of the loom discussion check.
// The check tests the decision record's cited paths against the worktree.
// It tests its cited symbols through the wired `PlanIndex`, so it reads that field beside the discussion paths.
// A darn run tests no citation.
//
// # Validate plan lens review
//
// `lym loom validate-plan --lens-review` runs the flag-less check set plus one more check.
// The check is that the plan directory's `lens-review.md` holds a recorded lens review.
// This is the check set of a twill Plan-Write run's own gate.
// It runs alone: combined with `--rework`, `--require-approved` or `--overlay` it is refused before the plan is parsed.
//
// `lym loom validate-plan --part <name>` runs the check a part writer's own gate runs over the named part's plan:
// `loomshed.ValidatePart`, with the plan context but no decision record.
// It runs alone: combined with any other mode flag it is refused before the parts file is read.
// It is refused on a recipe with no Parts-Plan-Write row, read from the recipe's rows.
//
// `lym loom validate-parts` runs `partsparser.Validate`, the check the parts gate runs.
// It checks the parts file against the most parts `parts_max` allows.
// It runs on the lightweight wiring, reads the decision record and the worktree, and changes nothing.
//
// `lym loom join-parts` is the chair's one call that joins the parts' plans into the plan directory:
// it re-checks every part with `loomshed.ValidatePart` and writes the joined plan through `loomshed.JoinParts`.
// It runs on the full wiring.
// It refuses unless the run's status is at the running Parts-Plan-Write row.
// It also refuses unless the caller's `LYM_STRAND_NAME` is the strand name the seat engine gives that row's chair.
// A halted or later run, a part writer, a Plan-Burler-Fix fixer and Webster cannot reach it.
// It overwrites only the plan directory,
// and a rerun within the row discards whatever the chair added after the last join.
//
// # Validate plan overlay
//
// `lym loom validate-plan --overlay` checks the plan with proposed plan file contents laid over it, in memory,
// so a fix can be checked before any file is edited.
// Stdin is one JSON object whose keys are plan file names and whose values are their full proposed content;
// the names are `00-overview.md` and the card files the Card Index on disk lists.
// The verb builds a `planparser.Overlay` over the plan directory and parses it.
// It runs the flag-less check set (`loomshed.ValidatePlan`) over the overlaid plan.
// It reports through the verb's usual envelope.
// Handle canonicalization and the done-card comparison read and rewrite through the overlay,
// and no byte under the plan directory is written.
// The overlay cannot change the tree the glyphs resolve against,
// so it validates target-list, verify-command and card-text changes, never a proposed code change,
// and it adds, removes or renames no plan file.
// It is exclusive with `--rework` and `--require-approved`, refused before stdin is read.
// Each of these is refused with its way forward in `overlayRefusal`:
// a terminal or empty stdin;
// input that is not one object of distinct names and string values;
// an unusable name;
// a card file missing from disk.
//
// # Arbiter
//
// `lym loom arbiter <run-id> --envelope <path> [--report <path>]` is run only by the run's driver.
// It runs at a stop the driver cannot settle.
// It spawns a cold arbiter agent on the `arbiter` model through shuttle.
// It waits for its verdict file and prints the validated verdict.
// The arbiter's model, timeout and spawn budget come from `shed.yaml` and its layers.
// The keys are `arbiter`, `arbiter_timeout_min` and `arbiter_max_spawns`.
// A loom recipe's own values come through its profile block's `shed` object.
// It is a role verb, so the orch's command index leaves it out.
// The verb's body is `internal/shedarbiter`'s:
// the call, the prompt, the launch spec, the verdict gate and the consumption of a resume are that package's,
// and this package fills its deps with loom's seams.
//
// A stop is eligible only if the verb's closed list admits it:
// an error stop of kind `producer` or `bootstrap`;
// an `interrupted` stop under the `reinvoke` policy or with a transient class;
// a halted `blocked` stop other than a Parent-Align or Publish verify halt whose own failure record names a target;
// a halted `awaiting` stop whose Bouncer row has a pending escalation of cause `hard-cap`, `circling` or `decision`.
// A `budget` escalation therefore goes to the run's parent.
// The rule lives in `internal/arbiterstop`, which batten's hold shares.
// Every other stop is refused with the hand-back to the run's parent as the way forward.
// So are a missing status file, an unreadable envelope, `arbiter_max_spawns` of 0 and a spent budget.
//
// Each spawn is an attempt, numbered from 1 under the run's arbiter directory in `attempt-<N>`.
// `attempt.json` is written before the spawn,
// so the attempt counts against `arbiter_max_spawns` whatever happens next.
// The budget is counted from those records, so a restart never resets it.
// The directory also holds `prompt.md`, the agent's `verdict.yaml` and the verb's validated `envelope.json`.
// Once the re-launch consumed a `resume`, it also holds the `applied` mark.
// While an arbiter may be live an ephemeral marker names the attempt, its strand and its deadline.
// The status commit stages the arbiter directory with the run records.
//
// The arbiter's run, started or attached, carries a shuttle gate that reads `verdict.yaml` as the verb does.
// The gate re-prompts the arbiter with the reader's error while the file does not read.
// The verbs the file records have already run by then.
// An absent file passes the gate.
// Once its re-prompt budget is spent the gate lets the run through,
// so a verdict that still does not read hands on with cause `unparseable`.
// The arbiter stencil asks for every free-text value as a literal block scalar.
// A `: ` or ` #` in a command or sentence then keeps its meaning.
//
// The prompt carries `amendment_target`:
// the run's recipe's `amendment_target` key once the current generation's plan is approved, and `none` otherwise.
// Loom's value is `Plan-Burler-Review`; darn and batten set none.
// It is read at each spawn.
// The driver holds its arbiter seams for the whole drive.
// The plan is approved mid-drive, so a drive that began before approval still routes.
// The arbiter may then record a plan amendment and name that target as its goto.
// Each `--amends-plan` call re-admits one goto onto the target.
// The arbiter's repeats are capped by `arbiter_max_spawns`, counted from its attempt records.
// The arbiter's entry joins the answer key Plan-Review judges against.
// The stencil's hand-on rule therefore bounds scope widening.
//
// The stop key is the status file's current producer, history length and state.
// A re-run for a stop key an attempt already arbitrated spawns nothing:
// it returns that attempt's verdict or waits on its live arbiter for the time left to its deadline.
// A re-run stops an arbiter past its deadline through shuttle's stop verb and reads `hand-on` with cause `timeout`.
// An applied `resume` whose stop key is unchanged is refused, since the run stopped at the same place again.
//
// The printed envelope carries `verdict`, `reason`, `question`, `goto` and `verbs`.
// It also carries `cause`, `attempt`, `attempt_dir` and `verdict_path`.
// For a `resume` it also carries the `relaunch` line.
// A `resume` the verb finds unsound reads `hand-on` with cause `precondition`:
// no circling decision pending at an escalation;
// no goto target at a `blocked` stop;
// a goto target the run's current row does not admit;
// or a goto target the told goto guard refuses.
// The verb moves no run itself.
// The verdict's goto is performed by `lym shed step <run-id> --until-stop --arbiter-attempt <N>`;
// it runs under the run and status locks, once per attempt;
// a run is admitted with no second goto when it is already paused at the verdict's goto target:
// only goto entries lie since the spawn, and the last names that target.
// A guard refusal inside that re-launch refuses the attempt, and the driver hands back with the refusal text.
//
// The told goto guard runs its arms in this order:
// the Finalize arm refuses a goto onto the Finalize row while the parent's code checkout holds tracked dirt,
// and it names the checkout, the paths and the goto and resume that retry it;
// the settled-segment arm refuses a goto onto a review or fix row whose verdict is settled.
// The Finalize arm admits the row when the parent pair cannot be read or holds a fabric merge record.
// Finalize's own merge guard then words the problem.
// The guard is wrapped to end every fixer role registered under the pair.
// It does so once both arms admit a goto or a consume's target, before the move is written.
//
// A pair's session end ends a live arbiter strand with the pair's other strands.
//
// The Go driver reaches the arbiter through its arbiter seam, which calls this same path in-process.
// Eligibility, `arbiter_max_spawns`, the attempts, the applied mark and the deadlines are therefore shared;
// it passes the tackler's case brief, which the arbiter's prompt asks it to check against the facts.
//
// # Go driver
//
// `lym loom drive` is the hidden role verb that drives a go-seeded run.
// `lym loom start`, `lym loom resume` for a halted go run, and a hand-over spawn it.
// It mounts `internal/sheddriver` with loom's seams, and it serves darn runs as well as tapestry runs;
// the loop, rule table, hand-back and tackler below are that package's.
// It takes the run's driver lock, waiting a short grace for a momentary probe to let go.
// It holds the lock for its whole life.
// A settling driver therefore reads as live while the run lock is free between steps,
// and a second driver refuses once the grace runs out.
// On start it removes the park marker and the resume report.
//
// The driver registers itself in the owner record under the run's slug with the role `driver`.
// It registers before it takes the driver lock.
// The run's step loop and the keeper that starts it register themselves with the role `loop`.
// They do so through the loop's `OnStart` hook:
// the loop once its pid file is written, and the keeper once the loop process has started.
// A driver the lock refuses exits at once and its record is pruned at the next read.
//
// A step at a row waiting on the operator (Beam-Await, PR-Review, Sampler-Session) reads as active to the step watchdog.
// It also reads as a live wait to dobby, so neither dobby nor the watchdog kills it or sends a quiet notice.
//
// The loop steps the run through the shared `--until-stop` body.
// It saves the envelope of each stop it decides under the run's drive-reports directory, a yielded step saving none.
// It decides the stop by a closed rule table over the envelope's fields and the facts it reads from disk.
// The table never reads a row name or the reason text.
// The table settles a done run and a busy exit.
// It settles a re-step of an interrupted step under the `reinvoke` policy, up to a cap of two per stop key.
// It settles the one verify goto to the Publish failure record's target;
// it asks the tackler at a stop no rule settles;
// it asks the arbiter at an awaiting stop with a pending hard-cap, circling or decision escalation only;
// it asks the arbiter after a tackler `escalate`;
// it asks the arbiter when an applied tackler `resume` returns to the same stop;
// and it hands over at a binary-eligible stop whose last step ran another build than the `lym` on disk.
// Everything else hands back.
// Every action appends a settle record to the friction directory.
// The repair cap, the one verify retry and the one hand-over per stop key and build are counted from disk.
// A restarted driver therefore keeps every cap.
//
// Before it acts on a stop the driver re-reads the status.
// A running status or a run lock another process holds ends the driver with a `running-exit` record;
// a status at another stop key is a moved run;
// an orch goto sends it to a re-step, and anything else decides it afresh;
// a moved run is never parked and gets no friction note.
// While a pause covers the slug the driver holds in the pause wait before any re-step, goto, agent spawn or hand-over.
//
// `lym loom pr-continue` is the PR-Review agent's verb for the operator's ending of a review:
// it sets the continue flag in the run's pull-request record and nothing else,
// and it is refused until the PR-Review row has opened the pull request.
//
// `lym loom sampler-done` ends a sampler run's build:
// it sets the done flag in the Sampler-Session record to the row's current entry stamp and nothing else.
// It is refused on a recipe with no SamplerSession row.
// It is also refused while the run stands at another row.
// It is refused when the record names no session or the task worktree has uncommitted paths.
// Each refusal names its way forward.
// It is not limited to the sampler agent's strand, so the operator's own shell may run it.
// It is accepted while the run is halted at the row, so the next `lym loom resume` ends the row Done at once.
//
// A hand-back leaves a Go-authored stop report.
// It commits the run records and writes the park marker through the code `lym loom commit-records --park` runs.
// It queues the parent messages the LLM driver's notify rules send.
// A watching batten announces the run and retries a matching verify halt itself.
// Only the parent notice, a question and a friction-failed done are therefore sent;
// an unwatched run also gets one line naming the run-id and the report.
// A paused stop that carries no reason also writes a `go-driver-paused` friction note.
// A done run commits its records.
// One that finished with `friction: failed` also gets a report and a notice without being parked.
// `lym loom commit-records` pushes the pair's records with its task branch;
// a push skipped while the slug's teardown lock is held exclusive is an ok envelope with `push_skipped`.
// The commit stays local for the pair's next push.
//
// Every commit and push of a loom run's pair passes the pre-push check first:
// the per-transition status commit and push, `commit-records` and PR-Review's push of the task branch.
// The check runs the run-scope origin reconcile from the prime, the one the landing rows run.
// The reconcile fetches what origin moved and fast-forwards or replays the run's own branches where it can.
// It is skipped when pushes are off, in the prime, while a pause covers the run, and while the run stands at Finalize.
// A pause's network gate holds the push, and Finalize judges origin and its owners itself.
// An unreachable origin, a failed check and a failed read of whether the parent holds the run's work let the push go,
// so the push's own failure is recorded.
// A run another machine owns is paused under the owner, naming `lym adopt <subject>`;
// a run a branch stops is paused naming each of its branches and its parent branch that stopped;
// that pause also names the way forward and `lym tender resume <slug>`;
// and a run origin ended is paused naming `lym cleanup --apply`.
// The exception is a run whose parent already holds its work.
// A raddle run standing at Raddle-File whose entry origin shows done is the other exception.
// Such a raddle run is not read as ended:
// its own Raddle-File row ended the entry, and Land and Worktree-Teardown are still to come.
// Each stop posts one notice line to the nearest live orch.
// An active run whose own branches the reconcile moved posts one line naming them.
// One whose parent branch origin moved past this machine's tip posts another.
// A stopped status transition commits and pushes nothing.
// `commit-records` answers an ok envelope with `push_stopped` and pushes nothing after a stopped commit.
// PR-Review halts on the stop as on any push refusal.
// Bound: the check reads origin once per commit and push,
// so a move that lands between the check and the push is caught only by the push's own refusal.
//
// A hand-over runs only at a binary-eligible stop, once per stop key and build.
// It runs over a clean tree outside the run's records, and never under a pause.
// It records the hand-over and leaves the stop report and the park marker.
// It releases the driver lock and spawns a fresh `lym loom drive` of the on-disk build, which re-steps;
// a driver starting at a blocked Finalize stop first runs the Finalize arm,
// and it hands back, stepping nothing, while the parent's code checkout is dirty.
//
// A driver restarted over a halted run finds an agent attempt that no later hand-back closed.
// The agent's own call reuses that attempt, so no second agent is spawned.
//
// # Tackler
//
// The tackler is the cold agent the Go driver asks at a stop no rule settles.
// Each call is an attempt under the run's tackler directory in `attempt-<N>`.
// `attempt.json` is written before the spawn, so the attempt counts against `tackler_max_spawns` whatever happens next.
// A `tackler_max_spawns` of 0 turns the tackler off.
// The record holds the stop key, the pause conditions, a snapshot of the task worktree and the spawn time.
// It also holds the deadline and the envelope and report paths.
// The directory also holds `prompt.md`, the agent's `verdict.yaml` and an escalate's `case-brief.md`.
// It holds the call's validated `envelope.json` too.
// Once a re-launch consumed a `resume`, it also holds the `applied` mark.
// While a tackler may be live an ephemeral marker names the attempt, its strand and its deadline.
//
// The tackler's verdict is `resume`, `hand-back` or `escalate`.
// A `resume` names a goto only to the run's current row or to the row the halting entry was routed from,
// and a blocked stop needs one;
// a `hand-back` needs a question for the run's parent, and an `escalate` needs a non-empty case brief.
// A verdict that is missing, unparseable, unsound or late reads as a hand-back.
// Its cause is `missing`, `unparseable`, `precondition`, `timeout`, `died`, `api-error`, `question` or `case-brief`.
// A deadline that ran out while the hub's pause covered the run moves out by the paused time instead.
//
// A re-run for a stop key an attempt already judged spawns nothing:
// it returns that attempt's verdict or waits on its live tackler, unless a hand-back record for the key closed it.
// An applied `resume` whose stop key is unchanged goes to the arbiter.
// The driver consumes a `resume` through the shed's re-launch.
// The re-launch refuses under the status lock unless one of these holds:
// the run is still where the tackler found it;
// or the run already stands paused at the verdict's goto target after only goto entries.
// It marks the attempt applied in the same hold.
// A guard refusal inside that re-launch refuses the attempt, and the driver hands back with the refusal text.
//
// Tampering is the spawn snapshot of the worktree differing from a fresh one.
// It is read only after the stop key proved unchanged:
// a tackler that changed the code worktree hands back with cause `tampered`,
// and a run an orch moved meanwhile discards the verdict and acts on the fresh state instead.
// A pair's session end ends a live tackler strand with the pair's other strands.
// The tackler is an interactive strand in the pair's reed session.
//
// # Peers
//
// `lym loom peers` lists the other runs in flight in the hub.
// `lym loom peers show <slug> --record decision|plan` prints one peer's decision record or its plan's `00-overview.md`.
// The printed text sits under a `content` key.
// An agent in one pair uses them to compare its run with the others without forming another worktree's path itself.
//
// The peer set is every board task whose status is a run status, whatever the run's state.
// The caller's own slug is left out.
// Each peer line carries `slug`, `title`, `status`, `row`, `parent_branch`, `changed_paths` and `unreadable`.
// It also carries `decision_record` and `plan`, which report whether the peer holds them.
// `row` is the 1-based position of the status's producer in the peer's own recipe, 0 when it names no row.
// `changed_paths` are the committed paths between the peer's merge base with its parent branch and its head.
// A peer's own recipe is the one its worktree's self-run seed records when the worktree is on this machine.
// Otherwise it is the one the board entry seeds:
// pawl for a project entry, else its `recipe` field or, when that is empty, the board's `default_recipe`;
// a recipe that names no embedded recipe gives row 0.
// A peer whose pair, origin record, branch or records cannot be read is listed with that reason in `unreadable`.
// It is never dropped and never an error.
// From a task pair the list also carries `own_row` and `parent_commits`.
// `own_row` is the caller's own position by the same rule, 0 when the caller has no board task.
// `parent_commits` are the commits on the pair's parent branch since the pair forked.
// Each is a short SHA and subject, newest first.
// The prime has none.
// Both verbs refuse in a hub with no board.
// `show` also refuses an unknown slug, a task holding no run status or the caller's own slug.
// It refuses an unknown record name and a record the peer lacks.
// Each refusal names its way forward.
//
// The verbs write nothing.
// They read the board, the peers' decision and plan records and self-run seed.
// They read git history through `internal/fabricengine` and nothing else.
// They resolve the location through `lymcwd` in the group's own pre-run, as the `review` and `decision` groups do;
// every loom verb is hub-only, so `internal/cliwire` does not apply.
// A bare `lym loom peers` outside a lym worktree lists the group's verbs instead of resolving.
//
// # Decision add --from
//
// `lym loom decision add --from <file>` reads a JSON array of objects.
// Each object has the keys `title`, `decision` and `rationale`.
// It may also have `pattern_entry`, `pattern_action` and `pattern_text`.
// `--by` and `--amends-plan` apply to every entry, and all entries land in one write and one commit, or none does.
// Each live agent gets one notice naming the headings (`headings`).
// A run that is not running wakes no Webster Master session left live from its last run:
// the envelope names each such strand and why under `notice_held_back`.
// Two entries may not share a title or a PATTERN entry.
// A `pattern_action` of `withdraws`, like `--pattern-action withdraws`, strikes a declared `## PATTERN changes` bullet.
// The strike lands in the entry's commit, and the envelope names the ID under `pattern_withdrawn`.
//
// # Decision amends-plan
//
// `lym loom decision add --amends-plan` records a decision that amends the approved plan:
// besides the entry, the call appends the title of each entry to the tracked plan amendment record.
// A `--from` batch appends every title in input order.
// One commit lands the entries and the amendment record.
// The commit pathspec of the discussion artifacts names the record when it exists or git tracks it.
// Otherwise it names nothing, so git is never handed a pathspec matching nothing.
// The flag is refused on a recipe with no Plan-Write row, which has no plan to amend,
// and it is refused while the current generation has no approved plan;
// a plan that does not parse is refused naming `lym loom validate-plan`,
// and a plan or record read failure is refused with the verb's retry way forward.
// Both refusals tell the caller to record the decision without the flag.
// Plan-Write and Plan-Review read the decision record.
// After a successful call the parent runs `lym loom goto --to Plan-Burler-Review` and `lym loom resume`.
//
// A failed append puts both records back to their bytes before the call.
// It removes the amendment record when the call created it, and a decision record that was absent stays absent.
// A status commit may have already landed the call's amendment bytes.
// The verb then commits the restored record alone, so HEAD never keeps a title whose entry did not land;
// a failure of that commit is refused naming `lym fabric commit`.
// A failed commit of the entry reads HEAD:
// both records at HEAD succeeds with `commit_error` on the envelope;
// the amendment record without the entry, or a HEAD that cannot be read, keeps both files and names `lym fabric commit`;
// a HEAD that differs restores both files and names the re-run of the verb, never `lym fabric commit`.
//
// The landing wiring reads the run's start from the pair's seed and fails on a seed it cannot decode.
// `landingDeps` tells landingshed the follow-up of a removal, through `pairteardown.FollowUpFor`.
// It runs over the prime's batten record of the run's slug and the owner pause.
// A stopped reason that sends the operator to `lym fabric remove` thus names the step that settles that record.
//
// # Review subjects
//
// The wiring tells each Bouncer row what its segment judges.
// A re-entry over an unchanged subject settles without a review,
// and a changed one reviews only the change under `review_amend_max_bounces`.
// A recipe row names its subject by its `subject` key:
// Discussion-Bouncer and Plan-Bouncer `document`, Code-Bouncer `code`, in the tapestry and darn recipes alike.
// A document subject hashes the row's artifact paths in order:
// the decision record through its identity text;
// a plan directory as its overview's identity text and its top-level card files;
// and an absent path as absent.
// Its Base is the commit the run records' checkout stands at.
// A code subject is the task worktree's change against the task's merge base.
// It leaves out the run's own state directories and the harness's own files while they are untracked.
// A run with no recorded parent branch tells no code subject, so its code segments write no records.
// A change confined to the harness's own files reads as an unchanged subject;
// the landing gate at Parent-Align and Finalize bounds it.
//
// The task's merge base is the one commit every reader of the task's diff starts from:
// the code subject, the Code-Review rubric's range, Describe's diff, and the verify gate's lints and tree scope.
// It is the merge base of HEAD with the parent tip the merge record names.
// The local parent branch can lag a tip the task merged.
// It falls back to the merge base with the local parent branch when there is no merge record.
// A merge record that cannot be decoded is logged at Warn and treated the same way.
//
// The Code-Review rubric's `redundancy_report` marker is filled from the worktree's redundancy report.
// A `lym gate test` call with the `redundancy` report step writes that report.
// The marker lists the report's tests when `gatecli.RedundancyReportMatches` matches the report against HEAD.
// Otherwise it is one line saying no current report exists and why.
// That line names the gate key of a report written on a red tier.
//
// Every loom recipe builds its Describe session from one `landingshed.DescribeTable`.
// It lists the run records that exist.
// It renders `(none)` for a run that has none, as a darn run has no webster summary.
// One `commitDescription` commits the landing directory for the Describe row and for the Darn-Write row,
// so every loom recipe has one description commit.
//
// The Drift rows' records guard rests on the seams `driftDeps` wires over the worktree's location.
// `CommitWithTrailers` commits anchored paths through `fabricengine.CommitAnchoredPathsWithTrailers`.
// Its message is `loom: drift records for <slug>`.
// `TrailerCommit` finds the oldest or newest records commit carrying a trailer key.
// It goes through `fabricengine.FindTrailerCommit`.
// `FileAt` reads an anchored file at a records commit through `fabricengine.AnchoredFileAt`.
//
// # The recorded driver's one reading site
//
// A recorded seed driver value is read only in a recipe's bootstrap verbs:
// loom's start and resume, and `lym loom drive`'s own package.
// Each branch on it selects the run's driving surface and nothing else:
// which driver spawns and whether the session carries the loom status strand.
// No producer, generic verb or engine reads the recorded value, and no code path gates behaviour on it.
// A second reader would let what a run is doing diverge silently from what it was seeded as.
//
// One carve-out, and only this shape:
// a CLI verb may compare a driver value the operator typed against the addressed run's recorded one.
// It may refuse on the envelope when they disagree, as `battencli`'s `refuseAdoptedSeed` does.
// A refusal decided by the recorded value alone, with nothing typed to compare it against, stays barred.
// The rule governs the read, not the vocabulary:
// the driver constants are named at the seeding sites, which validate a flag before a seed exists.
// Loom's seed writer reads `shed.yaml`'s `driver_kind` and its layers only at seeding.
// A loom recipe's own value comes through its profile block's `shed` object.
// `internal/sheddriver` reads no seed driver field.
//
// `bootstrap_test.go` is a tripwire, not a completeness proof:
// it fails when a production file outside `internal/shedrun` reads the seed's driver field.
// The exceptions are this package and the functions in its `driverFieldReadCarveOuts`.
// The carve-outs are keyed by file and enclosing function, each with its justification beside it.
package loomcli
