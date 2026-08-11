# Plan: fabric: accumulate the result envelope from mutations, not control flow (slice 14)

```yaml
task: 'fabric: accumulate the result envelope from mutations, not control flow (slice 14)'
slug: 'fabric-mutation-record-envelope'
approved: false
started: '20260811-150313'
parent: 'main'
root: ""
verify: go build ./...
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: mutation-vocabulary
    file: 01-mutation-vocabulary.md
    depends-on: []
    verify: go test ./internal/fabricengine/ ./internal/output/
  - number: 2
    name: exported-check-enum
    file: 02-exported-check-enum.md
    depends-on: []
    verify: go test ./internal/fabricengine/
  - number: 3
    name: result-types-carry-record
    file: 03-result-types-carry-record.md
    depends-on: [1, 2]
    verify: go test ./internal/fabricengine/ ./internal/fabriccli/ ./internal/boardengine/ && go vet -tags integration ./internal/fabricengine/...
  - number: 4
    name: gate-auto-recording
    file: 04-gate-auto-recording.md
    depends-on: [2, 3]
    verify: go test ./internal/fabricengine/ ./internal/fabriccli/ && go vet -tags integration ./...
  - number: 5
    name: constructive-recording
    file: 05-constructive-recording.md
    depends-on: [4]
    verify: go test ./internal/fabricengine/ ./internal/fabriccli/ && go test -tags integration -run TestMutationRecord ./internal/fabricengine/
  - number: 6
    name: cli-envelope
    file: 06-cli-envelope.md
    depends-on: [5]
    verify: go test ./internal/fabriccli/ ./internal/output/ && go test -tags integration ./internal/fabriccli/
  - number: 7
    name: fabrictest-truthfulness-oracle
    file: 07-fabrictest-truthfulness-oracle.md
    depends-on: [6]
    verify: go test -tags integration ./internal/fabricengine/fabrictest/
  - number: 8
    name: guard-and-docs
    file: 08-guard-and-docs.md
    depends-on: [7]
    verify: go test ./cmd/lyx/ ./internal/lyxcwd/
```

## Shared Decisions

### Decision: the recorder is a `*Mutations` owned by the verb, populated by `defer`

- **Decision:** every mutating entry point declares **named results** and, immediately after constructing its recorder, installs `defer func() { res.Mutations = rec.Snapshot() }()`.
  The record therefore reaches the caller through *every* return statement — including the existing `return XResult{}, err` zero-result sites — without editing each one.
  Do **not** rewrite the existing `return XResult{}, err` sites into `return res, err`;
  the defer is what makes a newly-added early return unable to drop the record, and rewriting each site by hand reintroduces exactly the per-site obligation this slice exists to remove.
- **Rationale:** `_mill/discussion.md`'s `record-survives-the-error-return` Decision names the named-result-plus-`defer` idiom as the mechanism and states the goal as "a newly added early return cannot silently drop the record". A `defer` that assigns onto the named result satisfies both halves at once, and `internal/fabricengine/add.go` alone carries 28 zero-result returns that would otherwise each need a hand edit.
- **Applies to:** all batches.

### Decision: `Target` conversion happens in `Mutations.Append`, never at a call site

- **Decision:** `Mutations` carries the hub root as an unexported field set once at construction. `Append` converts an absolute path to a hub-relative, `filepath.ToSlash`'d `Target`, falling back to an absolute slashed path when the argument does not descend from the hub root. `AppendRef` records a bare git ref name with no path arithmetic at all.
  No caller — gate executor, verb, or CLI handler — performs the conversion itself.
- **Rationale:** `_mill/discussion.md`'s `mutation-entry-shape` Decision: `pathRequest` carries `container`, never the hub root, so the eight gate sites cannot convert;
  one conversion site also keeps hub geometry out of `destroy.go`.
- **Applies to:** all batches.

### Decision: record only after the primitive observably changed state

- **Decision:** an executor appends **after** its primitive succeeded and actually changed something. `removePath`'s already-absent early return records nothing;
  `removeGitWorktree` and `deleteBranch` record only when `err == nil` **and** `exitCode == 0`;
  the two minters record only after the create succeeded.
- **Rationale:** a record containing no-ops would fail the truthfulness cross-check's commission direction on correct behaviour, and would commit the mirror image of the campaign defect — claiming a destruction that never happened.
- **Applies to:** batch 4, batch 5.

### Decision: record at the covering root, and derive the inventory rather than enumerating it

