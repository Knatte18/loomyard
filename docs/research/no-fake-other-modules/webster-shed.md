# Stand-ins: webster, shed, plan and review packages

Judged against [PATTERN-test-stand-ins](../../../pattern/PATTERN-test-stand-ins.md), from `*_test.go`, `testdata/` and kits.
A tier named in the last column is where the replacement pushes the test.

## Shared across packages

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
|---|---|---|---|---|
| `shuttlefake.Engine`, `SignalEngine`, and their `*Fn` scripts (`recordAudit`, `instructionFileEngine`, `noAuditEngines`) | `websterengine`, `webstercli`, `weaverengine`, `gatefixer`, `burlerengine`, `burlercli`, `seatengine` fixtures | The provider engine (`claudeengine`) | real | `claudeengine` over recorded events, screens and transcripts, with the one LLM stand-in below tmux |
| `shuttlefake.Reed` | The same packages | `reedengine` | real | `reedengine` over one shared tmux stand-in; without that stand-in the test is tier2 |
| `shuttleengine.ForkAudit`, `ForkReport`, `WriteEvent` literals, including `shuttlefake.TokencountRunReads` | `websterengine` audit and record tests, `webstercli/reset_test.go`, `burlerengine` | Audit output of `claudeengine` | real | `claudeengine.AuditForks` over recorded transcripts; recordings come from tier3 |
| `shuttleengine.Result`, `GateOutcome`, `EndedShell` literals | Most fixtures that script a shuttle | `shuttleengine` | real | The result a real `shuttleengine.Runner` returns once the shuttle fakes above are gone |
| `shuttleengine.Config`, `modelspec.Registry`, `gateslot.Config` (`testGateConfig`) literals | `websterengine`, `weaverengine`, `webstercli`, `samplerengine` | Those packages' loaded config | real | Each package's own loader over its template, as `shuttlefake.Agents` already does for `agentconfig` |
| `planparser.Plan`, `Card`, `TargetGroup` literals | `websterengine`, `batcher`, `runplan`, `weavercli`, `cardverify` | `planparser` parse results | real | `plankit.Write` then `planparser.ParsePlan`; tier0 |
| `planindex.Finding` and `verifytree.Result` literals | `websterengine`, `weaverengine`, `runplan`, `shedrecipe` | `planglyph`, `verifytree` | real | Provoke the finding or result in the real module; `verifytree` is tier1 |

