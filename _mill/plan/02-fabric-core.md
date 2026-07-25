# Batch: fabric-core

```yaml
task: 'fabric: unify warp + weft into one git-coordination module'
batch: fabric-core
number: 2
cards: 5
verify: go test ./internal/fabricengine ./internal/configreg
depends-on: []
```

## Batch Scope

Creates `internal/fabricengine` with everything that is pure logic or config plumbing:
the package doc, the one `fabric.yaml` config (registered in configreg), the
`WeftBranchName` derivation, the `Warp-SHA` trailer helpers, the git-free correspondence
index component, and the `Fabric` handle with its sync options. Everything here is
Tier-1 testable (untagged, no git spawns) except nothing — this batch spawns no git at
all; `testmain_test.go` is still created here so later batches' integration tests land
in a package that already satisfies the Hermetic Git Test Environment Invariant.
External interface consumed by batches 3–6: `Config`/`LoadConfig`/`ConfigTemplate`,
`WeftBranchName`, `appendWarpSHATrailer`/`WarpSHATrailerKey`, the `corrIndex` component,
`Fabric`/`New`/`SyncOptions`/`EnvSyncOptions`/`DefaultCommitMessage`/`ScopedPathspec`.
No batch-local decisions differ from the overview.

## Cards

### Card 3: package skeleton, config, configreg registration

- **Context:**
  - `internal/warpengine/config.go`
  - `internal/warpengine/config_test.go`
  - `internal/warpengine/template.go`
  - `internal/warpengine/template.yaml`
  - `internal/warpengine/template_test.go`
  - `internal/warpengine/testmain_test.go`
  - `internal/lyxtest/hermetic.go`
  - `internal/weftengine/config.go`
  - `internal/weftengine/template.yaml`
  - `internal/configengine/config.go`
  - `manifest/designs/fabric.md`
  - `_mill/discussion.md`
- **Edits:**
  - `internal/configreg/configreg.go`
  - `internal/configreg/configreg_test.go`
- **Creates:**
  - `internal/fabricengine/doc.go`
  - `internal/fabricengine/config.go`
  - `internal/fabricengine/config_test.go`
  - `internal/fabricengine/template.go`
  - `internal/fabricengine/template.yaml`
  - `internal/fabricengine/template_test.go`
  - `internal/fabricengine/testmain_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** `doc.go`: package comment for `fabricengine` stating the module's
  thesis (one git-coordination module over two `gitrepo.Repo` instances; parallel build
  alongside warp/weft until cutover; uniform `<host>`/`<host>-weft` branch scheme;
  trailer-backed correspondence; explicit-list `StageAndCommit` only — never
  `StageAllAndCommit`), seeded from `manifest/designs/fabric.md`'s durable rationale.
  `config.go`: `type Config struct { BranchPrefix string; Pathspec string }` with yaml
  tags `branch_prefix` and `pathspec`, `func (c Config) Dirs() []string`
  (strings.Fields split, mirroring `weftengine.Config.Dirs`), and
  `func LoadConfig(baseDir string) (Config, error)` calling
  `configengine.Load(baseDir, "fabric", []byte(ConfigTemplate()))` then
  `yaml.Unmarshal`. `template.go`/`template.yaml`: the warp/weft embed pattern;
  template keys `branch_prefix: ${env:LYX_BRANCH_PREFIX:-}` and `pathspec: _lyx`
  (comment each like the source templates). Register in
  `configreg.Modules()` as `{Name: "fabric", Template: fabricengine.ConfigTemplate}`
  in alphabetical position (between `burler` and `loom`) with the import added, and
  extend `TestNames`'s `want` slice in `configreg_test.go` accordingly.
  `testmain_test.go`: package `fabricengine`, `TestMain` calling
  `lyxtest.HermeticGitEnv()` before `m.Run()`. `config_test.go`/`template_test.go`
  (untagged): mirror warpengine's — happy-path load, env resolution for
  `branch_prefix`, not-initialized error, template is valid YAML with both keys,
  `pathspec` resolves to `_lyx`.
- **Commit:** `feat(fabricengine): package skeleton, fabric.yaml config, configreg registration`

### Card 4: branch-name derivation

- **Context:**
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/warpengine/add.go`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/branchname.go`
  - `internal/fabricengine/branchname_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** `branchname.go`: `func WeftBranchName(hostBranch string) string`
  returning `hostBranch + hubgeometry.WeftSuffix` — the ONLY place fabric composes a
  weft branch name; godoc states the uniform scheme (host `<branch>` ↔ weft
  `<branch>-weft`, primary `main` ↔ `main-weft`, no exceptions) and that the inverse is
  `hubgeometry.WeftHostSlug`. The `-weft` literal must not appear in the Go source
  (Hub Geometry Invariant token ban) — use the exported constant. Untagged
  `branchname_test.go`: `main` → `main-weft`; prefixed task branch `hanf/foo` →
  `hanf/foo-weft`; round-trip with `hubgeometry.WeftHostSlug` recovers the host branch;
  empty-prefix task branch (plain slug) works.
