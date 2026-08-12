MILL_REVIEW_BEGIN
# Review: fabric: close the corrindex two-phase read-modify-write race (slice 15)

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: claude-opus-4-class (self-assessed; exact build unknown)
reviewed_file: _mill/discussion.md
date: 2026-08-12
```

## Findings

### [BLOCKING:decision] Roadmap edit inventory misses the Planned campaign item
**Section:** Scope / Technical context "Files to change"
**Issue:** Slice 15 is not a discrete Planned bullet — it sits inside the fabric-crucible Planned item (`manifest/roadmap.md:12-33`), whose framing goes stale on landing (`:26` "is next in the chain", `:27` "the two remaining slices", `:29`, `:30-32` "Placed ahead of `Shed`"), and Done entry `:201` says "Slices 14-15 remain — see Planned above"; the discussion names only `:33`, `:208`, `:215` plus an unlocated "Planned → Done" move.
**Fix:** State the disposition of the whole Planned campaign item (delete it outright vs. keep a residual entry naming the still-open `RebuildIndex` window) and add `:201` and the `:21-32` framing lines to the edit inventory.

### [NIT:consistency] "Five sites across four files" enumeration is inaccurate
**Section:** Scope "Out" / updatejson-adoption-stays-at-one-consumer
**Issue:** `internal/treadleengine/state.go:151`→`:172` is not a read-modify-write pair — `:151` is `TerminalOutcome`, a read-only accessor that never writes, and `:172` is `saveState`, whose read partner is `loadOrInitState` at `:110`; so treadle does not carry "two independent pairs" and the count of five is not derived reliably.
**Fix:** Re-derive the list from actual read→write pairs (or state it as an approximate inventory), since the disposition — migrate none — is unaffected either way.

### [NIT:design] `UpdateJSON`'s directory-creation contract unstated
**Section:** updatejson-signature-mirrors-readjson
**Issue:** The signature spec says nothing about `os.MkdirAll(filepath.Dir(path))`, but `ReadJSON`/`WriteJSON` both do it before acquiring the lock (`state.go:29-32`, `:53-56`) while `ReadJSONStrict` deliberately does not (`:82`) — and `flock.New(lockPath).Lock()` fails outright if the parent directory is absent.
**Fix:** State which precedent `UpdateJSON` follows, and if it MkdirAlls, that it does so before acquiring the lock.

### [NIT:design] Post-fix redundant load at `record()`'s only caller not dispositioned
**Section:** record-keeps-its-method-shape
**Issue:** The stated rationale ("the handle is still needed for `exact`/`nearestAtOrBefore`/`entries`") does not hold at `record()`'s sole production caller — `RecordCorrespondence` (`index.go:118-123`) calls `loadCorrIndex` and then uses the handle for nothing but `record`, so after the fix that load is a now-pointless read plus lock acquisition.
**Fix:** Say explicitly that the redundant load is kept (call-site churn avoided) rather than resting on a receiver-still-needed argument that is false at that one site.

## Verdict

REQUEST_CHANGES
Roadmap edit inventory is incomplete; the Planned campaign item's disposition is undecided.
MILL_REVIEW_END
