// Package driftengine owns regression around a task:
// the hub-wide `drift` config module, the drift paths, and the records and runs the Drift rows read and write.
// Regression is no test tier.
// It records a profile of the suite's behavior on the task's branch point and compares the task's code against it.
// `lym gate test` never runs it.
//
// `drift.yaml` is a hub-wide config module with these keys:
// `enabled`, which switches the Drift rows on through their entry gate;
// `capture` and `compare`, each a command step written whole as `{command, dir, env, timeout_min}`;
// the compare runs only under `diff: command`;
// `diff`, `lines` or `command`, how a run compares a capture with the baseline;
// `key_patterns`, the regular expressions with the named groups `key` and `value` that key a captured line;
// `volatile_files` and `volatile_keys`, the capture files and line keys the diff ignores;
// `expect_model` and `judge_model`, the model-specs of the expect agent and the judge;
// `judge_interactive`, which makes the judge a session with the operator in its pane;
// and `scope`, a task's regression scope, which a task sets in its board entry's `run_settings`.
// A hub's harness may write provenance that differs in every capture, and lym hard-codes no name of it.
// `diff`, `key_patterns`, `volatile_files` and `volatile_keys` are guard keys:
// only the hub's file sets take them, and a task's run settings are refused them.
// `skip` and `verify_max_runs` are retired keys that ConfigSpec lists in RetiredKeys.
// A board entry whose `run_settings` carries one seeds and the load drops the key;
// `lym config reconcile` prunes them from a hub base.
// LoadConfig refuses a scope other than empty, `off`, `partial` or `full`;
// it reads a stored `partial` as `full` and sets Config.ScopeWasPartial, so a base not yet edited still loads.
// A declared command holds the placeholder `{baseline_dir}`:
// the directory the capture writes its profile files to and the compare reads them from.
// A command holding the retired `{skip_file}` placeholder is refused when it runs.
// CheckRetiredPlaceholders refuses the same command before the run starts while drift is enabled,
// and the Preflight row of a recipe with Drift rows runs it.
// The refusal names `lym config reconcile --layer default --apply` and the edit of that step as the way forward.
// It addresses the operator or the hub orch, since a task or beam session is refused hub-wide config writes.
// A step's `dir` resolves by gateslot.ResolveCommandDir and means what a gate tier's `dir` means:
// relative to the worktree root, absolute as written, and the worktree's anchored directory when unset.
// Every drift run is told both directories.
// A `dir` that is no existing directory fails before the step's command starts.
// The failure names the resolved path and the `drift.yaml` key.
// The way forward is `lym config drift --set <step>.dir=<dir>` from the prime.
// A capture then needs `lym drift capture` and `lym loom resume`, and a compare needs `lym loom resume`.
// CheckDirs refuses the same `dir` over the loaded config against a told worktree;
// a hub-wide `lym config drift` write runs it against the prime.
// RebaseDirsOntoRoot, the module's reinterpretation DirBaseReinterpretation, runs once per hub.
// It moves a relative `dir` written against the anchored directory onto the root.
// A dir naming a directory under the prime's anchored directory is rewritten to name it from the root.
// One naming a directory under the root alone is kept as already written for it.
// One naming neither is flagged with `lym config drift --set <step>.dir=<dir>`.
// `capture` and `compare` are executable keys: only the template and the hub's board default layer set them.
// A first declaration sets a step whole, as in `lym config drift --set 'capture={command: ..., timeout_min: N}'`.
// Once a step is declared, `--set` also changes one of its keys alone and refuses a key the step lacks.
// Examples are `--set capture.env.NAME=1` and `--set compare.timeout_min=10`.
// A step left with keys but no command fails the load check;
// a first declaration therefore sets `command` and `timeout_min` together.
// LoadConfig loads the file strictly and refuses each of these, naming the key:
// a declared step without `{baseline_dir}`;
// a `timeout_min` below 1;
// a model-spec that fails its grammar;
// a `diff` other than `lines` or `command`;
// a `key_patterns` entry CompileKeyPatterns refuses;
// and `diff: command` with no `compare` declared, which names `compare` as the key to set.
// MigrateConfig renames `baseline_model` to `expect_model` and `verify_model` to `judge_model`.
// It renames `verify_interactive` to `judge_interactive`.
// It keeps each value.
// It removes each whole `-e NAME={skip_file}` argument, in its `--env` forms too, from the capture and compare commands.
// It leaves an argument that ends a quoted shell string, as in `sh -c 'docker run -e A={skip_file}'`.
// The quote that closes the string follows such an argument, so FlagDiffMode names it instead.
// Drift's registration runs MigrateConfig for a hub base at reconcile, the walk of a new build included.
// It also runs it for a task's run settings, with a Warn.
// FlagDiffMode, the module's reinterpretation DiffModeReinterpretation, rewrites nothing.
// It flags a `compare` set with no `diff`, a command holding `{skip_file}` and `scope: partial`.
// Each flag carries its way forward.
// Config.Step returns a declared step and refuses an undeclared one.
// Config.StepWithHubEnv adds the env config module's variables beneath the step's own `env`.
// Every row and verb that runs a command calls it.
// A command runs with the inherited environment, then the hub's and this machine's variables, then the step's.
// Each replaces the one before.
//
// Paths is the one declarer of every drift path.
// The durable directory `_lym/drift/` holds the scope and judged records and the expectation.
// It also holds the judgement file and the logs of every run.
// The ephemeral directory `.lym/drift/` holds the expect agent's identity listing and the judge request.
// It also holds the capture record, the capture lock and the compare marker.
// The baseline and current-capture directories live there too.
// A profile file holds a test's profile when its path relative to its profile directory is the test's name.
// The path may omit its last extension.
// A dotfile has no extension to drop, so its whole name is the test's name.
//
// Every record is JSON written through `internal/state`, so an unknown key survives a rewrite.
// The scope record (`scope.json`) holds the recorded scope (`off`, `partial` or `full`) and its skipped tests.
// It also holds when the baseline was awaited or deferred and what the checks record, which the Records section lists.
// Legacy acceptance, compare and waiver fields stay readable, for the verdict and for an in-flight `partial` scope.
// Verdict renders the `## Regression check` section a landing carries and runs no capture.
// It reads the copies of `scope.json` and `judged.json` in the newest `Drift-Records` commit through ReadRecords,
// or the files as they are on a run with no such commit.
// The section holds:
// the scope and, for `off`, its source;
// a deferred capture, saying no compare ran;
// for a legacy `partial` record, the cases its skip list left out;
// the diff mode, and under `command` that containment was the judge's;
// the last check's counts of changed, expected, matching, reused and unexpected cells;
// each expectation entry that covered no cell;
// each open range with its `why` and the judge's sentence;
// and each amendment with who made it.
// A record with no last check renders its legacy compare, acceptance and waiver fields.
// A line above them says the check ran under the earlier Drift.
// With no scope record Verdict renders nothing and creates no directory.
// The capture record (`capture.json`) holds the state of the latest main capture (`running`, `passed` or `failed`).
// It also holds the pid, HEAD, skipped tests, times, exit code, reason and log.
// A flag records that it exited 0 with no file written.
//
// # Capture
//
// Every capture and compare takes its gate slot before any lock;
// a run waiting for a slot holds no lock, and a lock holder always holds its slot.
// A request's InheritedSlot is the slot a gate holds around the run, read by the wiring from the process environment.
// When the pool reports that slot held, the run uses it and acquires none, as a command inside a gate does;
// otherwise it waits for one slot as a non-tier holder with no deadline.
// RunCapture is the main capture.
// It takes its slot, then the capture lock with a bounded non-blocking retry that rides out a reader's probe.
// Under the lock it runs the caller's precondition,
// and it writes a `running` capture record naming the HEAD that precondition checked.
// It moves a non-empty baseline directory to the first free numbered backup beside it.
// It then runs the declared command in its shell, with `{baseline_dir}` rendered as a quoted path.
// The command's stdout and stderr go to the open log file.
// A timeout, counted from the command's start, kills the command's whole tree.
// The record ends `passed` for exit 0 with a file under the baseline directory, else `failed` with the reason.
// The slot and the lock are released afterward.
// A failed record's reason ends naming the directory the command ran in.
// It then gives the last non-empty log lines, folded onto one bounded line, or says the command printed nothing.
// A halt thus states the cause and not only the log path.
// An exit 0 with no file marks the record `wrote_no_file`.
// A told OnEnd is handed the final record while the lock is still held.
// The capture child uses it to record a failed capture's FailedCaptureError in its own log.
// The error carries the reason, the log and the ways forward.
// The child then exits non-zero before the Drift-Await row may commit that log.
// RunCapture, CaptureCurrent and RunCompare refuse a command holding the retired `{skip_file}` placeholder.
// They refuse before taking a slot or a lock or moving the baseline.
// CaptureCurrent returns the refusal as the reason of a result that did not pass.
// RunCapture holds no job object of its own, since it also runs in a row's step process;
// the detached child holds the capture's named job (CaptureJobName) around it.
// CaptureCurrent is the current capture every check runs.
// It creates a fresh directory under the current-capture directory named by the UTC stamp and the pid.
// Concurrent checks therefore never share one.
// It takes its slot, then waits for the capture lock with no bound of its own;
// a lock holder already holds its slot.
// It polls the lock through the request's Sleep from one second, doubling to thirty while the lock stays held.
// It runs the declared command with `{baseline_dir}` rendered as that directory while it holds the lock.
// It releases the lock and the slot on return.
// It writes no capture record and never touches the baseline directory or its backups.
// Its log is a sibling of the fresh directory.
// The result passes for exit 0 within the timeout with at least one file written.
// A failure, a timeout, no file and a refused step are results that did not pass.
// A step is refused before its command starts when, for example, its dir is no directory.
// A refusal names its way forward as the reason, and only an I/O failure is an error.
// CheckMainCapture is the main capture's precondition:
// the run is at Drift-Baseline, or blocked at Drift-Await, HEAD equals the branch point and the tracked tree is clean;
// each refusal names its way forward.
// StartCapture spawns `drift capture-child` detached, with breakaway, in the anchored directory.
// The anchored directory is where a lym verb's cwd must be.
// It registers the child under the run's processes with role `capture`.
// It returns the pid only once the capture record carries it, in any state, since a capture can fail between two reads.
// A child that ends or stalls before that record is an error.
// The error carries the last JSON error envelope's error from the child's log, folded onto one bounded line.
// The child may mark the fault with LastingFaultKey as one a fresh start in the same directory cannot change.
// The error records whether it did.
// A start Windows refuses for breakaway is retried plain-detached when the caller sets FallbackDetach;
// otherwise it fails with ErrBreakawayRefused.
// ProbeLock reads whether an OS lock is held without creating a directory.
// Active reads the capture lock and the compare marker.
//
// # Diff
//
// DiffCaptures is the pure diff of two capture directories, over files with no config read:
// the caller passes the key patterns CompileKeyPatterns compiled from `drift.key_patterns`.
// ListCaptures reads every regular file under a directory, at any depth, as a case named by its slash-separated path.
// It normalises CRLF to LF and drops the empty line a trailing newline leaves.
// The first pattern that matches a line with a non-empty `key` group keys the line by that group.
// The `value` group becomes its value.
// A line no pattern keys that way is unkeyed, and its value is the whole line.
// An Identity names a line:
// a keyed line by its Key and its occurrence N counted from 1 among the file's lines holding that key;
// an unkeyed line by its Line position among the file's unkeyed lines.
// Key and occurrence are separate fields, so a key such as `Test #3` reads unambiguously.
// A unique key keeps its cell when a keyed line is inserted before it.
// A Cell is a case added or removed, or a line changed, added or removed.
// Old holds the baseline's value and New the current one.
// DiffCaptures returns the cells in case order and then file order.
// It skips a case it is told to leave out on both sides, which serves a run whose scope record skipped tests.
// Volatile holds the `volatile_files` and `volatile_keys` lists:
// a file entry is a glob over a case's file name at any depth, or over its whole path when it holds a slash;
// a key entry names a line key in every case.
// Check drops their cells after the diff, and the expect agent's listing leaves them out.
// The baseline digest still covers them.
// Under `diff: command` the declared compare decides alone.
//
// # Expectation
//
// ParseExpectation is the sole reader of `_lym/drift/expected.yaml`, the expectation the expect agent writes.
// The file is agent-written, so it is decoded strictly and an unknown key is refused;
// an empty file is the empty expectation.
// Under `diff: lines` it holds `cases`, a map from case name to `keys` and `lines`, besides `added` and `removed`.
// Each of `keys` has a `key`, an optional `n`, an optional `added: true` and a `why`.
// Each of `lines` has a `range` of `12`, `12-20` or the open `12-` and a `why`.
// `added` and `removed` are lists of a `case` and a `why`.
// Under `diff: command` a case holds one `why` and nothing else.
// Every `why` must be non-blank, and a case, key and occurrence are checked against the baseline's listings.
// The expect agent's `expectation` entry re-prompts on the refusal's text.
// Expectation.Cover returns the entry covering a cell and whether it is an open range.
// Expectation.Uncovering names the entries that cover no cell.
// Expectation.Merge reads the sealed file and every amendment as one expectation.
//
// # Judgement
//
// ParseJudgement is the sole reader of `_lym/drift/judgement.yaml`, the rulings the judge writes.
// The file is the judge's output and no record: Drift-Verify removes it once the rulings are stored in `judged.json`.
// It is strict like the expectation's parser.
// Under `diff: lines` the file holds a `spawn_id`, `cells` and `open_ranges`.
// Each of `cells` has an `id`, a `ruling` of `matches` or `does not match` and a `reason`.
// `open_ranges` lists every open range covering a judged cell, each with a `case`, a `range` and what was `checked`.
// Under `diff: command` it holds the `spawn_id` and `cases`.
// Each of `cases` has a `case`, a `ruling` of `matches`, `does not match` or `unexpected` and a `reason`.
// JudgementWant carries what the spawn handed the judge.
// The refusal lists every finding, since the judge's `judgement` gate entry hands it back as the findings to fix.
//
// # Judge
//
// JudgeTable builds the judge's seat.
// It is one lead on the `review` segment, role `drift-judge`, over the stencil `drift-template-judge`.
// Its model is that of `judge_model`.
// Under `judge_interactive` the seat is interactive:
// its mode text has it present each cell or case to the operator and record the operator's ruling.
// Its one output is judgement.yaml, and the table carries no gate.
// The row sets the `judgement` entry, NewJudgementGate.
// It parses the file with ParseJudgement against the JudgementWant of the JudgeRequest.
// A refusal fails the entry with the parser's message.
// Drift-Verify writes the JudgeRequest, `.lym/drift/judge-request.json`, through WriteJudgeRequest before each spawn:
// the spawn id and the mode;
// the cells, with their exact old and new value and covering why;
// the open ranges covering a judged cell or, under `command`, the compare's log;
// and the expectation and amendments as text.
// TakeJudgeSnapshot records under Deps.JudgeSnapshotDir:
// both checkouts' HEAD;
// a content hash of every changed tracked path and every untracked path, with a copy of each untracked file;
// and the hash of every file of the current capture's scratch directory.
// ReadJudgeSnapshot reads it back, found false when it is gone.
// A new snapshot clears only the copies and replaces the record in one atomic write;
// a reader therefore never finds the snapshot gone while one is taken.
// JudgeSnapshot.Changes names what the judge session changed since the snapshot:
// a moved code HEAD, each changed, added or deleted path of either checkout, and each changed scratch file.
// Changes are judged by content, so a later commit of content that was already there is no change.
// Exempt are judgement.yaml and its timestamped siblings.
// The siblings keep an earlier judge's file when the next judge starts.
// On the records checkout, what lym writes itself around a session is exempt too.
// That covers every path under a Deps.LoomRecordsDirs directory or starting with one of its prefixes.
// Those are the records paths loom writes during a row, such as the usage records of its sessions.
// It covers the drift logs, which a check under `command` adds to.
// It covers `scope.json` and `judged.json` while both match their newest `Drift-Records` commit.
// Every lym write of those two lands in such a commit.
// A hand edit or an unstamped commit of either is therefore still the judge's.
// A moved records HEAD counts only through the paths Deps.RecordsChangedSince names since the snapshot's head.
// Loom's own commits are therefore never read as the judge's edits.
// Bound: an edit the judge session makes under the exempt loom paths or the drift logs goes unseen;
// they are lym's own run bookkeeping and hold no code, no expectation and neither record.
//
// # Records
//
// Two durable records are guarded: `scope.json` and `judged.json`.
// `judged.json` holds the judge's stored rulings as exact tuples:
// case, identity, kind, old value, new value, ruling and reason.
// It also holds the judge's notes on open ranges with the covering entry's why.
// Under `command` it also holds the judge's rulings per case and code head.
// `scope.json` also holds what the checks record:
// the config's `diff`, `key_patterns` and volatile lists as first seen;
// the source of a scope `off`;
// the agent-session marks;
// the parent's amendments;
// the last check's result;
// the head and amendment count of the last passing check;
// and the `unsealed` and `snapshot_lost` marks.
// Every one is optional, so a record written without it still reads.
// WriteRecords is the one writer.
// It starts from the copies ReadRecords returns, applies the caller's change and writes both files.
// It commits both at once through Deps.CommitWithTrailers.
// The trailer is `Drift-Records: <RecordsDigest of the written files>`, plus any extra trailer the caller names.
// ReadRecords finds the newest records commit carrying that trailer and returns both files as that commit holds them.
// It sets Mismatch when the files on disk digest differently.
// A hand edit or a write interrupted before its commit causes that.
// The committed copy is returned either way.
// With no such commit it returns the files as they are.
// A run seeded without the guard thus reads its records unguarded until its first write.
// WriteRecords refuses over a mismatch, and only RestoreRecords clears one.
// RestoreRecords writes both files back from the newest such commit, removes one that commit lacks, and adds no commit.
// `expected.yaml` and `judgement.yaml` lie beside the records under Paths and outside the guard:
// the expectation is sealed by its own trailer, and the judge's file is removed once its rulings are stored.
//
// # Seal
//
// SealExpectation commits `expected.yaml` alone with the trailer `Drift-Seal: <sha256>`, plus any extra trailer.
// An extra trailer is, for example, the `Drift-Baseline` digest of a counted capture that has none yet.
// CheckSeal decides the expectation containment reads.
// It returns Sealed bytes, Empty, NotYet or a Fault naming its way forward, in this order:
// a `discard` amendment or an `unsealed` record is Empty with no check;
// a status row not past Drift-Expect, which PastRow decides from Deps.Rows, is NotYet;
// with no seal commit, a missing file is Empty and a present one a Fault naming `lym drift amend <slug> --discard`;
// otherwise the oldest commit carrying the trailer is the seal;
// its copy must hash to the trailer's value, else a Fault naming `--discard`;
// its code head must equal the baseline's, else the same Fault;
// the file on disk must hash to the trailer's value, else a Fault naming `--restore`;
// that Fault keeps the seal's copy in Sealed.
// BaselineDigest hashes the baseline directory's tree:
// each regular file's slash-separated path and content, in path order.
// CheckBaseline recomputes the baseline digest and compares it with the one the oldest `Drift-Baseline` commit recorded.
// An edit of the baseline after its capture counted is a Fault.
// The Fault opens with DeferHaltPrefix and names `lym drift defer`.
// A missing directory is the same Fault with or without such a commit.
// With no such commit the baseline is Unguarded, which skips only the compare.
//
// # Check
//
// Check is the containment every drift check runs.
// It writes no tracked record and decides nothing; Drift-Verify maps its CheckReport to outcomes.
// It stops at the first step that ends the check:
// no scope record, scope off, a deferred capture and a waiver set Skip with the reason;
// a status row not past Drift-Expect sets Skip as not sealed yet, before any baseline check;
// the order holds because a real run has no awaited_at before Drift-Await counts the capture;
// a scope record with no awaited_at sets BaselineFault as a capture that never counted;
// CheckBaseline's fault sets it for a missing directory or a changed digest;
// each names `lym drift defer`.
// Whether the capture counted is read from the tracked scope record alone, never from the capture record.
// A cleaned `.lym` loses the capture record.
// A records mismatch sets RecordsFault naming `lym drift amend <slug> --restore`,
// and the check goes on over the committed copies;
// a failed CheckSeal against the baseline's code head sets SealFault,
// and the check goes on against the sealed copy, or an empty expectation when there is none.
// The sealed bytes and every amendment that adds entries are parsed by ParseExpectation under the mode and merged.
// The mode, key patterns and volatile lists are the scope record's, or the config's when the record holds no mode yet.
// A run thus keeps the lists it started with.
// A run started before they were set still needs `lym drift amend`.
// Under `lines` the current capture is taken through Deps.CaptureCurrent.
// A capture that did not pass sets CaptureFault naming its log and the way forward;
// the diff leaves out the cases of the scope record's skipped tests when its scope is the legacy `partial`;
// each cell is classified by Expectation.Cover, with Expectation.Uncovering listing the entries no cell uses.
// Under `command` Deps.Compare runs the declared compare and the report carries its exit and log.
// CheckReport.Findings renders the report:
// one line per unexpected cell with its case, identity, old and new value, and each fault with its way forward.
// An identity names its occurrence only for a key that occurs more than once in its case.
// NewDriftGate returns the gate of the `drift` entry on a writer's gate list.
// It passes on a skip and on a baseline fault, which no writer session can clear.
// Under `command` it passes whatever the compare's exit.
// It logs the findings it passes with at Info.
// It fails on a capture fault and, under `lines`, on any unexpected cell.
// The failure carries the findings, which also name a records or seal fault found beside them.
// Its only write is the current capture's scratch directory, and Drift-Verify repeats the check as the guarantee.
//
// # Compare
//
// RunCompare is the mechanical compare, on the runner the capture uses.
// It takes the compare marker, an OS lock, before it waits for its gate slot, and holds it until it ends.
// A compare queued for a slot thus reads as activity, and a marker a dead compare left reads as none.
// A marker still held after a bounded retry, which rides out a reader's probe, is refused.
// It holds one slot as a non-tier holder and renders `{baseline_dir}` as the baseline directory.
// It kills the command's tree at its timeout.
// The result carries the exit code, the log and whether the compare passed, which means exit 0 within the timeout.
// A compare that did not pass carries a reason naming the directory and the last output, as a failed capture's does.
// The compare writes no record.
//
// # Verbs
//
// The `lym drift` verbs' rules are functions over a VerbStatus:
// the run status's row, state and error, and whether the capture lock is held.
// A verb run at a row it does not admit refuses naming that row and `lym loom status`.
// A refused verb writes nothing.
// Defer sets awaited_at with deferred through WriteRecords.
// It admits a run blocked at Drift-Await and a run at a Drift-Baseline capture halt.
// It admits a halt of Drift-Expect, a writer row, Drift-Verify or Drift-Recheck in two cases:
// the halt's error, past an optional leading decision halt prefix, opens with DeferHaltPrefix;
// or the run's scope is not off, it holds no deferral, and its baseline check fails.
// Defer refuses while the capture lock is held.
// A running capture, a `lym drift check` and the drift gate's check all take that lock.
// The refusal names `lym loom resume` and, for a check, waiting for it to finish.
// Amend records the parent's amendment through WriteRecords in one commit, and never edits the seal commit.
// It admits a blocked writer row, Drift-Verify or Drift-Recheck, and a blocked Drift-Expect for `--restore` alone.
// A scope that is off admits `--restore` alone;
// records that differ from their commit need writing back whatever the scope.
// An AmendRequest holds exactly one of Entries, Restore and Discard.
// Its Check refuses any other shape, naming `--entries`, `--restore` and `--discard`, before anything is read.
// Entries are parsed by ParseExpectation under the recorded mode against the baseline's listing;
// an entry it refuses is refused with its message.
// The stored rulings of every cell the entries cover are dropped from `judged.json`;
// under `command`, those of every case they name are dropped.
// Restore writes the seal commit's copy back over an expected.yaml edited after the seal.
// It also runs RestoreRecords over records that differ from their commit.
// It is refused when neither failed.
// Discard records that the expectation is set aside, so CheckSeal returns the empty expectation.
// Containment then reads the amendments alone.
//
// # Agents
//
// The expect agent and the judge are each one lead seat, built by ExpectTable and JudgeTable.
// The seats run over the stencils `drift-template-expect` and `drift-template-judge`.
// The seat engine reads them at call time and renders them with the parent and edit directives.
// The expect seat is never interactive; the judge seat is interactive under `judge_interactive`.
// The expect seat's only output is `expected.yaml` and the judge's is `judgement.yaml`:
// the records are Go-written, and a fresh spawn archives its outputs.
// The task's subject comes from the decision record in a tapestry run (SubjectDecisionRecord).
// In a darn run it comes from the board entry (SubjectBoardEntry).
// The record's `Added after Discussion` entries win in both.
// The seats take the `plan` and `review` segment colors and their models from `expect_model` and `judge_model`.
// Their session timeouts are package constants, since drift.yaml declares no timeout key.
// Neither table carries a gate; the Drift-Expect and Drift-Verify rows set theirs.
// `drift-baseline` and `drift-verify` are roles with no seat, stencil or table.
//
// # Drift-Baseline
//
// Deps holds the seams the Drift producers share, and a producer fails loudly naming the field when a seam it reads is nil.
// Row holds a row's portable config: the subject its agent reads the task from.
// Each Drift producer reads `drift.enabled` at every pass and is Done at once, committing nothing, when it is off.
// The entry gate reads the switch only when the run first enters a row,
// so this lets a resume pass a row already entered once the switch goes off, in the hub or in the task's local layer.
// A drift config that fails to load there is a Stuck naming `lym loom resume`.
// DriftBaseline spawns no agent and writes no `baseline.md`.
// It first ends, through Deps.EndRoles, every live strand of the removed `drift-baseline` and `drift-verify` agents,
// so no pane keeps an old stencil.
// It then records the scope once through WriteRecords when none is recorded.
// The scope is `off` when `drift.scope` is `off`, with ScopeSource `drift.scope`.
// Otherwise it is `full`, with the config's `diff`, `key_patterns` and volatile lists.
// A config scope read from `partial` is recorded `full` with one Warn naming the key.
// A recorded scope, `off`, `full` or an old `partial`, is kept;
// a later config change changes neither it nor the recorded mode and patterns.
// Scope off, a set awaited_at and a passed capture take no capture; a capture holding the lock is left running.
// Otherwise it captures again.
// An undeclared capture command is a Stuck naming `lym config drift`.
// A refusal of the capture's precondition is a Stuck whose reason opens with CaptureHaltPrefix and then DeferHaltPrefix.
// It names `lym drift defer` with `lym loom resume`, and on a dirty tree restoring or stashing the listed paths.
// The capture is the detached child:
// the pass returns Done once the child's record exists, and the recipe's Drift-Await row waits for the capture to end.
// A child that does not start is a Stuck opening with CaptureHaltPrefix and DeferHaltPrefix.
// It carries the child's error.
// It names `lym loom resume` and `lym drift defer` with `lym loom resume`;
// for an error the child marked lasting, it says a resume cannot change the error,
// and it names only `lym drift defer` with `lym loom resume` and a fix of the cause.
// When Windows refuses the detached start breakaway, the capture runs in the step's own process.
// Its command then dies with the step.
// A failed capture is a Stuck opening with CaptureHaltPrefix and DeferHaltPrefix.
// It names the log, `lym drift capture` with `lym loom resume`, and `lym drift defer` with `lym loom resume`.
// The row carries no on_stuck, so each Stuck halts the run.
// The scope write is committed with the records.
// A pass commits through Deps.Commit only once a capture has ended in this process.
// It then commits all of the durable drift directory.
//
// # Drift-Expect
//
// DriftExpect seals the expectation of changes before the writer starts.
// It works over Deps.ExpectTable, CommitsSince and the records and seal seams.
// ExpectTable builds one non-interactive lead on the `plan` segment, role `drift-expect`.
// The lead takes the model of `expect_model` and runs over the stencil `drift-template-expect`.
// Its one output is expected.yaml.
// The agent reads the identity listing Paths.Listing holds, the baseline and the task's record and plan.
// The pass is Done at once, spawning nothing, when any of these holds:
// drift is off, no scope record exists, an expectation is already sealed, the scope is off or the capture was deferred.
// The last two record `unsealed` first, so containment reads the empty expectation.
// Each of these is a Stuck for the run's parent:
// a capture that never counted, a baseline that is missing or no longer matches its digest,
// and a code HEAD other than the capture's head.
// The capture's head is the scope record's `baseline_head`, or the one capture.json names when the record holds none.
// Each such Stuck opens with shedadapters.DecisionHaltPrefix and then DeferHaltPrefix and names `lym drift defer`.
// The moved HEAD also lists the commits since.
// It also names the operator's reset of the run's code branch to the capture's head as the other way forward.
// A scope record with no diff mode first records the config's `diff`, `key_patterns` and volatile lists.
// That keeps them for the run.
// A pass that goes on to write over records differing from their newest records commit is a Stuck for the run's parent.
// It opens with shedadapters.DecisionHaltPrefix and names `lym drift amend <slug> --restore`.
// Amend admits that command at the blocked row for `--restore` alone.
// An `expect_session` mark closed with its `opened_at` kept records an ended session.
// An agent return without an error leaves such a mark.
// With an expected.yaml that validates, such a mark is sealed with no spawn.
// An open mark, a mark never opened or closed after an error, or a file that does not validate runs the agent.
// It resumes a live chair.
// The pass writes the listing before the agent runs.
// Under `diff: lines` it holds each case's lines that pass the recorded volatile lists, with identities and text;
// under `diff: command` it holds the case names alone.
// An occurrence `n` is written only for a key that repeats in its case.
// The agent's table is gated by the row's `expectation` entry, NewExpectationGate.
// The gate parses expected.yaml with ParseExpectation under the recorded mode against the baseline's cases,
// and it re-prompts the agent with the parser's message; a missing file fails asking for it.
// The session's return closes the mark.
// A Done seals the file through SealExpectation when no commit recorded it yet.
// The seal carries the baseline's digest as a second trailer.
// An exhausted entry is the shed's Stuck, whose resume spawns the agent again.
//
// # Drift-Await
//
// DriftAwait is the mechanical row that holds the run until the baseline exists.
// It spawns nothing and commits all of the durable drift directory, the capture log included, on every way out.
// It is Done with no scope record, with scope off, and with awaited_at set whatever HEAD is,
// so a later entry after the code changed passes.
// A capture holding the lock is waited on by awaitCaptureEnd with no bound of its own.
// The capture record is read only after the lock reads free, so a capture that just ended is read as ended.
// A passed capture taken at HEAD is Done once the row writes awaited_at into the scope record through WriteRecords.
// That write makes the capture the counted one.
// That records commit carries the extra trailer `Drift-Baseline: <BaselineDigest of the baseline directory>`,
// so the digest lives in a records commit made before the writer.
// A missing record, a failed one, and a running one whose lock is free are Stuck, naming the log when there is one;
// so is a passed record taken at another commit than HEAD.
// While HEAD is the branch point, a Stuck names two ways forward:
// `lym drift capture` followed by `lym loom resume`, which captures again and re-enters the row;
// and `lym drift defer` followed by `lym loom resume`.
// Once HEAD has moved, it names only `lym drift defer` followed by `lym loom resume`.
// No capture may run on changed code.
// No Stuck of the row carries the decision halt prefix, so the arbiter may settle one with the way forward it names.
// An adopting machine has no `.lym/drift/`, so before awaited_at the row halts on the missing record.
// `lym drift capture` captures on that machine.
//
// # Drift-Verify
//
// DriftVerify is the containment check and the judge.
// It checks the code's captured changes against the sealed expectation,
// and it has one judge rule on the expected changes no stored ruling covers.
// It first ends, through Deps.EndRoles, every live strand of the removed `drift-baseline` and `drift-verify` agents,
// so no pane keeps an old stencil.
// It then runs these steps in order:
// no scope record, scope off, a deferred capture and a waiver are Done,
// and a records mismatch is a Stuck naming `lym drift amend <slug> --restore`;
// a scope record with no diff mode records the config's `diff`, `key_patterns` and volatile lists and keeps them;
// a capture that never counted, a missing baseline directory and an edited baseline are a Stuck naming `lym drift defer`,
// and the first is read from the tracked scope record alone;
// a head equal to the last passing check's with the same number of amendments is Done;
// an open `judge_session` mark is settled before anything else:
// a change JudgeSnapshot.Changes names is a Stuck;
// a gone snapshot is recorded as lost and is a Stuck only under a live chair, whose changes cannot be told;
// a session that is intact, or whose snapshot is gone with no live chair, goes on to be resumed;
// a run past Drift-Expect with neither a seal commit nor an expected.yaml records `unsealed` once;
// then Check runs, and a seal fault, a failed current capture and any cell the expectation does not cover are a Stuck;
// each cell whose exact tuple has a stored ruling reuses it;
// a stored `does not match` is a Stuck, and with no cell left to judge the pass is recorded and Done.
//
// Otherwise one judge is spawned.
// Drift-Verify removes a stale judgement.yaml and writes the JudgeRequest under a fresh spawn id.
// It takes the snapshot, opens the session mark through WriteRecords, and runs the judge through the seat engine.
// The judge's table is gated by the row's `judgement` entry.
// A session an earlier entry left open keeps its spawn id and its snapshot:
// the request is written again under the recorded id, a live chair is resumed and none is respawned,
// and a snapshot recorded lost has its mark cleared and is then taken anew first.
// On the session's return, before the mark closes, these are a Stuck that leaves the mark open:
// a change to either checkout or to the scratch directory, a snapshot gone while the judge ran,
// and a baseline that no longer matches its digest.
// The way forward of a changed checkout names the scratch directory by its path and keeps the snapshot directory.
// The resume compares against the snapshot directory.
// The rulings are stored in judged.json as exact tuples with the judge's strand and time.
// Each listed open range carries the covering entry's why and the judge's sentence.
// A listing replaces an earlier listing of the same range.
// The mark closes and the last check is recorded in the same write, and judgement.yaml is removed after it.
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
