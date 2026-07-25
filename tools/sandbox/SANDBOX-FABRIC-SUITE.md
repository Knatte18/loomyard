# SANDBOX-FABRIC-SUITE -- lyx fabric black-box agent suite

## What this is

A structured test-loop for exercising `lyx fabric` against **dedicated** GitHub test
repos (`Knatte18/lyx-fabric-test` as host, `Knatte18/lyx-fabric-test-weft` as weft) --
never the shared `lyx-test`/`lyx-test-weft` repos the main and per-module suites use.
fabric is a parallel-build module (see `docs/overview.md`): it exists alongside warp
and weft, and this suite proves it holds up on its own dedicated hub rather than
interfering with (or being interfered with by) the warp/weft sandbox state.

Like the other suites, the value is a Claude session driving `lyx fabric` by hand in a
real hub, treating every break, surprise, or rough edge as a LoomYard finding to
record in the report.

## Pre-conditions

Before starting a session:

1. **Deploy a fresh binary.** Run `deploy.cmd` so `lyx.exe` on PATH is current source.
   The deployed binary is a snapshot -- re-deploy after any source change you want to test.
2. **Materialize the fabric hub.** Run `sandbox-fabric-suite.cmd`. Unlike the main and
   per-module suites (which assume `sandbox-build.cmd` already ran), this launcher
   clones the dedicated fabric hub itself via `lyx fabric clone` -- idempotently: if
   `lyx-fabric-test-HUB` already exists it is reused as-is, never reset or re-cloned.
   The very first run on a machine performs the real clone; every run after that starts
   from whatever state the previous session left the hub in.
3. **`lyx` on PATH.** Confirm `lyx --help` works from any directory.

### PowerShell JSON-quoting

See the PowerShell JSON-quoting note in `SANDBOX-CORE-SUITE.md`'s Pre-conditions --
the same caveat applies here whenever a scenario below passes JSON as an argument.

### Operating model

lyx resolves against the current directory's own `_lyx/` and does **not** walk up to a
parent. The hub host repo is initialized at its root, so the agent runs the entire
session from there (cwd is fixed at the root).

## Black-box rule

**The agent under test works exclusively inside the dedicated fabric Hub host repo
(`lyx-fabric-test-HUB/lyx-fabric-test`). It tests `lyx.exe` as a black box -- exactly
as a real user with only the binary on PATH. It must not look for, read, or reason
about the lyx source tree. No peeking at `C:\Code\loomyard\` or any other path outside
the Hub.**

Discovering the command surface is done via `lyx fabric`, `lyx fabric <subcommand>`,
and `lyx fabric <subcommand> --help` alone -- not from documentation outside the Hub.

## Fingerprint header

The launcher prepends a "binary under test" fingerprint block to this file when it
copies it into the fabric Hub host repo. The fingerprint records the absolute path,
file size, modification time, and a short SHA-256 of the `lyx.exe` binary at launch
time.

The same fingerprint identifies the binary for the report's provenance: a separate
fetch step (run after this session) stamps it into `meta.fingerprint` of the fetched
`sandbox-report.json` so a maintainer can reproduce the exact binary that produced
each finding. The agent does not need to transcribe the fingerprint anywhere itself.

## How to run a scenario

For each scenario below:

- Read the **Goal** -- it names the task, not the commands. Discover the commands via
  `lyx fabric`, `lyx fabric <subcommand>`, and `--help` flags (F0 ethos).
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
      "ref": "F2",
      "title": "…",
      "body": "verdict: WARN\n\n…repro…"
    }
  ]
}
```

- `source` is the literal string `"sandbox-report"`.
- `items[]` holds only `WARN`/`FAIL` findings -- do not record `OK` scenarios here.
- `ref` is the scenario id (`F0`-`F3`).
- `title` is a short one-line summary.
- `body` folds the detail, repro steps, and verdict into one markdown string.

Write only `source` and `items` -- a separate fetch step (run after the session)
stamps `meta` (including the binary fingerprint). Confine all free text to the
`title`/`body` string fields so the JSON stays well-formed.

## Scenarios

### F0 -- Discovery (help surface smoke test)

**Goal:** "You have `lyx` on PATH and nothing else inside this repo. Find out what
`lyx fabric` can do and report its full command tree."

