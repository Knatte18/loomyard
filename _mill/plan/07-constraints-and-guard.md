# Batch: constraints-and-guard

```yaml
task: "Rename the fabric host vocabulary to warp, and name the composite repo Fabric"
batch: "constraints-and-guard"
number: 7
cards: 5
verify: go test ./... && go test -tags integration ./...
depends-on: [6]
```

## Batch Scope

The judgment core of the task and its only structural change: `CONSTRAINTS.md`'s Fabric Vocabulary Invariant is rewritten, its other retired-identifier citations are updated, and `internal/lyxcwd/enforcement_test.go`'s guard is **tightened** so a fabric-sense `host` now fails inside the owner dirs too.

Everything before this batch was a cleanup.
This batch is what stops it being a one-off: today the owner dirs are skipped entirely by the host half of `TestEnforcement_FabricVocabulary`, so nothing machine-proves the rename stays done and a future card could reintroduce `hostBranch` freely.
After batches 2 and 3 the fabric packages contain zero "host" in any sense, so the tightened rule has no false positives left to accommodate.

Two files in this batch are the reason the whole task has a "never run over" list: `CONSTRAINTS.md` and `internal/lyxcwd/enforcement_test.go` **name the retired vocabulary in order to forbid it**.
Their phrase lists, identifier lists and sense-discrimination fixtures must keep the word `host`, letter for letter.
Neither file has ever been passed to `wordswap` and neither is passed to it here — every edit in this batch is by hand.

The tightening is a compile-and-pass gate, not a preference: `internal/weftname/weftname.go` and `internal/boardengine/board.go` carry policed host phrases today, which is why batch 3's sweep had to include them.
Their fixes landed in batch 3;
this batch is where the guard that requires those fixes arrives.

## Cards

### Card 21: rewrite the Fabric Vocabulary Invariant

- **Context:**
  - `internal/lyxcwd/enforcement_test.go`
  - `_mill/discussion.md`
- **Edits:**
  - `CONSTRAINTS.md`
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Rewrite `CONSTRAINTS.md`'s `## Fabric Vocabulary Invariant` section (currently lines 157 through 172) in full, rather than appending bullets.
  The retirement of `host`, the Fabric name, and the warp/weft CLI carve-out are three faces of one rule;
  expressed as accreted bullets the next reader has to reconstruct it.

  The rewritten invariant must state all six of the following, and must not overclaim beyond them:

  1. **Fabric** (capital F) is the name of the fully wired-up composite — the warp repo with junctions into weft inside it.
     Any reader meaning *the repo as a whole* says Fabric.
  2. **warp** and **weft** name the two sides and are used — including in CLI help text and user-visible messages — at exactly those points where the two sides genuinely must be told apart, e.g. `lyx fabric clone <warp-url> <weft-url>` and `fabric: warp/weft out of sync`.
     "repo" alone is too vague to denote warp and is never a substitute for it.
  3. **`host` is retired** and is never used in any of these senses, anywhere — including inside the owner set.
  4. The phrase predicate is **retained**, unchanged, as the sense-discriminator: `host` is policed via the fabric-sense phrase list (`host repo`, `host repository`, `host worktree`, `host working tree`, `host checkout`, `host branch`, `host junction`, `host path`, `host side`, `host HEAD`, any case, hyphenated or spaced) plus the policed geometry identifiers (`hostBranch`, `hostLayoutFor`, `hostReason`, `HostJunction`, `hostClean`), never as a bare word.
     The bare word — verb sense, machine/OS sense, and the PowerShell `Write-Host` cmdlet — still passes untouched, because a whole-word ban would rewrite ordinary English in modules with no connection to fabric.
     Keep these lists verbatim: they are the ban list, and renaming them would delete the rule.
  5. **The owner set carves out the bare weft/warp rule only, never the host rule.**
     Owner set: `internal/fabricengine`, `internal/fabriccli`, `internal/weftname`, `internal/lyxtest`, `internal/boardengine`, `internal/configsync` (string literals and comments, never identifiers).
     **Drop `tools/` and `sandbox/` from the owner set** and record instead that they lie outside the enforcement walk entirely — the Go walk covers `internal/` and `cmd/` only, so an owner-map row for them would be dead code that never matches, and calling them "owners" implies a carve-out from a check that never reaches them.
     Vocabulary in `tools/` and `sandbox/` is a review obligation.
  6. **What the machine check does and does not reach — state it honestly, do not imply full coverage.**
     Production Go under `internal/` and `cmd/` is machine-guarded, plus an `internal/**/*.md` walk and the embedded agent prompt templates.
     `*_test.go` files are excluded from all three rules.
     `hostGeometryIdentifiers` is five exact lowercased names, so `HostJunctions`, `hostPath`, `hostBare`, `CopyHostHub` and `HostFixture` are matched only by the phrase half, and only where they occur inside a policed phrase.
     Test files, documentation outside `internal/`, shell, and `tools/` remain a **review obligation**, not a machine check.
     Keep the existing prose-doc-split bullet, extended with the Fabric rule from point 1.

  Then update `docs/overview.md` line 80, which restates this invariant's ban list and owner set, so the two agree exactly — in particular it must lose `tools/` and `sandbox/` from the owner set it lists, and must reflect that the host rule no longer carves out the owner dirs.
  This is why batch 6 card 17 deliberately left that one line alone.

  Follow the repo's semantic-line-break markdown rule throughout both files.
