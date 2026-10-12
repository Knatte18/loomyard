// records_test.go — untagged Tier-1 tests for the attempt records, the verdict schema and the decision snapshot.

package alignment

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// writeFile writes content at path, creating its directory.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v; want nil", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) = %v; want nil", path, err)
	}
}

// TestReadVerdict verifies the three verdict words are accepted with their fields and that each invalid verdict file, and a missing or empty report, is refused.
func TestReadVerdict(t *testing.T) {
	t.Parallel()
	const report = "# Report\n"
	tests := []struct {
		name      string
		verdict   string
		report    string
		noReport  bool
		want      Verdict
		wantFound bool
		wantErrIn string
	}{
		{name: "an approve", verdict: "verdict: approve\nreason: in scope\n", report: report, want: Verdict{Verdict: VerdictApprove, Reason: "in scope"}, wantFound: true},
		{name: "an approve superseding entries", verdict: "verdict: approve\nreason: lands the entry\nsuperseded: [old-a, old-b]\n", report: report, want: Verdict{Verdict: VerdictApprove, Reason: "lands the entry", Superseded: []string{"old-a", "old-b"}}, wantFound: true},
		{name: "a reject", verdict: "verdict: reject\nreason: leaves out a Done-when point\n", report: report, want: Verdict{Verdict: VerdictReject, Reason: "leaves out a Done-when point"}, wantFound: true},
		{name: "an overlap escalate with its question", verdict: "verdict: escalate\nreason: overlap\nquestion: peer-x also changes internal/alignment; who owns it?\n", report: report, want: Verdict{Verdict: VerdictEscalate, Reason: "overlap", Question: "peer-x also changes internal/alignment; who owns it?"}, wantFound: true},
		{name: "an absent verdict file", wantFound: false},
		{name: "a file that does not parse", verdict: "verdict: [approve\n", report: report, wantErrIn: "parse verdict"},
		{name: "an unknown key", verdict: "verdict: approve\nreason: ok\nnote: extra\n", report: report, wantErrIn: "parse verdict"},
		{name: "an unknown verdict word", verdict: "verdict: maybe\nreason: ok\n", report: report, wantErrIn: `verdict "maybe"`},
		{name: "an empty reason", verdict: "verdict: approve\nreason: \"  \"\n", report: report, wantErrIn: "reason is empty"},
		{name: "an escalate with no question", verdict: "verdict: escalate\nreason: unsure\n", report: report, wantErrIn: "needs a question"},
		{name: "superseded on a reject", verdict: "verdict: reject\nreason: no\nsuperseded: [old-a]\n", report: report, wantErrIn: "only an approve supersedes"},
		{name: "superseded on an escalate", verdict: "verdict: escalate\nreason: no\nquestion: why\nsuperseded: [old-a]\n", report: report, wantErrIn: "only an approve supersedes"},
		{name: "a missing report", verdict: "verdict: approve\nreason: ok\n", noReport: true, wantErrIn: "read the report"},
		{name: "an empty report", verdict: "verdict: approve\nreason: ok\n", report: " \n", wantErrIn: "is empty"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			a := Attempt{Dir: t.TempDir(), Token: "tok"}
			if tt.verdict != "" {
				writeFile(t, a.VerdictPath(), tt.verdict)
			}
			if !tt.noReport && tt.report != "" {
				writeFile(t, a.ReportPath(), tt.report)
			}

			got, found, err := ReadVerdict(a)
			if tt.wantErrIn != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErrIn) {
					t.Fatalf("ReadVerdict() error = %v; want it to contain %q", err, tt.wantErrIn)
				}
				return
			}
			if err != nil {
				t.Fatalf("ReadVerdict() error = %v; want nil", err)
			}
			if found != tt.wantFound || !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ReadVerdict() = %+v, %v; want %+v, %v", got, found, tt.want, tt.wantFound)
			}
		})
	}
}

