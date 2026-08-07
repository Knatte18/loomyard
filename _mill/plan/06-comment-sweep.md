# Batch: comment and test vocabulary sweep

```yaml
task: 'fabric: close the weft-visibility leak (slice 8)'
batch: 'comment and test vocabulary sweep'
number: 6
cards: 5
verify: go vet -tags integration ./... && go test ./cmd/lyx/
depends-on: [2, 3]
```

## Batch Scope

Rewords every remaining production `weft`/`warp`/fabric-sense-`host` mention outside the owner set — the files no code batch touches — plus the test-file hand-clean.
Owner set (vocabulary stays): `fabricengine`, `fabriccli`, `weftname`, `lyxtest`, `boardengine`, `configsync` (string literals), `tools/`, `sandbox/`.
Files already edited by batches 02/03/05 were reworded there;
this batch covers the remainder, so batch 07's enforcement test can be enabled against a clean tree.
Classification per decision `comment-fidelity`: sync-semantics comments substitute mechanically ("weft-synced" → "fabric-synced");
two-repo-mechanics comments reword case by case so the physical location information survives.
Carve-outs that stay verbatim everywhere: `WEFT_SKIP_GIT`/`WEFT_SKIP_PUSH` env-var names, the PowerShell cmdlet `Write-Host`, `internal/lyxtest` owner-API references (`WeftPrime`, `WeftBare`, `WeftPath`, `CopyWeft`, `CopyPaired`, …), and the `tools/sandbox` GitHub URLs/identifiers.

## Cards

### Card 20: websterengine, builderengine, batcher prose

- **Context:**
  - `_mill/discussion.md`
- **Edits:**
  - `internal/websterengine/doc.go`
  - `internal/websterengine/state.go`
  - `internal/websterengine/integration.go`
  - `internal/websterengine/beginbatch.go`
  - `internal/websterengine/recoverbatch.go`
  - `internal/websterengine/pause.go`
  - `internal/websterengine/awaitbatch.go`
  - `internal/builderengine/doc.go`
  - `internal/builderengine/state.go`
  - `internal/batcher/doc.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Reword every `weft`/`warp` comment mention and fabric-sense `host` phrase in the listed files: `websterengine/beginbatch.go:62` ("the host repo checkout" → "the repo checkout"), `:89` ("the host HEAD" → "the repo HEAD" or "HEAD"), `:222` ("fresh fork on the host repo" → "fresh fork on the repo");
  `websterengine/state.go:132` and `builderengine/state.go:95,104` ("the host HEAD" likewise);
  `websterengine/doc.go` (~7), `integration.go` (~5, minus what card 12 already reworded — grep, don't assume), `recoverbatch.go`, `pause.go`, `awaitbatch.go`;
  `builderengine/doc.go` (~9);
  `batcher/doc.go:9` ("host-side logic" → "orchestrator-side logic").
  Comments-only card: zero behavioural or identifier changes;
  `go build ./...` must be a no-op diff at the object level.
- **Commit:** `docs(websterengine,builderengine): fabric-vocabulary comment sweep`

### Card 21: cli-package prose

- **Context:**
  - `_mill/discussion.md`
- **Edits:**
  - `internal/webstercli/cli.go`
  - `internal/buildercli/cli.go`
  - `internal/buildercli/status.go`
  - `internal/perchcli/cli.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Reword every `weft`/`warp` mention and fabric-sense `host` phrase in the four listed files.
  These are the residual cli-package files that cards 7-9 do not already own — every `weftCommit`/`weftErr` call site, its `output.Err` string, and every cobra `Long` body carrying the vocabulary belongs to cards 7, 8, or 9, because the helper rename would otherwise break the package compile.
  Check each file for operator-visible text before assuming a hit is a comment: any `Short`/`Long`/flag-usage string that rewords falls under the CLI/Cobra Invariant's help-accuracy obligation, so re-read the surrounding command definition after editing.
- **Commit:** `docs(webstercli,buildercli,perchcli): fabric-vocabulary sweep`

### Card 22: low-level package prose

- **Context:**
  - `_mill/discussion.md`
