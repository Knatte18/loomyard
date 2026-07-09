# SANDBOX-BURLER-SUITE -- lyx burler + perch black-box suite

## What this is

A structured test-loop for exercising `lyx burler` and `lyx perch` against a **live psmux
server and a logged-in claude** in the sandbox Hub host repo. Like `SANDBOX-SHUTTLE-SUITE.md`,
the value here is partly **visual**: a burler round doing real review+fix work in a
pane, a verdict coming back. Not an automated suite -- an agent drives it, an
operator watches.

burler drives one review+fix round over an artifact: an A phase reviews the
target against a fasit (a source of truth) and writes a structured review file
(verdict + findings), then a B phase fixes what A found and writes a fixer
report. `perch` is the gate loop built on top: it spawns fresh burler rounds
until the artifact is `APPROVED` or definitively `STUCK`, with an operational
`PAUSED` exit in between. Scenarios S1-S3 prove one burler round end-to-end
through the debug CLI (`lyx burler run`), never review quality -- they are
deliberately trivial (a toy chair/table color mismatch) so the assertions are
about the mechanics (verdict parse, file contract, fix actually applied), not
about whether the review is insightful. S4 proves the perch gate loop
end-to-end (`lyx perch run`/`pause`) the same way -- convergence, run-dir
contents, weft commit, and pause/resume -- never judge quality.

## Pre-conditions

Before starting a session:

1. **Deploy a fresh binary.** Run `deploy.cmd` so `lyx.exe` on PATH is current source.
   The deployed binary is a snapshot -- re-deploy after any source change you want to test.
2. **Materialize the hub.** Run `sandbox-build.cmd` (or `sandbox-build.cmd -reset`
   to start clean); the session cwd is the Hub host repo root, the same operating model
   as the main suite.
3. **Live-psmux and claude requirement.** `psmux.exe` on PATH (installed at
   `C:\Code\tools\bin\psmux.exe`), PowerShell 7, and a logged-in `claude` on PATH.
   If any of these is unavailable in the session, **note that as the session outcome
   rather than treating it as a burler/perch defect** -- the `**Covers:** burler` tag on
   S1 and the `**Covers:** perch` tag on S4 satisfy the sandbox coverage guard
   (`sandbox_coverage_test.go`) regardless of runtime availability.
4. **`lyx init` first.** `lyx burler run` requires an initialized worktree
   (`_lyx/config/shuttle.yaml` and `mux.yaml`) exactly like `lyx shuttle` and
   `lyx mux` do -- burler wires the real shuttle substrate (mux + claude) on
   every invocation and has no config file of its own; the profile YAML is the
   only burler-specific input. `lyx perch run`/`pause` additionally need
   `_lyx/config/perch.yaml` (also created by `lyx init`) -- perch's own config
   holds only the judge model/effort and the default round-cap ladder, all of
   which the sandbox-build default template already sets sanely.
5. **Attached interactive terminal.** Launch `sandbox-burler-suite.cmd` from a
   real, attached console -- never redirected, backgrounded, or detached.
   Without a TTY the driving claude session cannot idle between turns waiting
   for notifications, so the process ends as soon as a turn ends and the
   remaining scenarios are silently abandoned (observed live: S1 completed,
   S2/S3 never ran, no `sandbox-report.json`). The launcher prints a warning
   when it detects non-console stdio.

## Black-box rule

