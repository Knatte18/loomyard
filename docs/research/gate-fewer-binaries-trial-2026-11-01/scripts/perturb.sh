#!/usr/bin/env bash
# perturb.sh <package dir> <value>: adds <dir>/zz_benchperturb.go, whose init stores <value> in a package variable,
# so the package's object code changes and every test binary that links the package is relinked.
# (An unused variable would not do: the linker drops it and most binaries stay identical.)
# perturb.sh --revert removes every such file.
set -euo pipefail
if [ "${1:-}" = "--revert" ]; then
  git clean -fq -- '*zz_benchperturb.go'
  exit 0
fi
dir=${1:?package dir}
value=${2:?value}
pkg=$(go list -f '{{.Name}}' "./$dir")
cat >"$dir/zz_benchperturb.go" <<GO
// zz_benchperturb.go changes the package for a gate-binary trial and is never committed.

package $pkg

// BenchPerturb is written by init so the linker keeps it.
var BenchPerturb int

func init() {
	BenchPerturb = $value
}
GO