- **Commit:** `docs(constraints): rewrite the Fabric Vocabulary Invariant for the warp/Fabric rule`

### Card 22: update the remaining retired-identifier citations in `CONSTRAINTS.md`

- **Context:**
  - `docs/shared-libs/lyxcwd.md`
  - `internal/fabricengine/junction.go`
  - `_mill/discussion.md`
- **Edits:**
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Five occurrences outside the Fabric Vocabulary Invariant still name the retired vocabulary.
  The **semantics** of every invariant below are unaffected by this task — no path, anchor, or cwd-resolution behaviour changes — but their identifier citations and prose are not.

  - Line 26, the **Cwd Resolution Invariant** — `` `WeftWorktree`/`WeftRepoRoot`/`HostLyxLink`/`HostJunctions`/portal and launcher paths `` becomes `WarpLyxLink`/`WarpJunctions`, matching the symbols batch 3 renamed.
    `docs/shared-libs/lyxcwd.md` line 82 is this bullet's doc mirror and was already updated in batch 6 card 15;
    read it and make the two agree.
  - Line 175, the **Fabric Git Invariant** heading paragraph — "on **either** the weft repo or the warp/host repo" becomes "on **either** the weft repo or the warp repo", dropping the retired alias rather than keeping both names.
  - Line 180 — "coordinated host↔weft topology" becomes "coordinated warp↔weft topology".
  - Line 188 — "An agent does commit its own code to the **host** repo (commit-per-fix) — the weft, never." becomes "**warp** repo".
    This is a genuine two-sided distinction, so warp is correct here rather than Fabric.
  - Line 200 — "**Unwire** removes host junctions and their warp `.git/info/exclude` entries only" becomes "removes warp junctions".

  One further occurrence lives outside the Fabric Git Invariant and must not be missed: line 214, in the **Review Round Invariant** — "commit-per-fix on host source, never push" becomes "commit-per-fix on warp source, never push".
  `_mill/discussion.md` lists this line under the Fabric Git Invariant;
  that attribution is wrong, the line is correct to change, and it is the last retired citation in the file.

  Do not touch lines 160 and 161.
  They are the ban list — the phrase list, the identifier list, and the sentence explaining that the bare word passes untouched — and card 21 keeps them verbatim for that reason.
- **Commit:** `docs(constraints): rename retired host citations in the cwd, git and review invariants`

### Card 23: tighten the enforcement guard's host half in both walks

- **Context:**
  - `CONSTRAINTS.md`
  - `internal/weftname/weftname.go`
  - `internal/boardengine/board.go`
