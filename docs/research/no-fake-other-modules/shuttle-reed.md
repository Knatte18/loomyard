# Stand-ins: shuttle, reed and session packages

Classified against [PATTERN-test-stand-ins](../../../pattern/PATTERN-test-stand-ins.md).
Paths are under `internal/`.

## The LLM stand-ins

### `testkit/shuttlefake`

No part of the kit stands for the LLM session itself.
Each double sits above a repository module, so the outside-thing rules do not excuse it.

| Part | Stands for | Verdict |
|---|---|---|
| `Engine` (canned `Launch`, `Events`, `Audit`, `Startup` and the `*Fn` overrides) | `claudeengine` and `codexengine` | `real` |
| `SignalEngine` | the session-signal reading of `claudeengine` | `real` |
| `EngineSet`, `EngineName` | the registry `shuttleengines`, holding only the fake | `goes` with `Engine` |
| `Reed`, `RelaunchCall` and the other call records | `reedengine` | `real` |
| `TokencountRunReads`, `tokencount-run-reads.json` | a golden of `claudeengine`'s transcript reading | `real`: `tools/tokencount` drives the real reader over the recorded transcripts, and the golden moves to `claudeengine`, whose `transcriptread_test.go` compares it |
| `Agents` | nothing: it calls `agentconfig.Load` | `real` |

For `Engine`, "real" means a real engine over a recorded session; a scripted failure no state provokes goes.
For `Reed`, it means real reed at tier2; a scripted refusal goes.

The consumer groups are burler, creel, gatefixer, loom, orch, seat, shuttle (`shuttlecli`, `claudeengine`,
`shuttleengines`, `shuttleparity`), tender, weaver and webster.

### Recordings and hand-written data

Recorded from a real session, with source lines:

- `shuttleengine/claudeengine/testdata/corpus/*/` names the session, run, date and Claude version in each `case.yaml`.
  Cases marked `events_reconstructed: true` hold events lines rebuilt by hand, which the rules forbid.
  The transcripts are trimmed, and `hook-capture` has its ids and paths scrubbed.
- `codexengine`'s hook payloads in `events_test.go` and screens in `pane_test.go` cite
  `docs/research/codex/codex-spike-2026-10-24/report.md`.

Recorded, with source lines missing or partial:

- `claudeengine/testdata/turnprogress/*.jsonl` carry real ids and `version` 2.1.296 but no source lines.
  Some end in hand-written entries (`hb-api-error`, `hb-interrupt`, `hb-max-tokens`), new content in a recording.
- `claudeengine/testdata/fork-spawn-boundary.jsonl` has real ids; only `audit_spawnboundary_test.go` names a version.
- `shuttleengine/testdata/pane-scroll-baseline.txt` and `pane-scroll-delivered.txt` are called real captures in
  `run_test.go`, with a pane size and no date or version.
- `claudeengine/testdata/pane-placeholder-styled.txt`, `pane-restored-prompt-styled.txt`, `pane-rewind-picker.txt`
  look captured and name no source.
- `claudeengine/startup_test.go` holds trust and bypass gate screens "transcribed from a live pane",
  with a version and no date.

Hand-written, or with nothing to show they were recorded:

- `claudeengine/testdata/pane-autocompact-picker.txt`, `pane-idle-draft.txt`, `pane-idle-empty.txt`,
  `pane-idle-eol-mark.txt`, `pane-permission-prompt.txt`, `pane-turn-running.txt`.
- `claudeengine/testdata/fork-clean.jsonl`, `fork-mutating.jsonl`, `fork-nested-agent.jsonl`, `parent-spawns.jsonl`,
  `parent-writes-results.jsonl`, `skillload-*.jsonl`, `usage-*.jsonl` and `tokencount-run/`.
- The inline screens, transcript lines, hook payloads and rollout lines in the engines' test files.

`claudeengine/testdata/settings-golden.json` is the package's own output, not a recording.

### `tier3` contract tests

No `tier3` test in these packages compares a live session's output with a recording.
`TestSmokeTurnsHookCapture` (`shuttlecli/smoke_turns_test.go`) can record:
it keeps the events file, transcript and pane readings under `LYM_TURN_KEEP_DIR`,
and the corpus case `hook-capture` is promoted from them by hand.
The other turn scenarios keep events and transcript only when they fail (`keepOnFailure`).
No test records Claude's pane screens as fixtures, and none records anything from Codex.

