# Batch: CLI verbs and docs

```yaml
task: 'fabric: merge-conflict primitive'
batch: CLI verbs and docs
number: 6
cards: 3
verify: go test ./cmd/lyx/ ./internal/fabriccli/ ./internal/lyxcwd/ && go test -tags integration -run Merge ./internal/fabriccli/
depends-on: [5]
```

## Batch Scope

Delivers the outward proof of the one-repo illusion: `lyx fabric merge <branch> [--squash] [-m <message>]` / `merge --continue [-m <message>]` / `merge --abort` and `lyx fabric merge-in <branch>`, with git's own flags, git-mirroring exit codes (0 clean or already-up-to-date, 1 with a failure envelope carrying `conflicts`), the dedicated conflict-envelope helper whose `partial` stays `false`, and every CLI-invariant test entry (help-tree, arity, envelope contract).
Closes the task's Documentation Lifecycle obligations: `internal/fabricengine/doc.go`, the `manifest/designs/finalize.md` reword, the `manifest/roadmap.md` move, and `docs/overview.md`.
Batch-local decision: the two verbs join the weft-verb family (`weftVerbNames` + `PersistentPreRunE`-resolved `fab` handle) because they need the resolved pair handle exactly as `commit`/`pull` do;
`-m` uses `StringP` (the repo's one existing shorthand precedent is `internal/selfreportcli`).

## Cards

### Card 16: fabric merge and merge-in CLI verbs plus the conflict envelope helper

- **Context:**
  - `internal/fabriccli/fabric.go`
  - `internal/fabricengine/merge.go`
  - `internal/fabricengine/mergelifecycle.go`
  - `internal/fabricengine/mergeerrors.go`
  - `internal/clihelp/exec.go`
  - `internal/output/output.go`
- **Edits:**
  - `internal/fabriccli/weft_verbs.go`
  - `internal/fabriccli/envelope.go`
- **Creates:**
  - `internal/fabriccli/merge_verbs.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  **`envelope.go`**: add `errConflictsWithRecord(w io.Writer, rec fabricengine.Mutations, conflicts []string) int` per the Shared Decision — sets `fields["mutations"] = rec.Entries()`, `fields["partial"] = false` (the literal, never computed: the engine returned a nil error, so the Mutation Record Invariant's `error ≠ nil ∧ record non-empty` rule yields false), `fields["conflicts"] = conflicts` (never null), and delegates to `output.ErrFields` with the fixed, side-free message `"merge produced conflicts; resolve them, then run \"lyx fabric merge --continue\""`.
  Extend the package doc comment's helper enumeration.

  **`merge_verbs.go`**: one registration function with this exact signature:

  ```go
  func addMergeVerbs(cmd *cobra.Command, fabric func() *fabricengine.Fabric)
  ```

  The handle must be reached **indirectly**, through that getter closure. `fab` is a local of `addWeftVerbs` that `PersistentPreRunE` assigns at run time (`weft_verbs.go:41,52`), so passing `*fabricengine.Fabric` by value at registration time would capture the nil zero value and every merge verb would nil-panic.
  `addWeftVerbs` calls `addMergeVerbs(cmd, func() *fabricengine.Fabric { return fab })`, and each `RunE` body reads `fabric()` after `PersistentPreRunE` has run.
  Both verbs otherwise follow `weft_verbs.go`'s verb shape (raw `RunE`, `clihelp.ShouldAbort` first, `clihelp.SetExit`, always `return nil`):
  - `merge-in`: `Use: "merge-in <branch>"`, `Args: cobra.ExactArgs(1)`, `Short: "merge a branch into this worktree, surfacing conflicts"`.
    `Long` explains: this is the workflow step that runs in the task worktree before `merge`; conflicts are reported for resolution here and concluded with `lyx fabric merge --continue` (or abandoned with `--abort`) — the lifecycle is shared with `merge`, and this asymmetry is stated, not implied.
    Calls `fabric().MergeIn(args[0])`.
  - `merge`: `Use: "merge (<branch> | --continue | --abort) [--squash] [-m <message>]"`, `Short: "merge a branch into this worktree, or continue/abort a merge"`.
    Flags: `--squash` (bool), `--continue` (bool), `--abort` (bool), and `StringP("message", "m", "", "commit message for the merge commit")`.
    Mode-dependent arity via a custom `Args` validator (flags are parsed before `Args` runs): with `--continue` or `--abort` exactly zero positionals, otherwise exactly one — rejecting violations with a usage-shaped error.
    `--continue` and `--abort` are mutually exclusive (`MarkFlagsMutuallyExclusive`), and `--squash` with either is rejected in `RunE` as a usage error (bare `output.Err` + `clihelp.Abort`, the pre-flight carve-out — nothing was mutated).
    Dispatch: `--continue` → `fabric().MergeContinue(message)`;
    `--abort` → `fabric().MergeAbort()`;
    default → `fabric().Merge(args[0], fabricengine.MergeOptions{Squash: squash, Message: message})` — the CLI default is git's default (no squash, no message).
    `Long` carries an `Example:` block with `lyx fabric merge-in my-task`, `lyx fabric merge my-task --squash`, `lyx fabric merge --continue`.

  Both verbs' `Long` text must additionally spell out the identifier-to-verb mapping, because the engine's pinned error messages name Go identifiers the CLI does not expose: `MergeIn` is `lyx fabric merge-in`, `MergeContinue` is `lyx fabric merge --continue`, and `MergeAbort` is `lyx fabric merge --abort`.
  Those messages travel to the operator verbatim through the envelope (they are pinned by the discussion and asserted byte-exactly by batch 5's vocabulary test), so the help text is where the mismatch is closed.

  Envelope mapping, shared by all modes:
  - engine error non-nil → `clihelp.SetExit(cmd.Context(), errWithRecord(out, res.Mutated(), err))` — the typed errors' fixed messages are the envelope's error text.
  - `len(res.Conflicts) > 0` → `clihelp.SetExit(cmd.Context(), errConflictsWithRecord(out, res.Mutated(), res.Conflicts))` — exit 1, mirroring `git merge`'s nonzero conflict exit.
  - otherwise → `okWithRecord(out, res.Mutated(), map[string]any{"committed": res.Committed, "already_up_to_date": res.AlreadyUpToDate})` — exit 0 on clean or already-up-to-date.

  **`weft_verbs.go`**: add `"merge"` and `"merge-in"` to `weftVerbNames` (so `PersistentPreRunE` resolves the pair handle for them) and call `addMergeVerbs(cmd, func() *fabricengine.Fabric { return fab })` from inside `addWeftVerbs`, alongside the existing subcommand registrations — inside the function body, where `fab` is in scope.
  Update the file's header comment, which enumerates the six weft verbs and says `PersistentPreRunE` is "scoped to these six verb names only".
- **Commit:** `feat(fabriccli): lyx fabric merge and merge-in verbs`

### Card 17: CLI invariant tests and end-to-end merge CLI coverage

- **Context:**
  - `internal/fabriccli/merge_verbs.go`
  - `internal/fabriccli/envelope.go`
  - `internal/fabriccli/testmain_test.go`
  - `internal/hubforge/hub.go`
  - `internal/hubforge/seed.go`
  - `internal/gitkit/gitkit.go`
  - `internal/lyxcwd/lyxcwd.go`
- **Edits:**
  - `cmd/lyx/helptree_test.go`
  - `internal/fabriccli/argsarity_test.go`
  - `internal/fabriccli/envelope_test.go`
  - `internal/fabriccli/cli_test.go`
  - `internal/fabriccli/envelopecontract_integration_test.go`
- **Creates:**
  - `internal/fabriccli/merge_cli_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - `cmd/lyx/helptree_test.go`: add `"merge"` and `"merge-in"` to the fabric row's `wantSubs`.
  - `internal/fabriccli/argsarity_test.go`: add `tooMany` entries `"merge": ["a", "b"]` and `"merge-in": ["a", "b"]` (two positionals are invalid in every mode);
    if the harness's cobra-validator introspection cannot express `merge`'s mode-dependent custom validator, use a `handRolledArity` entry with a one-line reason instead, following `clone`'s precedent.
  - `internal/fabriccli/envelope_test.go`: contract coverage for `errConflictsWithRecord` — `mutations` from the record and never null, `partial` the literal `false` even with a non-empty record (the property the Shared Decision exists to pin), `conflicts` never null, reserved keys win over collisions, return value 1.
  - `internal/fabriccli/cli_test.go`: help assertions in the existing `TestRunCLI_*Help` style — `merge --help` names `--squash`, `--continue`, `--abort`, `--message`;
    `merge-in --help` has a non-empty Short;
    `merge --continue extra-arg` and `merge --continue --abort` fail with usage-shaped error envelopes.
  - `internal/fabriccli/envelopecontract_integration_test.go`: extend the existing property walk so the conflict envelope is asserted against the fixed key contract (exit 1, `"ok":false`, `mutations` array present, `partial` present and `false`, `conflicts` array non-empty).
  - `internal/fabriccli/merge_cli_integration_test.go` (`//go:build integration`, `package fabriccli_test`, hubforge fixture per `cli_test.go`'s `setupCLIRepo` pattern, driven via `fabriccli.RunCLIIn(<pair worktree>, …)`;
    test names containing `Merge`):
    - `merge-in <branch>` with manufactured divergence and a conflict → exit 1, failure envelope with sorted `conflicts`, `partial` false;
      resolving the conflict (edit + `git add` via `gitkit.MustRun`) then `merge --continue` → exit 0, `committed` true.
    - `merge-in` then `merge --abort` → exit 0, pair restored (SHA assertions).
    - clean `merge <branch> --squash` from the target pair's worktree → exit 0, `committed` true.
    - `merge <branch>` that would conflict → exit 1, envelope error text is `ErrMergeInRequired`'s fixed message, target unchanged.
    - `merge nonexistent-branch` → exit 1 with the aggregated guard error text.
    - already-up-to-date `merge-in` → exit 0, `already_up_to_date` true.
- **Commit:** `test(fabriccli): merge verb help, arity, envelope, and end-to-end coverage`

### Card 18: documentation — fabricengine doc, finalize reword, roadmap, overview

- **Context:**
  - `internal/fabricengine/merge.go`
  - `internal/fabricengine/mergelifecycle.go`
  - `internal/fabricengine/mergeerrors.go`
  - `manifest/designs/raddle.md`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/fabricengine/doc.go`
  - `manifest/designs/finalize.md`
  - `manifest/roadmap.md`
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  All prose in semantic line breaks (one sentence per line), per the repo's markdown rule;
  every touched inline link must still resolve (Markdown Link Integrity walks `manifest/` and `docs/` sources).

  **`internal/fabricengine/doc.go`**: add a `# The merge surface` godoc section in the doc's existing section style (~40–60 lines) covering:
  the two verbs and why (`MergeIn` in the task pair where conflicts are resolved; `Merge` on a handle opened at the target, squash-capable, expected conflict-free, self-aborting to `ErrMergeInRequired`);
  the recorded merge (`fabric-merge.json` beside the correspondence index, why derivation from git alone fails — ff-defeats-`--no-commit`, squash records no `MERGE_HEAD`, the human's foreign half-merge);
  the lifecycle quartet and crash recovery;
  unified worktree-relative conflict reporting and the `ErrUnmergeableState` refusal;
  SHA-labelled conflict markers;
  the sibling-verb refusals and the combined write lock's role (mutating steps only, never across the resolution window);
  no post-conclude undo — verify-before-conclude plus `MergeAbort` is the recovery model.
  This doc explains fabric's own mechanism, so warp/weft vocabulary is permitted (prose-doc split).

  **`manifest/designs/finalize.md`** (a consumer doc — side-free wording throughout, per the prose-doc split):
  - Rework the "Only Raddle's output forwards to the parent" section: replace the sentence "Merge-back forwards only Raddle's regenerated output …, via a Fabric commit scoped to `["_lyx"]` …" so it reads as Finalize's own content policy — Finalize deletes or unwires whatever must not cross the merge boundary, then calls an ordinary, content-blind Fabric merge — and delete the sentence "The exact commit call this uses is part of the `fabric: merge-conflict primitive` task's scope, not fixed here", resolving it per the discussion: it is not a scoped commit call at all.
  - Update the status blockquote (line 3) and the "not implemented anywhere today" paragraph (line 21): the merge primitive now exists — name the shipped surface (`MergeIn`, `Merge`, `MergeContinue`, `MergeAbort`, `MergeInProgress` on Fabric; `lyx fabric merge`/`merge-in`) and drop the "blocked on" framing.
  - Update the Related-list bullet (line 51) that says fabricengine "does not yet include the merge-conflict primitive".
  - Keep the existing anchors and link targets intact (`raddle.md` anchors, the `CONSTRAINTS.md` fragment).

  **`manifest/roadmap.md`**: the planned item "fabric: merge-conflict primitive" is completing — move or remove it per the file's own convention for completed planned items (inspect the file;
  if no completed section exists, delete the entry, since git history and the module docs carry it), and update the "loom: phase-machine scaffolding" item's Finalize row: "Depends on `fabric: merge-conflict primitive` above — blocked until that lands" becomes a plain dependency-satisfied statement.

  **`docs/overview.md`**: extend the fabric module row's CLI-surface enumeration (`clone|add|…|diff`) with `merge-in|merge`, and mention the merge/conflict lifecycle in the row's prose in one phrase;
  check the `## Status` sub-bullet that re-lists the weft content-sync verbs and extend it only if its list claims completeness.
- **Commit:** `docs: merge surface — fabricengine doc section, finalize reword, roadmap, overview`

## Batch Tests

`verify` runs the untagged `cmd/lyx` suite (help-tree, drift/Short, json-help, registration), the untagged fabriccli suite (arity, envelope contracts), the `internal/lyxcwd` suite (vocabulary walk over production Go and `internal/**/*.md`, and the Markdown Link Integrity walk over `manifest/`/`docs/` — both doc cards' obligations), and the `Merge`-named fabriccli integration tests end-to-end.
