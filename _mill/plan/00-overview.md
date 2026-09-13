# Plan: Deploy cited spec/design docs to target repos like stencils

```yaml
task: "Deploy cited spec/design docs to target repos like stencils"
slug: "deploy-specs-like-stencils"
approved: false
started: "20260913-173726"
parent: "main"
root: ""
verify: go build ./...
discussion_sha: bc037efc203df49550c8de1b2366fbd2e4320f75
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: specs-registry
    file: 01-specs-registry.md
    depends-on: []
    verify: go test ./contracts/specs/... ./internal/stencilstore/...
  - number: 2
    name: specs-geometry
    file: 02-specs-geometry.md
    depends-on: []
    verify: go test ./internal/fabricengine/... ./internal/standalonegeom/... ./internal/hubgeom/... ./internal/websterengine/... ./internal/lyxcwd/...
  - number: 3
    name: seeded-subtree-commit
    file: 03-seeded-subtree-commit.md
    depends-on: [2]
    verify: go test ./internal/fabricengine/... ./internal/stencilcli/... ./cmd/lyx/... && go test -tags integration ./internal/fabricengine/...
  - number: 4
    name: specs-seeding-wiring
    file: 04-specs-seeding-wiring.md
    depends-on: [1, 2, 3]
    verify: go test ./cmd/lyx/... ./internal/cliwire/... ./internal/stencilcli/... && go test -tags integration ./cmd/lyx/... ./internal/cliwire/...
  - number: 5
    name: specs-dir-marker-plumbing
    file: 05-specs-dir-marker-plumbing.md
    depends-on: [2, 4]
    verify: go test ./internal/shedadapters/... ./internal/shedrecipe/... ./internal/loomrecipe/... ./internal/loomengine/... ./internal/websterengine/... ./internal/loomcli/... ./internal/burlercli/... ./internal/burlerengine/...
  - number: 6
    name: citation-rewrite-and-enforcement
    file: 06-citation-rewrite-and-enforcement.md
    depends-on: [5]
    verify: go test ./contracts/stencils/... ./internal/loomengine/... ./internal/websterengine/... ./internal/shedadapters/... ./internal/burlerengine/... ./internal/lyxcwd/...
```

## Shared Decisions

### Decision: deploy-not-bake

- **Decision:** The two normative docs travel by the same embed-and-deploy mechanism the producer stencils already use — a second embed registry reconciled into a second board directory — never by baking their text into a stencil through marker substitution.
- **Rationale:** The format contract's own status line states that the LLM-facing subset is pinned separately in the producer's stencil so the agent's prompt never duplicates the file and the two cannot drift from being the same doc.
  Baking in inverts that documented decision and adds 482 lines to three prompts.
  Deploy is nearly free: the reconcile pass is already fully parameterised over both its base directory and its registry, so a second registry against a second base directory needs no change to the reconcile implementation at all.
- **Applies to:** all batches

### Decision: stencilstore-is-not-modified

- **Decision:** `internal/stencilstore` gains no production change in this plan — no new `Mode`, no force-sync carve-out, no change to `Classify`, `RelPath`, `Path`, `ApplyStamp`, `BodyHash`, or `Reconcile`.
  Only a test is added to it, in batch 1 card 4.
- **Rationale:** Deployed specs reuse the existing policy verbatim: absent seeds stamped, untouched-but-stale refreshes in production and warns in dev, reconciled restamps, edited warns and is never overwritten.
  A second policy for a second document class would be a second thing to keep correct, and the never-overwrite-a-hash-mismatch rule is an invariant.
  An operator who edits a deployed spec has deliberately forked it, and warn-and-leave-alone is the right response.
- **Applies to:** all batches

### Decision: two-embed-sites-one-registry

- **Decision:** Two Go packages carry one `//go:embed` directive each — one beside each travelling document — and a single registry over both lives in the specs package, which imports the other.
- **Rationale:** `//go:embed` reaches only files at or below its own directory and its patterns may not contain `..`.
  The two documents live in directories with no common ancestor below the repository root, so a single covering site would have to be a package at the repository root, a placement this repository does not otherwise use.
  The registry interface is two methods and is indifferent to how many packages back it, so the split is mechanical.
  This is settled, not a plan-level open question: the root-package option is not to be re-litigated.
