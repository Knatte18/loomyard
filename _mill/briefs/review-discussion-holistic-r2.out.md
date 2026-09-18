MILL_REVIEW_BEGIN
# Review: reed: AddStrand and attach self-heal a cold worktree

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: claude-opus-5
reviewed_file: _mill/discussion.md
date: 2026-09-18
```

## Findings

### [BLOCKING:design] Unreadable reed.json boots a session as residue
**Section:** `config-errors-still-precede-the-boot` / `foreign-session-refusal-preserved`
**Issue:** `refuseRecordedForeignSessionBeforeBootLocked` (`generation.go:171-175`) returns nil when `LoadState` errors, so on a cold worktree with a corrupt `reed.json` the self-heal spawns a tmux server and only then fails in `loadOrInitStateLocked` (`spawn.go:196-200`) — today `requireSessionLocked` refuses pre-boot with `noSessionMessage(0,false)`'s "state could not be read" text and spawns nothing, contradicting this decision's own "a rejected call must never boot a session as residue".
**Fix:** State the disposition for the unreadable-state branch explicitly — either accept the boot-then-fail (matching `lyx reed up` today) and say so, or require a pre-boot readability refusal — and pin it with a test.

### [BLOCKING:consistency] Attach site cannot see `booted` under the stated constraints
**Section:** `selfheal-observability` + Scope "Out"
**Issue:** Three statements cannot all hold: `upLocked` returns `booted` with exported `Up()` discarding it, `UpResult`'s shape stays stable, and "no new exported engine API beyond what already exists" — yet the attach self-heal site lives in `internal/reedcli/attach.go` and can only call exported `Up()`, so it has no boot signal to guard its `Info` log on.
**Fix:** Decide the exported seam for the CLI site (a `UpResult.Booted` field, a new exported method, or logging at that site unconditionally/not at all) instead of leaving the signature to mill-plan.

### [BLOCKING:design] Observability rationale rests on a false premise
**Section:** `selfheal-observability`
**Issue:** The rationale claims Live-Substrate Spawn Observability is newly at risk, but `lifecycle.go:331` already logs `"reed: spawned tmux server"` at `Info` inside the boot path reached by `ensureServerAndSessionLocked`, so the invariant is satisfied without any new log — the whole `upLocked` signature change is motivated by a requirement that is already met.
**Fix:** Re-ground the decision on operator legibility ("add joined vs booted") rather than invariant compliance, or drop the flag and the signature change.

### [BLOCKING:design] Warm attach becomes a state-writing, layout-applying verb
**Section:** `attach-heals-at-the-cli-preflight`
**Issue:** The cost is priced as "one extra has-session round trip and a layout re-apply", but the change also makes every warm `reed attach` write `reed.json` via `upLocked`'s `reconcileApplyPersistLocked` and apply a layout immediately before `AttachArgv` (`attach.go:74`) computes and chains a second, client-sized layout — with attached clients present and `select-layout` able to reap panes absent from its string (`doc.go`'s reap grammar), that double apply is an unstated behavioural change.
**Fix:** State the decision for the warm-attach path: whether the pre-flight apply is acceptable alongside the chained apply, and what a test pins about panes/clients surviving it.

### [NIT:decision] `drive.go:88`'s own `reed.Up()` has no stated disposition
**Section:** `redundant-up-sites-kept` / Scope "Out"
**Issue:** The inventory of now-redundant `Up()` sites names `sharedbootstrap.go:185` and the tasks.json row, but `internal/loomcli/drive.go:88` is a third `c.reed.Up()` call of the same shape and is never mentioned.
**Fix:** Name it in the "Out" list with the same "redundant is not wrong" disposition.

## Verdict

REQUEST_CHANGES
Four blocking gaps: residue on corrupt state, unreachable boot flag, false-premise observability rationale, undecided warm-attach writes.
MILL_REVIEW_END
