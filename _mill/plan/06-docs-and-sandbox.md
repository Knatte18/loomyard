# Batch: docs and sandbox suites

```yaml
task: 'fabric: store the warp-URL binding in weft:main; fold bootstrap into clone (slice 10)'
batch: 'docs and sandbox suites'
number: 6
cards: 5
verify: go test ./internal/lyxcwd/ ./cmd/lyx/
depends-on: [5]
```

## Batch Scope

This batch updates every document that spells the clone argument order or describes the binding, folds the durable behaviour into the package doc, marks slice 10 shipped, and adds the sandbox scenario that exercises the one-argument bound clone.
It is one batch because the edits are all prose over a settled implementation, they share one mental model, and two of them are machine-checked by the same two test packages.

No external interface is produced;
this is the last batch.

Batch-local decision: `tools/sandbox/SANDBOX-CORE-SUITE.md` has a known-stale claim about unwire clearing weft-side content, unrelated to this task.
Do not fix it here — it belongs to whoever audits the sandbox docs.

Markdown in this repo uses semantic line breaks: one sentence per line, with an extra break at an internal independent-clause boundary.
Never hard-wrap at a fixed column, and never use trailing double-spaces or a backslash for a break.
This applies to every line touched in this batch, in every `.md` file, not only newly written ones.

## Cards

### Card 17: fold the durable behaviour into the package doc

- **Context:**
  - `internal/fabricengine/clone.go`
  - `internal/fabricengine/warpbinding.go`
  - `internal/fabricengine/warpprobe.go`
  - `internal/fabricengine/reconcile.go`
- **Edits:**
  - `internal/fabricengine/doc.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  The package doc's clone-does-everything paragraph currently describes a clone that takes two URLs and records the anchor and the repo-wide config on the weft's main branch.
  Extend that passage — do not rewrite the surrounding slice-5 narrative — to state:
  the warp binding is a fourth repo-wide record beside the anchor and the repo-wide config, held as a plain single-line file at the board root containing the warp URL only;
  clone resolves the effective warp URL from that record when no warp URL is supplied, and writes the record when none exists and one is supplied;
  a supplied URL that disagrees with the record is a hard error and is never silently re-pointed;
  the resolution happens through a throwaway pre-hub probe clone of the weft remote, because the hub is named after the warp repo and so has no path until the warp URL is known;
  and `Reconcile` backfills the record once per hub from the warp side's `origin`, with the CLI layer driving the commit and push.

  Also state that the three repo-wide records — anchor, repo-wide config, and binding — are what let a later reconcile re-wire a hub with no re-clone, and that unwire leaves all three untouched.

  Keep the existing vocabulary rules: this package is in the Fabric Vocabulary Invariant's owner set, so warp and weft are used freely, but `host` in any fabric sense is banned here too.
  The enforcement test walks this file, so re-read the added prose for accidental `host repo` / `host worktree` phrasing before committing.
- **Commit:** `docs(fabricengine): document the warp binding in the package doc`

### Card 18: mark slice 10 shipped in the design doc and the roadmap

- **Context:**
  - `internal/fabricengine/clone.go`
  - `internal/fabricengine/warpbinding.go`
  - `internal/fabriccli/fabric.go`
- **Edits:**
  - `manifest/designs/fabric-unified-view.md`
  - `manifest/roadmap.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In the design doc's slice 10 section:
  - Mark the slice shipped in the same style the neighbouring shipped slices use.
  - Correct the pre-rename `.fabric-anchor` reference to the current anchor marker name.
  - Record the divergence from the doc's own "warp URL + `--subpath`" wording: the record holds the warp URL ONLY.
    The subpath already has one authoritative home in the anchor marker, and a second copy would create two records that can disagree with no rule for which wins.
  - Record the hub-name collision as a known limitation rather than a fix: the hub stays `<cwd>/<warp-name>-HUB`, so two wefts of the same warp collide on that directory name.
    That is true today, it is not what this slice is about, and nothing here changes it.
  - Record the additions the doc never anticipated: the pre-hub probe, the old-order bootstrap guard and its `--force-bootstrap` escape, the options-struct signature, and the reconcile backfill with its outcome set.
  - Leave the two example lines alone — they already show the correct weft-first order.
    Delete instead the prose claim in the following paragraph that this file's own examples still show today's order and must be corrected in the same commit as the flip;
    that claim is inverted and is falsified by the examples immediately above it.

  In the slice 9 section of the same file, update the sequencing note: the sentence saying slice 10 is still pending and still collides on the clone handler is falsified by this task and must state instead that slice 10 has now shipped.

  Do not delete the design doc.
  Its header says it is deleted once slice 10 and slice 6's open orchestration half are both done;
  slice 6's half is still open, so the file survives this task.
  Card 17 is what folds the durable behaviour into the package doc so the eventual deletion loses nothing.

  In the roadmap, move slice 10 to completed: the entry currently lists slices 8 and 10 as remaining and describes slice 10 as pending.
  This is a planned-item completion, which is exactly what the roadmap moves for.

  While rewriting that entry, resolve the contradiction it carries rather than preserving it: the roadmap calls slice 8's CLI-wording policy question open, but the design doc's own slice 8 section is headed shipped and states that the question was resolved — consumer-emitted prose says "fabric," never "weft," while the wrapped error detail fabric itself produces keeps naming the weft repo and path freely.
  The design doc is the authority here, so the roadmap entry should record slice 8 as resolved too.
  With both slices settled, the entry reduces to a statement that the fabric campaign's slices are complete;
  keep the pointer to the design doc, which survives this task because slice 6's orchestration half is still open.

  Relocate the bullet accordingly: it currently sits under the roadmap's Planned heading, and the file's own Maintenance section says an item moves from Planned to Done when it ships.
  A "complete" statement left under Planned would contradict itself, so move the whole entry to the Done section, preserving the design-doc pointer and renumbering per whatever convention the Done section already uses.

  The move orphans one same-file cross-reference: the Someday section's repo-wide-config item points readers at "the Planned `fabric` item's slices 7-10" when explaining that the fabric config is the sole `_board`-anchored exception.
  Update that pointer to name the Done section instead, in the same edit — otherwise the file contradicts itself the moment the bullet moves.
