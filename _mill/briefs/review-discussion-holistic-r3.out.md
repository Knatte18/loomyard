MILL_REVIEW_BEGIN
# Review: scoutengine told-geometry (optional uniformity pass)

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Opus-class), exact build unknown
reviewed_file: /home/knatte/Code/loomyard/wts/scout-told-geometry/_mill/discussion.md
date: 2026-08-18
```

## Findings

### [BLOCKING:design] Hub-test registry assertion is unsatisfiable
**Section:** Testing → "the hub-mode branch, in a new `cli_integration_test.go`", case 1
**Issue:** Case 1 requires asserting "the registry is the loaded overlay rather than `BuiltinRegistry()`, pinning that `LoadRegistry` still anchors where it did", but `internal/scoutengine/load.go:25-30` returns `builtins()` when `servers.yaml` is absent, and a fresh `hubforge.NewHub` fixture has no `servers.yaml` — the loaded value is byte-identical to `BuiltinRegistry()`, so the assertion cannot be written as specified and would pin nothing about anchoring.
**Fix:** Either state that the test first seeds a distinguishing overlay (`hubforge.SeedConfig(t, h, map[string]string{"servers": …})`, which writes at `h.WeftBase` so `configengine.ConfigFile(AnchorPath(), "servers")` reads it) and then asserts the seeded entry is present, or drop the registry half of case 1 and keep only the discriminating anchor-root assertion.

### [NIT:consistency] `filepath.Clean` equivalence stated without its precondition
**Section:** Decisions → "the `filepath.Abs` error fallback is preserved byte-for-byte"
**Issue:** `filepath.Join(filepath.Dir(d), filepath.Base(d), ".")` equals `filepath.Clean(d)` only for an already-cleaned `d`; for a trailing-separator input (`"foo/"`) the old synthesis yields `foo/foo` while `filepath.Clean` yields `foo`. Unreachable from production (all four call sites pass a `Clean`/`Join`/`Getwd` result), but reachable from a direct unit test of `lookupContext`.
**Fix:** State the precondition ("for the already-defaulted, cleaned `dir` every call site passes") so the byte-for-byte claim is not quotable as unconditional.

## Verdict

REQUEST_CHANGES
One named test assertion is impossible against the real `LoadRegistry` absent-file fallback.
MILL_REVIEW_END
