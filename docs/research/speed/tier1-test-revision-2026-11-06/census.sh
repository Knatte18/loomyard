#!/usr/bin/env bash
# Usage: census.sh <trace-dir> > procs.tsv
# Prints one line per git process the trace2 event files in <trace-dir> hold: the first two git arguments, F or C, the process's seconds, and the repository path.
# F marks a process that carried the fixture marker LYM_FIXTURE_GIT (the gitkit helpers, every git while a hub builds, and the production verbs a fixture drives), C every other process.
# Set the trace in the environment of the run, then run the tier through the gate:
#   GIT_TRACE2_EVENT=<trace-dir> GIT_TRACE2_ENV_VARS=LYM_FIXTURE_GIT GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=trace2.maxFiles GIT_CONFIG_VALUE_0=0 lym gate test --wait --tier 1 ./internal/fabricengine -- -count=1
# A count by call is then `cut -f1,2 procs.tsv | sort | uniq -c | sort -nr`, and a count by test is the `Test...` element of the repository path.
# Needs bash, find, jq and awk; reads only.
set -euo pipefail
export LC_ALL=C

find "$1" -type f -print0 | xargs -0 cat |
  jq -r 'select(.event == "start" or .event == "def_param" or .event == "exit" or .event == "def_repo")
    | [.sid, .event, (if .event == "start" then (.argv[1:3] | join(" "))
                      elif .event == "def_param" then .param
                      elif .event == "exit" then (.t_abs | tostring)
                      else .worktree end)] | @tsv' |
  awk -F'\t' '
    { s = $1; if (!(s in seen)) { seen[s] = 1; order[++n] = s }
      if ($2 == "start") args[s] = $3
      else if ($2 == "def_param" && $3 == "LYM_FIXTURE_GIT") fixture[s] = 1
      else if ($2 == "exit") seconds[s] = $3
      else if ($2 == "def_repo" && !(s in repo)) repo[s] = $3 }
    END { for (i = 1; i <= n; i++) { s = order[i]; printf "%s\t%s\t%s\t%s\n", args[s], (s in fixture) ? "F" : "C", seconds[s], repo[s] } }'
