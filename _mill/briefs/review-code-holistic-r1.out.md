MILL_REVIEW_BEGIN
# Review: Diagnostic tracing (trace) on the logger module — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-08-01
```

## Findings

### [NIT] Header bytes not counted toward the durable-sink size cap
**Location:** `internal/logger/sink.go:201` (header write) vs `internal/logger/sink.go:249-258` (`writeDurable`'s `sinkBytesWritten`)
**Issue:** `ensureDurableSink` writes the header line directly via `f.WriteString(headerLine())`, outside `writeDurable`'s mutex-guarded byte counter, so the header's bytes never increment `sinkBytesWritten` — the 8 MiB cap is enforced against a count that slightly understates the file's real on-disk size.
**Fix:** Either route the header write through `writeDurable` or add `len(headerLine())` to `sinkBytesWritten` right after the header write succeeds. Cosmetic only (header is a few hundred bytes against an 8 MiB cap).

### [NIT] Opened file handle leaks if the header write fails
**Location:** `internal/logger/sink.go:195-204`
**Issue:** If `os.OpenFile` succeeds but the subsequent `f.WriteString(headerLine())` fails, the function sets `sinkOK = false` and returns without closing `f` — the file descriptor is never released for the remaining process lifetime.
**Fix:** `defer f.Close()` or an explicit `f.Close()` on the write-error path before returning.

### [NIT] Plan's Card 41 `Edits:` list omits two files it actually requires changing
**Location:** `_mill/plan/13-scoutengine-logger-conversion.md` Card 41 (`Edits:` lists only `internal/scoutengine/lspclient.go`)
**Issue:** Card 41's own requirements prose instructs setting `client.lang = lang` at three sites in `ensureserver.go` and one in `refs.go`, both of which the implementation correctly edited (verified in `internal/scoutengine/ensureserver.go:206,366,596` and `internal/scoutengine/refs.go:114`) — but the card's `Edits:` metadata lists neither file, only `Context:`. This is a plan-authoring inconsistency, not an implementation defect: both files are still named in the card (under `Context:`) and the edits match the prose exactly.
**Fix:** None needed for this round; a future plan-quality pass could move `ensureserver.go`/`refs.go` from `Context:` to `Edits:` in Card 41.

## Verdict

APPROVE
All 14 batches, cross-batch contracts, constraints, and tests verified consistent with the plan; only cosmetic NITs found.
MILL_REVIEW_END
