# Plan: Rename hub container suffix from -HUB to -LYXHUB

```yaml
task: "Rename hub container suffix from -HUB to -LYXHUB"
slug: "hub-suffix-lyxhub"
approved: false
started: "20260913-110942"
parent: "main"
root: ""
verify: null
discussion_sha: "62d8c73e025450080df1e67c318d9d4bb8caf033"
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: core-constants
    file: 01-core-constants.md
    depends-on: []
    verify: go test ./internal/lyxcwd/... ./internal/fabricengine/...
  - number: 2
    name: production-prose
    file: 02-production-prose.md
    depends-on: [1]
    verify: go test ./internal/fabricengine/... ./internal/fabriccli/... ./internal/hubforge/... ./cmd/lyx/...
  - number: 3
    name: fabricengine-tests
    file: 03-fabricengine-tests.md
    depends-on: [1]
    verify: go test ./internal/fabricengine/... && go test -tags integration -run TestOwnership_FabricHubKind ./internal/fabricengine/...
  - number: 4
    name: peripheral-tests
    file: 04-peripheral-tests.md
    depends-on: [1]
    verify: go test ./cmd/lyx/... ./internal/hubgeom/... ./internal/logger/... ./internal/loomengine/... ./internal/lyxcwd/... ./internal/tokenvocab/... ./internal/weftname/... ./internal/reedengine/... && go test -tags integration -run TestResolve_FromWorktreeRoot ./internal/lyxcwd/... && go vet -tags smoke ./internal/reedcli/...
  - number: 5
    name: sandbox-fixture
    file: 05-sandbox-fixture.md
    depends-on: [1]
    verify: go test ./tools/sandbox/...
  - number: 6
    name: docs-prose
    file: 06-docs-prose.md
    depends-on: [1, 5]
    verify: go test ./internal/lyxcwd/...
  - number: 7
    name: final-sweep-gate
    file: 07-final-sweep-gate.md
    depends-on: [1, 2, 3, 4, 5, 6]
    verify: go test ./internal/lyxcwd/...
```

## Shared Decisions

_Cross-cutting decisions every batch inherits: naming conventions, error-handling posture, test frameworks, style/lint constraints.
One subsection per decision.
Batch-local decisions live in each batch file._

### Decision: Clean break

- **Decision:** `-LYXHUB` fully replaces `-HUB`.
  No code anywhere parses, trims, or recognises the retired suffix after this task — no dual-suffix parse, no compatibility shim, no deprecation window, no migration command.
- **Rationale:** hub discovery is entirely name-independent.
  The hub is `filepath.Dir(workTreeRoot)`, and `looksLikeHub` is structural — it looks for a board entry or a weft sibling and never inspects the container's own name.
  The suffix is used in exactly two directions: construction, by `HubPath`, and `RepoName` derivation, by the trim in `buildLocation`. `RepoName` is never used to build a path;
  it is copied forward into geometry and layout structs and consumed only by the `repo` display token.
  A container still carrying the retired suffix therefore keeps working, and its only degradation is a stale display string.
  A dual parse would permanently enshrine the exact literal this task exists to retire, including a row for it in the enforcement registry, in exchange for that display string.
- **Applies to:** all batches

### Decision: Test-literal classification

- **Decision:** every occurrence of the retired token is sorted into one of four classes before anything is edited, and only class (a) is substituted.
  (a) A real-suffix literal — the string stands for the hub suffix, whether as a synthetic path fixture, a table want value, a socket-key expectation, or a doc comment describing the container — becomes `-LYXHUB`.
  (b) A coincidental substring — an arbitrary recognisable sentinel that contains the token but does not denote the hub — is left byte for byte.
  (c) A recorded historical capture — verbatim text from a real past run — is left byte for byte.
  (d) Conceptual prose — text that does mean the hub but names the concept rather than the directory suffix — is reworded, not substituted.
- **Rationale:** a mechanical global replace corrupts (b), whose whole point is to be an arbitrary marker;
  falsifies (c), which is a record of what happened rather than a claim about current code;
  and produces nonsense in (d).
  The distinguishing rule for documents is the same: one asserting something about the current code is updated with the code, one asserting what happened on a given date is never retro-edited.
- **Applies to:** all batches

### Decision: No physical rename

- **Decision:** neither the tooling nor the documentation offers or recommends renaming an existing container directory.
  The documented path for an operator who wants the new name is to remove the old directory by hand and re-create the container.
- **Rationale:** a container's wiring embeds its absolute path at creation time.
  Portal links and launcher scripts are both built from the hub path and materialised against it, and `ServerName` derives the tmux socket key from the container basename plus a hash of its absolute path.
  Renaming the directory invalidates every link inside it and silently orphans any running reed server — it breaks a working container rather than migrating it.
  The reset flag is not a migration tool either: both the collision guard and the reset teardown key on the derived path, which now carries the new suffix, so a re-clone neither refuses nor removes the old directory and instead creates a second, parallel container for the same warp.
- **Applies to:** all batches

### Decision: Migration prose placement

- **Decision:** the operator procedure for disposing of a container that still carries the retired suffix is recorded in exactly two places — the canonical hub-path document, and the new invariant section in the constraints file.
  It is not added to the `lyx fabric clone` help text, and not to the sandbox suite documents.
