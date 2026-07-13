# HANDOFF — Windows benchmark run (transient)

> **You are an agent running on a Windows machine.** This file is a one-shot task
> brief. It is a transient artifact on the `benchmark-suite-linux` branch — do not
> merge it to `main`; the operator will delete it once the numbers are recorded.
> Read it fully before acting.

## Goal

Measure Loomyard's test suite + benchmarks on **this** Windows machine and produce
a results file. The point is two comparisons the existing docs cannot make:

1. **Isolate the antivirus (AV) cost.** Run everything **twice on this same
   machine** — once with Microsoft Defender real-time protection active, once with
   this repo + the temp dir excluded. Same CPU, same OS, only AV differs → the
   delta is the pure AV tax.
2. **A clean, fast Windows datapoint.** This box is a **Ryzen 7 9800X3D**. The
   existing Windows benchmarks were on an Intel Core Ultra 7 155U **with Cortex
   XDR** (corporate endpoint AV) live; the Linux baseline is an AMD Ryzen AI 7 445.
   So do **not** compare your raw numbers against the old Windows column to judge
   AV — that mixes CPU + AV. The same-machine A/B run above is the clean AV signal.

Background (optional reading): `docs/benchmarks/*.md` hold the recorded numbers;
`docs/research/linux-portability-survey.md` explains the Linux portability work.
You do **not** need to read code to do this task.

## Do NOT

- Do **not** run `mill-setup` or set up the mill hub — the benchmarks make their
  own git fixtures in temp; a plain checkout is enough.
- Do **not** touch the wiki.
- Do **not** edit source or docs. This is measurement only.
- Do **not** commit anything **except** the single results file in Step 4/5 —
  that one you commit + push, because it is how the numbers reach the operator's
  machine (you two share the same git remote). Nothing else should ever be staged.

## Step 0 — Verify environment (report, don't fix silently)

Run and record each:

```powershell
git branch --show-current          # expect: benchmark-suite-linux
go version                          # record the exact version
git --version
Get-Command psmux -ErrorAction SilentlyContinue   # is the multiplexer on PATH?
[System.Environment]::OSVersion.Version           # Windows version
Get-CimInstance Win32_Processor | Select-Object Name, NumberOfLogicalProcessors
Get-Process -Name "cortex*","cyserver*" -ErrorAction SilentlyContinue   # is Cortex XDR present?
```

Notes:
- If **psmux is missing**, the mux tests (`internal/muxcli`, `internal/muxengine`
  multiplexer-contract) will FAIL. That is acceptable for timing — the wall-clock
  table still prints above the failure. Record that they failed and why.
- If **Cortex XDR is present** and you cannot exclude it, say so — the AV isolation
  will then only cover Defender, and Cortex will confound Run B. Report it clearly.

## Step 1 — Warm the build cache once

```powershell
go build ./...
```

## Step 2 — Run the measurement block (this is RUN A: Defender active)

Run each command; capture the noted output.

```powershell
# Tier 1 — offline. Run 3×; record each "Wall-clock:" line, report the median.
go run ./cmd/testtiming
go run ./cmd/testtiming
go run ./cmd/testtiming

# Tier 2 — integration (real git). Run 3×; record each "Wall-clock:" line + RESULT.
go run ./cmd/testtiming -full
go run ./cmd/testtiming -full
go run ./cmd/testtiming -full

# Fixture-copy — the most AV-sensitive benchmark. Record the 4 ns/op lines.
go test -tags integration -bench BenchmarkCopy -run "^$" -benchtime 10x ./internal/lyxtest

# Board offline — Render (pure) + UpsertFacade. Record ns/op per size.
go test -run "^$" -bench . -benchmem ./internal/boardengine/boardtest

# Board CLI (integration) — Upsert/Get/List. Record ns/op per size.
go test -tags integration -run "^$" -bench "Upsert|Get|List" -benchmem ./internal/boardengine/boardtest
```

## Step 3 — Exclude Defender, then repeat as RUN B

In an **admin** PowerShell, exclude this repo and the temp dir:

```powershell
$repo = (Resolve-Path .).Path
Add-MpPreference -ExclusionPath $repo
Add-MpPreference -ExclusionPath $env:TEMP
Get-MpPreference | Select-Object -ExpandProperty ExclusionPath   # verify both listed
```

If Tamper Protection allows and you prefer a full off (stronger signal):
`Set-MpPreference -DisableRealtimeMonitoring $true` (record which method you used).

Then run the **entire Step 2 block again** — this is RUN B (Defender excluded).

**Restore afterward** (important):

```powershell
Remove-MpPreference -ExclusionPath $repo
Remove-MpPreference -ExclusionPath $env:TEMP
# If you disabled RTP: Set-MpPreference -DisableRealtimeMonitoring $false
```

## Step 4 — Write the results file

Write everything to **`WINDOWS-BENCH-RESULTS.md` at the repo root** — a committed
transient file, so it reaches the operator's machine via git (Step 5). Use exactly
this structure so it can be folded into the docs verbatim:

```markdown
# Windows bench results — Ryzen 7 9800X3D

- Machine: Ryzen 7 9800X3D, Windows <version>, <N> logical CPUs
- Go: <go version>
- psmux present: yes/no    Cortex XDR present: yes/no
- Defender isolation method for Run B: exclusions | RTP off

## Run A — Defender ACTIVE
- Tier 1 wall-clock: <r1>, <r2>, <r3>  → median <x> s   (RESULT: all passed / FAILs: ...)
- Tier 2 wall-clock: <r1>, <r2>, <r3>  → median <x> s   (RESULT: ...)
- Fixture-copy (ns/op):
  - BenchmarkCopyPaired-<N>: <ns>
  - BenchmarkCopyPairedLocal-<N>: <ns>
  - BenchmarkCopyPairedParallel-<N>: <ns>
  - BenchmarkCopyPairedLocalParallel-<N>: <ns>
- Board Render (ns/op): n=10 <x>, n=100 <x>, n=1000 <x>
- Board UpsertFacade (ns/op): n=10 <x>, n=100 <x>, n=1000 <x>
- Board CLI (ns/op): Upsert n=10/100/1000 <x>/<x>/<x>; Get <...>; List <...>

## Run B — Defender EXCLUDED
(same structure)

## Notes
- Any test failures and whether psmux-related.
- Anything surprising (e.g. a number that got *slower* without AV).
```

## Step 5 — Commit the results back (this is the delivery mechanism)

Deliver the numbers **through git**, not by pasting into a chat — the operator is
on a different machine that shares this same remote. Stage only the results file:

```powershell
git add WINDOWS-BENCH-RESULTS.md
git commit -m "bench: Windows Ryzen 7 9800X3D results (Defender on/excluded)"
git pull --rebase        # in case the operator pushed to the branch meanwhile
git push
```

Commit **only** `WINDOWS-BENCH-RESULTS.md` — verify with `git status` that nothing
else is staged. Then print a one-line confirmation with the commit hash as your
final message. The operator's Loomyard session will `git pull`, fold the numbers
into `docs/benchmarks/`, and delete both this file and `HANDOFF.md`. Do not
summarize away the raw numbers — leave them verbatim in the file.
