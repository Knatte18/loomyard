I now have enough verified evidence for my findings. Here is the review.

MILL_REVIEW_BEGIN
# Review: Add lyx init --undo / deinit command — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-07-01
```

## Findings

### [BLOCKING] Card 3 missing Context for the identifiers its Requirements name
**Location:** `01-weftengine-commit-message.md`, Card 3
**Issue:** Card 3's `Context:` is `none`, but its Requirements repeatedly reference `DefaultCommitMessage` (defined in `weft.go`) and "the new signature from Card 1" (in `sync.go`) — neither file is listed in `Context:` or `Edits:` (only `sync_test.go` is). Per the Context-completeness rule the implementer for this card cannot read those files.
**Fix:** Add `internal/weftengine/weft.go` and `internal/weftengine/sync.go` to Card 3's `Context:`.

### [BLOCKING] Card 10 missing Context for lyxtest and fslink identifiers its Requirements depend on
**Location:** `04-initcli-undo.md`, Card 10
**Issue:** Requirements name `lyxtest.CopyPaired` and its `WeftBare`-left-empty-on-`CopyPairedLocal` distinction (verified: this exact contract lives in `internal/lyxtest/lyxtest.go`'s doc comments, lines ~449-517), and separately name `fslink.Remove` for the real-directory-guard/target-mismatch setup — but neither `internal/lyxtest/lyxtest.go` nor `internal/fslink/fslink.go` is in Card 10's `Context:` (only `initcli.go`, `undo.go`, `initcli_test.go`, `junction.go`, `gitignore.go`, `hubgeometry.go` are listed).
**Fix:** Add `internal/lyxtest/lyxtest.go` and `internal/fslink/fslink.go` to Card 10's `Context:`.

### [NIT] Card 10 references a non-existent `initcli.RunCLI`
**Location:** `04-initcli-undo.md`, Card 10, `TestRunInit_Undo_HappyPath`
**Issue:** The card says to call "`initcli.RunCLI(&buf, []string{"--undo"})` (or the equivalent...)" — verified `internal/initcli` only ever exports `RunInit` (`internal/initcli/initcli.go:54`), never `RunCLI`; this module's seam name deviates from the CLI/Cobra Invariant's `RunCLI` naming, which the plan doesn't fix and shouldn't need to for this task's scope, but citing a symbol that doesn't exist is confusing to an implementer building the test.
**Fix:** Reword to just `initcli.RunInit(&buf, []string{"--undo"})`, dropping the `RunCLI` reference entirely.

## Verdict

REQUEST_CHANGES
Two cards (3, 10) reference identifiers from files absent from their `Context:`, a review-blocking gap.
MILL_REVIEW_END