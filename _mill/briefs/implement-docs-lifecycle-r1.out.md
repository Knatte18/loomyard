One commit matching Card 14's exact `Commit:` message. This batch has only 1 card, and it is fully committed — verify (`go test ./...`) passes, and the working tree is clean. 1 of 1 cards committed — all complete.

{"status":"success","commit_sha":"357b853c","session_id":"7b1b89e9-e1af-41f4-b7f3-614e24e4ac54","cards_done":[14]}

Summary of work done:

- `/home/knatte/Code/loomyard/wts/treadle/internal/treadleengine/doc.go` — added "Judge-maintained handoff" and "Pre-round targeting" package-doc sections absorbing the relevant rationale from `manifest/designs/treadle.md` before its deletion.
- `/home/knatte/Code/loomyard/wts/treadle/internal/perchengine/doc.go` — final coherence pass noting perch's Profile deliberately never sets `PreRoundTargeting`.
- `/home/knatte/Code/loomyard/wts/treadle/docs/overview.md` — added `internal/treadleengine/` to the package tree, updated the `perch` module bullet and execution-stack `builds-on` note, added an `## Other docs` entry.
- `/home/knatte/Code/loomyard/wts/treadle/manifest/roadmap.md` — moved the Treadle item from Planned to Done (linking the `internal/treadleengine` package documentation), fixed the now-stale "Treadle item above" cross-reference in the Shed entry.
- `/home/knatte/Code/loomyard/wts/treadle/manifest/designs/shed.md` and `/home/knatte/Code/loomyard/wts/treadle/manifest/designs/hardener.md` — retargeted all 8 `treadle.md` links (including the anchored one in shed.md, reworded to stand alone without the anchor) to point at the `internal/treadleengine` package documentation.
- Deleted `/home/knatte/Code/loomyard/wts/treadle/manifest/designs/treadle.md` (`git rm`).

All changes committed as a single commit `357b853c` (`docs: absorb treadle design into package docs and close the roadmap item`), pushed to `origin/treadle`. `go test ./...` passes across the whole module (the batch's `verify:` command). Working tree is clean (only the untracked, out-of-scope brief file `_mill/briefs/implement-docs-lifecycle-r1.md` remains, which is mill-go's own artifact, not part of this batch's declared scope).

{"status":"success","commit_sha":"357b853c","session_id":"7b1b89e9-e1af-41f4-b7f3-614e24e4ac54","cards_done":[14]}
