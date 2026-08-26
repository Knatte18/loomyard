# SANDBOX-CORE-SUITE -- lyx black-box agent suite

## What this is

A structured test-loop for exercising `lyx` against the real GitHub test repos (`Knatte18/lyx-test` as warp, `Knatte18/lyx-test-weft` as weft).
Not an automated suite -- the value is a Claude session driving `lyx` by hand in a real hub, treating every break, surprise, or rough edge as a LoomYard finding to record in the report.

This parallels how millhouse was bootstrapped: get lyx working well enough that an agent can operate it in a real repo, then use that experience to harden lyx.

## Pre-conditions

Before starting a session:

1. **Deploy a fresh dev binary.**
   Run `deploy-dev` to build `lyx.exe` into `.dev-bin` as current source.
   The suite resolves `.dev-bin` itself and prepends it to the agent's PATH (the fingerprint header's `Source: dev` line confirms the dev build is under test) -- no PATH setup needed, and production `lyx` stays untouched.
   The deployed binary is a snapshot -- re-deploy after any source change you want to test.
2. **Materialize the hub.**
   Run `sandbox/build.cmd` (or `sandbox/build.cmd -reset` to start clean) to clone the warp and weft into a fresh `lyx-test-HUB`.
3. **`lyx` on PATH.**
   Confirm `lyx --help` works from any directory.

### PowerShell JSON-quoting

When driving the suite from Windows PowerShell (the assumed session shell on Windows), backslash-escaping a JSON argument is the intuitive-but-wrong move and yields:

```
{"error":"invalid json: invalid character '\\' looking for beginning of object key string","ok":false}
```

The working form is a single-quoted string with literal inner double quotes, e.g.

```powershell
lyx board upsert '{"slug":"s3-demo","title":"S3 demo"}'
```

### Operating model

