# Stand-ins: creel, orch, slack, dobby and board packages

Inventory against [PATTERN-test-stand-ins](../../../pattern/PATTERN-test-stand-ins.md).
File names are relative to the section's package, and tiers are `tiergate.Require` tiers.
It is read from the test sources; no test was run, so no replacement is proven to work.

## `testkit/creelfake`

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
|---|---|---|---|---|
| `Delivery`, `LiveStrand`, `HookStrand`, `DormantStrand` | `creelengine` tests; `creelcli/cli_test.go`; `orchcli/pushnotifier_integration_test.go`; `dobbyengine/landnotice_creel_test.go` | Strands, idle, pause, send and interrupt answers (`creelcli` `hubDelivery` over `reedengine` and `shuttleengine`) | `real` | `hubDelivery` over real reed and shuttle with one recorded pane stand-in below them, else tier2. `creelengine` rows move to an external test package or to `creelcli`. |
| `Hubs`, with its hand-written refusal texts | `creelengine` `crosshub_test.go`, `outbound_test.go`, `server_test.go`, `verbs_test.go` | The machine's other hubs (`creelcli` `hubDirectory`) | `real` | `hubDirectory` over a temp registry dir and `hubforge` hubs; tier1. |
| `Bridge` | `creelengine` `machinemail_test.go`, `verbs_test.go`, `deliver_test.go` | Machine names, send and relay refusals (`creelcli` `slackBridge`) | `real` | `slackBridge` over files written by `slackengine` writers, as `readySlackBridge` does; stays tier0. |

`creelfake.Clock` is a clock, so it is not a stand-in.

## `creelcli`

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
|---|---|---|---|---|
| `stubOps`, `newStubbedDelivery` | `delivery_test.go` | A worktree's status, idle probe, send and interrupt (`reedengine`, `shuttleengine`) | `real` | The real wiring over a recorded pane stand-in, else tier2. |
| `recordingCommands` | `delivery_test.go` | Session commands (`shuttleengine.Runner`) | `real` | The real runner over a recorded pane, asserting what reached the pane. |
| `restoringPane`, `restoringEngine`, `shuttlefake.Engine` and `Reed` | `delivery_test.go` | Engine pane reading and keys (`claudeengine`), reed pane calls (`reedengine`), a hand-drawn Claude screen | `real` | `claudeengine` and reed over recorded captures of an interrupt. |
| `rewakeRenderer`, `queueingRenderer` | `hookread_test.go` | Hook output and idle wake (`claudeengine`, `codexengine`) | `real` | The real engines; a failed wake needs a recorded failure or tier1. |
| `fmt.Errorf` wraps of `shuttleengine.Err*` and `reedengine.ErrNoSession` | `delivery_test.go` | Errors of shuttle and reed | `real` | Provoke each through the pane or reed state; unprovokable rows of the error mapping go. |
| Journal lines written into `JournalDir`, including a malformed one | `cli_test.go` | `creelengine`'s journal | `real` | Write them through `creelengine.Store` or the daemon; the malformed-line case goes to `creelengine`. |
| `writeBoardCreelConfig` YAML | `hubdirectory_test.go` | `creel.yaml` (`creelconfig`) | `real` | `configengine.Set` over `creelconfig`'s template. |
| `{not json` hub record | `hubdirectory_test.go`, `cli_test.go` | `hubregistry`'s record | `goes` | The corrupt-record case belongs in `hubregistry`; keep only a failure provoked without writing its format. |
| `slackengine.Geometry` literals, `geometry` and `names` closures of `slackBridge` | `slackbridge_test.go`, `cli_test.go` (`readySlackBridge`) | `hubgeom.SlackGeometry`, `machinename.Names` | `real` | Both over a temp hub and config home. The files they point at already come from real writers. |
| Hook payload constants (`stopInput`, `postToolUseInput` and siblings) | `cli_test.go` | Claude Code's hook stdin | `external` | Recorded payloads with source lines, in one home beside `claudeengine`, plus a tier3 recorder. |
| `fakeProviderScript`, `scriptProviderEngine` | `smoke_tmux_test.go` | An LLM session's pane | `external` | A recorded screen played by the one LLM stand-in kit; hand-written today. Stays tier2. |

