Excellent! All conflicts have been successfully resolved. Here's a summary of the resolution:

## Resolution Summary

**File 1: `contracts/recipes/loom-recipe.yaml`**
- **Conflict 1 (Webster-Bouncer comment, lines 213-218)**: Kept HEAD's version stating "No commit_seam key, deliberately unlike both Discussion-Bouncer and Plan-Bouncer" — this correctly reflects the post-merge state where both Discussion and Plan Bouncers now carry `commit_seam` keys, making Webster-Bouncer the only one without one.

- **Conflict 2 (Webster-Burler comment, lines 257-262)**: Kept HEAD's version stating Webster-Burler is "the only row in the recipe still carrying this value now that Discussion-Burler and Plan-Burler both run overlay" — this accurately reflects the final configuration after the fix.

**File 2: `manifest/roadmap.md`**
- **Conflict (roadmap Done section, lines 121-122)**: Kept HEAD's version which moves the "loom: `Discussion-Burler` fix-scope corrected to `overlay`" item into the Done section, as required by the task completion rule (observable behavior changes move their roadmap items from Planned to Done in the same commit).

All resolutions preserve the intent of this branch to apply the Fabric Git Invariant fix, and both files have been staged. No conflict markers remain.

```json
{"status":"success"}
```
