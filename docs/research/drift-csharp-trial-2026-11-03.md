# Drift trial on the C# sandbox hub, 2026-11-03

## Verdict

All three trial tasks landed on the `cs` hub (`lym-csharp-LYMHUB`, anchor `lym-csharp/dotnet`), and every Drift row and gate ran from the anchor.
Drift captured a baseline on each task's branch point, compared a pure refactor clean, reported a new case as drift that its verify accepted and the parent ruled, and reported an intended output change on exactly the line it touched, which the verify accepted case by case with reasons.
The dotnet scoper narrowed every diff-scoped gate to the test classes and projects the change reaches, and the shared build ran once per `lym gate test` call before tiers run with `--no-build`.

None of the three ran clean on its own.
The trial found eleven lym faults, each filed as a bug note on the loomyard board; by the trial's end six were fixed on `main`, three ran as tasks and two stood as notes (see Faults).
The ones that stopped runs were Drift's `dir` base (T1), a baseline stencil that skips intended drift (T3) and an arbiter that died on an API error and held a stop for 28 minutes (T3).
Two gaps remain open: a change made after Drift-Verify lands with no compare, and a lym binary that changed what an existing `drift.yaml` means came with no migration.

## Setup

- Hub `cs`, owned by Neve through `lym adopt root`; its prime `lym-csharp`, with the .NET solution under `dotnet/`, the anchor.
- `gate.yaml`: `scoper: dotnet`; a `build` step `dotnet build LymCsharp.sln -m` in `dotnet`; tier0 and tier1 `dotnet test {targets} --no-build {filter} {args}` with `shared_build: true`, `all_targets: LymCsharp.sln`, filters `Category!=Slow` and `Category=Slow`, `dir: dotnet`; gate keys `darn-write`, `code-burler-fix` and `drift-verify` at tier0 scope diff, `finalize` and `adopt` at tiers 0 and 1 scope all.
- `drift.yaml`: `enabled: true`, capture and compare through `dotnet run --project src/LymCsharp.Regression -- capture|compare {baseline_dir} --skip-file {skip_file}`.
  The harness writes one profile file per regression case and reports the first deviating line of each drifted case.
- Three darn `light` tasks chained by `depends_on`, released one at a time once the one before had landed.
- Times are CET.
  Gate run records are `_bolt/.lym/gate/runs/*.json`; Drift records are each task worktree's `_lym/drift` and `.lym/drift`.

## Adopt check

The adopt check of `root` started at 15:15:31 on the gate config of 2026-10-28; the trial's `gate.yaml` landed 24 s later and `drift.yaml` 31 s later, so the check tested the old config.
Its tier0 `dotnet test` aborted after one second with `Internal CLR error. (0x80131506)` and dumped core, a one-off of the .NET runtime.
The MSBuild worker nodes it had started outlived it and held the tier's stdout pipe, so the check reported `tier0 running` past tier0's 10-minute timeout (`gate-orphan-pipe-hang`).

I stopped the check and killed the orphaned nodes.
The check then ended red, and its fix loop filed and started `adopt-fix-root-t1`, a task the trial forbade; I cancelled it and removed its entry.
The removal command the fix loop's notice names, `lym board remove <slug>`, was refused (`board-remove-slug-form`).
The removal ran the entry's adopt-check action: a fresh check on the trial config ended green at 15:26:42, with the shared build once (4.5 s) and tier0 (102 passed) and tier1 (3 passed) with `--no-build` in `lym-csharp/dotnet`.

## T1 `pipeflow-reynolds-helper`: a behavior-preserving refactor

Released 15:27, landed 16:08:49 as `c84741f`.
Task worktree `pipeflow-reynolds-helper`, anchor `pipeflow-reynolds-helper/dotnet`.

| Row | What it did |
|---|---|
| Drift-Baseline | Agent recorded scope `full`; `baseline.md` expects no drift. |
| Drift-Await | First capture failed at once: Drift joined `dir: dotnet` onto the anchor and ran in the missing `…/dotnet/dotnet`, reported as `fork/exec /usr/bin/sh: no such file or directory` (`drift-dir-base`, `drift-missing-dir-error`). `cs:orch` set the dirs to `.`, which then meant the anchor, and captured again on `7f50ffe`: passed in 2 s, five cases. |
| Darn-Write | One commit extracting `PipeFlowCalculator.Reynolds`. |
| Drift-Verify | Compare in the anchor, exit 0, no `drift:` line; `done` in 2 s with no agent session. |
| Code-Review | Code-Burler-Fix added one test (`3f81f55`). |
| Describe, PR-Review | The PR body and the reviewer asked for the regression compare by hand, "not run", though Drift-Verify had run it clean (`drift-verdict-in-pr`). PR-Review waited 32 minutes for the operator until the hub orch reviewed for them. |

