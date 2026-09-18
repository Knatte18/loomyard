# Discussion: Rename loom CLI run/drive/step for verb/engine symmetry, plus rename ly-supervise

```yaml
task: Rename loom CLI run/drive/step for verb/engine symmetry, plus rename ly-supervise
slug: loom-cli-rename
status: discussing
parent: main
```

## Problem

loom's CLI verb names don't say what each verb calls.
`lyx loom run` never calls `shedengine.Shed.Run` — it bootstraps (seed the status file, commit it weft-side, ensure the reed substrate, spawn the detached driver, hand the terminal to tmux) and is the only verb that seeds.
`lyx loom drive` is what actually calls `Shed.Run`: the foreground, no-status-strand, no-terminal-handover escape hatch for debugging and CI.
An operator reading `lyx loom run` reasonably expects the engine's `Run`, and gets the bootstrap instead;
an operator reading `drive` has no way to guess it is the plain engine loop.

The `/ly:ly-supervise` skill has the mirror problem in the other direction: it *drives* the whole loop over `lyx loom step`, but "supervise" reads as passive observation, and the name is long for something invoked by hand.

Why now: this is the last cheap moment.
The verbs are pre-release, there is one operator, and the two Next Up items that build on this surface — `reed: born-as-strand for the operator's loom run attach` and `generalize ly-supervise and loom's CLI verbs into a Shed-generic watchdog` — both take the current names as their starting point.
Renaming after either lands means renaming a wider surface, and the generic-watchdog item in particular would bake the asymmetry into a generic abstraction where it is much harder to unpick.

## Scope

**In — stated as an enumeration rule, not a file whitelist.**
Scope is *every* hit of the grep set below, anywhere in the repository tree, each one read and classified before it is touched.
A file is in scope because it contains a hit, never because it appears on a list;
the named sites elsewhere in this document are worked examples and landmarks, never the boundary.

The grep set, run over the whole tree (including `README.md`, which a file-by-file enumeration missed).
The set is deliberately over-broad: it is a *candidate* generator feeding the read-and-classify step below, not a list of edits.

- **Word-boundary `drive` and `run`, case-insensitive, over the whole tree.**
  This is the primary pattern and it subsumes the rest.
  It is broad on purpose: prose names a verb as a bare backticked `` `drive` `` or `` `run` ``, with no "loom" nearby and no quotes, in many places — `docs/overview.md:244,331`, `manifest/roadmap.md:154`, `manifest/designs/loom.md:440`, `manifest/designs/loom-step.md:48,49`, `manifest/designs/self-report-tier2.md:42`, and `internal/loomcli/drive.go:1` among them.
  A narrower pattern keyed on the two-word `loom run` form matches none of those, which is exactly how round 2's whitelist came to be short.
  Most hits of this pattern are disposition 2 (leave);
  that is expected and is what the classify step is for.
- **Split-argv forms** — `"loom", "drive"`, `"loom", "run"`, `"loom", "step"` as adjacent Go string-literal arguments.
  These match no textual `loom run` pattern at all, and they are load-bearing;
  see the split-argv decision below.
- `ly-supervise` — the skill hits, in every file type, not only under `plugins/`.
  The skill name is not a verb, so it needs its own pass;
  it appears in `manifest/roadmap.md` Done entries, several `manifest/designs/*.md`, and Go comments.
- `driveCmd`, `runCmd`, `RunAliasCommand` — the Go identifier hits.
  Note `driveCmd` is also a *local variable* name at `internal/loomcli/run.go:137`, unrelated to the method of the same name;
  it holds the detached driver's `*exec.Cmd` and is renamed for the same reason.
- `lyx run` — the alias hits, caught by the bare-`run` pattern but listed because the alias is its own decision.

Each hit is classified into exactly one of three dispositions:

1. **Rename** — it names a loom verb, the alias, the skill, or a renamed identifier.
2. **Leave** — it is a known false positive: "run" used as a *noun* meaning "an execution", or a `run`/`drive` belonging to another module.
   The known false-positive sites are listed under Technical context;
   a hit not on that list still gets read rather than assumed.
3. **Structural** — a file rename, a file deletion, or a link fix, handled by the decisions below rather than by editing text in place.

**Carve-out: meta and archival trees are excluded wholesale, before classification.**
`_mill/`, `crucible/`, and `docs/research/` are out of scope entirely — not classified hit-by-hit, just skipped.
`_mill/` in particular is tracked (`.gitignore` excludes only `**/_mill/*.active`), so the whole-tree rule would otherwise sweep in this very discussion file, `status.md`, and every file under `reviews/` and `briefs/` — around 70 hits across 13 files, none of them describing the tree.
These files record *what a task decided*, at the time it decided it;
rewriting them would falsify the record rather than update it.
That is the opposite of the `historical-prose-rewritten-not-glossed` decision, which governs documents asserting what the tree currently *is* — a Done roadmap entry and a shipped-status design doc describe today's repo, whereas a review file describes a conversation that happened.
The "~233 hits across ~57 files" figure quoted elsewhere in this document was counted with these trees already excluded;
it is the in-scope count, not the raw grep count.

