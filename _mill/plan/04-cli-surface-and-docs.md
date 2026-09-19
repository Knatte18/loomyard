# Batch: cli-surface-and-docs

```yaml
task: 'fabric: no remote/GitHub branch deletion'
batch: 'cli-surface-and-docs'
number: 4
cards: 5
verify: go test ./internal/fabriccli/... ./cmd/lyx/... && go test -tags integration ./internal/fabriccli/...
depends-on: [3]
```

## Batch Scope

This batch puts the feature in front of the operator: the two `--remote` flags, the three help texts that currently assert the behaviour this task removes, the two fields maps that carry the remote outcome into the JSON envelope, the exit-code change that keeps a remote failure from reporting success, the CLI-level tests, and the roadmap move.
It is one batch because the flag, the help text, the envelope keys, and the exit code are one observable contract — shipping any subset would document behaviour the code does not have, or emit a verdict no help text explains.

Batch-local decisions beyond the overview's Shared Decisions:

- `runCleanupWithFlags` keys its failure exit on BOTH `Error` and `RemoteError`, not on `RemoteError` alone.
  This is a deliberate change to an existing path: a `cleanup --apply` whose local `git branch -D` failed exits non-zero from now on, where today it exits 0.
  Shipping the alternative would mean two verdicts for the same class of failure in the same struct.
- The synthesised error strings are fixed by this plan rather than left to the implementer, because `errWithRecordFields` calls `err.Error()` and the engine returns nil, so the text is newly observable CLI output.
- No `refusal` key appears on these envelopes.
  `errWithRecordFields` attaches one only when `RefusalOf` matches a gate refusal, and a synthesised `fmt.Errorf` never will — correct, since a push the remote rejected is not a gate refusal.

## Cards

### Card 13: The two --remote flags and the three help texts

