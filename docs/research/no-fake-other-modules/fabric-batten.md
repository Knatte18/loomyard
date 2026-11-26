# Stand-ins: fabric, git, hub and batten packages

Stand-ins in these packages, classified against [PATTERN-test-stand-ins](../../../pattern/PATTERN-test-stand-ins.md).
Paths in the tables are relative to the package named in the heading, or to `internal/` where a heading names several.

## fabricengine

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
|---|---|---|---|---|
| `boardEntry` and `boardFileOf` marshal `boardengine.Task` into a hand-framed `board.json` | `replay_integration_test.go`, `boltpullfirst_`, `originreconcile*_`, `pushclasses_integration_test.go` | Board store (`boardengine`) | `real` | `boardengine.Board.UpsertTask` on the fixture hub, then commit; already tier1 |
| Literal `board.json` texts with only `slug` and `title` | `replay_integration_test.go` | Board store (`boardengine`) | `real` | Same writer; a key the writer cannot produce is a case for `boardengine`'s parser |
| `heddleFileOf` marshals a `heddleengine.Store` | `replay_integration_test.go` | Heddle store (`heddleengine`) | `real` | `heddleengine.Save`; already tier1 |
| `statusJSON`, `plantRunTransition`, `plantTakeover`, `writePairStatus` and literal run `status.json` texts | `replay_`, `originreconcile*_`, `originreads_`, `codetip_`, `rejectedtakeover_`, `anchorchange_integration_test.go` | Run status (`shedengine`) | `real` | `shedengine`'s status writer, exported where it is not; already tier1 |
| `shedengine.Takeover` literals; `takeoverkit.WriteTakeover` into a marshalled status | `archiveparent_`, `handover_`, `takeoverrun_`, `codetipsettle_integration_test.go` | Takeover entry (`shedengine`) | `real` | Build the entry and its status through `shedengine`'s writer |
| `fakePushNotifier`, and closures passed to `InstallPushNotifier` | `pushnotifier_helper_test.go`, `failedpush_test.go`, `remoteteardown*_test.go`, the push integration tests | Failed-push mail (`orchcli`) | `real` | `orchcli.InstallPushNotifier` over a real box; a pure recording wrapper may stay; likely tier1 |
| `teardownRecorder` filling `OriginSeams.Teardown` | `originreconcileended_integration_test.go` | Ended-pair teardown (`pairteardown`) | `real` | The `pairteardown` origin seams; rows needing a live session are tier2 or move there |
| `OriginSeams.OwnerOf` closures | `originreconcile_`, `originreconcilemoves_`, `boltreplayers_integration_test.go` | Owner read (`pairteardown`, `boardengine`) | `real` | `pairteardown`'s `ownerOf` over a board written by `boardengine.SetOwners`; tier1 |
| `fakeWorktreeSeam` | `worktreeseam_integration_test.go` | Worktree render (`heddleengine`) | `real` | `heddleengine.Seam` inside a recording wrapper; refusals provoked through heddle state; tier1 |
| `fakeReplayer`, `emptyEntryLedger`, `scribbleThenFailReplay` | `boltreplayers_`, `replay_integration_test.go` | Bolt replayers (`boardengine`, `heddleengine`) | `goes` | Drive `boardengine.FabricReplay`; cases for answers no real replayer gives are removed |
| `lymcwd.Location` literals and `fakeLayout` | most tier0 files, e.g. `destroy_test.go`, `warpjunction_test.go`, `refscanner_test.go` | Resolved location (`lymcwd`) | `real` | `lymcwd.Resolve` over a real repository; tier1 |
| Porcelain output texts | `dirtiness_parse_test.go`, `worktreelist_parse_test.go` | git output | `external` | Recordings with sources, and a contract test; one home beside the git reader |
| `pushRecorder` | `export_test.go` | `fabricengine`'s own detached push spawn | `own` | Stays |
| `runTeardownStandIn` (the test binary as teardown child), `recordingStdin` | `testmain_test.go`, `remoteteardown_childhold_integration_test.go` | `fabricengine`'s own child process | `own` | Stays |
| Hand-written move record, hand-over mark, teardown job and merge record | `moverecord_helper_test.go`, `rejectedtakeover_`, `remoteteardown_integration_test.go` | `fabricengine`'s own records | `own` | Stays |

