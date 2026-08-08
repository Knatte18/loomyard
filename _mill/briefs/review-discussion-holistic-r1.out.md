MILL_REVIEW_BEGIN
# Review: Scoutengine: rewrite CONSTRAINTS.md as a seam rule, convert leaf test to banned-list, add LSP guard

```yaml
verdict: GAPS_FOUND
reviewer_model: opusmedium
reviewer_self_id: Claude (Opus-class), Anthropic
reviewed_file: _mill/discussion.md
date: 2026-08-08
```

## Findings

### [GAP] Converted test's header comment/failure text undecided
**Section:** Decisions → "Banned-list test: rename the file, reuse the three existing predicates"
**Issue:** `leaf_enforcement_test.go` lines 1–7 assert the allowlist "keeps the LSP subprocess client stdlib-only" and line 101 names "the allowlist (stdlib + configengine + lock + proc + logger + yaml.v3)" — the same false claim the discussion hunts down in `doc.go`, yet the decision only covers the filename, function name, and `allowedImports` map.
**Fix:** State explicitly that the file header comment and the `t.Errorf` message are rewritten to the seam/banned-list framing, with the "stdlib-only" wording removed.

### [GAP] Fate of the stdlib heuristic and the catch-all branch unstated
**Section:** Decisions → banned-list test; Technical context → Gotchas
**Issue:** With `allowedImports` deleted, the `isStdlib` heuristic (lines 61–70) and the trailing catch-all `failures = append(failures, relPath+": "+importPath)` (lines 90–91) become dead or wrong — the catch-all would fail `logger`/`yaml` — but the Gotchas say "reuse the stdlib heuristic rather than inventing a second one", and only the new guard file actually needs it.
**Fix:** Decide whether the catch-all is deleted and where the stdlib helper lives after the conversion (kept in the seam file as a shared package-scope helper, or moved into `lspclient_guard_test.go`).

### [GAP] Directory-scan shape for the converted test left open
**Section:** Decisions → banned-list test vs Technical context → "Shape to mirror"
**Issue:** The decision keeps the current file's `filepath.WalkDir` predicates verbatim, while "shape to mirror" specifies `shuttleengine`'s `os.ReadDir` "so it scans only the package's own files and not subpackages" — the discussion never picks one.
**Fix:** Name the scan API the converted test uses (behaviourally identical today, since `internal/scoutengine` has no subpackages).

### [NOTE] Peer-precedent survey overstates "no import guard of any kind"
**Section:** Decisions → "Drop the allowlist entirely"
**Issue:** `internal/shuttleengine/seam_enforcement_test.go` (`TestProviderSeamImportRule`) *is* an import guard on an engine package, and the same discussion cites it as the shape to mirror; the survey lists `shuttleengine` among engines with none (`boardengine`/`websterengine`/`builderengine` also carry call-site guards in `cmd/lyx`).
**Fix:** Reword the claim to "no *allowlist*" — which is the property the decision actually rests on and is accurate.

### [NOTE] Commit state of the pre-staged CONSTRAINTS.md edit is ambiguous
**Section:** Decisions → "CONSTRAINTS.md pre-staged during mill-start"
**Issue:** The text says both "applied to the working tree" and "the committed section now reads", so a plan writer cannot tell whether mill-go must still commit that hunk or only the code/doc changes.
**Fix:** State plainly whether the scout section is already committed on this branch, and which commit-per-fix step (if any) owns it.

### [NOTE] `doc.go` line ranges cited inconsistently
**Section:** Scope / Decisions / Q&A
**Issue:** The paragraph to rewrite is cited as lines 24–34, 22–34, and 24–27 in three places; the actual "# The engine/CLI split" block is lines 22–34, with the allowlist enumeration at 24–26.
**Fix:** Use one range (22–34) throughout so the plan targets the whole paragraph, not just the enumeration.

## Verdict

GAPS_FOUND
Three deliverable-shaping details of the test conversion remain undecided.
MILL_REVIEW_END
