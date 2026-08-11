{"status":"success","commit_sha":"1f05900faed558fb1fdd1db3c21c97a154995812","session_id":"0c1a5c76-fbe9-43c7-b155-124ab6538167","cards_done":[8,9,10,11,12]}

Summary: 5 of 5 cards committed (cards 8-12), all in this batch. verify (`go test ./internal/lyxcwd/ -run TestEnforcement_MarkdownLinks`) passed. All three acceptance greps confirmed by hand: zero `Plan-Review-Gate` outside `_mill/`, `Discussion-Review-Gate` surviving only in `manifest/designs/shed-followups.md` outside `_mill/` (plus the new override note), and `loom.md` rows 2-8 clean of `discussion.md`/`plan.md` artifact references (row 9's stale `plan.md` intentionally kept per scope).

Files touched:
- `/home/knatte/Code/loomyard/wts/format-docs-name-producers/manifest/designs/loom.md` (producer table insert/renumber/rewrite; line-75 open-question repair)
- `/home/knatte/Code/loomyard/wts/format-docs-name-producers/manifest/designs/shed.md` (`:13`, `:41` rename + insert)
- `/home/knatte/Code/loomyard/wts/format-docs-name-producers/manifest/roadmap.md` (`:45`, `:46`, `:47` rename sweep)
- `/home/knatte/Code/loomyard/wts/format-docs-name-producers/manifest/designs/shed-followups.md` (`:304`, `:306` rename + override note under section C)

{"status":"success","commit_sha":"1f05900faed558fb1fdd1db3c21c97a154995812","session_id":"0c1a5c76-fbe9-43c7-b155-124ab6538167","cards_done":[8,9,10,11,12]}
