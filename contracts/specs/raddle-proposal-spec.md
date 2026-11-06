# Raddle proposal-spec — how a Sketch splits into filed entries

> **Status: Contract — pinned.** This doc pins **proposal-format**: the proposal the raddle agent writes at `_lym/raddle/proposal.yaml`, which the `raddle` gate, `lym loom raddle-propose`, `lym loom raddle-approve` and Raddle-File check and `internal/raddleparser` parses. It is `internal/raddleparser`'s own contract, kept as a durable Go-to-Go reference doc under `contracts/specs/`. The LLM-facing rules for the agent's work are pinned in the raddle stencil, which points here for the format and restates none of it.

## Producer and contract

The raddle agent of Raddle-Write writes the proposal.
It is checked by the gate of the `raddle` recipe, by `lym loom raddle-propose` and `lym loom raddle-approve`, and again by Raddle-File before it files; each check runs over `internal/raddleparser`, the proposal's sole parser.
The parser imports the standard library and `internal/discussionparser` only, and reads the Sketch's decision record through `internal/discussionparser`.

## File form

The file is one object in YAML's JSON-compatible flow form, so a JSON object is a valid proposal.
The reason: the sole parser imports the standard library and `internal/discussionparser` only, and neither holds a YAML decoder, while a JSON object is a valid YAML file.
The parser refuses an unknown key at any level and a file that is not one object, each with an error naming the key or the line and the way forward.

## Keys

The proposal holds these keys:

- `sketch` — the Sketch's slug.
- `shape` — one of `tapestry`, `tasks`, `project`, `beam`.
- `project` — `{slug, title, type}`, present exactly when `shape` is `project` or `beam`, and absent otherwise.
  `type` is required: one of the board's type labels other than `sketch`, the agent's pick of the kind of work, which the operator approves with the rest of the proposal.
- `board_checked` — a list of `{slug, revised}`, one per board entry the agent weighed.
  `revised` is the entry's `revised` stamp in RFC 3339, as `lym board list` prints it.
- `parts` — a list of parts.
- `cost` — the proposal's cost.

A part holds:

- `slug` and `title`.
- `type` — a type label of the board's vocabulary, never `sketch`.
- `recipe` — `tapestry` or `darn`.
- `profile` — a profile of `profiles.yaml` that holds a block for that recipe.
- `decisions` — the Sketch decision ids the part builds, such as `D3`.
- `packages` — a list of `{path, size}`, `size` the rough number of changed lines.
- `contract` — `{takes, delivers, exports}`, each a text.
- `depends_on` — part slugs or board slugs.
- `open_risks` and `criteria` — the Sketch's Open risks and Acceptance criteria bullets the part carries, each the bullet's whole text as the decision record holds it.
- `pattern_changes` — the PATTERN entry ids the part carries, as the Sketch's record names them.
- `pattern_clusters` — optional, a list of `{sketch_id, entry, text}`, one per carried change a live `PATTERN.md` entry absorbs: `sketch_id` is the carried id, `entry` the live entry's id and `text` its whole new text.
  Raddle-File writes every record it files with that change as the bullet `` - `<entry>` — changes: <text> ``, whatever the Sketch's action was, so the Sketch's id never reaches the plan gate.
  The Sketch's own record keeps its bullets.
- `expected_cost_usd` — a number, or null when a cell the figure rests on reads "too few runs".
- `cost_line` — one line saying what the figure rests on.

`cost` holds:

- `expected_usd` — a number, or null when a cell the figure rests on reads "too few runs".
- `line` — one line saying what the figure rests on.
- `next_best` — `{split, expected_usd, line}`: the next-best split in one line, its expected cost and the line that says what that figure rests on.
- `cells` — the `lym report cost` cells both figures rest on, each `{recipe, profile, stored_record}`.

Shape `tapestry` has exactly one part, which builds every decision.
Every filed slug, each part's and the project's, is new: free on the board and in the raddle store, and not the Sketch's.

## Checks

The parser and the checks name each finding by its check name, with the part, id, slug or entry it concerns.

The gate, `raddle-propose` and `raddle-approve` run the checks of the proposal alone, and Raddle-File runs them before it files:

