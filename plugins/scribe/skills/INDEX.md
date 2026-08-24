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

`prose` and `conversation` are marked always-active by convention: any skill in this plugin that produces user-facing text should load `prose` first (and `conversation` too, if the text is a chat reply), the same way `mill-start` loads `mill:conversation` unconditionally as its own first step. Which skills in a future orchestration layer count as that entry point is not yet decided — see `discussion.md` for this task.
