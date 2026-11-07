// publish_verify_test.go holds writeVerifyLog, the fake-verify log writer the Merge-In and Finalize gate tests share.

package landingshed

import (
	"os"
	"path/filepath"
	"testing"
)

// writeVerifyLog makes each fake verify, passed or failed, write its log to the path it is given.
// It also names that path on the result, as a real verify run does.
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
