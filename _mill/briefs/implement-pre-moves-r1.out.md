All 4 cards of the `pre-moves` batch are complete and committed:

1. **Card 1** (`26e9e89c`) — extracted `internal/weftname` as a stdlib-only leaf owning the `-weft` naming convention; converted the geometry-literal enforcement guard from a single allowlisted directory to a per-token ownership map.
2. **Card 2** (`e55c9e82`) — moved `LyxDirName`/`ConfigDir`/`ConfigFile` to `internal/configengine` and `DotEnv` to `internal/envsource`; ~50 call sites retargeted; `hubgeometry` keeps a private transitional `lyxDirName` const.
3. **Card 3** (`aa6da988`) — moved `worktreelist.go` (`List`/`WorktreeEntry`) via `git mv` into `internal/fabricengine`, added `fabricengine.PrimeName`/`WeftRepoRoot`, deleted `Layout.Prime`/`PrimeName()`/`WeftRepoRoot()`/`deriveRepo` from `hubgeometry`, and switched `Repo` derivation to the `-HUB`-trim form (no more per-`Resolve` subprocess).
4. **Card 4** (`f78d47d7`) — deleted `(*Layout).WorktreePath(slug)` (replaced by `fabricengine.WorktreePath(l, slug)`) and `(*Layout).SiblingLayout` (inlined into `fabricengine`'s `hostLayoutFor`), removing `internal/hubgeometry/siblinglayout_test.go`.

Three additional `plan:` commits extend the batch file's `Edits:` lists for compile-fallout files the original plan's file enumeration missed (`internal/hubgeometry/{discussionpath,loomstatus,planpath,webstergeom}_test.go`, `siblinglayout_test.go`, `junction_test.go`), per the brief's stop-and-extend-plan protocol.

Final state: `go vet -tags "integration smoke scout" ./...`, `go test ./...`, and `go test -tags integration ./...` all pass repo-wide; `git status --porcelain --untracked-files=no` is clean.

Relevant files: `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/weftname/`, `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/configengine/config.go`, `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/envsource/envsource.go`, `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/hubgeometry/hubgeometry.go`, `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/{worktreelist.go,junction.go,hostlayout.go}`, `/home/knatte/Code/loomyard/wts/fabric-illusion-core/_mill/plan/01-pre-moves.md`.

{"status":"success","commit_sha":"f78d47d7584f056fdca89ad723641aad63bda7d6","session_id":"b1d5d8e9-6bd3-40ae-8a6a-d915176ec397","cards_done":[1,2,3,4]}