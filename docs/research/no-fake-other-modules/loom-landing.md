# Stand-ins: loom, landing and drift packages

The rule and its terms are in [PATTERN-test-stand-ins](../../../pattern/PATTERN-test-stand-ins.md).
"tier1" or "tier2" in a cell means the replacement moves a `tier0` test to that tier.

## `loomcli`

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
| --- | --- | --- | --- | --- |
| `decisionFake` seam closures, `weaverSeam` apart | `decision_test.go`, `decisionreplace_test.go` | Record reads, commits, trailers and history (`fabricengine`), board copy and raddle reads (`boardengine`), parent and strands (`hubgeom`, `reedengine`), recipe rows (`loomrecipe`), notices (`creelengine`) | `real` | Drive the verb over a `hubforge` pair with creel's daemon in process; tier1, tier2 for live strands; injected errors with no on-disk cause go |
| `recordingNoticePoster`, error-returning notify closures (`notifyParent`, `Post`, `notify`) | `decision_test.go`, `stencilpins_test.go`, `parentreview_test.go`, `raddleverbs_test.go`, `prepush_test.go` | A mail post (`creelengine`) | `real` | The real notice client over an in-process creel daemon; provoke failures through its state; tier1 |
| `circlingFake` | `circling_test.go` | Status (`shedengine`), circling records and caps (`shedadapters`), Finalize acceptance (`landingshed`) | `real` | Wire `circlingDeps` as production does over a run directory the owners wrote; tier0 |
| `fakeDriverSender`, `eventSender` | `driverresume_test.go`, `resume_test.go`, `start_midmerge_test.go` | Typing into the driver pane (`shuttleengine`) | `real` | `shuttleengine.Runner` over the tmux stand-in; tier2 |
| `fakeDriverStarter`, `orderingStarter`, `stubDriverHandle` | `start_driver_test.go`, `resume_test.go`, `stencilpins_test.go` | Driver session launch (`shuttleengine`) | `real` | `runnerDriverStarter` over a shell script as the provider binary, as the smoke files do; tier2 |
| `fakeDriverPaneProbeForArm`, `fakeDriverPaneProbeFull`, `fakeDriverDirectory`, closures in `reedDriverPaneProbe` and `reedDriverDirectory`, `StrandStatus` and `DirectoryRow` literals | `start_driver_test.go`, `resume_test.go`, `driverlaunch_test.go` | Strand list and removal (`reedengine`) | `real` | A real reed engine over a `tmuxkit` server; tier2 |
| `fakeMidMerge`, `midMerge` closures, `MidMergeState` literals | `start_midmerge_test.go`, `resume_test.go`, `start_driver_test.go` | Mid-merge probe (`fabricengine`) | `real` | A `hubforge` pair left mid-merge; tier1 |
| `commitStatusDeps` closures, `stubRecordsDeps` | `commitrecords_test.go`, `wiring_commitstatus_test.go`, `drivewiring_test.go`, `halt_test.go` | Merge probe, commit, push start and board status write (`fabricengine`, `boardengine`) | `real` | The production deps over a hub pair; tier1 |
| `gotoAdmitted`, `retireRuns`, `restoreStatusPane`, `spawnGoDriver`, `spawnWatchdog`, `runVerdict`, `ownerOf`, `recordedParentBranch` and `endGateFixers` seams with canned `ownerVerdict`, `runfsm.Verdict` and `runproc.Ended` | `gotoresume_test.go`, `resume_test.go`, `ownercheck_test.go`, `fixerendguard_test.go`, `start_watchdog_test.go`, `finalizeguard_test.go`, `prreviewgoto_test.go` | Run processes, ownership, origin records, status pane and session stop (`runproc`, `runfsm`, `fabricengine`, `reedengine`) | `real` | Production seams over a hub pair; tier1, tier2 for panes |
| `peersFakeDeps`, `peersFakeTasks` | `peers_test.go` | Board entries (`boardengine`), peer records and parent commits (`fabricengine`) | `real` | A hub board written through `boardengine`, records committed in pairs; tier1 |
| `fakeReviewTarget` | `review_test.go` | Prime name and worktree resolution (`lymcwd`), seed read (`shedrun`) | `real` | A `hubforge` hub with seeded pairs; tier1 |
| `fakeBoardReader` | `hardenwiring_test.go` | Board entry read (`boardengine`) | `real` | The real board over `boardkit.HubWithBoardConfig`, entries added by `boardengine`; unreadable store provoked on disk |
| `parentReviewDeps.parent`, `head`, `parentReviewPRAsker` `Resolve` and `Post` | `parentreview_test.go`, `parentseatwiring_test.go` | Parent resolution (`hubgeom`), task head (`fabricengine`), post (`creelengine`) | `real` | A hub pair with a recorded parent orch; tier1 |
| `renumberDeps.loadState` returning `runplan.State` literals or errors | `renumber_test.go` | Begun batches (`websterengine`, `weaverengine` run state) | `real` | `SaveState` of either engine, as `TestRenumberDepsFor` does |
| `raddleVerbDeps` `row`, `currentRow`, `Board` closures, `raddleTestBoard`, `raddleTestProposal`, `raddleTestRecord` | `raddleverbs_test.go`, `raddlewiring_test.go` | Recipe row (`loomrecipe`), board context (`boardengine`), proposal and record formats (`raddleparser`, `discussionparser`) | `real` | The wired verb over a hub board; the proposal comes from a `raddleparser`-side writer |
| `joinPartsDeps.readStatus` and `join` literals | `joinparts_test.go` | Status (`shedengine`), join result and refusal (`planparser`) | `real` | A parts directory and a status the owners wrote |
| `gatecli.ReachReport`, `MutationReport`, `RedundancyReport` literals and `matchesHead` closures | `reachnote_test.go`, `mutationnote_test.go`, `redundancynote_test.go`, `redundancynote_integration_test.go` | Gate reports and the HEAD match (`gatecli`) | `goes` | `gatecli` exposes no writer to drive here; the rendering of a report moves to the report's owner, or `gatecli` gains a writer |
| `verifyWaiting` and `gateWaiting` `holders` closures | `reviewstatus_test.go` | Gate slot holders (`gateslot`) | `real` | Hold real slots through `gateslot` |
| `shedfake.Shuttle` as `reflectionShuttle` | `halt_test.go`, `friction_test.go` | Reflection agent run (`shuttleengine`) | `real` | `shuttleengine.Runner` over the session stand-in; tier2 |
| `choiceEngineNamer` | `gatefixer_test.go` | Engine naming (`shuttleengine` engine set behind `gatefixer.Engines`) | `real` | `shuttleengine.EngineSet` over `claudeengine` |
| `shellLaunchEngine`, `gateRepromptReadEngine` embedding `shuttlefake.Engine`; `shuttlefake.EngineSet`, `Reed`, `Agents` | `smoke_attachprobe_test.go`, `smoke_gate_test.go`, `wiring_test.go` and siblings | A provider engine (`claudeengine`) and reed (`reedengine`) | `real` | `claudeengine` over a script as the provider binary, answering from recordings; already tier2 where smoke |
| `probingHalfRunner` | `smoke_attachprobe_test.go` | Burler halves (`burlerengine`) | `real` | The real burler half runner over the same live strand; tier2 |
| Provider stand-in scripts `writeStubDriverScript`, `integrationWriteStubDriverScript`, `stubRecordSessionStart` with `IdleInputBoxFixture` and `SessionStartupPayloadFixture` | `smoke_driverstrand_test.go`, `integration_driverbootstrap_test.go` | The `claude` binary and its screen (LLM session) | `external` | Move to one kit home; recordings with source lines, a `tier0` read and a `tier3` contract test |
| `creelfake.Delivery`, `Clock`, `LiveStrand` under creel's real daemon | `notifystamp_integration_test.go`, `prreviewask_integration_test.go` | The hub's strands and panes (`reedengine`, filling `creelengine.Delivery`) | `real` | Delivery over a real reed and `tmuxkit` server; tier2. The clock stays |
| `indexkit.ValidateStub` | `validate_test.go` | Plan index findings (`planglyph`) | `real` | A plan fixture that makes the real index report a finding only it reports |
| `envkit.LandingDeps`, `envkit.NilSeams` host env | `rownotice_test.go` | Landing seams (`fabricengine` and others) | `real` | See `testkit/envkit` below; `NilSeams` itself stays |
| Status files and `shedengine.Status` or `HistoryEntry` literals (`writeTestRunState*`, `writeStatusFixture`, `seedStatus`, `writeRunStatus`, raw JSON) | `circling_test.go`, `resume_test.go`, `status_test.go`, `looplock_test.go`, `parentseatwiring_test.go`, `checklist_test.go`, `landingdeps_test.go` and others | The run status (`shedengine`) | `real` | `loomshed.Seed` plus a driven `shedengine` run; undecodable-file cases move to `shedengine` |
| Hand-written run records: `run.json`, `wait.yaml`, verify markers, `settle-record.json`, circling decision and review round files, rework `record.json`, `store.json`, state and envelope files, `pause.json`, seed text | `reviewstatus_test.go`, `settledguard_test.go`, `finalizeguard_test.go`, `wiring_commitstatus_test.go`, `resume_test.go`, `looplock_test.go`, `wiring_rework_integration_test.go`, `raddlecontext_integration_test.go`, `redundancynote_integration_test.go`, `checklist_test.go`, `arm_seed_test.go`, `peers_integration_test.go` | `shuttleengine`, `verifytree`, `shedadapters`, `burlerengine`, `loomshed`, `boardengine`, `gatecli`, `loomengine`, `pauserecord`, `shedrun` | `real` | Each owner's writer; garbled-input cases move to the owner's tests |
| `board.json` written as text, `boardkit.WriteBoard` entries | `projectrecord_test.go`, `projectrecord_integration_test.go`, `raddlecontext_integration_test.go`, `bootstrap_test.go` | The board file (`boardengine`) | `real` | Board writes through `boardengine`; beam entries need an owner-side writer first |
| `boardengine.Task`, `RaddleStoreEntry`, `fabricengine.OriginReport`, `BranchOutcome`, `AnchoredFileVersion`, `TrailerCommit` literals fed to pure functions and `prePushCheck` seams | `prepush_test.go`, `recordhistory_test.go`, `raddlehandoff_test.go`, `hardenwiring_test.go` | Reconcile reports and history reads (`fabricengine`), entries (`boardengine`) | `real` | Produce each shape from a hub with an origin; tier1 |
| Hand-written decision records, support logs, relevant lists, `requiredDiscussionSectionsForTest` | `validate_test.go`, `decision_test.go`, `discussionrecord_integration_test.go`, `wiring_carryover_integration_test.go`, `validateparts_test.go`, `smoke_gate_test.go` | The discussion record format (`discussionparser`) | `goes` | Format verdict cases move to `discussionparser`; verb tests keep one record built by a `discussionparser` writer |
| `plankit` plans, lens review, parts file, card text edits | `parity_test.go`, `validate_test.go`, `renumber_test.go`, `validateparts_test.go` | Plan and parts formats (`planparser`, `partsparser`) | `real` | A plan writer in `planparser` that `plankit` calls |
| Review and findings files (`writePRFindingsFile`, parent findings YAML) | `prfindings_test.go`, `parentreview_test.go` | Burler review format (`burlerengine`) | `real` | `burlerengine`'s writer for the findings; parse-refusal rows move there |
| Hand-written or text-edited config layers (`review: {hard_max_bounces: 5}`, `fastDeadlineLoomConfig`, `seedLandingConfig`, `seedDarnHubConfig`, `gate.yaml`, `sampler.yaml`, `language.yaml`, `claude.yaml` and `shuttle.yaml` replacements, `drift.yaml`, `env.yaml`) | `hubfixture_helper_test.go`, `circling_test.go`, `wiring_test.go`, `wiring_darn_test.go`, `landingdeps_test.go`, `driftwiring_integration_test.go`, `smoke_helpers_test.go` | Config files of `landingshed`, `darnengine`, `gateslot`, `claudeengine`, `shuttleengine`, `driftengine` and others | `real` | The owner's loader and a `configengine` set, with no byte edit; see hard cases |
| `gateslot.Config`, `modelspec.Registry` literals and `settings` lookup closures | `gatefixer_test.go`, `start_driver_test.go`, `parentseatwiring_test.go` | Loaded config (`gateslot`, `modelspec`, `configengine`) | `real` | Load through each owner |
| `lymcwd.Location` literals, `locationkit.Location`, a fake `.git` file or directory | `halt_test.go`, `resume_test.go`, `hubfixture_helper_test.go`, `pausehold_test.go` and many | A resolved location and git's worktree layout (`lymcwd`) | `real` | `hubforge` hub and `lymcwd.Resolve`; tier1; see hard cases |