## `creelengine`

The `creelfake` rows above are this package's stand-ins.

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
|---|---|---|---|---|
| `hubFake`, `ringingFake` | `crosshub_test.go`, `server_test.go` | A daemon speaking `creelengine`'s own wire protocol | `own` | Stays. |
| Hand-written journal lines and `record` values | `journal_test.go`, `server_test.go`, `verbs_test.go`, `deliver_test.go`, `outbound_test.go` | Its own journal | `own` | Stays. |
| `exitsAtOnce` and spawn functions returning canned `Spawn` values | `outbound_test.go`, `crosshub_test.go` | A started daemon process (operating system) | `external` | Needs one home and a recorded exit; or a real short-lived process at tier1. |

## `orchcli`

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
|---|---|---|---|---|
| `fakeCreel`, `scriptedCreel` | `fakecreel_test.go`, `start_test.go`, `cli_test.go` and users of `newTestCLI` | Creel's mail, hold and withdraw behaviour (`creelengine`) | `real` | `creelSeam` over an in-process daemon; its delivery needs the pane stand-in. |
| `fakeCreelClient`, hand-built `creelengine.LogEntry`, `Message`, `WithdrawResult` | `fakecreel_test.go`, `creelseam_test.go` | The creel client and journal (`creelengine`) | `real` | A real client against a daemon with a fake clock; journal folds read what the daemon logged. |
| `fakePoster` and its refusals | `notify_test.go` | Creel posting (`creelengine`) | `real` | Real strand and program clients; may need tier1 for a hub. |
| `fakeStrands` | `cli_test.go`, `start_test.go`, `remotecontrol_test.go`, others using `newTestCLI` | Strand list, add, replace, remove (`reedengine`) | `real` | Real reed over the one tmux stand-in, else tier2. |
| `fakeStarter`, `starterFunc`, `startedSession` answers, `noTranscriptErr`, hand-built `SessionHeldError` | `start_test.go` | Starting and resuming a session (`shuttleengine`, `claudeengine`) | `real` | Real shuttle and `claudeengine` over recorded sessions; resume errors from real transcript state. |
| `shuttlefake.Engine` | `cli_test.go` | An engine without a capability (`shuttleengine.Engine`) | `goes` | No real engine lacks it here; the refusal case moves to `shuttleengine` or goes. |
| `orchengine.Paths` and `Config` literals in `newTestCLI` | `cli_test.go` | `orchengine`'s paths and config | `real` | `orchPaths` and the real config loader over a temp location. |
| Closures `reconcile`, `claimPrime`, `isBeam`, `endOwned`, `pauseOwned` results, `reedUp`, `currentBuild`, `machineName`, `injectRemoteControl`, the dobby and relay start seams | `start_test.go`, `startdobbies_test.go`, `remotecontrol_test.go` | `fabricengine`, `reedengine`, `statuspane`, `machinename`, `creelengine`, `dobbyengine`, `slackengine` | `real` | Real wiring over a `hubforge` hub; tier1. Failure rows that cannot be provoked go. |
| `changeStore`, `resolve`, `beamLive` closures | `rules_test.go`, `startdobbies_test.go`, `notify_integration_test.go` | `heddleengine.Store`, `lymcwd`, a beam orch's liveness (`reedengine`) | `real` | The real store, resolver and liveness; tier1, `beamLive` tier2. |
| `notifyPrime` orch lookup and `postNotice` recorded-orch closures | `notify_test.go` | The recorded orch strand (`orchroute`) | `real` | `orchroute` over a state written by `orchengine.SaveState`; tier1. |
| Hand-written `beam.yaml`, `orch.yaml`, agents file, edited `shuttle.yaml` template, board shortname file | `notifyhalt_integration_test.go`, `refusal_integration_test.go`, `agents_integration_test.go`, `denypython_integration_test.go`, `start_test.go` | Config of `dobbyengine`, `orchengine`, the agents config, `shuttleengine`; `fabricengine`'s shortname | `real` | `fabricengine.WriteShortname`, and `configengine.Set` over each owner's template. |
| `boardkit.WriteBoard` entries | `runstitle_integration_test.go` | `board.json` (`boardengine`) | `real` | `boardengine.New(cfg).UpsertTask`, as `underadopt_integration_test.go` does. |
| `not json` in `board.json` and the pause record | `underadopt_integration_test.go`, `cli_test.go` | `boardengine` and `pauserecord` files | `goes` | Malformed-input cases belong in those packages. |
| Shell script written as `tmux` | `remotecontrol_test.go` | tmux output | `external` | A recorded `list-clients` answer in the one tmux kit. |

