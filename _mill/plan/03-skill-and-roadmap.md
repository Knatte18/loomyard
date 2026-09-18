# Batch: skill-and-roadmap

```yaml
task: "Launch ly-supervise and orchestrator via lyx reed add"
batch: "skill-and-roadmap"
number: 3
cards: 2
verify: go test ./internal/lyxcwd/ -run TestEnforcement_MarkdownLinks
depends-on: [1, 2]
```

## Batch Scope

This batch lands the operator-facing half: the `/ly:ly-supervise` skill stops telling the operator to open a side terminal and instead states that the session running it *is* the orchestrator strand, with a three-outcome self-check;
and the roadmap item moves from Planned to Done, with the Someday items that point at it re-worded.
It is one batch because both cards are prose over the same convention, and it depends on batches 1 and 2 because a roadmap item moves only once the thing it describes has shipped, and a skill telling an operator to relaunch through a chain that does not exist yet would be wrong.
It touches no Go source and delivers no interface a later batch consumes.

## Cards

### Card 8: the supervisor session is the orchestrator strand

- **Context:**
  - `internal/reedcli/status.go`
  - `internal/reedcli/attach.go`
  - `internal/reedengine/doc.go`
  - `internal/vscode/config.go`
- **Edits:**
  - `plugins/ly/skills/ly-supervise/SKILL.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Rewrite the `## Preconditions` section of `plugins/ly/skills/ly-supervise/SKILL.md`.
  Keep the existing cwd requirement and its reason unchanged — `lyx loom step` derives everything from cwd, and `lyxcwd.Resolve` requires that cwd to be a git worktree root.
  Replace the two sentences telling the operator to open `lyx reed attach` in a side terminal, which are now wrong: the operator is already inside the session, and a side terminal would be a second attach to it.
  State the launch convention instead: a worktree opened through `lyx ide spawn`'s generated VS Code task starts `lyx reed up`, `lyx reed add --if-absent --cmd claude --name claude --focus`, `lyx reed attach` in sequence, so the session running this skill is itself the strand named `claude`, and the panes loom spawns are its siblings in that same session.
  Add the self-check with exactly three outcomes, in this shape: read `$TMUX_PANE` from the environment, run `lyx reed status`, and compare that pane id against the tracked strands the envelope reports.
  With `$TMUX_PANE` set and the pane tracked, proceed silently.
  With `$TMUX_PANE` set and the pane absent from the tracked set, tell the operator this session runs in a pane reed does not track, name the launch chain, and offer the choice of relaunching that way or proceeding without reed supervision — as a numbered text list, per this skill's own `## Operator choices` section.
  With `$TMUX_PANE` unset, report the check as unconfirmed rather than failed: say the check could not run, that the session may or may not be a strand, and that proceeding is fine.
  Record why the third outcome exists, in one or two sentences: on Windows reed drives psmux, a tmux-compatible port, and nothing in this repo verifies that psmux exports `TMUX_PANE` into a pane's environment, so a two-outcome check would tell every correctly-launched Windows operator to relaunch.
  State the relaunch advice only in the tracked-absent branch.
  Add a short note that a worktree created before this convention keeps its existing `tasks.json`, because the generator never clobbers, and that the manual upgrade is to delete `.vscode/tasks.json` and re-run `lyx ide spawn`.
  Follow the repo's semantic-line-break rule for every line written here.
- **Commit:** `docs(ly-supervise): make the orchestrator strand the skill's precondition`

### Card 9: move the roadmap item to Done

- **Context:**
  - `docs/overview.md`
  - `plugins/ly/skills/ly-supervise/SKILL.md`
- **Edits:**
  - `manifest/roadmap.md`
  - `manifest/designs/reed-header-selvage.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `manifest/roadmap.md`, move the **ly-supervise + orchestrator: launch via `lyx reed add`, not ad hoc** item out of `## Planned` and into `## Done`, written as `1.` like every other entry — the file's own Maintenance section records that numbering is automatic and that no renumbering is needed anywhere.
  Rewrite the moved entry in the past tense and keep it to the couple of sentences the Maintenance section allows: the generated VS Code `folderOpen` task is now the reed launch chain with both binary paths stamped absolute, `lyx reed add` gained `--if-absent` so a reopen is idempotent, and the `/ly:ly-supervise` skill now treats the session running it as the orchestrator strand.
  Update the three Someday entries that point at this item by its old status, so none of them still calls it Planned: **reed: born-as-strand for the operator's `loom run` attach**, whose opening clause names it as the Planned item that covers its two siblings;
  **reed: strand-based mailbox/addressing system**, which names it in its dependency sentence;
  and **loom CLI: rename `run`/`drive`/`step` for verb/engine symmetry, plus rename `ly-supervise`**, if it refers to the item's status rather than only to the skill's name.
  Refer to it by bold item name in each, per the Maintenance section's cross-reference rule.
  In `manifest/designs/reed-header-selvage.md`, check the Selvage section's `lyx reed add` example for consistency with the shipped flag surface and adjust only if it now reads as wrong;
  it cites `lyx reed add` as the thing you would type in the control terminal to spawn a new Claude, which the new flag does not invalidate, so leaving that sentence as it stands is the expected outcome.
  Build nothing on Selvage, which remains a separate Planned item.
  Keep every inline markdown link in both files resolvable, file part and anchor alike, per CONSTRAINTS.md's Markdown Link Integrity invariant.
- **Commit:** `docs(roadmap): move the ly-supervise launch-convention item to Done`

## Batch Tests

`verify: go test ./internal/lyxcwd/ -run TestEnforcement_MarkdownLinks` runs the repo's markdown link-and-anchor gate, which walks every `.md` file under `manifest/` and `docs/` and fails on an inline link whose file part or `#anchor` does not resolve.
That is the one mechanical gate this batch's edits can break: card 9 moves an entry carrying links between sections and re-words three cross-references, and the design-doc check may adjust a sentence beside a link.
The gate lives in `internal/lyxcwd/docslink_test.go` as a file-layout convenience, reusing that package's enforcement walkers, and running it by name scopes the batch to it rather than to the whole `lyxcwd` suite.
The skill file in `plugins/ly/skills/` is outside the gate's roots and has no mechanical test in this repo;
its precondition is operator-facing, exercised by opening a worktree and reading `lyx reed status`, exactly as the discussion's Testing section records.
