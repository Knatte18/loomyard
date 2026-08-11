# Batch: verdict-record

```yaml
task: 'gitexec: decide whether RunGit should return a typed error carrying stderr'
batch: 'verdict-record'
number: 1
cards: 3
verify: go test ./internal/lyxcwd/ -run TestEnforcement_MarkdownLinks
depends-on: []
```

## Batch Scope

This batch delivers the whole task: the design doc is rewritten from an open question into a recorded verdict, the roadmap's decision item is replaced in place by the implementation item, and the implementation task is filed in the wiki with its dependency.
It is one batch because all three cards are transcriptions of one settled document — `_mill/discussion.md` — and share the same context: no card can be reviewed sensibly without the discussion, and loading it once serves all three.
There is no external interface for a later batch to consume;
the follow-on work is a separate task, filed by card 3 and sequenced behind the fabric chain.

Batch-local decision beyond `## Shared Decisions` in the overview: **card 3 produces no git diff.**
The wiki is a separate repository owned by mill's wiki daemon, which serialises and commits its own writes.
Card 3 therefore carries `Commit: none` and its verification is a read-back through the same client, not a test.

## Cards

### Card 1: rewrite the design doc as a recorded verdict

- **Context:**
  - `_mill/discussion.md`
  - `manifest/designs/fabric-crucible-followups.md`
  - `CONSTRAINTS.md`
  - `docs/overview.md`