`SaveState`, `pauserecord.Update` and `stencilkit.Seed` go through real writers and are not stand-ins.

## `orchengine`

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
|---|---|---|---|---|
| `fakeSession`, `flakySession`, hand-built `Event`, `SkillLoadReport`, `CompactionBoundary`, `UsageLimit`, wrapped `shuttleengine.Err*` | `watcher*_test.go`, `watchloop_test.go`, `restart_test.go`, `reinterpret_test.go` | Session readings (`orchcli` `runnerSession` over `shuttleengine` and `claudeengine`) | `real` | The real session over recorded events and transcripts; external test package, or the cases move to `orchcli`. |
| `fakeCreel`, `fakePost`, `fakeInject` | `fakecreel_test.go` and the watcher tests | Creel behaviour (`orchcli` `creelSeam` over `creelengine`) | `real` | `creelSeam` over an in-process daemon, from `orchengine_test` or `orchcli`. |
| `OrchRestart` closures | `restart_test.go` | The resume and fresh launch (`orchcli`) | `goes` | Restart outcomes are tested in `orchcli`; the watcher keeps only what real wiring can drive. |
| Hand-marshalled `pauserecord.NetOp` file | `watcher_pause_test.go` | A network operation marker (`pauserecord` and its writer) | `real` | The production marker writer. |
| `not json` pause record, a file where the marker dir belongs | `watcher_pause_test.go` | `pauserecord` state | `goes` | Move the unreadable-record behaviour to `pauserecord`. |
| `PauseWatch.Alive` closure | `watcher_test.go` | Process liveness (operating system) | `external` | One home and a real process check at tier1, or stays as the single OS stand-in. |
| Overwritten stencil files | `prompt_test.go`, `watcher_test.go`, `watcher_compact_test.go` | A hub's edited stencil, which `orchengine` renders | `own` | Stays; see Hard cases. |

## `orchroute`

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
|---|---|---|---|---|
| `strands` closures with `reedengine.StrandStatus` literals and a wrapped `ErrNoSession` | `live_test.go` | Reed's strand list (`reedengine`) | `real` | Real reed state and session; tier2. |
| `beamLive` closure | `asker_integration_test.go` | A beam orch's liveness (the caller's reed read) | `real` | Real liveness over reed; tier2. |

