// recordupsert_test.go — tests of Board.UpsertTaskWithRecord.
// They cover the refusals that write nothing and the replacement of the stored record with the payload.
// They also cover the family project exemption.

package boardengine

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// formerRecord is the decision record the seeded entries of these tests store.
const formerRecord = "former record\n"

// sha256Hex returns the hex sha256 of body, the stamp a write of a decision record leaves on its entry.
func sha256Hex(body string) string {
	sum := sha256.Sum256([]byte(body))
	return hex.EncodeToString(sum[:])
}

// newRecordUpsertBoards returns two boards over one directory, and the directory.
// seeder acts as the operator; actor carries strandName and pairExists.
func newRecordUpsertBoards(t *testing.T, strandName string, pairExists func(slug string) bool) (seeder, actor *Board, boardPath string) {
	t.Helper()
	boardPath = t.TempDir()
	types := []Label{{Name: SketchType}, {Name: "bug"}, {Name: "enhancement"}}
	config := Config{Path: boardPath, Readme: "Home.md", DesignPrefix: "proposal-", Types: types, Labels: []Label{{Name: "board"}}, SkipGit: true}
	seeder = New(config)
	config.StrandName, config.PairExists = strandName, pairExists
	return seeder, New(config), boardPath
}

// seedDiscussedEntry files the entry slug of kind with labels, body "former body" and a stored record of formerRecord.
func seedDiscussedEntry(t *testing.T, b *Board, slug, kind string, labels []string) {
	t.Helper()
	upsertForRaddle(t, b, map[string]any{"slug": slug, "title": "Former title", "kind": kind, "labels": labels, "body": "former body"})
	if err := b.StoreDecisionRecord(slug, []byte(formerRecord)); err != nil {
		t.Fatalf("StoreDecisionRecord(%q) error = %v; want nil", slug, err)
	}
}

// seedDiscussedSketch files the Sketch slug with a stored record.
func seedDiscussedSketch(t *testing.T, b *Board, slug string) {
	t.Helper()
	seedDiscussedEntry(t, b, slug, KindNote, []string{SketchType, "board"})
}

// seedDiscussedTask files the task slug with a stored record.
func seedDiscussedTask(t *testing.T, b *Board, slug string) {
	t.Helper()
	seedDiscussedEntry(t, b, slug, KindTask, []string{"bug", "board"})
}

// probeNeverCalled returns a weftLive that fails t when it is called.
func probeNeverCalled(t *testing.T) func(string) (bool, error) {
	t.Helper()
	return func(slug string) (bool, error) {
		t.Errorf("weftLive(%q) was called; want the refusal or the exemption to come before the probe", slug)
		return false, nil
	}
}

