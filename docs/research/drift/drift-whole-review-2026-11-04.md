# Drift as a whole: map, fault analysis and a reworked design, 2026-11-04

## Verdict

The trial faults share a cause the per-fault fixes could not reach: lym never looks at what drifted.
A compare is an opaque command that exits 0 or not; the drifted cases, lines and values exist only in the harness's log, which an agent reads.
So every question that matters (is this drift expected, is it the whole of it, does it still hold after later commits) is answered by an agent's reading, or not asked.
The fixes so far reworded stencils and closed gaps in the flow, and each was right, but they leave the judgment where it was.

The design below moves the line:

- Code diffs the baseline profile against the current profile itself, cell by cell, so a drift outside the expectation is a regression decided by code.
- The expectation is structured, written by an agent before any code and sealed.
- A second agent, which writes neither the expectation nor the code, judges only whether the new values inside the expected cells match it.
- No test is skipped, no agent picks the scope, and an acceptance covers exact (case, line, old value, new value) cells, never a whole test.

The review ran no Drift run.
Its evidence is the two trial reports, the Drift code and stencils as they stand on `main`, and the recipes.
Not seen: the `cs` harness source, which lives in the sandbox hub, and the baseline and verify agents' own reports of the trials, which went with torn-down worktrees.
Where the design leans on how the harness writes its profile (one text file per case, one line per quantity), the source is trial 1's setup section and says so.

## The map

### Rows

| Row | Engine | Decided by | What it decides |
|---|---|---|---|
| Drift-Baseline | `DriftBaseline` | agent, or code when `drift.scope` is set | The scope (`off`, `partial`, `full`) and the skipped tests; the baseline report `baseline.md`; then code starts the detached capture. |
| Drift-Await | `DriftAwait` | code | Holds the run until a passed capture at the branch point exists, or the parent deferred it. |
| (writer) | `DarnWrite`, `Webster` | agent | Writes the code. Has no view of the baseline report and runs no compare. |
| Drift-Verify | `DriftVerify` | code, then agent on drift | Compares; on drift spawns one strong agent that accepts or fixes. |
| Code-Review perch | `Bouncer` ring | agents | Reviews the code including Drift-Verify's fixes; may commit more code. |
| Drift-Recheck | `DriftRecheck` | code | Compares again when code was committed after Drift-Verify's last compare; any drift goes back to Drift-Verify. |

Darn and tapestry run the same rows in the same order; tapestry puts Drift-Baseline before the parts and plan writers and Drift-Await before Webster, and Drift-Verify after Parent-Align.
Every row has the entry gate `drift.enabled`, and every producer re-reads it on each pass.

### Verbs

| Verb | Admitted at | Effect |
|---|---|---|
| `scope` | running Drift-Baseline (agent); blocked Drift-Baseline or Drift-Await (parent) | Records the scope and skips once. |
| `capture`, `capture --side` | Drift-Baseline or an Await halt; Drift-Verify (agent) | Main capture in a detached child; a side capture spends a run. |
| `compare` | Drift-Verify | Runs the compare command with accepted and skipped tests left out; spends a run. |
| `accept <test> --reason` | agent session, or a caller at a halt | Records the test as accepted. Checks nothing about the name. |
| `withdraw <test>` | Drift-Verify | Moves an acceptance back to the compare. |
| `defer`, `waive` | blocked Await or capture halt; blocked Verify with no baseline | Goes on with no baseline, or with no compare. |
| `status` | any | Reads the records. |

### Config keys