- **Edits:**
  - `internal/lyxcwd/enforcement_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Remove the owner-dir skip from the **host half** of `TestEnforcement_FabricVocabulary`, in both walks, leaving the bare weft/warp owner skip untouched in both.

  In the Go walk (`internal/` and `cmd/`, `.go`), line 886 currently reads:

```
			if !fabricVocabularyOwners[dir] && hostHit {
```

  Change it so the owner check no longer gates the host hit — the condition becomes `hostHit` alone.

  In the `internal/**/*.md` walk, line 903 currently reads:

```
			if !fabricVocabularyOwners[dir] && fabricSenseHostPhrase(text) {
```

  Change it the same way, to `fabricSenseHostPhrase(text)` alone.
  This half is free: no owner directory contains a `.md` file, verified by enumeration, so the tightened rule cannot fire there today.

  Leave line 883's bare-token condition (`!shouldSkipBareVocabularyCheck(dir) && failsBareVocabularyCheck(...)`) and line 900's `!fabricVocabularyOwners[dir] && bareVocabularyToken(text)` exactly as they are.
  The owner set continues to carve out weft/warp;
  it stops carving out host.

  Three existing sites are **falsified** by this change and must be edited in the same card, not merely left alone:

  - The `fabricVocabularyOwners` doc comment at lines 591 through 595, which claims "Card 26 scopes the host-phrase rule to 'the same files' as the bare-token rule, so both rules share this one owner set".
    Rewrite it to say the owner set governs the bare weft/warp rule only, and that the host rule applies everywhere including the owner dirs.
  - The `TestEnforcement_FabricVocabulary` doc comment at lines 734 through 744, which restates the same shared-owner-set claim.
  - The `owner_set_file_with_all_of_the_above_passes` sub-test at lines 787 through 798.
    Its comment says an owner file "is skipped outright, for both the bare-token rule and the host-phrase rule", and its second assertion (`if !fabricVocabularyOwners["internal/fabricengine"] { … "expected internal/fabricengine to skip the host-phrase check entirely" }`) asserts precisely the behaviour being removed.
    Rename the sub-test to reflect what it now proves (e.g. `owner_set_file_skips_bare_token_rule_only`), rewrite the comment, and replace the second assertion with one proving the host half is **no longer** skipped.

  Do **not** rename `hostPhrases`, `hostGeometryIdentifiers`, `fabricSenseHostPhrase`, or any of their values.
  They name the retired vocabulary in order to forbid it;
  renaming them would delete the rule this card exists to strengthen.
- **Commit:** `test(lyxcwd): tighten the fabric-vocabulary guard so host fails inside owner dirs`

### Card 24: add the cases proving the tightened guard

- **Context:**
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/lyxcwd/enforcement_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add sub-tests under `TestEnforcement_FabricVocabulary`'s existing `predicate` group proving the four properties the tightening is supposed to have, using the file's existing `parseWithComments` fixture helper rather than touching the real tree:

  - A fabric-sense host phrase inside an **owner** directory now **fails**, where it previously passed.
    Drive it the way the tree-scan does — `fabricVocabularyHits` on a fixture containing a policed phrase, then assert the walk's new condition treats the hit as a failure for `"internal/fabricengine"`.
  - The bare weft/warp owner skip is **unchanged**: `shouldSkipBareVocabularyCheck("internal/fabricengine")` still reports true, so an owner dir may still say warp/weft freely.
  - `internal/configsync`'s narrower literal-and-comment carve-out is unaffected: the existing `configsync_row_passes_on_literal_and_comment_but_fails_on_identifier` sub-test still passes, and `configsync` still fails on a bare-token identifier.
  - The sense discrimination still holds: the existing `host_repo_phrase_fails`, `host_verb_sense_passes`, `host_machine_sense_passes` and `write_host_cmdlet_passes` sub-tests continue to pass untouched.
    They are what keeps the tightened rule from firing on `internal/reedengine`'s "can host a strand" or `internal/reedcli`'s `Write-Host`.

  Then run the full tree-scan and confirm it is green against the real repository.
  It should be, because batches 2 and 3 left zero `host` of any sense in the six owner directories and left every non-owner production file free of a bare `warp` token;
  if it fails, the failing path is a file one of those batches missed, and it is fixed there in spirit — correct the file, do not weaken the guard.
- **Commit:** `test(lyxcwd): prove the tightened host rule fires inside owner dirs`

### Card 25: repo-wide completeness check

- **Context:**
  - `CONSTRAINTS.md`
  - `internal/lyxcwd/enforcement_test.go`
  - `_mill/discussion.md`
  - `cmd/lyx/crosscompile_test.go`
  - `cmd/lyx/gitrepoboundary_test.go`
  - `docs/benchmarks/fixture-copy.md`
  - `docs/benchmarks/test-suite-timing.md`
  - `docs/overview.md`
  - `docs/research/linux-portability-survey.md`
  - `docs/research/scout-spike.md`
  - `docs/shared-libs/yamlengine.md`
  - `internal/boardengine/boardtest/concurrency_test.go`
  - `internal/buildercli/poll.go`
  - `internal/buildercli/poll_test.go`
  - `internal/builderengine/spawn.go`
  - `internal/builderengine/spawn_test.go`
  - `internal/configengine/config_test.go`
  - `manifest/designs/fabric-unified-view.md`
- **Edits:** none
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Verification only — this card changes no file.

  Grep the whole repository, case-insensitively, for any remaining `host` and confirm every surviving hit falls in the exclusion list.
  The exclusion list is exactly two things: the "never run over" set, and the machine-/verb-sense keeps.

  Surviving hits are expected, and only these:
  - `CONSTRAINTS.md` lines 160 and 161, and `internal/lyxcwd/enforcement_test.go`'s `hostPhrases`, `hostGeometryIdentifiers`, `fabricSenseHostPhrase` and its sense-discrimination sub-tests — the ban list.
  - `docs/overview.md`'s restatement of that ban list.
  - `manifest/designs/fabric-unified-view.md` line 122 ("a fabric-sense `host`"), same reason.
  - The four historical-record docs: `docs/benchmarks/test-suite-timing.md`, `docs/benchmarks/fixture-copy.md`, `docs/research/scout-spike.md`, `docs/research/linux-portability-survey.md`.
  - `crucible/` — all five hits are machine-sense ("exhaust the host's RAM", "a host process killed") and must survive verbatim.
  - `internal/buildercli/poll.go` line 321 and `internal/buildercli/poll_test.go` line 212 — the one deliberate verb-sense keep, recorded in batch 3's `-skip` set.
  - `internal/builderengine/spawn.go` lines 9, 178, 236 and 277, and `internal/builderengine/spawn_test.go` line 582 — machine-sense and verb-sense.
  - `internal/configengine/config_test.go`'s `server:` / `host: localhost` YAML fixture.
  - `internal/boardengine/boardtest/concurrency_test.go` line 111 ("an unthrottled host"), `cmd/lyx/crosscompile_test.go` line 5, `cmd/lyx/gitrepoboundary_test.go` lines 13 and 21.
  - Everything in `_mill/`, which is task state and not repository content.
  - The machine-sense and unrelated set enumerated in `_mill/discussion.md`'s "Exhaustive repo-wide classification" table — `plugins/prowler`, `internal/reedengine`, `internal/reedcli`, `internal/treadleengine`, `internal/shuttlecli`, `internal/burlerengine`, `internal/shuttleengine`, `internal/shell`, `internal/stencil`, `internal/modelspec`, `internal/boardcli`, `internal/githubclient`, `docs/reference/`, `docs/research/`, `docs/shared-libs/yamlengine.md`, `go.mod` and `go.sum`.

  Any hit outside that list is a miss: fix it, then re-run.

  This check is and stays **manual**.
  The tightened guard covers production Go under `internal/` and `cmd/` plus `internal/**/*.md` only — never `*_test.go`, never `docs/`, never `tools/`, never shell.
  Do not describe the tightened test as encoding this check permanently, in a commit message or anywhere else;
  it does not.
  Record the outcome in the batch notes;
  make no code change.
- **Commit:** none

## Batch Tests

`verify: go test ./... && go test -tags integration ./...` is the unbounded repo-wide suite in both tiers, and this is the second and last batch where that scope is justified.

The justification is that cards 23 and 24 change a **repo-wide enforcement test**.
`TestEnforcement_FabricVocabulary` walks every production `.go` file under `internal/` and `cmd/` plus every `internal/**/*.md` file;
tightening it changes the verdict for six owner directories at once, and the only way to know the new rule holds is to run it against the whole tree.
A scoped `verify` on `./internal/lyxcwd/...` would run the guard itself but would not catch the second-order failures — a package that only now trips the rule.
The integration-tagged pass is included because it is the last batch of the task and the final gate before handoff.

The specific gates that matter here:

- `TestEnforcement_FabricVocabulary` must pass **with the tightened rule**, which is the whole point of the batch.
  It passing is the machine proof that batches 2 and 3 left zero fabric-sense `host` in `internal/fabricengine`, `internal/fabriccli`, `internal/weftname`, `internal/lyxtest`, `internal/boardengine` and `internal/configsync`.
- Its sense-discrimination sub-tests must still pass unchanged, proving the tightening did not turn into a bare-word ban that would fire on `internal/reedengine`'s "can host a strand" or `internal/reedcli`'s `Write-Host`.
- `TestEnforcement_GeometryLiterals` and `TestLeafInvariant_AllowlistOnly` in the same package must stay green, confirming card 23's edits did not disturb the shared `walkEnforcementRoots` helper they all use.

Card 25's completeness grep is deliberately outside the test suite and stays that way — it is a review obligation the guard's bounded reach cannot absorb.
