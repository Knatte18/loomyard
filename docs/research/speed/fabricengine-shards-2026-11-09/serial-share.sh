#!/usr/bin/env bash
# Usage: serial-share.sh <events.json>
# <events.json> is the timestamped test2json stream shard-runner.sh writes for a one-process run.
# Prints how much of the package's test time ran in tests not marked parallel, and the longest serial chains.
# A test is marked parallel when it has a pause event, which t.Parallel() causes.
# Go runs every top-level test that is not parallel one after the other first (phase 1),
# then resumes the paused ones together (phase 2), so phase 1 is a serial chain of the whole package.
# Needs jq; reads only.
set -euo pipefail
export LC_ALL=C

jq -s -r '
  def seconds: capture("^(?<whole>[0-9T:-]+)(?<fraction>\\.[0-9]+)?(Z|[+-][0-9:]+)$") | (.whole + "Z" | fromdate) + ((.fraction // "0") | tonumber);
  [ .[] | select(.Test != null) | . + {at: (.Time | seconds)} ] as $events
  | ($events | map(select(.Action == "pause") | .Test) | unique) as $paused
  | ($events | map(select(.Action == "pass" or .Action == "fail") | {test: .Test, elapsed: .Elapsed, at: .at})) as $ended
  | ($ended | map(.test)) as $endedNames
  | ($ended | map(select(.test | contains("/") | not))) as $tops
  | ($ended | map(select(.test as $name | ($endedNames | map(select(startswith($name + "/"))) | length) == 0))) as $leaves
  | ($events | map(select(.Action == "run")) | map(.at) | min) as $firstRun
  | ($events | map(select(.Action == "cont" and (.Test | contains("/") | not))) | map(.at) | min) as $firstCont
  | ($ended | map(.at) | max) as $lastEnd
  | ($tops | map(select(.test as $name | ($paused | index($name)) == null))) as $serialTops
  | ($leaves | map(select(.test as $name | ($paused | index($name)) == null))) as $unmarkedLeaves
  | def sum: (map(.elapsed) | add // 0);
    def share($part; $whole): (100 * $part / $whole);
    {
      package_seconds: ($lastEnd - $firstRun),
      top_level_tests: ($tops | length),
      top_level_tests_marked_parallel: (($tops | length) - ($serialTops | length)),
      leaf_tests: ($leaves | length),
      leaf_tests_marked_parallel: (($leaves | length) - ($unmarkedLeaves | length)),
      leaf_seconds_total: ($leaves | sum),
      leaf_seconds_unmarked: ($unmarkedLeaves | sum),
      leaf_unmarked_percent: share(($unmarkedLeaves | sum); ($leaves | sum)),
      serial_top_level_seconds: ($serialTops | sum),
      phase1_seconds: ($firstCont - $firstRun),
      phase2_seconds: ($lastEnd - $firstCont),
      phase1_percent_of_package: share($firstCont - $firstRun; $lastEnd - $firstRun),
      longest_top_level_seconds: ($tops | max_by(.elapsed) | {test, elapsed}),
      longest_serial_top_levels: ($serialTops | sort_by(-.elapsed) | .[:8] | map({test, elapsed})),
      phase2_busy_top_level_seconds: ($tops | map(select(.test as $name | ($paused | index($name)) != null)) | sum),
      phase2_mean_concurrent_top_level_tests: (($tops | map(select(.test as $name | ($paused | index($name)) != null)) | sum) / ($lastEnd - $firstCont))
    }
' "$1"