Structural changes, enumerated because they are not text edits a grep finds:

- `internal/loomcli/run.go` → `start.go`, `internal/loomcli/drive.go` → `run.go`.
- `plugins/ly/skills/ly-supervise/` → `plugins/ly/skills/ly-drive/` (directory rename), plus the `SKILL.md` frontmatter `name`/`description`, and the skill's own `.scratch/ly-supervise/` → `.scratch/ly-drive/` step-envelope path.
- `manifest/designs/loom-cli-rename.md` deleted, with both inbound links fixed.
- `manifest/roadmap.md`'s Planned entry for this item moved to Done.

**Out:**

- `lyx loom step` keeps its name. It already matches `shedengine.Shed.Step`, so the symmetry problem the task exists to fix does not apply to it, despite the roadmap item's title naming all three verbs.
- `lyx loom status`, `lyx loom pause`, `lyx loom validate-discussion`, `lyx loom validate-plan` — untouched.
- The `run` verbs on other modules (`lyx webster run`, `lyx burler run`, `lyx shuttle run`) — untouched; each already names what it calls, and this task does not audit them.
- Any behavioural change. This is a pure rename: no verb gains, loses, or reorders a step, and no flag is added or removed.
- Genericity. Folding these verbs into a `Shed`-generic watchdog is the separate Next Up `shed-generic-watchdog` item, explicitly orthogonal.
- The launcher script *filename* (`run<ext>`) — see the `launcher-filename-unchanged` decision.
- Back-compat aliases for the retired names — see the `no-back-compat-aliases` decision.

## Decisions

### verb-names

- Decision: `drive` → `run`; today's `run` → `start`; `step` unchanged.
  Final verb set: `lyx loom start|run|step|status|pause|validate-discussion|validate-plan`.
- Rationale: `run` then names the verb that calls `Shed.Run`, which is the whole point of the task.
  `start` names the bootstrap accurately — it starts a session rather than running a machine — and reads correctly in the launcher and in operator instructions.
  `step` already matches `Shed.Step` and needs nothing.
- Rejected: `run` → `up` (collides conceptually with `reed up`, which means something different — bring the tmux substrate up, not seed and hand over a task);
  keeping `run` and renaming `drive` → `foreground` (leaves the original asymmetry exactly as it is).

### root-alias-becomes-start

- Decision: the bare root alias becomes `lyx start`, delegating to `lyx loom start`.
  `loomcli.RunAliasCommand` is renamed `StartAliasCommand` and returns the receiver's `startCmd()` unchanged, with `resolvePersistentPreRun` attached as its own `PersistentPreRunE` exactly as today.
  `lyx run` stops existing.
- Rationale: the alias function returns the subtree verb's own `*cobra.Command` unchanged, so the root command's name *is* the subtree verb's name — keeping them equal is structurally free and requires no new machinery.
  The alias must keep pointing at the bootstrap, since that is the everyday operator call.
- Rejected: keeping the name `lyx run` while pointing it at `lyx loom start` — root `run` would then differ from `loom run`, which is a fresh instance of the exact asymmetry this task removes, and would require the alias to override the returned command's `Use`;
  pointing `lyx run` at the new `lyx loom run` — makes the everyday muscle-memory call launch the foreground debug driver, which never seeds and refuses on an unseeded worktree.

### no-back-compat-aliases

- Decision: hard rename. No hidden alias, no deprecation shim, no warning path for `lyx loom run` in its old bootstrap sense or for `lyx loom drive`.
- Rationale: pre-release, one operator, and the whole repo is renamed in the same commit.
  A hidden `drive` alias would keep the misleading name discoverable in shell history and scripts, perpetuating precisely what the task removes.
  Worse, `run` is being *reused* with a new meaning, so a compatibility alias for the old `run` is not even expressible — the name is taken.
- Rejected: hidden deprecated aliases with a warning for one release — there is no release cadence to hang "one release" on, and the reuse of `run` makes the old-`run` half impossible anyway.

### skill-renamed-to-ly-drive

- Decision: `ly-supervise` → `ly-drive`. The skill directory becomes `plugins/ly/skills/ly-drive/`, its frontmatter `name` becomes `ly-drive`, and `INDEX.md`'s row and its explicit-invocation note follow.
- Rationale: the skill drives the loop; "drive" says that and "supervise" doesn't.
  The CLI rename frees `drive` from meaning a loom verb, so the word carries no competing meaning in this vocabulary any more — it is available precisely because of the other half of this task.
  It is also shorter, which matters for a skill invoked by hand.
