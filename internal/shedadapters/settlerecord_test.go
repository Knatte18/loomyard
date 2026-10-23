package shedadapters

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

var settleTestInstant = time.Date(2026, 8, 16, 15, 13, 26, 0, time.UTC)

func documentSettleRecord(generation, round int) settleRecord {
	return settleRecord{
		Subject:    Subject{Kind: SubjectDocument, Hash: "hash-1"},
		Generation: generation,
		Round:      round,
		SettledAt:  settleTestInstant.Format(time.RFC3339),
		Settled:    true,
	}
}

func writeRecordFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", dir, err)
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile(%s): %v", name, err)
		}
	}
}

func TestSettleLive(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		setup      func(t *testing.T, runDir string)
		wantLive   bool
		wantAt     time.Time
		wantErrNil bool
	}{
		{
			name:       "no record is not live",
			setup:      func(t *testing.T, runDir string) {},
			wantErrNil: true,
		},
		{
			name: "a record naming the current generation with no later report is live",
			setup: func(t *testing.T, runDir string) {
				if err := writeSettleRecord(runDir, documentSettleRecord(2, 1), nil); err != nil {
					t.Fatal(err)
				}
				if err := writeGenerationRecord(runDir, generationRecord{Number: 2}); err != nil {
					t.Fatal(err)
				}
				writeRecordFiles(t, runDir, map[string]string{"round-1-review.md": "review"})
			},
			wantLive:   true,
			wantAt:     settleTestInstant,
			wantErrNil: true,
		},
		{
			name: "a later round's report is not live",
			setup: func(t *testing.T, runDir string) {
				if err := writeSettleRecord(runDir, documentSettleRecord(1, 1), nil); err != nil {
					t.Fatal(err)
				}
				writeRecordFiles(t, runDir, map[string]string{"round-1-review.md": "review", "round-2-review.md": "review"})
			},
			wantErrNil: true,
		},
		{
			name: "a generation record past the record's generation is not live",
			setup: func(t *testing.T, runDir string) {
				if err := writeSettleRecord(runDir, documentSettleRecord(1, 1), nil); err != nil {
					t.Fatal(err)
				}
				if err := writeGenerationRecord(runDir, generationRecord{Number: 2}); err != nil {
					t.Fatal(err)
				}
			},
			wantErrNil: true,
		},
		{
			name: "a record with no generation record is generation 1 and live",
			setup: func(t *testing.T, runDir string) {
				if err := writeSettleRecord(runDir, documentSettleRecord(1, 3), nil); err != nil {
					t.Fatal(err)
				}
			},
			wantLive:   true,
			wantAt:     settleTestInstant,
			wantErrNil: true,
		},
		{
			name: "a record file holding an unknown key decodes and is live",
			setup: func(t *testing.T, runDir string) {
				writeRecordFiles(t, runDir, map[string]string{
					settleRecordFile: `{"subject":{"kind":"document","hash":"h","future":1},"generation":1,"round":1,"settled_at":"2026-08-16T15:13:26Z","settled":true,"future":"x"}`,
				})
			},
			wantLive:   true,
			wantAt:     settleTestInstant,
			wantErrNil: true,
		},
		{
			name: "a record file with none of the generation keys decodes as generation 1",
			setup: func(t *testing.T, runDir string) {
				writeRecordFiles(t, runDir, map[string]string{
					settleRecordFile: `{"subject":{"kind":"document","hash":"h"},"round":1,"settled_at":"2026-08-16T15:13:26Z","settled":true}`,
				})
			},
			wantLive:   true,
			wantAt:     settleTestInstant,
			wantErrNil: true,
		},
		{
			name: "an unparseable settled_at is an error",
			setup: func(t *testing.T, runDir string) {
				rec := documentSettleRecord(1, 1)
				rec.SettledAt = "yesterday"
				if err := writeSettleRecord(runDir, rec, nil); err != nil {
					t.Fatal(err)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			runDir := filepath.Join(t.TempDir(), "run")
			writeRecordFiles(t, runDir, nil)
			tt.setup(t, runDir)

			settledAt, live, err := SettleLive(runDir)
			if (err == nil) != tt.wantErrNil {
				t.Fatalf("SettleLive() error = %v; want nil error: %t", err, tt.wantErrNil)
			}
			if live != tt.wantLive {
				t.Errorf("SettleLive() live = %t; want %t", live, tt.wantLive)
			}
			if !settledAt.Equal(tt.wantAt) {
				t.Errorf("SettleLive() settledAt = %v; want %v", settledAt, tt.wantAt)
			}
		})
	}
}