- **Decision:** a constructive record entry is appended at the **coarsest root that covers what the operation created**, not once per file written. `writeLaunchers` records one entry for the launcher directory it minted, not three for the scripts inside it;
  `cloneRepo` records one `worktree_created` for the clone root, not one per cloned file.
  The oracle's coverage rule (batch 7 card 29) already resolves a diff change against a record entry by segment-wise subtree containment, so one root entry accounts for every path beneath it.
- **The inventory is derived, never hand-enumerated.** Before finishing batch 5, the implementer greps `internal/fabricengine` and `internal/fabriccli` for `os.WriteFile(`, `os.MkdirAll(`, `os.Mkdir(`, `os.Rename(`, `fslink.CreateDirLink(` and `cloneRepo(`, and classifies **every** production hit into exactly one of three buckets, writing the classification into the batch's completion note:
  1. **Recorded here** — the write is the coarsest root of something new, so it gets an entry.
  2. **Covered by an enclosing recorded root** — e.g. the weft lock directory `.weft/`, which lives under a weft worktree already recorded as `worktree_created`.
  3. **Invisible to the manifest** — the write lands under the `.git` metadata directory, which `CaptureManifest` excludes wholesale, so no entry is needed and none should be added.
- **Implicitly created and pruned ancestor directories are covered by the entry they exist for, not by entries of their own.** Fabric mints and reaps the *ancestors* of the roots it records: `fslink.CreateDirLink` `MkdirAll`s `filepath.Dir(link)`, so creating a portal materialises the `_portals/<anchor>/` chain;
  `writeLaunchers` `MkdirAll`s `_launchers/<anchor>/<slug>`;
  and `removePortal`/`removeLaunchers` both call `pruneEmptyAncestors` on the way out.
  Recording each of those would mean an entry per directory level, which says nothing the leaf entry does not already say.
  Batch 7 card 29's coverage rule therefore extends **upward** as well as downward — see that card for the exact form.
  The harness already concedes this shape: it declares `filepath.Dir(portal)` and `filepath.Dir(launcher)` as permitted roots today for exactly these implicit ancestors.
- **Rationale:** batch 7's omission direction requires every unfiltered-diff change to be covered by *some* record entry, so a hand-written list that misses one hub-visible write fails a cell on correct behaviour. Round 2's review found exactly that: `writeLaunchers` and six separate `CloneHub` construction sites were missing from a list that read as complete. A derivation the implementer runs against the source is the only form of this inventory that can be checked;
  a list in a plan file cannot.
- **Applies to:** batch 5, batch 6.

### Decision: `mutation.go` is the single declarer of the `Kind` enum

- **Decision:** every `Kind` constant is declared in `internal/fabricengine/mutation.go` and nowhere else.
  No other file writes a kind string literal.
  A new member lands in the same commit as its recording site and its `cmd/lyx/destructiveguard_test.go` guard entry.
- **Rationale:** `_mill/discussion.md`'s `mutation-entry-shape` Decision;
  the guard test can only pin an enumerated set, and a second declaration is what lets the two drift.
- **Applies to:** all batches.

### Decision: `ok` is unchanged; `mutations` and `partial` are always present

- **Decision:** `ok` keeps meaning "no error was returned". Every envelope emitted from a **verb outcome** always carries `mutations` (an array, empty rather than `null`) and `partial` (a bool, `false` rather than absent), on success and failure alike. `partial` is true from exactly one rule: `error ≠ nil ∧ record non-empty`. The four read-only verbs (`list`, `pairs`, `status`, `diff`) carry neither key.
- **The pre-flight failure class is explicitly outside the fixed key set.** A handler that fails *before* calling its verb — cwd/location resolution, `LoadConfig`, an argument `usage: …` error — keeps emitting a bare `output.Err` with neither key. Nothing has been mutated at those points and there is no result to read a record from, so an invented empty record would put `mutations` on an envelope that is not a verb outcome at all. Every statement of the fixed-key-set rule — card 24's doc comment and card 35's `CONSTRAINTS.md` wording — must carry this carve-out in the same breath, or the invariant is machine-quotable and false.
- **Rationale:** `_mill/discussion.md`'s `ok-semantics-and-error-path-fields` Decision. A fixed key set means a consumer never distinguishes absent from false, and redefining `ok` in place would silently change a field every existing consumer already reads.
- **Applies to:** batch 6.

### Decision: Fabric Vocabulary Invariant applies to every new identifier and string

