MILL_REVIEW_BEGIN
# Review: reed: extract Selvage-pane lifecycle

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: claude-opus-4 class (self-assessed; brief dictates "opusmedium")
reviewed_file: _mill/discussion.md
date: 2026-09-19
```

## Findings

### [BLOCKING:decision] reed-header-selvage.md has no disposition
**Section:** `docs-in-the-same-commit` / Scope Out
**Issue:** `manifest/designs/reed-header-selvage.md:41` asserts as shipped fact that "`internal/reedengine/lifecycle.go`'s `ensureSelvagePaneLocked` ensures Selvage exists … via `splitSelvagePaneAtBottomLocked`"; after this extraction that file attribution is false, yet the docs decision enumerates only `doc.go`, `reed-selvage-pane-extraction.md` and `roadmap.md` and presents the list as exhaustive ("not `docs/overview.md`, not `CONSTRAINTS.md`").
**Fix:** State a disposition for `reed-header-selvage.md` — either correct line 41's file attribution in this commit, or record why a shipped-item design doc is allowed to keep a stale owning-file name (its line 13 "~230 lines across `apply.go`/`reconcile.go`/`spawn.go`/`lifecycle.go`" is historical framing and plausibly stays).

### [NIT:scope] Audit enumeration omits the two YAML templates
**Section:** Problem ("Why now") / Technical context
**Issue:** The per-file `Selvage` baseline lists only `.go` files and explicitly dispositions `template.go`, but `internal/reedengine/template_posix.yaml:14` and `template_windows.yaml:14` each carry one case-sensitive `Selvage` hit, so a re-run of "the per-file `Selvage` count" over the package has two unenumerated before-figures.
**Fix:** Either scope the audit method to non-test `.go` files explicitly, or add the two template YAML lines to the baseline as expected-unchanged.

### [NIT:design] Allowlist granularity for state.go/config.go unstated
**Section:** `enforcement-test-is-the-teeth`
**Issue:** `selvagepane.go` is allowlisted "entirely" while `state.go`/`config.go` are allowlisted "for" specific declarations — unclear whether the mechanism is a file set (as in `internal/cliwire/bannedecl_enforcement_test.go`) or a file+identifier pair set; both pass identically today, so the choice is unpinned rather than forced.
**Fix:** Say which, so the plan does not have to invent a stricter mechanism than the cited pattern provides.

## Verdict

REQUEST_CHANGES
One shipped design doc names a moved function's old file and gets no stated disposition.
MILL_REVIEW_END