- **Applies to:** specs-registry

### Decision: specs-dir-is-told-never-derived

- **Decision:** The deployed-specs directory is resolved once per mode and told to every consumer, following whatever route that consumer already uses for its stencils directory — a geometry-struct field for webster, a plain told parameter for loom and the shed registry.
  It is never derived inside an engine, and never derived as a sibling of whatever stencils directory was told.
- **Rationale:** The Told-Geometry Invariant bars an engine from deriving its own paths, and the existing per-consumer shapes are already non-uniform; normalising them here would be a larger change than this task.
  The sibling derivation is specifically barred because the stencils directory is a told override with no sibling guarantee — an operator may point it anywhere — so the specs directory must be resolved independently.
- **Applies to:** specs-geometry, specs-seeding-wiring, specs-dir-marker-plumbing

### Decision: specs-dir-marker-is-required

- **Decision:** `specs_dir` is a required marker wherever it appears: plain fill semantics, never the optional-marker list, at every one of the four render routes and in the rubric render helper.
- **Rationale:** A blank render would silently reproduce exactly the dead reference this task exists to fix, and it would do so in the form hardest to notice — a prompt that renders, looks complete, and points at nothing.
  The fill helper already errors on an absent-or-empty required marker, which is the loud, early failure wanted.
- **Applies to:** specs-dir-marker-plumbing, citation-rewrite-and-enforcement

### Decision: absolute-path-is-load-bearing

- **Decision:** The rendered specs directory is always an absolute path, and at least one test in the plan asserts it.
- **Rationale:** The Hub Containment Invariant keeps the board reachable from the hub only, so a deployed spec sits outside the agent's own worktree.
  A hub-relative spelling is not merely inconvenient there, it is unusable.
  The premise this rests on is that the autonomous producer agents these four stencils drive can read outside their worktree root, which they can — they run with permission checks disabled.
  Recording the premise is the point: it is load-bearing and would otherwise be implicit.
- **Applies to:** specs-seeding-wiring, specs-dir-marker-plumbing, citation-rewrite-and-enforcement

### Decision: rubric-is-filled-at-read-time-stencil-sourced-only

- **Decision:** One shared helper reads a stencil-sourced rubric, strips its stamp banner, and fills it as its own single-marker template; the three stencil-sourced rubric sites all route through it.
  A literal rubric value — one that reaches a producer through a configuration key rather than through the stencil store — is never filled and never stripped, and passes through exactly as today.
- **Rationale:** A rubric is interpolated as a marker *value*, and the fill's required-marker check only inspects the template actually being executed, so a marker sitting inside a value is invisible to it and would ship literally into a judge prompt.
  Filling the rubric as its own template at read time closes that hole and gives the marker the same error-on-empty property it has everywhere else, which neither a string replacement nor a custom placeholder token would.
  The literal route is excluded because running author-written prose through the fill would turn any bare `{{` in it into a parse-template error and impose specs-directory semantics on text that never had them.
- **Applies to:** specs-dir-marker-plumbing, citation-rewrite-and-enforcement

### Decision: one-marker-allowlist-replaces-the-no-marker-rule

- **Decision:** The existing "a rubric contains no stencil marker" rule is relaxed to a one-marker allowlist — a rubric may contain the specs-directory marker and nothing else — rather than deleted, and the rubric tests additionally assert each rubric parses successfully through the render helper.
- **Rationale:** What changed is the rule's shape, not its existence: a second marker would still be invisible at the value site and must still fail loudly.
  The parse assertion covers the genuinely new failure mode a marker-name check cannot see — a bare `{{` in author prose, which has no marker name to inspect and becomes a runtime parse error once the rubric is filled.
- **Applies to:** citation-rewrite-and-enforcement

### Decision: seeded-subtree-generalisation-has-two-halves

