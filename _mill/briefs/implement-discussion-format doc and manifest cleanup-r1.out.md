Summary: all 6 cards of batch 1 (`_mill/plan/01-discussion-format-doc-and-manifest-cleanup.md`) are complete — 5 of 6 cards produced their own commit each (cards 1–5), and card 6 was a zero-diff verification-only card (`Commit: none`), whose acceptance greps and `verify:` I ran and confirmed pass.

Files touched:
- `/home/knatte/Code/loomyard/wts/loom-redesign-discussion-format/manifest/designs/loom-format-discussion.md` (new)
- `/home/knatte/Code/loomyard/wts/loom-redesign-discussion-format/manifest/designs/loom.md`
- `/home/knatte/Code/loomyard/wts/loom-redesign-discussion-format/manifest/designs/plan-card-format.md`
- `/home/knatte/Code/loomyard/wts/loom-redesign-discussion-format/manifest/designs/review-finding-classification.md`
- `/home/knatte/Code/loomyard/wts/loom-redesign-discussion-format/manifest/roadmap.md`

`go test ./internal/lyxcwd/ -run TestEnforcement_MarkdownLinks` passes, and card 6's three acceptance greps confirm: (1) no remaining outright-supersession claim on the discussion stencil in `plan-card-format.md`/`roadmap.md`, both reworded sentences read grammatically, and the new doc's scoped Step 2/Step 3 claim appears; (2) the roadmap item appears exactly once, under `## Done`; (3) `loom.md` carries both `### Discussion-Review rubric` subsections in order.

Working tree is clean (`git status --porcelain --untracked-files=no` reports nothing) aside from the pre-existing untracked brief file, which is out of scope.

{"status":"success","commit_sha":"6ea7f5b5fb226cc4b8fd739f912341280ac5113c","session_id":"6c2fdc77-e431-48eb-8d44-2ebd7597ba9a","cards_done":[1,2,3,4,5,6]}
