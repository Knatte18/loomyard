# SANDBOX-CORE-SUITE -- lym black-box agent suite

## What this is

A structured test loop that exercises `lym` against the real GitHub test repos.
`Knatte18/lym-test` is the warp, and `Knatte18/lym-test-weft` is the weft.
It is not an automated suite.
A Claude session drives `lym` by hand in a real hub.
It records every break, surprise, or rough edge as a LoomYard finding in the report.

The findings harden `lym` until an agent can operate it in a real repo.

## Pre-conditions

Before starting a session:

1. **Deploy a fresh dev binary.**
   Run `deploy-dev` to build `lym.exe` from the current source into `.dev-bin`.
   The suite resolves `.dev-bin` itself and prepends it to the agent's PATH, so no PATH setup is needed.
   Production `lym` stays untouched.
   The fingerprint header's `Source: dev` line confirms the dev build is under test.
   The deployed binary is a snapshot: re-deploy after any source change you want to test.
2. **Materialize the hub.**
   Run `sandbox/build.cmd` to clone the warp and weft into a fresh `lym-test-LYMHUB`.
   Pass `-reset` to start clean.
3. **`lym` on PATH.**
   Confirm `lym --help` works from any directory.

### PowerShell JSON-quoting

In Windows PowerShell, the assumed session shell on Windows, backslash-escaping a JSON argument fails with:

```
{"error":"invalid json: invalid character '\\' looking for beginning of object key string","ok":false}
```

The working form is a single-quoted string with literal inner double quotes:

```powershell
lym board upsert '{"slug":"s3-demo","title":"S3 demo","labels":["enhancement"]}'
```

### Operating model

