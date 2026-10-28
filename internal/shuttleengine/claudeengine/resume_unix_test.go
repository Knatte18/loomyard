//go:build tier0 && !windows

// resume_unix_test.go holds the resume checks that need a stat error other than not-exist, which a path through a file gives only off Windows.

package claudeengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCheckResumable_UnstattableTranscriptRefusesNamingTheStatFailure pins that a stat error other than not-exist still refuses, worded as a stat failure rather than a missing transcript.
func TestCheckResumable_UnstattableTranscriptRefusesNamingTheStatFailure(t *testing.T) {
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
