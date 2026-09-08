All 3 cards (8, 9, 10) committed since batch start commit `23f3f13f6`. 3 of 3 cards committed. Final HEAD SHA: `38988c78a4e94cb468781e00dde141c38061a448`.

{"status":"success","commit_sha":"38988c78a4e94cb468781e00dde141c38061a448","session_id":"5e64eb22-9d7a-4e4f-81f0-17d2649de026","cards_done":[8,9,10]}

Card-count self-check: 3 of 3 cards committed (cards 8, 9, 10), all with real content commits matching this batch's declared `## Cards` entries. Verify passed both halves (`go test ./...` and the tagged integration run over cliwire/webstercli/burlercli/standalonegeom). Working tree is clean with no uncommitted tracked changes.

Relevant files:
- `/home/knatte/Code/loomyard/wts/unify-webster-burler-wiring/internal/cliwire/callerset_enforcement_test.go` (new)
- `/home/knatte/Code/loomyard/wts/unify-webster-burler-wiring/internal/cliwire/bannedecl_enforcement_test.go` (new)
- `/home/knatte/Code/loomyard/wts/unify-webster-burler-wiring/CONSTRAINTS.md` (edited)
- `/home/knatte/Code/loomyard/wts/unify-webster-burler-wiring/docs/overview.md` (edited)

One deviation from the plan text worth flagging for review: the plan's card 8 header comment draft said "three in `internal/burlercli/wiring_test.go`, one in `internal/webstercli/wiring_test.go`," but the tree as it stands after batch 2 actually has four and two respectively (ten total, not eight) — I verified this by grepping actual `:= standalonestate.Derive(` call sites before writing the header comment, per the card's own instruction to correct the count if batch 2's changes shifted it.

{"status":"success","commit_sha":"38988c78a4e94cb468781e00dde141c38061a448","session_id":"5e64eb22-9d7a-4e4f-81f0-17d2649de026","cards_done":[8,9,10]}