- **Edits:**
  - `internal/lyxcwd/anchor.go`
  - `internal/lyxcwd/lyxcwd.go`
  - `internal/gitrepo/doc.go`
  - `internal/configengine/config.go`
  - `internal/scoutengine/daemonstate.go`
  - `internal/logger/sink.go`
  - `internal/reedengine/lifecycle.go`
  - `internal/reedcli/cli.go`
  - `internal/selfreportcli/cli.go`
  - `internal/shuttlecli/cli.go`
  - `internal/burlercli/cli.go`
  - `internal/treadleengine/doc.go`
  - `internal/treadleengine/run.go`
  - `internal/treadleengine/engine.go`
  - `internal/perchengine/doc.go`
  - `internal/perchengine/identity.go`
  - `internal/perchengine/engine.go`
  - `internal/configsync/configsync.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Comment-only reword per decision `comment-fidelity`.
  Sync-semantics mentions substitute mechanically ("the durable, weft-synced `_lyx`" → "the durable, fabric-synced `_lyx`").
  Named two-repo-mechanics cases handled individually: `lyxcwd/anchor.go:2,4,32,39` — "the weft:main root" → "the board root" (lossless: the marker lives at `<boardDir(hub)>/.lyx-anchor`, and `_board` is a geometry token `lyxcwd` co-owns);
  `gitrepo/doc.go:132,252` and `configengine/config.go:5` — reword case by case so the reader keeps the pointer to which physical location holds what, without the weft/warp words;
  `treadleengine/run.go:213`'s "the host repo's own build/test surface" → "the repo's own build/test surface".
  OS-sense `host` (e.g. `reedengine/lifecycle.go`'s "cannot host a strand") stays untouched — only the one weft/warp mention in that file rewords.
  `lyxcwd`'s import cap and code are untouched: comments only (Cwd Resolution Invariant).
  `internal/configsync/configsync.go` is carved out of the machine check for both literals and comments (see the overview's `vocabulary owner set and carve-outs` decision), so nothing here is load-bearing for batch 07 — but the finer distinction is a review obligation this card discharges.
  Leave `:24`'s `legacyFabricConfigModules = []string{"warp", "weft"}` and the comments at `:21`, `:36-37`, `:46`, `:160` verbatim: they name the on-disk legacy filenames `warp.yaml`/`weft.yaml` and the module names those literals hold, and cannot be reworded without becoming wrong.
  Reword only `:28`'s `// Module is the name of the config module (e.g., "board", "worktree", "weft")` — an illustrative example list, not documentation of the legacy literal — replacing `"weft"` with another live module name (e.g. `"fabric"`).
- **Commit:** `docs(internal): fabric-vocabulary comment sweep in low-level packages`

### Card 23: test-file hand-clean

- **Context:**
  - `_mill/discussion.md`
  - `CONSTRAINTS.md`