Gates, run dir `pipeflow-reynolds-helper/dotnet` throughout:

- `darn-write`: build once, then tier0 over the computed scope `tests/LymCsharp.Hydraulics.Tests` filtered to `PipeFlowTests|ResultFormatterTests` and `tests/LymCsharp.Regression.Tests` filtered to `BaselineHarnessTests|RegressionCommandTests`; 36 passed.
  `LymCsharp.Units.Tests` was left out, as it should be: Units does not depend on the changed project.
- `code-burler-fix`: build once, the same scope, 38 passed.
- A hand `lym gate test --targets ./tests/LymCsharp.Hydraulics.Tests` in review: tier0 with `--no-build`, 54 passed.
- `finalize`: build once, tier0 over `LymCsharp.sln` 105 passed, tier1 3 passed.
  No Parent-Align ran: `main` had not moved.

While watching, `lym loom status --watch` printed a line every second during each verify, because the wait text carries its elapsed seconds (`watch-elapsed-churn`).
On the operator's order I set `batten.open_window=false` at the `cs` default layer and closed T1's window by detaching its tmux client; the run went on.

## T2 `churchill-friction`: a case the baseline lacks

Released 16:27, landed 17:03:38 as `9017f2d`.
Task worktree `churchill-friction`, anchor `churchill-friction/dotnet`.

