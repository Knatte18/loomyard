# Loom plan-checks spec — the plan gate's validation checks

> **Status: Contract — pinned.** This doc pins the machine checks over **plan-format**, whose card format `loom-plan-spec.md` defines. `lym loom validate-plan` prints each finding with its check ID, its card and its way forward; an entry here says what raises the check, for a finding the printed way forward does not settle.

## Where the checks run

- **`internal/planparser`** checks plan text.
  `ValidateFormat` runs every planparser check but `plan-unapproved`, and `Validate` runs them all.
  Both plan gates, `Plan-Write`'s and `Plan-Burler-Fix`'s, run the format set, since review writes `approved: true` after them; the standalone consumers `internal/websterengine`, `internal/webstercli` and `internal/batcher` run `Validate`.
  The order of the findings is the order `internal/planparser/validate.go`'s file comment lists.
- **`internal/planglyph`**'s resolve pass adds the findings that read the tree on top of both entry points; its package doc lists every one.
  Entries below mark them *planglyph*.
- **The plan index** appends `card-fabric-reference`, `fabric-vocabulary-name` and `edit-test-file` to its format and rework validation, and `fabric-vocabulary-name` to dispatch too.
- **`loomshed`** appends the context checks, marked *loomshed*, in every `lym loom validate-plan` mode but `--require-approved`: the default, `--lens-review`, `--overlay`, `--rework` and `--part`, the plan gates, PR-Rework and a part writer's Accept.
  `--part` runs them without the decision record, which leaves the record's checks to the joined plan's gate.
