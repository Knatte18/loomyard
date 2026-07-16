41 chars including newline = 40-char SHA, confirmed as `957ad2249a6f446ef01346f6391447dce44dc9cc`. This matches the earlier commit output (`957ad224...`). Good, this is the correct HEAD SHA.

{"status":"success","commit_sha":"957ad2249a6f446ef01346f6391447dce44dc9cc","session_id":"0c1ecebb-7f0b-4a79-9f0d-faa85b1d4146"}

Summary (2 of 2 cards committed, verified via `git log 15aa5de8..HEAD --oneline` matching both cards' `Commit:` messages):

- **Card 1** — `e6d827ce`: Added `Layout.Repo string` field (godoc'd) to `/home/knatte/Code/loomyard/wts/mux-operator-console/internal/hubgeometry/hubgeometry.go`, plus the pure `deriveRepo(prime, worktreeRoot string) string` helper, wired into `Resolve`'s returned `&Layout{...}` literal with no new git spawn.
- **Card 2** — `957ad224`: Added `TestDeriveRepo` table test in `/home/knatte/Code/loomyard/wts/mux-operator-console/internal/hubgeometry/hubgeometry_unit_test.go` (had to switch that file's package from `hubgeometry_test` to internal `hubgeometry` so it can call the unexported `deriveRepo` directly per the card's "do NOT call Resolve" requirement — the rest of that file's tests were de-qualified to match); extended `TestResolve_FromWorktreeRoot` in `/home/knatte/Code/loomyard/wts/mux-operator-console/internal/hubgeometry/hubgeometry_test.go` to assert `layout.Repo == filepath.Base(layout.Prime)`.
- **In-scope ripple fix** (same file already in Card 1's `Edits:` list, no plan edit needed): `Layout.SiblingLayout` in `hubgeometry.go` now propagates `Repo: l.Repo` — discovered because the pre-existing `TestSiblingLayout_EquivalentToResolve` (`internal/hubgeometry/siblinglayout_test.go`, `-tags integration`) does a `reflect.DeepEqual` against `Resolve`'s output and failed once `Repo` existed but wasn't mirrored.

Verify (`go test ./internal/hubgeometry/...`) passes, and I additionally confirmed `go build ./...`, `go vet ./internal/hubgeometry/...`, and `go test -tags integration ./internal/hubgeometry/...` are all clean. `gofmt`/`goimports` are clean on every file I touched (one pre-existing `gofmt` finding in untouched `weft_test.go` predates this batch and is out of scope). Working tree is clean and both commits are pushed.

{"status":"success","commit_sha":"957ad2249a6f446ef01346f6391447dce44dc9cc","session_id":"0c1ecebb-7f0b-4a79-9f0d-faa85b1d4146"}