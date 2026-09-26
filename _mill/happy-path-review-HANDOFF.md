# happy-path crucible — orchestrator handoff

Campaign question: does a normal, real task get from start to landed with the deployed `lyx`, under the `go` and `llm` drivers?
Scope was one review+fix round plus the orchestrator's own verification; the campaign stops there.

## State

- Round `opus-medium-r1` (Opus, medium) is complete; its fixes are commits `242d46983`..`ff6e654ba` on `crucible-happy-path`, not pushed.
- Orchestrator verification is on this branch's HEAD (see `.scratch/result-crucible-happy-path.md` for the run log, which is gitignored and only this session's report).

## CLOSED-AND-VERIFIED

- F1 (`242d46983`): weft primary and `_board` follow the warp prime's branch — sabotage-proven (`TestCloneHub_EmptyWeftRemoteWithForeignDefaultBranch`), and live on a fresh hub whose weft bare had a `master` HEAD.
- F3 (`5550dd00d`): Finalize pushes the parent — sabotage-proven (`TestFinalize_PushesParentAfterMerge`), and live: the go run's squash reached the bare warp's `main`.
- F6 (`f51cb430f`): config lists are set and reconciled whole — sabotage-proven (`TestReconcile_CarriesListsWhole`), and live: `lyx config landing --set 'require_pr_to_base=[]'` worked.
- F5 (`c4f0b1a39`, `3d59b14c6`), F8 (`655bcb6be`), F10 (`ff6e654ba`): `ly-drive` skill text; F5 verified live (llm driver passed Preflight, step envelopes under `mktemp -d`).
- F4 partial (`efc1f7053`, `d4f6a83d8`), F9 (`761dc64a5`): help text and driver prompt.

## Residual — seed for a next round, only if the operator starts one

- F4 remainder: `lyx` cannot make `ly-drive` available to a driver session without the `ly` plugin installed.
  `ly@loomyard` is not installed on this machine; the operator must `/plugin install ly@loomyard` before a real llm-driven run, and the marketplace copy carries `main`'s stale skill until this branch lands.
  Bundling the skill into the binary needs a decision against the Stencil Ownership Invariant.
- F7 (LOW, NOT-FIXED-THIS-ROUND): a Stuck row with no `on_stuck` reports only `stuck with no OnStuck target`; the real cause is in the trace alone. Needs a `ShedProducer` seam change.

## Issues filed

- #275 — a landed loom run leaves its board task open.
- The round's other out-of-scope observation (`fabric add` rollback strands the warp branch) is already #269.

## Next action

None inside this campaign: report to the operator and wait. Push/merge is the operator's call.
