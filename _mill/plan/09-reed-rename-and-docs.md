# Batch: reed-rename-and-docs

```yaml
task: "Relocate producer prompt files into a stencils/ directory"
batch: "reed-rename-and-docs"
number: 9
cards: 4
verify: go build ./... && go test ./internal/reedengine/... ./internal/lyxcwd/... ./cmd/lyx/...
depends-on: [8]
```

## Rename mechanic

For each `Moves:` pair the implementer MUST:

1. Run `git mv <old> <new>` FIRST, before making any other change to the moved file.
2. Make ONLY surgical edits — touch only the lines that must change after the move (package or module declaration, imports, identifier retargeting, seam splits).
3. Use a full-file `Creates:` entry only for genuinely new files that have no predecessor.
4. Never write the relocated file from scratch and delete the original — that breaks git rename history and inflates review diffs.

## Batch Scope

The closing batch: reed's rename, the new CONSTRAINTS invariant plus the three amended bullets, the module-table row in `docs/overview.md`, and the stale-reference sweep across prose that still names the fifteen old filenames.

Reed's file is the sixteenth `.md` under `internal/` and the one false positive in that count — it is a tmux pane display banner rendered through `internal/tokenvocab` by `internal/reedengine/header.go`, not a producer prompt.
It stays in `internal/reedengine`, stays embedded, and stays entirely outside the stencil mechanism; only its filename and its doc comment change.
Dropping "template" from the name stops the word denoting three unrelated things, and `console-header.md` says what it is where a bare `header.md` would collide visually with `header.go` beside it.

Batch-local decision on what this plan does **not** do: the discussion's scope includes rewriting the wiki task's body so it describes the mechanism actually built rather than the junction layout the spike disproved.
That is deliberately excluded from every card here.
`CLAUDE.md` states that all wiki interaction goes through mill's wiki module and that raw `git`, `Edit`/`Write`, or `cp` on wiki files is never permitted, so no implementer card can perform it.
It is an operator action to take through `/mill-*` after this task merges.

## Cards

### Card 37: Rename reed's header asset to `console-header.md`

- **Context:**
  - `internal/reedengine/header.go`
  - `internal/tokenvocab/render.go`
- **Edits:**
  - `internal/reedengine/headertemplate.go`
  - `internal/reedengine/console-header.md`
  - `.gitattributes`
- **Creates:** none
- **Deletes:** none
- **Moves:**
  - `internal/reedengine/header-template.md` -> `internal/reedengine/console-header.md`
- **Requirements:**
  Update the `//go:embed header-template.md` directive at `headertemplate.go:10` to `//go:embed console-header.md`.
  Keep the package var `headerTemplate` and the exported accessor `HeaderTemplate()` exactly as they are — this is a file rename, not an identifier rename, and `header.go:13`'s `template = HeaderTemplate()` call must not change.

  Rewrite `headertemplate.go`'s doc comment at lines 2-4.
  It currently names the asset by its old filename and describes the `*-template.md` convention this rename retires for it.
  The new comment must name `console-header.md` and state the render path precisely: the asset is rendered via `tokenvocab.Render` (`internal/tokenvocab/render.go:12`), which is itself a thin wrapper over `stencil.Fill`.
  The existing "rendered via internal/stencil" wording is **true** and must be preserved in substance rather than "corrected" to say otherwise — `tokenvocab.Render` calls `stencil.Fill` directly.
  Add that this asset is deliberately outside the stencil mechanism: it is a tmux pane display banner, not a producer prompt, so it stays embedded and is never seeded, stamped, or read from the hub's stencils directory.

  The asset's own leading banner names its old filename at `console-header.md:1` and moves with it — rewrite that line to name `console-header.md`.
  Do not change the `{{.repo}}` or `{{.hub}}` tokens, the token-vocabulary sentence, or the `Config.Header.Template` override sentence.

  In `.gitattributes`, add `internal/reedengine/console-header.md text eol=lf` with the other embedded-asset pins.
  This file is unpinned today — an asymmetry with burler's and treadle's rows — so this closes a pre-existing gap.
