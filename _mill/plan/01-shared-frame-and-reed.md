# Batch: shared-frame-and-reed

```yaml
task: 'Reed and Fabric as standalone modules: public API design'
batch: 'shared-frame-and-reed'
number: 1
cards: 7
verify: go test ./internal/lyxcwd/ -run TestEnforcement_MarkdownLinks
depends-on: []
```

## Batch Scope

This batch creates `manifest/designs/reed-fabric-standalone-api.md` and writes its shared front half plus the entire Reed half.
It is one batch because every card writes into one file and each card's section depends on the framing the previous card established — the extraction rubric and the corrections must exist before the Reed verdict can lean on them.
The external interface batch 2 consumes is the file itself, its heading vocabulary, and two things batch 2 must not restate: the measurement-method-and-caveat section written by card 1, and the contract-is-the-type-set framing written by card 2.

Batch-local decision that differs from `## Shared Decisions`: card 1 alone creates the file;
every other card in this plan, in both batches, appends or edits sections in a file that already exists.
A card must never rewrite a section an earlier card wrote.

## Cards

### Card 1: Create the doc, its framing, its measurement method, and its own disposition

- **Context:**
  - `_mill/discussion.md`
  - `docs/overview.md`
  - `manifest/designs/loom.md`
  - `manifest/designs/semantic-index.md`
  - `manifest/designs/hardener.md`
  - `CONSTRAINTS.md`
- **Edits:** none
- **Creates:**
  - `manifest/designs/reed-fabric-standalone-api.md`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create the file with an H1 naming both modules and the subject, then write its first three sections.
  (1) `## What it is` — one short section stating that the document answers, for two lyx modules, what each one's public contract would have to be for its repo to be usable by a non-loomyard consumer, and whether paying that price is worth it;
  and stating that the deliverable is a design document about unbuilt work, not documentation of the shipped modules, whose design lives in their own package doc comments.
  (2) `## How the numbers in this document were produced` — state that every figure was measured in this worktree at commit `ca388c0ce` using `go list`, `go doc` and `grep`, that each figure is stated with its producing command, and carry the comment-prose contamination caveat once: a naive grep over a package name also matches doc-comment prose, and two figures in this task's own first draft were wrong for exactly that reason, so every grep-derived count in this document means "referenced in code by production packages", excluding `_test.go` files and doc comments.
  (3) `## Disposition of this document` — state that the doc lands with no owning entry in the roadmap **by design**, because its content is the evidence needed to decide whether any roadmap entry is warranted, so writing the entry first would presuppose the verdict;
  that the follow-up decision has two legitimate outcomes — one or more Someday entries are added pointing at this doc, or none is;
  and that in the second case this document is **deleted** rather than left orphaned.
  Name that deletion trigger explicitly so a later reader of an entry-less designs file knows it is intentional and knows what closes it.
  This section must also state the lifecycle classification and link to the lifecycle rule it satisfies, using a repo-relative link from `manifest/designs/` to the `Documentation lifecycle` heading in the overview doc, written so it resolves under GitHub's slug rule:

  ```text
  [docs/overview.md](../../docs/overview.md#documentation-lifecycle)
  ```

  Read the three sibling design docs listed in `Context:` for the prevailing heading vocabulary before choosing headings;
  do not import their content.
- **Commit:** `docs(designs): add reed/fabric standalone-api doc with framing and method`

### Card 2: The extraction rubric — the frozen contract is the exported type set

- **Context:**
  - `_mill/discussion.md`
  - `internal/shuttleengine/reed.go`