## `loomengine`

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
| --- | --- | --- | --- | --- |
| Marshalled `shedengine.Status` as status file | `seedownership_test.go` | The run status (`shedengine`) | `real` | A status seeded by its production writer; the undecodable case stays only as a provoking state |
| `partsPlanHooks` `gates`, `accept`, `findingsPath` | `partsplan_test.go` | Per-part gate and accept seams (`loomshed`, `loomcli`) | `goes` | The wiring is tested in the filling package; table cases keep only that the seams are placed |
| Hand-written parts file | `partsplan_test.go`, `parts_test.go` | Parts format (`partsparser`) | `real` | A writer in `partsparser` |
| Hand-written `PATTERN.md` and decision record with `## PATTERN changes` | `discussiontable_test.go`, `plan_test.go` | `pattern` and `discussionparser` formats | `real` | Owner-side writers; presence-only cases need no more than a file the owner accepts |
| Hand-written `lenses.yaml` layers | `discussiontable_test.go` | Lens and fan config (`lenses`) | `real` | The `lenses` loader's template plus a `configengine` set |

## `loomrecipe`

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
| --- | --- | --- | --- | --- |
| `shedfake.Shuttle`, `SeatRunner`, `BurlerHalfRunner` with `newLoomShuttle`, `newLoomSeats`, `newLoomBurler`, `writeBouncerJudge`, `designWrongBouncerJudge` | `fixture_test.go`, `sequence_test.go`, `resume_test.go`, `escalation_test.go`, `arrasrecipe_test.go`, `reviewbudget_test.go` | Agent runs (`shuttleengine`, `seatengine`, `burlerengine`) and the files agents write, parsed by `shedadapters`, `planparser`, `summaryparser` | `real` | The three real runners over one session stand-in that replays recorded agent output; tier2 |
| `validDecisionRecord`, review file, parts text, join record, description, parent-review `request.json`, proposal text | `fixture_test.go`, `arrasrecipe_test.go` | Formats of `discussionparser`, `burlerengine`, `partsparser`, `summaryparser`, `raddleparser` | `real` | Come from the session recordings above; the old-binary request moves to its reader's tests |
| `fakeAlwaysDoneProducer`, `funcProducer`, `haltingProducer`, `countingProducer`, `arrasScriptedRow` replacing built rows | `fixture_test.go`, `frictionreflect_test.go`, `escalation_test.go`, `resume_test.go`, `arrasrecipe_test.go` | Rows of `loomshed` and `landingshed` | `real` | Drive the real rows over a hub; Preflight and Finalize spawn git (tier1) |
| `fakeWebsterRun` | `fixture_test.go` | Webster run (`websterengine`) | `real` | `websterengine.Run` over the session stand-in; tier2 |
| `envkit.FullEnv`, `envkit.LandingDeps`, `Reconcile` returning a stopped `OriginReport` | `fixture_test.go` and the files over it | Every Env seam; origin reconcile (`fabricengine`) | `real` | Production wiring over a `hubforge` hub; tier1 |
| `plankit` plans | `fixture_test.go` | Plan format (`planparser`) | `real` | A plan writer in `planparser` |
| `batcher.yaml` text | `fixture_test.go`, `resume_test.go` | Batcher config (`batcher`) | `real` | The `batcher` loader's template; the malformed case stays only as the state that provokes the stop |
| `testdata/tapestry-rows-before-motif.yaml` | `blockrecipes_test.go` | A golden of an earlier recipe (`contracts/recipes`) | `goes` | Assert block expansion on the current recipe's rows |
| `testdata/split-perch-recipe.yaml`, `shedbuild.Row` literals | `recipekeys_test.go` | A recipe file and rows (`shedbuild` format) | `real` | Read the split perch rows of the embedded `arras` recipe |

