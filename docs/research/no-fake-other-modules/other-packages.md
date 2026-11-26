# Stand-ins: gate, config, report, tools and remaining packages

Classified against [PATTERN-test-stand-ins](../../../pattern/PATTERN-test-stand-ins.md).
Tiers are those of `tiergate.Require`; a replacement that names no tier keeps the test's current one.

## `internal/gatecli`

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
| --- | --- | --- | --- | --- |
| Hand-written or `map`-marshalled `gate.yaml` (`scenarioConfig`, `scopesConfig`, `mutateGate`, per-test `gateYAML` strings) and `tier_parallel` machine files | `test_integration_test.go`, `scopes_integration_test.go`, `testmap_integration_test.go`, `mutate_integration_test.go`, `reach_integration_test.go`, `redundant_integration_test.go`, `redundancy_integration_test.go`, `detachfront_integration_test.go`, `redfirstdotnet_integration_test.go`, `dotnetfixture_integration_test.go` | the gate config (`gateslot`) | `real` | A `gateslot`-owned writer, or `configengine`'s writer over `gateslot.ConfigTemplate()`; refusals of bad values move to `gateslot`. |
| `gateslot.Config`, `gateslot.Tier` and `envconfig.Config` literals handed to `gatecli` functions | `tiers_test.go`, `cardfixer_test.go`, `dotnetscope_test.go`, `redfirstdotnet_test.go`, `testmapstamp_test.go`, `copies_test.go` | loaded config (`gateslot`, `envconfig`) | `real` | Load through the real loader from a file its writer wrote; stays `tier0`. |
| Hand-written `env.yaml` bodies | `test_integration_test.go`, `testmap_integration_test.go` | the env module's config (`envconfig`) | `real` | `configengine`'s writer over `envconfig`'s template. |
| Hand-written run record `{"slug":"s1","tier":0}` and an undecodable sibling | `report_test.go` | run records (`gateslot`) | `goes` | The `ReadRunRecords` cases move to `gateslot`; the good record comes from `WriteRunRecord`. |
| `fakeCardFixer` with its `gatefixer.Record` and `gatefixer.Result` answers | `cardfixer_test.go` | `gatefixer.Fixer` (`gatefixer`) | `real` | The real fixer over one LLM-session stand-in (`shuttlefake`, recorded answers); `tier2`. |
| `keepWarm` closures returning `shuttleengines.KeepWarmAnswer` | `test_integration_test.go` | keep-warm answer (`shuttleengines`) | `real` | `shuttleengines.KeepWarm` over a seeded engine config; caps the real engine cannot give move to its tests. |
| `memoryTree`, `failingTree`, `exportBase` closure | `redfirst_test.go`, `redfirstdotnet_test.go` | the base commit's tree and export (`fabricengine`) | `real` | A real commit read through `commitTree` and `ExportCodeTreeAt`; `tier1`. |
| `testreach.ChangedTest` literals | `redfirst_test.go` | changed-test list (`testreach`) | `real` | `testreach` over source fixtures; stays `tier0` where no git is read. |
| Scripted `sample`, `list`, `kill`, `pressure`, `describe` seams and `proc.ProcessSample` / `proc.MemoryLimit` literals | `memoryguard_test.go`, `watchdog_test.go`, `processcap_test.go`, `tierrun_test.go`, `testmap_integration_test.go` | the operating system's process table, behind `proc` | `external` | One stand-in below `proc`, with recorded process trees and failures, a `tier0` reader test and a contract test. |
| Stand-in `go` and build: the re-exec'd test binary as tier command, `runStandIn`, `runTestMapStandIn`, the coverage binaries it writes, the shell-script `go` | `test_integration_test.go`, `testmap_integration_test.go`, `copies_test.go` | the Go toolchain and test binaries (outside) | `external` | One home for the stand-in `go`, shared with `testcover` and `testtiming`; recorded output and failures; a contract test. |
| Hand-written `go test -json` lines, `Total tests:` console text, TRX XML | `counters_test.go`, `trx_test.go`, `tierrun_spawn_test.go`, `tierrun_test.go`, `redfirstdotnet_test.go` | `go` and `dotnet test` output (outside) | `external` | Recordings with source lines, picked and cut only; the counters run over all of them in `tier0`; a contract test. |
| `fakeMaps` and other `testcover.Map` literals written through `testcover.WriteMap` | `redundant_integration_test.go`, `mutate_test.go`, `reachverb_integration_test.go` | coverage maps (`testcover`) | `real` | Already written by the owner's writer; maps `--map` can build should be built by it. |
| `fixerRig`, `detachRig`, `helperLaunch`, `treeIdentity` and `runTiers` closures, `lym` shell stub that execs the real gate role | `cardfixer_test.go`, `redfirst_test.go`, `detachfront_integration_test.go`, `test_integration_test.go` | `gatecli`'s own seams | `own` | Stays. |

