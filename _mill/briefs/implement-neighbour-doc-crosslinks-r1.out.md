All 3 of 3 cards committed (Card 4, 5, 6), matching the batch's declared card count. `go build ./...` passed. `git status --porcelain --untracked-files=no` shows no tracked in-scope modifications outstanding (only pre-existing untracked scratch files `_mill/adhoc-review-discussion.md` and `_mill/briefs/implement-neighbour-doc-crosslinks-r1.md`, which are out of this batch's scope).

One note for the record: the batch's own "Repo-wide no-dangling sweep" grep check (`grep -rn "](plan-format-v3.md)" docs/ manifest/` "returns nothing") is self-contradictory with Card 6's own requirement — the Shared Decision `link-paths-up-and-over` mandates exactly that same-directory bare-filename link form (`](plan-format-v3.md)`) from `docs/reference/*.md` siblings, which Card 6 requires me to add in `plan-format.md`, `builder-contract.md`, and `model-spec.md`. I followed the Shared Decision (authoritative) and Card 6's literal requirements verbatim; all three new links plus the earlier ones resolve to the real `docs/reference/plan-format-v3.md` file (verified with `test -e`), so nothing is actually dangling — this is a wording artifact in the batch's advisory grep check, not a functional problem, and the batch's real `verify:` (`go build ./...`) is unaffected and passes.

Relevant files:
- `/home/knatte/Code/loomyard/wts/plan-format-v3/docs/reference/plan-format.md` (Card 4)
- `/home/knatte/Code/loomyard/wts/plan-format-v3/manifest/roadmap.md` (Card 5)
- `/home/knatte/Code/loomyard/wts/plan-format-v3/docs/overview.md` (Card 6)
- `/home/knatte/Code/loomyard/wts/plan-format-v3/docs/reference/builder-contract.md` (Card 6)
- `/home/knatte/Code/loomyard/wts/plan-format-v3/docs/reference/model-spec.md` (Card 6)

{"status":"success","commit_sha":"5d82f3c75862fddfa44e1b35a676937047a2c518","session_id":"da4a8b47-8a7e-4754-a538-b27423261ca4"}
