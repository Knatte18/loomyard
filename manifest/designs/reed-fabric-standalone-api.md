# Reed and Fabric as standalone modules: public API design

## What it is

This document answers, for two lyx modules — `internal/reedengine` (Reed) and `internal/fabricengine` (Fabric) — what each one's public contract would have to be for its repo to be usable by a non-loomyard consumer, and whether paying that price is worth it.
The deliverable is a design document about unbuilt work, not documentation of the shipped modules — the shipped modules' own design lives in their own package doc comments (`internal/reedengine/doc.go`, `internal/fabricengine/doc.go`).

## How the numbers in this document were produced

Every figure in this document was measured in this worktree at commit `ca388c0ce`, using `go list`, `go doc` and `grep`.
Each figure is stated alongside the command that produced it.
One caveat applies to every grep-derived count in this document: a naive grep over a package name also matches doc-comment prose, not just code, and two figures in this task's own first draft were wrong for exactly that reason.
Every grep-derived count in this document therefore means "referenced in code by production packages" — excluding `_test.go` files and doc comments — unless stated otherwise.

## Disposition of this document

This document lands with no owning entry in `manifest/roadmap.md`, **by design**: its content is the evidence needed to decide whether any roadmap entry is warranted at all, so writing the entry first would presuppose the verdict this document exists to reach.
The follow-up decision has two legitimate outcomes: one or more Someday entries are added pointing at this document, or none is.
In the second case, this document is **deleted** rather than left orphaned.
That deletion trigger is named here explicitly, so a later reader of an entry-less designs file under `manifest/designs/` knows the state is intentional and knows what closes it.

This document's lifecycle classification follows the [docs/overview.md](../../docs/overview.md#documentation-lifecycle) Documentation Lifecycle rule: `manifest/designs/<module>.md` is scoped to planned, not-yet-built work, deleted when the module lands or the plan is abandoned.
This document describes a possible future extraction and its prerequisites, not the shipped Reed and Fabric modules themselves — so it satisfies that rule even though both named modules are already shipped.

## The extraction rubric — the frozen contract is the exported type set

For a Go module, the extraction contract is the set of exported identifiers consumers reference in code — not an interface.
A consumer-side interface narrows *coupling* and buys testability, but it does not narrow that contract by one name.

The shipped example that grounds this: `shuttleengine.ReedOps` (`internal/shuttleengine/reed.go`) is held up as the model narrow seam.

```go
type ReedOps interface {
	AddStrand(spec reedengine.AddSpec) (reedengine.Strand, error)
	RemoveStrand(guid string, recursive bool) (reedengine.Removed, error)
	Status() (reedengine.StatusResult, error)
	SendText(guid, text string, submit bool) error
	SendKey(guid, key string) error
	CapturePane(guid string) (string, error)
}
```

Yet every one of its six methods takes or returns a type from the provider package — `reedengine.AddSpec`, `reedengine.Strand`, `reedengine.Removed`, `reedengine.StatusResult` — so a consumer behind this interface still pins four exported types from `reedengine`.

The consequence: the background framing that Fabric's blocker is "no interface seam anywhere on the consumer side" misdiagnoses the problem.
Adding fifteen consumer-side interfaces to Fabric's consumers would not shrink its 74-identifier contract by one name.
What shrinks a contract is deleting or unexporting identifiers, or moving them behind a façade that re-exports a curated subset.

Two metrics are rejected as the readiness measure, and each fails for a distinct reason:

- **Interface count** measures coupling and testability, not contract size — as `ReedOps` demonstrates above, an interface can exist and pin the same four types anyway.
- **Raw importer count** measures the wrong axis of "extractable": Reed and Fabric have nearly identical transitive-dependent counts, 26 and 27 respectively, and are nowhere near equally extractable — Fabric's contract and size dwarf Reed's, as the per-module sections below measure.

## Four corrections to the task's background notes

This investigation found four background claims to be false.
Each is recorded here as a correction, with its evidence, because the doc's value depends on being checkable, and because a naive re-check of any of these four is the obvious way for a later reader to get the same claim wrong again.

1. **The `gitkit`/`gitrepo`/`gitexec` "trio" framing is wrong.**
   `internal/gitkit` is not in Fabric's dependency set at all, transitively or directly — confirmed by `go list -deps ./internal/fabricengine`, which does not list it.
   Its only non-test production importer anywhere in the repo is one file, `internal/hubforge/seed.go`; every other reference to `gitkit` is from a `_test.go` file, which is what makes it test-fixture machinery.
   The `gitkit` Leaf Invariant (`CONSTRAINTS.md`) is therefore untouched by anything in this document.

