# fabric — independent review, round 4 (`fable-high-r4`)

Reviewer: crucible round agent `fable-high-r4` (Fable 5, high effort).
Worktree: `/home/knatte/Code/loomyard/wts/fabric-crucible-hardening`, branch `fabric-crucible-hardening`.
Round context: FINAL round of a fixed 4-round campaign — broad whole-module sweep plus the four carried-forward items from round 3's independent verification;
the destruction chokepoint's containment/TOCTOU property is spot-check only (closed-and-verified in round 3).

## Executive summary

(to be written after the findings list is complete)

## Scope assessment (plan vs shipped)

(in progress)

## Code findings (severity-ranked)

(provisional findings appended below as formed; final ordering at the end of Job 1)

## Docs & operability findings

(in progress)

## Provisional findings (working list, unordered)

- **P1 (`reconcile.go` `applyStaleRemoval`, ~line 813-833):** a stale-junction removal that fails with an OPERATIONAL error (non-refusal — e.g. `fslink`/OS failure inside `removeLink`) still lands the name in `removed` and is reported in Detail as "stale junction(s) removed: <name>" — only a `*destructiveRefusal` is filtered out. Additionally, when EVERY stale name is refused, `removed` is empty yet the code still appends "stale junction(s) removed: " (empty list) and still flips `Action` from `already_healthy` to `stale_removed` — a convergence report for a pass that converged nothing. PLAUSIBLE (traced, not yet driven live). Severity: LOW.
- **P2 (`add.go` Add, lines 78-81/99-101 vs 125/147):** create-side TOCTOU shape (carried item 3): `os.Stat(target)` absent-check runs at one instant, `git worktree add` acts later; a symlink planted at `target` (or `weftTarget`) in that window may direct git's worktree materialisation through the link. Needs live verification of git's own behavior at a symlinked add target. UNCONFIRMED — to test.

- **P3 (`add.go` rollbackAdd step 5 + `destroy.go` resolveManagedBranch):** under the DEFAULT config (`template.yaml` `branch_prefix` = empty), the warp branch Add creates is the bare slug — no `-weft` suffix, no prefix — so `ownedManagedBranch(l, "")`'s predicate (`WeftWarpSlug` false AND prefix test inapplicable) structurally refuses it, and rollbackAdd's step-5 `deleteBranch` can NEVER delete the warp branch this same Add call created. Every Add failure after warp-worktree creation (weft failure, portal failure, push failure) silently leaves the branch behind (all callers discard rollbackAdd's return), and a retry of `lyx fabric add <slug>` is then refused with the manual `git branch -D` remedy. Also semantically odd: the branch request's checked-out dirtiness check consults `listWeftBranches` (WEFT branches) for a WARP branch — vacuously passing. Traced; to CONFIRM live by forcing a post-creation Add failure. Severity: LOW (operability/rollback-contract, self-describing remedy on retry).

## What was tested

- `go build ./...` + `go vet` (4 fabric packages): PASS (background task bhxxfja79, exit 0).
- Hermetic `go test ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitexec/... ./internal/gitrepo/... ./cmd/lyx/... -count=5`: PASS, all 5 packages ok (task btq2urt4t, exit 0).

Reading completed so far: `doc.go` (full), `CONSTRAINTS.md` (full), `destroy.go`, `ancestors.go`, `add.go`, `remove.go`, `prune.go`, `cleanup.go`, `reconcile.go`, `junction.go`, `weftwiring.go`.