**The agent under test works exclusively inside the Hub host repo (`lyx-test-HUB/lyx-test`).
It tests `lyx.exe` as a black box -- exactly as a real user with only the binary on PATH.
It must not look for, read, or reason about the lyx source tree. No peeking at
`C:\Code\loomyard\` or any other path outside the Hub.**

Discovering the command surface is done via `lyx burler --help`/`lyx burler run --help`
(S1-S3) and `lyx perch --help`/`lyx perch run --help`/`lyx perch pause --help` (S4) alone
-- not from documentation outside the Hub. The profile YAML file is the one artifact the
agent must construct itself (paths, rubric, fix-scope, output paths, and for S4 the gate/
round-caps keys) per the scenario's Goal below; each command's `--help` example profile is
the reference for the file's shape.

## Fingerprint header

The launcher prepends a "binary under test" fingerprint block to this file when it
copies it into the Hub host repo. The fingerprint records the absolute path, file size,
modification time, and a short SHA-256 of the `lyx.exe` binary at launch time.

The same fingerprint identifies the binary for the report's provenance: a separate
fetch step (run after this session) stamps it into `meta.fingerprint` of the
fetched `sandbox-report.json` so a maintainer can reproduce the exact binary that
produced each finding. The agent does not need to transcribe the fingerprint
anywhere itself.

## How to run a scenario

For each scenario below:

- Read the **Goal** -- it names the task, not the commands. Discover the commands via
  each module's own `--help` (S0 ethos).
- Run every scenario-driving command in the **foreground** and wait for it to
  return before moving on. **Never background or detach a command** -- `lyx
  burler run` and `lyx perch run` both block until they reach a terminal outcome
  by design, so there is nothing to wait for asynchronously, and no completion
  notification will ever be delivered back into this session. Assume no async
  signal arrives, ever. (The one exception is S4's pause step, which deliberately
  runs `lyx perch pause` from a SECOND terminal while the first `lyx perch run`
  invocation is still blocking in its own terminal.)
- **Watch** what lyx does. Note where it stalls, guesses wrong, or hits an error.
- Record the outcome per the verdict buckets: `OK` (worked) / `WARN` (rough edge) /
  `FAIL` (broke).

## Verdict key

- `OK`   -- completed without friction
- `WARN` -- completed but with confusion, awkward UX, or a non-fatal error
- `FAIL` -- did not complete; lyx broke, panicked, or gave wrong output

## Capturing findings

After all scenarios are run, write **all** `WARN`/`FAIL` findings to `./sandbox-report.json`
(in the host-repo cwd) on this exact schema. **Always write the file, even when there are
zero `WARN`/`FAIL` findings** -- in that case `items` is an empty array.

```json
{
  "source": "sandbox-report",
  "items": [
    {
      "ref": "S2",
      "title": "…",
      "body": "verdict: WARN\n\n…repro…"
    }
  ]
}
```

- `source` is the literal string `"sandbox-report"`.
- `items[]` holds only `WARN`/`FAIL` findings -- do not record `OK` scenarios here.
- `ref` is the scenario id (`S1`-`S5`).
- `title` is a short one-line summary.
- `body` folds the detail, repro steps, and verdict into one markdown string.

Write only `source` and `items` -- a separate fetch step (run after the session)
stamps `meta` (including the binary fingerprint). Confine all free text to the
`title`/`body` string fields so the JSON stays well-formed.

## Scenarios

### S1 -- The toy round (BLOCKING path)

**Covers:** burler

**Goal:** "Create a fixture text file describing a chair and a table whose colors
DO NOT match, write a profile YAML that reviews it against the rule 'the chair's
color must match the table's color', run `lyx burler run --profile <file>`, and
confirm the round finds the mismatch, fixes it, and reports the fix."

**Watch:** Create a small fixture file in the Hub host repo (e.g. `chair-table.txt`)
whose text states a chair color and a table color that disagree (e.g. "The chair
is red. The table is blue."). Write a profile YAML naming that file as `target`,
an inline `fasit.instructions` stating the rule "the chair's color must match the
table's color" (no fasit paths needed -- the rule itself is the source of truth
here), a short `rubric` mapping a color mismatch to a BLOCKING finding, `fix-scope:
overlay`, `tool-use: false`, `cluster-n: 0`, and fresh `review-path` /
`fixer-report-path` (files that do not already exist). Run
`lyx burler run --profile <file>`. The command blocks until the round finishes;
the printed JSON envelope reports `"outcome":"done"` and `"verdict":"BLOCKING"`;
the review file at `review-path` opens with YAML frontmatter carrying at least one
`BLOCKING`-severity finding; the fixture file's content has actually changed so
the chair's and table's colors now match; and the fixer-report at
`fixer-report-path` exists and is non-empty (it must describe what was fixed).

**Verdict:** `OK` / `WARN` / `FAIL`

---

### S2 -- The APPROVED path

**Covers:** burler

**Goal:** "Repeat S1's setup but with the chair and table colors already matching,
and confirm the round reports APPROVED instead of BLOCKING."

**Watch:** Reuse S1's fixture file (or a fresh copy) but edit it so the chair's and
table's colors already match (e.g. "The chair is red. The table is red."). Write a
profile YAML identical in shape to S1's but with fresh `review-path` /
`fixer-report-path` (S1's output files already exist and must not be reused). Run
`lyx burler run --profile <file>`. The JSON envelope reports `"outcome":"done"`
and `"verdict":"APPROVED"`; the review file's frontmatter carries zero
`BLOCKING`-severity findings (non-blocking MEDIUM/LOW/NIT polish findings are
legal and do not fail this scenario); the fixer-report still exists and is
non-empty (it is written unconditionally every round, even with nothing to fix --
it should say so).

**Verdict:** `OK` / `WARN` / `FAIL`

---

### S3 -- Black-box error paths

**Covers:** burler

**Goal:** "Confirm four profile-level mistakes are each rejected with a distinct,
sane error in the JSON envelope: an unsupported cluster count, an empty fasit, a
re-run against an already-existing review-path, and a review-path identical to
the fixer-report-path."

**Watch:** Four separate `lyx burler run` invocations, each expected to exit
non-zero with an error in the JSON envelope (not a panic, not a silent
zero-exit):

1. Take a valid profile (e.g. a copy of S1's) and set `cluster-n: 1`. The run
   must fail with an error naming cluster fan-out as unsupported in v1.
2. Take a valid profile and clear `fasit` entirely (empty `paths` and empty
   `instructions`). The run must fail with a validation error naming the empty
   fasit -- not silently degrade to reviewing the target in isolation.
3. Re-run S1's exact profile file unmodified (same `review-path` that S1 already
   wrote). The run must fail with shuttle's pre-existing-output-file rejection
   -- burler never silently overwrites a prior round's artifact.
4. Take a valid profile and set `fixer-report-path` to the SAME value as
   `review-path` (a plausible copy-paste mistake). The run must fail with a
   validation error naming the two fields and stating they must not be the same
   path -- burler must never let the two-artifact file contract collapse into
   one file wearing both hats.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### S4 -- The perch gate loop (convergence, then pause/resume)

**Covers:** perch

**Goal:** "Write a small fixture doc with a couple of deliberate flaws and a perch profile
(`gate.mode: llm-verdict`, `round-caps: [2, 3]`) reviewing it against a short fasit rule, run
`lyx perch run --profile <file>` to convergence, and inspect the run dir and the weft commit.
Then start a second, longer-running block, pause it mid-flight with `lyx perch pause --run-id
<id>`, and confirm it exits `PAUSED` and that re-running `lyx perch run` resumes at the recorded
round instead of starting over."

**Watch:** Discover the command surface via `lyx perch --help` and `lyx perch run --help` alone
(S0 ethos) -- `--help`'s example profile is the reference for the profile file's shape. Create a
small fixture file (e.g. `essay.txt`) with two or three sentences carrying a couple of clear,
easy-to-fix flaws (a factual contradiction, a typo repeated twice). Write a profile YAML naming
that file as `target`, an inline `fasit.instructions` stating the correctness rule, a short
`rubric` mapping each flaw to a BLOCKING finding, `fix-scope: overlay`, `gate: {mode:
llm-verdict}`, `round-caps: [2, 3]`. Run `lyx perch run --profile <file>`. The command blocks
until the block reaches a terminal outcome; the printed JSON envelope reports
`"outcome":"APPROVED"` within the 3-round cap, plus `roundsRun`, `runId`, `runDir`, and
`weftCommitted`. Inspect `runDir` (the path from the envelope): it holds `state.json` and one
`round-<N>-review.md` / `round-<N>-fixer-report.md` pair per round actually run, numbered from 1;
the fixture file's content has actually changed and no longer carries the seeded flaws. Confirm
the weft commit landed (`git -C <weft worktree> log -1` on the host's weft sibling shows a
`perch: <runId> APPROVED` commit, or use `lyx weft status`).

For the pause/resume step, write a SECOND fixture + profile pair whose flaws are subtler (so the
block is likely to still be running after round 1) with a fresh `run-id` implied by the new
profile's filename (or pass an explicit `--run-id`). Start `lyx perch run --profile <file2>` and,
while it is still running its later rounds, run `lyx perch pause --run-id <id>` from a second
terminal against the same worktree. The pause command's own JSON envelope reports `runId` and
`pauseFile`. The running block's own envelope (once it returns) reports `"outcome":"PAUSED"` --
never mid-round, always at the next round boundary. Re-run `lyx perch run --profile <file2>
--run-id <id>` (same run-id): the envelope confirms the block resumed rather than restarting
(`roundsRun` continues climbing from where it paused, not from 0) and eventually reaches a
terminal `APPROVED`/`STUCK` outcome.

**Verdict:** `OK` / `WARN` / `FAIL`

### S5 -- The perch command gate (convergence decided by a real command)

**Covers:** perch

**Goal:** "Run one perch block whose convergence is decided by a real command instead of the
review verdict (`gate.mode: command`), watch a failing command block convergence and feed its
output forward, then watch a passing command converge the block regardless of the verdict."

**Watch:** Write a tiny fixture file plus a profile with `gate: {mode: command, command:
[<argv>], timeout: "1m"}` and `round-caps: [2]`, where `<argv>` is a command that FAILS in the
host repo (e.g. an unknown `git` verb). Run `lyx perch run --profile <file>`: each round's
review may even come back APPROVED, but the block must NOT converge while the command fails --
it runs to the hard cap and exits `STUCK`/`hard-cap`. Inspect the run dir: every round has a
`round-<N>-gate.md` carrying the command's real combined output with a FAIL header, and (from
round 2 on) the previous round's gate file is part of what the next round's agent was told
about. Then edit the profile to a command that PASSES (e.g. `["git", "status"]`), re-run with
a fresh `--run-id`: the block converges `APPROVED` in round 1 even if the review found
BLOCKING findings -- in command mode only the command decides. A command that exceeds
`gate.timeout` counts as a FAILING gate (its `round-<N>-gate.md` notes the timeout), never as
a crash of the block.

**Verdict:** `OK` / `WARN` / `FAIL`

## Session log format

After running all scenarios, record a short session summary:

```
Date: <YYYY-MM-DD>
Binary fingerprint: <copy from the header above>

