# Batch: docs-smokes-and-residue

```yaml
task: "Replace reed's header pane with a status-line and Selvage"
batch: "docs-smokes-and-residue"
number: 7
cards: 12
verify: go test ./internal/lyxcwd/ ./cmd/lyx/ ./internal/reedcli/ ./tools/sandbox/ && go vet -tags smoke ./internal/reedcli/
depends-on: [6]
```

## Rename mechanic

For each `Moves:` pair the implementer MUST:

1. Run `git mv <old> <new>` FIRST, before making any other change to the moved file.
2. Make ONLY surgical edits — touch only the lines that must change after the move (package or module declaration, imports, identifier retargeting, seam splits).
3. Use a full-file `Creates:` entry only for genuinely new files that have no predecessor.
4. Never write the relocated file from scratch and delete the original — that breaks git rename history and inflates review diffs.

## Batch Scope

This batch finishes the task: the module design doc is rewritten from "Planned, not yet built" to what shipped, the roadmap item moves to Done, `docs/overview.md` and the standalone-API inventory are corrected, `tools/sandbox/SANDBOX-REED-SUITE.md` is rewritten per scenario so the sandbox suite still exercises reed against the code rather than asserting a pane that no longer exists, and every reed smoke file plus the residual prose the mandated `header` scan surfaced is adapted, retargeted or deleted.
It is one batch because each of these is a leaf edit against already-shipped behaviour — none of them is a dependency of any other batch, and grouping them keeps the plan's earlier batches focused on code that must compile together.

Batch-local decisions beyond `## Shared Decisions`: the sandbox suite rewrite is mandatory rather than optional — CONSTRAINTS.md's Sandbox Suite Coverage invariant keeps every registered module exercised or explicitly excluded with a reason, and reed stays exercised.
M19's central assertion is **inverted** by this task rather than merely retargeted: the old "a bare shell prompt in that pane is a `FAIL`" clause becomes the pass condition.

## Cards

### Card 44: rewrite the module design doc to what shipped

- **Context:**
  - `_mill/discussion.md`
  - `internal/reedengine/doc.go`
  - `internal/reedcli/watchdog.go`
  - `internal/reedcli/statusline.go`
  - `internal/reedengine/windowsize.go`
  - `internal/reedengine/lifecycle.go`
- **Edits:**
  - `manifest/designs/reed-header-selvage.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Rewrite `manifest/designs/reed-header-selvage.md` from a forward-looking design into a record of what shipped. Replace the `> **Status: Planned, not yet built.**` blockquote with a status line stating the design is implemented, and drop the per-claim *(confirmed live)* / *(not yet confirmed)* annotations that only made sense while the doc was speculative — except where a claim is still genuinely unverified, which is card 46's subject. Rewrite "The design" section's three subsections to describe the shipped mechanisms by their real identifiers: the status-line half names `pinGeometryOptionsLocked`'s seven `set-option` calls, `escapeStatusText`, `statusLeftLength` and the `#{status}` readback; the Selvage half names `ensureSelvagePaneLocked`, `splitSelvagePaneAtBottomLocked`, `ReedState.SelvagePaneID`, `render.Selvage` and `reed.yaml`'s `selvage.height_rows`; the daemon half names `lyx reed watchdog --hub-path <abs> --tmux <path>`, `reedengine.ListSessions`, the `reed-watchdog.lock` under `fabricengine.HubScratchDir`, `watchdogHubDiscoveryCycle`/`watchdogHubIdleCycles`, and the three spawn sites. Close the four "Open items" the doc records as decided rather than deleting the section wholesale: the `worktree` token now exists; the `apply.go` band flip is implemented and tested; Selvage's shell is `reed.yaml`'s existing `shell:` key; and the daemon's lifecycle is self-exit on three consecutive non-affirmative enumerations, with `reed down` never killing it. Add the module-local Selvage rules the `no-new-cross-cutting-invariant` decision keeps out of `CONSTRAINTS.md`: bottom-most physical position, never a strand, never written to. Add the standalone disposition — standalone reed runs `Engine.Watch` as an in-process goroutine off the `reedUp` seam, never the daemon. Keep the "Related" section's link to `loom-step.md` intact so the Markdown Link Integrity invariant still holds. Follow this repo's semantic-line-break rule throughout: one sentence per line, breaking inside a long sentence at an internal independent-clause boundary, never a fixed-column hard wrap.
