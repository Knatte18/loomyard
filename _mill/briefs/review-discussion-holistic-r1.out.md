Confirmed: `bench_test.go` is genuinely in scope (config-path literals at lines 39-49). The discussion's inclusion criteria are sound; the issue is purely omissions, not mis-inclusions.

I have enough to produce the review. Summary of verification:

- Paths helpers (`ConfigDir`, `ConfigFile`, `LyxDirName`, unexported `configDirName`) all confirmed in `paths.go`.
- Root cause confirmed: `cli_test.go:29-35` writes stale `_lyx/worktree.yaml`; `cli.go:78` `LoadConfig(cwd,"worktree")`. Default case at line 147 (discussion says 146 — trivially off).
- Reference impl `config_test.go` confirmed (uses `ConfigFile` for write, hardcodes mkdir).
- Borderline `configcli_integration_test.go:78` confirmed and correctly handled.
- **Two config-path-hardcoding test files omitted from scope:** `internal/weft/config_test.go` and `internal/configsync/configsync_test.go` — both genuine `_lyx/config` literal violations, structurally identical to in-scope files.

MILL_REVIEW_BEGIN
# Review: Fix failing TestRunCLI in internal/worktree

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-06-24
```

## Findings

### [GAP] Sweep scope omits two config-path-hardcoding test files
**Section:** ## Scope (In, file list) / decision `resolve-paths-via-internal-paths-never-hardcode`
**Issue:** `internal/configsync/configsync_test.go` (lines 13/19, 67/73, 113, 133 — `filepath.Join(tmpDir,"_lyx","config")` + `board.yaml`/`weft.yaml`) and `internal/weft/config_test.go` (lines 50-56, `_lyx`/`config` mkdir for a `LoadConfig` test) both construct genuine `_lyx`/config paths from literals — the exact anti-pattern the sweep targets — yet neither is listed In or Out; they are structural twins of the in-scope `update_test.go` and `worktree/config_test.go`.
**Fix:** Add both files to the In list (or explicitly justify excluding them); otherwise the "repo-wide consistency, no latent migration breakage" rationale is violated by silent omission.

### [NOTE] Sweep-completeness method not stated
**Section:** ## Scope / ## Q&amp;A
**Issue:** The file list is asserted as "~12" with no described enumeration method, so a plan writer cannot tell whether omissions (above) are deliberate or missed; a grep for `"_lyx"` in `*_test.go` returns 24 files needing manual triage.
**Fix:** State the discovery query and the triage rule (config-file path construction = in; pathspec/junction-geometry/string-content/`paths` self-test = out) so the plan writer can reproduce and close the set.

### [NOTE] cli.go default-case line cite is off by one
**Section:** ## Technical context (UnknownSubcommand note)
**Issue:** Cites `internal/worktree/cli.go:146` for the `default` case; it is at line 147. Behavioural claim (config now loads, `bogus` hits default → exit 1) is otherwise correct.
**Fix:** Update the citation to 147 (non-blocking; reasoning is sound).

## Verdict

GAPS_FOUND
Two literal-config-path test files (`configsync`, `weft/config`) fall in the sweep rule but are unscoped.
MILL_REVIEW_END