## websterengine

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
|---|---|---|---|---|
| `fakeGit`, `mergeOnlyGit`, the `newBeginFixture`, `newRecordFixture`, `newRecoverFixture`, `newRunFixture` family over them | `gitfake_test.go` and the tier0 bracket tests | git behind `websterengine.Git` | real | The module's own git wrapper over a `gitkit` repository, as the `*_git_test.go` files do; tier1 |
| `fakeIndex`, `fakeDelta`, `laterReferencesIndex`, `dispatchFindingsIndex`, hand-built `quarry.GitDeltaAnswer` behind them | `gitfake_test.go`, `recordbatch_test.go`, `beginbatch_test.go` | `planglyph` | real | `planglyph.NewIndex` over a real tree; a delta needs git, tier1 |
| `fixedBatcher`, `sizeBatcher`, `beforeBatcher`, `baseBatcher`, `emptyBatcher` | `rebaseline_test.go`, `partition_test.go`, `runlevel_test.go` | `batcher` | real | `batcher.Identity` or a configured batchifier over a plan and sizes that give the wanted grouping |
| `failingBatcher` | `runlevel_test.go` | A `batcher` failure | goes | No real batchifier is made to fail this way; the case leaves with it |
| `linesSizes` | `partition_test.go` | `batcher`'s disk size source | real | The disk size source over files in a temp directory |
| `runFakeStarter`, `runFakeHandle`, `erroringStarter` | `runlevel_helper_test.go`, `recoverbatch_test.go` | `shuttleengine.Runner` | real | A real runner, as `realMasterStarter` does, over the LLM and tmux stand-in |
| `recordingStopper` | `runlevel_helper_test.go` | `shuttleengine.Runner.StopStrand` | real | A recording wrapper that passes each call to the real runner |
| `fabricMatcher`, `everyCommandMatches` | `recordbatch_test.go`, `acceptbatch_test.go` | `fabricengine.RefScanner` | real | `fabricengine.NewRefScanner` over a temp `Location` |
| `gateFake`'s `verify`, `readLog` and `changedPaths` seams | `verifygate_test.go` | `verifytree` results and logs, git | real | `verifytree.Verify` over a scratch repository; tier1 |
| `seedShuttleRunState` writing `run.json` | `runlevel_helper_test.go` | `shuttleengine` run state | real | `shuttleengine.Runner.Start` writes it |
| Hand-written batch report YAML (`writeReport`, `writeRecoverReport`, `validReport`) | Record, recover, begin and run tests | `runplan`'s report format, written by the agent | real | `runplan.WriteReport` |
| `outcome.yaml` and `summary.md` text (`writeContractFiles`) | `runlevel_helper_test.go`, `archivereset_test.go`, `recoverbatch_test.go` | The Master agent's output | external | A recording of each, with its source line and a tier3 contract test |
| `appendClaudeHook` payloads and hand-written transcript lines | `recoverbatch_test.go` | Claude Code hook and transcript output | external | Recorded hooks from `claudeengine`'s corpus, cut and ordered, never written |
| Hand-written card and overview markdown | `recordbatch_helper_test.go`, `rebaseline_test.go`, `recoverbatch_test.go`, `runlevel_test.go` | `planparser`'s plan format | real | `plankit.Write` of the edited plan |
| `gateFake`'s `outcome`, `head`, `recordPreFix`, `commitLog` seams | `verifygate_test.go` | The module's own closures | own | Stays |

## webstercli and weavercli

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
|---|---|---|---|---|
| `verbsFakeMasterStarter`, `verbsDiedMaster` | `webstercli/verbs_test.go` | `shuttleengine.Runner` | real | The fixture's real runner, dying through the LLM and tmux stand-in |
| `sizeProbeBatcher` | `webstercli/verbs_test.go` | `batcher` (it passes through to `Identity`) | own | Stays as a recording wrapper |
| `indexkit.ValidateStub` | `webstercli/cli_test.go` | `planglyph` | real | `planglyph.NewIndex` over a plan whose finding names which index ran |
| `statusFailingGit` | `webstercli/reset_test.go` | `websterengine.Git` failure | real | A corrupted index in a scratch repository; tier1 already |
| `failingInstructionFiles`, `reedUp` and `openFabric` error closures | `webstercli` refusal tests | Errors of `websterengine`, `reedengine`, `fabricengine` | real | Provoke each: an unknown engine in `webster.yaml`, no records sibling; a reed failure is tier2 |
| `seedRunState`, `startedAt`, `withRecoveryStrands` state literals, `state.json` and `status.json` set to `{}` | `refusedverb_helper_test.go`, `reset_test.go`, `sync_integration_test.go` | `websterengine` and `shedengine` records | real | `websterengine` run and begin-batch verbs, and a stepped `shedengine.Shed` |
| `seedHubGateConfig`, `verifyGateConfig`, `fabric.yaml` text | `refusedverb_helper_test.go`, `verify_integration_test.go`, `sync_integration_test.go`, `secretpaths_wiring_test.go` | `gateslot` and `fabricengine` config | real | Each config's own template and writer |
| Report YAML, `outcome.yaml` and fixer reply text | `verbs_test.go`, `verify_parity_integration_test.go`, `fixplan_test.go`, `fixverify_test.go` | Agent output in `runplan` and `websterengine` formats | real | `runplan.WriteReport`; the free agent replies need recordings |
| Undecodable pause record | `webstercli/batchpause_test.go` | A malformed `pauserecord` file | goes | The refusal is tested in `pauserecord` |
| `claudeengine.LymBinary` replaced by `/stub/lym` | `webstercli/testmain_test.go` | The built `lym` binary's path | own | Stays: the path is only passed on |
| `stubGit`, `stubRunner`, no-op `ResetCode`, `Commit` seams, state literal | `weavercli/reset_test.go` | `websterengine.Git`, `shuttleengine`, `fabricengine` | real | A real pair and runner; tier1 |

