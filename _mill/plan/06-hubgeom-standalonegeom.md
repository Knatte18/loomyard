# Batch: hubgeom.WebsterGeometry and the standalonegeom sibling

```yaml
task: websterengine + webstercli told-geometry, and Webster standalone entry
batch: hubgeom.WebsterGeometry and the standalonegeom sibling
number: 6
cards: 5
verify: go test ./internal/hubgeom/... ./internal/standalonegeom/...
depends-on: [1, 4, 5]
```

## Batch Scope

This batch delivers both tellers of `websterengine.Geometry` plus the told-mode `reedengine.Geometry` builder standalone webster needs in order to spawn anything at all.
`internal/hubgeom` gains `WebsterGeometry(l *lyxcwd.Location) websterengine.Geometry`;
the new package `internal/standalonegeom` is hubgeom's told-mode sibling, exporting `ReedGeometry` and `WebsterGeometry` builders that are told `(target, stateDir, hash8 string)` and resolve nothing themselves.
The builders never call `standalonestate.Derive`: `Derive` reads live `XDG_STATE_HOME`/`HOME`/`LOCALAPPDATA`, so a builder that called it would resolve `<state>` to the operator's real home directory in every test that touched it.
`Derive` is instead called exactly once, at the CLI argument boundary in batch 8, and its two outputs are passed down — which is what makes this package pure path math with no environment dependency.
The external interface batch 8 consumes is these three builders.

## Cards

### Card 17: Add `hubgeom.WebsterGeometry`

- **Context:**
  - `internal/websterengine/geometry.go`
  - `internal/websterengine/state.go`
  - `internal/planparser/parse.go`
  - `internal/fabricengine/junctionnames.go`
  - `internal/hubgeom/hubgeom.go`
- **Edits:**
  - `internal/hubgeom/doc.go`
- **Creates:**
  - `internal/hubgeom/webstergeom.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/hubgeom/webstergeom.go` exporting `WebsterGeometry(l *lyxcwd.Location) websterengine.Geometry`, populating every field by reading what `l`'s caller already resolved and performing no cwd resolution of its own, exactly as `ReedGeometry` does:
  `AnchorRoot` and `WorktreeRoot` both from `l.AnchorPath()`;
  `WebsterDir` from `websterengine.Dir(l.AnchorPath())`;
  `ReportsDir`, `ScratchDir` and `PromptsDir` from the matching accessors over the same value;
  `StencilsDir` from `fabricengine.StencilsDir(l.HubPath)`;
  `PlanDir` from `planparser.PlanDir(l.AnchorPath())`.
  `WorktreeRoot` is `l.AnchorPath()` and **not** `l.WorktreePath()`: every one of webster's CLI call sites passes the anchor path today, and converging on the worktree path would silently change behaviour in a subpath-anchored hub.
  State that in the function's doc comment, since it is the opposite of the neighbouring `ReedGeometry`'s own `WorktreeRoot`.
  In `internal/hubgeom/doc.go`, update the sentence listing hubgeom's contract as "ReedGeometry" alone, and the sentence promising `WebsterGeometry` as a later wave's addition, so both describe what the package now exports.
  Leave the closing sentence stating that standalone CLIs do not call `hubgeom` exactly as it is — card 19 is what makes it durable rather than aspirational.
- **Commit:** `feat(hubgeom): add WebsterGeometry, the hub-mode teller of websterengine.Geometry`

### Card 18: Pin `hubgeom.WebsterGeometry`

- **Context:**
  - `internal/hubgeom/webstergeom.go`
  - `internal/websterengine/state.go`
  - `internal/websterengine/geometry.go`
  - `internal/hubgeom/hubgeom_test.go`
- **Edits:** none
- **Creates:**
  - `internal/hubgeom/webstergeom_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/hubgeom/webstergeom_test.go` with a table test asserting `WebsterGeometry(l)` returns exactly the values the four `websterengine` accessors, `planparser.PlanDir` and `fabricengine.StencilsDir` produce for the same `l`, at two rows: an unanchored Location and a nested-`AnchorRel` one.
  Follow the fixture discipline the existing `hubgeom_test.go` already documents — keep hub, worktree root and anchor path three distinct directories so a field mix-up surfaces instead of passing silently.
  The nested row must additionally assert `WorktreeRoot == l.AnchorPath()` and `WorktreeRoot != l.WorktreePath()`, which is the assertion that catches a later "consistency fix" converging webster's field on reed's.
- **Commit:** `test(hubgeom): pin WebsterGeometry at both anchoring shapes`

### Card 19: Create `internal/standalonegeom` and its reed builder

- **Context:**
  - `internal/reedengine/geometry.go`
  - `internal/reedengine/name.go`
  - `internal/hubgeom/hubgeom.go`
  - `internal/hubgeom/doc.go`
  - `internal/standalonestate/standalonestate.go`
  - `internal/standalonestate/doc.go`