## `internal/testcover`, `internal/testreach`

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
| --- | --- | --- | --- | --- |
| `standIn` / `runStandIn`: `go list`, `go env`, `go test -c` and coverage-binary answers, canned compile errors and profile lines | `testcover/testmain_test.go`, `testcover/runner_test.go` | the Go toolchain (outside) | `external` | Same single home as the `gatecli` and `testtiming` stand-ins; answers recorded; contract test at `tier1`. |
| A `{not json` file planted among coverage maps | `testreach/reader_test.go` | a coverage map (`testcover`) | `goes` | Refusing a malformed map is tested in `testcover`. |

## Config packages: `configcli`, `configreg`, `configsync`, `reviewbudget`

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
| --- | --- | --- | --- | --- |
| Hand-written YAML for `loom`, `tapestry`, `board`, `gate`, `lenses`, `models` and other modules, including stale, rival and planted-key files | `configcli/*_test.go`, `configsync/configsync_test.go`, `configreg/configreg_test.go`, `configreg/runsettings_test.go`, `configreg/settinglookup_test.go` | each module's config file (the module declaring its template) | `real` | Seed from the module's `ConfigTemplate()` through `configengine`'s writers; stale and retired shapes are the hard case below. |
| Templates re-marshalled or edited (`lensesFixture`, `lensSetFixture`, node removal in `fill_test.go`) | `configsync/configsync_test.go`, `configreg/fill_test.go` | `lenses` and other templates | `real` | `configengine`'s set and unset writers. |
| `testdata/profiles-flat.yaml`, a copy of an earlier profiles file | `configsync/configsync_test.go` | an older `runprofile` template | `goes` | Move the old-shape case to `runprofile`, which declares the format. |
| Synthetic `configengine.Spec` values named `loom`, `darn`, `tapestry` with made-up templates | `reviewbudget/reviewbudget_test.go` | module specs and templates (`configreg` and the owning modules) | `real` | `configreg.Lookup(name).Spec()`; stays `tier0`. |
| `fakeEditor`, `makeNeverCalledEditor` | `configcli/*_test.go` | the operator's editor (outside) | `external` | Stays in its one home; the content it saves must follow the first row. |
| `fakeSyncTracker`, `fakeHubCommit`, `injectedSync` | `configcli/configcli_test.go`, `configcli_helper_test.go`, `configcli_integration_test.go`, `reconcile_integration_test.go` | `fabric sync` (`fabriccli`) and the hub commit over `fabricengine`'s checkout | `real` | `resolveReal`'s sync and `newHubCommit` over a `hubforge` hub, wrapped to count calls; `tier1`. |
| `hubWriteGuard` closures | `configcli/configcli_test.go` | `configcli`'s own guard over prime-worktree state | `real` | `realHubWriteGuard` over a real hub; `tier1`. |

## `internal/reportengine`

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
| --- | --- | --- | --- | --- |
| `fakeRepo`, `fakeCommit`, `world` tags and branches (`Reader`: log, blame, pre-image lines, files at a revision) | `fake_test.go` and every test that builds a `world` | the code, records and board repositories (`gitrepo`) | `real` | A real repository fixture read through `gitrepo`; `tier1`. |
| Marshalled `shedengine.Status`, history entries, `stateChange` text, seed JSON, settle-record lines | `fake_test.go`, `timing_test.go`, `runs_test.go`, `rounds_test.go`, `wakes_test.go`, `weeks_test.go`, `landings_test.go` | shed run records (`shedengine`, `shedrun`, `battenshed`) | `real` | `shedrun`'s writers into the fixture repository; `tier1`. |
| Session usage, run and landed records as `fmt.Sprintf` JSON | `cost_test.go`, `raddle_test.go` | usage records (`shedrun`) | `real` | `shedrun.WriteSessionUsage` and its sibling writers. |
| Parent-review round files, arbiter attempt and alignment files, drive reports, review files, decision records | `fake_test.go` | `loomengine`, `shedadapters`, `shedrun` run-directory records; the decision record is agent-written | `real` | Each owner's writer; the decision record is the hard case below. |
| `boardTask` literals marshalled as `board.json`, `boardengine.RaddleStoreEntry` store files | `fake_test.go`, `raddle_test.go` | the board (`boardengine`) | `real` | `boardengine`'s writers over a fixture board. |
| `proposalText` JSON | `raddle_test.go` | a raddle proposal (agent-written, format of `raddleparser`) | `goes` | Proposal decoding cases move to `raddleparser`; the report test uses a recorded agent proposal. |
| `trailerKey`, hand-composed commit trailers and archive-tag commits | `fake_test.go` | commit and tag shapes (`fabricengine`) | `real` | `fabricengine`'s commit and archive paths over the fixture; `tier1`. |

