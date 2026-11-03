// Package loomcli is the `lym loom` command: it bootstraps and drives one task's phase machine per worktree on the generic shed engine, and carries the verbs through which the operator and the run's parent answer its gates.
//
// The run, step, status, pause and goto bodies are the shared `internal/shedverbs` verbs, mounted here with loom's own text.
// Friction-Reflect, the last row, runs the Tier 2 friction reflection under run and under step alike, before the run records done.
// A `blocked` or `failed` halt, and an interrupted stop, write their Go halt note and spawn no reflection, so no stop holds the driver lock for a friction session and `lym loom resume` never waits on one; the note waits for the run's Friction-Reflect row, which reflects every note in the friction directory.
// An interrupted stop's note carries the `interrupted-stop` anomaly kind, never `producer-hard-failure`, with the stop's cause as its reason and the in-flight step's trace file, or its captured stderr, where the stop knows it.
// It records the state the status file holds after the stop, which stays `paused` for a step that died before its resume write over a paused run,
// and an error the status file holds that the stop did not write, such as the step's own producer failure, on a `status error:` line of its reason.
//
// A loom run's shed and routing come from one recipe-keyed builder in `internal/loomrecipe`, `loomrecipe.New` and `loomrecipe.Routing`, taking the run's recipe name, pawl's included.
// The row names a run parked under an old recipe may still carry are that recipe's retired-row data, read through `loomrecipe.RetiredRows`: arming renames them in the status file with no branch on the recipe name, and a recipe with none renames nothing.
//
// The wiring branches on the recipe's rows and keys, never its name, through the `loomrecipe` accessors on the run's recipe, an empty recipe reading as tapestry.
// The verify source follows the recipe's `verify_source` key, the tree-scope verify seams follow `verify_source: tree_scope`, the Darn-Write seams are filled for a recipe with a Darn-Write row, the perch-off reading follows the Code-Review rows' entry gate as well as the budget, and the merge fixer's plan directory and failure tracer follow a Plan-Write row.
// A recipe with a SamplerSession row loads the hub's `sampler.yaml` strictly in `wire`, which refuses a malformed file before any row builds, and fills the row's seams from `samplerDeps`.
// The row's entry stamp counts the times the run entered it, from the status file and the rows that route into it.
// `wireLanding`, which both the `run` and `step` building paths pass through and `status` and `pause` never do, fills the session's start and stop, its liveness and its process registration as for PR-Review's session, and the watch on the row's record that wakes the wait.
// A run's decision record follows its recipe's rows through `runRecordOf`: a recipe with a Beam-Await row keeps the beam's project record under the beam's slug, and every other recipe the discussion record.
// A task run's review rows are told its record as the PATTERN declaration path, a beam's project record declares none, and a loom recipe holds a landing's escalate halt for the arbiter first.
// Discussion-Bouncer's approve seam stores the approved decision record on the run's board entry through the one `storeDecisionRecord` helper, logging and passing a done entry and failing the settle on any other store failure.
// The Motif block's undiscussed gate seam does the reverse for an entry flagged discussed: it writes the entry's stored record as the run's decision record, writes a support log saying so when none exists and commits the discussion directory, so the gate passes over Discussion.
// `decision add` copies the committed record onto the board entry after its commit once the run is past that approval, reporting a failed copy as `record_store_error` and never failing the verb, and `decision store` makes the same copy by hand, admitting the unnamed shell, the run's arbiter and its parent orch.
// Each status transition writes `<state> · <producer>` on the run's board entry.
// A finished run of a recipe with a Finalize row writes nothing, its landing having marked the entry done.
// A finished run of a recipe with no Finalize row, a motif run, writes the draft status, so the entry leaves its run lock and waits for the operator with its record stored.
//
// `wire` fills `Env.GateFixer` for every recipe through `gateFixerSeam`, which wraps the gate of a Darn-Write, Plan-Write or Discussion-Write row whose `fixer` element names a model-spec, so a failure at the writer's idle boundary goes to a cold fixer before the writer hears of it.
// The seam returns the spec unchanged on the prime and outside a hub, where no pair owns a fixer.
// A fixer's records live in the run's scratch directory under `gatefix`, in a directory `rowVisitDir` keys by the row and the status history's length and newest repeat count, so a crash re-step finds the fixer it left and a goto or a fresh entry starts clean.
// `fixerSpec` renders the prompt from the shared cold frame and one body stencil per gate entry, `gatefix-body-darn`, `gatefix-body-plan` or `gatefix-body-discussion`.
//
// The `lym loom` verbs serve loom runs only.
// `arm`, the subtree's entry, sets `loomPath` before `armAt` and `armAt` then refuses, right after the recipe is resolved and before any wiring, a recipe that does not declare the loom contract: an embedded non-loom recipe is refused naming the `lym shed` verbs, and a seed naming no embedded recipe naming the re-seed.
// It then refuses `validate-plan`, `insert-card`, `remove-card` and `restore-renumber` on a recipe with no Plan-Write row, which has no plan, `validate-parts` on a recipe with no Parts-Write row, which writes no parts file, and `join-parts` on a recipe with no Parts-Plan-Write row, which has no parts to join.
// The target resolver the `circling` and `decision` groups share judges the target's own seed recipe the same way.
// `ArmAt`, which `lym shed` shares, leaves `loomPath` false and never refuses a recipe, and `lym loom checklist` is exempt, being a read-only pane the pre-run never arms.
//
// A run seeded with the recipe `pawl` is a beam's life, batten's Seed-Child seeds it in the beam's pair.
// Its shed reads Preflight, Record-Write, Beam-Up, Beam-Await, Parent-Align, Beam-Describe, PR-Review, Finalize and Beam-Down, with no plan, no review round and no reroute edge.
// Its Record-Write writes the project record and its support log from the project entry's body and commits them under the beam's slug.
// wire fills `Env.Pawl` for it through `pawlDeps`: the run's seed id is the seed's start time, the status is written on the beam's project entry through the board, and Beam-Up's one step is `orchcli.BeamUp` with no prepare, since the run already judged origin; the warnings its report carries are logged and posted to the beam's orch as one `PAWL` notice, and none halts the row.
// The beam's dobby is stopped through `runproc.EndRole` by its `RoleDobby` record, and restarted through `orchcli.BeamUpWorktree` naming the beam, which brings the beam's orch up with it.
// Beam-Await's re-check reads `battencli.ReadBeamLandBlockers` and `battencli.BeamLandRefusal` with the beam's owner, its parent's owner and the beam's ready record, and words each failure as `pawlRecheckLines` does, naming `lym adopt <beam>` or the parent's adopt subject, `root` for the prime, and `lym beam ready <slug>` for a beam not flagged ready.
// A running shed also watches the land request's directory, so a request wakes Beam-Await at once.
// Beam-Describe writes the change description from the beam's board entry, the commits the beam adds over the parent branch its pair records and the project record's added entries.
// Its Finalize reads the hub's `finalize` gate key over the whole merged tree and halts for the orch on a fixer that is spent or over its caps.
// A land check backstops the re-check: Finalize halts, behind its own prefix, while an open member or a pair forked from the beam still works, reading `pawlLandBlockers` and naming the beam's parent worktree as where a cancel runs.
//
// A member's Discussion-Write lead and darn writer get the project record directive, `loomengine.ProjectRecordDirective` over `projectRecordPaths`: the record of each beam above the pair, read in that beam's own worktree at each spawn, nearest first.
// `decision add <beam-slug>` resolves the beam's project record, takes the beam's own orch as the caller its rule admits, and posts its notice to the live runs of the beam and of every member pair `memberPairs` reads under it.
//
// The caller rule of `decision add` admits a named session that is the run's parent orch, the run's own arbiter strand with `--by arbiter`, or the strand a sampler run's Sampler-Session row has recorded as its session.
// `samplerStrandReader` admits that strand by the recipe's rows, only while the run's current row is the session row and whether or not the run is halted there, and only with `--by operator`; any other label from it is refused naming `--by operator`.
// A strand it does not admit meets the parent rule.
//
// # Start
//
// `lym loom start` works in order.
// It resolves the recorded parent branch, seeds the status file when absent and commits the seed into the fabric before anything else touches it.
// Every start or step commits whatever the friction directory holds along with the seed.
// A first seed clears the friction directory first, so the clear is committed with the seed and notes the fabric carried in, such as a beam pawl run's halt note, neither reach the task's reflection nor leave Preflight a dirty worktree.
// An unseeded run is seeded with the driver `shed.yaml`'s `driver_kind` and its layers resolve to, `llm` by default;
// a seeded run keeps its recorded driver whatever the key says later.
// It brings up the worktree's tmux session;
// it ensures the status strand on either driver, and the per-hub watchdog daemon is spawned best-effort.
// The status strand runs the hidden `lym loom checklist` verb in an overview window named for the run's recipe, placed right after the runs strand's overview window in a beam's session, drawing the checklist frame: the recipe's rows in order, each entry numbered after its box and a review segment taking one number, each with its box, a row the run passed on a false entry gate and never visited boxed `[-]` apart from one not reached, the time spent in it, its visit count and its visit lines under it, each carrying no output path, a gate line per gate entry under a visit that recorded gate spans, opening with the UTC clock time its gate entry started, the visit after a gate row's refusal marked as rework for it and its time added to the gate row's line, a review segment's rounds each carrying their total, every closed time drawn to the second and every time that includes the open visit in whole minutes, then the run's state line, with the note of what a running run waits on (the same note `lym loom status` reports), and the wait line.
// A Stuck visit whose status entry records where Shed routed it reads `bounced` and an awaiting one reads `escalated`, in this window only.
// A block row's Done visit reads `entered`, and a row inlined from a block is named `<row> in <block path>` on its line and its visit lines.
// The verb reads the status file and the seed without arming a run, and reports no content size to reed.
// Right after ensuring the status strand, every start entry, a fresh run, a re-entrant start on a live run and a resume of a parked run, turns the session's follow state on and selects the run's relevant window;
// a failure there is logged and never fails the start, and `lym loom resume`, which never brings reed up, leaves the follow state as it is.
// Unless a driver is already alive, it spawns the driver the seed records, which is never chosen by a flag;
// a second invocation while a driver runs ensures the substrate instead.
//
// A live loom driver parked at a hand-back is resumed by typing one line into its pane, and start refuses after a bounded wait when that pane is not ready, since the driver may be busy having resumed on its own.
// A pane that holds a draft, an active operator or another busy reading is refused with the retryable kind `driver_busy`, the park marker kept, and the message names the draft and the way forward;
// a resume line that never appeared stays a non-retryable refusal.
// The stop-report path in the resume line is picked once per park and kept in a resume-report file beside the park marker, keyed to the marker's content, so every rerun against one park types the identical line;
// a landed resume or a fresh spawn removes the file.
// A live loom driver over a run halted at a hand-back (awaiting, blocked, paused or failed) that has not yet written its park marker is still writing its stop report, so start refuses with the kind `driver_not_parked`;
// a live driver over a running run is left working.
// Before anything else, start and resume refuse a run whose status file exists and that another machine owns, with the kind `owned_elsewhere` and the owner's name, naming `lym adopt <subject>`, the beam or `root` the owner verdict carries;
// the owner is judged by a judge-only run-scope origin reconcile from the hub's prime, or from the local board when origin is unreachable, and nothing is written.
// Before spawning or resuming a driver, start refuses with the kind `merge_in_progress` over an unfinished merge, naming the conflicted paths and the remedy, the fabric verbs for a fabric merge and git for one fabric did not start.
// The exception is a run whose current producer is Parent-Align or Finalize, which goes through over a parked fabric merge-in of its own parent branch, since that row aborts and redoes it.
// A loom driver strand reed marked retiring is never adopted: start removes it and spawns a fresh driver.
//
// start returns once the driver's readiness signal confirms it is up, and for a parked loom driver once delivery of the resume line is verified.
// For the Go driver, `lym loom drive`, the signal is the driver lock being taken, since the verb takes it first and holds it for life while the run lock is free between steps;
// for a loom driver it is the provider TUI coming up ready within shuttle's `startup_timeout_s`, with any one-time startup gate its provider requires dismissed along the way.
// A readiness refusal removes the driver strand, so the next start spawns a fresh one.
// The signal is checked only for a driver this invocation spawns, so a strand left live because shuttle got no liveness answer from reed, or could not remove the strand, is returned over by a later start without re-checking.
//
// A held driver lock with no live strand reads as a live Go driver: a run halted at a hand-back is refused with the kind `driver_not_parked` while its Go driver is still settling, and a running run is left working.
// The detached Go driver writes its stdout and stderr to the driver log in the ephemeral tree, never to start's own output;
// a loom driver strand writes no such log, since its pane holds its output.
// A loom driver strand launches from the driver stencil, read from the stencils directory at start time.
//
// An operator session started with `lym reed add --cmd claude --name claude` is the strand named `claude`, and the run's panes are its siblings.
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
//   - awaiting at a review segment's Bouncer row with a circling decision recorded: handled as a halted run, so a live, parked driver is woken and its step acts on the decision.
//   - awaiting at a Bouncer row with no decision: refused, naming the circling verbs and then resume.
//   - awaiting at a Bouncer row with an unreadable decision: refused naming the file, to fix it or delete it and record the decision again.
//   - awaiting at any other row: refused, naming the verb the run's waiting note names, after which a batten run watching the child resumes it, or `lym batten run <slug>` from the prime when none does.
//   - running: a no-op success when the driver is live or holds the run lock or the driver lock, and refused as a halted run otherwise.
//   - halted with the run lock or the driver lock held, or with a live driver and no park marker: refused with the kind `driver_not_parked`, to retry shortly; a held driver lock means the Go driver is still settling the stop.
//   - halted with a live driver and a park marker: resumed, refused with `merge_in_progress` over an unfinished merge under start's Publish and Finalize exception, and with `driver_busy` when the driver's pane would not take the resume line.
//   - halted with no live driver and a seed recording the Go driver: refused over an unfinished merge with `merge_in_progress`, otherwise a Go driver is spawned, which re-steps the run unless it starts at a blocked Finalize stop under a dirty parent code checkout, where it hands back and steps nothing, and the success envelope names its pid.
//   - halted with no live driver otherwise: refused over an unfinished merge with `merge_in_progress`, otherwise naming `lym batten run <slug>` from the prime and then resume for a dead strand, and `lym loom start` for a retiring strand or none.
//
// The driver strand is read from reed's directory, which still lists a session that died with the machine, never from the run's seed;
// the seed is read only to tell a go-seeded run that has no driver, which gets a Go driver.
// `lym loom circling continue` refuses at or past the perch's hard cap, with `circling accept` or raising the cap as its way forward.
// A resume line that cannot be delivered is refused, naming `lym loom status` and a re-run of resume.
//
// # Plan card verbs
//
// `lym loom insert-card <position> <slug> --intent <intent>`, `lym loom remove-card <position>` and `lym loom restore-renumber` are the sanctioned way to add or drop one card of the plan and renumber the cards after it.
// A renumber moves each later card's file name prefix, heading number, explicit `Commit:` prefix and Card Index number, and writes a heading-only stub for an inserted card, which the agent fills with Edit or Write.
// A card id naming a moved card, `<N>-<slug>` or its zero-padded file form `<NN>-<slug>`, is rewritten to the card's new number in the overview and every card file, since the slug pins which card it means.
// A bare card reference, `card <N>`, a list such as `cards <N>, <M> and <K>` or a range `cards <N>–<M>`, is read against the numbering before the call and rewritten the same way.
// A bare reference naming the removed card, or a range the inserted or removed card falls inside, has no faithful rewrite and is left as is.
// The success envelope carries `plan_dir`, `moved`, `created` or `removed`, `rewritten_refs` and `prose_mentions`.
// `rewritten_refs` holds one `file`, `line`, `from` and `to` per rewritten card id or bare reference.
// `prose_mentions` holds the lines left holding a bare reference with no faithful rewrite, which the agent fixes by hand.
// `restore-renumber` reports `plan_dir`, `journal`, `restored` and `removed`.
//
// Each verb takes webster's state-mutation lease first and releases it before it prints, so no begin-batch records a batch between the bound check and the renumber.
// `insert-card` and `remove-card` refuse a position at or before the last card of any batch webster has begun, terminal or not, since a begun card is frozen.
// They touch only the current generation's card files and the Card Index of the overview, never an archive, a prior generation, the overview's frontmatter or any other section, and they never commit, approve or set `approved:`.
//
// Every call writes a journal under the plan's never-tracked mirror before it changes the plan directory, and removes it as its last step.
// A failure undoes the call; one whose undo fails keeps the journal, and `restore-renumber` completes the undo from it.
// The restore writes back whole pre-call files, the overview included, and removes every created name whatever it holds by then, so it discards any edit made to the journaled files since the interrupted call.
// A journal naming anything but the overview or a card file at the top level of the plan directory is refused before any file is written.
//
// The three verbs read only the resolved anchor, so they join the lightweight wiring and are not held up by an unrelated module's config.
// `renumberRefusal` words every refusal with its way forward.
//
// # Format verb
//
// `lym loom fmt <file>...` rewrites each named `.go` file in place to its `go/format` form, in-process with no `gofmt` binary, so an agent fixes the formatting `cmd/lym/gofmt_test.go` fails the way its edit directive names.
// It reads only the worktree root and the caller's cwd, so it joins the lightweight wiring, and its arguments are files, never a run-id.
// `planGoFormat` checks and formats every file before any write: a path that does not exist, lies outside the worktree (a symlink resolving outside it included), does not end in `.go`, cannot be read or does not parse refuses the whole call and writes nothing.
// The success envelope's `changed` lists the files rewritten as named; a file already formatted is left untouched and unreported.
// A write that fails stops the call, naming `written` and `failed`.
//
// # Validate plan card checks
//
// Every `lym loom validate-plan` mode but `--require-approved` also runs three card checks of the plan gates, from the context `validatePlanCmd` builds beside the rename tools and refused raw commands.
// `test-tier-unstated` reads the tier keys of the hub's `gate.yaml`, `seam-test-unlisted` reads the `//lym:seam` markers of the wired worktree through `loomshed.SeamTestReader`, and `doc-go-unlisted` reads the package `doc.go` files there.
// An absent `gate.yaml` skips the tier check, as `loomCLI.wire` does for the gates it builds.
//
// # Validate discussion
//
// `lym loom validate-discussion` on a tapestry run also runs the citation check of the loom discussion check, which tests the decision record's cited paths against the worktree and its cited symbols through the wired `PlanIndex`, so it reads that field beside the discussion paths.
// A darn run tests no citation.
//
// # Validate plan lens review
//
// `lym loom validate-plan --lens-review` runs the flag-less check set plus the check that the plan directory's `lens-review.md` holds a recorded lens review, the check set of a twill Plan-Write run's own gate.
// It runs alone: combined with `--rework`, `--require-approved` or `--overlay` it is refused before the plan is parsed.
//
// `lym loom validate-plan --part <name>` runs the check a part writer's own gate runs over the named part's plan, `loomshed.ValidatePart`, with the plan context but no decision record.
// It runs alone: combined with any other mode flag it is refused before the parts file is read.
// It is refused on a recipe with no Parts-Plan-Write row, read from the recipe's rows.
//
// `lym loom validate-parts` runs `partsparser.Validate`, the check the parts gate runs, over the parts file with the most parts `parts_max` allows; it runs on the lightweight wiring, reads the decision record and the worktree, and changes nothing.
//
// `lym loom join-parts` is the chair's one call that joins the parts' plans into the plan directory: it re-checks every part with `loomshed.ValidatePart` and writes the joined plan through `loomshed.JoinParts`.
// It runs on the full wiring and refuses unless the run's status is at the running Parts-Plan-Write row and the caller's `LYM_STRAND_NAME` is the strand name the seat engine gives that row's chair, so a halted or later run, a part writer, a Plan-Burler-Fix fixer and Webster cannot reach it.
// It overwrites only the plan directory, and a rerun within the row discards whatever the chair added after the last join.
//
// # Validate plan overlay
//
// `lym loom validate-plan --overlay` checks the plan with proposed plan file contents laid over it, in memory, so a fix can be checked before any file is edited.
// Stdin is one JSON object whose keys are plan file names and whose values are their full proposed content;
// the names are `00-overview.md` and the card files the Card Index on disk lists.
// The verb builds a `planparser.Overlay` over the plan directory, parses it, and runs the flag-less check set (`loomshed.ValidatePlan`) over the overlaid plan, reporting through the same envelope.
// Handle canonicalization and the done-card comparison read and rewrite through the overlay, and no byte under the plan directory is written.
// The overlay cannot change the tree the glyphs resolve against, so it validates target-list, verify-command and card-text changes, never a proposed code change, and it adds, removes or renames no plan file.
// It is exclusive with `--rework` and `--require-approved`, refused before stdin is read; a terminal or empty stdin, input that is not one object of distinct names and string values, an unusable name and a card file missing from disk are each refused with their way forward in `overlayRefusal`.
//
// # Arbiter
//
// `lym loom arbiter <run-id> --envelope <path> [--report <path>]` is run only by the run's driver, at a stop it cannot settle.
// It spawns a cold arbiter agent on the `arbiter` model through shuttle, waits for its verdict file and prints the validated verdict.
// The arbiter's model, timeout and spawn budget are `shed.yaml`'s `arbiter`, `arbiter_timeout_min` and `arbiter_max_spawns` and their layers, a loom recipe's own values coming through its profile block's `shed` object.
// It is a role verb, so the orch's command index leaves it out.
// The verb's body is `internal/shedarbiter`'s: the call, the prompt, the launch spec, the verdict gate and the consumption of a resume are that package's, and this package fills its deps with loom's seams.
//
// A stop is eligible only if the verb's closed list admits it: an error stop of kind `producer` or `bootstrap`, an `interrupted` stop under the `reinvoke` policy or with a transient class, a halted `blocked` stop other than a Parent-Align or Publish verify halt whose own failure record names a target, and a halted `awaiting` stop whose Bouncer row has a pending escalation of cause `hard-cap` or `circling`, so a `budget` escalation goes to the run's parent.
// The rule lives in `internal/arbiterstop`, which batten's hold shares.
// Every other stop, a missing status file, an unreadable envelope, `arbiter_max_spawns` of 0 and a spent budget are refused with the hand-back to the run's parent as the way forward.
//
// Each spawn is an attempt, numbered from 1 under the run's arbiter directory in `attempt-<N>`.
// `attempt.json` is written before the spawn, so the attempt counts against `arbiter_max_spawns` whatever happens next, and the budget is counted from those records so a restart never resets it.
// The directory also holds `prompt.md`, the agent's `verdict.yaml`, the verb's validated `envelope.json` and, once the re-launch consumed a `resume`, the `applied` mark.
// While an arbiter may be live an ephemeral marker names the attempt, its strand and its deadline.
// The status commit stages the arbiter directory with the run records.
//
// The arbiter's run, started or attached, carries a shuttle gate that reads `verdict.yaml` as the verb does and re-prompts the arbiter with the reader's error while the file does not read, since the verbs it records have already run by then.
// An absent file passes the gate, and once its re-prompt budget is spent the gate lets the run through, so a verdict that still does not read hands on with cause `unparseable`.
// The arbiter stencil asks for every free-text value as a literal block scalar, so a `: ` or ` #` in a command or sentence keeps its meaning.
//
// The prompt carries `amendment_target`: the run's recipe's `amendment_target` key (loom's `Plan-Burler-Review`; darn and batten set none) once the current generation's plan is approved, and `none` otherwise.
// It is read at each spawn, since the driver holds its arbiter seams for the whole drive and the plan is approved mid-drive, so a drive that began before approval still routes.
// The arbiter may then record a plan amendment and name that target as its goto.
// Each `--amends-plan` call re-admits one goto onto the target, the arbiter's repeats are capped by `arbiter_max_spawns` counted from its attempt records, and scope widening is bounded by the stencil's hand-on rule, since the arbiter's entry joins the answer key Plan-Review judges against.
//
// The stop key is the status file's current producer, history length and state.
// A re-run for a stop key an attempt already arbitrated spawns nothing:
// it returns that attempt's verdict, waits on its live arbiter for the time left to its deadline, or stops an arbiter past its deadline through shuttle's stop verb and reads `hand-on` with cause `timeout`.
// An applied `resume` whose stop key is unchanged is refused, since the run stopped at the same place again.
//
// The printed envelope carries `verdict`, `reason`, `question`, `goto`, `verbs`, `cause`, `attempt`, `attempt_dir` and `verdict_path`, and for a `resume` the `relaunch` line.
// A `resume` the verb finds unsound reads `hand-on` with cause `precondition`: no circling decision pending at an escalation, no goto target at a `blocked` stop, a goto target the run's current row does not admit, or a goto target the told goto guard refuses.
// The verb moves no run itself;
// `lym shed step <run-id> --until-stop --arbiter-attempt <N>` performs the verdict's goto under the run and status locks, once per attempt;
// a run already paused at the verdict's goto target with only goto entries since the spawn, the last naming that target, is admitted with no second goto.
// A guard refusal inside that re-launch refuses the attempt, and the driver hands back with the refusal text.
//
// The told goto guard has two arms, run in this order:
// the Finalize arm refuses a goto onto the Finalize row while the parent's code checkout holds tracked dirt, naming the checkout, the paths and the goto and resume that retry it;
// the settled-segment arm refuses a goto onto a review or fix row whose verdict is settled.
// The Finalize arm admits the row when the parent pair cannot be read or holds a fabric merge record, since Finalize's own merge guard then words the problem.
// The guard is wrapped so that once both arms admit a goto, or a consume's target, every fixer role registered under the pair is ended before the move is written.
//
// A pair's session end ends a live arbiter strand with the pair's other strands.
//
// The Go driver reaches the arbiter through its arbiter seam, which calls this same path in-process, so eligibility, `arbiter_max_spawns`, the attempts, the applied mark and the deadlines are shared;
// it passes the tackler's case brief, which the arbiter's prompt asks it to check against the facts.
//
// # Go driver
//
// `lym loom drive` is the hidden role verb that drives a go-seeded run, spawned by `lym loom start`, by `lym loom resume` for a halted go run and by a hand-over.
// It mounts `internal/sheddriver` with loom's seams, and serves darn runs as well as tapestry runs; the loop, rule table, hand-back and tackler below are that package's.
// It takes the run's driver lock, waiting a short grace for a momentary probe to let go, and holds it for its whole life, so a settling driver reads as live while the run lock is free between steps, and a second driver refuses once the grace runs out.
// On start it removes the park marker and the resume report.
//
// The driver registers itself in the owner record under the run's slug with the role `driver`, before it takes the driver lock, and the run's step loop registers itself with the role `loop` once its pid file is written, through the loop's `OnStart` hook;
// a driver the lock refuses exits at once and its record is pruned at the next read.
//
// A step at a row waiting on the operator (Beam-Await, PR-Review, Sampler-Session) reads as active to the step watchdog and as a live wait to dobby, so neither kills it nor sends a quiet notice.
//
// The loop steps the run through the shared `--until-stop` body, saves each stop envelope under the run's drive-reports directory and decides the stop by a closed rule table over the envelope's fields and the facts it reads from disk, never over a row name or the reason text.
// The table settles a done run, a busy exit, a re-step of an interrupted step under the `reinvoke` policy up to a cap of two per stop key, and the one verify goto to the Publish failure record's target;
// it asks the tackler at a stop no rule settles, the arbiter at an awaiting stop with a pending hard-cap or circling escalation only, after a tackler `escalate` and when an applied tackler `resume` returns to the same stop, and hands over at a binary-eligible stop whose last step ran another build than the `lym` on disk.
// Everything else hands back.
// Every action appends a settle record to the friction directory, so the repair cap, the one verify retry and the one hand-over per stop key and build are counted from disk and a restarted driver keeps every cap.
//
// Before it acts on a stop the driver re-reads the status.
// A running status or a run lock another process holds ends the driver with a `running-exit` record;
// a status at another stop key is a moved run, which an orch goto sends to a re-step and anything else decides afresh, and which is never parked and gets no friction note.
// While a pause covers the slug the driver holds in the pause wait before any re-step, goto, agent spawn or hand-over.
//
// `lym loom pr-continue` is the PR-Review agent's verb for the operator's ending of a review: it sets the continue flag in the run's pull-request record and nothing else, and is refused until the PR-Review row has opened the pull request.
//
// `lym loom sampler-done` ends a sampler run's build: it sets the done flag in the Sampler-Session record to the row's current entry stamp and nothing else.
// It is refused by the recipe's rows on a recipe with no SamplerSession row, and while the run stands at another row, the record names no session or the task worktree has uncommitted paths, each with its way forward.
// It is not limited to the sampler agent's strand, so the operator's own shell may run it, and it is accepted while the run is halted at the row, so the next `lym loom resume` ends the row Done at once.
//
// A hand-back leaves a Go-authored stop report, commits the run records and writes the park marker through the code `lym loom commit-records --park` runs, and queues the parent messages the LLM driver's notify rules send.
// A watching batten announces the run and retries a matching verify halt itself, so only the parent notice, a question and a friction-failed done are sent; an unwatched run also gets one line naming the run-id and the report.
// A paused stop that carries no reason also writes a `go-driver-paused` friction note.
// A done run commits its records, and one that finished with `friction: failed` also gets a report and a notice without being parked.
//
// A hand-over runs only at a binary-eligible stop, once per stop key and build, over a clean tree outside the run's records and never under a pause.
// It records the hand-over, leaves the stop report and the park marker, releases the driver lock and spawns a fresh `lym loom drive` of the on-disk build, which re-steps;
// a driver starting at a blocked Finalize stop first runs the Finalize arm and hands back, stepping nothing, while the parent's code checkout is dirty.
//
// A driver restarted over a halted run finds an agent attempt that no later hand-back closed and lets the agent's own call reuse it, so no second agent is spawned.
//
// # Tackler
//
// The tackler is the cold agent the Go driver asks at a stop no rule settles.
// Each call is an attempt under the run's tackler directory in `attempt-<N>`, and `attempt.json` is written before the spawn, so the attempt counts against `tackler_max_spawns` whatever happens next; 0 turns the tackler off.
// The record holds the stop key, the pause conditions, a snapshot of the task worktree, the spawn time, the deadline and the envelope and report paths.
// The directory also holds `prompt.md`, the agent's `verdict.yaml`, an escalate's `case-brief.md`, the call's validated `envelope.json` and, once a re-launch consumed a `resume`, the `applied` mark.
// While a tackler may be live an ephemeral marker names the attempt, its strand and its deadline.
//
// The tackler's verdict is `resume`, `hand-back` or `escalate`.
// A `resume` names a goto only to the run's current row or to the row the halting entry was routed from, and a blocked stop needs one;
// a `hand-back` needs a question for the run's parent, and an `escalate` needs a non-empty case brief.
// A verdict that is missing, unparseable, unsound or late reads as a hand-back whose cause is `missing`, `unparseable`, `precondition`, `timeout`, `died`, `question` or `case-brief`.
// A deadline that ran out while the hub's pause covered the run moves out by the paused time instead.
//
// A re-run for a stop key an attempt already judged spawns nothing: it returns that attempt's verdict or waits on its live tackler, unless a hand-back record for the key closed it.
// An applied `resume` whose stop key is unchanged goes to the arbiter.
// The driver consumes a `resume` through the shed's re-launch, which refuses under the status lock unless the run is still where the tackler found it or already stands paused at the verdict's goto target after only goto entries, and marks the attempt applied in the same hold.
// A guard refusal inside that re-launch refuses the attempt, and the driver hands back with the refusal text.
//
// Tampering is the spawn snapshot of the worktree and a fresh one differing, read only after the stop key proved unchanged:
// a tackler that changed the code worktree hands back with cause `tampered`, and a run an orch moved meanwhile discards the verdict and acts on the fresh state instead.
// A pair's session end ends a live tackler strand with the pair's other strands, since it is an interactive strand in the pair's reed session.
//
// # Peers
//
// `lym loom peers` lists the other runs in flight in the hub, and `lym loom peers show <slug> --record decision|plan` prints one peer's decision record or its plan's `00-overview.md` under a `content` key.
// An agent in one pair uses them to compare its run with the others without forming another worktree's path itself.
//
// The peer set is every board task whose status is a run status, whatever the run's state, with the caller's own slug left out.
// Each peer line carries `slug`, `title`, `status`, `row` (the 1-based position of the status's producer in the peer's own recipe, 0 when it names no row), `parent_branch`, `changed_paths` (the committed paths between the peer's merge base with its parent branch and its head), `decision_record` and `plan` (whether the peer holds them) and `unreadable`.
// A peer's own recipe is the one its worktree's self-run seed records when the worktree is on this machine, and otherwise the one the board entry seeds: pawl for a project entry, else its `recipe` field or, when that is empty, the board's `default_recipe`; a recipe that names no embedded recipe gives row 0.
// A peer whose pair, origin record, branch or records cannot be read is listed with that reason in `unreadable`, never dropped and never an error.
// From a task pair the list also carries `own_row`, the caller's own position by the same rule, 0 when the caller has no board task, and `parent_commits`, the commits on the pair's parent branch since the pair forked, as short SHA and subject, newest first; the prime has none.
// Both verbs refuse in a hub with no board, and `show` refuses an unknown slug, a task holding no run status, the caller's own slug, an unknown record name and a record the peer lacks, each naming its way forward.
//
// The verbs write nothing and read only the board, the peers' two records and self-run seed and git history through `internal/fabricengine`.
// They resolve the location through `lymcwd` in the group's own pre-run, as the `review` and `decision` groups do; every loom verb is hub-only, so `internal/cliwire` does not apply.
// A bare `lym loom peers` outside a lym worktree lists the group's verbs instead of resolving.
//
// # Decision add --from
//
// `lym loom decision add --from <file>` reads a JSON array of objects with the keys `title`, `decision`, `rationale` and optionally `pattern_entry`, `pattern_action` and `pattern_text`.
// `--by` and `--amends-plan` apply to every entry, and all entries land in one write and one commit, or none does.
// Each live agent gets one notice naming the headings (`headings`), and two entries may not share a title or a PATTERN entry.
//
// # Decision amends-plan
//
// `lym loom decision add --amends-plan` records a decision that amends the approved plan:
// besides the entry, the call appends the title of each entry to the tracked plan amendment record, a `--from` batch appending every title in input order, and one commit lands the entries and the amendment record.
// The commit pathspec of the discussion artifacts names the record when it exists or git tracks it, and nothing otherwise, so git is never handed a pathspec matching nothing.
// The flag is refused on a recipe with no Plan-Write row, which has no plan to amend, and while the current generation has no approved plan;
// a plan that does not parse is refused naming `lym loom validate-plan`, and a plan or record read failure is refused with the verb's retry way forward.
// Both refusals tell the caller to record the decision without the flag, since Plan-Write and Plan-Review read the decision record.
// After a successful call the parent runs `lym loom goto --to Plan-Burler-Review` and `lym loom resume`.
//
// A failed append puts both records back to their bytes before the call, removing the amendment record when the call created it, and leaves a decision record that was absent absent.
// When a status commit has already landed the call's amendment bytes, the verb commits the restored record alone, so HEAD never keeps a title whose entry did not land;
// a failure of that commit is refused naming `lym fabric commit`.
// A failed commit of the entry reads HEAD:
// both records at HEAD succeeds with `commit_error` on the envelope;
// the amendment record without the entry, or a HEAD that cannot be read, keeps both files and names `lym fabric commit`;
// a HEAD that differs restores both files and names the re-run of the verb, never `lym fabric commit`.
//
// The landing wiring reads the run's start from the pair's seed and fails on a seed it cannot decode.
//
// # Review subjects
//
// The wiring tells each Bouncer row what its segment judges, so a re-entry over an unchanged subject settles without a review and a changed one reviews only the change under `review_amend_max_bounces`.
// A recipe row names its subject by its `subject` key: Discussion-Bouncer and Plan-Bouncer `document`, Code-Bouncer `code`, in the tapestry and darn recipes alike.
// A document subject hashes the row's artifact paths in order: the decision record through its identity text, a plan directory as its overview's identity text and its top-level card files, and an absent path as absent.
// Its Base is the commit the run records' checkout stands at.
// A code subject is the task worktree's change against the task's merge base, leaving out the run's own state directories and the harness's own files while untracked.
// A run with no recorded parent branch tells no code subject, so its code segments write no records.
// A change confined to the harness's own files reads as an unchanged subject; the landing gate at Parent-Align and Finalize bounds it.
//
// The task's merge base is the one commit every reader of the task's diff starts from: the code subject, the Code-Review rubric's range, Describe's diff, and the verify gate's lints and tree scope.
// It is the merge base of HEAD with the parent tip the merge record names, since the local parent branch can lag a tip the task merged.
// With no merge record, or one that cannot be decoded and is logged at Warn, it falls back to the merge base with the local parent branch.
//
// Every loom recipe builds its Describe session from one `landingshed.DescribeTable`, which lists the run records that exist and renders `(none)` for a run that has none, as a darn run has no webster summary.
// One `commitDescription` commits the landing directory for the Describe row and for the Darn-Write row, so every loom recipe has one description commit.
//
// # The recorded driver's one reading site
//
// A recorded seed driver value is read only in a recipe's bootstrap verbs: loom's start and resume, and `lym loom drive`'s own package.
// Each branch on it selects the run's driving surface — which driver spawns and whether the session carries the loom status strand — and nothing else.
// No producer, generic verb or engine reads the recorded value, and no code path gates behaviour on it, because a second reader lets what a run is doing diverge silently from what it was seeded as.
//
// One carve-out, and only this shape: a CLI verb may compare a driver value the operator just typed against the addressed run's recorded one and refuse on the envelope when they disagree, as battencli's refuseAdoptedSeed does.
// A refusal decided by the recorded value alone, with nothing typed to compare it against, stays barred.
// The rule governs the read, not the vocabulary: the driver constants are named at the seeding sites, which validate a flag before a seed exists.
// Those constant consumers are the shed CLI's seed command, batten's flag validation in internal/battencli/arm.go, batten's child-driver reader in internal/battencli/wire.go, and loom's seed writer in sharedbootstrap.go.
// The seed writer reads `shed.yaml`'s driver_kind and its layers only at seeding, a loom recipe's own value coming through its profile block's `shed` object.
// internal/sheddriver reads no seed driver field.
//
// bootstrap_test.go is a tripwire, not a completeness proof:
// it fails when a production file outside internal/shedrun reads the seed's driver field anywhere but this package and the functions in its driverFieldReadCarveOuts.
// The carve-outs are keyed by file and enclosing function, each with its justification beside it.
package loomcli
