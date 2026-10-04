# PATTERN-fabric-vocabulary

**Fabric** names the wired composite.
**warp** and **weft** name the two sides, used only where they must be told apart.
"repo" alone never substitutes for warp.
**`host` is retired** in the fabric sense, everywhere.

- Owner set (bare weft and warp carve-out): `fabricengine`, `fabriccli`, `weftname`, `gitkit`, `hubforge`, `boardengine`, `configsync`.
- Scope of the bare weft and warp rule outside the owner set:
  - every `*_test.go` file anywhere in the module;
  - every non-test `.go` file under `internal/` and the doc roots;
  - every `.md` and `.yaml` file under `contracts/`, `docs/`, `plugins/`, `crucible/` and `cmd/`;
  - every `.md` file under `internal/`;
  - `CLAUDE.md` and `README.md`.
- Skipped: `_lyx`, `.lyx` and the repo-root `sandbox/` fixture tree, matched on the path's first segment, and the paths in `vocabScanAllowlist` in `internal/lyxcwd/enforcement_test.go`.
- `internal/configsync` keeps a narrower carve-out in its test files too: string literals and comments may name the sides, identifiers may not.
- `vocabExempt` in `internal/lyxcwd/enforcement_test.go` is the closed list of owner-set spellings a non-owner file may carry despite naming a side.
  The scan strips exactly those whole tokens before matching, so any other identifier carrying a side, including a new owner-set export, still fails.
  An entry needs a reason naming its owner-set declaration.
- `internal/weftname` is imported only from the owner set and from `internal/lyxcwd/geometry_test.go`, which tests `weftname.SiblingPath`.
- Enforced by `TestEnforcement_FabricVocabulary`, which `internal/lyxcwd/vocabscan_test.go` backs with fixtures.
