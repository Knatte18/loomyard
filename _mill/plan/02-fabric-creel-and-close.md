# Batch: fabric-creel-and-close

```yaml
task: 'Reed and Fabric as standalone modules: public API design'
batch: 'fabric-creel-and-close'
number: 2
cards: 4
verify: go test ./internal/lyxcwd/ -run TestEnforcement_MarkdownLinks
depends-on: [1]
```

## Batch Scope

This batch writes the Fabric half of `manifest/designs/reed-fabric-standalone-api.md`, the strand-mailbox placement decision, the other-modules note, and the document's closing sections.
It is one batch because all four cards append to a file batch 1 created, and because the Fabric verdict, the split recommendation and the mailbox placement all lean on the same measured import-and-vocabulary evidence, which is read once.
It depends on batch 1 for the file itself and for two sections it must cross-reference rather than restate: the measurement-method-and-caveat section and the contract-is-the-type-set framing.

Batch-local decision that differs from `## Shared Decisions`: this batch is the only place the document is permitted to describe an unbuilt module (`creel`), and that section carries an explicit "illustrative sketch" label on its one code listing — everywhere else in the document, a listing is transcribed from or derived from shipped source.

## Cards

### Card 8: Fabric — the measured contract today

- **Context:**
  - `_mill/discussion.md`
  - `internal/fabricengine/doc.go`
