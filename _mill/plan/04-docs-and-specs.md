# Batch: docs-and-specs

```yaml
task: "Producer-agnostic final-summary artifact + wire Finalize"
batch: "docs-and-specs"
number: 4
cards: 4
verify: go test ./internal/lyxcwd/...
depends-on: [3]
```

## Batch Scope

This batch lands the documentation half of the task: the artifact's contract stops being webster's and moves to its own durable spec file, `webster-spec.md` shrinks to a pointer plus webster's own writer-side additions, `docs/overview.md` learns about both, and the roadmap item is marked complete.
It runs last so every claim it pins is already true in the code.

It is four cards ordered by dependency: the new spec file exists before `webster-spec.md` points at it and before `docs/overview.md` links to it, which is what keeps the Markdown Link Integrity invariant satisfied at each card's own commit.

Card 11 also repairs pre-existing drift on the exact line it is editing: `contracts/specs/loom-plan-spec.md` exists and is already linked from the module list, but is missing from the kept-durable-contract-docs enumeration.
Adding to a knowingly incomplete list without fixing it would make the next reader trust it less, not more.

## Cards

### Card 9: Pin the final-summary artifact contract in its own spec

- **Context:**
  - `contracts/specs/webster-spec.md`
  - `contracts/specs/loom-status-spec.md`
  - `internal/summaryparser/summary.go`
  - `internal/landingshed/publish.go`
  - `internal/landingshed/finalize.go`
- **Edits:** none
- **Creates:**
  - `contracts/specs/final-summary-spec.md`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `contracts/specs/final-summary-spec.md` pinning the final-summary artifact producer-agnostically, following the shape and the pinned-status blockquote convention of the sibling files in `contracts/specs/`.
  Write it as one sentence per line, breaking long sentences at internal independent-clause boundaries.

  It must state:

  - The artifact is the prose final-summary a run's last content-producing step writes: a first non-blank line `# <title>`, then free-form prose narrating what was actually built, including deviations from the original task.
  - The format is fail-loud and minimally validated — the file must be readable, must not be whitespace-only, its first non-blank line must be a `# ` heading, and the title after that prefix must be non-empty — with each violation its own distinct error.
  - `internal/summaryparser` is the sole declarer of the filename and the sole parser of the format, and it takes a told path.
  - The contract names no location: the artifact's directory belongs to whichever producer writes it, and the consumer is handed the path.
  - Its two consumers today, both in `internal/landingshed`: `Publish` uses the parsed title and body as the pull request's own title and body fields, and `Finalize` uses `CommitMessage` — the title, a blank line, and the body with its leading whitespace trimmed — as the landing merge commit's message.
  - `Finalize`'s read is unconditional and a missing or malformed artifact there is a hard error, while `Publish`'s is reached only when the parent branch requires a pull request and no pull request already exists.
  - A producer may append its own sections to the body after writing it, and any such section rides into both consumers unchanged.

  It must NOT say anything about warp or weft, or about which side of a pair a commit lands on — the pair-side reach is an implementation fact of `internal/fabricengine`, not part of this contract.
  It must NOT restate webster's own archive discipline or its integration-failure escalation; those stay in `webster-spec.md`.
  Every inline markdown link written here must resolve, which is review discipline rather than test coverage: the Markdown Link Integrity invariant scans `manifest/` and `docs/` only, so `contracts/specs/` is a link target and never a scanned source.
- **Commit:** `docs(specs): pin the producer-agnostic final-summary artifact contract`

### Card 10: Reduce webster-spec.md to a pointer plus its writer-side additions

- **Context:**
  - `contracts/specs/final-summary-spec.md`
  - `internal/landingshed/publish.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `contracts/specs/webster-spec.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Shrink `contracts/specs/webster-spec.md`'s `## The summary artifact — _lyx/webster/summary.md` section to a pointer at `contracts/specs/final-summary-spec.md` plus webster's own writer-side additions only.
  Delete the restated format and validation rules — the `# <title>` heading, the non-empty title, the required-and-fail-loud-on-`outcome: done` sentence's format half — and replace them with a link to the new spec, per the Producer Pointer-Rule Invariant.
  Keep what is webster's own: that the artifact is required on `outcome: done`, that it follows the same archive-never-refuse discipline as every other stale artifact, and that a long-lived Master session is the party with full oversight of what actually shipped.

  Correct the retained Master-oversight sentence while keeping it.
  It reads today "It is Finalize's PR-text source, because a long-lived Master session is the only party with full oversight of what actually shipped", and its "Finalize's PR-text source" clause carries the same Finalize/Publish mislabel as the `## Integration suite failed` sentence below it: the pull-request text is `Publish`'s, and after this task `Finalize` consumes the artifact for the landing commit message instead.
  Rewrite the clause to point at `contracts/specs/final-summary-spec.md` for both consumers rather than naming either one wrongly, and keep the Master-oversight reason unchanged.

  Rewrite, rather than copy across, the retained `## Integration suite failed` sentence.
  It reads today "because Finalize dumps `summary.md` verbatim into the PR body", which is wrong on both counts: the pull-request body is `Publish`'s, and nothing dumps the file verbatim — `Publish` passes the parsed title and body as separate fields.
  Name `Publish` and drop "verbatim".
  Keep `internal/websterengine`'s `AppendIntegrationFailure` as the named writer and keep the statement that the bisect mechanism producing it stays webster-internal.

  Add `final-summary-spec.md` to the file's `## See also` list, as a relative link in the same form the existing `llm-model-spec.md` and `loom-status-spec.md` entries use.
  Leave the `## _lyx/webster/ as an ownership boundary` section alone: webster still owns that directory and `internal/websterengine`'s `Dir`/`ReportsDir` helpers remain the sole declarers of the path segment.
  Write every edited line as one sentence per line.
