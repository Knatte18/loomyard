# batten — fixer report (round 4, tag `opus-medium-r4`)

Companion to `_mill/batten-review-opus-medium-r4.md`. Every finding recorded in that review is fixed here; nothing is deferred as "low priority".

## Counts

| Severity | Found | Fixed | Deferred |
| --- | --- | --- | --- |
| BLOCKING | 2 (F5, F6) | 2 | 0 |
| MEDIUM | 1 (F1) | 1 | 0 |
| LOW | 1 (F2) | 1 | 0 |
| NIT | 2 (F3, F4) | 2 | 0 |

## What was implemented

### F1 (MEDIUM) — `Worktree-Create` idempotent against an already-present task worktree

Commit `d7961d493`.

- `internal/battencli/wire.go`: new `taskWorktreePresent(prime, slug) (bool, error)`, called first inside the `CreateWorktree` closure. A worktree already on disk and resolvable satisfies the row's post-condition, so the closure logs it at Info and returns nil; a genuinely absent one still reaches `Topology.Add`, so every real create failure keeps its `Stuck` disposition. An absent worktree answers `false, nil`, never an error; a stat error that is not "absent" and a path that does not resolve as a worktree are both returned, because neither answers the question.
- `internal/battencli/wire_test.go`: `TestTaskWorktreePresent_AbsentIsAnAnswerNotAnError` — untagged, spawns no git.
- `internal/battencli/lifecycle_integration_test.go`: `TestBattenIntegration_CreateRow_IsIdempotentAgainstAnAlreadyPresentWorktree` reconstructs the exact killed-drive state (worktree created via `hubforge.AddPair`, status still naming the create row) and asserts `Outcome == done` and `Next == "Seed-Child"`.
- `docs/overview.md`: the batten entry states the idempotency and why.
- `tools/sandbox/SANDBOX-FABRIC-SUITE.md`: F22 gains the scenario (delete prime's `status.json` with the worktree present; the step must advance, not block).

**Proven to catch the bug:** with `taskWorktreePresent` stubbed to `return false, nil`, the new integration test fails with exactly the observed production symptom —
`Outcome = "stuck"; want "done"`, `Next = "Worktree-Create"; want "Seed-Child"`, and the producer log line `branch "batten-create-idempotent" already exists; switch a pair onto it with "lyx fabric checkout ..." or delete it first with "git branch -D ..."`. The stub was then reverted.

### F5 (BLOCKING) — the Driver Choice Single-Site tripwire was red at HEAD

Commit `e839b1e8f`.

- `internal/loomcli/bootstrap_test.go`: new `driverFieldReadCarveOuts` map with exactly one entry, `internal/battencli/arm.go`, and the justification written next to it. The scan consults it alongside the existing `internal/loomcli/` prefix.
- `CONSTRAINTS.md`: the Driver Choice Single-Site Invariant's "no code path gates a refusal on it" bullet is corrected to "gates *behaviour* on it", with the one permitted shape spelled out (a CLI verb comparing a **just-typed** value against the recorded one and refusing on the envelope), and its opposite explicitly still barred (a refusal decided by the recorded value alone). The permitted-consumers bullet, which asserted `arm.go` does not read the field, now says it appears twice over and why. The enforcement bullet names the carve-out map.

No production behaviour changed: `refuseAdoptedSeed` was left doing exactly what it did, because deleting it would restore the defect an earlier round added it for.

### F6 (BLOCKING) — the Fabric Vocabulary tripwire was red at HEAD, and two invariants contradicted each other

Commit `18510bbad`.

- `internal/fabricengine/fabric.go`: new `RequireDrivableWorktree(l)`, `RequireWarpWorktree`'s vocabulary-neutral spelling. Purely a forwarding wrapper, no behaviour of its own. It follows the pattern fabric already established for this exact problem (`CommitAnchoredPaths`, `PushAnchored`, `Fabric.PushBranch`).
- `internal/fabricengine/doc.go`: the neutral-entry-point paragraph names it alongside `PushAnchored` and `MergeStateActive`, with the reason a non-owner cannot say the original name at all.
- `internal/battencli/arm.go`: calls the neutral spelling; the sibling-prime comment and both `has already completed` refusal strings reworded to pair-neutral wording.
- `internal/battencli/cli.go`, `internal/battencli/refusal.go`: comments and the group `Long` text reworded.
- `CONSTRAINTS.md`: the Batten Bookend Invariant names the spelling batten is actually allowed to use.

This is additive to fabric and changes no fabric behaviour, so it stays clear of the round's "fabric correctness is out of scope" boundary while being the only way to satisfy both invariants at once.

### F3 (NIT) — a typed `--driver llm` reported as an impossibility on a seeded run too

Commit `c06c7b9f7`.

- `internal/battencli/arm.go`: `armSeed` validates a flag the operator actually typed (`driverFlagSet` / `childDriverFlagSet`) ahead of the seed read, because that verdict does not depend on the seed. The auto-seed path keeps its own validation, which validates the value about to be written — cobra's default when nothing was typed.
- `internal/battencli/arm_seed_test.go`: `TestArmSeed_OwnDriverLLMRefusesTheSameWayOnASeededRun` and `TestArmSeed_TypedChildDriverIsValidatedAheadOfTheSeedRead`, both asserting the *better* message and explicitly rejecting the weaker one.

### F2 (LOW) — the status-commit skip's uncommitted-drift cost

Commit `0072d0b69`.

- `internal/battencli/commitstatus.go`: the package comment states the cost of its own skip — the durable status file stays uncommitted for the whole watch — and names the operator-visible consequence.
- `docs/overview.md`: the batten entry says the same in operator terms, beside the paragraph that already explains the self-bounce.

### F4 (NIT) — the design doc's second shipped residual

Commit `415034ed0`.

- `internal/battenshed/doc.go`: the residual that a cleanly finished driver leaves its strand and run directory behind is now recorded beside the dead-or-parked-strand one, so the deleted design doc's substance lives in the module docs as the Documentation Lifecycle requires.

## Deliberately NOT done

- **The llm-driver workspace-trust hang (F6[R2]).** Reproduced this round (see the review's scenario F) with its trigger finally pinned down, but the root cause is in `internal/loomcli`'s llm arm and `internal/shuttleengine`, outside batten's three packages, and the round prompt bars fixing it here. It still needs its own mill-wiki task before anyone relies on `--child-driver llm` against a worktree path this host has never trusted.
- **R1-F9's recreate-from-branch half.** Still blocked on a fabric capability that does not exist (`Topology.Add` refuses a pre-existing branch by design), and the round prompt bars building it. Re-confirmed accurate; note that F1 fixes the *mirror image* of it, which needed no new capability.
- **R1-F6's `step`-mode pacing cost.** Re-measured at 30.33s and re-confirmed as the accepted shape. Moving pacing out of the producer body is a `shedengine` change, out of scope.
- **Suppressing the per-bounce history append** (which would remove F2's drift at the source). A `shedengine` change, out of scope; documented instead.

## Test commands run

Hermetic — green before any change and after every commit:

- `go build ./...`
- `go vet ./internal/battenshed/... ./internal/battencli/... ./internal/battenrecipe/...`
- `go test -count=5 ./internal/battenshed/... ./internal/battencli/... ./internal/battenrecipe/... ./cmd/lyx/...`
- `go test -tags integration -count=1 ./internal/battencli/...`

**Added to this campaign's standing list, and the reason F5 and F6 went unseen for three rounds:**

- `go vet ./...`
- `go test -count=1 ./...` — the two tripwires that were red at HEAD live in `internal/loomcli` and `internal/lyxcwd`, which the per-round command never touched.

Live driving — dev binary redeployed with `./deploy-dev` and every scenario re-driven directly; see the review report's "What was tested" for the full list and observations.

## Changed files

| File | Finding |
| --- | --- |
| `internal/battencli/wire.go` | F1 |
| `internal/battencli/wire_test.go` | F1 |
| `internal/battencli/lifecycle_integration_test.go` | F1 |
| `tools/sandbox/SANDBOX-FABRIC-SUITE.md` | F1 |
| `internal/loomcli/bootstrap_test.go` | F5 |
| `internal/fabricengine/fabric.go` | F6 |
| `internal/fabricengine/doc.go` | F6 |
| `internal/battencli/arm.go` | F3, F6 |
| `internal/battencli/arm_seed_test.go` | F3 |
| `internal/battencli/cli.go` | F6 |
| `internal/battencli/refusal.go` | F6 |
| `internal/battencli/commitstatus.go` | F2 |
| `internal/battenshed/doc.go` | F4 |
| `docs/overview.md` | F1, F2 |
| `CONSTRAINTS.md` | F5, F6 |
