// manifest_test.go covers the launch manifest record, its comparison and the copy a launch keeps.

package shuttleengine

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestManifest_Record(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	delivered := ManifestItem{Channel: ManifestChannelNotice, Key: "creel-trust", Delivery: ManifestDelivered}
	omitted := ManifestItem{Channel: ManifestChannelPluginContext, Key: "p@m/run.sh", Delivery: ManifestOmitted, Reason: "exit status 3", Detail: "12 bytes"}
	full := Manifest{Engine: "codex", Role: "writer", PromptEngine: "codex", Items: []ManifestItem{delivered, omitted}}

	t.Run("round trip carries the omitted items as the verdict", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		if err := WriteManifest(dir, full); err != nil {
			t.Fatalf("WriteManifest() error: %v", err)
		}
		got, found, err := ReadManifest(dir)
		want := full
		want.Verdict = []ManifestItem{omitted}
		if err != nil || !found || !reflect.DeepEqual(got, want) {
			t.Errorf("ReadManifest() = %+v, %v, %v; want %+v", got, found, err, want)
		}
		if got := full.Omitted(); !reflect.DeepEqual(got, []ManifestItem{omitted}) {
			t.Errorf("Omitted() = %+v; want the omitted item alone", got)
		}
	})

	t.Run("a record with nothing omitted has an empty verdict", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		if err := WriteManifest(dir, Manifest{Engine: "claude", Items: []ManifestItem{delivered}}); err != nil {
			t.Fatalf("WriteManifest() error: %v", err)
		}
		got, _, err := ReadManifest(dir)
		if err != nil || len(got.Verdict) != 0 {
			t.Errorf("ReadManifest().Verdict = %+v, %v; want empty", got.Verdict, err)
		}
	})

	t.Run("an unknown key is ignored", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		record := `{"engine":"codex","future":1,"items":[{"channel":"notice","key":"creel-trust","delivery":"delivered","extra":true}]}`
		if err := os.WriteFile(filepath.Join(dir, ManifestFileName), []byte(record), 0o644); err != nil {
			t.Fatal(err)
		}
		got, found, err := ReadManifest(dir)
		want := Manifest{Engine: "codex", Items: []ManifestItem{delivered}}
		if err != nil || !found || !reflect.DeepEqual(got, want) {
			t.Errorf("ReadManifest() = %+v, %v, %v; want %+v", got, found, err, want)
		}
	})

	t.Run("an absent record is not found", func(t *testing.T) {
		t.Parallel()
		if _, found, err := ReadManifest(t.TempDir()); found || err != nil {
			t.Errorf("ReadManifest() found = %v, error = %v; want not found", found, err)
		}
	})

	t.Run("missing items are compared by channel and key", func(t *testing.T) {
		t.Parallel()
		absent := ManifestItem{Channel: ManifestChannelGuard, Key: "question-deny", Delivery: ManifestDelivered}
		held := ManifestItem{Channel: ManifestChannelSkillLoad, Key: "scribe:prose", Delivery: ManifestDelivered}
		engineKeyed := ManifestItem{Channel: ManifestChannelStandingPassage, Key: "mine", Engine: "claude", Delivery: ManifestDelivered}
		settings := ManifestItem{Channel: ManifestChannelSettings, Key: "settings.json", Delivery: ManifestDelivered}
		from := Manifest{Items: []ManifestItem{absent, held, engineKeyed, settings}}
		in := Manifest{Items: []ManifestItem{{Channel: ManifestChannelSkillLoad, Key: "scribe:prose", Delivery: ManifestOmitted}}}
		if got := MissingItems(from, in); !reflect.DeepEqual(got, []ManifestItem{absent}) {
			t.Errorf("MissingItems() = %+v; want only %+v", got, absent)
		}
	})
}

