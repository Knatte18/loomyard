Perfect! All 5 cards have been implemented successfully:

1. **Card 1**: Added `PortalLink(slug)` method that returns mirrored portal junction links with subpath support
2. **Card 2**: Modified `LauncherDir(slug)` to return mirrored paths by subpath, and added `MenuLauncherPath()` method
3. **Card 3**: Added `LauncherSpawnRel(slug)` and `MenuLauncherRel()` climb helper methods that compute relative paths for .cmd scripts
4. **Card 4**: Extended paths_test.go with comprehensive subtests covering:
   - PortalLink at root and subpath levels
   - LauncherDir backward compatibility and subpath mirroring
   - MenuLauncherPath at root and subpath
   - LauncherSpawnRel and MenuLauncherRel climb calculations
   - Multi-subpath no-collision validation
5. **Card 5**: Added codeguide_guard_test.go to ensure no production code references _codeguide

All tests pass (`go test ./internal/paths/...` ✓). The implementation follows the batch plan exactly:
- All new methods include proper Go doc comments
- Subpath mirroring uses `filepath.Join(l.Container, "_portals"/"_launchers", l.RelPath, ...)` pattern
- At `RelPath == "."`, paths collapse to the backward-compatible form via `filepath.Join`'s "." collapsing behavior
- Relative climb methods use `filepath.Rel` without hand-normalization
- All methods are in `internal/paths/paths.go` and no domain modules hand-roll geometry

{"status":"success","commit_sha":"564b16f22fef4b340db3d104f2308cb957738850","session_id":"fa8145c8-6543-4ae5-850f-49e25f79301b"}