- `decision-unbuilt` — a decision id of the record that no part builds.
- `decision-unknown` — a decision id a part builds that the record does not hold.
- `criterion-uncarried`, `risk-uncarried` — an Acceptance criterion or Open risk bullet of the record that no part carries.
- `criterion-unknown`, `risk-unknown` — a bullet a part carries that the record does not hold.
- `pattern-uncarried`, `pattern-shared` — a PATTERN change of the record that no part carries, or that two parts carry.
- `pattern-unknown` — a PATTERN id a part carries that the record's PATTERN changes do not hold.
- `pattern-cluster-invalid` — a `pattern_clusters` mapping of an id its part does not carry, a second mapping of one id, or a `text` that is empty, spans lines or runs past the PATTERN rule-text cap.
- `pattern-cluster-unknown` — a mapping whose `entry` the worktree's `PATTERN.md` does not hold.
- `pattern-cluster-shared` — a mapping whose `entry` another mapping or another declared change claims.
- `criteria-miss-decision` — a part whose carried criteria do not name every decision id it builds, so its record would fail the discussion check.
- `carried-decision-unbuilt` — a carried criterion or risk that names a decision id neither the part nor a part on its `depends_on` path builds.
- `board-unchecked` — an open task or project, other than the run's own entry, that carries a module label the Sketch carries and is absent from `board_checked`.
- `depends-cycle` — a cycle in `depends_on`.
- `depends-unknown` — a `depends_on` that names no part, no board entry and no `board_checked` entry; a `board_checked` entry since gone from the board passes as landed.
- `packages-shared` — two parts with no `depends_on` path between them that list a common package.
- `recipe-invalid` — a recipe that is neither `tapestry` nor `darn`.
- `profile-unknown` — a profile that `profiles.yaml` does not hold, or holds without a block for the part's recipe.
- `type-invalid` — a part's or the project's type that is not in the board's vocabulary, or is `sketch`.
- `slug-taken` — a new slug already on the board or in the raddle store; the run's own entry is exempt, which `slug-sketch` judges.
- `slug-sketch` — a filed slug equal to the Sketch's.
- `slug-duplicate` — a slug that two filed entries share.
- `shape-invalid` — a `tapestry` shape with other than one part or with a part that does not build every decision, a `project` or `beam` shape with no project, or another shape with one.
- `cost-missing` — a missing proposal cost line or next-best cost line.

The re-check against the board runs at `raddle-approve` and in Raddle-File, which runs it before its first step and again inside its board write.
It passes only when it names none of these failures:

- `revised-moved` — a `board_checked` entry still open whose `revised` stamp differs from the proposal's.
- `board-unchecked` — as above, whenever the entry was created or promoted.
- `slug-taken` — as above.

A `board_checked` entry now done or gone from the board counts as landed: the re-check names it and does not fail on it, and Raddle-File drops a `depends_on` on one gone from the board.

A Raddle-File retry whose board write landed but whose raddle store write did not finds every filed entry on the board and the stage holding the proposal's hash.
It skips the checks, the approval match and the re-check's failures, which would count the run's own filed slugs as taken, reads only the entries the re-check names as landed, and writes the store.
A proposal edited since is not skipped: the stage then holds another hash, and every check runs again.

## Approval hash

The approval hash is the SHA-256 of the proposal file's bytes, in hex.
`raddle-approve` records it, and in interactive mode Raddle-File files only a proposal whose hash matches the recorded approval.

## Example

```json
{
  "sketch": "usage-records",
  "shape": "tasks",
  "board_checked": [{"slug": "report-cost", "revised": "2026-11-04T09:30:00Z"}],
  "parts": [
    {
      "slug": "usage-writer",
      "title": "Usage record writer",
      "type": "feature",
      "recipe": "tapestry",
      "profile": "standard",
      "decisions": ["D1", "D2"],
      "packages": [{"path": "internal/shedrun", "size": 120}],
      "contract": {"takes": "a session's usage reading", "delivers": "one record file per session", "exports": "WriteSessionUsage"},
      "depends_on": [],
      "open_risks": [],
      "criteria": ["D1 and D2: every session leaves one record file"],
      "pattern_changes": [],
      "expected_cost_usd": null,
      "cost_line": "the tapestry cell reads too few runs"
    }
  ],
  "cost": {
    "expected_usd": null,
    "line": "one tapestry part; its cell reads too few runs",
    "next_best": {"split": "two parts", "expected_usd": null, "line": "two tapestry parts; the same cell reads too few runs"},
    "cells": [{"recipe": "tapestry", "profile": "standard", "stored_record": true}]
  }
}
```