## fabriccli

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
|---|---|---|---|---|
| `anchorFake` seams for hub, anchor, owner, lock and move-record reads, and `fakeChange` | `anchor_test.go` | `fabricengine`, `lymcwd`, `boardengine`, `parentlock` | `real` | `anchorDeps` over a `hubforge` hub and the real `AnchorChange`; tier1 |
| `anchorFake` seams for reed, dobbies, hibernation, relay and gates | `anchor_test.go` | `reedengine`, `dobbyengine`, `hibernatecli`, `gateslot`, Slack relay | `goes` | Live-session answers cannot be produced below tier2; those cases move to the owners or go to tier2 |
| `fabricengine.RemoveResult` and `pairteardown.SessionResult` literals | `removefields_test.go` | Removal results | `real` | Results of a real removal; tier1, tier2 for ended sessions |
| Hand-written webster `state.json` and `outcome.yaml` | `mergescenario_integration_test.go` | Webster state (`websterengine`) | `real` | `websterengine.SaveState` and the outcome writer; already tier1 |
| Marshalled `hibernateengine.Request` file | `hibernatedaemon_test.go` | Hibernate request (`hibernateengine`) | `real` | The request writer, exported; the file stays tier0 |
| Hand-written `board.yaml` and `tapestry.yaml` seeds through `hubforge.SeedConfig` | `hubmutation_`, `reconcile_anchor_`, `mergescenario_integration_test.go` | Config of `boardengine` and `loomengine` | `real` | The owner's template or config writer; already tier1 |
| `fakeWorktree` `.git` layout | `roleguard_test.go` | git repository layout | `external` | One kit that records a real layout, with a contract test |
| Hand-built `github.PullRequest` values | `openprheads_test.go` | GitHub API | `external` | A recorded response, read in tier0, with a contract test |

## gitexec and gitrepo

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
|---|---|---|---|---|
| Transport and push-rejection stderr texts | `gitexec/transport_test.go`, `gitrepo/pushretry_test.go` | git output | `external` | Recorded stderr with source lines; `transport_integration_test.go` is the start of the contract test |
| Version probe answers | `gitexec/gitexec_test.go` (`TestCheckFloor`) | `git --version` | `external` | Recorded version lines and a contract test |
| Start seam returning `os.PathError` | `gitexec/startretry_test.go` | OS refusing to start git | `external` | A recorded failure in the one home, `gitexec` |
| Shell scripts as `ext::` remotes that hang or print | `gitexec/gitexec_unix_test.go` | A remote that real git talks to | `external` | Stays under real git; `gitexec` is its single home |
| `GitError` values built to test its rendering | `gitexec/errorrender_test.go` | `gitexec`'s own type | `own` | Stays |

## dotgit, lymcwd, lock and other git layouts

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
|---|---|---|---|---|
| Hand-built `.git` directories, `gitdir:` files, `HEAD` and `commondir` | `dotgit/dotgit_test.go`, `dotgit/worktrees_test.go`, `lymcwd/lymcwd_test.go`, `lock/tracked_test.go`, `hubgeom/hubgeom_test.go`, `runpusher/seams_test.go`, `battencli/dobbynotice_test.go`, `battencli/wire_seams_test.go`, `battencli/cli_test.go` | git repository layout | `external` | One kit beside `dotgit` holding recorded layouts, with a tier1 contract test |
| Hand-written anchor marker files | `lymcwd/lymcwd_test.go`, `lymcwd/anchorread_test.go` | Anchor marker (written by `fabricengine`, named by `lymcwd`) | `own` | Stays; see Hard cases |