- **Rationale:** help text is read by every future operator forever, long after no such container exists anywhere;
  a permanent paragraph about a retired suffix is clutter there.
  The constraints file is where reviewers are required to read every session, which is the right home for the lockstep-declarer rule;
  the hub-path document is where an operator looks for procedure.
- **Applies to:** production-prose, sandbox-fixture, docs-prose

### Decision: Sanctioned duplication is preserved

- **Decision:** both declarers stay.
  The private constant in `internal/lyxcwd` and the exported one in `internal/fabricengine` keep their names, their visibility, and their separate declarations;
  only the value and the doc-comment examples change, in one commit, together with the enforcement registry's owner row.
- **Rationale:** the duplication exists because `internal/lyxcwd` cannot import `internal/fabricengine` — it is an entry-gate leaf below it — and it is sanctioned by the enforcement test's owner map rather than merely by convention.
  Changing one declarer without the other, or either without the map, fails `TestEnforcement_GeometryLiterals`.
  Collapsing them into one declarer would introduce an import cycle and is out of scope.
- **Applies to:** core-constants

### Decision: Go verify commands carry no `PYTHONPATH=` prefix

- **Decision:** every batch's `verify:` is a native Go command with no environment prefix, scoped to the packages that batch edits.
  A batch that edits a build-tagged test file chains a second invocation carrying that tag.
- **Rationale:** this is a Go repository, so the `PYTHONPATH=` isolation prefix does not apply.
  Scoping to edited packages keeps each verify cheap enough to run after every implementer and fixer round;
  the repository-wide net is `pipeline.done_gate`, already configured as `go test ./... && go test -tags integration ./...` and run once from the repository root before the task is marked done.
  Building requires a C compiler on `PATH` and cgo enabled, since the binary links tree-sitter grammars — that is the ambient default on a developer machine and needs no per-command setting.
- **Applies to:** all batches

### Decision: Roadmap is not moved

- **Decision:** `manifest/roadmap.md` is not touched by this task.
- **Rationale:** the repository convention is that the roadmap moves only on completing or adding a planned item.
  This is a rename, not either.
  Its record is the git history, the module documentation, and the new invariant section.
- **Applies to:** all batches

## All Files Touched

_Full union of every `Creates:` / `Edits:` / `Moves:` **target** path across every batch, sorted alphabetically (Move **source** paths are excluded — they disappear, like `Deletes:` tokens).
Cards are the source of truth;
this section is the input `_plan_validate.py`'s `all-files-touched-mismatch` check cross-references against the derived union of every card's `Edits:`/`Creates:`/Move-target paths, to catch drift between the hand/agent-maintained list here and that derived union._

- `CONSTRAINTS.md`
- `cmd/lyx/constructoranchoring_test.go`
- `cmd/lyx/notransients_test.go`
- `docs/overview.md`
- `docs/sandbox-howto.md`
- `docs/sandbox-hub.md`
- `docs/shared-libs/lyxcwd.md`
- `internal/fabriccli/fabric.go`
- `internal/fabricengine/clone.go`
- `internal/fabricengine/clone_reset_guard_test.go`
- `internal/fabricengine/destructivegaps_integration_test.go`
- `internal/fabricengine/fabric_test.go`
- `internal/fabricengine/hubscratch_test.go`
- `internal/fabricengine/junction_test.go`
- `internal/fabricengine/junctionnames.go`
- `internal/fabricengine/junctionnames_test.go`
- `internal/fabricengine/livestate_doc_test.go`
- `internal/fabricengine/livestate_verbs_test.go`
- `internal/fabricengine/origin_test.go`
- `internal/fabricengine/portallauncher_test.go`
- `internal/fabricengine/warpbinding_clone_integration_test.go`
- `internal/fabricengine/warplayout_fastpath_test.go`
- `internal/hubforge/hub.go`
- `internal/hubgeom/hubgeom_test.go`
- `internal/hubgeom/webstergeom_test.go`
- `internal/logger/logsdir_test.go`
- `internal/loomengine/config_test.go`
- `internal/loomengine/discussionpath_test.go`
- `internal/loomengine/friction_test.go`
- `internal/loomengine/loomstatus_test.go`
- `internal/loomengine/review_test.go`
- `internal/lyxcwd/enforcement_test.go`
- `internal/lyxcwd/geometry_test.go`
- `internal/lyxcwd/lyxcwd.go`
- `internal/lyxcwd/lyxcwd_test.go`
- `internal/lyxcwd/reponame_test.go`
- `internal/reedcli/smoke_teardown_test.go`
- `internal/reedengine/server_test.go`
- `internal/reedengine/state_test.go`
- `internal/tokenvocab/tokenvocab_test.go`
- `internal/weftname/weftname_test.go`
- `manifest/designs/reed-fabric-standalone-api.md`
- `tools/sandbox/SANDBOX-BURLER-SUITE.md`
- `tools/sandbox/SANDBOX-CORE-SUITE.md`
- `tools/sandbox/SANDBOX-FABRIC-SUITE.md`
- `tools/sandbox/SANDBOX-REED-SUITE.md`
- `tools/sandbox/SANDBOX-REED-WATCH-SUITE.md`
- `tools/sandbox/SANDBOX-SHUTTLE-SUITE.md`
- `tools/sandbox/SANDBOX-WEBSTER-SUITE.md`
- `tools/sandbox/main.go`
- `tools/sandbox/suite.go`
