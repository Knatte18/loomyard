# Batch: code-rename

```yaml
task: Rename internal/ghissues → selfreport
batch: code-rename
number: 1
cards: 4
verify: go build ./... && go test ./...
depends-on: []
```

## Rename mechanic

This batch renames two packages. For each `Moves:` pair the implementer MUST:

1. Run `git mv <old> <new>` FIRST, before making any other change to the moved file.
   The engine and cli live in their own directories, so the `git mv` of the single file in
   each directory effectively renames the directory (git tracks files, not dirs).
2. Make ONLY surgical edits — touch only the lines that must change after the move:
   the `package` declaration, `import` lines, the package-qualifier prefix on cross-package
   identifiers (`ghissuesengine.X` → `selfreportengine.X`), `Short`/`Long`/`Use` strings,
   and comments/identifiers naming the old module. Do NOT rewrite the file from scratch.
3. Use a full-file `Creates:` entry only for genuinely new files — there are none here.
4. Never write the relocated file from scratch and delete the original — that breaks git
   rename history and inflates the review diff.

## Batch Scope

This batch performs the behaviour-preserving Go-source rename of the `ghissues` module to
`selfreport` and updates every consumer and pinned guard so the build and full test suite
stay green. It moves `internal/ghissuesengine/ghissues.go` → `internal/selfreportengine/selfreport.go`
and `internal/ghissuescli/{cli.go,cli_test.go}` → `internal/selfreportcli/`, renames the
CLI verb to `selfreport`, reframes the help prose, and retargets the three pinned guard
sites (`cmd/lyx`, `internal/lyxtest`). The exported engine API (`RunGH`, `CreateIssue`,
`realRunGH` seam behaviour) keeps identical signatures — only package/identifier names and
help strings change. The next batch (docs) consumes nothing from this batch at the code
level; it depends on it only for ordering so the docs describe the already-renamed command.

Batch-local decision: the engine and cli packages must be renamed together with all their
consumers in this single batch because `go build ./...` only compiles when every reference
is updated atomically — an intermediate state with a half-renamed package does not build.

## Cards

### Card 1: Rename ghissuesengine → selfreportengine

- **Context:**
  - `CONSTRAINTS.md`
- **Edits:** none
- **Creates:** none
- **Deletes:** none
- **Moves:**
  - `internal/ghissuesengine/ghissues.go` -> `internal/selfreportengine/selfreport.go`
- **Requirements:** After `git mv`, change the `package ghissuesengine` declaration to
  `package selfreportengine`. Update the two leading file/package doc comments (the
  `ghissues.go contains …` header comment and the `Package ghissuesengine provides …`
  doc comment) to name `selfreport.go` and `selfreportengine` respectively, and reword
  any phrasing that calls this "the original ghissues package" to refer to the
  selfreport module. Do NOT rename any exported or unexported identifier
  (`targetRepo`, `RunGH`, `realRunGH`, `buildCreateArgs`, `CreateIssue`,
  `lastNonEmptyLine`) and do NOT change `targetRepo = "Knatte18/loomyard"` or any logic.
  The package must keep importing only `internal/proc` (no cli/cobra import — engine
  purity per the CLI/Cobra Invariant).
- **Commit:** `refactor(selfreport): rename ghissuesengine package to selfreportengine`

### Card 2: Rename ghissuescli → selfreportcli and reframe help

- **Context:**
  - `_mill/discussion.md`
  - `CONSTRAINTS.md`
- **Edits:** none
- **Creates:** none
- **Deletes:** none
- **Moves:**
  - `internal/ghissuescli/cli.go` -> `internal/selfreportcli/cli.go`
  - `internal/ghissuescli/cli_test.go` -> `internal/selfreportcli/cli_test.go`