## hubforge

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
|---|---|---|---|---|
| `ReanchorPair` writes the anchor marker, `git mv`s records and commits to mimic an anchor move | `reanchor.go`, used by tests of two-anchor hubs | Anchor move (`fabricengine`) | `real` | `fabricengine.BeginAnchorChange`; already tier1 |
| `NewRecordsStopError` builds a `fabricengine.WeftStopError` | `recordsstop.go`, `battencli/land_test.go` | Pull stop (`fabricengine`) | `real` | Provoke the stop with a diverged records branch; tier1 |
| `SeedConfig` and `SeedFabricConfig` write caller-supplied YAML bytes | `seed.go` and its callers | Config files of other modules | `real` | The kit takes values and writes through each module's config writer |

## hubgeom

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
|---|---|---|---|---|
| Literal `board.json` texts | `autonomy_`, `parentof_`, `tasksetting_`, `engineworktree_integration_test.go`, `hubgeom_test.go` | Board store (`boardengine`) | `real` | `boardengine.Board` writes; `hubgeom_test.go` rows then spawn git, tier1 |
| `boardengine.Owners`, `OwnerEntry` and `Registry` literals | `dobbygeom_test.go` | Owner read (`boardengine`) | `real` | `boardengine.SetOwners` and `ReadOwners` |
| `hubWithOrigin`: hand-written origin record, shortname file and lock directory; hand-written move record | `hubgeom_test.go`, `parent_test.go`, `dobbygeom_test.go` | `fabricengine` records and hub layout | `real` | `fabricengine.WriteOrigin` and a `hubforge` hub; tier1 |
| Literal run `seed.json` texts | `parent_test.go`, `spawnorder_test.go` | Run seed (`shedrun`) | `real` | `shedrun.WriteSeed`; unreadable or legacy forms move to `shedrun` |
| Hand-written autonomy, gate, env, desktop and beam-mode YAML layers | `autonomy_`, `beammodes_`, `geometry_integration_test.go`, `webstergeom_test.go`, `hubgeom_test.go` | Config of the owning modules, e.g. `gateslot` | `real` | The owner's config writer; malformed layers move to its parser tests |
| `lymcwd.Location` literals | `hubgeom_test.go`, `weavergeom_test.go`, `webstergeom_test.go`, `spawnorder_test.go` | Resolved location (`lymcwd`) | `real` | `lymcwd.Resolve`; tier1 |

## hubreconcile

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
|---|---|---|---|---|
| Literal `board.json` texts | `carry_test.go`, `ensure_integration_test.go` | Board store (`boardengine`) | `real` | `boardengine` writes; old keys and unparseable text are Hard cases |
| Hand-written fabric merge record `fabric-merge.json` | `ensure_integration_test.go` | Merge record (`fabricengine`) | `real` | Stop a real `MergeIn` on a conflict; already tier1 |
| `refusingSeam` | `ensure_integration_test.go` | Worktree render (`heddleengine`) | `real` | `heddleengine.Seam` refusing through its own state |
| `recordNotices` filling `InstallNotifier` | `reinterpret_integration_test.go` | Notice mail (`orchcli`) | `real` | The installed production notifier over a real box |
| `recordBoardSyncs` filling `SetSpawnBoardSyncForTest` | `ensure_integration_test.go` | Detached board sync (`boardengine`) | `real` | Run the real sync in the foreground inside a recording wrapper; tier1 |
| Hand-written `slack`, `loom` and `heddle` files, broken or fixed | `ensure_`, `reinterpret_integration_test.go` | Config and store of other modules | `real` | The owner's writer for valid files; broken ones are Hard cases |

## standalonegeom

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
|---|---|---|---|---|
| Hand-written `gate.yaml` | `standalonegeom_test.go` | Gate config (`gateslot`) | `real` | `gateslot.ConfigTemplate` or its config writer |