## weaverengine

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
|---|---|---|---|---|
| `fakeGit` | `run_fixture_test.go` | `websterengine.Git` | real | `websterengine`'s git wrapper over a scratch repository; tier1 |
| `fakeIndex`, `fakeDelta`, including its own drift rewrite | `run_fixture_test.go` | `planglyph` | real | `planglyph.NewIndex`; tier1 |
| `chunkBatcher` | `run_fixture_test.go` | `batcher` | real | A real batchifier and plan sizes that give that chunking |
| `scriptedRunner` and `attemptScript` (it writes the report, commits, replaces gate closures) | `run_fixture_test.go`, `TestRun` | `shuttleengine` and the implementer agent | real | The real runner over the LLM and tmux stand-in |
| `fakeFixer`, `fixerScript` | `run_fixture_test.go` | `gatefixer` | real | `gatefixer.NewStrandFixer` over the real runner |
| `scriptedVerify` and the `go test` log it writes | `run_fixture_test.go` | `verifytree` | real | `verifytree.Verify` over a scratch module; tier1 |
| `resetHarness` seams: `StateHistory`, `ReadPlanAt`, `ResetCode`, `RecordPending`, `Commit`, `DirtyTracked` | `reset_test.go` | `fabricengine`, git | real | A real pair from `hubforge`; tier1 |

## runplan, batcher, planglyph

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
|---|---|---|---|---|
| `onePerCardBatcher` | `runplan/pending_test.go` | `batcher` | real | `batcher.Identity` |
| `doneCheckFakeIndex`, `bindFakeIndex`, `recordingDelta` | `runplan/postbatch_test.go` | `planglyph` | real | `planglyph.NewIndex` over a tree that fails at that step; the delta is tier1 |
| Hand-written decision record (`writeDroppingRecord`) | `runplan/postbatch_test.go` | `discussionparser`'s record | real | The writer the `lym loom decision add` verb uses |
| `noFabricReference` | `planglyph/index_test.go` | `fabricengine`'s reference rule | real | `fabricengine.NewReferenceRule` |
| Raw card bodies written over `plankit` output (`writePlanFixture`, inline card files) | `planfixture_helper_test.go`, `planglyph_test.go`, `handle_test.go`, `drift_test.go` | `planparser`'s card grammar | real | `plankit.Card` fields; the kit grows the fields it lacks |
| `failingTypesLoader`, `fatalTypesLoader` | `planglyph/testmain_test.go`, `glyphchain_integration_test.go` | `go list` behind `packages.Load` | external | One home already; its failure needs a recorded one, and the typed tests are the contract |