## `loomshed`

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
| --- | --- | --- | --- | --- |
| `fakeInnerProducer`, `planInnerProducer`, `raddleInner`, `darnSession` | `discussionwrite_test.go`, `planwrite_test.go`, `raddlewrite_test.go`, `darnwrite_test.go` | The wrapped agent producer (`shedadapters`) | `real` | The `shedadapters` producer over the session stand-in; tier2 |
| `commitRecorder`, `planCommitRecorder`, `DarnDeps` closures, `PRReworkDeps.ReadCommitted` over a map | `discussionwrite_test.go`, `planwrite_test.go`, `darnwrite_test.go`, `prrework_test.go`, `generation_test.go` | Commits and committed reads (`fabricengine`), record check (`discussionparser`) | `real` | Production closures over a git worktree or hub pair; tier1 |
| `websterArchiveRecorder` | `planwrite_test.go` | Run record archive (`websterengine`) | `real` | Wrap the real archive and record its calls |
| `raddleWriteFixture` closures, `raddleFileFixture` seams, `seam` failures | `raddlewrite_test.go`, `raddlefile_test.go`, `raddlecheck_test.go` | Filing, board status, beam creation, commits and notices (`boardengine`, `fabricengine`, `creelengine`, `loomcli`) | `real` | A hub with its board; tier1. Failures with no state that provokes them go |
| `fakeWebsterRun`, `fakeWeaverRun` | `webster_test.go`, `weaver_test.go` | Implementer runs (`websterengine`, `weaverengine`) | `real` | The real run over the session stand-in; tier2 |
| `fakeSamplerSessions` | `samplersession_test.go` | Sampler session start and stop (`shuttleengine`) | `real` | `shuttleengine.Runner` over the session stand-in; tier2 |
| `samplerWorld` `covered`, `dirty`, `findings`, mail, spec and register seams | `samplersession_test.go` | Pause records, dirt reads, mail and strand registration (`pauserecord`, `fabricengine`, `creelengine`, `reedengine`) | `real` | Production seams over a hub pair; tier1. The clock and sleep stay |
| `unavailableIndex` | `citations_integration_test.go` | A quarry outage (`planglyph`) | `goes` | The outage cannot be provoked here; it moves to `planglyph` |
| `budgetRig` `Notify` closure | `verifybudget_test.go` | Mail post (`creelengine`) | `real` | The real notice client; tier1. `now` and `after` are clocks and stay |
| Hand-built `verifytree.Result` values and verify log text | `verifybudget_test.go`, `verifygate_test.go` | A verify run's result and log (`verifytree`, `gateslot`) | `real` | Run a command through `verifytree`; tier1 |
| `plankit` plans, `plankit.Repo`, overview and lens review text edits | `fixture_test.go`, `gates_test.go`, `partcheck_test.go`, `prrework_test.go`, `gatefindings_test.go` | Plan and parts formats (`planparser`, `partsparser`) | `real` | A plan writer in `planparser` |
| Hand-written decision records, support logs, relevant and advisor files, pattern overviews, parts text | `fixture_test.go`, `gates_test.go`, `partcheck_test.go`, `redfirstgate_test.go`, `recordwrite_test.go` | `discussionparser`, `pattern`, `partsparser` formats | `real` | Owner-side writers; format refusals move to the parsers |
| `planindex.Finding` and `discussionparser.Finding` literals | `gates_test.go`, `darnwrite_test.go` | Validator findings (`planglyph`, `discussionparser`) | `real` | Provoke each finding in the validator; the severity table over `hasBlockingFinding` goes to `planindex` |
| `batcher.yaml`, `gate.yaml` text, `state.json` garbage | `webster_test.go`, `weaver_test.go`, `verifygate_integration_test.go`, `publishfailure_test.go` | Config and run state of `batcher`, `gateslot`, `websterengine` | `real` | Owner loaders and writers; the garbage stays only as a provoking state |
| Hand-written status file bodies | `seed_test.go` | The run status (`shedengine`) | `real` | A `shedengine` writer; undecodable bodies stay only as a provoking state |

