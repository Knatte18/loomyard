# Batch: mutation-vocabulary

```yaml
task: 'fabric: accumulate the result envelope from mutations, not control flow (slice 14)'
batch: 'mutation-vocabulary'
number: 1
cards: 3
verify: go test ./internal/fabricengine/ ./internal/output/
depends-on: []
```

## Batch Scope

This batch introduces the two purely additive primitives every later batch consumes: the shared mutation-record vocabulary in a new `internal/fabricengine/mutation.go`, and `internal/output`'s new fields-carrying error function.
Nothing existing changes behaviour — no verb, no gate executor, and no envelope is touched here, so the batch compiles and its tests pass with the rest of the tree exactly as it stands today.
The external interface later batches consume is: `Kind` and its fifteen constants, `Mutation`, `Mutations` (constructor, `Append`, `AppendRef`, `Entries`, `Snapshot`, and its `MarshalJSON`), the embeddable `MutationRecord` with its `Mutated()` accessor, and `output.ErrFields`.

Batch-local decision: `Mutations` is a value type carrying an unexported `hubRoot` and an unexported `entries` slice, and it is passed around as `*Mutations` while accumulating.
`Snapshot()` returns a value copy with its own backing array, so a result type embedding the record cannot be mutated through the recorder after the verb returns.

## Cards

### Card 1: the mutation vocabulary

