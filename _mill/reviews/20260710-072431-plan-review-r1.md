MILL_REVIEW_BEGIN
# Review: Facilitate Linux support (Win11-side prep) — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-07-10
```

## Findings

### [BLOCKING] Batch 3 contract test is never executed by any verify
**Location:** Batch 3 (mux-contract-and-godoc), card 10 + batch `verify`
**Issue:** `TestMultiplexerContract` is added as the batch's core deliverable, but batch 3's verify is `go vet -tags integration ... && go test ...` — `go vet` only compiles it and the untagged `go test` skips it, so the live contract assertions run nowhere, even though `psmux.exe` is present on the dev box and the test self-skips when the binary is absent (the "deferred" part is genuinely only the tmux swap, not psmux).
**Fix:** Change batch 3 `verify` to `go test -tags integration -run TestMultiplexerContract ./internal/muxengine/...` so the test executes against the on-box psmux (and skips cleanly if absent), validating the psmux contract the card 9 godoc claims.

### [NIT] Card 15 targets an overview.md sentence that does not exist
**Location:** Batch 4, card 15
**Issue:** The card says to add `internal/shell` "in the shared-infrastructure / portability-family sentence alongside `internal/proc`, `internal/fslink`, and `internal/fsx`" and to an "`internal/` directory tree", but overview.md has no such sentence — `fslink`/`fsx` appear nowhere, and the actual shared-infra sentence (overview.md:272-274) lists `configengine`/`gitexec`/`lock`/`output`/`hubgeometry`/`state`. The anchor is fictional, so the instruction is unfollowable as written.
**Fix:** Reword card 15 to add `internal/shell` to the real shared-infra sentence (line 273) and optionally the execution-stack module map, dropping the proc/fslink/fsx "family" reference.

### [NIT] Card 4 self-contradicts on the `_windows.go` suffix
**Location:** Batch 2, card 4
**Issue:** The card instructs "Do not use a `_windows.go`/`_linux.go` filename suffix here — use explicit build tags", yet names the Windows embed file `template_windows.go`, which carries the `_windows` GOOS suffix (redundant with its `//go:build windows` tag). Functionally harmless, but contradicts the card's own rule; the posix file (`template_posix.go`) is correctly non-suffixed.
**Fix:** Either name it without the suffix (e.g. `template_win.go`) or scope the "no suffix" instruction to the posix file only.

### [NIT] Per-card Context omits same-batch producer files
**Location:** Cards 3, 6, 7, 12, 18
**Issue:** Several cards reference identifiers from a file created by an earlier card in the same batch without listing it in `Context:` — card 12 uses `shell.Shell`/`Invoke`/`ReadFile`/`Quote` but omits `internal/shell/shell.go` (sibling card 13 lists it); card 3 uses `proctree.go` helpers; card 6 uses `version.go`; card 7 uses `probe.go`; card 18 uses `launcher_content.go`. Same-batch/same-package so low cold-start risk, but per-card completeness is inconsistent.
**Fix:** Add the producer file to each consuming card's `Context:` (mirror card 13's treatment of shell.go).

### [NIT] launchers.go package doc goes stale after the .sh branch
**Location:** Batch 5, card 18
**Issue:** Card 18 replaces the non-Windows no-op with a real `.sh` branch but does not update the file's package comment (`launchers.go:1-2`: "Launchers are Windows-only; elsewhere it is a no-op"), which becomes false.
**Fix:** Have card 18 also correct the `launchers.go` package doc comment to describe the cross-platform `.cmd`/`.sh` behavior.

## Verdict

REQUEST_CHANGES
Plan is thorough and constraint-clean; fix the unexecuted integration test plus minor doc/context gaps.
MILL_REVIEW_END