// TestUpsertTaskWithRecord_Refusals pins each refusal of UpsertTaskWithRecord:
// the message names a way forward and the command or step it points at, and nothing is written.
func TestUpsertTaskWithRecord_Refusals(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	done := StatusDone
	fileFamily := func(t *testing.T, b *Board, _ string) {
		t.Helper()
		if err := b.FileRaddle(familyFiling(startArrasRun(t, b, "plan"))); err != nil {
			t.Fatalf("FileRaddle() error = %v; want nil", err)
		}
	}
	seedDiscussedProject := func(t *testing.T, b *Board, _ string) {
		t.Helper()
		seedDiscussedEntry(t, b, "proj", KindProject, []string{"enhancement", "board"})
	}
	tests := []struct {
		name       string
		strandName string
		pairExists func(slug string) bool
		// slug is the entry the payload names; empty means "idea".
		slug  string
		setup func(t *testing.T, b *Board, boardPath string)
		// weftLive is the probe; nil means the refusal comes before the probe, which then fails the test when called.
		weftLive func(slug string) (bool, error)
		wantErr  []string
	}{
		{
			name: "RunAgentSession", strandName: "ly:some-task:webster",
			setup:   func(t *testing.T, b *Board, _ string) { seedDiscussedSketch(t, b, "idea") },
			wantErr: []string{"lym loom decision replace"},
		},
		{
			name: "SlugNotOnTheBoard", slug: "ghost",
			setup:   func(t *testing.T, b *Board, _ string) { seedDiscussedSketch(t, b, "idea") },
			wantErr: []string{`no board entry "ghost"`, "lym board upsert"},
		},
		{
			name: "EntryNotDiscussed",
			setup: func(t *testing.T, b *Board, _ string) {
				upsertForRaddle(t, b, map[string]any{"slug": "idea", "title": "Former title", "kind": KindNote, "labels": []string{SketchType}, "body": "former body"})
			},
			wantErr: []string{"has no stored decision record", "drop --record-file"},
		},
		{
			name: "RecordFileMissing",
			setup: func(t *testing.T, b *Board, boardPath string) {
				seedDiscussedSketch(t, b, "idea")
				if err := os.Remove(filepath.Join(boardPath, DecisionFileName("idea"))); err != nil {
					t.Fatalf("Remove(decision record of idea) = %v; want nil", err)
				}
			},
			wantErr: []string{"has no stored decision record", "drop --record-file"},
		},
		{
			name: "DoneEntry",
			setup: func(t *testing.T, b *Board, _ string) {
				seedDiscussedTask(t, b, "idea")
				if err := b.SetStatus("idea", &done); err != nil {
					t.Fatalf("SetStatus() error = %v; want nil", err)
				}
			},
			wantErr: []string{"is done", "as a new entry"},
		},
		{
			name: "RunHeldEntry",
			setup: func(t *testing.T, b *Board, _ string) {
				seedDiscussedTask(t, b, "idea")
				if err := b.SetRunStatus("idea", RunStatus("running", "Webster")); err != nil {
					t.Fatalf("SetRunStatus() error = %v; want nil", err)
				}
			},
			wantErr: []string{"is held by a run", "lym loom decision replace idea"},
		},
		{
			name: "TaskWorktreeStands", pairExists: func(string) bool { return true },
			setup:   func(t *testing.T, b *Board, _ string) { seedDiscussedSketch(t, b, "idea") },
			wantErr: []string{"has a task worktree in the hub", "lym batten cancel idea"},
		},
		{
			name:     "WeftBranchLive",
			setup:    func(t *testing.T, b *Board, _ string) { seedDiscussedSketch(t, b, "idea") },
			weftLive: func(string) (bool, error) { return true, nil },
			wantErr:  []string{"is live", "wait for the pair's teardown"},
		},
		{
			name:     "WeftProbeFails",
			setup:    func(t *testing.T, b *Board, _ string) { seedDiscussedSketch(t, b, "idea") },
			weftLive: func(string) (bool, error) { return false, errors.New("origin unreachable") },
			wantErr:  []string{"origin unreachable", "resolve the cause above, then re-run the upsert"},
		},
		{
			name: "FiledPartWorktreeStands", slug: "plan-a", pairExists: func(string) bool { return true },
			setup:   fileFamily,
			wantErr: []string{"has a task worktree in the hub", "lym batten cancel plan-a"},
		},
		{
			name: "UnfiledProjectWorktreeStands", slug: "proj", pairExists: func(string) bool { return true },
			setup:   seedDiscussedProject,
			wantErr: []string{"has a task worktree in the hub", "lym batten cancel proj"},
		},
		{
			// The store records the project "proj" as filed by a run whose family project is "other".
			name: "FiledProjectOfAnotherFamilyWorktreeStands", slug: "proj", pairExists: func(string) bool { return true },
			setup: func(t *testing.T, b *Board, boardPath string) {
				seedDiscussedProject(t, b, boardPath)
				storeDir := filepath.Join(boardPath, raddleStoreDirName, "proj")
				if err := os.MkdirAll(storeDir, 0o755); err != nil {
					t.Fatalf("MkdirAll(%q) = %v; want nil", storeDir, err)
				}
				storeJSON := `{"created":"2026-10-01T08:00:00Z","hash":"h","filed":["proj","other"],"project":"other"}`
				if err := os.WriteFile(filepath.Join(storeDir, raddleStoreFileName), []byte(storeJSON), 0o644); err != nil {
					t.Fatalf("WriteFile(store.json) = %v; want nil", err)
				}
			},
			wantErr: []string{"has a task worktree in the hub", "lym batten cancel proj"},
		},
		{
			name: "UnfiledProjectWeftLive", slug: "proj",
			setup:    seedDiscussedProject,
			weftLive: func(string) (bool, error) { return true, nil },
			wantErr:  []string{"is live", "wait for the pair's teardown"},
		},
		{
			name: "FamilyProjectHeldByARun", slug: "plan", pairExists: func(string) bool { return true },
			setup: func(t *testing.T, b *Board, boardPath string) {
				fileFamily(t, b, boardPath)
				if err := b.SetRunStatus("plan", RunStatus("running", "Webster")); err != nil {
					t.Fatalf("SetRunStatus() error = %v; want nil", err)
				}
			},
			wantErr: []string{"is held by a run", "lym loom decision replace plan"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			seeder, actor, boardPath := newRecordUpsertBoards(t, tt.strandName, tt.pairExists)
			tt.setup(t, seeder, boardPath)
			slug := cmp.Or(tt.slug, "idea")
			recordPath := filepath.Join(boardPath, DecisionFileName(slug))
			recordBefore, recordErrBefore := os.ReadFile(recordPath)
			entryBefore, foundBefore, _ := seeder.GetTask(slug)
			weftLive := tt.weftLive
			if weftLive == nil {
				weftLive = probeNeverCalled(t)
			}

			_, _, err := actor.UpsertTaskWithRecord(map[string]any{"slug": slug, "title": "New title", "body": "new body"}, []byte("new record\n"), weftLive)

			if err == nil {
				t.Fatalf("UpsertTaskWithRecord() error = nil; want a refusal containing %v", tt.wantErr)
			}
			for _, want := range append([]string{"way forward"}, tt.wantErr...) {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("UpsertTaskWithRecord() error = %q; want it to contain %q", err, want)
				}
			}
			recordAfter, recordErrAfter := os.ReadFile(recordPath)
			if string(recordAfter) != string(recordBefore) || (recordErrAfter == nil) != (recordErrBefore == nil) {
				t.Errorf("decision record of %q = %q, %v after the refusal; want %q, %v as before", slug, recordAfter, recordErrAfter, recordBefore, recordErrBefore)
			}
			entryAfter, foundAfter, _ := actor.GetTask(slug)
			if foundAfter != foundBefore || !reflect.DeepEqual(entryAfter, entryBefore) {
				t.Errorf("entry %q = %+v, found %t after the refusal; want %+v, found %t as before", slug, entryAfter, foundAfter, entryBefore, foundBefore)
			}
		})
	}
}