- **Commit:** `feat(fabricengine): uniform WeftBranchName derivation`

### Card 5: Warp-SHA trailer helpers

- **Context:**
  - `manifest/designs/fabric.md`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/trailer.go`
  - `internal/fabricengine/trailer_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** `trailer.go`: `const WarpSHATrailerKey = "Warp-SHA"`;
  `func appendWarpSHATrailer(message, warpSHA string) string` appending a git-trailer
  block (`Warp-SHA: <sha>` separated from the body by a blank line, per git's trailer
  conventions — same shape as `Co-authored-by:`); `func parseWarpSHATrailer(message
  string) (sha string, ok bool)` extracting the trailer value from a full commit
  message (last `Warp-SHA:` line wins; tolerate surrounding whitespace; `ok=false` when
  absent). Untagged `trailer_test.go`: round-trip append→parse for single-line and
  multi-paragraph messages; message already ending in a trailer block gains the line
  without a stray blank line; parse on a message without the trailer → `ok=false`;
  multiple trailers → last wins.
- **Commit:** `feat(fabricengine): Warp-SHA trailer format and parse`

### Card 6: correspondence-index component (git-free)

- **Context:**
  - `internal/state/state.go`
  - `manifest/designs/fabric.md`
  - `_mill/discussion.md`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/corrindex.go`
  - `internal/fabricengine/corrindex_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** `corrindex.go` — the pure component per the discussion's layering
  decision: it takes an explicit file path and never touches git.
  `type corrEntry struct { WarpSHA string; WeftSHA string; WarpSeq int }` with json
  tags `warp_sha`, `weft_sha`, `warp_seq`;
  `type corrIndex struct` holding the path and a `WarpSeq`-sorted entry slice;
  `func loadCorrIndex(path string) (*corrIndex, error)` via
  `state.ReadJSON[[]corrEntry](path, path+".lock")` (missing file → empty index);
  `func (ix *corrIndex) record(e corrEntry) error` upserting by `WarpSHA`, keeping the
  slice sorted by `WarpSeq` (stable for equal seq), persisting via
  `state.WriteJSON(path, path+".lock", entries)`;
  `func (ix *corrIndex) exact(warpSHA string) (corrEntry, bool)`;
  `func (ix *corrIndex) nearestAtOrBefore(seq int) (corrEntry, bool)` — binary search
  (`sort.Search`) returning the greatest entry with `WarpSeq <= seq`, and of entries
  sharing that seq the last recorded; `func (ix *corrIndex) entries() []corrEntry`
  (copy, for RebuildIndex equality assertions). Untagged `corrindex_test.go` against a
  `t.TempDir()` file path, no git: record/reload round-trip; upsert overwrites the
  weft SHA for an existing warp SHA; exact hit and miss; nearestAtOrBefore on empty
  index → `false`; nearest with target below all seqs → `false`; exact-seq and
  between-seqs hits; persistence is atomic (file parses after every record).
- **Commit:** `feat(fabricengine): sorted git-free correspondence-index component`

### Card 7: Fabric handle and sync options

- **Context:**
  - `internal/weftengine/weft.go`
  - `internal/weftengine/sync.go`
  - `internal/gitrepo/gitrepo.go`
  - `manifest/designs/fabric.md`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/fabric.go`
  - `internal/fabricengine/fabric_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** `fabric.go`: `type Fabric struct { Warp *gitrepo.Repo; Weft
  *gitrepo.Repo; warpPath, weftPath string }` (exported repo fields per the design —
  consumers call `f.Warp.StageAndCommit(...)` directly, no forwarding methods);
  `func New(warpPath, weftPath string) (*Fabric, error)` stat-checking both paths are
  existing directories (typed error naming the missing path) and wrapping each in
  `gitrepo.New`; `type SyncOptions struct { SkipGit, SkipPush bool }`;
  `func EnvSyncOptions() SyncOptions` reading `WEFT_SKIP_GIT`/`WEFT_SKIP_PUSH` == "1"
  (same env vars as weftengine — parity decision in the overview);
  `const DefaultCommitMessage = "weft sync"`; `func ScopedPathspec(relPath string,
  dirs []string) []string` mirroring `weftengine.ScopedPathspec` exactly. Untagged
  `fabric_test.go`: `New` with a missing warp or weft dir errors naming that path;
  happy path yields non-nil `Warp`/`Weft`; `EnvSyncOptions` mapping via `t.Setenv`
  (unset/`"1"`/other values); `ScopedPathspec` mirrors the weftengine cases (root
  relPath, nested relPath).
- **Commit:** `feat(fabricengine): Fabric handle, sync options, scoped pathspec`

## Batch Tests

`verify: go test ./internal/fabricengine ./internal/configreg` — every test in this
batch is untagged Tier-1 (no git spawns; the corrindex component deliberately takes a
temp file path per the layering decision), so the plain run covers all five cards plus
configreg's pinned `TestNames`. Integration coverage of these pieces arrives with their
consumers in batches 3–5.
