//go:build !windows

// resume_unix_test.go holds the resume checks that need a stat error other than not-exist.
// A path through a file gives that error only off Windows.

package claudeengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestCheckResumable_UnstattableTranscriptRefusesNamingTheStatFailure pins refusal on a stat error other than not-exist.
// It words the refusal as a stat failure, not a missing transcript.
func TestCheckResumable_UnstattableTranscriptRefusesNamingTheStatFailure(t *testing.T) {
	tiergate.Require(t, 0)
	_, registryDir := resumeFixture(t, false)
	projectFile := filepath.Join(t.TempDir(), "project")
	if err := os.WriteFile(projectFile, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := checkResumable(resumeTestID, projectFile, registryDir, func(int) bool { return false }, func(int) (string, bool) { return "", false })
	if err == nil || !strings.Contains(err.Error(), "cannot stat the transcript") {
		t.Fatalf("err = %v, want a refusal naming the stat failure", err)
	}
}
