# PATTERN-fabric-isolation

Fabric is `internal/fabricengine` and `internal/fabriccli`: the paired worktrees, their branches and their git.
It serves every module the same way and knows none of them.

- A seam fabric offers is general: a path, an event or a hook any module can take, never a call, field or type shaped for one consumer.
  A seam only one module could use belongs in that module, which drives fabric through its general verbs.
- Fabric learns nothing new of IDEs, shuttle, the board or heddle: it gains no import of their packages and no knowledge of their files or records.
- An import fabric already holds of such a module is debt.
  A change that touches it moves the knowledge out to the module, and never extends it.
