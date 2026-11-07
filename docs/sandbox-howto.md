# How-to: run the sandbox suite agent

Operator runbook for exercising the deployed `lym.exe` against the sandbox Hub.
This is the **ordered procedure**;
for the topology, repo layout, and design rationale see [sandbox-hub.md](sandbox-hub.md).

All commands run from the lym repo root (`C:\Code\loomyard\wts\loomyard` on Windows, the repo root on POSIX) unless stated otherwise.
The sandbox launchers under `sandbox/win` hardcode this machine's Hub parent `C:\Code`.
The deploy launchers hardcode nothing: `update-plugins.cmd` installs production `lym` into the machine's own `go env GOBIN` (else `GOPATH\bin`),
and `deploy-dev.cmd` installs a dev build into a derived, per-worktree `.dev-bin` directory.
Each sandbox launcher does one thing: build, run one suite, or fetch.

> **POSIX equivalents.** Every command below has a `sandbox/posix/*.sh` twin with the same subcommands and flags, and `$HOME/Code` stands in for `C:\Code`. `update-plugins.sh` and `deploy-dev` are the POSIX twins of `update-plugins.cmd` and `deploy-dev.cmd`.

**Run every suite launcher in a real, attached interactive terminal** — never backgrounded, detached, or with stdout/stderr redirected.
The agent session is an interactive `claude` process;
without a TTY it cannot idle between turns waiting for notifications, so it may end early and silently abandon the remaining scenarios.
The launcher prints a warning when it detects non-console stdio.

## What the suite does

`sandbox/win/core-suite.cmd` (`sandbox/posix/core-suite.sh` on POSIX) resolves the `lym` binary to test:
the derived `.dev-bin/lym.exe` when it exists, else the binary on PATH as a prod fallback.
It fingerprints the binary and drops a fresh `SANDBOX-CORE-SUITE.md` into the Hub code repo, stamped with the fingerprint and a `Source: dev` / `Source: prod` marker.
It then launches an interactive black-box agent there.
When the resolved binary is the dev build, the suite prepends `.dev-bin` to the agent's own child-process PATH, so its bare `lym` invocations resolve to it.
The agent still drives `lym` from PATH only (never the source tree), scoped to its own session, not your shell.
Panes the agent itself spawns inherit the same resolution from reed rather than from the launcher.
Reed prepends the spawning binary's directory to each strand pane's `PATH` and exports `LYM_BIN`.
It writes those statements to the strand pane's launch script, which the pane shell sources,
so a nested `lym …` inside a spawned pane resolves to the binary under test too.
The two mechanisms are complementary: the launcher composes a Go child process's environment, and reed composes a pane shell statement.
The agent writes WARN/FAIL findings to `sandbox-report.json` in the code repo.
The suite only launches the agent;
collecting the report is a separate step.
After the session ends, run `sandbox/win/fetch.cmd` (`sandbox/posix/fetch.sh`).
It fetches a normalized copy into this repo's `.scratch/sandbox-report-<fingerprint>.json`.

The agent tests the resolved binary, dev-first with a prod fallback.
A stale `.dev-bin` binary therefore means you are testing old code.
Always deploy before a run (step 2); `deploy-dev.cmd` is the fast path because it never touches the production binary.

## Prerequisites (one-time)

