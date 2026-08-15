# Discussion format — the `_lyx/discussion/` ↔ `Plan-Write` contract

> **Status: Contract — pinned.** This doc pins the `_lyx/discussion/` directory contract: the artifact `Discussion-Write` produces and `Plan-Write` consumes. Durable reference doc — kept, not deleted on landing — the loom analogue of [plan-format.md](plan-format.md).

## Producer and contract

`_lyx/discussion/` is produced by `Discussion-Write`.
It is validated by `Discussion-Validate` — see [Validation checks](#validation-checks-spec-for-the-future-validator) below, this file's own validation-checks section.
It is reviewed by `Discussion-Review`.

- **`decision-record.md`** — Output, shape pinned in [`decision-record.md` shape](#decision-recordmd-shape) below.
  Consumed as Input by `Plan-Sweep` and `Plan-Write`.
- **`support-log.md`** — Output, shape pinned in [`support-log.md` shape](#support-logmd-shape) below.
  Consumed as Input by `Discussion-Review` only.

## What it is, and who consumes it

`_lyx/discussion/` is a **directory with two files**, a hard access boundary between what `Plan-Write` sees and what stays review-only:

- **`decision-record.md`** — the distilled record.
  `Plan-Write`'s **sole** input: it never reads anything else out of `_lyx/discussion/`.
- **`support-log.md`** — the raw support log.
  Opened and reasoned over by **`Discussion-Review`** — the LLM/`perch` producer that reads it for the anti-circling Review-rounds ledger below — and only existence-checked, never read, by `Discussion-Validate`.
  **Never** read by `Plan-Write`.

Two files, not two sections of one file, on purpose: a distilled digest, never raw prose, is what `Plan-Write` should ever see.
This follows the same principle `internal/websterengine`'s package documentation states directly for the distilled-`Digest`-persisted-at-terminal contract (see also `recordbatch.go`'s `RecordResult.Digest` handling),
and that [`docs/overview.md`](../overview.md#principles) states architecturally: "Go-distilled digests, never raw prose."
A hard filesystem boundary is stronger than a convention about which section an agent may read — `Plan-Write` cannot accidentally ingest the raw interview transcript, or pay its token cost, because the file isn't in its input set.
Filenames are self-describing rather than terse, matching the existing naming.

Both paths are durable **weft-overlay state**: they live under `_lyx/` (git-synced via weft), not `.lyx/`'s ephemeral machine-local state — that is what makes them survive a resume across machines.
Their paths resolve via `internal/loomengine` (`DiscussionDir`, `DiscussionDecisionRecord`, `DiscussionSupportLog`), joined onto `internal/lyxcwd`'s resolved coordinates;
this doc describes the files, it does not construct the paths.

## `decision-record.md` shape

**No frontmatter.**
Two fields plan-format needs are deliberately absent here:

- **No `format:`** — see [status-schema.md](status-schema.md)'s `no-schema-version`: at this scale a version stamp is a rarely-exercised guard that goes stale;
  reintroduce only if a real incompatibility ever forces it.
- **No `approved:`** — approval is recorded in `_lyx/loom/status.json`'s `history` (`{"phase": "discussion", "outcome": "approved", ...}`) — the status file is loom's single total-status locus, so a lone `approved:` flag here would duplicate it.
  This differs from `plan-format.md`, whose `approved:` exists because `lyx webster run` can be invoked standalone, outside loom;
  loom always drives `Plan-Write` *after* approval, so the record needs no standalone gate of its own.

Sections, in this order:

1. **Goal**
2. **Scope**
3. **Decisions**
4. **Constraints**
5. **Auto-mode assumptions**
6. **Open risks**
7. **Acceptance criteria**

Plus an **optional, non-binding** subsection at the end:

8. **Notes for the plan writer** (optional)

Compaction rules `Discussion-Write` follows when writing this file:

- **Decisions carry Decision + Rationale only.**
  Rejected alternatives do **not** appear here — they belong in `support-log.md`'s Rejected alternatives section.
  A decision record that re-litigates what was *not* chosen is not distilled.
  Do not enumerate rejected alternatives here at all — that belongs to `support-log.md`, not to this file.
- **Must-cover test scenarios go under Acceptance criteria**, not a standalone "Testing" section — there is no separate Technical-context/Testing pair the way millhouse's discussion template had one.
- **No italic prose-coaching.**
  The rendered record is terse, structured prose for `Plan-Write` to act on — not a template with meta-commentary about how to fill it in.
- **"Notes for the plan writer" is a non-exhaustive head-start, never a completeness requirement.**
  `Plan-Write` explores the codebase itself, so a useful pointer, helper, or gotcha may go here, but nothing downstream depends on this subsection being present or complete.
  Do not write this subsection to be complete, and do not add one solely to avoid its absence — omitting it entirely is a valid, non-deficient record.
- **Do not enumerate call sites or cross-references here.**
  Incomplete call-site or cross-reference enumeration is not this stage's job — that belongs to the compiler and to `Plan-Sweep`'s mechanical inventory, not to discussion writing.

## `support-log.md` shape

Sections, in this order:

1. **Interview** — turn-by-turn, distilled, not a verbatim transcript.
2. **Rejected alternatives** — what was considered and not chosen, and why (the detail intentionally excluded from `decision-record.md`).
3. **Review rounds** — one entry per Discussion-review round: verdict, findings, how each finding was resolved.
4. **Question ledger** — the running list of open and resolved questions, including which picks `--auto` mode made.

**Review rounds is the anti-circling store.**
Its primary purpose: each new Discussion-review round reads it *before* raising findings, so successive reviewers do not re-raise a point an earlier round already settled.
This is the same shape of problem `discussion-on-disk-split`'s record↔log boundary solves at the file level, applied within the log itself — round N's context includes round N−1's resolutions.

Who *writes* the Review-rounds ledger — `Discussion-Write` itself,
or `Discussion-Review` — is a later milestone-12 implementation detail, not pinned by this doc.
What this doc pins is the contract: the ledger exists, its purpose is anti-circling,
and its shape is verdict + findings + resolution per round.

## Validation checks (spec for the future validator)

Per-run checks:

- Both files exist under `_lyx/discussion/` (`decision-record.md` and `support-log.md`).
- `decision-record.md` has all seven required sections present (Goal, Scope, Decisions, Constraints, Auto-mode assumptions, Open risks, Acceptance criteria);
  "Notes for the plan writer" is optional and its absence is not a violation.

This mechanical producer is **exhaustively defined by the checks listed above** — it has no judgment, and nothing beyond these two checks is "its" to look for.

**The `Plan-never-reads-support-log` boundary is not a per-run check.**
The boundary itself: `Plan-Write`'s declared input set never names `support-log.md`.
It is asserted once, at build/test time, over `Plan-Write`'s producer *definition* — never re-evaluated per run — because it is a property of the definition itself, and there is nothing per-run for a mechanical producer to evaluate about it.
This assertion lands with `Shed`.
Recorded here explicitly so nobody re-files it later as a missing per-run check.

## Discussion-Review rubric — what not to flag

This section is the text the future `perch` profile for `Discussion-Review` must **point at**, per the pointer rule — never copy or paraphrase into the profile itself.

`Discussion-Review` is the LLM producer, not the mechanical one — over-flagging is a judgment failure mode a mechanical producer (which has only checks, never judgment) cannot exhibit.
Do not flag any of the following as a finding:

- **A missing "Notes for the plan writer" subsection.**
  It is optional by contract; its absence is never a deficiency.
- **Missing rejected alternatives in `decision-record.md`.**
  Rejected alternatives belong in `support-log.md`'s Rejected alternatives section, not in `decision-record.md`;
  their absence from `decision-record.md` is by design, not an omission.
- **Incomplete call-site or cross-reference enumeration.**
  That enumeration belongs to the compiler and to `Plan-Sweep`'s mechanical inventory, not to `Discussion-Review`.

## Worked example

A minimal `decision-record.md` for a fictional task ("add a `--json` flag to `lyx board list`"):

```markdown
# Discussion: add --json to `lyx board list`

## Goal

Let scripts consume `lyx board list` output as JSON instead of parsing the table.

## Scope

In: a `--json` flag on `lyx board list`, one envelope per row.
Out: no other `board` subcommand gets the flag in this task.

## Decisions

### json-envelope-reuse

- **Decision:** `--json` marshals each row through the existing `internal/output.Ok`
  envelope.
- **Rationale:** one JSON emission path for the whole CLI; a second envelope shape
  would fork behavior for no gain.

## Constraints

Existing table output must be byte-identical when `--json` is not passed.

## Auto-mode assumptions

None — this task ran with a human present for every question.

## Open risks

None identified.

## Acceptance criteria

- `lyx board list --json` emits one `output.Ok` envelope per row.
- `lyx board list` (no flag) output is unchanged.
- Help text documents the new flag.

## Notes for the plan writer

`internal/boardengine/rows.go` already has the row struct; the JSON path can reuse it.
```

A minimal `support-log.md` for the same task:

```markdown
# Support log: add --json to `lyx board list`

## Interview

- Operator asked for JSON output on `board list` for scripting.
- Confirmed scope is `list` only, not every `board` subcommand.
- Confirmed reuse of `internal/output.Ok` rather than a bespoke envelope.

## Rejected alternatives

- A dedicated `ListJSON` envelope type — rejected: forks emission behavior for no
  gain over reusing `output.Ok`.

## Review rounds

### Round 1

- **Verdict:** approved.
- **Findings:** none.
- **Resolved:** n/a.

## Question ledger

- **Q:** Should `--json` also change exit codes on empty results? **A:** No — exit
  code behavior is unchanged; only the output format changes.
```
