MILL_REVIEW_BEGIN
# Review: Replace reed's header pane with a status-line and Selvage — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-09-19
```

## Findings

### [BLOCKING:scope] doc.go's status-line pin bullet still describes the removed "status off" mechanism
**Location:** `internal/reedengine/doc.go:601-619`
**Issue:** The "Multiplexer contract surface" bullet titled "The two geometry option pins (windowsize.go)" still says `pinGeometryOptionsLocked` pins `"status off"` and `"window-size latest"`, and that `"#{status}"` maps `"off"->0, "on"->1, N->N`. Batch 4 (cards 24-27) completely rewrote `pinGeometryOptionsLocked` to pin `status on`, `status-position bottom`, `status-left`, `status-right`, `status-left-length`, `window-status-format`, `window-status-current-format`, plus `window-size latest` — seven-plus-one pins, not two — but no batch-4 card lists `doc.go` as an `Edits:` target, so this bullet was never updated and now misdescribes current behaviour as if it were still "status off".
**Fix:** Rewrite this bullet (or fold it into the status-line design already described earlier in the file) to name the seven status-line `set-option` calls actually issued and drop the stale `"off"`/`"on"`/`N` `#{status}` characterization that predates the status-line render.

### [NIT:consistency] overlay.go's execHook doc still names the deleted ensureHeaderPaneLocked
**Location:** `internal/reedengine/overlay.go:28`
**Issue:** `TmuxCmd.execHook`'s doc comment reads "...a composed engine call site (e.g. `ensureHeaderPaneLocked`'s header-rebuild split)...", but batch 3 (card 17) renamed that function to `ensureSelvagePaneLocked`. This is a stale identifier reference the header re-scan (card 56) should have caught, since `overlay.go` is not touched by batch 3's rename cards.
**Fix:** Update the example to `ensureSelvagePaneLocked`'s Selvage-rebuild split.

### [NIT:consistency] attach.go's AttachArgv doc still says "a lone header pane"
**Location:** `internal/reedengine/attach.go:71-73`
**Issue:** The doc comment reads "...which has nothing to pin anyway because a lone header pane takes render.Rules' sole-cell branch." Batch 4's card 26 only rewrote the two comment blocks directly above the `pinGeometryOptionsLocked`/`readStatusRowsLocked` calls inside `AttachArgv`, not this earlier paragraph, so "header pane" survives describing what is now Selvage's sole-cell branch.
**Fix:** Reword to name Selvage instead of "a lone header pane".

### [NIT:consistency] render/rules_test.go points at a test that was renamed
**Location:** `internal/reedengine/render/rules_test.go:320-322`
**Issue:** The comment says "...exactly the shape `TestHeaderNeverGetsZeroHeightLayoutCell` exists to forbid...", but card 23 (batch 3) renamed that integration test to `TestSelvageNeverGetsZeroHeightLayoutCell` (confirmed present under the new name in `contract_integration_test.go`, and correctly cross-referenced by that new name in `doc.go:367`). This one cross-reference in the `render` package's own test file was not updated in the same pass.
**Fix:** Update the comment to `TestSelvageNeverGetsZeroHeightLayoutCell`.

### [NIT:consistency] cli_test.go's no-arg listing comment undercounts the verb list
**Location:** `internal/reedcli/cli_test.go:17-19`
**Issue:** `TestRunCLI_NoArgs`'s doc comment says "lists all seven registered verbs", but `wantSubs` (and the actual `parent.AddCommand` call in `cli.go`) now register nine verbs (`up, down, add, remove, status, resume, attach, statusline, watchdog`). Card 36 added `statusline`/`watchdog` to the slice but did not touch this stale count in the surrounding prose.
**Fix:** Update "seven" to "nine".

### [NIT:consistency] docs/overview.md's tokenvocab directory-tree entry omits the worktree token
**Location:** `docs/overview.md:261`
**Issue:** The `internal/tokenvocab/` line in the `docs/overview.md` directory-tree ASCII listing still reads "shared token vocabulary (repo, hub) + Render compose over stencil, a leaf" — a fourth "repo, hub" occurrence beyond the three locations card 48 explicitly named and fixed (the reed module bullet at line 301, the tokenvocab-is-a-leaf bullet at line 408, and the package-documentation list entry at line 462, all of which correctly say "worktree" now).
**Fix:** Add "worktree" to this directory-tree line for consistency with the other three corrected mentions.

### [NIT:consistency] reed-header-selvage.md's token-count sentence still says "the header's ... two tokens"
**Location:** `manifest/designs/reed-header-selvage.md:25`
**Issue:** "The header's only two tokens today, `{{.repo}}` and `{{.hub}}` (`tokenvocab.Ctx`), are static and never need a live update." This "Implemented" design doc otherwise correctly describes three tokens (line 27 references the added `worktree` token), but this sentence still frames the pipeline as "the header's" and "today" as if two tokens is the current state, which reads as contradicting the surrounding paragraph.
**Fix:** Reword to describe the status-line's now-three static tokens, or make explicit this sentence is describing the pre-task starting point.

## Verdict

REQUEST_CHANGES — one doc.go bullet (batch 4's status-line rewrite) still describes the removed "status off" mechanism as current; the rest are minor stale-reference NITs.
MILL_REVIEW_END