- **Commit:** `refactor(reedengine): rename header-template.md to console-header.md`

### Card 38: Record the Stencil Ownership Invariant and amend three existing bullets

- **Context:**
  - `internal/stencilstore/doc.go`
  - `internal/stencilcli/cli.go`
  - `internal/lyxcwd/enforcement_test.go`
  - `stencils/stencils.go`
  - `cmd/lyx/seamsignature_test.go`
- **Edits:**
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add a new `## Stencil Ownership Invariant` section stating the rules only, in the file's existing rules-not-rationale voice:
  every producer prompt is read at call time from `<hub>/_board/_lyx/stencils/`, never from embedded bytes;
  `//go:embed` in the top-level `stencils` package carries seed defaults only and is never a live read path;
  `internal/stencilstore` is the sole owner of seeding, hash-stamping, edit detection, reading, and validation, and takes a fully resolved absolute base directory from its caller;
  a file whose body hash does not match its stamp is never overwritten;
  the seed/refresh pass runs once per process at `cmd/lyx`'s root pre-run, never lazily inside `stencilstore.Read`;
  and the seeding commit is a `board.lock`-holding, positive-pathspec commit through `internal/fabricengine`, never `Bolt` and never a stage-all.
  Close with an **Enforced by** line naming `stencils/registry_test.go` for registry completeness, `internal/stencilstore`'s edit-detection tests, and `internal/lyxcwd/enforcement_test.go` for the vocabulary walk — and state honestly what is *not* reached: `stencils/stencils.go` is production Go outside `internal/` and `cmd/`, so it falls outside the Go half of the Fabric Vocabulary walk, whose `.md` half does now cover `stencils/**/*.md`.

  Amend the **CLI / Cobra Invariant** section:
  - The seam counts are hardcoded in its own text. "Ten of the eleven seam modules" becomes eleven of the twelve, and every other count in that section moves from eleven to twelve accordingly, including the **Enforced by** line's "across all eleven modules" and "across the ten modules that carry it".
  - Record the **named deviation** from the package-naming rule: `stencilcli`'s domain kernel is `internal/stencilstore`, not `stencilengine`. The reason is that `internal/stencil` already holds the singular name and the top-level `stencils` package holds the plural, so a `stencilengine` would make three packages one character apart, and `stencilstore` says what the package actually is. State it as a deviation with a reason so the next reader does not find a rule apparently broken.

  Amend the **Fabric Vocabulary Invariant** section's coverage paragraph and its **Enforced by** line: the `.md` walk now covers `internal/**/*.md` **and** `stencils/**/*.md`.

  Do not restate the Treadle Runner-Seam Invariant amendment — batch 5 card 23 already landed it in the same commit as the import that needed it.
- **Commit:** `docs(constraints): record the Stencil Ownership Invariant and amend the CLI seam counts`

### Card 39: Add the `stencil` row to the module table and retarget `docs/overview.md`'s prompt references

- **Context:**
  - `internal/stencilcli/cli.go`
  - `CONSTRAINTS.md`
  - `CLAUDE.md`
  - `stencils/loom/loom-template-discussion.md`
  - `stencils/loom/loom-template-plan.md`
- **Edits:**
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add a `stencil` row to `docs/overview.md`'s module table, following the table's existing column shape and alphabetical or positional convention, describing the module as the operator surface over the hub's producer-prompt stencils: `list`, `validate`, `diff`, `sync`, `promote`.

  Do **not** touch the "Execution stack (orchestration layers)" section at `docs/overview.md:309`.
  That section is the proc/reed/shuttle/burler/perch spawn layering, and a file-reading CLI module is not a layer in it.
  `CLAUDE.md`'s task-completion rule is "the module table **or** the execution stack", so the table alone keeps this compliant.

  Retarget the two stale filename references in the same file: `docs/overview.md:288` names `internal/loomengine`'s `discussion-template.md` and `docs/overview.md:289` names its `plan-template.md`.
  Rewrite both to name `stencils/loom/loom-template-discussion.md` and `stencils/loom/loom-template-plan.md`, and adjust the surrounding sentence so it no longer implies the prompt lives inside `internal/loomengine`.

  Any new link added from this file into `stencils/` is resolved by `TestEnforcement_MarkdownLinks`, so a path must be exactly right — the Markdown Link Integrity rule scans `docs/` and `manifest/` as sources and resolves every target wherever it lands.
