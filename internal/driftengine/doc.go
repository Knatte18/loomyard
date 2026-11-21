// Package driftengine owns regression around a task:
// the hub-wide `drift` config module, the drift paths, and the records and runs the Drift rows read and write.
// Regression is no test tier.
// It records a profile of the suite's behavior on the task's branch point and compares the task's code against it.
// `lym gate test` never runs it.
//
// `drift.yaml` keys:
// `enabled` switches the Drift rows on through their entry gate;
// `capture` and `compare` are command steps written whole as `{command, dir, env, timeout_min}`;
// the compare runs only under `diff: command`;
// `diff` is `lines` or `command`;
// `key_patterns` are regular expressions with the named groups `key` and `value` that key a captured line;
// `volatile_files` and `volatile_keys` name the capture files and line keys the diff ignores;
// `expect_model` and `judge_model` are model-specs;
// `judge_interactive` makes the judge a session with the operator in its pane;
// `judge_ask_parent` and `judge_parent_timeout_min` route the judge's rulings to the run's parent orch;
// and `scope` is a task's regression scope, set in its board entry's `run_settings`.
// `diff`, `key_patterns`, `volatile_files` and `volatile_keys` are guard keys:
// only the hub's file sets take them, and a task's run settings are refused them.
// `capture` and `compare` are executable keys: only the template and the hub's board default layer set them.
// A step left with keys but no command fails the load check.
// LoadConfig loads the file strictly and refuses an invalid key, naming it.
//
// A declared command holds `{baseline_dir}`, the directory the capture writes its profile files to.
// It may hold `{meta_dir}`, the capture's metadata directory, where a harness writes provenance.
// Drift keeps that directory with the capture but never lists, compares or shows it to an agent.
// A command holding the retired `{skip_file}` placeholder is refused before any run.
// A step's `dir` resolves like a gate tier's `dir` and must be an existing directory.
// Every row and verb that runs a command takes its step from Config.StepWithHubEnv.
//
// Paths is the one declarer of every drift path.
// The durable directory `_lym/drift/` holds the records, the expectation, the judgement file, the logs and the findings.
// The ephemeral directory `.lym/drift/` holds the rest, such as the capture record, lock and directories.
// A profile file holds a test's profile when its path relative to its profile directory is the test's name.
// The path may omit its last extension.
//
// Every record is JSON written through `internal/state`, so an unknown key survives a rewrite.
// Verdict renders the `## Regression check` section a landing carries and runs no capture.
// With no scope record it renders nothing and creates no directory.
//
// # Capture
//
// Every capture and compare takes its gate slot before any lock;
// a run waiting for a slot holds no lock, and a lock holder always holds its slot.
// A request's InheritedSlot is the slot a gate holds around the run, and the run then acquires none.
// RunCapture is the main capture.
// It moves a non-empty baseline directory, with its metadata directory, to a numbered backup,
// so no baseline is deleted or overwritten.
// The record ends `passed` for exit 0 with a file under the baseline directory, else `failed` with the reason.
// RunCapture, CaptureCurrent and RunCompare refuse a command holding `{skip_file}`.
// They refuse before taking a slot or a lock or moving the baseline.
// CaptureCurrent is the current capture every check runs, in a fresh directory per check.
// It writes no capture record and never touches the baseline directory or its backups.
// A failure, a timeout, no file and a refused step are results that did not pass, with the way forward as the reason;
// only an I/O failure is an error.
// StartCapture spawns `drift capture-child` detached and returns the pid only once the capture record carries it.
// A child that ends or stalls before that record is an error carrying the child's last JSON error envelope.
// A start Windows refuses for breakaway is retried plain-detached when the caller sets FallbackDetach;
// otherwise it fails with ErrBreakawayRefused.
//
// # Diff
//
// DiffCaptures is the pure diff of two capture directories, with no config read.
// An Identity names a line:
// a keyed line by its Key and its occurrence N counted from 1 among the file's lines holding that key;
// an unkeyed line by its Line position among the file's unkeyed lines.
// Check drops the cells of the volatile lists and the metadata marks after the diff.
// The baseline digest still covers them.
// Under `diff: command` the declared compare decides alone.
//
// # Expectation
//
// ParseExpectation is the sole reader of `_lym/drift/expected.yaml`, the expectation the expect agent writes.
// It decodes strictly, refuses an unknown key, and reads an empty file as the empty expectation.
// Under `diff: command` a case holds one `why` and nothing else.
// Under both modes, `metadata` marks the capture files and line keys that hold metadata, and no results.
// Every `why` must be non-blank, and a case, key, occurrence and mark are checked against the baseline's listings.
//
// # Judgement
//
// ParseJudgement is the sole reader of `_lym/drift/judgement.yaml`, the rulings the judge writes.
// The file is no record: Drift-Verify removes it once the rulings are stored in `judged.json`.
// It is strict like the expectation's parser, and its refusal lists every finding.
//
// # Judge
//
// JudgeTable builds the judge's seat: one lead on the `review` segment, role `drift-judge`.
// Its one output is judgement.yaml, and the table carries no gate; the row sets the `judgement` entry, NewJudgementGate.
//
// Under `judge_ask_parent` Drift-Verify appends the `parent-agreement` entry, ParentAgreementGateEntry.
// The ask mails the run's parent orch and records a digest of the rulings it presented.
// The entry passes only on an answer to those rulings, a timeout, or an ask that never reached the parent.
// A judgement changed after the ask gets a new ask.
// Bound: the entry checks that the judge changed its rulings after the parent's findings, not that it took each one.
//
// TakeJudgeSnapshot records the state of both checkouts, the decision record and the capture's scratch directory.
// JudgeSnapshot.Changes names what the judge session changed since the snapshot, judged by content.
// The snapshot and every comparison with it hold the run's decision lock, Deps.LockDecisionRecord.
// Exempt are judgement.yaml and its timestamped siblings, what lym writes itself around a session;
// entries a `lym loom decision add` or `replace` added to the decision record during the session are exempt too.
// Any other change to the decision record, and any change to a file outside it, is named as a change.
// Bound: an edit under the exempt loom paths or the drift logs goes unseen;
// they are lym's own run bookkeeping and hold no code, no expectation and neither record.
//
// # Records
//
// Two durable records are guarded: `scope.json` and `judged.json`.
// `judged.json` holds the judge's stored rulings as exact tuples, and under `command` its rulings per case and code head.
// `scope.json` holds the recorded scope (`off` or `full`) and when the baseline was awaited or deferred.
// It also holds what the checks record, such as the config's `diff`, `key_patterns` and volatile lists as first seen.
// Every field is optional, so a record written without it still reads.
// WriteRecords is the one writer.
// It commits both files at once through Deps.CommitWithTrailers with the trailer `Drift-Records: <RecordsDigest>`.
// ReadRecords returns both files as the newest commit carrying that trailer holds them.
// It sets Mismatch when the files on disk digest differently.
// WriteRecords refuses over a mismatch, and only RestoreRecords clears one.
// `expected.yaml` and `judgement.yaml` lie outside the guard: the expectation is sealed by its own trailer.
//
// # Seal
//
// SealExpectation commits `expected.yaml` alone with the trailer `Drift-Seal: <sha256>`, plus any extra trailer.
// CheckSeal decides the expectation containment reads:
// Sealed bytes, Empty, NotYet or a Fault naming its way forward.
// CheckBaseline recomputes the BaselineDigest and compares it with the one the oldest `Drift-Baseline` commit recorded.
// An edit of the baseline after its capture counted is a Fault opening with DeferHaltPrefix and naming `lym drift defer`.
// With no such commit the baseline is Unguarded, which skips only the compare.
//
// # Check
//
// Check is the containment every drift check runs.
// It writes no tracked record and decides nothing; Drift-Verify maps its CheckReport to outcomes.
// Whether the capture counted is read from the tracked scope record alone, never from the capture record.
// A cleaned `.lym` loses the capture record.
// A records or seal fault does not end the check: it goes on over the committed copies, or against the sealed copy.
// NewDriftGate returns the gate of the `drift` entry on a writer's gate list.
// It passes on a skip and on a baseline fault, which no writer session can clear, and under `command` whatever the exit.
// It fails on a capture fault and on any unexpected cell left.
// When every unexpected cell is unchangeable by code, the failure is Terminal and the findings go to the run's parent.
//
// # Compare
//
// RunCompare is the mechanical compare, on the runner the capture uses.
// It takes the compare marker, an OS lock, before it waits for its gate slot, and writes no record.
//
// # Verbs
//
// The `lym drift` verbs' rules are functions over a VerbStatus.
// A verb run at a row it does not admit refuses naming that row and `lym loom status`, and writes nothing.
// Defer refuses while the capture lock is held.
// Amend records the parent's amendment through WriteRecords in one commit, and never edits the seal commit.
// An AmendRequest holds exactly one of Entries, Restore and Discard.
// Restore writes the seal commit's copy back over an edited expected.yaml;
// it runs RestoreRecords over differing records.
// Discard sets the expectation aside, so CheckSeal returns the empty expectation.
//
// # Agents
//
// The expect agent and the judge are each one lead seat, built by ExpectTable and JudgeTable.
// The expect seat is never interactive; the judge seat is interactive under `judge_interactive`.
// The expect seat's only output is `expected.yaml` and the judge's is `judgement.yaml`; the records are Go-written.
// The task's subject comes from the decision record in a tapestry run (SubjectDecisionRecord).
// In a darn run it comes from the board entry (SubjectBoardEntry).
// Neither table carries a gate; the Drift-Expect and Drift-Verify rows set theirs.
//
// # Drift-Baseline
//
// Deps holds the seams the Drift producers share; a producer fails loudly naming the field when a seam it reads is nil.
// Each Drift producer reads `drift.enabled` at every pass and is Done at once, committing nothing, when it is off.
// DriftBaseline spawns no agent.
// It records the scope once through WriteRecords: `off` when `drift.scope` is `off`, otherwise `full`.
// A recorded scope is kept; a later config change changes neither it nor the recorded mode and patterns.
// Scope off, a set awaited_at and a passed capture take no capture; otherwise it captures again.
// The capture is the detached child: the pass returns Done once the child's record exists.
// An undeclared capture command is a Stuck naming `lym config drift`.
// A refused precondition, a child that does not start and a failed capture are each a Stuck.
// Each opens with CaptureHaltPrefix and then DeferHaltPrefix, naming `lym drift defer` with `lym loom resume`.
// When Windows refuses the detached start breakaway, the capture runs in the step's own process and dies with the step.
// The row carries no on_stuck, so each Stuck halts the run.
//
// # Drift-Expect
//
// DriftExpect seals the expectation of changes before the writer starts.
// ExpectTable builds one non-interactive lead on the `plan` segment, role `drift-expect`.
// The pass is Done at once, spawning nothing, when drift is off or no scope record exists.
// It is also Done at once when an expectation is already sealed, or when the scope is off or the capture was deferred;
// in the last two cases it records `unsealed` first.
// Each of these is a Stuck for the run's parent:
// a capture that never counted, a baseline that is missing or no longer matches its digest,
// and a code HEAD other than the capture's head.
// Each opens with shedadapters.DecisionHaltPrefix and then DeferHaltPrefix and names `lym drift defer`.
// A pass that would write over records differing from their newest records commit is a Stuck.
// It names `lym drift amend <slug> --restore`.
// An ended session whose expected.yaml validates is sealed with no spawn; any other state runs the agent.
// The agent's table is gated by the row's `expectation` entry, NewExpectationGate.
// A Done records the metadata marks in the scope record and seals the file through SealExpectation.
// The seal carries the baseline's digest as a second trailer.
//
// # Drift-Await
//
// DriftAwait is the mechanical row that holds the run until the baseline exists.
// It spawns nothing and commits all of the durable drift directory, the capture log included, on every way out.
// It is Done with no scope record, with scope off, and with awaited_at set whatever HEAD is.
// A capture holding the lock is waited on with no bound of its own.
// A passed capture taken at HEAD is Done once the row writes awaited_at into the scope record through WriteRecords.
// That write makes the capture the counted one, and its commit carries `Drift-Baseline: <BaselineDigest>`.
// A missing, failed or taken-elsewhere capture record is a Stuck, naming the log when there is one.
// While HEAD is the branch point, the Stuck names `lym drift capture` and `lym drift defer`.
// Each is followed by `lym loom resume`.
// Once HEAD has moved, it names only `lym drift defer`; no capture may run on changed code.
// No Stuck of the row carries the decision halt prefix, so the arbiter may settle one with the way forward it names.
//
// # Drift-Verify
//
// DriftVerify is the containment check and the judge.
// It checks the code's captured changes against the sealed expectation.
// It judges the expected changes and the review range's loosened checks that no stored ruling covers.
// No scope record, scope off, a deferred capture and a waiver are Done.
// A records mismatch is a Stuck naming `lym drift amend <slug> --restore`;
// a capture that never counted, a missing baseline directory and an edited baseline are a Stuck naming `lym drift defer`.
// A head equal to the last passing check's with the same number of amendments is Done.
// An open `judge_session` mark is settled first: a change JudgeSnapshot.Changes names is a Stuck.
// A gone snapshot is recorded as lost, and is a Stuck only under a live chair, whose changes cannot be told.
// Then Check runs, and a seal fault, a failed current capture and any cell the expectation does not cover are a Stuck.
// A cell whose exact tuple has a stored ruling reuses it, and a stored `does not match` is a Stuck.
//
// Where Deps.ReviewBase is told, the pass also lists the review range's files.
// A standalone run records no parent branch, tells no base and lists no file.
// A file's key is the sha256 of its diff, a NUL and the range's `Loosened-Check:` trailer lines,
// so a commit that only adds a trailer changes every key.
// The judge rules a file `none` or `loosened`, and each loosened check `caused` or `uncaused`.
// A loosened check with no measured cause is a Stuck.
// Its way forward is a `Loosened-Check:` trailer on a commit of the range, or a revert, then `lym loom resume`.
// Bound: a trailer's measured cause is the committer's attestation, which neither the judge nor the code re-measures.
//
// Otherwise one judge is spawned under the snapshot guard.
// A session an earlier entry left open keeps its spawn id and snapshot, and a live chair is resumed, not respawned.
// On the session's return, a change to either checkout or to the scratch directory, a snapshot gone while the judge ran,
// and a baseline that no longer matches its digest are a Stuck that leaves the mark open.
// Each change the Stuck names carries its file's modification time, so a change by another process can be told.
// A resume re-checks the guard against the snapshot before anything else.
// The rulings are stored in judged.json as exact tuples, and judgement.yaml is removed after.
// A ruling of `does not match` is a Stuck over its cells, and all `matches` records the pass and is Done.
// A Stuck over rulings groups cells that share a case, key, kind and reason into one finding.
// It names the findings file, which holds every cell's finding.
//
// Under a recorded `command` diff, Check runs the declared compare against the baseline directory.
// The compare replaces a capture and a diff:
// an exit 0 is Done with no judge;
// a nonzero exit at a code head with stored `does not match` or `unexpected` case rulings is a Stuck naming them;
// this holds only when no amendment came since, and it spawns no new judge;
// otherwise one judge is spawned, and its `judgement` entry refuses a judge that rules no case after a failing compare.
// A ruling of `matches` is not stored.
// A ruling on a case the metadata marks name counts as `matches`.
// Bound: `command` gives up containment by code, and a drifted case the log does not show goes unjudged.
//
// The route that fixes code, which every Stuck over a regression names, is derived from Deps.Rows:
// `lym loom goto --to Darn-Write` when the rows hold Darn-Write, `--to Sampler-Session` when they hold Sampler-Session;
// otherwise `--to Webster`.
// Each is followed by `lym loom resume`.
// Every Stuck for the parent opens with shedadapters.DecisionHaltPrefix.
// One that names `lym drift defer` carries DeferHaltPrefix right after it.
// The row carries no on_stuck, and the judge edits no code, expectation or record.
// The Drift-Recheck row is this engine in a second row, after the rows that may commit code once Drift-Verify passed.
// It has the same config and no on_stuck.
package driftengine
