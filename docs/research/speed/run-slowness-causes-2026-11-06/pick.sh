#!/usr/bin/env bash
# Usage: pick.sh [runs.tsv] > pick.tsv
# Picks the landed runs whose session transcripts the study reads, at most 40, and prints one line per run:
# slug tag recipe start end period band lines active
# Darn: code runs with no machine sleep, no wait on anyone, a known size and a transcript directory on this machine, whose slug no other run shares.
# Three periods by the run's start: before (before f08df88b, the first agent-turns change), mid (from the batching hook eea96fe2 to its widening 119c1c05)
# and late (after 48f17897, which took a push's transfer out of the remote-refs lock, so late is the code as it runs now).
# Four size bands by changed lines, each with a target at its middle; per band the runs nearest the target are taken, 4 before, 2 mid and 2 late,
# so the periods hold the same sizes.
# Tapestry: eight runs named below, four before and four after, paired by size; the pairing is the report's to explain.
# Needs bash, awk and git; reads only. PROJECTS names Claude Code's projects directory (default ~/.claude/projects).
set -euo pipefail
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
root=$(git -C "$here" rev-parse --show-toplevel)
runs=${1:-$here/runs.tsv}
prime=${PRIME:-$(git -C "$root" worktree list --porcelain | awk 'NR == 1 { print $2 }')}
projects=${PROJECTS:-$HOME/.claude/projects}
hub=$(dirname "$prime")
encoded=$(printf '%s' "$hub" | tr -c 'A-Za-z0-9' '-')

# utc <commit>: the commit's committer time in UTC, as the timing report writes a start.
utc() { TZ=UTC git -C "$prime" log -1 --format=%cd --date=format-local:%Y-%m-%dT%H:%M:%SZ "$1"; }

tapestry_before="reed-status-buttons gate-run-keeps-cache-warm warp-visibility run-verbs-shared"
tapestry_after="shed-sub-run-block batten-remote-order pawl-orch-landing raddle-recipe"

have=$(for d in "$projects/$encoded"-*; do [ -d "$d" ] && printf '%s\n' "${d#"$projects/$encoded"-}"; done)

printf 'slug\ttag\trecipe\tstart\tend\tperiod\tband\tlines\tactive\n'
awk -F'\t' -v OFS='\t' \
  -v cut_before="$(utc f08df88b)" -v hook="$(utc eea96fe2)" -v widened="$(utc 119c1c05)" -v unlocked="$(utc 48f17897)" \
  -v have="$have" -v tap_before="$tapestry_before" -v tap_after="$tapestry_after" '
  function abs(x) { return x < 0 ? -x : x }
  BEGIN {
    n = split(have, h, "\n"); for (i = 1; i <= n; i++) transcript[h[i]] = 1
    n = split(tap_before, t, " "); for (i = 1; i <= n; i++) tap[t[i]] = "before"
    n = split(tap_after, t, " "); for (i = 1; i <= n; i++) tap[t[i]] = "after"
    take["before"] = 4; take["mid"] = 2; take["late"] = 2
  }
  NR == 1 { next }
  { count[$1]++; line[NR] = $0 }
  END {
    for (r = 2; r <= NR; r++) {
      split(line[r], f, "\t")
      slug = f[1]; start = f[4]
      if (!(slug in transcript) || count[slug] > 1) continue
      if (f[3] == "tapestry") {
        if (slug in tap) print slug, f[2], f[3], start, "-", tap[slug], "-", f[28], f[7]
        continue
      }
      if (f[3] != "darn" || f[23] == "docs" || slug ~ /bench|trial|audit|measure|survey|spike|remeasure/) continue
      if (f[6] != 0 || f[8] + f[9] + f[10] + f[11] != 0 || f[28] == "-" || f[28] + 0 == 0) continue
      period = ""
      if (start < cut_before) period = "before"
      else if (start > hook && start <= widened) period = "mid"
      else if (start > unlocked) period = "late"
      if (period == "") continue
      lines = f[28] + 0
      if (lines < 100) { band = "1: under 100"; target = 50 }
      else if (lines < 300) { band = "2: 100 to 299"; target = 200 }
      else if (lines < 1000) { band = "3: 300 to 999"; target = 600 }
      else { band = "4: 1000 and over"; target = 1500 }
      key = period SUBSEP band
      m = ++members[key]
      dist[key, m] = abs(lines - target); row[key, m] = slug OFS f[2] OFS f[3] OFS start OFS "-" OFS period OFS band OFS lines OFS f[7]
    }
    for (key in members) {
      split(key, k, SUBSEP)
      for (picked = 0; picked < take[k[1]] && picked < members[key]; picked++) {
        best = 0
        for (m = 1; m <= members[key]; m++) if (!((key, m) in used) && (best == 0 || dist[key, m] < dist[key, best])) best = m
        used[key, best] = 1
        print row[key, best]
      }
    }
  }' "$runs" | sort -t$'\t' -k3,3 -k6,6 -k7,7 -k4,4 |
  while IFS=$'\t' read -r slug tag recipe start _ period band lines active; do
    end=$(awk -F'\t' -v t="$tag" '$2 == t { print $4, $5 }' "$runs" | { read -r s span; date -u -d "$s + $span seconds" +%Y-%m-%dT%H:%M:%SZ; })
    printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n' "$slug" "$tag" "$recipe" "$start" "$end" "$period" "$band" "$lines" "$active"
  done
