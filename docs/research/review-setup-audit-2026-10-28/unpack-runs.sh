#!/usr/bin/env bash
# Usage: unpack-runs.sh <weft-repo> <out-dir>
# Unpacks each archive tag listed in runs.txt from the weft repository into <out-dir>/<slug>,
# the layout every extract-*.sh script reads.
set -euo pipefail
weft=$1
out=$2
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
while read -r tag; do
  slug=$(echo "$tag" | cut -d/ -f2)
  mkdir -p "$out/$slug"
  git -C "$weft" archive "$tag" | tar -x -C "$out/$slug"
done < "$here/runs.txt"