- **Context:**
  - `_mill/discussion.md`
  - `internal/fabricengine/prune.go`
  - `internal/fabricengine/doc.go`
  - `cmd/lyx/destructiveguard_test.go`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/mutation.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `internal/fabricengine/mutation.go` in `package fabricengine`, the single declarer of the mutation-record vocabulary.

  Declare `type Kind string` with exactly these fifteen constants, spelled exactly as the string values shown (these values are part of fabric's public JSON contract):
  `KindPathRemoved = "path_removed"`, `KindWorktreeRemoved = "worktree_removed"`, `KindLinkRemoved = "link_removed"`, `KindBranchDeleted = "branch_deleted"`, `KindWorktreeReset = "worktree_reset"`, `KindDirCreated = "dir_created"`, `KindWorktreeCreated = "worktree_created"`, `KindBranchCreated = "branch_created"`, `KindBranchPushed = "branch_pushed"`, `KindCommitCreated = "commit_created"`, `KindLinkCreated = "link_created"`, `KindFileWritten = "file_written"`, `KindPushSpawned = "push_spawned"`, `KindWorktreeSwitched = "worktree_switched"`, `KindRepoAdvanced = "repo_advanced"`.
  The type's doc comment states the single-declarer rule: a new member lands in the same commit as its recording site and its `cmd/lyx/destructiveguard_test.go` guard entry, and no other file declares a kind string literal.

  `KindRepoAdvanced`'s doc comment must additionally state that "repo" is **deliberately side-agnostic** here and is not vocabulary drift: the kind is recorded for both the weft fast-forward pull and the warp advance, and the entry's `Target` — the advanced worktree root — is what tells the two sides apart.
  The Fabric Vocabulary Invariant polices `host`-compounded phrases, not the bare word "repo", so this neither breaks the build nor bends the rule;
  the comment exists so the reviewer of the next slice reads it as the decision it is rather than as a warp/weft naming lapse.

  Declare `type Mutation struct` with exactly three exported fields and these JSON tags: `Kind Kind \`json:"kind"\``, `Target string \`json:"target"\``, `Detail string \`json:"detail,omitempty"\``.
  Ordering is carried by array order alone — do not add a sequence field.

  Declare `type Mutations struct` with two unexported fields, `hubRoot string` and `entries []Mutation`, and:

  - `func NewMutations(hubRoot string) *Mutations` — the only constructor. `hubRoot` may be empty, in which case `Append` never converts and records the absolute slashed path.
  - `func (m *Mutations) Append(kind Kind, target, detail string)` — appends one entry, converting `target` from an absolute filesystem path to a hub-relative `filepath.ToSlash`'d string via `filepath.Rel(m.hubRoot, target)`, falling back to `filepath.ToSlash(target)` when `filepath.Rel` errors or when the result escapes the hub root (a `..` first segment). A `target` that resolves to the hub root itself is recorded as the literal `"."`, matching `filepath.Rel`'s own output — this is `CloneHub`'s hub-minting case and is deliberately kept rather than special-cased away.
  - `func (m *Mutations) AppendRef(kind Kind, ref, detail string)` — appends one entry with `Target` set to `ref` verbatim, performing no path arithmetic at all. This is the git-ref recording path (`branch_created`, `branch_deleted`, `branch_pushed`).
  - `func (m *Mutations) Entries() []Mutation` — returns a copy, never the internal slice, and never `nil`: an empty record returns an empty non-nil slice.
  - `func (m *Mutations) Snapshot() Mutations` — returns a value copy whose `entries` is a freshly allocated copy of the current entries, so later appends through the recorder cannot mutate an already-returned result.
  - `func (m Mutations) MarshalJSON() ([]byte, error)` — marshals to a JSON array of entries, emitting `[]` rather than `null` for an empty or zero-value record. Declared on the value receiver so both `Mutations` and `*Mutations` marshal identically.
  - `func (m *Mutations) Len() int` — the entry count, so callers can test "record non-empty" without copying the slice. The receiver is a **pointer**, not a value, because card 7 requires `Len()` to be nil-safe and a value receiver would dereference a nil pointer and panic. One consequence the implementer must carry into batch 6: `res.Mutated()` returns a non-addressable `Mutations` value, so `res.Mutated().Len()` does not compile — `errWithRecord` takes the record as a parameter and calls `Len()` on its own addressable local.

  A nil `*Mutations` receiver must be safe on `Append` and `AppendRef` (both return without panicking), so a not-yet-threaded call site degrades to recording nothing rather than crashing.

  Declare the embeddable carrier:

  ```go
  type MutationRecord struct {
  	Mutations Mutations `json:"mutations"`
  }

  func (r MutationRecord) Mutated() Mutations { return r.Mutations }
  ```

  `MutationRecord` is what every mutating result type embeds in batch 3, and `Mutated()` is the one accessor `internal/fabricengine/fabrictest` reads a heterogeneous result through.
  Its doc comment says so.

  Follow the package's existing doc-comment density: every exported identifier carries a comment naming the decision it implements.
- **Commit:** `feat(fabricengine): add the shared mutation-record vocabulary`

### Card 2: mutation vocabulary unit tests

- **Context:**
  - `internal/fabricengine/mutation.go`
  - `internal/fabricengine/slug_test.go`
  - `internal/fabricengine/testmain_test.go`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/mutation_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `internal/fabricengine/mutation_test.go` — an **untagged** test file in `package fabricengine`, containing no `gitexec.RunGit`, no `exec.Command`, and no `lyxtest.Copy*` token anywhere including comments (Test Tier Purity Invariant, raw substring match).

  Table tests covering:

  - `Append` ordering: three appends come back from `Entries()` in append order.
  - `Append`'s hub-relative conversion: a target below the hub root yields a slash-separated hub-relative `Target`;
    a target that is the hub root itself yields `"."`;
    a target outside the hub root yields the absolute slashed path;
    an empty `hubRoot` yields the absolute slashed path.
    Build the path inputs with `filepath.Join` so the test passes on Windows, and assert against slash-separated expectations.
  - `AppendRef` records its ref verbatim and is unaffected by `hubRoot`.
  - `Entries()` returns a copy: mutating the returned slice does not change what a second `Entries()` call reports, and an empty record returns a non-nil zero-length slice.
  - `Snapshot()` isolates: appending through the recorder after taking a snapshot does not change the snapshot's `Len()`.
  - `MarshalJSON`: an empty record marshals to `[]` (never `null`);
    a zero-value `Mutations{}` also marshals to `[]`;
    a one-entry record with an empty `Detail` marshals with no `detail` key;
    a one-entry record with a non-empty `Detail` marshals all three keys.
    Assert against the exact JSON byte string.
  - A `MutationRecord` embedded in a throwaway local struct marshals its record under the `mutations` key, and `Mutated()` returns the same entries.
  - A nil `*Mutations` receiver: `Append` and `AppendRef` do not panic.
- **Commit:** `test(fabricengine): cover the mutation-record vocabulary`

### Card 3: `output.ErrFields`

- **Context:**
  - `internal/fabricengine/mutation.go`
- **Edits:**
  - `internal/output/output.go`
  - `internal/output/output_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add to `internal/output/output.go`:

  ```go
  func ErrFields(w io.Writer, msg string, fields map[string]any) int
  ```

  It mirrors `Ok`'s existing shape: it writes one JSON line to `w` and returns exit code 1.
  It lays the caller's `fields` down first, then injects `"ok": false` and `"error": strings.TrimSpace(msg)` **after**, so those two reserved keys always win over a caller-supplied key of the same name — matching `Ok`'s existing in-place `fields["ok"] = true` mutation rather than inventing a second collision policy.
  A `nil` `fields` map is accepted and behaves exactly like `Err(w, msg)`.
  `msg` is trimmed exactly as `Err` trims it today.
  The doc comment states the reserved-key rule and names `Err` as the zero-field case.

  `Err` and `Ok` keep their current signatures and behaviour verbatim — this change is strictly additive, since `internal/output` is shared by every module.

  Extend `internal/output/output_test.go` with:

  - `ErrFields` emits `ok:false`, the trimmed message, and the supplied fields.
  - `ErrFields` with a `nil` map emits byte-identical output to `Err` for the same message.
  - A caller supplying `"ok"` or `"error"` in `fields` is overridden by the injected values.
  - Regression assertions that `Ok` and `Err` behaviour is unchanged (keep the existing cases;
    add one explicitly-named regression case per function if the file does not already carry one).
- **Commit:** `feat(output): add ErrFields for a fields-carrying error envelope`

## Batch Tests

`verify: go test ./internal/fabricengine/ ./internal/output/` runs both packages' untagged unit tests, covering the two new test surfaces (`internal/fabricengine/mutation_test.go`, `internal/output/output_test.go`) plus every existing untagged test in `internal/fabricengine`, which this batch must not disturb — nothing existing is edited, so a failure there means the new file broke compilation or shadowed an identifier.
The scope is two packages rather than the repo, matching what the batch touches.