## `internal/reportcli`

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
| --- | --- | --- | --- | --- |
| Marshalled `shedengine.Status`, hand-written seed JSON and decision record | `convergence_integration_test.go` | `shedrun` records; the agent's decision record | `real` | `shedrun.WriteSeed` and the status writer; the decision record is the hard case below. |
| Hand-written pause record JSON | `convergence_integration_test.go`, `cli_test.go` | the pause record (`pauserecord`) | `real` | `pauserecord`'s pause and resume writers. |
| Hand-written journal lines and `journalctl` run errors | `cli_test.go` | `journalctl` (outside) | `external` | Recorded journal output and failures, one home, a contract test. |

## `internal/arbiterstop`

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
| --- | --- | --- | --- | --- |
| `shedengine.Status`, `StopKey` and stop literals with reason strings composed from other modules' prefixes | `arbiterstop_test.go`, `holdreading_test.go` | a stopped run (`shedengine`, `landingshed`, `shedadapters`, `loomshed`) | `real` | Statuses written and read through the owners' writers; see hard cases. |
| `Readers` closures: `PendingEscalation` answers and errors, `InterruptPolicy` | `arbiterstop_test.go` | `shedadapters` escalation records, `loomshed.InterruptPolicyFor` | `real` | The production readers over escalation records `shedadapters` wrote, with the error provoked there. |
| Escalation, review and circling-decision markdown | `holdreading_test.go` | Bouncer run-directory records (`shedadapters`) | `real` | `shedadapters`' writers. |
| `loom.yaml` and `shed.yaml` template bytes written and string-replaced | `holdreading_test.go` | `loomengine` and `sheddriver` config | `real` | `configengine`'s writer. |

## `internal/failuretrace`

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
| --- | --- | --- | --- | --- |
| `Enclose`, `Resolve`, `Delta` closures with `quarry` result literals | `trace_test.go` | `planglyph` over a real tree | `real` | `planglyph` over a committed fixture module; `tier1`. |
| `planparser.Plan` and `runplan.State` literals, hand-written plan files | `trace_test.go` | the plan (`planparser`) and implementer state (`runplan`) | `real` | `plankit` plus `planparser`, and the engines' `SaveState`. |
| `buildFailure` and other `go test` log text | `trace_test.go`, `locations_test.go` | Go toolchain output (outside) | `external` | Recorded build and test failures shared with the other Go-output recordings. |

## `internal/ideengine`, `internal/idecli`

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
| --- | --- | --- | --- | --- |
| `CodeLauncher` recorders and failing launchers | `idecli/cli_test.go`, `ideengine/menu_test.go`, `ideengine/spawn_*_integration_test.go` | VS Code, reached through `vscode.Launch` | `external` | One stand-in below `vscode` in place of the `code` process, with recorded failures; today two packages write their own. |
| `LymBinary` set to a made-up path | `ideengine/testmain_test.go` | `selfexec.LymBinary` | `own` | Stays where the path is only passed on; see hard cases. |
| Hand-written `board.json` and `board.yaml` | `ideengine/menu_test.go`, `ideengine/look_test.go` | the board (`boardengine`) | `real` | `boardengine`'s writers; the `not json` board case moves to `boardengine`. |
| Hand-written `ide.yaml` and `desktop.yaml` overrides | `ideengine/spawn_hub_integration_test.go` | `ideconfig`, `desktophook` config | `real` | `configengine`'s writer over each template. |
| `writeBars` settings JSON, old `tasks.json` chain | `ideengine/look_test.go`, `ideengine/spawn_clean_integration_test.go` | files `vscode` writes into `.vscode/` | `real` | `vscode`'s writers; the legacy chain case moves to `vscode`. |
| `KeybindingsPath` redirected to a temp file | `ideengine/testmain_test.go`, `idecli/testmain_test.go` | a path value | `own` | Stays. |

