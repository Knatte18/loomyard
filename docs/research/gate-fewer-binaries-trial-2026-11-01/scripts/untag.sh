#!/usr/bin/env bash
# untag.sh <package dir>...: makes each package's test files tier-agnostic for the one-binary-per-package trial.
# The tierN term leaves every //go:build line (a line left with none is dropped), and each Test function of a file
# that carried a tier tag opens with tiergate.Require(t, N) for that file's tier.
# A package whose tier1 and non-tier1 TestMain files differ keeps the tier1 one, untagged.
set -euo pipefail
import='"github.com/Knatte18/loomyard/internal/testkit/tiergate"'
for dir in "$@"; do
  if [ -f "$dir/testmain_integration_test.go" ] && head -1 "$dir/testmain_test.go" | grep -q '^//go:build !tier1'; then
    git rm -q "$dir/testmain_test.go"
  fi
  for file in "$dir"/*_test.go; do
    [ -f "$file" ] || continue
    first=$(head -1 "$file")
    case "$first" in //go:build*) ;; *) continue ;; esac
    expr=${first#//go:build }
    tier=$(echo "$expr" | grep -o 'tier[0-3]' | head -1 | tr -d 'a-z')
    [ -n "$tier" ] || continue
    rest=$(echo "$expr" | sed -E 's/ && tier[0-3]//g; s/tier[0-3] && //g')
    if echo "$rest" | grep -Eq '^tier[0-3]( \|\| tier[0-3])*$'; then
      sed -i '1,2d' "$file"
    else
      sed -i "1s|.*|//go:build $rest|" "$file"
    fi
    if grep -Eq '^func Test[A-Za-z0-9_]*\(t \*testing\.T\) \{$' "$file"; then
      sed -i -E "s/^(func Test[A-Za-z0-9_]*\(t \*testing\.T\) \{)\$/\1\n\ttiergate.Require(t, $tier)/" "$file"
      if grep -q '^import ($' "$file"; then
        sed -i "0,/^import (\$/s||import (\n\t$import|" "$file"
      else
        sed -i "0,/^package .*\$/s||&\n\nimport $import|" "$file"
      fi
    fi
  done
done
