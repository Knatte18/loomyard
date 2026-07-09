# SANDBOX-BURLER-SUITE -- lyx burler black-box suite

## What this is

A structured test-loop for exercising `lyx burler` against a **live psmux
server and a logged-in claude** in the sandbox Hub host repo. Like `SANDBOX-SHUTTLE-SUITE.md`,
the value here is partly **visual**: a burler round doing real review+fix work in a
pane, a verdict coming back. Not an automated suite -- an agent drives it, an
operator watches.

burler drives one review+fix round over an artifact: an A phase reviews the
target against a fasit (a source of truth) and writes a structured review file
(verdict + findings), then a B phase fixes what A found and writes a fixer
report. Scenarios S1-S3 prove one burler round end-to-end through the debug
CLI (`lyx burler run`), never review quality -- they are deliberately trivial
(a toy chair/table color mismatch) so the assertions are about the mechanics
(verdict parse, file contract, fix actually applied), not about whether the
review is insightful. `perch`, the gate loop built on top of burler (it spawns
fresh burler rounds until an artifact is `APPROVED` or definitively `STUCK`),
has its own scenarios in `SANDBOX-PERCH-SUITE.md` -- not tested here.

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
   rather than treating it as a burler defect** -- the `**Covers:** burler` tag on
   S1 satisfies the sandbox coverage guard (`sandbox_coverage_test.go`) regardless
   of runtime availability.
4. **`lyx init` first.** `lyx burler run` requires an initialized worktree
   (`_lyx/config/shuttle.yaml` and `mux.yaml`) exactly like `lyx shuttle` and
   `lyx mux` do -- burler wires the real shuttle substrate (mux + claude) on
   every invocation and has no config file of its own; the profile YAML is the
   only burler-specific input.
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
alone -- not from documentation outside the Hub. The profile YAML file is the one
artifact the agent must construct itself (paths, rubric, fix-scope, output paths) per
the scenario's Goal below; each command's `--help` example profile is the reference for
the file's shape.

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
  `lyx burler`'s own `--help` (S0 ethos).
- Run every scenario-driving command in the **foreground** and wait for it to
  return before moving on. **Never background or detach a command** -- `lyx
  burler run` blocks until it reaches a terminal outcome by design, so there
  is nothing to wait for asynchronously, and no completion notification will
  ever be delivered back into this session. Assume no async signal arrives,
  ever.
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
- `ref` is the scenario id (`S1`-`S3`).
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

## Session log format

After running all scenarios, record a short session summary:

```
Date: <YYYY-MM-DD>
Binary fingerprint: <copy from the header above>

S1: <OK|WARN|FAIL> -- <one-line note if not OK>
S2: <OK|WARN|FAIL> -- <one-line note if not OK>
S3: <OK|WARN|FAIL> -- <one-line note if not OK>

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
  `SANDBOX-SHUTTLE-SUITE.md`, perch gate-loop scenarios stay in
  `SANDBOX-PERCH-SUITE.md`; this suite holds only burler's own review+fix round
  scenarios -- add `S` scenarios here, not in any other suite.
