# Linux portability survey (kartlegging)

> **Status: ALL FINDINGS RESOLVED (re-verified 2026-07-15).** Every failure this survey
> documented — B1, B2, B3, all three Category-A tests, and the Category-C perf
> pathology — currently **passes** on this Linux box (the perf test dropped from ~130s
> to ~0.02s). This is kept as the historical record of the original investigation, not
> as an open issue list; nothing below still needs action. The fix work landed across
> several subsequent tasks (including `mux-psmux-to-tmux-rename` for B3's path-resolution
> sub-issue); this file was not updated commit-by-commit as each finding closed, so no
> single commit reference is given per row — the point-in-time re-verification above is
> what's authoritative now.

Empirical map of what breaks when Loomyard's Go test suite is run on **Linux** for the
first time. The codebase was written and exercised exclusively on Windows 11; nothing
here had ever run on Linux before this survey. The trigger was the *"run the full
benchmark suite on Linux, mark OS in results"* task — which was **blocked** until the
suite went green on Linux, because you cannot record a comparable Linux baseline from a
red suite. This file was that blocker, written down.

All claims below are hands-on, verified on this box unless marked **UNVERIFIED**.

## Environment (verified 2026-07-13)

- OS: **Ubuntu 26.04 LTS** (`resolute`), `linux/amd64`
- CPU: **AMD Ryzen AI 7 445 w/ Radeon 840M**, **12 logical CPUs**
- Go: **go1.26.0 linux/amd64** (Windows benches used **1.26.4** — minor skew, note it)
- git: **2.53.0**; `core.symlinks` unset (Linux default = true)
- **tmux: NOT installed** — `command -v tmux` fails
- `go build ./...` and `go build -tags integration ./...` both **compile clean**. Every
  gap below is **runtime behaviour**, not a build break — the OS-split file model
  (`*_windows.go` / `*_linux.go`, `//go:build` tags) is intact and complete enough to
  compile on Linux.

**Interpretive frame (operator):** the Windows machines that produced every existing
benchmark run **Cortex XDR**, which throttles all file-heavy operations. So a Linux
number that is *faster* than Windows is expected and uninteresting; a Linux number that
is *slower* (e.g. the 130 s boardtest below) signals a genuine Linux-specific pathology,
not AV overhead.

## How the map was built

- **Dynamic:** `go run ./cmd/testtiming` (Tier 1) and `go test -tags integration ./...`
  (Tier 2), collecting every FAIL and its message.
- **Static:** grep sweep for Windows assumptions — drive-letter paths, `PosixPath`,
  `runtime.GOOS == "windows"` branches, `*_windows.go` files without POSIX siblings,
  `.exe` defaults, junction/symlink handling, CRLF.

## Failure summary

| Tier | Package | Failing test(s) (at survey time) | Category | Status (2026-07-15) |
|------|---------|-----------------|----------|----------|
| 1+2 | `internal/shuttleengine/claudeengine` | 5× `TestPrepare_*` | **B1 — prod bug (critical)** | ✅ FIXED — passes |
| 1+2 | `internal/muxengine` | `TestLoadConfig_TemplateDefaultsResolve` | A — Windows-only test assertion | ✅ FIXED — passes |
| 1+2 | `internal/shuttleengine` | `TestRunDirRoot_AbsoluteUsedVerbatim`, `TestSpec_Validate_AbsoluteOutputFilesPassThroughVerbatim` | A — Windows-only test assertion | ✅ FIXED — passes |
| 2 | `internal/warpengine` | `TestStatus_LyxPollutionDetected` | **B2 — junction≠symlink (deep)** | ✅ FIXED — passes |
| 2 | `internal/warpengine` | `TestPrune_DoubleRemovalFailureNoStderrLeak` | A — Windows-FS-semantics test | ✅ FIXED — passes |
| 2 | `internal/muxcli` | `TestRunCLI_AddNotUp_FriendlyError`, `TestRunCLI_RemoveNotUp_FriendlyError` | B3 — env + robustness | ✅ FIXED — passes |
| 1 | `internal/boardengine/boardtest` | `TestConcurrentReadsDuringUpserts` (**passes**, but **130 s**) | **C — perf pathology** | ✅ FIXED — 0.02s |

Tier 2 is a superset of Tier 1, so it re-hits the three Tier-1 packages and adds
`warpengine` + `muxcli`. Nothing in `internal/warpengine`'s real-git worktree/junction
machinery failed *except* the two rows above — the symlink model otherwise carries the
weft/host topology fine on Linux.

