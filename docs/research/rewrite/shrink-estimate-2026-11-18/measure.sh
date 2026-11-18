#!/bin/bash
# Step 1 of the shrink estimate: per module, codestats lines and dupl clones at 50 tokens over code and over tests.
# Usage: measure.sh <output-dir>   (run from anywhere inside the repository)
# Writes <module>.stats.json, <module>.{code,test}.list and <module>.{code,test}.dupl; union.sh turns a .dupl into line totals.
set -eu
root=$(git rev-parse --show-toplevel)
cd "$root"
out=${1:?output directory}
mkdir -p "$out"
GOBIN="$out" go install github.com/mibk/dupl@v1.0.0
for m in loomcli fabricengine landingshed shuttleengine battencli websterengine driftengine; do
  go run ./tools/codestats -root "internal/$m" -json -out "$out/$m.stats.json"
  find "internal/$m" -name '*.go' ! -name '*_test.go' | sort > "$out/$m.code.list"
  find "internal/$m" -name '*_test.go' | sort > "$out/$m.test.list"
  for k in code test; do
    "$out/dupl" -t 50 -plumbing -files < "$out/$m.$k.list" > "$out/$m.$k.dupl" 2>&1
  done
done
