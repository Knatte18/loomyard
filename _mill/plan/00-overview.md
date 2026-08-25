# Plan: loom: Discussion-Burler Fabric Git Invariant fix

```yaml
task: 'loom: Discussion-Burler Fabric Git Invariant fix'
slug: 'loom-discussion-burler-fix-scope'
approved: true
started: '20260825-140240'
parent: 'loom-webster-review-producer'
root: ""
verify: null
skip_checks: ["verify-full-suite"]
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: fabric-git-invariant-fix
    file: 01-fabric-git-invariant-fix.md
    depends-on: []
    verify: go test ./... && go test -tags integration ./...
```

## Shared Decisions

_Cross-cutting decisions every batch inherits: naming conventions, error-handling posture, test frameworks, style/lint constraints.
One subsection per decision.
Batch-local decisions live in each batch file._

### Decision: one-batch-because-the-change-is-one-recipe-value-and-its-fallout

- **Decision:** The whole task is a single batch of three cards.
- **Rationale:** The change is two recipe values plus the comments, tests, and docs that assert something about them.
  Every file in the task is small and every card shares the same handful of `Context:` entries, so splitting would push well past the "adjacent batches sharing >80% of their Context" merge rule.
- **Applies to:** all batches

### Decision: card-1-is-atomic-and-cannot-be-split

- **Decision:** The recipe flip, the recipe comment rewrites, `internal/loomrecipe/sequence_test.go`'s count change, and the new guard test all land in one card and therefore one commit.
- **Rationale:** Flipping the `Discussion-Bouncer` row to carry a commit seam changes the shipped runtime behaviour the moment it lands: the segment's approved settle now reaches `Env.CommitDiscussion`, so `sequence_test.go`'s existing `commitDiscussionCalls != 1` assertion fails on that same commit unless it moves with it.
  The guard test likewise fails against the unflipped recipe by construction — that failure is the point of writing it, but it must not be committed red.
  Per the plan-format rule that two changes which must land together are one card rather than two cards linked by a "same commit" note, they are one card.
- **Applies to:** fabric-git-invariant-fix

### Decision: tdd-proof-is-an-execution-step-not-a-red-commit

- **Decision:** The "watch the guard fail first" proof `_mill/discussion.md` calls for is performed during implementation of card 1, before the recipe edit within that same card, and is never committed as a red tree.
- **Rationale:** Per-card commits gate on the batch `verify:` command, so a deliberately-failing commit would halt the run.
  Writing the guard, running it against the unmodified recipe, recording the two expected failures, and only then applying the flip gives the identical proof without a red commit.
- **Applies to:** fabric-git-invariant-fix

### Decision: no-python-prefix-on-verify

- **Decision:** The batch `verify:` command is `go test ./... && go test -tags integration ./...` with no `PYTHONPATH= ` prefix.
- **Rationale:** This is a Go repository, so the native test runner is used directly per the plan-format rule that the `PYTHONPATH= ` isolation prefix applies to Python/mill projects only.
  The repo-wide sweep is deliberate rather than a scoping oversight: the recipe is embedded into the binary by `//go:embed` and consumed repo-wide, so a scoped run over the three changed packages would not catch a consumer elsewhere in the tree.
  This is the same shape as the already-configured `pipeline.done_gate`.
- **Applies to:** all batches

### Decision: roadmap-item-is-split-not-wholly-moved

- **Decision:** `manifest/roadmap.md`'s Planned item for this task moves to `## Done` covering the `fix-scope`/`commit_seam` correction only.
  Its second paragraph — the two folded defects (both review segments resolving `_lyx` paths against `Env.WorktreeRoot` while their commit closures anchor at `AnchorPath()`, and neither segment clearing its Bouncer run directory on re-entry) — is lifted out into a new Planned item in the same `### loom: real LLM producers` sub-category rather than moving to Done with it.
- **Rationale:** `_mill/discussion.md` says to move the item Planned → Shipped, but its own `## Scope` section lists neither folded defect as in scope and its `## Out` section rules out the recipe and layout changes either would need.
  Moving the item whole would record two unfixed defects as shipped, which is the failure mode a roadmap exists to prevent.
  Splitting keeps the discussion's stated intent (this task completes a Planned item, so the item moves) without the false claim.
  Note also that the section is literally named `## Done`, not "Shipped" — the discussion's wording is informal, and the existing section name governs.
- **Applies to:** fabric-git-invariant-fix

### Decision: comment-only-and-docs-cards-are-separated-from-the-behaviour-card

- **Decision:** Card 2 (two Go doc comments) and card 3 (two manifest docs) are separate cards from card 1.
- **Rationale:** Neither carries behaviour, so neither is atomic with the flip — nothing in them can fail a test on card 1's commit.
  Keeping them separate makes the behaviour change reviewable as its own diff.
  The project's task-completion rule requires the docs to land in the same *task*, which three commits on one branch satisfy; it does not require one commit.
- **Applies to:** fabric-git-invariant-fix

### Decision: markdown-semantic-line-breaks

- **Decision:** Every prose edit to a `.md` file in this task uses one sentence per line, with additional breaks at internal independent-clause boundaries, and never a fixed-column hard wrap.
- **Rationale:** The repo's `CLAUDE.md` states this rule and applies it to every `.md` file in the repo, not only newly-written ones.
- **Applies to:** fabric-git-invariant-fix

## All Files Touched

_Full union of every `Creates:` / `Edits:` / `Moves:` **target** path across every batch, sorted alphabetically (Move **source** paths are excluded — they disappear, like `Deletes:` tokens).
Cards are the source of truth;
this section is the input `_plan_validate.py`'s `all-files-touched-mismatch` check cross-references against the derived union of every card's `Edits:`/`Creates:`/Move-target paths, to catch drift between the hand/agent-maintained list here and that derived union._

- `contracts/recipes/loom-recipe.yaml`
- `internal/loomcli/wiring.go`
- `internal/loomrecipe/overlay_seam_guard_test.go`
- `internal/loomrecipe/sequence_test.go`
- `internal/shedadapters/bouncer_commit_test.go`
- `manifest/designs/loom.md`
- `manifest/roadmap.md`
