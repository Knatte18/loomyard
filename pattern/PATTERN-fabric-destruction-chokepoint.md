# PATTERN-fabric-destruction-chokepoint

`internal/fabricengine/destroy.go` is the only file in `package fabricengine` permitted a destructive primitive.

- Every executor checks, in order, stopping at the first failure: containment, ownership, dirtiness, force.
  The one exception is `removeWorktreeLeftover`, which keeps containment through `os.Root` and takes ownership and dirtiness from the gated `git worktree remove` its caller just ran on the same target.
- `--force` answers dirtiness only, never containment or ownership.
- A gate refusal is never silently discarded.
- The `rec *Mutations` recorder is threaded into `destroy.go` only.