S1: <OK|WARN|FAIL> -- <one-line note if not OK>
S2: <OK|WARN|FAIL> -- <one-line note if not OK>
S3: <OK|WARN|FAIL> -- <one-line note if not OK>
S4: <OK|WARN|FAIL> -- <one-line note if not OK>
S5: <OK|WARN|FAIL> -- <one-line note if not OK>

sandbox-report.json written: <count of WARN/FAIL items>
```

`./sandbox-report.json` must be written before the session ends, per the Capturing
findings section above -- with `items: []` when every scenario was `OK`.

## Teardown

After the session summary is recorded and `./sandbox-report.json` is written, run
`lyx mux down` to tear down the psmux session/server the scenarios booted with
`lyx mux up`. An orphaned psmux server holds open handles inside the Hub host
repo and blocks the next `sandbox-build.cmd -reset`. The launcher also runs
`lyx mux down` itself after the session ends (deterministic backstop), but run
it here anyway -- defense-in-depth, and it keeps the Hub clean while the session
is still open for inspection.

## Notes

- Host/weft scenarios stay in `SANDBOX-CORE-SUITE.md`, mux/psmux scenarios stay in
  `SANDBOX-MUX-SUITE.md`, shuttle black-box agent scenarios stay in
  `SANDBOX-SHUTTLE-SUITE.md`; this suite holds both burler (review+fix round
  scenarios) and perch (the round-loop gate built on top of burler) -- add `S`
  scenarios here, not in any other suite.
