The overview's `Layout` method list at line 47 omits `PortalLink(slug)` and `MenuLauncherPath()`, `LauncherSpawnRel(slug)`, `MenuLauncherRel()`. But this is comparing against the full method set — the old version probably had the same omissions, so this isn't introduced by this task. Let me check whether the plan actually specifies this needs to be updated in overview.md (Card 13). Card 13 says to replace `HubName()` with `PrimeName()` in the Layout methods list at approximately line 47. That specific fix is done — `PrimeName()` is present. The partial method list in overview.md is pre-existing, not introduced by this task.

I now have enough to form my complete assessment. Let me summarize findings:

1. **Roadmap milestone entries missing task numbers (Card 14 deviation):** Card 14 explicitly requires "Each milestone entry should include: task number, title, and a one-sentence summary." Milestones 5, 6, 7 in `roadmap.md` include title and summary but omit the task numbers (006, 007, 008). This is a NIT — the content is correct, only the format requirement was missed.

2. **`worktree.md` state schema still uses `container` field name (line 74):** The deferred registry schema shows `slug → { path, branch, container }`. Card 16 said to replace "hub" used for the container with "Hub", but the `container` key in this JSON snippet was not updated to `hub`. This is arguably a NIT since the plan targeted "hub" text replacements, not "container" text, and this is a schema field name in a deferred feature that isn't implemented yet.

3. **All code changes** (Batch 1 rename, Batch 2 migration) appear correctly implemented. No old identifiers remain in Go source.

4. **All doc changes** (Batch 3) appear implemented including the new Weft overlay model section, deprecation notices on portal methods, config path updates, and CONSTRAINTS.md.

5. **No constraint violations** — no raw `os.Getwd` or `git rev-parse` outside the permitted files; no out-of-plan files; no duplicate utility implementations.

MILL_REVIEW_BEGIN
# Review: Weft repo — companion-repo overlay for lyx — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-06-18
```

## Findings

### [NIT] Roadmap milestone entries omit task numbers
**Location:** `C:\Code\loomyard\wts\weft-repo\docs\roadmap.md:49-56`
**Issue:** Card 14 requires each new milestone entry to include the task number, title, and a one-sentence summary; milestones 5, 6, and 7 include title and summary but do not carry the task identifiers (006, 007, 008) that the batch spec mandates.
**Fix:** Prefix each entry title with the task number, e.g. "**Task 006 — Weft engine.**" to satisfy the Card 14 format requirement.

### [NIT] Deferred state-registry schema retains `container` field name
**Location:** `C:\Code\loomyard\wts\weft-repo\docs\modules\worktree.md:74`
**Issue:** The deferred registry schema `slug → { path, branch, container }` still uses `container` as the field name; Card 16 renames Hub/Prime terminology throughout this file, and the accompanying prose in the same section was updated, making this key inconsistent with the surrounding terminology.
**Fix:** Change `container` to `hub` in the schema snippet to align with the renamed Hub term.

## Verdict

APPROVE
All code and documentation changes are correctly implemented; two cosmetic NITs only.
MILL_REVIEW_END