## `shuttleengine`

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
|---|---|---|---|---|
| `fakeReed` and its wrappers (`skillReed`, `idleReed`, `reactingReed`, `swallowingReed`, `relaunchReed` and the other `*Reed` types) | `fakes_test.go`, `startup_test.go`, `run_test.go`, `attach_test.go` and the other run-loop tests | `reedengine` | `real` | Real `reedengine` with a pane process that plays recordings (tier2); cases scripting a reed error per tick `goes` |
| `fakeEngine` and its capability variants (`sessionFakeEngine`, `waitingEngine`, `idleEngine`, `readinessEngine`, `pasteEngine`, `switchFakeEngine`, `dispatchEngine` and the other `*Engine` types) | same | `claudeengine`, `codexengine` | `real` | The real engine, from an external `shuttleengine_test` package, over recordings; stays tier0 for parsing and screen reading |
| The `STOP:`, `ASK:`, `START`, `SESSIONEND:` event language written to `events.jsonl` | `gate_test.go`, `wait_test.go`, `startup_test.go` and most others | The provider hooks, whose format the engines read | `goes` | Recorded hook lines, picked and cut |
| Hand-drawn pane screens (`IDLE`, `RUNNING`, `REWIND-PICKER`, the frames of `tuiReed` and `restoreReed`) | `fakes_test.go` and its users | The LLM TUI, outside | `external` | Recorded screens, one shared kit, a contract test; a keystroke-reacting TUI cannot be built from recordings alone |
| `testdata/pane-scroll-*.txt` | `run_test.go` | The LLM TUI | `external` | Source lines and a `tier3` test that records them again |
| Output files written as `done` or `result` | `wait_test.go`, `startup_test.go`, `attach_test.go` and others | The agent's file work | `own` | Stays: only the file's presence counts |
| Hand-built `reedengine.StatusResult`, `StrandStatus`, `Strand`, `Removed` | `liveStatus` and every run-loop test | `reedengine` records | `real` | Values a real reed returns (tier2) |
| Corrupt or absent `reed.json` written by hand | `attach_test.go` (`seedUnreadableReedState`), `run_test.go` | `reedengine` state | `goes` | Refusal of bad bytes is `reedengine`'s test; absent state needs no write |
| `InstallPassageReader` and notifier closures, where set | run-loop and manifest tests | `heddleengine` and the parent notification path | `real` | The real reader over a real heddle store; recording wrapper around the real notifier |
| `pauserecord.Update` fixtures, `reedengine.SaveState` seeds | `fixture_test.go`, `gate_test.go`, `attach_test.go` | Written by the owners' writers | `real` | Already real |
| `run.json` seeds, shuttle config layers, `belowshuttle` config | `attach_test.go`, `rundir_test.go`, `config_test.go` | `shuttleengine`'s own state | `own` | Stays |
| `fakeClock`, `scriptedClock`, `hookClock` and the other clocks | many | Clock | `own` | Stays; the events some of them append fall under the event-language row |

## `shuttleengine/claudeengine`

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
|---|---|---|---|---|
| `testdata/corpus/` | `corpus_test.go`, `signals_test.go` | Claude Code | `external` | Re-record reconstructed events; a `tier3` contract test per kind |
| `testdata/turnprogress/`, `fork-spawn-boundary.jsonl` | `prober_test.go`, `audit_spawnboundary_test.go` | Claude transcripts | `external` | Source lines, no appended entries, a `tier3` recorder |
| Other `testdata/*.jsonl` and `tokencount-run/` | `audit_test.go`, `usage_test.go`, `skillload_test.go`, `transcriptread_test.go` | Claude transcripts | `external` | Recordings with source lines in place of hand-written lines |
| `testdata/pane-*.txt` and inline screens | `session_test.go`, `startup_test.go`, `start_test.go` and others | Claude's TUI | `external` | Recorded screens with source lines and a contract test |
| Inline transcript lines, hook payloads, session registry and `meta.json` files | `events_test.go`, `signals_test.go`, `activity_test.go`, `background_test.go`, `audit_*_test.go`, `prober_test.go`, `transcriptread_test.go`, `batchingnudge_test.go` | Claude Code | `external` | Cuts of recordings; a single home for the line builders |
| Claude user files: `installed_plugins.json`, `hooks.json`, credentials, settings, `CLAUDE.md` | `usersetup_test.go`, `manifest_test.go`, `accountpick_test.go`, `accountkeeper_test.go`, `accountswitch_test.go` | Claude Code's and the operator's files | `external` | Shapes recorded from a real install, with a contract test |
| `shuttlefake.Reed` | `start_test.go` | `reedengine` | `real` | Real reed with a pane playing `pane-idle-eol-mark.txt` (tier2) |
| `#!/bin/sh` stub `lym` | `hookcmd_integration_test.go`, `envfile_integration_test.go` | `lym` verbs (`shuttlecli`, `creelcli`) | `real` | The built binary (`lymbin`, tier1); exit codes no state provokes `goes` |
| Stub turn hook, `date`, `cygpath` | same | The operator's hook and OS tools | `external` | Stays in this package; a recording for `cygpath` output |
| `cannedLaunchEngine`, `settings-golden.json`, `claude.yaml` layers | `start_test.go`, `settings_test.go`, `config_test.go` | The package's own | `own` | Stays |