## shedadapters and shedrecipe

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
|---|---|---|---|---|
| `shedfake.Shuttle`, `MergeShuttle` | Bouncer tests, `shedrecipe` entry tests | `shuttleengine.Runner` | real | A real runner over the LLM and tmux stand-in |
| `shedfake.BurlerHalfRunner`, `resumeWritingRunner`, `ReviewResult`, `FixResult` literals | `burlerreview_test.go`, `burlerfix_test.go`, `entries_burlerhalves_test.go` | `burlerengine` | real | `burlerengine` over the real runner |
| `shedfake.SeatRunner`, `seatengine.Result` literals | `multillm_test.go`, `shedrecipe` seat entry tests | `seatengine` | real | `seatengine.Engine` over the real runner |
| `fakeWebsterRunner`, `fakeWebsterRun`, `weaverRunner`, `fakeWeaverRun` | `webster_test.go`, `weaver_test.go`, `shedrecipe/fixture_test.go` | `websterengine.Run`, `weaverengine` run | real | The real run functions; tier1 |
| Review, verdict and ledger text a fake agent writes (`judgeFakeShuttle`, `circlingOpenLedger`, `writeFactsReview`) | Bouncer tests, `shedrecipe` bouncer and harden entries | The reviewer and judge agents | external | Recorded agent outputs, one home, with tier3 contract tests |
| `round-1-*` files and snapshot JSON (`writeFixRound`, `writeJudgedRoundOne`) | `shedrecipe/entries_burlerhalves_test.go` | `shedadapters` records | real | Drive the review, bouncer and fix producers to write them |
| Decision record, amendment record and `## PATTERN changes` text | `roundcontext_test.go`, `bouncer_judge_test.go`, `shedrecipe` | `discussionparser` and the amendment writer | real | The owners' writers |
| `weaver.yaml` text | `shedrecipe/entries_weaver_test.go` | `weaverengine` config | real | `weaverengine`'s template and loader |
| `fakeTranscripts`, `fakePricer`, `assistantMessage` | `sessionusage_test.go` | `claudeengine` reading and pricing | real | `claudeengine` over recorded transcripts and its own price table |
| `carryOverRecorder`, `seamLog`, `circlingSeams`, `Approve` and `Commit` closures | `bouncer_*_test.go` | `discussionparser` carry-over, `fabricengine` commits | real | Recording wrappers over the real writer and a real pair; tier1 |
| `parentFixture` seams: `Post`, `Resolve`, `Wait`, `RecordWrittenAt` | `parentseat_test.go`, `bouncer_parent_test.go` | `creelengine`, config, the record's writer | real | A real creel and settings; delivery to a pane is tier2 |
| `CommandDeps` `Setting`, `ExecutableSetting`, `TaskDir` | `command_test.go` | Settings and path lookups | real | The real loaders over a temp hub |
| `CommandDeps.Run` | `command_test.go` | A shell command on the OS | external | One home for the command boundary, with recorded results |
| `doneProducer` | `webster_test.go` | A `shedengine.ShedProducer`, which many modules fill | own | Stays |
| `noopSamplerSessions`, `validSamplerDeps`, `validPawlDeps`, `validRaddle*`, `shedfake.WebsterSeams`, `WeaverSeams`, `placeholderIndex` | `shedrecipe/fixture_test.go` | Seams of `loomshed`, `pawlengine`, `websterengine`, `weaverengine` | real | The production wiring over a temp hub; tier1 |
| `noFindingsPlanIndex` | `shedrecipe/fixture_test.go` | `planglyph` | real | `planglyph.NewIndex` over a clean plan |
| Drift, batten, merge and harden seam closures (`newTestCampaign`, `campaignRecorder`) | `shedrecipe/entries_*_test.go` | `driftengine`, `battenshed`, `fabricengine`, `hardenshed` wiring | real | The production wiring over a real pair; tier1 |

## shedengine, shedverbs, shedcli

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
|---|---|---|---|---|
| `funcProducer`, `callbackProducer`, `stubProducer`, `outcomeSequence`, `erroringRow` | `shedengine` and `shedverbs` tests | Any `ShedProducer`, which many modules fill | own | Stays |
| `fakeGate` | `shedengine/entry_test.go` | An `EntryGate` that `shedbuild` builds | real | `shedbuild`'s gate over a settings lookup, in an external test package |
| `recordingCommitStatus` | `shedengine/run_commitstatus_test.go` | `fabricengine` status commit | real | A recording wrapper over the real commit; tier1 |
| `shedengine.Status` and `HistoryEntry` written or returned by hand | `shedverbs`, `shedcli`, `samplerengine/record_test.go`, `shedadapters` | `shedengine`'s status | real | Step a real `shedengine.Shed` to that state |
| `fakeChild`, `hangingChild`, scripted trace, stderr and log files | `shedverbs/loop_test.go`, `loopwait_test.go` | The child `lym` process on the OS | external | One home already; stderr and crash text need recordings |
| `childScript` records written through `StepEnvelope`, `waitWorld` spawn | `shedverbs/loop_test.go`, `loopwait_test.go` | The module's own records and seams | own | Stays |
| `Hooks.InterruptPolicyFor` closure | `shedverbs/loop_test.go` | The recipe's interrupt policy | real | The hooks a real recipe builds |
| Invalid model spec in a hand-written config file | `shedcli/cli_test.go`, `parity_test.go` | Another module's config file | goes | The refusal is tested where the model spec is parsed |

