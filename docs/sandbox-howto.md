# How-to: run the sandbox suite agent

Operator runbook for exercising the deployed `lyx.exe` against the sandbox Hub.
This is the **ordered procedure**;
for the topology, repo layout, and design rationale see [sandbox-hub.md](sandbox-hub.md).

All commands run from the lyx repo root (`C:\Code\loomyard\wts\loomyard` on Windows, the repo root on POSIX) unless stated otherwise.
The launchers (`deploy.cmd`, `deploy-dev.cmd`, `sandbox/win/build.cmd`, `sandbox/win/core-suite.cmd`, `sandbox/win/reed-suite.cmd`, `sandbox/win/shuttle-suite.cmd`, `sandbox/win/burler-suite.cmd`, `sandbox/win/fetch.cmd`) hardcode this machine's paths: `deploy.cmd`'s deploy target `C:\Code\tools\bin`, Hub parent `C:\Code`. `deploy-dev.cmd` is the exception — it installs into a derived, per-worktree `.dev-bin` directory, never a hardcoded path.
Each sandbox launcher does exactly one thing (build / one suite / fetch).

> **POSIX equivalents.** Every command below has a `sandbox/posix/*.sh` twin (`build.sh`, `core-suite.sh`, `reed-suite.sh`, `shuttle-suite.sh`, `burler-suite.sh`, `fetch.sh`), same subcommands and flags, `$HOME/Code` standing in for `C:\Code`. `deploy.cmd`/`deploy-dev.cmd` have no POSIX port yet — that is still Windows-only.

**Run every suite launcher in a real, attached interactive terminal** — never backgrounded, detached, or with stdout/stderr redirected.
The agent session is an interactive `claude` process;
without a TTY it cannot idle between turns waiting for notifications, so it may end early and silently abandon the remaining scenarios.
The launcher prints a warning when it detects non-console stdio.

## What the suite does

`sandbox/win/core-suite.cmd` (`sandbox/posix/core-suite.sh` on POSIX) resolves the `lyx` binary to test — the derived `.dev-bin/lyx.exe` when it exists, else the binary on PATH as a prod fallback — fingerprints it, drops a fresh `SANDBOX-CORE-SUITE.md` (stamped with the fingerprint and a `Source: dev` / `Source: prod` marker) into the Hub warp repo, and launches an interactive black-box agent there.
When the resolved binary is the dev build, the suite prepends `.dev-bin` to the agent's own child-process PATH, so its bare `lyx` invocations resolve to it — the agent still drives `lyx` from PATH only (never the source tree), just scoped to its own session, not your shell.
The agent writes WARN/FAIL findings to `sandbox-report.json` in the warp repo.
The suite only launches the agent;
collecting the report is a separate step — after the session ends, run `sandbox/win/fetch.cmd` (`sandbox/posix/fetch.sh`) to fetch a normalized copy into this repo's `.scratch/sandbox-report-<fingerprint>.json`.

Because the agent tests the resolved binary (dev-first, prod fallback), a stale `.dev-bin` binary means you are testing old code.
Always deploy before a run (step 2) — `deploy-dev.cmd` is the fast path since it never touches the production binary.

## Prerequisites (one-time)

