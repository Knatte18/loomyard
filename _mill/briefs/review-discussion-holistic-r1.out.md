These are illustrative input fixtures where `contains` is just `"muxpoc-"` (a prefix check), so the `mhgo` in the input cwd does not feed an expected output here — the test only checks the prefix and allowed-character pattern. So renaming `mhgo`→`loomyard` in these inputs is purely cosmetic and won't break the test regardless. The discussion's "move input + expected output together" concern applies to a *different* case (the doc-comment example in state.go line 176, and possibly elsewhere). This is fine — no paired-output break here. There's a minor note that line 116 (`/home/user/repos/mhgo-mux-design`) is a second fixture the discussion's scope text only loosely covers under "example slug `mhgo-mux-design`", but it's adequately covered.

I have enough to finalize. Let me compile findings.

Key findings:
1. **GAP** — `cmd/mhgo/main_test.go` contains `_mhgo` literals (`mhgoDir`, `_mhgo`, `_mhgo/board.yaml`) at lines 43-50, 73-80 that will break after config.go switches to `_lyx`. The scope only names `main.go` strings + the git-mv, not main_test.go's content.
2. **GAP** — root tracked `.gitignore` binary-ignore patterns `/mhgo` and `mhgo.exe` (lines 4-5) and `.mhgo/` (line 8) are not addressed in scope; after `cmd/lyx`, the binary becomes `lyx`/`lyx.exe`.

Let me double check `main_test.go` whether `BOARD_SKIP_GIT` and these tests would actually fail — yes, `run([]string{"board",...})` → `board.RunCLI` → config Load expects `_lyx` after rename, but test creates `_mhgo`, so it fails. This is real.

MILL_REVIEW_BEGIN
# Review: Rename mhgo to Loomyard (lyx)

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-06-16
```

## Findings

### [GAP] cmd/mhgo/main_test.go _mhgo literals unspecified
**Section:** Scope → CLI binary + directory
**Issue:** Scope says git-mv `cmd/mhgo/` and rename `main.go` strings, but `cmd/mhgo/main_test.go` hardcodes `_mhgo` (`mhgoDir`, `filepath.Join(cwd,"_mhgo")`, `_mhgo/board.yaml`) at lines 43-50 and 73-80; after config.go switches to `_lyx`, `TestRunDispatchesToBoard`/`...ErrorPropagatesExitCode` create the wrong dir and FAIL.
**Fix:** Explicitly list `cmd/mhgo/main_test.go`'s `_mhgo`/`mhgoDir` literals as rename sites (the git-mv moves the file but not its content).

### [GAP] Root tracked .gitignore binary patterns not in scope
**Section:** Scope → gitignore / Out
**Issue:** The tracked root `.gitignore` has hand-written `/mhgo`, `mhgo.exe` (binary-ignore, lines 4-5) and `.mhgo/` (line 8); scope only covers the `mhgo-managed` marker constants (in gitignore.go) and the init.go-generated block, never the root file's binary names, which go stale once the binary becomes `lyx`/`lyx.exe`.
**Fix:** State whether root `.gitignore` lines `/mhgo`, `mhgo.exe`, `.mhgo/` are in scope (they should be: rename to `/lyx`, `lyx.exe`, `.lyx/`).

### [NOTE] PortalTarget _mhgo vs out-of-scope portals scaffolding
**Section:** Technical context (paths.go) / Out
**Issue:** `Layout.PortalTarget` joins literal `"_mhgo"` (paths.go:141); Technical Context flags it, but Scope/Out lists `portals`/`launchers` scaffolding as OUT, creating apparent tension over whether this literal is renamed.
**Fix:** Confirm `PortalTarget`'s `"_mhgo"`→`"_lyx"` is in scope (the managed-dir name, distinct from the out-of-scope portal scaffolding).

### [NOTE] cmd/mhgo/main.go has no os.Getwd; allowlist claim overstated
**Section:** Technical context / Constraints (enforcement)
**Issue:** Discussion says failing to update the `cmd/mhgo` allowlist "breaks the build," but main.go contains no `os.Getwd`/`--show-toplevel`, so a stale allowlist would not actually fail enforcement_test; the change is still correct, just not build-critical.
**Fix:** Soften the causal claim; keep the allowlist update in scope as correctness/consistency.

## Verdict

GAPS_FOUND
Two enumerated rename sites (main_test.go literals, root .gitignore binary patterns) missing from scope.
MILL_REVIEW_END
