# Batch: go-sweep

```yaml
task: "Rename the fabric host vocabulary to warp, and name the composite repo Fabric"
batch: "go-sweep"
number: 3
cards: 3
verify: go test ./... && go test -tags integration ./...
depends-on: [2]
```

## Batch Scope

The mechanical heart of the task: one `wordswap` invocation rewrites every fabric-sense `host` token — identifiers, comments, test-function names, string literals, struct tags and shell variables — across 94 Go and shell files in one pass, followed by hand adjudication of the ambiguity report the tool prints.

This batch is deliberately **not** split by package.
`HostJunctions`, `HostLyxLink`, `DeriveHostName`, `HostWorktree` and `lyxtest.CopyHostHub`/`HostFixture` have callers outside their own packages (`internal/fabriccli/clone.go:41`, `internal/loomengine/preflight_integration_test.go:450` and `:521`, `internal/idecli/cli_test.go`, `internal/lyxcwd/anchor_test.go`, `internal/lyxcwd/lyxcwd_test.go`), so any per-package split would leave the tree uncompilable at its own batch boundary.
The atomic sweep is what makes `go build ./...` a completeness proof rather than a partial one.

The file set is an explicit list, not a glob, because five categories of file must be kept out of it and no glob expresses that:

- `internal/lyxcwd/enforcement_test.go` and `CONSTRAINTS.md` name the retired vocabulary in order to forbid it (batch 7 hand-edits both).
- `internal/configcli/configcli.go`, `internal/builderengine/spawn.go` and `internal/buildercli/poll.go` are non-owner production files (batch 2 hand-edited the first two; the third is untouched by this task).
- `internal/builderengine/spawn_test.go`, `internal/boardengine/boardtest/concurrency_test.go`, `cmd/lyx/crosscompile_test.go` and `cmd/lyx/gitrepoboundary_test.go` carry only machine-sense or verb-sense `host`.
- `internal/configengine/config_test.go`'s `server:\n  host: localhost` YAML fixture is machine-sense test data.
- Every `.md` file, `crucible/`, and the four historical-record docs — batch 6's business, or nobody's.

This batch also carries the task's **one observable behaviour change**: the `json:"host_worktree"`/`json:"host_branch"` struct tags become `json:"warp_worktree"`/`json:"warp_branch"`, changing the fields emitted by `lyx fabric status --json`, `lyx fabric reconcile --json` and `lyx fabric prune --json`.

## Cards

### Card 8: run the sweep over the Go and shell file set

- **Context:**
  - `tools/wordswap/main.go`
  - `tools/wordswap/swap.go`
  - `_mill/discussion.md`
