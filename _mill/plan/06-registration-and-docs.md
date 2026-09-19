# Batch: registration and docs

```yaml
task: "Worktree spawn/teardown as Shed producers"
batch: "registration and docs"
number: 6
cards: 6
verify: go test ./cmd/lyx/...
depends-on: [5]
```

## Batch Scope

This batch registers the new module under the cobra root and lands every documentation change the task owes in the same commit: the new invariant, the two amended invariant lines, the module map and execution-stack entries, the roadmap move, the design doc's deletion, and the sandbox scenario.

Registration is what makes four repo-wide guards in `cmd/lyx` fire at once — the help tree, the exists-implies-registered walk, the sandbox coverage guard, and the transient-path guard — so the registration card and the guard-satisfying cards belong together: splitting them would leave the tree red between commits.

Batch-local decision: the design doc is deleted rather than rewritten as as-built.
See the overview's `design-doc-is-deleted-not-rewritten` Shared Decision for why this departs from what the discussion recorded.

## Cards

### Card 29: register the module under the root

- **Context:**
  - `internal/lifecyclecli/cli.go`
  - `cmd/lyx/registration_test.go`
  - `cmd/lyx/sandbox_coverage_test.go`
- **Edits:**
  - `cmd/lyx/main.go`
  - `cmd/lyx/helptree_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add the `internal/lifecyclecli` import to `cmd/lyx/main.go` and `lifecyclecli.Command()` to the `root.AddCommand` list, placed alongside the other module commands rather than beside the run alias, which is a second registration of an existing subtree and not a module of its own.
  In `cmd/lyx/helptree_test.go` add `"lifecycle"` to `TestHelpTree_RootNamesAllModules`'s `requiredModules` slice, and add the module's two verbs to the per-module subcommand assertions in `TestHelpTree_VerbModuleSubcommands` in the shape that test already uses for every other module.
  Make no other change in either file: the exists-implies-registered guard and the sandbox coverage guard need no edit here — the first is satisfied by the `AddCommand` line, and the second is satisfied by card 33's scenario tag.
- **Commit:** `feat(lyx): register the lifecycle module under the cobra root`

### Card 30: the transient-path guard entries

- **Context:**
  - `internal/lifecyclecli/paths.go`
  - `internal/lyxdirs/dirs.go`
  - `internal/lyxcwd/lyxcwd.go`
- **Edits:**
  - `cmd/lyx/notransients_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add the five exported `internal/lifecyclecli` path constructors to this guard's walk over every module's exported path constructors, in the `.lyx` group, so both synthetic fixtures — the one whose anchor is the worktree root and the one anchored at a subdirectory — assert each resolves under `.lyx` at the mirrored subpath and moves down by the anchor.
  This is the Durable-vs-Ephemeral State Invariant's machine half for the lifecycle tree, and the guard is a hand-maintained table rather than a scan, so the entries do not appear on their own.
  Add the `internal/lifecyclecli` import this needs, which is legal here for the reason the file's own header already records: `cmd/lyx` is the one package that may import every owning module at once.
  Make no assertion here about which worktree the lifecycle tree is anchored to — that claim belongs to `internal/lifecyclecli`'s own path-derivation tests, which hold the Bookend proxy.
- **Commit:** `test(lyx): pin the lifecycle paths as ephemeral in the transient guard`

### Card 31: the new invariant and the two amended lines

- **Context:**
  - `internal/lifecycleshed/doc.go`
  - `internal/lifecyclecli/paths.go`
  - `internal/lifecyclecli/paths_test.go`
  - `internal/lifecycleshed/seam_enforcement_test.go`
- **Edits:**
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add a new `## Lifecycle Bookend Invariant` section stating: a producer that creates or destroys a task worktree never runs from inside that worktree; the lifecycle Shed is driven from the hub's prime worktree, and its status file and locks live under prime's own ephemeral tree, never under the worktree being managed; and a teardown row sequences session shutdown before worktree removal, in one producer, never two rows.
  Record its enforcement honestly as review discipline with two partial mechanical proxies, and say why there is no enforcing test: the invariant constrains which directory a running process is driven from, which has no static shape an AST scan can see.
  Name both proxies so a reviewer knows what is already covered — `internal/lifecycleshed`'s seam-enforcement scan bars a direct resolver import so the package cannot resolve its way into the managed worktree, and `internal/lifecyclecli`'s path-derivation tests pin the status and lock paths to prime's anchor so a relocation under the managed worktree fails there — and state that neither proves the driver's own working directory, which stays a review obligation.
  Place the new section in the file's existing ordering next to the fabric invariants it neighbours in subject matter.
  In `## Told-Geometry Invariant`, add `internal/lifecycleshed` and `internal/lifecyclerecipe` to the bound-packages list.
  In `## CLI / Cobra Invariant`, change the module-count line from eleven of twelve to twelve of thirteen.
- **Commit:** `docs(constraints): record the Lifecycle Bookend Invariant and amend two counts`

### Card 32: the overview's module map and execution stack