## battencli

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
|---|---|---|---|---|
| `newFakeReceiver`: a whole `shedrecipe.Env` of no-op and canned seams, plus an `{}` child status | `run_test.go`, `step_`, `pausehold_`, `awaiting_`, `cli_`, `owner_`, `predrive_`, `driverdeps_test.go` and others | `fabricengine`, `shedengine`, `loomengine`, `boardengine` and more through `battenshed` | `real` | The wired `Env` over a `hubforge` hub, tier1; the child session cases below go to tier2 |
| Canned `InnerRun.Spawn`, `DriverAlive`, `ReadStatus` and `Shutdown` closures | `records_`, `lifecycle_`, `teardownpush_integration_test.go`, `run_test.go` | Child driver start and liveness (`loomcli`, `reedengine`) | `goes` | A child needs an LLM session; keep only what a tier2 run over the session stand-in drives |
| `cancelFake` | `cancel_test.go` | `fabricengine`, `boardengine`, `pairteardown`, `runproc`, `runfsm` results | `real` | `hubCancelDeps` as in `cancel_integration_test.go`, tier1; live-session rows tier2 or `pairteardown` |
| `fakeOwners`, `ownedBy`, `boardengine.Owners` literals | `owner_helper_test.go`, `owner_test.go`, `wire_seams_test.go` | Board owners (`boardengine`, `machinename`) | `real` | `hubOwnerSeams` over a real board and machine file; tier1 |
| `fakeReadopt` with hand-built `OriginReport`, `RecordsTag`, error values and marshalled child statuses | `owner_readopt_test.go` | `fabricengine`, `shedengine` | `real` | The wired `readoptSeams` over states fabric reads, as `readopt_integration_test.go`; tier1 |
| `cleanupStubs`, `buildCleanupFixture`, `statusJSON`, `{}` seeds | `cleanup_test.go`, `cleanup_helper_test.go`, `cleanup_`, `beamguard_integration_test.go` | Records git (`fabricengine`); status and seed (`shedengine`, `shedrun`) | `real` | Wired deps, `shedrun.WriteSeed` and the status writer; tier1 |
| `codeTipDeps` closures and `CodeTipReading` literals | `codetipsettle_test.go` | Code tip reads and pushes (`fabricengine`) | `real` | The wired deps over a hub with a missing tip; tier1 |
| `commitStatusDeps` closures and `countingHold` | `commitstatus_test.go`, `predrive_`, `driverdeps_`, `run_test.go` | Records commit and push (`fabricengine`), prime lock | `real` | The wired seam over a `hubforge` hub, counting through a wrapper; tier1 |
| `startGuard` closures | `startguard_test.go` | `fabricengine`, `boardengine`, run records | `real` | `hubStartGuard` over real state, as `beamland_integration_test.go`; tier1 |
| `reconcile`, `parentOwner`, `landingHeld`, `catchUp`, `startPusher` and `readopt.Gone` closures | `cli_`, `continue_pass_`, `owner_`, `step_`, `run_test.go`, `readopt_integration_test.go` | `fabricengine`, `boardengine`, `dobbyengine`, `runpusher` | `real` | The wired closure, with a recording wrapper where only the call matters; tier1 |
| `dobbyLive`, `pairStart`, `passSpawn`, `clientAttached`, `reedengine.StatusResult` literals | `cli_test.go`, `continue_pass_test.go`, `wire_test.go` | Strand and client liveness (`reedengine`, `dobbyengine`) | `goes` | Needs tmux; move to the owner or a tier2 case |
| `Command.Run`, `Setting` and `ExecutableSetting` closures | `reopen_window_`, `continue_pass_`, `run_test.go` | Command runner (`shedadapters`) and task settings | `real` | The real runner with a harmless command and a written setting; tier1 |
| Literal `board.json` texts (`writeHubBoard`) and `boardkit.WriteBoard` entries | `cli_`, `beamfork_`, `dobbyreading_`, `commanddeps_test.go`, `leftoverpairs_`, `dobbyruns_integration_test.go` | Board store (`boardengine`) | `real` | `boardengine.Board` writes; `boardkit.WriteBoard` must stop framing the file itself |
| `shedengine.Status` values written with `state.WriteJSON` or `json.Marshal`, and literal status and seed texts | `run_test.go` (`writeStatus`), `goto_`, `overview_`, `cancelmarker_`, `land_`, `dobbynotice_test.go`, `cancel_`, `catchup_`, `dobbyruns_integration_test.go` | Run status (`shedengine`) and seed (`shedrun`) | `real` | `shedengine`'s status writer and `shedrun.WriteSeed` |
| Shuttle `run.json`, `wait.yaml`, verify marker, child status texts | `wire_seams_test.go`, `dobbynotice_test.go`, `parentlock_test.go` | `shuttleengine`, `verifytree`, `shedengine` | `real` | `shuttleengine.WriteManifest` and each owner's writer; export one where none exists |
| Hand-written turn-end event lines | `wire_seams_test.go` | LLM session hook events | `external` | Cut from `claudeengine`'s recorded corpus |
| `battenshed` memory record literals | `dobbynotice_test.go` | Inner-run memory (`battenshed`) | `real` | Produce it by driving `battenshed.NewInnerRun`, or move the cap reading there |
| Edited config templates and hand-written `shed`, `profiles`, autonomy, claude account and fabric YAML | `wire_seams_`, `childdriverkind_`, `runsettings_`, `startguard_test.go`, `lifecycle_`, `noticestamp_`, `accountpick_`, `readopt_integration_test.go` | Config of the owning modules | `real` | The owner's config writer; editing a template's lines is editing its bytes |
| Marshalled `fabricengine.PendingTeardown` | `cancel_integration_test.go` | Teardown record (`fabricengine`) | `real` | Leave it by ending a real teardown child early |
| `dobbyengine.View` and `statuspane.StatusPoll` literals | `overviewdobby_test.go`, `overview_test.go` | Dobby view and status poll | `real` | Build the view from real state through `dobbyengine`; see Hard cases |
| `shedengine.Status` and `OriginReport` literals passed to pure readings | `land_`, `cancel_`, `wire_`, `wire_seams_test.go` | `shedengine`, `fabricengine` | `real` | Values the owner produced; see Hard cases |
| `lymcwd.Location` literals and `locationkit.Location` | most tier0 files | Resolved location (`lymcwd`) | `real` | `lymcwd.Resolve`; tier1 |
| `pickAccount` recorder, `inProcessUntilStop`, `runVerdict` closures | `accountpick_integration_test.go`, `run_`, `cli_`, `pausehold_test.go` | `battencli`'s own seams | `own` | Stays; see Hard cases for `runVerdict` |
| `pairPushCounter`, the foreground `spawnPairPush`, `failedStopRecorder`, `madeRecorder` | `testmain_`, `readopt_`, `lifecycle_integration_test.go` | Counting seams | `own` | Stays where it passes calls on; `failedStopRecorder` swallows them and follows `fakeReadopt` |

