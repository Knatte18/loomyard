Excellent! All merge conflicts have been successfully resolved. Here's a summary of the resolution:

## Conflict Resolution Summary

I resolved all four conflicting files by preserving the intent of the current branch (preflight-loom-agnostic) while incorporating the architectural improvements from the parent branch (origin/main):

1. **internal/loomshed/loomshed.go**: Combined the 13-row structure with Loom-Preflight from HEAD with the OnDone routing from origin/main. Each producer now has explicit OnDone targets for routing, with Preflight→Loom-Preflight→Discussion-Write→... workflow.

2. **internal/loomshed/loomshed_test.go**: Updated the test table to include 13 rows with the onDone field, matching the resolved producer definitions.

3. **manifest/designs/loom.md**: Merged the architectural description to reflect both Preflight and Loom-Preflight as separate rows while documenting the explicit OnDone routing mechanism.

4. **manifest/roadmap.md**: Combined both completed items in the Done section - the "preflight: split into two Shed rows" and "shedengine: per-producer bounce budget + explicit OnDone routing" tasks. Updated the scaffolding entry to say "13-row" instead of "12-row" to match the resolved structure.

All files are now internally consistent:
- 13 producer definitions with OnDone routing
- Test table with 13 rows matching the producer list
- Design documentation describing the 13-row architecture
- Roadmap documenting both completed architectural tasks

No conflict markers remain, and all contradictory claims have been resolved.

{"status":"success"}