// TestAttempts verifies the attempt record round trip with its token-named paths, the listing order, an absent root, a directory without a record and an unparseable record.
func TestAttempts(t *testing.T) {
	t.Parallel()

	t.Run("a written attempt reads back with its directory and token paths", func(t *testing.T) {
		t.Parallel()
		dir := filepath.Join(t.TempDir(), "attempt-1")
		spawned := time.Date(2026, 3, 3, 12, 0, 0, 0, time.UTC)
		written := Attempt{Number: 1, Key: "round-1", Point: PointParentReview, Token: "abc123", SpawnedAt: spawned, Deadline: spawned.Add(20 * time.Minute), StrandGUID: "guid", Snapshot: "snap", PRNumber: 7, HeadSHA: "deadbeef", Applied: VerdictApprove, Notified: true, Dir: dir}
		if err := WriteAttempt(written); err != nil {
			t.Fatalf("WriteAttempt() error = %v; want nil", err)
		}

		got, err := ReadAttempt(dir)
		if err != nil {
			t.Fatalf("ReadAttempt() error = %v; want nil", err)
		}
		if !reflect.DeepEqual(got, written) {
			t.Errorf("ReadAttempt() = %+v; want %+v", got, written)
		}
		if got.VerdictPath() != filepath.Join(dir, "verdict-abc123.yaml") || got.ReportPath() != filepath.Join(dir, "report-abc123.md") || got.PromptPath() != filepath.Join(dir, "prompt.md") {
			t.Errorf("paths = %q, %q, %q; want the token-named verdict and report and prompt.md", got.VerdictPath(), got.ReportPath(), got.PromptPath())
		}
		if _, err := ReadAttempt(t.TempDir()); err == nil {
			t.Errorf("ReadAttempt(empty dir) error = nil; want an error")
		}
	})

	t.Run("listing sorts by number, skips a directory without a record and lists none for an absent root", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		for _, n := range []string{"3", "1", "2"} {
			number := int(n[0] - '0')
			if err := WriteAttempt(Attempt{Number: number, Dir: filepath.Join(root, "attempt-"+n)}); err != nil {
				t.Fatalf("WriteAttempt(%s) error = %v; want nil", n, err)
			}
		}
		if err := os.MkdirAll(filepath.Join(root, "stray"), 0o755); err != nil {
			t.Fatal(err)
		}

		attempts, err := ListAttempts(root)
		if err != nil {
			t.Fatalf("ListAttempts() error = %v; want nil", err)
		}
		var numbers []int
		for _, a := range attempts {
			numbers = append(numbers, a.Number)
		}
		if want := []int{1, 2, 3}; !reflect.DeepEqual(numbers, want) {
			t.Errorf("ListAttempts() numbers = %v; want %v", numbers, want)
		}

		none, err := ListAttempts(filepath.Join(root, "absent"))
		if err != nil || len(none) != 0 {
			t.Errorf("ListAttempts(absent) = %v, %v; want none and no error", none, err)
		}
	})

	t.Run("listing refuses an unparseable record, naming it", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		writeFile(t, filepath.Join(root, "attempt-1", "attempt.json"), "{not json")

		_, err := ListAttempts(root)
		if err == nil || !strings.Contains(err.Error(), filepath.Join(root, "attempt-1", "attempt.json")) {
			t.Errorf("ListAttempts() error = %v; want it to name the record", err)
		}
	})

	t.Run("ParseAttemptDir reads tolerantly and names the failing file", func(t *testing.T) {
		t.Parallel()
		const attemptJSON = `{"number":2,"point":"pr-gate","token":"tok","applied":"approve","pr_number":9,"extra":1}`
		tests := []struct {
			name        string
			files       map[string][]byte
			want        Attempt
			wantVerdict *Verdict
			wantErr     string
		}{
			{
				name: "approve verdict with superseded slugs and an unknown key",
				files: map[string][]byte{
					"attempt.json":     []byte(attemptJSON),
					"verdict-tok.yaml": []byte("verdict: approve\nreason: ok\nsuperseded: [a, b]\nextra: x\n"),
					"prompt.md":        []byte("ignored"),
				},
				want:        Attempt{Number: 2, Point: PointPRGate, Token: "tok", Applied: VerdictApprove, PRNumber: 9},
				wantVerdict: &Verdict{Verdict: VerdictApprove, Reason: "ok", Superseded: []string{"a", "b"}},
			},
			{
				name:  "no verdict file",
				files: map[string][]byte{"attempt.json": []byte(attemptJSON)},
				want:  Attempt{Number: 2, Point: PointPRGate, Token: "tok", Applied: VerdictApprove, PRNumber: 9},
			},
			{
				name: "escalate verdict without a question",
				files: map[string][]byte{
					"attempt.json":     []byte(attemptJSON),
					"verdict-tok.yaml": []byte("verdict: escalate\nreason: unsure\n"),
				},
				want:        Attempt{Number: 2, Point: PointPRGate, Token: "tok", Applied: VerdictApprove, PRNumber: 9},
				wantVerdict: &Verdict{Verdict: VerdictEscalate, Reason: "unsure"},
			},
			{name: "undecodable attempt.json", files: map[string][]byte{"attempt.json": []byte("{")}, wantErr: "attempt.json"},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				attempt, verdict, err := ParseAttemptDir(tc.files)
				if tc.wantErr != "" {
					if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
						t.Fatalf("ParseAttemptDir() error = %v; want one naming %s", err, tc.wantErr)
					}
					return
				}
				if err != nil {
					t.Fatalf("ParseAttemptDir() error = %v; want nil", err)
				}
				if !reflect.DeepEqual(attempt, tc.want) || !reflect.DeepEqual(verdict, tc.wantVerdict) {
					t.Errorf("ParseAttemptDir() = %+v, %+v; want %+v, %+v", attempt, verdict, tc.want, tc.wantVerdict)
				}
			})
		}
	})
}

// TestSnapshot verifies equal input hashes equal and that an altered record, an altered, added or removed note, and a moved note boundary each change the hash.
func TestSnapshot(t *testing.T) {
	t.Parallel()
	base := Snapshot([]byte("record"), []string{"note one", "note two"})
	if again := Snapshot([]byte("record"), []string{"note one", "note two"}); again != base {
		t.Errorf("Snapshot(equal input) = %q; want %q", again, base)
	}
	changed := map[string]string{
		"an altered record":     Snapshot([]byte("record!"), []string{"note one", "note two"}),
		"an altered note":       Snapshot([]byte("record"), []string{"note one", "note 2"}),
		"an added note":         Snapshot([]byte("record"), []string{"note one", "note two", "note three"}),
		"a removed note":        Snapshot([]byte("record"), []string{"note one"}),
		"a reordered note":      Snapshot([]byte("record"), []string{"note two", "note one"}),
		"a moved note boundary": Snapshot([]byte("recordnote one"), []string{"note two"}),
	}
	for name, got := range changed {
		if got == base {
			t.Errorf("Snapshot(%s) = the unchanged hash; want a different one", name)
		}
	}
}
