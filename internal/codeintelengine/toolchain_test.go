// toolchain_test.go covers resolveGoToolchain's three observable paths —
// already-installed fast path, cold-cache install, and concurrent-install
// serialization — entirely offline: every sub-test swaps installGoToolchain
// for a fake before running and restores the original via t.Cleanup, so no
// sub-test ever spawns a real `go install`. userCacheDir is likewise
// redirected at a t.TempDir() per sub-test, so nothing here touches the
// real machine-global toolchain cache.

package codeintelengine

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// withFakeInstaller swaps installGoToolchain for fake for the duration of
// the calling test and restores the original (runGoInstall) via
// t.Cleanup, mirroring lspclient_test.go's newLSPClientFromRW test-seam
// convention for this package.
func withFakeInstaller(t *testing.T, fake toolchainInstaller) {
	t.Helper()
	original := installGoToolchain
	installGoToolchain = fake
	t.Cleanup(func() { installGoToolchain = original })
}

// withTempUserCacheDir redirects userCacheDir at a fresh t.TempDir() for the
// duration of the calling test and restores the original (os.UserCacheDir)
// via t.Cleanup, so goToolchainCacheDir/goToolchainInstallLock never resolve
// to the real machine-global cache during this package's tests.
func withTempUserCacheDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	original := userCacheDir
	userCacheDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { userCacheDir = original })
	return dir
}

func TestResolveGoToolchain_AlreadyInstalled(t *testing.T) {
	withTempUserCacheDir(t)

	// Counter guarded by its own mutex, independent of the toolchain's own
	// install lock, so the assertion below doesn't depend on the code under
	// test being correct about locking.
	var mu sync.Mutex
	calls := 0
	withFakeInstaller(t, func(ctx context.Context, version, destDir string) error {
		mu.Lock()
		calls++
		mu.Unlock()
		return nil
	})

	const version = "v0.23.0"
	cacheDir := goToolchainCacheDir(version)
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatalf("seed cache dir: %v", err)
	}
	binPath := filepath.Join(cacheDir, "gopls")
	if err := os.WriteFile(binPath, nil, 0o755); err != nil {
		t.Fatalf("seed pre-installed binary: %v", err)
	}

	got, err := resolveGoToolchain(context.Background(), version)
	if err != nil {
		t.Fatalf("resolveGoToolchain() error = %v; want nil", err)
	}
	if got != binPath {
		t.Errorf("resolveGoToolchain() = %q; want %q", got, binPath)
	}

	mu.Lock()
	gotCalls := calls
	mu.Unlock()
	if gotCalls != 0 {
		t.Errorf("fake installer invoked %d times; want 0 (already-installed fast path must not install)", gotCalls)
	}

	if _, err := os.Stat(goToolchainInstallLock()); !os.IsNotExist(err) {
		t.Errorf("goToolchainInstallLock() file exists after the fast path; want no lock file created (err = %v)", err)
	}
}

func TestResolveGoToolchain_ColdCacheInstalls(t *testing.T) {
	withTempUserCacheDir(t)

	var mu sync.Mutex
	calls := 0
	var gotVersion, gotDestDir string
	withFakeInstaller(t, func(ctx context.Context, version, destDir string) error {
		mu.Lock()
		calls++
		gotVersion = version
		gotDestDir = destDir
		mu.Unlock()
		// Actually create the binary the real installer would have produced,
		// so resolveGoToolchain's post-install os.Stat succeeds and the
		// function exercises its real return path rather than a
		// hand-faked string.
		return os.WriteFile(filepath.Join(destDir, "gopls"), nil, 0o755)
	})

	const version = "v0.23.0"
	wantBinPath := filepath.Join(goToolchainCacheDir(version), "gopls")

	got, err := resolveGoToolchain(context.Background(), version)
	if err != nil {
		t.Fatalf("resolveGoToolchain() error = %v; want nil", err)
	}
	if got != wantBinPath {
		t.Errorf("resolveGoToolchain() = %q; want %q", got, wantBinPath)
	}

	mu.Lock()
	gotCalls := calls
	mu.Unlock()
	if gotCalls != 1 {
		t.Fatalf("fake installer invoked %d times; want exactly 1", gotCalls)
	}
	if gotVersion != version {
		t.Errorf("fake installer called with version = %q; want %q", gotVersion, version)
	}
	wantDestDir := goToolchainCacheDir(version)
	if gotDestDir != wantDestDir {
		t.Errorf("fake installer called with destDir = %q; want %q", gotDestDir, wantDestDir)
	}
}

func TestResolveGoToolchain_ConcurrentInstallSerializes(t *testing.T) {
	withTempUserCacheDir(t)

	// Counter guarded by its own mutex — deliberately not the toolchain's
	// own install lock — so this assertion doesn't accidentally depend on
	// the code under test being correct about locking.
	var mu sync.Mutex
	calls := 0
	withFakeInstaller(t, func(ctx context.Context, version, destDir string) error {
		mu.Lock()
		calls++
		mu.Unlock()
		return os.WriteFile(filepath.Join(destDir, "gopls"), nil, 0o755)
	})

	const version = "v0.23.0"

	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = resolveGoToolchain(context.Background(), version)
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("resolveGoToolchain() goroutine %d error = %v; want nil", i, err)
		}
	}

	mu.Lock()
	gotCalls := calls
	mu.Unlock()
	// Exactly one goroutine must win the race to install; the other must
	// hit the double-check-after-lock branch and skip installing entirely.
	if gotCalls != 1 {
		t.Errorf("fake installer invoked %d times across two concurrent callers; want exactly 1", gotCalls)
	}
}
