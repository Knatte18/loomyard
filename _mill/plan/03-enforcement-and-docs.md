# Batch: enforcement-and-docs

```yaml
task: "Unify webster/burler CLI wiring into a shared module"
batch: "enforcement-and-docs"
number: 3
cards: 3
verify: go test ./... && go test -tags integration ./internal/cliwire/... ./internal/webstercli/... ./internal/burlercli/... ./internal/standalonegeom/...
depends-on: [2]
```

## Batch Scope

This batch adds the two mechanical checks that keep a third copy of the wiring from appearing, and lands the documentation the new module owes.
It depends on batch 2 because both checks fail while `internal/webstercli` and `internal/burlercli` still carry their own copies: the caller-set pin would see two extra production `standalonestate.Derive` call sites, and the banned-declaration check would see nine banned function names.

The two checks are complementary and neither is sufficient alone.
The `Derive` pin catches a whole third copy built from the bottom up but misses a CLI that re-hand-rolls target resolution while still calling `ResolveStandalone` for the rest;
the banned-declaration check catches exactly that partial re-implementation but misses a new CLI deriving its own state directory from scratch.

Batch-local decision beyond `## Shared Decisions`: both checks are **production-only**, skipping every `_test.go` file, following `internal/treadleengine/seam_enforcement_test.go`'s skip rather than `internal/gitkit/callerset_enforcement_test.go`'s package-directory-only exclusion.
The invariant is about production wiring.
Eight `Derive` test call sites exist today and stay where they are — they build fixtures and assert the real derivation end-to-end, they are not a second copy of the wiring, and forcing them through `cliwire` would make packages with no reason to depend on it do so.
An explicit test-file allowlist was rejected as maintenance on something a code review would see anyway;
production drift is what slips in silently.

## Cards

### Card 8: standalonestate.Derive caller-set pin

- **Context:**
  - `internal/gitkit/callerset_enforcement_test.go`
  - `internal/treadleengine/seam_enforcement_test.go`
  - `internal/standalonestate/standalonestate.go`
  - `internal/cliwire/standalone.go`
  - `internal/webstercli/wiring.go`
  - `internal/burlercli/wiring.go`
  - `internal/webstercli/wiring_test.go`
  - `internal/burlercli/wiring_test.go`
  - `internal/webstercli/cli_integration_test.go`
  - `internal/standalonegeom/reedgeom_symlink_integration_test.go`
- **Edits:** none
- **Creates:**
  - `internal/cliwire/callerset_enforcement_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/cliwire/callerset_enforcement_test.go` in `package cliwire`, untagged, modelled on `internal/gitkit/callerset_enforcement_test.go`'s structure.

  Declare `const allowedDeriveCallerDir = "internal/cliwire"` with a comment naming it as the only package directory, relative to the repository root, whose production code may call `standalonestate.Derive`.

  Write `TestDeriveCallerSet_CliwireOnly` which:
  - resolves the repository root from `runtime.Caller(0)` by walking up from this file's directory, the way the gitkit test does;
  - walks every `.go` file under `internal/` and `cmd/`;
  - skips `internal/standalonestate` itself, whose own definition and doc comments name `Derive`;
  - skips every file whose base name ends in `_test.go`, following `internal/treadleengine/seam_enforcement_test.go`'s own skip;
  - skips the directory named by `allowedDeriveCallerDir`;
  - parses each remaining file with `go/parser`, finds the local identifier the file uses for the `github.com/Knatte18/loomyard/internal/standalonestate` import (honouring an explicit alias, falling back to `standalonestate`), and matches a `*ast.CallExpr` whose `Fun` is a `*ast.SelectorExpr` with `Sel.Name == "Derive"` and whose receiver `*ast.Ident` is that import identifier;
  - collects every offending repository-relative path and fails with a message naming `allowedDeriveCallerDir`, the offenders, and the Cliwire Sole-Wiring Invariant as the rule being enforced.

  The match must be on the AST, never on raw text, so a doc comment naming the qualified call cannot trip it.
  The test spawns no process and carries no build tag.
  Give the file a header comment stating what it enforces and why the pin is production-only, naming the eight test call sites that legitimately remain: three in `internal/burlercli/wiring_test.go`, one in `internal/webstercli/wiring_test.go`, two in `internal/webstercli/cli_integration_test.go`, and two in `internal/standalonegeom/reedgeom_symlink_integration_test.go`.
  Verify that count against the tree as it stands after batch 2 before writing it down, and correct the header to whatever the tree actually holds if batch 2's test prune changed it.
- **Commit:** `test(cliwire): pin standalonestate.Derive to cliwire in production code`

### Card 9: banned-declaration check over the two CLI packages

- **Context:**
  - `internal/treadleengine/seam_enforcement_test.go`
  - `internal/cliwire/callerset_enforcement_test.go`
  - `internal/cliwire/paths.go`
  - `internal/cliwire/module.go`
  - `internal/cliwire/standalone.go`
  - `internal/webstercli/wiring.go`
  - `internal/burlercli/wiring.go`
