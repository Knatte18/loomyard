No tracked modifications remain dirty. 1 of 1 card committed — this batch declares exactly 1 card (Card 12), and its commit matches the range-start scan. All complete.

{"status":"success","commit_sha":"567a74f80c2998457945599455cbde8b0b9aa7fa","session_id":"eb701b0f-4bc9-4b17-aec2-178dec2aa100","cards_done":[12]}

Summary: 1 of 1 card committed (Card 12 — the batch's only card). `verify: null` per the batch frontmatter, so no verify command was run.

Files touched:
- `/home/knatte/Code/loomyard/wts/fabric-commit-lock-coalescing/manifest/designs/fabric-unified-view.md` — marked slice 3 DONE in `## Build order` (pointing at `internal/fabricengine/doc.go` for the shipped combined write lock, `fabricengine.CoalescePush` primitive, and rebase-free async push), and resolved both slice-3 bullets in `## Open questions` (lock shape → single combined `.weft/weft.write.lock`; coalescing-loop home → `fabricengine.CoalescePush`), each citing the matching `_mill/discussion.md` decision heading.
- Left `docs/overview.md` and `manifest/roadmap.md` unedited per the card's explicit due-diligence instructions (no new module registered; this is a design-doc-tracked slice completion, not a roadmap-level item).

Commit `567a74f80c2998457945599455cbde8b0b9aa7fa` pushed to `origin/fabric-commit-lock-coalescing`.

{"status":"success","commit_sha":"567a74f80c2998457945599455cbde8b0b9aa7fa","session_id":"eb701b0f-4bc9-4b17-aec2-178dec2aa100","cards_done":[12]}