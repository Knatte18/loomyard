// structuraldirs.go holds the two structural directory sets, which never come from config.
// Each is a function, so no caller can change a set.

package fabriclayout

import "github.com/Knatte18/loomyard/internal/lymdirs"

// StructuralCommittedDirs returns the durable, git-tracked structural directory set: `_lym` alone.
// Its membership is structural, never config or env-overridable (the Cwd Resolution Invariant):
// every lym module fails without `_lym`, so an operator-editable fabric.yaml `pathspec` must not decide it.
// Injecting it in code means no config value can tear away the durable tree.
// Each call returns a fresh slice.
func StructuralCommittedDirs() []string {
	return []string{lymdirs.LymDirName}
}

// StructuralNeverCommittedDirs returns the machine-local, never-git-tracked structural directory set: `.lym` alone.
// Its membership is structural, never config or env-overridable (the Cwd Resolution Invariant):
// every lym module fails without `.lym`, so an operator-editable fabric.yaml `pathspec` must not decide it.
// Injecting it in code means no config value can leave machine-local scratch unwired.
// Each call returns a fresh slice.
func StructuralNeverCommittedDirs() []string {
	return []string{lymdirs.DotLymDirName}
}