## battenshed

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
|---|---|---|---|---|
| `newInnerRunDeps`, `waitChild`, `{}` child status, `statusResult` scripts, `pause` flag | `innerrun*_test.go`, `stuck_`, `stophold_test.go` | Child run status and spawn (`shedengine`, `shedrun`, `loomcli`) | `real` | A real child status through `shedengine`'s writer; spawn and liveness cases follow the next row |
| `noticeHarness`, `restepHarness`, `respawnHarness`, `lockHarness` seams for liveness, strands, arbiter, tackler, agents | `notice_`, `innerrun_halted_`, `innerrun_respawncap_`, `innerrun_parentlock_test.go` | `reedengine`, `shuttleengine`, `parentlock`, `battencli` closures | `goes` | Agent answers need tmux and a session; keep rows a real `parentlock` or status can drive |
| `pauserecord.Coverage`, `OwnerVerdict` and decision values returned by seams | `notice_`, `innerrun_owner_`, `innerrun_parentlock_`, `innerrun_wait_test.go` | `pauserecord`, `boardengine` via `battencli` | `real` | `pauserecord.Update` and a real board |
| `landFixture` | `land_test.go` | Git reads, pull, deploy, board prune (`fabricengine`, `boardengine`) | `real` | Wired `LandDeps` over a hub; tier1, and the cases move to `battencli` if `battenshed` cannot wire them |
| `teardownCallRecorder`, `recordingPrimeLock` | `teardown_test.go` | Shutdown, push, level, removal (`pairteardown`, `fabricengine`) | `real` | Real removal in a recording wrapper; tier1, shutdown tier2 |
| `seedChildCalls`, `newSeedChildDeps`, `withNoRunSettings` | `seamchild_test.go`, `stuck_test.go` | Board read, seed write, records commit (`boardengine`, `shedrun`, `fabricengine`) | `real` | Wired `SeedChildDeps` over a hub; tier1 |
| Hand-built `shedengine.Status` seed with history | `innerrun_awaiting_test.go` | Run status (`shedengine`) | `real` | Let the real `shedengine.Shed` reach that history |
| `fakeClock` time side, `producerFunc`, `passDriver` | `innerrun_test.go` | Clock and test adapters | `own` | Stays; `fakeClock`'s child status file and pause flag follow the first row |

