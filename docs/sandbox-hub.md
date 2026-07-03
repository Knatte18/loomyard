# Sandbox Hub: lyx-test

## Overview

> **Just want to run it?** See the operator runbook: [sandbox-howto.md](sandbox-howto.md)
> (deploy → clone Hub → run suite). This document is the reference for topology and design.

The **sandbox Hub** is a dedicated bench for manual testing of lyx's core workflows. It exercises the actual deployed `lyx` binary, testing the real command surface, JSON output, and topology wiring that users encounter. Its purpose is **dogfooding** — running lyx against itself to catch regressions early.

The Hub consists of two dedicated GitHub repositories and a local working directory on disk:

- **Host repo:** `https://github.com/Knatte18/lyx-test` — the source repository
- **Weft repo:** `https://github.com/Knatte18/lyx-test-weft` — the companion overlay repository
- **Board repo:** `https://github.com/Knatte18/lyx-test-weft.wiki.git` — the task board (the weft repo's GitHub wiki)

## Hub Location and Structure

The Hub is cloned to `C:\Code\lyx-test-HUB` on this machine (the host basename `lyx-test` + `-HUB` suffix, derived via `internal/warpengine/clone.go`'s `DeriveHostName()`).

**Important:** The Hub lives **outside `C:\Code\loomyard\`** so it is never mistaken for part of Loomyard itself. This separation keeps the sandbox separate from the orchestrator codebase.

The Hub directory structure mirrors the lyx topology model:

```
C:\Code\lyx-test-HUB/
  ├── lyx-test/           (host repo worktree)
  ├── lyx-test-weft/      (weft repo worktree)
  └── _board/             (board repo with task store)
```

## Prerequisites

### GitHub Wiki Initialization

The board repo is the weft repo's GitHub wiki. **This wiki must already exist and be initialized** before cloning:

1. The weft repo (`lyx-test-weft`) must have **Wikis enabled** in its GitHub settings.
2. The wiki must have **at least one page** created (a dedicated page can be the only content initially).

If the wiki does not exist or is not initialized, `lyx warp clone` will fail when trying to clone the board, and the Hub will be torn down.

### Current lyx Binary on PATH

The sandbox tool invokes `lyx warp clone` as a subprocess and requires `lyx` to be on your system PATH. The `lyx` binary must be deployed separately (via `deploy.cmd`) before the Hub can be built.

If `lyx` is not on PATH, the sandbox tool will fail with a clear error message.

## Building and Rebuilding the Hub

### First Build

```cmd
sandbox-build.cmd
```

This command:
1. Resolves the parent directory (`C:\Code`) from the launcher.
2. Computes the Hub path as `C:\Code\lyx-test-HUB`.
3. Checks if the Hub already exists; if not, proceeds to clone.
4. Runs `lyx warp clone https://github.com/Knatte18/lyx-test https://github.com/Knatte18/lyx-test-weft` with the parent directory set to `C:\Code`.
5. Streams all output (stdout/stderr) to the terminal.
6. Exits with the clone command's exit code (0 on success, 1 on failure).

### Rebuild (Reset)

To remove and rebuild the Hub:

```cmd
sandbox-build.cmd -reset
```

The `-reset` flag:
1. Removes the existing Hub directory at `C:\Code\lyx-test-HUB`.
2. Clones a fresh Hub as above.

**Caution:** `-reset` destroys the entire Hub directory, including any local changes or uncommitted work. Back up any work before using `-reset`.

## Running the Suite Agent

Once the Hub is built, the `suite` subcommand runs an automated black-box test session
against the deployed `lyx.exe`.

### Prerequisites

- Hub already built (`sandbox-build.cmd`).
- `lyx` on PATH (deployed via `deploy.cmd`).

### Usage

```cmd
sandbox-suite.cmd
```

This command, run from the lyx repo directory:

1. Locates the Hub host repo at `C:\Code\lyx-test-HUB\lyx-test`.
2. Fingerprints the deployed `lyx.exe` (absolute path, size, modtime, SHA256 prefix).
3. Copies a fresh `SANDBOX-SUITE.md` into the Hub host repo, prepending the fingerprint
   block to the embedded template (`tools/sandbox/SANDBOX-SUITE.md`). Any previous copy
   is overwritten so every session starts from a clean slate.
4. Adds `SANDBOX-SUITE.md` to `lyx-test-HUB/lyx-test/.git/info/exclude` so the
   copied file does not show up as an untracked change inside the host repo.
5. Launches an interactive `claude --dangerously-skip-permissions` session with the
   host repo as the working directory and a single instruction:
   `"Read ./SANDBOX-SUITE.md and follow the instructions in it exactly."`

The agent works entirely as a black box: it sees only `lyx` on PATH and the copied
scheme. It must not access the lyx source tree. Findings (WARN or FAIL verdicts) are
written to `sandbox-report.json` in the host repo. The suite subcommand only launches
the agent — it does **not** fetch the report. An interactive `claude` session never
self-terminates and its manual exit gives a non-zero code, so gating a fetch on a
clean exit would never fire. Collecting the report is a separate operator step
(`fetch`, below).

### Optional flags

```cmd
sandbox-suite.cmd -claude <path>   # override the claude binary (default: resolve from PATH)
sandbox-suite.cmd -prompt <text>   # override the instruction string (default: built-in)
```

### Exit-code note

The suite treats any exit code from the interactive `claude` session as normal — a
manual exit is expected — so `runSuite` always returns success and prints a reminder
to run `sandbox-fetch.cmd`. The claude session's precise exit code is not
otherwise acted upon.

## Fetching the report

After the suite session ends, collect the agent-written report into this repo's
`.scratch/`:

```cmd
sandbox-fetch.cmd
```

This command:

1. Locates the Hub host repo at `C:\Code\lyx-test-HUB\lyx-test`.
2. Re-fingerprints the `lyx.exe` currently on PATH (for the normal run-then-fetch flow
   this is the same binary the suite fingerprinted).
3. Reads `sandbox-report.json` from the host repo, validates it against the shared
   sandbox-report-json contract (millhouse#586), stamps `meta.fingerprint`, and writes
   a normalized copy to `<loomyard>/.scratch/sandbox-report-<fingerprint>.json`.

On success it prints the fetched path and, when there are findings, the exact
`/mill-report-to-tasks "<path>"` triage command to run next (nothing is written to
the wiki until you approve); a clean run says so and points at nothing.

If the agent produced no report, `fetch` fails with a distinct "not found"
error so the operator can tell "the agent wrote nothing" from "the agent wrote garbage".
Only `sandbox-fetch.cmd` passes `-loomyard` (as `"%~dp0."`, the loomyard repo
root); it is required only by this subcommand.

### Future: psmux launch

The direct `claude` launch used today will be replaced by a psmux interactive session
once the `mux` module is available. The file contract (`SANDBOX-SUITE.md` driving the
agent) is unchanged; only the launch mechanism will differ.

## Running the mux suite

Alongside the main suite, `mux-sandbox-suite.cmd` runs a dedicated black-box suite
against `lyx mux`. It mirrors the main-suite flow: it copies a fingerprinted
`MUX-SANDBOX-SUITE.md` into the Hub host repo, git-excludes the copy the same way
`SANDBOX-SUITE.md` is excluded, clears any stale `sandbox-report.json`, and launches
the interactive agent there. Because it exercises live psmux panes (crash simulation,
layout verification, attach), it needs a live psmux (`psmux.exe` on PATH) as a
precondition beyond what the main suite requires. Findings land in the same
`sandbox-report.json` in the host repo, so `sandbox-fetch.cmd` collects a mux-suite
report exactly as it collects a main-suite report — the two suites share one report
pipeline, one run at a time.

## Launchers and subcommands

The single Go tool (`tools/sandbox`) still dispatches four subcommands
internally — `build` (default), `suite`, `mux-suite`, and `fetch` — but each is
fronted by its own single-purpose launcher, mirroring how `deploy.cmd` does one thing:

```cmd
sandbox-build.cmd            # go run ./tools/sandbox -parent C:\Code build
sandbox-build.cmd -reset     # ... build -reset  (tear down and re-clone)
sandbox-suite.cmd            # ... suite  (run the interactive agent)
mux-sandbox-suite.cmd        # ... mux-suite  (run the mux-specific interactive agent)
sandbox-fetch.cmd            # ... -loomyard "%~dp0." fetch  (collect the report)
```

`-reset` is a flag of the `build` subcommand (parsed after the `build` token), so
`sandbox-build.cmd -reset` forwards `%*` straight through to `... build -reset`.

## Purpose: dogfooding lyx

The sandbox Hub serves as a **testbed for lyx's core agent-driven workflows**. Point lyx's agent-driven orchestrator at the `lyx-test` host repo and exercise the full pipeline:

- Init, board, weft, warp, and config operations.
- Phased runs (Setup → Discussion → Plan → Builder → Finalize).
- Review gates and agent dispatch.

**If the orchestrator breaks on this known-good Hub, it is a LoomYard bug to be fixed.** The point of dogfooding is to catch regressions early and keep the real lyx surface tested.

## Dedicated Use

The two repositories (`lyx-test` and `lyx-test-weft`) are **dedicated to this sandbox use only**. They are not synced with any other project or use case. Do not use them for other purposes.

## See Also

- [internal/warpengine/clone.go](../internal/warpengine/clone.go) — the hub cloning orchestration and URL derivation logic.
- [overview.md](overview.md#weft-overlay-model) — the weft overlay model and Hub topology.
