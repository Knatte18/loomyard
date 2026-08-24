MILL_REVIEW_BEGIN
# Review: loom: Discussion-Write producer

```yaml
duration_s: 188.0
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-24
```

## Findings

### [NIT:decision] Archive siblings have no stated disposition
**Demoted-from:** BLOCKING
**Section:** Decisions → `commit-produced-artifacts`
**Issue:** `shedadapters.archiveStaleOutputs` (archive.go:33–47) renames a stale output to a timestamped **sibling in the same directory**, so a bounce round leaves `_lyx/discussion/decision-record-<stamp>.md` beside the live files; the chosen pathspec is the whole directory (`[]string{loomengine.DiscussionDirRel()}`), so those archives are silently committed into the weft and accumulate per round, while a two-file pathspec would instead leave them as exactly the untracked weft dirt this decision exists to eliminate.
**Fix:** Decide the archive disposition explicitly — commit them, delete them after a successful commit, or route them under `.lyx` — and state which of directory-pathspec vs two-file-pathspec follows from that choice.

### [BLOCKING:design] "Already-clean no-op" premise is false on a bounce
**Section:** Decisions → `commit-produced-artifacts`, rationale
**Issue:** The rationale claims "Committing an already-clean, already-tracked path is a no-op … so the decorator is safe to run on every `Done`, including a `Discussion-Validate` bounce round" — but on a bounce round the content has changed (archive + rewrite), so the commit is real, and the decorator fires on `Discussion-Write`'s `Done` **before** `Discussion-Validate` has run, committing records the validator is about to reject.
**Fix:** Either restate the rationale to accept committing pre-validation output as intentional, or move the commit behind validation (e.g. a decorator on `Discussion-Validate`) and record the choice.

### [NIT:design] Dev-mode reconcile will not refresh the rewritten stencil
**Section:** Constraints → Stencil Ownership Invariant
**Issue:** The constraint notes only that no registry change is needed; `stencilstore.reconcileOne`'s `StateUntouched` branch returns `false` with a `logger.Warn` under `ModeDev`, so a rewritten `loom-template-discussion.md` never reaches an already-seeded hub built with a dev binary — the running agent keeps the old Step 2/Step 3 text.
**Fix:** Record the dev-mode non-refresh as a known operator step (as `scribe-best-effort` already does for the plugin install), or name the verb that forces the refresh.

### [NIT:consistency] Pointer-rule bullet vs. the blind-bounce rationale
**Section:** Constraints → Producer Pointer-Rule Invariant vs. Decisions → `blind-revalidate-bounce`
**Issue:** The constraint says the stencil "must not restate `Discussion-Validate`'s checklist", while `blind-revalidate-bounce`'s whole rationale rests on the stencil enumerating the seven required H2 headings in Step 5 — which is exactly that checklist, admitted only obliquely via the "its own two output files' shape" carve-out.
**Fix:** State plainly that the seven-heading list stays in Step 5 as the output-shape carve-out, so a plan writer does not delete it in the name of the pointer rule.

### [NIT:scope] Commit message text unspecified
**Section:** Decisions → `commit-produced-artifacts`
**Issue:** The `CommitAnchoredPaths(... , msg, ...)` call names `msg` but the discussion never says what it contains, unlike `run.go:120`'s pinned `"loom: seed session bootstrap for %s"` shape it claims to mirror.
**Fix:** State the message format (and whether it carries the slug), since it is durable weft history.

## Verdict

REQUEST_CHANGES
Commit pathspec, archive disposition, and commit-before-validation ordering are undecided.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 1._
MILL_REVIEW_END
