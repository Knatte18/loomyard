//go:build integration

// acceptaudit_test.go covers the accept-audit verb through its cobra.Command, on the verbs fixture's hub.
package webstercli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// seedPendingFinding points the geometry at the fixture's real repo, records a done batch at its HEAD, and leaves one pending finding naming the tracked base.txt.
func (fx *verbsFixture) seedPendingFinding(t *testing.T) {
	t.Helper()
	fx.CLI.geom.WorktreeRoot = fx.Worktree
	st := fx.initState(t, "master-model")
	head := gitkit.RevParse(t, fx.Worktree, "HEAD")
	st.Batches[1] = &websterengine.BatchState{Slug: "only", Kind: "fork", Terminal: true, Status: "done", Digest: &websterengine.Digest{Status: "done", HeadSHA: head}}
	st.PendingAuditFindings = []websterengine.PendingAuditFinding{{ID: "sess/parent-write-1", Class: "parent-write", Detail: "wrote outside the contract", Paths: []string{"base.txt"}}}
	if err := websterengine.SaveState(fx.CLI.geom.WebsterDir, fx.CLI.geom.ScratchDir, st); err != nil {
		t.Fatalf("SaveState() error = %v", err)
	}
}

// TestAcceptAuditCmd_AcceptsPendingThenIsIdempotent proves the verb lists a pending finding, clears it and records no disposition, and that a second call reports nothing.
func TestAcceptAuditCmd_AcceptsPendingThenIsIdempotent(t *testing.T) {
	t.Setenv("WEFT_SKIP_GIT", "1")
	fx := newVerbsFixture(t)
	fx.seedPendingFinding(t)

	var out strings.Builder
	if code := clihelp.Execute(fx.CLI.acceptAuditCmd(), &out, []string{}); code != 0 {
		t.Fatalf("accept-audit = %d; want 0, output: %s", code, out.String())
	}
	for _, want := range []string{"parent-write", "wrote outside the contract", "base.txt"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output missing %q; got %q", want, out.String())
		}
	}
	loaded, err := websterengine.LoadState(fx.CLI.geom.WebsterDir, fx.CLI.geom.ScratchDir)
	if err != nil || loaded == nil {
		t.Fatalf("LoadState() = %v, %v; want a state, nil", loaded, err)
	}
	if len(loaded.PendingAuditFindings) != 0 {
		t.Errorf("PendingAuditFindings = %v; want none", loaded.PendingAuditFindings)
	}
	if len(loaded.AuditDispositions) != 0 {
		t.Errorf("AuditDispositions = %v; want none", loaded.AuditDispositions)
	}
	// The accepted path is no contract file, so the envelope carries no next field.
	if strings.Contains(out.String(), `"next"`) {
		t.Errorf("output = %q; want no next field", out.String())
	}

	out.Reset()
	if code := clihelp.Execute(fx.CLI.acceptAuditCmd(), &out, []string{}); code != 0 {
		t.Fatalf("second accept-audit = %d; want 0, output: %s", code, out.String())
	}
	if !strings.Contains(out.String(), `"accepted":[]`) {
		t.Errorf("second call output = %q; want an empty accepted list", out.String())
	}
}

