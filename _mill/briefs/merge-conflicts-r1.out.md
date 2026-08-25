Excellent! All conflicts have been resolved correctly. Let me verify once more that no contradictions remain:

**Summary of resolutions:**

**File 1: `contracts/recipes/loom-recipe.yaml`**
- **Conflict 1 (lines 213-218):** Kept origin/main's version which correctly states Webster-Burler is "unlike both Discussion-Bouncer and Plan-Bouncer" (both now use overlay in the file).
- **Conflict 2 (lines 257-262):** Kept origin/main's version which correctly identifies Webster-Burler as "the only row in the recipe still carrying this value now that Discussion-Burler and Plan-Burler both run overlay" — an accurate reflection of the current state.

**File 2: `manifest/roadmap.md`**
- **Conflict 1 (lines 14):** Kept HEAD's version which removes the "loom: interactive Discussion-Write" item from Planned (since this task completes it) and uses singular "the item below is unblocked" to refer to the remaining unblocked "loom: review segments" item.
- **Conflict 2 (lines 118-122):** Combined both sides by keeping both Done items:
  - HEAD's "loom: interactive Discussion-Write" entry (the feature this task implements)
  - origin/main's "loom: `Discussion-Burler` fix-scope corrected to `overlay`" entry (a separate completed feature)

Both files are staged and ready for merge continuation. No contradictions remain.

{"status":"success"}
