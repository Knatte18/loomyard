# Batch: overview-docs

```yaml
task: "invariants and docs for the told-geometry rule"
batch: "overview-docs"
number: 4
cards: 3
verify: go test ./internal/lyxcwd/...
depends-on: [1]
```

## Batch Scope

This batch makes the three targeted edits to `docs/overview.md` that the told-geometry work requires: an accuracy sentence plus a pointer in its own Cwd Resolution Invariant section, a new sentence in `## Modules` mapping the three packages absent from the module map, and a standalone-mode paragraph in the Execution stack section.
It is one batch because all three land in one file and share one guard.

It depends on batch 1 because two of the three cards link to `../CONSTRAINTS.md#told-geometry-invariant`, and `internal/lyxcwd/docslink_test.go` fails the build on a link whose anchor does not resolve.
Batch 1 is what creates that anchor.

**The one trap in this batch:** `internal/lyxcwd/docslink_test.go` carries a self-expiring `docsLinkAllowlist` whose first entry keys this very file, `docs/overview.md`, with target `../CONSTRAINTS.md#package-naming`.
An allowlist entry whose key matches no break in a scan is reported as *deletable*, which is a test **failure**, not a pass.
So that link must be left exactly as it is — incidentally repairing or removing it while editing this file strands the entry and fails the build.
For the same reason in reverse, do not add a `## Hub geometry invariants` heading to this file: the allowlist's second entry keys `manifest/designs/loom.md` with target `../../docs/overview.md#hub-geometry-invariants`, and creating that anchor would strand that entry too.

`docs/shared-libs/README.md` is explicitly out of scope for this batch and this task.
Its own package list is missing the same three packages, and that looks like the same gap but is not — that file documents *shared libraries*, things the modules sit **on**, and all three of these sit **above** the engines.
Adding them there would repeat a false-layering error in a second file rather than fix a real omission.

## Cards

### Card 12: the Cwd Resolution Invariant section — accuracy sentence and pointer

- **Context:**
  - `CONSTRAINTS.md`
  - `internal/lyxcwd`
  - `internal/lyxcwd/docslink_test.go`
- **Edits:**
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In the `## Cwd Resolution Invariant` section, add an accuracy sentence stating what `lyxcwd.Resolve` proves and — the load-bearing half — what it does not: it proves cwd is the root of a git worktree and nothing more.
  It succeeds in any ordinary git repository run from its root, and the `HubPath` and `RepoName` it returns are fiction in that case.
  Proving a worktree is lyx-initialized and Fabric-wired is a different layer's job.

  Add a pointer, immediately after, to `CONSTRAINTS.md`'s new Told-Geometry Invariant for the tier map, written as an inline markdown link with the target `../CONSTRAINTS.md#told-geometry-invariant`.
  That target's file part and anchor must both resolve — the anchor slug is fixed by batch 1's heading text.

  Place the two sentences where a reader meets them before the per-token ownership detail: after the paragraph describing the three-operation contract and the `Location` fields, and before the "**Raw `os.Getwd` and `git rev-parse --show-toplevel` are banned**" paragraph.

  Do not touch the existing link to `../CONSTRAINTS.md#package-naming` anywhere in this file — it is a known-broken link held by a self-expiring allowlist entry in `internal/lyxcwd/docslink_test.go`, and repairing or removing it fails the build.
  Do not touch the existing `See [CONSTRAINTS.md](../CONSTRAINTS.md) for details.` line that closes the section.
- **Commit:** `docs(overview): state what lyxcwd.Resolve proves and point at the Told-Geometry Invariant`

### Card 13: the `## Modules` map — a separate sentence, never an entry in the parenthetical

- **Context:**
  - `CONSTRAINTS.md`
  - `internal/preflight`
  - `internal/preflight/predicates.go`
  - `internal/preflight/preflight.go`
  - `internal/hubgeom`
  - `internal/standalonegeom`
  - `internal/fabricengine`
  - `docs/shared-libs/README.md`