- **Edits:**
  - `cmd/lyx/tierpurity_test.go`
  - `internal/boardengine/board.go`
  - `internal/buildercli/poll_test.go`
  - `internal/buildercli/sync_integration_test.go`
  - `internal/buildercli/sync_test.go`
  - `internal/buildercli/validate_test.go`
  - `internal/builderengine/gitquery_test.go`
  - `internal/configcli/configcli_integration_test.go`
  - `internal/fabriccli/cli_test.go`
  - `internal/fabriccli/clone.go`
  - `internal/fabriccli/fabric.go`
  - `internal/fabriccli/pushbypass_integration_test.go`
  - `internal/fabriccli/unwire.go`
  - `internal/fabricengine/add_branch_exists_test.go`
  - `internal/fabricengine/add.go`
  - `internal/fabricengine/add_rollback_adopt_test.go`
  - `internal/fabricengine/add_test.go`
  - `internal/fabricengine/boardjunction_integration_test.go`
  - `internal/fabricengine/boardweft.go`
  - `internal/fabricengine/branchname.go`
  - `internal/fabricengine/branchname_test.go`
  - `internal/fabricengine/checkout.go`
  - `internal/fabricengine/checkout_rollback_test.go`
  - `internal/fabricengine/classify_test.go`
  - `internal/fabricengine/cleanreason_integration_test.go`
  - `internal/fabricengine/cleanup.go`
  - `internal/fabricengine/clone_adopt_test.go`
  - `internal/fabricengine/clone.go`
  - `internal/fabricengine/clone_test.go`
  - `internal/fabricengine/coalesce.go`
  - `internal/fabricengine/coalesce_integration_test.go`
  - `internal/fabricengine/config_driven_junctions_integration_test.go`
  - `internal/fabricengine/config.go`
  - `internal/fabricengine/diff.go`
  - `internal/fabricengine/diff_integration_test.go`
  - `internal/fabricengine/doc.go`
  - `internal/fabricengine/dotlyxjunction_integration_test.go`
  - `internal/fabricengine/drift.go`
  - `internal/fabricengine/fabric.go`
  - `internal/fabricengine/fabric_test.go`
  - `internal/fabricengine/healthreason_integration_test.go`
  - `internal/fabricengine/hook.go`
  - `internal/fabricengine/hook_test.go`
  - `internal/fabricengine/hostclean.go`
  - `internal/fabricengine/hostjunction_test.go`
  - `internal/fabricengine/hostlayout.go`
  - `internal/fabricengine/index.go`
  - `internal/fabricengine/index_integration_test.go`
  - `internal/fabricengine/junction.go`
  - `internal/fabricengine/junctionnames.go`
  - `internal/fabricengine/junction_pattern_integration_test.go`
  - `internal/fabricengine/junction_repoint_test.go`
  - `internal/fabricengine/junction_test.go`
  - `internal/fabricengine/launcher_content.go`
  - `internal/fabricengine/launcher_content_test.go`
  - `internal/fabricengine/open_integration_test.go`
  - `internal/fabricengine/portallauncher_test.go`
  - `internal/fabricengine/post-checkout.sh`
  - `internal/fabricengine/prune.go`
  - `internal/fabricengine/reconcile.go`
  - `internal/fabricengine/reconcile_stale_registration_test.go`
  - `internal/fabricengine/reconcile_stale_removal_test.go`
  - `internal/fabricengine/remove.go`
  - `internal/fabricengine/remove_junctions_integration_test.go`
  - `internal/fabricengine/snapshot_integration_test.go`
  - `internal/fabricengine/status.go`
  - `internal/fabricengine/structuraldirs_test.go`
  - `internal/fabricengine/unwire.go`
  - `internal/fabricengine/unwire_test.go`
  - `internal/fabricengine/weftgit_exclude_test.go`
  - `internal/fabricengine/weftgit_unborn_warp_test.go`
  - `internal/fabricengine/weftpaths_test.go`
  - `internal/fabricengine/weftwiring.go`
  - `internal/fabricengine/weftwiring_test.go`
  - `internal/fabricengine/worktreelist_test.go`
  - `internal/idecli/cli_test.go`
  - `internal/loomengine/preflight_integration_test.go`
  - `internal/lyxcwd/anchor_test.go`
  - `internal/lyxcwd/lyxcwd_test.go`
  - `internal/lyxtest/doc.go`
  - `internal/lyxtest/lyxtest.go`
  - `internal/lyxtest/lyxtest_test.go`
  - `internal/perchcli/run_integration_test.go`
  - `internal/webstercli/cli_test.go`
  - `internal/webstercli/sync_integration_test.go`
  - `internal/webstercli/verbs_test.go`
  - `internal/websterengine/audit_test.go`
  - `internal/weftname/weftname.go`
  - `tools/sandbox/main.go`
  - `tools/sandbox/main_test.go`
  - `tools/sandbox/report.go`
  - `tools/sandbox/report_test.go`
  - `tools/sandbox/suite.go`
  - `tools/sandbox/suite_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Write the exact `Edits:` list above, one path per line, to `.scratch/sweep-files.txt` — it is the tool's argument list, and `.scratch/` is gitignored so the file is never committed.
  Then run a dry pass first and read the report:

  ```
  go run ./tools/wordswap -from host -to warp -dry-run -skip 'pane hosting an idle agent' $(cat .scratch/sweep-files.txt)
  ```

  Confirm the report shows zero `MISMATCH` lines and an `AMBIGUOUS (unresolved)` bucket of exactly five entries — `internal/fabricengine/drift.go:3`, `internal/fabricengine/hostclean.go:1`, `internal/fabricengine/hostjunction_test.go:1`, `internal/fabricengine/hostlayout.go:1`, `internal/lyxtest/lyxtest.go:128`, and nothing else — plus a `SKIPPED (deliberate)` bucket containing only `internal/buildercli/poll_test.go:212`.
  Treat that list as the work list for card 9, never as a checksum: if the report differs, read the extra entries and classify them, do not assume the run is wrong.
  A `MISMATCH` line means the reversibility invariant failed for that file — stop and investigate rather than proceeding, since the tool leaves such a file untouched by design.

  Then run the same command without `-dry-run` to apply it.
  The single `-skip` pattern claims `internal/buildercli/poll_test.go:212`'s verb-sense "a live pane hosting an idle agent", which is a deliberate keep rather than a reword — that skip set is the audit record of the one occurrence this task consciously leaves alone.

  Do not hand-edit anything in this card;
  every change here is the tool's output.
  In particular, the four `json:"host_worktree"`/`json:"host_branch"` struct tags at `internal/fabricengine/status.go:40` and `:44`, `internal/fabricengine/reconcile.go:73` and `internal/fabricengine/prune.go:22` are swapped by the same pass — that is intended, and it is this task's only observable behaviour change, so it must be named in this card's commit message body.
- **Commit:** `refactor(fabric): rename the host vocabulary to warp across Go and shell sources`

### Card 9: adjudicate the ambiguity report to a clean exit-zero run

- **Context:**
  - `tools/wordswap/swap.go`
  - `_mill/discussion.md`
- **Edits:**
  - `internal/fabricengine/drift.go`
  - `internal/fabricengine/hostclean.go`
  - `internal/fabricengine/hostjunction_test.go`
  - `internal/fabricengine/hostlayout.go`
  - `internal/lyxtest/lyxtest.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Resolve every entry card 8's run left in the unresolved-AMBIGUOUS bucket by hand.
  All five are all-lowercase compounds the tool cannot classify mechanically, and all five are fabric-sense:

  - `internal/fabricengine/hostclean.go` line 1 — the file-comment token `hostclean.go` becomes `warpclean.go`.
  - `internal/fabricengine/hostlayout.go` line 1 — the file-comment token `hostlayout.go` becomes `warplayout.go`.
  - `internal/fabricengine/hostjunction_test.go` line 1 — the file-comment token `hostjunction_test.go` becomes `warpjunction_test.go`.
  - `internal/fabricengine/drift.go` line 3 — the cross-reference `(hostclean.go)` becomes `(warpclean.go)`.
  - `internal/lyxtest/lyxtest.go` line 128 — the `os.MkdirTemp` prefix literal `"lyxtest-hosthub-*"` becomes `"lyxtest-warphub-*"`.

  The first four name files that batch 4 renames;
  editing the comments here is what lets this batch reach exit zero, and batch 4's `git mv` is what makes the names accurate.
  This is the intended order, not a mistake.

  Then re-run the tool and require a clean result:

  ```
  go run ./tools/wordswap -from host -to warp -skip 'pane hosting an idle agent' $(cat .scratch/sweep-files.txt)
  ```

  It must report `0 changed`, an empty unresolved-AMBIGUOUS bucket, the single deliberate skip, and **exit 0**.
  A non-zero exit here means an occurrence is still unadjudicated — resolve it by hand-editing or by extending the `-skip` set, never by loosening the tool's rule.
