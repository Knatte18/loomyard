# SANDBOX-SUITE -- lyx black-box agent suite

## What this is

A structured test-loop for exercising `lyx` against the real GitHub test repos
(`Knatte18/lyx-test` as host, `Knatte18/lyx-test-weft` as weft). Not an automated
suite -- the value is a Claude session driving `lyx` by hand in a real hub, treating
every break, surprise, or rough edge as a LoomYard bug to file.

This parallels how millhouse was bootstrapped: get lyx working well enough that an
agent can operate it in a real repo, then use that experience to harden lyx.

## Pre-conditions

Before starting a session:

1. **Deploy a fresh binary.** Run `deploy.cmd` so `lyx.exe` on PATH is current source.
   The deployed binary is a snapshot -- re-deploy after any source change you want to test.
2. **Materialize the hub.** Run `sandbox.cmd` (or `sandbox.cmd -reset` to start clean)
   to clone the host and weft into a fresh `lyx-test-HUB`.
3. **`lyx` on PATH.** Confirm `lyx --help` works from any directory.
4. **`gh` installed and authenticated.** The `lyx selfreport create` command delegates to
   the `gh` CLI. Run `gh auth status` to confirm authentication before starting.

### PowerShell JSON-quoting

When driving the suite from Windows PowerShell (the assumed session shell on Windows),
backslash-escaping a JSON argument is the intuitive-but-wrong move and yields:

```
{"error":"invalid json: invalid character '\\' looking for beginning of object key string","ok":false}
```

The working form is a single-quoted string with literal inner double quotes, e.g.

```powershell
lyx board upsert '{"slug":"s3-demo","title":"S3 demo"}'
```

### Operating model

lyx resolves against the current directory's own `_lyx/` and does **not** walk up to a
parent. The hub host repo is initialized at its root, so the agent runs the entire
session from there (cwd is fixed at the root). Running a lyx command from a subdirectory
that has not itself been initialized correctly reports
`not initialized here; run "lyx init"` — that is expected behaviour, **not a finding**.
Note: `lyx init` in a subdirectory would create `_lyx/` there and make lyx work in that
subdir, but the agent must **not** scaffold nested `_lyx/` during a session.

## Black-box rule

