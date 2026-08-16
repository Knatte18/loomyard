MILL_REVIEW_BEGIN
# Review: Shed engine adapters: SingleLLMProducer, perch, Webster

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-16
```

## Findings

### [BLOCKING:design] TerminalOutcome probe needs an existing scratch dir
**Section:** "Perch run identity — a run-id that advances only past a terminal block"
**Issue:** `perchengine.TerminalOutcome` → `treadleengine.TerminalOutcome` → `state.ReadJSON(path, <scratchDir>/state.json.lock)`, and `lock.AcquireReadLock` opens the lock file without creating its parent, so the probe errors outright when `scratchDir` does not exist; the shipped `perchcli/pause.go:72-84` `os.MkdirAll`s it first with a comment saying exactly that. Because `runDirBase` is `_lyx` (tracked/fabric-synced) and `scratchDirBase` is `.lyx` (never tracked), a `<prefix>-<N>` run dir with no scratch sibling is a normal state after a clone or on another machine — and the discussion's stated error disposition ("a `TerminalOutcome` probe error … propagates as the adapter's own error, failing the `Call`") turns that normal state into a permanent producer failure.
**Fix:** state in the run-identity Decision that the adapter creates `filepath.Join(scratchDirBase, runID)` before probing (mirroring `perchcli`), and scope the propagate-the-error rule to a genuinely unreadable/corrupt `state.json`.

### [NIT:scope] Testing section omits the missing-scratch-dir row
**Section:** "Testing → `PerchProducer`"
**Issue:** The run-id advancement rows seed only `<prefix>-N` run dirs and their `state.json`; no row covers a run dir whose scratch sibling is absent — the exact case the finding above describes, and the one a `t.TempDir()` fixture reproduces by default unless the test deliberately creates it.
**Fix:** add a row asserting a `<prefix>-N` run dir with no scratch sibling resolves normally (probe succeeds, non-terminal) rather than failing the `Call`.

### [NIT:decision] state.json seeding format for the perch tests
**Section:** "Testing → `PerchProducer`"
**Issue:** `treadleengine`'s `runState` is unexported and written via `state.WriteJSON`, so the "seed a `<prefix>-1` run dir whose `state.json` records a terminal `Outcome`" rows have to hand-write JSON against a private schema; the discussion does not say so, leaving a plan writer to discover it.
**Fix:** name the seeding mechanism explicitly (hand-written `{"outcome": …}` JSON at `<runDir>/state.json`) so the tier-1 fixture is unambiguous.

## Verdict

REQUEST_CHANGES
Perch run-identity probe fails when the never-tracked scratch dir is absent.
MILL_REVIEW_END
