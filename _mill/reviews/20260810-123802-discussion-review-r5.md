MILL_REVIEW_BEGIN
# Review: fabric: one ownership-and-dirtiness gate for all destruction (slice 12)

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-10
```

## Findings

### [BLOCKING:design] Absent target: idempotent teardown becomes hard failure
**Section:** `ownership-is-a-closed-enum` + `a-refusal-is-never-best-effort` (predicate table; `fslink.Remove` disposition table)
**Issue:** Several gated sites are documented as idempotent no-ops when the target is already gone — `removePortal` ("Returns nil if the link does not exist", `portals.go:50-51`), `removeJunctionRecords` ("Returns nil if empty or all absent", `weftwiring.go:149-152`), `removeLaunchers` (`os.IsNotExist` tolerated, `launchers.go:165,170`) — yet `ownedWiredJunction` is defined as "`target` is a link, and it resolves to `expectedTarget`" and `ownedRegisteredLinkedWorktree` as `List` membership, both of which an absent path fails; combined with "a refusal is never discardable", `Remove`'s `_ = removePortal(...)`/`_ = removeLaunchers(...)` (`remove.go:57-58`) and its documented tolerance of an already-absent weft worktree (`remove.go:99-110`) would start hard-failing on paths that succeed today.
**Fix:** State the gate's absent-target rule explicitly — e.g. an absent target is a no-op success, not a refusal, for every ownership kind — and say so in the disposition tables, since the Out section forbids behaviour changes beyond the three named gaps.

### [BLOCKING:design] `ownedTransactionCreatedBranch` is the trust-me the doc rejects
**Section:** `ownership-is-a-closed-enum` predicate table; `branch-deletion-is-ref-shaped`
**Issue:** The document's own rule is "if a kind cannot state what it verifies, it is a trust-me and does not belong in the enum", and it upgraded `ownedFreshlyCreatedPath` from a free-text reason to a gate-minted `createdToken` for exactly this reason — but `ownedTransactionCreatedBranch`'s stated predicate is "the branch was created earlier in this same invocation", which the gate cannot verify and no token backs; it is also spelled `ownedTransactionCreatedBranch(reason)` at one place and bare at three others.
**Fix:** Either give ref creation the same gate-minted token treatment (a `createdBranchToken` from a gate-owned branch-creation call at `add.go`/`checkout.go`) or state a verifiable ref predicate, and fix the signature to one spelling.

### [BLOCKING:design] `container` is a required field declared at only two sites
**Section:** "The five primitives and their current sites" tables; `gate-call-shape`
**Issue:** `pathRequest` requires `container`, and rounds 3 and 4 added ownership and dirtiness columns precisely because a required field left blank is left to the implementer — but no table has a container column; only `teardownHub` (operator-named `cwd`) and `resetHardTo` (`l.HubPath`) state one, leaving `clone.go:569` (`resetHub`, whose signature `resetHub(hubPath string)` carries no `cwd`), `remove.go:197`, `prune.go:276`, and the six link sites unspecified — and the doc itself warns that containment "proves nothing if the caller chose the container".
**Fix:** Add a container column to every gated row (and say `resetHub` must take `cwd` from `CloneHub`), so containment is declared per site like ownership and dirtiness.

### [NIT:consistency] `primaryWeftBranch` line citations are off
**Section:** `branch-deletion-is-ref-shaped`; "Ingredients that already exist"
**Issue:** `primaryWeftBranch` is at `cleanup.go:206`, not `~190`, and its fail-closed message is at `cleanup.go:210-212`, cited as `200-211`.
**Fix:** Correct both citations, as round 4 did for `clone.go:212`/`:220`.

### [NIT:scope] Two small enumeration slips
**Section:** `ownership-is-a-closed-enum`; `bypass-guard-shape-and-home`
**Issue:** `ownedUnderGeometryRoot`'s closed root set `{portalsDir(l), launchersDir(l)}` includes `portalsDir`, which no listed site uses (`portals.go:57` declares `ownedWiredJunction`); and the guard token `warp.ResetHard(` is evaded by aliasing the field to a local (`r := f.warp; r.ResetHard(...)`), which the blind-spot sentence does not mention.
**Fix:** Drop or justify the unused root-set member, and add the aliasing case to the invariant's blind-spot sentence.

## Verdict

REQUEST_CHANGES
Absent-target semantics, an unverifiable ref ownership kind, and undeclared containment remain open.
MILL_REVIEW_END
