I have verified the plan against all source files. The inventory is complete (13 product files, all covered), the Batch Index DAG is acyclic with both batch files present, Moves are well-formed with the rename mechanic section present in batch 1, global card numbering is sequential (1-6), and the pinned-guard retargeting (helptree, jsonhelp, leaf-enforcement, registration/longlist) matches the actual source lines. The reframed help prose stays mechanism-accurate (still files a GitHub issue via gh), satisfying the CLI/Cobra Invariant. One minor enumeration gap found.

MILL_REVIEW_BEGIN
# Review: Rename internal/ghissues → selfreport — holistic

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-06-30
```

## Findings

### [NIT] Card 2 omits cli.go file-header comment
**Location:** Batch 1 / Card 2
**Issue:** Card 2 lists "the package doc comment and the `Command`/`RunCLI`/`runCreate` doc comments" but `cli.go` has a separate top-of-file header block (lines 1-5: "cli.go assembles the Cobra command tree for the ghissues module") that is neither the package doc comment nor a function doc comment, so a literal reading leaves "the ghissues module" stale — violating the `rename-all-identifiers` decision and the zero-`ghissues`-grep check asserted at end of Card 6 (which does not edit cli.go, so the stale line survives; `go build`/`go test` won't catch a comment).
**Fix:** Add the cli.go file-header comment (lines 1-5) to Card 2's enumerated comments, mirroring how Card 1 explicitly names both the `ghissues.go` header and the `selfreportengine` package doc comment.

## Verdict

APPROVE
Plan is complete, constraint-compliant, and well-sequenced; one NIT on a stale-comment enumeration gap.
MILL_REVIEW_END
