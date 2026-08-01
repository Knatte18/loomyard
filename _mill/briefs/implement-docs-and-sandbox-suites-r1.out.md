All 5 cards (32, 33, 34, 35, 36) committed and match their `Commit:` messages exactly — 5 of 5 cards complete. Verify (`go test ./cmd/lyx/...`) passed. Working tree is clean.

{"status":"success","commit_sha":"9f947322ae519edc08fb43d9ea5bb3a866a6d7cf","session_id":"dc44d257-da15-48ea-a223-0912cbf9b11e","cards_done":[32,33,34,35,36]}

Summary of work (all file paths absolute):

- `/home/knatte/Code/loomyard/wts/fabric-clone-subpath/internal/fabricengine/doc.go` — added a slice-5 documentation paragraph block and reworded 4 stale `lyx init` mentions.
- `/home/knatte/Code/loomyard/wts/fabric-clone-subpath/docs/overview.md` — removed the `init` module entry/summary, swept the `lyx init` cross-reference in the loom bullet, rewrote the Junction model section for clone-does-everything + repo-wide pathspec.
- `/home/knatte/Code/loomyard/wts/fabric-clone-subpath/manifest/designs/fabric-unified-view.md` — marked slice 5 DONE (kept original text for history), resolved the RelPath open question; file not deleted (slice 6 remains).
- `/home/knatte/Code/loomyard/wts/fabric-clone-subpath/tools/sandbox/SANDBOX-SHUTTLE-SUITE.md`, `SANDBOX-WEBSTER-SUITE.md`, `SANDBOX-PERCH-SUITE.md`, `SANDBOX-BURLER-SUITE.md`, `SANDBOX-BUILDER-SUITE.md` — rewrote the "`lyx init` first" precondition to "Wired worktree required" (clone/add wire eagerly).
- `/home/knatte/Code/loomyard/wts/fabric-clone-subpath/tools/sandbox/SANDBOX-FABRIC-SUITE.md` — reframed precondition 4 around `lyx fabric clone`, updated F0's verb list to 15 (added `unwire`), added new scenario F5 (junction deactivation via `lyx fabric unwire`), and updated the ref-field doc and session-log template for the F0-F5 range.

`go test ./cmd/lyx/...` passes; working tree clean; all 5 commits pushed to `fabric-clone-subpath`.
