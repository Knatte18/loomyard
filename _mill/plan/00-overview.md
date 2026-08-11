# Plan: gitexec: decide whether RunGit should return a typed error carrying stderr

```yaml
task: 'gitexec: decide whether RunGit should return a typed error carrying stderr'
slug: 'gitexec-error-shape-decision'
approved: false
started: '20260811-034300'
parent: 'main'
root: ""
verify: null
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: verdict-record
    file: 01-verdict-record.md
    depends-on: []
    verify: go test ./internal/lyxcwd/ -run TestEnforcement_MarkdownLinks
```

## Shared Decisions

### Decision: no production code is touched

- **Decision:** This task edits exactly two files — the design doc and the roadmap — plus one wiki-side task record created through mill's wiki daemon client.
  No file under `internal/`, `cmd/`, or `tools/` is read for editing purposes or changed, and `CONSTRAINTS.md` is not edited.
- **Rationale:** The task's output is a verdict, not a migration.
  The `GitError` type, the `gitexec.Run` entry point, the `gitrepo` checked sibling, the gitexec Checked-Call Invariant and its guard test are all *specified* here and *built* by the follow-on task.
  An invariant with no enforcing test is exactly the rot `CONSTRAINTS.md` exists to prevent, so it lands with its guard test in the implementation commit.
- **Applies to:** all batches

### Decision: the discussion is the source material, not a starting point to re-derive

- **Decision:** Every fact, table, query, snapshot date and rejected-alternative in the rewritten verdict comes from `_mill/discussion.md`.
  The implementer transcribes and reorganises;
  it does not re-measure the tree, re-run any grep, or "check" a count against the code.
- **Rationale:** The counts in the discussion were measured on 2026-08-10/2026-08-11 and are deliberately labelled as snapshots.
  Re-measuring now produces a different snapshot with no more authority and silently desynchronises the doc from the reasoning that produced it.
  The doc's contract is that its inventories are *re-derivable* — it carries the query, not fresh coordinates.
- **Applies to:** all batches

### Decision: file:line coordinates are always labelled as snapshots

- **Decision:** Wherever the verdict carries a file:line list, it is introduced by an explicit snapshot marker naming the date it was measured, and is accompanied by the query that regenerates it.
  The acceptance bar written into the doc is **re-derivable from the doc alone**, never "executable from the doc alone".
- **Rationale:** The implementation sits behind the serialised fabric chain, which rewrites the exact code the inventories point at.
  Presenting stale coordinates as fact is the specific failure this framing exists to prevent.
- **Applies to:** all batches

### Decision: markdown style is semantic line breaks, enforced by mdreflow

- **Decision:** Both edited files use one sentence per line with extra breaks at internal clause boundaries, no fixed-column wrapping, and table cells and blockquotes on one line.
  Each card runs `go run ./tools/mdreflow` over the file it wrote and resolves any reported MISMATCH before committing.
- **Rationale:** Repo convention, stated in `CLAUDE.md` and in the discussion's Repo-conventions section.
  `mdreflow` verifies that a reflow changed line breaks only — a MISMATCH means content moved and must be investigated, not overwritten.
- **Applies to:** all batches

### Decision: every relative link in both files must resolve

- **Decision:** The rewritten design doc keeps its two existing outgoing links live, the roadmap keeps its link to the design doc live, and any intra-document anchor the verdict introduces must match a real heading in the file it points into.
- **Rationale:** Markdown Link Integrity is a `CONSTRAINTS.md` invariant enforced by `TestEnforcement_MarkdownLinks`, which is this plan's batch verify.
  The design doc is rewritten rather than deleted by this task precisely so the roadmap link survives;
  removing that link is the implementation task's job, in the commit that deletes the doc.
- **Applies to:** all batches

## All Files Touched

- `manifest/designs/gitexec-error-shape.md`
- `manifest/roadmap.md`