## sheddriver and shedarbiter

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
|---|---|---|---|---|
| `agentFake`, `agentFakeRun`, `fakeShuttle`, `fakeRun` | `sheddriver/tackler_test.go`, `shedarbiter/call_test.go` | `shuttleengine` behind `AgentShuttle` | real | `sheddriver.RunnerShuttle` over a real runner |
| Tackler and arbiter verdict YAML | `tackler_test.go`, `call_test.go`, `consume_test.go` | The tackler and arbiter agents | external | Recorded verdicts with tier3 contract tests |
| Hand-built step envelopes (`driveEnvelopeJSON`, `stopEnvelope` strings) | `sheddriver/drive_test.go`, `shedarbiter/call_test.go` | `shedverbs` envelope output | real | `shedverbs.StepEnvelope` over real step results |
| `rawEnvelope` text | `shedarbiter/consume_test.go` | A malformed `loomengine` envelope | goes | The refusal is tested in `loomengine` |
| `ReadStatus` closures over literal statuses | `drive_test.go`, `tackler_test.go`, `call_test.go` | `shedengine` | real | The status file of a real `Shed` |
| `fakeWorktree`, `Snapshot` closure | `call_test.go`, `drive_test.go`, `tackler_test.go` | `shedworktree` | real | `shedworktree.ArbiterSeam` over a hub; tier1 |
| `Stop` readers, `PendingCirclingDecision`, `Closed`, `PausedWithin`, `Pause`, `Notify`, `OnDiskBuild` and the other driver seams | `drive_test.go`, `tackler_test.go`, `call_test.go` | `shedadapters`, `pauserecord`, `creelengine`, `buildvcs` | real | The owners' readers over files they write; notices to a pane are tier2 |

## gatefixer

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
|---|---|---|---|---|
| `memoryStore` | `fixture_test.go` | The `Store` of `websterengine`, `weaverengine`, `gatecli` | real | The module's own row store in a temp directory |
| `recordingStopper` | `fixture_test.go` | `shuttleengine.Runner.StopStrand` | real | A recording wrapper over the real runner |
| `scriptedGate` | `rowgate_test.go` | A gate closure many modules build | own | Stays |
| `fakeFixer` | `rowgate_test.go` | The module's own `Fixer` | own | Stays |
| `recordingStarter` | `fixture_test.go` | Passes to a real runner, bar a set error | own | Stays; the error row provokes the runner instead |

## seatengine, burlerengine, burlercli, samplerengine

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
|---|---|---|---|---|
| `seatengine` `fakeShuttle`, `fakeHandle` | `seatfake_test.go` | `shuttleengine` behind `seatengine.Shuttle` | real | `RunnerShuttle` over a real runner, as `realRunnerShuttle` does |
| `burlerengine` `fakeShuttle`, `fakeHandle`, `halfScript` | `engine_test.go` and its users | `shuttleengine` and the agents | real | The same real runner shuttle |
| `halfShuttle`, `halfHandle` | `burlercli/wiring_test.go` | `shuttleengine` and the agents | real | The same real runner shuttle |
| Review text (`approvedReview`, `blockingReview`, `content` rows) | `burlercli/wiring_test.go`, `cli_test.go` | The reviewer agent, in `burlerengine`'s format | external | Recorded reviews; parser refusals move to `burlerengine` |
| `fixedEngineNamer` | `samplerengine/spec_test.go` | `shuttleengine`'s engine naming | real | The real namer over an engine set |

