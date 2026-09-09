# `loom` crucible review — round 1, tag `opus5-high-r1`

> Independent clean-room review + fix of the two behavior-preserving refactors
> (`centralize-glyph-shape-enum`, `quarry-bump-v0-2-0-status-helpers`) driven LIVE through the real
> built `cmd/lyx` binary. Per `_mill/loom-review-prompt.md`.
> Clean-room constraint honored: nothing under `_mill/loom-review-*` was opened before this file's
> own findings list was complete.

## Status

- Job 1 (review): IN PROGRESS — notes appended as each command/scenario returns.
- Job 2 (fix): not started.

## Environment

| Thing | Value |
| --- | --- |
| Host | Linux, `7.0.0-30-generic` |
| Go | `go1.26.0 linux/amd64` |
| `CGO_ENABLED` | `1` (gcc on PATH at `/usr/bin/gcc`) |
| `tmux` | present at `/usr/bin/tmux` — smoke tests will NOT skip-as-pass |
| quarry | `v0.2.0` (module cache `github.com/!knatte18/quarry@v0.2.0`) |
| Branch / HEAD | `crucible-loom-refshape-registry` @ `8503e22f3` |

## What was tested

### Hermetic baseline (before any edit)

| # | Command | Result |
| --- | --- | --- |
| H1 | `go build ./...` | exit 0, clean |
| H2 | `go vet ./internal/loomengine/... ./internal/loomcli/... ./internal/loomshed/... ./internal/planparser/... ./internal/planglyph/... ./internal/websterengine/... ./internal/webstercli/...` | exit 0, clean |
| H3 | `go test -count=5 ./internal/loomengine/... ./internal/loomcli/... ./internal/loomshed/... ./internal/planparser/... ./internal/planglyph/... ./internal/websterengine/... ./internal/webstercli/... ./cmd/lyx/...` | all 8 packages `ok`, exit 0 |

So the baseline is green: whatever this campaign finds, the unit suite does not see it. That is the
premise the campaign was created on and it holds.

### quarry v0.2.0 contract, read from source (not assumed)

`$(go env GOMODCACHE)/github.com/!knatte18/quarry@v0.2.0/internal/engine/answer.go`:

- `Status` is `string`; the closed vocabulary is `found` / `not_found` / `ambiguous` / `multipart`.
- `func (s Status) Known() bool` — `true` for exactly those four, `false` for `""` **and for any
  other non-empty value**.
- `func (r ResolveResult) Rejected() bool { return r.Status == "" }` — reads `Status`, deliberately
  NOT `Error != ""`, so a rejection with an empty message still reports `Rejected() == true`.

**The two predicates are therefore NOT complements.** `!Known()` is strictly weaker than `Rejected()`:

| `Status` | `Known()` | `Rejected()` |
| --- | --- | --- |
| `found`/`not_found`/`ambiguous`/`multipart` | true | false |
| `""` (pre-resolution rejection) | false | true |
| any other non-empty value (future vocabulary) | false | **false** |

Any call site that treats `!Known()` and `Rejected()` as the same predicate is wrong for the third
row. This is the exact seam the refactor introduced, so it is where I looked hardest.

(Sections below are appended as each scenario runs.)

## Findings

(Appended provisionally as formed; severity ordering finalized last.)