- **Commit:** `docs(designs): rewrite reed-header-selvage.md to what shipped`

### Card 45: move the roadmap item to Done

- **Context:**
  - `manifest/designs/reed-header-selvage.md`
- **Edits:**
  - `manifest/roadmap.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `manifest/roadmap.md`, move the Planned item beginning "**reed: replace the header pane with a native tmux status-line, a permanent \"Selvage\" terminal pane, and a detached per-hub watchdog process**" out of `## Planned` and into `## Done`, reshaping its text to the tense and shape the neighbouring Done entries already use and keeping its `See [designs/reed-header-selvage.md](designs/reed-header-selvage.md).` link so Markdown Link Integrity still holds. Update the Someday entry `reed: cross-worktree columns`, whose parenthetical today reads "and now Selvage — claimed by the Planned header-replacement item above" — the item is no longer Planned and is no longer above it, so that clause must name it as claimed by the shipped work instead. Change no other Planned or Someday item: this task completes exactly one roadmap item and adds none.
- **Commit:** `docs(roadmap): move the reed status-line/Selvage/watchdog item to Done`

### Card 46: record the psmux verification as an open item

- **Context:**
  - `internal/reedengine/windowsize.go`
  - `internal/reedengine/proctree_windows.go`
  - `_mill/discussion.md`
- **Edits:**
  - `manifest/designs/reed-header-selvage.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add to `manifest/designs/reed-header-selvage.md` an explicitly open verification item for the status-line under Windows/psmux, per the `windows-psmux-verification-is-an-open-item-carried-by-this-plan` Shared Decision. State the exact check: run `lyx reed up` under psmux and read back `#{status}`, `#{status-position}`, `#{status-left}` and `#{window-status-format}`, recording which of the options survived. State the named degrade this task accepted in the meantime — if psmux refuses them, Windows loses the identity text, while Selvage, the layout, the reap rules and the watchdog are all unaffected, and `status-position` falling back to `top` is acceptable on its own since the status-line is not a pane and its edge is independent of Selvage's. State why reed does not branch on Windows here: a refused `set-option` fails loudly into the log, changes nothing, and is answered by the `#{status}` readback, whereas the one place reed *does* branch (`hookInstalledLocked`) is where the consequence would be silent and unrecoverable. State that this decision is to be revisited against the verification's result rather than treated as settled. Keep it in the doc as an open item — do not write it as a closed one.
- **Commit:** `docs(designs): record the psmux status-line verification as an open item`

### Card 47: correct docs/overview.md's three reed passages

- **Context:**
  - `internal/reedcli/statusline.go`
  - `internal/reedcli/watchdog.go`
  - `internal/tokenvocab/tokenvocab.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Correct `docs/overview.md` in the three places this task falsifies. In the **reed** module bullet: change the verb list `lyx reed up|add|remove|status|attach|resume|header|down` so it names `statusline` and `watchdog` in place of `header`; and rewrite the sentence naming reed's two registered interactive-handoff exceptions — today "`reed attach` and `reed header --blocking`" with `header --blocking` described as printing the rendered header-pane text then blocking forever as the pane's keepalive — so the second exception is `reed watchdog`, described as the detached per-hub resize daemon that blocks for its process lifetime, with the same closing clause that every fallible step runs pre-flight on the envelope and only the handover/daemon tail is exempt. In the `tokenvocab` bullet, change "(the `repo`/`hub` token registry + the `Render` compose over `internal/stencil`)" and "reed's header text pipeline consumes it today" so both name the three-token registry and reed's status-line pipeline. In the package-documentation list entry for `internal/tokenvocab`, change "(`repo`/`hub` + `Render` over `internal/stencil`), consumed by reed's header pipeline" the same way. Change nothing else in the file, and keep every inline markdown link resolving so the Markdown Link Integrity invariant holds.
- **Commit:** `docs(overview): correct reed's verb list, exception list and tokenvocab prose`

