# Batch: quarry-dependency

```yaml
task: "Adopt quarry's glyph alphabet as the plan alphabet"
batch: "quarry-dependency"
number: 1
cards: 2
verify: go build ./...
depends-on: []
```

## Batch Scope

This batch makes `github.com/Knatte18/quarry` reachable from loomyard's own Go code and records the build-posture change that follows from it.
It is one batch because the `go.mod` require and the cgo prerequisite it introduces are a single decision with a single consequence — after this batch `lyx` is a cgo binary and every build host needs a C toolchain.
The external interface the next batch consumes is the `github.com/Knatte18/quarry/glyph` import path: `glyph.Language`, `glyph.Go`, `glyph.Glyph`, `glyph.Parse`, `glyph.Self`, `(Glyph).IsSelf`, `(Glyph).String`, and — per the two-preconditions-outside-this-worktree Shared Decision — `(Glyph).UnitPath`.

Batch-local decision, differing from nothing in `## Shared Decisions` but worth stating: this batch adds **no** import of the `github.com/Knatte18/quarry/quarry` facade anywhere in `internal/`.
Card 1 proves the module resolves and builds; the first real facade call site arrives in batch 4.

## Cards

### Card 1: require quarry at a real semver tag

- **Context:**
  - `CONSTRAINTS.md`
- **Edits:**
  - `go.mod`
  - `go.sum`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add a `require github.com/Knatte18/quarry <tag>` line to `go.mod`, where `<tag>` is the **existing** semver tag the operator cut on quarry `main`.
  Discover the tag with `git ls-remote --tags https://github.com/Knatte18/quarry` and pick the highest semver tag; the module path is `github.com/Knatte18/quarry` and the Go directive there is `go 1.26`, matching this module's own.
  Run `go get github.com/Knatte18/quarry@<tag>` from the repository root so `go.sum` is populated by the toolchain rather than hand-edited, then `go mod tidy`.
  Do not introduce a pseudo-version on `main` as a stopgap and do not add a `replace` directive to a local worktree — a `replace` is unbuildable for anyone else, and a pseudo-version silently turns every quarry commit into a loomyard upgrade decision.
  If `git ls-remote --tags` returns no semver tag (only `archive/*` names), stop and report that the quarry-side precondition is unmet rather than inventing a version: this card cannot proceed without it.
  Confirm the dependency links by building `./cmd/lyx` — the quarry engine links tree-sitter's C grammars through cgo, so this build is the first that requires `CGO_ENABLED=1` and a C compiler on PATH.
- **Commit:** `1: build(deps): require github.com/Knatte18/quarry at a semver tag`

### Card 2: record the cgo prerequisite and pin it in the deploy build

- **Context:**
  - `go.mod`
- **Edits:**
  - `README.md`
  - `CLAUDE.md`
  - `tools/deploy/main.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Document the new build prerequisite and make the repo's own build path honour it.
  In `README.md`, extend the `## Building` section's prerequisite list (which currently reads `- Go 1.26+`) with a C toolchain entry stating that `lyx` links quarry's tree-sitter grammars through cgo, so `CGO_ENABLED=1` and a C compiler (gcc/clang on POSIX, mingw-w64 on Windows) are required.
  State the fact that `CGO_ENABLED` already defaults to `1` for a native build when a C compiler is on PATH, so nothing needs setting on an ordinary developer machine, and that `go env -w CGO_ENABLED=1` pins it per-user but is machine-local and does not install a compiler. No file read needed.
  In `CLAUDE.md`, add a short `## Build prerequisite: cgo` section carrying the same two facts, so an agent reading only `CLAUDE.md` learns it before its first build.
  In `tools/deploy/main.go`, set `CGO_ENABLED=1` explicitly on the `build` command the deploy path runs: after `build.Dir = root` is assigned, assign `build.Env = append(os.Environ(), "CGO_ENABLED=1")`, so a deploy from an environment that has disabled cgo fails at the compiler rather than producing a broken binary.
  Do not add a `.github/workflows` file — this repository has no CI and none is being introduced here.
- **Commit:** `2: docs(build): record the cgo/C-toolchain prerequisite and pin CGO_ENABLED in deploy`

## Batch Tests

`verify: go build ./...` is the whole gate for this batch, and it is the right one: the batch's only functional claim is that the quarry module resolves, downloads, and links, and a full-module build is exactly what proves it.
No new Go test file is added — there is no new loomyard behaviour here, only a dependency edge and two documentation edits.
`tools/deploy/main.go`'s existing `tools/deploy/main_test.go` stays tier-1 pure and is unaffected: the card assigns `build.Env` on the already-constructed command and adds no spawn to any test path.
The module-wide `verify: go build ./...` from the overview is the same command, which is deliberate for this one batch and diverges from batch 2 onward.
</content>