## `shuttleengine/codexengine`

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
|---|---|---|---|---|
| Spike hook payloads and screens | `events_test.go`, `pane_test.go` | Codex | `external` | Move to recording files in this package; a `tier3` contract test, which does not exist |
| Rollout lines, other hook and transcript JSON | `rollout_test.go`, `usage_test.go`, `skillload_test.go`, `batchingnudge_test.go`, `editrecord_test.go`, `guards_run_test.go` | Codex | `external` | Recordings and a contract test |
| Stub `codex` scripts and the `SandboxProbe` seam | `sandbox_test.go`, `hookrun_test.go`, `fixture_test.go` (`newEngine`) | The `codex` binary | `external` | One stand-in, answering recorded output, exit codes and failures |
| `auth.json`, Codex home and `sessions/` | `fixture_test.go`, `usersetup_test.go` | Codex's files | `external` | Shapes recorded from a real home |
| Hand-written `.git`, gitfile and `commondir` | `fixture_test.go` | git | `external` | Real git through `gitkit` or `hubforge` (tier1) |
| `fakeUserSetupSource` | `usersetup_test.go`, `manifest_test.go`, `instructionfiles_test.go` | `claudeengine`'s user-setup source | `real` | Claude's real source over a fixture config directory; source errors provoked there |
| Stub `lym` scripts | `hookrun_test.go`, `guards_run_test.go` | `lym` verbs | `real` | `lymbin` (tier1) |
| `codex.yaml` layers | `fixture_test.go` | The package's own | `own` | Stays |

## `shuttleengines`

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
|---|---|---|---|---|
| `shuttlefake.Reed` with a seeded strand | `shuttleengines_test.go` | `reedengine` | `real` | Real reed (tier2) |
| `shuttlefake.Engine` as an engine without a capability | `keepwarm_test.go` | A provider engine | `real` | The registered `codexengine` where it lacks the capability, else `goes` |
| `forksWithoutAuditEngine`, `namingEngine`, `readingEngine` | `shuttleengines_test.go` | Provider engines | `goes` | Assert over the registered engines; combinations no engine has go |
| Hand-written `claude.yaml`, `codex.yaml`, `shuttle.yaml` layers | `shuttleengines_test.go`, `codexlaunch_test.go`, `keepwarm_test.go` | `claudeengine`, `codexengine`, `shuttleengine` config | `real` | `configengine.Set` over each owner's template |
| `RunHook`, `SandboxProbe` closures in `LaunchSeams` | `codexlaunch_test.go` | `sh` and `codex`, reached by `codexengine` | `goes` | Drive the real fallbacks over the one `codex` stand-in (tier1) |
| Hand-written `.git`, `auth.json`, plugin files | `codexlaunch_test.go`, `codexhook_integration_test.go` | git, Codex, Claude | `external` | A shared kit for each outside thing; no second writer here |

## `shuttleparity`

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
|---|---|---|---|---|
| `manifestEngine` over `shuttlefake.Engine` | `compare_test.go`, `report_test.go` | Provider engines | `real` | Both real engines, as `seats_test.go` does; divergences they cannot produce `goes` |
| Hand-built `shuttleengine.Manifest`, `ManifestItem` and fixture `seatengine.Table` rows | `compare_test.go`, `report_test.go` | Engines and recipe seat tables | `real` | Manifests from real `Prepare`; see hard cases |
| `InstallPassageReader` closure, hand-assembled `AGENTS.override.md` and `CLAUDE.local.md` | `seats_test.go` | `heddleengine` | `real` | Render a real heddle store and install its reader |
| Hand-written `codex.yaml`, `claude.yaml`, `language.yaml` | `seats_test.go` | Their modules' config | `real` | `configengine.Set` over each owner's template |
| `.git`, `auth.json`, plugin and skill files | `seats_test.go` | git, Codex, Claude | `external` | The shared kits |

