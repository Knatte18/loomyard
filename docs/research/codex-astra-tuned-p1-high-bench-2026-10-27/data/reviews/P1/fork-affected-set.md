Affected-set coverage:

- Gate shape, keys, migration, rendering, and failure kinds: cards 1–5 cover gateslot, Webster, loom wiring, landing, recipe constructors, and their named fixtures.
- Darn tree scope and retired `tree_scope_verify`: card 6 covers config, migration, editor refusal, wiring, tests, and docs.
- Green carry, diff bases, run records, summaries, counters, and reports: cards 7–10 cover gate execution and record consumers.
- Tag-dependent package loads and guards: cards 11–16 cover impactset, planglyph, planparser, shellguard, testtiming, purity, LLM, and isolation scans; the tree-sitter dependency scan and tmuxkit’s runtime permission switch remain uncovered.
- Entry state, goto/relaunch, routing, histories, settings, and consumers: cards 17–21 cover the principal engine, reader, builder, batten, and darn paths.
- Documentation, retagging, and transition cleanup: cards 22–33 cover the listed packages and follow-up entry, but neither the retag instructions nor follow-up scope addresses the two uncovered tag consumers below.

Finding:

**BLOCKING — design — `_lyx/plan/11-package-load-tags.md`, cards 24–33**  
**The tag migration enumerates only part of the existing tag-dependent consumers.**

Card 11 promises that every Go package load listing tests moves to both spellings, but `TestTreeSitterLink_OnlyAllowedTestBinariesLinkIt` still invokes `go list -test -deps -tags integration,tmux,llm` at `cmd/lyx/treesitterlink_integration_test.go:40`.
Card 24 changes that file’s build header only; no card changes this command.
Once other test files become `tier0`/`tier1`/`tier2`/`tier3`, this guard excludes them, contrary to D5 and `PATTERN-test-speed`’s requirement that the tree-sitter scan inspect every tier.
The guard also rejects allowlisted packages missing from its reduced dependency graph (`cmd/lyx/treesitterlink_test.go:114`).
For example, the two test files directly importing planglyph in loomshed are untagged and have no guard marker (`internal/loomshed/gates_test.go:30`, `internal/loomshed/gatefindings_test.go:27`); card 26 changes them to `tier0`, which this scan does not request.

The same enumeration omission leaves `internal/testkit/tmuxkit/allow_tmux.go:1–9` accepting only `tmux || llm`.
Its `init` controls `tmuxAllowed`, and `afterRun` passes that switch to `checkSockets` (`internal/testkit/tmuxkit/tmuxkit.go:105`), which fails even registered surviving sockets when false (`:145–147`).
The transitional hub commands mask this omission by retaining old tags, but card 33’s prescribed removal of those tags exposes it: a pure `tier2` or `tier3` run compiles the retagged tests without enabling their runtime tmux allowance.
Card 29 only retags test files, whereas this switch is a non-test Go file, and card 33’s follow-up scope does not name it.
These are one recurring affected-set enumeration defect, rather than independent per-file findings.

Tool-call record:

1. Searched gate-key, retired-config, and Command-field references outside the inherited definitions to enumerate remaining consumers.
2. Read Webster’s gate-config caller and searched tag-dependent loaders and fixtures absent from the manifest.
3. Read tmuxkit’s build-tag switch and the tree-sitter dependency scan because neither was in the inherited reading.
4. Traced tree-sitter imports and history-outcome consumers beyond the named cards to test the affected-set boundaries.
5. Read tmuxkit’s runtime socket check and darn’s latest-outcome consumer to establish the consequences of those dependencies.
6. Checked the loomshed import files’ tags and markers and the tree-sitter scan’s stale-allowlist check to ground the migration finding.