- **Context:**
  - `internal/fabricengine/cleanup.go`
  - `internal/fabricengine/remove.go`
  - `cmd/lyx/helptree_test.go`
  - `cmd/lyx/jsonhelp_test.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/fabriccli/fabric.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Register a `remote` boolean flag, default false, on both `cleanupCmd` and `removeCmd`, alongside their existing `--force` and (for cleanup) `--apply` registrations.
  Give each a usage string naming the act as irreversible and visible to every other clone.
  Read each through the same closure-over-the-command pattern the existing flags already use, and pass the value into `runCleanupWithFlags` and `runRemoveWithFlag` as a new trailing parameter on each — cards 14 and 15 widen those two functions' own signatures and bodies.

  Update both `Use` strings: `cleanup [--apply] [--force]` becomes `cleanup [--apply] [--force] [--remote]`, and `remove [--force] <slug>` becomes `remove [--force] [--remote] <slug>`.
  Both commands keep a non-empty `Short`, per the CLI / Cobra Invariant.

  Update `runRemoveWithFlag`'s own usage-error string, currently `usage: lyx fabric remove [--force] <slug>`, to match the new `Use` string exactly — the same line exists in three places and all three must agree.

  Rewrite `cleanupCmd.Long`'s final paragraph rather than appending to it.
  It currently reads:

```
Deletion is local to the hub's weft repo: a deleted branch's copy on the
weft remote, if it was ever pushed, is left untouched.
```

  That asserts the exact behaviour this task removes.
  The replacement must state: deletion is local to the hub's weft repo by default;
  `--remote` additionally deletes each deleted branch's copy on the weft remote;
  `--remote` requires `--apply` to delete anything, so `--remote` alone is still a dry run;
  a dry run makes no network call and reports no remote-specific verdict, and the existing "protected: false in a dry run means --apply would delete this" contract extends unchanged to "and, with --remote, would attempt the remote copy too";
  a weft repo with no `origin` remote configured reports the reason once in `remote_skipped_reason` and still exits 0;
  and a remote deletion that fails exits non-zero, with the per-branch reason in `entries[].remote_error`.
  Extend the flag matrix block in the same `Long` with a `--remote` row and a `--apply --remote` row saying the same thing in matrix form.
  State that `--remote` is independent of `--force`.

  Also record in that `Long` the one existing-path change this task makes: `--apply` now exits non-zero when a local branch deletion fails, where it previously exited 0, while a `protected` entry still exits 0 because protection sets no error at all.

  Extend `removeCmd.Long` with a paragraph covering `--remote` in the same terms, scoped to its single weft branch: the pair's weft branch is deleted locally as before, `--remote` additionally deletes its copy on the weft remote, a missing `origin` reports `remote_skipped_reason` and exits 0, and a failed remote deletion exits non-zero with the reason in `remote_branch_error`.

  Write both help texts with the repo's own semantic-line-break style, matching the surrounding `Long` strings.
  The help-tree tests assert subcommand names as supersets and so tolerate the new flag without a change of their own;
  the `--json` help schema tests likewise read the live flag set rather than a pinned list.
- **Commit:** `feat(fabric): add --remote to cleanup and remove, and rewrite their help texts`

### Card 14: runCleanupWithFlags — remote threading, envelope keys, failure exit

- **Context:**
  - `internal/fabricengine/cleanup.go`
  - `internal/fabriccli/envelope.go`
  - `internal/fabricengine/doc.go`
  - `internal/fabricengine/prune.go`
- **Edits:**
  - `internal/fabriccli/fabric.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Widen `runCleanupWithFlags` to take a trailing `remote bool` and pass it through to `Cleanup` as its new fourth argument, replacing card 10's interim literal `false`.

  Build the fields map with the existing `"entries"` key plus one new key, `"remote_skipped_reason"`, set from the result's `RemoteSkippedReason`.
  Emit it unconditionally, on both the success and the failure exit, whatever the struct's `omitempty` tag says: `CleanupResult` itself is never marshalled — only `Entries` is, as a map value — so a verb-level field reaches the envelope because the map names it, never because the struct declares it.
  The per-entry `remote_deleted` and `remote_error` keys need no map entry: they marshal from `CleanupBranchEntry` automatically because `Entries` is itself a map value.

  Change which helper the handler exits through.
  After a nil engine error, count the entries whose `Error` is non-empty as `localFailed`, the entries that reached the local deletion at all — neither protected nor dry-run, which is exactly the entries with a non-empty `Error` or with `Deleted == true` — as `attemptedLocal`, the entries whose `RemoteError` is non-empty as `failed`, and the entries with `Deleted == true` as `attempted`.
  `Deleted == true` is the remote-attempted set and needs no new field: the local-first-then-remote rule means a remote deletion is tried exactly when the local one succeeded, and a protected, skipped, or dry-run entry never has `Deleted` set.
  When either count of failures is non-zero, exit through `errWithRecordFields` with the same fields map and a synthesised error;
  otherwise exit through `okWithRecord` as today.
  A pre-existing non-nil engine error keeps its current `errWithRecord` path unchanged.

  The synthesised error is a summary, not a transcript — the per-branch reasons already ride `entries[].remote_error` and `entries[].error`, so repeating them in the error string would duplicate the envelope's own content.
  Build it with `fmt.Errorf` in exactly one of three shapes, composed from the four counts above so a run with one failing class never emits the other half's wording.
  Remote failures only:

```
remote branch deletion failed for %d of %d orphan branches (%s); each branch's reason is in entries[].remote_error
```

  with `failed`, `attempted`, and the failed branch names joined with `", "` in enumeration order, so the string is deterministic across runs.
  Local failures only:

```
branch deletion failed for %d of %d orphan branches (%s); each branch's reason is in entries[].error
```

  with `localFailed`, `attemptedLocal`, and the locally-failed branch names joined the same way.
  It never mentions the remote.
  Both classes in one run: `errWithRecordFields` takes one `error`, so join the two into one string with `fmt.Errorf("%v; additionally, %v", localErr, remoteErr)`, each half being the exact string its own format above produces, local first.
  The joined string adds no content of its own.

  A non-empty `RemoteSkippedReason` never feeds any of these counts and never selects the failure exit: a missing `origin` exits 0, uniformly across both verbs, because there was no remote to delete from and the local work completed exactly as specified.

  Keying the failure exit on `Error` as well as `RemoteError` is the deliberate existing-path change this batch's scope names.
  Record it in a comment at the condition, with the reason: `Cleanup` sets no `Error` on a protected or unmanaged entry, so its `Error` is always a genuine failure and never a designed refusal — unlike `prune`'s, which stays in `doc.go`'s carve-out untouched.
  Leave `runPruneWithFlags` alone.