- **Edits:**
  - `manifest/designs/reed-fabric-standalone-api.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add a section stating the document's central framing: for a Go module the extraction contract is the set of exported identifiers consumers reference in code, and a consumer-side interface narrows *coupling* and buys testability but does not narrow that contract by one name.
  Ground it in the shipped example named in `_mill/discussion.md`: `shuttleengine.ReedOps` is held up as the model narrow seam, yet every one of its six methods takes or returns a type from the provider package, so a consumer behind it still pins four exported types.
  Read `internal/shuttleengine/reed.go` to transcribe the interface's method set exactly rather than paraphrasing it.
  State the consequence the discussion draws: the background framing that Fabric's blocker is "no interface seam anywhere on the consumer side" misdiagnoses the problem, because adding fifteen consumer-side interfaces would not shrink a 74-identifier contract at all;
  what shrinks a contract is deleting or unexporting identifiers, or moving them behind a façade that re-exports a curated subset.
  State the two rejected metrics and why each measures the wrong thing: interface count, and raw importer count — the latter because the two modules have nearly identical transitive-dependent counts, 26 and 27, and are nowhere near equally extractable.
- **Commit:** `docs(designs): frame the extraction contract as the exported type set`

### Card 3: The four corrections to the task's background notes

- **Context:**
  - `_mill/discussion.md`
- **Edits:**
  - `manifest/designs/reed-fabric-standalone-api.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add a section stating, as corrections with their evidence, the four background claims this investigation found to be false.
  (1) The `gitkit` / `gitrepo` / `gitexec` "trio" framing is wrong: `internal/gitkit` is not in the Fabric dependency set at all, transitively or directly;
  its only non-test production importer anywhere in the repo is one file in `internal/hubforge`, and every other reference is from a `_test.go` file, which is what makes it test-fixture machinery.
  State that the `gitkit` Leaf Invariant is therefore untouched by anything in this document.
  (2) Fabric's cwd coupling is structural, not incidental: true self-resolution of cwd is only two sites, but six further sites re-derive a location from a stored path, so the coupling is structural typing rather than an incidental parameter.
  (3) The missing-interface framing misdiagnoses Fabric's blocker — cross-reference the rubric section card 2 wrote rather than restating its argument.
  (4) `internal/lyxcwd` does **not** import `internal/fabricengine`: a naive `grep -rl` lists two files under `internal/lyxcwd`, but both matches are doc-comment prose, and `go list` confirms that package's only internal import is `internal/gitexec`, exactly as the Cwd Resolution Invariant requires.
  State that this document records correction (4) precisely because it is the obvious thing a reader would suspect and the obvious way to get it wrong, and that it is the same trap the method section already named.
  Each correction states the command or the `go list` observation that produced it.
- **Commit:** `docs(designs): record the four corrections to the background notes`

### Card 4: The quarry precedent — this project's one completed extraction

- **Context:**
  - `_mill/discussion.md`
  - `manifest/roadmap.md`
  - `docs/research/quarry-holistic-fix-log.md`
- **Edits:**
  - `manifest/designs/reed-fabric-standalone-api.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add a section on the one completed extraction this project has to learn from, cited by module path `github.com/Knatte18/quarry`.
  Record its layout — a root façade package, a separately-importable cgo-free leaf, an `internal/` tree holding the CLI among other packages, and its `cmd/` binaries — and the façade mechanism: it re-exports internal types by alias, so the extraction never required hand-designing a narrow interface up front;
  it curated *what* is exported rather than *how*.
  State plainly, without softening, the piece of evidence on extraction timing: this repo's `go.mod` still carries no requirement on that module, and adoption is a single unstarted item in the roadmap's Planned section.
  Name the residue cost recorded in the port's own log — loomyard-internal references surviving in ported comments, a stale checksum file, and cross-repo review rounds needing a two-repo authorization decision.
  State explicitly, as the discussion requires, that the layout and façade observations were read off a local checkout and are the one block of evidence no other reader can re-verify from this repository alone;
  do not write a machine-local absolute path into the document, and make no claim that rests on one.
  If the research log named in `Context:` does not carry a detail this card asserts, cite the discussion's record of it rather than inventing a citation.
- **Commit:** `docs(designs): record the quarry precedent and its timing evidence`

### Card 5: Reed — the measured contract today

- **Context:**
  - `_mill/discussion.md`
  - `internal/reedengine/doc.go`
- **Edits:**
  - `manifest/designs/reed-fabric-standalone-api.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add the first Reed section: what the module's contract measurably is today.
  Transcribe from the discussion, each with its producing command where one is recorded: production line counts for the engine package and its render leaf;
  the direct internal import list;
  the transitive internal dependency set, stating that two of its members enter *only* through the logging package;
  the public surface as 7 free functions, 20 types and one handle type with 17 exported methods, listing those method names;
  and the external contract footprint of **13** distinct exported package-level identifiers referenced in code by production packages outside the module, listed by name.
  State the metric's definition alongside the number, and note that the render leaf's own exported names and the 17 methods reached through the handle are counted separately.
  Name the three identifiers a bare grep additionally returns and why none is a real external reference — two are doc-comment prose and one is not even exported — and state that counting them is how the first draft reached 15 instead of 13.
  Record the nine direct production importers and the consumer shapes measured per importer: one consumer already holds Reed behind the transport interface;
  two construct and hand off;
  two build the told-geometry value;
  one calls a config-template function only;
  one is the sole consumer *retaining* a concrete engine pointer as a struct field;
  and Reed's own CLI legitimately uses the whole surface.
  Record that the geometry type is eight told string fields with a documented no-validation, no-derivation contract, and that neither of its two constructors would move into a standalone Reed.
  Record that the one provider-specific name in the public API has no caller outside its own package.
  Every file-and-line citation this section makes goes inside a fenced block, transcribed from the discussion:

  ```text
  internal/reedengine/doc.go
  internal/reedengine/env.go, lifecycle.go:299
  internal/reedengine/geometry.go
  internal/loomcli/cli.go:44
  ```
