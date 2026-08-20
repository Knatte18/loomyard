MILL_REVIEW_BEGIN
# Review: Extract scout into its own standalone repo — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-08-20
```

## Findings

### [BLOCKING:consistency] go.sum lacks a full hash for cobra's windows-only mousetrap dependency
**Location:** `/home/knatte/Code/quarry/wts/quarry/go.sum:4`, `go.mod:1-14`
**Issue:** `go.sum` carries only a `/go.mod` hash for `github.com/inconshreveable/mousetrap` (no `h1:` content hash), and `go.mod` doesn't list it as an indirect require at all. Loomyard's own `go.sum` — same cobra v1.10.2, same windows-tagged file pattern via `internal/proc/proc_windows.go` — has the full `h1:` content hash for the identical module, because cobra's own windows-only file imports it. This strongly indicates quarry's `go.mod`/`go.sum` were last tidied before `internal/cli` (which imports cobra) existed and were never regenerated afterward.
**Fix:** Re-run `go mod tidy` against the final tree (including `internal/cli`) and confirm `GOOS=windows go build ./...` succeeds under default `-mod=readonly`, since the "supported platforms are linux and windows" decision makes this a real target, not a hypothetical one.

### [BLOCKING:scope] Loomyard/mill-internal residue survives in ported production comments, uncaught by the "lyx" sweep
**Location:** `quarry/errors.go:1-2`, `quarry/refs.go:9`, `quarry/registry.go:4`, `quarry/ensureserver.go:1-2`, `quarry/detect.go:2`, `quarry/refs_test.go:389`, `quarry/ensureserver_test.go:326`, `quarry/refs_integration_test.go:55`
**Issue:** Cards 17/21/32's vocabulary sweep greps only for the substring `lyx`, so it misses a whole class of dead Loomyard-internal references that don't contain that substring: `errors.go` still says "the scoutengine package returns to its sole caller, internal/scoutcli"; `refs.go:9` says "the CLI layer (internal/scoutcli, batch 3) calls"; `registry.go:4` says "It mirrors internal/modelspec's registry.go"; `ensureserver.go:1-2` cites `manifest/designs/scout-redesign.md`, a file that doesn't exist in this repo. Several other files cite bare "batch N" numbering meaningless outside the mill task. `registry.go` and `ensureserver.go` were both in card 17's own `Edits:` list, so this isn't merely an unscoped file — the sweep method itself was too narrow to catch it.
**Fix:** Add a second sweep pass (grep for `scoutcli`, `scoutengine package`, `modelspec`, `manifest/designs`, `batch \d`) across the ported `quarry/` package and rewrite or drop each stale cross-reference, same discipline `doc.go`/`load.go` already received.

### [NIT:consistency] card 32's "grep -ric 'lyx' ... confirm the total is zero" is not actually zero, and the discrepancy is unrecorded
**Location:** `/home/knatte/Code/quarry/wts/quarry/README.md:41-44`, `internal/cli/cli.go:448`, `quarry/quarrydaemon_test.go`, `internal/cli/resolve_test.go`, `docs/scout-*.md`
**Issue:** A repo-wide `grep -ric 'lyx'` returns 9 files, not zero as card 32 claims to confirm. Most are legitimate — README's card-2-mandated "Upgrading from `lyx scout`" section, `cli.go`'s contrastive "a lyx hub" doc comment — but this directly contradicts card 32's literal instruction, and batch 1 card 2 (which mandates the README mention) was never reconciled against batch 4 card 32's later zero-count requirement anywhere in the plan or the (now-deleted) port log.
**Fix:** No code fix needed; note for future plan authoring that "grep -ric 'lyx' must be zero" should have been scoped to exclude the README's deliberately-mandated mentions, or reworded as "zero unjustified hits."

### [NIT:consistency] Garbled cross-reference in the equivalence document's intro
**Location:** `/home/knatte/Code/quarry/wts/quarry/docs/port-equivalence.md:7`
**Issue:** `(see docs/research/quarry-multilang.md/scout-multilang.md's benchmark: ...)` joins a nonexistent Loomyard-side path (`docs/research/quarry-multilang.md` was never created; the actual deleted Loomyard path was `docs/research/scout-multilang.md`) with an unqualified quarry-relative filename in one broken slash-joined reference.
**Fix:** Rewrite as a single valid pointer to quarry's own `docs/scout-multilang.md`.

## Verdict

REQUEST_CHANGES
go.sum is stale for the windows target and Loomyard-internal residue leaked past the vocabulary sweep into public doc comments.
MILL_REVIEW_END
