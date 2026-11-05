// Package driftengine owns regression around a task: the hub-wide `drift` config module, the drift paths, and the records and runs the Drift rows read and write.
// Regression is no test tier: it records a profile of the suite's behavior on the task's branch point and compares the task's code against it, and `lym gate test` never runs it.
//
// `drift.yaml` is a hub-wide config module with these keys: `enabled`, which switches the Drift rows on through their entry gate; `capture` and `compare`, each a command step written whole as `{command, dir, env, timeout_min}`, the compare running only under `diff: command`; `diff`, `lines` or `command`, how a run compares a capture with the baseline; `key_patterns`, the regular expressions with the named groups `key` and `value` that key a captured line, which with `diff` only the hub's file sets and a task's run settings are refused; `expect_model` and `judge_model`, the model-specs of the expect agent and the judge; `judge_interactive`, which makes the judge a session with the operator in its pane;
// and `scope`, a task's regression scope, which a task sets in its board entry's `run_settings`.
// `skip` and `verify_max_runs` are retired keys: ConfigSpec lists them in RetiredKeys, so a board entry whose `run_settings` still carries one seeds and the load drops it, and `lym config reconcile` prunes them from a hub base.
// LoadConfig refuses a scope other than empty, `off`, `partial` or `full`; it reads a stored `partial` as `full` and sets Config.ScopeWasPartial, so a base not yet edited still loads.
// A declared command holds the placeholder `{baseline_dir}`, the directory the capture writes its profile files to and the compare reads them from.
// A command still holding the retired `{skip_file}` placeholder is refused when it runs, naming the edit of that step in the hub's `drift.yaml`.
// A step's `dir` resolves by gateslot.ResolveCommandDir, so it means the directory a gate tier's `dir` means: relative to the worktree root, absolute as written, and the worktree's anchored directory when unset; every drift run is told both directories.
// Before a step's command starts, a `dir` that is no existing directory fails naming the resolved path, the `drift.yaml` key and, as the way forward, `lym config drift --set <step>.dir=<dir>` from the prime, then `lym drift capture` and `lym loom resume` for the capture and `lym loom resume` for the compare.
// CheckDirs refuses the same `dir` over the loaded config against a told worktree, which a hub-wide `lym config drift` write runs against the prime.
// A relative `dir` was once joined onto the anchored directory, so RebaseDirsOntoRoot, the module's reinterpretation DirBaseReinterpretation, carries a hub's drift.yaml across that move once per hub:
// a dir naming a directory under the prime's anchored directory is rewritten to name it from the root, one naming a directory under the root alone is kept as already written for it, and one naming neither is flagged with `lym config drift --set <step>.dir=<dir>`.
// `capture` and `compare` are executable keys, so only the template and the hub's board default layer set them, with `lym config drift --set 'capture={command: ..., timeout_min: N}'`.
// Once a step is declared, `--set` also changes one of its keys alone, as in `--set capture.env.NAME=1` or `--set compare.timeout_min=10`, and refuses a key a step lacks;
// a step left with keys but no command fails the load check, so a first declaration sets `command` and `timeout_min` together.
// LoadConfig loads the file strictly and refuses a declared step without `{baseline_dir}` or with a `timeout_min` below 1 and a model-spec that fails its grammar, each naming the key, and likewise a `diff` other than `lines` or `command`, a `key_patterns` entry CompileKeyPatterns refuses, and `diff: command` with no `compare` declared, which names `compare` as the key to set.
// MigrateConfig renames `baseline_model`, `verify_model` and `verify_interactive` to `expect_model`, `judge_model` and `judge_interactive`, keeping each value, and drift's registration runs it for a hub base at reconcile and for a task's run settings with a Warn.
// FlagDiffMode, the module's reinterpretation DiffModeReinterpretation, rewrites nothing and flags a `compare` set with no `diff`, a command holding `{skip_file}` and `scope: partial`, each with its way forward.
// Config.Step returns a declared step and refuses an undeclared one, so every row and verb that runs a command goes through it.
// Config.StepWithHubEnv, which every row and verb that runs a command calls, adds the env config module's variables beneath the step's own `env`, so a command runs with the inherited environment, then the hub's and this machine's variables, then the step's, each replacing the one before.
//
// Paths is the one declarer of every drift path.
// The durable directory `_lym/drift/` holds the scope and judged records, the expectation, the judgement file and the logs of every run; the ephemeral directory `.lym/drift/` holds the expect agent's identity listing, the judge request, the capture record, the capture lock, the compare marker, the baseline and current-capture directories.
// A profile file holds a test's profile when its path relative to its profile directory, with or without its last extension, is the test's name; a dotfile has no extension to drop, so its whole name is the test's name.
//
// Every record is JSON written through `internal/state`, so an unknown key survives a rewrite.
// The scope record (`scope.json`) holds the recorded scope (`off`, `partial` or `full`) and its skipped tests, when the baseline was awaited or deferred, and what the checks record, which the Records section lists.
// The old acceptance, compare and waiver fields of a record an earlier Drift wrote stay readable, for the verdict and for an in-flight `partial` scope.
// Verdict renders the `## Regression check` section a landing carries from the copies of `scope.json` and `judged.json` in the newest `Drift-Records` commit, read through ReadRecords, or from the files as they are on a run with no such commit, and runs no capture:
// the scope and, for `off`, its source; a deferred capture, saying no compare ran; for an old `partial` record, the cases its skip list left out; the diff mode, and under `command` that containment was the judge's;
// the last check's counts of changed, expected, matching, reused and unexpected cells; each expectation entry that covered no cell; each open range with its `why` and the judge's sentence; and each amendment with who made it.
// A record with no last check renders its old compare, acceptance and waiver fields under a line saying the check ran under the old Drift; with no scope record Verdict renders nothing and creates no directory.
// The capture record (`capture.json`) holds the state of the latest main capture (`running`, `passed` or `failed`) with its pid, HEAD, the tests it skipped, times, exit code, reason, whether it exited 0 with no file written, and log.
//
// # Capture
//
// Every capture and compare takes its gate slot before any lock, so a run waiting for a slot holds no lock and a lock holder always holds its slot.
// A run told the slot a gate holds around it, the request's InheritedSlot that the wiring reads from the process environment, runs in that slot when the pool reports it held and acquires none, as a command inside a gate does; otherwise it waits for one slot as a non-tier holder with no deadline.
// RunCapture is the main capture: it takes its slot, then the capture lock with a bounded non-blocking retry, so a reader's probe never makes it refuse, runs the caller's precondition under the lock and writes a `running` capture record naming the HEAD that precondition checked.
// It moves a non-empty baseline directory to the first free numbered backup beside it, and runs the declared command in its shell with `{baseline_dir}` rendered as a quoted path and the command's stdout and stderr the open log file.
// A timeout, counted from the command's start, kills the command's whole tree.
// The record ends `passed` for exit 0 with at least one file under the baseline directory, else `failed` with the reason, and then the slot and the lock are released.
// A failed record's reason ends naming the directory the command ran in and its last non-empty output lines from the log, folded onto one bounded line, or that it printed nothing, so a halt states the cause and not only the log path.
// An exit 0 with no file marks the record `wrote_no_file`.
// A told OnEnd is handed the final record while the lock is still held, so the capture child records a failed capture's FailedCaptureError, the reason, the log and the ways forward, in its own log and exits non-zero before the Drift-Await row may commit that log.
// RunCapture, CaptureCurrent and RunCompare refuse a command still holding the retired `{skip_file}` placeholder before they take a slot or a lock or move the baseline; CaptureCurrent returns the refusal as the reason of a result that did not pass.
// RunCapture holds no job object of its own, since it also runs in a row's step process; the detached child holds the capture's named job (CaptureJobName) around it.
// CaptureCurrent is the current capture every check runs: it creates a fresh directory under the current-capture directory named by the UTC stamp and the pid, so concurrent checks never share one, takes its slot, and then waits for the capture lock with no bound of its own, since a lock holder already holds its slot.
// It polls the lock through the request's Sleep from one second, doubling to thirty while the lock stays held, runs the declared command with `{baseline_dir}` rendered as that directory while it holds the lock, and releases the lock and the slot on return.
// It writes no capture record and never touches the baseline directory or its backups, and its log is a sibling of the directory.
// The result passes for exit 0 within the timeout with at least one file written; a failure, a timeout, no file and a step refused before its command started, such as a dir that is no directory, are results that did not pass, with the refusal naming its way forward as the reason, and only an I/O failure is an error.
// CheckMainCapture is the main capture's precondition: the run is at Drift-Baseline, or blocked at Drift-Await, HEAD equals the branch point and the tracked tree is clean; each refusal names its way forward.
// StartCapture spawns `drift capture-child` detached in the anchored directory, where a lym verb's cwd must be, with breakaway, registers it under the run's processes with role `capture` and returns its pid only once the capture record carries that pid, in any state, since a capture can fail between two reads.
// A child that ends or stalls before that record is an error carrying the last JSON error envelope's error from the child's log, folded onto one bounded line, and whether the child marked it with LastingFaultKey as one a fresh start in the same directory cannot change.
// A start Windows refuses for breakaway is retried plain-detached when the caller sets FallbackDetach, and is ErrBreakawayRefused otherwise.
// ProbeLock reads whether an OS lock is held without creating a directory, and Active reads the capture lock and the compare marker.
//
// # Diff
//
// DiffCaptures is the pure diff of two capture directories, over files with no config read: the caller passes the key patterns CompileKeyPatterns compiled from `drift.key_patterns`.
// ListCaptures reads every regular file under a directory, at any depth, as a case named by its slash-separated path, normalises CRLF to LF and drops the empty line a trailing newline leaves.
// The first pattern that matches a line with a non-empty `key` group keys it by that group, with the `value` group as its value; a line no pattern keys that way is unkeyed and its value is the whole line.
// An Identity names a line: a keyed line by its Key and its occurrence N counted from 1 among the file's lines holding that key, an unkeyed line by its Line position among the file's unkeyed lines.
// Key and occurrence are separate fields, so a key such as `Test #3` reads unambiguously, and a unique key keeps its cell when a keyed line is inserted before it.
// A Cell is a case added or removed, or a line changed, added or removed, with the baseline's value as Old and the current one as New.
// DiffCaptures returns the cells in case order and then file order, and skips a case it is told to leave out on both sides, which serves a run whose scope record skipped tests.
//
// # Expectation
//
// ParseExpectation is the sole reader of `_lym/drift/expected.yaml`, the expectation the expect agent writes.
// The file is agent-written, so it is decoded strictly and an unknown key is refused; an empty file is the empty expectation.
// Under `diff: lines` it holds `cases`, a map from case name to `keys`, each with a `key`, an optional `n`, an optional `added: true` and a `why`, and `lines`, each with a `range` of `12`, `12-20` or the open `12-` and a `why`, besides `added` and `removed`, lists of a `case` and a `why`.
// Under `diff: command` a case holds one `why` and nothing else.
// Every `why` must be non-blank, and a case, key and occurrence are checked against the baseline's listings, so the expect agent's `expectation` entry re-prompts on the refusal's text.
// Expectation.Cover returns the entry covering a cell and whether it is an open range, Expectation.Uncovering names the entries that cover no cell, and Expectation.Merge reads the sealed file and every amendment as one expectation.
//
// # Judgement
//
// ParseJudgement is the sole reader of `_lym/drift/judgement.yaml`, the rulings the judge writes; the file is a judge's output and no record, and Drift-Verify removes it once the rulings are stored in `judged.json`.
// It is strict like the expectation's parser.
// Under `diff: lines` the file holds a `spawn_id`, `cells`, each with an `id`, a `ruling` of `matches` or `does not match` and a `reason`, and `open_ranges`, each with a `case`, a `range` and what was `checked`, for every open range covering a judged cell.
// Under `diff: command` it holds the `spawn_id` and `cases`, each with a `case`, a `ruling` of `matches`, `does not match` or `unexpected` and a `reason`.
// JudgementWant carries what the spawn handed the judge, and the refusal lists every finding, since the judge's `judgement` gate entry hands it back as the findings to fix.
//
// # Judge
//
// JudgeTable builds the judge's seat: one lead on the `review` segment, role `drift-judge`, over the stencil `drift-template-judge`, with the model of `judge_model`, interactive under `judge_interactive`, where its mode text has it present each cell or case to the operator and record the operator's ruling.
// Its one output is judgement.yaml, and the table carries no gate: the row sets the `judgement` entry, NewJudgementGate, which parses the file with ParseJudgement against the JudgementWant of the JudgeRequest and fails with the parser's message.
// Drift-Verify writes the JudgeRequest, `.lym/drift/judge-request.json`, through WriteJudgeRequest before each spawn:
// the spawn id, the mode, the cells with their exact old and new value and covering why, the open ranges covering a judged cell or, under `command`, the compare's log, and the expectation and amendments as text.
// TakeJudgeSnapshot records under Deps.JudgeSnapshotDir both checkouts' HEAD, a content hash of every changed tracked path and every untracked path with a copy of each untracked file, and the hash of every file of the current capture's scratch directory; ReadJudgeSnapshot reads it back, found false when it is gone.
// JudgeSnapshot.Changes names what the judge session changed since: a moved code HEAD, each changed, added or deleted path of either checkout and each changed scratch file, judged by content so a later commit of content that was already there is no change.
// judgement.yaml is exempt, and so, on the records checkout, is every path under a Deps.LoomRecordsDirs directory or starting with one of its prefixes, which are the records paths loom writes during a row.
// A moved records HEAD counts only through the paths Deps.RecordsChangedSince names since the snapshot's head, so loom's own commits are never read as the judge's edits.
// Bound: an edit the judge session makes under the exempt loom paths goes unseen; they are loom's own run bookkeeping and hold no code, no expectation and neither record.
//
// # Records
//
// Two durable records are guarded: `scope.json` and `judged.json`, the judge's stored rulings as exact tuples of case, identity, kind, old value, new value, ruling and reason, its notes on open ranges with the covering entry's why, and under `command` its rulings per case and code head.
// `scope.json` also holds what the checks record: the config's `diff` and `key_patterns` as first seen, the source of a scope `off`, the two agent-session marks, the parent's amendments, the last check's result, the head and amendment count of the last passing check, and the `unsealed` and `snapshot_lost` marks; every one is optional, so a record written before it existed still reads.
// WriteRecords is the one writer: it starts from the copies ReadRecords returns, applies the caller's change, writes both files and commits both at once through Deps.CommitWithTrailers with the trailer `Drift-Records: <RecordsDigest of the written files>` and any extra trailer the caller names.
// ReadRecords finds the newest records commit carrying that trailer and returns both files as that commit holds them, with Mismatch set when the files on disk digest differently, which a hand edit or a write interrupted before its commit causes; the committed copy is returned either way.
// With no such commit it returns the files as they are, so a run seeded before the guard existed reads its records unguarded until its first write.
// WriteRecords refuses over a mismatch, and only RestoreRecords clears one: it writes both files back from the newest such commit, removes one that commit lacks, and adds no commit.
// `expected.yaml` and `judgement.yaml` lie beside the records under Paths and outside the guard: the expectation is sealed by its own trailer, and the judge's file is removed once its rulings are stored.
//
// # Seal
//
// SealExpectation commits `expected.yaml` alone with the trailer `Drift-Seal: <sha256>`, plus any extra trailer, such as the `Drift-Baseline` digest of a counted capture that has none yet.
// CheckSeal decides the expectation containment reads, and returns Sealed bytes, Empty, NotYet or a Fault naming its way forward, in this order:
// a `discard` amendment or an `unsealed` record is Empty with no check; a status row not past Drift-Expect, which PastRow decides from Deps.Rows, is NotYet; with no seal commit a missing file is Empty and a present one a Fault naming `lym drift amend <slug> --discard`;
// otherwise the oldest commit carrying the trailer is the seal, whose copy must hash to the trailer's value and whose code head must equal the baseline's, else a Fault naming `--discard`, and the file on disk must hash to the trailer's value, else a Fault naming `--restore` that keeps the seal's copy in Sealed.
// BaselineDigest hashes the baseline directory's tree, each regular file's slash-separated path and content in path order.
// CheckBaseline recomputes it and compares it with the digest the oldest commit carrying `Drift-Baseline` recorded, so an edit of the baseline after its capture counted is a Fault opening with DeferHaltPrefix and naming `lym drift defer`; a missing directory is the same Fault with or without such a commit, and with none the baseline is Unguarded, which skips only the compare.
//
// # Check
//
// Check is the containment every drift check runs, and it writes no tracked record and decides nothing: Drift-Verify maps its CheckReport to outcomes.
// It stops at the first step that ends the check:
// no scope record, scope off, a deferred capture and a waiver set Skip with the reason;
// a status row not past Drift-Expect sets Skip as not sealed yet, before any baseline check, since a real run has no awaited_at before Drift-Await counts the capture;
// a scope record with no awaited_at sets BaselineFault as a capture that never counted, and CheckBaseline's fault sets it for a missing directory or a changed digest, each naming `lym drift defer`;
// whether the capture counted is read from the tracked scope record alone, never from the capture record, which a cleaned `.lym` loses.
// A records mismatch sets RecordsFault naming `lym drift amend <slug> --restore` and the check goes on over the committed copies;
// a failed CheckSeal against the baseline's code head sets SealFault and the check goes on against the sealed copy, or an empty expectation when there is none.
// The sealed bytes and every amendment that adds entries are parsed by ParseExpectation under the mode and merged.
// The mode and key patterns are the scope record's, or the config's when the record holds none yet.
// Under `lines` the current capture is taken through Deps.CaptureCurrent, and one that did not pass sets CaptureFault naming its log and the way forward;
// the diff leaves out the cases of the scope record's skipped tests when its scope is the old `partial`, and each cell is classified by Expectation.Cover, with Expectation.Uncovering listing the entries no cell uses.
// Under `command` Deps.Compare runs the declared compare and the report carries its exit and log.
// CheckReport.Findings renders the report: one line per unexpected cell with its case, identity, old and new value, and each fault with its way forward.
// An identity names its occurrence only for a key that occurs more than once in its case.
// NewDriftGate returns the gate of the `drift` entry on a writer's gate list.
// It passes on a skip and on a baseline fault, which no writer session can clear.
// Under `command` it passes whatever the compare's exit.
// It logs the findings it passes with at Info.
// It fails on a capture fault and, under `lines`, on any unexpected cell, with the findings, which also name a records or seal fault found beside them.
// Its only write is the current capture's scratch directory, and Drift-Verify repeats the check as the guarantee.
//
// # Compare
//
// RunCompare is the mechanical compare, on the runner the capture uses.
// It takes the compare marker, an OS lock, before it waits for its gate slot and holds it until it ends, so a compare queued for a slot reads as activity and one a dead compare left reads as none; a marker still held after a bounded retry, which rides out a reader's probe, is refused.
// It holds one slot as a non-tier holder, renders `{baseline_dir}` as the baseline directory, and kills the command's tree at its timeout.
// The result carries the exit code, the log and whether the compare passed, which is exit 0 within the timeout, and a compare that did not pass carries a reason naming the directory and the last output as a failed capture's does; the compare writes no record.
//
// # Verbs
//
// The `lym drift` verbs' rules are functions over a VerbStatus: the run status's row, state and error, and whether the capture lock is held.
// A verb run at a row it does not admit refuses naming that row and `lym loom status`, and a refused verb writes nothing.
// Defer sets awaited_at with deferred through WriteRecords while blocked at Drift-Await, at a Drift-Baseline capture halt, or at a halt of Drift-Expect, a writer row, Drift-Verify or Drift-Recheck whose error, past an optional leading decision halt prefix, opens with DeferHaltPrefix or, on a run whose scope is not off and holds no deferral, whose baseline check fails;
// it refuses while the capture lock is held, which a running capture, a `lym drift check` and the drift gate's check all take, naming `lym loom resume` and, for a check, waiting for it to finish.
// Amend records the parent's amendment through WriteRecords in one commit, at a blocked writer row, Drift-Verify or Drift-Recheck, and at a blocked Drift-Expect for `--restore` alone, and never edits the seal commit.
// A scope that is off admits `--restore` alone, since records that differ from their commit need writing back whatever the scope.
// An AmendRequest holds exactly one of Entries, Restore and Discard, and Check refuses any other shape naming `--entries`, `--restore` and `--discard` before anything is read.
// Entries are parsed by ParseExpectation under the recorded mode against the baseline's listing, so an entry it refuses is refused with its message, and the stored rulings of every cell they cover, under `command` of every case they name, are dropped from `judged.json`.
// Restore writes the seal commit's copy back over an expected.yaml edited after the seal, and RestoreRecords over records that differ from their commit, and is refused when neither failed.
// Discard records that the expectation is set aside, so CheckSeal returns the empty expectation, and containment then reads the amendments alone.
//
// # Agents
//
// The expect agent and the judge are each one lead seat, built by ExpectTable and JudgeTable over the stencils `drift-template-expect` and `drift-template-judge`, which the seat engine reads at call time and renders with the parent and edit directives.
// The expect seat is never interactive; the judge seat is interactive under `judge_interactive`.
// The expect seat's only output is `expected.yaml` and the judge's `judgement.yaml`: the records are Go-written, and a fresh spawn archives its outputs.
// The task's subject is read from the decision record in a tapestry run (SubjectDecisionRecord) and from the board entry in a darn run (SubjectBoardEntry), the record's `Added after Discussion` entries winning in both.
// The seats take the `plan` and `review` segment colors, their models from `expect_model` and `judge_model`, and session timeouts that are package constants, since drift.yaml declares no timeout key.
// Neither table carries a gate; the Drift-Expect and Drift-Verify rows set theirs.
// `drift-baseline` and `drift-verify` are the roles of two removed agents and have no seat, stencil or table.
//
// # Drift-Baseline
//
// Deps holds the seams the Drift producers share, and a producer fails loudly naming the field when a seam it reads is nil.
// Row holds a row's portable config: the subject its agent reads the task from.
// Each Drift producer reads `drift.enabled` at every pass and is Done at once, committing nothing, when it is off: the entry gate reads the switch only when the run first enters a row, so this lets a resume pass a row already entered once the switch goes off, in the hub or in the task's local layer.
// A drift config that fails to load there is a Stuck naming `lym loom resume`.
// DriftBaseline spawns no agent and writes no `baseline.md`.
// It first ends, through Deps.EndRoles, every live strand of the removed `drift-baseline` and `drift-verify` agents, so no pane keeps an old stencil, and then records the scope once through WriteRecords when none is recorded:
// `off` when `drift.scope` is `off`, with ScopeSource `drift.scope`, and `full` otherwise, with the config's `diff` and `key_patterns`.
// A config scope read from `partial` is recorded `full` with one Warn naming the key.
// A recorded scope, `off`, `full` or an old `partial`, is kept, and a later config change changes neither it nor the recorded mode and patterns.
// Scope off, a set awaited_at and a passed capture take no capture; a capture holding the lock is left running.
// Otherwise it captures again: an undeclared capture command is a Stuck naming `lym config drift`, and a refusal of the capture's precondition is a Stuck whose reason opens with CaptureHaltPrefix and then DeferHaltPrefix and names `lym drift defer` with `lym loom resume`, and on a dirty tree restoring or stashing the listed paths.
// The capture is the detached child: the pass returns Done once the child's record exists, and the recipe's Drift-Await row waits for the capture to end;
// a child that does not start is a Stuck opening with CaptureHaltPrefix and DeferHaltPrefix and carrying the child's error, naming `lym loom resume` and `lym drift defer` with `lym loom resume`, or, for an error the child marked lasting, saying a resume cannot change it and naming only `lym drift defer` with `lym loom resume` and a fix of the cause;
// when Windows refuses the detached start breakaway, the capture runs in the step's own process, so its command dies with the step.
// A failed capture is a Stuck opening with CaptureHaltPrefix and DeferHaltPrefix and naming the log, `lym drift capture` with `lym loom resume`, and `lym drift defer` with `lym loom resume`; the row carries no on_stuck, so each Stuck halts the run.
// The scope write is committed with the records; a pass commits through Deps.Commit only once a capture has ended in this process, and then all of the durable drift directory.
//
// # Drift-Expect
//
// DriftExpect seals the expectation of changes before the writer starts, over Deps.ExpectTable, CommitsSince and the records and seal seams.
// ExpectTable builds one non-interactive lead on the `plan` segment, role `drift-expect`, over the stencil `drift-template-expect`, with the model of `expect_model`, whose one output is expected.yaml; the agent reads the identity listing Paths.Listing holds, the baseline and the task's record and plan.
// It is Done at once, spawning nothing, when drift is off, no scope record exists, an expectation is already sealed, the scope is off or the capture was deferred;
// the last two record `unsealed` first, so containment reads the empty expectation.
// A capture that never counted, a baseline that is missing or no longer matches its digest, and a code HEAD other than the capture's head, which the scope record's `baseline_head` names and capture.json names when the record holds none, are each a Stuck for the run's parent.
// Each opens with shedadapters.DecisionHaltPrefix and then DeferHaltPrefix and names `lym drift defer`; the moved HEAD also lists the commits since and names the operator's reset of the run's code branch to the capture's head as the other way forward.
// A scope record with no diff mode records the config's `diff` and `key_patterns` first, which keeps them for the run.
// A pass that goes on to write over records differing from their newest records commit is a Stuck for the run's parent opening with shedadapters.DecisionHaltPrefix and naming `lym drift amend <slug> --restore`, which Amend admits at the blocked row for `--restore` alone.
// A closed `expect_session` mark that kept its `opened_at`, which an agent return without an error leaves and which records an ended session, with an expected.yaml that validates is sealed with no spawn; an open mark, a mark never opened or closed after an error, or a file that does not validate, runs the agent, resuming a live chair.
// The pass writes the listing before the agent runs: every baseline case's lines with their identities and text under `diff: lines`, the case names alone under `diff: command`; an occurrence `n` is written only for a key that repeats in its case.
// The agent's table is gated by the row's `expectation` entry, NewExpectationGate, which parses expected.yaml with ParseExpectation under the recorded mode against the baseline's cases and re-prompts the agent with the parser's message; a missing file fails asking for it.
// The session's return closes the mark, a Done seals the file through SealExpectation with the baseline's digest as a second trailer when no commit recorded it yet, and an exhausted entry is the shed's Stuck, whose resume spawns the agent again.
//
// # Drift-Await
//
// DriftAwait is the mechanical row that holds the run until the baseline exists; it spawns nothing and commits all of the durable drift directory, the capture log included, on every way out.
// It is Done with no scope record, with scope off, and with awaited_at set whatever HEAD is, so a later entry after the code changed passes.
// A capture holding the lock is waited on by awaitCaptureEnd with no bound of its own, and the capture record is read only after the lock reads free, so a capture that just ended is read as ended.
// A passed capture taken at HEAD is Done after the row writes awaited_at into the scope record through WriteRecords, which makes the capture the counted one;
// that records commit carries the extra trailer `Drift-Baseline: <BaselineDigest of the baseline directory>`, so the digest lives in a records commit made before the writer.
// A missing record, a failed one, and a running one whose lock is free are Stuck, naming the log when there is one, as is a passed record taken at another commit than HEAD.
// While HEAD is the branch point a Stuck names `lym drift capture` followed by `lym loom resume`, which captures again and re-enters the row, and `lym drift defer` followed by `lym loom resume`;
// once HEAD moved it names only `lym drift defer` followed by `lym loom resume`, since no capture may run on changed code.
// No Stuck of the row carries the decision halt prefix, so the arbiter may settle one with the way forward it names.
// An adopting machine has no `.lym/drift/`, so before awaited_at the row halts on the missing record and `lym drift capture` captures there.
//
// # Drift-Verify
//
// DriftVerify is the containment check and the judge: it checks the code's captured changes against the sealed expectation and has one judge rule on the expected changes no stored ruling covers.
// It first ends, through Deps.EndRoles, every live strand of the removed `drift-baseline` and `drift-verify` agents, so no pane keeps an old stencil, and then runs these steps in order:
// no scope record, scope off, a deferred capture and a waiver are Done, and a records mismatch is a Stuck naming `lym drift amend <slug> --restore`;
// a scope record with no diff mode records the config's `diff` and `key_patterns` and keeps them;
// a capture that never counted, a missing baseline directory and an edited baseline are a Stuck naming `lym drift defer`, the first read from the tracked scope record alone;
// a head equal to the last passing check's with the same number of amendments is Done;
// an open `judge_session` mark is settled before anything else: a gone snapshot is recorded as lost and is a Stuck, a change JudgeSnapshot.Changes names is a Stuck, and an intact session goes on to be resumed;
// a run past Drift-Expect with neither a seal commit nor an expected.yaml records `unsealed` once;
// then Check runs, and a seal fault, a failed current capture and any cell the expectation does not cover are a Stuck;
// each cell whose exact tuple has a stored ruling reuses it, a stored `does not match` is a Stuck, and with no cell left to judge the pass is recorded and Done.
//
// Otherwise one judge is spawned: Drift-Verify removes a stale judgement.yaml, writes the JudgeRequest under a fresh spawn id, opens the session mark through WriteRecords, takes the snapshot and runs the judge through the seat engine, its table gated by the row's `judgement` entry.
// A session an earlier entry left open keeps its spawn id and its snapshot: the request is written again under the recorded id, a live chair is resumed and none is respawned, and a snapshot recorded lost is taken anew first.
// On the session's return, before the mark closes, a change to either checkout or to the scratch directory and a baseline that no longer matches its digest are a Stuck that leaves the mark open.
// The rulings are stored in judged.json as exact tuples with the judge's strand and time, each listed open range with the covering entry's why and the judge's sentence, replacing an earlier listing of the same range; the mark closes and the last check is recorded in the same write, and judgement.yaml is removed after it.
// A ruling of `does not match` is a Stuck naming the cells and their reasons, and all `matches` records the pass and is Done.
//
// Under a recorded `command` diff the steps run as far as Check, which runs the declared compare against the baseline directory in place of a capture and a diff, and the pass goes on by the compare's exit:
// an exit 0 records the pass and is Done with no judge;
// a nonzero exit at a code head whose stored case rulings in `judged.json` include a `does not match` or an `unexpected`, with no amendment recorded since they were stored, is a Stuck naming them, with no new judge;
// otherwise one judge is spawned under the same snapshot guard, its request carrying the compare's log, the sealed expectation and every amendment as text, and its `judgement` entry checks the `command` form, so a judge that rules no case after a failing compare is refused.
// A ruling of `matches` is not stored; each `does not match` and `unexpected` ruling is stored as a case ruling keyed by code head and case with its reason, and any of them is a Stuck naming the cases, their reasons, the parent's `lym drift amend <slug>` and the route that fixes code.
// All `matches` records the pass, which counts as passing for the skip of a head already checked.
// Bound: `command` gives up containment by code, and a drifted case the log does not show goes unjudged.
//
// The route that fixes code, which every Stuck over a regression names, is derived from Deps.Rows: `lym loom goto --to Darn-Write` then `lym loom resume` when the rows hold Darn-Write, else `lym loom goto --to Webster` then `lym loom resume`.
// A re-entered Webster with every card done still starts Merriam, whose run-level `drift` entry meets the cells and hands them to the verify fixer.
// Every Stuck for the parent opens with shedadapters.DecisionHaltPrefix, and one that names `lym drift defer` carries DeferHaltPrefix right after it.
// An exhausted `judgement` entry is the shed's Stuck, whose resume respawns the judge under the same spawn id.
// The row carries no on_stuck, and the judge edits no code, expectation or record.
// The Drift-Recheck row is this engine in a second row, after the rows that may commit code once Drift-Verify passed, with the same config and no on_stuck, so its Stuck goes through the stop chain to the parent as Drift-Verify's does.
// A head already checked with no amendment since is Done with no capture.
package driftengine