## Category B — production Windows assumptions (real Linux bugs)

### B1 (CRITICAL, RESOLVED) — the Claude engine cannot `Prepare()` on Linux

`internal/shuttleengine/claudeengine/claudeengine.go:109`

```go
eventsPath := filepath.Join(runDir, "events.jsonl")
eventsPathPosix, err := shuttleengine.PosixPath(eventsPath)   // <-- unconditional
if err != nil {
    return shuttleengine.Launch{}, fmt.Errorf("convert events path to posix: %w", err)
}
```

`shuttleengine.PosixPath` (`internal/shuttleengine/posix.go`) is a **Windows→git-bash**
converter: it rewrites `C:\a b\c` → `/c/a b/c` and **returns an error unless the input is
drive-rooted** (`"<letter>:/..."`). On Linux the events path is `/tmp/.../events.jsonl`,
which is not drive-rooted, so `PosixPath` rejects it and `Prepare()` fails with
`shuttle: PosixPath: not a drive-rooted absolute path`. All five `TestPrepare_*` failures
are this one call.

**Why it matters most:** per `CLAUDE.md`, the Claude engine is what drives *every* agent
lyx spawns (loom producers, review handler, cluster reviewers, progress-judge) as tmux
sessions. If `Prepare()` cannot run on Linux, agent-driving — the whole architecture — is
blocked on Linux, independent of benchmarks.

**Direction:** the fix pattern already exists two lines below (`sh := shell.ForGOOS()`).
`PosixPath` conversion is only meaningful for the git-bash pane shell on Windows; on POSIX
the path is already what the native shell/hook wants. Guard the conversion by GOOS (or make
`PosixPath` a pass-through for already-absolute POSIX paths). Only call site is this one —
narrow blast radius.

### B2 (DEEP, RESOLVED) — `_lyx` junction vs symlink: git refuses pathspecs "beyond a symbolic link"

`internal/warpengine` — `TestStatus_LyxPollutionDetected`:

```
fatal: pathspec '/tmp/.../hub/_lyx/accidental.txt' is beyond a symbolic link
```

On Windows, `_lyx`/`_raddle` are **directory junctions** (mount-point reparse points) — git
treats a junction as an ordinary directory, so `git add hub/_lyx/...` works. On Linux,
`internal/fslink` creates a **symlink** (`fslink_linux.go`, `os.Symlink`), and git
**refuses** any pathspec that traverses a symlink as a safety measure — a categorical git
behaviour, *not* a `core.symlinks` toggle. This is the Hub Geometry / fslink contract
meeting a semantic that does not translate: the junction model assumes the linked tree is
addressable through the link, which holds on Windows and does not on Linux.

**Direction (needs design, not a one-liner):** options include (a) addressing `_lyx`
contents via the real target path rather than through the link when invoking git; (b)
re-evaluating whether `_lyx`/`_raddle` should be links at all on Linux vs bind-style or
real dirs; (c) confining the affected operations. This is the one finding that touches a
`CONSTRAINTS.md` invariant (Hub Geometry / fslink) and deserves its own task and
discussion. It is **not** in scope for "record benchmark numbers."

### B3 (RESOLVED) — mux multiplexer binary: raw exec error + path resolution

`internal/muxcli` — `TestRunCLI_AddNotUp_FriendlyError` / `RemoveNotUp`:

```
RunCLI(add) before up error = "check session: exec: \"tmux\": executable file not found in $PATH";
  want "no mux session; run \"lyx mux up\""
```

Two sub-issues (now addressed by the `mux-psmux-to-tmux-rename` task):
- **Environment:** `tmux` is not installed on this box; the POSIX mux path shells out to
  `tmux` (Windows uses tmux via the psmux port). Any mux run needs tmux present.
- **Robustness (path resolution):** the friendly-error path maps "session not found"
  but not "multiplexer binary missing" — older documentation and examples hardcoded
  specific absolute paths like `C:\Code\tools\bin\psmux.exe` / `pwsh.exe`, which are
  unverifiable across different machine setups. This has been fixed by the `mux-psmux-to-tmux-rename`
  rename task, which updated all examples to resolve tmux via PATH and use env-var overrides
  (e.g., `LYX_MUX_TMUX`) for customization.

**Direction (CLOSED):** the hardcoded-path problem is resolved by genericizing examples to
use PATH-resolved binary names and documenting env-var overrides.

## Category A (RESOLVED) — Windows-only test assertions (test-level; no prod impact)