- **Commit:** `feat(fabric): exit non-zero when cleanup's local or remote branch deletion fails`

### Card 15: runRemoveWithFlag — remote threading, envelope keys, failure exit

- **Context:**
  - `internal/fabricengine/remove.go`
  - `internal/fabriccli/envelope.go`
- **Edits:**
  - `internal/fabriccli/fabric.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Widen `runRemoveWithFlag` to take a trailing `remote bool` and pass it through to `Remove` as its new fourth argument, replacing card 10's interim literal `false`.

  This handler hand-builds its fields map and never marshals `RemoveResult` at all, so every new field is invisible unless the map names it.
  Add three keys to the existing `"slug"`, `"path"`, and `"links_removed"` set, spelled to match the struct tags: `"remote_branch_deleted"` from `RemoteBranchDeleted`, `"remote_branch_error"` from `RemoteBranchError`, and `"remote_skipped_reason"` from `RemoteSkippedReason`.
  The map already emits its three existing keys unconditionally;
  emit these three the same way, on both the success and the failure exit, whatever the struct's `omitempty` tags say.

  After a nil engine error, exit through `errWithRecordFields` with that same fields map whenever `RemoteBranchError` is non-empty, and through `okWithRecord` otherwise.
  Key the exit on `RemoteBranchError` alone — never on `RemoteSkippedReason`, so that a missing `origin` produces exit 0 here exactly as it does from `cleanup`, and the identical configuration state never yields two different verdicts across the two verbs.
  A pre-existing non-nil engine error keeps its current `errWithRecord` path unchanged.

  There is one branch and therefore no aggregation, so the synthesised error inlines its reason rather than pointing at an array.
  Build it with `fmt.Errorf` in exactly this shape:

```
weft branch %q was deleted locally, but its copy on %q was not: %s
```

  with the weft branch name, the remote name, and `RemoteBranchError`.

  The remote name is the literal `"origin"`, hardcoded here at the `fabriccli` site.
  `originRemoteName` is unexported in `internal/fabricengine`, so this package cannot read it, and it stays unexported: exporting a constant purely to spell one error string would widen the engine's API for no caller that needs it.
  Hardcoding the literal matches the discussion's `hardcoded-origin` decision, which fixes `origin` throughout fabric's geometry and rejects making it configurable, so the two spellings cannot drift into disagreement about a value neither side can change.

  The weft branch name is not currently in scope in this handler — derive it the same way the engine does, or read it from the result, rather than reconstructing a suffix by hand.
  If neither is available without widening `RemoveResult` further, report that as a plan defect rather than hand-spelling the suffix.
- **Commit:** `feat(fabric): report and fail on remove's remote branch deletion outcome`

### Card 16: CLI envelope and flag-matrix tests

- **Context:**
  - `internal/fabriccli/fabric.go`
  - `internal/fabriccli/envelope.go`
  - `internal/fabriccli/envelopecontract_integration_test.go`
  - `internal/fabriccli/testmain_test.go`
  - `internal/fabricengine/cleanup.go`
  - `internal/fabricengine/remove.go`
  - `internal/fabricengine/branchname.go`
  - `internal/hubforge/hub.go`
  - `CONSTRAINTS.md`
