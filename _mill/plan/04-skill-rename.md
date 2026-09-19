# Batch: skill-rename

```yaml
task: "Rename loom CLI run/drive/step for verb/engine symmetry, plus rename ly-supervise"
batch: "skill-rename"
number: 4
cards: 1
verify: null
depends-on: [3]
```

## Rename mechanic

For each `Moves:` pair the implementer MUST:

1. Run `git mv <old> <new>` FIRST, before making any other change to the moved file.
2. Make ONLY surgical edits — touch only the lines that must change after the move (package or module declaration, imports, identifier retargeting, seam splits).
3. Use a full-file `Creates:` entry only for genuinely new files that have no predecessor.
4. Never write the relocated file from scratch and delete the original — that breaks git rename history and inflates review diffs.

The directory rename is carried by the file move: `git mv plugins/ly/skills/ly-supervise/SKILL.md plugins/ly/skills/ly-drive/SKILL.md` relocates the only file the directory contains, and git removes the now-empty `ly-supervise/` directory on its own.
Do not create `plugins/ly/skills/ly-drive/` by hand first and copy the file into it.

## Batch Scope

This batch renames the `/ly:ly-supervise` skill to `/ly:ly-drive`: the directory, the frontmatter, the body's self-references, the step-envelope scratch path, and the plugin skill index.
It is one batch and one card because the rename is a single indivisible unit — the frontmatter `name` and the directory name must agree for the skill to resolve at all, and `INDEX.md`'s link would dangle if either moved without it.

It depends on batch 3 because `internal/shedadapters/bouncer_seed_test.go` already names `ly-drive` in a comment by then, and the skill should exist under that name before anything else in the tree refers to it.

Batch-local decision: the plugin manifest `plugins/ly/.claude-plugin/plugin.json` is deliberately not edited.
It names no individual skill, so it needs no change, and per this repo's convention its `version` stays at `1.0.0` regardless — unpublished plugins do not take a version bump for a feature change.

## Cards

### Card 13: rename ly-supervise to ly-drive

- **Context:**
  - `plugins/ly/.claude-plugin/plugin.json`
  - `internal/loomcli/start.go`
- **Edits:**
  - `plugins/ly/skills/INDEX.md`
- **Creates:** none
- **Deletes:** none
- **Moves:**
  - `plugins/ly/skills/ly-supervise/SKILL.md` -> `plugins/ly/skills/ly-drive/SKILL.md`
- **Requirements:** Perform the `git mv` first, per the `## Rename mechanic` section above, then make surgical edits to the relocated file.
  In the relocated SKILL.md at the move pair's destination, change the frontmatter `name:` from `ly-supervise` to `ly-drive` and rewrite the frontmatter `description:` so it describes driving a loom task through a loop over `lyx loom step` rather than supervising one, keeping its "Explicit invocation only." sentence and the `disable-model-invocation: true` key unchanged.
  Change the per-step envelope scratch path from `.scratch/ly-supervise/step-<n>.json` to `.scratch/ly-drive/step-<n>.json`, leaving the surrounding explanation that `.scratch/` is the mandated scratch location and is already gitignored repo-wide exactly as it is.
  Rewrite every place the body refers to the skill by its own name so it reads `ly-drive`.
  Then sweep the whole relocated file rather than only the sites named here, classifying every hit of a loom verb name, the skill's own name, or a pre-move filename before touching it.
  Three landmarks: the pre-loop-baseline section names `lyx loom run` as the bootstrap remedy, which becomes `lyx loom start`;
  and the `## Self-report` section names the foreground verb twice — once saying loom's two automatic self-report tiers both hang off `lyx loom drive`'s own run, and once saying `step` never spawns the reflection pass that `drive` runs — both of which become the `run` form.
  That second pair matters beyond consistency: the sentence's whole point is that the tiers fire on one verb and not on `step`, so naming the wrong verb inverts what the skill tells the operator about its own reporting duty.
  The body's `lyx loom step` invocations are unchanged throughout — `step` keeps its name — as are the uses of "drives"/"driver" as ordinary English, such as reed driving psmux and the operator having a driver running.
  In `plugins/ly/skills/INDEX.md`, update the table row so both the link text and the link target name `ly-drive` and point at `ly-drive/SKILL.md`, rewrite the row's description to match the new frontmatter `description:`, and change the explicit-invocation note on the following line so it names `ly-drive`.
  Leave `plugins/ly/.claude-plugin/plugin.json` untouched, including its `version` field.
- **Commit:** `refactor(ly): rename the ly-supervise skill to ly-drive`

## Batch Tests

`verify:` is `null`: this batch is a markdown-and-directory rename with no runnable surface.
No Go test imports, parses, or asserts on `plugins/ly/skills/`, and the plugin manifest — the only machine-read file in that tree — is deliberately not edited.

The batch is not unverified, though.
`INDEX.md`'s link to `ly-drive/SKILL.md` is a relative markdown link, but the Markdown Link Integrity invariant's test (`internal/lyxcwd/docslink_test.go`) walks `manifest/` and `docs/` only, so it does not reach `plugins/`;
the real check on this batch is batch 5's own sweep, which retargets every `manifest/` and `docs/` reference to the skill's old path and is covered by that test.
A mismatch between this batch's new path and batch 5's references therefore surfaces at batch 5's verify rather than here.