- **Requirements:** After `git mv` of both files:
  - In `cli.go`: change `package ghissuescli` → `package selfreportcli`; change the import
    `github.com/Knatte18/loomyard/internal/ghissuesengine` →
    `.../internal/selfreportengine` and retarget the two call sites
    `ghissuesengine.CreateIssue` → `selfreportengine.CreateIssue` (in `runCreate`). Change
    the parent command `Use: "ghissues"` → `Use: "selfreport"`. Reframe the parent
    `Short` to lead with the self-report responsibility while still naming gh/GitHub
    (e.g. "self-report a LoomYard bug or enhancement to lyx's own repo via gh"). Reframe
    the `create` subcommand `Short` similarly (e.g. "file a self-report issue on the
    LoomYard repository via gh"). In the `create` `Long`, keep the gh-prerequisite note
    and the three worked examples but change every `lyx ghissues create …` → `lyx
    selfreport create …`. Update the package doc comment and the `Command`/`RunCLI`/
    `runCreate` doc comments that say "ghissues"/"ghissuesengine"/"the ghissues module"
    to the selfreport equivalents. Keep both `Short` values non-empty (drift guard) and
    keep the `RunCLI` seam exactly `return clihelp.Execute(Command(), out, args)`.
  - In `cli_test.go`: change `package ghissuescli` → `package selfreportcli`; change the
    import `.../internal/ghissuesengine` → `.../internal/selfreportengine` and retarget
    every `ghissuesengine.RunGH` reference (in `installFakeGH` and its `t.Cleanup`) to
    `selfreportengine.RunGH`. Update the file header comment and the `installFakeGH`
    doc comment that name "the ghissues CLI"/"package ghissuescli"/"ghissuesengine.RunGH
    seam" to the selfreport equivalents. The `TestRunCreate_*` function names contain no
    `Ghissues` token and stay as-is.
- **Commit:** `refactor(selfreport): rename ghissuescli package and reframe help prose`

### Card 3: Update cmd/lyx registration and pinned help-tree guards

- **Context:**
  - `CONSTRAINTS.md`
- **Edits:**
  - `cmd/lyx/main.go`
  - `cmd/lyx/helptree_test.go`
  - `cmd/lyx/jsonhelp_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - `cmd/lyx/main.go`: change the import `github.com/Knatte18/loomyard/internal/ghissuescli`
    → `.../internal/selfreportcli`; change the `newRoot()` registration
    `ghissuescli.Command()` → `selfreportcli.Command()`; in the root command `Long`,
    change the trailing module-list token `ghissues.` → `selfreport.` on the
    `Available modules: …` line.
  - `cmd/lyx/helptree_test.go`: in the `requiredModules` set, change `"ghissues"` →
    `"selfreport"`; in the module table row, change the `name:` and `module:` fields from
    `"ghissues"` to `"selfreport"` (its `wantSubs` already lists `create`, unchanged).
  - `cmd/lyx/jsonhelp_test.go`: in the pinned module-names list change `"ghissues"` →
    `"selfreport"`; rename the test functions `TestJSONHelp_GhissuesSchema` →
    `TestJSONHelp_SelfreportSchema` and `TestJSONHelp_GhissuesCreateLeaf` →
    `TestJSONHelp_SelfreportCreateLeaf`; change the argv literals `{"ghissues", "--json"}`
    → `{"selfreport", "--json"}` and `{"ghissues", "create", "--help", "--json"}` →
    `{"selfreport", "create", "--help", "--json"}`; and update every comment / error-message
    string in those two functions that says "ghissues" to "selfreport".
  - Per the CLI/Cobra Invariant, registration (import + AddCommand + root `Long` token)
    must all change together so `registration_test.go` and `longlist_test.go` stay green.
- **Commit:** `refactor(selfreport): retarget cmd/lyx registration and help-tree guards`

### Card 4: Update lyxtest leaf-enforcement banned imports

- **Context:**
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/lyxtest/leaf_enforcement_test.go`
  - `internal/lyxtest/doc.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - `internal/lyxtest/leaf_enforcement_test.go`: in the banned-import string list change
    `"github.com/Knatte18/loomyard/internal/ghissuesengine"` →
    `".../internal/selfreportengine"` and
    `"github.com/Knatte18/loomyard/internal/ghissuescli"` → `".../internal/selfreportcli"`;
    update the two header comments listing the engine/cli pairs so
    `ghissuesengine/ghissuescli` reads `selfreportengine/selfreportcli`.
  - `internal/lyxtest/doc.go`: in the package doc comment listing the engine/cli pairs,
    change `ghissuesengine/ghissuescli` → `selfreportengine/selfreportcli`.
  - The banned-import entries must exactly match the new import paths from Cards 1–2 so the
    lyxtest Leaf Invariant guard keeps catching a reintroduced edge.
- **Commit:** `refactor(selfreport): update lyxtest banned-import entries`

## Batch Tests

`verify: go build ./... && go test ./...` (full build + full suite). This wide scope is
justified, not a default: the rename retargets package import paths, the cobra
registration, and four pinned guard suites (`registration_test.go`, `longlist_test.go`,
`helptree_test.go`, `jsonhelp_test.go` in `cmd/lyx`; `drift_test.go` via the reframed
`Short`s; `leaf_enforcement_test.go` in `internal/lyxtest`), and any missed reference
anywhere in the tree breaks the build. `go build ./...` cheaply catches every missed
import/identifier; `go test ./...` runs the renamed `internal/selfreportcli` white-box
tests (`TestRunCreate_*`) and all the pinned guards. Go's full build+test for this repo is
fast, and the discussion's guardrail is explicitly "go build ./... and go test ./... pass
after the rename". No new tests are added — the change is mechanical and behaviour-preserving.
