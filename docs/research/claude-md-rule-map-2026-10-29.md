# CLAUDE.md rule map, 2026-10-29

Each rule of `CLAUDE.md` mapped to where lym's agents already get it, so the operator decides per rule before the file is deleted.
Line numbers are read from this worktree at the commit that adds this report; the report is a snapshot and changes no other file.

## How CLAUDE.md and PATTERN.md reach agents

`CLAUDE.md` reaches an agent through two routes:

- Claude Code loads it as an instruction file in every session whose working directory is a worktree of this repository: `internal/shuttleengine/claudeengine/capabilities.go:33` names `CLAUDE.md` and `CLAUDE.local.md` as the instruction files.
  Deleting it removes every rule below from every such session at once, whether or not the role needs it.
- Three stencils tell the agent to read it: `contracts/stencils/darn/darn-template-write.md:26` and `:76`, and `contracts/stencils/drift/drift-template-baseline.md:22`.
  `contracts/stencils/orch/orch-template-role.md:186` points orchs at it as the place for workarounds every agent needs.

`PATTERN.md` reaches an agent only by injection, through `pattern.Directive` (`internal/pattern/pattern.go:106`), which fills a role's directive stencil with the overview and returns nothing when the role is unknown.
Roles that receive it:

| Role | Call site | Variant |
|---|---|---|
| Loom discussion writer and advisors | `internal/loomengine/discussiontable.go:85` | `RoleDesigner` |
| Loom plan writer | `internal/loomengine/plan.go:76` | `RoleImplementer` |
| Darn writer | `internal/darnengine/spec.go:88` | `RoleImplementer` |
| Drift verify seat | `internal/driftengine/seats.go:211` | `RoleImplementer` |
| Webster Merriam (Master) | `internal/websterengine/render.go:487`, `internal/websterengine/merriambase.go:61` | `RoleOrchestrator` |
| Webster recovery and cold verify fixer | `internal/websterengine/render.go:247`, `internal/websterengine/render.go:383` | `RoleImplementer` |
| Bouncer judge | `internal/shedadapters/bouncer.go:1761` | `RoleJudge` |
| Burler review half | `internal/burlerengine/review.go:91` | `RoleReviewFix` |

Roles whose stencil has no `pattern_directive` marker, found by searching `contracts/stencils/` for the marker, receive no `PATTERN.md` and no `pattern/` pointer:

- the orch role (`contracts/stencils/orch/`);
- the burler fixer (`contracts/stencils/burler/burler-template-fixer.md`);
- the landing writers: describe, conflict, fixer and fix-review (`contracts/stencils/landing/`);
- the treadle judges, twill, the shed driver and tackler, the loom arbiter and chair, the webster plan fixer, the beam board-tidy session and the friction reflection;
- the implementer forks of webster, which inherit Merriam's session and so the orchestrator directive, never a directive of their own.

The edit and parent directives (`contracts/stencils/edit/edit-directive.md`, `contracts/stencils/parent/`) are rendered into every spawned role's opening stencil and carry no repository rule beyond editing and escalation.

## Rule table