- **Commit:** `docs(specs): point webster-spec at the final-summary contract`

### Card 11: Register summaryparser and the new spec in docs/overview.md

- **Context:**
  - `contracts/specs/final-summary-spec.md`
  - `contracts/specs/loom-plan-spec.md`
  - `internal/summaryparser/doc.go`
  - `internal/summaryparser/summary.go`
- **Edits:**
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Edit `docs/overview.md` at three sites.

  First, the `## Documentation lifecycle` section's kept-durable-contract-docs enumeration, which today reads `loom-status-spec.md`, `webster-spec.md`, `llm-model-spec.md`.
  Add `final-summary-spec.md` and `loom-plan-spec.md` to it, keeping the existing backticked-name form rather than converting the entries to links.
  The `loom-plan-spec.md` addition repairs pre-existing drift: the file exists in `contracts/specs/` and is already linked from the module list further down, so its absence from this enumeration is a one-word omission on a line this card is editing anyway.

  Second, the module list: add a `summaryparser` row immediately after the existing `discussionparser` row and before the `batcher` row, matching its two neighbours' shape — bolded module name, a one-or-two-sentence description, the parenthesized package path, and the `✅ Implemented.` marker.
  The description states that it is the sole declarer of the final-summary artifact's filename and the sole parser of its format, that it takes told paths and declares no directory of its own, that it is stdlib-only so neither consumer depends on a producer, and that `internal/landingshed`'s `Publish` and `Finalize` and `internal/websterengine` all consume it.
  Include a relative link to the new spec in the same form the `webster` row links `webster-spec.md` and the `planparser` row links `loom-plan-spec.md`.

  Third, the `## Other docs` list.
  Its `webster-spec.md` entry today describes that file as covering "the `_lyx/webster/` boundary, `outcome.yaml`, and the `summary.md` artifact Finalize consumes", which card 10 makes stale on two counts: `webster-spec.md` no longer describes the artifact directly, and the artifact's consumer set is `Publish` and `Finalize`, not `Finalize` alone.
  Rewrite that entry so it names only what `webster-spec.md` still covers, and add a sibling `final-summary-spec.md` entry to the same list describing the artifact contract and both of its consumers.
  Both entries keep the list's existing shape — a relative markdown link, an em-dash, the description, and the `(as-built; kept as a durable contract doc, not deleted on landing)` parenthetical.

  Every link added here must resolve — both the file part and any `#anchor` — since the Markdown Link Integrity invariant is enforced by test over `docs/`.
  Write every edited line as one sentence per line.
- **Commit:** `docs(overview): register summaryparser and the final-summary spec`

### Card 12: Mark the roadmap item complete

- **Context:**
  - `contracts/specs/final-summary-spec.md`
- **Edits:**
  - `manifest/roadmap.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Move the `producer-agnostic final-summary artifact` entry from `manifest/roadmap.md`'s `## Planned` section into its `## Done` section — never merely delete it.
  The file's own `## Maintenance` note states the convention verbatim: "Move an item from Planned or Someday to Done, with a link to its module doc if one exists, when it ships — no renumbering needed anywhere."

  Rewrite the entry to Done-entry length on the way across — a bold name plus one or two sentences of what shipped, per the same Maintenance note's "Entries are short" rule — and drop the Planned entry's trailing "See …" line naming the three source files that were to be changed, which pointed at the code rather than at durable detail.
  Point the Done entry at `contracts/specs/final-summary-spec.md` instead, which is this feature's durable contract doc.
  Write the entry's list marker as the literal `1.`, like every other entry in the file — numbering is rendered, never hand-maintained.

  The item is complete in full: the read contract is a producer-agnostic leaf, `landingshed` takes a told path rather than a producer's directory, and `Finalize`'s `MergeOptions.Message` is wired to the composed title and body.
  Leave the `## Someday` and `## Maintenance` sections untouched.
  Every inline markdown link that survives the edit must still resolve — this file is a scanned source for the Markdown Link Integrity invariant.
  Write any edited line as one sentence per line.
- **Commit:** `docs(roadmap): move the producer-agnostic final-summary artifact to Done`

## Batch Tests

`verify: go test ./internal/lyxcwd/...` runs `docslink_test.go`, the Markdown Link Integrity invariant's enforcing test, which walks every `.md` file under `manifest/` and `docs/` and resolves each inline link's file part and `#anchor`.
That is the only runnable surface this batch has: cards 11 and 12 edit scanned sources, so a broken link either introduces or a stale link left behind fails here.

Cards 9 and 10 are covered by review discipline rather than by test.
`contracts/specs/` is a link *target* for that invariant, never a scanned source, so a broken link inside `final-summary-spec.md` or `webster-spec.md` will not fail `go test` — the distinction matters and is why both cards state the requirement explicitly.
The batch changes no Go code, so no package's own suite is affected; the task's `pipeline.done_gate` still runs the repo-wide suite before the task is marked done.
