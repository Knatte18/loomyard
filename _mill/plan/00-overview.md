# Plan: fabric: one ownership-and-dirtiness gate for all destruction (slice 12)

```yaml
task: 'fabric: one ownership-and-dirtiness gate for all destruction (slice 12)'
slug: 'fabric-destructive-chokepoint'
approved: false
started: '20260810-124739'
parent: 'main'
root: ""
verify: go build ./...
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: dirtiness-probe
    file: 01-dirtiness-probe.md
    depends-on: []
    verify: go test ./internal/fabricengine/...
  - number: 2
    name: the-gate
    file: 02-the-gate.md
    depends-on: [1]
    verify: go test ./internal/fabricengine/...
  - number: 3
    name: path-callsites
    file: 03-path-callsites.md
    depends-on: [2]
    verify: go test ./internal/fabricengine/... && go test -tags integration ./internal/fabricengine/...
  - number: 4
    name: clone-callsites
    file: 04-clone-callsites.md
    depends-on: [2]
    verify: go test ./internal/fabricengine/... && go test -tags integration ./internal/fabricengine/...
  - number: 5
    name: branch-callsites
    file: 05-branch-callsites.md
    depends-on: [3]
    verify: go test ./internal/fabricengine/... && go test -tags integration ./internal/fabricengine/...
  - number: 6
    name: guard-and-docs
    file: 06-guard-and-docs.md
    depends-on: [4, 5]
    verify: go test ./cmd/lyx/... ./internal/lyxcwd/...
  - number: 7
    name: gap-integration-tests
    file: 07-gap-integration-tests.md
    depends-on: [4, 5]
    verify: go test -tags integration ./internal/fabricengine/...
```

## Shared Decisions

_Cross-cutting decisions every batch inherits: naming conventions, error-handling posture, test frameworks, style/lint constraints.
One subsection per decision.
Batch-local decisions live in each batch file._

### Decision: the discussion document is the specification

- **Decision:** `_mill/discussion.md` is the authoritative specification for this slice.
  Every card below implements a decision already argued there;
  where a card and the discussion appear to disagree, the discussion wins unless this section names the divergence explicitly.
  Read the discussion's `## Decisions` and `## Technical context` sections before starting any batch.
- **Rationale:** the discussion went through five review rounds and its site enumeration was re-run against the tree and verified line-for-line during planning.
  Re-deriving any of it is wasted work and a source of drift.
- **Applies to:** all batches

### Decision: exact Go type shapes for the gate

- **Decision:** the discussion argues *what* the gate must model but leaves several Go-level spellings open.
  They are pinned here once so no batch invents its own:
  - `type destructiveCheck int` with `checkContainment`, `checkOwnership`, `checkDirtiness`, `checkForce`.
  - `type destructiveRefusal struct { Check destructiveCheck; What, Target, Reason string }` with an `Error() string` method, returned as `*destructiveRefusal`.
  - `type pathRequest struct { what, container, target string; slug *slugSpec; ownership pathOwnership; dirtiness pathDirtiness; force bool }` and `type branchRequest struct { what, repoDir, branch string; ownership branchOwnership; dirtiness branchDirtiness; force bool }`.
    `branchRequest` carries no `container` and no `target`, and it carries no `branchPrefix` either — the branch prefix is an input `ownedManagedBranch`'s predicate needs, so by the "each check's inputs travel with the check" rule it rides on that constructor as `ownedManagedBranch(l *lyxcwd.Location, branchPrefix string)` rather than as a bare request field.
  - `type pathOwnership struct` and `type branchOwnership struct` — two distinct types, each with unexported fields and a fixed set of constructor functions.
    `pathRequest.ownership` is a `pathOwnership`;
    `branchRequest.ownership` is a `branchOwnership`.
    This is what makes "a ref-shaped kind on a path request does not compile" true by construction rather than by test.
  - `type pathDirtiness struct` (constructors `dirtyScopeTracked()`, `dirtyScopeAll()`, `dirtinessNA(reason string)`) and `type branchDirtiness struct` (single constructor `dirtyCheckedOutBranch()`).
    Same shape-separation reason.
    A zero-value of either type is refused by the pipeline, which is what makes an omitted declaration fail loudly rather than silently pass.
  - `type slugSpec struct { name string; junctionNames []string }`, reached as `pathRequest.slug *slugSpec`, nil when the target is not slug-derived.
  - `type createdToken struct { path string; worktree bool }` — one type, a boolean discriminating a bare directory from a git worktree, so a directory token cannot satisfy `ownedFreshlyCreatedWorktree` or the reverse.
- **Rationale:** every one of these is a decision the discussion implies but does not spell;
  leaving them to the implementer invites two batches picking incompatible spellings.
- **Applies to:** all batches

### Decision: the token's unforgeability is enforced by the guard, not by the type system

- **Decision:** the discussion states that `createdToken` "is unexported and the gate is its sole minter", so "a site cannot declare this kind for a path the gate did not create".
  Inside a single Go package that is not true — any file in `package fabricengine` can write a composite literal for an unexported type.
  The property is therefore established the same way every other property in this slice is: the bypass guard's banned-token set gains `createdToken{`, with `destroy.go` already on its allowlist.