| Rule (CLAUDE.md line) | Where it already stands | Agents that need it | Verdict |
|---|---|---|---|
| "This repository develops LoomYard and is itself developed by LoomYard" (L3) | none for agents; `docs/overview.md:102` states the records contract | none act on it | `drop`: background, no instruction |
| "Read `PATTERN.md`, and the `pattern/` background files that touch a change, before writing or reviewing code" (L7) | `contracts/stencils/pattern/pattern-directive-implementer.md:18`, `-designer.md:18`, `-review-fix.md:18`, `-orchestrator.md:18` | every role that edits or reviews code | `covered`: the directive carries the overview and the read-the-background step; the burler fixer and landing writers get neither, see the gap list |
| "Its invariants are enforced partly by `go test` and partly by review" (L8) | each overview entry's `(test)` suffix in `PATTERN.md` | none | `covered`: the entries mark their own enforcement |
| "A new cross-cutting invariant goes into PATTERN in the same commit as the code" (L9) | loom: `## PATTERN changes` in `contracts/specs/loom-discussion-spec.md` and the plan checks at `internal/loomshed/patternplan.go:114`; darn and drift: none | darn writer, drift seats | `move to` `contracts/stencils/darn/darn-template-write.md` step 4.3 and `contracts/stencils/drift/drift-template-baseline.md`: loom enforces it, the other two roles lose it |
| "`lym` links tree-sitter grammars through cgo, so a build needs `CGO_ENABLED=1` … and a C compiler" (L13) | `README.md:220`; `cmd/lym/crosscompile_test.go:14`; quarry's `cgoguard` fails the compile under `CGO_ENABLED=0` | any agent that builds or gates | `covered`: the build refuses `CGO_ENABLED=0` itself and README states the toolchain |
| "`update-plugins.sh` … is the only route to production: from a clean, pushed `main`, it deploys the plugins and builds `lym` into the Go bin dir" (L17) | `README.md:212`; `update-plugins.sh:4`; `tools/deploy/main.go:4` refuses a dirty tree or an unpushed HEAD; `internal/orchcli/doc.go:87` and `contracts/stencils/orch/orch-template-role.md:142` for the landing `deploy` key | orch, any agent that deploys | `covered`: the script enforces its own precondition and README states the route |
| "Never run it unasked, never install `lym` elsewhere" (L18) | none | orch, darn writer, implementers | `move to` the hub-wide orch rules file through `lym orch rules` (`contracts/stencils/orch/orch-template-role.md:186`), the one role the orch doc says deploys by hand; no code stops another install path |
| "`./deploy-dev` builds into `.dev-bin` instead" (L19) | `README.md:213`; `deploy-dev:3` | agents running internal tests | `covered`: README and the launcher header |
| "There is no versioning: plugins stay at `1.0.0`" (L20) | `.claude-plugin/marketplace.json:4`; `tools/deploy/main_test.go:60` pins the `ly/1.0.0` target | agents editing plugins | `covered`: the manifest and the deploy test hold the value |
| "A rule for developing LoomYard itself goes in this file … A rule for every use of lym … goes into lym … never into this file … nor into a package's `doc.go`" (L24–26) | `contracts/stencils/orch/orch-template-role.md:186` points at `CLAUDE.md`; nothing else states the placement | orch, darn writer | `move to` `contracts/stencils/orch/orch-template-role.md:186`: rewrite the pointer to name the orch rules file and the stencils, since the file it names goes away |
| "Push to `main` only from the worktree checked out on `main`; never from a task pair" (L30) | `contracts/stencils/darn/darn-template-write.md:78`, `contracts/stencils/burler/burler-step-fix.md:59`, `contracts/stencils/landing/landing-template-fixer.md:41` tell roles never to push; `contracts/stencils/orch/orch-template-role.md:129` has Finalize push `main`; `PATTERN-parent-lock` limits the move to a run's Finalize | every role in a task pair, the orch | `covered`: Finalize is the one pusher and the roles are told never to push; a hand push by an operator session has no home |
| "Push to `main` only after `lym gate test --upto 2` is green on `main` … run as its own Bash call … `tools/` follows PATTERN … a docs-only commit is pushed without the test run" (L31–33) | `internal/gatecli/cli.go:63` and `:83` describe `--upto`; the gate rule is in the session system prompt, not in the repository | operator-driven sessions pushing `main` by hand | `move to` the orch rules file through `lym orch rules`: no lym role pushes by hand, and the `--upto 2` run and docs-only exemption stand nowhere else |
| "Work only in your own worktree: never edit, commit or push in another, and never create one, unless the user says so; if work belongs elsewhere, say so and ask" (L34–35) | `contracts/stencils/orch/orch-template-role.md:114` for task pairs; `contracts/stencils/webster/webster-body-implementer.md:83` bans mutating `lym fabric` verbs | every spawned role | `move to` `contracts/stencils/edit/edit-directive.md`: that stencil is rendered into every spawned role, and the rule reaches none of them otherwise |
| "Every agent lym spawns runs as an interactive tmux session, driven through reed, never `claude -p`" (L37–39) | `README.md:96`; `internal/shuttleengine/doc.go:8`; no scan or test forbids a headless spawn | agents that write spawn code | `move to` `PATTERN.md` as an entry for spawning an agent, with a scan for a `claude` spawn carrying `-p`; today it is doc text only |
| "A task that adds a module, changes observable CLI behavior or adds cross-cutting infrastructure updates, in the same commit: the module doc in `doc.go`, `docs/overview.md` …, and `PATTERN.md`" (L43) | `contracts/stencils/darn/darn-template-write.md:76` and `contracts/stencils/drift/drift-template-baseline.md:22` point to `CLAUDE.md` for it; loom: `doc-go-unlisted` plan check at `contracts/stencils/loom/loom-template-plan.md:98` | darn writer, drift seats | `move to` `contracts/stencils/darn/darn-template-write.md` step 4.3 and `drift-template-baseline.md:22`: both name `CLAUDE.md` as the only source of the doc list |
| "A built design lives in its package's `doc.go`, written there by the task that builds it" (L44) | `docs/overview.md:97`; `contracts/stencils/loom/loom-template-plan.md:98` | plan writer, darn writer | `covered`: overview states it and the plan check enforces it for loom; darn takes the line with the row above |
| "One sentence per line, never fixed-column wrapping, in every `.md` file; see `scribe:prose`. Table cells and blockquotes stay on one line" (L48–49) | the `scribe:prose` skill, loaded at `internal/darnengine/spec.go:31`, `internal/websterengine/roles.go:33`, `internal/loomengine/discussion.go:9`, `internal/landingshed/describe.go:54`; `plugins/ly/skills/board/SKILL.md:166` | every role that writes markdown | `covered`: the skill is delivered on the launch spec of the writing roles |
| "These are conversational shorthands; never rename code, files or docs to them unless told to" (L53) | none | implementers | `drop`: the stencils use the real names and no agent has cause to rename to a shorthand |
| "**Merriam**: webster's orchestrating session, named `Master` in `internal/websterengine`; its strand is named `<shortname>:<slug>:webster`" (L55–56) | `contracts/stencils/webster/webster-prefix-fork.md:27`, `webster-prefix-verify-fix-cold.md:27`; `docs/overview.md:416` | webster roles, orch | `covered`: the webster stencils and overview use the term |
| "**perch**: a `Bouncer` row … whose `OnStuck` points at a `Burler`-round row … there is no perch type" (L57–58) | `internal/runprofile/doc.go:5` and `:11` | orch, darn writer reading profiles | `covered`: the profile doc uses and explains the term |
| "**board**: the task tracker, `lym board`; **Bolt** is the nickname of the records repository's `main` branch" (L59) | `plugins/ly/skills/board/SKILL.md`; `internal/cleanupcli/doc.go:17`; `internal/runprofile/doc.go:1` | orch, any agent reading the board | `covered`: the board skill and the package docs use both terms |
| "All links go through `internal/fslink` (`CreateDirLink`): directory junctions on Windows, symlinks elsewhere. Never rely on Windows file symlinks" (L63–64) | `internal/fslink/fslink.go` package doc; no PATTERN entry; no scan for `os.Symlink` outside `internal/fslink`; `internal/fabricengine/doc.go:1641` notes the rule's Windows claim is unsettled | agents adding code that links | `move to` `PATTERN.md` as an entry with a scan for `os.Symlink` and `os.Link` outside `internal/fslink`; today only `CLAUDE.md` states it as a rule |