## hardenshed, mergeresolve, verifytree, cardverify

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
|---|---|---|---|---|
| `fakeProducer` | `hardenshed/round_test.go` | The decorated `ShedProducer`, which many modules fill | own | Stays |
| `rowFixture` seams: `Head`, `Commits`, `Commit` | `hardenshed/round_test.go` | `fabricengine`, git | real | A real pair; tier1 |
| `reapFake` process and stat lists, `EndProcess` | `hardenshed/reap_test.go` | The OS process table | external | One home for the process boundary, with a recorded table |
| `reapFake` `Owners`, `End`, `StopServer` | `hardenshed/reap_test.go` | `runproc`, `reedengine` | real | Real owner records (tier1) and server stop (tier2) |
| `LiveRun` literals with hand-written `go test` lines | `hardenshed/live_test.go`, `shedrecipe/entries_harden_test.go` | `lym gate test` output | real | The real gate run over a small module; tier1 |
| `fakeMergeSurface`, `fabricengine.MergeResult` literals | `mergeresolve/mergeresolve_test.go` | `fabricengine` | real | A real pair with a conflicting parent; tier1 |
| `unrecognizedMergeError` | `mergeresolve/mergeresolve_test.go` | A merge error no module produces | goes | The catch-all row leaves |
| `Pending` and inner gate scripts (`turnStep`) | `verifytree/reach_test.go` | The reach report reader and a gate another module builds | real | The real reader over a written report; the scripted inner gate stays |
| Stub `lym` shell script on `PATH` | `cardverify/runner_test.go` | The `lym` binary | real | `lymbin.PutFirstOnPath`; tier1 |
| `limits` closures | `cardverify/runner_test.go` | `gateslot`'s configured limits | real | `gateslot`'s loader over a written config |

## testkit/shedfake, testkit/indexkit, testkit/plankit

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
|---|---|---|---|---|
| `shedfake`'s fakes and seam placeholders, and its self-tests | `shedfake.go`, `shedfake_test.go` | `shuttleengine`, `burlerengine`, `seatengine`, run seams | goes | The kit keeps `CallOK` and `RequireOutcome` only |
| `indexkit.ValidateStub` | `indexkit.go` | `planglyph` | goes | The package is removed with its users' stub |
| `plankit`'s hand-rendered overview and card grammar | `plankit.go` and its users | `planparser`'s plan format | real | A writer in `planparser` that the kit calls |

## Packages with none

- `planparser`: its plan text, `testdata/goodplan` and `RenumberIO` faults are its own format and seams.
- `planindex`: no test files.
- `shedbuild`: its recipe text is the format it parses.
- `shedrun`: its seed and landed-commit files are its own.
- `shedtransient`
- `shedworktree`
- `hardencli`: the `Deploy` stub replaces the package's own function.
- `verifyrun`
- `testkit/shedcheckkit`

## Hard cases

- Generic closures: `shedengine.ShedProducer` and `shuttleengine.GateEntry` are filled by many modules.
  The rule admits a scripted one in the declaring module's tests;
  whether a consumer's test of the same interface may script it is open.
- `websterengine.Git`: the seam and its production wrapper are both the module's own, so only git itself is faked.
  A computed commit graph can never be a recording, so it is judged `real`, which moves most bracket tests to tier1.
- Agent-written files: the format's declarer parses it, but an agent writes it in production.
  The rule lets a module hand-write files it parses, and also demands recordings of LLM output;
  review, verdict, ledger, report and `outcome.yaml` text fall under both.
- `plankit`: a kit may build fixtures through a module's real code, but `planparser` has no plan writer.
- `quarry.GitDeltaAnswer` literals in `planglyph`: `quarry` is a library of another repository,
  neither a module here nor on the rule's list of outside things.
- Constructor checks: panic-on-call placeholders (`WebsterSeams`, `placeholderIndex`) simulate no behaviour,
  yet they implement another module's seam.
- Write-failure and malformed-input rows: some errors cannot be provoked through state on every operating system.
