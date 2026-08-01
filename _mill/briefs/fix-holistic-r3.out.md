## Summary

Both round-3 review findings (NITs) were verified as accurate and fixed under the mill-receiving-review protocol:

1. **`internal/fabricengine/weftwiring.go:129-133`** — `removeHostJunction`'s doc comment still said `Remove` sources names from "the removed slug's own weft base"; reworded to "the repo-wide `BoardDir` base" to match `remove.go`'s current comment (batch 2's `RepoWiredNames` migration). Committed as `acad66fa`.

2. **`internal/fabricengine/weftgit_pathspec_integration_test.go:108`** — `TestCommitWeft_IndexOnlyDeletionCountsAsMatch`'s doc comment referenced the deleted `internal/initengine/undo.go`/`lyx init --undo`. This file wasn't in any batch plan's Edits list, so per brief rule 6 it was first added to batch 06's Card 28 Edits list and requirements (`_mill/plan/06-init-dissolution-and-unwire.md`), committed the plan edit (`8ecd4abf`), then reworded the comment to reference `fabricengine.Unwire`'s `_lyx` clear-and-commit step. Committed as `33608625`.

All 7 batch `verify:` commands (hubgeometry, fabricengine, configsync, fabricengine+fabriccli, fabricengine, fabricengine+fabriccli+cmd/lyx+loomengine, cmd/lyx) passed with exit code 0. No tracked uncommitted changes remain. HEAD advanced from baseline `1eeca3cfa0a744478a59b9dae7c5a5d2a62ec6b7` to `336086253d9b6cf01798ed92f764151cdcbea498`.

Files touched:
- `/home/knatte/Code/loomyard/wts/fabric-clone-subpath/internal/fabricengine/weftwiring.go`
- `/home/knatte/Code/loomyard/wts/fabric-clone-subpath/internal/fabricengine/weftgit_pathspec_integration_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-clone-subpath/_mill/plan/06-init-dissolution-and-unwire.md`

{"status":"success","commit_sha":"336086253d9b6cf01798ed92f764151cdcbea498","session_id":"0ff50dd2-7d55-4bc9-8ba4-3601e31ebc1e"}
