#!/usr/bin/env bash
# Usage: cost.sh <from> <to> [archive-dir]
# Prints one line per archived `lym gate test` trace that started in [from, to), both as 2026-11-19T12:17 local time:
# slug, trace file, gate key, call seconds, seconds from the last tier command's end to the call's end,
# then for each of the map refresh, the redundancy step and the mutation step
# its slot wait seconds, the seconds from its first to its last line, and the test processes it started.
# A step that built or ran nothing prints "- - 0".
# archive-dir defaults to the hub's per-run log archive.
# Needs bash and an awk with mktime; reads only.
from=$1 to=$2
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
prime=${PRIME:-$(git -C "$here" worktree list --porcelain | awk 'NR == 1 { print $2 }')}
archive=${3:-$(dirname "$prime")/_bolt/.lym/log-archive}

# A run archived twice holds the same trace twice, so one file name is read once.
grep -rl 'argv=gate test ' "$archive" --include='trace-*.log' |
  awk -F/ '!seen[$NF]++' |
  while read -r f; do
    slug=${f#"$archive"/}
    awk -v from="$from" -v to="$to" -v slug="${slug%%/*}" -v file="${f##*/}" '
      function ep(s,   t) { t = substr(s, 1, 19); gsub(/[-T:]/, " ", t); return mktime(t) + ("0" substr(s, 20, 4)) }
      function dur(s,   v, n, u) {
        v = 0
        while (match(s, /^[0-9.]+(ms|h|m|s|µs)/)) {
          u = substr(s, RSTART, RLENGTH); n = u + 0; sub(/^[0-9.]+/, "", u)
          v += n * (u == "h" ? 3600 : u == "m" ? 60 : u == "s" ? 1 : u == "ms" ? 0.001 : 0.000001)
          s = substr(s, RLENGTH + 1)
        }
        return v
      }
      NR == 1 { key = "-"; if (match($0, /--gate [a-z-]+/)) key = substr($0, RSTART + 7, RLENGTH - 7) }
      !/^time=/ { next }
      {
        stamp = substr($1, 6); now = ep(stamp)
        if (first == "") { first = now; if (substr(stamp, 1, 16) < from || substr(stamp, 1, 16) >= to) { skip = 1; exit } }
        last = now
      }
      /tier command ended/ { tierend = now }
      # A slot wait is logged only when the step had to wait.
      /msg="gate slot wait end"/ {
        step = /site="lym gate test --map"/ ? "map" : /site="lym gate test redundancy"/ ? "red" : /site="lym gate test mutation"/ ? "mut" : ""
        if (step != "") { match($0, /waited=[^ ]+/); wait[step] += dur(substr($0, RSTART + 7, RLENGTH - 7)) }
      }
      # A test binary names its step by the temporary directory it was built in:
      # testmap- for the map refresh, redundancy- for the redundancy step, redundant- for the mutation step.
      # A line naming no directory, such as the end of a binary, belongs to the step named last.
      /testmap-/ { s = "map" } /redundancy-/ { s = "red" } /redundant-|gate test mutation/ { s = "mut" }
      /testmap-|redundancy-|redundant-|gate test mutation|testcover: / {
        if (s != "") { if (!(s in begin)) begin[s] = now; end[s] = now }
        if (/spawning a coverage binary for one test/) procs[s]++
      }
      END {
        if (skip || first == "") exit
        printf "%s\t%s\t%s\t%.0f\t%s", slug, file, key, last - first, (tierend == "" ? "-" : sprintf("%.0f", last - tierend))
        split("map red mut", steps, " ")
        for (i = 1; i <= 3; i++) {
          s = steps[i]
          if (s in begin) printf "\t%.0f\t%.0f\t%d", wait[s], end[s] - begin[s], procs[s]
          else printf "\t-\t-\t0"
        }
        print ""
      }' "$f"
  done
