// open_test.go covers Runner.Open: the strand it adds, the chain its pane runs, and the refusals made before anything is created.

package shuttleengine

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/reedengine/render"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shell"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
)

// TestRunner_Open pins the strand Open adds and what it refuses before creating one.
func TestRunner_Open(t *testing.T) {
	// One subtest captures the process-global logger, so the test runs serially and no parallel test of the package swaps the logger output during the capture.
	sh := shell.ForGOOS()
	launch := Launch{Cmd: "open-cmd", ResumeCmd: "resume-cmd", SessionID: "session-9"}
	spec := OpenSpec{Role: "explore", Segment: segmentcolor.Segment("review")}

	tests := []struct {
		name string
		// engine builds the runner's engine; nil means a capable opener returning launch.
		engine  func() Engine
		spec    OpenSpec
		covered bool
		wantErr string
	}{
		{name: "an empty role is refused before anything is created", spec: OpenSpec{}, wantErr: "the role must not be empty"},
		{name: "a live pause covering the worktree is refused", spec: spec, covered: true, wantErr: "re-run lyx shuttle open",
			engine: func() Engine { return &openerFakeEngine{openErr: errors.New("provider down")} },
		},
		{name: "an engine without the capability is refused", spec: spec, engine: func() Engine { return &fakeEngine{} }, wantErr: "cannot open a session"},
		{
			name: "a failed open launch is returned", spec: spec, wantErr: "provider down",
			engine: func() Engine { return &openerFakeEngine{openErr: errors.New("provider down")} },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			reed := &fakeReed{}
			var engine Engine = &openerFakeEngine{launch: launch}
			if tt.engine != nil {
				engine = tt.engine()
			}
			fx := newFixture(t, reed, engine)
			if tt.covered {
				geom, _ := coverHub(t, time.Now())
				fx.Runner.SetPause(geom)
			}

			_, err := fx.Runner.Open(tt.spec)

			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Open() error = %v; want it to contain %q", err, tt.wantErr)
			}
			if tt.covered && !strings.Contains(err.Error(), "way forward") {
				t.Errorf("Open() error = %v; want it to name the way forward", err)
			}
			if len(reed.AddStrandCalls) != 0 {
				t.Errorf("AddStrand calls = %v; want none from a refused open", reed.AddStrandCalls)
			}
			markerDir := filepath.Join(fx.Anchor, lyxdirs.DotLyxDirName, openMarkerDirName)
			if _, err := os.Stat(markerDir); !os.IsNotExist(err) {
				t.Errorf("marker directory stat err = %v; want it absent from a refused open", err)
			}
		})
	}

	// It captures the process-global logger, so it runs serially like the test.
	t.Run("adds an own-window strand by role", func(t *testing.T) {
		buf := logcapture.CaptureVerbose(t)
		reed := &fakeReed{AddStrandResult: reedengine.Strand{GUID: "guid-1", Name: "ly:task:explore"}}
		engine := &openerFakeEngine{launch: launch}
		fx := newFixture(t, reed, engine)
		bypassSpec := spec
		bypassSpec.PermissionMode = "bypass"

		result, err := fx.Runner.Open(bypassSpec)
		if err != nil {
			t.Fatalf("Open() error = %v", err)
		}

		if len(engine.openSpecs) != 1 || engine.openSpecs[0].PermissionMode != "bypass" {
			t.Errorf("OpenLaunch specs = %+v; want exactly one carrying PermissionMode bypass", engine.openSpecs)
		}
		if !strings.Contains(buf.String(), "permissionMode=bypass") {
			t.Errorf("log output = %q; want it to hold permissionMode=bypass", buf.String())
		}

		if len(reed.AddStrandCalls) != 1 {
			t.Fatalf("AddStrand calls = %v; want one", reed.AddStrandCalls)
		}
		add := reed.AddStrandCalls[0]
		if add.Role != "explore" || add.NameOverride != "" || add.Segment != "review" || add.SessionID != "session-9" || add.Provider != "fake" {
			t.Errorf("AddStrand spec = %+v; want role explore, no name override, segment review, the launch's session id, the engine's name as provider", add)
		}
		if add.Display != (render.Display{Anchor: render.AnchorOwnWindow}) {
			t.Errorf("Display = %+v; want the own-window anchor alone", add.Display)
		}
		markerDir := filepath.Join(fx.Anchor, lyxdirs.DotLyxDirName, openMarkerDirName)
		if len(add.DoneWhen) != 1 || filepath.Dir(add.DoneWhen[0]) != markerDir || !strings.HasSuffix(add.DoneWhen[0], ".exited") {
			t.Fatalf("DoneWhen = %v; want one .exited marker under %s", add.DoneWhen, markerDir)
		}
		marker := add.DoneWhen[0]
		selfRemove := sh.Invoke("lyx") + " reed remove --name " + sh.EnvRef(agentname.StrandNameEnv) + " --detach"
		if want := sh.Chain("open-cmd", sh.Touch(marker), selfRemove, "exit"); add.Cmd != want {
			t.Errorf("Cmd = %q; want %q", add.Cmd, want)
		}
		if want := sh.Chain("resume-cmd", sh.Touch(marker), selfRemove, "exit"); add.ResumeCmd != want {
			t.Errorf("ResumeCmd = %q; want %q", add.ResumeCmd, want)
		}
		if result != (OpenResult{StrandGUID: "guid-1", StrandName: "ly:task:explore", SessionID: "session-9", Engine: "fake"}) {
			t.Errorf("result = %+v; want the strand's guid and name, the session id and the default engine's name", result)
		}
		if _, err := os.Stat(fx.RunRoot); !os.IsNotExist(err) {
			t.Errorf("run-directory root stat err = %v; want it absent, since an open writes no run directory", err)
		}
	})

	t.Run("a second open of the same role passes the role again", func(t *testing.T) {
		t.Parallel()
		reed := &fakeReed{}
		fx := newFixture(t, reed, &openerFakeEngine{launch: launch})

		for range 2 {
			if _, err := fx.Runner.Open(spec); err != nil {
				t.Fatalf("Open() error = %v", err)
			}
		}

		if len(reed.AddStrandCalls) != 2 {
			t.Fatalf("AddStrand calls = %v; want two", reed.AddStrandCalls)
		}
		for _, add := range reed.AddStrandCalls {
			if add.Role != "explore" || add.NameOverride != "" {
				t.Errorf("AddStrand spec = %+v; want role explore and an empty name override, so reed numbers the second", add)
			}
		}
		if reed.AddStrandCalls[0].DoneWhen[0] == reed.AddStrandCalls[1].DoneWhen[0] {
			t.Errorf("both opens share the marker %s; want distinct markers", reed.AddStrandCalls[0].DoneWhen[0])
		}
	})
}
