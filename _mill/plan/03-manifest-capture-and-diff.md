# Batch: manifest-capture-and-diff

```yaml
task: 'fabric: live-state integration harness (slice 13)'
batch: 'manifest-capture-and-diff'
number: 3
cards: 2
verify: go build ./... && go test -tags integration ./internal/fabricengine/fabrictest/ && go test ./cmd/lyx/ -run 'TestTierPurity_UntaggedTestsSpawnNothing|TestNoDestructiveBypass_FabricengineProductionSource' && go test ./internal/lyxcwd/ -run TestEnforcement
depends-on: [2]
```

## Batch Scope

This batch delivers the survival-assertion mechanism: whole-hub manifest capture, the prefix-rooted permit diff, and the narrow `.git` allowlist.
It is one batch because the capture and the diff are one contract — the entry shape decides what the diff can report, and the `.git` rule is only expressible as a property of both — and because its TDD suite asserts round-trip properties that need both halves present.

The external interface batches 5-7 consume is `CaptureManifest`, `DiffManifest`, and the permit-root type.

Batch-local decision: sentinels are not a separate mechanism.
A state plants named files as part of its own `Apply`, and the manifest diff is what reports their disappearance;
the sentinel's only job is making the failure message legible ("the operator's uncommitted file is gone" rather than "a path list changed").

## Cards

### Card 10: manifest capture, prefix-rooted permit diff, and the `.git` allowlist

- **Context:**
  - `internal/fabricengine/fabrictest/hub.go`
  - `internal/fabricengine/fabrictest/doc.go`
  - `internal/fslink/fslink.go`
  - `internal/fslink/fslink_linux.go`
  - `internal/fslink/fslink_windows.go`
  - `internal/fabricengine/destroy.go`
  - `internal/lyxcwd/anchor.go`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/fabrictest/manifest.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** `//go:build integration` on the first line.
  **The entry.** Define `Manifest` as a map from a `filepath.ToSlash`-normalised **hub-relative** path to an `Entry` carrying exactly three things: a `Kind` (`KindFile`, `KindDir`, `KindLink`);
  for a link, its raw one-hop target obtained from `fslink.RawTarget` — **not** `fslink.PointsTo`, for the same reason the gate's `ownedWiredJunction` check uses the raw target: a fully-resolved chain fails outright when a later segment is gone, and silently walks past a target that is itself a further link;
  and for a file, a content hash (SHA-256 of the bytes is fine).
  Deliberately **not** recorded, each with a one-line comment saying why: permission bits (meaningless on Windows), mtime, and size-without-hash.
  This is the minimum that makes "it vanished", "it was replaced by a different kind of thing" and "its content changed" all observable, and nothing beyond it.
  **The walk.** `CaptureManifest(tb testing.TB, hubRoot string) Manifest` walks the hub root and **records a link as a leaf, never descending through it, on every platform.**
  Detect links with `fslink.IsLink` before deciding to descend — never `os.Lstat` mode bits, which do not answer the question for a Windows junction.
  This rule is load-bearing, not an optimisation: fabric's wired junctions carry **absolute** targets that point back inside the same hub (`warp/_lyx` → `<hub>/warp-weft/_lyx`, `warp/.lyx` → …, `warp/_board` → `<hub>/_board`), so a descending walk would record the weft sibling's contents twice — once under its own path and again under every warp junction chaining into it — and every legitimate weft-side change would surface as an unpermitted mutation under a *warp* key, while permit roots written against warp paths would silently license weft destruction.
  Windows junctions and POSIX symlinks differ in how a directory walk traverses them, so a walk that happens not to descend on one platform may descend on the other;
  state this in the function's doc comment.
  The link's identity is fully captured by its kind plus its raw one-hop target, and whatever the target contains is recorded under the target's own path, exactly once.
  **The `.git` rule.** Inside any `.git` directory — or a `.git` *file*, which is what a linked worktree carries — record **only** the `.git` entry itself and each `.git/worktrees/<name>` directory, both at existence granularity with no content hash.
  Everything else under `.git` is excluded.
  Comment both halves of the reasoning: a blanket `.git` exclusion would blind the harness to linked-worktree deregistration, which is R3's shape exactly, while hashing everything under `.git` would drown every cell in `index`, `logs/**`, `refs/**`, `objects/**`, `FETCH_HEAD`, `ORIG_HEAD` and `packed-refs` churn until the permit lists permitted everything.
  Branch existence is deliberately not carried by the manifest — it is asserted through git itself in the per-verb effect assertions, where the answer is authoritative rather than inferred from a ref file.
  **The diff.** `DiffManifest(before, after Manifest, permitted []string) []Change` returns every path that disappeared, changed kind, changed link target, or changed content hash and that is **not** at or below any permitted root.
  A permitted root is a `ToSlash` hub-relative path prefix;
  matching is on path segments, so `_portals/x` never permits `_portalsfoo/y`.
  Comparison is case-folding on Windows only: `lyxcwd.samePath` (`internal/lyxcwd/anchor.go`) does exactly this via `strings.EqualFold` but is unexported, so this file carries its own equivalent rather than reaching for it.
  Additions are reported separately from removals and mutations, so a cell can assert on destruction without being noisy about creation.
  Provide `AssertNoUnpermittedChange(tb testing.TB, before, after Manifest, permitted []string)` whose failure message names each offending path, its kind, and the nearest permitted root that did *not* cover it — failure triage is the reason the manifest exists alongside sentinels rather than instead of them.