## battenrecipe

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
|---|---|---|---|---|
| `testEnv` over `envkit.FullEnv`'s no-op and nil seams | `fixture_test.go` | Every module that fills `shedrecipe.Env` | `goes` | A kit holds no stand-in; wiring checks move to `battencli`, which wires the real `Env` |
| `shedengine.Status` and `HistoryEntry` literals | `land_test.go` | Run history (`shedengine`) | `real` | Histories a real `Shed` run records; see Hard cases |

## runpusher

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
|---|---|---|---|---|
| `runFake` | `run_test.go` | Ahead, push and refusal reads (`fabricengine`), pause cover (`pauserecord`) | `real` | `NewDeps` over a hub, as `newdeps_integration_test.go`; tier1 |
| `startFake` reads of fabric, pause and hand-over state | `start_test.go` | `fabricengine`, `pauserecord` | `real` | `newStarter` over real state; tier1, with `spawn` kept as a recorder |
| Marshalled `pauserecord.Record` | `seams_test.go` | Pause record (`pauserecord`) | `real` | `pauserecord.Update` |
| `lymcwd.Location` literals | `start_`, `seams_test.go` | Resolved location (`lymcwd`) | `real` | `lymcwd.Resolve`; tier1 |

## runproc

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
|---|---|---|---|---|
| `world`: a scripted `proc.StatEntry` table and signal reactions | `end_test.go` | OS process table, reached through `proc` | `real` | Real `proc` over spawned processes, tier1; see Hard cases |

## proc

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
|---|---|---|---|---|
| Kernel journal OOM lines and `memory.events` texts | `processscope_parse_linux_test.go`, `memoryevents_linux_test.go` | Linux output | `external` | Recordings with kernel version and source; contract test where a scope can be made |
| Hand-written cgroup limit and task-limit files | `memorylimit_test.go`, `tasklimit_test.go` | cgroup filesystem | `external` | Recorded file contents |
| Scripted unit states and a stand-in starter script | `processscope_state_linux_test.go`, `memorycap_linux_test.go` | systemd | `external` | Recorded states; `proc` stays the single home |

## cleanupcli

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
|---|---|---|---|---|
| `boardengine.Task` literals and a scripted board error | `scratch_test.go` | Board read (`boardengine`) | `real` | A real board's read; the error provoked through its store |
| Literal run `status.json` texts and marshalled `shedengine.Status` | `cleanup_integration_test.go` | Run status (`shedengine`) | `real` | `shedengine`'s status writer; already tier1 |
| `locationkit.Location` | `cli_test.go` | Resolved location (`lymcwd`) | `real` | `lymcwd.Resolve`; tier1 |
| `fake` segments passed to `runSegments` | `cli_test.go` | `cleanupcli`'s own `segment` type | `own` | Stays |