## `landingshed`

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
| --- | --- | --- | --- | --- |
| `fakePullRequestAPI`, `scriptedCall` | `prreview_test.go`, `prreview_wait_test.go`, `prreview_session_test.go`, `prreviewask_test.go`, `pullrequestmode_test.go`, `finalize_pr_test.go` | The pull request client (`pullrequest`) | `real` | `pullrequest.Client` over the one recorded GitHub stand-in |
| `recordingParentMerger`, `mergeCallResult` literals | `finalize_test.go`, `finalize_*_test.go`, `guardedrows_test.go`, `parentalign_test.go` | Parent checkout merge, push and fast-forward (`fabricengine`) | `real` | A `hubforge` pair with an origin, as `finalize_integration_test.go`; tier1 |
| `recordingResolver`, `scriptedResolver`, `countingResolver`, `orderedResolver`, `orderRecordingResolver`, `eventResolver` | `publish_test.go`, `finalize_*_test.go`, `parentalign_test.go`, `commitstatus_test.go`, `guardedrows_test.go` | Merge resolution (`mergeresolve`) | `real` | The real resolver over a git pair and the session stand-in; tier1, tier2 for conflicts. Order recording wraps it |
| `fakeVerifyTree`, `fixedGateTiers`, `Result` literals, verify log text | `verifygate_test.go`, `finalize_*_test.go`, `prreview_wait_test.go`, `fixerrun_test.go`, `publish_verify_test.go` | Dirt check and verify run (`verifytree`), tier lists (`gateslot`) | `real` | `verifytree` over a git worktree with a shell command; tier1 |
| `fakeFixerSeats` | `fixerrun_test.go`, `finalize_gate_test.go`, `parentalign_fixer_test.go` | Fixer and reviewer seats with their gate loop (`seatengine`) | `real` | `seatengine` over the session stand-in; tier2 |
| `fakePRSessions` | `prreview_session_test.go` and the PR review files | Session start, stop and liveness (`shuttleengine`) | `real` | `shuttleengine.Runner` over the session stand-in; tier2 |
| `shedfake.MergeShuttle` | `finalize_integration_test.go`, `finalize_gate_test.go` | Conflict agent (`shuttleengine`) | `real` | The real runner over the session stand-in; tier2 |
| `waitWorld` seams, `newFinalizeGateFixture` and `newMergeInFixture` git closures (`TaskHead`, `LineChange`, `HoldsWork`, `TaskRemoteTip`, `ReadParentTips`, `DirtyPaths`, `PushTask`) | `prreview_wait_test.go`, `finalize_gate_test.go`, `parentalign_test.go`, `fixerrun_test.go`, `newTestDeps` in `publish_test.go` | Git reads and pushes (`fabricengine`) | `real` | Production closures over a hub pair; tier1. The clock and sleep stay |
| `MailAgent`, `PauseCovered`, `ShedStatus`, `VerifyWaitMark`, `RowEntry`, `CodeReviewOff`, `MarkTaskDone`, `RecordChanges` closures | `prreview_wait_test.go`, `parentalign_test.go`, `finalize_*_test.go` | Mail, pause record, run status, status pane, board and review settings (`creelengine`, `pauserecord`, `shedengine`, `reedengine`, `boardengine`, `loomcli`) | `real` | The owners over a hub; tier1, tier2 for the pane. `RecordChanges` as a file watch stays real |
| `BeamCommit` literals with entry and commit seams | `beamdescribe_test.go` | Beam history (`fabricengine`) and board entry (`boardengine`) | `real` | A repository with those commits; tier1 |
| `describeStencilFixture`, `finalizeConflictStencilFixture` | `describe_test.go`, `finalize_integration_test.go` | Stencil text (`stencils`) | `real` | `stencilkit` seeding of the shipped stencils |
| `writeSummary` text, webster `summary.md` | `publish_test.go`, `describe_test.go`, `prreview_test.go`, `fixaccept_test.go` | Description and run summary formats (`summaryparser`) | `real` | A writer in `summaryparser`; malformed rows move there |
| `parentlock.Store` with an always-true `Alive` | `parentalign_test.go`, `publish_test.go` | Process liveness (`parentlock`) | `real` | `parentlock.ProcessAlive` over owners naming live processes |

