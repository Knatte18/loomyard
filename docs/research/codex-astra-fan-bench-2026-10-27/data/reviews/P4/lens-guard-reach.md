- **Card 1 — hostname validation:** reachable through an empty hostname or `.local`.
- **Cards 2–3 — effective-owner branches and payload refusal:** reachable through board reads, upsert, batch upsert and merge.
- **Card 3 — setter guards:** task takeover, failed pull/push and lost-write branches are reachable; the equivalent prime-owner write remains unguarded.
- **Cards 5–6 — reconcile and push-retry owner guards:** reachable; replay also occurs through an unguarded pull path.
- **Card 7 — owner-label coverage and duplicate-pause suppression:** reachable with live slug pause entries.
- **Card 8 — start, resume, step, window and claim guards:** reachable after their existing status and pause checks.
- **Card 9 — existing-run owner refusal:** reachable when status exists and `refuseCovered` passes; absent-status and nil-seam bypasses are reachable.
- **Cards 10–11 — sleep/hold-end checks, spawn refusals, owner holds and Finalize lock exceptions:** reachable through the wait loop and the four existing spawn sites.
- **Card 12 — run/parent ownership halts and repair branches:** reachable; the fully offline Finalize halt and past-merge continuation do **not** reach the proposed checks.
- **Card 13 — wake and orch owner-pause branches:** reachable after successful reconciliation; failure and unwired branches are reachable.
- **Cards 14–15 — status-error and window branches:** reachable through status reads and resumes past Open-Window.
- **Card 16 — adoption validation, local-commit/live-pair stops, write failures and selective resumes:** reachable through the prescribed ordered steps.

1. **MEDIUM · consistency · `_lyx/plan/05-reconcile-owner-outcome.md:42` — Pull can bypass the guarded replay retry.**  
   An old machine can run `lyx fabric pull` with divergent weft commits: `Fabric.Pull` replays those commits onto origin’s tip locally. Its subsequent push is then a fast-forward and succeeds without entering `retryWeftPush`, so the owner guard never runs. The card’s claim that every subsequent push meets the guarded retry is false.  
   **Evidence:** `internal/fabriccli/weft_verbs.go:349`; `internal/fabricengine/pull.go:379`; `internal/fabricengine/pushclassified.go:118` and `:125`.

2. **MEDIUM · decision · `_lyx/plan/03-board-owner-writes.md:47` — The non-takeover guard does not protect the prime-owner write.**  
   A batten resume can read a stale local Bolt with no prime owner and request a first-act claim. `SetOwners` then pulls an existing remote prime owner and, as prescribed, overwrites it because `Prime` is set. The guard covers slugs only. This write is based on the newly pulled tip, so origin-wins replay need not occur, and the post-push read reports success. This requires no simultaneous drivers.  
   **Evidence:** `_lyx/plan/03-board-owner-writes.md:47–48`; `_lyx/plan/08-batten-owner-check.md:50`; `internal/fabricengine/bolt.go:145` and `:154`.

3. **MEDIUM · consistency · `_lyx/plan/12-finalize-owner-checks.md:42` — An upstream parent fetch prevents the prescribed offline Finalize branches.**  
   With origin already unreachable and pushing enabled, `Finalize.Call` reaches `holdParent` before the new `ownerChecks`. That guard calls `ReadParentTips(true)`, whose failed fetch returns immediately. Neither the owner-prefixed unreachable halt nor the past-merge continuation to the parent push is reached. A records-only outage can reach the new branch, but the fully offline case cannot; a fake unreachable reconcile report misses this upstream interception.  
   **Evidence:** `_lyx/plan/12-finalize-owner-checks.md:42–43` and `:50`; `internal/landingshed/finalize.go:204`; `internal/landingshed/stuck.go:115`; `internal/landingshed/parentguard.go:126`; `internal/fabricengine/parenttips.go:45–47`.