### Card 48: update the standalone-API public-surface inventory

- **Context:**
  - `internal/reedengine/statusline.go`
  - `internal/reedengine/overlay.go`
- **Edits:**
  - `manifest/designs/reed-fabric-standalone-api.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `manifest/designs/reed-fabric-standalone-api.md`'s **Public surface** section, update the `*Engine` method inventory: `HeaderText` becomes `StatusLineText` and `ValidateHeader` becomes `ValidateStatusLine`, with the method count staying **17** since the two are renames rather than additions. In the **External contract footprint** section, add `ListSessions` to the sorted list of exported package-level identifiers referenced in code by production packages outside `reedengine`, and raise the stated count from 13 to 14 in both the bolded figure and any prose restating it — `internal/reedcli`'s watchdog daemon is the new external reference. Re-run each section's own stated producing command before writing the new figures rather than incrementing by hand: `go doc ./internal/reedengine Engine | grep -c '^func (e \*Engine)'` for the method count, and the footprint section's own definition for the identifier count. Leave the line-count figures near the top of the file alone unless re-running `wc -l` over the package's non-`_test.go` files shows them materially wrong, in which case update them to the measured values.
- **Commit:** `docs(designs): update reed's public-surface inventory for the renamed methods`

### Card 49: rewrite the sandbox reed suite per scenario

- **Context:**
  - `internal/reedcli/statusline.go`
  - `internal/reedcli/watchdog.go`
  - `internal/reedengine/lifecycle.go`
  - `internal/fabricengine/junctionnames.go`
  - `tools/sandbox/SANDBOX-REED-WATCH-SUITE.md`
  - `CONSTRAINTS.md`
- **Edits:**
  - `tools/sandbox/SANDBOX-REED-SUITE.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Rewrite `tools/sandbox/SANDBOX-REED-SUITE.md` scenario by scenario. **M19** ("Always-on header pane (operator console)") is rewritten end to end as "Always-on Selvage pane": its central assertion inverts, so the extra pane is physically **bottom**-most and its visible content is a **shell prompt** — the old "a bare shell prompt … is a `FAIL`" clause becomes the pass condition — while the rendered identity text moves to a separate check against `#{status-left}` via `tmux -L <socket> display-message -p '#{status-left}'`. The survives-its-last-strand, excluded-from-the-`strands`-count, and heal-after-its-process-dies assertions all carry over unchanged in substance, retargeted onto Selvage, and `header.height_rows` becomes `selvage.height_rows` throughout the file. **Lines 371-375** (the wedged-heal scenario): "the one-row header band at the top" becomes the bottom band, and "the freshly rebuilt header ends up alone, full-height, at the very top" becomes Selvage alone filling the window, which is tmux's own tiling rather than something reed computes. **Lines 409 and 427** (the watchdog dormancy scenarios): "check the renamed session's header pane log" is falsified twice over — there is no header pane, and the daemon's warnings land in `fabricengine.HubLogsDir(hub)` — so both become a check of the hub log file for exactly one vanished-worktree-root warning, with the one-warning-not-one-every-two-seconds assertion itself unchanged, since it is about the dormant cadence this task does not touch. **Line 439-440** (the attach geometry scenario): "a session holding a header pane and at least two strands" becomes Selvage, and `header.height_rows` becomes `selvage.height_rows`. **Line 444**'s "A header that grows past its configured row count" becomes Selvage. **Line 451** — a third header-pane-log check the discussion's own list does not enumerate, found by re-running its mandated scan — becomes a check of the hub log file for exactly one `promoting resize watchdog to signal mode` line for the session, with the absent-means-never-matched and repeated-means-flapping readings unchanged. **Line 284**: "the untracked reap now fires from the alive header this `up` boots" becomes the alive Selvage; the reap policy is unchanged. Leave the fingerprint-header section (line 39) and the `<copy from the header above>` instruction alone — both are the suite's own document structure, not the header pane. `SANDBOX-REED-WATCH-SUITE.md` needs **no** change and must not be edited: its only three `header` occurrences are its own fingerprint-header section and boilerplate.
