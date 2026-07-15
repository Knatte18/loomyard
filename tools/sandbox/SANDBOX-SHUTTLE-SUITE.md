# SANDBOX-SHUTTLE-SUITE -- lyx shuttle black-box agent suite

## What this is

A structured test-loop for exercising `lyx shuttle` against a **live tmux server and a
logged-in claude** in the sandbox Hub host repo. Like `SANDBOX-MUX-SUITE.md`, the value
here is partly **visual**: a strand's pane doing real agent work, an outcome coming back.
Not an automated suite -- an agent drives it, an operator watches.

## Pre-conditions

Before starting a session:

1. **Deploy a fresh binary.** Run `deploy.cmd` so `lyx.exe` on PATH is current source.
   The deployed binary is a snapshot -- re-deploy after any source change you want to test.
2. **Materialize the hub.** Run `sandbox-build.cmd` (or `sandbox-build.cmd -reset`
   to start clean); the session cwd is the Hub host repo root, the same operating model
   as the main suite.
3. **Live-tmux and claude requirement.** tmux (or the Windows tmux port) on PATH, PowerShell 7, and a logged-in `claude` on PATH.
   If any of these is unavailable in the session, **note that as the session outcome
   rather than treating it as a shuttle defect** -- the `**Covers:** shuttle` tag on S1
   satisfies the sandbox coverage guard (`sandbox_coverage_test.go`) regardless of
   runtime availability.
4. **`lyx init` first.** `lyx shuttle` requires an initialized worktree
   (`_lyx/config/shuttle.yaml` and `mux.yaml`) exactly like `lyx mux` does.

## Black-box rule

**The agent under test works exclusively inside the Hub host repo (`lyx-test-HUB/lyx-test`).
It tests `lyx.exe` as a black box -- exactly as a real user with only the binary on PATH.
It must not look for, read, or reason about the lyx source tree. No peeking at
`C:\Code\loomyard\` or any other path outside the Hub.**

Discovering the command surface is done via `lyx shuttle`, `lyx shuttle <subcommand>`,
and `lyx shuttle <subcommand> --help` alone -- not from documentation outside the Hub.

### Controlled tmux exceptions

One sanctioned deviation from the pure black-box rule, mirroring the mux suite's own
controlled-exception note:

- **Direct `tmux -L <socket> list-panes`/`ls`** is allowed only to confirm a strand's
  pane exists (or was cleaned up), where `<socket>` is read from the shuttle run's
  strand guid cross-referenced against `lyx mux status` output.
- **Scenario S2's operator attach** is operator-assisted -- see S2 below.

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
  `lyx shuttle`, `lyx shuttle <subcommand>`, and `--help` flags (S0 ethos).
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

### S1 -- Autonomous happy path

**Covers:** shuttle

**Goal:** "Ask a shuttle agent to write a specific file, and confirm the run reports
`done` once it does."

**Watch:** `lyx shuttle run --prompt "write the single line OK into result.md and
nothing else" --output-file result.md` starts a strand (visible as a pane, confirmable
via `lyx mux status` or `tmux -L <socket> list-panes`); the command blocks until the
agent finishes; the printed JSON envelope reports `"outcome":"done"` with a `sessionId`
and `guid`; `result.md` exists with the expected content; and afterward the strand's
pane and run directory are cleaned up (no leftover pane, `lyx mux status` no longer
lists the guid).

**Verdict:** `OK` / `WARN` / `FAIL`

---

### S2 -- Asking path (operator-assisted)

**Covers:** shuttle

**Goal:** "Give a shuttle agent a task it cannot complete without a decision only the
operator can make, and confirm the run reports `asking` with the question, then let the
operator answer it directly in the pane."

**Watch:** `lyx shuttle run --prompt "before writing decision.md, stop and ask me
which of two options you should pick — do not guess" --output-file decision.md
--interactive` blocks, then returns with `"outcome":"asking"` and a non-empty
`lastAssistantMessage` carrying the question; the strand and its pane are still alive
(`lyx mux status` still lists the guid; `decision.md` does not exist yet). The agent
then instructs the operator to attach (`lyx mux attach` in a second terminal, per the
mux suite's M7/M14 pattern), answer the question in the pane, and confirm the agent
continues and eventually writes `decision.md`.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### S3 -- Interrupt and send (cross-terminal poke)

**Covers:** shuttle

**Goal:** "Start a long-running shuttle agent from one terminal, then from a second
terminal interrupt its current turn and send it a one-line update, and confirm the
agent continues from the new instruction."

**Watch:** Start a long-running run in one terminal, e.g. `lyx shuttle run --prompt
"count slowly to a very large number out loud, one number per line, before writing
done.md" --output-file done.md`. From a second terminal, note the `guid` (via
`lyx mux status`) and run `lyx shuttle interrupt <guid>` -- the agent's current turn
stops without killing its pane or session (`lyx mux status` still shows it `live:
true`). Then run `lyx shuttle send <guid> "stop counting and write done.md right
away"` -- a single-line update only. The deterministic property to verify is that
`done.md` eventually appears with the redirected content: the first terminal's
envelope may report either `"outcome":"done"` or `"outcome":"asking"`, because the
interrupted turn's own Stop event can resolve the blocking run before the redirect
turn starts (the documented v1 no-re-wait limitation) -- an `asking` envelope with
`done.md` correctly written is a PASS, not a failure. Only `died`/`timeout` (or a
missing/wrong `done.md`) is a real failure here. Sending multiline text
must be rejected outright (a "must be a single line" error), not silently truncated
or mis-submitted.

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

## Notes

- Host/weft scenarios stay in `SANDBOX-CORE-SUITE.md`, mux/tmux scenarios stay in
  `SANDBOX-MUX-SUITE.md`; this suite grows with shuttle (a second engine, cluster
  reviews) -- add `S` scenarios here, not in either other suite.
