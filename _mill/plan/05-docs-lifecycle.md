# Batch: docs-lifecycle

```yaml
task: "Build burler - the review+fix round worker"
batch: "docs-lifecycle"
number: 5
cards: 1
verify: go build ./... && go test ./...
depends-on: [4]
```

## Batch Scope

Executes the Documentation Lifecycle for the landed module: delete the design doc (an
immediate staleness source once code exists), fold its durable pointers into
`docs/overview.md`, move every not-delivered idea into a roadmap deferred-enhancements
section, mark the burler half of milestone 11 done, and retarget every inbound
`modules/burler.md` link. The package doc (batch 2, card 6) already holds the durable
design; this batch only touches markdown.

## Cards

### Card 11: Delete burler.md, fold into overview/roadmap, retarget links

- **Context:**
  - `_mill/discussion.md`
  - `docs/modules/burler.md`
- **Edits:**
  - `docs/overview.md`
  - `docs/roadmap.md`
  - `docs/modules/README.md`
  - `docs/modules/perch.md`
  - `docs/modules/loom.md`
  - `docs/modules/hardener.md`
  - `docs/reviews/README.md`
  - `docs/shared-libs/stencil.md`
  - `internal/burlercli/run.go` (discovered during implementation: the `run` subcommand's `Long`
    help text has an example profile whose `target.paths` example was
    `docs/modules/burler.md` — the file this card deletes; repointed to a still-existing example
    path)
- **Creates:** none
- **Deletes:**
  - `docs/modules/burler.md`
- **Moves:** none
- **Requirements:** (1) Delete `docs/modules/burler.md`. (2) `docs/overview.md`: rewrite
  the `- **burler** —` bullet in `## Modules` to ✅ Implemented status describing the
  landed shape — one review+fix round (A-review → B-fix, one agent, no self-grading) over
  the shuttle file contract; `internal/burlerengine` + `internal/burlercli`; profile-driven
  (`{overlay, source}` fix-scope, tool-use, cluster-N rejected until mux own-window);
  strict frontmatter verdict parse; debug CLI `lyx burler run`; "See the
  `internal/burlerengine` package documentation" (matching the mux/shuttle bullet
  pattern) — and update the `## Execution stack` block's burler line from design-tense if
  needed plus any other `modules/burler.md` link in the file. (3) `docs/roadmap.md`:
  in milestone 11 mark the **burler half** done — annotate the burler portion with ✅ and a
  pointer to the `internal/burlerengine` package documentation, leaving the perch half and
  the milestone itself open — and retarget the milestone's `modules/burler.md` links;
  add a new `### Deferred burler enhancements` section (mirroring `### Deferred mux
  enhancements`'s register) listing, with one short rationale each: cluster-N > 0
  (blocked on milestone 24 own-window anchoring; burler today rejects it with a typed
  error), a generic tools-restriction on the shuttle `Spec` for read-only cluster
  reviewers (meaningless for the single-session A→B agent, which must write in B),
  bulk-mode clusters + provider-side context caching modelled as one shared prefix + N
  distinct suffixes (the modelling constraint that keeps caching possible — carry the
  rationale over from burler.md's cluster section verbatim in spirit), and a per-round
  provider selector on the shuttle `Spec` when a second engine lands; fix the
  `roadmap.md` line that points at `modules/burler.md#cluster-support` (milestone 24's
  back-reference) to point at the deferred section or the package doc. (4)
  `docs/modules/README.md`: drop the burler.md row from the table (or retarget it to the
  package doc following how mux/shuttle rows are handled — match the existing register
  for implemented modules), keep the stack diagram and dispatch-question lines accurate
  (`burler` is now built; wording may keep the name, only links must not 404). (5)
  `docs/modules/perch.md`, `docs/modules/loom.md`, `docs/modules/hardener.md`,
  `docs/reviews/README.md`, `docs/shared-libs/stencil.md`: replace every
  `[...](burler.md)` / `[...](../modules/burler.md)` / `[...](modules/burler.md)` link
  with a non-link reference to the `internal/burlerengine` package documentation (the
  established pattern for landed modules — see how these docs already refer to mux and
  shuttle), preserving surrounding prose; grep the whole `docs/` tree for `burler.md`
  afterward and confirm zero hits remain. Do NOT add any bugfix/hardening notes to the
  roadmap beyond the milestone-11 annotation and the deferred section (roadmap is planned
  milestones only, per CLAUDE.md).
- **Commit:** `docs(burler): fold design doc into package doc + overview; add deferred enhancements`

## Batch Tests

`verify:` runs the full `go test ./...` as the terminal whole-repo gate (this is the last
batch; earlier batches ran scoped suites). The docs edits themselves have no runnable
surface — the full-suite run guards against any stray regression before handoff, and Go
test suites here are fast enough for a one-shot full run.