- **Edits:**
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  At the end of the `## Modules` section sits a sentence beginning "The user-facing modules sit on a thin layer of shared infrastructure", carrying a long parenthetical package list and a link to the shared-libs README, followed by a sentence about `internal/pattern`.

  Add a **separate new sentence after** that shared-infrastructure sentence, mapping `internal/preflight`, `internal/hubgeom`, and `internal/standalonegeom` as the precondition-and-geometry layer that sits **above** the engines, with an inline markdown link to `../CONSTRAINTS.md#told-geometry-invariant`.
  Describe `internal/preflight` as the tier-1/tier-2 precondition layer and the two geom packages as the hub-mode and told-mode constructors of the geometry the engines are handed.

  Do **not** add any of the three to the shared-infrastructure parenthetical.
  That sentence describes a thin layer the user-facing modules sit **on**, and all three sit the other way round: `internal/preflight` imports `internal/fabricengine`, `internal/hubgeom` imports the engines, and `internal/standalonegeom` imports the same engines.
  Listing them there would state a layering the import graph contradicts, and `internal/standalonegeom` is no exception — it is an engine-importing adapter exactly as `internal/hubgeom` is.

  Verify that import direction against the tree before writing, so the sentence states a fact rather than an assumption.

  Leave the `internal/pattern` sentence that follows untouched, and leave the shared-libs README link untouched.
- **Commit:** `docs(overview): map preflight and the two geom adapters as the layer above the engines`

### Card 14: the Execution stack — a standalone-mode paragraph

- **Context:**
  - `CONSTRAINTS.md`
  - `internal/hubgeom`
  - `internal/standalonegeom`
  - `internal/preflight`
- **Edits:**
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `## Execution stack (orchestration layers)`, add a paragraph stating that the stack now has two entry modes, which the section currently describes as one.

  The paragraph states three things:
  every layer from `reed` up is told its geometry rather than deriving it;
  `internal/hubgeom` and `internal/standalonegeom` are the two constructors that tell it, in hub mode and told mode respectively, with `preflight.ResolveMode` selecting between them at a standalone-capable CLI's pre-run;
  and the consequence a reader needs — a producer verb therefore runs in a directory that is not a git repository, with no hub, no fabric, and no orchestrator status seed.

  Link to `../CONSTRAINTS.md#told-geometry-invariant` for the rule.

  Place it after the fenced ASCII stack diagram and the "The whole stack runs **headless** (auto mode)" line, before the bulleted "**reed is three things, and it is built**" entry.
  Do not edit the fenced diagram itself — its contents are a layer map, not a mode map, and adding a mode column would make it unreadable.
- **Commit:** `docs(overview): describe the standalone entry mode in the execution stack`

## Batch Tests

`verify: go test ./internal/lyxcwd/...` runs `TestEnforcement_MarkdownLinks`, which is the real gate for this batch and the reason the batch depends on batch 1.
`docs/overview.md` is a link **scan source** under that guard, so every inline markdown link this batch adds has its file part and its `#anchor` resolved — a mistyped `#told-geometry-invariant` slug fails the build immediately rather than rotting silently.
The same test enforces the self-expiring `docsLinkAllowlist` contract, so it also catches the trap named in this batch's scope: if the `../CONSTRAINTS.md#package-naming` link is incidentally repaired or removed, its allowlist entry becomes unmatched and the test reports it as deletable, failing.

The package's `TestEnforcement_FabricVocabulary` does **not** reach `docs/overview.md` — its `.md` walk covers `internal/**/*.md` and `contracts/stencils/**/*.md` only.
Fabric vocabulary in this file is therefore a review obligation: say Fabric for the wired composite, and use warp/weft only where the two sides genuinely must be told apart.

No Go file is touched, so the overview's module-wide `verify: go build ./...` is a no-op here.
