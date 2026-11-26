// Package loomcli is the `lym loom` command:
// it bootstraps and drives one task's phase machine per worktree on the generic shed engine.
// It carries the verbs through which the operator and the run's parent answer its gates.
//
// The run, step, status, pause and goto bodies are the shared `internal/shedverbs` verbs, mounted with loom's own text.
// A run's shed and routing come from one recipe-keyed builder in `internal/loomrecipe`, pawl's recipe included.
// The wiring branches on the recipe's rows and keys, never its name, and an empty recipe reads as tapestry.
//
// Friction-Reflect runs the Tier 2 friction reflection before the run records done.
// A `blocked` or `failed` halt and an interrupted stop write their Go halt note and spawn no reflection,
// so no stop holds the driver lock for a friction session and `lym loom resume` never waits on one.
// An interrupted stop's note carries the `interrupted-stop` anomaly kind, never `producer-hard-failure`.
// A failed halt on a board stencil that a newer lym build wrote carries `build-skew`:
// the run's lym was older than a board stencil it read, which is a binary to update and not a lym defect;
// the driver hands such a run to a fresh driver when a newer build is on disk.
//
// The `lym loom` verbs serve loom runs only.
// `armAt` refuses a recipe that does not declare the loom contract.
// It does so right after the recipe is resolved and before any wiring.
// It also refuses the plan verbs on a recipe with no Plan-Write row;
// it refuses `validate-parts` on a recipe with no Parts-Write row;
// it refuses `join-parts` on a recipe with no Parts-Plan-Write row.
// `ArmAt`, which `lym shed` shares, never refuses a recipe.
// `lym loom checklist` is exempt, being a read-only pane the pre-run never arms.
//
// A run's decision record follows its recipe's rows:
// a recipe with a Beam-Await row keeps the beam's project record under the beam's slug,
// and every other recipe keeps the discussion record.
// Each status transition writes `<FSM state> · <producer>` on the run's board entry.
// A finished run writes nothing when its recipe has a Finalize row, whose landing marked the entry done;
// it writes nothing with a RaddleFile row, whose filing ended the entry or turned it into its project.
// A finished motif run makes its entry a Sketch, a note of the built-in type `sketch`, with its record stored.
// The board can refuse that as an invalid write.
// In that case the run warns, leaves the entry draft and mails its parent the refusal.
//
// A run seeded with the recipe `pawl` is a beam's life.
// It has no plan, no review round and no reroute edge.
// Its Finalize halts, behind its own prefix, while an open member or a pair forked from the beam still works.
//
// `decision add`, `replace` and `store` copy the committed record onto the board entry, never a beam's project record.
// A failed copy is reported as `record_store_error` and never fails the verb.
// `decisionCallerRefusal` is the caller rule of `decision add` and `decision replace`:
// the run's parent orch, the run's own arbiter with `--by arbiter`,
// and a sampler run's recorded session strand with `--by operator` only while its Sampler-Session row is current.
// `decision add`, `replace` and `store` refuse a raddle run, naming the goto or the filed parts.
// `decision add` with `--amends-plan` or a PATTERN change is refused while the run's drift judge session is open.
// `decision replace` commits a whole record with a `Lym-Record-Replaced` trailer under the run's decision lock.
//
// `parent-review answer` records the parent orch's answer to the run's latest ask.
// It admits the operator's unnamed shell, the run's parent orch and the strand the ask went to.
// It also answers a latest PR-Review ask that ended no-parent or undelivered,
// so a parent the ask never reached can still settle its head.
// `parent-review await` is the asking agent's capped wait on the same ask, and it writes nothing but a timeout.
// `parent-review ask` records and posts a PR-Review ask at the task head for the review's session.
// It signs the post as the session's own strand, where the PR-Review row's own ask signs as the loom program.
// Only that session runs it.
// It admits only a review recorded under autonomy, and it refuses an ask that already awaits an answer at the head.
// It also refuses an ask at a head where a satisfied answer stands.
// The Discussion-Review parent seat reads `loom.discussion.review_ask_parent`.
// The Code-Review parent seat reads `loom.code.review_ask_parent`.
// They read them on each call, and an unreadable value reads as false.
//
// The rows `loom.notify_parent_on` names post a `[row reached]` notice once per visit.
// A name that is no row of the recipe refuses the shed.
// A failed read, write or post only warns: a notice never stops the run.
//
// `wire` loads the hub's `sampler.yaml` and `hardener.yaml` strictly, refusing a malformed file before any row builds.
// It fills `Env.GateFixer` through `gateFixerSeam`, so a failure at a writer's idle boundary goes to a cold fixer first.
// The seam returns the spec unchanged on the prime and outside a hub.
//
// # Start
//
// `lym loom start` resolves the recorded parent branch, seeds the status file when absent and commits the seed.
// A first seed clears the friction directory and pins the body hash of every installed registry stencil.
// An unseeded run is seeded with the driver `shed.yaml`'s `driver_kind` resolves to, `llm` by default;
// a seeded run keeps its recorded driver, and no flag chooses one.
// It brings up the worktree's tmux session, ensures the status strand and spawns the per-hub watchdog best-effort.
// A failure of the follow-state and window selection is logged and never fails the start.
//
// Start and resume refuse, before anything else, a run whose status file another machine owns:
// the kind is `owned_elsewhere` and the message names `lym adopt <subject>`.
// Before spawning or resuming a driver they refuse an unfinished merge with `merge_in_progress`.
// A run at Parent-Align or Finalize is the exception, since it redoes its own parent merge-in.
// A live loom driver parked at a hand-back is resumed by typing one line into its pane.
// A pane holding a draft or an active operator is refused with the retryable kind `driver_busy`.
// A halted run whose driver has not yet written its park marker is refused with `driver_not_parked`;
// a live driver over a running run is left working.
// A loom driver strand reed marked retiring is never adopted: start removes it and spawns a fresh driver.
//
// start returns once the driver's readiness signal confirms it is up.
// For the Go driver the signal is the driver lock being taken, which the driver holds for life.
// For a loom driver it is the provider TUI coming up within shuttle's `startup_timeout_s`.
// A readiness refusal removes the driver strand, so the next start spawns a fresh one.
//
// # Resume
//
// `lym loom resume` takes the bootstrap lock start takes and decides by the run's state:
//
//   - no status file: refused, naming `lym loom start`.
//   - done: a no-op success.
//   - awaiting at a Bouncer row: handled as a halted run when a circling decision is recorded, otherwise refused.
//   - awaiting at any other row: refused, naming the verb the run's waiting note names.
//   - running: a no-op success when the driver is live or a lock is held, otherwise handled as halted.
//   - halted with a lock held or an unparked live driver: refused with `driver_not_parked`.
//   - halted with a live parked driver: resumed, or refused with `merge_in_progress` or `driver_busy`.
//   - halted with no live driver and a Go-driver seed: a Go driver is spawned and the envelope names its pid.
//   - halted with no live driver otherwise: refused, naming `lym batten run <slug>` or `lym loom start`.
//
// The driver strand is read from reed's directory, never from the run's seed.
// A start or resume that spawns or wakes a driver compares the run's stencil pins with the installed stencils.
// One notice to the run's parent orch names each changed or missing stencil,
// and the envelope lists them under `stencils_changed`.
// A failed compare never fails the verb.
// `lym loom circling continue` refuses at or past the perch's hard cap, naming `circling accept` or raising the cap.
//
// # Plan card verbs
//
// The sanctioned way to add or drop one plan card and renumber the cards after it is these verbs:
//
//   - `lym loom insert-card <position> <slug> --intent <intent>`
//   - `remove-card <position>`
//   - `restore-renumber`
//
// A renumber rewrites card ids, headings, `Commit:` prefixes and the Card Index.
// The success envelope's `prose_mentions` lists the lines holding a bare card reference with no faithful rewrite.
// The agent fixes those by hand.
// `insert-card` and `remove-card` refuse a position at or before the last card of any batch the implementer has begun.
// They touch only the current generation's card files and the overview's Card Index, and never commit or approve.
// Every call writes a journal before it changes the plan directory.
// A failure undoes the call; one whose undo fails keeps the journal, and `restore-renumber` completes the undo from it.
// The restore discards any edit made to the journaled files since the interrupted call.
//
// # Format verb
//
// `lym loom fmt <file>...` rewrites each named `.go` file in place to its `go/format` form.
// It runs in-process with no `gofmt` binary.
// A path that is missing, outside the worktree, not `.go`, unreadable or unparseable refuses the whole call.
// Such a call writes nothing.
// A file already formatted is left untouched and unreported.
//
// # Validate plan card checks
//
// Every `lym loom validate-plan` mode but `--require-approved` also runs these card checks:
// `test-tier-unstated`, `seam-test-unlisted` and `doc-go-unlisted`.
// An absent `gate.yaml` skips the tier check.
//
// # Validate discussion
//
// `lym loom validate-discussion` on a tapestry run also tests the decision record's cited paths and symbols.
// A darn run tests no citation.
//
// # Validate plan lens review
//
// `lym loom validate-plan --lens-review` runs the flag-less check set plus one more check.
// That check is that the plan directory's `lens-review.md` holds a recorded lens review.
// `--part <name>` runs the check a part writer's own gate runs over the named part's plan.
// Both run alone, refused when combined with another mode flag.
// `lym loom validate-parts` runs the check the parts gate runs and changes nothing.
// `lym loom join-parts` joins the parts' plans into the plan directory.
// It refuses unless the run stands at the running Parts-Plan-Write row and the caller is that row's chair strand.
// A rerun within the row discards whatever the chair added after the last join.
//
// # Validate plan overlay
//
// `lym loom validate-plan --overlay` checks the plan with proposed plan file contents laid over it in memory.
// The contents are read from stdin as JSON.
// No byte under the plan directory is written.
// It validates target-list, verify-command and card-text changes.
// It never validates a code change or an added, removed or renamed plan file.
// It is exclusive with `--rework` and `--require-approved`.
//
// # Arbiter
//
// `lym loom arbiter <run-id> --envelope <path> [--report <path>]` is run only by the run's driver.
// The driver runs it at a stop it cannot settle.
// It spawns a cold arbiter agent through shuttle, waits for its verdict file and prints the validated verdict.
// The arbiter's model, timeout and spawn budget come from `shed.yaml`:
// the keys `arbiter`, `arbiter_timeout_min` and `arbiter_max_spawns`.
// A loom recipe's own values come through its profile block's `shed` object.
// The verb's body is `internal/shedarbiter`'s, and the eligibility rule lives in `internal/arbiterstop`.
// The verb refuses each of these with the hand-back to the run's parent as the way forward:
// every ineligible stop, a missing status file, an unreadable envelope, a zero `arbiter_max_spawns` and a spent budget.
//
// Each spawn is an attempt, and `attempt.json` is written before it,
// so the attempt counts against the budget whatever happens next.
// The budget is counted from those records, so a restart never resets it.
// The arbiter may make one fix of its own, committed on the task branch.
//
// The prompt carries `amendment_target`:
// the recipe's key once the current generation's plan is approved, and `none` otherwise.
// It is read at each spawn, so a drive that began before approval still routes.
//
// A re-run for a stop key an attempt already arbitrated spawns nothing:
// it returns that attempt's verdict or waits on its live arbiter until its deadline.
// An applied `resume` whose stop key is unchanged is refused, since the run stopped at the same place again.
// A `resume` the verb finds unsound reads `hand-on` with cause `precondition`.
// The verb moves no run itself:
// the verdict's goto is performed by `lym shed step <run-id> --until-stop --arbiter-attempt <N>`.
// A guard refusal inside that re-launch refuses the attempt, and the driver hands back with the refusal text.
//
// The told goto guard refuses a goto onto Finalize while the parent's code checkout holds tracked dirt,
// and a goto onto a review or fix row whose verdict is settled.
// It ends every fixer role registered under the pair once a goto is admitted, before the move is written.
// Once the move is written, the goto sets aside the worktree's interrupted shuttle runs,
// so the rows it restarts start fresh sessions instead of resuming one.
//
// # Go driver
//
// `lym loom drive` is the hidden role verb that drives a go-seeded run, spawned by start, resume and a hand-over.
// It mounts `internal/sheddriver` with loom's seams, whose loop, rule table, hand-back and tackler govern it.
// It holds the run's driver lock for its whole life, so a settling driver reads as live and a second driver refuses.
// The driver's detached stdout and stderr go to the driver log, never to start's output.
//
// A step at a row waiting on the operator reads as active to the step watchdog and as a live wait to dobby.
// Those rows are Beam-Await, PR-Review and Sampler-Session.
//
// `lym loom pusher --worktree <code worktree> [--slug <slug>]` is the hidden role verb of a worktree's background pusher.
// It resolves the worktree its flag names, never the cwd, and arms no run.
// A pair's pusher, not the commit, runs the pre-push reconcile before each push.
// The prime's pusher pushes its records for every batten run and runs no check.
//
// `lym loom pr-continue` sets the continue flag in the run's pull-request record when the operator ends a review.
// It is refused until the PR-Review row has opened the pull request.
// Under a review whose record holds autonomy true, it sets the flag only on the parent's satisfied answer.
// That answer must stand at the current head.
// A record whose autonomy is false or absent takes no parent answer.
// `lym loom pr-findings <file>` takes findings in burler's review format.
// It sends the set back to Code-Review when the reroute edge can fire.
// That needs a BLOCKING finding or findings exceeding `pr.review_small_max`; otherwise it only counts them.
// It is refused while the task worktree has uncommitted paths.
// `lym loom sampler-done` sets the done flag in the Sampler-Session record and nothing else.
// It is refused on a recipe with no SamplerSession row and while the run stands at another row.
// It is also refused when the record names no session or the task worktree has uncommitted paths.
//
// A hand-back leaves a Go-authored stop report and writes the park marker.
// `lym loom commit-records` commits the pair's records and starts the pair's pusher, which pushes them;
// its ok envelope carries `committed` and `pusher_started`.
//
// The pair's pusher passes the pre-push origin reconcile before each push.
// The landing's commits and pushes and the pull-request record's commits pass it too.
// The reconcile is skipped when pushes are off, in the prime, under a pause and at Finalize.
// An unreachable origin or a failed check lets the push go, so the push's own failure is recorded.
// A run another machine owns stops the push with no pause, naming `lym adopt <subject>`.
// A run a branch stops is paused naming `lym tender resume <slug>`,
// and a run origin ended is paused naming `lym cleanup --apply`.
// The check reads origin once per commit and push,
// so a move between the check and the push is caught only by the push's refusal.
//
// # Tackler
//
// The tackler is the cold agent the Go driver asks at a stop no rule settles.
// Each call is an attempt under the run's tackler directory, counted against `tackler_max_spawns` from `attempt.json`.
// A `tackler_max_spawns` of 0 turns the tackler off.
// Its verdict is `resume`, `hand-back` or `escalate`.
// A missing, unparseable, unsound or late verdict reads as a hand-back.
// A tackler that changed the code worktree hands back with cause `tampered`.
// An applied `resume` whose stop key is unchanged goes to the arbiter.
//
// # Peers
//
// `lym loom peers` lists the other runs in flight in the hub.
// `peers show <slug> --record decision|plan` prints one peer's record under a `content` key.
// The verbs write nothing and refuse in a hub with no board.
// A peer whose pair, origin record, branch or records cannot be read is not dropped.
// It is listed with a reason in `unreadable`.
//
// # Decision add --from
//
// `lym loom decision add --from <file>` reads a JSON array of entries.
// Each entry has `title`, `decision` and `rationale`.
// It may also have `pattern_entry`, `pattern_action` and `pattern_text`.
// All entries land in one write and one commit, or none does.
// Two entries may not share a title or a PATTERN entry.
// A run that is not running wakes no Webster Master session or Weaver strand;
// the envelope names each such strand and why under `notice_held_back`.
//
// # Decision drops-create
//
// `lym loom decision add --drops-create <target>` (repeatable) records that the decision drops a Create target.
// The target is spelled as the card spells it.
// The entry gets one `Drops Create: `<target>“ line per target after its Rationale paragraph.
// Webster's done-check skips a dropped target.
// The flag applies to one entry: a call that records several is refused, naming a separate call as the way forward.
// An empty target, or one holding a backtick or a line break, is refused.
//
// # Decision amends-plan
//
// `lym loom decision add --amends-plan` records a decision that amends the approved plan.
// It also appends each entry's title to the tracked plan amendment record in the same commit.
// It is refused on a recipe with no Plan-Write row and while the current generation has no approved plan.
// After a successful call the parent runs `lym loom goto --to Plan-Burler-Review` and `lym loom resume`.
// A failed append restores both records to their bytes before the call.
// HEAD therefore never keeps a title whose entry did not land.
// A failure to restore names `lym fabric commit`.
//
// # Review subjects
//
// Each Bouncer row's `subject` key says what its segment judges: `document` or `code`.
// A re-entry over an unchanged subject settles without a review,
// and a changed one reviews only the change under `review.amend_max_bounces`.
// A code subject is the task worktree's change against the task's merge base.
// The merge base is the merge base of HEAD with the parent tip the merge record names.
// A run with no recorded parent branch tells no code subject, so its code segments write no records.
// A change confined to the harness's own files reads as an unchanged subject;
// the landing gate at Parent-Align and Finalize bounds it.
//
// The Code-Review rubric's `redundancy_report`, `reach_report` and `mutation_report` markers list the worktree's report.
// They do so only when it matches HEAD, and otherwise say no current report exists.
// A report written in the prime never matches.
//
// # The recorded driver's one reading site
//
// A recorded seed driver value is read only in a recipe's bootstrap verbs:
// loom's start and resume, and `lym loom drive`'s own package.
// Each branch on it selects the run's driving surface and nothing else.
// No producer, generic verb or engine reads it.
// A second reader would let what a run is doing diverge silently from what it was seeded as.
//
// One carve-out: a CLI verb may compare a driver value the operator typed against the addressed run's recorded one.
// It may refuse when they disagree.
// A refusal decided by the recorded value alone stays barred.
//
// `bootstrap_test.go` is a tripwire, not a completeness proof:
// it fails when a production file outside `internal/shedrun` reads the seed's driver field.
// This package and the functions in its `driverFieldReadCarveOuts` are exempt.
package loomcli
