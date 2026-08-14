# fabric — independent review, round 3 (`fable-high-r3`)

Clean-room review of the `fabric` module per `_mill/fabric-review-prompt.md`.
Primary mission: the destruction chokepoint (`internal/fabricengine/destroy.go`), specifically the seeded containment TOCTOU residual.
Report is built incrementally during Job 1; executive summary and final severity ordering are written last.

## Executive summary

(to be written after the findings list is complete)

## Scope assessment (plan vs shipped)

(to be written)

## Code findings

(provisional entries appended as formed; final ordering at the end)

## Docs & operability findings

(provisional entries appended as formed)

## What was tested

Observations appended immediately after each command/scenario returns.

### Code-reading pass (pre-substrate)

Read in full: `internal/fabricengine/doc.go` (spec), `destroy.go`, `ancestors.go`, `remove.go`, `launchers.go`, `portals.go`, `prune.go`, `cleanup.go`, `dirtiness.go`, and the pathRequest regions of `junction.go`, `weftwiring.go`, `clone.go`, `add.go`, `unwire.go`; `CONSTRAINTS.md` in full; `internal/fslink` API surface.
Enumerated all 16 `pathRequest{` construction sites via grep (junction.go:591, weftwiring.go:177/209, clone.go:629/685, launchers.go:209/230, prune.go:278/310, add.go:270, unwire.go:151, portals.go:64, remove.go:224/273, destroy.go:778/888).

Static observations feeding the TOCTOU hunt (to be confirmed live):

- `checkPathRequest`'s absent-target short-circuit (`os.Lstat(req.target)` → `nil` on ENOENT) means a target that is absent at check time SKIPS all four checks, yet `removePath` re-`Lstat`s at act time and removes if the target is present THEN — so a dangling-then-live intermediate symlink gets an entirely UNCHECKED removal, not merely a containment-fallback one. Two distinct windows: (1) absent-at-check → present-at-act = zero checks run; (2) dangling-symlink ancestry at check → `resolveAncestorSymlinks` lexical fallback treats path as contained → act follows the now-live symlink.
- Executors act on the NOMINAL `req.target`: `removePath` (`RemoveAll`/`os.Remove`), `removeLink` (`fslink.Remove` = `os.Remove`). `removeGitWorktree` and `resetHardTo` delegate the act to git, which re-validates worktree registration at its own instant — much narrower exposure.
- Go toolchain is 1.26 (`go.mod`), so `os.Root` (openat-based, symlink-escape-proof traversal; `Root.Remove`/`Root.RemoveAll` available) is a candidate act-time enforcement layer that closes the window rather than shrinking it.
- `fslink.Remove` is plain `os.Remove` with idempotence — no Windows-specific removal path, so a root-relative removal has identical leaf semantics.
- The exported `RemoveAll` seam var in destroy.go has no current test consumer (grep found zero assignments outside its declaration).