- **Commit:** `docs(sandbox): rewrite the reed suite for Selvage, the status-line and the daemon`

### Card 50: adapt the keepalive and seed smokes, delete the scrollback smoke

- **Context:**
  - `internal/reedcli/statusline.go`
  - `internal/reedengine/lifecycle.go`
  - `internal/reedengine/state.go`
  - `internal/clihelp/annotations.go`
- **Edits:**
  - `internal/reedcli/smoke_selvage_keepalive_test.go`
  - `internal/reedcli/smoke_statuslineseed_test.go`
- **Creates:** none
- **Deletes:**
  - `internal/reedcli/smoke_headerscrollback_test.go`
- **Moves:**
  - `internal/reedcli/smoke_header_keepalive_test.go` -> `internal/reedcli/smoke_selvage_keepalive_test.go`
  - `internal/reedcli/smoke_headerseed_test.go` -> `internal/reedcli/smoke_statuslineseed_test.go`
- **Requirements:** Delete `internal/reedcli/smoke_headerscrollback_test.go` outright: its subject is `headerBlockingPayload`'s ED2/ED3 screen-and-scrollback clear, which batch 5 deleted, and Selvage is never written to or cleared by reed, so there is nothing left to assert. After `git mv`, adapt `internal/reedcli/smoke_selvage_keepalive_test.go` surgically — the keepalive subject survives and only its mechanism changes: retarget every `HeaderPaneID` reference onto `SelvagePaneID`, rename its test functions to name Selvage, and extend it with the assertions the discussion's Testing section names — kill every strand pane and assert the session survives with Selvage remaining; assert Selvage is physically bottom-most after a series of adds and removes; assert Selvage survives a Ctrl-C sent to it at an idle prompt; and assert it survives a Ctrl-C that kills a foreground job inside it. After `git mv`, adapt `internal/reedcli/smoke_statuslineseed_test.go` surgically: retarget it from `lyx reed header` onto `lyx reed statusline`, keeping its subject intact — `clihelp.SkipStencilSeedAnnotation` survives on the replacement verb, and what the test protects (a preview command leaving no stencilstore warnings and no git commits in the hub) is still true and still worth pinning. Both adapted files keep their existing build tags.
- **Commit:** `test(reedcli): adapt the keepalive and seed smokes, drop the scrollback smoke`

### Card 51: retarget the remaining reed smoke files

- **Context:**
  - `internal/reedengine/state.go`
  - `internal/reedengine/windowsize.go`
  - `internal/reedengine/render/rules.go`
  - `internal/reedengine/spawn.go`
