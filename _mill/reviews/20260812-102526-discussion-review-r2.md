MILL_REVIEW_BEGIN
# Review: fabric: close the corrindex two-phase read-modify-write race (slice 15)

```yaml
duration_s: 190.0
verdict: APPROVE
reviewer_model: opus
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-12
```

## Findings

### [NIT:scope] Files-to-change omits the four inbound-reference docs
**Section:** Technical context → "Files to change" **Issue:** The list names `state.go`, `corrindex.go`, `doc.go`, the two test files, the deleted design doc and `roadmap.md`, but not `manifest/designs/{lyxtest-real-hubs,fabric-windows-verification,fabric-unified-view,gitexec-error-shape}.md`, which carry six of the nine references and are edited by this task. **Fix:** Add those four files to the list, or state that the reference table is the authoritative file list for the docs half.

### [NIT:design] `UpdateJSON`'s behaviour on an undecodable existing file is unstated
**Section:** Decisions → `updatejson-signature-mirrors-readjson`; Testing → `state` scenarios **Issue:** The contract specifies missing-file (`zero`, `found=false`), existing-file, and mutate-error, but never says what happens when `path` exists and fails to unmarshal — abort with an error (ReadJSON's precedent) or hand `mutate` the zero value and let it overwrite. The test scenario list has no such case. **Fix:** State the disposition explicitly (inheriting `ReadJSON`'s error) and add it to the scenario list, since `record()`'s failure now propagates to the weft-commit path.

### [NIT:consistency] `internal/state`'s "package header" is not a godoc package comment
**Section:** Decisions → `no-new-constraints-invariant`; Scope "In" **Issue:** The decision rejects a `CONSTRAINTS.md` invariant on the grounds that the package header is "where the next author actually meets the rule", but `internal/state/state.go:1-5` is separated from `package state` (line 7) by a blank line at line 6, so it is a detached file comment and never appears in `go doc`/godoc; there is no `doc.go`. **Fix:** Say where the rule lands so it is actually godoc-visible (attach the block to the package clause, or add a package doc), rather than assuming the existing block is one.

## Verdict

APPROVE
Decisions, residual windows, call-graph reasoning and reference inventory all verify against source.
MILL_REVIEW_END
