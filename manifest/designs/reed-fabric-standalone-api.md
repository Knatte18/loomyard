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
