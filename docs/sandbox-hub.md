# Sandbox Hub: lyx-test

## Overview

> **Just want to run it?** See the operator runbook: [sandbox-howto.md](sandbox-howto.md) (deploy → clone Hub → run suite). This document is the reference for topology and design.

> **Windows vs. POSIX.** Everything below is written for the Windows launchers (`sandbox/win/*.cmd`) and this machine's Windows paths (`C:\Code\...`). POSIX (Linux/macOS) equivalents ship alongside them under `sandbox/posix/*.sh`, invoked the same way (e.g. `sandbox/posix/build.sh`, `sandbox/posix/core-suite.sh -reset`) with `$HOME/Code` standing in for `C:\Code` as the Hub's parent directory — same subcommands, same flags, same `tools/sandbox` Go tool underneath.

The **sandbox Hub** is a dedicated bench for manual testing of lyx's core workflows.
It exercises the resolved `lyx` binary under test — the dev binary deployed via `deploy-dev` into the derived `.dev-bin` directory when present, else the production binary on PATH deployed via `deploy.cmd` — against the real command surface, JSON output, and topology wiring users encounter.
Its purpose is **dogfooding**: running lyx against itself to catch regressions early.

The Hub consists of two dedicated GitHub repositories and a local working directory on disk:

- **Warp repo:** `https://github.com/Knatte18/lyx-test` — the source repository
- **Weft repo:** `https://github.com/Knatte18/lyx-test-weft` — the companion overlay repository
- **Board repo:** `https://github.com/Knatte18/lyx-test-weft.wiki.git` — the task board (the weft repo's GitHub wiki)

## Hub Location and Structure

The Hub is cloned to `C:\Code\lyx-test-HUB` on Windows, or `$HOME/Code/lyx-test-HUB` on POSIX (the warp basename `lyx-test` + `-HUB` suffix, derived via `internal/fabricengine/clone.go`'s `DeriveWarpName()`).

**Important:** The Hub lives **outside `C:\Code\loomyard\`**, so it is never mistaken for part of Loomyard itself and stays separate from the orchestrator codebase.

The Hub directory structure mirrors the lyx topology model:

```
C:\Code\lyx-test-HUB/
  ├── lyx-test/           (warp repo worktree)
  ├── lyx-test-weft/      (weft repo worktree)
  └── _board/             (board repo with task store)
```

## Prerequisites

### GitHub Wiki Initialization

The board repo is the weft repo's GitHub wiki.
**This wiki must already exist and be initialized** before cloning:

1. The weft repo (`lyx-test-weft`) must have **Wikis enabled** in its GitHub settings.
2. The wiki must have **at least one page** created (a dedicated page can be the only content initially).

If the wiki does not exist or is not initialized, `lyx fabric clone` will fail when trying to clone the board,
and the Hub will be torn down.

### Current lyx Binary

The sandbox tool invokes `lyx fabric clone` as a subprocess and resolves which `lyx` to run: the derived `.dev-bin/lyx` binary when it exists (deployed via `deploy-dev`), else `lyx` on PATH as a fallback (deployed via `deploy.cmd`).
Deploy one of the two before the Hub can be built — if neither resolves, the sandbox tool fails with a clear error.

## Building and Rebuilding the Hub

### First Build

```cmd
sandbox/win/build.cmd
```

```sh
sandbox/posix/build.sh
```

This command:
1. Resolves the parent directory (`C:\Code`) from the launcher.
2. Computes the Hub path as `C:\Code\lyx-test-HUB`.
3. Checks if the Hub already exists;
   if not, proceeds to clone.
4. Runs `lyx fabric clone https://github.com/Knatte18/lyx-test-weft https://github.com/Knatte18/lyx-test` with the parent directory set to `C:\Code`.
5. Streams all output (stdout/stderr) to the terminal.
6. Exits with the clone command's exit code (0 on success, 1 on failure).

### Rebuild (Reset)

To remove and rebuild the Hub:

```cmd
sandbox/win/build.cmd -reset
```

```sh
sandbox/posix/build.sh -reset
```

The `-reset` flag:
1. Removes the existing Hub directory at `C:\Code\lyx-test-HUB`.
2. Clones a fresh Hub as above.

**Caution:** `-reset` destroys the entire Hub directory, including any local changes or uncommitted work — back up first.

## Running the Suite Agent

Once the Hub is built, the `suite` subcommand runs an automated black-box test session against the resolved `lyx.exe` under test.

### Prerequisites

- Hub already built (`sandbox/win/build.cmd` / `sandbox/posix/build.sh`).
- `lyx` resolvable: dev binary in `.dev-bin` (deployed via `deploy-dev`), or, as a fallback, on PATH (deployed via `deploy.cmd`).

### Usage

```cmd
sandbox/win/core-suite.cmd
```

```sh
sandbox/posix/core-suite.sh
```

This command, run from the lyx repo directory:

1. Locates the Hub warp repo at `C:\Code\lyx-test-HUB\lyx-test`.
2. Resolves the `lyx` binary under test (derived `.dev-bin/lyx` first, else PATH as a fallback) and fingerprints it (absolute path, size, modtime, SHA256 prefix,
   and a `Source: dev`/`Source: prod` marker recording which one was picked).
3. Copies a fresh `SANDBOX-CORE-SUITE.md` into the Hub warp repo, prepending the fingerprint block to the embedded template (`tools/sandbox/SANDBOX-CORE-SUITE.md`).
   Any previous copy is overwritten so every session starts from a clean slate.
4. Adds `SANDBOX-CORE-SUITE.md` to `lyx-test-HUB/lyx-test/.git/info/exclude` so the copied file does not show up as an untracked change inside the warp repo.
5. Launches an interactive `claude --dangerously-skip-permissions` session with the warp repo as the working directory and a single instruction: `"Read ./SANDBOX-CORE-SUITE.md and follow the instructions in it exactly."`

The agent works entirely as a black box: it sees only `lyx` on PATH and the copied scheme, and must not access the lyx source tree.
Findings (WARN or FAIL verdicts) are written to `sandbox-report.json` in the warp repo.
The suite subcommand only launches the agent — it does **not** fetch the report: an interactive `claude` session never self-terminates and its manual exit gives a non-zero code, so gating a fetch on a clean exit would never fire.
Collecting the report is a separate operator step (`fetch`, below).

### Optional flags

```cmd
sandbox/win/core-suite.cmd -claude <path>   # override the claude binary (default: resolve from PATH)
sandbox/win/core-suite.cmd -prompt <text>   # override the instruction string (default: built-in)
```

```sh
sandbox/posix/core-suite.sh -claude <path>   # override the claude binary (default: resolve from PATH)
sandbox/posix/core-suite.sh -prompt <text>   # override the instruction string (default: built-in)
```

### Exit-code note

The suite treats any exit code from the interactive `claude` session as normal — a manual exit is expected — so `runSuite` always returns success and prints a reminder to run `sandbox/win/fetch.cmd`/`sandbox/posix/fetch.sh`.

## Fetching the report

After the suite session ends, collect the agent-written report into this repo's `.scratch/`:

```cmd
sandbox/win/fetch.cmd
```

```sh
sandbox/posix/fetch.sh
```

This command:

1. Locates the Hub warp repo at `C:\Code\lyx-test-HUB\lyx-test`.
2. Re-fingerprints the `lyx.exe` currently on PATH (for the normal run-then-fetch flow this is the same binary the suite fingerprinted).
3. Reads `sandbox-report.json` from the warp repo, validates it against the shared sandbox-report-json contract (millhouse#586), stamps `meta.fingerprint`, and writes a normalized copy to `<loomyard>/.scratch/sandbox-report-<fingerprint>.json`.

On success it prints the fetched path and, when there are findings, the exact `/mill-report-to-tasks "<path>"` triage command to run next (nothing is written to the wiki until you approve);
a clean run says so and points at nothing.

If the agent produced no report, `fetch` fails with a distinct "not found" error so the operator can tell "the agent wrote nothing" from "the agent wrote garbage".
Only `fetch` passes `-loomyard` (`sandbox/win/fetch.cmd` as `"%~dp0..\..\."`, `sandbox/posix/fetch.sh` via its resolved `$REPO_ROOT`), and only this subcommand needs it.

### Future: tmux launch

The direct `claude` launch used today will be replaced by a tmux interactive session once the `reed` module is available.
The file contract (`SANDBOX-CORE-SUITE.md` driving the agent) is unchanged;
only the launch mechanism will differ.

## Running the reed suite

Alongside the main suite, `sandbox/win/reed-suite.cmd`/`sandbox/posix/reed-suite.sh` runs a dedicated black-box suite against `lyx reed`.
It mirrors the main-suite flow: copies a fingerprinted `SANDBOX-REED-SUITE.md` into the Hub warp repo, git-excludes it the same way, clears any stale `sandbox-report.json`, and launches the interactive agent there.
Because it exercises live tmux panes (crash simulation, layout verification, attach), it needs a live tmux (`tmux.exe` on PATH on Windows, `tmux` on PATH on POSIX) beyond what the main suite requires.
Findings land in the same `sandbox-report.json`, so `fetch` collects a reed-suite report exactly as it collects a main-suite report — the two suites share one report pipeline, one run at a time.

## Launchers and subcommands

The single Go tool (`tools/sandbox`) still dispatches four subcommands internally — `build` (default), `suite`, `reed-suite`, and `fetch` — but each is fronted by its own single-purpose launcher, mirroring how `deploy.cmd`/`deploy-dev.cmd` each do one thing:

```cmd
sandbox/win/build.cmd            # go run ./tools/sandbox -parent C:\Code build
sandbox/win/build.cmd -reset     # ... build -reset  (tear down and re-clone)
sandbox/win/core-suite.cmd            # ... suite  (run the interactive agent)
sandbox/win/reed-suite.cmd       # ... reed-suite  (run the reed-specific interactive agent)
sandbox/win/fetch.cmd            # ... -loomyard "%~dp0..\..\." fetch  (collect the report)
```

```sh
sandbox/posix/build.sh            # go run ./tools/sandbox -parent "$HOME/Code" build
sandbox/posix/build.sh -reset     # ... build -reset  (tear down and re-clone)
sandbox/posix/core-suite.sh            # ... suite  (run the interactive agent)
sandbox/posix/reed-suite.sh       # ... reed-suite  (run the reed-specific interactive agent)
sandbox/posix/fetch.sh            # ... -loomyard "$REPO_ROOT" fetch  (collect the report)
```

`-reset` is a flag of the `build` subcommand (parsed after the `build` token), so `build -reset` forwards its remaining args straight through to `... build -reset` regardless of which launcher (`sandbox/win/build.cmd`'s `%*` or `sandbox/posix/build.sh`'s `"$@"`) invoked it.

## Purpose: dogfooding lyx

The sandbox Hub serves as a **testbed for lyx's core agent-driven workflows**.
Point lyx's agent-driven orchestrator at the `lyx-test` warp repo and exercise the full pipeline:

- Init, board, weft, warp, and config operations.
- Phased runs (Setup → Discussion → Plan → Webster → Finalize).
- Review gates and agent dispatch.

**If the orchestrator breaks on this known-good Hub, it is a LoomYard bug to be fixed.**

## Dedicated Use

The two repositories (`lyx-test` and `lyx-test-weft`) are **dedicated to this sandbox use only** — not synced with any other project, and not to be used for anything else.

## See Also

- [internal/fabricengine/clone.go](../internal/fabricengine/clone.go) — the hub cloning orchestration and URL derivation logic.
- [overview.md](overview.md#weft-overlay-model) — the weft overlay model and Hub topology.