## `pullrequest`

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
| --- | --- | --- | --- | --- |
| `scriptedServer` with hand-written JSON replies | `client_test.go` | GitHub's REST and GraphQL API | `external` | One GitHub stand-in home shared with `githubclient`, recorded answers with sources, a contract test against GitHub |
| `prkit.API` in the merge-state test | `mergestate_test.go` | `Client` and GitHub behind it | `real` | `Client` over the recorded stand-in; see hard cases. `fakeClock` stays |

## `githubclient`

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
| --- | --- | --- | --- | --- |
| `newScriptedServer` status codes and inline `httptest` handlers | `githubclient_test.go`, `transient_test.go` | GitHub's HTTP answers | `external` | Recorded responses in the single GitHub stand-in, a contract test; hangs and drops need recorded failures |
| `withFakeGHAuthToken` | `githubclient_test.go` | The `gh auth token` command | `external` | Recorded `gh` output and failure, kept in this package, with a contract test |

## `driftengine`

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
| --- | --- | --- | --- | --- |
| `commitStore`, `Head`, `Checkouts`, `RecordsChangedSince`, `CommitsSince`, `changedFiles` and diff fields | `commitstore_test.go`, `verify_test.go`, `baseline_test.go`, `expect_test.go`, `await_test.go` | Records commits, trailers and range reads (`fabricengine`) | `real` | A git pair with trailers; tier1 |
| `shedfake.SeatRunner`, `gateLoopSeats`, `fakeChair`, `tableCapturingSeats` | `verify_test.go`, `judgeparent_test.go`, `baseline_test.go`, `expect_test.go` | Seat runs and the gate loop (`seatengine`, `shuttleengine`) | `real` | `seatengine` over the session stand-in; tier2 |
| `Status`, `Post`, `Resolve`, `ReplacedDecisionRecord`, `LockDecisionRecord` closures | `verify_test.go`, `judgeparent_test.go`, `check_test.go` | Run status, mail, parent resolution and decision record reads (`shedengine`, `creelengine`, `hubgeom`, `loomcli`) | `real` | The owners over a hub; tier1 |
| Hand-written `env.yaml` | `config_test.go` | Env vars config (`envsubst`) | `real` | The owner's template or writer; broken-file row stays only as a provoking state |

