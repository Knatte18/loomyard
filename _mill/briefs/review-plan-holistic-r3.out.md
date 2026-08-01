MILL_REVIEW_BEGIN
# Review: fabric: clone-does-everything + subpath-in-weft + init dissolution — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: claude-sonnet-4.5 (Sonnet 5, per system context)
reviewed_file: plan/
date: 2026-08-01
```

## Findings

### [BLOCKING] Batch 4's BoardDir config migration breaks TestRunCLI_EnvMapToOption
**Location:** Batch 4, Card 17 (weft_verbs.go:124 migration) / Card 19 (cli_test.go)
**Issue:** Card 17(a) migrates `weft_verbs.go:124`'s `fabricengine.LoadConfig(weftBaseDir)` to `fabricengine.LoadConfig(hubgeometry.BoardDir(l.Hub))` — this is the config-load site every weft verb (status/commit/push/pull/sync) goes through. `internal/fabriccli/cli_test.go`'s existing `TestRunCLI_EnvMapToOption` (lines 179-212) drives `push` against a `lyxtest.CopyPaired(t)` fixture with `lyxtest.SeedConfig(t, fixture.WeftPrime, {"fabric": ...})` — config seeded at `fixture.WeftPrime`, not `hubgeometry.BoardDir(fixture.Hub)`. `CopyPaired`/`buildHostHub`/`buildWeftPrime` (internal/lyxtest/lyxtest.go) never materialize a `_board` directory at all, so `BoardDir(l.Hub)` = `<Container>/_board` doesn't exist in this fixture. After Card 17's migration, `push` in normal mode would fail `LoadConfig`'s `_lyx/` existence check and the test would fail — the exact same masked-regression pattern Card 19 already fixes for `setupCLIRepo` (called out in the round-1 review), but for the *other* CLI fixture in the same file, seeding at a different location. Card 19 only updates `setupCLIRepo` (used by the topology-verb subtests); it does not touch `TestRunCLI_EnvMapToOption`'s `CopyPaired`+`SeedConfig(WeftPrime,...)` pattern. Batch 4's own `verify:` (`go test -tags integration ./internal/fabricengine/... ./internal/fabriccli/...`) would fail on this pre-existing test.
**Fix:** Add a card/requirement updating `TestRunCLI_EnvMapToOption`'s fixture to seed the fabric config at `hubgeometry.BoardDir(fixture.Hub)` (materializing a `_board` dir in the fixture, or seeding config there directly) instead of `fixture.WeftPrime`.

### [NIT] Card 14 omits the third no-cleanup CloneHub return path from its return-threading list
**Location:** Batch 4, Card 14
**Issue:** Card 14 enumerates two "early no-cleanup" return statements needing the new third return value (empty-name clone.go:60-63, hub-already-exists clone.go:69-71) but misses the third: `os.MkdirAll(hubPath, ...)`'s failure at clone.go:74-76 (`return "", err`), which also needs `return "", "", err` under the new 3-value signature.
**Fix:** Add clone.go:74-76 to the enumerated no-cleanup return paths in Card 14's requirements.

### [NIT] Card 9's stale-removal Detail string can silently overwrite the add-missing outcome's Detail
**Location:** Batch 2, Card 9
**Issue:** Stale-removal runs after add-missing/re-point in the same pair iteration and writes its own abort/removed-names text into `pr.Detail`, but the requirements don't specify how to compose this with a Detail already set by `junctionRepointedDetail` in the just-repaired case — one could silently clobber the other depending on write order.
**Fix:** Specify append (not overwrite) semantics for `pr.Detail` when both add-missing and stale-removal produce text for the same pair.

### [NIT] Card 18 doesn't call out updating clone_adopt_test.go's four pre-existing CloneHub call sites
**Location:** Batch 4, Card 18
**Issue:** `CloneHub`'s signature change (Card 14) ripples to all four existing call sites in `clone_adopt_test.go` (adopt/fresh/strict-abort/orphan tests), each of which must add a `subpath` arg and capture the new `anchor` return to keep compiling. Card 18's requirements describe only the new subtests to add, not this compile-forced update to the existing ones.
**Fix:** Note explicitly that the four existing `CloneHub(...)` call sites in this file need the new argument/return threaded through.

## Verdict

REQUEST_CHANGES
Batch 4's BoardDir config migration leaves an existing fabriccli test broken; everything else checked out.
MILL_REVIEW_END
