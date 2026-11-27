#!/usr/bin/env bash
# Usage: findings.sh <tag>...
# Prints one line per darn run's archive tag, tab-separated: slug, when the run started, code review rounds,
# findings of the merged code reviews, those of severity HIGH or BLOCKING, those of MEDIUM,
# and those whose origin is the test lens.
# A tag of another recipe prints nothing.
# Needs bash, git, jq and an awk; reads only.
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
prime=${PRIME:-$(git -C "$here" worktree list --porcelain | awk 'NR == 1 { print $2 }')}
records=${WEFT:-$prime-weft}
for tag in "$@"; do
  slug=$(printf '%s' "$tag" | cut -d/ -f2)
  seed=$(git -C "$records" show "$tag:_lym/shed/$slug/seed.json" 2>/dev/null) || continue
  [ "$(printf '%s' "$seed" | jq -r .recipe)" = darn ] || continue
  reviews=$(git -C "$records" ls-tree -r --name-only "$tag" -- _lym/reviews/code | grep -E '/round-[0-9]+-review\.md$')
  for review in $reviews; do
    # Only the frontmatter lists findings.
    git -C "$records" show "$tag:$review" | awk 'NR == 1 && /^---$/ { on = 1; next } on && /^---$/ { exit } on'
  done | awk -v slug="$slug" -v started="$(printf '%s' "$seed" | jq -r .started_at)" -v rounds="$(printf '%s' "$reviews" | grep -c .)" '
    /^ *- id:/ { n++ }
    /^ *severity: *(HIGH|BLOCKING)/ { high++ }
    /^ *severity: *MEDIUM/ { medium++ }
    /^ *origin: *.*code-tests/ { tests++ }
    END { printf "%s\t%s\t%d\t%d\t%d\t%d\t%d\n", slug, started, rounds, n, high, medium, tests }'
done
