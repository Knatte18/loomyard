#!/usr/bin/env bash
# Usage: extract-keys.sh <runs-dir>
# Prints one TSV row per bouncer-ledger key of every perch of every run under <runs-dir>:
# run perch key first_round class severity fix_kind rounds_seen final_status
# class, severity and fix_kind come from the ledger round that first lists the key;
# final_status is the key's status in the perch's last ledger.
set -euo pipefail
runs=$1
printf 'run\tperch\tkey\tfirst_round\tclass\tseverity\tfix_kind\trounds_seen\tfinal_status\n'
for dir in "$runs"/*/_lyx/reviews/*/; do
  ls "$dir"round-*-bouncer-ledger.md >/dev/null 2>&1 || continue
  run=${dir#"$runs"/}; run=${run%%/*}
  perch=$(basename "$dir")
  awk -v run="$run" -v perch="$perch" '
    FNR == 1 { fileno++ }
    /^  - key:/ { key = $3; seen[key] = 1; if (!(key in order)) order[key] = ++n; names[order[key]] = key }
    /^    rounds:/ { s = $0; gsub(/[^0-9,]/, "", s); m = split(s, parts, ","); rs[key] = m; if (!(key in first)) first[key] = parts[1] }
    /^    status:/ { st[key] = $2 }
    /^    class:/ { if (!(key in cls)) cls[key] = $2 }
    /^    severity:/ { if (!(key in sev)) sev[key] = $2 }
    /^    fix:/ { if (!(key in fx)) fx[key] = $2 }
    END { for (i = 1; i <= n; i++) { k = names[i]; printf "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n", run, perch, k, first[k], cls[k], sev[k], fx[k], rs[k], st[k] } }
  ' $(ls "$dir"round-*-bouncer-ledger.md | sort -V)
done
