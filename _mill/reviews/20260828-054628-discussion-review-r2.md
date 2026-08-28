MILL_REVIEW_BEGIN
# Review: Audit internal/logger coverage across spawn/hard-error paths

```yaml
duration_s: 102.3
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-28
```

## Findings

### [BLOCKING:design] Sharpened invariant wording never stated
**Section:** `constraints-md-prose-only` / Constraints
**Issue:** The decision says exhaustively what the CONSTRAINTS.md edit must *not* contain, but never states what the new scope wording *is*; the current "for a round/strand/session" phrasing does not cover the `vscode` launcher or the `$EDITOR` spawn the task adds `Info` lines to, so the plan writer must invent the substantive half of the amendment.
**Fix:** State the replacement sentence (or its precise scope predicate — e.g. every production `exec.Command*` site, with the `Debug`-only polling carve-out) in the decision.

### [NIT:scope] Publish's githubclient caller is not the one named
**Demoted-from:** BLOCKING
**Section:** Scope In / `githubclient-leaf`
**Issue:** The Problem section names Publish's GitHub calls as the gap, but `internal/githubclient` has two production callers — `internal/selfreportengine/selfreport.go` and `internal/landingshed/publish.go` (imports at `publish.go:22`, uses `ParseOwnerRepo` and `ErrTokenUnresolvable`) — and only `selfreportengine` is in scope, so the flagged gap stays open.
**Fix:** Give `internal/landingshed/publish.go` an explicit verdict (it already imports `logger` via `stuck.go`, so it is cheap), or state why Publish is deliberately deferred.

### [BLOCKING:design] Hard-error half has no enumeration result
**Section:** `error-universe` / Scope In
**Issue:** The spawn half gets a 19-row verdict table, but the hard-error half gets only a one-line judgment rule ("terminates an orchestration unit") with no seed enumeration, no count, no per-site verdicts, and no guard test — so neither the audit document's completeness nor the derived code-change list (currently only `singlellm.go`) is checkable, and two plan writers would produce different tables.
**Fix:** Either give the hard-error universe a mechanically stated selector plus the sites it currently yields, or narrow the deliverable to the named orchestration seams (Shed producer outcome mappings, engine `Run` returns) and say so.

### [NIT:consistency] Tier-purity handling is stated backwards and unscoped
**Demoted-from:** BLOCKING
**Section:** Constraints ("must not be written in a way that trips `tierpurity_test.go`")
**Issue:** Sibling guards do not avoid the banned tokens — `cmd/lyx/tierpurity_test.go`'s `allowedSpawners` map (lines 32–41) lists all nine of them with written reasons, so the new guard requires an edit to an existing test file that the Scope In section never lists as a deliverable.
**Fix:** Replace the "avoid tripping it" phrasing with an explicit in-scope item: add `cmd/lyx/spawnobservability_test.go` to `tierpurity_test.go`'s `allowedSpawners` with a reason.

### [NIT:scope] Excluded sites still need guard allowlist entries
**Section:** Scope Out / `enforcement-guard`
**Issue:** `internal/hubforge/hub.go` and `cmd/testtiming/main.go` are production non-`_test.go` files inside the guard's declared walk scope, so they must appear in the guard allowlist; the discussion states this only for the three `blocked` sites.
**Fix:** Say that `excluded` sites inside `internal/`/`cmd/` also carry an allowlist entry with their exclusion reason.

## Verdict

REQUEST_CHANGES
Missing invariant wording, an uncovered githubclient caller, an unenumerated error half, and an unscoped allowlist edit.
_Note: 2 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 2._
MILL_REVIEW_END