`lym` resolves against the current directory's own `_lym/` and does **not** walk up to a parent.
The Hub's Fabric repo is initialized at its root, so the agent runs the entire session from the root.
A `lym` command run from a subdirectory that is not itself initialized reports `not initialized here; run "lym fabric reconcile"`.
That is expected behaviour, **not a finding**.
The agent must **not** scaffold a nested `_lym/` during a session.
The one exception is **S6**, which clones a second, subpath-anchored hub to prove the subpath-anchoring contract,
and reverses the scaffolding with `lym fabric unwire --apply` at session end (see S6's durability note).
Outside S6, a nested `_lym/` is out of scope.

## Black-box rule

**The agent under test works exclusively inside the Hub's Fabric repo (`lym-test-LYMHUB/lym-test`).
It tests `lym.exe` as a black box -- exactly as a real user with only the binary on PATH.
It must not look for, read, or reason about the lym source tree.
No peeking at `C:\Code\loomyard\` or any other path outside the Hub.**

Discover the command surface through `lym`, `lym <module>`, and `lym <module> <subcommand> --help` alone.
Documentation outside the Hub is not a source.

## Fingerprint header

The launcher prepends a "binary under test" fingerprint block when it copies this file into the Hub's Fabric repo.
The fingerprint records the `lym.exe` binary's absolute path, size, modification time, and short SHA-256 at launch.

The same fingerprint records the report's provenance.
A separate fetch step, run after the session, stamps it into `meta.fingerprint` of the fetched `sandbox-report.json`,
so a maintainer can reproduce the exact binary that produced each finding.
The agent does not need to transcribe the fingerprint anywhere itself.

## How to run a scenario

For each scenario below:

- Read the **Goal** -- it names the task, not the commands.
  Discover the commands via `lym`, `lym <module>`, and `--help` flags (S0 ethos).
- **Watch** what lym does.
  Note where it stalls, guesses wrong, or hits an error.
- Record the outcome per the verdict buckets: `OK` (worked) / `WARN` (rough edge) / `FAIL` (broke).

## Verdict key

- `OK`   -- completed without friction
- `WARN` -- completed but with confusion, awkward UX, or a non-fatal error
- `FAIL` -- did not complete: lym broke, panicked, or gave wrong output

## Capturing findings

After all scenarios are run, write **all** `WARN`/`FAIL` findings to `./sandbox-report.json` in the Fabric-repo cwd.
Use this exact schema:
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
- `ref` is the scenario id, such as `S5`.
- `title` is a short one-line summary.
- `body` folds the detail, repro steps, and verdict into one markdown string.

Write only `source` and `items`.
A separate fetch step, run after the session, stamps `meta`, including the binary fingerprint.
Confine all free text to the `title`/`body` string fields so the JSON stays well-formed.

## Scenarios

### S0 -- Discovery (help surface smoke test)

**Goal:** "You have `lym` on PATH and nothing else inside this repo.
Find out what it can do and report the full command tree."

**Covers:** help

**Watch:** Does `lym` alone list modules?
Does `lym <module>` list subcommands?
Is each description accurate and useful?
Any command that cannot be discovered from the binary alone is a help gap.
Also run `lym help index` and check that it lists the operator commands one per line.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### S1 -- Hub orientation

**Goal:** "You are inside a hub that was set up from a warp and a weft.
Figure out what the hub contains and what state it is in."

**Watch:** Can you tell warp from weft from board using only `lym` commands?
Does any `lym` command report hub geometry or status?
If you have to `ls` and guess, that is a missing command surface.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### S2 -- First real work in the warp

**Goal:** "Create something in the warp repo, such as a file or a small change.
Get it committed and tracked the way lym intends."

**Watch:** The warp is an ordinary git repo.
Committing warp changes with plain `git` is acceptable and **not** a finding.
Watch lym's actual responsibility: warp/weft coordination (junctions wired correctly, weft mirroring behaves).
The absence of a lym-owned warp-commit command is an intentional design choice, not a gap.
Do not file it as an enhancement suggestion.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### S3 -- Board and task interaction

**Goal:** "Add a task to the board, list tasks, change its state."

**Covers:** board

**Storage note:** `_bolt` is a second weft worktree checked out on the warp's own unsuffixed default branch.
That branch is `weft:main` in the common case, and `_bolt` is never a separate clone with its own `board-url`.
The scenario below is pure CRUD and needs nothing beyond this note.

**Note:** When passing JSON in PowerShell, use single-quoted strings with literal inner double quotes.
See the PowerShell JSON-quoting note in Pre-conditions.

**Durability note:** The board is durable across sessions and starts non-empty.
For example, a `T1 "Test task from S3"` task persists from prior runs.
Do not assume a fresh board.
Use `lym board list` to observe current state before adding tasks.
Use `lym board remove` to clean up any test tasks you create at session end.

**Watch:** Board CRUD via `lym board`.
`lym board labels` prints `types` and `labels`, each in `board.yaml` file order.
JSON output sane.
State transitions work.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### S4 -- Config round-trip

**Goal:** "Inspect lym's config for this hub, change a value, confirm it took."

**Covers:** config

**Watch:** From the worktree root, write a value with `lym config <module> --set key=value`.
The flag is non-interactive and bypasses the editor.
It is mutually exclusive with `--print` and requires a module argument.
Read the value back with `lym config <module> --print`, then run `lym config reconcile`.
Does the write/read round-trip the correct `_lym/config/` file,
and does `reconcile` report a clean result (no unexpected added/removed keys) against the value you just wrote?

**Verdict:** `OK` / `WARN` / `FAIL`

---

### S5 -- Wrong-directory and error ergonomics

**Goal:** "Run a hub-only command from outside the hub.
Run a command with a bad flag.
Run an unknown subcommand."

**Watch:** Are errors legible?
Does lym say what to do, or just fail?
This is where standalone usability lives or dies.
A legible `not initialized` or "run from the initialized root" message is the `OK` outcome, not a `FAIL`.
Do not file it as a finding.
`lym`'s error output is a JSON envelope (`{"ok":false,"error":"..."}`) on every error path.
That is the machine-parseable contract, not a defect.
"Legible" means the `error` field's message names the problem.
It does not mean the output reads as human prose with a hint or usage suggestion.
A raw subprocess or tool string leaking unwrapped into the `error` field is still a legitimate `WARN`/`FAIL` finding.
Examples are a bare git `fatal:` line and any other tool's raw stderr.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### S6 -- Subpath-anchored clone

**Goal:** "Clone a second hub from the same warp/weft repos into a scratch location.
Anchor it at a subdirectory, not the repo root, with `lym fabric clone --subpath <sub> <weft-url> <warp-url>`.
From inside the anchored subpath, run `config` and `board`.
Finally, tear the pair down with `lym fabric unwire --apply`."

**Durability note:** The lym-anchor subpath is recorded once, at clone time, onto `weft:main`.
S6 therefore cannot retrofit the already-root-anchored main sandbox hub.
S6 instead clones a separate scratch hub from the same warp/weft URLs.
For its own duration it is not bound by the Black-box rule's "operate only inside the main Hub" scope.
Use `--reset` on the clone so repeated sandbox sessions are idempotent.
At session end, run `lym fabric unwire --apply` from inside the anchored subpath.
It removes the junctions, clears the weft-side `_lym` content, and reverts `.gitignore` there.
It is not purely local:
clearing weft-side `_lym` content commits and pushes that deletion to the shared `lym-test-weft` remote.
`unwire` leaves the scratch hub's recorded anchor and repo-wide `fabric.yaml` on `weft:main` untouched.
A later `lym fabric reconcile` can therefore re-wire it.
Once `unwire` completes, delete the scratch hub directory with a plain filesystem removal.
`unwire` does not remove it.

**Watch:** Does `lym fabric clone --subpath <sub>` scaffold junctions and config at `<sub>/_lym`?
Are they inside the new hub's warp worktree, not at the warp worktree root?
Is no follow-up activation command needed?
Does `lym config --print`/`--set` run from inside `<sub>` resolve against `<sub>/_lym/config`?
This is the actual subpath-anchoring demonstrator.
Does `lym board` still run cleanly from inside `<sub>`?
This is a smoke check only.
Board's data lives at the hub level, so it does *not* prove subpath resolution the way `config` does.
Does a lym command run from outside the anchored subpath hard-error instead of silently resolving the wrong subpath?
An example is the new warp worktree's root, when `<sub>` is not `.`.
Does `lym fabric unwire --apply` cleanly remove the junctions, clear the weft `_lym` content, and revert `.gitignore`?
Does it leave the recorded anchor and repo-wide config intact?

**Verdict:** `OK` / `WARN` / `FAIL`

---

### S7 -- Stencil registry inspection

**Goal:** "List the board's stencil prompts and confirm they validate cleanly."

**Covers:** stencil

**Durability note:** Like `_bolt`, the board's stencils tree is seeded on first run and persists across sessions.
An ordinary `lym` invocation does not reseed or rewrite it once every registered stencil is present and untouched.
Do not assume a fresh seed on this run.

**Watch:** Does `lym stencil list` name every registered stencil?
Does each have a board-copy path and an edit state (`absent`/`untouched`/`edited`)?
Does `lym stencil validate` report a clean tree (no `error`-severity findings) against an unmodified board copy?
Is the JSON output for both sane -- a well-formed envelope, no raw tool output leaking through?
This scenario is read-only.
`promote` and `sync` both mutate the operator's tree.
`internal/stencilcli`'s own integration tests exercise them, not this suite.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### S8 -- Loom status and pause over a seeded fixture

**Goal:** "Inspect and pause a tapestry task's phase machine without actually bootstrapping one."

**Covers:** loom

**Fixture note:** This scenario hand-writes `_lym/shed/<slug>/seed.json` and `_lym/shed/<slug>/status.json`.
`<slug>` is the sandbox worktree's own directory name.
A fixture is needed because no shipped verb seeds a run without `lym loom start`'s tmux bootstrap,
and `lym loom pause` on an absent seed refuses by naming the run as unseeded.
A status file with no seed beside it is the inconsistency that `internal/loomcli`'s seed-presence refusal catches.
Write the seed first: `{"recipe": "tapestry", "driver": "go"}`.
Write the status fixture with a realistic `current_producer`/`state`/`activity`/`history` shell.
Give it a `product` carrying a `slug` and `parent` of your choosing.
Follow the shape in the worked example of `contracts/specs/loom-status-spec.md`.

**Driver note:** This scenario never seeds or exercises `driver: llm`.
Bootstrapping an `llm`-driven run spawns a live, billed provider session that runs for as long as the task takes.
A sandbox scenario can neither bound that session's length nor assert against its output.
Every other scenario here relies on both properties.
`internal/loomcli`'s own Tier 1 and smoke tests cover them against a stubbed binary:
the branch itself;
the conjunction predicate that decides whether a spawn is needed at all;
the spec composition the driver hands to shuttle;
and the driver strand count staying stable across relaunches.
The tests are:
- `internal/loomcli/driverlaunch_test.go`
- `internal/loomcli/driverspec_test.go`
- `internal/loomcli/smoke_driverstrand_test.go`

This is a prose note with no mechanical enforcement, not a gate.
Loom stays covered under this scenario's own `**Covers:** loom` tag above.
The Sandbox Suite Coverage gate is module-keyed with no sub-module slot for one path within a covered module.
An exclusion covers the whole module.
Adding loom to the excluded-modules list in `cmd/lym/sandbox_coverage_test.go` would drop loom's existing coverage.
Do not use that list for this gap.

**Watch:** Before writing either fixture, run `lym loom status` and `lym loom pause` on the never-bootstrapped pair.
Does each refuse by naming the seed as absent?
The message is `loom: no seed found for run "self"; no run is seeded yet. run "lym loom start" first to bootstrap this task`.
It must not leak an internal lock path such as `.lym/shed/<slug>/status.json.lock: no such file or directory`.
This is the supervisor skill's literal first instruction, so it is the first thing to check, not an afterthought.

Then write both fixtures and continue.
Does `lym loom status` round-trip the fixture's `slug`/`parent`/`current_producer`/`state`/`activity`/`history` values?
They must come back unchanged through its JSON envelope, which also pins that envelope against the status contract.
Does the same envelope carry an `interrupt_policy` matching the fixture's `current_producer`?
The value is `handback` for `Webster` and `reinvoke` for every other row name.
It is absent for a name that is no row at all.
Does a bare `lym loom pause` refuse and leave the status file unchanged?
The refusal must name both ways forward:
`lym tender pause <slug>` stops the run's agents now;
`lym loom pause --after <current_producer>` halts it at the next producer boundary.
Does `lym loom pause --after <a row name>` record `pause_after` as that row?
Every other field must stay untouched: `current_producer`, `state`, `activity`, `history`, and `product`.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### S9 -- Glyph alphabet queries over the sandbox's own tree

**Goal:** "Walk quarry's read-only glyph queries against the sandbox repository itself.
Copy a glyph out of the output of `glyphs` rather than typing one by hand."

**Covers:** quarry

**Watch:** Does `lym quarry toc <a package directory>` render a well-formed JSON table of contents for it?
Does `lym quarry glyphs <that same directory>` render a flat, non-empty glyph index?
It must be a `symbols` list with an `id` on each entry.
Copy one entry's `id` value verbatim from that output, rather than typing a spelling by hand.
Does `lym quarry glyphs --text <that same directory>` print one line per symbol, each ending in the symbol's id?
Dropping lines containing `_test.go` must remove exactly the test-file symbols.
Does `lym quarry resolve <that copied glyph>` report it `found`?
Does `lym quarry resolve <the copied glyph> <a made-up glyph>` print two answers in argument order?
The second must have `status` `not_found`, and the command must exit non-zero without any `{"ok":false}` envelope.
Pick a type glyph from the same index, or a directory glyph if none of the package's exported symbols is a type.
Does `lym quarry expand <that glyph>` report its head plus its members?
Do all these commands run against the sandbox's own worktree with no repository-path flag?
Does each answer read as quarry's own rendering rather than something reshaped or filtered?

**Verdict:** `OK` / `WARN` / `FAIL`

---

### S10 -- Slotted go test through the gate

**Goal:** "Run one package's tests through `lym gate test`, then misuse it.
Confirm the exit codes and the refusal."

**Covers:** gate

**Watch:** Does `lym gate test <one package of the sandbox's own module>` stream go's own test output?
Does it exit 0 when that package's tests pass?
Does `lym gate test` with no package argument print one `{"ok":false}` envelope?
Does its `error` name the missing package and a way forward, and does the command exit 1?
Is the exit code of the first run go's own, and the second not go's?
Does `lym gate test <that package> -- -run NoSuchTest` pass the flag after `--` to go test and exit with go's code?

**Verdict:** `OK` / `WARN` / `FAIL`

---

### S11 -- Routine cleanup as a dry run

**Goal:** "Ask lym what its routine cleanup would remove from the hub's prime.
Confirm that asking removes nothing."

**Covers:** cleanup

**Watch:** Does `lym cleanup` from the sandbox's prime print one `{"ok":true}` envelope with `"applied": false`?
Does its `segments` list name every segment in order, each with what `--apply` would remove?
Is anything on disk or on the board different afterwards, such as a run directory, a board entry or a handoff file?
Does `lym cleanup` run from a task worktree instead refuse, name the prime worktree and say to re-run it there?

**Verdict:** `OK` / `WARN` / `FAIL`

---

### S12 -- Pause with nothing running, then resume

**Goal:** "Pause the sandbox hub while nothing runs, see the pause held, and lift it again."

**Covers:** tender

**Watch:** Does `lym p` print one `{"ok":true}` envelope under `pause`?
Does it name the hub scope and report empty lists?
Does a second `lym p` report `already_paused` and add no entry?
Does `lym tender status` list the one live entry and change nothing?
While the pause stands, does `lym board` print one line naming the pause and the `lym tender resume` that lifts it?
Does it continue only after the resume?
Does `lym r` lift the entry, and does a second `lym r` report nothing resumed?
Does `lym tender revive` with nothing stalled print an empty report and succeed?
Does `lym p no-such-slug` refuse and list the slugs that have a pair?
Does `lym p --grace 61` refuse and name the range 0 to 60?

**Verdict:** `OK` / `WARN` / `FAIL`

---

### S13 -- Convergence report from the prime

**Goal:** "Ask lym for the convergence report of the hub's finished runs and landings.
Confirm that asking changes nothing."

**Covers:** report

**Watch:** Does `lym report convergence` from the sandbox's prime print one `{"ok":true}` envelope?
Does it hold `runs`, `weeks` and `findings`?
Does `lym report convergence --format markdown` print its section headings, each opening a table or `no data`?
Does `lym report convergence --fresh-days 0` refuse, naming the flag?
When `lym report convergence` runs from a task worktree, does it refuse instead?
Does it name the prime worktree and say to re-run it there?
Does `lym report timing` from the prime print one `{"ok":true}` envelope?
Does it hold `runs`, `phases`, `buckets` and `sleep_note`?
Does `lym report timing --format markdown` print its section headings?
Does `lym report timing --by month` refuse, naming the flag?
Is anything on disk or in the hub's history different afterwards?

**Verdict:** `OK` / `WARN` / `FAIL`

---

reed has its own suite, `SANDBOX-REED-SUITE.md` in this same directory, launched via `sandbox/reed-suite.cmd`.
It needs a live tmux server and visual verification, a different test mode from this suite.

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
S9: <OK|WARN|FAIL> -- <one-line note if not OK>
S10: <OK|WARN|FAIL> -- <one-line note if not OK>
S11: <OK|WARN|FAIL> -- <one-line note if not OK>
S12: <OK|WARN|FAIL> -- <one-line note if not OK>
S13: <OK|WARN|FAIL> -- <one-line note if not OK>

sandbox-report.json written: <count of WARN/FAIL items>
```

`./sandbox-report.json` must be written before the session ends, as the Capturing findings section above requires.
Write it with `items: []` when every scenario was `OK`.

## Notes

- The scenario set is warp/weft-centric, the spine of the suite.
  Add scenarios as modules grow.
  A module whose testing model is fundamentally different gets its own sibling suite file (`*SUITE.md`).
  Reed's `SANDBOX-REED-SUITE.md` is an example.
  The coverage guard scans all of them.
- The tmux interactive launcher will replace the direct `claude` launch in a future iteration.
  The file contract, this `SANDBOX-CORE-SUITE.md` driving the agent, is unchanged.
- The warp repo `Knatte18/lym-test` README uses the phrase "cwd-relpath mirroring".
  It refers to **weft path mirroring**: how the weft worktree mirrors warp subpaths.
  It does not refer to running lym from subdirectories.
  "cwd-relpath" does not appear elsewhere in this scheme.
