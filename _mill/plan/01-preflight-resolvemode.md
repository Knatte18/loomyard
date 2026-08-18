# Batch: preflight-resolvemode

```yaml
task: "the standalone CLI path"
batch: "preflight-resolvemode"
number: 1
cards: 3
verify: go test ./internal/preflight/... && go test -tags integration ./internal/preflight/...
depends-on: []
```

## Batch Scope

This batch adds the single mode trigger all three standalone-capable CLIs consume: `preflight.ResolveMode`, plus the `Mode` type it returns.
It is one batch because the function, its package doc, and its seven-row integration table are one indivisible contract — the two bold rows of that table (a plain repo's subdirectory versus a wired worktree's subdirectory, which both arrive as the same `ErrCwdOutsideAnchor` sentinel and must diverge) are the entire reason the function exists, so shipping the code without the table would ship the bug the design's r4 review caught.
The change is strictly additive: `Wired` and `HubPresent` keep their exact current bodies and signatures, and their existing consumers (`cmd/lyx`'s stencil-seed gate, composing orchestrators) are untouched.

**External interface batches 3, 4 and 5 consume:** `preflight.ResolveMode(cwd string) (*lyxcwd.Location, preflight.Mode, error)` and the exported constants `preflight.ModeHub` / `preflight.ModeStandalone`.
A non-nil error means refuse; the zero `Mode` value is never a valid mode and appears only alongside that error.

## Cards

### Card 1: add the `Mode` type and `ResolveMode` to `internal/preflight`

- **Context:**
  - `internal/lyxcwd/lyxcwd.go`
  - `internal/lyxcwd/anchor.go`
  - `internal/fabricengine/junctionnames.go`
  - `internal/lyxdirs/dirs.go`
- **Edits:**
  - `internal/preflight/predicates.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add to `internal/preflight/predicates.go`, alongside the existing `Wired` and `HubPresent`:
  a named integer type `Mode`, with exported constants `ModeHub` and `ModeStandalone`.
  Neither constant may be the zero value: declare them via `const ( ModeHub Mode = iota + 1; ModeStandalone )` so the zero `Mode` is never a valid mode and can only appear in the refuse return.
  Document on the type that the zero value is not a mode and is returned only alongside a non-nil error.

  Extract the board-level lyx stat that `HubPresent` performs today into an unexported package-level helper
  `func boardLyxPresent(l *lyxcwd.Location) bool`, whose body is the existing
  `_, err := os.Stat(filepath.Join(fabricengine.BoardDir(l.HubPath), lyxdirs.LyxDirName)); return err == nil`.
  Rewrite `HubPresent`'s body to call it. `HubPresent`'s signature, doc comment, and observable behaviour must be unchanged — this is a body-level extraction so the predicate has exactly one implementation shared with `ResolveMode`, not a second copy.

  Add `func ResolveMode(cwd string) (*lyxcwd.Location, Mode, error)` implementing this behaviour in this order:
  1. Call `lyxcwd.Resolve(cwd)`. On success: return `(l, ModeHub, nil)` when `boardLyxPresent(l)`, otherwise `(nil, ModeStandalone, nil)`.
     The standalone return deliberately discards the resolved `*lyxcwd.Location` — a standalone session must never be handed a Location, fictional or otherwise.
  2. On `errors.Is(err, lyxcwd.ErrNotAGitRepo)`: return `(nil, ModeStandalone, nil)`.
  3. On `errors.Is(err, lyxcwd.ErrCwdOutsideAnchor)`: call `lyxcwd.ResolveWorktree(cwd)`, the ungated sibling that runs the same `git rev-parse` and anchor read with no cwd gate.
     If that call succeeds **and** `boardLyxPresent` is true for its returned Location, return `(nil, 0, err)` — the **original gated error** from step 1, never the second probe's error and never a newly constructed one, so the operator still gets the message naming both paths and the marker file.
     In every other case (the second probe errors, or it succeeds with no board-level lyx directory) return `(nil, ModeStandalone, nil)`.
  4. On any other error — including `lyxcwd.ErrStaleAnchorMarker` and an `ErrInvalidAnchor`-wrapping failure — return `(nil, 0, err)`, surfacing it verbatim.

  Give `ResolveMode` a doc comment stating: that the discriminator is whether lyx geometry exists at this location, never the error class alone; that a non-nil error means refuse and is never degraded to standalone; that the extra `git rev-parse` cost is paid on the `ErrCwdOutsideAnchor` path only, never by the hub path or the not-a-repository path; and that the residual class is a hub damaged precisely at `<hub>/_board/_lyx`, which degrades to standalone because at that point nothing distinguishes it from a plain repo.
  Do not modify `Wired`.
- **Commit:** `feat(preflight): add ResolveMode, the three-way hub/standalone/refuse resolver`

### Card 2: repoint `internal/preflight`'s package doc onto the three-way resolver

- **Context:**
  - `internal/preflight/predicates.go`
- **Edits:**
  - `internal/preflight/doc.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  The `# Why there are two predicates` section currently states that `HubPresent` "is both what cmd/lyx's stencil seed gates on and the mode-selection trigger a standalone-capable CLI's pre-run consults".
  That second half is now false: the CLIs consult `ResolveMode`.

  Rewrite the section so it describes three functions rather than two, preserving every existing rationale sentence about why `Wired` is not the trigger (the `fabricengine.Ready` paired-sibling argument and the three healthy hub situations) verbatim in substance — that argument is unchanged and is what keeps the override documented as considered rather than accidental.
  Add to it: `HubPresent` remains the stencil-seed gate for `cmd/lyx`, whose never-block-a-command contract is genuinely different from a CLI choosing how to wire itself; `ResolveMode` is the mode-selection trigger every standalone-capable CLI consumes, and it exists because `HubPresent` alone collapses two very different negatives — "there is no hub here" and "`Resolve` could not answer" — and `lyxcwd.Resolve` returns `ErrCwdOutsideAnchor` from any ordinary subdirectory of a healthy wired worktree, so a `HubPresent`-only trigger would silently start a standalone session there and relocate a live hub's state into the per-OS state directory.

  Rename the section heading from `# Why there are two predicates` to a heading naming three, and update the package doc's opening paragraph where it says "plus the two cheap predicates a standalone-capable CLI's pre-run consults before every command" so it no longer undercounts.
  Follow the repo's semantic-line-break convention: one sentence per line, no fixed-column hard wrap.
- **Commit:** `docs(preflight): document ResolveMode alongside the two existing predicates`

### Card 3: pin `ResolveMode`'s seven-row table in the integration suite

- **Context:**
  - `internal/preflight/predicates.go`
  - `internal/preflight/testmain_test.go`
  - `internal/hubforge/hub.go`
  - `internal/gitkit/gitkit.go`
  - `internal/lyxcwd/anchor.go`
  - `internal/fabricengine/junctionnames.go`
- **Edits:**
  - `internal/preflight/preflight_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add a test function `TestResolveMode` to the existing `package preflight_test` file, following the file's established fixture style (`setupFixture`, `hubforge.NewHub`).
  It spawns git, so it belongs in this already-`//go:build integration`-tagged file and nowhere else.

  Pin all seven rows explicitly — never by inference from a neighbouring row:

  | Situation | Expected |
  |---|---|
  | cwd is not a git repository at all | `ModeStandalone`, nil error, nil Location |
  | a plain git repo, at its root | `ModeStandalone`, nil error, nil Location |
  | a plain git repo, in a subdirectory | `ModeStandalone`, nil error, nil Location |
  | a wired hub worktree, at the anchor | `ModeHub`, nil error, non-nil Location |
  | a wired hub worktree, in a subdirectory | non-nil error, `Mode` zero value, nil Location |
  | a stale anchor marker | non-nil error, `Mode` zero value, nil Location |
  | a hub whose `<hub>/_board/_lyx` is missing | `ModeStandalone`, nil error, nil Location |

  Rows three and five are the pair the design's r4 review exposed: both arrive as `lyxcwd.ErrCwdOutsideAnchor` from `lyxcwd.Resolve` and must diverge.
  State that in a comment on each of the two, so a later reader cannot collapse them.
  For row five, assert `errors.Is(err, lyxcwd.ErrCwdOutsideAnchor)` **and** that the returned error is the gated one by asserting its message names the anchor marker file — the whole point of returning the original error rather than the second probe's.
  For row six, assert `errors.Is(err, lyxcwd.ErrStaleAnchorMarker)`.
  Build row seven by removing (or never creating) the `<hub>/_board/_lyx` directory under an otherwise-real hub fixture, and add a comment naming it as the design's recorded residual rather than a bug.
  For the plain-repo rows, create a bare `git init` repository via the existing `gitkit` fixture helpers the file already imports — never a `hubforge` hub, which would supply the board directory these rows must not have.

  Every standalone and refuse row must additionally assert the returned `*lyxcwd.Location` is nil, since handing a caller a Location outside hub mode is the fictional-`Location` shape the whole design rejects.
  Do not add a `TestMain` — `internal/preflight/testmain_test.go` already provides the hermetic git environment and is untagged, so it compiles into both test binaries.
- **Commit:** `test(preflight): pin ResolveMode's seven-row hub/standalone/refuse table`

## Batch Tests

`verify:` runs `go test ./internal/preflight/...` followed by `go test -tags integration ./internal/preflight/...`.
The second invocation is required, not decorative: `internal/preflight/preflight_integration_test.go` carries `//go:build integration`, so the untagged run does not compile card 3's new table at all and would report a passing suite that never executed the batch's only new test.
The untagged run still matters — it compiles `predicates.go` against `report_test.go` and catches a signature or import break in card 1 without paying for git fixtures.

Card 1's `boardLyxPresent` extraction changes `HubPresent`'s body while pinning its behaviour; the existing `TestHubPresent`-family rows in the integration file are the regression coverage for that extraction and must keep passing unchanged.