## `internal/stencilstore`, `internal/stencilcli`, `internal/pattern`

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
| --- | --- | --- | --- | --- |
| `fakeRegistry`, `skewRegistry` | `stencilstore/reconcile_test.go`, `stencilstore/skew_test.go` and the tests that call them | the stencil registry (`contracts/stencils`, `contracts/specs`) | `real` | `stencils.Registry()` where any default serves; newer-default and skew cases are a hard case. |
| `fakeRegistry` overriding defaults to play the next deploy | `stencilcli/cli_integration_test.go` | a later `contracts/stencils` | `goes` | The next-deploy step moves to `stencilstore`'s own reconcile tests. |
| Seeded fragment stencil overwritten with extra markers | `pattern/pattern_test.go` | a stencil (`contracts/stencils`) | `goes` | The unknown-marker refusal is tested in `stencil`. |

## `internal/preflight`, `internal/preflightshed`

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
| --- | --- | --- | --- | --- |
| `Checks` closures returning canned errors, a fabricated `batcher.ErrRetiredKey` wrap | `preflightshed/preflight_test.go` | `batcher`, stencil and drift checks wired by `loomcli` | `real` | The real checks over a config or store in the failing state; a check that only cancels the context or is nil stands for no module and stays. |
| `{}` placeholder files at `shedrun` and `loomengine` run-state paths, hand-written halt note | `preflight/preflight_integration_test.go` | run records (`shedrun`, `haltnote`) | `real` | `shedrun`'s writers and `haltnote.Render`. |
| `fabric.yaml` overwritten with invalid YAML | `preflight/preflight_integration_test.go` | `fabricengine` config | `goes` | The malformed-config case belongs to the config loader; keep another provoked junction failure. |

## `internal/raddleparser`, `internal/partsparser`

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
| --- | --- | --- | --- | --- |
| `LivePattern` and `ValidateSlug` closures in `validBoard` | `raddleparser/fixture_test.go`, `check_test.go` | `pattern.EntryBullet`, `boardengine.ValidateEntrySlug`, `fabricengine.ValidatePairSlug` | `real` | The real functions from an external test package; stays `tier0`. |
| Hand-written decision record | `partsparser/check_test.go` | an agent-written record read by `discussionparser` | `goes` | See hard cases; id reading is tested where the record is parsed. |

## `internal/cliwire`

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
| --- | --- | --- | --- | --- |
| `websterFixture`, `burlerFixture` `Module` descriptors with copied refusal text | `cliwire_test.go` | the descriptors `webstercli` and `burlercli` fill | `goes` | Descriptor-specific cases move to `webstercli` and `burlercli`; `cliwire` keeps its generic rules. |
| `placeholder` plan file in `seedPlanDir` | `cliwire_test.go` | a plan (`planparser`) | `real` | `plankit.Write`. |
| `.git` file holding a `gitdir:` line | `cliwire_test.go` | git's linked-worktree file (outside) | `external` | A recorded file or a real `git worktree add` (`tier1`). |

## `internal/termwindow`, `internal/desktoplauncher`

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
| --- | --- | --- | --- | --- |
| `lym stub` shell scripts exiting 0 or 3 and removing the directory | `termwindow/termwindow_integration_test.go` | `lym reed attach` (`cmd/lym`, `reedcli`) | `goes` | The exit codes are tested in `reedcli`; driving real `lym reed attach` is `tier2`. See hard cases. |
| `getenv`, `lookPath`, `fileExists`, `isDir`, desktop-directory seams, planted `user-dirs.dirs` | `termwindow/termwindow_test.go`, `desktoplauncher/locate_test.go` | the operating system (outside) | `external` | Each stays in its one package; values taken from real WSL, Linux and Windows hosts, with a contract test. |

## `internal/lymtemp`, `internal/selfexec`, `internal/shellguard`

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
| --- | --- | --- | --- | --- |
| `root.remove` returning composed `*fs.PathError` values | `lymtemp/sweep_test.go` | the filesystem (outside) | `external` | Record the real errno failures; stays in `lymtemp`. |
| Scripted `stat` identities and `exec` errors | `selfexec/selfexec_test.go` | the operating system (outside) | `external` | Stays in `selfexec`; failures recorded. |
| Hand-written `PreToolUse` hook payload | `shellguard/rows_integration_test.go` | Claude Code's hook input (outside) | `external` | A recorded payload with its source, one home beside the Claude engine, a `tier3` contract test. |

