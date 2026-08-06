# Batch: documentation

```yaml
task: 'fabric: close the weft-visibility leak (slice 8)'
batch: 'documentation'
number: 8
cards: 3
verify: go vet ./internal/fabricengine/
depends-on: [7]
```

## Batch Scope

Lands the durable documentation per decisions `documentation` and `doc-vocabulary-split`: `fabricengine`'s package doc absorbs the new contract, `CONSTRAINTS.md` gains the "Fabric Vocabulary Invariant" section, `docs/overview.md` widens its Cwd-invariant section, the design doc's slice-8 section compacts, and the consumer-behaviour prose docs reword.
Runs after batch 07 so the constraint text can name `TestEnforcement_FabricVocabulary` as an existing test.
The prose-doc split rule: a doc explaining fabric's own mechanism keeps the vocabulary (README architecture section, Fabric Git Invariant, the design doc);
a doc describing a consumer module's behaviour rewords.

## Cards

### Card 27: fabricengine package doc

- **Context:**
  - `internal/fabricengine/open.go`
  - `internal/fabricengine/ready.go`
  - `internal/fabricengine/refscanner.go`
  - `internal/fabricengine/commit.go`
  - `internal/fabricengine/drift.go`
  - `_mill/discussion.md`
- **Edits:**
  - `internal/fabricengine/doc.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** The package doc absorbs the durable contract: `Open` is the only constructor outside the package;
  `Committed()` is the only commit result a consumer reads (the raw `CommitResult` fields are fabriccli-only);
  `RefScanner` is how a consumer asks about fabric-driving commands;
  `Healthy` returns the typed `HealthReason`;
  and the vocabulary rule with its owner set (`fabricengine`, `fabriccli`, `weftname`, `lyxtest`, `boardengine`, `configsync` string-literal-only, `tools/`/`sandbox/`).
  As an owner file, `doc.go` keeps warp/weft vocabulary freely when explaining the mechanism.
- **Commit:** `docs(fabricengine): package doc records the one-repo contract`

### Card 28: CONSTRAINTS.md and docs/overview.md

- **Context:**
  - `internal/lyxcwd/enforcement_test.go`
  - `_mill/discussion.md`
- **Edits:**
  - `CONSTRAINTS.md`
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `CONSTRAINTS.md`:
  (a) add a new top-level section "Fabric Vocabulary Invariant" (its own section, NOT under the Cwd Resolution Invariant) stating: the policed tokens (`weft`, `warp` as bare tokens; `host` via the phrase predicate with the phrase list), the owner set, the prose-doc mechanism-vs-behaviour split from `doc-vocabulary-split` as a review obligation, the note that the test's placement in `internal/lyxcwd/enforcement_test.go` is a walk-helper convenience and not an ownership claim, and an **Enforced by** bullet naming `TestEnforcement_FabricVocabulary`;
  (b) the Cwd Resolution Invariant's fabric bullet (`:25-26`) gains a cross-reference to the new section and nothing more;
  (c) the Fabric Git Invariant keeps its warp/weft heading and body, but its **Enforced by** bullet (`:148-151`) is corrected — it names `websterengine`'s `weftReferencePattern`, a symbol this task deleted;
  it becomes `fabricengine.RefScanner`;
  and its "agent prompt templates never instruct a weft git op" clause is restated to the stronger `templates-describe-one-repo` rule: templates never mention the two-repo structure at all.
  Any clause left invalid is removed, never left stale.
  In `docs/overview.md`: the Cwd Resolution Invariant section (`:63-79`) gains the vocabulary rule alongside the existing enforcement-test description.
  Follow the repo's semantic-line-break markdown convention.
- **Commit:** `docs: add Fabric Vocabulary Invariant to CONSTRAINTS.md and overview`

### Card 29: design doc and consumer-facing prose docs

- **Context:**
  - `_mill/discussion.md`
  - `manifest/roadmap.md`
- **Edits:**
  - `manifest/designs/fabric-unified-view.md`
  - `README.md`
  - `docs/skills.md`
  - `docs/reference/builder-contract.md`
  - `docs/benchmarks/test-suite-timing.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** `manifest/designs/fabric-unified-view.md`: compact slice 8's section to a shipped summary and resolve/remove the "Open questions" entry "Slice 8's CLI-wording question" (`:185`);
  the file itself survives (its own header says it lives until slice 10 and slice 6's open half land).
  Per decision `doc-vocabulary-split` — KEEP vocabulary in `README.md:50,55,57,58,61,81` (the architecture section) and the rest of the design doc;
  REWORD `README.md:62` ("`_lyx/` is durable and weft-synced" → fabric-synced state semantics), `docs/skills.md:14,167,184`, `docs/reference/builder-contract.md:22,24` ("Performs the loop's exit-time backstop weft commit" → fabric-worded: builder's contract must not say weft exists), and `docs/benchmarks/test-suite-timing.md`'s weft mentions.
  `manifest/roadmap.md` does NOT move — slice 8 is a planned item inside an existing campaign.
  Follow the semantic-line-break convention;
  do not hard-wrap.
- **Commit:** `docs: slice-8 shipped summary and consumer-doc vocabulary split`

## Batch Tests

`verify:` runs `go vet ./internal/fabricengine/` — the only compiled surface this batch touches is `doc.go`;
the markdown edits have no runnable surface and are review-checked, and the repo-wide done gate re-runs the full suite after this final batch.
