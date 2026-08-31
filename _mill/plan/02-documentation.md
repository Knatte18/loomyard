# Batch: documentation

```yaml
task: Add cross-repo code search to prowler
batch: documentation
number: 2
cards: 1
verify: null
depends-on: [1]
```

## Batch Scope

This batch delivers the documentation the new capability needs to be reachable and usable: the skill's dispatch surface (`SKILL.md` frontmatter and the duplicate row in `INDEX.md`), the skill body (the new script's invocation, the search-versus-tree routing rule, and the rewritten argument-disambiguation paragraph), and the plugin `README.md` contract section.
It is one batch because all three files document the same capability from three angles and are only writable once batch 1 has settled the script's actual contract;
it depends on batch 1 for exactly that reason.

It is a single card because the three files must land together: `INDEX.md`'s row duplicates `SKILL.md`'s `description` verbatim by convention, so updating one without the other ships two documents that disagree, and the repo's own Documentation Lifecycle rule wants the docs in the same commit as the change they describe.

Batch-local decision: nothing here is verified by a test.
Documentation correctness is a review property, and this batch introduces no runnable surface — see `## Batch Tests`.

## Cards

### Card 6: document the new script, its routing rule, and the rewritten argument disambiguation

- **Context:**
  - `plugins/prowler/scripts/github-code-search.sh`
  - `plugins/prowler/scripts/github-tree.sh`
