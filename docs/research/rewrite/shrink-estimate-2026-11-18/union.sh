#!/bin/bash
# Usage: union.sh <dupl plumbing file>
# Prints the duplicated-line union (a line in several clones counts once), the directed pair count and the 3 largest clone pairs.
f=$1
grep -E '^[^ ]+:[0-9]+-[0-9]+: duplicate of' "$f" | sed -E 's/^([^:]+):([0-9]+)-([0-9]+):.*/\1 \2 \3/' | sort -k1,1 -k2,2n |
awk '
{ if ($1 != cf) { if (cf != "") tot += ce - cs + 1; cf = $1; cs = $2; ce = $3; next }
  if ($2 <= ce + 0) { if ($3 > ce) ce = $3 } else { tot += ce - cs + 1; cs = $2; ce = $3 } }
END { if (cf != "") tot += ce - cs + 1; print tot + 0 }'
echo "pairs(directed) $(grep -cE 'duplicate of' "$f")"
grep -E 'duplicate of' "$f" | sed -E 's/^([^:]+):([0-9]+)-([0-9]+): duplicate of ([^:]+):([0-9]+)-([0-9]+)/\3 \2 \1:\2-\3 \4:\5-\6/' | awk '{print $1-$2+1, $3, $4}' | sort -rn |
awk '{k1=$2" "$3; k2=$3" "$2} !(k1 in s) {s[k1]; s[k2]; print; n++} n>=3 {exit}'