2. **Fabric's cwd coupling is structural, not incidental.**
   True self-resolution of cwd is only two sites (`clone.go:403`, `unwire.go:53`, both calling `lyxcwd.Resolve(cwd)`).
   But six further sites re-derive a `Location` from a stored path via `lyxcwd.ResolveWorktree` (`commit.go:138`, `mergelifecycle.go:199`, `merge.go:121`, `pull.go:456`, `warplayout.go:25`, `worktreelist.go:130`).
   That makes the `lyxcwd` coupling structural typing across the package, not an incidental parameter on a couple of entry points.

3. **The missing-interface framing misdiagnoses Fabric's blocker.**
   See the extraction rubric section above: adding interfaces on the consumer side narrows coupling, never contract size, so "no interface seam anywhere on the consumer side" is not what stands between Fabric and extraction.

4. **`internal/lyxcwd` does not import `internal/fabricengine`.**
   A naive `grep -rl "internal/fabricengine"` lists two files under `internal/lyxcwd` (`lyxcwd.go`, `anchor.go`), but both matches are doc-comment prose, not import statements.
   `go list -deps ./internal/lyxcwd` confirms that package's only internal import is `internal/gitexec`, exactly as the Cwd Resolution Invariant requires.
   This correction is recorded precisely because it is the obvious thing a reader would suspect and the obvious way to get it wrong — the same comment-prose-contamination trap the measurement-method section above already named.

## The quarry precedent — this project's one completed extraction

This project has one completed extraction to learn from: `lyx scout` became the standalone `github.com/Knatte18/quarry` repo, cited here by module path as the reference.

**Layout.**
Quarry's shipped shape is a root façade package (`quarry/`), a separately-importable cgo-free leaf (`glyph/`), an `internal/` tree holding the CLI among other packages (`internal/{cgoguard,cli,engine,gitsrc,mcpserver,repopath}`), and its `cmd/` binaries (`cmd/quarry`, `cmd/quarry-mcp`).

**Façade mechanism.**
The façade re-exports internal types by alias rather than by hand-designed interface, so the extraction never required designing a narrow seam up front — it curated *what* is exported rather than *how*:

```go
type Symbol = engine.Symbol
const KindFunction = engine.KindFunction
```

The façade never had to invent a narrower method set than the engine already had, only decide which of the engine's types the façade's own names re-export.

**The evidence on extraction timing, stated plainly.**
This repo's `go.mod` still carries no requirement on `github.com/Knatte18/quarry`, and adoption is a single unstarted item in `manifest/roadmap.md`'s Planned section ("Adopt quarry's glyph alphabet as the plan alphabet").
Quarry shipped as a standalone repo months before this document was written, and loomyard — the only real candidate consumer — has still not drawn the dependency edge to it.
That is the measured cost of extracting before a consumer exists, and it is the one piece of evidence this document weighs most heavily against extracting Reed or Fabric now.

**Residue cost.**
The port's own record, `docs/research/quarry-holistic-fix-log.md`, documents real cleanup cost after the port: loomyard-internal references surviving in ported comments, a stale checksum file (`go.sum`), and cross-repo review rounds needing a two-repo authorization decision.
None of this is free, even for a clean extraction candidate.

**A caveat on this section's own verifiability.**
The layout and façade observations above were read off a local checkout of the quarry repo, not from anything in this repository.
That local-checkout read is the one block of evidence in this document no other reader can re-verify from this repo alone — it is not backed by a machine-local absolute path, and no claim in this document rests on one; a reader who wants to confirm the layout or façade shape must clone `github.com/Knatte18/quarry` and look themselves.

## Reed — the measured contract today

**Size.**
`internal/reedengine` is 6,615 production lines (17,474 with tests, measured by `wc -l` over non-`_test.go` files).
`internal/reedengine/render` adds 773 production lines (1,893 with tests).

**Direct internal imports.**
`configengine`, `lock`, `logger`, `lyxdirs`, `proc`, `shell`, `state`, `tokenvocab`, `reedengine/render` — from `internal/reedengine/doc.go` and `go list`.

