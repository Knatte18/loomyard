# Unused features: verbs and config knobs with no recent use

A candidate list for the operator, who decides which entries go.
Nothing is removed by this study.
It records one measurement taken on 2026-11-18 at commit `64e58795` and is not rewritten when the code changes.

## Method

- **Verbs.**
  `collect.sh 2026-10-28` walks the `lym` command tree and counts, per verb, the `Bash` tool commands in every Claude Code
  transcript under `~/.claude/projects` modified since 2026-10-28 that start `lym <verb>`.
  Every leaf and group verb of every module is counted, 250 in all, and 34 of them have a count of 0.
- **Config keys.**
  `lym config --resolved <module>` annotates each key with the layer that set it.
  A key whose layer is `template` is one no layer of this hub overrides.
- **Landed features.**
  Landing commits on `main` since 2026-11-10 were searched for the text a feature leaves in its commit message.

## What the counts cannot see

- A verb the operator types in their own terminal leaves no transcript.
  The operator-facing verbs below (`fabric *`, `config menu`, `ide menu`, `slack away`, `tender reap`) are the most exposed to this.
- Only Claude transcripts are searched; a Codex session's commands are not counted.
- A verb a Go caller spawns itself is not a `Bash` tool command.
  The "referenced by" column separates a verb nothing but its own package mentions from one a stencil, recipe or refusal tells agents to run.
- One hub (this one) and about three weeks of transcripts: a feature a second hub uses, or one that fires rarely, can read as unused.
- Flags are not counted.
  A flag list needs per-flag parsing of the command lines and is left to the operator's choice of verbs to pursue.

## Verbs with no transcript use

### Nothing but its own package refers to them (strongest candidates)

| Verb | Note |
| --- | --- |
| `board rerender` | no reference anywhere outside tests |
| `ide menu` | no reference anywhere outside tests |
| `board retire-legacy` | a one-off migration verb; its own CLI is the only reference |
| `creel forward`, `creel hold`, `creel release` | declared in `internal/creelcli/cli.go`, named nowhere else |
| `webster await-batch` | `internal/webstercli/awaitbatch.go` only |
| `loom restore-renumber` | `internal/loomcli` only |
| `orch withdraw` | `internal/orchcli` only |
| `slack away` | `internal/slackcli`, `internal/slackengine/start.go` |
| `stencil promote` | `internal/stencilcli`, `internal/stencilstore/reconcile.go` |
| `report raddle` | `internal/reportcli`, `internal/boardcli` |
| `fabric diff` | `internal/fabriccli/weft_verbs.go` and the engine's package doc |
| `claude account`, `claude account set`, `claude account show` | `internal/claudecli` and `docs/reference/claude-accounts.md` |
| `config menu` | `internal/configcli` and the overview |

### Referenced by a stencil, recipe, guard or another package, still never run

These are in a prompt or a refusal's way forward, so an agent could meet them, but none did in the window.
Whether they are unused or only rare needs the operator's call.

| Verb | Referenced by |
| --- | --- |
| `loom pr-findings` | the darn, sampler and tapestry recipes and the PR-review stencil |
| `loom parent-review await` | the parent-consolidation stencil |
| `webster restore-plan` | the Master and plan-fix stencils and `websterengine` |
| `batten lock release` | `landingshed/parentguard.go` and the orch role |
| `beam pr close` | `beamcli` refusals; pair teardown closes the pull request itself |
| `loom arbiter` | `loomcli/driverprompt.go` |
| `fabric cleanup`, `fabric prune`, `fabric checkout`, `fabric unwire`, `fabric status` | batten, the shed driver guide, `fabricengine` |
| `tender reap` | `beamcli`, `runproc` |
| `weaver reset` | `weaverengine`, `loomshed` |
| `reed watchdog` | `reedengine/spawnwatchdog.go` |
| `burler run`, `shed run` | `burlerengine/fix.go`, `battencli/arm.go` |

## Verbs with one to five uses

`beam pr update` 2, `creel daemon` 2, `heddle remove` 2, `loom commit-records` 2, `slack back` 2, `stencil validate` 2,
`webster fix-plan` 2, `webster rebaseline` 2, `batten goto` 3, `claude` 3, `loom parent-review answer` 4,
`board upsert-batch` 5, `fabric sync` 5, `machine start` 5, `orch stop` 5.

## The features the `landingshed` reading named

| Feature | Evidence of use since 2026-11-10 |
| --- | --- |
| Flaky rerun | 8 of 151 landing commits carry a `## Flaky test failures` section, so it fires |
| Beam landing record | `lym beam land` has 24 transcript uses, `lym beam describe` 24; in use |
| Adopter-first repair note | no evidence either way: the note is text in a Stuck reason, and no record of a Stuck reason is kept to count |

## Config keys no layer of this hub overrides

Counts of top-level keys per module, read from `lym config --resolved`.
An untuned key is a knob nobody here has turned; the operator may judge it a candidate to become a constant.
Modules with no override and the most keys: `batten`, `shuttle`, `shed`, `webster`, `fabric`, `sampler`.

| Module | Untuned | Overridden |
| --- | --- | --- |
| loom | 51 | 7 |
| tapestry | 22 | 1 |
| landing | 16 | 3 |
| shuttle | 16 | 0 |
| batten | 12 | 0 |
| reed | 11 | 2 |
| orch | 10 | 2 |
| codex | 9 | 1 |
| gate | 9 | 7 |
| drift | 8 | 3 |
| shed | 7 | 0 |
| claude | 6 | 4 |
| webster | 6 | 0 |
| beam | 5 | 2 |
| fabric | 4 | 0 |
| darn | 4 | 2 |
| board | 3 | 2 |
| sampler | 2 | 0 |
| lenses | 2 | 2 |
| weaver | 2 | 1 |
| agents, desktop, hibernate, ide, language, logger, report, slack | 1 each | 0 to 2 |

## How to read the list

Start with the first table: no prompt, recipe or code path outside the verb's own package names it, and no transcript ran it.
The second table needs a look at each caller before any removal, since removing the verb also means removing the prompt text that names it.
Nothing here is a finding that a feature is dead; it is the set the operator may choose from.