## `internal/testkit/envelope`

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
| --- | --- | --- | --- | --- |
| Hand-written `{"ok":...}` envelope strings | `envelope_test.go` | command envelopes (`output`) | `real` | Text produced by `output`'s writers; the kit's stdlib-only import limit applies to its production file. |
| `recordingTB` | `envelope_test.go`, `tiergate/tiergate_test.go` | `testing.TB` (Go, not a module here) | `own` | Stays. |

## `cmd/lym`

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
| --- | --- | --- | --- | --- |
| Hand-written heddle store JSON | `passagereader_test.go` | the heddle store (`heddleengine`) | `real` | `heddleengine`'s writer. |
| Hand-written `board.yaml` and `logger.yaml` | `main_integration_test.go`, `exitcode_test.go` | `boardengine`, `loggerconfig` config | `real` | `configengine`'s writer over each template. |
| Seeded dead-process trace files with composed names | `exitsweep_test.go`, `main_integration_test.go` | trace files (`logger`) | `real` | Traces written by `logger`, then aged with `os.Chtimes`; see hard cases for the dead pid. |

## `cmd/testtiming`

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
| --- | --- | --- | --- | --- |
| `runStandInGo` | `testmain_test.go`, `prebuild_integration_test.go` | the Go toolchain (outside) | `external` | Same single home as the `testcover` stand-in `go`. |
| Hand-written git trace2 event lines | `census_test.go` | `GIT_TRACE2` output (outside) | `external` | Recorded trace2 files and a `tier1` contract test. |
| `testcover.RunResult` and block literals | `redundancy_test.go` | measured test runs (`testcover`) | `real` | `testcover` runs over a fixture module (`tier1`), or move the containment judgement into `testcover`. |
| Hand-written `gate.yaml` | `takegateslot_integration_test.go` | gate config (`gateslot`) | `real` | The same writer as the `gatecli` row. |

## `contracts/stencils`

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
| --- | --- | --- | --- | --- |
| `shedadapters.RubricNotes` with made-up record, tier and report strings | `rubric_test.go` | values `loomshed` and the gate reports fill | `own` | Stays as opaque markers the test only places; see hard cases. |

## `tools/tokencount`

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
| --- | --- | --- | --- | --- |
| `fakeTranscripts`, `fakePricer`, `fakeInstructions`, `fakeEngine`, and `shuttleengine` read and entry literals | `fixtures_test.go`, `count_test.go`, `calibrate*_test.go`, `orch_test.go`, `segment_test.go`, `profile_test.go`, `time_integration_test.go` | the engine transcript reader, pricer and instruction list (`shuttleengines`, `claudeengine`) | `real` | The real Claude readers over recorded transcripts with source lines; recordings are cut, never written. |
| `fakeHistory`, `fakeBase`, `fakeRecords`, `fakeArchiveTrees`, `fakeLandingRepo` | `fixtures_test.go`, `count_test.go`, `landed_test.go`, `calibrate_test.go`, `profile_test.go` | the records and code repositories (`gitrepo`) | `real` | Real commits through `gitrepo`, as `calibrate_integration_test.go` does; `tier1`. |
| Marshalled `websterengine.State`, shed seed and status JSON, review files | `profile_test.go`, `count_test.go`, `calibrate_integration_test.go`, `fixtures_test.go` | `websterengine`, `shedrun`, review-file records | `real` | The owners' writers; the review file is agent-written and needs a recording. |
| `batcher.yaml` in `seedProfile`, `claude.yaml` machine file, `{}` session transcript | `fixtures_test.go`, `reads_test.go` | `batcher` and `claudeengine` config; a Claude transcript | `real` | `configengine`'s writer; a recorded transcript. |

## `tools/retro`, `tools/forkaudit`

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
| --- | --- | --- | --- | --- |
| Hand-written usage records, shed `status.json`, review facts markdown | `retro/retro_integration_test.go` | records of `shedrun`, `shedengine` and the review facts writer | `real` | The owners' writers into the fixture repository. |
| `transcriptLines`, task notification, `testdata/*.jsonl` transcripts, truncated-line suffix | `retro/retro_integration_test.go`, `forkaudit/forkaudit_integration_test.go` | Claude Code transcripts (outside) | `external` | Recordings with source lines, one home with the Claude engine's corpus, a `tier3` contract test. |
| `stateText` webster state, plan card files | `forkaudit/forkaudit_integration_test.go` | `websterengine` state, `planparser` plan | `real` | `websterengine`'s state writer and `plankit`. |
| `traceLine`, `cardPackLine`, `writeTrace` | `forkaudit/forkaudit_integration_test.go` | trace log lines (`websterengine` through `logger`) | `goes` | The cross-check cases need lines only a live webster run logs; see hard cases. |
| Failing `date` script | `forkaudit/forkaudit_integration_test.go` | an OS command (outside) | `external` | Stays; its failure recorded. |