## `driftcli`

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
| --- | --- | --- | --- | --- |
| `drift.yaml` and `env.yaml` text; status writes by `writeStatus` | `capture_integration_test.go`, `anchor_integration_test.go`, `check_integration_test.go` | Operator config (`driftengine`, `envsubst`) and run status (`shedengine`) | `real` | Template plus `configengine` set; status from a seeded run; see hard cases |

## `pairteardown`

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
| --- | --- | --- | --- | --- |
| `fakeSubstrates` | `teardown_test.go`, `cancel_test.go`, `originseams_test.go` | Processes, locks, strands, session end, removal, pause and parent reads (`runproc`, `reedengine`, `fabricengine`, `pauserecord` and others) | `real` | Production substrates over a hub, as `teardown_integration_test.go`; tier1, tier2 for strands. Unprovokable errors go |
| `runFakes` | `runread_test.go` | Board, run status, origin reads, machine name and push debt (`boardengine`, `shedengine`, `fabricengine`, `machinename`) | `real` | A hub with an origin, as `runread_integration_test.go`; tier1 |
| `removeRecorder`, `recordingRemover` | `originseams_test.go`, `removethrough_test.go` | Pair removal (`fabricengine`) | `real` | Wrap the real removal and record the request; tier1 |
| `closeRecordingAPI` | `teardown_test.go` | Pull request client (`pullrequest`) | `real` | `pullrequest.Client` over the recorded GitHub stand-in |
| `writeInflight` JSON, `board.json` text, raw and marshalled status, `WaitingMarker` JSON | `teardown_helper_test.go`, `originseams_integration_test.go`, `handover_integration_test.go`, `awaitrunlock_integration_test.go`, `teardown_test.go` | Step records (`shedverbs`), board, status (`shedengine`), pause marker (`pauserecord`) | `real` | Each owner's writer; garbled cases move to the owner |
| `takeoverkit.WriteTakeover` | `handover_integration_test.go` | The takeover entry (`fabricengine` take-over writing `shedengine.Status`) | `real` | `fabricengine`'s take-over; see `testkit/takeoverkit` |

