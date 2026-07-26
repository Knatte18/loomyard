All 6 cards in batch D3 are complete and committed. Summary of work:

**Card 22** (`b6aa52b8`) — Rewrote `internal/fabriccli/fabric.go`'s package comment, `Short`/`Long` help text, and the `cleanup` verb's weft-branch note to drop parallel-build/warp-weft-coexistence framing; dropped the deleted `manifest/designs/fabric.md` link.

**Card 23** (`dde7fc73`) — Swept every `//` comment in `internal/fabricengine/` and `internal/fabriccli/` (36 files, including test-file provenance comments) naming a deleted module. Discovered two files not in the batch's declared scope mid-card (`internal/fabricengine/clone_adopt_test.go`, `internal/fabricengine/reconcile_stale_registration_test.go`) and followed the STOP protocol: extended `_mill/plan/06-deparallel-and-gate.md`'s card 23 Edits list via dedicated plan-edit commits (`2483bb53`, `16b0829d`) before editing them.

**Card 24** (`55714d5c`) — Reframed `tools/sandbox/main.go` and `tools/sandbox/SANDBOX-FABRIC-SUITE.md` to describe the dedicated fabric hub as hosting fabric's stricter branch-naming suite rather than isolating from warp/weft; fixed a stale "`lyx init` is still warp/weft-based" precondition note (init now wires junctions via fabricengine).

**Card 25** (`0e450cd0`) — Repointed stale `internal/warpengine`/`internal/weftcli` full-path references in `internal/lyxtest/doc.go`, `internal/hubgeometry/hubgeometry.go`, `internal/hubgeometry/siblinglayout_test.go`, `internal/codeintelcli/cli.go`.

**Card 26** (`d9455fcf`) — Repointed bare `warpengine`/`weftengine` mentions in `internal/perchengine/{doc,engine}.go`, `internal/reedengine/config{,_test}.go`, `internal/lyxtest/hermetic.go`, `internal/websterengine/audit.go` to fabricengine, leaving the `weftRef` regex/`ClassWeftReference` concept untouched as instructed.

**Card 27** (`efae2aa9`) — Ran the final acceptance grep-clean gate. Tier 1 was clean immediately. Tier 2/2b's whole-tree grep surfaced four files an earlier, already-committed batch had missed (`internal/buildercli/weft.go`, `internal/webstercli/weft.go`, `internal/perchcli/run.go` — a stale "mirroring weftengine.Commit's" provenance comment from batch A; `cmd/lyx/registration_test.go` — dead `"warpcli"/"weftcli"` allowlist entries batch C added as a temporary bridge that batch D1 never removed). Followed the STOP protocol again (plan-edit commit `a27f10a8`) to bring these into card 27's scope before fixing them. Re-ran all gates to a fully clean state, then ran `go build ./... && go test ./... -tags integration` — full suite green.

Card-count self-check: 9 commits since batch start (`8b6d23b1..HEAD`), 6 of which match the batch's declared `Commit:` messages for cards 22–27 (all 6 of 6 declared cards), plus 3 plan-extension commits for STOP-protocol scope additions. 6 of 6 cards complete — this is a genuine "all complete" claim, verified by matching commit subjects.

Working tree is clean (only the untracked brief file remains, which is expected and not an in-scope modification).

{"status":"success","commit_sha":"efae2aa9afa5ead39a34ac1dbd67d35218a0fe81","session_id":"d67a4c77-8c38-4e54-ae64-a87f274dcecd","cards_done":[22,23,24,25,26,27]}
