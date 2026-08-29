All conflicts have been successfully resolved and staged. Here's what was done:

**Resolution Summary:**

1. **apply_test.go**: Used origin/main's more comprehensive test structure with the `newZeroPinApply` helper function, which handles both watchdog on/off cases elegantly.

2. **doc.go**: Combined explanations from both sides into a single coherent paragraph explaining why the plain-first/-a-after set-hook pattern keeps rebuilds idempotent.

3. **reapply.go**: Used origin/main's cleaner implementation using the `hookArrayEntries()` helper function for parsing hook array entries, with comprehensive comments explaining exact-match requirements and position independence.

4. **reapply_test.go**: Used origin/main's more comprehensive test cases covering various multi-entry hook array scenarios (pins, trailing newlines, embedded commands, etc.).

5. **windowsize.go**: Used origin/main's version (refactoring of watchdog signal hook infrastructure).

6. **windowsize_test.go**: Used origin/main's version with improved test naming and documentation.

All resolutions preserve the intent of the current branch (reed-pane-reap-consistency) by keeping the most comprehensive and well-structured versions of conflicting code. No functionality was lost - where one side was chosen, it was because it was more complete or better organized than the other.

{"status":"success"}
