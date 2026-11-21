#!/usr/bin/env bash
# Usage: timing.sh <slug>
# Prints one line per archived trace of a row's `lym gate test --wait ... --gate <key>` call:
# its start, gate key, call seconds, tier seconds (first line to the last "tier command ended"),
# seconds after the tiers, and the spans of its map refresh (testmap- binaries), redundancy step (redundancy- binaries)
# and mutation step (from its first mutants line to the call's end).
# A call whose tiers were all carried has no tier command, so all of it counts as after the tiers.
# Needs bash, awk and GNU date; reads only.
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
traces=$(ls -d "$ARCHIVE/$1"/*/trace)
for f in $(grep -l 'argv=gate test --wait.*--gate ' "$traces"/*.log); do
  key=$(grep -m1 -o -- '--gate [a-z-]*' "$f" | cut -d' ' -f2)
  awk -v key="$key" '
    function ep(t,   c, s) { c = "date -d " t " +%s.%N"; c | getline s; close(c); return s }
    /^time=/ {
      t = substr($1, 6); if (first == "") first = t; last = t
      if ($0 ~ /tier command ended/) tierend = t
      if ($0 ~ /testmap-/) { if (m0 == "") m0 = t; m1 = t }
      if ($0 ~ /redundancy-/) { if (r0 == "") r0 = t; r1 = t }
      if ($0 ~ /mutants|gate test mutation/) { if (u0 == "") u0 = t }
    }
    END {
      if (first == "") exit
      f = ep(first); l = ep(last); te = (tierend == "" ? f : ep(tierend))
      printf "%s %s call=%.0f tiers=%.0f after=%.0f", first, key, l - f, te - f, l - te
      printf " refresh=%s", (m0 == "" ? "-" : sprintf("%.0f", ep(m1) - ep(m0)))
      printf " redundancy=%s", (r0 == "" ? "-" : sprintf("%.0f", ep(r1) - ep(r0)))
      printf " mutation=%s\n", (u0 == "" ? "-" : sprintf("%.0f", l - ep(u0)))
    }' "$f"
done | sort