## Rules whose only home is CLAUDE.md

These stand nowhere else for agents, so deleting the file loses them for the roles named:

- "Never run `update-plugins.sh` unasked, never install `lym` elsewhere": lost by orch, darn writer and implementers.
- The `--upto 2` gate before a push to `main`, the own-Bash-call rule and the docs-only exemption: lost by an operator session pushing by hand.
- "Work only in your own worktree … never create one": lost by every spawned role.
- The placement rule for where a new rule goes: lost by the orch, whose stencil at `orch-template-role.md:186` points at it.
- The docs-in-the-same-commit list: lost by the darn writer and the drift seats, whose stencils name `CLAUDE.md` as its source.
- The PATTERN-in-the-same-commit rule: lost by the darn writer and the drift seats; loom enforces it in its plan checks.
- The filesystem-links rule: lost by any agent adding code that links.
- The interactive-tmux rule: stated in `README.md:96` and `internal/shuttleengine/doc.go:8`, which agents do not receive by injection, and enforced by no check.

Three stencils and one calibration constant name the file itself and need an edit when it goes: `contracts/stencils/darn/darn-template-write.md:26`, `contracts/stencils/drift/drift-template-baseline.md:22`, `contracts/stencils/orch/orch-template-role.md:186`, and the comment at `internal/websterengine/merriambase.go:22`, whose `merriamFixedContext` counts `CLAUDE.md`'s size in Merriam's start base.

## Rules enforced in code

| Rule | Check |
|---|---|
| a build needs `CGO_ENABLED=1` | quarry's `cgoguard` fails the compile under `CGO_ENABLED=0`, and `cmd/lym/crosscompile_test.go:14` builds with it set |
| production deploy from a clean, pushed `main` | `tools/deploy/main.go:4` and `:55` refuse a dirty tree or a HEAD not pushed to `origin/main` |
| plugins stay at `1.0.0` | `tools/deploy/main_test.go:60` pins the manifest version and the install target |
| a new invariant and its doc land with the change, for loom runs | the plan gate's PATTERN checks at `internal/loomshed/patternplan.go:114` and the `doc-go-unlisted` check named at `contracts/stencils/loom/loom-template-plan.md:98` |
| only a run's Finalize moves a local parent branch | `PATTERN-parent-lock` through `internal/parentlock`; the entry carries no `(test)` mark, so review also holds it |

Every other rule above is held by prose in a stencil, a README or a package doc, or by review.