- **Commit:** `fabrictest: add whole-hub manifest capture and the prefix-rooted permit diff`

### Card 11: TDD suite for capture and diff

- **Context:**
  - `internal/fabricengine/fabrictest/manifest.go`
  - `internal/fabricengine/fabrictest/hub.go`
  - `internal/fslink/fslink.go`
  - `internal/fslink/fslink_linux.go`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/fabrictest/manifest_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** `//go:build integration`, `package fabrictest`, every test `t.Parallel()` on its own hub from `NewHub`.
  Round-trip properties, one subtest each: an unchanged hub diffs empty;
  deleting a file outside every permitted root is reported;
  deleting a file *under* a permitted root is not;
  a link whose raw one-hop target changes is reported;
  a path replaced by a different kind of thing (a file where a link was) is reported.
  Two `.git`-rule assertions, one per direction, and both are required — they are what pin the allowlist against being widened or narrowed later.
  First, running an ordinary `git status` and then a commit against the hub produces an **empty** diff, proving the churn the allowlist excludes (`index`, `logs/**`, `refs/**`, `objects/**`) really is excluded.
  Second, removing a linked worktree's `.git/worktrees/<name>` registration directory **is** reported, proving the R3-shaped deregistration case the allowlist deliberately keeps observable.
  Two portability assertions: every key in a captured manifest is `ToSlash`-normalised (no `\` anywhere), and a permit root written with `/` matches on the running platform.
  One walk assertion, which is the one a Linux-only run would otherwise miss: capture a hub, then assert that a path reachable **only** by descending through a wired junction does not appear in the manifest under the junction's key — i.e. the weft sibling's contents are recorded once, under the weft sibling's own path, and never a second time under a warp junction path.
  Build and inspect every link in this file through `internal/fslink`, never `os.Symlink`/`os.Readlink`.
- **Commit:** `fabrictest: prove manifest capture, the permit diff, and the .git allowlist`

## Batch Tests

`verify:` runs card 11's suite plus three guards.
`go test -tags integration ./internal/fabricengine/fabrictest/` is the substantive gate and re-runs batch 2's hub suite alongside the new one, which is correct here — the manifest is captured against a factory-built hub, so a factory regression must fail this batch too.
`go test ./cmd/lyx/ -run 'TestTierPurity_UntaggedTestsSpawnNothing|TestNoDestructiveBypass_FabricengineProductionSource'` is included because `manifest.go` is the first file in the package to carry `os.Remove`-shaped tokens in its own test fixtures;
the destructive guard must still be excluding the directory (batch 1, card 3) rather than flagging them, and a broken exclusion would otherwise surface far from here.
`go test ./internal/lyxcwd/ -run TestEnforcement` catches a geometry literal in the new `.git`/permit-root path handling.