## `shuttlecli`

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
|---|---|---|---|---|
| `shuttlefake.Engine`, `forkListEngine`, `apiErrorEngine`, `stateEngine`, `openRecorderEngine` | `cli_test.go` | Provider engines | `real` | Real engines over recordings; spec capture by a recording wrapper around a real engine |
| `shuttlefake.Reed` | `cli_test.go` | `reedengine` | `real` | Real reed (tier2) |
| `fakeResolver` | `cli_test.go` | `reedengine.ResolveStrandGUID` | `real` | A real reed engine over saved state |
| `run.json` marshalled by the test, invented event lines | `cli_test.go` | `shuttleengine` | `real` | A run started by the real runner; invented lines become recordings |
| `manifestReport` closure | `manifest_test.go` | `shuttleparity` | `real` | The real report; verdicts it cannot produce move to `shuttleparity` |
| Edited `shuttle.yaml` content | `hookpython_test.go`, `hookpython_standalone_test.go` | `shuttleengine` config | `real` | `configengine.Set` over the owner's template; malformed input moves to the parser's test |
| Hook payloads and transcript lines | `hookbatching_test.go`, `hookedit_test.go` | Claude Code | `external` | Recordings shared with `claudeengine` |
| Hand-written `.git/HEAD` | `hookpython_standalone_test.go` | git | `external` | Real git (tier1) |

## `reedengine`

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
|---|---|---|---|---|
| `fakeTmux` and the pane listings typed into it | `faketmux_test.go` and its users | tmux | `external` | Stays as the single tmux stand-in; answers and failures recorded with a tmux version, plus a contract test |
| Recording `tmux` shell scripts | `slacktoggle_test.go` (`recordingTmux`), `bindings_test.go` | tmux | `external` | Fold into `fakeTmux`: one stand-in per boundary |
| `countingProbe`, `exitCodeErr`, `/proc` stat and uptime text, stale server log text | `tmuxfloor_test.go`, `endsession_test.go`, `proctree_test.go`, `serverlog_test.go` | tmux and the OS | `external` | Recordings with their source |
| `testScrub` | `enginefixture_helper_test.go` | `claudeengine`'s declared scrub | `real` | An external test package taking `shuttleengines.EnvScrub`, or any scrub that is plainly invented data |
| `fakeNamer` | `namerepair_test.go` | `claudeengine` as `SessionNamer` | `real` | Claude over recorded transcripts, from an external test package |
| `Geometry.Buttons` and `Geometry.SpawnOrder` closures | `buttonrow_test.go`, `lifecycle_test.go` | `hubgeom` and the modules behind it | `real` | `hubgeom` over a `hubforge` hub (tier1) |
| Stub `lym` and `lym-recorder` scripts | `window_integration_test.go`, `windoworder_integration_test.go`, `ownwindow_integration_test.go`, `bindings_integration_test.go`, `buttonclick_integration_test.go` | The `lym` CLI | `real` | `lymbin` (tier2 already); a pure sleeper needs no `lym` name |
| `Geometry` literals, `reed.json` documents | `enginefixture_helper_test.go`, `contract_integration_test.go`, `retiredkeys_test.go`, `state_test.go` | The package's own types and file | `own` | Stays |