- **Commit:** `docs(manifest): mark fabric slice 10 shipped`

### Card 19: flip the constraint example and fix the stale anchor name

- **Context:**
  - `internal/fabriccli/fabric.go`
  - `internal/lyxcwd/anchor.go`
- **Edits:**
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In the Fabric Vocabulary Invariant, the illustrative command that shows where warp and weft genuinely must be told apart spells the old warp-first order.
  Flip it to the new form, `lyx fabric clone <weft-url> [<warp-url>]`, keeping the sentence's structure and the adjacent `fabric: warp/weft out of sync` example untouched.

  Drive-by fix while in the file: the Cwd Resolution Invariant's bullet describing where the anchor subpath resolves from still names the pre-rename `.fabric-anchor` marker, stale since the rename.
  Correct it to the current marker name.
  Do not change what that bullet asserts — only the filename.

  Do not add a new invariant.
  This change introduces no new cross-cutting rule: the binding is governed by the invariants already listed (Cwd Resolution keeps it out of the cwd resolver, Fabric Git keeps its git through the engine, Fabric Vocabulary governs its naming, Never Force-Add governs its staging).
  Say nothing in this file about the binding itself.
- **Commit:** `docs(constraints): flip the clone example and fix the stale anchor marker name`

### Card 20: update the overview and the sandbox-hub walkthrough

- **Context:**
  - `internal/fabriccli/fabric.go`
  - `tools/sandbox/main.go`
