# Dogfood Hub: lyx-test

## Overview

The **dogfood Hub** is a dedicated bench for manual testing of lyx's core workflows. It exercises the actual deployed `lyx` binary, testing the real command surface, JSON output, and topology wiring that users encounter.

The Hub consists of two dedicated GitHub repositories and a local working directory on disk:

- **Host repo:** `https://github.com/Knatte18/lyx-test` — the source repository
- **Weft repo:** `https://github.com/Knatte18/lyx-test-weft` — the companion overlay repository
- **Board repo:** `https://github.com/Knatte18/lyx-test-weft.wiki.git` — the task board (the weft repo's GitHub wiki)

## Hub Location and Structure

The Hub is cloned to `C:\Code\lyx-test-HUB` on this machine (the host basename `lyx-test` + `-HUB` suffix, derived via `internal/warp/clone.go`'s `deriveHostName()`).

**Important:** The Hub lives **outside `C:\Code\loomyard\`** so it is never mistaken for part of Loomyard itself. This separation keeps dogfood separate from the orchestrator codebase.

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
sandbox.cmd
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
sandbox.cmd -reset
```

The `-reset` flag:
1. Removes the existing Hub directory at `C:\Code\lyx-test-HUB`.
2. Clones a fresh Hub as above.

**Caution:** `-reset` destroys the entire Hub directory, including any local changes or uncommitted work. Back up any work before using `-reset`.

## Dogfood Purpose

The dogfood Hub serves as a **testbed for lyx's core agent-driven workflows**. Point lyx's agent-driven orchestrator at the `lyx-test` host repo and exercise the full pipeline:

- Init, board, weft, warp, and config operations.
- Phased runs (Setup → Discussion → Plan → Builder → Finalize).
- Review gates and agent dispatch.

**If the orchestrator breaks on this known-good Hub, it is a LoomYard bug to be fixed.** The dogfood purpose is to catch regressions early and keep the real lyx surface tested.

## Dedicated Use

The two repositories (`lyx-test` and `lyx-test-weft`) are **dedicated to this dogfood use only**. They are not synced with any other project or use case. Do not use them for other purposes.

## See Also

- [internal/warp/clone.go](../internal/warp/clone.go) — the hub cloning orchestration and URL derivation logic.
- [overview.md](overview.md#weft-overlay-model) — the weft overlay model and Hub topology.
