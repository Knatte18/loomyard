All 3 cards in batch D2 -- doc repoint are committed (3 of 3), verify (`go build ./...`) passed, and the working tree is clean.

Summary of work:
- Card 19: `git rm manifest/designs/fabric.md`; moved the roadmap's fabric entry from Planned to a short Done entry in `/home/knatte/Code/loomyard/wts/fabric-cutover/manifest/roadmap.md` (commit `aabbcaac`).
- Card 20: repointed every inbound `fabric.md` link/mention to `internal/fabricengine/doc.go` across `manifest/designs/board-weft-storage.md`, `manifest/designs/raddle.md`, `manifest/designs/loom-finalize.md`, `manifest/designs/host-visibility.md`, `manifest/designs/codeintel-redesign.md`, `docs/reference/plan-format-v3.md`, `crucible/fabric-review-prompt.md`, `crucible/gitrepo-review-prompt.md` — verified every rewritten relative path resolves (commit `f728752f`).
- Card 21: rewrote `/home/knatte/Code/loomyard/wts/fabric-cutover/docs/overview.md` onto post-cutover fabric — dropped the separate warp/weft module-table rows and the package tree's warp/weft entries, removed fabric's parallel-build banner and its `fabric.md` link, and fixed intro prose/naming examples that still named warp/weft as live modules, while leaving the weft repo/role descriptions (fabric's weft sibling concept) untouched per the batch's Shared Decision (commit `65889a07`).

Files touched by this session are all noted above (absolute paths under `/home/knatte/Code/loomyard/wts/fabric-cutover/`). Out-of-scope files that also reference `fabric.md`/warp/weft (e.g. `internal/fabriccli/fabric.go`, `tools/sandbox/SANDBOX-FABRIC-SUITE.md`) were left untouched — those are explicitly owned by batch D3 (cards 22 and 24) per `_mill/plan/06-deparallel-and-gate.md`.

{"status":"success","commit_sha":"65889a07de6d22de901df0062344c1fe5b52739e","session_id":"a3cd5c05-c087-4f72-883c-3c077a480546","cards_done":[19,20,21]}