- **Edits:** none
- **Creates:**
  - `internal/fabriccli/remoteenvelope_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create the file with a leading `//go:build integration` line, a file-header comment naming what it covers, and `package fabriccli_test`, matching `internal/fabriccli/envelopecontract_integration_test.go`, which is the existing model for every assertion here since it covers exactly the per-item-failure defect these decisions follow.
  Drive the real CLI through the same `RunCLIIn` seam that file uses, build every hub through `hubforge.NewHub`, and add no `TestMain` — the package already has one.

  Six scenarios:

  1. `--remote` alone on cleanup, without `--apply`, performs no deletion on either side and exits 0.
     This is the flag-matrix corner an operator is most likely to get wrong, and the one the help text now promises explicitly.
  2. `remove --remote`'s success envelope carries `remote_branch_deleted`.
     This is the regression guard for the hand-built fields map, which would otherwise drop every new field silently while the struct still declared them.
     Assert the key's presence, not only its value, so a dropped key fails rather than reading as false.
  3. A cleanup run where one entry carries a `RemoteError` exits non-zero, emits `"ok":false` and `"partial":true`, and still carries the full `entries` array with that entry's `remote_error` populated.
     Induce the failure by pointing the weft repo's `origin` at a filesystem path that does not exist, as batch 3's engine tests do.
     Assert the envelope carries no `refusal` key — a synthesised `fmt.Errorf` can never match `RefusalOf`, and claiming a gate refusal would misreport which layer said no.
  4. A cleanup run whose entries are all `Protected` — primary weft, checked out, or unmanaged — and therefore carry no `Error` at all still exits 0.
     Assert `Protected` with an EMPTY `Error`: `Cleanup` never sets `Error` on a protected or unmanaged entry, so a test asserting a protected entry with a non-empty `Error` would be asserting a state the verb cannot produce.
  5. A cleanup run where one entry carries a local `Error` — the `git branch -D` itself failed — exits non-zero.
     This is the one existing-path change this task makes deliberately, and it needs its own pinned test because nothing else in the suite would notice the verdict flipping back.
  6. A `prune` run with a `Protected` or `Unowned` entry still exits 0 — the regression guard that `prune`'s carve-out survived untouched while `cleanup` left it.

  Cover the no-`origin` path from the CLI on both verbs: a weft repo with its remote removed, run under `cleanup --apply --remote`, exits 0 with the reason in `remote_skipped_reason` and no `entries[].remote_error`;
  the same condition under `remove --remote` exits 0 with the reason in the same key.
  Both halves are needed — an asymmetric exit code for one configuration state across the two verbs is the defect that decision exists to prevent, and only a CLI-level test can observe an exit code at all.

  Derive weft branch names through `WeftBranchName` rather than hand-spelling a suffix.
- **Commit:** `test(fabric): pin the --remote envelope, exit codes, and flag matrix at the CLI`

### Card 17: Roadmap move

- **Context:**
  - `manifest/roadmap.md`
  - `CLAUDE.md`
- **Edits:**
  - `manifest/roadmap.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Move the **fabric: no remote/GitHub branch deletion** item out of the `## Planned` section and into `## Done`, following that file's own Maintenance rules: every item is written literally as `1.`, numbering is automatic and restarts per section, so no number anywhere is edited.

  Rewrite the entry's body for its new section.
  A Done entry states what shipped and points at the module's own package documentation rather than at a design doc — this item never had a `designs/` doc, so there is none to delete.
  Keep it to the section's own length discipline: a bold item name plus one or two sentences.
  The name stays byte-identical so existing cross-references by bold item name keep resolving.
  The sentences should name the opt-in `--remote` flag on `lyx fabric cleanup` and `lyx fabric remove`, and point at the `internal/fabricengine` package documentation.

  Place it among the existing Done entries in whatever position that section's ordering implies;
  the section carries no explicit ordering rule beyond the numbering note, so appending at the top of the list alongside the other recently-cleared items is correct.

  Write it with the repo's semantic-line-break style.
  Change nothing else in the file — no other entry moves, and no section heading changes.
- **Commit:** `docs(fabric): move the remote branch deletion item to Done`

## Batch Tests

`verify:` runs `go test ./internal/fabriccli/... ./cmd/lyx/...` and then `go test -tags integration ./internal/fabriccli/...`.

The untagged half covers the compile surface of cards 13 through 15 plus the `cmd/lyx` help-tree, `--json` help-schema, and registration tests, which are the ones that see the two new flags and the three changed `Use`/usage strings.
Those tests assert supersets and read the live flag set, so they are expected to pass without an edit of their own — but they are the tests that would catch a `Use` string that no longer parses or a command left without a `Short`, so running them here is not optional.

The `-tags integration` half on `internal/fabriccli` is where card 16's new file actually executes, and where every exit-code assertion in this batch lives — an exit code is observable only through the CLI seam, which is why none of these scenarios could be covered in batch 3.

Card 17 has no runnable surface;
it is covered by the untagged half only in the sense that it changes no Go file at all.
