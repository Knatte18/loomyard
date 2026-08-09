# Plan: finalize: fold Raddle into its own contract and repair the dead links in raddle.md, finalize.md and self-report.md

```yaml
task: 'finalize: fold Raddle into its own contract and repair the dead links in raddle.md, finalize.md and self-report.md'
slug: raddle-finalize-fold-and-link-repair
approved: false
started: '20260809-123122'
parent: main
root: ""
verify: null
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: raddle-fold-and-link-guard
    file: 01-raddle-fold-and-link-guard.md
    depends-on: []
    verify: go test ./internal/lyxcwd/
```

## Shared Decisions

### Decision: one batch, because the guard and the repairs are one unit

- **Decision:** the whole task is a single batch.
  The enforcement test and the 11 link repairs are mutually dependent — the test's red step is defined against the pre-repair repo state (19 breaks), and the repairs' acceptance criterion is the test going green.
  Splitting them across two batches would force one of the two orderings to be wrong: test-first would leave batch 1's `verify:` gate red, and repairs-first would destroy the red step's known-good expected set.
- **Rationale:** `verify:` runs at the batch boundary, not per card, so a single batch lets the cards follow `_mill/discussion.md`'s stated TDD sequence (`## Testing`, "The natural sequence") while still presenting a green gate to mill-go.
- **Applies to:** all batches

### Decision: mid-batch commits are deliberately red; the batch gate is the green boundary

- **Decision:** cards 2 and 3 commit a test that fails (19 breaks, then 11 breaks).
  That is intended, not an oversight.
  Green is restored by cards 5–9 and asserted once by the batch `verify:`.
- **Rationale:** the discussion's red step exists to validate the checker against `.scratch/linkcheck.py`'s known 19-problem output before any repair moves the target.
  That validation is only possible pre-repair.
  The task branch is squash-merged by `/mill-merge`, so the red intermediate commits never reach `main` as separate commits.
- **Applies to:** all batches

### Decision: `CONSTRAINTS.md` lands in its own card, satisfied at squash-merge

- **Decision:** the `## Markdown Link Integrity` section is card 9, a separate commit from the test cards.
- **Rationale:** `CLAUDE.md` requires a new cross-cutting invariant to be recorded in `CONSTRAINTS.md` "in the same commit" as the infrastructure.
  mill commits per card, and `/mill-merge` squash-merges the branch, so the invariant and its enforcing test do arrive in `main` as one commit.
  Forcing them into one card would mean writing a ~350-line Go test and a CONSTRAINTS section as one indivisible unit, which is worse for review granularity and buys nothing.
- **Applies to:** all batches

### Decision: the checker is factored into pure helpers plus one scan driver

- **Decision:** `internal/lyxcwd/docslink_test.go` exposes four unexported test-scope symbols — `docsLinkSlug(string) string`, `docsLinkHeadingAnchors([]byte) map[string]bool`, `docsLinkExtract([]byte) []docsLink`, and `docsLinkScan(t, repoRoot string, roots []string, allow map[docsLinkKey]string) (breaks []docsLinkBreak, unmatched []docsLinkKey)`.
- **Rationale:** `_mill/discussion.md`'s "Fixture seam — decided, because the obvious choice silently does nothing" requires the grammar and slug scenarios to be table tests over data, never a filesystem tree, because `walkEnforcementRoots` skips any directory whose name contains `testdata` and a `testdata/` fixture would pass vacuously.
  `docsLinkScan` takes `repoRoot` as an ordinary parameter so a `t.TempDir()` root composes with it directly.
- **Applies to:** all batches

### Decision: stdlib-only imports in the new test file

- **Decision:** `docslink_test.go` imports stdlib only.
- **Rationale:** `internal/lyxcwd`'s Leaf Invariant (`CONSTRAINTS.md:56`, enforced by `internal/lyxcwd/leaf_enforcement_test.go`) caps *production* imports at stdlib + `internal/gitexec`.
  A test file is outside that cap, but the checker needs nothing beyond stdlib, so there is no reason to spend the exemption.
- **Applies to:** all batches

### Decision: semantic line breaks in every edited `.md`

- **Decision:** every markdown line this batch writes or rewrites follows `CLAUDE.md`'s semantic-line-break rule — one sentence per line, plus a break at an internal independent-clause boundary in a long sentence, with plain newlines (never trailing double-spaces or a backslash).
  Table cells and blockquotes stay on one line.
- **Rationale:** `CLAUDE.md` applies the rule to every `.md` in the repo, not only to newly-written ones, and this batch rewrites existing prose in six files.
- **Applies to:** all batches

### Decision: `manifest/roadmap.md`, `shed.md`, `loom.md`, `docs/overview.md`, and `docs/reference/*` are untouchable

- **Decision:** no card edits any of those files, even to fix a break the new test reports.
  Their breaks are allowlisted instead.
- **Rationale:** `_mill/discussion.md`'s `## Scope` "Out" section — each is inside a multi-owner chain in the `shed-producer-model-scoping` follow-up set, and editing one here recreates exactly the shared-file collision that forced task E to be serialized.
- **Applies to:** all batches

## All Files Touched

- `CONSTRAINTS.md`
- `docs/shared-libs/README.md`
- `internal/lyxcwd/docslink_test.go`
- `manifest/designs/finalize.md`
- `manifest/designs/raddle.md`
- `manifest/designs/self-report.md`
- `manifest/designs/semantic-index.md`
- `manifest/designs/webster-parallel-execution.md`
