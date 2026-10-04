# PATTERN-hub-suffix

`-LYXHUB` is the sole hub container suffix, and no code parses, trims or recognises the retired `-HUB`.

## Declaration sites

- The suffix is declared twice by sanction: `internal/lyxcwd` holds the private `hubSuffix` const, used for `RepoName` derivation, and `internal/fabricengine` holds the exported `HubSuffix` const, used by `HubPath`.
- Both move together, and `TestEnforcement_GeometryLiterals`'s `geometryTokenOwners` row is the third site that must move with them.

## Discovery

- Hub discovery is name-independent: the hub is `filepath.Dir(workTreeRoot)` and `looksLikeHub` is structural, so a hub still carrying the retired suffix still resolves.
- Only `Location.RepoName`, a display-only value never used to construct a path, degrades.

## Retired-suffix hubs

- A hub carrying the retired suffix is never renamed in place: `PortalLink` and `LauncherDir` materialise links against the hub's absolute path at creation time, and `ServerName` hashes that absolute path into the tmux socket key.
- The operator removes the old container by hand and re-clones.
- `clone --reset` resolves the new suffix only and never reaches the old container.
