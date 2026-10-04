// bodyfile_test.go covers applyBodyFile with temp files and in-memory stdin only.

package boardcli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeBodyFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "body.md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write body file: %v", err)
	}
	return path
}

func TestApplyBodyFile_File(t *testing.T) {
	fields := map[string]any{"slug": "s"}
	if err := applyBodyFile(fields, writeBodyFile(t, "# Body\n\n\"quoted\"\n"), "{}", strings.NewReader("")); err != nil {
		t.Fatalf("applyBodyFile: %v", err)
	}
	if fields["body"] != "# Body\n\n\"quoted\"\n" {
		t.Fatalf("body = %q", fields["body"])
	}
}

func TestApplyBodyFile_Stdin(t *testing.T) {
	fields := map[string]any{"slug": "s"}
	if err := applyBodyFile(fields, "-", "{}", strings.NewReader("from stdin")); err != nil {
		t.Fatalf("applyBodyFile: %v", err)
	}
	if fields["body"] != "from stdin" {
		t.Fatalf("body = %q", fields["body"])
	}
}

func TestApplyBodyFile_EmptyFile(t *testing.T) {
	fields := map[string]any{"slug": "s"}
	if err := applyBodyFile(fields, writeBodyFile(t, ""), "{}", strings.NewReader("")); err != nil {
		t.Fatalf("applyBodyFile: %v", err)
	}
	if body, ok := fields["body"]; !ok || body != "" {
		t.Fatalf("body = %#v, want empty string present", body)
	}
}

func TestApplyBodyFile_MissingFileNamesPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "absent.md")
	err := applyBodyFile(map[string]any{}, path, "{}", strings.NewReader(""))
	if err == nil || !strings.Contains(err.Error(), path) {
		t.Fatalf("err = %v, want one naming %s", err, path)
	}
}

func TestApplyBodyFile_BodyInPayloadRefused(t *testing.T) {
	fields := map[string]any{"slug": "s", "body": "x"}
	err := applyBodyFile(fields, writeBodyFile(t, "y"), "{}", strings.NewReader(""))
	if err == nil || !strings.Contains(err.Error(), "drop") {
		t.Fatalf("err = %v, want a refusal naming the way forward", err)
	}
	if fields["body"] != "x" {
		t.Fatalf("body overwritten: %q", fields["body"])
	}
}

func TestApplyBodyFile_StdinForBothRefused(t *testing.T) {
	err := applyBodyFile(map[string]any{}, "-", "-", strings.NewReader("x"))
	if err == nil || !strings.Contains(err.Error(), "stdin") {
		t.Fatalf("err = %v, want a stdin refusal", err)
	}
}
