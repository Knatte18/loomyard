MILL_REVIEW_BEGIN
# Review: fabric: accumulate the result envelope from mutations, not control flow (slice 14) — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: claude-opus-5 (harness-reported); best-effort self-assessment, Anthropic Claude Opus-class
reviewed_file: plan/
date: 2026-08-11
```

## Findings

### [BLOCKING:consistency] Card 22's `res.Mutated().Entries()` cannot compile
**Location:** batch 5 / card 22 (Requirements, "reading the record through `res.Mutated().Entries()`")
**Issue:** Card 1 declares `Entries()` with a pointer receiver and explicitly warns that `res.Mutated()` returns a non-addressable `Mutations` value, so `res.Mutated().Len()` does not compile; `res.Mutated().Entries()` is the identical construct and fails the same way.
**Fix:** Have card 22 prescribe `rec := res.Mutated()` into a local first (or give `Entries()` a value receiver in card 1 and drop the pointer-receiver rationale there).

### [BLOCKING:scope] Exclude-file target is not in scope at card 17's recording site
**Location:** batch 5 / card 17 ("records the resolved exclude-file path with `Append` only when `changed` is true")
**Issue:** The path is produced by `resolveGitExcludePath` *inside* `mutateGitExclude` (gitexclude.go:60-64) and never returned; `seedGitExclude`/`unseedGitExclude` hold only `WorktreePath(l, slug)`. The card forbids touching `mutateGitExclude`, so no in-scope value satisfies the requirement, and `seedGitExclude` currently returns bare `error` and discards `changed` (junction.go:611-612).
**Fix:** State the disposition explicitly — either widen `mutateGitExclude`'s return to carry the resolved path, or name the worktree path as the recorded `Target` — and say that `seedGitExclude` must capture `changed`.

### [BLOCKING:design] Card 21's launcher entries contradict record-only-on-observed-effect
**Location:** batch 5 / card 21 ("It records **two** entries on success")
**Issue:** `writeLaunchers` uses `os.MkdirAll(launcherDir)` (launchers.go:89), which succeeds on a pre-existing directory, and the menu launcher is never-clobber — it returns nil early at launchers.go:116-118 when the menu already exists. `removeLaunchers` deliberately leaves the menu in place, so `restorePortalAndLaunchers`'s repair path hits exactly that early return; an unconditional `dir_created`/`file_written` pair records writes that did not happen and fires batch 7's commission direction on correct behaviour.
**Fix:** Card 21 must state the observation predicate for each entry — stat-before-MkdirAll for the directory, and the menu entry only on the branch that actually reaches the `os.WriteFile` at launchers.go:141.

### [BLOCKING:consistency] Card 27's reserved-key assertion is unsatisfiable for `okWithRecord`
**Location:** batch 6 / card 27 ("Reserved keys") vs card 23
**Issue:** Card 23 has `okWithRecord` delegate to `output.Ok`, which injects only `"ok"` (output.go:17-22) and never touches `"error"`. Card 27 nonetheless asserts that a caller supplying `ok` **or `error`** to `okWithRecord` is overridden; that case cannot pass as specified.
**Fix:** Scope the `error`-override assertion to `errWithRecord` only, or make card 23's `okWithRecord` strip/override `"error"` and say so in its doc-comment contract.

### [BLOCKING:decision] `runReconcile`'s error returns have no stated disposition
**Location:** batch 6 / card 25 (runReconcile half) with batch 6 / card 24's pre-flight carve-out
**Issue:** Card 24 removes `runReconcile` from its conversion list and hands it to card 25, but card 25 only says "emit through the card-23 helpers" for the success envelope. Its three `output.Err` sites (fabric.go:569, 574, 581) are unassigned, and card 24's carve-out justification — "nothing has been mutated at those points", naming `LoadConfig` — is false here because `configsync.ReconcileFabricAt` at fabric.go:568 already ran and may have written a file. `runCloneWithReset`'s post-`CloneAndWire` `output.Err` (clone.go:100) is likewise unassigned.
**Fix:** Card 25 must enumerate which returns become `errWithRecord` and restate the pre-flight carve-out as "before any CLI-layer mutation", not "before the verb call".

### [NIT:scope] Card 21 names helpers whose files are absent from Context, and ignores a conditional write
**Location:** batch 5 / card 21
**Issue:** `ensureBoardWorktree` lives in `internal/fabricengine/boardweft.go` and `writeWarpBinding` in `internal/fabricengine/warpbinding.go`; neither file is in Context or Edits. The `.lyx-anchor` write is also conditional — clone.go:354 runs only on the create branch, while the adopt branch (clone.go:323-344) writes nothing — and the card records it unqualified.
**Fix:** Add both files to Context and qualify the `.lyx-anchor` entry to the create branch.

### [NIT:scope] Card 25's "import already established" premise is wrong
**Location:** batch 6 / card 25 (`configengine.ConfigFile` justification)
**Issue:** `internal/fabriccli/cli_test.go` is `package fabriccli_test` behind the `integration` tag, so its imports establish nothing for production `internal/fabriccli/clone.go`, which needs a new `configengine` import. `fabricengine.BoardDir` (junctionnames.go:100) and `gitrepo.PushCoalesced`/`HasUnpushed` are also named in Requirements with their files absent from Context.
**Fix:** Drop the false premise, state the new import explicitly, and add the two files to Context.

### [NIT:consistency] Card 10 prescribes a recorder placement card 14 immediately rewrites
**Location:** batch 3 / card 10 vs batch 4 / card 14
**Issue:** Card 10 puts `rec = NewMutations(hubPath)` before `createExclusiveDir` at clone.go:225; card 14 then moves it up to follow each `hubPath = HubPath(cwd, name)`. The plan elsewhere avoids exactly this double-edit (card 14's own `CloneAndWire` rationale).
**Fix:** Have card 10 place the assignment at its final position and let card 14 merely rely on it.

## Verdict

REQUEST_CHANGES
Five blocking defects: one non-compiling expression, three under-specified recording rules, one contradictory assertion.
MILL_REVIEW_END
