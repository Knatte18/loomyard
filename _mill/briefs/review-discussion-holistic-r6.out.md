MILL_REVIEW_BEGIN
# Review: fabric: fold snapshot-tracking into the Warp-SHA trailer

```yaml
verdict: GAPS_FOUND
reviewer_model: fablehigh
reviewer_self_id: Claude (Fable 5, claude-fable-5)
reviewed_file: _mill/discussion.md
date: 2026-07-31
```

## Findings

### [GAP] Deletion footprint misses four comment sites
**Section:** Technical context — Deletion footprint
**Issue:** The footprint is "presented as exhaustive" and "verified by grep and by reading each file", but a case-insensitive `snapshot` grep finds four unlisted stale sites: `internal/gitrepo/gitrepo.go:429` (ChangedFilesSince's own godoc, "matching the snapshot model's SHA-to-SHA determinism" — dangles once doc.go's "# Snapshot remote model" section is deleted, and ChangedFilesSince is otherwise untouched so the re-read obligation never reaches it), `internal/gitrepo/doc.go:6` (package summary lists "snapshot tracking" as a capability), `doc.go:100` ("snapshot/correspondence tracking" in the operations-covered paragraph), and `doc.go:269` (the PlainOpen worked example "reports every existing refs/loomyard/snapshot/* key as absent"); additionally the listed `doc.go:72-74` omits its own section heading "# The self-correcting snapshot pattern" at line 70.
**Fix:** Add all four sites (plus the line-70 heading) to the footprint, or drop the exhaustiveness claim and require the implementer to re-grep case-insensitively for `snapshot` across `internal/gitrepo` non-test source.

### [NOTE] gogitLinkedFixture still writes a retired-namespace ref
**Section:** Technical context — deletion footprint (`gogit_test.go`)
**Issue:** `gogit_test.go`'s surviving `gogitLinkedFixture` (lines 89-139) and `commonSnapshotRef = "refs/loomyard/snapshot/gogittest"` (line 109, seeded via `update-ref` at 127, read at 180) compile fine after the cut — they use raw git and generic `handle.Reference()` — but the const name and comments (94-95, 102-103, 107-108) frame the retired `refs/loomyard/snapshot/*` namespace as live, the same stale-fixture-doc class the sweep explicitly covers for `linkedParityFixture`.
**Fix:** Add these fixture comments/const to the stale-comment sweep (any common-dir ref name serves the test), or record explicitly that they stay as-is and why.

## Verdict

GAPS_FOUND
Footprint's exhaustiveness claim is falsified by four unlisted stale comment sites; everything else verified accurate.
MILL_REVIEW_END