## `reedcli`

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
|---|---|---|---|---|
| `fakeStrandOps` | `removedetach_test.go`, `down_test.go`, `removename_test.go` | `reedengine.Engine` | `real` | Real engine (tier2 for strand calls); call order by a recording wrapper |
| `watchdogListSessions` closures | `watchdog_test.go` | `reedengine` over tmux | `real` | Real listing against a real server (tier2) |
| `downLockProbe`, `pidAlive`, `watchdogOpenWatch` failures | `down_test.go`, `removename_test.go`, `watchdog_test.go` | The lock, `proc` and `fswatch` modules | `real` | A really held lock and a real process (tier1); an unopenable watch `goes` |
| `readKills` closure and hand-built `StrandStatus` | `oomnotice_test.go` | `proc` cgroup reading and `reedengine` | `real` | Real status (tier2); a real OOM count cannot be provoked, so those cases `goes` to the owner |
| `placeWindow` closure | `window_test.go` | The window-placing module | `real` | The real function over a window stand-in kept by its owner |
| Rewritten or truncated `reed.json` | `smoke_warmpath_test.go`, `smoke_coldstart_test.go`, `smoke_staterecovery_test.go` | `reedengine` state | `goes` | Move to `reedengine`; restoring a file reed itself wrote byte for byte may stay |
| Line-edited `reed.yaml`, planted board stencil | `smoke_warmpath_test.go`, `smoke_switchseed_test.go` | `reedengine` config, `stencilstore` | `real` | `configengine.Set` over reed's template; the stencil through `stencilstore` |
| `openTerminal`, `watchdogExeStat` closures, fake server log, hand-written `.git` | `window_test.go`, `watchdog_test.go`, `smoke_debuglog_test.go`, `button_test.go`, `cli_test.go` | Terminal, OS, tmux, git | `external` | One stand-in each, recorded; `.git` from real git (tier1) |
| `sleep 300` as the pane command of a strand named `claude` | `cli_integration_test.go`, `unlessname_integration_test.go` | The agent process | `external` | Stays; it answers nothing |
| `spawnRemove`, `respawnWatchdog`, `endProcesses` seams, `downLockSleep` | `removedetach_test.go`, `watchdog_test.go`, `down_test.go` | The package's own functions and a sleeper | `own` | Stays |

## `tenderengine`

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
|---|---|---|---|---|
| `fakePair` | `pause_test.go`, `resume_test.go` | `tendercli`'s pair over shuttle and reed | `real` | The real pair over shuttle and reed (tier2), or the cases move to `tendercli` |
| `errNoReedSession` | `pause_test.go` | `reedengine.ErrNoSession` as `Status` words it | `real` | Provoke it in reed with no session |
| `fakeCreel` | `pause_test.go` | The `creelengine` journal | `real` | A real journal in a temp directory (tier0) |
| `Processes`, `RunProcs`, `EndOwner`, `EndProcess` seams and their hand-built records | `reap_test.go`, `resume_test.go` | `proc`, `runproc` | `real` | Registry written by `runproc`, real child processes (tier1); survivors that cannot be provoked `goes` |
| `NetOps`, `Locks`, `Worktrees`, push and board-sync closures | `pause_test.go`, `resume_test.go` | `pauserecord`, `parentlock`, `fabricengine`, `boardengine` | `real` | Owners' writers and readers over a hub (tier1); recording wrappers for the spawns |
| `{not json` pause record | `resume_test.go` | `pauserecord` | `goes` | Move to `pauserecord`'s tests |
| `Orch`, `PairOwner`, `SessionInfo`, `BoltHoldState` values, `fakeClock` | `pause_test.go`, `bolthold_test.go` | The package's own types and a clock | `own` | Stays |

## `tendercli`

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
|---|---|---|---|---|
| `signalEngine` | `deps_test.go` | Provider engines | `real` | Real engine over recorded hook lines |
| `run.json` by `Sprintf`, one-byte events file | `deps_test.go` | `shuttleengine` | `real` | A run record from the real runner |
| `creelengine.LogEntry` literals | `deps_test.go` | `creelengine` journal | `real` | Entries the real journal writes |
| `boardengine.Owners`, `parentOf` and probe closures | `deps_test.go` | `boardengine`, the origin record | `real` | Real board over a hub (tier1); probe errors move to the owners |
| Hand-written batten status JSON | `cli_test.go`, `pauseintents_test.go` | `shedengine` status | `goes` | `shedengine`'s writer; the unreadable row moves to its parser |
| Constructed `fabricengine.PushFailedError` | `runintents_test.go` | `fabricengine` | `real` | A refused push in a real hub (tier1) |

## `frictionengine`

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
|---|---|---|---|---|
| `shedfake.Shuttle` | `reflect_test.go` | `shuttleengine.Runner` | `real` | Real runner over real reed and the agent stand-in (tier2) |
| `reflectionStencilFixture` over the seeded stencil | `reflect_test.go` | The shipped stencil | `goes` | Keep what `stencilkit` seeds |
| Note and report files written by the test | `reflect_test.go` | The agent's output | `external` | Stays; one helper |
| `fakeClock`, the covered-notes record via `writeRecord` | `reflect_test.go` | Clock and own state | `own` | Stays |

