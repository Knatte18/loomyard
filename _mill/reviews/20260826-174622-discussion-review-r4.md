MILL_REVIEW_BEGIN
# Review: loom's status file can conflict on the landing merge

```yaml
duration_s: 204.0
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewer_self_id: Claude (Opus-class, Anthropic)
reviewed_file: _mill/discussion.md
date: 2026-08-26
```

## Findings

### [BLOCKING:design] Enumeration passes miss bare `_lyx/` mentions
**Section:** Scope (the two enumeration bullets) / Technical context
**Issue:** All three stated passes — literal `_lyx/loom/status.json`, the `LoomStatusFile`/`LoomStatusRel` identifiers, and `durable`/`tracked`/`weft-synced` wording — miss a fourth class that names the status file's home as bare `_lyx/`: verified hits at `manifest/designs/loom.md:269` ("reads loom's **status file** in `_lyx/`"), `:318` ("cold-starts from the `_lyx/` status file"), `:449` ("loom writes the `_lyx/` status file"), and `:303` (the "not bare `_lyx/status.json`" product-scoping rationale), none of which fall in the three loom.md sections the scope bullet names; `internal/shedengine/doc.go:44` states the caller obligation as "the status file durable, both locks never-tracked transients", which this change falsifies for loom and which the scope bullet does not list (it lists only `shed.go:13`).
**Fix:** Add a third pass over `_lyx` + "status file" co-occurrence (or simply grep `_lyx` in `manifest/designs/loom.md`, `manifest/designs/shed.md`, and `internal/shedengine/*.go`), and add `internal/shedengine/doc.go`'s Told-never-derived paragraph to the doc-edit list; drop or qualify the claim that the wording pass's hit list is "complete".

### [BLOCKING:design] Operator migration step names no mechanism
**Section:** Decisions § No code-level migration
**Issue:** The step says to "delete the now-orphaned `_lyx/loom/status.json` from the weft branches that carry it" but names no procedure, and the weft repo is not operator-facing: `CONSTRAINTS.md`'s Fabric Git Invariant routes weft mutation through `internal/fabricengine`, the Constraints section of this discussion never cites that invariant, and the note's obvious naive spelling (raw `git rm` inside the weft sibling worktree) is the one shape the repo's model discourages — while a sanctioned spelling exists (`rm` through the worktree's `_lyx` junction, then `lyx fabric commit`, whose staging pathspec already covers `_lyx`, verified at `internal/fabriccli/weft_verbs.go:136-169`).
**Fix:** State the sanctioned command sequence the loom.md note must carry, and where it is performed (each affected worktree, parent included), so the plan writer does not invent a raw-git procedure.

### [NIT:scope] `commitweftpaths.go` doc comment references the deleted symbol
**Section:** Scope § second enumeration pass ("Known hits")
**Issue:** `internal/fabricengine/commitweftpaths.go:92` says relPaths are "the same shape OriginRecordRel and LoomStatusRel already return" — an identifier-grep hit that names a function this task deletes, and it is absent from the bullet's hit list even though that list claims to be complete.
**Fix:** Add it to the known-hit list (it is doc-comment drift only; the stated identifier grep does reach it).

## Verdict

REQUEST_CHANGES
Enumeration method has a fourth-class blind spot; the operator migration step lacks a mechanism.
MILL_REVIEW_END