func TestSubject_Unchanged(t *testing.T) {
	t.Parallel()
	codeSettled := Subject{Kind: SubjectCode, MergeBase: "base-1", Head: "head-1", FullHash: "full-1", PatchID: "patch-1"}
	mergeInOnly := func(answer bool) func(string) (bool, error) {
		return func(since string) (bool, error) {
			if since != "head-1" {
				t.Errorf("mergeInOnly(%q); want the settled head head-1", since)
			}
			return answer, nil
		}
	}
	failing := errors.New("history unreadable")

	tests := []struct {
		name        string
		live        Subject
		settled     Subject
		mergeInOnly func(string) (bool, error)
		want        bool
		wantErr     error
	}{
		{"DocumentEqualHash", Subject{Kind: SubjectDocument, Hash: "a"}, Subject{Kind: SubjectDocument, Hash: "a"}, nil, true, nil},
		{"DocumentChangedHash", Subject{Kind: SubjectDocument, Hash: "b"}, Subject{Kind: SubjectDocument, Hash: "a"}, nil, false, nil},
		{"MergeEqualHash", Subject{Kind: SubjectMerge, Hash: "a"}, Subject{Kind: SubjectMerge, Hash: "a"}, nil, true, nil},
		{"KindChanged", Subject{Kind: SubjectMerge, Hash: "a"}, Subject{Kind: SubjectDocument, Hash: "a"}, nil, false, nil},
		{"CodeSameBaseEqualFullHash", Subject{Kind: SubjectCode, MergeBase: "base-1", FullHash: "full-1"}, codeSettled, nil, true, nil},
		{"CodeSameBaseUnequalFullHash", Subject{Kind: SubjectCode, MergeBase: "base-1", FullHash: "full-2"}, codeSettled, nil, false, nil},
		{"CodeMovedBaseMergeInOnlyEqualPatchID", Subject{Kind: SubjectCode, MergeBase: "base-2", PatchID: "patch-1"}, codeSettled, mergeInOnly(true), true, nil},
		{"CodeMovedBaseHistoryNotMergeInOnly", Subject{Kind: SubjectCode, MergeBase: "base-2", PatchID: "patch-1"}, codeSettled, mergeInOnly(false), false, nil},
		{"CodeMovedBaseDifferentPatchID", Subject{Kind: SubjectCode, MergeBase: "base-2", PatchID: "patch-2"}, codeSettled, mergeInOnly(true), false, nil},
		{"CodeMovedBaseWithoutSeam", Subject{Kind: SubjectCode, MergeBase: "base-2", PatchID: "patch-1"}, codeSettled, nil, false, nil},
		{
			"MergeInOnlyError", Subject{Kind: SubjectCode, MergeBase: "base-2", PatchID: "patch-1"}, codeSettled,
			func(string) (bool, error) { return false, failing }, false, failing,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := tt.live.Unchanged(tt.settled, tt.mergeInOnly)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Unchanged() error = %v; want %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("Unchanged() = %t; want %t", got, tt.want)
			}
		})
	}
}

