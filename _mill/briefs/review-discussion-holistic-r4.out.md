MILL_REVIEW_BEGIN
# Review: lyxtest: build real fabric hubs, invert the lyxtest/fabric dependency

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-12
```

## Findings

### [BLOCKING:design] `PrimeWeft()` is not `WeftBase` at a non-"." anchor
**Section:** § Decisions → "Config seeding on a real hub", point 2 (and the matching Testing bullet).
**Issue:** The decision asserts `hubforge.SeedConfig` writes into `h.PrimeWeft()` and that "This is `res.WeftBase`" — false for a subpath anchor: `internal/fabricengine/clone.go:406` sets `weftBase := filepath.Join(WeftWorktree(l), l.AnchorRel)`, while `fabrictest/hub.go:163-165`'s `PrimeWeft()` returns `fabricengine.WeftWorktree(h.Location)` with no `AnchorRel`. Since `NewHub` explicitly supports the `"backend"` anchor, a seed would land at `<weft>/_lyx/config` while `configsync`/every module loader reads `<weft>/backend/_lyx/config`, so the override would silently not take effect.
**Fix:** State that the seed base is `res.WeftBase` (anchor-joined) and that `hubforge` exposes it as such — either redefine the accessor or add a distinct anchored accessor — rather than equating it with the un-anchored weft-sibling path.

### [NIT:consistency] `gitkit` fixture consumer set stated two ways
**Section:** § Constraints, first bullet vs § Decisions "Migrate all 132 above-fabric sites".
**Issue:** The Constraints summary says gitkit's "primitive repo fixtures serve `gitrepo` and `lyxcwd` only", while the Decision (and the already-applied `CONSTRAINTS.md`) pins `CopyRepo`'s caller set to `internal/lyxcwd` alone, noting `gitrepo` has zero `Copy*` sites and uses `MustRun` only.
**Fix:** Reword the Constraints bullet to match the one-package allowlist, so a plan writer does not encode `{lyxcwd, gitrepo}` in the guard test.

### [NIT:scope] `protocol.file.allow` change to `HermeticGitEnv` is unlisted work
**Section:** § Decisions "Local bare repos are the remote substrate"; § Scope "In".
**Issue:** `internal/lyxtest/hermetic.go:33-43`'s neutral config sets user/init/core/maintenance/gc only, so "Set `protocol.file.allow always` in the hermetic git env" is a real edit to a helper the discussion elsewhere describes as carried over unchanged; it appears in no scope item and no test bullet.
**Fix:** List the `HermeticGitEnv` neutral-config edit as its own in-scope item, or drop it explicitly as unnecessary for path-reached bares.

### [NIT:scope] Unexported access for `fabricengine`'s 14 moved in-package files
**Section:** § Scope; § Decisions "The two stuck in-package files move to external test packages".
**Issue:** `export_test.go` shims are decided only for `treadleengine`/`loomengine`; the 14 `fabricengine` in-package files that become `package fabricengine_test` are covered only by a "Reusable pieces" mention of the existing `export_test.go`, with no stated disposition for the unexported identifiers they use (while `NewPairedForTest` is simultaneously deleted from that file).
**Fix:** State that `internal/fabricengine/export_test.go` is extended as needed for the moved files, so the shim's growth is planned rather than discovered.

## Verdict

REQUEST_CHANGES
Seeding base is wrong for subpath anchors; the rest is wording and unlisted scope.
MILL_REVIEW_END
