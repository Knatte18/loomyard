# Batch: checked-call-invariant-and-docs

```yaml
task: 'gitexec: add the checked entry point and migrate the call sites'
batch: checked-call-invariant-and-docs
number: 8
cards: 6
verify: go test ./... && go test -tags integration ./...
depends-on: [2, 6, 7]
```

## Batch Scope

This batch closes the task: it adds the guard that makes a raw site a deliberate, reviewed act, teaches the three existing token guards about the new entry point, records both invariants in `CONSTRAINTS.md`, and completes the documentation lifecycle by deleting the design doc and moving the roadmap entry to `## Done`.
It depends on batches 2, 6, and 7 because the new guard asserts the invariant holds on the tree **as migrated** — it cannot pass until every call site has moved and every raw site carries its marker.
Its `verify:` is the only unbounded one in the plan, deliberately: this is the batch whose whole subject is repo-wide guards.

Batch-local decisions beyond `## Shared Decisions`:

- The new guard and the three token guards use **opposite** spellings of the same substring, and neither may be harmonised with the other.
  The three token guards ban *any* git spawn, so the shorter `gitexec.Run` prefix is right for them: its extra matches are exactly what they want.
  The new guard distinguishes raw from checked, so for it the same prefix's extra matches are exactly what it must not flag.
  That contrast must be written into the new guard's header comment, because a later reader comparing the four files will otherwise "fix" one spelling to match the others and silently invert a guard.
- A package with no key in the pinned map is treated as pinned zero, and a raw site found there is a guard failure.
  The alternative — an unlisted package silently passing — would mean the guard covers only the packages that happened to have raw sites the day it was written, and a brand-new package reaching for `RunGit` is the case the invariant most needs to catch.
- The design-doc deletion and the roadmap link removal are one commit, because Markdown Link Integrity fails on a dangling relative link.

## Cards

### Card 33: the Checked-Call Invariant guard

- **Context:**
  - `cmd/lyx/gitrepoboundary_test.go`
  - `internal/gitrepo/noforceadd_test.go`
  - `cmd/lyx/rawgitmutation_test.go`
  - `cmd/lyx/tierpurity_test.go`
  - `cmd/lyx/hermeticenv_test.go`
  - `cmd/lyx/destructiveguard_test.go`
  - `internal/gitrepo/pull.go`
  - `internal/gitrepo/gitrepo.go`
  - `internal/fabricengine/weftwiring.go`