// TestUpsertTaskWithRecord_ReplacesRecordWithPayload pins the one write:
// the payload lands on the entry, the record file holds the new bytes, and the stamp is the digest of the body left.
func TestUpsertTaskWithRecord_ReplacesRecordWithPayload(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	seeder, actor, boardPath := newRecordUpsertBoards(t, "", nil)
	seedDiscussedSketch(t, seeder, "idea")
	var probed []string
	weftLive := func(slug string) (bool, error) {
		probed = append(probed, slug)
		return false, nil
	}

	task, siblings, err := actor.UpsertTaskWithRecord(map[string]any{"slug": "idea", "title": "New title", "body": "new body"}, []byte("new record\n"), weftLive)

	if err != nil {
		t.Fatalf("UpsertTaskWithRecord() error = %v; want nil", err)
	}
	if task.Slug != "idea" || task.Title != "New title" || len(siblings) != 0 {
		t.Errorf("UpsertTaskWithRecord() = slug %q, title %q, %d siblings; want idea, New title, none", task.Slug, task.Title, len(siblings))
	}
	if !reflect.DeepEqual(probed, []string{"idea"}) {
		t.Errorf("weftLive called for %v; want [idea]", probed)
	}
	if got, err := os.ReadFile(filepath.Join(boardPath, DecisionFileName("idea"))); err != nil || string(got) != "new record\n" {
		t.Errorf("decision record of idea = %q, %v; want %q", got, err, "new record\n")
	}
	entry := entryForRaddle(t, actor, "idea")
	if entry.Title != "New title" || entry.Body != "new body" || !entry.Discussed || entry.Kind != KindNote {
		t.Errorf("entry title, body, discussed, kind = %q, %q, %t, %q; want New title, new body, true, note", entry.Title, entry.Body, entry.Discussed, entry.Kind)
	}
	if want := sha256Hex("new body"); entry.RecordBodySHA256 != want {
		t.Errorf("entry record_body_sha256 = %q; want %q, the digest of the new body", entry.RecordBodySHA256, want)
	}
}