// TestAcceptAuditCmd_Refusals proves a suspect path that moved since the run recorded it refuses with its way forward and leaves state.json byte-identical:
// an edited path refuses with the git way forward, and a commit on top of the recorded head refuses even when a second commit restores the file's content.
// It sets WEFT_SKIP_GIT, so it is not parallel.
func TestAcceptAuditCmd_Refusals(t *testing.T) {
	t.Setenv("WEFT_SKIP_GIT", "1")

	cases := []struct {
		name string
		// move changes the fixture's worktree after the pending finding is seeded.
		move   func(t *testing.T, fx *verbsFixture)
		wantIn []string
	}{
		{
			name: "edited path",
			move: func(t *testing.T, fx *verbsFixture) {
				if err := os.WriteFile(filepath.Join(fx.Worktree, "base.txt"), []byte("edited"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
			wantIn: []string{"base.txt", "git checkout"},
		},
		{
			name: "commit past the recorded head",
			move: func(t *testing.T, fx *verbsFixture) {
				basePath := filepath.Join(fx.Worktree, "base.txt")
				original, err := os.ReadFile(basePath)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(basePath, []byte("suspect write"), 0o644); err != nil {
					t.Fatal(err)
				}
				gitkit.Git(t, fx.Worktree, "commit", "-am", "master write")
				if err := os.WriteFile(basePath, original, 0o644); err != nil {
					t.Fatal(err)
				}
				gitkit.Git(t, fx.Worktree, "commit", "-am", "restore content")
			},
			wantIn: []string{"move HEAD back"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fx := newVerbsFixture(t)
			fx.seedPendingFinding(t)
			tc.move(t, fx)
			statePath := filepath.Join(fx.CLI.geom.WebsterDir, "state.json")
			before, err := os.ReadFile(statePath)
			if err != nil {
				t.Fatal(err)
			}

			var out strings.Builder
			if code := clihelp.Execute(fx.CLI.acceptAuditCmd(), &out, []string{}); code == 0 {
				t.Fatalf("accept-audit = 0; want non-zero, output: %s", out.String())
			}
			for _, want := range tc.wantIn {
				if !strings.Contains(out.String(), want) {
					t.Errorf("output missing %q; got %q", want, out.String())
				}
			}
			after, err := os.ReadFile(statePath)
			if err != nil {
				t.Fatal(err)
			}
			if string(before) != string(after) {
				t.Errorf("state.json changed by a refused accept-audit")
			}
		})
	}
}

// TestAcceptAuditCmd_FabricSyncFailureNamesWayForward proves a fabric sync failure still leaves the acceptance saved and names the sentinel and `lyx fabric commit`.
func TestAcceptAuditCmd_FabricSyncFailureNamesWayForward(t *testing.T) {
	t.Setenv("WEFT_SKIP_GIT", "")
	fx := newVerbsFixture(t)
	fx.seedPendingFinding(t)
	syncErr := errors.New("probe: fabric unreachable")
	fx.CLI.openFabric = func() (*fabricengine.Fabric, error) { return nil, syncErr }

	var out strings.Builder
	if code := clihelp.Execute(fx.CLI.acceptAuditCmd(), &out, []string{}); code == 0 {
		t.Fatalf("accept-audit = 0; want non-zero, output: %s", out.String())
	}
	for _, want := range []string{"audit findings accepted but the fabric sync failed", syncErr.Error(), "lyx fabric commit"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output missing %q; got %q", want, out.String())
		}
	}
	loaded, err := websterengine.LoadState(fx.CLI.geom.WebsterDir, fx.CLI.geom.ScratchDir)
	if err != nil || loaded == nil {
		t.Fatalf("LoadState() = %v, %v; want a state, nil", loaded, err)
	}
	if len(loaded.PendingAuditFindings) != 0 {
		t.Errorf("PendingAuditFindings = %v; want none", loaded.PendingAuditFindings)
	}
}

// TestAcceptAuditCmd_AbsentContractFilesCarryNext proves accepting a finding on absent contract files succeeds and the envelope's next field names the re-run that has Master write them again.
func TestAcceptAuditCmd_AbsentContractFilesCarryNext(t *testing.T) {
	t.Setenv("WEFT_SKIP_GIT", "1")
	fx := newVerbsFixture(t)
	fx.seedPendingFinding(t)
	st, err := websterengine.LoadState(fx.CLI.geom.WebsterDir, fx.CLI.geom.ScratchDir)
	if err != nil || st == nil {
		t.Fatalf("LoadState() = %v, %v", st, err)
	}
	outcome := websterengine.OutcomePath(fx.CLI.geom.WebsterDir)
	if err := os.Remove(outcome); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	st.PendingAuditFindings = []websterengine.PendingAuditFinding{{ID: "sess/fork-contract-write-1", Class: "fork-contract-write", Detail: "a fork wrote outcome.yaml", Paths: []string{outcome}}}
	if err := websterengine.SaveState(fx.CLI.geom.WebsterDir, fx.CLI.geom.ScratchDir, st); err != nil {
		t.Fatal(err)
	}

	var out strings.Builder
	if code := clihelp.Execute(fx.CLI.acceptAuditCmd(), &out, []string{}); code != 0 {
		t.Fatalf("accept-audit = %d; want 0, output: %s", code, out.String())
	}
	for _, want := range []string{`"next"`, "re-run", "outcome.yaml and summary.md"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output missing %q; got %q", want, out.String())
		}
	}
}