- **Edits:**
  - `internal/reedcli/smoke_lifecycle_test.go`
  - `internal/reedcli/smoke_dotfill_test.go`
  - `internal/reedcli/smoke_panecwd_test.go`
  - `internal/reedcli/smoke_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** `internal/reedcli/smoke_lifecycle_test.go` is the largest single test edit in this task: retarget its four `st.HeaderPaneID` assertions onto `SelvagePaneID`, and replace its `pollPaneContains(..., "hub: "+HubPath)` assertion — unassertable once the text lives in a tmux option rather than a pane's screen — with a `tmux -L <socket> display-message -p '#{status-left}'` readback asserting the rendered text, not a pane-content poll. Rename its affected test functions to name Selvage. In `internal/reedcli/smoke_dotfill_test.go`, retarget any assertion naming the header band onto the Selvage band; keep the file and its subject, since the dot-fill artifact is a resize-render property of the strand stack rather than anything band-specific, and `internal/reedengine/doc.go`'s "Measurement record (repaint candidates)" block still governs it unchanged. In `internal/reedcli/smoke_panecwd_test.go` — a file the discussion's own disposition list does not enumerate, found by re-running its mandated scan — retarget the file's leading comment and its `"first strand (splits off the header)"` subtest name onto Selvage, and retarget the comment describing `planPaneTarget`'s header-as-last-resort fallback and its tallest-alive-non-header preference onto the Selvage spellings card 19 introduced. In `internal/reedcli/smoke_test.go`, the phrase "the header-noise assertions need the full scrollback" in `capturePaneScrollback`'s doc comment names the deleted scrollback smoke's assertions; leave `capturePaneScrollback` itself alone if another caller still uses it, and update that one sentence to name whatever its surviving callers assert — or delete the helper with its comment if the scrollback smoke was its only caller.
- **Commit:** `test(reedcli): retarget the lifecycle, dot-fill and pane-cwd smokes onto Selvage`

### Card 52: add the status-line and upgrade smokes

- **Context:**
  - `internal/reedengine/windowsize.go`
  - `internal/reedengine/state.go`
  - `internal/reedengine/lifecycle.go`
  - `internal/reedcli/smoke_selvage_keepalive_test.go`
  - `internal/reedcli/smoke_staterecovery_test.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/reedcli/smoke_selvage_keepalive_test.go`
  - `internal/reedcli/smoke_staterecovery_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add the status-line smoke to `internal/reedcli/smoke_selvage_keepalive_test.go`, beside the Selvage assertions card 50 put there: after `lyx reed up`, assert `#{status}` reads `on`, `#{status-position}` reads `bottom`, and `#{status-left}` contains both the repo name and the worktree name. On Windows the same smoke asserts the **self-correcting** half instead of the values — whatever `#{status}` reads back, the reserved-row count derived from it matches the window the layout was planned against — so that a psmux which refuses the options fails the identity assertion loudly rather than the layout silently, per the `windows-status-line-is-an-unbranched-accepted-degrade` Shared Decision. Add the upgrade smoke to `internal/reedcli/smoke_staterecovery_test.go`, whose existing subject is exactly this shape of recovery: seed a `reed.json` carrying the pre-rename `headerPaneId` key plus a live header-shaped pane, run `lyx reed up`, and assert Selvage exists at the bottom and the stale pane is gone — which is what pins the `no-migration-for-the-renamed-state-field` Shared Decision's claim that the operator sees one stale pane for less than one op. Both additions keep their files' existing build tags and build their hub fixtures the way those files already do.
- **Commit:** `test(reedcli): add the status-line and post-upgrade smokes`

### Card 53: retire the header-keepalive test stand-ins

- **Context:**
  - `internal/reedengine/lifecycle.go`
  - `internal/gitkit/reexecguard.go`
  - `cmd/lyx/hermeticenv_test.go`
  - `cmd/lyx/tierpurity_test.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/reedcli/testmain_test.go`
  - `cmd/lyx/tiersleep_test.go`
- **Creates:** none
- **Deletes:**
  - `internal/reedengine/testmain_test.go`
- **Moves:** none
- **Requirements:** Delete `internal/reedengine/testmain_test.go` outright — its whole subject is standing in for `lyx reed header --blocking` when the test binary is re-exec'd as the header pane's command, and with that re-exec gone it has no subject. Its `TestMain` does nothing else (no `gitkit.HermeticGitEnv()` call), and no `internal/reedengine` test file carries a git-spawning token, so deleting it leaves the Hermetic Git Test Environment Invariant satisfied. **Edit** `internal/reedcli/testmain_test.go` rather than deleting it, per the `reedcli-testmain-loses-its-stand-in-but-keeps-its-file` Shared Decision: delete the `os.Args[1] == "reed"` branch, its `fmt.Println` marker, its `time.Sleep` loop and the now-unused `fmt`, `os` and `time` imports, leaving a `TestMain` that calls `gitkit.HermeticGitEnv()` and then `os.Exit(m.Run())` — `os` stays for that last call. Rewrite the file's leading comment so it no longer claims to guard the binary against being run as lyx by a header pane. In `cmd/lyx/tiersleep_test.go`, delete **both** `allowedLongSleepers` entries — `internal/reedengine/testmain_test.go` and `internal/reedcli/testmain_test.go` — since neither file contains a long sleep any more. Do not weaken `gitkit.refuseCLIReexec`: it stays the backstop it is, and card 33's `suppressWatchdogSpawn` is what prevents the daemon spawn from re-exec'ing under test, which is why no replacement stand-in is needed.
- **Commit:** `test(reedengine,reedcli,lyx): retire the header-keepalive TestMain stand-ins`

