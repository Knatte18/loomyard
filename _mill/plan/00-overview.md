# Plan: pattern told-geometry

```yaml
task: "pattern told-geometry"
slug: "pattern-told-geometry"
approved: true
started: "20260818-060111"
parent: "standalone-producers"
root: ""
verify: null
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: pattern told geometry
    file: 01-pattern-told-geometry.md
    depends-on: []
    verify: go test ./internal/pattern/... ./internal/burlerengine/... ./internal/websterengine/... ./internal/loomengine/... ./cmd/lyx/...
```

## Shared Decisions

_Cross-cutting decisions every batch inherits: naming conventions, error-handling posture, test frameworks, style/lint constraints.
One subsection per decision._

### Decision: told-value-is-anchorpath

- **Decision:** `pattern.Directive` takes `anchorPath string` as its first parameter, and every one of the four production call sites passes `l.AnchorPath()` computed inline.
  No `internal/hubgeom` helper, no new exported constructor, no stored struct field.
- **Rationale:** `FileHere(l)` is `File(filepath.Join(l.WorktreePath(), l.AnchorRel))`, which is `File(l.AnchorPath())` — an equality currently pinned by `TestFileHere_EqualsFileOfAnchorPath`.
  A single string parameter has nothing for a geometry struct to carry, and `internal/hubgeom` exists for multi-field conversions (T3's `ReedGeometry`), not for one accessor call.
- **Applies to:** all batches

### Decision: empty-string-guard-placement

- **Decision:** `Directive` keeps an explicit early return for `anchorPath == ""`, positioned exactly where today's `if l == nil` return sits, before the `isActive` call.
  `isActive` is never handed the empty string and gains no sentinel branch of its own.
- **Rationale:** `File("")` is `filepath.Join("", "_lyx", "PATTERN.md")` = the *relative* path `_lyx/PATTERN.md`, which `os.Stat` resolves against the process working directory.
  Guarding in `Directive` preserves today's control-flow shape one-for-one and keeps the `("", nil)`-without-a-read contract inside the function whose doc comment states it.
- **Applies to:** all batches

### Decision: engine-and-cli-signatures-stay-put

- **Decision:** `burlerengine.New`, `websterengine.RenderRecoveryPrompt`, `websterengine.RenderMasterPrompt` and `loomengine.PlanSpec` keep their `*lyxcwd.Location` parameters.
  No `anchorRoot` field is added to `burlerengine.Engine`.
- **Rationale:** T6 and T7 of `manifest/designs/producers-standalone.md` own those signature conversions, and T4 sits in an earlier wave precisely so file contention on `internal/burlerengine/engine.go` and `internal/websterengine/render.go` is serialised rather than merged.
- **Applies to:** all batches

### Decision: nil-guard-is-not-re-created-at-the-call-sites

- **Decision:** The defensive nil-`Location` guard disappears with the signature and is not re-created at any call site.
- **Rationale:** All four call sites already dereference `l` one to six lines before the `Directive` call — `e.layout.WorktreePath()` at `engine.go:97`, `l.HubPath` at `render.go:173` and `render.go:236`, `layout` at `plan.go:67-68` — so a nil `Location` panics in every one of those functions regardless of what `internal/pattern` does.
  A re-created guard would add a branch that changes no observable outcome.
- **Applies to:** all batches

### Decision: lyxcwd-leaves-the-package-including-its-tests

- **Decision:** `internal/lyxcwd` is removed from every file in `internal/pattern`, test files included.
- **Rationale:** `TestLeafInvariant_AllowlistOnly` skips `_test.go` files, so a surviving test import would pass green while keeping the dependency alive in `go list` terms and re-seeding it for the next editor of the file.
- **Applies to:** all batches

### Decision: transposition-detectors-are-per-call-site

- **Decision:** Each of the four call sites needs its own behavioural detector; a fixture in one package proves nothing about another package's argument order.
- **Rationale:** After the change both leading parameters of `Directive` are `string`, so a transposed `Directive(stencilsDir, anchorPath, role)` compiles cleanly and fails only at runtime, silently, as an inactive PATTERN.
  `render.go:174` is covered by `template_test.go`'s "PATTERN active" sub-test, `render.go:237` by `TestRenderMasterPrompt_MissingPatternStencilErrors`; `plan.go:71` and `engine.go:103` gain their detectors in this plan.
- **Applies to:** all batches

### Decision: no-bare-lyx-token-in-go-source

- **Decision:** Every doc-comment and code path in `internal/pattern` keeps building `_lyx` from `lyxdirs.LyxDirName` rather than a bare `"_lyx"` literal.
- **Rationale:** `internal/lyxcwd/enforcement_test.go`'s `TestEnforcement_GeometryLiterals` polices bare geometry tokens by whole-token equality and cannot see `"_lyx/PATTERN.md"` as a token, so this is a review obligation `doc.go` itself records.
  A doc rewrite must not weaken it.
- **Applies to:** all batches

## All Files Touched

- `CONSTRAINTS.md`
- `cmd/lyx/constructoranchoring_test.go`
- `internal/burlerengine/engine.go`
- `internal/burlerengine/engine_test.go`
- `internal/loomengine/plan.go`
- `internal/loomengine/plan_test.go`
- `internal/pattern/doc.go`
- `internal/pattern/leaf_enforcement_test.go`
- `internal/pattern/pattern.go`
- `internal/pattern/pattern_test.go`
- `internal/pattern/patternpath_test.go`
- `internal/websterengine/render.go`
- `internal/websterengine/template_test.go`