- **Edits:**
  - `docs/overview.md`
  - `docs/sandbox-hub.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In the overview, check both places that mention clone — the junction-model paragraph describing eager wiring at clone/add time, and the fabric module entry in the module list.
  The junction-model paragraph does not spell an argument order and needs no change unless the reader would be misled;
  leave it alone if so.
  The fabric module entry describes the CLI surface as a verb list;
  extend its clone description so it states that clone takes the weft URL first with the warp URL optional, derived from the binding recorded on the weft's main branch, and that reconcile backfills that binding for hubs predating it.
  Keep the entry's existing shipped-status marker and its pointer to the package documentation.

  In the sandbox-hub walkthrough, the numbered step that spells the full clone command is warp-first.
  Flip it to weft-first so it matches what the launcher now runs after the sandbox call sites were flipped.
  Leave the surrounding steps, the hub path, and the exit-code step unchanged.
- **Commit:** `docs: describe the weft-first clone form in the overview and sandbox walkthrough`

### Card 21: flip the suite docs and add the bound-clone scenario

- **Context:**
  - `internal/fabriccli/fabric.go`
  - `tools/sandbox/main.go`
  - `internal/fabricengine/warpbinding.go`
- **Edits:**
  - `tools/sandbox/SANDBOX-CORE-SUITE.md`
  - `tools/sandbox/SANDBOX-FABRIC-SUITE.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In the core suite, the subpath-anchored-clone scenario spells the full command warp-first in its Goal line only — its Watch line names the command without URLs and needs no change.
  Flip the Goal line to `lyx fabric clone --subpath <sub> <weft-url> <warp-url>`.
  Leave the durability note, the reset guidance, and the rest of the scenario as they are, and do not touch the unrelated stale claim about unwire clearing weft-side content — that belongs to a separate audit.

  In the fabric suite, flip any spelled clone argument order in the preamble and the existing scenarios.
  While in the file, correct one stale marker name: the unwire scenario's Watch line names the pre-rename `.fabric-anchor` when listing the repo-wide records that survive unwire.
  Correct it to the current anchor marker name — the same staleness card 19 fixes in the constraints file, in the same batch — and, since the binding is now a third such record, add it to that same list.
  Then add a new scenario at the end of the scenario list, taking the next number in the file's existing `F<N>` sequence — appended rather than inserted next to the clone-geometry scenario it is thematically closest to, because inserting would renumber every scenario after it and the session-log block that mirrors them.
  It must carry a `**Covers:** fabric` line, a Goal, a Watch, and the same `**Verdict:** \`OK\` / \`WARN\` / \`FAIL\`` line every other scenario in the file ends with.
  Append the matching `F<N>: <OK|WARN|FAIL> -- <one-line note if not OK>` line to the file's closing session-log-format block, which carries one line per scenario and would otherwise be short by one.

  The scenario's substance: after the dedicated fabric hub has been cloned once with both URLs — which is what writes the binding — delete the hub directory outright and re-clone with the weft URL alone.
  Watch that the warp URL is derived rather than asked for;
  that the hub comes up at the same path and identically wired (the same `_board`-is-a-weft-worktree and `main-weft` branch checks the clone-geometry scenario already makes);
  that the success envelope reports the derived warp URL and reports that the binding was not re-written;
  and that the binding record is present and tracked at the board root.
  Mention that the record's filename is the one the fabric module documents, so the operator confirms it rather than guessing.

  Note that `tools/` is outside the Fabric Vocabulary Invariant's enforcement walk, so vocabulary in these two files is a review obligation rather than a machine check — use warp and weft precisely and never `host` in a fabric sense.
  The suite-coverage test counts modules, not scenarios, and fabric is already a covered module, so this adds depth rather than a new coverage row.
- **Commit:** `docs(sandbox): flip the clone order and add the bound one-argument scenario`

## Batch Tests

`verify:` is `go test ./internal/lyxcwd/ ./cmd/lyx/`, both untagged.

`internal/lyxcwd` is where the two enforcement tests live that this batch can actually break: the fabric-vocabulary walk covers production Go under `internal/` and every `internal/**/*.md`, so card 16's new package-doc prose is machine-checked there, and the geometry-literal check runs in the same package.

`cmd/lyx` carries the sandbox-coverage test, which parses `**Covers:**` lines out of the suite files card 20 edits, plus the help-tree and drift tests that would catch a `Use` string this batch's prose disagrees with.

No `-tags integration` is needed: this batch edits no test file and no integration-tagged file, and both target packages' relevant tests are untagged.
The overview's module-wide `go build ./...` still runs at the batch boundary and covers card 16's edit to a production Go file.