- **Commit:** `refactor(fabric): resolve the host->warp ambiguous compounds by hand`

### Card 10: confirm the two merged names introduced no shadowing

- **Context:**
  - `internal/fabricengine/status.go`
  - `internal/fabricengine/prune.go`
  - `internal/fabricengine/reconcile.go`
  - `internal/fabricengine/cleanup.go`
  - `internal/fabricengine/coalesce.go`
  - `internal/fabricengine/commit.go`
  - `internal/fabricengine/index.go`
- **Edits:** none
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Verification only — this card changes no file.
  Two swept names merge into a pre-existing twin rather than introducing a new one: `hostPath` merges into `warpPath`, and `hostBare` merges into `warpBare`.
  Both were verified pre-sweep, by enumerating the files containing each name across `internal/`, `cmd/` and `tools/`, to have **zero** files containing both names of a pair — so the substitution removes a duplicate name rather than creating a clash.
  No occurrence count is pinned here deliberately: counts drift with every commit and would fire as a false tripwire, while the file-overlap property is what the safety argument actually rests on.

  Confirm that still holds after the sweep, because `go build` alone cannot: it catches package-level redeclaration, but a function-local `warpPath` shadowing a same-named package-level symbol compiles silently.
  Run, from the repo root:

  ```
  go build ./... && go vet ./...
  ```

  and then, for each of `warpPath` and `warpBare`, grep `internal/fabricengine` for declarations (`warpPath :=`, `warpPath =`, `var warpPath`, `warpPath ` in a struct field or parameter list) and confirm no file declares the name at both package scope and function scope.
  If the rename table is ever widened beyond these two merges, this check must be re-run for the added pairs.
  Record the outcome in the batch's review notes;
  make no code change.
