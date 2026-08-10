MILL_REVIEW_BEGIN
# Review: fabric: one ownership-and-dirtiness gate for all destruction (slice 12)

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude Opus-class model (env reports claude-opus-5); self-assessment only
reviewed_file: _mill/discussion.md
date: 2026-08-10
```

## Findings

### [BLOCKING:design] Link kinds carry no wired-name-set input
**Section:** `ownership-is-a-closed-enum` (two link-shaped kinds)
**Issue:** The stated rule is "the path is a link **and** its location is one fabric wires", but `ownedWiredJunction(expectedTarget)` gets only a target and `ownedDriftedWiredJunction` takes no parameters at all — under the document's own "each check's inputs travel with the check" rule neither kind can evaluate the wired-location half, so `ownedDriftedWiredJunction` degenerates to bare link-ness, exactly the R1 rule the decision rejects (verified: `junction.go:146-161` derives `link` from the loop's name-set, which the kind never receives).
**Fix:** Give both link kinds the wired-name-set (or the wired link path set) as an explicit parameter, and state which helper produces it at each of the six sites.

### [BLOCKING:design] `createExclusiveDir` collapses two sequences the network probe separates
**Section:** `rollback-paths-go-through-the-gate` (transaction identity)
**Issue:** The stat guards (`clone.go:163`, `:212`) and `os.MkdirAll(hubPath)` (`:220`) are not adjacent — `probeWeftBinding` and the bootstrap guard run between them, and `clone.go:208-214` documents the offline-before-network ordering as deliberate; the document says `createExclusiveDir` replaces all three but never says where the single call lands, and both placements change behaviour (create-early leaks a residual hub when the probe fails, since that path returns without teardown; create-late defers "hub already exists" past a network call).
**Fix:** State the call's position, which of the two existing error messages it preserves, and how the offline "hub already exists" refusal stays offline.

### [BLOCKING:scope] Disposition tables declare ownership but not dirtiness
**Section:** "The five primitives and their current sites"
**Issue:** Dirtiness is a required, caller-declared field ("an omitted check is a compile error"), yet only four sites get an explicit member (remove fallback, `resetHardTo`, `branch -D`, the rollback N/As); `launchers.go:165,170`, `prune.go:258,276`, `clone.go:569`, `remove.go:177`, `weftwiring.go:175` and the four `removeLink` rows leave the implementer to guess — the same gap round 3 closed for the ownership column.
**Fix:** Add a dirtiness column to every gated row, naming the member and, for `dirtinessNA`, the reason string.

### [BLOCKING:design] `ownedHubGeometryChild` has no stated predicate
**Section:** `ownership-is-a-closed-enum` / launchers rows
**Issue:** Every other kind names the predicate the gate runs (`isRegisteredLinkedWorktreeIn`, `looksLikeHub`, `List` membership, token equality) but this one is only ever named, and it is the kind mapped to `launchers.go:165`, whose targets are script files two levels below the declared container `launchersDir(l)` — so "child" is not even literally true there.
**Fix:** Define what `ownedHubGeometryChild(container)` verifies, and whether it admits non-direct descendants and non-directory targets.

### [NIT:consistency] `removeDir` executor used for file removals
**Section:** `gate-call-shape` executors
**Issue:** `launchers.go:165` removes `ide`/`fabric-checkout` script files, and `os.Remove(` is a banned token, so those calls must route through an executor named `removeDir`.
**Fix:** Rename to `removePath`, or add a file-shaped executor.

### [NIT:consistency] Line references drift by one in the clone guard citations
**Section:** `rollback-paths-go-through-the-gate`
**Issue:** The one-argument stat guard is `clone.go:212` and `os.MkdirAll(hubPath, 0o755)` is `:220`, cited as `:211` and `:219`.
**Fix:** Correct both, in all places they appear.

## Verdict

REQUEST_CHANGES
Four unresolved design/scope gaps: link-kind inputs, clone creation placement, dirtiness column, undefined ownership kind.
MILL_REVIEW_END