lyx resolves against the current directory's own `_lyx/` and does **not** walk up to a parent.
The Hub's Fabric repo is initialized at its root, so the agent runs the entire session from there (cwd is fixed at the root).
Running a lyx command from a subdirectory that has not itself been initialized correctly reports `not initialized here; run "lyx fabric reconcile"` — that is expected behaviour, **not a finding**.
The agent must **not** scaffold nested `_lyx/` during a session, with exactly one controlled exception: **S6** deliberately clones a second, subpath-anchored hub to prove the subpath-anchoring contract, and reverses that scaffolding with `lyx fabric unwire` at session end (see S6's durability note).
Outside of S6, creating a nested `_lyx/` is out of scope for a session and not something to try "just to see what happens."

## Black-box rule

**The agent under test works exclusively inside the Hub's Fabric repo (`lyx-test-HUB/lyx-test`).
It tests `lyx.exe` as a black box -- exactly as a real user with only the binary on PATH.
It must not look for, read, or reason about the lyx source tree.
No peeking at `C:\Code\loomyard\` or any other path outside the Hub.**

Discovering the command surface is done via `lyx`, `lyx <module>`, and `lyx <module> <subcommand> --help` alone -- not from documentation outside the Hub.

## Fingerprint header

The launcher prepends a "binary under test" fingerprint block to this file when it copies it into the Hub's Fabric repo.
The fingerprint records the absolute path, file size, modification time, and a short SHA-256 of the `lyx.exe` binary at launch time.

The same fingerprint identifies the binary for the report's provenance: a separate fetch step (run after this session) stamps it into `meta.fingerprint` of the fetched `sandbox-report.json` so a maintainer can reproduce the exact binary that produced each finding.
The agent does not need to transcribe the fingerprint anywhere itself.

## How to run a scenario

For each scenario below:

- Read the **Goal** -- it names the task, not the commands.
  Discover the commands via `lyx`, `lyx <module>`, and `--help` flags (S0 ethos).
- **Watch** what lyx does.
  Note where it stalls, guesses wrong, or hits an error.
- Record the outcome per the verdict buckets: `OK` (worked) / `WARN` (rough edge) / `FAIL` (broke).

## Verdict key

- `OK`   -- completed without friction
- `WARN` -- completed but with confusion, awkward UX, or a non-fatal error
- `FAIL` -- did not complete;
  lyx broke, panicked, or gave wrong output

## Capturing findings

After all scenarios are run, write **all** `WARN`/`FAIL` findings to `./sandbox-report.json` (in the Fabric-repo cwd) on this exact schema.
**Always write the file, even when there are zero `WARN`/`FAIL` findings** -- in that case `items` is an empty array.

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
- `ref` is the scenario id (`S0`-`S6`).
- `title` is a short one-line summary.
- `body` folds the detail, repro steps, and verdict into one markdown string.

Write only `source` and `items` -- a separate fetch step (run after the session) stamps `meta` (including the binary fingerprint).
Confine all free text to the `title`/`body` string fields so the JSON stays well-formed.

## Scenarios

### S0 -- Discovery (help surface smoke test)

**Goal:** "You have `lyx` on PATH and nothing else inside this repo.
Find out what it can do and report the full command tree."

**Watch:** Does `lyx` alone list modules?
Does `lyx <module>` list subcommands?
Is each description accurate and useful?
Any command that cannot be discovered from the binary alone is a help gap.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### S1 -- Hub orientation

**Goal:** "You are inside a hub that was set up from a warp and a weft.
Figure out what the hub contains and what state it is in."

**Watch:** Can you tell warp from weft from board using only `lyx` commands?
Does any `lyx` command report hub geometry or status?
If you have to `ls` and guess, that is a missing command surface.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### S2 -- First real work in the warp

**Goal:** "Create something in the warp repo (a file, a small change) and get it committed and tracked the way lyx intends."

**Watch:** The warp is an ordinary git repo — committing warp changes with plain `git` is acceptable and **not** a finding.
Watch lyx's actual responsibility: warp/weft coordination (junctions wired correctly, weft mirroring behaves).
The absence of a lyx-owned warp-commit command is an intentional design choice, not a gap — do not file it as an enhancement suggestion.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### S3 -- Board and task interaction

**Goal:** "Add a task to the board, list tasks, change its state."

**Covers:** board

**Storage note:** `_board` is a second weft worktree checked out on the warp's own unsuffixed default branch (`weft:main` in the common case), never a separate clone with its own `board-url` — the scenario below is pure CRUD and needs no awareness of that beyond this note.

**Note:** When passing JSON in PowerShell, use single-quoted strings with literal inner double quotes — see the PowerShell JSON-quoting note in Pre-conditions.

**Durability note:** The board is durable across sessions — it starts non-empty (e.g. a `T1 "Test task from S3"` task persists from prior runs).
Do not assume a fresh board.
Use `lyx board list` to observe current state before adding tasks, and use `lyx board remove` to clean up any test tasks you create at session end.

**Watch:** Board CRUD via `lyx board`.
JSON output sane.
State transitions work.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### S4 -- Config round-trip

**Goal:** "Inspect lyx's config for this hub, change a value, confirm it took."

**Covers:** config

**Watch:** From the worktree root, write a value with `lyx config <module> --set key=value` (non-interactive, bypasses the editor;
mutually exclusive with `--print`;
requires a module argument), read it back with `lyx config <module> --print`, then run `lyx config reconcile`.
Does the write/read round-trip the correct `_lyx/config/` file, and does `reconcile` report a clean (no unexpected added/removed keys) result against the value you just wrote?

**Verdict:** `OK` / `WARN` / `FAIL`

---

### S5 -- Wrong-directory and error ergonomics

**Goal:** "Run a hub-only command from outside the hub.
Run a command with a bad flag.
Run an unknown subcommand."

**Watch:** Are errors legible?
Does lyx say what to do, or just fail?
This is where standalone usability lives or dies.
A legible `not initialized` / "run from the initialized root"-style message is the `OK` (ergonomics-pass) outcome — not a `FAIL`.
Do not file it as a finding. `lyx`'s error output is a JSON envelope (`{"ok":false,"error":"..."}`) on every error path by design — that is the deliberate machine-parseable contract, not a defect.
"Legible" means the `error` field's message text clearly identifies the problem, not that the output reads as human prose with a hint or usage suggestion.
This does not cover a raw subprocess/tool string leaking unwrapped into the `error` field (e.g. a bare git `fatal:` line,
or any other tool's raw stderr) — that is still a legitimate `WARN`/`FAIL` finding.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### S6 -- Subpath-anchored clone

**Goal:** "Clone a second hub from the same warp/weft repos into a scratch location, anchored at a subdirectory instead of the repo root, using `lyx fabric clone --subpath <sub> <weft-url> <warp-url>`.
From inside the anchored subpath, run `config` and `board`.
Finally, tear the pair down with `lyx fabric unwire`."

**Durability note:** Unlike the old subfolder-init scenario, S6 cannot retrofit the already-root-anchored main sandbox hub — the lyx-anchor subpath is recorded once, at clone time, onto `weft:main`.
S6 instead clones a genuinely separate scratch hub from the same warp/weft URLs, so it is not bound by the Black-box rule's "operate only inside the main Hub" scope for its own duration.
Use `--reset` on the clone so repeated sandbox sessions are idempotent. `lyx fabric unwire`, run from inside the anchored subpath at session end, removes the junctions, clears the weft-side `_lyx` content, and reverts `.gitignore` there — it is not purely local: clearing weft-side `_lyx` content commits and pushes that deletion to the shared `lyx-test-weft` remote. `unwire` deliberately leaves the scratch hub's recorded anchor and repo-wide `fabric.yaml` on `weft:main` untouched (that is what lets a later `lyx fabric reconcile` re-wire it), so delete the scratch hub directory itself with a plain filesystem removal once `unwire` completes, rather than expecting `unwire` to remove it.

**Watch:** Does `lyx fabric clone --subpath <sub>` scaffold junctions and config at `<sub>/_lyx` inside the new hub's warp worktree, not at the warp worktree root — with no follow-up activation command needed?
Does `lyx config --print`/`--set` run from inside `<sub>` resolve against `<sub>/_lyx/config` — the actual subpath-anchoring demonstrator?
Does `lyx board` still run cleanly from inside `<sub>` — a "still works from the anchored subpath" smoke check only;
board's data lives at the hub level, so this does *not* itself prove subpath resolution the way `config` does.
Does running a lyx command from outside the anchored subpath (e.g. the new warp worktree's root, when `<sub>` is not `.`) correctly hard-error rather than silently resolving the wrong subpath?
Does `lyx fabric unwire` cleanly remove the junctions, clear the weft `_lyx` content, and revert `.gitignore`, while leaving the recorded anchor and repo-wide config intact?

**Verdict:** `OK` / `WARN` / `FAIL`

---

### S7 -- Stencil registry inspection

**Goal:** "List the board's stencil prompts and confirm they validate cleanly."

**Covers:** stencil

**Durability note:** Like `_board` itself, the board's stencils tree is seeded on first run and persists across sessions -- it is not reseeded or rewritten by an ordinary `lyx` invocation once every registered stencil is present and untouched. Do not assume a fresh seed on this run.

**Watch:** Does `lyx stencil list` name all eighteen registered stencils, each with a board-copy path and an edit state (`absent`/`untouched`/`edited`)? Does `lyx stencil validate` report a clean tree (no `error`-severity findings) against an unmodified board copy? Is the JSON output for both sane -- a well-formed envelope, no raw tool output leaking through?
This scenario is deliberately read-only: `promote` and `sync` both mutate the operator's tree and are exercised by `internal/stencilcli`'s own integration tests, not by hand here.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### S8 -- Loom status and pause over a seeded fixture

**Goal:** "Inspect and pause a loom task's phase machine without actually bootstrapping one."

**Covers:** loom

**Fixture note:** This scenario hand-writes `.lyx/loom/status.json` as a fixture rather than reaching a seeded state through any shipped verb, because no shipped verb seeds one without going through `lyx loom run`'s tmux bootstrap handover, and `lyx loom pause` on an absent status file is specified to error.
Write the fixture with a realistic `current_producer`/`state`/`activity`/`history` shell and a `product` carrying a `slug` and `parent` of your choosing, following the shape in `contracts/specs/loom-status-spec.md`'s worked example.

**Watch:** Does `lyx loom status` round-trip the fixture's own `slug`/`parent`/`current_producer`/`state`/`activity`/`history` values back out through its JSON envelope unchanged -- this also pins that envelope against the status contract?
Does `lyx loom pause` set `pause_requested` true while leaving every other field -- `current_producer`, `state`, `activity`, `history`, and `product` -- untouched?

**Verdict:** `OK` / `WARN` / `FAIL`

---

reed has its own dedicated suite, `SANDBOX-REED-SUITE.md` in this same directory, launched via `sandbox/reed-suite.cmd` -- reed needs a live tmux server and visual verification, a different test mode from this suite.

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

`./sandbox-report.json` must be written before the session ends, per the Capturing findings section above -- with `items: []` when every scenario was `OK`.

## Notes

- Scenario set is deliberately small and warp/weft-centric -- that is the spine that matters now.
  Add scenarios as modules grow (shuttle, review, loom).
  A module whose testing model is fundamentally different gets its own sibling suite file (`*SUITE.md`), with reed (`SANDBOX-REED-SUITE.md`) as the precedent;
  the coverage guard scans all of them.
- The tmux interactive launcher will replace the direct `claude` launch in a future iteration;
  the file contract (this `SANDBOX-CORE-SUITE.md` driving the agent) is unchanged.
- The warp repo `Knatte18/lyx-test` README uses the phrase "cwd-relpath mirroring";
  this refers to **weft path mirroring** (how the weft worktree mirrors warp subpaths) — not to running lyx from subdirectories. "cwd-relpath" does not appear elsewhere in this scheme.