### Card 54: repoint the loom bootstrap comment at the surviving analog

- **Context:**
  - `internal/reedcli/spawnwatchdog.go`
  - `internal/shell/shell.go`
- **Edits:**
  - `internal/loomcli/bootstrap.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/loomcli/bootstrap.go`, rewrite `statusStrandCmd`'s doc comment, which today reads "composes the status strand's pane command line through the shell seam, exactly as the reed header pane's own builder (headerLaunchCmd, headerpane.go) composes its command line". `headerpane.go` was deleted in batch 3, so repoint the comment at the surviving analog rather than dropping it: the watchdog daemon spawn in `internal/reedcli/spawnwatchdog.go`, which composes an `os.Executable()` command line the same way. The comment's value is naming a live example, which is why it is repointed rather than deleted. Change no code in this file.
- **Commit:** `docs(loomcli): repoint the exe-command-line example at the watchdog spawn`

### Card 55: re-run the header scan and confirm no unhandled site remains

- **Context:**
  - `CONSTRAINTS.md`
  - `_mill/discussion.md`
  - `manifest/designs/reed-header-selvage.md`
  - `tools/sandbox/SANDBOX-REED-SUITE.md`
  - `docs/overview.md`
- **Edits:** none
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Re-run the discussion's mandated scan — a case-insensitive `header` search over `.md` and `.go` files, excluding `.git/` and `_mill/` — and confirm every surviving hit falls into one of the unrelated senses the discussion names: HTTP headers (`plugins/prowler/`, `internal/githubclient/`), markdown or table headings, the sandbox suites' own fingerprint-header sections, `docs/`-style document headers, and Go file/package header comments referred to as such. Report any hit that names reed's header pane, the `reed header` verb, `HeaderPaneID`, `HeaderConfig`, `HeaderText`, `ValidateHeader`, `headerLaunchCmd`, `headerpane`, `console-header` or `clampHeaderHeight` as an unhandled case rather than closing it out — every one of those should have been retargeted or deleted by an earlier card, so a survivor is a defect in this plan's coverage, not a new scope item. This card changes no file: it is the coverage gate the discussion's Testing section requires, and its value is that the scan is actually run once more against the finished tree rather than assumed from the planning-time run.
- **Commit:** none

## Batch Tests

`verify: go test ./internal/lyxcwd/ ./cmd/lyx/ ./internal/reedcli/ ./tools/sandbox/` names the four packages whose tests actually gate this batch's edits: `internal/lyxcwd` owns `docslink_test.go`, the Markdown Link Integrity enforcement over `manifest/` and `docs/`, which is what the design-doc, roadmap, overview and standalone-API edits must not break; `cmd/lyx` owns the tier-purity and long-sleep guards whose allowlist card 53 shrinks, plus the help-tree and drift guards; `internal/reedcli` is where the adapted and deleted smoke files must still compile in the untagged tier; and `tools/sandbox` owns `suite_test.go`, which is what `cmd/lyx/sandbox_coverage_test.go`'s Sandbox Suite Coverage check is paired with.
The reed smoke files this batch rewrites are `smoke`-tagged, so neither the untagged `go test` above nor `pipeline.done_gate` — which runs only the untagged and `integration` tiers — so much as *compiles* them, and a smoke file left naming a deleted identifier would otherwise go unnoticed until an operator ran the suite by hand.
The chained `go vet -tags smoke ./internal/reedcli/` closes exactly that gap: it type-checks every `smoke`-tagged file in the package against the finished tree without booting a single tmux server, which is the right trade for a fixer loop that may run this verify several times.
Their live behaviour stays the sandbox suite's job, which is exactly why card 49's rewrite of `SANDBOX-REED-SUITE.md` is mandatory rather than cosmetic.
Card 55 runs no test of its own and produces no commit: it is a grep-and-confirm coverage gate over the finished tree.