- **Decision:** `Kind` enum values, JSON keys, doc comments, and CLI help text added by this task use fabric/warp/weft vocabulary only.
  The word `host` (and the policed geometry identifiers) must not appear in any new production `.go` file under `internal/` or `cmd/`, nor in any `internal/**/*.md`.
- **Rationale:** `CONSTRAINTS.md`'s Fabric Vocabulary Invariant, enforced by `internal/lyxcwd/enforcement_test.go`'s `TestEnforcement_FabricVocabulary`, which the batch-8 verify runs.
- **Applies to:** all batches.

### Decision: Go, not Python — verify commands carry no `PYTHONPATH=` prefix

- **Decision:** this is a Go repository;
  every `verify:` is a native `go test` invocation with no `PYTHONPATH=` prefix.
  Batch 7's verify carries `-tags integration`, since `internal/fabricengine/fabrictest` is entirely behind that build tag.
- **Rationale:** mill-plan's own "Verify command shape" rule scopes the `PYTHONPATH=` prefix to Python/mill projects and directs non-Python projects to the native runner.
- **Applies to:** all batches.

### Decision: Test Tier Purity — new harness work stays behind the `integration` tag

- **Decision:** no untagged `_test.go` file added or edited by this task may contain `gitexec.RunGit`, `exec.Command`, or `lyxtest.Copy*` — not even inside a comment, since the guard is a raw substring match.
  Batch 1's, 2's, and 6's new tests are pure-data/table tests with no spawn;
  every spawning assertion belongs in an `integration`-tagged file.
- **Rationale:** `CONSTRAINTS.md`'s Test Tier Purity Invariant, enforced by `cmd/lyx/tierpurity_test.go`.
- **Applies to:** all batches.

## All Files Touched

- `CONSTRAINTS.md`
- `cmd/lyx/destructiveguard_test.go`
- `docs/overview.md`
- `internal/fabriccli/cli_test.go`
- `internal/fabriccli/clone.go`
- `internal/fabriccli/envelope.go`
- `internal/fabriccli/envelope_test.go`
- `internal/fabriccli/fabric.go`
- `internal/fabriccli/unwire.go`
- `internal/fabriccli/weft_verbs.go`
- `internal/fabricengine/add.go`
- `internal/fabricengine/checkout.go`
- `internal/fabricengine/cleanup.go`
- `internal/fabricengine/clone.go`
- `internal/fabricengine/clone_reset_guard_test.go`
- `internal/fabricengine/coalesce.go`
- `internal/fabricengine/coalesce_integration_test.go`
- `internal/fabricengine/commit.go`
- `internal/fabricengine/destroy.go`
- `internal/fabricengine/destroy_test.go`
- `internal/fabricengine/doc.go`
- `internal/fabricengine/export_test.go`
- `internal/fabricengine/fabrictest/doc.go`
- `internal/fabricengine/fabrictest/matrix_test.go`
- `internal/fabricengine/fabrictest/mutationoracle.go`
- `internal/fabricengine/fabrictest/mutationoracle_test.go`
- `internal/fabricengine/fabrictest/refusal.go`
- `internal/fabricengine/fabrictest/refusal_test.go`
- `internal/fabricengine/fabrictest/verbs.go`
- `internal/fabricengine/fabrictest/verbs_test.go`
- `internal/fabricengine/gitexclude.go`
- `internal/fabricengine/gitexclude_integration_test.go`
- `internal/fabricengine/junction.go`
- `internal/fabricengine/junction_test.go`
- `internal/fabricengine/launchers.go`
- `internal/fabricengine/mutation.go`
- `internal/fabricengine/mutation_record_integration_test.go`
- `internal/fabricengine/mutation_test.go`
- `internal/fabricengine/portallauncher_test.go`
- `internal/fabricengine/portals.go`
- `internal/fabricengine/prune.go`
- `internal/fabricengine/pull.go`
- `internal/fabricengine/reconcile.go`
- `internal/fabricengine/refusalof_test.go`
- `internal/fabricengine/remove.go`
- `internal/fabricengine/spawn.go`
- `internal/fabricengine/spawn_test.go`
- `internal/fabricengine/unwire.go`
- `internal/fabricengine/warpforward_integration_test.go`
- `internal/fabricengine/weftgit.go`
- `internal/fabricengine/weftwiring.go`
- `internal/fabricengine/weftwiring_test.go`
- `internal/output/output.go`
- `internal/output/output_test.go`
- `manifest/designs/fabric-crucible-followups.md`
- `manifest/roadmap.md`
