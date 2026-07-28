# Batch: pattern-package

```yaml
task: 'PATTERN wiring: conditional constraint-injection into every agent'
batch: pattern-package
number: 6
cards: 3
verify: go test ./internal/pattern/... ./internal/hubgeometry/... ./cmd/lyx/...
depends-on: [5]
```

## Batch Scope

This batch builds `internal/pattern`, the leaf that answers one question — is PATTERN active in this worktree, and what should the agent be told — and returns the directive text. It is a new package rather than an addition to an existing one because four feature packages import it (`builderengine`, `websterengine`, `burlerengine`, `loomengine`), and only a leaf can be imported that widely without cycle risk; the repo already has three precedents with exactly this shape in `internal/modelspec`, `internal/tokenvocab` and `internal/codeintelengine`. It is not in `internal/stencil`, which is a pure text leaf whose lack of any filesystem or geometry dependency is what makes it trivially testable, and not in `internal/fabricengine`, which owns the junction primitive but would be a heavy, wrong-direction dependency for four feature packages.

The external interface batch 7 consumes is `pattern.Directive(l *hubgeometry.Layout, role Role) string` plus the three `Role` constants. Every test in this batch is untagged Tier 1 — the package is `os.Stat` over `t.TempDir()` and spawns nothing.

## Cards

### Card 21: build `internal/pattern` with the active check and three role directives

- **Context:**
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/websterengine/master-template.md`
  - `internal/burlerengine/review-prompt-template.md`
- **Edits:** none
- **Creates:**
  - `internal/pattern/doc.go`
  - `internal/pattern/pattern.go`
  - `internal/pattern/pattern_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/pattern` importing only the standard library and `internal/hubgeometry`. Define an exported `Role` type with three constants — `RoleImplementer`, `RoleReviewFix`, `RoleOrchestrator` — and `Directive(l *hubgeometry.Layout, role Role) string`, returning the empty string when PATTERN is inactive and the role's directive block when active. **The active check:** PATTERN is active **iff `_pattern/PATTERN.md` exists**, resolved via `l.PatternFileHere()` and nothing else — the package must never construct the path itself, which the Hub Geometry Invariant's enforcement test makes impossible anyway from batch 2 onward. Pure existence: the `_pattern/` directory may exist without the file, which is the normal inactive state since `Init` always creates the directory. Three edge rules must be implemented deliberately and each is pinned by a test. A `PATTERN.md` that exists but is **empty** counts as **active** — degenerate but harmless, and preferable to a content-inspecting check that would turn a benign empty file into a runtime error in five agents. A `PATTERN.md` present as a **directory** rather than a regular file counts as **inactive**: it is not a readable index. And a stat error that is **not** `os.IsNotExist` — a permission or I/O failure — is treated as **active, not inactive**: `Directive` keeps its plain `string` return, so it resolves the ambiguity by failing loud in the prompt rather than silently, returning the directive anyway so the agent reads the file itself and reports a real, visible failure if it genuinely cannot. The rule is: absent file means inactive, every other outcome means active. A nil `*hubgeometry.Layout` returns `""` without panicking — several `Deps` structs are assembled field-by-field by CLI callers that could leave `Layout` unset, and a nil dereference inside prompt assembly would take down all five agent paths for a slip unrelated to PATTERN. An unknown or zero `Role` must have a defined, documented behaviour rather than falling through a `switch`. **The three directive blocks** are literal constants, each carrying its own `##` heading *inside* the value so that an inactive render leaves no orphan heading behind, each an imperative `- [ ]` checklist rather than a single sentence, and each containing the literal relative pointer `_pattern/PATTERN.md` — never an interpolated absolute path, which would make the value vary per worktree and defeat the fixed-string tests. The directive injects a pointer and never the constraints inline, so prompt size stays constant however large PATTERN grows. `RoleImplementer`, used by builder implementer, webster fork and loom plan, is headed `## Constraints — do this before you write any code` and its bullets are: a bolded **STOP** telling the agent to read `_pattern/PATTERN.md` in full before editing a single file; read every detail doc under `_pattern/` that PATTERN.md points to and that touches what is about to change; these constraints are BINDING, and a change that violates one is wrong even if the verify command passes; if a constraint conflicts with anything else in the prompt the constraint wins, and say so in the report instead of silently picking one. `RoleReviewFix`, used by the burler round, is headed `## Constraints — do this before you judge or change anything` and covers both of that round's phases in the round's own order — read PATTERN.md in full before forming any judgment; read the relevant detail docs; **in part A** every violation of a listed constraint is a BLOCKING finding, recorded no matter how small it looks and never waved through because the code works or the tests pass; **in part B** the fix must not introduce a violation, since a fix that trades one finding for a constraint breach is not a fix; and the same conflict-resolution bullet. It is one combined variant rather than a reviewer variant plus an implementer variant because the burler round is the only reviewing template in the set and it also fixes — its own prompt says it has two jobs, in order, in one session — so a pure reviewer role would have no user. `RoleOrchestrator`, used by webster Master, is headed `## Constraints — do this before you fork anything` and is worded for forking rather than editing, because Master's own prompt says outright that it never edits code, which would make an implementer-worded instruction one it cannot carry out: read PATTERN.md in full before forking a single implementer; read the relevant detail docs; every fork inherits its context, so reading this once here is what puts the constraints in front of all of them, and it must not be skipped on the grounds of not editing code; and the constraints are BINDING on the forks it spawns, so a batch report trading a constraint for a passing verify is a failed batch, not a success. Create `internal/pattern/pattern_test.go`, untagged, using `t.TempDir()`, covering: PATTERN.md present gives a non-empty directive for each of the three roles; the directory present without the file gives `""`; neither present gives `""`; an empty PATTERN.md is active; PATTERN.md as a directory is inactive; a nil Layout returns `""` without panicking; the zero/unknown `Role` behaves as documented; the three variants are pairwise different and each contains the literal `_pattern/PATTERN.md`; and each variant begins with its own `##` heading. The **`RelPath != "."` case is the regression guard for the worst failure mode in this task** and must be present: a Layout whose `RelPath` is a nested subdirectory must find `PATTERN.md` at `<WorktreeRoot>/<RelPath>/_pattern/PATTERN.md` and must **not** find one planted at `<WorktreeRoot>/_pattern/PATTERN.md` — without it, a root-anchored resolution renders PATTERN silently inactive in all five agents with no error anywhere. Cover the non-`IsNotExist` stat error by whatever mechanism is portable (an unreadable parent directory on POSIX, or an unexported seam if no portable filesystem trick works on Windows); if neither is portable, state that in a test comment rather than dropping the case silently. `internal/pattern/doc.go` carries the package documentation: what the active check is, why it is pure existence, why the three roles are what they are, and why the pointer stays relative.
