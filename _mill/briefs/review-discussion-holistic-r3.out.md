MILL_REVIEW_BEGIN
# Review: loom's status file can conflict on the landing merge

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Opus-class), Anthropic
reviewed_file: _mill/discussion.md
date: 2026-08-26
```

## Findings

### [BLOCKING:scope] loomengine's own path tests not dispositioned
**Section:** Testing → "Tier 1 — path guards", and Technical context ("its two guard tests")
**Issue:** `internal/loomengine/loomstatus_test.go` pins `LoomStatusFile` under `lyxdirs.LyxDirName` at lines 26 and 65 (plus its file-header comment) and fails at *runtime*, not compile time, after the move; `internal/loomengine/config_test.go`'s `TestLoomStatusRel` (line 196) and `TestLoomStatusFile_EqualsAnchorPathJoinedWithLoomStatusRel` (line 206) also exist — so "the one constructor and its two guard tests" is three guard files, not two.
**Fix:** Name `internal/loomengine/loomstatus_test.go` and `config_test.go` in the Testing section with explicit dispositions (moved assertion vs deleted-with-the-function), and correct the "two guard tests" claim.

### [BLOCKING:consistency] Migration rationale contradicts the conflict mechanics
**Section:** Decisions → "No code-level migration for hubs carrying the tracked file"
**Issue:** The rationale asserts a leftover tracked `_lyx/loom/status.json` "would keep conflicting on the landing merge exactly as it does today", but the doc's own mechanics (Problem, and Q&A "the divergence requires both sides to have rewritten the file since their merge base") make a conflict impossible once no code writes that path — post-upgrade both sides stay identical to the merge base, so the leftover is inert junk, not a live bug.
**Fix:** Restate the rationale on its true grounds (removing dead tracked junk, plus the in-flight-run `history`/budget gate, which is sound as written) and drop the "keeps conflicting" premise, so the operator note in `loom.md` is not written from a false claim.

### [NIT:consistency] Wording pass misses the `loomDirName` doc comment
**Section:** Scope → second enumeration pass
**Issue:** `internal/loomengine/config.go:29–31` says `loomDirName` "joins onto lyxdirs.LyxDirName or lyxdirs.DotLyxDirName"; after the move nothing joins it onto `LyxDirName`, yet the comment carries neither `LoomStatusFile`/`LoomStatusRel` nor `durable`/`tracked`/`weft-synced`, so neither stated grep reaches it.
**Fix:** Add it to the known-hit list, and note that after the move `loomengine` exposes a `.lyx` scratch tree with no `_lyx` counterpart at all — worth one sentence confirming the Durable-vs-Ephemeral mirrored-subpath wording is satisfied vacuously rather than violated.

## Verdict

REQUEST_CHANGES
One enumeration gap in test dispositions and one false premise under the migration decision.
MILL_REVIEW_END