- **Edits:**
  - `plugins/prowler/skills/github-repo-explorer/SKILL.md`
  - `plugins/prowler/skills/INDEX.md`
  - `plugins/prowler/README.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Update all three documentation files in one commit.
  Every prose line follows the semantic-line-break rule from the overview's Shared Decisions;
  the `INDEX.md` table row stays on one line.

  **The skill's dispatch surface.**
  Replace the `description` field in the skill's frontmatter with exactly this text, verbatim:

  ```
  Search code across GitHub repos, browse a repo's file tree, and read files via the gh CLI, without cloning
  ```

  Replace the `argument-hint` field with exactly this value, verbatim, quotes included:

  ```
  "<owner/repo>... [path | search term] [question]"
  ```

  Then replace the description cell of the `github-repo-explorer` row in the skills index table with the identical new description text.
  The frontmatter description is the skill's dispatch surface — it is what decides whether the skill is reached at all — and today it names only tree-browsing and file-reading, so leaving it unchanged would ship a search capability that a cross-repo search query would never route to, defeating the task's own trigger case.
  The index row duplicates that description verbatim today, so the two must move together.
  Change nothing else in the skills index: that file receives this one row edit and no other.

  **The new script's invocation, in the skill body.**
  Add a section documenting `github-code-search.sh` beside the existing tree-listing section, following the same pattern the tree section already establishes: resolve the script's absolute path first, while the skill-directory variable is still set, because a dispatched subagent will not have it;
  then run it with `bash`.
  Document the synopsis (a query followed by one or more repo refs), the tab-separated three-field record shape, the fact that a hit with no fragment still emits a record with an empty third field, and the ten-repo cap with the ten-requests-per-minute rate-limit bucket that motivates it.
  Restate the exit-code contract the same way the existing tree section does — on failure the script prints exactly one line to stderr, emits nothing on stdout, and exits non-zero, so an empty result on success and an empty result on failure are indistinguishable unless the exit code is checked.
  Document the two rejections a caller is most likely to hit by accident: a query containing the `repo:` qualifier is refused because the script appends its own and the last one wins, with a raw `gh api` call as the escape hatch;
  and an empty or whitespace-only query is refused because it degrades into a worse tree listing on the scarcest bucket in the plugin.
  Document that a repo with more matches than one page returns a stderr note naming the true total while still exiting 0, so a capped listing is never silently partial.

  **The routing rule, as a rule rather than a preference.**
  Add an explicit routing rule to the skill body: a known term, import, symbol, or dependency name anywhere in a repo routes to the search script;
  understanding a repo's overall structure or layout routes to the tree script plus selective reads;
  and a repo list to relevance-map against one trait routes to the search script over the whole list in one invocation.
  Restate at the same point that deep single-repo work is out of this skill's remit and goes through a local clone instead, so the skill's boundary stays visible where it is acted on.

  **The rewritten argument-disambiguation paragraph.**
  The skill body's existing disambiguation paragraph — the one stating that a second token is forwarded as a path only when it matches an accepted character set and contains no whitespace — is replaced wholesale by a two-part rule.
  The old predicate was written when the only second-slot meaning was a path, and it breaks in two ways under the new hint: a bare search term satisfies it and would route to the tree script, and a plural repo list has no stated boundary at all.
  Write the replacement to encode exactly this:

  Part one decides where the repo list ends.
  A repo-shaped token is a leading whitespace-separated token matching the same `<owner>/<repo>` predicate named in the overview's Shared Decisions — the one the tree script already enforces, reused rather than a second looser one.
  Take the maximal leading run of repo-shaped tokens, then resolve the run by its length.
  A run of length one is one repo, and the remainder is the second slot, decided by part two.
  A run of length three or more is always a repo list — nobody writes one repo followed by two paths, and the tree script takes at most one path anyway.
  A run of length exactly two is genuinely ambiguous, because a repo-relative path is repo-shaped too;
  break it on phrasing, where a sweep marker in the request ("which of these", "any of", "across", "compare", "both", or an explicit enumeration of the repos in prose) makes it a two-repo list, and otherwise the second token is a path against the first repo.
  Give the caller two explicit overrides for certainty: a trailing slash on the second token always forces path, and naming the repos in prose always forces sweep.
  State that the list never extends past that run and that no terminator token is introduced.

  Part two decides what the remainder is.
  With two or more repo refs the remainder is never a path — a repo-relative path has no meaning across a repo list — so it is the search term or question, and the invocation routes to the search script.
  With exactly one repo ref, the remainder is a path for the tree script only when the phrasing asks for structure ("what's in", "list", "show me the tree", a trailing slash), and is a search term otherwise.
  State explicitly that when the phrasing settles it either way, phrasing wins over token shape.
  Close the paragraph with the ambiguous-single-repo default: when neither phrasing nor shape settles it, route to the search script, because a mis-routed search costs one request and still answers whether the token appears in the repo and where, while a mis-routed tree call returns the whole file list, answers nothing about the token, and costs the caller a second turn to recover.

  **The plugin README contract section.**
  Add a section for the new script beside the existing one-call-repo-tree-listing section, in the same voice and at the same level.
  Document the contract (query plus one or more repo refs, one tab-separated record per matching file, buffered until the whole sweep succeeds so a partial prefix never reaches stdout), the rate-limit budget (one preflight call per repo against the 5000-per-hour core bucket plus one search call per repo against the ten-per-minute search bucket, which is what fixes the cap at ten repos), and the API quirks the contract is built around: repeated repo qualifiers do not combine — the last one wins and the earlier ones are silently discarded, which is why the script issues one call per repo rather than one call for the sweep;
  a nonexistent repo answers 200 with a zero total, indistinguishable from a real repo with no matches, which is why the preflight exists;
  a partial result set arrives as a 200 carrying an incomplete-results flag, which the script treats as a hard failure;
  and results are capped at one page here and at 1000 by the API regardless.
  Document the `repo:`-in-query limitation and its raw-call escape hatch here too, since this is the script's contract section.
  Note, as the tree section does for itself, that the only runtime dependency is gh — every field is extracted through gh's own embedded jq and no system jq is invoked at run time — and that the offline harness carries the one extra dependency of system jq, which it checks for up front.
- **Commit:** `docs(prowler): document github-code-search.sh, routing, and argument disambiguation`

## Batch Tests

`verify: null`.
This batch touches three Markdown files and no executable surface, so there is nothing runnable to assert: no script changes behaviour, no test file is added or edited, and the repo has no Markdown linter wired into a verify command.
The documentation's correctness is a review property — that the frontmatter and index descriptions are byte-identical to each other, that the documented contract matches the script batch 1 actually shipped, and that the disambiguation rule is complete enough for a model to apply — and the holistic code review is where it is checked.

The task-wide done gate configured for this hub still runs the Go suites before the task is marked done;
this batch adds no Go code, so it neither affects nor depends on that gate.