These are tests that bake Windows path/FS semantics into their expectations. Production
code is fine; the *tests* are non-portable and should be made OS-aware or POSIX-tagged.

- **`internal/muxengine` `TestLoadConfig_TemplateDefaultsResolve`** (`config_test.go:47-52`)
  hardcodes `cfg.Tmux == \`tmux\`` (bare command name, resolved via PATH) and `cfg.Pwsh == \`bash\`` on POSIX.
  The `muxengine.ConfigTemplate()` is **OS-split** (`template_windows.go` /
  `template_posix.go`); on Linux it resolves `Tmux=tmux`, `Pwsh=bash`. The test must
  assert the OS-appropriate default (mirror the template split).
- **`internal/shuttleengine` `TestRunDirRoot_AbsoluteUsedVerbatim`** and
  **`TestSpec_Validate_AbsoluteOutputFilesPassThroughVerbatim`** feed `D:\elsewhere\runs`
  and expect it passed through as absolute. On Linux `D:\...` is a *relative* path, so it
  gets joined onto the worktree (`C:\worktree/D:\elsewhere\runs`). The absolute-passthrough
  behaviour is correct; the test's fixture is a Windows path. Use an OS-appropriate
  absolute fixture (`/abs/...` on POSIX).
- **`internal/warpengine` `TestPrune_DoubleRemovalFailureNoStderrLeak`** asserts removal
  **fails** while a file has an open handle + lock (`Removed=true; want false`). That is
  Windows filesystem semantics — Windows blocks deleting an open file; Linux happily
  unlinks it. The invariant "a locked/open entry must not be reported removed" is
  Windows-specific; the test needs a Linux-appropriate "make removal fail" mechanism
  (e.g. a read-only parent dir) or a `//go:build windows` guard with a POSIX analogue.

## Category C (RESOLVED) — performance pathology (blocks a meaningful Tier 1 Linux baseline)

**`internal/boardengine/boardtest` `TestConcurrentReadsDuringUpserts`: ~130 s on Linux vs
0.45 s on Windows** (~290×). It *passes*, but single-handedly dominates the Tier 1
wall-clock (~130 s vs Windows ~10 s), so any Tier 1 Linux number is meaningless until this
is understood.

Shape (`concurrency_test.go`): one writer doing `writes = 10` upserts, `readers = 8`
goroutines each **hot-spinning with no yield** — `for { select { <-stop: return; default: }
GetTask(); ListTasksBrief() }` — until the writer closes `stop`. On Windows the writer's 10
upserts (each = 3 atomic temp+rename, AV-scanned) finish in ~0.45 s. On Linux the same 10
upserts apparently take ~130 s.

**Hypothesis (UNVERIFIED):** on unthrottled Linux the 8 hot-spinning readers saturate all
12 cores and starve the single writer's rename/commit progress; on Windows, Cortex XDR
incidentally throttled the *readers* too, so the writer was never starved. The test comment
itself notes it was tuned against "endpoint AV" write cost — a Windows-shaped assumption.
Needs profiling to confirm; likely fix is a small yield/`runtime.Gosched()` or bounded
reader rate, without changing what the test demonstrates.

## What this means for the benchmark task

1. The benchmark-recording deliverable is **blocked** on a green Linux suite. Recording
   numbers from the current red/pathological state would be dishonest.
2. Recommended sequencing (each arguably its own task):
   - **A-tier + C:** make the Category-A tests OS-aware and fix the boardtest hot-spin →
     gets Tier 1 green and fast on Linux. Low risk, mechanical.
   - **B1:** guard `PosixPath` by GOOS → unblocks the Claude engine on Linux. Small, high value.
   - **B3:** install tmux + map "binary missing" to the friendly error → mux packages green.
   - **B2:** separate design task — the `_lyx` junction/symlink git behaviour touches a
     `CONSTRAINTS.md` invariant and must not be rushed into a benchmark card.
3. Only after Tier 1 + Tier 2 are green on Linux does "record parallel OS-marked numbers"
   become executable — at which point each existing `Machine:` line gets marked
   `Windows 11 Enterprise` and a parallel dated Linux section is added per doc (the
   originally-agreed format).

## Environment prerequisites for a fair Linux run (once unblocked)

- Go **1.26.4** to match the Windows toolchain (currently 1.26.0 — note the skew).
- **tmux** installed and on `PATH`.
- Confirm git symlink support (`core.symlinks=true`, default on Linux) — but note B2 is a
  separate git-pathspec-through-symlink issue, not fixed by that flag.