func TestRunner_Start_KeepsTheManifestCopy(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	manifest := Manifest{Engine: "claude", Items: []ManifestItem{{Channel: ManifestChannelNotice, Key: "creel-trust", Delivery: ManifestDelivered}}}
	writeRecord := func(t *testing.T, runDir string) {
		if err := WriteManifest(runDir, manifest); err != nil {
			t.Errorf("WriteManifest() error: %v", err)
		}
	}
	tests := []struct {
		name string
		hook func(t *testing.T, runDir string)
		// blocked puts a regular file where the copies' directory goes.
		blocked bool
		// saveFails makes the start fail on saving the run record, once Prepare has written its record.
		saveFails    bool
		wantCopy     bool
		wantNoCopies bool
	}{
		{name: "the copy outlives the run directory", hook: writeRecord, wantCopy: true},
		{name: "no record leaves no copies directory", hook: func(*testing.T, string) {}, wantNoCopies: true},
		{name: "a failed copy does not fail the run", hook: writeRecord, blocked: true},
		{
			name: "an unreadable record does not fail the run",
			hook: func(t *testing.T, runDir string) {
				if err := os.WriteFile(filepath.Join(runDir, ManifestFileName), []byte("not json"), 0o644); err != nil {
					t.Errorf("write record: %v", err)
				}
			},
			wantNoCopies: true,
		},
		{
			name: "a launch whose run record is not saved keeps no copy",
			hook: func(t *testing.T, runDir string) {
				writeRecord(t, runDir)
				if err := os.Mkdir(filepath.Join(runDir, runStateFileName), 0o755); err != nil {
					t.Errorf("block the run record: %v", err)
				}
			},
			saveFails:    true,
			wantNoCopies: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			reed := &fakeReed{AddStrandResult: reedengine.Strand{GUID: "strand-1"}}
			engine := &fakeEngine{PrepareLaunch: Launch{Cmd: "cmd", SessionID: "session-1"}}
			engine.PrepareHook = func(runDir string) { tt.hook(t, runDir) }
			readyStart(reed, engine)
			fx := newFixture(t, reed, engine, withConfig(fastConfig), withClock(newFakeClock(time.Now())))
			copies := filepath.Join(fx.RunRoot, manifestsDirName)
			if tt.blocked {
				if err := os.MkdirAll(fx.RunRoot, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(copies, nil, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			outputFile := filepath.Join(t.TempDir(), "out.md")

			run, err := fx.Runner.StartGated(Spec{Prompt: "x", OutputFiles: []string{outputFile}}, GateSpec{})
			if tt.saveFails {
				if err == nil || !strings.Contains(err.Error(), "save run state") {
					t.Errorf("StartGated() error = %v; want the run record save to fail", err)
				}
				if _, err := os.Stat(copies); !os.IsNotExist(err) {
					t.Errorf("copies directory stat error = %v; want it absent", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("StartGated() error: %v", err)
			}
			runDir := run.RunDir()
			if err := os.WriteFile(filepath.Join(runDir, eventsFileName), []byte("STOP:done\n"), 0o644); err != nil {
				t.Fatalf("seed events: %v", err)
			}
			touchOutputFile(t, outputFile)
			result, err := run.Wait()
			if err != nil || result.Outcome != OutcomeDone {
				t.Fatalf("Wait() = %q, %v; want %q", result.Outcome, err, OutcomeDone)
			}
			if _, err := os.Stat(runDir); !os.IsNotExist(err) {
				t.Errorf("run directory stat error = %v; want it removed", err)
			}

			copyPath := filepath.Join(copies, filepath.Base(runDir)+".json")
			got, found, err := state.ReadJSON[Manifest](copyPath, copyPath+".lock")
			switch {
			case tt.wantCopy:
				if err != nil || !found || !reflect.DeepEqual(got, manifest) {
					t.Errorf("copy = %+v, %v, %v; want %+v", got, found, err, manifest)
				}
			case found:
				t.Errorf("copy = %+v; want none", got)
			}
			if tt.wantNoCopies {
				if _, err := os.Stat(copies); !os.IsNotExist(err) {
					t.Errorf("copies directory stat error = %v; want it absent", err)
				}
			}
		})
	}
}
