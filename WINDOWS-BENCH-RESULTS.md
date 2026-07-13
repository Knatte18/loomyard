# Windows bench results — Ryzen 7 9800X3D

- Machine: AMD Ryzen 7 9800X3D, Windows 10.0.26200 (Windows 11), 16 logical CPUs
- Go: go1.26.3 windows/amd64
- psmux present: yes (3.3.4)    Cortex XDR present: no
- Defender isolation method for Run B: exclusions (repo path + `%TEMP%`, via `Add-MpPreference`)

## Note on a bug fixed before this run

`TestMultiplexerContract` (`internal/muxengine`) failed on this fresh machine because
`internal/muxengine/template_windows.yaml` hardcoded the `psmux`/`pwsh` defaults to one
operator's personal machine layout (`C:\Code\tools\bin\psmux.exe`,
`C:\Code\tools\powershell7\pwsh.exe`) instead of resolving via `PATH`, unlike the POSIX
template which already deferred to `tmux`/`bash`. Fixed in commit `94a38639` (same
branch, prior commit) to resolve `psmux`/`pwsh` via PATH by default, with
`LYX_MUX_PSMUX`/`LYX_MUX_PWSH` still available to pin an explicit path. All numbers
below are from the post-fix, all-green tree.

## Run A — Defender ACTIVE
- Tier 1 wall-clock: 3.24s, 3.29s, 3.29s → median 3.29s   (RESULT: all packages passed, all 3 runs)
- Tier 2 wall-clock: 18.90s, 18.41s, 18.67s → median 18.67s   (RESULT: all packages passed, all 3 runs)
- Fixture-copy (ns/op):
  - BenchmarkCopyPaired-16: 91277210
  - BenchmarkCopyPairedLocal-16: 49932010
  - BenchmarkCopyPairedParallel-16: 11953270
  - BenchmarkCopyPairedLocalParallel-16: 11183060
- Board Render (ns/op): n=10 8136, n=100 72225, n=1000 1407005
- Board UpsertFacade (ns/op, offline): n=10 2516697, n=100 6121658, n=1000 10717552
- Board GetDuringUpsert (ns/op, offline): 281901
- Board CLI (ns/op, integration):
  - Upsert: n=10 36445842, n=100 36293628, n=1000 36342819
  - Get: n=10 33380024, n=100 35556936, n=1000 33658615
  - List: n=10 33466785, n=100 33586121, n=1000 33473003
  - UpsertFacade (re-measured alongside CLI bench): n=10 2455388, n=100 5997859, n=1000 10845463
  - GetDuringUpsert (re-measured alongside CLI bench): 262980

## Run B — Defender EXCLUDED
- Tier 1 wall-clock: 3.95s, 1.53s, 1.44s → median 1.53s   (RESULT: all packages passed, all 3 runs)
- Tier 2 wall-clock: 16.34s, 16.09s, 15.26s → median 16.09s   (RESULT: all packages passed, all 3 runs)
- Fixture-copy (ns/op):
  - BenchmarkCopyPaired-16: 85498090
  - BenchmarkCopyPairedLocal-16: 46981540
  - BenchmarkCopyPairedParallel-16: 11916530
  - BenchmarkCopyPairedLocalParallel-16: 11783650
- Board Render (ns/op): n=10 8467, n=100 68404, n=1000 788177
- Board UpsertFacade (ns/op, offline): n=10 1498663, n=100 2107805, n=1000 7617035
- Board GetDuringUpsert (ns/op, offline): 134330
- Board CLI (ns/op, integration):
  - Upsert: n=10 35613823, n=100 35317121, n=1000 35390370
  - Get: n=10 33569871, n=100 33655138, n=1000 33704006
  - List: n=10 33491238, n=100 33909503, n=1000 33427988
  - UpsertFacade (re-measured alongside CLI bench): n=10 1564974, n=100 1892855, n=1000 7857152
  - GetDuringUpsert (re-measured alongside CLI bench): 135027

## Notes

- No test failures in either run (both Tier 1 and Tier 2 passed all 3× in Run A and Run B) —
  the only failure seen this session was `TestMultiplexerContract`, root-caused and fixed
  *before* these runs (see note above), not an AV artifact.
- Tier 1 Run B's first sample (3.95s) is an outlier relative to its own next two samples
  (1.53s, 1.44s) — most likely first-run recompilation/cache warm-up rather than an AV
  effect, given Run A's three samples were flat (3.24s/3.29s/3.29s). Using the median
  avoids this outlier either way.
- Clear AV tax shows up in the **Tier 1 median** (3.29s → 1.53s, ~54% faster) and in
  **Board UpsertFacade** (offline, in-process, allocation-heavy): n=10 2.52ms → 1.50ms,
  n=100 6.12ms → 2.11ms (~65% faster at n=100), n=1000 10.7ms → 7.6ms. `GetDuringUpsert`
  similarly drops from ~282µs to ~134µs (~2×) offline.
- Tier 2 shows a smaller but real delta (18.67s → 16.09s median, ~14% faster) — plausible
  since Tier 2 is dominated by process-spawn overhead (git, subprocess exec) rather than
  the raw small-file I/O Defender's real-time scanner inspects per-write.
- Surprising/notable: fixture-copy (BenchmarkCopy*, the benchmark this task specifically
  flagged as "most AV-sensitive") and the Board **CLI** benchmarks (Upsert/Get/List,
  which shell out to real git) both showed **no meaningful delta** between runs A and B
  (fixture-copy within noise; CLI benchmarks flat at ~33-36ms regardless of Defender).
  These are both dominated by process-spawn/exec cost (git subprocess invocations), which
  apparently isn't where Defender's real-time scanner spends time on this box — the AV
  tax shows up specifically in raw small-file read/write and in-process allocation-heavy
  work (Tier 1 compile+test, Board UpsertFacade), not in process-spawn-bound paths.
- Cortex XDR: not present on this machine (checked via `Get-Process -Name "cortex*","cyserver*"`,
  no matches) — Run A/B on this box are Defender-only, so the two Run A/B numbers above
  are a clean, single-variable (Defender on/off) comparison, unconfounded by any endpoint AV.
