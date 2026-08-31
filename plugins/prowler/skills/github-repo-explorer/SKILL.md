---
name: github-repo-explorer
description: Browse a GitHub repo's file tree and read files via the gh CLI, without cloning
argument-hint: "<owner/repo> [path] [question]"
---

This skill browses a public or accessible-to-you GitHub repository's file tree and reads individual files through the `gh` CLI, without ever cloning the repository.

**Hard prerequisite:** `gh` must be installed and authenticated (`gh auth status`).
There is no fallback path — if `gh` is missing or unauthenticated, report that and stop.

**Resolve both scripts' absolute paths now, while `${CLAUDE_SKILL_DIR}` is still set** — a dispatched subagent won't have it:

1. `TREE_SH="$(cd "${CLAUDE_SKILL_DIR}/../../scripts" && pwd)/github-tree.sh"`
2. `READ_SH="$(cd "${CLAUDE_SKILL_DIR}/../../scripts" && pwd)/github-read.sh"`

**List the repository's file tree with one script call, instead of composing the `gh api` walk by hand:**

Run `github-tree.sh` in whichever mode fits what the caller already knows:

- `paths=$(bash "$TREE_SH" <owner/repo>)` for a whole-repository listing, when the caller does not yet know which subtree it needs.
- `paths=$(bash "$TREE_SH" <owner/repo> <path>)` for a scoped recursive listing, when the caller already knows which subtree it needs.
  The script handles GitHub's recursive-tree truncation cap internally, so the listing it returns is never silently partial.
- `paths=$(bash "$TREE_SH" --children <owner/repo> [path])` for the children mode, listing one directory's direct entries without recursing — the right choice for exploring one directory at a time, top-down.
  In this mode a directory entry carries a single trailing slash and a file entry carries no marker.

A listing in any mode can abort on an entry-count ceiling that defaults to 1000 entries: the abort is an ordinary failure — one stderr line and a non-zero exit, exactly like every other failure below — and the caller's options are to scope the listing to a subdirectory, switch to the children mode, or raise the ceiling with `--max-entries N`.
This makes a full dump of a very large repository a deliberate, visible act rather than an accident.

**Read a file's content with one script call:** `content=$(bash "$READ_SH" <owner/repo> <path>)`.
It takes an owner-and-repository reference plus one repo-relative path, reads exactly one file per call, and writes that file's content verbatim to stdout with no banner or delimiter around it.
Reads are pinned to the default branch — there is no ref argument, and no way to read anything but the default branch's current content.

**Check the exit code, always:** on failure either script prints exactly one line to stderr, emits nothing on stdout, and exits non-zero — an empty result on success (a genuinely empty repository, or an empty file) and an empty result on failure look identical unless the exit code is checked, so the exit code must be checked and any failure reported rather than read as an empty repository or an empty file.
This applies equally to an entry-count guard abort: it is still a one-stderr-line, non-zero-exit failure, not a distinct outcome to special-case.

**Deciding whether the skill's second argument is a `path` or the `question`:** this skill's own argument hint is `<owner/repo> [path] [question]`, and a second token is forwarded to a script as `<path>` only when it looks like a repo-relative directory path — matching the accepted set `^[A-Za-z0-9._/-]+$` and containing no whitespace.
Anything else, a natural-language question in particular, is the question, and is never passed to either script.

**Before reading many files, load the `distill-subagent` skill by name** (`prowler:distill-subagent`) via the Skill tool and apply its rule, so a broad repo browse does not bloat the caller's context.
