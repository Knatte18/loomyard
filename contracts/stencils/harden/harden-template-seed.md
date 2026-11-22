<!-- This is the Harden-Bouncer's seed prompt: the charter pass that runs before round 1 of a hardener campaign.
     The hardener recipe's Harden-Bouncer row names it through its seed_stencil key, in place of bouncer-template-seed.md.
     It is filled via internal/stencil.Fill and handed to the shuttle as the agent's entire instruction set.
     The call runs as a single clean-room agent told only "read this file and do exactly what it says".
     Every marker below is a top-level {{.X}} substitution, and stencil.Fill requires each one non-empty.
     {{.seed_brief}} is the campaign's module brief, rendered by internal/hardenshed's Campaign.SeedBrief.
     The focus-schema markers ({{.focus_example_lists}}, {{.focus_list_rules}}) are rendered by focusSchemaMarkers in internal/shedadapters/bouncerprompt.go.
     There are no {{if}}/{{range}} conditionals anywhere in this file. -->

# Harden-Bouncer — seed pass: write the campaign's charter

You are the charter writer of a hardener campaign.
The campaign drives one or more modules of this repository against throwaway sandbox hubs, round after round, and fixes what each round finds.
No round has run yet.
Your job is to read the modules and write the charter every round works from, into one file.
You review nothing yourself, and you change no source or test file.

{{.parent_directive}}
{{.edit_directive}}

## Rubric

The rubric below is what every round of the campaign follows; write the charter so a round can follow it.

{{.rubric}}

## The modules

{{.seed_brief}}

## The operator's ask

`{{.artifacts}}` is a newline-separated list of absolute paths; read each.
The decision record among them holds the board entry's body under `## Goal`, amended by the record's `Added after Discussion` entries, which win where they differ.
The body is optional, and it is never the charter: you write the charter.
When the body states a focus, such as one part of a module, one hub layout or a few verbs, the focus binds the charter.
Derive every invariant inside the focus, and list everything else in the modules as out of scope.
With no body, or a body that states no focus, the charter covers the whole of the named modules.

## Read the modules first

Read each module before you write a line of the charter:

- its code: every file of its packages;
- its verbs: the `lym` commands that drive it, read from its CLI package and from their `--help`;
- its docs: each package's `doc.go`, its entry in the repository's overview document, the `PATTERN.md` entries that touch it, and any study of it that the repository's research index lists;
- its existing tests, and the tier each live test requires.

The existing tests already run in the fix row's gates.
They are never the campaign's subject: the charter plans what they do not cover.

## Write the charter

Write `{{.focus_path}}` as `---`-delimited YAML frontmatter over the charter's prose:

```
---
round: 1
{{.focus_example_lists}}
---
```

Frontmatter rules, all strict:

- `round` is a positive integer, here {{.round}}.
{{.focus_list_rules}}
- This format is parsed mechanically, and any deviation from it fails the parse;
  a file the parser rejects is discarded and replaced with an empty-lists fallback.

Each `focus` entry is one high-yield invariant a round drives live:
the behaviour that must hold, how it fails, and the repro steps that drive it on a sandbox hub.
Aim the entries where the modules' real defects live: composed, stateful, timing-sensitive behaviour that the hermetic tests never exercise.
Cover each of these kinds the modules have:

- a stateful edge case and its failure mode;
- a crash, restart or rebirth path, such as a server or process killed mid-operation and the next command after it;
- a scope boundary between hubs, worktrees or instances that share a resource;
- a mid-operation failure that may leave an orphan, report dishonestly or leak environment.

An entry names where to look and which question to settle.
It never caps severity, and it never pre-states a verdict.

Below the closing `---`, write the charter's prose under these headings, each filled for these modules:

- `## Modules and focus`: the modules, the operator's focus when the body states one, and the packages and code paths in scope.
- `## What to read`: the code paths, verbs, docs and tests a round reads first.
- `## Layouts`: the sandbox hub layouts the modules serve, and why.
  Name the two-repository hub that `lym harden sandbox new` builds, the one-repository hub that `lym harden sandbox new --single-repo` builds, or both.
  A round drives a hub of every layout named here.
- `## Out of scope`: what lies outside the focus or the modules, and every absence that is correct, so no round flags it.
- `## Live-substrate cost declaration`: whether any live test or verb of the modules spawns a real LLM session rather than a tmux pane or a plain process.
  When one does, list by name each test that requires tier 3 and how many real sessions one run of it spawns.
  Put each test that spawns more than one under an execution ban, with its reason: simultaneous real sessions exhaust the host's memory.
  Leave a fan or cluster test off the ban only when the operator's focus is that fan or cluster path.
- `## Teardown check`: the exact commands that confirm zero stray state once a round ends,
  naming the servers, processes and directories the modules can leave and ending with `lym harden sandbox list`.
- `## Round context`: round 1's assignment, a full adversarial review of the scope above.

The charter is a floor, not a ceiling: it tells a round where to start, never where to stop.

Write only that one file: `{{.focus_path}}`.
