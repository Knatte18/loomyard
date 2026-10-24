//go:build tier0

// publish_verify_test.go holds writeVerifyLog, the fake-verify log writer the Merge-In and Finalize gate tests share.

package landingshed

import (
	"os"
	"path/filepath"
	"testing"
)

// writeVerifyLog makes every passed or failed fake verify write log to its own log path and name it on its result, as a real verify run does.
func writeVerifyLog(t *testing.T, gate *gateFixture, log string) {
	t.Helper()
	gate.fake.writeLog = func(path string) {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Error(err)
			return
		}
		if err := os.WriteFile(path, []byte(log), 0o644); err != nil {
			t.Error(err)
		}
	}
}