- **Edits:** none
- **Creates:**
  - `cmd/lyx/checkedcall_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create the guard as an untagged `package main` test file in `cmd/lyx`, mirroring the structure `cmd/lyx/gitrepoboundary_test.go` and `internal/gitrepo/noforceadd_test.go` already use: skip cleanly when the go toolchain is not on PATH, resolve the module root via `go env GOMOD`, and walk every non-test `.go` file under both `internal/` and `cmd/`.
  It matches two raw tokens by raw substring, and both must be spelled exactly as given, for opposite reasons: `gitexec.RunGit` **without** the paren (the shorter `gitexec.Run` would match every migrated call and demand a marker at all of them), and `r.run(` **with** the paren (`r.runChecked(` contains the substring `r.run`, so an unparenthesised token would demand a marker at all nineteen migrated `gitrepo` sites).
  For every line matching either token, assert an adjacent `//gitexec:raw` marker is present — on the same line or on the line immediately above.
  Separately, pin a per-package count of raw sites in a literal map: `internal/gitrepo` 3, `internal/fabricengine` 2, `internal/lyxcwd` 0, `internal/fabriccli` 0, `internal/websterengine` 0.
  Treat a package with no map key as pinned zero and fail on any raw site found there, with the message naming the package, its raw-site count, and the two-line remedy (add the marker, add the map entry).
  Carry a `checkedCallMinScannedFiles` vacuous-scan floor, for the same reason both sibling guards carry one.
  Write the header comment to state: what the invariant is, that test files are exempt from the marker requirement, the two-token spelling contrast against `cmd/lyx/tierpurity_test.go`, `cmd/lyx/hermeticenv_test.go`, and `cmd/lyx/rawgitmutation_test.go`, why the walk covers `cmd/` even though zero production sites live there today, and the known blind spot — a raw call slipped into an already-marked region, or an alternative spelling the substring match misses — which is the same class `cmd/lyx/destructiveguard_test.go` and `cmd/lyx/gitrepoboundary_test.go` already name.
  Add a subtest asserting the token spellings themselves: that the `r.run(` token does not match a `runChecked` call, and that the `gitexec.RunGit` token does not match a `gitexec.Run` call.
  That is the one place a plausible-looking edit silently inverts the guard.
- **Commit:** `test(cmd/lyx): add the gitexec Checked-Call Invariant guard`

### Card 34: teach the three token guards the shorter prefix

- **Context:**
  - `cmd/lyx/checkedcall_test.go`
  - `cmd/lyx/testmain_test.go`
  - `internal/websterengine/gitwrap.go`
- **Edits:**
  - `cmd/lyx/tierpurity_test.go`
  - `cmd/lyx/hermeticenv_test.go`
  - `cmd/lyx/rawgitmutation_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Replace the `"gitexec.RunGit"` entry with the shorter prefix `"gitexec.Run"` in `bannedTokens` (`cmd/lyx/tierpurity_test.go`), in `gitSpawnTokens` (`cmd/lyx/hermeticenv_test.go`), and — as `"gitexec.RunGit("` today — in `rawGitMutationBannedTokens` (`cmd/lyx/rawgitmutation_test.go`).
  All three match by raw substring and their own header comments already justify prefix matching in exactly these terms, so one token covers both entry points and no set can go half-updated later.
  Add an `allowedSpawners` entry in `cmd/lyx/tierpurity_test.go` for `cmd/lyx/checkedcall_test.go` with a one-line reason, alongside the existing entries for `gitrepoboundary_test.go`, `rawgitmutation_test.go`, `destructiveguard_test.go`, `ghguard_test.go`, and `boardguard_test.go`: it carries the banned token strings as its own scan data and resolves its scan root via `go env GOMOD`.
  Reword `cmd/lyx/rawgitmutation_test.go`'s allowlist reason for `internal/websterengine/gitwrap.go`, which names `gitexec.RunGit`, since that file now uses the checked entry point and remains allowlisted.
  Check `cmd/lyx/hermeticenv_test.go`'s own `allowedNonHermetic` map: package `cmd/lyx` already carries the hermetic presence token via `cmd/lyx/testmain_test.go`, so no new entry should be needed — confirm that by running the guard, and add a file-level entry only if it actually fails.
  Update each guard's header comment where it names the old token.
- **Commit:** `test(cmd/lyx): key the three token guards on the gitexec.Run prefix`

### Card 35: record both invariants in CONSTRAINTS.md

- **Context:**
  - `cmd/lyx/checkedcall_test.go`
  - `cmd/lyx/gitrepoboundary_test.go`
  - `internal/gitrepo/gitrepo.go`
  - `internal/gitrepo/pull.go`
- **Edits:**
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add a new `## gitexec Checked-Call Invariant` section stating: `gitexec.Run` / `runChecked` is the default and every remaining raw `gitexec.RunGit` or `r.run` call site in non-test source carries an adjacent `//gitexec:raw — <why the raw form is correct here>` marker;
  the justification must be true, and the two truthfully-markable classes are a pure predicate whose signature has no error channel and a test-pinned deliberate-suppression contract;
  the per-package pinned counts, listed in full including the explicit zeros;
  that a package with no entry is pinned zero;
  that test files are exempt from the marker requirement;
  the known blind spot, stated honestly, matching the wording in the guard's own header;
  and an `**Enforced by** cmd/lyx/checkedcall_test.go` line in the same shape every other section uses.
  Amend the existing `## gitrepo Client Boundary Invariant` section: replace the "exactly one `gitexec.` occurrence" claim with the two-call-expression assertion the repaired guard now makes, and note that the pinned method set is keyed on `r.run` and `r.runChecked` together.
  Confirm the CLI-bound method list is still accurate — the migration changed which helper each method calls, not which methods reach the CLI.
  Add a one-line cross-reference in each direction between the two sections: the Client Boundary Invariant answers *which methods may reach the git CLI at all* and is keyed by method name;
  the Checked-Call Invariant answers *which call sites may use the raw form* and is keyed by call site.
  A new CLI call inside an already-pinned method trips the second and not the first;
  a new method reaching the CLI trips both.
  Update the two other sections that name the old token: the `## Test Tier Purity Invariant` bullet listing `gitexec.RunGit` among the banned tokens, and the `## Hermetic Git Test Environment Invariant` bullet defining a git-spawning package — both become `gitexec.Run`.
- **Commit:** `docs(constraints): add the Checked-Call Invariant and amend the Client Boundary one`

### Card 36: correct the remaining falsified prose

- **Context:**
  - `internal/gitexec/gitexec.go`
  - `internal/fabricengine/destroy.go`
  - `internal/gitrepo/doc.go`
  - `CONSTRAINTS.md`
  - `docs/overview.md`
  - `docs/shared-libs/lyxcwd.md`
- **Edits:**
  - `docs/shared-libs/README.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `docs/shared-libs/README.md`, correct the line describing `internal/gitexec` as the "windowless `RunGit` primitive", which names one entry point where there are now two: describe the pair and which is correct where, in one line matching the file's existing terse table style.
  Then re-run the prose regeneration sweep rather than trusting this hand-list — grep for `RunGit`, `gitexec`, `exit code`, and `exitCode` across the markdown files and `doc.go` files under `docs/`, `manifest/`, and `internal/`, plus `CONSTRAINTS.md`, excluding the `_mill/` tree, and read every hit.
  Correct any the change falsifies and leave the rest alone;
  `docs/overview.md`'s module table describes `internal/gitexec` as "shared git operations", which stays true, and `docs/shared-libs/lyxcwd.md`'s import-cap statement is unaffected because the cap names the package, not the function.
  Prose is the one inventory in this task with no compiler behind it, so a miss here survives silently — the sweep is the mechanism, the hand-list is only what it found today.
  The `internal/gitrepo/doc.go` corrections and the `internal/fabricengine/destroy.go` executor godocs were already made in batches 2 and 3;
  confirm them in the sweep rather than redoing them.
- **Commit:** `docs(shared-libs): describe gitexec's two entry points`

### Card 37: complete the documentation lifecycle

- **Context:**
  - `internal/gitexec/gitexec.go`
- **Edits:**
  - `manifest/roadmap.md`
- **Creates:** none
- **Deletes:**
  - `manifest/designs/gitexec-error-shape.md`
- **Moves:** none
- **Requirements:** Delete `manifest/designs/gitexec-error-shape.md` and, in the same commit, move `manifest/roadmap.md`'s "gitexec: checked entry point + call-site migration" entry from `## Planned` to `## Done`, removing its `See [designs/gitexec-error-shape.md](…)` line.
  Markdown Link Integrity fails on a dangling relative link, so these two edits cannot be split across commits.
  Do not delete the roadmap entry: this is a completed planned item, which is exactly what the `## Done` section exists for.
  Rewrite its wording to state what shipped rather than what was decided — the two entry points and which is correct where, `gitrepo`'s matching `run`/`runChecked` pair, and the Checked-Call Invariant as the mechanism keeping raw sites deliberate — following the phrasing style of the existing `## Done` entries.
  Drop the entry's stale "roughly 70 call sites" figure rather than carrying it into Done, and replace the design-doc link with a pointer to `internal/gitexec`'s package documentation, since that is where the durable rationale now lives.
- **Commit:** `docs(manifest): retire the gitexec design doc and move the roadmap entry to Done`

### Card 38: full-tree confirmation gate

- **Context:**
  - `cmd/lyx/checkedcall_test.go`
  - `cmd/lyx/destructiveguard_test.go`
  - `cmd/lyx/tierpurity_test.go`
  - `cmd/lyx/hermeticenv_test.go`
  - `cmd/lyx/rawgitmutation_test.go`
  - `cmd/lyx/gitrepoboundary_test.go`
- **Edits:** none
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Run the whole suite at both tiers and confirm, without editing anything, that: the five raw sites and only those five carry a `//gitexec:raw` marker;
  the per-package pinned counts in `cmd/lyx/checkedcall_test.go` match the tree;
  `cmd/lyx/destructiveguard_test.go` still passes, which proves batch 3's executor re-signature left every banned destructive argument slice inside `destroy.go` — confirm it, do not assume it;
  and no `//nolint:errcheck` comment remains beside a migrated `gitexec` call.
  This card is verification-only and produces no diff.
  If any of the four checks fails, do not patch it here — report the failure and which earlier card's requirement it contradicts, so the fix lands in the card that owns the file.
- **Commit:** none

## Batch Tests

`verify:` is the unbounded `go test ./...` followed by `go test -tags integration ./...`.
That scope is justified rather than lazy here: this batch's subject is four repo-wide guards, and three of them (`cmd/lyx/tierpurity_test.go`, `cmd/lyx/hermeticenv_test.go`, `cmd/lyx/rawgitmutation_test.go`) scan every package in the tree, so a narrower selection would not exercise what the batch changes.
It is also the last batch, where the whole migration's regression net should run once in full anyway.
The new `cmd/lyx/checkedcall_test.go` is Tier 1 and spawns no git beyond the `go env GOMOD` root resolution every sibling guard already uses.
`pipeline.done_gate` is already configured to the same repo-wide pair, so mill-go's own done gate re-runs this after the batch completes;
that repetition is intentional, not redundant — the done gate runs from the git root after every batch has landed.