- **Rationale:** the claim is load-bearing (it is the entire answer to "what ownership kind does `teardownHub` declare"), so it must be enforced by something rather than asserted.
  A guard token is the mechanism the slice already uses for its other four unforgeable properties, and it costs one line.
- **Applies to:** batch 2, batch 6

### Decision: existing per-site refusal messages are kept, not replaced

- **Decision:** where the discussion says a gate ownership kind "subsumes the site's existing refusal" — `unwire.go`'s is-it-a-link check and `junction.go`'s unwire-sweep target comparison — the existing check and its existing error message stay exactly where they are.
  The gate call is added in front of the removal, not in place of the site's own guard.
- **Rationale:** those messages are observable behaviour, and the discussion's Out section permits behaviour changes only for the three named gaps.
  A duplicated check is a floor plus a nicer message, not a hole;
  deleting the site check to avoid duplication is a separate, later cleanup with its own regression risk.
- **Applies to:** batch 3

### Decision: how a refusal is surfaced on a best-effort path

- **Decision:** `destroy.go` exposes `surfaceRefusal(err error) error`, which returns `err` when `errors.As(err, new(*destructiveRefusal))` holds and `nil` otherwise.
  Every best-effort call site applies exactly one of two shapes:
  - a site that can return an error (`Remove`'s `_ = removePortal(...)`, `rollbackAdd`'s accumulator) wraps the call in `surfaceRefusal` and propagates a non-nil result.
  - a site that cannot return an error at all (`rollbackSwitch`, which is `void`) logs the refusal via `logger.Warn` and continues.
    Widening `rollbackSwitch`'s signature is out of scope.
- **Rationale:** the discussion fixes the policy ("a refusal is never discardable") but not its mechanics, and the `void` case has no propagation path at all.
  `logger.Warn` is this repo's existing answer for a non-fatal event that must not vanish, already used for the non-fatal hook-install failures in the same files.
- **Applies to:** batch 3, batch 4, batch 5

### Decision: commit granularity — per card here, one commit on `main`

- **Decision:** each card carries its own conventional-commit message and commits separately on the task branch, despite the discussion's `one-commit-for-the-slice` decision.
- **Rationale:** the task branch is squash-merged into `main` by `/mill-merge`, so the slice still lands on `main` as exactly one commit carrying gate, conversions, guard and all four docs together — which is the property the discussion's decision is about (CLAUDE.md's same-commit docs rule, and never leaving `main` with a green guard over an unconverted tree).
  Per-card commits on a branch that is squashed are invisible to that rule and are what make review and rollback tractable during implementation.
- **Applies to:** all batches

### Decision: regression posture — an existing test that needs editing is a stop

- **Decision:** every existing test named in the discussion's `### Existing regression cover` section must pass unchanged.
  If an implementer finds that a card's change requires editing an existing named test, that is a behaviour change outside the three named gaps: stop, do not edit the test, and report it.
  Adding new tests is always fine.
- **Rationale:** the ~29 existing destructive-verb integration files are the only thing policing this consolidating refactor, and silently editing one deletes the evidence that the refactor was safe.
- **Applies to:** all batches

### Decision: no new import of `internal/gitrepo` and no cwd resolution inside the gate

- **Decision:** `destroy.go` and `dirtiness.go` add nothing to `internal/gitrepo`, never call `lyxcwd.Getwd`/`Resolve`, and never construct a weft or junction path from a string literal.
  Every path the gate touches arrives on a request or is built by an existing exported geometry helper.
- **Rationale:** the Cwd Resolution Invariant and the `dirtiness-probe-stays-fabric-local` decision, both of which a reviewer will check directly.
- **Applies to:** all batches

## All Files Touched

- `CONSTRAINTS.md`
- `cmd/lyx/destructiveguard_test.go`
- `cmd/lyx/tierpurity_test.go`
- `internal/fabricengine/add.go`
- `internal/fabricengine/checkout.go`
- `internal/fabricengine/cleanup.go`
- `internal/fabricengine/clone.go`
- `internal/fabricengine/destroy.go`
- `internal/fabricengine/destroy_test.go`
- `internal/fabricengine/destructivegaps_integration_test.go`
- `internal/fabricengine/dirtiness.go`
- `internal/fabricengine/doc.go`
- `internal/fabricengine/junction.go`
- `internal/fabricengine/launchers.go`
- `internal/fabricengine/portals.go`
- `internal/fabricengine/prune.go`
- `internal/fabricengine/pull.go`
- `internal/fabricengine/reconcile.go`
- `internal/fabricengine/remove.go`
- `internal/fabricengine/unwire.go`
- `internal/fabricengine/warpclean.go`
- `internal/fabricengine/warpforward.go`
- `internal/fabricengine/weftwiring.go`
- `manifest/designs/fabric-crucible-followups.md`
- `manifest/roadmap.md`