## `retrocli`

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
|---|---|---|---|---|
| Marshalled `shedengine.Status` files, seed JSON by `Sprintf` | `fileretro_integration_test.go` | `shedengine`, `shedrun` | `real` | Their writers (tier1 already) |
| Hand-made archive tag and landing steps | `fileretro_integration_test.go` | `fabricengine`'s landing | `real` | Land through `fabricengine` (tier1) |

## `heddleengine` and `heddlecli`

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
|---|---|---|---|---|
| Hand-built `fabricengine.WorktreeEvent` with its `Excluded` list | `seam_test.go` | `fabricengine` | `real` | Events fabric raises, as `seam_integration_test.go` does (tier1) |
| `spawnSync` closure | `cli_integration_test.go` (`recorder`) | `boardengine.SpawnSync` | `real` | Wrap the real spawn and record; the spawn error case needs a provoked failure |
| Claude settings JSON and repository instruction files | `settings_test.go`, `render_test.go` | Claude's and the operator's files | `external` | Shapes recorded from Claude Code |
| `recorder.save`, temp directories as the Bolt checkout, `heddle.json` and `heddle.yaml` | `cli_integration_test.go`, `render_test.go`, `store_test.go` | A pass-through wrapper and own files | `own` | Stays |

## `pawlengine`

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
|---|---|---|---|---|
| `world`'s seams (status, dobby, recheck, push, notify, hand-off, ready, drain) and the `Steps` closures | `world_test.go`, `beamup_test.go`, `beamawait_test.go`, `ready_test.go` | The modules `pawlcli` wires: shed status, orch, fabric, creel | `real` | Wire the real modules over a hub (tier1, tier2 for sessions); see hard cases |
| Land request, ready, landing and marker files, the fixed `Now` and recorded `Sleep` | `landrequest_test.go`, `landing_test.go`, `ready_test.go`, `world_test.go` | Own files and a clock | `own` | Stays |

## `sleepjournal`

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
|---|---|---|---|---|
| `journalctl -o json` lines, stderr hints and run errors | `sleepjournal_test.go` | `journalctl` | `external` | Recordings with date and systemd version, a contract test, one home |

## `statuspane`

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
|---|---|---|---|---|
| Marshalled `shedengine.Status`, `{not json` status file | `cache_test.go` | `shedengine` status | `real` | `shedengine`'s writer; the undecodable rows `goes` unless the cache's two-strike rule counts as own |
| `Size` closure | `pane_test.go` | The terminal | `external` | Stays in this package; sizes and a failure recorded from a real terminal |
| `After`, `Resized`, `Check` seams, the sidecar file | `pane_test.go`, `build_test.go` | A timer, a signal and own state | `own` | Stays |

## `testkit/llmkit`

| Stand-in | Test files | Simulates (owner) | Verdict | What replaces it |
|---|---|---|---|---|
| Empty `claude` script on `PATH` | `llmkit_test.go` | The `claude` binary | `external` | Stays: only its presence is read, so there is nothing to record |

## Packages with none

- `shuttleengine/bashlookup`
- `reedengine/render`
- `friction`
- `retroengine`
- `testkit/tmuxkit`: it drives real tmux; its fake run and process snapshot feed its own check.

## Hard cases

Operator-written config has no writer.
A test of another module that needs `claude.yaml` or `codex.yaml` with a non-default key can only write it by hand,
which "written by that module's own writer or loader" forbids.

`shuttleengine`'s wait loop depends on what reed and the pane show tick by tick.
Real tmux cannot reproduce that timing deterministically, and the cases cannot move to `reedengine`,
because the rules under test are shuttle's.

`reedengine`'s only tmux seam (`execHook`) is unexported.
A package above reed can stand in for tmux below it only through a new kit or exported seam, or else runs at tier2.

`pawlengine` and `tenderengine` declare seams so that the engine never imports the modules behind them.
Driving the real modules turns their rule tests into CLI tests, and the failure rows have no state that provokes them.

`shuttleparity`'s comparison is only interesting on manifests that differ, which two correct engines never produce.

An unreadable or truncated `reed.json` is both another module's malformed input and the state that provokes its error.
The rule allows neither a byte edit nor testing the refusal outside the parser's package.

A keystroke-reacting Claude pane (`tuiReed`, `restoreReed`) answers with screens composed from the input.
Picking and cutting recordings cannot express that.
