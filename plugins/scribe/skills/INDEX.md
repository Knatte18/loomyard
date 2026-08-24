# Scribe Skills

| Skill | Description |
| --- | --- |
| [prose](prose/SKILL.md) | Precision and terseness rules for every piece of text an agent writes — chat replies, markdown files, code comments, docstrings. Always active. |
| [conversation](conversation/SKILL.md) | Interaction rules for chat replies — tone, user choices, file/shell conventions. Always active. Builds on `prose`. |
| [code-quality](code-quality/SKILL.md) | Strict, clean code guidelines, including comments and docstrings — naming, abstraction, error handling, file organization, and comment content. Use before editing code. |
| [testing](testing/SKILL.md) | Language-agnostic testing principles. Use when writing or reviewing tests. |
| [golang-comments](golang-comments/SKILL.md) | Godoc and inline comment mechanics for Go. Use when writing or reviewing Go comments. |
| [golang-build](golang-build/SKILL.md) | Build and test commands for Go. Use after completing a task. |
| [golang-testing](golang-testing/SKILL.md) | Testing conventions for Go projects. Use when writing tests. |
| [handoff](handoff/SKILL.md) | Write a handoff document so a fresh session can continue this conversation's work. Explicit invocation only (`/handoff`). |

`prose` and `conversation` are always-active through two distinct mechanisms, depending on context:

- **By default, for any session with this plugin installed:** `hooks/hooks.json` ships a `SessionStart` hook that injects an instruction to load `scribe:conversation` (which builds on `scribe:prose`) once, at the start of the session.
  This is a strong nudge, not a platform-enforced guarantee: the hook can only inject text asking the agent to load the skill, it cannot force-load it.
- **Inside a lyx-generated prompt** (a loom producer stencil, or any prompt lyx itself writes): the stencil carries an explicit "Load these skills: ..." line naming the relevant skills directly.
  This wiring now ships in `contracts/stencils/loom/loom-template-discussion.md`'s Step 0, which loads `scribe:prose` then `scribe:conversation`, and in `contracts/stencils/loom/loom-template-plan.md`'s Step 0, which loads `scribe:prose` then `scribe:testing`.
  The load is best-effort: nothing in the tree installs or verifies plugins, so a missing plugin degrades prose quality rather than breaking a run.
