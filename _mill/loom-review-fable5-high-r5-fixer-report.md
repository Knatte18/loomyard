# loom — fixer report, round 5 (`fable5-high-r5`)

> Companion to `_mill/loom-review-fable5-high-r5.md`. Job 2: every finding from that review fixed, verified, and committed on branch `crucible-loom-refshape-registry` (no push).

## Findings fixed (3 of 3 — all severities, none deferred)

| ID | Severity | Status | Fix commit (subject) |
|----|----------|--------|----------------------|
| F1 | MEDIUM | fixed, live-reverified | `loom: fix F1 — Wait's two mechanism-failure caps must consult the file contract` |
| F3 | MEDIUM | fixed, live-reverified | `loom: fix F3 — a malformed-JSON status file must not make 'lyx loom run' refuse at the Seed step` |
| F2 | NIT | fixed (comment-only) | `loom: fix F2 — correct decode-failure attribution in VerifySeedOwnership docs` (loom.md portion folded into the F3 commit) |

No finding was deferred. None required an operator decision or an external (quarry/reed upstream) change.

## F1 — Wait's mechanism-failure exits now honour the file contract

**Defect:** `internal/shuttleengine/wait.go`'s two retry-exhausted exits — the events-unreadable cap (`maxEventsReadRetries`) and the reed-status-error cap (`maxStatusRetries`) — finalized a mechanism-failure error without first checking whether the run's declared output files already exist, while every other negative-answer branch in the same file (not-tracked, not-live, both deadlines) does. A finished run whose reed bookkeeping was reset (a crash-corrupted reed.json, a torn-down session, a renamed worktree) was recorded as failed, and the next resume archived the finished files and re-ran the step. Same shape as round `opus5-high-r4`'s two deadline paths, one exit-type over.

**Fix:** added `func (run *Run) finishedDespiteMechanismFailure() (Result, error, bool)`, which finalizes `OutcomeDone` when `allOutputFilesExist(run.spec.OutputFiles)` and reports whether it fired; both caps call it before returning their mechanism error. Mirrors `classifyDeadlineExpiry`'s file-contract-first shape.

**Tests:** `TestRun_Wait_StatusFailureCap_SatisfiedFileContractWins` and `TestRun_Wait_EventsUnreadableCap_SatisfiedFileContractWins` (hermetic, `internal/shuttleengine/wait_test.go`). Both were proven to FAIL when the helper is stubbed to always report not-satisfied (revert-check), so the guard is real — not the round-3-F1 shape of a test that passes with the fix reverted.

**Doc:** `manifest/designs/loom.md` crash-recovery step 1 updated from "each of the five" to "each of the seven", naming the two retry-exhausted mechanism failures and the `fable5-high-r5` live repro.

**Live re-verification:** rebuilt `lyx`, re-drove the C1 scenario (real `lyx shuttle run`, output file written, reed.json truncated mid-run) → now returns `{"ok":true,"outcome":"done"}` (was the mechanism-failure error before the fix).

## F3 — a malformed-JSON status file no longer makes `lyx loom run` refuse at the Seed step

**Defect:** `manifest/designs/loom.md` promises a poisoned status file (malformed JSON OR an unknown field) never looks like bootstrap's own gate. The unknown-field half held; the malformed-JSON half did not on the run path. `loomshed.Seed` reads the existing file through the lenient `state.UpdateJSON`, which tolerates an unknown field (found=true → `ErrSeedExists`, tolerated) but aborts on malformed JSON with a raw decode error, so `lyx loom run` refused on the envelope at step 2 before ever spawning a driver. `lyx loom drive` was already correct (no Seed call).

**Fix (two files):**
- `internal/state/state.go`: the lenient `readJSONUnlocked` decode error now wraps `state.ErrDecode` (alongside the underlying json error, via double `%w`), matching `ReadJSONStrict`. No production caller previously depended on the lenient path NOT wrapping it (verified by grep); `ErrDecode`'s own doc already claims to sentinel all decode failures.
- `internal/loomshed/seed.go`: `Seed` maps an `errors.Is(err, state.ErrDecode)` failure to its own `ErrSeedExists` sentinel — a present-but-undecodable file is still present, and a corrupt file must not be overwritten (it may be the only forensic record). `UpdateJSON` aborts before its mutate on a decode failure, so the file is left untouched. The bootstrap tolerates `ErrSeedExists` and proceeds; the spawned driver's own `Shed.Run` step-1 read gate diagnoses the decode failure in its log.

**Tests:**
- `internal/loomshed/seed_test.go` `TestSeed_RefusesUndecodableFileAsExists` (table: malformed JSON, not-JSON-at-all, unknown field) — asserts `ErrSeedExists` and that the corrupt file is left byte-for-byte untouched.
- `internal/state/state_test.go` `TestCorruptFile` extended — asserts `errors.Is(err, ErrDecode)` on both `ReadJSON` and `UpdateJSON`, and that `UpdateJSON` never calls its mutate on a corrupt file.
- `internal/loomcli/smoke_test.go` `TestSmokeBootstrap_MalformedStatusProceedsToHandoverAndLogsWhy` (real `lyx loom run` against a weft-committed malformed status file → reaches the tmux handover, driver log names "decode"). Proven to FAIL with the exact Seed-refusal envelope when the mapping is stubbed (revert-check). The existing unknown-field `TestSmokeBootstrap_DiedDriverProceedsToHandoverAndLogsWhy` still passes unchanged.

