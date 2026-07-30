# Batch: skills-and-docs

```yaml
task: 'prowler: site-adapter mechanism + github-repo-explorer skill (Claude reading the web)'
batch: skills-and-docs
number: 2
cards: 6
verify: null
depends-on: [1]
```

## Batch Scope

This batch delivers the two new codeless skills — `distill-subagent` (the extracted, model-agnostic cheap-subagent judgment rule) and `github-repo-explorer` (a `gh`-CLI wrapper letting Claude browse a repo tree and read files without cloning) — refactors the existing `prowler` skill to load `distill-subagent` instead of inlining the rule, and lands the accompanying doc/registration updates (`INDEX.md`, `README.md`, and the version bump in both `plugin.json` and the repo-root `marketplace.json`). It depends on batch 1 so the README's site-adapter description documents the shipped mechanism. Every card is Markdown/JSON, one-line-per-paragraph prose (repo rule); there is no runnable surface, so `verify: null`.

## Cards

### Card 9: Create the distill-subagent guidance skill

- **Context:**
  - `plugins/prowler/skills/prowler/SKILL.md`
- **Edits:** none
- **Creates:**
  - `plugins/prowler/skills/distill-subagent/SKILL.md`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `plugins/prowler/skills/distill-subagent/SKILL.md` with YAML frontmatter (`name: distill-subagent`; `description:` marking it a helper, e.g. "Judgment rule for wrapping expensive web/repo reads in a cheap distillation subagent — used by the prowler and github-repo-explorer skills"). The body holds ONLY the provider/command-agnostic judgment rule and subagent contract, no fetch/read command specifics: (1) when NOT to wrap — an already-small isolated worker (e.g. a dedicated research subagent) reads the content inline itself; (2) when to wrap — a general-purpose or long-lived thread, or any expensive tier, dispatches a cheap-tier subagent (Agent tool, `model: haiku` — worded as "the cheap distillation tier, currently Haiku") to do the read and return only a distilled answer; (3) batch all related sources for one batch of questions into a single subagent dispatch (dispatch overhead is paid once), and split into multiple subagents only when sources need independent unmixed answers — in that case dispatch them in parallel (one message), never a sequential loop; (4) the subagent returns ONLY its distilled answer, never raw fetched content, and the caller never dumps raw content to the user or carries it into its own context. Keep it terse (~15-20 lines of body). One line per paragraph/bullet.
- **Commit:** `feat(prowler): add distill-subagent guidance skill`

### Card 10: Create the github-repo-explorer skill

- **Context:**
  - `plugins/prowler/skills/prowler/SKILL.md`
  - `plugins/prowler/skills/distill-subagent/SKILL.md`