## `beamcli`

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
| --- | --- | --- | --- | --- |
| `createWorld` | `create_test.go` | Board owners, pairs, origin, run status and run start (`boardengine`, `fabricengine`, `shedengine`) | `real` | A `hubforge` hub with an origin; tier1, tier2 for run start |
| `landWorld` | `land_test.go`, `describe_test.go`, `mergeparent_test.go` | Board, origin records, statuses, pawl land records, git pushes and merges (`boardengine`, `fabricengine`, `pawlengine`) | `real` | Same hub, as `deps_integration_test.go`; tier1 |
| `prkit.API` | `land_test.go`, `describe_test.go`, `mergeparent_test.go` | Pull request client (`pullrequest`) | `real` | `pullrequest.Client` over the recorded GitHub stand-in |
| Start bring-up and hub stop closures with `HubPause` and `EntryRef` literals | `start_test.go`, `hubstop_test.go` | Session bring-up, hub pause and reap (`reedengine`, `hibernateengine`, `tenderengine`) | `real` | The owners over a hub; tier2 |
| `creelfake.Delivery`, `Clock` under creel's daemon | `dobbydeps_integration_test.go`, `dobbynoticestamp_integration_test.go` | Strands and panes (`reedengine`) | `real` | Delivery over reed and a `tmuxkit` server; tier2. The clock stays |
| Raw status, seed, `check.json`, relay state, `board.json`, pause record and description text | `dobbypairanchor_test.go`, `status_integration_test.go`, `dobbydeps_integration_test.go`, `ownerarchive_integration_test.go`, `deps_integration_test.go` | `shedengine`, `shedrun`, `adoptcli`, `creelengine`, `boardengine`, `pauserecord`, `summaryparser` | `real` | Each owner's writer |
| Hand-written `beam.yaml`, `landing.yaml`, `loom.yaml` layers | `deps_integration_test.go`, `dobbydeps_integration_test.go` | Config of `pawlengine`, `landingshed`, `loomengine` | `real` | Template plus `configengine` set |