**Doc:** `manifest/designs/loom.md`'s "poisoned status file" paragraph rewritten to name all three pre-spawn/step-1 gates (`VerifySeedOwnership`, `Seed`, `Shed.Run`'s step-1 read gate) and to correct the diagnosis attribution (also closing part of F2).

**Live re-verification:** rebuilt `lyx`, re-drove F3 (`lyx loom run` against `THIS IS NOT JSON AT ALL {{{`) → now reaches the handover ("not a terminal"), no error envelope on stdout, driver log names "decode failed" (was the refusal envelope before the fix).

## F2 — corrected decode-failure attribution in three doc/comment sites

**Defect (docs only, behavior was correct):** commit `69886823e` attributed the decode-failure diagnosis to "CheckSeed run as the Loom-Preflight producer inside Shed.Run". On a decode failure the Loom-Preflight producer never runs — `Shed.Run`'s step-1 strict read gate errors before any producer is looked up (`CheckSeed`'s own doc comment states this pre-emption). Confirmed live: `lyx loom drive`'s envelope reads `shedengine: read status file …: state: decode failed`, from the step-1 gate.

**Fix (comment-only):** corrected `internal/loomengine/seed.go`'s `VerifySeedOwnership` doc comment and `internal/loomengine/seedownership_test.go`'s `DecodeFailurePasses` comment to attribute the diagnosis to `Shed.Run`'s step-1 read gate. The `manifest/designs/loom.md` site was corrected in the F3 commit. No behavior change, so no new test.

## Deferred / re-evaluated

- **AddStrand/run.json crash-mid-registration "Accepted residual"** (`internal/shuttleengine/run.go` `Start`): re-evaluated per the review prompt's specific angle. Concur with rounds 3-4 — it is a genuine two-independent-stores (reed's persisted strand table vs shuttle's `run.json`) two-phase-commit gap, not closable by reordering, and the marker-record detection mitigation is a real trade (a hard resume refusal for `2 x startup_timeout_s` after any crash in the window) rather than a free win. Stays documented, not code-fixed. No new angle found. **Not a finding.**
- **Windows path behavior:** unreachable from this Linux host; noted as a never-executed gap, not faked.

## Verification — exact commands, all green

- `go build ./...` — exit 0.
- `go vet` over the target set + `internal/state` — exit 0.
- `goimports -l` over all 9 changed Go files — no output (clean).
- `go test -count=5` over the target set + `internal/state` + `cmd/lyx` — all ok.
- `go test ./...` (full repo, twice: after F1+F3, and final) — exit 0, no failures.
- `go test -tags smoke ./internal/loomcli/... -run Smoke -count=1` — all Smoke cases PASS (see What-was-tested; the new malformed-status case included).
- Live driving: F1 (C1) and F3 both reproduced before the fix and re-driven green after, against a freshly-rebuilt `lyx` binary in a real wired hub (`demo-HUB/loom-r5`).

## Changed files

- `internal/shuttleengine/wait.go` (F1: helper + two call sites)
- `internal/shuttleengine/wait_test.go` (F1: two regression tests)
- `internal/state/state.go` (F3: lenient decode error wraps ErrDecode)
- `internal/state/state_test.go` (F3: TestCorruptFile extended)
- `internal/loomshed/seed.go` (F3: ErrDecode → ErrSeedExists mapping)
- `internal/loomshed/seed_test.go` (F3: TestSeed_RefusesUndecodableFileAsExists)
- `internal/loomcli/smoke_test.go` (F3: malformed-status helper + smoke test)
- `internal/loomengine/seed.go` (F2: comment)
- `internal/loomengine/seedownership_test.go` (F2: comment)
- `manifest/designs/loom.md` (F1 + F3 + F2 doc updates)

## Teardown

Live substrate torn down after each scenario and finally: `lyx reed down` confirmed ok, no stray `loom drive` processes, no stray shuttle-owned tmux server (only the pre-existing user tmux PID 485544 that was present at session start remains — not this round's). Scratch fixture repos live under the session scratchpad and are session-local.

## Merge-readiness verdict

This round is **NOT clean** — it found and fixed three real defects (two MEDIUM, one NIT), all of the campaign's core defect class. That is a residual outcome: the orchestrator should re-seed and rotate once more. Whether thread B/C is converged is the orchestrator's call after this report plus its own gate-checking; a genuinely clean safety pass on thread B/C has not yet happened. Thread A remains converged (re-confirmed by independent live driving). All gates are green and every fix is committed on the branch; nothing is pushed.
