All fixes applied and committed, all verify commands pass, working tree clean, HEAD advanced from baseline.

Summary of work:
- Fixed 5 NIT findings from `/home/knatte/Code/loomyard/wts/fabric-collapse-external-surface/_mill/reviews/20260802-144625-code-review-r2.md`:
  - `internal/fabricengine/doc.go`: `CommitWeftAt` → `commitWeftAt` casing; added `Diff`, `Status` to the cross-repo-ops enumeration.
  - `internal/fabricengine/index.go`: `Fabric.SnapshotWarpSHA` → `snapshotWarpSHA` at all four stale mentions (lines 191, 202, 262, 387).
  - `internal/fabriccli/fabric.go`: added `diff` to the weft-git verb enumeration.
  - `manifest/designs/fabric-unified-view.md`: `RevertWithWeft` → `resolveRevertTarget`/`classifyCorrespondence` (line 117 per the finding); also swept the same file's line-115 `Fabric.SnapshotWarpSHA` mention since it's the identical stale-casing pattern in a file already being touched.
- All target files were already in the plan's "All Files Touched" list, so no plan-file update was needed.
- Committed as `ef72fab524e1e7bb88297b499d137c99009754e9` and pushed.
- Ran all 6 batch `verify:` commands plus the module-wide `go build ./...` — all passed.
- Working tree clean (`git status --porcelain` empty), HEAD advanced from baseline `bc7806abd9f5a54cee824a3e19936719d9d63841`.

{"status":"success","commit_sha":"ef72fab524e1e7bb88297b499d137c99009754e9","session_id":"7aca3f03-f2f9-475c-a570-0034f60e3857"}

{"status":"success","commit_sha":"ef72fab524e1e7bb88297b499d137c99009754e9","session_id":"7aca3f03-f2f9-475c-a570-0034f60e3857"}
