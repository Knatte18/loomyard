Note: `init.go` has a `status["mhgo_dir"]` JSON key (line 50/53/117). The naming-map's local-idents row lists `mhgoDir`/`mhgoPath`/`mhgoFile`/`mhgoIdx` but NOT a JSON map key `"mhgo_dir"`. Card 5 says "rename any `_mhgo`/`MHGO_` references" for config.go and explicitly names env vars/`_mhgo`/`mhgoDir` for init.go — but the JSON output key `"mhgo_dir"` is a runtime-observable string not explicitly named. The discussion.md (line 62) lists `mhgo_dir` among locals to rename, but the plan card does not name it. This is a residual brand string in JSON output; whether it must change is a judgment call (it's a status key, not asserted by any test per the grep). The catch-all naming-map covers it but it's not explicitly named — a NIT at most.

I have sufficient grounding. The plan is well-structured: DAG is valid, batches sequenced correctly, atomic-batch rationale sound, import-precision and prose-voice decisions faithfully described and implemented across cards, completeness fields present. The residual concerns are about a few explicitly-unnamed but naming-map-covered literals. These don't rise to BLOCKING because the naming-map table is exhaustive and each card invokes it, and the verify gate plus reviewer grep catch misses. I'll record them as NITs.

MILL_REVIEW_BEGIN
# Review: Rename mhgo to Loomyard (lyx) — holistic

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-06-16
```

## Findings

### [NIT] Card 6/8: CLI usage strings not named explicitly
**Location:** batch 1, card 6 (ide), card 8 (worktree)
**Issue:** Load-bearing runtime strings `usage: mhgo ide ...` (cli.go), the generated `mhgo ide spawn`/`mhgo ide menu` in launchers.go (asserted in launchers_test.go), and `run "mhgo init"` (config.go, asserted in config_test.go) are only covered by the catch-all "apply prose-voice to comments," which technically scopes to comments — these are non-comment strings.
**Fix:** Name these CLI-invocation literals explicitly under prose-voice (`lyx ide`, `lyx ide spawn`, `run "lyx init"`); the test assertions move with them.

### [NIT] Card 6: TestMenuRequiresMhgoDir function name not named
**Location:** batch 1, card 6 (ide)
**Issue:** `internal/ide/menu_test.go` defines `func TestMenuRequiresMhgoDir`; this Go identifier is neither a comment nor in the naming-map local-idents row, yet the reviewer grep `grep -rI mhgo --include='*.go'` (batch-1 gate) would fail on it. `docs/benchmarks/test-suite-timing.md` also references this name (covered by card 9).
**Fix:** Explicitly direct renaming the test func to `TestMenuRequiresLyxDir` and the matching doc cell.

### [NIT] Card 5: board init.go JSON key "mhgo_dir" not named
**Location:** batch 1, card 5 (board)
**Issue:** `internal/board/init.go` emits a JSON status key `status["mhgo_dir"]` (a runtime-observable output string); discussion.md lists `mhgo_dir` for renaming but card 5 names only env-vars, `_mhgo`, and `mhgoDir`.
**Fix:** Name the `"mhgo_dir"` output key explicitly (→ `"lyx_dir"`) or confirm it is intentionally retained.

## Verdict

APPROVE
DAG, sequencing, decisions, and completeness are sound; only minor unnamed-literal nits remain.
MILL_REVIEW_END