- Rejected: `ly-watch` (undersells — it decides and advances, it does not observe);
  `ly-run` (collides with both `lyx run`'s successor and `lyx loom run`).

### split-argv-sites-are-scope-targets

- Decision: every site that passes a loom verb as a separate Go string-literal argument is a scope target, enumerated by the split-argv grep pattern rather than by any textual `loom run` search.
  Three concrete dispositions:
  1. `internal/loomcli/run.go:137` — `exec.Command(exe, "loom", "drive")`, the detached driver the bootstrap spawns.
     After the rename the `start` verb spawns `exec.Command(exe, "loom", "run")`.
     The local variable holding it, currently `driveCmd`, is renamed with it.
  2. `internal/loomcli/smoke_test.go:249-279` — `findDriverPIDs` locates that detached process by filtering on `cwd == worktree` plus a single argv element equal to `"drive"`.
     It must **not** simply start scanning for a lone `"run"` element.
     `"drive"` is unique to the loom driver, but `"run"` is a verb on `shuttle`, `burler`, `webster`, and loom itself, so a lone-`"run"` probe matches any such process sharing the worktree cwd and the function silently over-reports.
     Require the **adjacent `"loom"` + `"run"` argv pair** instead, preserving the discriminator's uniqueness.
     Its doc comment's "the detached `lyx loom drive` process" is rewritten to match.
  3. `internal/loomcli/smoke_test.go` lines ~464-1023 — roughly ten `runLoomCLINoFatal(exe, …, "loom", "run")` invocations, which today mean the bootstrap.
     Every one becomes `"loom", "start"`.
- Rationale: this is the single most dangerous class of hit in the task, because `run` is *reused* rather than retired.
  A missed `"loom", "drive"` fails loudly as an unknown verb.
  A missed `"loom", "run"` does not fail at all — it silently invokes the new foreground driver, which never seeds and refuses on an unseeded worktree, so a smoke test that meant "bootstrap this worktree" quietly becomes "run the debug driver against an unbootstrapped worktree".
  The `/proc` argv probe carries this hazard in both directions: left unchanged it scans for `"drive"`, an argv element no process will carry any more, so `findDriverPIDs` returns nil forever and every assertion built on it passes vacuously;
  changed naively to a lone `"run"` it over-matches instead, since `run` is no longer a name unique to loom's driver.
  Only the adjacent-pair form is correct.
- Rejected: relying on the textual grep patterns to surface these — they match none of the split-argv forms, which is why this needs its own pattern and its own decision.

### historical-prose-rewritten-not-glossed

- Decision: retrospective records — `manifest/roadmap.md`'s Done entries, `manifest/designs/loom-step.md`'s "Shipped" header and its Done list, and Go comments narrating past incidents — are rewritten to the new names outright.
  No "(formerly `ly-supervise`)" gloss, no preserved old name anywhere.
  This includes the literal path `plugins/ly/skills/ly-supervise/SKILL.md` in `manifest/designs/loom-step.md:22`, which becomes the new path.
- Rationale: these documents describe the tree as it stands, not as it stood;
  a Done entry pointing at `plugins/ly/skills/ly-supervise/SKILL.md` after the rename names a path that no longer exists, which is worse than a mild anachronism in the narrative.
  The repo already has precedent for removing a retired name rather than carrying it: the Hub Suffix Invariant states outright that "no code parses, trims, or recognises the retired `-HUB`".
  A gloss would also defeat the point of the rename by keeping the old name grep-discoverable, which is the same objection that rejected back-compat aliases.
- Rejected: preserving old names in historical prose with a one-time gloss — leaves dangling path references and keeps the retired name in the vocabulary;
  leaving historical prose untouched entirely — same problem, without even the gloss to explain it.
- Known sites (landmarks, not the boundary — the Scope rule governs): `manifest/roadmap.md:120,157`, `manifest/designs/loom-step.md:3,22`, `manifest/designs/reed-mailbox.md:7,15`, `manifest/designs/reed-header-selvage.md:72`, `manifest/designs/self-report-tier1.md:17`, `internal/shedadapters/bouncer_seed_test.go:475`, `internal/loomcli/smoke_bootstrapwiring_test.go:147`.

### launcher-filename-unchanged

- Decision: the per-worktree launcher script keeps its filename `run<ext>`.
  Only the command embedded in it changes, from `loom run` to `loom start`, via the `launcherScript(runtime.GOOS, spawnRel, ...)` call in `internal/fabricengine/launchers.go`.
  `removeLaunchers`' explicit name list keeps `"run"+ext`.
- Rationale: the filename names the operator's action ("run this pair"), not the CLI verb, and `writeLauncherScriptIfChanged` rewrites an existing file's content in place, so every existing hub picks up the corrected command with no migration.
  Renaming the file to `start<ext>` would instead leave a stale `run<ext>` in every already-created worktree's launcher directory, and that stale file causes two concrete failures: it invokes `lyx loom run`, which after this task is the foreground debug driver that never seeds and refuses on an unseeded worktree;
  and `removeLaunchers` deletes by an explicit name list and then removes the directory non-recursively, so the orphan makes teardown fail outright (the comment on that slice calls it a mandatory edit point for exactly this reason).
- Rejected: renaming to `start<ext>` plus a stale-`run<ext>` sweep in `removeLaunchers` — new migration machinery, plus a destructive path added to `destroy.go`'s chokepoint, for a cosmetic gain on a filename that was never the asymmetry being fixed.

### identifiers-and-filenames-follow-the-verbs

- Decision: rename the Go surface to match: `internal/loomcli/run.go` → `start.go`, `internal/loomcli/drive.go` → `run.go`, `driveCmd` → `runCmd`, the existing `runCmd` → `startCmd`, `RunAliasCommand` → `StartAliasCommand`.
  `internal/loomcli/step.go` and `stepCmd` are untouched.
  Do the two-step rename carefully — `runCmd` and `run.go` both exist before and after, naming different things.
- Rationale: leaving the file and method names on the old words while the cobra `Use` strings change would create a second, fresher name/behaviour asymmetry one layer down, in the exact package the task is cleaning up.
- Rejected: changing only the cobra `Use` strings.

### repo-wide-in-one-commit

- Decision: one task, one coherent rename across code, tests, contracts, docs, manifest, plugins, and the sandbox suite doc.
- Rationale: roughly 233 matches across 57 files name one of these verbs or the skill.
  A partial rename leaves specs and design docs asserting things that are no longer true, and CLAUDE.md requires docs for observable CLI behaviour changes to land in the same commit.
- Rejected: code-and-tests first with a docs follow-up.

### specs-hash-mismatch-accepted

- Decision: edit `contracts/specs/*.md` freely.
  An operator whose hub already seeded the old spec files keeps them until they delete the stale copies and let `stencilstore` re-seed.
  No force-sync is added.
- Rationale: this is `stencilstore`'s existing, deliberate behaviour for any spec edit — a hash-mismatched file is never overwritten — and is not introduced by this task.
  The Stencil Ownership Invariant explicitly bans a force-sync carve-out for specs.
- Rejected: adding a force-sync path — banned by CONSTRAINTS.md.

### doc-lifecycle-on-landing

- Decision: delete `manifest/designs/loom-cli-rename.md`;
  move the roadmap's `loom CLI: rename run/drive/step ...` item from Planned to Done, recording the decided names in the Done entry's one-line what/why;
  rewrite `manifest/designs/shed-generic-watchdog.md`'s "Related" bullet so it no longer links the deleted file.
- Rationale: the Documentation Lifecycle deletes a design doc when its work lands, and the Markdown Link Integrity invariant fails the build on a link to a deleted file.
  Both inbound links to the doc are known: `manifest/roadmap.md:16` and `manifest/designs/shed-generic-watchdog.md:17`.
- Rejected: keeping the design doc — contradicts the lifecycle, and its whole content ("naming not yet decided") is obsolete once this lands.

## Technical context

**`internal/loomcli` structure.**
`cli.go` builds the cobra tree: `Command()` constructs a `loomCLI` receiver, sets `resolvePersistentPreRun` as the parent's `PersistentPreRunE`, and calls `parent.AddCommand(c.runCmd(), c.driveCmd(), c.stepCmd(), c.statusCmd(), c.pauseCmd(), c.validateDiscussionCmd(), c.validatePlanCmd())`.
The parent's `Long` string spells out what each verb does by name and carries an `Example:` block listing all seven invocations — both need rewriting, not just find-and-replace, because the sentences describe the verbs' roles.

`verbUsesLightweightWiring(name string) bool` (`cli.go`) switches on verb names: `status`, `pause`, `validate-discussion`, `validate-plan` take the lightweight path.
Its doc comment explicitly discusses why `"step"` is excluded and names `"run"` and `"drive"` as the comparison — the comment needs rewriting for the new names, but the switch's own case list is untouched since none of the renamed verbs appear in it.

`run.go` (→ `start.go`) holds `runCmd` (→ `startCmd`), the `bootstrapHandshakePollInterval`/`bootstrapHandshakeAttempts` constants, and `RunAliasCommand` (→ `StartAliasCommand`).
Its `Long` string is a numbered four-step description plus an `Example:` block.
The file's own header comment names the verb.

`drive.go` (→ `run.go`) holds `driveCmd` (→ `runCmd`).
Its `Long` string contains several sentences contrasting itself with `lyx loom run`, all of which now must contrast with `lyx loom start` — these are the highest-risk lines in the rename, because a mechanical replace would turn "exactly as `lyx loom run` does" into a self-reference.
Its `RunE` also emits a runtime refusal naming the bootstrap verb: `loom: no status file at <path>; run "lyx loom run" first to bootstrap this task`.

Two further verbs emit the same shape of refusal naming the same verb: `pause.go:40`, and `status.go:116` on its `!found` branch.
All three refusal strings must name `lyx loom start` after this change, and `tools/sandbox/SANDBOX-CORE-SUITE.md` asserts on that exact text in two scenarios.

`step.go`'s `Long` also references `"lyx loom run"` as the bootstrap it mirrors.
`bootstrap.go`, `selfreport.go`, `loomshed/seed.go`, `loomshed/interruptpolicy.go`, `websterengine/strand.go`, and `landingshed/deps.go` all carry comments naming one of the verbs.

**The root alias.**
`cmd/lyx/main.go:115` registers `loomcli.RunAliasCommand()` as a bare root child alongside `loomcli.Command()`, with a four-line comment explaining why it is a real registered child rather than argv splicing.
`cmd/lyx/helptree_test.go:28` lists `"run"` among the root's children (this becomes `"start"`), and line 114 lists loom's subcommands as `{"run", "drive", "step", "status", "pause", "validate-discussion", "validate-plan"}` (this becomes `{"start", "run", "step", ...}`).
Note what this test actually checks: lines 133-137 loop over `wantSubs` doing a per-item `strings.Contains` against the rendered help text.
It is a presence check, not set equality — it can confirm `start` appeared, but it can never catch a `drive` command that survived the rename, since an extra subcommand in the help output fails no `Contains` call.

The exact-set guard for the loom subtree already exists elsewhere: `internal/loomcli/cli_test.go:43-77`'s `TestCommand_RegisteredVerbs_ExactSet` walks `Command().Commands()` and checks both directions against `want := []string{"drive", "pause", "run", "status", "step", "validate-discussion", "validate-plan"}` (line 56), so a surviving or stray loom verb fails it.
That `want` literal is a mandatory edit — the test fails the moment `start` is registered.

`cmd/lyx/sandbox_coverage_test.go:31` is a third mandatory edit the doc-level reading misses: its `excludedModules` allowlist carries a `"run"` key ("alias of loom's own bootstrap verb; covered by the loom module's scenario"), and the test asserts every excluded name is actually registered, so the key must become `"start"` or the assertion fails with "excludedModules names %q but no such module is registered".

`internal/loomcli/cli_test.go:79-93` holds `TestRunAliasCommand_StaysOneCommandWithSubtreeVerb`, which asserts the alias has a non-empty `Short`, that `alias.Use == "run"`, and that it exposes the `--parent` flag.
Rename the test and flip the expected `Use` to `"start"`.

**Launchers.**
`internal/fabricengine/launchers.go:162` builds the run launcher's content with `launcherScript(runtime.GOOS, spawnRel, "loom run")`;
the file header comment at line 6 states what it invokes.
`removeLaunchers` (line ~257) deletes by the explicit list `{"ide"+ext, "fabric-checkout"+ext, "run"+ext}` and then removes the directory non-recursively.
Per the `launcher-filename-unchanged` decision only the `"loom run"` argument and the header comment change here;
the two name lists stay as they are.
`internal/fabricengine/launcher_content_test.go` asserts on launcher content and will need its expected command string updated.

**The skill.**
`plugins/ly/skills/ly-supervise/SKILL.md` has YAML frontmatter with `name: ly-supervise`, a `description:` naming `lyx loom step`, and `disable-model-invocation: true`.
Its body invokes `lyx loom step` throughout (unchanged), names `lyx loom run` as the bootstrap remedy in the pre-loop-baseline section (changes to `lyx loom start`), writes per-step envelopes to `.scratch/ly-supervise/step-<n>.json` (the scratch path should follow the rename to `.scratch/ly-drive/`), and refers to itself by name in several places.
`plugins/ly/skills/INDEX.md:5` carries its table row and line 7 an explicit-invocation note.
The plugin manifest `plugins/ly/.claude-plugin/plugin.json` names no individual skill, so it needs no change — and per this repo's convention its `version` stays at `1.0.0`.

**Docs.**
`docs/overview.md` lines 23, 31, 298, 329-330, 337, 414 all name renamed verbs;
line 31's "Convenience alias: `lyx run` → `lyx loom run`" and line 337's interactive-handoff sentence both need rewriting rather than substitution.
`manifest/designs/loom.md` names the verbs in roughly twenty places, including its module table (line 466, 472) and the "Human boundaries" section where `lyx run --auto` appears.
`CONSTRAINTS.md`'s CLI/Cobra Invariant lists the interactive-handoff exceptions as "`lyx loom status --watch`, `lyx loom run`/`lyx run`" — this becomes "`lyx loom start`/`lyx start`".

**Gotcha: `run` appears in unrelated senses.**
`grep` for `lyx run` or `loom run` catches prose like "a `lyx burler` run is not a loom run" (`internal/burlercli/wiring.go:131,190`), "can never stall an autonomous lyx run" (`internal/githubclient/doc.go:84`), "the last row of a loom run" (`internal/landingshed/deps.go:88`), and "hang an autonomous lyx run indefinitely" (`internal/selfreportengine/selfreport.go:84`).
These use "run" as a noun meaning "an execution", not as a verb name, and must be left alone.
This is the Scope rule's disposition-2 list.
It rules out a blind repo-wide textual substitution;
each hit needs reading.

**Counterpart gotcha: genuine verb hits live in packages a file-by-file scope would not think to name.**
`internal/loomengine/config.go:253` ("a second `lyx loom run`"), `internal/loomengine/seed.go:138`, `internal/frictionengine/spec.go:29` ("lyx loom run is by definition the unattended path"), `internal/webstercli/wiring.go:116`, `internal/loomengine/seedownership_test.go:77-78`, and `internal/loomshed/seed_test.go:95,101` all name a renamed verb in comments or fixtures.
`README.md:20,37,42,204` does too, including its own "Convenience alias: `lyx run` → `lyx loom run`" line at 42 — the same sentence `docs/overview.md:31` carries, in a file no verb-oriented enumeration would have reached.
These are landmarks confirming the Scope rule is the right shape;
they are not a substitute for running the grep set.

**Third gotcha: prose whose *meaning* turns on the two verbs being different.**
Some prose must be rewritten as sentences, never token-substituted, because a mechanical swap inverts what it asserts.
The sites below are landmarks, never the boundary — the Scope rule's classify step is what bounds this class, exactly as it bounds the other two gotchas.
Any hit whose sentence contrasts the two verbs against each other belongs here, whether or not it is listed:

- `internal/loomcli/drive.go`'s `Long` string contrasts the verb against `lyx loom run` repeatedly ("ensures that session itself, exactly as `lyx loom run` does").
  Substituting turns each contrast into a self-reference.
- `manifest/designs/self-report-tier1.md:13-20` states an exemption that turns on `lyx loom drive` and `lyx loom run` being two different verbs.
  A token swap collapses both sides onto the same name and silently inverts the stated rule.
- `internal/shuttleengine/attach.go:415-421` — "recreated in-band by `lyx reed up`, or simply by `lyx loom run` and `lyx loom drive`, which both call `reed.Up()` themselves", plus a later sentence naming `lyx loom drive` in a crucible-round finding.
  The "both" is the whole point of the sentence;
  substitution makes it name one verb twice.
- `internal/loomengine/seedownership_test.go:77-78` — the same contrast in a test comment.

Each needs a human-legible rewrite against the new verb set, and they are collectively the reason the classify step exists rather than a `--replace` flag.

## Constraints

From `CONSTRAINTS.md`:

- **CLI / Cobra Invariant** — every module exposes `Command()` and `RunCLI`/`RunCLIIn`;
  non-empty `Short` on every command;
  errors are JSON via `internal/output`;
  every `RunE` checks `clihelp.ShouldAbort` first;
  an alias command may delegate into another module's subtree with no seam function of its own.
  Its interactive-handoff exception list names the verbs by name and is itself an edit target.
- **Stencil Ownership Invariant** — no force-sync carve-out for `contracts/specs`;
  a hash-mismatched file is never overwritten.
- **Markdown Link Integrity** — every inline markdown link under `manifest/` or `docs/` must resolve, file part and `#anchor`.
  Deleting `manifest/designs/loom-cli-rename.md` requires fixing both inbound links in the same commit.
- **Documentation Lifecycle** (`docs/overview.md#documentation-lifecycle`) — a module-design doc is deleted when its work lands.
- **Fabric Destruction Chokepoint Invariant** — `internal/fabricengine/destroy.go` is the only file permitted a destructive primitive.
  Relevant only as a reason not to add launcher-migration deletion logic.
- **Test Tier Purity Invariant** — untagged test files perform no expensive spawns.
  Any new guard test must stay tier 1: no `exec.Command`, no `gitexec`, no `hubforge.NewHub`.
- **Sandbox Suite Coverage** — every registered lyx module is exercised or explicitly excluded.
  Enforced by `cmd/lyx/sandbox_coverage_test.go`'s `excludedModules` allowlist, whose `"run"` key (line 31) must become `"start"`, keeping its existing reason text — the test asserts every excluded name is a registered module, so a stale key fails outright.
  No suite doc changes for this: no `**Covers:**` tag names `run`.

From `CLAUDE.md`:

- Docs land in the same commit as the behaviour change: the module doc in `manifest/designs/`, `docs/overview.md` when the module table or execution stack changes, and `CONSTRAINTS.md` for any cross-cutting invariant text.
- `manifest/roadmap.md` moves on completing a planned item — which this is, so the Planned→Done move is required.
- Markdown uses semantic line breaks, one sentence per line, never fixed-column hard-wrap.
- Building requires `CGO_ENABLED=1` and a C compiler on `PATH`.
- Unpublished plugins stay at version `1.0.0`.

## Testing

No new behaviour ships, so the testing job is threefold: keep every existing assertion honest under the new names, add guards that make a future half-rename fail, and confirm nothing outside the rename moved.

**`cmd/lyx/helptree_test.go` — adapt in place.**
The root-children list becomes `"start"` instead of `"run"`, and loom's subcommand list becomes `{"start", "run", "step", "status", "pause", "validate-discussion", "validate-plan"}`.
These two assertions confirm the new names reached cobra, so write them before touching `loomcli` — a genuine TDD candidate, since neither `start` nor any `Short`/`Long` containing it exists on today's tree, so both fail loudly now and pass only once the rename lands.
They do **not** prove the old names are gone: the test is a per-item `strings.Contains` sweep, so a surviving `drive` command fails nothing here.
The two exact-set guards below close that half.

**`internal/loomcli/cli_test.go`'s `TestCommand_RegisteredVerbs_ExactSet` — adapt, and it is already the loom-subtree guard.**
Update its `want` literal to `{"pause", "run", "start", "status", "step", "validate-discussion", "validate-plan"}`.
It walks the parent's registered commands and errors in both directions, so it already fails on a surviving `drive` — no new test is needed for the loom subtree, and none should be written.
This is a mandatory edit, not optional: the test fails as soon as `start` is registered.

**`cmd/lyx/sandbox_coverage_test.go` — adapt.**
Change the `excludedModules` key at line 31 from `"run"` to `"start"`, keeping its existing reason text.
The suite asserts each excluded name is a registered module, so a stale key fails outright.

**`internal/loomcli/cli_test.go` — adapt and extend.**
Rename `TestRunAliasCommand_StaysOneCommandWithSubtreeVerb` to match the new function and flip its expected `Use` to `"start"`;
its existing non-empty-`Short` and `--parent`-flag assertions carry over unchanged.
Add one assertion the current test lacks: the alias's `Use` equals the name of the subtree verb it was built from, so the two can never drift apart again.
This is the assertion that encodes the `root-alias-becomes-start` decision structurally rather than as a literal.

**A retired-name guard — new, tier 1, root tree only.**
Assert the root tree carries no bare child named `run`.
Scope it to exactly that: the loom subtree's half is already covered by `TestCommand_RegisteredVerbs_ExactSet` above, and duplicating it would leave two tests to keep in sync for one property.
The root half genuinely is uncovered — `helptree_test.go:28` only does `Contains` over the root's children, and `sandbox_coverage_test.go`'s map asserts that excluded names *are* registered, never that a retired one is absent.
A pure `*cobra.Command` walk over the root builder, so it spawns nothing.
Second TDD candidate: write it first, watch it fail on today's tree.

**Refusal-text coverage — adapt, do not author.**
`internal/loomcli/cli_test.go:129-151` already holds `TestVerbRefusals`, a table test covering the `drive` and `pause` verbs' unseeded-status refusals with `wantRemedy: "lyx loom run"`.
Do not write a second test for this: change both rows' `wantRemedy` to `lyx loom start`, rename the `Drive_SeedMissing` row to match the renamed verb, repoint its `buildCmd` at the renamed method, and **add a third row** for `status.go:116`'s `!found` refusal, which the existing table does not cover.
That third row is the one genuinely new assertion here.
The test builds leaf commands directly against a hand-populated `*loomCLI` with `shedPaths` pointing at a `t.TempDir()`, so it stays tier 1 and the new row must follow that same shape.

**`internal/fabricengine/launcher_content_test.go` — adapt.**
Update the expected embedded command to `loom start`.
Add a scenario asserting the launcher's *filename* is still `run<ext>` while its content names `start`, which is the `launcher-filename-unchanged` decision made executable — without it, a later tidying pass renames the file and silently breaks teardown on every existing hub.

**`internal/loomcli/parity_test.go`, `validate_test.go`, `status_test.go`, `step_test.go`, `friction_test.go`, and the smoke/integration files** — these exercise verbs that are not being renamed, but several construct commands by name or assert on help text.
Sweep them for verb-name literals and update only what names a renamed verb;
do not restructure them.

**Docs gate.**
The Markdown Link Integrity test already fails on a dangling link, so deleting the design doc without fixing both inbound links is caught automatically.
Run the full suite rather than package-scoped tests, since the enforcement scans in `internal/lyxcwd` and the link-integrity test walk the whole tree.

**Whole-suite verification.**
`go test ./...` with `CGO_ENABLED=1`, plus a `go build ./...`, is the completion gate.
A rename that compiles and passes both, with the three new guards in place, has landed.

## Q&A log

- **Q:** What are the final verb names? **A:** [auto-pick] `drive` → `run`, today's `run` → `start`, `step` unchanged. **Why:** `run` then names the verb that calls `Shed.Run`, which is the task's whole point; `step` already matches `Shed.Step`, so despite the roadmap title naming all three verbs it needs no rename.
- **Q:** What happens to the root alias `lyx run`? **A:** [auto-pick] It becomes `lyx start`, delegating to `lyx loom start`. **Why:** the alias function returns the subtree verb's command unchanged, so the alias's name *is* the subtree verb's name — keeping them equal is free, and the alias must keep pointing at the bootstrap since that is the everyday call.
- **Q:** Keep back-compat aliases for the old names? **A:** [auto-pick] No — hard rename. **Why:** pre-release with one operator, and `run` is being reused with a new meaning, so an alias for the old `run` is not even expressible.
- **Q:** What does `ly-supervise` become? **A:** [auto-pick] `ly-drive`. **Why:** the skill drives rather than observes, and the CLI rename frees `drive` from meaning a loom verb, so the word carries no competing meaning.
- **Q:** Does the `_launchers` script file `run<ext>` get renamed too? **A:** [auto-pick] No — only the command inside it changes to `loom start`. **Why:** renaming the file would orphan a stale `run<ext>` in every existing hub, which would invoke the new foreground driver and would also break `removeLaunchers`' non-recursive directory removal, since that function deletes by an explicit name list.
- **Q:** Do Go filenames and identifiers follow the verbs? **A:** [auto-pick] Yes — `run.go`→`start.go`, `drive.go`→`run.go`, `runCmd`→`startCmd`, `driveCmd`→`runCmd`, `RunAliasCommand`→`StartAliasCommand`. **Why:** leaving them on the old words creates a fresh name/behaviour asymmetry one layer down, inside the package being cleaned up.
- **Q:** Code-and-tests now with a docs follow-up, or everything in one commit? **A:** [auto-pick] One commit, repo-wide. **Why:** ~233 matches across 57 files; a partial rename leaves specs and design docs asserting untruths, and CLAUDE.md requires docs in the same commit.
- **Q:** How is the `contracts/specs/` hash-mismatch on already-seeded hubs handled? **A:** [auto-pick] Accepted as pre-existing `stencilstore` behaviour; no force-sync. **Why:** the Stencil Ownership Invariant bans a force-sync carve-out for specs, and this hazard is not introduced by the rename.
- **Q:** What happens to `manifest/designs/loom-cli-rename.md` on landing? **A:** [auto-pick] Deleted, with the roadmap item moved Planned→Done and both inbound links fixed. **Why:** the Documentation Lifecycle deletes a design doc when its work lands, and Markdown Link Integrity fails the build on a dangling link.
- **Q:** Should Scope enumerate the files to change, or state a rule for finding them? **A:** [auto-pick] A rule — a bounded grep set run over the whole tree, with every hit read and classified into rename / leave / structural. **Why:** review round 2 demonstrated the enumeration was already short by inspection, missing `README.md` entirely (including its own copy of the alias sentence) and every verb-naming comment in `loomengine`, `frictionengine`, and `webstercli`. With ~233 hits across ~57 files, a whitelist cannot be both complete and maintainable, and a silently-short one is worse than no list because it reads as authoritative.
- **Q:** Does the rename need a new exact-set test proving `drive` is gone? **A:** [auto-pick] No for the loom subtree, yes for the root tree. **Why:** `internal/loomcli/cli_test.go:43-77`'s `TestCommand_RegisteredVerbs_ExactSet` already checks both directions against a `want` literal, so it fails on a surviving `drive` — and its `want` is a mandatory edit anyway. Duplicating it would leave two tests to keep in sync for one property. The root tree genuinely has no absence guard, so the new test is scoped to that alone.
- **Q:** Do the split-argv Go sites (`exec.Command(exe, "loom", "drive")`, the `/proc` argv probe, the smoke-test invocations) need their own decision? **A:** [auto-pick] Yes — their own grep pattern and their own decision, since no textual pattern finds them. **Why:** because `run` is reused rather than retired, a missed `"loom", "run"` fails silently instead of loudly — it invokes the new foreground driver against an unbootstrapped worktree rather than erroring as an unknown verb. The `/proc` probe inverts the same way: left scanning for `"drive"` it matches nothing forever, so every assertion built on it passes vacuously.
- **Q:** Is there a hazard in doing this as a textual find-and-replace? **A:** [auto-pick] Yes — it must be done by reading each hit. **Why:** "run" appears as a noun meaning "an execution" in at least four unrelated files (`burlercli/wiring.go`, `githubclient/doc.go`, `landingshed/deps.go`, `selfreportengine/selfreport.go`), and `drive.go`'s `Long` string contrasts itself against `lyx loom run` in sentences a blind replace would turn into self-references.
