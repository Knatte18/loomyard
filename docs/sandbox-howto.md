# How-to: run the sandbox suite agent

Operator runbook for exercising the deployed `lyx.exe` against the sandbox Hub.
This is the **ordered procedure**; for the topology, repo layout, and design
rationale see [sandbox-hub.md](sandbox-hub.md).

All commands run from the lyx repo root (`C:\Code\loomyard\wts\loomyard`) unless
stated otherwise. The launchers (`deploy.cmd`, `sandbox-build.cmd`,
`sandbox-core-suite.cmd`, `sandbox-mux-suite.cmd`, `sandbox-shuttle-suite.cmd`,
`sandbox-burler-suite.cmd`, `sandbox-perch-suite.cmd`, `sandbox-builder-suite.cmd`,
`sandbox-fetch.cmd`) hardcode the machine-specific paths for this machine: deploy
target `C:\Code\tools\bin`, Hub parent `C:\Code`. Each sandbox launcher does
exactly one thing (build / one suite / fetch).

**Run every suite launcher in a real, attached interactive terminal** — never
backgrounded, detached, or with stdout/stderr redirected. The agent session is
an interactive `claude` process; without a TTY it cannot idle between turns
waiting for notifications, so it may end early and silently abandon the
remaining scenarios. The launcher prints a warning when it detects
non-console stdio.

## What the suite does

`sandbox-core-suite.cmd` fingerprints the `lyx.exe` on PATH, drops a fresh
`SANDBOX-CORE-SUITE.md` into the Hub host repo, and launches an interactive black-box
agent that drives `lyx` from PATH only (never the source tree). The agent writes
WARN/FAIL findings to `sandbox-report.json` in the host repo. The suite only
launches the agent; collecting the report is a separate step — after the session
ends you run `sandbox-fetch.cmd` to fetch a normalized copy into this repo's
`.scratch/sandbox-report-<fingerprint>.json`.

Because the agent tests **the binary on PATH**, a stale binary means you are
testing old code. Always deploy before a run (step 2).

## Prerequisites (one-time)

1. **Sandbox wiki initialized** — the board repo is the weft repo's GitHub wiki.
   `lyx-test-weft` must have Wikis enabled and at least one page, or
   `warp clone` fails and the Hub is torn down. See
   [sandbox-hub.md#prerequisites](sandbox-hub.md#prerequisites).
2. **`C:\Code\tools\bin` is on PATH** — that is where `deploy.cmd` installs `lyx`.

## Each run

### 1. Confirm the repo builds and tests green

Never deploy a red tree.

```cmd
go build ./...
go test ./...
```

### 2. Deploy a fresh `lyx.exe`

Rebuilds `lyx` from the current checkout and installs it to `C:\Code\tools\bin`
(on PATH), overwriting the old binary.

```cmd
deploy.cmd
```

Verify the deployed binary is the new one — e.g. confirm an expected surface
change is present:

```cmd
lyx config --help
```

(After the cobra-cli-engine sweep, `lyx update` is gone and `lyx config reconcile`
exists. If you still see `update` in `lyx --help`, the deploy did not take.)

### 3. Build the Hub (first time, or when you want a clean slate)

**First time** — clone the Hub to `C:\Code\lyx-test-HUB`:

```cmd
sandbox-build.cmd
```

**Reset** — tear down and re-clone a clean Hub (destroys all local Hub state):

```cmd
sandbox-build.cmd -reset
```

Skip this step on repeat runs if the existing Hub is fine — `sandbox-core-suite.cmd`
does not require a reset each time. Reset when the Hub topology may be stale
(e.g. after a warp/weft change) or when a previous run left it dirty.

### 4. Run the suite

```cmd
sandbox-core-suite.cmd
```

This copies a fresh `SANDBOX-CORE-SUITE.md` (fingerprint + embedded scheme) into the
Hub host repo and launches the interactive agent there. Let it run; it records
findings to `sandbox-report.json` itself. Exit the agent session when it is done —
the suite treats any exit code as normal and does not fetch the report itself.

Optional overrides:

```cmd
sandbox-core-suite.cmd -claude <path>   # override the claude binary (default: from PATH)
sandbox-core-suite.cmd -prompt <text>   # override the instruction string
```

### 4b. Run the mux suite (optional, needs live tmux)

```cmd
sandbox-mux-suite.cmd
```

This copies a fingerprinted `SANDBOX-MUX-SUITE.md` into the Hub host repo and
launches the interactive agent there, same as step 4 but for `lyx mux`'s
scenarios. It needs a live tmux (`tmux.exe` on PATH) and PowerShell 7. The
attach scenario (M7) pauses for the operator to run `lyx mux attach` in a
second terminal and confirm visually. Findings go to the same
`sandbox-report.json`, so step 5 (`sandbox-fetch.cmd`) and step 6 (triage)
apply unchanged — fetch between sessions, do not run both suites and fetch
once.

Same `-claude`/`-prompt` overrides as `sandbox-core-suite.cmd`:

```cmd
sandbox-mux-suite.cmd -claude <path>   # override the claude binary (default: from PATH)
sandbox-mux-suite.cmd -prompt <text>   # override the instruction string
```

### 4c. Run the shuttle or burler suite (optional, needs live tmux + logged-in claude)

```cmd
sandbox-shuttle-suite.cmd
sandbox-burler-suite.cmd
```

Same operating model as 4b, for `lyx shuttle`'s and `lyx burler`'s scenarios
respectively; both need a live tmux, PowerShell 7, a logged-in `claude`, and
an `lyx init`-ed host repo. Same `-claude`/`-prompt` overrides. After the
session ends, the launcher runs `lyx mux down` in the host repo (for the mux,
shuttle, burler, and perch suites) so no tmux server outlives the run — an
orphaned server holds handles inside the Hub and blocks the next
`sandbox-build.cmd -reset`.

### 4d. Run the perch suite (optional, needs live tmux + logged-in claude)

```cmd
sandbox-perch-suite.cmd
```

Same operating model as 4c, for `lyx perch`'s gate-loop scenarios (convergence,
pause/resume, the command gate) — perch wires the real burler substrate (which
in turn wires shuttle) on every invocation, so the same live-tmux,
PowerShell 7, logged-in-`claude`, and `lyx init`-ed prerequisites apply. Same
`-claude`/`-prompt` overrides.

