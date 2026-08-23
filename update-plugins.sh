#!/usr/bin/env bash
# Force-sync all loomyard plugins from this repo's plugins/ dir into the
# Claude Code plugin cache. `claude plugin update`/`/plugin update` is
# version-gated and skips when marketplace.json's version is unchanged, so it
# cannot deploy in-place file edits without a version bump. This mirrors the
# current source into the cache regardless of version, run manually only --
# never a live/auto-update.
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
manifest_path="$repo_root/.claude-plugin/marketplace.json"
marketplace="$(jq -r '.name' "$manifest_path")"
cache_base="$HOME/.claude/plugins/cache/$marketplace"

jq -c '.plugins[]' "$manifest_path" | while read -r plugin; do
    name="$(jq -r '.name' <<<"$plugin")"
    version="$(jq -r '.version' <<<"$plugin")"
    source_dir="$repo_root/plugins/$name"
    target_dir="$cache_base/$name/$version"

    if [[ ! -d "$target_dir" ]]; then
        echo "Skipped (not installed): $name@$marketplace -- run '/plugin install $name@$marketplace' first."
        continue
    fi

    # Mirror source -> target, deleting anything in target that source no
    # longer has -- including a stale built binary, which forces a rebuild on
    # next invocation rather than silently running old code.
    rsync -a --delete "$source_dir/" "$target_dir/"
    echo "Force-synced: $name@$marketplace ($version)"
done

echo ""
echo "Done."