| Row | What it did |
|---|---|
| Drift-Baseline | Scope `full`; `baseline.md` expected the new case "reported as added, not as drift". |
| Drift-Await | Capture on `c84741f` (T1's merge) in the anchor: passed in 5 s, the same five cases. |
| Darn-Write | `ChurchillFriction` with tests, and one regression case `water-pvc-2in-transitional-churchill`. |
| Drift-Verify | The first compare exited 1 with `drift: water-pvc-2in-transitional-churchill line 1: baseline <absent>, current "fluid = water"`; every existing case clean. The agent accepted the new case with a reason (`by: agent`, `ruling: unruled`); under scope `full` that ended in a decision halt for the parent, `cs:orch` stood by it with `lym loom resume`, and the compares after were clean. |
| Code-Review | Two test-only commits pinning the transitional value; Drift's verdict stands. |
| Describe, PR-Review | The PR body again asked to "Run the Drift regression suite" by hand; the hub orch approved. |

Drift has no notion of an added case: the harness reads a missing profile as empty, so a new case drifts like a changed one and goes through a ruled acceptance.
That path worked.

Gates, run dir `churchill-friction/dotnet`:

- `darn-write`: build once, then tier0 over `tests/LymCsharp.Hydraulics.Tests` filtered to `ChurchillFrictionTests` alone (a new file no other test class references) and `tests/LymCsharp.Regression.Tests` filtered to `BaselineHarnessTests|RegressionCommandTests`; 27 passed.
- Hand `lym gate test --targets ./tests/LymCsharp.Hydraulics.Tests` during review and fix: each ran the build first, then tier0 with `--no-build` (65 passed; one red run, 64 passed and 1 failed, while the fixer worked).
- `code-burler-fix`: build once, the computed scope, 28 passed.
- `finalize`: build once, tier0 116 passed, tier1 3 passed.

The `drift-verify` gate key ran in neither T1 nor T2: its agent changed no code, and a compare is no gate tier.

## T3 `report-significant-digits`: an intended output change

Released 17:04, landed 18:07:35 as `fa6627b`.
Task worktree `report-significant-digits`, anchor `report-significant-digits/dotnet`.

| Row | What it did |
|---|---|
| Drift-Baseline | Following the stencil ("`partial` for a task that changes some behavior on purpose; name each test that will drift totally with `--skip`"), the agent recorded scope `partial` with all six cases skipped, and noted in `baseline.md` that the skip hides any regression in the cases' raw lines (`drift-baseline-skips-intended`). |
| Drift-Await | The capture on `9017f2d` exited 0 and wrote nothing: "exit 0 but wrote no file", with empty logs and a way forward that repeats the failure (`drift-skip-all-capture`). The tackler re-launched the row blind, failed the same way, and escalated; the arbiter's turn ended three seconds in on `API Error: 500` and it sat idle with the stop, telling no one (`arbiter-api-error-stall`). |
| (repair) | `cs:orch`, told by me at 17:32, recorded scope `full`. Its capture ran in the worktree root and failed `exit 1`: `c896481c`, the `drift-dir-base` fix deployed at 17:23, had moved Drift's `dir` base to the worktree root, so `cs`'s `dir: .` now meant the root, with no migration or notice (`drift-dir-base-migration`). It set the dirs to `dotnet` and captured six cases in the anchor. Its five `lym loom resume` calls were refused `driver_not_parked` while the dead arbiter held the stop; a fresh arbiter resumed the run at 17:34:41. |
| Darn-Write | `ResultFormatter` to 4 significant digits, its tests updated. |
| Drift-Verify | The compare drifted five cases, each first at line 12, the report line, e.g. `Re = 127069, dP = 7925.267 Pa` → `Re = 127100, dP = 7925 Pa`. `oil-steel-6in-turbulent` did not drift: its values already had 4 significant digits. The agent accepted each of the five with a reason naming the old and new values; the parent ruled on all five; the compares after were clean. |
| Code-Review | Code-Burler-Fix rewrote `ResultFormatter`'s formatting path (`4b31b22`) after Drift-Verify, and no compare followed (`drift-after-review`). |
| Describe, PR-Review | The PR body said "the Drift verify was not run here". The hub orch approved on condition of a fresh compare; `lym drift compare` cannot give one past Drift-Verify and skips accepted tests, so `cs:orch` ran the harness by hand with no skip file: the same five lines with the accepted values, a match. |

Gates, run dir `report-significant-digits/dotnet`:

- `darn-write`: build once, then tier0 over `tests/LymCsharp.Hydraulics.Tests` filtered to `ResultFormatterTests` and `tests/LymCsharp.Regression.Tests` filtered to `BaselineHarnessTests|RegressionCommandTests` (the harness renders the report through `LymCsharp.Reporting`); 23 passed.
- A hand `lym gate test --targets ./tests/LymCsharp.Hydraulics.Tests`: build first, 70 passed.
- `code-burler-fix`: build once, the computed scope, 25 passed.
- `finalize`: build once, tier0 121 passed, tier1 3 passed.

## The anchor, the scoper and the shared build

- Every gate record of the three runs has its run dir in the task worktree's `dotnet/`, and every Drift capture and compare that ran ran there, once its `dir` matched the base of the binary at hand.
- The dotnet scoper's scopes followed the diff: the changed source file's test classes in the test project that covers it, and the regression harness's tests whenever the regression project or a project it renders through changed.
  Projects the change does not reach (`LymCsharp.Units.Tests`) never ran in a diff-scoped gate.
- Each `lym gate test` call that ran a `shared_build` tier ran `dotnet build LymCsharp.sln -m` once, before the first such tier, and no tier rebuilt.

## Faults

Each fault is a bug note filed on the loomyard board, named by slug, since a handle changes when the note is promoted.
State as of the trial's end.

| Slug | Fault | State |
|---|---|---|
| `gate-orphan-pipe-hang` | A tier or build whose shell exits while a grandchild holds its pipe never returns; no `WaitDelay`, and the timeout cannot fire once the shell has exited. | Fixed, `db24f264` |
| `board-remove-slug-form` | Way-forward texts name `lym board remove <slug>`, which the verb refuses. | Fixed, `7800a688` |
| `drift-dir-base` | `drift.yaml` joined `dir` onto the anchor, `gate.yaml` onto the worktree root. | Fixed, `c896481c` |
| `drift-missing-dir-error` | A Drift step in a missing dir failed as `fork/exec /usr/bin/sh`, with a way forward that repeats it. | Fixed, `c896481c` |
| `watch-elapsed-churn` | `lym loom status --watch` prints every poll while a verify runs. | Fixed, `832697b1` |
| `drift-verdict-in-pr` | Drift-Verify's verdict reaches neither the summary nor the PR; each PR asked for the compare by hand. | Task running |
| `drift-skip-all-capture` | A scope that skips every test ends in "wrote no file" with no cause named; the tackler re-launches blind. | Task running |
| `drift-baseline-skips-intended` | The baseline stencil skips the tests an intended change drifts, which hides regressions in them. | Fixed, `ea7de4cb` |
| `arbiter-api-error-stall` | An arbiter whose turn ends on an API error holds the stop silently until its deadline; the parent's resume is refused. | Task running |
| `drift-dir-base-migration` | The binary that moved Drift's `dir` base reinterpreted existing `drift.yaml` files with no migration or notice. | Note |
| `drift-after-review` | Code changed after Drift-Verify lands with no compare, and no Drift verb compares past Drift-Verify. | Note |

## Interventions

- I stopped the hung adopt check, cancelled and removed the fix task its red end filed, and posted the causes of the T1 and T3 Drift stops to `cs:orch`; `cs:orch` settled every stop itself.
- On the operator's order, through `ly:orch`: `batten.open_window=false` on `cs`, and the hub orch reviewed each PR in the operator's place, its word relayed to `cs:orch`, which told each PR-Review agent to continue.
- The brief's `cs:ORCH` is no creel name on `cs`; `cs:orch` is.
