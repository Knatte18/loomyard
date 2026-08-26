MILL_REVIEW_BEGIN
# Review: loom's status file can conflict on the landing merge — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Sonnet 5 (claude-sonnet-5)
reviewed_file: plan/
date: 2026-08-26
```

## Findings

### [BLOCKING:scope] run.go's own file-header comment and `Long` help text left stale
**Location:** Batch 1, Card 4 (`internal/loomcli/run.go`)
**Issue:** Card 4 rewrites only the comment block directly above `commitPaths :=`, but two other spots in the same file assert the exact claim this fix falsifies — the file-header comment ("seeds the status file when absent, commits that seed into the fabric") and the `runCmd()` `Long` text's step 1 ("seed the status file when it is absent, and commit that seed into the fabric before anything else touches it"). After the fix only the origin record is committed. `docs/overview.md:312` carried the identical claim and a prior discussion round (r5) caught it, which card 15 fixes there — but the same sentence inside `run.go` itself, including its user-facing Cobra `Long` text, is never touched by any card. CONSTRAINTS.md's CLI/Cobra Invariant makes `Long`/`Short` accuracy after a behavior change a review obligation.
**Fix:** Add to Card 4: rewrite the file-header comment (lines 1-4) and the `Long` string's step-1 line to say the origin record, not the seed, is committed.

### [BLOCKING:scope] Card 7's doc-comment list misidentifies which accessor still calls the file durable
**Location:** Batch 1, Card 7 (`internal/loomengine/config.go`)
**Issue:** Card 7 lists five sibling doc comments to rewrite: `loomDirName`, `LoomStatusLock`, `LoomRunLock`, `LoomBootstrapLock`, `LoomScratchDir` — naming `LoomRunLock`'s and `LoomBootstrapLock`'s as the two that "call the status file durable." In the actual source, `LoomRunLock`'s comment contains no durability claim at all, while `LoomDriverLog`'s comment carries the identical sentence to `LoomBootstrapLock`'s ("living under the ephemeral tree at the mirrored subpath of the durable status file per the Durable-vs-Ephemeral State Invariant") — and `LoomDriverLog` is never named anywhere in Card 7. Following the card literally leaves `LoomDriverLog`'s false "durable status file" claim shipped.
**Fix:** Replace `LoomRunLock` with `LoomDriverLog` in the list of comments requiring the durability-claim rewrite; `LoomRunLock`'s comment needs no edit.

### [NIT:consistency] Card 6's "three other callers" claim for `weftCommitCount` is wrong
**Location:** Batch 1, Card 6 (`internal/loomcli/smoke_test.go`)
**Issue:** Card 6 says "`weftCommitCount` keeps three other callers and stays." Grep shows exactly one other test function calls it (`TestSmokeDriveStandalone_RefusesOnNeverSeededPair`, two call sites), beyond the two calls inside the test this card itself renames. The instruction to keep the helper is still correct; only the count is inaccurate.
**Fix:** Say "one other caller" (or "two other call sites") rather than "three other callers."

### [NIT:scope] Newly-migrated `LoomStatusFile` not added to the subpath-anchored two-roots regression map
**Location:** Batch 1, Card 1 (`cmd/lyx/constructoranchoring_test.go`)
**Issue:** `TestConstructorAnchoring_SubpathAnchored`'s `dotLyxConstructors` map is the file's own documented "regression guard for the two-roots bug this whole re-anchoring exists to remove." Card 1 moves `LoomStatusFile`'s `assertPath` call into the `.lyx` group but never adds it to this map, so the one accessor actually being migrated by this task is the one omitted from the guard purpose-built to catch a wrong-root migration bug (the individual `assertPath` call still catches it, so this is defense-in-depth, not a live gap). Separately, the header paragraph Card 1 edits to add `LoomStatusFile` to "the `.lyx` group in full" was already missing `LoomRunLock` before this edit and stays incomplete after it.
**Fix:** Add `loomengine.LoomStatusFile` to `dotLyxConstructors`; optionally also add the missing `LoomRunLock` to the "in full" enumeration while the sentence is being touched anyway.

## Verdict
REQUEST_CHANGES
Two BLOCKING gaps leave stale/false durability and commit-behavior claims in shipped doc comments and CLI help text.
MILL_REVIEW_END