**Watch:** Does `lyx fabric` list all 14 verbs (`clone`, `add`, `list`, `remove`,
`checkout`, `pairs`, `reconcile`, `prune`, `cleanup`, `status`, `commit`, `push`,
`pull`, `sync`)? Does each `--help` explain itself? Is each description accurate and
useful?

**Verdict:** `OK` / `WARN` / `FAIL`

---

### F1 -- Clone geometry

**Covers:** fabric

**Goal:** "Confirm the dedicated fabric hub the launcher just materialized (or reused)
looks the way `lyx fabric clone` promises."

**Watch:** The board passenger's origin URL is the **default derived** form --
`<weft-url>.wiki.git` (the operator has already initialized that wiki; do not attempt
to create it yourself). The weft prime's checked-out branch is **`main-weft`**, not
`main` -- fabric's uniform branch-suffix scheme applies from the very first pair, unlike
`warp clone`'s mirrored (identical) branch names. Use `lyx fabric pairs` and plain git
(`git -C <weft-prime> branch --show-current`, `git -C _board remote -v`) to confirm both;
neither should require guessing or `ls`-ing around.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### F2 -- Topology lifecycle

**Goal:** "Add a new host+weft worktree pair, inspect it, coordinate-checkout it,
reconcile it, then prune and clean it up."

**Watch:** Run `lyx fabric add <slug>` and confirm the new weft branch is named
`<host-branch>-weft` (the fixed suffix, not a mirrored name -- e.g. adding slug `foo`
with an empty branch prefix yields host branch `foo` and weft branch `foo-weft`). Does
`lyx fabric pairs` report the new pair as in-sync? Does `lyx fabric checkout` switch
both sides together and re-point the junction? After removing the host side by hand
(or via `lyx fabric remove`), does `lyx fabric reconcile` report and repair drift
sanely, and do `lyx fabric prune`/`lyx fabric cleanup` (dry-run first, then `--apply`)
correctly identify the orphaned `<slug>-weft` branch as fabric-managed (by its suffix)
and handle it per the flag matrix described in `lyx fabric cleanup --help`?

**Verdict:** `OK` / `WARN` / `FAIL`

---

### F3 -- Weft content sync

**Goal:** "Make a small, clearly-marked change inside the weft-tracked scope and run
it through `fabric status`, `commit`, `push`, `pull`, and `sync`."

**Watch:** Does `fabric status` report the change accurately? Do `commit`/`push`
mirror it to the weft remote? The commit message is always the fixed string
`"weft sync"` -- it is not generated from changed files and there is no `-m` flag to
customize it (confirm via `lyx fabric commit --help`). Every fabric weft commit also
carries a trailing `Warp-SHA: <sha>` trailer naming the paired host repo's current
HEAD -- inspect the commit body (e.g. `git -C <weft-worktree> log -1`) and confirm the
trailer is present and names a real, resolvable warp commit. `fabric sync` pushes via
a detached child process, so `status` immediately after `sync` may lag behind the
actual push -- a confusing-but-expected rough edge to note as a `WARN`, not to
pre-judge here (mirroring `SANDBOX-CORE-SUITE.md`'s S7 guidance for `weft sync`).
Staging is scoped to the directories listed in the fabric config (default `_lyx`), so
the test change should land inside that scope to be picked up at all.

**Verdict:** `OK` / `WARN` / `FAIL`

---

## Session log format

After running all scenarios, record a short session summary:

```
Date: <YYYY-MM-DD>
Binary fingerprint: <copy from the header above>

F0: <OK|WARN|FAIL> -- <one-line note if not OK>
F1: <OK|WARN|FAIL> -- <one-line note if not OK>
F2: <OK|WARN|FAIL> -- <one-line note if not OK>
F3: <OK|WARN|FAIL> -- <one-line note if not OK>

sandbox-report.json written: <count of WARN/FAIL items>
```

`./sandbox-report.json` must be written before the session ends, per the Capturing
findings section above -- with `items: []` when every scenario was `OK`.

## Notes

- This suite is deliberately scoped to fabric alone and runs against its own
  dedicated hub -- it does not touch, and is not touched by, `SANDBOX-CORE-SUITE.md`'s
  warp/weft scenarios against `lyx-test`/`lyx-test-weft`.
- fabric is a parallel-build module (see `docs/overview.md` and
  `manifest/designs/fabric.md`): it is not yet the default, and warp/weft remain the
  owners of the shared sandbox hub until cutover.