- **Edits:**
  - `manifest/designs/reed-fabric-standalone-api.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add the first Fabric section: what the module's contract measurably is today, each figure with its producing command where one is recorded in the discussion.
  Record the production line count of 14,610, making it the largest package in the repo;
  the direct internal import list;
  the public surface as roughly 50 free functions, roughly 60 types, 7 constants, 6 sentinel errors, roughly 10 error types and one handle type with 26 methods;
  the external contract footprint of **74** distinct exported identifiers referenced by production code outside the package, of which one consumer alone accounts for 40;
  and the 15 direct production importers.
  State the section's core observation: the names on that surface are hub-layout vocabulary, not git-coordination vocabulary, and list the measured hub-layout identifiers by name, with the string values the discussion records for the three that carry one.
  Record that 33 exported signatures are parameterized on a four-field location type describing lyx's own directory model, and name those four fields — so the API is parameterized on lyx's directory model, not on two git URLs.
  Record which parts of the surface *are* genuinely generic, per the module's own package documentation: the hub-clone entry point taking two plain git URLs, the handle over two repository values, the commit trailer and its rebuildable correspondence index, the uniform branch-naming rule, and the two-sided commit/pull/push/merge surface.
  Then record the two prompt-template package imports individually, because they land on opposite sides of the split card 9 draws and only one is where a reader would guess.
  The stencil-store package enters through **one file alone**, on the hub-layout side, which is the expected placement;
  the pattern package enters through **one file alone** which is a *pair-kernel* file, with exactly one code use — the two further occurrences in that file are doc-comment prose naming the same identifiers and must not be cited as use sites.
  State that a third file widely assumed to put stencil versioning in the engine imports neither, so the common framing is half wrong and this document does not repeat it.
  Put the file names, line citations and both producing commands in fenced blocks, transcribed from the discussion:

  ```text
  internal/fabricengine/stencilhistory.go   -- stencilstore.RelPath, stencilstore.BodyHash
  internal/fabricengine/pull.go:23          -- import; single code use at pull.go:464
  internal/fabricengine/pull.go:423, :441   -- doc-comment prose, not use sites
  internal/fabricengine/stencilcommit.go    -- imports neither
  ```

  ```text
  grep -ln "loomyard/internal/pattern" $(ls internal/fabricengine/*.go | grep -v _test)
  grep -ln "loomyard/internal/stencilstore" $(ls internal/fabricengine/*.go | grep -v _test)
  ```

  Finally, record the candidate travelling companions with their sizes and why none simply moves: the two git packages both have non-Fabric production consumers, and one of them is pinned to the cwd-resolution package by the Cwd Resolution Invariant.
  Cross-reference the corrections section batch 1 wrote for the `gitkit` finding rather than restating it.
- **Commit:** `docs(designs): record fabric's measured contract surface`

### Card 9: Fabric — the verdict and the in-repo split recommendation

- **Context:**
  - `_mill/discussion.md`
  - `internal/fabricengine/doc.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `manifest/designs/reed-fabric-standalone-api.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add Fabric's verdict section and, after it, the recommendation section.
  The verdict is explicit and unhedged: do not extract Fabric, stated as a **size-and-shape mismatch rather than a "not yet"**.
  What is generic is a paired-repo coordination kernel of roughly **6,275 production lines** living inside a 14,610-line engine, and getting it out is a rewrite-by-subtraction, not a move.
  Carry the measured file partition as one defensible assignment with its producing commands, never as a canonical answer: 6,275 production lines over 28 files on the pair-kernel side, of which the merge surface alone is 2,209 lines;
  8,321 production lines over 39 files on the hub-layout side;
  and the two sum to 14,596 against the package's 14,610, the shortfall being a small number of files the partition does not assign.
  State that the kernel side is still not shippable as-is, because those files carry most of the 33 location-typed signatures — which is precisely why it is a rewrite-by-subtraction.
  Then draw the consequence the discussion requires, because it cuts against the split's own rationale: **the pair kernel is not domain-free.**
  The pattern import identified by card 8 exists to populate a residue report naming which post-anchor weft commits touch the pattern files and therefore need review after a warp history rewrite — a loomyard-domain feature living in the most generic-looking file in the package — and the pattern package itself depends on three further internal packages, so the edge drags the whole prompt-template subtree into the kernel with it.
  State the honest conclusion: the split isolates the stencil-versioning coupling cleanly on the hub-layout side and does **not** isolate the pattern coupling, which would have to be cut separately by making residue reporting a caller-supplied predicate rather than a package import.
  State that this is a further argument for the no-extract verdict rather than against it.
  State the two rejected alternatives: extracting the whole engine, and a "defer, revisit later" with no verdict.
  The recommendation section then proposes, as a roadmap candidate with its own justification, splitting Fabric's surface *inside the repo* into two named halves — a pair kernel and a hub-layout surface — and explicitly does **not** tie that recommendation to any extraction.
  Fix its depth so the follow-up item need not guess: the split is file grouping inside the single package, plus a matching section split in the package's own documentation file, plus an enforcement test asserting which files may reference which — and deliberately **not** sub-packages.
  State the reason sub-packages are ruled out by the package's own design rather than by taste: the handle holds two unexported repository fields the package documentation says are reachable only from inside the package, and every hub-layout verb reaches them, so a sub-package boundary would force exporting both and hand every caller exactly the uncoordinated single-sided access the Fabric Git Invariant exists to prevent.
  State that naming beyond the two half-names is left to the follow-up item, and that what this document fixes is the boundary and the mechanism, because those are what determine whether the item is worth picking up.
  Justify the split on its own terms — it makes the Fabric Git Invariant's perimeter visible, and it is the only route by which the pair kernel would ever become extractable — and state the two rejected options: recommending nothing, and recommending the split as extraction step 1.
- **Commit:** `docs(designs): fabric verdict and the in-repo pair-kernel split`

### Card 10: creel — where strand messaging lives

- **Context:**
  - `_mill/discussion.md`
  - `internal/reedengine/doc.go`
  - `manifest/roadmap.md`
- **Edits:**
  - `manifest/designs/reed-fabric-standalone-api.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add the section deciding where the strand-messaging concept lives, answering all four questions the task named — placement, addressing, message model, sender universe — plus transport, and saying what it deliberately leaves unspecified.
  Placement: a sibling module named `creel`, not part of Reed and not dropped.
  Justify it from Reed's own stated contract, read from its package documentation: Reed is the dumb carrier for its caller's strand data, storing every field a caller writes and reading none of them semantically, with deliberately no domain type field on a strand — and a mailbox must read addresses semantically, so putting it inside Reed contradicts the one invariant Reed states about itself.
  Add the structural argument: Reed's geometry type carries a worktree root, a repository name and a hub path, but no slug, no branch and no role, so the addressing a mailbox needs lives in webster's and loom's state, not Reed's.
  State why this is not "drop it": the roadmap already carries a daemon relay item, which is bidirectional messaging by another name and would be the module's first non-agent sender.
  Addressing: an address is `<worktree-slug>/<role>`, optionally with a round, resolved to a live strand identifier only at delivery time, and an identifier is never an address.
  Justify it from the measured lifetime: the engine mints a fresh 128-bit random identifier on every strand add, and webster re-mints on every batch respawn — removing the prior strand by its persisted identifier and adding a fresh one — so an identifier names an *incarnation* while an address must outlive one.
  Note that the durable semantic identity already exists on both sides, in webster's per-role batch state and in the shuttle run record, and that resolution at delivery time uses those records plus a liveness query;
  an address that resolves to nothing leaves the message queued, which is the point of a mailbox rather than a pipe.
  State the two rejected addressing schemes and why each dies: identifier addressing, and tmux pane id.
  Message model: a durable FIFO inbox per address;
  delivery appends a message and sends one keystroke notification;
  the recipient reads on its own turn boundary;
  **no interrupt tier**.
  Justify it: interruption already exists as a separate, synchronous, explicitly-authorized operation sitting on the engine's key-send method, and folding it into the mailbox would make every queued message a potential context-destroying interrupt the recipient cannot distinguish from an ordinary one, while the sender universe is deliberately open — so the authority to interrupt would be granted to anyone who can write a file.
  Keeping the two separate means the module needs no authorization model beyond filesystem permissions.
  State the two rejected models: a two-tier model with an interrupt severity, and polling with no notification.
  Transport and sender universe: a directory of JSON files under the hub's scratch area, plus a CLI send verb for humans and non-Go processes, plus a Go API for in-process senders — no MCP server, no JSON-RPC, no socket.
  Justify it from the task's own principle that a wire-format protocol earns its cost only across a real OS-process boundary with no shared substrate, and here the filesystem is that shared substrate and already this project's idiom for exactly this kind of state.
  Note that it delivers the open sender universe for free: any process that can write a file and knows an address is a sender, with no client library, no schema negotiation and no daemon required.
  State the two rejected transports: a protocol server, and in-memory queues.
  Include the message struct as a Go listing, and label it explicitly as an **illustrative sketch** — it is the one listing in the document with no shipped source to transcribe from, so it must not be presented as a measured or verifiable contract.
  Record the naming rationale: a creel is the rack holding one bobbin per feed position, which is the same object as a rack of per-address inboxes;
  the name is verified unused in this repo, as are seven other loom-vocabulary candidates the discussion lists.
  State that this is a name proposed by a design document, not a rename instruction for any existing identifier.
  Close the section by stating what it deliberately stops short of: a full module design, which would need the durable address registry designed against webster's and loom's state, and is a task of its own.
  This card does not create a module directory, a config-registry entry or a CLI verb.
- **Commit:** `docs(designs): place strand messaging in a creel sibling module`

### Card 11: Other modules, open questions, and the document's closing sections

- **Context:**
  - `_mill/discussion.md`
  - `tools/mdreflow/reflow.go`
  - `tools/mdreflow/reflow_test.go`
  - `manifest/designs/loom.md`
  - `manifest/designs/semantic-index.md`
- **Edits:**
  - `manifest/designs/reed-fabric-standalone-api.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add the document's three closing sections and then verify the whole file.
  (1) One short section — a note, not a follow-up task — recording that three further leaf packages are clean but poor extraction candidates, with the measured reason for each: one is 410 production lines with zero internal dependencies and five consumers;
  one is smaller with four consumers;
  and one wraps an external binary, depends on a process package, and exists to satisfy the GitHub Auth Invariant rather than to be reusable.
  State the economics plainly: a separate repository, module path, release cadence and CI never pays for that size absent an external consumer, and the same second-consumer gate that defers Reed defers all three more strongly.
  Name Reed's render leaf as the best-shaped extraction candidate in the repo, because it is already stdlib-only and pure and is the piece that would ship first if Reed ever ships.
  Propose no follow-up task.
  (2) `## Open questions` — the questions this document genuinely leaves open, drawn from the discussion's own boundaries rather than invented: whether any roadmap entry is warranted at all (the decision this document exists to inform, and whose negative outcome deletes the document);
  the final identifier name for the renamed environment-cleaning function;
  the two half-names for the Fabric split;
  and the durable address registry the mailbox section stops short of designing.
  (3) `## Related` — repo-relative links to the sibling documents and to the lifecycle rule, matching the closing-section shape used by the sibling design docs listed in `Context:`.
  Every link must resolve, file part and anchor alike, and no allowlist entry may be added.
  Then verify the finished document end to end, and fix what fails, as part of this card: confirm both modules carry an explicit unhedged verdict with a stated trigger;
  confirm no quantitative claim appears without a producing command;
  confirm the comment-prose caveat appears exactly once;
  confirm the document states its own entry-less-on-landing disposition and deletion trigger;
  confirm no claim rests on a machine-local absolute path;
  confirm every recommendation is checked against the shipped enforcement test it would have to pass, with any that would fail one either scoped away from in-repo or carrying its price in writing;
  confirm the two prompt-template import sites are named individually and the pair kernel is not claimed to be domain-free;
  confirm the four corrections are stated as corrections with evidence;
  confirm the mailbox section answers all four named questions and says what it leaves unspecified;
  and confirm the document reads as a design document about unbuilt work rather than as documentation of the shipped modules.
  Read the reflow tool and its test named in `Context:` for the semantic-line-break rules, and review the whole document against them — one sentence per line, breaks at internal independent-clause boundaries, no fixed-column wrapping, no trailing double-spaces or backslashes, with table cells and blockquotes on one line.
  Do not add a new test and do not modify the reflow tool.
- **Commit:** `docs(designs): close the reed/fabric doc with the leaf-module note and related links`

## Batch Tests

`verify: go test ./internal/lyxcwd/ -run TestEnforcement_MarkdownLinks` runs the same Markdown Link Integrity enforcement test batch 1 uses, scoped by `-run` to that one test.
It is the check that matters most for this batch, because card 11 adds a `## Related` section whose entire content is repo-relative links to sibling documents and to the overview's lifecycle anchor — the densest concentration of link targets in the document.
A failure there means a path or an anchor slug is wrong;
no entry may be added to `docsLinkAllowlist` to make it pass.

The same two non-mechanical checks apply as in batch 1 and are performed by reading, inside card 11's own verification pass: external `http`, `https` and `mailto` targets are skipped by the enforcement test by design, and the semantic-line-break convention is reviewed against the rules `tools/mdreflow` implements rather than asserted by a new test.

This batch adds no `.go` file and no test file.
The repo-wide check that no code file was touched by accident is `pipeline.done_gate`, which mill-go runs once before marking the task done.