## `adoptcli`

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
| --- | --- | --- | --- | --- |
| `world` | `adopt_test.go`, `deploy_test.go`, `caller_test.go`, `archives_test.go` | Board owners and holds, pair add and take-over, run records, deploy and session bring-up (`boardengine`, `fabricengine`, `shedengine`, `landingshed`) | `real` | A hub with an origin, as `adopt_integration_test.go`; tier1, tier2 for bring-up |
| `holdCheckFixture` | `holdcheck_test.go` | Bolt pull and holds (`fabricengine`, `boardengine`) | `real` | The same hub, as `holdcheck_integration_test.go`; tier1 |
| `fixFixture`, `greenFixture` with `fronttask.Deps` closures and `Task` literals | `fixloop_test.go`, `green_test.go` | Board tasks, front-task filing, run records, cancel (`boardengine`, `fronttask`, `fabricengine`, `pairteardown`) | `real` | A hub board and real `fronttask` deps; tier1 |
| `checkFixture` with `scriptedTier`, `RunRecord` literals, `Reconcile`, `Notice` | `check_test.go`, `checkview_test.go` | Gate run and its records (`gatecli`, `gateslot`), reconcile, mail | `real` | `gatecli` over a small module with passing and failing tests; tier1. The clock stays |
| Marshalled `shedengine.Status`, `landing` config text | `adopt_integration_test.go`, `deploy_integration_test.go` | Run status (`shedengine`), landing config (`landingshed`) | `real` | Owner writers and template plus set |

## `statuscommit`

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
| --- | --- | --- | --- | --- |
| `Deps` closures `MergeActive`, `Commit`, `StartPush` | `statuscommit_test.go` | Merge probe, commit and push start (`fabricengine`, pusher), filled by `loomcli` and others | `real` | Production closures over a git pair; tier1 |

## `parentlock`

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
| --- | --- | --- | --- | --- |
| `fakeProcesses` | `store_test.go`, `acquire_test.go` | `Alive`, which production fills with `parentlock.ProcessAlive` | `own` | Stays |

## `testkit/prkit`

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
| --- | --- | --- | --- | --- |
| `API`, `Answer` | `prkit.go` | `pullrequest.API` (`pullrequest`) | `goes` | Deleted; callers drive `pullrequest.Client` over a kit holding recorded GitHub answers |

## `testkit/takeoverkit`

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
| --- | --- | --- | --- | --- |
| `WriteTakeover` rewriting the status JSON | `takeoverkit.go` | The takeover write (`fabricengine`, format `shedengine`) | `goes` | Deleted; tests call the production take-over |

## `testkit/envkit`

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
| --- | --- | --- | --- | --- |
| `FullEnv`, `LandingDeps`, `ReworkDeps`, `driftDeps`, `raddleWriteEnv`, `raddleFileDeps`, `hardenCampaign`: canned closures and literals for every seam | `envkit.go` | `fabricengine`, `verifytree`, `websterengine`, `boardengine` and the other seam fillers | `real` | Production wiring over a `hubforge` hub; tier1 |
| `noopSamplerSessions`, `noFindingsIndex`, `noopFixerSeats`, `noopSessions`, `shedfake` runners | `envkit.go` | `shuttleengine`, `planglyph`, `seatengine`, `burlerengine` | `real` | The real index and runners over the session stand-in; tier2 for tests that call them |
| `TemplateSetting` | `envkit.go` | Setting lookup (`configengine`) | `real` | The production lookup over seeded config |
| `NilSeams`, `testFuncInfo` scan | `envkit.go`, `envkit_test.go` | Nothing; reflection over the Env | `own` | Stays |

## Packages with none

`darnengine` (its config files are its own), `summaryparser`, `discussionparser`.

## Hard cases

Operator-authored config layers have no writer but the operator.
"Written by the owner's loader or writer" gives no clean route for a local layer or a malformed one in another package.

`plankit` renders `planparser`'s format by hand from a kit.
It is called `real` here on the assumption that `planparser` gains the writer.
Until then it is a forbidden kit stand-in.

An LLM agent writes some files another package parses (review, verdict, ledger, decision record).
The rule gives each of them two owners.
They are either outside-thing recordings of the session or the parser's format to be written by the parser.

`shedengine.Status` literals stored through `state.WriteJSON` use the production write primitive with a made-up record.
They are called `real` here, though no owner writer produces an arbitrary mid-run status short of driving the run.

`lymcwd.Location` literals and `locationkit` feed pure path arithmetic.
Strictly they are another module's value, and resolving a real one pushes a large share of `tier0` to `tier1`.

`prkit.API` in `pullrequest`'s `ReadMergeState` test fakes an interface the tested module declares and fills.
The seam exemption names functions only, so the fake is judged by what it simulates, which is GitHub.

Pure decisions over another package's reports (`prePushCheck`, record history, report notes) need many shapes.
Several cannot be provoked cheaply.
The owner cannot test the consumer's decision, so "move to the owner" does not fit.

Error injection into file operations (`failingOps` in `renumber_test.go`) wraps the real `planparser.RenumberIO`.
It stands for the operating system rather than a module, outside the recording rules, and is not listed.