- **Commit:** `pattern: add the active check and the three role directives`

### Card 22: enforce the Pattern Leaf Invariant

- **Context:**
  - `internal/modelspec/leaf_enforcement_test.go`
  - `internal/pattern/pattern.go`
- **Edits:** none
- **Creates:**
  - `internal/pattern/leaf_enforcement_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/pattern/leaf_enforcement_test.go` with `TestLeafInvariant_AllowlistOnly`, modelled directly on `internal/modelspec/leaf_enforcement_test.go`: locate the package directory via `runtime.Caller(0)`, walk every non-`_test.go` `.go` file, parse each with `go/parser` in `ImportsOnly` mode so only real import declarations are inspected and never a string in a doc comment, and fail on any import that is neither stdlib (no `.` in the first path segment) nor in an `allowedImports` allowlist. The allowlist holds exactly one entry: `github.com/Knatte18/loomyard/internal/hubgeometry`. An allowlist rather than a denylist is the point — a future stray dependency is caught with no list maintenance. The file is untagged: it spawns nothing and copies nothing.
- **Commit:** `pattern: enforce the leaf import allowlist`

### Card 23: record the Pattern Leaf Invariant and the new module

- **Context:**
  - `internal/pattern/doc.go`
  - `internal/pattern/leaf_enforcement_test.go`
- **Edits:**
  - `CONSTRAINTS.md`
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add a **Pattern Leaf Invariant** section to `CONSTRAINTS.md`, placed with the other leaf invariants and following their established shape: a one-line statement that `internal/pattern` production code imports only the standard library and `internal/hubgeometry`, that the reverse import — `pattern` to any feature package — is never allowed, and an **Enforced by** line naming `internal/pattern/leaf_enforcement_test.go` (`TestLeafInvariant_AllowlistOnly`) as running on every `go test`. Model the wording on the neighbouring Modelspec, Tokenvocab and Codeintelengine leaf invariants. Then update `docs/overview.md` so `internal/pattern` appears where the other shared leaves do — the source-tree listing that already names `internal/modelspec` and `internal/tokenvocab`, and the prose sentence listing the thin shared-infrastructure layer beneath the user-facing modules. Describe it in one line as the leaf that computes whether `_pattern/PATTERN.md` is present and returns the role-appropriate constraints directive injected into every code-touching agent prompt. This is the Documentation Lifecycle obligation for adding a module, and it lands in the same commit as the module. Write every paragraph and list item as one unwrapped line.
- **Commit:** `docs: record the Pattern Leaf Invariant and the internal/pattern module`

## Batch Tests

`verify: go test ./internal/pattern/... ./internal/hubgeometry/... ./cmd/lyx/...` needs no `-tags integration`: this batch creates no integration-tagged file, and keeping the entire `internal/pattern` suite in Tier 1 is a deliberate design property rather than an accident — the package is a leaf over `os.Stat` and `t.TempDir()`, so it spawns nothing, copies no fixture tree, and needs no `TestMain` calling `lyxtest.HermeticGitEnv()`. `internal/hubgeometry` is in scope because card 21 is the first real consumer of `PatternFileHere()` and would surface any mistake in that accessor's `WorktreeRoot`+`RelPath` anchoring. `./cmd/lyx/...` is in scope for two guards this batch could trip: `tierpurity_test.go`, which fails on a raw-substring match if either new untagged test file so much as mentions `exec.Command` or `lyxtest.Copy` in a comment, and `hermeticenv_test.go`, which would demand a `TestMain` the moment a file in the new package contained a git-spawning token. Both are satisfied by construction here, and running them is what proves it.
