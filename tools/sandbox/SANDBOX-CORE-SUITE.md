# SANDBOX-CORE-SUITE -- lyx black-box agent suite

## What this is

A structured test-loop for exercising `lyx` against the real GitHub test repos
(`Knatte18/lyx-test` as host, `Knatte18/lyx-test-weft` as weft). Not an automated
suite -- the value is a Claude session driving `lyx` by hand in a real hub, treating
every break, surprise, or rough edge as a LoomYard finding to record in the report.

This parallels how millhouse was bootstrapped: get lyx working well enough that an
agent can operate it in a real repo, then use that experience to harden lyx.

## Pre-conditions

Before starting a session:

1. **Deploy a fresh binary.** Run `deploy.cmd` so `lyx.exe` on PATH is current source.
   The deployed binary is a snapshot -- re-deploy after any source change you want to test.
2. **Materialize the hub.** Run `sandbox-build.cmd` (or `sandbox-build.cmd -reset`
   to start clean) to clone the host and weft into a fresh `lyx-test-HUB`.
3. **`lyx` on PATH.** Confirm `lyx --help` works from any directory.

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
The agent must **not** scaffold nested `_lyx/` during a session, with exactly one
controlled exception: **S6** deliberately runs `lyx init` in a subdirectory to prove
the subfolder-scoping contract, and reverses that scaffolding with `lyx init --undo`
at session end (see S6's durability note). Outside of S6, creating a nested `_lyx/`
is out of scope for a session and not something to try "just to see what happens."

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

The same fingerprint identifies the binary for the report's provenance: a separate
fetch step (run after this session) stamps it into `meta.fingerprint` of the
fetched `sandbox-report.json` so a maintainer can reproduce the exact binary that
produced each finding. The agent does not need to transcribe the fingerprint
anywhere itself.

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

After all scenarios are run, write **all** `WARN`/`FAIL` findings to `./sandbox-report.json`
(in the host-repo cwd) on this exact schema. **Always write the file, even when there are
zero `WARN`/`FAIL` findings** -- in that case `items` is an empty array.

```json
{
  "source": "sandbox-report",
  "items": [
    {
      "ref": "S5",
      "title": "…",
      "body": "verdict: WARN\n\n…repro…"
    }
  ]
}
```

- `source` is the literal string `"sandbox-report"`.
- `items[]` holds only `WARN`/`FAIL` findings -- do not record `OK` scenarios here.
- `ref` is the scenario id (`S0`-`S8`).
- `title` is a short one-line summary.
- `body` folds the detail, repro steps, and verdict into one markdown string.

Write only `source` and `items` -- a separate fetch step (run after the session)
stamps `meta` (including the binary fingerprint). Confine all free text to the
`title`/`body` string fields so the JSON stays well-formed.

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
coordination (junctions wired correctly, weft mirroring behaves). The absence of a
lyx-owned host-commit command is an intentional design choice, not a gap — do not file
it as an enhancement suggestion.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### S3 -- Board and task interaction

**Goal:** "Add a task to the board, list tasks, change its state."

**Covers:** board

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

**Covers:** config

**Watch:** From the worktree root, write a value with `lyx config <module> --set
key=value` (non-interactive, bypasses the editor; mutually exclusive with `--print`;
requires a module argument), read it back with `lyx config <module> --print`, then run
`lyx config reconcile`. Does the write/read round-trip the correct `_lyx/config/` file,
and does `reconcile` report a clean (no unexpected added/removed keys) result against
the value you just wrote?

**Verdict:** `OK` / `WARN` / `FAIL`

---

### S5 -- Wrong-directory and error ergonomics

**Goal:** "Run a hub-only command from outside the hub. Run a command with a bad flag.
Run an unknown subcommand."

**Watch:** Are errors legible? Does lyx say what to do, or just fail? This is where
standalone usability lives or dies. A legible `not initialized` / "run from the
initialized root"-style message is the `OK` (ergonomics-pass) outcome — not a `FAIL`.
Do not file it as a finding. `lyx`'s error output is a JSON envelope
(`{"ok":false,"error":"..."}`) on every error path by design — that is the deliberate
machine-parseable contract, not a defect. "Legible" means the `error` field's message
text clearly identifies the problem, not that the output reads as human prose with a
hint or usage suggestion. This does not cover a raw subprocess/tool string leaking
unwrapped into the `error` field (e.g. a bare git `fatal:` line, or any other tool's
raw stderr) — that is still a legitimate `WARN`/`FAIL` finding.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### S6 -- Subfolder init

**Goal:** "From a non-root subdirectory of the already-initialized host repo, run
`lyx init` there. Then run `config` and `board` from that same subdir. Finally, reverse
it with `lyx init --undo`."

**Covers:** init

**Durability note:** S6 scaffolds a real nested `_lyx/` in the subfolder — a directory
junction into the weft worktree, not a plain directory — and touches `.gitignore`
there. That state persists across sandbox sessions unless the hub is rebuilt with
`sandbox-build.cmd -reset` (optional, not mandatory, per Pre-conditions), so S6 must run
`lyx init --undo` from the subdir at session end to restore the "not yet initialized"
state. `init --undo` is not purely local: clearing the weft-side `_lyx` content commits
and pushes that deletion to the shared `lyx-test-weft` remote, so each S6 run leaves an
init-then-undo commit pair in the weft repo's history. It is a clean no-op on a
never-initialized directory, so it is always safe to run at session end even if S6
bailed early.

**Watch:** Does `lyx init` scaffold a subdir-scoped `_lyx/` (not at the repo root)? Does
`lyx config --print`/`--set` run from the subdir resolve against the subdir's own
`_lyx/config` rather than the root's — the actual subfolder-scoping demonstrator? Does
`lyx board` still run cleanly from the subdir — a "still works from any subfolder" smoke
check only; board's data lives at the hub level, so this does *not* itself prove
subfolder-scoped resolution the way `config` does. Does `lyx init --undo` cleanly reverse
the scaffolding?

**Verdict:** `OK` / `WARN` / `FAIL`

---

### S7 -- Weft lifecycle

**Goal:** "Make a small, clearly-marked change inside the weft-tracked scope and run it
through `weft status`, `commit`, `push`, `pull`, and `sync`."

**Covers:** weft

**Durability note:** S7 runs against the real shared sandbox remotes. Make a small,
clearly-marked test change and do not leave the weft/host remotes diverged or broken for
the next session.

**Watch:** Does `weft status` report the change accurately? Do `commit`/`push` mirror it
to the weft remote? The commit message is always the fixed string `"weft sync"` — it is
not generated from changed files and there is no `-m` flag to customize it. Staging is
scoped to the directories listed in the weft config (default `_lyx`), so the test change
should land inside that scope to be picked up at all. `weft sync` pushes via a detached
child process, so `status` immediately after `sync` may lag behind the actual push — a
confusing-but-expected rough edge to note as a `WARN`, not to pre-judge here.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### S8 -- Warp introspection

**Goal:** "Exercise `warp list`, `warp pairs`, `warp reconcile`, and `warp checkout` on
a healthy pair."

**Covers:** warp

**Durability note:** Record the branch active before the scenario starts. Run
`warp checkout <other-branch>` to prove the coordinated switch works, then
`warp checkout <original-branch>` to restore it, leaving hub state clean for the rest of
the session.

**Watch:** Do `warp list`/`warp pairs` report sane host↔weft geometry? Is
`warp reconcile` a safe no-op/idempotent read+report on an already-healthy pair — note
it has no `--apply`/dry-run flag, unlike `config reconcile`; it always performs its
repair check directly, so a destructive result on a healthy pair would itself be a
finding worth recording. Does `warp checkout` perform a coordinated host+weft switch
cleanly? A *bad* `warp checkout` (e.g. an unknown branch) now yields a clean wrapped
error (`host switch to branch %q failed (git exit %d)`), not raw git stderr — a legible
error there is the expected `OK` outcome, not a finding.

**Verdict:** `OK` / `WARN` / `FAIL`

---

mux has its own dedicated suite, `SANDBOX-MUX-SUITE.md` in this same directory,
launched via `sandbox-mux-suite.cmd` -- mux needs a live psmux server and visual
verification, a different test mode from this suite.

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
S5: <OK|WARN|FAIL> -- <one-line note if not OK>
S6: <OK|WARN|FAIL> -- <one-line note if not OK>
S7: <OK|WARN|FAIL> -- <one-line note if not OK>
S8: <OK|WARN|FAIL> -- <one-line note if not OK>

sandbox-report.json written: <count of WARN/FAIL items>
```

`./sandbox-report.json` must be written before the session ends, per the Capturing
findings section above -- with `items: []` when every scenario was `OK`.

## Notes

- Scenario set is deliberately small and host/weft-centric -- that is the spine that
  matters now. Add scenarios as modules grow (shuttle, review, loom). A module whose
  testing model is fundamentally different gets its own sibling suite file
  (`*SUITE.md`), with mux (`SANDBOX-MUX-SUITE.md`) as the precedent; the
  coverage guard scans all of them.
- The psmux interactive launcher will replace the direct `claude` launch in a future
  iteration; the file contract (this `SANDBOX-CORE-SUITE.md` driving the agent) is unchanged.
- The host repo `Knatte18/lyx-test` README uses the phrase "cwd-relpath mirroring"; this
  refers to **weft path mirroring** (how the weft worktree mirrors host subpaths) — not
  to running lyx from subdirectories. "cwd-relpath" does not appear elsewhere in this
  scheme.
