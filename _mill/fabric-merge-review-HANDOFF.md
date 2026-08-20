# fabric merge — crucible campaign HANDOFF (orchestrator-only, round 4+)

Off-limits to round agents: this file matches the `fabric-merge-review-*` pattern the round prompt declares unreadable.

**Last refreshed:** 2026-08-20, before round 4 spawn.

## What this task is

Mill task `fabric-merge-crucible-round4`, worktree `/home/knatte/Code/loomyard/wts/fabric-merge-crucible-round4`, branch of the same name, based on `main` (the prior campaign's task-85 worktree was merged and torn down; `_mill/fabric-merge-review-*` history lives at `archive/fabric-merge-crucible-hardening~1`).

Continuation of the campaign task 85 (`fabric-merge-crucible-hardening`) ran and stopped after round 3, closing V1 (round 3's `sideConcludeAlreadyLanded` adoption arm — silent false success on an unrelated commit) and V2 (the AST closure test scoped to `mergeerrors.go` only), then continuing the crucible loop as ordinary rounds.

Orchestrator-driven, not mill-go. The task's mill phase stays `discussing`; nothing in this campaign advances it. **The orchestrator role for this task is Claude itself (no separate human operator steering per-round)** — the user gave the round plan up front and is not steering live; still apply every Hard Rule from `crucible/orchestrator-prompt.md`, especially never trusting a round's self-verdict.

## Operator's round plan (given 2026-08-20, this task)

Exactly four rounds, model + effort fixed in advance, UNLESS convergence is reached first:

| Round | Model | Effort | Tag | Status |
|---:|---|---|---|---|
| r4 | Opus | medium | `opus-medium-r4` | seeded, about to spawn |
| r5 | Fable | high | `fable-high-r5` | pending |
| r6 | Opus | high | `opus-high-r6` | pending |
| r7 | Opus | medium | `opus-medium-r7` | pending, only if not converged by r6 |

Round numbering continues from the prior campaign's r1–r3 (this campaign's round 4 was originally planned as `opus-high-r4` and never spawned; the operator has now replaced that single-round plan with this four-round rotation instead — the numbering stays continuous, the model/effort assignment does not match the original plan).
Hard Rule 2 (explicit effort pick required before every spawn) is satisfied for all four by this table.
Convergence = a safety-pass round (self-reports nothing new) AND the orchestrator's independent gates agree — see "Convergence" below. If reached before r7, stop there and report; do not spawn the remaining rounds.

## Baseline established before round 4 (all green, committed tree at `4471041a`)

- `go build ./...`
- `go vet ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitrepo/...`
- `go test -count=1 ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitrepo/... ./cmd/lyx/...`
- `go test -tags integration -count=1 -timeout 30m ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitrepo/...` — fabricengine 28.1 s, fabriccli 2.2 s, gitrepo 1.3 s

**V1 and V2 confirmed still present in the current tree before spawning r4** (read directly, not assumed from the prior HANDOFF):
- V1: `sideConcludeAlreadyLanded` (`internal/fabricengine/mergelifecycle.go:105-121`) has no second-parent / squash-vs-non-squash discriminator — matches the prior campaign's description exactly.
- V2: `TestMergeVocabulary_GuardReasonSetMatchesConstBlock` (`internal/fabricengine/mergevocab_test.go:50`) parses `"mergeerrors.go"` as a hardcoded filename — matches exactly.

## Campaign-specific facts carried forward (do not rediscover)

- fabric's live tier is `-tags integration`, not `-tags smoke`. Zero `//go:build smoke` files under `internal/fabricengine`, `internal/fabriccli`, `internal/gitrepo`.
- Not LLM-driving. Substrate is real git repos. N-concurrent amplifier is safe but NOT required — the merge bar is single-instance correctness (deliberate scoping carried from the prior campaign).
- `./deploy-dev` is the POSIX deploy script (not `.cmd`) on this host.
- Hub recipe: `GIT_CONFIG_GLOBAL` with `[init] defaultBranch = main` BEFORE the first `git init`, bare warp + bare weft, `lyx fabric clone`. A merge source must be a fabric-managed pair (branch exists on both warp and weft).
- `internal/fabricengine/doc.go`'s "# The merge surface" section (~line 846–960) is the authoritative prose contract.
- SPEC lives only in git history: `git show 3b800bc8:_mill/discussion.md` + `3b800bc8:_mill/plan/0*.md`; rejected alternatives at `967916ea:_mill/discussion-meta.md`.
- Full round 1–3 history (findings, sabotage proofs, live-drive recipes): `git show archive/fabric-merge-crucible-hardening~1:_mill/fabric-merge-review-HANDOFF.md`.

## Round 4 (`opus-medium-r4`) — seeded, not yet spawned

Seed commit: (this file + the review prompt, committed together before spawn).
Prompt: `_mill/fabric-merge-review-prompt.md`, residual = V1 (BLOCKING) + V2 (MEDIUM), explicit acceptance bars for both, plus "review the whole merge surface as an ordinary round, not just these two" instruction and the "refusal-shaped predicate repurposed as a positive claim" lesson as its high-yield focus.

## Next action

Spawn round 4: `subagent_type: crucible-reviewer-medium`, `model: opus`, prompt = "Read `_mill/fabric-merge-review-prompt.md` and do exactly what it says.", tag `opus-medium-r4`. Wait for completion, then independently verify per the protocol in `crucible/orchestrator-prompt.md` before re-seeding round 5.
