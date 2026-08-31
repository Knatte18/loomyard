# Batch: skill and README documentation

```yaml
task: "Prefer raw fetch, scope large tree listings"
batch: "skill and README documentation"
number: 3
cards: 2
verify: null
depends-on: [1, 2]
```

## Batch Scope

This batch updates the two documents that describe the scripts batches 1 and 2 changed: the `github-repo-explorer` skill, which is what the calling agent actually reads, and the plugin README, which carries the script-level cost and dependency claims.
It is one batch because both documents describe the same finished CLI surface and must agree with each other and with the shipped scripts;
splitting them would let the two drift.

It depends on batch 1 for the finished tree-listing surface — the children mode, the trailing-slash convention, and the entry-count guard's abort shape — and on batch 2 for the read script's surface and, specifically, for the symlink observation card 8's live capture records, which decides whether one sentence in the skill is written at all.

The project's task-completion rule requires these documents to land in the same task as the code, which is what this batch satisfies.
No `manifest/designs/` module doc is created, since no prowler design doc exists today and this change does not warrant one;
`manifest/roadmap.md` does not move, since this is hardening of an already-merged change;
and `CONSTRAINTS.md` is unchanged, since no new cross-cutting invariant arises.

## Cards

### Card 16: `github-repo-explorer` skill — read order, listing modes, and the guard

- **Context:**
  - `plugins/prowler/scripts/github-read.sh`
  - `plugins/prowler/scripts/github-tree.sh`
  - `plugins/prowler/scripts/testdata/github-read/CAPTURE.md`
  - `plugins/prowler/skills/distill-subagent/SKILL.md`
- **Edits:**
  - `plugins/prowler/skills/github-repo-explorer/SKILL.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Edit `plugins/prowler/skills/github-repo-explorer/SKILL.md` so it documents the finished surface of both scripts.
  Replace the two file-read lines — the one instructing a contents call piped through a base64 decode, and the lighter-alternative line naming the raw host — with a single instruction to call the read script, since a script makes the preference order structural rather than dependent on the agent remembering it under context pressure.
  Resolve the read script's absolute path by the same idiom and in the same numbered step where the tree script's path is already resolved while the skill directory variable is still set, and for the same stated reason: a dispatched subagent will not have that variable.
  Both resolutions belong together in that step so neither is reached after the variable is gone.
  State the read script's signature as an owner-and-repository reference plus one repo-relative path, that it reads exactly one file per call, that stdout is the content verbatim with no banner or delimiter, and that reads are pinned to the default branch with no ref argument.
  Add listing-mode guidance covering the three choices: the children mode for exploring one directory at a time top-down, a scoped recursive listing when the caller already knows which subtree it needs, and a whole-repository listing when it does not.
  State the children mode's output convention explicitly — a directory carries a trailing slash and a file does not — because this document is what the calling agent reads, and a marker documented only in the README is a marker the caller does not have.
  State that a listing can now abort on an entry-count ceiling that defaults to one thousand entries, that the abort is an ordinary failure with one stderr line and a non-zero exit like every other, and that the caller's options are to scope the listing to a subdirectory, switch to the children mode, or raise the ceiling with the override flag — making a full dump a deliberate, visible act rather than an accident.
  Extend the existing paragraph about always checking the exit code so it covers the read script and the guard abort too;
  its reasoning, that an empty result on success and an empty result on failure look identical, applies to both unchanged.
  Leave the path-versus-question disambiguation paragraph's accepted character set exactly as it is — it is unaffected by this task, and it must not drift out of sync with the scripts' validation.
  Leave the closing pointer to the distillation skill in place;
  it is the documented answer to reading many files, which is why the read script needs no batch mode.
  Finally, consult `plugins/prowler/scripts/testdata/github-read/CAPTURE.md`: if the capture recorded that the raw host answers a symlink with a 200 and its target path as the body, add one sentence recording that a symlink path reads back as its target path rather than failing, since the type guard is reachable only on the fallback path and probing before every read would erase the entire measured win.
  If the capture recorded a non-2xx instead, that limitation is empty in practice and the sentence is omitted.
- **Commit:** `docs(prowler): document raw-first reads and scoped listings in github-repo-explorer`

### Card 17: plugin README — the read script, the new mode, and the guard

- **Context:**
  - `plugins/prowler/scripts/github-read.sh`
  - `plugins/prowler/scripts/github-tree.sh`
  - `plugins/prowler/scripts/github-read-selftest.sh`
  - `plugins/prowler/scripts/github-tree-selftest.sh`
- **Edits:**
  - `plugins/prowler/README.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Edit `plugins/prowler/README.md`.
  Extend the existing tree-listing section so it describes the children mode and the entry-count guard alongside the two modes it already covers, keeping its one-call cost claim intact for an untruncated listing and stating that the guard's default ceiling exists to stop an unscoped listing against a very large repository from silently returning tens of thousands of paths.
  Correct that section's claim that its only runtime dependency is `gh`: it remains true of the tree script itself, but the new read script adds an optional `curl` dependency whose absence costs speed rather than capability, so the claim must be scoped to the script it is actually about rather than left reading as a plugin-wide statement.
  Add a section for the read script, placed adjacent to the tree section since the two are siblings.
  State what it does, that it prefers the raw host and falls back to an authenticated call only on failure, the measured reason for that order — a flat cost per file against one that grows with file size, roughly an order of magnitude apart on the operation the skill performs most often — that its hard prerequisite is `gh` alone while `curl` is optional, that it has no build step and no lock since there is nothing to compile, and that it invokes no system `jq` at run time.
  Name its offline harness and record, as the tree section already does for its own, that the harness carries the one extra dependency of system `jq` which it checks for up front.
  Do not claim either harness is wired into CI, because neither is.
- **Commit:** `docs(prowler): document github-read.sh and the tree listing guard in the README`

## Batch Tests

`verify: null`.
This batch is pure documentation: two markdown files with no runnable surface, no script, and no test fixture.
Running either harness here would assert nothing about the change, since neither reads a markdown file.

Correctness is verified by review instead, against a concrete and checkable standard: every CLI surface either document describes must match what batches 1 and 2 actually shipped — the flag names and their defaults, the trailing-slash directory convention, the guard's exit-code and one-stderr-line shape, the read script's two-positional signature, and its prerequisite split between a hard `gh` and an optional `curl`.
The one conditional sentence in card 16 is checkable the same way: it is written if and only if the capture record from card 8 says the raw host answers a symlink with a 200.

The markdown itself is bound by the project's semantic-line-break rule, which applies to lines edited in place as well as newly-added ones, and is likewise a review check rather than a runnable one since the repository has no markdown linter.
