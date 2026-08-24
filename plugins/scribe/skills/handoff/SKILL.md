---
name: handoff
description: Write a handoff document so a fresh session can continue this conversation's work. Explicit invocation only.
argument-hint: "[path/to/file.md] [what will the next session focus on?]"
disable-model-invocation: true
---

Write a handoff document summarising the current conversation so a fresh agent can continue the work. Save to `.scratch/handoff.md` under the current working directory by default — never the OS temp directory (see `conversation`'s file-writing rule).

If the arguments include a file path, save there instead (resolved relative to the current working directory unless absolute).

The document's first line instructs the next agent to load `scribe:conversation` (which in turn loads `scribe:prose`) before reading the rest of the document.

Include a "suggested skills" section in the document, naming which skills the next agent should call the Skill tool for.

Do not duplicate content already captured in other artifacts (specs, plans, ADRs, issues, commits, diffs). Reference them by path or URL instead.

Redact any sensitive information, such as API keys, passwords, or personally identifiable information.

If the arguments include anything other than a path, treat that part as a description of what the next session will focus on and tailor the doc accordingly.