- **Commit:** `docs(overview): add the stencil module row and retarget loom prompt paths`

### Card 40: Sweep the remaining stale filename references across `manifest/` and `CLAUDE.md`

- **Context:**
  - `docs/overview.md`
  - `stencils/stencils.go`
  - `README.md`
- **Edits:**
  - `manifest/designs/loom.md`
  - `manifest/designs/scout-plan-symbol-fields.md`
  - `manifest/designs/shed-followups.md`
  - `CLAUDE.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Re-run the sweep rather than trusting a recorded list: `grep -rn` for each of the fifteen **old** filenames across `docs/`, `manifest/`, `CLAUDE.md`, `README.md`, and `internal/**/*.md`, and fix every hit that names a relocated file.
  The sweep must cover all five roots, not `docs/` alone: none of these references is a markdown link, so `TestEnforcement_MarkdownLinks` catches none of them, and a grep is the only thing that finds them.

  Known hits at planning time, to be confirmed and fixed:
  - `manifest/designs/loom.md:193` — names both `discussion-template.md` and `plan-template.md` in the producers table row.
  - `manifest/designs/scout-plan-symbol-fields.md` — seven mentions of `plan-template.md`, in a design whose whole subject is editing that file. Retarget each to `stencils/loom/loom-template-plan.md`. Do not otherwise rewrite that document's argument; it remains speculative and unscoped, and this is a path correction only.
  - `manifest/designs/shed-followups.md:180` — names `internal/loomengine, including plan-template.md`.
  - `CLAUDE.md:67` — names `master-template.md` inside the Merriam terminology section. Retarget to `stencils/webster/webster-template-master.md`. Do not rename any identifier: that section explicitly states it is shorthand for talking about the session, not an instruction to rename anything.

  Where a sentence describes a relocated prompt as living inside its engine package, rewrite it to say the prompt ships as an embedded default in the top-level `stencils` package and is read at call time from the hub's stencils directory.
  Do not edit any file under `_mill/` — those are this task's own planning artifacts, not repo documentation.
  Do not edit `internal/burlerengine/doc.go` or `internal/websterengine/render.go`'s header comment: batches 4 and 6 already retargeted them, and re-editing them here would produce a redundant diff.
- **Commit:** `docs: retarget stale producer-prompt filename references repo-wide`

## Batch Tests

`verify: go build ./... && go test ./internal/reedengine/... ./internal/lyxcwd/... ./cmd/lyx/...`

`internal/reedengine` covers the rename: its existing header tests, including `TestValidateHeader_UnknownTopLevelTokenErrors` and `TestUp_BadHeaderTemplateFailsBeforeAnyTmuxContact`, fail if the `//go:embed` directive and the moved file disagree.
`internal/lyxcwd` runs both `TestEnforcement_MarkdownLinks` — the guard on any new link this batch's doc edits introduce into `stencils/` — and the Fabric Vocabulary walk over the renamed reed asset.
`cmd/lyx` runs the seam and coverage guards one final time against the completed module set.
`go build ./...` catches an embed directive left pointing at the pre-rename filename, which is the single most likely way to get card 37 wrong.
The repo-wide regression gate for the whole task is `pipeline.done_gate`, already configured as `go test ./... && go test -tags integration ./...` — that is what runs the five engines, the enforcement walk, the import allowlist, and the cobra root together, and it is deliberately not duplicated into this batch's `verify:`.