- **Decision:** The seeded-subtree commit verb is generalised over both the pathspec prefix and the absolute directory recorded in the mutation record, as two told parameters, and is then called once per seeded subtree rather than gaining a sibling verb.
- **Rationale:** Generalising only the prefix would file a seeded spec under the stencils directory in the mutation record, which the Mutation Record Invariant makes a false record rather than a cosmetic one.
  Generalising beats a sibling verb because the two call sites would otherwise be byte-identical apart from one constant, and the Fabric Git Invariant's positive-only-pathspec requirement is then satisfied in one place instead of two.
  Both calls keep passing a positive-only file list, exactly as today.
- **Applies to:** seeded-subtree-commit, specs-seeding-wiring

### Decision: specs-reconcile-passes-no-source-dir

- **Decision:** Every specs reconcile and force-refresh passes an empty source directory, so no port-back drift comparison ever runs for a deployed spec.
- **Rationale:** That parameter exists only to drive the drift warning, which serves a port-back authoring workflow: an operator edits a board copy and the warning reminds them to promote it back.
  Specs have no such workflow — the loomyard-side file is the single source of truth and a deployed copy is never authored — so the warning has nothing to say.
  It also could not work as-is: the two travelling documents live in different directories and one's basename differs from its registered name, so neither the existing source-directory shape nor a single replacement fits.
  A per-name mapping would fit but costs either a reconcile change or a second structure to keep in sync with the registry.
  **Accepted consequence, stated so it is not discovered later:** board-versus-worktree drift detection is unavailable for specs, and the diff and promote verbs deliberately do not cover them.
- **Applies to:** specs-seeding-wiring

### Decision: specs-seed-does-not-inherit-the-told-stencils-skip

- **Decision:** In standalone mode the specs reconcile runs unconditionally, including when the stencils directory was told by flag.
- **Rationale:** The existing skip protects an operator's curated *prompt* set from being rewritten from under them.
  A stencils override says nothing about specs, no specs flag exists or is being added, and a spec has no customisation story a curated set would express.
  Inheriting the skip would leave the specs directory resolvable but empty, reproducing the dead reference in the one mode hardest to notice it.
  An operator who has edited a deployed spec is still protected by the unchanged edited-file row.
- **Applies to:** specs-seeding-wiring

### Decision: per-verb-specs-coverage-is-decided

- **Decision:** `list` and `sync` cover deployed specs; `validate`, `diff`, and `promote` do not.
  No verb gains a flag or a subcommand, and no new command subtree is added.
- **Rationale:** `sync` is force-refresh plus a weft commit and is as useful for a stale or edited spec as for a stencil — both halves, not only the commit.
  `list` is the only remaining verb that surfaces an edited deployed spec, which the unchanged reconcile policy depends on an operator being able to see.
  `validate` compares top-level marker sets; a spec is not a template, so both sides are empty and the pass would be a guaranteed no-op that falsely implies a check ran.
  `diff` and `promote` both need the worktree source directory specs deliberately lack.
  A dedicated command subtree is YAGNI for two files nobody edits, and the once-per-process pre-run pass is already the invariant-mandated home for seeding.
- **Applies to:** specs-seeding-wiring

### Decision: the-enforcement-test-is-the-deliverable

- **Decision:** The plan's centre of gravity is a test that fails on any newly introduced bare cross-repository citation in a stencil body, not the thirteen rewrites.
  Its token rule is narrowed to a token under one of four prefixes that ends in a source or markdown extension and is neither glyph-suffixed nor handle-prefixed, with a justification-carrying allowlist keyed by stencil name and token.
- **Rationale:** Fixing thirteen sites fixes thirteen instances; the test closes the class.
  This bug reached production once already and the reverted one-line fix is the evidence that review discipline alone did not catch it.
  A bare prefix match over-fires on the glyph-grammar examples real stencil bodies legitimately carry, which is why the rule has three parts rather than one.
- **Applies to:** citation-rewrite-and-enforcement

### Decision: travelling-docs-keep-their-own-dangling-links

- **Decision:** The cross-repository references *inside* the two travelling documents are accepted as-is and deliberately not rewritten, including the two relative links that resolve from their source directories but not from a deployed tree.
  Neither travelling document's content changes in this plan, and neither source file is renamed.