## `tools/sandbox`, `tools/plandag`, `tools/deploy`

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
| --- | --- | --- | --- | --- |
| `devBinPath` closures | `sandbox/suite_test.go`, `main_test.go`, `report_test.go` | `tools/internal/devbin` | `real` | `devbin.BinPath` over a fixture layout. |
| `lookPath` closures | `sandbox/suite_test.go`, `main_test.go`, `report_test.go` | the PATH lookup (outside) | `external` | Stays in `sandbox`. |
| `launchAgent` closures | `sandbox/suite_test.go`, `main_test.go` | a `claude` session (outside) | `external` | Replace the `claude` process below `launchAgent`, answers recorded, a contract test. |
| Hand-written `sandbox-report.json` bodies | `sandbox/report_test.go`, `suite_test.go`, `main_test.go` | the report the agent session writes (outside) | `external` | Recorded agent reports; `sandbox`'s own refusal rows for malformed input stay. |
| `reedDown` no-op, `fakeBinaryInfo`, `makeFakeLym` bytes that are only fingerprinted | `sandbox/*_test.go` | `sandbox`'s own seam and data | `own` | Stays. |
| Hand-written plan directories (`writePlan`, `writePlanInLanguage`) | `plandag/main_test.go` | a plan (`planparser`) | `real` | `plankit`. |
| `git status --porcelain` strings | `deploy/main_test.go` | git output (outside) | `external` | Recorded from git, with a contract test at `tier1`. |

## Packages with none

`gateslot`, `impactset`, `dotnetimpact`, `commentlint`, `configengine`, `envconfig`, `envsource`, `languageconfig`,
`loggerconfig`, `logger`, `modelspec`, `agentconfig`, `reportconfig`, `stencil`, `ideconfig`, `vscode`,
`desktophook`, `clihelp`, `claudecli`, `claudeimports`, `quarrycli`, `editdirective`, `parentdirective`,
`twilldirective`, `lenses`, `lensreport`, `pauserecord`, `haltnote`, `buildinfo`, `buildvcs`, `entrymerge`,
`output`, `fslink`, `fswatch`, `fsx`, `secretpath`, `segmentcolor`, `consoletitle`, `durfmt`, `helpcli`, `shell`,
`runprofile`, `yamlengine`.

`internal/testkit/lymbin`, `logcapture`, `permkit`, `scankit`, `stencilkit`, `tiergate` and the testkit root.

`contracts/specs`, `tools/codestats`, `tools/godocreflow`, `tools/internal/devbin`, `tools/machinecost`,
`tools/mdreflow`, `tools/tapestryrename`, `tools/wordswap`.

## Hard cases

Pure decisions over another package's types (`arbiterstop.Check`, `reportengine`'s computations):
no writer produces an arbitrary `shedengine.Status`, so "drive the real module" can mean running a whole recipe,
and "move to the owner" fails because the decision is not the owner's.

Retired and older config shapes (`configsync`, `configcli`, `profiles-flat.yaml`):
today's writer cannot produce them, yet the migration under test exists only for them.

Agent-written files with a repository parser (decision record, review file, raddle proposal):
the owner is both the outside agent and the parsing package, so a hand-written one reads as `external` or as `goes`.

`stencilstore`'s `Registry` and version skew: the real registry holds one version, so a newer default cannot be driven,
while the interface looks like plain data of `stencilstore`.

`proc` seams in `gatecli`: the memory-pressure and sampling-failure cases need a stand-in below `proc`,
which `proc` must first offer, and a recorded failure that may not be reproducible on demand.

`LymBinary`, the `lym stub` window scripts and `rubric_test.go`'s notes:
each hands opaque text or a path that the code under test never interprets,
yet it sits in a seam another module fills.

Coverage maps given to `testcover.WriteMap`: the owner's writer accepts any values,
so a made-up map passes the letter of the rule while simulating a `--map` run.

State no live writer can leave (`logger` traces of dead processes, `webster` trace lines of an old run):
the real module writes them only with its own pid and clock.