1. **Sandbox wiki initialized** — the board repo is the records repo's GitHub wiki.
   The records repo must have Wikis enabled and at least one page, or the clone fails and the Hub is torn down.
   See [sandbox-hub.md#prerequisites](sandbox-hub.md#prerequisites).
2. **The Go bin dir is on PATH (production only)** — `update-plugins.cmd` installs the production `lym` into `go env GOBIN`, else `GOPATH\bin`.
   To install elsewhere, set it once per machine: `go env -w GOBIN=C:\Code\tools\bin`.
   The dev binary in `.dev-bin` does not need to be on PATH;
   the suite resolves it directly and threads it to the agent itself.

## Each run

### 1. Confirm the repo builds and tests green

Never deploy a red tree.

```cmd
go build ./...
go test ./...
```

### 2. Deploy a fresh dev `lym.exe`

Rebuilds `lym` from the current checkout and installs it into the derived `.dev-bin` directory at the repo root, overwriting the old dev binary.
This never touches the production `lym` in the Go bin dir — `deploy-dev.cmd` and `update-plugins.cmd` are independent targets.

```cmd
deploy-dev.cmd
```

Verify the deployed binary is the new one, for example by confirming an expected surface change is present.
The dev binary is not on PATH, so call it by its `.dev-bin` path directly:

```cmd
.dev-bin\lym.exe config --help
```

`lym config reconcile` exists and `lym update` does not.
If you see `update` in the output, the deploy did not take.
Once the suite session starts, the fingerprint header's `Source: dev` line confirms that the agent exercises this build.

### 3. Build the Hub (first time, or when you want a clean slate)

**First time** — clone the Hub to `C:\Code\lym-test-LYMHUB`:

```cmd
sandbox/win/build.cmd
```

```sh
sandbox/posix/build.sh
```

**Reset** — tear down and re-clone a clean Hub (destroys all local Hub state):

```cmd
sandbox/win/build.cmd -reset
```

```sh
sandbox/posix/build.sh -reset
```

Skip this step on repeat runs if the existing Hub is fine.
`sandbox/win/core-suite.cmd`/`sandbox/posix/core-suite.sh` does not require a reset each time.
Reset when the Hub topology may be stale (e.g. after a fabric change) or when a previous run left it dirty.

### 4. Run the suite

```cmd
sandbox/win/core-suite.cmd
```

```sh
sandbox/posix/core-suite.sh
```

This copies a fresh `SANDBOX-CORE-SUITE.md` (fingerprint + embedded scheme) into the Hub code repo.
It then launches the interactive agent there.
Let it run;
it records findings to `sandbox-report.json` itself.
Exit the agent session when it is done; the suite treats any exit code as normal and does not fetch the report.

Optional overrides:

```cmd
sandbox/win/core-suite.cmd -claude <path>   # override the claude binary (default: from PATH)
sandbox/win/core-suite.cmd -prompt <text>   # override the instruction string
```

```sh
sandbox/posix/core-suite.sh -claude <path>   # override the claude binary (default: from PATH)
sandbox/posix/core-suite.sh -prompt <text>   # override the instruction string
```

### 4b. Run the reed suite (optional, needs live tmux)

```cmd
sandbox/win/reed-suite.cmd
```

```sh
sandbox/posix/reed-suite.sh
```

This runs as step 4 does, but for `lym reed`'s scenarios.
It copies a fingerprinted `SANDBOX-REED-SUITE.md` into the Hub code repo and launches the interactive agent there.
It needs a live tmux (`tmux.exe` on PATH) and PowerShell 7.
The attach scenario (M7) pauses for the operator to run `lym reed attach` in a second terminal and confirm visually.
Findings go to the same `sandbox-report.json`, so steps 5 (fetch) and 6 (triage) apply unchanged.
Fetch between sessions; do not run both suites and fetch once.

The `-claude`/`-prompt` overrides are the same as for `sandbox/win/core-suite.cmd`/`sandbox/posix/core-suite.sh`:

```cmd
sandbox/win/reed-suite.cmd -claude <path>   # override the claude binary (default: from PATH)
sandbox/win/reed-suite.cmd -prompt <text>   # override the instruction string
```

```sh
sandbox/posix/reed-suite.sh -claude <path>   # override the claude binary (default: from PATH)
sandbox/posix/reed-suite.sh -prompt <text>   # override the instruction string
```

### 4c. Run the shuttle, creel or burler suite (optional, needs live tmux + logged-in claude)

```cmd
sandbox/win/shuttle-suite.cmd
sandbox/win/creel-suite.cmd
sandbox/win/burler-suite.cmd
```

These run as 4b does, for the scenarios of `lym shuttle`, `lym creel` and `lym burler` respectively.
Each needs a live tmux, PowerShell 7, a logged-in `claude`, and an `lym init`-ed code repo.
The `-claude`/`-prompt` overrides are the same.
After the session ends, the suite runs `lym reed down` in the code repo for every reed-backed suite except the reed watch suite (4d),
so no tmux server outlives the run.
An orphaned server holds handles inside the Hub and blocks the next `sandbox/win/build.cmd -reset` (`sandbox/posix/build.sh -reset`).

### 4d. Run the reed watch suite (optional, needs live tmux + logged-in claude)

```cmd
sandbox/win/reed-watch-suite.cmd
```

```sh
sandbox/posix/reed-watch-suite.sh
```

The non-destructive counterpart to 4b: no scenario here kills a server, kills a pane, renames a worktree, or tears the session down.
The operator attaches once at the start (W0) and stays attached through every later scenario, in one continuous window.
The operator watches strands appear and a parent collapse as a child appears.
The operator also watches a cheap Claude Code agent (Haiku) work in its own pane, and a live terminal resize reflow everything.
Use this instead of 4b to *watch* reed work rather than follow a crash/kill/rename scenario log.
Use 4b when you need the destructive coverage.
The `-claude`/`-prompt` overrides are the same as for the other suites.

**This suite does not run `lym reed down` after the session ends**, so the session stays live and attached for exploring by hand.
Tear it down yourself with `lym reed down` in the Hub code repo when you are done.

### 5. Fetch the report

```cmd
sandbox/win/fetch.cmd
```

```sh
sandbox/posix/fetch.sh
```

This reads `sandbox-report.json` from the Hub code repo and validates and stamps it.
It writes a normalized copy into this repo's `.scratch/sandbox-report-<fingerprint>.json`.
Run this after the suite session ends;
if the agent wrote no report, this fails with a distinct "not found" error.

### 6. Triage findings

The agent does not file GitHub issues itself.
The flow has three steps:
the suite emits `sandbox-report.json` in the Hub code repo;
`sandbox/win/fetch.cmd`/`sandbox/posix/fetch.sh` fetches it into this repo's `.scratch/sandbox-report-<fingerprint>.json`;
you run the report-to-tasks triage skill against that file:

```
/mill-report-to-tasks "<path-to-fetched-json>"
```

The path (the `.scratch/sandbox-report-<fingerprint>.json` that fetch printed) is a required positional argument.
The skill groups the findings into wiki tasks;
nothing is written until you approve.
Then groom/spawn as usual.

## Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| `lym` not found / old behaviour | dev binary in `.dev-bin` is stale, or (prod fallback) the Go bin dir (`go env GOBIN`, else `GOPATH\bin`) not on PATH | rerun `deploy-dev.cmd`; check the fingerprint header's `Source:` line — `dev` confirms the `.dev-bin` build ran, `prod` means the dev binary was missing and the suite fell back to PATH |
| the clone fails during build | sandbox wiki not initialized | enable Wikis + add a page on the records repo, then `sandbox/win/build.cmd -reset` (`sandbox/posix/build.sh -reset`) |
| Hub looks corrupt / half-cloned | interrupted earlier run | `sandbox/win/build.cmd -reset` (`sandbox/posix/build.sh -reset`) |
| `build -reset` fails: "being used by another process" (Windows) / "text file busy" (POSIX) | orphaned tmux from an earlier suite session still holds Hub handles | the launcher runs `lym reed down` after reed-backed suites **except the reed watch suite, which never auto-tears-down (see 4d)**; if hit anyway, find the Hub-scoped tmux PIDs by start time (Windows: `Get-Process -Name tmux \| Select Id,StartTime`; POSIX: `ps -o pid,lstart,cmd -C tmux`) and kill only those — never blanket-kill by image name |
| agent session ends early, scenarios abandoned, no report | launcher was backgrounded/redirected (no TTY) | rerun in a real attached terminal; heed the launcher's non-console stdio warning |
| exit code always 0/1, not claude's | launcher collapses claude's code | build and run `go build -o sandbox.exe ./tools/sandbox` for precise codes |

## See also

- [sandbox-hub.md](sandbox-hub.md) — Hub topology, repo layout, design rationale.
- [tools/sandbox/SANDBOX-CORE-SUITE.md](../tools/sandbox/SANDBOX-CORE-SUITE.md) — the embedded test scheme the agent follows.
- [tools/sandbox/SANDBOX-REED-SUITE.md](../tools/sandbox/SANDBOX-REED-SUITE.md) — the embedded reed-specific test scheme `sandbox/win/reed-suite.cmd`/`sandbox/posix/reed-suite.sh` follows.
- [tools/sandbox/SANDBOX-REED-WATCH-SUITE.md](../tools/sandbox/SANDBOX-REED-WATCH-SUITE.md) — the embedded, non-destructive reed scheme `sandbox/win/reed-watch-suite.cmd`/`sandbox/posix/reed-watch-suite.sh` follows.
- [tools/sandbox/SANDBOX-CREEL-SUITE.md](../tools/sandbox/SANDBOX-CREEL-SUITE.md) — the embedded creel test scheme `sandbox/win/creel-suite.cmd`/`sandbox/posix/creel-suite.sh` follows.