- **Edits:**
  - `cmd/lyx/boardguard_test.go`
  - `cmd/lyx/tierpurity_test.go`
  - `internal/webstercli/cli_test.go`
  - `internal/buildercli/poll_test.go`
  - `internal/buildercli/spawnbatch_test.go`
  - `internal/buildercli/validate_test.go`
  - `internal/buildercli/smoke_test.go`
  - `internal/buildercli/pause_spawnbatch_test.go`
  - `internal/buildercli/gitfixture_test.go`
  - `internal/builderengine/config_test.go`
  - `internal/builderengine/spawn_test.go`
  - `internal/builderengine/gitquery_test.go`
  - `internal/builderengine/runlevel_test.go`
  - `internal/websterengine/recordbatch_test.go`
  - `internal/websterengine/beginbatch_test.go`
  - `internal/websterengine/recoverbatch_test.go`
  - `internal/websterengine/config_test.go`
  - `internal/loomengine/config_test.go`
  - `internal/loomengine/preflight_integration_test.go`
  - `internal/websterengine/audit_test.go`
  - `internal/perchengine/run_test.go`
  - `internal/perchcli/cli_integration_test.go`
  - `internal/perchcli/run_test.go`
  - `internal/perchcli/run_integration_test.go`
  - `internal/configcli/configcli_integration_test.go`
  - `internal/gitrepo/commitempty_integration_test.go`
  - `internal/pattern/patternpath_test.go`
  - `internal/lyxcwd/geometry_test.go`
  - `internal/lyxcwd/anchor_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Hand-clean each listed test file of `weft`/`warp`/fabric-sense-`host` vocabulary that is NOT a reference to owner-package API.
  `internal/builderengine/runlevel_test.go:800` carries "the same host repo" in a comment and rewords to "the same repo".
  `internal/lyxcwd/anchor_test.go:203` names `fabricengine`'s `hostLayoutFor` in prose — that is an owner-package symbol reference, so keep the identifier spelled correctly and reword only any surrounding fabric-sense prose;
  it is listed here so card 24's gate has a classified owner rather than an unexplained hit.
  Specifically pinned by the discussion: `cmd/lyx/boardguard_test.go` calls the invariant "Weft Git Invariant" where `CONSTRAINTS.md` says "Fabric Git Invariant (warp + weft)" — align the name.
  KEEP verbatim: every `lyxtest` owner-API reference (`WeftPrime`, `WeftBare`, `WeftPath`, `CopyWeft`, `CopyPaired`, fixture struct fields), every `fabricengine`/`fabriccli` owner-API selector a test legitimately calls, `WEFT_SKIP_GIT`/`WEFT_SKIP_PUSH` env-var names wherever set (they are the literal names of variables this task deliberately does not rename), the PowerShell `Write-Host` cmdlet in `reedcli` `--cmd` strings (not in this card's list, but do not "fix" it if encountered), and `cmd/lyx/tierpurity_test.go`'s banned-token test data (its mentions are `lyxtest.Copy*` tokens carried as data — reword only genuine prose, if any).
  Two files carry vocabulary that cards 5 and 12 changed the code for but did not fully sweep, so they land here — this card is their only owner:
  `internal/loomengine/preflight_integration_test.go` (~40 hits) — the test name `TestPreflight_WeftWorktreeRemoved` renames to `TestPreflight_FabricNotReady` (or equivalent fabric wording) along with its doc comment at `:302`, and the narrative comments at `:322,323,351,440` reword;
  card 5(f) already changed this file's assertions, so this card touches only names and prose, and the two cards are DAG-ordered (batch 06 depends on batch 03, which batch 02 precedes via batch 04's join — verify the file's assertions still reference the post-card-5 `CheckID`s before renaming anything).
  `internal/websterengine/audit_test.go` (~38 hits) — `TestWeftReferencePattern` (`:28,39`) renames to `TestRefScannerMatches` or similar, and its surrounding prose rewords;
  card 12 already moved this file's helpers onto the scanner, so this card is prose and test-name only.
  `perchcli/run_integration_test.go` (~50 mentions) and `configcli/configcli_integration_test.go` (~24, including its "host worktree" comment cluster and the `hostLayout`/`hostWorktreePath` locals — rename locals only where they do not shadow a `lyxtest` fixture field name) are the two big files;
  fixture-construction code that genuinely builds paired worktrees via `lyxtest` keeps honest owner vocabulary, surrounding narrative prose rewords.
  Comments and test-local identifiers only — no assertion or behaviour changes;
  the full `-tags integration` suite runs unchanged at the done gate.
- **Commit:** `test: fabric-vocabulary hand-clean of non-owner test files`

### Card 24: sweep verification gate

- **Context:**
  - `_mill/discussion.md`
- **Edits:**
  - `internal/fabricengine/fabric_test.go` (only if the batch's own `verify:` surfaces the Test Tier Purity Invariant hit below; not a vocabulary edit)
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Verification-only card, no diff, with one documented exception: the batch's own `verify:` runs `go test ./cmd/lyx/`, whose `TestTierPurity_UntaggedTestsSpawnNothing` guard raw-substring-matches `lyxtest.Copy` even inside a comment.
  `internal/fabricengine/fabric_test.go:7` (added by batch 01, outside this batch's file scope) carries `a real lyxtest.CopyPaired fixture` in prose, tripping the guard though the file spawns no git.
  Since a same-task commit (batch 01) touches this file, the failure is in-scope for this batch's verify gate to fix, not pre-existing: reword the comment (e.g. "a real paired lyxtest fixture (CopyPaired)") so the literal `lyxtest.Copy` substring no longer appears, per tierpurity_test.go's own documented resolution ("rename the mention or tag the file") — no identifier, behavior, or vocabulary change.
  Run `grep -rniE '\bweft|\bwarp' internal cmd --include='*.go' --include='*.md'` and filter out the owner set (`fabricengine`, `fabriccli`, `weftname`, `lyxtest`, `boardengine`, `configsync`'s two string literals) and `*_test.go` carve-out references;
  then grep the fabric-sense `host` phrase list (`host repo`, `host repository`, `host worktree`, `host working tree`, `host checkout`, `host branch`, `host junction`, `host path`, `host side`, `host HEAD`, and `hostBranch`-style identifiers) the same way.
  Every surviving non-owner production hit must be fixed before this card completes (fold the fix into a follow-up commit amending the responsible card's file);
  every surviving test hit must be an owner-API reference or a named carve-out.
  This is the precondition gate for batch 07 — the enforcement test must go green on first activation, not flush out stragglers.
  If the gate finds a straggler, fix it and land it as its own new commit on this card (`docs: fabric-vocabulary sweep stragglers`) — never by amending an earlier card's commit, which would violate the repo's commit-per-fix discipline.
  The card is diff-free in the expected case;
  `Commit: none` describes that case, and a straggler commit is the documented exception.
- **Commit:** none

## Batch Tests

`verify:` runs `go vet -tags integration ./... && go test ./cmd/lyx/` — the sweep is comments/prose/test-text only, so a whole-tree vet (which also compiles every test file, integration-tagged included) plus the `cmd/lyx` guard suite (board guard, tier purity, raw-git-mutation, help tree) is the right-sized check;
the edited integration tests run at the done gate.
Card 24's grep gate is the batch's real exit criterion.