- **Edits:** none
- **Creates:**
  - `internal/cliwire/bannedecl_enforcement_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/cliwire/bannedecl_enforcement_test.go` in `package cliwire`, untagged.

  Declare the policed package directories as a slice of repository-relative paths, `internal/webstercli` and `internal/burlercli`, and the banned declaration names as a set: `resolveStandaloneTarget`, `repositoryRootOf`, `refuseNestedStandaloneGeometry`, `normalizeForContainment`, `pathContains`, `resolveToldDir`, `samePlanDir`, `standalonePlanDirHasContent`, `standaloneDefaultPlanDir`.
  Comment the set with the fact that these are the nine helper names the two CLIs used to declare for themselves, and that a package re-declaring any of them is re-implementing part of the prologue rather than calling into it.

  Write `TestBannedDeclarations_CliPackagesCallIntoCliwire` which resolves the repository root from `runtime.Caller(0)` exactly as card 8's test does, then for each policed directory parses every non-`_test.go` `.go` file with `go/parser` and inspects each top-level `*ast.FuncDecl`, flagging any whose `Name.Name` is in the banned set.
  Methods count: a `*ast.FuncDecl` with a receiver is matched the same way, which is what catches a re-declared `standaloneDefaultPlanDir`.
  Match on the AST rather than on raw text so a doc comment naming a function cannot trip it, and fail with a message naming the file, the declaration, the package, and the Cliwire Sole-Wiring Invariant.

  Skip `_test.go` files: the invariant is about production wiring, and a test helper is not a second copy of it.
  The test spawns no process and carries no build tag.
- **Commit:** `test(cliwire): ban re-declared wiring helpers in webstercli and burlercli`

### Card 10: the Cliwire Sole-Wiring Invariant and the module tree entry

- **Context:**
  - `internal/cliwire/doc.go`
  - `internal/cliwire/callerset_enforcement_test.go`
  - `internal/cliwire/bannedecl_enforcement_test.go`
  - `_mill/discussion.md`
- **Edits:**
  - `CONSTRAINTS.md`
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add a new section to `CONSTRAINTS.md` named `## Cliwire Sole-Wiring Invariant`, placed after `## Told-Geometry Invariant` so it sits beside the invariant it extends.
  Follow the file's existing shape exactly — a one-or-two-sentence statement of the FORM, then a short bullet list of the specific rules — and its semantic-line-break convention.
  The invariant states that `internal/cliwire` is the sole owner of standalone/hub CLI wiring resolution for standalone-capable CLIs;
  that a `<module>cli` never re-implements `--target-dir` resolution, the repository-root lift, mode-derived state/plan/stencils resolution, the nested-geometry guard, or the durable-sink redirect, but declares its own `cliwire.Module` descriptor and calls in;
  that `internal/cliwire` is the only **production** caller of `standalonestate.Derive`, while test files may call it to build fixtures and to assert the real derivation;
  and that both halves are enforced by tests in `internal/cliwire`.

  In the same file's `## Told-Geometry Invariant`, add `cliwire` to the bound-packages bullet's list.
  Do not change that invariant's other bullets — in particular its `internal/hubgeom`/`internal/standalonegeom` sole-constructor rule and its `NewDetachedRunner` clause both stay exactly as they are, since `cliwire` constructs no `Geometry` and standalone runners are still built from each CLI's own wiring.

  In `docs/overview.md`, add one entry for `internal/cliwire` to the module tree, immediately after the `internal/standalonegeom/` line and before `internal/preflightshed/`, matching the surrounding lines' `├── internal/<name>/` prefix, column alignment and one-line-description style.
  The description should name it as the shared standalone/hub wiring resolver for the standalone-capable CLIs, the layer that runs after `preflight.ResolveMode` has chosen a mode.
  Also extend the prose paragraph that names `internal/preflight`, `internal/hubgeom` and `internal/standalonegeom` as the precondition-and-geometry layer so it mentions `internal/cliwire` as the CLI-boundary resolver that sits between mode selection and the geometry builders.

  Do not add a file under `manifest/designs/` and do not touch `manifest/roadmap.md`.
  Use semantic line breaks in every prose line written into either file: one sentence per line, with an additional break at an internal independent-clause boundary in a long sentence, and never a fixed-column hard wrap.
- **Commit:** `docs(cliwire): record the Cliwire Sole-Wiring Invariant and the module entry`

## Batch Tests

`verify:` is the same two-command pair every batch runs, and for this batch the unscoped untagged half is not merely justified but load-bearing: both cards 8 and 9 add tests that parse every `.go` file under `internal/` and `cmd/`, so their verdict is a property of the whole tree.
A scoped run would still execute them (they live in `internal/cliwire`), but the regression they guard against is by definition a change made somewhere else, and only the full run proves the rest of the tree still compiles and passes alongside them.
This is also the same command as the hub's configured `pipeline.done_gate`.

The tagged half stays for the same reason as in the earlier batches: `internal/standalonegeom/reedgeom_symlink_integration_test.go` is one of the `Derive` call sites card 8's header comment enumerates, and it only runs under the `integration` tag.

New coverage in this batch is `internal/cliwire/callerset_enforcement_test.go` and `internal/cliwire/bannedecl_enforcement_test.go`.
Card 10 has no runnable surface of its own — it edits two markdown files — and is covered by the same gate only in the sense that it must not break the build;
its correctness is a review question, not a test question.
