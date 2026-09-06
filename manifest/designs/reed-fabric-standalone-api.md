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