## `slackcli`

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
|---|---|---|---|---|
| `fakeSlack` HTTP server with canned bodies | `cli_test.go` and users of `newHarness` | Slack Web API | `external` | Single home with `slackengine`'s in a kit, recorded answers and failures, a contract test. |
| `fakeLoginSocket`, `newDialer` closure and its hello app id | `cli_test.go` | Slack Socket Mode | `external` | The same Slack kit, below `slackengine`'s dialer, with a recorded hello. |
| `fakeStrandClient`, hand-built `creelengine.LogEntry` | `cli_test.go`, `hook_test.go` | The creel client and journal (`creelengine`) | `real` | A daemon in process and its real log. |
| `paneCaller` with `creelengine.Strand` literals | `cli_test.go` | Creel's strand list (`creelengine`, `reedengine`) | `real` | Strands from real delivery over the pane stand-in. |
| `slackengine.Geometry` literal | `cli_test.go` | `hubgeom.SlackGeometry` | `real` | The real geometry over a temp hub. |
| `ensure`, `stop`, `awaitFirstRead`, `awaitReparent`, `spawnIntermediate`, `runRelay` closures | `cli_test.go` | The relay lifecycle (`slackengine`, `runproc`) | `real` | Real starter and stop over a process stand-in; tier1. Verb cases that need only a failure go. |
| `worktreeOrch`, `orchAutonomous`, `paused`, `strandName`, `resolveLocation`, `showToggle` closures | `cli_test.go`, `toggle_test.go` | `orchroute`, `beamsetting` config, `pauserecord`, `agentname`, `lymcwd`, `reedengine` | `real` | Real readers over temp state; a location needs tier1, the redraw tier2. |
| `readHiddenLine` closure | `cli_test.go` | The operator's terminal | `external` | One home; stays. |
| `hookJSON`, `promptInput`, `stopInput` | `hook_test.go` | Claude Code's hook stdin | `external` | Recorded payloads shared with `creelcli`'s. |
| `{` as `board.json`, `not json` hub record | `wiring_test.go`, `wiring_geometry_test.go` | `boardengine`, `hubregistry` files | `goes` | Malformed-input cases move to the owners. |
| Bogus presence file | `toggle_test.go` | `slackengine`'s presence file | `goes` | Move to `slackengine`; `slackcli` cannot write it through the owner. |

`fixedClock` is a clock, and the credential, setup, presence and forwarding files come from `slackengine` writers.

## `slackengine`

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
|---|---|---|---|---|
| `fakeSlack` in-memory `SlackClient`, `fakeBot`, hand-built `APIError` and `RateLimitError` | `inbound_test.go` and the outbound, keeper, bridge and relay tests | Slack | `external` | Move below `API`: recorded answers and failures served to the real client, shared kit, contract test. |
| `fakeSlackAPI`, `documentedAnswers`, `socketServer` frames | `api_test.go`, `socket_test.go` | Slack Web API and Socket Mode | `external` | `documentedAnswers` are documentation examples, not recordings; record them and add a contract test. |
| `fakeDialer`, `fakeSocket`, hand-built `Envelope` | `holder_test.go`, `relay_test.go` | Slack Socket Mode | `external` | Recorded envelopes through the real socket, in the same kit. |
| `fakeMailbox`, `readStep`, hand-built `creelengine.Message`, `MachineOutbound`, wrapped creel errors | `inbound_test.go` and the outbound, bridge and relay tests | Creel's mail and machine queue (`creelengine`, through `slackcli`) | `real` | A real creel daemon or `Store`; refusals provoked through its state. |
| `fakeHub` (`slackengine.Hub` closures) | `inbound_test.go` and users of `harness` | Beams, pauses, projects, machine names (`slackcli` over `boardengine`, `pauserecord`, `machinename`) | `real` | `slackcli`'s hub wiring over a temp hub, board and pause record; tier1 where beams need git. |
| `fakeLive`, `keeperHarness.liveHubs` | `holder_test.go`, `keeper_test.go` | The live hubs (`slackcli` `liveHubGeometries`) | `real` | `liveHubGeometries` over a temp machine registry; `slackcli`-side cases if import forbids. |
| `fakeSpawn` | `relay_test.go` | The spawned relay (`slackcli`, `runproc`) | `real` | A real spawn of a relay stand-in process; tier1. |
| `{` in channel map and forwarding files, hand-built `Geometry` | `keeper_test.go`, `inbound_test.go` | Its own files and type | `own` | Stays. |
| `hintWriter` | `holder_test.go` | Wraps the real `WriteHint` | `own` | Stays; it blocks or fails around the real call. |

## `dobbyengine`