- **Rationale:** They are reference-document prose read by a human or a curious agent, not instructions the agent's output is mechanically validated against.
  Rewriting them would fork the deployed copies from their loomyard-side source, which is exactly what the deploy-not-bake decision exists to prevent.
  Stated explicitly so a reviewer does not read the omission as an oversight.
- **Applies to:** all batches

### Decision: out-of-scope-and-stays-out

- **Decision:** The two-build stencil-skew finding is not folded in; the Go-only comment-conventions document is not deployed; no document beyond the two normative ones travels; the roadmap does not move.
- **Rationale:** The skew finding is a distinct problem — build-versus-build skew rather than documents absent entirely — and solving it here would mean solving it for stencils too, a strictly larger task that would change the reconcile policy this plan deliberately leaves alone.
  The conventions document is loomyard's own house style, so shipping it would have a reviewer enforce the wrong conventions in an arbitrary target repository — worse than the dangling path it would fix.
  The full-tree audit found no third normative citation, so blanket-shipping the specs directory would carry four documents no stencil body cites.
  The roadmap moves only for a completed or added planned item, and this closes a filed bug.
- **Applies to:** all batches

## All Files Touched

- `CONSTRAINTS.md`
- `cmd/lyx/specsseed_integration_test.go`
- `cmd/lyx/stencilseed.go`
- `contracts/specs/specs.go`
- `contracts/specs/specs_test.go`
- `contracts/stencils/bouncer/bouncer-template-judge.md`
- `contracts/stencils/bouncer/bouncer-template-seed.md`
- `contracts/stencils/citation_enforcement_test.go`
- `contracts/stencils/loom/loom-rubric-plan-review.md`
- `contracts/stencils/loom/loom-rubric-webster-review.md`
- `contracts/stencils/loom/loom-template-plan.md`
- `contracts/stencils/rubric_test.go`
- `contracts/stencils/webster/webster-body-implementer.md`
- `contracts/stencils/webster/webster-prefix-recovery.md`
- `contracts/stencils/webster/webster-template-master.md`
- `docs/code-comment-conventions.md`
- `docs/overview.md`
- `internal/cliwire/specsseed_integration_test.go`
- `internal/cliwire/standalone.go`
- `internal/fabricengine/junctionnames.go`
- `internal/fabricengine/specsdir_test.go`
- `internal/fabricengine/stencilcommit.go`
- `internal/fabricengine/stencilcommit_integration_test.go`
- `internal/fabricengine/stencilhistory_integration_test.go`
- `internal/hubgeom/webstergeom.go`
- `internal/hubgeom/webstergeom_test.go`
- `internal/loomcli/wiring.go`
- `internal/loomcli/wiring_test.go`
- `internal/loomengine/config_test.go`
- `internal/loomengine/plan.go`
- `internal/loomengine/plan_test.go`
- `internal/loomrecipe/fixture_test.go`
- `internal/loomrecipe/shape_test.go`
- `internal/shedadapters/bouncer.go`
- `internal/shedadapters/bouncer_judge_test.go`
- `internal/shedadapters/bouncer_seed_test.go`
- `internal/shedadapters/rubric.go`
- `internal/shedadapters/rubric_test.go`
- `internal/shedrecipe/entries_bouncer.go`
- `internal/shedrecipe/entries_bouncer_test.go`
- `internal/shedrecipe/entries_burler.go`
- `internal/shedrecipe/entries_burler_test.go`
- `internal/shedrecipe/fixture_test.go`
- `internal/shedrecipe/recipe.go`
- `internal/standalonegeom/specsdir.go`
- `internal/standalonegeom/standalonegeom_test.go`
- `internal/standalonegeom/webstergeom.go`
- `internal/stencilcli/cli.go`
- `internal/stencilstore/stencilstore_test.go`
- `internal/websterengine/beginbatch.go`
- `internal/websterengine/geometry.go`
- `internal/websterengine/recoverbatch.go`
- `internal/websterengine/render.go`
- `internal/websterengine/template_test.go`
- `manifest/designs/designs.go`
- `manifest/designs/loom.md`