1. **Sandbox wiki initialized** — the board repo is the weft repo's GitHub wiki. `lyx-test-weft` must have Wikis enabled and at least one page, or `warp clone` fails and the Hub is torn down.
   See [sandbox-hub.md#prerequisites](sandbox-hub.md#prerequisites).
2. **`C:\Code\tools\bin` is on PATH (production only)** — that is where `deploy.cmd` installs the production `lyx`.
   The dev binary in `.dev-bin` does NOT need to be on PATH;
   the suite resolves it directly and threads it to the agent itself.

## Each run

### 1. Confirm the repo builds and tests green

Never deploy a red tree.

```cmd
go build ./...
go test ./...
```

### 2. Deploy a fresh dev `lyx.exe`

Rebuilds `lyx` from the current checkout and installs it into the derived `.dev-bin` directory at the repo root, overwriting the old dev binary.
This never touches the production `lyx` in `C:\Code\tools\bin` — `deploy-dev.cmd` and `deploy.cmd` are independent targets.

```cmd
deploy-dev.cmd
```

Verify the deployed binary is the new one — e.g. confirm an expected surface change is present.
The dev binary is deliberately not on PATH, so call it by its `.dev-bin` path directly:

```cmd
.dev-bin\lyx.exe config --help
```

(After the cobra-cli-engine sweep, `lyx update` is gone and `lyx config reconcile` exists.
If you still see `update` in the output, the deploy did not take.)
Once the suite session starts, the fingerprint header's `Source: dev` line is the ongoing confirmation that the agent is exercising this build — no further manual check needed.

### 3. Build the Hub (first time, or when you want a clean slate)

**First time** — clone the Hub to `C:\Code\lyx-test-HUB`:

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

Skip this step on repeat runs if the existing Hub is fine — `sandbox/win/core-suite.cmd`/`sandbox/posix/core-suite.sh` does not require a reset each time.
Reset when the Hub topology may be stale (e.g. after a warp/weft change) or when a previous run left it dirty.

### 4. Run the suite

```cmd
sandbox/win/core-suite.cmd
```

```sh
sandbox/posix/core-suite.sh
```

This copies a fresh `SANDBOX-CORE-SUITE.md` (fingerprint + embedded scheme) into the Hub warp repo and launches the interactive agent there.
Let it run;
it records findings to `sandbox-report.json` itself.
Exit the agent session when it is done — the suite treats any exit code as normal and does not fetch the report itself.

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

This copies a fingerprinted `SANDBOX-REED-SUITE.md` into the Hub warp repo and launches the interactive agent there, same as step 4 but for `lyx reed`'s scenarios.
It needs a live tmux (`tmux.exe` on PATH) and PowerShell 7.
The attach scenario (M7) pauses for the operator to run `lyx reed attach` in a second terminal and confirm visually.
Findings go to the same `sandbox-report.json`, so steps 5 (fetch) and 6 (triage) apply unchanged — fetch between sessions, don't run both suites and fetch once.

Same `-claude`/`-prompt` overrides as `sandbox/win/core-suite.cmd`/`sandbox/posix/core-suite.sh`:

```cmd
sandbox/win/reed-suite.cmd -claude <path>   # override the claude binary (default: from PATH)
sandbox/win/reed-suite.cmd -prompt <text>   # override the instruction string
```

```sh
sandbox/posix/reed-suite.sh -claude <path>   # override the claude binary (default: from PATH)
sandbox/posix/reed-suite.sh -prompt <text>   # override the instruction string
```

### 4c. Run the shuttle or burler suite (optional, needs live tmux + logged-in claude)

```cmd
sandbox/win/shuttle-suite.cmd
sandbox/win/burler-suite.cmd
```

Same operating model as 4b, for `lyx shuttle`'s and `lyx burler`'s scenarios respectively;
both need a live tmux, PowerShell 7, a logged-in `claude`,
and an `lyx init`-ed warp repo.
Same `-claude`/`-prompt` overrides.
After the session ends, the launcher runs `lyx reed down` in the warp repo (for the reed, shuttle, and burler suites) so no tmux server outlives the run — an orphaned one holds handles inside the Hub and blocks the next `sandbox/win/build.cmd -reset` (`sandbox/posix/build.sh -reset`).

### 5. Fetch the report

```cmd
sandbox/win/fetch.cmd
```

```sh
sandbox/posix/fetch.sh
```

Reads `sandbox-report.json` from the Hub warp repo, validates and stamps it, and writes a normalized copy into this repo's `.scratch/sandbox-report-<fingerprint>.json`.
Run this after the suite session ends;
if the agent wrote no report, this fails with a distinct "not found" error.

### 6. Triage findings

The agent no longer files GitHub issues itself.
Instead: the suite emits `sandbox-report.json` in the Hub warp repo → `sandbox/win/fetch.cmd`/`sandbox/posix/fetch.sh` fetches it into this repo's `.scratch/sandbox-report-<fingerprint>.json` → run the report-to-tasks triage skill against that file:

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
| `lyx` not found / old behaviour | dev binary in `.dev-bin` is stale, or (prod fallback) `C:\Code\tools\bin` not on PATH | rerun `deploy-dev.cmd`; check the fingerprint header's `Source:` line — `dev` confirms the `.dev-bin` build ran, `prod` means the dev binary was missing and the suite fell back to PATH |
| `warp clone` fails during build | sandbox wiki not initialized | enable Wikis + add a page on `lyx-test-weft`, then `sandbox/win/build.cmd -reset` (`sandbox/posix/build.sh -reset`) |
| Hub looks corrupt / half-cloned | interrupted earlier run | `sandbox/win/build.cmd -reset` (`sandbox/posix/build.sh -reset`) |
| `build -reset` fails: "being used by another process" (Windows) / "text file busy" (POSIX) | orphaned tmux from an earlier suite session still holds Hub handles | the launcher now runs `lyx reed down` after reed-backed suites; if hit anyway, find the Hub-scoped tmux PIDs by start time (Windows: `Get-Process -Name tmux \| Select Id,StartTime`; POSIX: `ps -o pid,lstart,cmd -C tmux`) and kill only those — never blanket-kill by image name |
| agent session ends early, scenarios abandoned, no report | launcher was backgrounded/redirected (no TTY) | rerun in a real attached terminal; heed the launcher's non-console stdio warning |
| exit code always 0/1, not claude's | launcher collapses claude's code | build and run `go build -o sandbox.exe ./tools/sandbox` for precise codes |

## See also

- [sandbox-hub.md](sandbox-hub.md) — Hub topology, repo layout, design rationale.
- [tools/sandbox/SANDBOX-CORE-SUITE.md](../tools/sandbox/SANDBOX-CORE-SUITE.md) — the embedded test scheme the agent follows.
- [tools/sandbox/SANDBOX-REED-SUITE.md](../tools/sandbox/SANDBOX-REED-SUITE.md) — the embedded reed-specific test scheme `sandbox/win/reed-suite.cmd`/`sandbox/posix/reed-suite.sh` follows.
