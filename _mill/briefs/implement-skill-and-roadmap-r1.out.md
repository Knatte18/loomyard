Both cards (8 and 9) have matching commits in the log — 2 of 2 cards committed, matching the batch's declared 2 cards. Verify passed on the first run with no self-fix needed.

{"status":"success","commit_sha":"feee04607c41008d28e0e18880fb81af3094cea4","session_id":"137a6886-35c6-447e-9b38-53be47b6af9c","cards_done":[8,9]}

Summary: 2 of 2 cards committed (Card 8: `plugins/ly/skills/ly-supervise/SKILL.md` Preconditions rewrite; Card 9: `manifest/roadmap.md` moved the ly-supervise item to Done and reworded the three Someday cross-references). `manifest/designs/reed-header-selvage.md` needed no change since its `lyx reed add` example already reads correctly. Verify (`go test ./internal/lyxcwd/ -run TestEnforcement_MarkdownLinks`) passed on the first run. Working tree is clean.

{"status":"success","commit_sha":"feee04607c41008d28e0e18880fb81af3094cea4","session_id":"137a6886-35c6-447e-9b38-53be47b6af9c","cards_done":[8,9]}