- **Edits:** none
- **Creates:**
  - `plugins/prowler/skills/github-repo-explorer/SKILL.md`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `plugins/prowler/skills/github-repo-explorer/SKILL.md` with YAML frontmatter (`name: github-repo-explorer`; `description:` e.g. "Browse a GitHub repo's file tree and read files via the gh CLI, without cloning"; an `argument-hint` such as `"<owner/repo> [path] [question]"`). Body documents, as concrete `gh` command recipes (one line per paragraph): a hard prerequisite that `gh` is installed and authenticated (no fallback); resolve the default branch — `gh api repos/{owner}/{repo} --jq .default_branch`; list the full recursive tree in one call — `gh api "repos/{owner}/{repo}/git/trees/{branch}?recursive=1" --jq '.tree[].path'`, noting that this response carries a `"truncated": true` field for very large repos (GitHub's API caps the recursive listing) — the skill must instruct checking `.truncated` (e.g. `gh api ... --jq .truncated`) and, when true, falling back to non-recursive per-directory tree calls (`gh api "repos/{owner}/{repo}/git/trees/{branch}"` then descending into subtree SHAs) so a large-repo browse is never silently partial; read a file — `gh api "repos/{owner}/{repo}/contents/{path}" --jq .content | base64 -d`, noting `https://raw.githubusercontent.com/{owner}/{repo}/{branch}/{path}` as a lighter alternative for public files; and an instruction to load the `distill-subagent` skill by name (`prowler:distill-subagent`) via the Skill tool and apply its rule before reading many files, so a broad repo browse does not bloat the caller's context. Do NOT reference lyx's internal githubclient Go package (a separate lyx-internal module, unavailable to a codeless skill). One line per paragraph.
- **Commit:** `feat(prowler): add github-repo-explorer skill`

### Card 11: Refactor the prowler skill to load distill-subagent

- **Context:**
  - `plugins/prowler/skills/distill-subagent/SKILL.md`
- **Edits:**
  - `plugins/prowler/skills/prowler/SKILL.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `plugins/prowler/skills/prowler/SKILL.md`, replace the inlined Haiku-wrapper judgment paragraphs (the "Raw output is often long and noisy…" decision block and the bulleted small-worker-vs-wrap rule, plus step 3's "If wrapping: dispatch one Haiku-tier subagent…") with an instruction to load the `distill-subagent` skill by name (`prowler:distill-subagent`) via the Skill tool and apply its rule to decide whether to read inline or wrap the fetch. Keep the prowler-specific mechanics (steps 1-2's `RUN_SH` resolution and the `run.sh <url>` invocation, and step 4's relay/never-dump-raw rule). Where the concrete dispatch is still described, soften "Haiku subagent" / `model: haiku` wording to "a cheap-tier subagent (currently Haiku), `model: haiku`" so the model is named as the default tier, not the identity of the pattern. Do not change the frontmatter `name`/`description`/`argument-hint`. One line per paragraph.
- **Commit:** `refactor(prowler): load distill-subagent from the prowler skill`

### Card 12: Register both skills in INDEX.md

- **Context:**
  - `plugins/prowler/skills/distill-subagent/SKILL.md`
  - `plugins/prowler/skills/github-repo-explorer/SKILL.md`
- **Edits:**
  - `plugins/prowler/skills/INDEX.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `plugins/prowler/skills/INDEX.md`, add two rows to the skills table: `github-repo-explorer` → `[github-repo-explorer](github-repo-explorer/SKILL.md)` with its description, and `distill-subagent` → `[distill-subagent](distill-subagent/SKILL.md)` with its helper description. Match the existing table's column format exactly. One line per row.
- **Commit:** `docs(prowler): index the github-repo-explorer and distill-subagent skills`

### Card 13: Document the site-adapter mechanism in README.md

- **Context:**
  - `plugins/prowler/adapter.go`
  - `plugins/prowler/reddit.go`
  - `plugins/prowler/hackernews.go`
- **Edits:**
  - `plugins/prowler/README.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `plugins/prowler/README.md`, add a short section describing the site-adapter mechanism: prowler routes a fetch through an ordered registry of site adapters (each matches a URL family and provides a higher-fidelity strategy than the generic HTML cascade, falling through when it cannot handle the page), and list the registered adapters — Reddit (rewrites to `old.reddit.com` and strips to body text, keeping comments) and Hacker News (reads the Algolia JSON API for `item?id=N` pages). Keep it consistent with the README's existing tone and the adapters' actual behavior as implemented in batch 1. One line per paragraph.
- **Commit:** `docs(prowler): document the site-adapter mechanism and adapters`

### Card 14: Bump the plugin version

- **Context:** none
- **Edits:**
  - `plugins/prowler/.claude-plugin/plugin.json`
  - `.claude-plugin/marketplace.json`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `plugins/prowler/.claude-plugin/plugin.json`, change the `"version"` field from `"1.0.0"` to `"1.1.0"` (a minor bump for the new adapters + two new skills). In the repo-root `.claude-plugin/marketplace.json`, change the prowler entry's `"version"` (`plugins[0].version`) from `"1.0.0"` to `"1.1.0"` to keep the marketplace listing in sync with the plugin manifest. Leave the marketplace's own top-level `"version"` field unchanged (it versions the catalog, not the plugin). Change nothing else in either file.
- **Commit:** `chore(prowler): bump plugin version to 1.1.0`

## Batch Tests

`verify: null` — this batch is entirely codeless (two new `SKILL.md` files, edits to an existing `SKILL.md`, `INDEX.md`, `README.md`, and a `plugin.json` version bump). There is no runnable surface: skills are Markdown instructions loaded at Claude Code runtime, not compiled or executed by a test. Verification is a manual read-through that the documented `gh` recipes are correct, the cross-skill `prowler:distill-subagent` load reference names a real skill created in card 9, and `plugin.json` remains valid JSON. The prowler binary's own tests are covered by batch 1's `verify` and are unaffected by this batch (no Go files touched).