func TestCarryRecords(t *testing.T) {
	t.Parallel()
	source := map[string]string{
		settleRecordFile:     "source settle",
		settlePatchFile:      "source patch",
		generationRecordFile: "source generation",
	}
	tests := []struct {
		name         string
		source       map[string]string
		target       map[string]string
		wantTarget   map[string]string
		absentSource bool
	}{
		{
			name:   "a target holding a generation record keeps all its files",
			source: source,
			target: map[string]string{
				generationRecordFile: "target generation",
				settleRecordFile:     "target settle",
			},
			wantTarget: map[string]string{
				generationRecordFile: "target generation",
				settleRecordFile:     "target settle",
			},
		},
		{
			name:   "a target holding only the settle record and patch of a part-way carry gets all three source files",
			source: source,
			target: map[string]string{
				settleRecordFile: "partial settle",
				settlePatchFile:  "partial patch",
			},
			wantTarget: source,
		},
		{
			name:         "an absent source directory leaves the target as it was",
			absentSource: true,
			target:       map[string]string{"round-1-review.md": "review"},
			wantTarget:   map[string]string{"round-1-review.md": "review"},
		},
		{
			name:       "an absent source file is skipped",
			source:     map[string]string{settleRecordFile: "source settle"},
			target:     map[string]string{},
			wantTarget: map[string]string{settleRecordFile: "source settle"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			parent := t.TempDir()
			fromDir, toDir := filepath.Join(parent, "from"), filepath.Join(parent, "to")
			if !tt.absentSource {
				writeRecordFiles(t, fromDir, tt.source)
			}
			writeRecordFiles(t, toDir, tt.target)

			if err := CarryRecords(fromDir, toDir); err != nil {
				t.Fatalf("CarryRecords() = %v; want nil", err)
			}
			for _, name := range []string{settleRecordFile, settlePatchFile, generationRecordFile, "round-1-review.md"} {
				want, wantPresent := tt.wantTarget[name]
				got, err := os.ReadFile(filepath.Join(toDir, name))
				if present := err == nil; present != wantPresent {
					t.Errorf("target %s present = %t; want %t", name, present, wantPresent)
					continue
				}
				if string(got) != want {
					t.Errorf("target %s = %q; want %q", name, got, want)
				}
			}
		})
	}
}

func TestSettleRoundStart(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 10, 23, 7, 30, 15, 0, time.UTC)
	withStart := func(startedAt string) settleRecord {
		record := documentSettleRecord(1, 1)
		record.RoundStartedAt = startedAt
		return record
	}
	tests := []struct {
		name      string
		setup     func(t *testing.T, runDir string)
		want      time.Time
		wantFound bool
	}{
		{
			name: "a live record with a whole-second start reads it back",
			setup: func(t *testing.T, runDir string) {
				if err := writeSettleRecord(runDir, withStart(start.Format(time.RFC3339)), nil); err != nil {
					t.Fatal(err)
				}
			},
			want:      start,
			wantFound: true,
		},
		{
			name: "a live record without the field reads as unknown",
			setup: func(t *testing.T, runDir string) {
				if err := writeSettleRecord(runDir, withStart(""), nil); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "an older record with no field in its file reads as unknown",
			setup: func(t *testing.T, runDir string) {
				writeRecordFiles(t, runDir, map[string]string{settleRecordFile: `{"subject":{"kind":"document","hash":"h"},"round":1,"settled_at":"2026-08-16T15:13:26Z","settled":true}`})
			},
		},
		{
			name: "a record that is not live reads as unknown",
			setup: func(t *testing.T, runDir string) {
				if err := writeSettleRecord(runDir, withStart(start.Format(time.RFC3339)), nil); err != nil {
					t.Fatal(err)
				}
				writeRecordFiles(t, runDir, map[string]string{"round-2-review.md": "review"})
			},
		},
		{
			name:  "no record reads as unknown",
			setup: func(t *testing.T, runDir string) {},
		},
		{
			name: "an unparsable start reads as unknown",
			setup: func(t *testing.T, runDir string) {
				if err := writeSettleRecord(runDir, withStart("yesterday"), nil); err != nil {
					t.Fatal(err)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			runDir := t.TempDir()
			tt.setup(t, runDir)
			got, found, err := SettleRoundStart(runDir)
			if err != nil {
				t.Fatalf("SettleRoundStart() error = %v; want nil", err)
			}
			if found != tt.wantFound || !got.Equal(tt.want) {
				t.Errorf("SettleRoundStart() = %v, %t; want %v, %t", got, found, tt.want, tt.wantFound)
			}
		})
	}
}