- **Dispatch** (`ValidateDispatch`, at Webster's begin-batch) runs the format set without a type load and drops findings against completed cards; an entry says when a check skips it.
- A done card is frozen, so the context checks skip it.

Under `language: none` the glyph checks are skipped: `bare-symbol-target`, `directory-target`, `glyph-malformed`, `resign-not-member`, `redundant-package-target`, `redundant-file-target`, `resign-head-mismatch`, `delete-before-reference`, `caller-uncovered`, `edit-test-file` and the resolve findings.
`loom-plan-csharp-spec.md` lists what changes under `language: csharp`.

## Frontmatter, index and numbering

- `format-unrecognized` — `format:` is not `5`.
- `plan-language-unrecognized` — `language:` is set to anything but `go`, `csharp` or `none`.
- `plan-language-mismatch` — `language:` does not fit the files the cards target: under `go`, the file targets are `.cs` files and no `.go` file, or *(planglyph)* the targets resolve into `.cs` files only; under `csharp`, they are `.go` files and no `.cs` file.
  A member glyph, a `plan:` handle or an extensionless self glyph silences the plan-text half.
  The resolve half runs only while no card is done, and is silent when a card creates a declaration whose head opens with a Go keyword.
  Way forward: set `language:` to the files' language, or to `none` for a plan that changes both.
- `plan-unapproved` — `approved:` is not `true`.
  Only `Validate` runs it; the plan writer never sets the flag, and Plan-Bouncer's approved settle writes it.
- `index-file-mismatch` — the Card Index and the card files disagree: numbering from `first_card` upward, slugs, a gap, or a card file the index does not list.
  A `first_card:` that is no positive integer is a finding of its own, and numbering is then checked from `1`.
- `card-numbering` — a card's heading number differs from its Card Index number.
- `commit-subject-mismatch` — a `Commit:` value does not start with the card's `N: `.
- `rework-first-card` — rework gate only: `first_card:` differs from the card number Go told the rework session to start at.

## Card fields

- `card-type-missing` — a card carries no type label.
- `card-custom-not-alone` — a card carries a `Custom` group beside a group of another type; one finding per card.
- `card-retired-label` — a format-3 label: `**What:**`, `**Context:**`, `**Edits:**`, `**Creates:**`, `**Deletes:**`, `**Moves:**`, `**Depends-on:**` or the lowercase `**verify:**`; one finding per occurrence.
- `card-missing-field` — a card lacks `Intent:`, or lacks `ImpactSummary:` while a group is `Edit`, `Extends` or `Delete`.
- `card-field-empty` — a label with nothing under it: an empty target group, an empty `Uses:`, a blank `Intent:` or `ImpactSummary:`.
- `card-field-overlap` — one entry in both a card's targets and its `Uses:`.
- `impact-summary-multiline` — lines that are not a label follow `ImpactSummary:`.
- `extends-overlap` — a ref under `Extends` and under another type label of the same card.
  Way forward: keep it under one label, `**Edit:**` when the card changes what is there.
- `extends-resign` — an arrow bullet under `Extends`.
  Way forward: move the ref under `**Edit:**`.

## Ref shapes and paths

- `card-path-malformed` — a path or self glyph in a card's targets or `Uses` whose disk path, after `root:` and `//` resolution, is empty, absolute, unclean or holds `..`.
- `bare-symbol-target` — a bare symbol (shape rule 5 of `loom-plan-spec.md`).
  Way forward: spell it as a glyph copied from a quarry answer.
- `directory-target` — a slashed path with no extension.
  Way forward: a package's unit glyph (`internal/foo#`), a non-code directory's files, or, for an extensionless file under a `root:` other than `.`, its repository-root-relative file self glyph (`internal/foo/Makefile#`).
- `glyph-malformed` — a ref holding `#` that `glyph.Parse` rejects: a doubled `#`, an empty unit, a member carrying a paren, a keyword.
- `prosa-symbol-target` — a `Prosa` group target that is no self glyph; under `none`, a symbol-shaped one.
- `path-missing` — a path or self glyph absent from disk and not created by any card's `Create` target or `Rename` `New` side.
  It reads every `Uses:` entry, the targets of `Edit`, `Extends`, `Delete`, `Move` and `Prosa` groups, and a `Rename` pair's `Old` side; `Custom` targets and member glyphs are exempt.
  A later card naming a file at the destination an earlier `Move` gave it is a false finding: reorder the cards, or name the pre-move path.
  A self glyph spelled relative to a `root:` other than `.` whose `root:`-joined path is present on disk or created by a card gets the way forward of its worktree-relative self glyph, or of the plain path `root:` joins.
- `glyph-not-found`, `glyph-ambiguous`, `glyph-rejected` *(planglyph)* — a glyph target that does not resolve, resolves to several declarations, or that quarry's grammar rejects.
  A `Create` target and a `Rename` `New` side are not resolved.
  A member declared once per mutually exclusive build-constraint set resolves as one member.
- `create-already-exists` *(planglyph)* — a `Create` target whose name a declaration already holds.
  `create-new-unit` is its informational partner for a target in a unit that does not exist yet.
- `redundant-package-target` — a package self glyph beside a member glyph, file self glyph or `plan:` handle of that package in one card's targets.
  A handle a `Rename` pair claims as its `New` side belongs to the `Old` glyph's unit.
  Way forward: keep the member glyphs and drop the package, or keep the package when the card changes all of it.
- `redundant-file-target` *(planglyph)* — a file self glyph beside a member glyph that resolves into that file.
  Way forward: keep the member glyphs and drop the file, or keep the file when the card changes all of it; for a `_test.go` file, keep the member glyphs.
  Not run at dispatch.

## Handles and renames

- `rename-format` — a `Rename:` bullet not in the `` `old` -> `new` `` form.
- `handle-dangling` — a handle in some card's targets or `Uses` that no `Create` bullet and no `Rename` `New` side declares.
- `handle-collision` — a handle declared twice: by two `Create` bullets, two `Rename` `New` sides, or one of each; one finding per handle.
- `handle-malformed` — a `Create` arrow bullet outside the `` `plan:<handle>` -> `<declaration head>` `` form, a handle with no `#`, or, under a glyph language, a `Create`-declared handle whose unit is a `.go` file instead of the package directory.
- `handle-name-failed` *(planglyph)* — a declaration head that does not parse as source or declares more than one symbol.
- `rename-to-not-handle` — a symbol pair's `New` side is no `plan:` handle; a file pair is exempt.
- `rename-from-not-glyph` — a symbol pair's `Old` side is no glyph, or is a self glyph beside a handle `New` side; a file pair is exempt.
- `rename-old-unresolved` *(planglyph)* — a pair's `Old` side does not resolve in the tree, as for a symbol the same plan creates.
  Way forward: give the `Create` target its final name.
- `rename-mechanic-missing` — a `Rename` group, but no `## Rename mechanic` section.
- `rename-leaves-test` — under `go`, a pair renames `unit#Name`, or the method `unit#T.M`, while a card lists a test named after it under `Edit` in the same unit and no pair renames that test.
  A test is named after it when it is `Test<base>` or `Test<base>_<case>`, with base `Name`, or `T_M` for a method.
  Way forward: add the test's rename as a pair on the renaming card.
- `rename-tool-missing` *(loomshed)* — a symbol pair of a pending card while `language.yaml` lists no `rename_tools`.
  Way forward: set `rename_tools` with `lym config language` from the prime, or recast the card.

## Re-signs, deletes and callers

- `resign-not-member` — an `Edit` arrow on a ref that is no member glyph.
  Way forward: put the arrow on the member glyph whose signature changes.
- `resign-head-mismatch` *(planglyph)* — a re-sign head that `quarry.Name` cannot name, or names as another member; a changed receiver type names another member, a value-to-pointer change does not.
  Way forward: write the member's new declaration head, receiver included for a method.
- `resign-interface-method` *(planglyph)* — an arrow on an interface method, which has no declaration a head can re-sign.
  Way forward: drop the arrow and state the change in the card's `Intent`.
  Not run at dispatch.
- `caller-uncovered` *(planglyph)* — Go code still references a member a card deletes or re-signs, and no admissible card covers the reference.
  A reference is covered by the file's self glyph, the package self glyph of its directory, or a member glyph whose body holds the line, on the re-signing card, or on the deleting card or an earlier one.
  References resolve by type where a `go/packages` load answers, and by an import-path token scan elsewhere; comments, strings and the directories the go tool ignores never match.
  A method matched by name alone, its receiver unresolved, is one informational finding.
  Way forward: list the file, or the member glyph whose body holds the reference, on that card, or for a delete on an earlier card; in a `_test.go` file, only the member glyph.
  Not run at dispatch or under a part scope.
  It does not see a caller in a file an earlier card creates, nor an interface break at an assignment site when a re-signed method stops satisfying it.
- `delete-before-reference` *(planglyph)* — a deleted member glyph or package self glyph that a later card's `Edit` or `Extends` code still references; blocking per line, informational for a method matched by name alone.
  Way forward: move the delete after the editing card.
- `delete-target-gone` *(planglyph)* — informational, at dispatch only once a batch has begun: a pending card's `Delete` target is already gone, in place of its `path-missing` or `glyph-not-found`.
- `uses-later-target` — a card's `Uses:` entry that a later card targets, matched as exact trimmed strings; one finding per consumer, producer and entry, on the consumer.

## Verify lines

Each reads a card's `**Verify:**` value, split at `&&`, `||`, `;`, `|` and newlines after joining line continuations, and never the overview's `## verify:`.
None sees a command behind a variable or `bash -c`.

- `verify-nested-module` — a `go` command whose relative package argument lies in a nested module other than the one the command runs in.
  Way forward: run it as `go -C <module>` with module-relative package paths.
- `verify-module-wide` — a raw Go test, build or vet command whose package argument holds `...` or is `all`, or a raw Go test whose `-tags` names `tmux`, `llm`, `tier2` or `tier3`; an agent's settings deny it.
  Way forward: `lym gate test --wait --tier <n> --card <id>`.
- `verify-package-missing` — a relative package argument naming no directory on disk or created by this or an earlier card; a card at or after a `Move` is skipped.
  Way forward: correct the line; a `_test`-suffixed argument names its directory's spelling.
- `verify-raw-command` *(loomshed)* — a command the hub's `gate.yaml` `refuse_raw` list names, past a leading `env`, `exec`, `xargs`, `sudo` or `time`.
  Way forward: one `lym gate test --wait` call with a tier list and a scope form.
- `verify-gate-no-wait` *(loomshed)* — a `lym gate test` call without `--wait`.
  Way forward: add `--wait`.
- `plan-verify-retired` *(loomshed)* — informational: the overview carries a `## verify:` section, which no gate runs.
- `card-fabric-reference` — a command that reaches the fabric repo: a sibling worktree path ending in the fabric suffix, or a spelling that drives the fabric repo.
  It scans `**Verify:**` values, the overview's `## verify:` and fenced blocks of every plan file for both, and inline code spans and card ids for the path.
  Way forward: take the data from committed test data an earlier card creates, or drop the command; for a card id, rename the card in the Card Index and its file name.

## PATTERN

`pattern-entry-line-cap` runs everywhere.
The other PATTERN checks are *loomshed*'s, and run only in a worktree with a `PATTERN.md` and a base overview, read at the task branch's merge base with its parent, or at `HEAD` with no parent.

- `pattern-entry-line-cap` — a fenced line opening ``- `PATTERN-`` whose rule text exceeds `pattern.MaxEntryTextChars` runes.
  Way forward: shorten the line or move detail to the entry's background file.
- `pattern-changes-mismatch` — tapestry runs only: an entry (ID, action and text) the decision record's `## PATTERN changes` and the overview's do not both declare.
  Way forward: copy the record's bullets into the overview, without the added-after-Discussion suffix.
  A malformed record section is an error naming `lym loom validate-discussion`.
- `pattern-removes-prescribed` — a pending card fences a line for a `removes` entry.
  Way forward: drop the line from the card.
- `pattern-headroom` — the overview projected from every card's fenced lines and the `removes` entries grows past the headroom line.
  Way forward: shorten the prescribed lines or move detail into background files; when that is not enough, record a `changes` decision that shortens other entries with `lym loom decision add`, with `--amends-plan` once the plan is approved.
- `pattern-card-headroom` — a pending card's own step grows the projected overview past the headroom line, the projection taking the cards in plan order, each applying its fenced lines and then dropping the `removes` entries it is first to name.
  Way forward: `pattern-headroom`'s, with the shortening on the growing card or before it.
- `pattern-line-unprescribed` — tapestry runs only: an `adds` or `changes` entry no card fences a line for, whose overview bullet still equals the base's.
  Way forward: prescribe the entry's whole overview line in a fenced block on a pending card that edits `PATTERN.md`, or on a card inserted with `lym loom insert-card`.

## Tests, docs and names

- `edit-test-file` — a `_test.go` file self glyph under `Edit` or `Extends`; a path holding a segment the go tool ignores is exempt, and so is a file `lym quarry toc` lists with no symbol, such as one holding only blank-identifier vars, since no member glyph can name the edit.
  Way forward: name each test function, fake or helper the card changes as a member glyph under `**Edit:**`, declare each new one as a `**Create:**` handle on the package directory or `<dir>_test`, keep a new test file a `**Create:**` path, and for a header-only change such as a build constraint, name every member of the file.
- `test-tier-unstated` *(loomshed)* — under `go`, a `**Create:**` `_test.go` file whose Intent lines name no valid tier.
  The tiers are the hub's `gate.yaml` tier keys, read as whole words on the Intent lines that name the file by path or base name; a line naming another test file counts toward that file too.
  It passes on exactly one tier, on several joined pairwise by `||` on one line, or on `untagged` alone for a support-only file.
  Way forward: an Intent line naming the file and its one tier, or `untagged`, on a line that names no other test file.
  It does not judge that the stated tier fits the file, nor see a test file created as a member handle.
- `seam-test-unlisted` *(loomshed)* — under `go`, a pending card touches a struct a `//lym:seam <unit>#<Type>` test marks, without covering that test.
  A card touches `u#T` through a `Create`, `Edit`, `Extends`, `Delete` or `Rename` ref of `u#T`, a member `u#T.<x>`, a file declaring `T`, or `u#`.
  It covers the test when its target groups name the test's glyph or that glyph's package counterpart (the same member in the other Go package of the directory, `<dir>` versus `<dir>_test`), its refs name the test's file or the package self glyph of the test's unit or of its counterpart unit, or a later card names the test or its counterpart under `Edit` or `Extends`.
  Way forward: list the test under `Edit` when the card changes it, under `Uses` when it stays as is and no later card targets it, leave it to the later card that edits it, else list it under `Edit`.
  It passes a test listed under `Uses` that the card breaks.
  An unparsable marker or test file halts the gate with the file and line, for the operator to correct.
- `doc-go-unlisted` *(loomshed)* — under `go`, a pending card changes a package's non-test Go code without covering its `doc.go`.
  A card changes a package through the refs of its `Create`, `Edit`, `Extends`, `Delete`, `Rename` and `Move` groups; a non-`.go` file and test code do not count, and a member named `Test…`, `Benchmark…`, `Fuzz…` or `Example…` that does not resolve yet is test code.
  A package with no `doc.go` on disk is skipped.
  The card covers the `doc.go` when any of its groups or its `Uses` names it, when a ref is the package self glyph, or when a later card names it under `Edit` or `Extends`.
  Way forward: list it under `Edit` when the card changes what it says, under `Extends` when it only adds to it, under `Uses` when it stays as is and no later card targets it, leave it to the later card that edits it, else list it under `Edit`.
  It passes a `doc.go` under `Extends` or `Uses` that the card falsifies, and counts a card that creates a test helper by a handle without a `Test`, `Benchmark`, `Fuzz` or `Example` prefix as non-test code.
- `fabric-vocabulary-name` — a name a `Create` handle or a `Rename` `New` handle declares that the fabric-vocabulary scan would refuse once it lands; the finding names `PATTERN-fabric-boundary` and the rule broken.
  Way forward: rename the member.

## Run state and parts

- `done-card-edited` *(loomshed)* — a card of a batch webster's run record holds done no longer hashes to what the batch recorded at begin.
  Way forward: restore the card file; a fix to its work is a follow-up card after the last begun batch, with its Card Index line.
- `part-spot-target` — under a part scope: a card's target is a shared spot's path.
  Way forward: drop the target and say in the card's `**Intent:**` what it needs from the spot.
- `part-export-missing` *(loomshed, `--part` only)* — an export of the part's contract that no card of the part creates.
  Way forward: create the export on a card of the part, a `plan:` export declared on a `**Create:**` bullet with the parts file's spelling; when the part's area cannot meet it, message the chair naming the contract line.

`loom-parts-spec.md` defines the part scope and the join.