- **Commit:** none

## Batch Tests

`verify: go test ./... && go test -tags integration ./...` is deliberately the **unbounded** repo-wide suite, in both tiers, and this is the one batch in the plan where that scope is justified.

The justification is the batch's own premise.
The sweep rewrites 94 files across 20 packages, renaming exported symbols (`HostJunctions`, `HostLyxLink`, `HostLyxLinkHere`, `HostJunction`, `WeftHostSlug`, `HostWorktree`, `DeriveHostName`, `HostBranch`), the `internal/lyxtest` exported fixture seam (`CopyHostHub`, `HostFixture`) that eight packages import, and roughly sixteen `Test*` function names.
A clean compile of the whole module plus a green run of every test is precisely the completeness proof the discussion nominates in place of a hand-maintained rename table — a scoped `verify` would prove only that the packages someone thought to name still build, which is the failure mode the generic-swap decision exists to avoid.
The integration-tagged pass is included because the renamed `Test*` functions and the `lyxtest` fixture seam are concentrated in `*_integration_test.go` files, which the untagged pass never compiles.

Specific existing tests that must stay green with renamed identities: the `Test*` functions listed in `_mill/discussion.md`'s "Measured identifier surface" — `TestOpen_MissingHostWorktree`, `TestWireJunctions_RefusesRealHostDirectory`, `TestWeftHostSlug`, `TestWeftBranchName_RoundTripsWithWeftHostSlug`, `TestUnwire_NeverWiredHostIsIdempotentNoOp`, `TestHostLyxLinkMethods`, `TestHostJunctionsHere`, `TestHostJunctions`, the three `TestDetectHostPollution_*`, `TestCoalescePushBothAt_AdvancesBothSidesAndLeavesNoHostRootLock`, `TestCleanup_DetachedHostHeadProtectsCheckedOutWeftBranch`, `TestDeriveHostName`, `TestCopyHostHub` and `TestCopyHostHub_Isolation`.
They are renamed by the sweep, so the gate is that they are still **discovered** under their new names and still pass — a silently-undiscovered renamed test would show up as a drop in the package's test count, not as a failure.

`TestEnforcement_FabricVocabulary` must also stay green here, unchanged and untightened.
It passes because the owner-dir skip is still in force for the host half (batch 7 removes it), because `*_test.go` is excluded from all three rules, and because batch 2 kept every non-owner production file free of a bare `warp` token.
