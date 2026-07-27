# Plan: Crucible review spawn as effort-selectable Agent profiles

```yaml
task: Crucible review spawn as effort-selectable Agent profiles
slug: crucible-effort-agent-profiles
approved: false
started: 20260727-121932
parent: main
root: ""
verify: null
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches. Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: effort-profiles-and-crucible-rewiring
    file: 01-effort-profiles-and-crucible-rewiring.md
    depends-on: []
    verify: null
```

## Shared Decisions

_Cross-cutting decisions every batch inherits: naming conventions, error-handling posture, test frameworks, style/lint constraints. One subsection per decision. Batch-local decisions live in each batch file._

### Decision: five effort tiers, one file each, no fallback file

- **Decision:** Ship exactly five agent-definition files — `.claude/agents/crucible-reviewer-low.md`, `-medium.md`, `-high.md`, `-xhigh.md`, `-max.md`. No bare unsuffixed `crucible-reviewer.md`.
- **Rationale:** The crucible use case explicitly wants both ends of the spectrum (a cheap low-effort wide sweep early, a max-effort correctness pass for the final safety round). An unsuffixed "inherit" file would be a silent default that undermines the per-round explicit-choice discipline this task exists to create.
- **Applies to:** all batches

### Decision: frontmatter carries exactly `name`, `description`, `effort`

- **Decision:** No `model:` key and no `tools:` key in any of the five files.
- **Rationale:** Omitting `model:` keeps effort (selected via `subagent_type`) and model (the orchestrator's per-call `model:` override) fully orthogonal — no N-effort × M-model file explosion. Omitting `tools:` inherits the full default tool set, which is what `general-purpose` rounds get today; enumerating a narrower list (the original proposal copied millhouse's 7-tool `mill-implementer` list) would silently drop `BashOutput`/`KillShell` — needed for live-substrate driving and the "zero stray processes" teardown discipline — plus `TodoWrite`/`WebFetch`/`WebSearch`. These profiles change exactly one thing about the spawn: the effort tier.
- **Applies to:** all batches

### Decision: the five bodies stay byte-identical except three tokens

- **Decision:** All five files are byte-identical apart from `name:`, the effort word inside `description:`, `effort:`, and the H1 heading. There is no automated drift guard, so this is a manual discipline point recorded in the batch file for whoever edits these next.
- **Rationale:** The bodies restate `crucible/review-prompt-template.md`'s commit-per-fix / sequencing / clean-room contract. If that contract wording changes later, all five must be updated in the same commit or the tiers silently diverge in what they promise a round agent.
- **Applies to:** all batches

### Decision: the live smoke check is operator-run, post-commit — not part of task completion

- **Decision:** The implementing session creates and commits the five files and the doc edits, then states plainly in its handoff that the live smoke check is pending and names the exact five `subagent_type` values to check. It does not run the check itself.
- **Rationale:** It is unknown whether an already-running Claude Code session picks up newly written agent definitions or requires a restart, so the check needs a session started after the files exist on disk — which an implementing session cannot satisfy by restarting itself. Task completion is therefore not blocked on the check. If a tier FAILs later, dropping it is a small follow-up commit that deletes the file and removes it from the `orchestrator-prompt.md` tier enumeration together.
- **Applies to:** all batches

### Decision: markdown is one line per paragraph, never hard-wrapped

- **Decision:** Every new and edited `.md` file in this task is written with one continuous line per paragraph and per list item. No fixed-column wrapping.
- **Rationale:** Root `CLAUDE.md` requires it repo-wide. It also keeps this task's diff from colliding with the separate `markdown-unwrap` task, which reflows whitespace in these same two crucible files; writing unwrapped from the start means no reflow is ever needed here.
- **Applies to:** all batches

### Decision: no plugin registration, no Go changes, no test harness

- **Decision:** This task touches only `crucible/*.md` and `.claude/agents/*.md`. No `.claude-plugin/plugin.json` entry, no `internal/*` Go code, no `CONSTRAINTS.md` or module-doc update, no automated agent-definition test.
- **Rationale:** loomyard is a plain repo, not a Claude Code plugin — it has no `.claude-plugin/` directory, so millhouse's `plugin.json` `agents` array registration step has no counterpart here; project-level `.claude/agents/*.md` files are auto-discovered from a worktree's own checkout. The change ships no behavior in Go and introduces no cross-cutting invariant, so the Documentation Lifecycle rule adds nothing. loomyard has no Python or plugin test harness for agent-definition files.
- **Applies to:** all batches

## All Files Touched

_Full union of every `Creates:` / `Edits:` / `Moves:` **target** path across every batch, sorted alphabetically (Move **source** paths are excluded — they disappear, like `Deletes:` tokens). mill-go reads this to warn if two parallel batches touch the same file — a sign of a misplaced dependency._

- `.claude/agents/crucible-reviewer-high.md`
- `.claude/agents/crucible-reviewer-low.md`
- `.claude/agents/crucible-reviewer-max.md`
- `.claude/agents/crucible-reviewer-medium.md`
- `.claude/agents/crucible-reviewer-xhigh.md`
- `crucible/README.md`
- `crucible/orchestrator-prompt.md`
- `crucible/review-prompt-template.md`