// TestUpsertTaskWithRecord_FamilyProjectSkipsWorktreeAndWeftRefusals pins the exemption:
// a family project whose raddle filing the store records is replaced although its worktree stands, with no weft probe.
//
//testtiming:keep pins the family-project exemption from the worktree and live-weft refusals, which TestFileRaddle and the other upsert tests never reach
func TestUpsertTaskWithRecord_FamilyProjectSkipsWorktreeAndWeftRefusals(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	seeder, actor, boardPath := newRecordUpsertBoards(t, "", func(string) bool { return true })
	run := startArrasRun(t, seeder, "plan")
	if err := seeder.FileRaddle(familyFiling(run)); err != nil {
		t.Fatalf("FileRaddle() error = %v; want nil", err)
	}

	_, _, err := actor.UpsertTaskWithRecord(map[string]any{"slug": "plan", "title": "Renamed plan"}, []byte("new project record\n"), probeNeverCalled(t))

	if err != nil {
		t.Fatalf("UpsertTaskWithRecord() error = %v; want nil", err)
	}
	if got, err := os.ReadFile(filepath.Join(boardPath, DecisionFileName("plan"))); err != nil || string(got) != "new project record\n" {
		t.Errorf("decision record of plan = %q, %v; want %q", got, err, "new project record\n")
	}
	project := entryForRaddle(t, actor, "plan")
	if project.Kind != KindProject || project.Title != "Renamed plan" || project.RecordBodySHA256 != sha256Hex(project.Body) {
		t.Errorf("project kind, title, record_body_sha256 = %q, %q, %q; want project, Renamed plan, the digest of its body %q", project.Kind, project.Title, project.RecordBodySHA256, sha256Hex(project.Body))
	}
}

// TestUpsertTaskWithRecord_RefusedPayloadKeepsFormerRecord pins that a refused payload leaves the record as it was.
// One payload is refused before the record file is written, the other after it, which then restores the former bytes.
func TestUpsertTaskWithRecord_RefusedPayloadKeepsFormerRecord(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name    string
		payload map[string]any
		wantErr string
	}{
		{"InvalidKindRefusedBeforeTheRecordWrite", map[string]any{"slug": "idea", "kind": "epic"}, "kind"},
		{"DiscussedNoteTurnedTaskWithNoRecipeRefusedAfterTheRecordWrite", map[string]any{"slug": "idea", "kind": "task"}, `"idea"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			seeder, actor, boardPath := newRecordUpsertBoards(t, "", nil)
			seedDiscussedSketch(t, seeder, "idea")
			entryBefore := entryForRaddle(t, seeder, "idea")

			_, _, err := actor.UpsertTaskWithRecord(tt.payload, []byte("new record\n"), func(string) (bool, error) { return false, nil })

			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("UpsertTaskWithRecord(%v) error = %v; want a refusal containing %q", tt.payload, err, tt.wantErr)
			}
			if got, err := os.ReadFile(filepath.Join(boardPath, DecisionFileName("idea"))); err != nil || string(got) != formerRecord {
				t.Errorf("decision record of idea = %q, %v; want %q, the former record", got, err, formerRecord)
			}
			if entryAfter := entryForRaddle(t, actor, "idea"); !reflect.DeepEqual(entryAfter, entryBefore) {
				t.Errorf("entry idea = %+v; want %+v as before", entryAfter, entryBefore)
			}
		})
	}
}
