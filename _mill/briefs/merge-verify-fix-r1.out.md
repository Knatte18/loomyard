Perfect! All tests pass. Let me create a summary of the fix:

## Summary

I successfully diagnosed and fixed the test failures in the merge-verify-fix task. The root cause was that the old watchdog hook installation mechanism in `pinGeometryOptionsLocked` was still being called and installing hooks at the wrong time in the sequence, conflicting with the new resize-pin hook mechanism introduced by the merge.

### Changes Made

**File: `/home/knatte/Code/loomyard/wts/reed-watchdog-daemon/internal/reedengine/windowsize.go`**
- Removed the old watchdog hook installation code that was executing when `watchdog: on`
- Removed the unused `shell` import that was only used by the old hook installation code
- Restructured the logic so that `pinGeometryOptionsLocked` now only:
  - Pins the geometry options (`status off`, `window-size latest`)
  - Clears any existing hooks when `watchdog: off`
- The new resize-pin hook installation is now exclusively handled by `installResizePinsLocked`, called from `apply.go` and `attach.go`

**File: `/home/knatte/Code/loomyard/wts/reed-watchdog-daemon/internal/reedengine/windowsize_test.go`**
- Removed unused `shell` import
- Updated `TestPinGeometryOptionsLocked_HookLifecycle/WatchdogOnInstallsAPlainReplacingHook` to reflect the new behavior where `pinGeometryOptionsLocked` does NOT install hooks anymore
- Renamed test to `WatchdogOnPinsGeometryOptionsOnly` and updated it to verify that only geometry options are pinned
- Updated `SetHookErrorIsNonFatal` test to `SetHookErrorIsNonFatalWhenWatchdogOff` to test the watchdog-off path which still uses hooks

### Test Results

All tests now pass:
- go test ./internal/shell/... ./internal/reedengine/...
- TestAttachArgv_InstallsResizePinsAfterStateAndPanesRead - hooks are now installed after list-panes as expected
- TestAttachArgv_DegradedPathsInstallNoResizePinHook - degraded paths no longer install hooks
- All other existing tests continue to pass

The fix ensures that the new resize-pin hook mechanism is the sole owner of hook installation, while `pinGeometryOptionsLocked` remains responsible only for geometry option pinning.

{"status":"success"}
