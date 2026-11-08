#!/usr/bin/env bash
# Usage, as the -exec program of a coverage-instrumented go test run, with the output directory as its first argument:
#   lym gate test --wait --tier 1 ./internal/fabricengine -- -count=1 -cover -coverpkg=./internal/fabricengine -exec "<abs path>/coverage-runner.sh <abs out dir>"
# go test then calls this script with the compiled test binary and the flags it would have passed the binary.
# The script runs the binary once per top-level test, with LYM_TEST_TIER as the gate set it, and leaves in <out dir>:
#   profiles/<Test>.out  the block coverage profile of that test's run
#   logs/<Test>.log      the test's `-test.v` output
#   git/<Test>/          one trace2 event file per git process the run started
# A test the tier skips leaves a log with `--- SKIP` and a profile that covers nothing; the analysis drops it.
# Each run is serial inside (-test.parallel=1), as the per-test times report measured.
# Set RUNNER_JOBS to change the number of runs in flight (default 4); use 1 for times that compare with that report.
# Needs bash, xargs and the test binary; writes only under <out dir>.
set -euo pipefail
export LC_ALL=C

out=$1
binary=$2
mkdir -p "$out/profiles" "$out/logs"
cp "$binary" "$out/fabric.test"

"$out/fabric.test" -test.list '^Test' | grep '^Test' > "$out/tests.txt"

export out
runone() {
  name=$1
  mkdir -p "$out/git/$name"
  GIT_TRACE2_EVENT="$out/git/$name" "$out/fabric.test" -test.count=1 -test.parallel=1 -test.v -test.run "^${name}\$" -test.coverprofile="$out/profiles/$name.out" \
    > "$out/logs/$name.log" 2>&1 || echo "FAIL $name" >> "$out/failed.txt"
}
export -f runone

xargs -a "$out/tests.txt" -P "${RUNNER_JOBS:-4}" -I{} bash -c 'runone {}'
