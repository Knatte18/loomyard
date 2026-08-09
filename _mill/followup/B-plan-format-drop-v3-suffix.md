```yaml
slug: plan-format-drop-v3-suffix
title: "plan-format: drop the v3 suffix and sweep every reference by script"
depends_on: ["builder-retire"]
brief: |
  Rename docs/reference/plan-format-v3.md to docs/reference/plan-format.md and sweep every reference to the old name — docs and Go identifiers alike — mechanically, by a scripted find/replace, with completion judged by a zero-hit case-insensitive repo grep across the full pattern set rather than by any file count written down beforehand.
```

# plan-format: drop the v3 suffix and sweep every reference by script

## Why

`v3` is the only live format once builder is gone,
and a version suffix on the sole format is exactly the kind of stale guard `discussion-format.md` already argues against, via its `no-schema-version` reference to `status-schema.md`.

A half-done rename is worse than either end state, because `planparser` and `websterengine` identifiers and template prose must move with the filename.

**Rejected alternatives:**

- A docs-only rename with Go identifiers deferred — leaves the codebase mid-rename.
- Renaming the file but keeping in-text "v3" as a historical label — the suffix is exactly what is being retired.

## What needs to happen

1. This task's first action re-derives the affected file list by grep rather than trusting any count written down beforehand.
   Do not trust a file count written anywhere else — this step is what bounds the list.

2. The affected clusters, as a starting inventory only, are:
   - `internal/planparser`
   - `internal/websterengine`
   - `internal/webstercli`
   - `internal/loomengine`, including `plan-template.md`
   - `internal/batcher/doc.go`
   - `docs/overview.md`
   - `docs/reference/model-spec.md`
   - `docs/reference/builder-contract.md`
   - `manifest/roadmap.md`
   - several `manifest/designs/*.md` files
   - `tools/sandbox/SANDBOX-WEBSTER-SUITE.md`
   - `CONSTRAINTS.md`'s Planparser Sole-Parser Invariant, whose wording changes for the renamed format.

### Hard exclusion — `gopkg.in/yaml.v3`

The sweep must never touch that import token.
It appears in ten Go files, including `internal/planparser/parse.go:21` — the sole plan parser, i.e. the file this task is most certain to be editing.
A broad `v3` replace corrupts the import and breaks the build in the least obvious place.
This task's script names the exclusion explicitly rather than relying on the pattern set being narrow enough.

### Execution discipline

A scripted find/replace followed by a full `go test ./...`, never a hand-edit pass.
Per this repo's own tooling rules the script must not use `sed`.

3. Record the deliberate window between task A and this task where `docs/reference/plan-format.md` does not exist at all: task A deletes v2 to free the name, this task re-creates it from v3.
   Links to `plan-format.md` dangle in between, by design and briefly.

4. Record what this task deliberately leaves broken: this task's rewrite of `loom.md:29` knowingly leaves that sentence self-contradicting, because a pure find/replace cannot repair an argument about two formats when only one survives.
   This is accepted, not overlooked — this task's grep criterion passes while the sentence reads wrong,
   and task E repairs the prose as `loom.md`'s final owner.

## Scope

This task's position in `loom.md`'s three-owner chain is B → C → E: this task is the mechanical owner, because its zero-hit criterion necessarily rewrites `loom.md:29` and table rows 5–7 at `:53–55`, which spell `plan-format-v3.md`.

This task changes paths and names only, never prose.
Because this task runs before both C and E, this is chain order rather than concurrency — no two owners hold the file at once.

## Sequencing

`depends_on: builder-retire` — the filename is not free until v2's doc is deleted.

Two tasks depend on this task: task C and task F, because both edit the renamed file.

## Acceptance

The completion criterion is the discussion's own case-insensitive repo grep returning zero hits for the full pattern set, plus a passing `go test ./...`.
The pattern set is: `plan-format-v3`, `plan_format_v3`, `plan-format v3`, `plan-v3`, and `Plan-format v3`.

The narrower three-pattern set was rejected because it would leave `loom.md:58`'s "plan-v3's card contract", `loom.md:94`'s "Webster/plan-v3 equivalent", and `internal/planparser/doc.go:32`'s "Plan-format v3" all passing, contradicting the decision's own intent.

`internal/planparser`'s existing tests and `internal/webstercli/cli_test.go` cover behaviour preservation.
The meaningful failure mode is incompleteness, checked by grep rather than by an assertion in a test file.
