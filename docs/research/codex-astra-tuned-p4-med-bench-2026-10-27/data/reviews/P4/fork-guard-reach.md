Guard reachability:

- Cards 1–3: empty-machine, payload-owner mismatch, non-takeover ownership, pull/push failure and lost-write refusals have reachable inputs and named tests.
- Cards 5–6: owned-elsewhere classification, replay suppression, nil owner reader and reader errors are reachable; ended precedence is explicitly covered.
- Cards 8–9: start, resume, step and window refusals are reachable with an existing owned-elsewhere entry and no covering pause. `refuseCovered` precedes the owner guard, but does not make these test inputs unreachable.
- Cards 10–11: sleep, hold-end, spawn-owner refusal and parent-lock release branches have reachable seam inputs. Finalize’s running/transient carve-out and its stopped counterpart are separately covered.
- Card 12: owner checks and past-merge exemption are reachable after the existing parent guard. A records-origin outage can reach the new unreachable-reconcile branch even when the preceding code-origin fetch succeeds.
- Card 13: wake owner-pause mapping is reachable. The orch owner-pause branch has a construction-versus-execution wiring problem described below.
- Cards 15–16: window reopen, adoption selection, local-commit stop, live-pair refusal, lost-write stop and owner-pause lifting have reachable inputs. The hub-only pause seam addresses the upstream liveness problem explicitly.

Finding:

- **MEDIUM — consistency — `_lyx/plan/13-wake-and-orch-owner-pause.md:36` — Copying the owner-pause callback during `Command()` construction captures it before the root pre-run installs it.**
  The card says `newRoot`’s pre-run installs `InstallOwnerPause`, while `Command` copies the installed callback onto `orchCLI.pauseOwned`.
  `cmd/lyx/main.go:119` defines the pre-run callback, but `cmd/lyx/main.go:161` calls `orchcli.Command()` while assembling the tree, before that callback executes.
  `internal/orchcli/cli.go:101` constructs the receiver at command construction; its existing runtime dependency assignment happens in its `PersistentPreRunE`, ending at `internal/orchcli/cli.go:196`.
  Following the proposed copy order leaves a fresh process’s `pauseOwned` nil, so an owned-elsewhere run takes the unwired-warning branch instead of receiving D4’s required pause.
  The planned test at card line 44 injects `pauseOwned` directly and does not exercise this production ordering.
  Specify that the installed callback is copied in orch’s persistent pre-run after the root hook, or looked up at call time, and cover the root-install-to-orch-use sequence.

Tool-call manifest:

1. Read orch runtime wiring, landing verdict/parent guards and InnerRun pause/lock handling; the inherited manifest lacked those hook and guard bodies.
2. Read network-gate installation, window execution, loom pause refusal, landing dependency wiring, parent-work test and wake hold handling; needed upstream checks and production callers.
3. Read `parentGuard.moved` and related seam references; needed to establish what executes before Finalize’s proposed owner checks.
4. Read `Fabric.ReadParentTips`, its production wiring and existing guarded landing test setup; needed to distinguish code-origin fetch failures from reachable reconcile failures.
5. Read InnerRun’s `Call` and child-wait lifecycle, binary re-step and wake-report mapping; needed the paths reaching new spawn and wake guards.
6. Read guarded test dependencies, tender’s pause sequence, construction references and numbered card clauses; needed concrete reaching fixtures and precise evidence locations.
