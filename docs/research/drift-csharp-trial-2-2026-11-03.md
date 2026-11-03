# Drift trial 2 on the C# sandbox hub, 2026-11-03

## Verdict

Three darn `light` tasks ran on the `cs` hub (`lym-csharp-LYMHUB`, anchor `lym-csharp/dotnet`) and all landed, on the lym binary deployed at 19:41 with Drift-Recheck.
Two of the three goals held on real runs, and one was not exercised:

- **Goal 2, a change after Drift-Verify, held on two runs.**
  Drift-Recheck compared again at the review fixer's head before Describe in T1 and T2, clean both times, and had nothing to compare in T3, whose review changed no code.
- **Goal 3, a partial scope, held.**
  T3 changed the `report` line of the transitional cases only; the baseline agent recorded scope `full` with no skips, Drift reported exactly the two transitional cases, the Drift-Verify agent accepted those two with reasons, and the other four cases stayed in the compare, clean.
- **Goal 1, an unintended regression, was not exercised.**
  T1's body contradicted itself (results unchanged, yet the factor computed at a rounded Reynolds number); the darn writer saw it before writing, asked the parent, and `cs:orch` recorded "exact keys, no rounding". The code stayed bit-identical, so Drift-Verify compared clean and nothing drifted.
  The loomyard hub orch ordered a fourth task with a regression the body does not reveal; the operator then ordered no more runs today, so that task was not written or run.
  Whether Drift reports an unintended regression, the verify refuses to accept it and the run does not land with it remains untested on real runs.

Drift's verdict reached the landing commit message on all three runs and the pull request body on the one run that opened a pull request (T1, PR #4); the operator turned PR review off for `cs` at about 19:14, and from then on lym opened and closed no review (`pull request off`).
No stop sat silent on an API error in these three runs.
The trial found three faults, each a bug note on the loomyard board; none stopped a run.

## Setup

- Hub `cs`, binary `0d155d1a` (deployed 19:41:40); `gate.yaml` and `drift.yaml` as in the first trial (`drift.yaml`: capture and compare `dir: dotnet`, against the worktree root).
- Three darn `light` tasks on the `cs` board, `enhancement`, all `draft`, chained by `depends_on`, released with `lym board set-status '{"slug":"<slug>","status":null}'`:
  1. `cached-friction-factor`: a `CachedFrictionModel` decorator, body calling it behavior-preserving but specifying a Reynolds number rounded to a whole number.
  2. `quantity-parse`: `Quantity.Parse`, output-neutral, culture and thousands separators left for review to find.
  3. `transitional-report-note`: a note on the transitional report line, stated as intended in the body.
- I told `cs:orch` up front (creel 31) that T1's drift would be a regression to reject.
- Times are CET. Gate run records are `_bolt/.lym/gate/runs/*.json`; the task worktrees were torn down at landing, so T2 and T3 are read from the trace archive under `_bolt/.lym/log-archive/<slug>/` and from the landing commits.

## T1 `cached-friction-factor`: a contradictory body

Released 19:44:52, landed 20:14:48 as `99a49ce`.

| Row | What it did |
|---|---|
| Drift-Baseline | Scope `full`, no skips. `baseline.md` named the contradiction and resolved it for the design: "Any drift in these cases is accepted with the reason 'friction factor evaluated at the rounded Reynolds number'", bounded by a relative 1e-3 (`drift-baseline-preaccepts`). |
| Drift-Await | Capture on `fa6627b` in the anchor, passed in 3 s, six cases. |
| Darn-Write | The writer asked the parent before writing (creel 32); `cs:orch` answered "A" and recorded "Cache keys on exact inputs, no Reynolds rounding … The invariant wins over the mechanism". One commit `efa12a1`, exact keys on a `ConcurrentDictionary`. |
| Drift-Verify | Compare at `efa12a1` clean; `done` with no agent session, nothing accepted. |
| Code-Review | APPROVED with one MEDIUM (no test pins that `Calculate` goes through the cache) and one LOW (doc says turbulent only); Code-Burler-Fix committed `a75caba` and `ceec8ba`. |
| Drift-Recheck | HEAD `ceec8ba` differed from the last compare's `efa12a1`, so it compared again at 18:52:36Z, clean in about 4 s; `compared_head` moved to `ceec8ba`. |
| Describe, PR-Review | The summary carries no regression text of its own. PR #4's body ends with `## Regression check`: scope full, baseline at `fa6627b`, "Compare: passed at `ceec8ba`", accepted drift none. PR-Review waited for a human from 18:53 to 19:14 with nothing in any status surface (`pr-review-wait-silent`); `cs:orch` ended it when the operator turned PR review off, and lym closed PR #4. |
| Finalize | Landing commit `99a49ce` ends with the same Regression check section. |

Gates, run dir `cached-friction-factor/dotnet`:

- `darn-write`: build once (6.2 s), tier0 `--no-build` over `CachedFrictionModelTests|PipeFlowTests|ResultFormatterTests` and the harness tests; 50 passed.
- A hand `lym gate test --targets ./tests/LymCsharp.Hydraulics.Tests` in review: build once, 78 passed.
- `code-burler-fix`: build once (6.2 s), computed scope, 51 passed.
- `finalize`: build once (2.4 s), tier0 over `LymCsharp.sln` 129 passed, tier1 3 passed.

## T2 `quantity-parse`: a review that asks for code

Released 20:16:06, landed 20:24:15 as `fd21dda`.

