MILL_REVIEW_BEGIN
# Review: reed: attach's layout computation scales header pane height with terminal height

```yaml
duration_s: 85.0
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Opus-class), Anthropic
reviewed_file: _mill/discussion.md
date: 2026-08-28
```

## Findings

### [BLOCKING:design] Hook body shape contradicts set-hook's one-arg form
**Section:** Technical context › Gotchas; Testing (`internal/reedengine` untagged)
**Issue:** The discussion mandates building the multi-`resize-pane` hook "as an argv, never by string-concatenating shell", with a literal `";"` argv element — but `set-hook -w -t <target> window-resized <body>` takes the body as a single argument, so a bare `";"` element there terminates the `set-hook` command itself rather than separating hook commands; the `chainedAttachArgv` precedent (`attach.go:31-48`) works only because tmux parses the top-level argv as a command sequence, which is not the case inside a hook value. Only the single-command form (`window-resized hook + resize-pane -y 1`) appears in the verified-behaviour table.
**Fix:** Decide and state how a multi-pin hook body is encoded (one concatenated command-list string in a single argv element, or per-pin `set-hook -a`, which the snapshot decision currently forbids), and note whether that form was verified live.

### [NIT:consistency] Strip pins use raw CollapsedStripRows, not clamped
**Demoted-from:** BLOCKING
**Section:** Decisions › `pins-come-from-render-policy-not-raw-config`; Scope › In
**Issue:** The decision pins the header at its post-`clampHeaderHeight` value but strips at `p.CollapsedStripRows`; `render/height.go`'s `clampToFit` reclaims strip rows first, down to 1, so on a short window a strip's actual rendered height is below `CollapsedStripRows` and a raw pin would contradict render policy — the exact hazard the decision cites as its rationale for the header. The stated pin source is therefore not "the heights `Rules` actually computed".
**Fix:** State that both pins come from the post-`clampToFit` placement heights, and reconcile the Scope bullet's "at `collapsed_strip_rows`" wording with it.

### [BLOCKING:design] Install point inside AttachArgv's degrade ladder undefined
**Section:** Scope › In; Decisions › `pins-are-a-snapshot-refreshed-at-every-apply`
**Issue:** The discussion names one early return per site, but `attach.go:62-146` degrades to the bare argv at seven points (`cols/rows<=0`, `requireSessionLocked`, `readWindowSizeLatestLocked`, `readStatusRowsLocked`, state load, `listPanes`, `len(live)<2`, `anyPlacedStrand`, `planLayout` error) and `apply.go:141-147` has a second guard (`anyPlacedStrand`) that is never mentioned; where the `set-hook` sits relative to these is unspecified. This matters because the decision claims the hook "covers the bare/degraded attach path", while an install placed after `planLayout` would not, and the integration case asserting `AttachArgv(0, 0)` holds the header at budget silently depends on a hook installed by an earlier apply.
**Fix:** Name the exact statement the install follows in each site, and restate which degrade paths therefore carry no hook and why that is acceptable.

### [BLOCKING:design] No disposition for hook failure at fire time
**Section:** Decisions › `hook-failure-is-non-fatal-everywhere`
**Issue:** The decision covers only a failing `set-hook` install; it says nothing about the installed hook failing when it fires — a pinned `%N` can name a pane destroyed between applies (reconcile kills dead panes, and a hook is a snapshot by design), and a `resize-pane` against a gone pane may abort the rest of the tmux command list, silently dropping the header pin that follows it.
**Fix:** State the disposition — pin ordering (header first), a per-command tolerance mechanism, or an explicit accepted-and-self-healing limitation with the reasoning.

### [NIT:consistency] Staleness limitation understates what can go stale
**Section:** Decisions › `pins-are-a-snapshot-refreshed-at-every-apply`
**Issue:** "The only value that could go stale is a *clamped* header height" is false if strip pins are clamp-derived (finding 2), and the same shrink-below-clamp window applies to them.
**Fix:** Broaden the limitation to any clamp-derived pin once the pin source is settled.

## Verdict

REQUEST_CHANGES
Hook encoding, strip pin source, install point, and fire-time failure all unresolved.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 3._
MILL_REVIEW_END
