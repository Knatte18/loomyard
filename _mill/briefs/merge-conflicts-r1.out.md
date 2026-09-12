Excellent! The conflict in `manifest/roadmap.md` has been successfully resolved. Here's what was done:

**Resolution Details:**

The conflict in the `## Planned` section involved two different proposed orderings:
- **HEAD (ours)**: Had `lyx loom step` item + Tier 2 item (referencing Tier 1 as "now-Done")
- **origin/main (theirs)**: Had Tier 1 item + Tier 2 item (referencing Tier 1 as current)

**Resolution Applied:**
Kept the ours side version, which correctly reflects the state of this branch:
1. Tier 1 is in the `## Done` section (already shipped by this branch)
2. `lyx loom step` is listed as first Planned item
3. Tier 2 is listed as second Planned item with correct reference to "now-Done self-report Tier 1"
4. No Tier 1 duplication in Planned section

This is the right resolution because the branch implements Tier 1 (so it belongs in Done), and the Tier 2 item correctly acknowledges Tier 1's completion.

The file has been staged and is ready for the merge to continue.

```json
{"status":"success"}
```