| Row | What it did |
|---|---|
| Drift-Baseline | Scope `full`; capture on `99a49ce` passed in about 4 s. |
| Darn-Write | One commit `80e63c9` adding `Quantity.Parse` and its tests. |
| Drift-Verify | Compare at 20:18:10 clean in 2 s; `done`, nothing accepted. |
| Code-Review | Three findings. Code-Burler-Fix committed `edbda77` (a code change: `Parse` reads the value without thousands separators, so `1,5 bar` is rejected, not read as 15) and `8a2852c` (tests for the non-numeric path and a pinned culture). |
| Drift-Recheck | After Code-Bouncer at 20:23:20 it spawned a compare at the fixer's head `8a2852c`, clean in 3 s; the row was done at 20:23:30. |
| Describe, PR-Review | The landing commit message ends with the Regression check: scope full, "Compare: passed at `8a2852c`", accepted drift none. PR review was off; no pull request opened. |

Gates, run dir `quantity-parse/dotnet`:

- `darn-write`: build once (5.9 s), tier0 over the Hydraulics and Regression test classes, 77 passed.
  `LymCsharp.Units.Tests` was left out of the computed scope although `Quantity.cs` is in `LymCsharp.Units`; the hand run `dotnet test tests/LymCsharp.Units.Tests` in review passed 29.
  I did not check whether the scoper omits a test project that covers the changed source file; the first trial's scoper read, from the diff, the test classes that reach it.
- `code-burler-fix`: build once (4.5 s), computed scope, 80 passed.
- `finalize`: build once (2.6 s), tier0 137 passed, tier1 3 passed.

## T3 `transitional-report-note`: a partial output change

Released at about 20:29 (not by me; the operator's correction at creel 611 had made it due after T2), landed 20:39:17 as `93bbb45`.

| Row | What it did |
|---|---|
| Drift-Baseline | Scope `full`, no skips; capture on `fd21dda` passed in about 4 s, six cases. |
| Drift-Await | Done at 20:29:58. |
| Darn-Write | One commit `6b1db61`: ` (transitional: friction factor uncertain)` appended in `ResultFormatter.Format` when the regime is transitional. |
| Drift-Verify | The compare at 20:31:59 exited 1. The agent accepted `water-pvc-2in-transitional` and `water-pvc-2in-transitional-churchill`, each with a reason that only the report line moved and the appended text matches the decision record. Under scope `full` the run halted for the parent's ruling at 20:32:38 and `cs:orch` was notified (batten notice, creel 42); the run resumed at 20:33:25, a second compare was clean, and the accepted-profile capture was taken at 20:33:31. The halt lasted 47 s. |
| Code-Review | The review held no findings; Code-Burler-Fix skipped ("fix skipped, the review held no findings and the gates pass"). |
| Drift-Recheck | HEAD was still the commit of the last compare, so the row was done with no compare at 20:36:05, as designed. |
| Describe, PR-Review | PR review off; the landing commit message lists both accepted tests with who accepted each and why, and "Compare: passed at `6b1db61`". |
| Finalize | The finalize gate's tier0 died with `Internal CLR error. (0x80131506)`, exit 134, 0 tests; Finalize treated that as a failing gate and spawned an agent fixer (`end=fixed spent=1 cap=2`) that changed no code; the rerun passed (`gate-runner-crash-fixer`). |

Gates, run dir `transitional-report-note/dotnet`:

- `darn-write`: build once, tier0 over `ResultFormatterTests` and the harness tests, 26 passed.
- `code-burler-fix`: build once, same scope, 26 passed.
- `finalize`: build once (2.3 s), tier0 crashed (10.7 s, 0 tests) and tier1 3 passed; after the fixer, build once (2.4 s), tier0 138 passed.

## Faults

Each fault is a bug note on the loomyard board, named by slug.
State as of the trial's end.

| Slug | Fault | State |
|---|---|---|
| `drift-baseline-preaccepts` | The baseline report pre-authorizes acceptance of drift that contradicts the task's stated invariant, steering Drift-Verify; the stencil has no rule for a body whose invariant and design disagree. | Folded by `ly:orch` into `drift-whole-review` at 20:06 |
| `pr-review-wait-silent` | A PR-Review waiting on the operator shows nothing: no `waiting` line in `lym loom status` or `lym batten status`, no creel post; T1's PR waited 21 min unseen. | Note |
| `gate-runner-crash-fixer` | A test runner that crashes with no result (exit 134) counts as a failing gate and spends a fixer session on a green tree; a second crash would spend the other attempt. | Note |

## Open and not run

- The fourth task (a regression the body does not reveal) was not written or run, by the operator's order; goal 1 stays open.
- Not checked: whether the dotnet scoper leaves out `LymCsharp.Units.Tests` for a change to `Quantity.cs` (T2's `darn-write` scope, above).
- Not checked: T2's and T3's Drift-Baseline reports and Drift-Verify agent reports, which went with the torn-down worktrees; T2 and T3 are read from traces, gate records and the landing commits.

## Interventions

- I released T1 and T2 and wrote the three task bodies.
  T3 was released by someone else at about 20:29, after I had acknowledged the operator's correction; I did not run its `set-status`.
- I posted the T1 heads-up to `cs:orch` (creel 31) and the notes' slugs to `ly:orch`.
- On `ly:orch`'s word, the operator turned PR review off on `cs` (about 19:14) and ordered, then corrected, the end of the trial (creel 604, 609, 611).
- I followed T1 with a Monitor on every row change, which woke me on each event; at the operator's request I replaced it with one that reports only Drift-Verify and Drift-Recheck results, PR-Review, stops and the run's end, and stopped it at the wrap-up.