## machinecli

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
|---|---|---|---|---|
| `fakeHubs` with `boardengine` results | `set_test.go`, `forget_test.go` | Hub registry, board pull and owner writes (`hubregistry`, `fabricengine`, `boardengine`) | `real` | Production deps over two hubs, as `sweep_integration_test.go`; tier1 |
| `newStartDeps`, `hubFixture` for hubs, owners, holds and identity | `start_test.go` | `hubregistry`, `boardengine`, `machinename`, `dobbyengine` | `real` | Real registry and boards over `hubforge` hubs; tier1 |
| `beamStart`, `startDeps`, `terminal`, `openWorkspace`, `desktop` | `start_test.go` | Dobby start (`dobbyengine`, `dobbycli`), terminal, editor, desktop | `goes` | Dobby outcomes need tmux and a session; move to the owner; OS launchers need one home |
| Literal `lym beam start` envelopes | `start_test.go` | `dobbycli`'s output | `real` | Output of the real verb, or move the parser cases beside the writer |
| Literal machine file texts, `locate` and `hostName` seams | `cli_`, `set_`, `forget_test.go` | Machine file (`machinename`) and OS hostname | `real` | `machinename`'s writer in a temp config directory; hostname stays as input data |

## hibernatecli

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
|---|---|---|---|---|
| `fabricengine.OriginReport` literals | `reconcile_test.go` | Origin reconcile report (`fabricengine`) | `real` | Reports of a real reconcile over a hub; tier1 |
| Hand-written `hibernate.yaml` | `cli_test.go` | Config (`hibernateconfig`) | `real` | `hibernateconfig`'s template or writer; unparsable text moves to its parser |

## hibernateengine

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
|---|---|---|---|---|
| `fakeWorld` pause, status and session readings (`tenderengine` literals), dobby liveness, hibernate, notify | `world_test.go`, `watch_`, `verbs_`, `pauseconfirm_test.go` | `tenderengine`, `dobbyengine`, mail | `goes` | Sessions need tmux and an LLM; keep what a tier2 run over the session stand-in reaches |
| `fakeWorld` run statuses, land lock and wake reconcile | `world_test.go` | `shedengine`, `parentlock`, `fabricengine` via `hibernatecli` | `real` | Real status, lock and reconcile over a hub; tier1, possibly moved to `hibernatecli` |
| `fakeWorld` `LookPath`, `Alive`, `GOOS`, self PID | `world_test.go` | OS and systemd | `external` | One home for these OS answers |
| `fakeWorld` request file, clocks and sleeps; real `tenderengine.Lift` wrapper | `world_test.go` | Own record, clock | `own` | Stays |

## testkit/locationkit

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
|---|---|---|---|---|
| `Location` builds a `lymcwd.Location` without resolving | `locationkit.go` and its importers | Resolved location (`lymcwd`) | `real` | `lymcwd.Resolve`, or a `lymcwd` constructor that derives the fields; see Hard cases |

## Packages with none

`fabricvocab`, `gitkit`, `hubregistry`, `standalonestate`, `machinename`, `runfsm`, `state` and `hibernateconfig`
(which has no test files).
`gitkit.SeedConfig` writes bytes its caller hands it, so a caller outside the config's owner makes the stand-in.

## Hard cases

A `lymcwd.Location` literal is a result struct of another module, but path arithmetic reads it only as data,
and resolving it for real moves a large body of tier0 tests to tier1.

Pure functions over another module's value type (`battenrecipe.LandOutcomeOf`, `fabriccli`'s remove fields,
`battencli`'s overview lines) are fed literals, and the rule does not say whether a value type is a "record".

A corrupt or legacy file of another module provokes an error through the state it reads,
yet is hand-written, and the writer cannot produce it.

Anchor markers are named and read by `lymcwd` but written by `fabricengine`, and the leaf `lymcwd` cannot drive fabric.

`runproc.world` fakes `proc`, a repository module, where the outside thing is the OS;
real processes cannot reliably produce the zombie, unkillable and signal-failure rows.

A `battencli` seam such as `runVerdict` is filled by `battencli` itself,
but its answer is computed from `runfsm`, pause and status reads, so the own-seam exception reads both ways.

Placeholder bytes at another module's path, never parsed (lock files, `reviews/record.json`),
only need to exist or conflict, and are not counted above.
