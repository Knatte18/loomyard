# How-to: run the sandbox suite agent

Operator runbook for exercising the deployed `lyx.exe` against the sandbox Hub.
This is the **ordered procedure**; for the topology, repo layout, and design
rationale see [sandbox-hub.md](sandbox-hub.md).

All commands run from the lyx repo root (`C:\Code\loomyard\wts\loomyard`) unless
stated otherwise. The launchers (`deploy.cmd`, `sandbox-build.cmd`,
`sandbox-suite.cmd`, `mux-sandbox-suite.cmd`, `sandbox-fetch.cmd`) hardcode the
machine-specific paths for this machine: deploy target `C:\Code\tools\bin`, Hub
parent `C:\Code`. Each sandbox launcher does exactly one thing (build / suite /
mux-suite / fetch).

## What the suite does

`sandbox-suite.cmd` fingerprints the `lyx.exe` on PATH, drops a fresh
`SANDBOX-SUITE.md` into the Hub host repo, and launches an interactive black-box
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

Skip this step on repeat runs if the existing Hub is fine — `sandbox-suite.cmd`
does not require a reset each time. Reset when the Hub topology may be stale
(e.g. after a warp/weft change) or when a previous run left it dirty.

### 4. Run the suite

```cmd
sandbox-suite.cmd
```

This copies a fresh `SANDBOX-SUITE.md` (fingerprint + embedded scheme) into the
Hub host repo and launches the interactive agent there. Let it run; it records
findings to `sandbox-report.json` itself. Exit the agent session when it is done —
the suite treats any exit code as normal and does not fetch the report itself.

Optional overrides:

```cmd
sandbox-suite.cmd -claude <path>   # override the claude binary (default: from PATH)
sandbox-suite.cmd -prompt <text>   # override the instruction string
```

### 4b. Run the mux suite (optional, needs live psmux)

```cmd
mux-sandbox-suite.cmd
```

This copies a fingerprinted `MUX-SANDBOX-SUITE.md` into the Hub host repo and
launches the interactive agent there, same as step 4 but for `lyx mux`'s
scenarios. It needs a live psmux (`psmux.exe` on PATH) and PowerShell 7. The
attach scenario (M7) pauses for the operator to run `lyx mux attach` in a
second terminal and confirm visually. Findings go to the same
`sandbox-report.json`, so step 5 (`sandbox-fetch.cmd`) and step 6 (triage)
apply unchanged — fetch between sessions, do not run both suites and fetch
once.

Same `-claude`/`-prompt` overrides as `sandbox-suite.cmd`:

```cmd
mux-sandbox-suite.cmd -claude <path>   # override the claude binary (default: from PATH)
mux-sandbox-suite.cmd -prompt <text>   # override the instruction string
```

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
| exit code always 0/1, not claude's | launcher collapses claude's code | build and run `go build -o sandbox.exe ./tools/sandbox` for precise codes |

## See also

- [sandbox-hub.md](sandbox-hub.md) — Hub topology, repo layout, design rationale.
- [tools/sandbox/SANDBOX-SUITE.md](../tools/sandbox/SANDBOX-SUITE.md) — the embedded test scheme the agent follows.
- [tools/sandbox/MUX-SANDBOX-SUITE.md](../tools/sandbox/MUX-SANDBOX-SUITE.md) — the embedded mux-specific test scheme `mux-sandbox-suite.cmd` follows.