- **Context:**
  - `internal/lifecyclecli/cli.go`
  - `internal/lifecycleshed/doc.go`
  - `internal/lifecyclerecipe/doc.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add a `**lifecycle**` bullet to the `## Modules` list, in the shape the neighbouring bullets use: name the two packages behind it plus the producer package, state the CLI surface as the two verbs, state that it drives one task worktree's create, run and teardown as a single Shed run from the hub's prime worktree, state that teardown is one row sequencing session shutdown before worktree removal and never forces, and mark it implemented with a pointer at the owning packages' own documentation rather than at a design doc.
  In `## Execution stack (orchestration layers)`, add one short paragraph after the stack diagram stating that the lifecycle Shed nests loom's: it is its own three-row recipe whose middle row drives a task's loom run as a child process and polls that run's own persisted status for the verdict, so there are two status files by design — the task's, committed on the task branch, and the lifecycle's, per-machine under prime's ephemeral tree — each resuming independently.
  Do not restate the invariant text from `CONSTRAINTS.md` here; link to it the way the surrounding paragraphs already link to the Told-Geometry Invariant.
  Every inline link added must resolve, file part and anchor, per the Markdown Link Integrity invariant.
- **Commit:** `docs(overview): add lifecycle to the module map and the execution stack`

### Card 33: the roadmap move and the design doc's deletion

- **Context:**
  - `docs/overview.md`
  - `internal/lifecyclecli/cli.go`
  - `internal/shedrecipe/registry.go`
- **Edits:**
  - `manifest/roadmap.md`
- **Creates:** none
- **Deletes:**
  - `manifest/designs/worktree-lifecycle-shed-producers.md`
- **Moves:** none
- **Requirements:** Move the Planned entry named `worktree spawn/teardown as Shed producers` into the `## Done` section, rewritten to a name plus one or two sentences of what shipped, per that file's own Maintenance rules, and pointing at the owning packages' documentation instead of a design doc.
  Drop its `See [designs/...]` link line entirely rather than repointing it: the design doc is deleted in this same card, and leaving a link to a removed file breaks the Markdown Link Integrity invariant.
  No renumbering is needed anywhere, since every entry is written literally as `1.` and each section renders its own sequence.
  In the entry's new text, correct what the old one got wrong rather than carrying it forward: the self-heal item it recorded as a hard dependency is not one, because the driven path already ensures the session before any producer that spawns into it runs, and that item stays Planned and untouched.
  Delete `manifest/designs/worktree-lifecycle-shed-producers.md`, per the Documentation Lifecycle: a module-design doc is deleted when its module lands, and the implementation, its tests and the packages' own documentation become the source of truth.
  Separately, in the Someday entry named `shedrecipe: capability-declaration instead of manual seam-threading`, change its trailing registry-entry count from fourteen to seventeen so that description does not go stale against the table this task grew.
  Leave the Next Up entry that references this item by bold name alone: it references by name, not by link, so it still resolves.
- **Commit:** `docs(roadmap): move the lifecycle item to Done and delete its design doc`

### Card 34: the sandbox scenario

- **Context:**
  - `tools/sandbox/SANDBOX-CORE-SUITE.md`
  - `cmd/lyx/sandbox_coverage_test.go`
  - `internal/lifecyclecli/cli.go`
  - `internal/lifecyclecli/run.go`
  - `internal/lifecyclecli/status.go`
  - `internal/lifecyclecli/paths.go`
- **Edits:**
  - `tools/sandbox/SANDBOX-FABRIC-SUITE.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add a new scenario after the suite's current last one, numbered one higher, carrying a `**Covers:** lifecycle` tag — the exact token the coverage guard matches against the registered module name.
  The fabric suite is the right home because the scenario's observable effects are worktree-pair and refusal behaviour, which that suite already creates and asserts on in a disposable hub.
  Carry a `**Fixture note:**` in the shape the core suite's own hand-written-status-file scenario uses, naming the same cause: the scenario deliberately never completes the middle row, because a freshly created pair has no task status file and the bootstrap spawns a real driver into the LLM rows whenever the run lock is free, and the lifecycle module exposes no step or pause verb to interpose a fixture mid-run.
  What the scenario covers instead is everything observable without an LLM: the status verb round-tripping a hand-written lifecycle status fixture; the run verb refusing on a `done` status and naming the per-slug directory to delete; the run verb refusing on a held per-slug run lock and naming the lock path; the run verb and the status verb each refusing when invoked from a task worktree rather than the hub's prime; and the create row halting blocked against a deliberately dirty prime.
  Say in the scenario that the full driven path is covered by the `integration`-tagged end-to-end instead, so a sandbox operator does not read the gap as an oversight.
  Add the matching verdict line to the suite's report template at the end of the file, in the same shape as the lines already there.
  Make no change under `sandbox/posix/` or `sandbox/win/`: those launchers are one-line per-suite entry points that carry no per-scenario steps, and the suite document itself is embedded by the sandbox tool, so a new scenario in an existing suite needs no runner edit.
- **Commit:** `docs(sandbox): add the lifecycle scenario to the fabric suite`

## Batch Tests

`verify:` runs `go test ./cmd/lyx/...`, which is where every guard this batch must satisfy lives: the help-tree test, the exists-implies-registered walk, the sandbox coverage guard that parses the suite documents for `**Covers:**` tags, the transient-path guard, and the long-list and unknown-subcommand guards that the new subtree extends.
Four of those fire only once the module is registered, which is why the registration card and the doc cards share this batch and this one verify command.
Untagged only: every guard named above is either a pure AST walk, a document parse, or `filepath.Join` arithmetic over a synthetic location, so none spawns a process and the run stays inside Test Tier Purity.
The documentation-only cards carry no runnable surface of their own beyond the coverage guard's document parse, which this same command covers.