### 4e. Run the builder suite (optional, needs live tmux + logged-in claude)

```cmd
sandbox-builder-suite.cmd
```

Same operating model as 4c/4d, for `lyx builder`'s batch-loop scenarios (the
autonomous `run` happy path, `poll`'s dead/timeout classification, pause as a
batch-boundary check, `run.lock` contention, and fingerprint/outcome
archiving) — builder branches off shuttle directly (real tmux + real
`claude`), so the same live-tmux, PowerShell 7, logged-in-`claude`, and
`lyx init`-ed prerequisites apply. Same `-claude`/`-prompt` overrides.

### 5. Fetch the report

```cmd
sandbox-fetch.cmd
```

Reads `sandbox-report.json` from the Hub host repo, validates and stamps it, and
writes a normalized copy into this repo's
`.scratch/sandbox-report-<fingerprint>.json`. Run this after the suite session
ends; if the agent wrote no report, this fails with a distinct "not found" error.

### 6. Triage findings

The agent no longer files GitHub issues itself. Instead: the suite emits
`sandbox-report.json` in the Hub host repo → `sandbox-fetch.cmd` fetches it
into this repo's `.scratch/sandbox-report-<fingerprint>.json` → run the
report-to-tasks triage skill against that file:

```
/mill-report-to-tasks "<path-to-fetched-json>"
```

The path (the `.scratch/sandbox-report-<fingerprint>.json` that fetch printed) is
a required positional argument. The skill groups the findings into wiki tasks;
nothing is written until you approve. Then groom/spawn as usual.

## Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| `lyx` not found / old behaviour | binary on PATH is stale or `C:\Code\tools\bin` not on PATH | rerun `deploy.cmd`; check PATH |
| `warp clone` fails during build | sandbox wiki not initialized | enable Wikis + add a page on `lyx-test-weft`, then `sandbox-build.cmd -reset` |
| Hub looks corrupt / half-cloned | interrupted earlier run | `sandbox-build.cmd -reset` |
| `sandbox-build.cmd -reset` fails: "being used by another process" | orphaned `tmux.exe` from an earlier suite session still holds Hub handles | the launcher now runs `lyx mux down` after mux-backed suites; if hit anyway, find the Hub-scoped `tmux.exe` PIDs by `StartTime` (`Get-Process -Name tmux \| Select Id,StartTime`) and kill only those — never blanket-kill by image name |
| agent session ends early, scenarios abandoned, no report | launcher was backgrounded/redirected (no TTY) | rerun in a real attached terminal; heed the launcher's non-console stdio warning |
| exit code always 0/1, not claude's | launcher collapses claude's code | build and run `go build -o sandbox.exe ./tools/sandbox` for precise codes |

## See also

- [sandbox-hub.md](sandbox-hub.md) — Hub topology, repo layout, design rationale.
- [tools/sandbox/SANDBOX-CORE-SUITE.md](../tools/sandbox/SANDBOX-CORE-SUITE.md) — the embedded test scheme the agent follows.
- [tools/sandbox/SANDBOX-MUX-SUITE.md](../tools/sandbox/SANDBOX-MUX-SUITE.md) — the embedded mux-specific test scheme `sandbox-mux-suite.cmd` follows.
- [tools/sandbox/SANDBOX-BUILDER-SUITE.md](../tools/sandbox/SANDBOX-BUILDER-SUITE.md) — the embedded builder-specific test scheme `sandbox-builder-suite.cmd` follows.