**The agent under test works exclusively inside the Hub host repo (`lyx-test-HUB/lyx-test`).
It tests `lyx.exe` as a black box -- exactly as a real user with only the binary on PATH.
It must not look for, read, or reason about the lyx source tree. No peeking at
`C:\Code\loomyard\` or any other path outside the Hub.**

Discovering the command surface is done via `lyx`, `lyx <module>`, and
`lyx <module> <subcommand> --help` alone -- not from documentation outside the Hub.

## Fingerprint header

The launcher prepends a "binary under test" fingerprint block to this file when it
copies it into the Hub host repo. The fingerprint records the absolute path, file size,
modification time, and a short SHA-256 of the `lyx.exe` binary at launch time.

**Every issue filed during this session must include that fingerprint in the issue body**
so that a maintainer can reproduce the exact binary that triggered the finding.

## How to run a scenario

For each scenario below:

- Read the **Goal** -- it names the task, not the commands. Discover the commands via
  `lyx`, `lyx <module>`, and `--help` flags (S0 ethos).
- **Watch** what lyx does. Note where it stalls, guesses wrong, or hits an error.
- Record the outcome per the verdict buckets: `OK` (worked) / `WARN` (rough edge) /
  `FAIL` (broke).

## Verdict key

- `OK`   -- completed without friction
- `WARN` -- completed but with confusion, awkward UX, or a non-fatal error
- `FAIL` -- did not complete; lyx broke, panicked, or gave wrong output

## Capturing findings

After all scenarios are run, file **each** non-`OK` finding as a GitHub issue on the
LoomYard repository using `lyx selfreport create` from inside the Hub host repo.

**Discover the command's flags via `lyx selfreport create --help`** before using it.

There is no harvester and no `lyx board upsert` step. `lyx selfreport create` is the only
capture path. Each issue must include the fingerprint header (see above) in the body.

## Scenarios

### S0 -- Discovery (help surface smoke test)

**Goal:** "You have `lyx` on PATH and nothing else inside this repo. Find out what it
can do and report the full command tree."

**Watch:** Does `lyx` alone list modules? Does `lyx <module>` list subcommands? Is each
description accurate and useful? Any command that cannot be discovered from the binary
alone is a help gap.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### S1 -- Hub orientation

**Goal:** "You are inside a hub that was set up from a host and a weft. Figure out what
the hub contains and what state it is in."

**Watch:** Can you tell host from weft from board using only `lyx` commands? Does any
`lyx` command report hub geometry or status? If you have to `ls` and guess, that is a
missing command surface.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### S2 -- First real work in the host

**Goal:** "Create something in the host repo (a file, a small change) and get it
committed and tracked the way lyx intends."

**Watch:** The host is an ordinary git repo — committing host changes with plain `git`
is acceptable and **not** a finding. Watch lyx's actual responsibility: host/weft
coordination (junctions wired correctly, weft mirroring behaves).

**Verdict:** `OK` / `WARN` / `FAIL`

---

### S3 -- Board and task interaction

**Goal:** "Add a task to the board, list tasks, change its state."

**Note:** When passing JSON in PowerShell, use single-quoted strings with literal inner
double quotes — see the PowerShell JSON-quoting note in Pre-conditions.

**Durability note:** The board is durable across sessions — it starts non-empty (e.g. a
`T1 "Test task from S3"` task persists from prior runs). Do not assume a fresh board.
Use `lyx board list` to observe current state before adding tasks, and use
`lyx board remove` to clean up any test tasks you create at session end.

**Watch:** Board CRUD via `lyx board`. JSON output sane. State transitions work.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### S4 -- Config round-trip

**Goal:** "Inspect lyx's config for this hub, change a value, confirm it took."

**Watch:** From the worktree root, does `lyx config` read/write the correct
`_lyx/config/` and round-trip a value?

**Verdict:** `OK` / `WARN` / `FAIL`

---

### S6 -- Wrong-directory and error ergonomics

**Goal:** "Run a hub-only command from outside the hub. Run a command with a bad flag.
Run an unknown subcommand."

**Watch:** Are errors legible? Does lyx say what to do, or just fail? This is where
standalone usability lives or dies. A legible `not initialized` / "run from the
initialized root"-style message is the `OK` (ergonomics-pass) outcome — not a `FAIL`.
Do not file it as a finding.

**Verdict:** `OK` / `WARN` / `FAIL`

## Session log format

After running all scenarios, record a short session summary:

```
Date: <YYYY-MM-DD>
Binary fingerprint: <copy from the header above>

S0: <OK|WARN|FAIL> -- <one-line note if not OK>
S1: <OK|WARN|FAIL> -- <one-line note if not OK>
S2: <OK|WARN|FAIL> -- <one-line note if not OK>
S3: <OK|WARN|FAIL> -- <one-line note if not OK>
S4: <OK|WARN|FAIL> -- <one-line note if not OK>
S6: <OK|WARN|FAIL> -- <one-line note if not OK>

Issues filed: <count> (links)
```

File one GitHub issue per WARN or FAIL finding via `lyx selfreport create`. Include the
fingerprint header in every issue body.

## Notes

- Scenario set is deliberately small and host/weft-centric -- that is the spine that
  matters now. Add scenarios as modules grow (mux, shuttle, review, loom).
- The psmux interactive launcher will replace the direct `claude` launch in a future
  iteration; the file contract (this `SANDBOX-SUITE.md` driving the agent) is unchanged.
- The host repo `Knatte18/lyx-test` README uses the phrase "cwd-relpath mirroring"; this
  refers to **weft path mirroring** (how the weft worktree mirrors host subpaths) — not
  to running lyx from subdirectories. "cwd-relpath" does not appear elsewhere in this
  scheme.
