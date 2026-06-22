All claims are verified. The discussion is accurate. Now I have a clear picture. Key findings:

1. The test files `status_test.go`, `junction_test.go`, and `portals_test.go` are all `//go:build integration` tagged. The discussion says new fslink tests run untagged but doesn't address the integration tag status of the *surviving* worktree tests (`portals_test.go`) or the migrated `status_test.go`. The discussion says "Update `TestStatus_JunctionOk_Windows`... to expect success" and "Keep the `mklink`-failed `t.Skip`" — but `status_test.go` is integration-tagged, so under plain `go test` it won't even compile/run. There's ambiguity about whether the integration tag is removed from status_test.go.

2. The `portals_test.go` is integration-tagged because it spawned mklink (via createPortal → createJunction). After migration to direct syscall, no process spawn occurs — but the discussion says portals_test.go "should pass unchanged" without addressing whether the integration tag should be removed (which would let it run under plain `go test`, consistent with the rationale given for the new fslink tests).

Let me verify whether status_test.go actually uses mklink and confirm the integration tag concern is real. It does at line 173. The `TestStatus_Junction` table tests use `os.Symlink` not mklink. Let me also check the `TestStatus_JunctionOk_Windows` skip removal — the discussion says remove the "not recognized by os.Lstat" skip branch. That matches lines 188-191. The concern is the file-level integration tag.

Let me compose the review.

MILL_REVIEW_BEGIN
# Review: Extract internal/fslink cross-OS link primitive

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-06-22
```

## Findings

### [GAP] status_test.go integration tag not addressed
**Section:** Testing → internal/weft
**Issue:** `status_test.go` is `//go:build integration` (line 1); the plan says to update `TestStatus_JunctionOk_Windows` to expect success and keep `TestStatus_Junction` passing under the suite, but never states whether the integration tag is removed — yet the rationale ("syscall spawns no process, run under plain `go test`") implies it should be, and `TestStatus_Junction` is the only non-mklink test in that file.
**Fix:** State explicitly whether `status_test.go` keeps the `integration` tag (and if so, how `go test ./...` exercises the preserved reason-string assertions) or drops it; if dropped, decide whether `TestStatus_JunctionOk_Windows` still spawns `mklink` or builds the junction via `fslink.Create`.

### [GAP] Surviving worktree test files' integration tag undecided
**Section:** Testing → internal/worktree
**Issue:** `portals_test.go` (and likely `remove_test.go`/`weft_test.go`) are `//go:build integration` *because* `createPortal`→`createJunction` spawned `mklink`; after migration to the no-spawn syscall the same rationale that untags the new fslink tests applies, but the discussion says these "should pass unchanged" without saying whether the tag is removed.
**Fix:** Decide and state whether the integration tag is stripped from the surviving worktree link tests now that no process is spawned, or deliberately retained.

### [NOTE] mklink-spawning test helper persists post-migration
**Section:** Testing → internal/weft
**Issue:** If `TestStatus_JunctionOk_Windows` keeps spawning `cmd /c mklink /J` (line 173) to build the test junction, the suite still depends on the very double-spawn this task removes, and tests a junction-creation path no longer used in production.
**Fix:** Consider building the test junction via `fslink.Create` so the test exercises the new primitive, not `mklink`.

### [NOTE] junction_test.go verify uses os.Readlink, deemed unreliable
**Section:** Scope (port junction_test cases) / Decisions → pointsto-returns-resolved-target
**Issue:** The ported `CreatesJunction` case currently verifies via `os.Readlink` (junction_test.go:55), which the discussion itself flags as carrying the `\??\` prefix; porting it verbatim would test through the path the design rejects.
**Fix:** Note that the ported test should assert via `fslink.PointsTo`/`EvalSymlinks` rather than `os.Readlink`.

## Verdict
GAPS_FOUND
Build-tag handling for migrated/surviving link tests is unspecified; resolve before plan writing.
MILL_REVIEW_END
