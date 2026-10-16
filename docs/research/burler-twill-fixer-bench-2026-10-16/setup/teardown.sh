#!/usr/bin/env bash
# teardown.sh <pair>...: reed down in each pair, fabric remove (forced, remote branches too) from the prime until finished, then the pair's note.
H=/home/knatte/Code/loomyard-LYXHUB
for p in "$@"; do
  echo "== $p"
  if [ -d "$H/$p" ]; then
    (cd "$H/$p" && lyx reed down 2>&1 | tail -c 200); echo
  fi
  for i in 1 2 3 4 5 6; do
    out="$(cd "$H/loomyard" && lyx fabric remove "$p" --force --remote 2>&1)"
    echo "$out" | tail -c 300; echo
    case "$out" in *'"finished":true'*) break ;; esac
    sleep 10
  done
  git -C "$H/loomyard" branch -D "$p" 2>/dev/null
  (cd "$H/loomyard" && lyx board remove "{\"slug\":\"$p\"}" 2>&1 | tail -c 150); echo
done
