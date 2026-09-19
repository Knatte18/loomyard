Both conflicts resolved by combining each side's non-overlapping edit; nothing was dropped.

CONSTRAINTS.md — CLI/Cobra Invariant, two separate bullets conflicted as one hunk. Verified against the merge base (abbd55be1): our branch never touched the interactive-handoff bullet (parent renamed header --blocking -> watchdog), and the parent never touched the package-naming bullet (our branch added the lifecyclecli deviation). Resolution keeps the parent's attach/watchdog wording and our lifecyclecli -> internal/lifecycleshed, internal/lifecyclerecipe deviation. The separately-clean line 140 ("twelve of thirteen") is unaffected -- the parent left that count at "eleven of twelve", so our amendment stands unopposed. No stale header --blocking survives anywhere in the file.

manifest/roadmap.md -- the Next Up orphan-reaper item. Base carried both "(see the Planned header-pane split)" and "the Planned worktree spawn/teardown as Shed producers item". The parent deleted the parenthetical (the header-pane split now sits under ## Done, lines 130/133) and dropped the bare "Planned" qualifier; our branch changed that qualifier to "Done" (the item is under ## Done at line 116). Resolution takes the parent's deletion and our "Done" qualifier.

Marker verification: grepped both working-tree files and both staged blobs for conflict markers -- zero matches in all four. Re-read both files in full; no contradictory losing-side claims remain. Both staged; git status reports no unmerged paths. Not committed, no git merge --continue run.

One unrelated observation, left untouched because it merged cleanly outside any conflict region: the parent-side commit removed the blank line between the last Next Up item and the ## Someday heading (manifest/roadmap.md:31-32). It is cosmetic -- an ATX heading still starts a new list block -- but it is a deviation from the file's own spacing convention that an operator may want to fix separately.

{"status":"success"}