`enabled`; `capture` and `compare` (command, dir, env, timeout); `baseline_model` and `verify_model`; `verify_interactive`; `verify_max_runs`; `scope` and `skip` (a task's `run_settings` set these, and then no agent runs for the baseline).

### Stencils

- `drift-template-baseline`: the baseline agent decides reach and scope, then writes `baseline.md` as free prose: expected drift per test, risks, skipped tests.
- `drift-template-verify`: the verify agent reads the compare log, the scope record and `baseline.md`, judges each drifted test, accepts or fixes, re-runs the compare within a budget, and writes `verify-report.md`.
  Operator-present mode swaps two paragraphs of text.

### Records

`scope.json` (scope, skips, awaited, waiver, acceptances with who and ruling, halt marks, last compare, accepted-profile commit), `capture.json`, `runs.json` (budget counter and agent-session mark), the profile directories (baseline, side, accepted), `baseline.md`, `verify-report.md`.

### What code decides and what an agent decides

| Code | Agent | Parent |
|---|---|---|
| Whether Drift runs; the capture's precondition (branch point, clean tree); a capture passes when it exits 0 and wrote any file | **Scope**: `off` skips Drift for the task, `partial` hides named tests | Ruling on the verify agent's acceptances under scope `full` |
| The skip list sent to the compare (scope skips plus every accepted test) | **Expectation**: free prose that no code reads | `withdraw`; `defer`; `waive` |
| A compare passes when its command exits 0 | **Which drifted tests exist**: read from the harness's log | Halts: unruled acceptances, a missing baseline |
| Budget, dirty tree, verify gate, accepted-profile capture and lapse, verdict text | **Accept or fix** each drifted test, with a reason; the fix itself | |

The left column is bookkeeping; every row of the middle column is a judgment about drift, and none of them is checked by code.

## Trial faults against the map

| Fault | Where it sat | What fixed it | Class that remains |
|---|---|---|---|
| `drift-dir-base`, `drift-missing-dir-error` | Config and capture | Code fix `c896481c` | None. |
| `drift-dir-base-migration` | Config | Reconcile migrates `6ea0fe61` | None; `PATTERN-config-reinterpretation` now covers it. |
| `drift-skip-all-capture` | Scope agent chose a skip list; capture ran | Failure names its cause `6ea0fe61` | The cause: the agent could choose a skip list that hides everything. |
| `drift-baseline-skips-intended` | Scope agent (stencil) | Stencil wording `ea7de4cb` | The agent still chooses `partial` and the skips; only its instruction changed. |
| `drift-baseline-preaccepts` | Expectation agent (stencil) | Stencil wording `bd51b652` | Expectation is prose; nothing stops a report from steering acceptance. |
| `drift-after-review` | Flow | `Drift-Recheck` `843a03f1` | Acceptances lapse whole (below). |
| `drift-verdict-in-pr` | Flow | Verdict section `58a85abf` | None found. |
| `watch-elapsed-churn`, `gate-runner-crash-fixer` | Status output and gates | Fixed | Outside Drift. |
| `gate-orphan-pipe-hang`, `board-remove-slug-form`, `arbiter-api-error-stall`, `pr-review-wait-silent` | Gates, board, arbiter, landing | Fixed or open elsewhere | Outside Drift. |

Of the faults in Drift, the ones fixed by wording are the ones that recur in kind: each moved a judgment into a sentence of instruction, and the next trial showed a neighbouring judgment.
The flow gaps were closed in code and stayed closed.

## Faults the trials did not show

Each was found by reading the code; none ran in a trial.

1. **`lym drift accept` checks nothing about its test name.**
   The name goes straight into the scope record, and an accepted name joins the compare's skip list (`SkipList`).
   A typo, or a test that never drifted, accepts into the skip list and hides that test from every later compare.
   The same holds for `--skip` names at `lym drift scope`.
2. **An acceptance covers a whole test, while the compare shows one line of it.**
   Trial 1 records that the harness reports "the first deviating line of each drifted case".
   An agent that accepts a test for a reason naming line 12 also accepts a regression on line 5 of the same test, which the log never showed.
   The "any other line that moves in the same test still shows as a regression" sentence in the baseline stencil holds only until the test is accepted; after that the test is in the skip list and nothing compares it.
3. **The accepted profile certifies the final output, not the output the acceptance reasoned about.**
   Drift-Verify captures the accepted profile at the end of its pass (`keepAcceptedProfile`), after the agent's fixes, and the agent's later compares skip accepted tests.
   An agent that accepts test X and then edits code that moves X again leaves a profile that records the moved output as the accepted one.
   Drift-Recheck compares against that profile, so it detects only a change after the profile.
4. **Scope `off` is an agent's call from a reading of project references.**
   The task then runs with no regression check, and the pull request's verdict says `off` with no reason; the reason lives only in `baseline.md`.
5. **The writer never sees the expectation or the compare.**
   Drift-Verify's agent, with less context than the writer, fixes the writer's regressions.
   Half of `verify.go` (dirty-tree gate, verify gate, attempt head, gate-halt marks, interrupted-agent probe) exists because that agent edits code.
6. **Whether the expectation was kept sealed is unchecked.**
   `baseline.md` sits in a tracked directory any later agent can edit.

## Questions the operator posed

**Does Drift-Recheck need a row of its own, or does sending the run back to Drift-Verify do the same?**
The row is needed, the engine is not.
A row is needed because the perch's fixes arrive after Drift-Verify and nothing else compares them; moving Drift-Verify after the perch would leave its fixes unreviewed.
In the design below Drift-Verify's agent writes no code, so the same engine can be placed twice, once after the writer and once after the perch, and `DriftRecheck`, its accepted-profile capture and its lapse logic go away.

**Can an acceptance lapse only the test that changed?**
Not today: the compare is opaque, so Recheck cannot tell which accepted test moved.
With code-side diffing it can, and finer: an acceptance names cells with their old and new values, a later compare either reproduces those pairs or not, and only a cell whose pair changed needs judging again.

**Was the wiring of `CaptureAccepted` and `CompareAccepted` untested?**
It has a test now: `TestDriftDeps_AcceptedSeamsKeepAcceptedTestsInTheRun` in `internal/loomcli/driftwiring_integration_test.go`.
Under the design below both seams are deleted, so no further test is owed to them.

## Design

### Principle

Judgment stays, but only where code cannot do the work, and always against facts code produced.
Whether a drift may occur at all is fixed before the code exists; whether it occurred is code's finding; whether a value inside the fixed region is right is a judgment by an agent that wrote neither.

### What the harness owes

Capture only.
A hub's `capture` command writes deterministic text files under `{baseline_dir}`, one file per case, one line per quantity, as the `cs` harness does today.
Code owns the compare: it captures the current code into a directory and diffs the two directories file by file, line by line.
Trial captures and compares each took seconds, so a capture and a diff replace a compare at no meaningful cost.
`compare` becomes optional: a hub may keep one as an extra check, and its exit code is then a further blocking finding; the default is the Go diff.
A hub whose compare tolerates noise declares ignore patterns in `drift.yaml`, set by the operator; no task or agent sets them.
This is the one external assumption: if a hub's profile files are not line-oriented text, the diff has nothing to read, and that hub keeps today's opaque compare with whole-test judgment.

### The diff and its cells

The diff yields a set of cells: for each case, one of `added`, `removed`, or `changed (line, baseline value, current value)`.
Added and removed lines inside a case are cells of their own.
The set is exact and complete, so the first-deviating-line limit of trial 1 disappears.

### The expectation

Drift-Baseline's agent is replaced by a Drift-Expect agent that runs after the capture, so it reads the real profile files and writes against real case names and line numbers.
It writes `expected.yaml`, read by one sole parser (`PATTERN-sole-parsers`), holding per case the lines it expects to change, as numbers or ranges (an open range such as `12-` when the task inserts lines), the cases it expects added or removed, and one sentence of why for each entry.
It states what it expects and why, never what may be accepted, and records no tolerance.
A stated output invariant yields an empty expectation and the contradictions as risks, as the stencil says today.
The file's hash is recorded in the scope record when the writer starts; every later compare checks it, so an edited expectation is a finding.

### Scope

Code chooses the scope: `off` when the task's run profile carries no code (a `docs` profile) or `drift.scope` says so, otherwise `full`.
The `partial` scope and the per-task `skip` go; a test that is nondeterministic by nature is excluded by the hub in `drift.yaml` once, by the operator, with the reason beside it.
The `skip_file` placeholder goes with them.

### Verify, in two parts

1. **Containment, by code.**
   Every changed cell must fall in an expected line range or an expected added or removed case.
   A cell outside is a regression: the row is Stuck, naming the cells, with no agent between the finding and the fix.
   An expected entry with no drift is reported, not failed: trial 2's `oil-steel-6in-turbulent` did not drift because its values already had four significant digits.
2. **Value judgment, by an agent.**
   When every changed cell is expected and any exist, one agent spawns that changes no code.
   It sees each cell's old and new value, the entry's why, and the task; it returns one decision per cell, `matches` or `does not match`, with a reason, as a structured file that code reads.
   It is a different session from the writer and the expectation agent, and may be a different model (`judge_model`).
   Operator-present mode remains: the same agent presents each cell and records what the operator decides.

A cell the judge accepts is stored as its exact tuple.
A later compare that reproduces the tuple needs no new judgment; a changed tuple is judged again; a cell the judge rejects goes back to the writer.

### Where regressions get fixed

The mechanical containment check is also offered to the writer as a gate entry of its own session, using the `GateEntry` shape Drift-Verify's table already uses for its `compare` entry.
The writer then meets "line 5 of case X moved, outside the expectation" while it still holds the context to fix it, up to the gate's attempts.
Drift-Verify after the writer repeats the check as the guarantee, and Drift-Recheck after the perch is the same engine in a second row.
What a cold fixer can do for a rejected cell follows `PATTERN-gate-fixer`: the fixer edits, Go's re-run judges.
Tapestry's writer is Webster, whose gate seam this review did not read; the design applies to it only if that seam admits the entry, and otherwise Drift-Verify's Stuck routes to a fixer row.

### Parent and operator

The parent rules only on a changed expectation: if the writer shows the expectation was wrong, the parent records an amendment with a verb of its own (as `lym loom decision add` does for a decision record), listed in the verdict.
The ruled and unruled acceptance bookkeeping, halt names and halt marks go.
`defer` stays for a capture that cannot run; `waive` and `withdraw` go.

### Row order

`Drift-Baseline` captures (detached, as now), `Drift-Await` waits, `Drift-Expect` writes the expectation, the writer writes code with the drift gate entry, `Drift-Verify` runs both parts, the perch reviews, `Drift-Recheck` runs the same engine.
Keeping the existing names for the first two keeps resume of in-flight runs simple (the recipes pin row names as durable identities).
Tapestry's overlap of the capture with planning is kept; Drift-Expect runs after the capture lands and before Webster.

### What the flow guarantees

1. Everything captured is compared; no test is skipped except hub-level exclusions.
2. The expectation exists, is structured and is sealed before the writer's first commit; a changed hash is a finding.
3. Every changed cell is classified by code as expected or unexpected; an unexpected cell blocks landing until the code or a recorded amendment changes.
4. Every expected, changed cell was judged against exact old and new values by an agent that wrote neither expectation nor code.
5. Any commit after the last judged compare is compared again; only cells whose tuple changed are judged again.
6. The pull request and landing commit state cell counts: expected, judged as matching, unexpected, amendments.
7. A skipped Drift (`off`) states its reason.

### What it removes

`partial`, `skip` and `skip_file`; `accept`, `withdraw`, `waive`; ruled and unruled acceptances and the halt marks that track them; the run budget and side capture; the accepted-profile capture and compare and their seams; gate-halt marks and the dirty-tree and verify-gate steps of the Verify agent's table; the language skills and PATTERN directive on that agent.
`DriftRecheck` as a separate engine goes with them.

### What it adds

A Go profile diff; the expectation file with its sole parser and seal; the Drift-Expect stencil and row; the judge's structured output and stencil; a drift gate entry for the writer; a verdict rewrite.
Config keys change meaning (`compare` optional, `scope` and `skip` go, `judge_model` added), so `PATTERN-config-reinterpretation` applies and the pruning of dead keys is `lym config reconcile`'s.
New stencils are read at call time (`PATTERN-stencil-ownership`), records stay tolerant of unknown keys (`PATTERN-tolerant-records`), and the new row is a `Constructor` entry in `internal/shedrecipe`.

## Decisions for the operator

1. **Move the compare into code?**
   It is the step everything else rests on and the one external assumption (line-oriented text profiles).
   The answer decides whether the rest is a rework or a repair.
2. **Open ranges in the expectation.**
   `12-` keeps the check usable when a task inserts lines, and an agent can write `1-` for everything.
   A tighter rule (open ranges and whole-case expectations of an existing case need the parent's ruling) costs a halt per use.
3. **Where does the fix loop live for tapestry?**
   Depends on Webster's gate seam, not read here.
4. **Seal strength.**
   A hash check catches an accidental rewrite, not a determined one; agents here are cooperative.
   Say so, or add a check that the sealed commit precedes the writer's first commit.
5. **Interim, if the rework waits.**
   Two small repairs stand alone: `accept` and `scope --skip` refuse a name that is not a file under the baseline directory, and the accepted profile is captured when the test is accepted (before the agent's fixes), not at the end of the pass.
   Neither touches the judgment boundary.

## Build

A tapestry can build most of it at once.
Slices that depend on nothing else: the profile diff with the expectation parser and seal; the stencils; the verdict.
Slices that wait for the diff: containment in Drift-Verify, the judge, the writer's gate entry.
The deletions go last, in one commit, once nothing reads the removed records.
Existing tests for the removed machinery are deleted with it, not kept.
