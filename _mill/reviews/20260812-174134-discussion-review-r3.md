MILL_REVIEW_BEGIN
# Review: lyxtest: build real fabric hubs, invert the lyxtest/fabric dependency

```yaml
duration_s: 199.0
verdict: APPROVE
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-12
```

## Findings

### [NIT:consistency] `CopyRepo` caller set stated two ways
**Demoted-from:** BLOCKING
**Section:** "Migrate all 132 above-fabric sites" **Issue:** The Decision bullet says the allowlist is `internal/lyxcwd` **alone** and that listing `internal/gitrepo` "would pre-authorise exactly the drift this task forbids", while the Rationale bullet two lines later says "A guard test pins `CopyRepo`'s caller set to `internal/lyxcwd` and `internal/gitrepo`"; Testing says `lyxcwd` alone, and the already-applied CONSTRAINTS gitkit invariant says the primitive fixtures "serve `internal/gitrepo` and `internal/lyxcwd` only". **Fix:** State one caller set for the `CopyRepo` guard test and reconcile the CONSTRAINTS sentence with it, distinguishing the `CopyRepo` allowlist from the broader `MustRun`/`SeedConfig` consumer set (`gitrepo` has 18× `MustRun`, zero `Copy*`).

### [NIT:decision] Build-tag disposition of the merged `hubforge` files unstated
**Section:** Scope / Testing **Issue:** `hubforge` merges an untagged source (`internal/lyxtest/lyxtest.go`) with an integration-tagged one (`internal/fabricengine/fabrictest/hub.go:1` is `//go:build integration`, as is every fabrictest file except `doc.go`), and the discussion never says which tag the new package's production and test files carry — tagging all of them would leave `internal/hubforge` with zero files in the untagged build that the stated `go vet ./...` gate runs. **Fix:** Name the tag disposition explicitly (e.g. production untagged, hubforge's own git-spawning tests `//go:build integration` per the Test Tier Purity Invariant, one untagged `doc.go`).

### [NIT:decision] `SeedFabricConfig` commit behaviour left open
**Section:** "Config seeding on a real hub", point 3 **Issue:** Case 2 says `hubforge.SeedConfig` writes into `PrimeWeft()` "and commits there", but case 3 only says repo-wide fabric config "goes to `res.BoardDir`" — silent on whether it commits; `BoardDir` is the `weft:main` checkout that `fabricengine.Bolt` commits and that the destruction gate's dirtiness check observes, so an uncommitted seed can change verb outcomes in fabric's own live-state cells. **Fix:** State whether `SeedFabricConfig` commits in `BoardDir` and, if not, why leaving the board dirty is safe for the live-state matrix.

## Verdict

APPROVE
One self-contradiction on the `CopyRepo` caller allowlist; otherwise well grounded and verifiable.
MILL_REVIEW_END
