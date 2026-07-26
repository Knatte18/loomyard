MILL_REVIEW_BEGIN
# Review: Treadle: shared round-loop engine + perch rewrite — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetmax
reviewed_file: plan/
date: 2026-07-26
```

## Findings

### [BLOCKING] Deleting treadle.md leaves dangling links in shed.md/hardener.md
**Location:** Batch 5 / Card 14
**Issue:** Card 14 deletes `manifest/designs/treadle.md` and instructs scanning only "the Someday Tenter/Hardener entries" (roadmap.md's Someday bullet, which names Treadle only in bold text, no literal link) for links to retarget. This misses the real dangling references: `manifest/designs/shed.md` contains four `[treadle.md](treadle.md)` links (e.g. lines 23, 38, 46, 58) and is not referenced by any card in this whole plan; `manifest/designs/hardener.md` contains four more (lines 26, 65, 109, 181) and is listed only as Context for card 14, never Edits, so nothing licenses fixing them there either.
**Fix:** Add `manifest/designs/shed.md` to card 14's Context/Edits, move `hardener.md` from Context to Edits, and retarget all eight `[treadle.md](treadle.md)` links in both files to point at `internal/treadleengine`'s package doc (or a short prose pointer), per the plan's own stated rule that deleting a linked file must not leave dangling links.

### [NIT] Model-spec registry loaded twice per perch invocation
**Location:** Batch 4 / Card 12 and Card 13
**Issue:** Card 12 has `perchengine.LoadConfig` call `modelspec.LoadRegistry(baseDir)` internally to resolve `perch.yaml`'s `judge_model`, while Card 13 separately has `perchcli`'s `PersistentPreRunE` call `modelspec.LoadRegistry(layout.Cwd)` again to resolve the profile file's model-spec keys — `models.yaml` is read and parsed twice per `lyx perch run` invocation.
**Fix:** Have `LoadConfig` return (or accept) the resolved `modelspec.Registry` so `perchcli` reuses one instance instead of loading it a second time.

### [NIT] treadleengine missing an "Other docs" entry in overview.md
**Location:** Batch 5 / Card 14
**Issue:** Card 14's `docs/overview.md` instructions cover the package tree, the perch module bullet, and the execution-stack table, but don't add a bottom-list "Other docs" bullet for `internal/treadleengine`'s package documentation — the convention this same file already follows for every other as-built engine with no `lyx <module>` command of its own (`tokenvocab`, `codeintelengine`).
**Fix:** Add an "Other docs" bullet for the `internal/treadleengine` package documentation alongside the existing `tokenvocab`/`codeintelengine` entries.

## Verdict

REQUEST_CHANGES
Fix the dangling treadle.md links in shed.md/hardener.md that Card 14's deletion introduces; the two NITs are optional polish.
MILL_REVIEW_END