- **Commit:** `docs(designs): record reed's measured contract surface`

### Card 6: Reed — the verdict, its trigger, and the cheap in-repo preparation

- **Context:**
  - `_mill/discussion.md`
  - `manifest/roadmap.md`
  - `internal/reedengine/env.go`
- **Edits:**
  - `manifest/designs/reed-fabric-standalone-api.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add Reed's verdict section, stating an explicit and unhedged call with a stated trigger — not a "revisit later".
  The verdict: do not extract Reed now.
  The gate is not code readiness, because Reed is already close to ready;
  the gate is the existence of a second consumer, or Reed becoming a long-running service rather than a library.
  Carry the reasoning: the contract is small enough to freeze, loomyard is its only user, and the quarry precedent measures what extracting ahead of a consumer buys.
  State that the three further Reed items in the roadmap are all in the **Someday** section — committed but unscheduled, not Planned — and therefore a weak churn argument that must not be read as imminent change;
  the verdict stands on the second-consumer gate alone, with the Someday items as a secondary note that the surface is not finished.
  Name the likelier trigger than "another project wants tmux orchestration": the daemon story — watchdog plus relay plus mailbox — which is when a repo boundary starts paying.
  State the two rejected alternatives and why each fails: extract now, and never extract.
  Then add the preparation subsection recommending exactly **two** in-repo changes as cheap hygiene independent of any extraction, each a roadmap candidate with its own standalone justification.
  (a) Rename the provider-specific environment-cleaning function, whose name is the only Claude-specific identifier in the public API of a package the Shuttle Provider-Seam Invariant says never references provider specifics.
  Fix its depth here so the follow-up item need not invent it: a provider-neutral name taking the key set as a caller-supplied parameter, with the exact signature shape transcribed from the discussion into a fenced Go block, and the provider-specific literals moving out of the function body into its single call site, which is where the provider knowledge belongs.
  State that only the identifier's final name is left to the follow-up item.
  (b) Give the one consumer that retains a concrete engine pointer a named interface seam, so no consumer retains the concrete type as a struct field except Reed's own CLI.
  State the wording precisely: two other consumers construct the engine and hold it transiently before handing it off, and a seam cannot change that, because construction always yields the concrete type;
  what (b) removes is the *retained field*, of which there is exactly one instance.
  State that the logging decoupling is deliberately **not** in this list, and cross-reference card 7's section for why.
  State the three rejected options: bundling the two into the extraction, doing nothing until a trigger fires, and keeping the logging decoupling in this list.
- **Commit:** `docs(designs): reed verdict, second-consumer gate, and in-repo prep`

### Card 7: Reed — the standalone repo shape, its published seams, and the logging price

- **Context:**
  - `_mill/discussion.md`
  - `internal/shuttleengine/reed.go`
  - `internal/logger/sink.go`
  - `cmd/lyx/spawnobservability_test.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `manifest/designs/reed-fabric-standalone-api.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add the section describing what a standalone Reed would look like, in three parts.
  (1) Layout, as a direct mirror of quarry's shipped shape: a root façade package, a separately-importable dependency-light render leaf, an `internal/` tree including the CLI, and one `cmd/` binary.
  Justify the render leaf as the exact analogue of quarry's cgo-free leaf — 773 production lines, zero internal dependencies, stdlib-only imports, and already consumed independently by three packages.
  Name the three stdlib imports exactly as the discussion records them and do not shorten the list to two;
  include the producing command for that import set in a fenced block.
  Include the façade re-export shape as a real Go listing, derived from quarry's alias mechanism, not described in prose.
  (2) Published seams: a standalone Reed publishes **two** named interface slices with compile-time satisfaction proofs — a transport slice and a session-lifecycle slice — as the documented contract, while consumers stay free to define their own narrower interfaces.
  Write both as real Go listings with a satisfaction proof line of the form shown in the discussion.
  Read `internal/shuttleengine/reed.go` and transcribe the transport slice from the shipped interface verbatim;
  the session slice is exactly the six methods the one concrete-pointer consumer calls today, and the discussion records the command that derived them.
  State explicitly that the two slices overlap on three methods and that the overlap is permitted, because a reader's first instinct is that two seams over one type should partition its methods — they do not: each slice states one consumer's real dependency, and a method two consumers both need appears in both.
  State that both slices are declared in the provider package, not in a consumer, because that placement is what makes the compile-time proof live next to the type it constrains.
  State explicitly that the document does **not** claim a wider lifecycle slice covering the four further session methods, because the only consumer calling them is Reed's own CLI, which holds the concrete type and needs no slice.
  (3) The logging decoupling: a standalone Reed takes an optional standard-library structured logger on its config or constructor, defaulting to a discard handler, and drops the internal logging package entirely — that package being the single edge that pulls three further packages into Reed's transitive set.
  State that this is a **standalone-repo design point and not in-repo preparation**, and state its in-repo price in writing rather than working around it: the Live-Substrate Spawn Observability constraint names the internal logging package by name and is enforced mechanically as a file-level import check, so removing that import in-repo would fail the shipped test unless three allowlist entries were added and the constraints document amended — which this task's scope bars.
  Read the enforcement test and the constraints document named in `Context:` to state the mechanism accurately, and quote the allowlist's own first entry, whose written reason documents the very import cycle at issue.
  State the three rejected options: a hand-rolled logger interface, extracting the logging package too, and listing the decoupling as cheap in-repo hygiene.
- **Commit:** `docs(designs): reed standalone layout, published seams, and logging price`

## Batch Tests

`verify: go test ./internal/lyxcwd/ -run TestEnforcement_MarkdownLinks` runs the Markdown Link Integrity enforcement test in `internal/lyxcwd/docslink_test.go`, which is the only mechanical check this batch's output can fail — it scans every `.md` file under `manifest/` and `docs/`, resolving each inline link's file part and `#anchor`.
The `-run` filter scopes the invocation to that one test rather than the package's whole suite.
Two link targets this batch introduces are covered by it: the overview's `#documentation-lifecycle` anchor written by card 1, and any cross-reference to a sibling document under `manifest/designs/`.
No entry may be added to `docsLinkAllowlist`;
a failure means the link is wrong, not that the allowlist is short.

Two checks this batch cannot run mechanically and which the implementer performs by reading:
external `http`, `https` and `mailto` targets are skipped by the enforcement test by design, so any external URL — including the quarry module path if it is written as a link — is verified by eye;
and the semantic-line-break convention is checked against the rules `tools/mdreflow` implements, by review rather than by a new test.

The batch adds no `.go` file and no test file, so there is no unit test to write.
The repo-wide "no code was touched by accident" check is `pipeline.done_gate`, which runs once at the end of the task rather than after every round.