- **Edits:** none
- **Creates:**
  - `internal/standalonegeom/doc.go`
  - `internal/standalonegeom/reedgeom.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create package `standalonegeom` at `internal/standalonegeom`, the told-mode sibling of `internal/hubgeom`.
  `internal/standalonegeom/doc.go` states the package contract: it builds engine geometry structs from told strings alone, it never resolves cwd, it never reads the environment, and it never calls `standalonestate.Derive` — the caller derives `<state>` and `hash8` once at the CLI argument boundary and passes them in, which is what keeps this package hermetic by construction rather than by each test remembering to redirect `XDG_STATE_HOME`.
  Note that it is deliberately not a leaf package: it imports `reedengine` and `websterengine`, and it must not be added to `internal/buildinfo`'s or `internal/standalonestate`'s leaf-enforcement allowlists.
  `internal/standalonegeom/reedgeom.go` exports `ReedGeometry(target, stateDir, hash8 string) reedengine.Geometry`, where `target` is the already-absolute standalone target directory.
  The pinned field values are:
  `SocketKey` is `"lyx-"` followed by `hash8`;
  `SessionName` is the target's basename, a hyphen, then `hash8`;
  `AnchorPath` is `stateDir`;
  `WorktreeRoot` is `target`;
  `LogsDir` is `stateDir` joined with `"logs"`, told directly and deliberately **not** `fabricengine.HubLogsDir(stateDir)`, which would produce a board-shaped path that does not exist here;
  `RepoName` is the target's basename;
  `HubPath` is `stateDir`;
  `PaneCwd` is `target`.
  The doc comment on `ReedGeometry` must call out the `AnchorPath`/`PaneCwd` divergence explicitly: reed's state files belong under `stateDir`, while every pane must start in `target`, because `target` is the git repository holding the source an implementer builds, tests and commits in.
  This package must not import `internal/fabricengine` or `internal/lyxcwd`.
- **Commit:** `feat(standalonegeom): add the told-mode reed geometry builder`

### Card 20: Add the standalone webster builder

- **Context:**
  - `internal/websterengine/geometry.go`
  - `internal/websterengine/state.go`
  - `internal/planparser/parse.go`
  - `internal/lyxdirs/dirs.go`
  - `internal/standalonegeom/doc.go`
- **Edits:** none
- **Creates:**
  - `internal/standalonegeom/webstergeom.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/standalonegeom/webstergeom.go` exporting `WebsterGeometry(target, stateDir string) websterengine.Geometry`, told the same already-absolute target and derived state directory the reed builder takes.
  Field values:
  `AnchorRoot` is `stateDir`;
  `WorktreeRoot` is `target`;
  `WebsterDir`, `ReportsDir`, `ScratchDir` and `PromptsDir` come from the four `websterengine` accessors applied to `stateDir`, so standalone's `_lyx`/`.lyx` pair are ordinary directory siblings under `<state>` at the same mirrored subpaths a hub uses;
  `PlanDir` is `planparser.PlanDir(stateDir)`;
  `StencilsDir` is `stateDir` joined with `lyxdirs.LyxDirName` and `"stencils"`.
  The builder takes no `hash8`, unlike the reed one, because none of webster's eight values is hash-derived — do not add an unused parameter for symmetry.
  Both `PlanDir` and `StencilsDir` are the *defaults*: batch 8 overrides either from a CLI flag after calling this builder, and this package knows nothing about flags.
  Say so in the doc comment, and state that `WorktreeRoot` is the fork-audit workdir and the `{{.worktree_root}}` token's value in standalone, which is why it is `target` and never `stateDir`.
- **Commit:** `feat(standalonegeom): add the told-mode webster geometry builder`

### Card 21: Pin both standalone builders

- **Context:**
  - `internal/standalonegeom/reedgeom.go`
  - `internal/standalonegeom/webstergeom.go`
  - `internal/websterengine/state.go`
  - `internal/planparser/parse.go`
  - `internal/reedengine/geometry.go`
- **Edits:** none
- **Creates:**
  - `internal/standalonegeom/standalonegeom_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/standalonegeom/standalonegeom_test.go` with pure path-math table tests over both builders, driven by a fixed target directory and **told literal** `stateDir` and `hash8` values.
  Nothing in this file may call `standalonestate.Derive`, read an environment variable, or touch disk;
  the whole point of the builders' told parameters is that these tests need no `t.Setenv` and can be `t.Parallel()`.
  The reed test must assert every field of the pinned table in card 19, one assertion per field, and specifically:
  `LogsDir` asserted as `stateDir` joined with `"logs"` — the wrong answer is one `fabricengine.HubLogsDir` call away and is a board-shaped path, so this row must be explicit rather than derived;
  and `PaneCwd == target` together with `AnchorPath == stateDir` **in the same test case**, with the fixture choosing a target that is not under `stateDir`, since the whole reason the field exists is that the two differ here.
  The webster test must assert all eight fields, with `AnchorRoot == stateDir` and `WorktreeRoot == target` asserted in the same case for the same reason.
  Use a target whose basename is distinctive so `RepoName` and `SessionName` cannot pass by coincidence against another field's value.
- **Commit:** `test(standalonegeom): pin every field of both told-mode geometry builders`

## Batch Tests

`verify:` is `go test ./internal/hubgeom/... ./internal/standalonegeom/...`, the two packages this batch adds to;
no other package's behaviour changes, and both additions are new exported functions with no existing callers until batch 8.
Card 18 pins the hub teller against the accessors themselves rather than against restated literals, so it cannot drift from `websterengine`'s own joins.
Card 21 is the pinned-table home the `standalonegeom` decision asks for, and its two same-case assertions — `PaneCwd` against `AnchorPath`, and `WorktreeRoot` against `AnchorRoot` — are the ones that would still pass under a wrong implementation if they were split across cases with coinciding fixture values.
Hermeticity is structural here rather than asserted: neither builder can reach the environment, so no test in this package needs the `t.Setenv` redirect batch 8's integration test does.