- **Edits:**
  - `manifest/designs/gitexec-error-shape.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Replace the entire contents of `manifest/designs/gitexec-error-shape.md`.
  The file stops being a list of open questions and becomes the recorded verdict.
  Every fact, table, query, snapshot date and rejected alternative below is transcribed from `_mill/discussion.md`;
  do not re-measure the tree, do not run any grep against the code, and do not verify a count.
  Read `_mill/discussion.md` in full first — its `## Decisions` and `## Technical context` sections are the source material for every section listed here.

  Write these sections, in this order.

  1. **Title and status blockquote.**
     Title the file for the verdict rather than the question.
     The status blockquote replaces the current "Status: a decision, not a plan" / "Deleted once the verdict is recorded, wherever it lands" pair, which on its face means delete now and is amended by this task.
     The new status states: the verdict is recorded here;
     the doc survives until the implementation lands and is then deleted under the Documentation Lifecycle rule, with the durable rationale moving into the `internal/gitexec` package header comment.
     Keep the existing sentence recording that this was filed as GitHub issue #145 and folded into the manifest.

  2. **`## The verdict`.**
     State the decision from the discussion's `verdict-second-entry-point`, `naming-run-vs-rungit`, `gitrepo-run-is-covered` and `guard-test-with-justification-comments` decisions in one compact block:
     a second, must-succeed entry point `gitexec.Run(args []string, cwd string) (string, error)` is added alongside `RunGit`;
     `RunGit` keeps its name, its four-value shape and its semantics;
     `gitrepo` gains the same pair (`run` stays raw, `runChecked` is added);
     and the remaining raw call sites are pinned by a guard test requiring a written justification comment.
     Say explicitly that this is not a legacy-vs-new split — the raw form is permanently correct for a real class of sites and is not deprecated.

  3. **`## Why`.**
     The rationale, from the same decisions: the split maps a distinction the code genuinely has;
     the short name goes to the form that should be reached for by default, so the path of least resistance is the safe one;
     the usual objection to incremental migration is answered by the guard test rather than by a big-bang rewrite;
     and the `go-git` feasibility spike is cited as an argument *for* the change, per the discussion's `go-git-spike-is-a-supporting-argument` decision — callers consuming an `error` rather than a synthesised stderr-plus-exit-code pair is what makes a backend swap possible at all.

  4. **`## The counter-argument, weighed`.**
     Preserve the existing section's substance: this is diagnostic quality, not correctness, and "not worth it, recorded" was a legitimate outcome.
     State why it was weighed and not accepted, per the discussion's `verdict-second-entry-point` rejected list — the 55-of-74 figure is what the API shape produces rather than who wrote the lines, and the blast radius outside fabric turned out to be four production sites.

  5. **`## Rejected alternatives`.**
     One short entry per rejection carried by the discussion, each with the reason it was rejected: breaking signature change on `RunGit` itself;
     guard test only with no shape change;
     "not worth it, recorded";
     the `RunGitE` name;
     renaming the raw form to `RunGitRaw`;
     adding a `Stdout` field to `GitError`;
     a minimal `{ExitCode, Stderr}` struct;
     a literal `(no stderr)` marker;
     wrapping exec-level failures as `GitError{ExitCode: -1}`;
     returning `(stdout string, exitCode int, err error)` from the checked form;
     returning `""` on error;
     scoping the verdict to `gitexec` only;
     deleting `gitrepo.run` in favour of direct calls;
     implementing `runChecked` on top of `run`;
     pinning raw sites without requiring a justification comment;
     and no guard test at all.

  6. **`## The new shape`.**
     The full specification, from the discussion's `giterror-shape`, `exec-level-failures-stay-unwrapped`, `drop-exitcode-from-the-checked-signature`, `stdout-is-returned-even-when-the-error-is-non-nil` and `gitrepo-run-is-covered` decisions.
     Carry the `GitError` struct definition in full in a Go fenced block, the `Error()` rendering rule (`git <args>: exit <code>: <trimmed stderr>`, with the trailing segment omitted entirely when stderr is empty), and the arg-joining rule (space-separated, `%q`-quoted only for an arg containing whitespace or an empty arg).
     Carry the credentials rule verbatim in substance: args are rendered with no redaction, the godoc must say so, and callers must not pass credentials in args.
     Carry the exec-level rule: a failure to execute git at all propagates unwrapped, so `errors.As` means precisely "git ran and rejected this".
     Carry the stdout rule: the first return value is never blanked, including when a `*GitError` is returned, and this must be stated in the function's godoc.
     Carry the exit-code rule: the checked form returns stdout and error only, with the code reachable on the error.
     Carry the `gitrepo` pair as a Go fenced block showing both one-line helper bodies, and state that `runChecked` calls `gitexec.Run` directly as a second chokepoint beside `run`.

  7. **`## How the migration goes`.**
     Everything from the discussion's `## Technical context` subsection of the same name, plus its `the-migration-is-a-two-message-merge-not-a-substitution` decision.
     This section must carry, each as its own labelled sub-part: the before/after fenced snippets for the dominant two-message merge shape;
     the default merge rule (the exit-path message wins, stderr-as-`%s` becomes error-as-`%w`, the exec-path message is dropped because the error's own text now carries it, and deviations are read individually and noted);
     the rule that the `(git exit %d)` fragment is dropped together with its argument, with the worked example and the merged form;
     the exception where a `%d` cites a *prior* call's exit code and must not be dropped;
     the sentinel clause (the sentinel keeps the `%w` verb and the error goes in as `%v`, and a bare sentinel return may stay bare), with the reason — `errors.Is` at its consumers and the exact-string test assertions that pin the bare-sentinel surface;
     the deliberate-suppression class that stays raw under a test-pinned contract, stated as a live counter-example to "every discard is an accident of the shape";
     the error-constructing-helper re-signature, with the decision to drop the helper's detail-selection branch and the standing instruction to check every error-constructing helper the merge touches for the same split;
     the `errors.As` predicate-recovery snippet as a Go fenced block;
     the content-sniffing class where stderr content rather than the exit code decides answer-versus-failure, with both worked examples and the disposition that the sniff moves onto the recovered error;
     the mixed tri-state class with its `errors.As` recovery snippet and the inverted two-row disposition table for the two `diff --cached --quiet` sites;
     the four outside-fabric dispositions as a table;
     the seven full-discard sites split into their two classes, with the note that `//nolint:errcheck` enforces nothing here because the repo has no external linter and the guard test is the mechanism instead;
     and the hand-read exceptions.

  8. **`## Site inventories — shapes and regeneration queries`.**
     Every inventory from the discussion's `## Technical context`, each recorded as a shape plus the query that regenerates it, each with its measurement date, and each explicitly labelled a snapshot that must be re-derived at implementation time.
     Include: the production call-site count table;
     the four concrete outside-fabric sites;
     the `gitrepo` fan-out with its total and discard-subset queries, including the note about why the naive discard query undercounts;
     the predicate-site inventory keyed to the call site with the comparison as a column;
     the aggregate-classification numbers with the discussion's caveat that they are approximate, the two traps a classifier hits, and the instruction that any future pass must assume there is another error-constructing helper it has not accounted for;
     the two-message-merge regeneration query with its coarse-versus-careful window note;
     the "two different 51s" warning that the merge-site count and the error-returning-comparison count are separate measurements over different units that coincide numerically and neither corroborates the other;
     the full-discard query;
     the error-constructing-helper query;
     the content-sniff query;
     and the unclassified sites listed as still to be read individually.
     Open the section with the snapshot framing from the discussion's `verdict-carries-shapes-and-a-regeneration-recipe-not-durable-line-numbers` decision, including the concrete instance of why the coordinates rot: one of the discard sites is a `branch -D` call that the in-flight chokepoint slice routes through an executing gate, after which the discarded return swallows a refusal.
     State the acceptance bar in this section: the doc must be **re-derivable** from the doc alone, not executable from it.

  9. **`## The guard test and its invariant`.**
     The full specification of the gitexec Checked-Call Invariant from the discussion's `guard-test-with-justification-comments` decision, with an opening sentence stating plainly that neither the invariant nor its test is written by this task — both land in the implementation commit, and `CONSTRAINTS.md` is not edited here, because an invariant with no enforcing test is the rot that file exists to prevent.
     Carry: the marker-comment form and the fact that the justification is "why the raw form is correct here" rather than the narrower wording, which is unfillable at the deliberate-suppression sites;
     the two raw classes named (pure predicate, and pinned deliberate-suppression contract);
     the keying decision — on the marker comment plus a per-package count tripwire, with no file:line list and no enclosing-function list, and the reason location keys and function keys were both rejected;
     the test-file exemption and the reason;
     the two worked guard-test examples the repo already has, named as the pattern to mirror, and the note that the repo has no external linter so "lint rule" here means a guard test;
     the composition rule against the gitrepo Client Boundary Invariant — which is keyed by method name and which by call site, which change trips which, and the requirement that each invariant's entry carries a one-line cross-reference to the other;
     the three Client Boundary assertions that break in the implementation commit, stated as fact rather than risk, with the reason each breaks;
     the three further guards that key on the literal old token and go blind to the new entry point, each named with the invariant it protects, plus the grandfather exemption that must be updated in the same commit;
     and the known blind spot inherited from the sibling invariant.

  10. **`## The implementation task`.**
      The slug, the dependency and the reasoning, from the discussion's `implementation-task-identity` decision.
      Carry the corrected size estimate: the original "55 discard sites need judgement, the rest is a sweep" cost model is close to inverted — the sites needing no thought are the small group, and roughly 51 need a message decision precisely *because* they already carry two messages.
      Carry the two scope exclusions: the implementation does not re-review the wording quality of the sites the crucible already fixed, and the fixed-wrong-cause paths remain a separate hand-read exception because a shape change alone does not remove a wrong cause.
      Carry the hand-off note from the discussion's `verdict-doc-lifecycle` decision: the implementation task deletes this doc and removes the roadmap's link to it **in the same commit**, or Markdown Link Integrity fails on a dangling relative link.

  11. **`## Related`.**
      Keep both existing links live and their existing explanations: the sibling design doc for the fabric-local classes from the same campaign, and the anchor into `CONSTRAINTS.md` for the gitrepo Client Boundary Invariant.
      Both link targets already exist;
      do not change either target.

  Every link in the finished file must resolve.
  If the verdict introduces an intra-document anchor, the heading it points at must exist in this same file with a matching GitHub-style slug.
  Do not add a link to any file that does not exist, and do not introduce an outbound link into another repo document unless its anchor is verified against that document's real headings.

  Write the file with semantic line breaks — one sentence per line, extra breaks at internal clause boundaries, no fixed-column wrapping, table cells and blockquotes on one line.
  Then run `go run ./tools/mdreflow` over this file.
  If it reports MISMATCH the file is left untouched and content moved during reflow — investigate and fix the prose rather than forcing the write.
- **Commit:** `docs(gitexec): record the RunGit error-shape verdict`

### Card 2: replace the roadmap decision item with the implementation item

- **Context:**
  - `_mill/discussion.md`
  - `manifest/designs/gitexec-error-shape.md`
- **Edits:**
  - `manifest/roadmap.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `manifest/roadmap.md`, replace the existing numbered item whose heading text begins "**gitexec: decide whether `RunGit` should return a typed error carrying git's stderr**" — currently the last item of its section, six lines long, ending with the `See [designs/gitexec-error-shape.md](designs/gitexec-error-shape.md).` line.
  Replace all six lines in place.
  Do not move the item to a different section and do not renumber anything around it;
  the surrounding list uses `1.` for every entry, so the replacement keeps that form.

  The replacement item is the implementation item, not the decision item.
  It must:
  - Lead with a bolded title naming what gets built rather than what was decided, matching the filed task's slug in spirit — the checked `gitexec` entry point and the call-site migration.
  - State the verdict in one line: a second must-succeed entry point `gitexec.Run` returning stdout and a typed error, `RunGit` unchanged and permanently correct for predicate sites, `gitrepo` gaining the same pair, and the remaining raw sites pinned by a guard test requiring a written justification comment.
  - State the sequencing: it is filed behind the tail of the serialised fabric chain, because it rewrites roughly 70 call sites in the package that chain exists to protect from concurrent edits.
  - State the corrected size signal in one clause: the bulk of the work is a per-site merge of two existing error messages under a stated default rule, not a mechanical sweep.
  - Keep the `See [designs/gitexec-error-shape.md](designs/gitexec-error-shape.md).` line as its last line, unchanged.
    That link must stay live — the design doc is rewritten by this task, not deleted, and removing the link belongs to the implementation commit that deletes the doc.

  Do not restate the crucible's raw counts;
  the design doc carries them and the roadmap entry points at it.
  Write with semantic line breaks — one sentence per line, no fixed-column wrapping.
  Then run `go run ./tools/mdreflow` over this file and resolve any reported MISMATCH before committing.
- **Commit:** `docs(roadmap): replace the gitexec decision item with the implementation item`

### Card 3: file the implementation task in the wiki

- **Context:**
  - `_mill/discussion.md`
  - `manifest/designs/gitexec-error-shape.md`
- **Edits:** none
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create the follow-on task in the wiki through mill's wiki daemon client.
  Never touch wiki files directly — no `git`, no editor, no copy.
  The daemon owns that repository and serialises every write, and it commits and pushes on its own.

  Run exactly one upsert, then read it back.
  The upsert is a single `upsert_task` call with `slug='gitexec-checked-entry-point'`, a title naming what is built, a one-paragraph `brief`, a `body` carrying the pointer to the design doc plus the shape summary, and `depends_on=['fabric-corrindex-record-race']`.

  Use this command shape, with the wiki path resolved by mill's own path helper rather than hardcoded:

  ```bash
  PYTHONPATH="$MILL_SCRIPTS" "$MILL_PYTHON" - <<'PY'
  from pathlib import Path
  import _paths
  from wiki import _client
  wiki = _paths.resolve_wiki_path(_paths.resolve_git_root())
  _client.upsert_task(
      wiki,
      "gitexec-checked-entry-point",
      title="...",
      brief="...",
      body="...",
      depends_on=["fabric-corrindex-record-race"],
  )
  PY
  ```

  `MILL_PYTHON` is already exported in this environment;
  derive `MILL_SCRIPTS` as the `scripts` directory beside it in the mill plugin install, or read it off the value `MILL_PYTHON` points into.
  If neither environment variable resolves, stop and report it rather than falling back to editing wiki files.

  Content requirements for the three text fields:
  - **title** — names the build, not the decision: adding the checked `gitexec` entry point and migrating the call sites.
  - **brief** — one paragraph: what is added (the checked entry point returning stdout plus a typed error, the `gitrepo` checked sibling, the Checked-Call Invariant and its guard test), the size signal (roughly 70 call sites in the fabric engine plus a further fan-out behind the `gitrepo` helper, and the bulk of it a per-site two-message merge under a stated default rule rather than a sweep), and the two known guard-test collisions that must be fixed in the same commit.
  - **body** — points at the design doc by path as the full specification, and records the four things the implementation commit must do beyond the migration itself:
    1. Land the Checked-Call Invariant's text and its guard test.
    2. Fix the gitrepo Client Boundary Invariant guard, whose three assertions all break once the gitrepo checked sibling exists — the exactly-one-occurrence count, the requirement that the one occurrence sit inside the raw helper's body, and the pinned method set keyed on calls to the raw helper, from which every migrated method silently drops out.
       This is the second of the two guard-test collisions the brief promises, and it is structurally distinct from item 3: it is keyed on occurrence counts and method names, not on a literal token.
    3. Update the three guards that key on the old literal token and therefore go blind to the new entry point, including the grandfather exemption that names one file by hand.
    4. Delete the design doc and remove the roadmap link to it in that same commit.

  Then verify by reading the record back with `get_task` for the same slug and confirming three things: the slug matches, the title is the one just written, and `depends_on` is exactly the one-element list naming the corrindex-race task.
  Print the read-back record.
  If any of the three does not match, report the mismatch and stop;
  do not attempt a second upsert to paper over it.
- **Commit:** none

## Batch Tests

`verify:` runs `TestEnforcement_MarkdownLinks` in the `internal/lyxcwd` package — the permanent guard behind the Markdown Link Integrity invariant, which scans every `.md` file under `manifest/` and `docs/` and resolves both the file part and the `#anchor` of every inline link.
It is the only runnable check that this task's two edited files, both under `manifest/`, can fail:
the rewritten design doc must keep its two outgoing links resolving and any new intra-document anchor must match a real heading, and the roadmap's link to the design doc must still point at a file that exists.
Scoping to this one test rather than the package or the repo is deliberate — the task changes no Go code, so every other test in the tree is unaffected by construction, and `pipeline.done_gate` already runs the repo-wide suite before the task is marked done as the negative check that nothing outside `manifest/` moved.

Two verifications sit outside `verify:` because they have no Go test surface.
Reflow conformance is checked per card by running mdreflow over the file that card wrote, with a reported MISMATCH treated as a blocker rather than a warning.
The wiki record is verified by card 3's own read-back through the daemon client, since the wiki is a separate repository and no test in this tree can see it.