**Transitive internal dependency set.**
`go list -deps ./internal/reedengine` adds `envsource`, `gitexec`, `lyxcwd`, `yamlengine`, `stencil`, `fsx` to the direct set above.
`lyxcwd` and `gitexec` enter *only* through `internal/logger` (`internal/logger/sink.go:21-22`, `:88-93`, where `LogsDir` takes a `*lyxcwd.Location` and `ensureDurableSink` falls back to `lyxcwd.Getwd()`/`Resolve()`).
`reedengine/doc.go` already documents this honestly and is quoted rather than re-derived: reed is told its geometry and derives none of it, so `internal/lyxcwd` is absent from reed's *direct* production imports even though it is present transitively.

**Public surface.**
7 free functions, 20 types, and `*Engine` with **17** exported methods and no value-receiver methods — `Socket`, `SessionName`, `TmuxPath`, `AddStrand`, `UpdateStrand`, `RemoveStrand`, `AttachArgv`, `SendText`, `SendKey`, `CapturePane`, `Up`, `Resume`, `Down`, `Status`, `Watch`, `HeaderText`, `ValidateHeader`.
Producing command: `go doc ./internal/reedengine Engine | grep -c '^func (e \*Engine)'`.

**External contract footprint.**
**13 distinct exported package-level identifiers** are referenced *in code* by production packages outside `reedengine`: `AddSpec`, `ConfigTemplate`, `Engine`, `Geometry`, `LoadConfig`, `LoadState`, `New`, `Removed`, `ServerName`, `SessionName`, `StatusResult`, `Strand`, `StrandStatus`.
The metric's definition is exactly that phrase — "referenced in code by production packages outside the module" — and it is stated here alongside the number because the same shape of count appears throughout this document.
This footprint counts `render`'s own exported names (`Display`, `Strand`, `Box`, `Params`, and the `Anchor` constants) and the 17 methods reached through `*Engine` separately from the 13 above, not folded into it.

A bare `grep -ro 'reedengine\.[A-Za-z0-9_]*'` over non-test files additionally returns three identifiers, none of which is a real external reference: `CleanClaudeEnv` and `AddStrand` appear only as doc-comment prose (`internal/burlerengine/doc.go:205`, `internal/reedcli/add.go:1,4`), and `requireSessionLocked` (`internal/reedcli/attach.go:52`) is not even exported.
Counting these three is how the first draft of the underlying investigation reached 15 external identifiers instead of 13.

**Direct production importers (9) and their consumer shapes.**
`burlercli`, `configreg`, `hubgeom`, `loomcli`, `reedcli`, `shuttlecli`, `shuttleengine`, `standalonegeom`, `webstercli` — transitive dependents: 26 packages.
Per-importer shape:

- `webstercli` already holds Reed behind the transport interface — `shuttleengine.ReedOps` (`internal/webstercli/wiring.go:220,226`).
- `burlercli` and `shuttlecli` construct and hand off: each calls `LoadConfig`+`New` and hands the engine to `shuttleengine.NewRunner`.
- `hubgeom` and `standalonegeom` build the told-geometry value: `Geometry` (plus `ServerName`/`SessionName`).
- `configreg` calls a config-template function only: `ConfigTemplate`.
- `loomcli` is the sole consumer *retaining* a concrete `*reedengine.Engine` as a struct field (`internal/loomcli/cli.go:44`), using `Up`, `Status`, `AddStrand`, `RemoveStrand`, `TmuxPath` and `AttachArgv` (`internal/loomcli/run.go:145-320`, `drive.go:64`).
- `reedcli` is Reed's own CLI and legitimately uses the whole surface.

**Geometry.**
`reedengine.Geometry` (`internal/reedengine/geometry.go`) is eight told string fields with a documented no-validation, no-derivation contract.
Its two constructors — `hubgeom.ReedGeometry(*lyxcwd.Location)` and `standalonegeom.ReedGeometry(target, stateDir, hash8)` — would neither move into a standalone Reed, since both are hub/standalone-layout tellers, not Reed's own concern.

**The one provider-specific name.**
`reedengine.CleanClaudeEnv` (`internal/reedengine/env.go`) is the one provider-specific name in Reed's public API, and it has no caller outside its own package (`internal/reedengine/lifecycle.go:299`).

```text
internal/reedengine/doc.go
internal/reedengine/env.go, lifecycle.go:299
internal/reedengine/geometry.go
internal/loomcli/cli.go:44
```
