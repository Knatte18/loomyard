MILL_REVIEW_BEGIN
# Review: Spawned agent panes resolve the spawning lyx binary — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewed_file: plan/
date: 2026-09-21
```

## Findings

### [BLOCKING:decision] Two discussion decisions cited by name never entered the plan's decision ledger
**Location:** Batch 2 Card 7; Batch 3 Batch Scope
**Issue:** Card 7 repoints a citation at "this task's `prelude-is-session-scoped-in-both-dialects` decision," and Batch 3's scope prose invokes "the `docs-describe-the-landed-mechanism` decision," but neither name appears anywhere in `00-overview.md`'s `## Shared Decisions` or as a labeled batch-local decision in any batch file — both live only in `_mill/discussion.md` (lines 91 and 200 respectively), which no card lists in `Context:`. Every sibling discussion decision that matters to implementation (`prelude-dialect-matches-the-launch-command` → `prelude-dialect-is-ForGOOS`, `pane-start-mode-stays-untouched` → `pane-start-mode-is-untouched`, `executable-error-warns-and-degrades`) was carried into Shared Decisions verbatim or near-verbatim; these two were dropped, so an implementer working only from the plan hits an undefined citation at exactly the two points that need it (why `ExportEnv` differs from `WithEnv`, why Batch 3 adds no interim warning).
**Fix:** Add both as `### Decision:` entries — either promoted to `00-overview.md`'s `## Shared Decisions` (matching the pattern already used for the other four) or as batch-local decisions in `02-reed-pane-binary-chokepoint.md` / `03-docs-and-sandbox-preconditions.md` respectively.

### [NIT:consistency] Card 3 cites resolve.go's exact declaration without it in Context
**Location:** Batch 2 Card 3
**Issue:** Card 3's Requirements models `executablePath` on "`tools/sandbox/resolve.go`'s own `var devBinPath = devbin.BinPath`" (verified: that exact line is `tools/sandbox/resolve.go:30`), but `resolve.go` is absent from Card 3's `Context:` — it only enters Context in the later Card 8. The declaration is inlined in full, so nothing further needs reading, but the citation doesn't use this same plan's own "signature inlined, no file read needed" escape-marker phrasing used elsewhere (Cards 4/5), so it reads as an unlisted-file reference rather than a deliberately self-contained citation.
**Fix:** Add `tools/sandbox/resolve.go` to Card 3's `Context:`, or mark the citation with the plan's own escape-marker convention.

### [NIT:consistency] Shell Mechanics Seam's method enumeration goes stale
**Location:** Batch 1 Card 1; CONSTRAINTS.md
**Issue:** CONSTRAINTS.md's "Shell Mechanics Seam" invariant parenthetically enumerates the seam as `(Quote/Invoke/ReadFile, stdlib-only)`. Card 1 adds three more production methods to that same `Shell` interface (`ExportEnv`/`PrependPathEntry`/`Chain`) and requires `shell.go`'s own file-header comment to list them, but no card updates CONSTRAINTS.md's parenthetical to match, so the file-header enumeration and the invariant's own enumeration diverge after this batch lands. (The discussion doc quotes the same stale three-name parenthetical rather than proposing to extend it, so this reads as inherited rather than newly introduced.)
**Fix:** Either extend CONSTRAINTS.md's parenthetical in Card 1's CONSTRAINTS.md-adjacent work, or note in the invariant that the list is illustrative rather than exhaustive.

## Verdict

REQUEST_CHANGES
Two cards cite plan-level decisions that were never recorded in the plan itself.
MILL_REVIEW_END
