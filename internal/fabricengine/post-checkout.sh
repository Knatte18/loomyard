#!/bin/sh
# FABRIC_SENTINEL: post-checkout drift warning — do not remove this line.
#
# Fires after git checkout/switch; resolves the current worktree root, derives
# the deterministic <base>-weft sibling, and warns (non-blocking) when the
# weft worktree is not checked out on the host branch's paired weft branch
# (<host-branch>-weft under fabric's uniform naming scheme). Exit 0 always per
# principle 6 (never hard-block).
#
# Note: git sets GIT_DIR (and related env vars) in the hook environment. Calling
# `git -C <path>` on a sibling repo inherits these vars and would incorrectly
# resolve against the current repo instead of the sibling. We unset them before
# any git call that targets a different repo.

# $1 = previous HEAD, $2 = new HEAD, $3 = 1 if branch checkout, 0 if file checkout.
# We only care about branch checkouts.
if [ "${3:-0}" != "1" ]; then
    exit 0
fi

# Unset git hook environment variables so that any git calls targeting
# a different repo (the weft sibling) resolve cleanly without inheriting
# the current hook context.
unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE GIT_OBJECT_DIRECTORY

# Resolve the current worktree root.
WORKTREE_ROOT="$(git rev-parse --show-toplevel 2>/dev/null)"
if [ -z "$WORKTREE_ROOT" ]; then
    exit 0
fi

# Derive the weft sibling path: parent-dir/<basename>-weft.
PARENT="$(dirname "$WORKTREE_ROOT")"
BASE="$(basename "$WORKTREE_ROOT")"
WEFT_WORKTREE="${PARENT}/${BASE}-weft"

# If the weft worktree does not exist, nothing to compare — exit silently.
if [ ! -d "$WEFT_WORKTREE" ]; then
    exit 0
fi

# Get the host branch name.
HOST_BRANCH="$(git -C "$WORKTREE_ROOT" rev-parse --abbrev-ref HEAD 2>/dev/null)"
if [ -z "$HOST_BRANCH" ] || [ "$HOST_BRANCH" = "HEAD" ]; then
    exit 0
fi

# Get the weft branch name.
WEFT_BRANCH="$(git -C "$WEFT_WORKTREE" rev-parse --abbrev-ref HEAD 2>/dev/null)"
if [ -z "$WEFT_BRANCH" ]; then
    exit 0
fi

# Under fabric's uniform branch scheme, the weft branch paired with HOST_BRANCH
# is always HOST_BRANCH plus the "-weft" suffix — this literal is shell-side
# data mirroring hubgeometry.WeftSuffix, not Go path construction, so it does
# not trip the Go-source geometry-literal ban.
EXPECTED_WEFT_BRANCH="${HOST_BRANCH}-weft"

# Warn when they differ; exit 0 always (non-blocking).
if [ "$EXPECTED_WEFT_BRANCH" != "$WEFT_BRANCH" ]; then
    echo "fabric: host/weft out of sync — run \`lyx fabric checkout <branch>\` to re-sync, or \`lyx fabric reconcile\` to inspect" >&2
    echo "  host: $HOST_BRANCH (expects weft: $EXPECTED_WEFT_BRANCH)" >&2
    echo "  weft: $WEFT_BRANCH" >&2
fi

exit 0