`beamcli`, `machinecli`, `battencli` and `hubgeom` fill the dependency structs in production.

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
|---|---|---|---|---|
| `tidyShuttleFake`, `startHookShuttle` | `tidy_test.go`, `describe_test.go`, loop tests | `AgentShuttle` (`shuttleengine`) | `real` | Real shuttle over the LLM session stand-in. |
| `passFake`, `RunView`, `TaskView`, `PassExit` builders such as `runningRun` | `passes_test.go`, `handover_test.go`, `loop_catchup_test.go` | Runs, board, owners, pauses, pass processes (`beamcli` over `boardengine`, `fabricengine`, shed state) | `goes` | Move to `beamcli` integration tests over a real hub; tier1. |
| `catchUpFake`, `fakeStart`, `teardownFake`, `landHarness` closures, `BeamDescribeDeps` closures, `ClearDoneRun`, `Orphans`, `SyncBoltHold` | `catchup_test.go`, `start_test.go`, `leftover_test.go`, `land_test.go`, `describe_test.go`, `donerun_test.go` | Git, Bolt, landing and beam operations (`beamcli`, `fabricengine`) | `goes` | Same move; these seams are git all the way down, tier1. |
| `noticeFake`, `reminderFake` | `runnotice_test.go`, `waitreminder_test.go` | Child status, agent runs, pause coverage, creel posts (`battencli`, `shuttleengine`, `pauserecord`, `creelengine`) | `real` | Real readers over state their writers wrote; posts into a creel `Store`. |
| `Notice`, `NearestOrch`, `LandNotice` closures | `landnotice_test.go`, `describe_test.go`, `handover_test.go` | Creel posting and routing (`creelengine`, `orchroute`) | `real` | As `landnotice_creel_test.go` does, over a real `Store`. |
| `creelengine.Geometry` literal | `landnotice_creel_test.go` | `hubgeom.CreelGeometry` | `real` | The real geometry over a temp hub. |
| Hand-written `board.json`, `boardengine.Owners` and `Registry` literals | `launcher_test.go`, `reinterpret_test.go` | `boardengine`'s file and results | `real` | Seed through `boardengine`; retired-key carry cases are a Hard case. |
| `boardengine.RegistrationError` literal | `loop_catchup_test.go` | `boardengine`'s refusal | `real` | Provoke it with an unresolvable config home. |
| `anchor-move.json` holding `{}` | `loop_move_test.go`, `loop_test.go`, `waitreminder_test.go` | An anchor move record (its writer) | `real` | Write it with the move's own writer. |
| `.git` file `gitdir: elsewhere` | `describe_test.go`, `loop_test.go`, `loop_worktreegone_test.go`, `handover_test.go`, `reinterpret_test.go` | A git worktree | `external` | A real `git worktree` at tier1, or one recorded fixture in a kit. |
| Report and draft file contents | `tidy_test.go`, `describe_test.go` | An LLM agent's output files | `external` | Recorded agent output in the LLM stand-in kit. |

`loopClock` is a clock; `state` files and `beam.yaml` written here are the package's own.

## `boardcli`

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
|---|---|---|---|---|
| `fakeInbox`, `fakeIssue`, `issueJSON` | `intake_test.go` | GitHub issues API | `external` | One home beside `hubissues`, recorded answers and failures, a contract test. |
| `openDeps` `Probe` and `Prime` closures | `open_test.go` | `boardserve.Probe`, `fabricengine.PrimeName` | `real` | A real in-process server to probe; `Prime` needs tier1. |
| `openDeps` `Running`, `StartOpener` closures | `open_test.go` | Process liveness and the desktop opener | `external` | One home; stays. |
| `openDeps` `Spawn` closure | `open_test.go` | `boardcli`'s own spawn, with `onSpawn` playing the `boardserve` server | `real` | Start a real in-process `boardserve.Run`. |
| Hand-marshalled `boardserve.State` file | `open_test.go` | The server's state file (`boardserve`) | `real` | Let a real `boardserve.Run` write it, or use its writer if exported. |
| `board.json`, `board.yaml`, `fabric.yaml` and raddle store files written by hand | `cli_test.go`, `verbs_test.go`, `intake_test.go` | `boardengine` and `fabricengine` files | `real` | Seed through board verbs, `ConfigTemplate` and the raddle filing path. |
| Stub `xdg-open` script | `serve_linux_integration_test.go` | The desktop opener | `external` | Stays; it records the call, one home. |

## `boardengine/boardtest` and `testkit/boardkit`

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
|---|---|---|---|---|
| `boardkit.WriteBoard` | `kit` users across `orchcli`, `battencli`, `beamcli`, `fabricengine`, `loomcli` and others | `board.json` (`boardengine`) | `real` | The kit writes through `boardengine`'s store; ids and kept keys then come from it. |
| `seedwiki` `board.yaml` and marshalled `board.json` | `boardtest/seedwiki_helper_test.go` and its benchmarks | `boardengine` config and file | `real` | `ConfigTemplate` and `boardengine` upserts; stays tier as is. |

`boardkit.HubWithBoardConfig` uses `boardengine.ConfigTemplate` and `InstallReplay` registers the real replayer.

## `boardserve`

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
|---|---|---|---|---|
| `Register` closure | `run_test.go` | Process registration (`boardcli` over `runproc`) | `real` | A wrapper counting calls to `runproc.Register` over a temp dir. |
| `Replaced` closure | `run_test.go` | The binary replaced on disk (`selfexec`, operating system) | `external` | One home; stays. |
| `fakeClock`, `fakeWaiter` | `run_test.go` | A clock | `own` | Stays. |

The board under test is a real `boardengine.Board`.

## `hubissues`, `selfreportcli`, `selfreportengine`

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
|---|---|---|---|---|
| `httptest` issue servers with hand-written bodies and statuses | `hubissues_test.go`, `selfreportcli/cli_test.go`, `selfreportengine/selfreport_test.go` | GitHub API | `external` | One GitHub kit shared with `boardcli`, recorded answers and failures, a contract test. |
| `failingTransport` returning `githubclient.ErrTokenUnresolvable` | `hubissues_test.go` | `githubclient`'s transport | `real` | The real transport with no resolvable token; may need tier1 for `gh`. |
| `NewGitHubClient` factory returning an error | `selfreportcli/cli_test.go`, `selfreportengine/selfreport_test.go` | `githubclient`'s resolution failure | `real` | The same real failure. |

`installGitHubClient` builds a real `go-github` client pointed at the server and adds nothing of its own.

## `shortcutcli`

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
|---|---|---|---|---|
| `fakeRootFactory`, `fakeScript`, `fakeRecorder` | `shortcut_test.go` | The `lym` command tree and its envelopes (`cmd/lym`) | `goes` | `cmd/lym` is `main` and cannot be imported; cases against real verbs move there. |

## `fronttask`

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
|---|---|---|---|---|
| `fakeBoard` | `fronttask_test.go` | The hub board and the batten cancel (`boardengine`, `battencli`) | `real` | A real `Board` with `SkipGit`, as `hub_integration_test.go` uses; cancel failures go. |
| `standInBinary` and process starter closures | `hub_test.go` | Starting a process (operating system) | `external` | One home; stays. |

## Packages with none

- `agentname`
- `beamsetting` (no tests)
- `creelconfig` (writes only its own config)

## Hard cases

`orchengine.Creel` is filled in production by `orchcli`'s own `creelSeam`.
In `orchcli` tests that reads as a seam the module fills itself, yet the fake plays creel; it is `real` here.

Retired-key `board.json` content, as in `dobbyengine`'s `reinterpret_test.go`, has no current writer.
The rule gives neither a real way to build it nor a clear owner for the carry test in `dobbyengine`.

`machine.json` is read by `machinename` and given ids by `boardengine`, whose tests write it by hand.
Whether it is `boardengine`'s own file depends on who "declares and alone writes" it, and neither does alone.

A stencil overwritten to break a render is text `orchengine` renders and `stencilstore` places.
It is `own` here, but `stencilstore` or the stencil's source package could be called the owner.

Config YAML such as `beam.yaml` and `creel.yaml` has a loader and no writer.
"Written by that module's own writer or loader" cannot be met when a test needs a non-default value.

The `claudeengine.LymBinary` stub path in `TestMain` fills another package's function seam.
It hands a value read as data, like an id source, so it is not listed.
