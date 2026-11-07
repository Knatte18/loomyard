// registrar_test.go covers Registrar: the pane process a live strand registers under its owner and role.
// It also covers the cases that register nothing.

package gatefixer

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/runproc"
	"github.com/Knatte18/loomyard/internal/testkit/shuttlefake"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// registeredRecords decodes the record files under owner in dir.
// It reads the files directly, since runproc's own read prunes a record whose process is gone.
func registeredRecords(t *testing.T, dir, owner string) []runproc.Record {
	t.Helper()
	files, err := os.ReadDir(filepath.Join(dir, owner))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatalf("read %s: %v", filepath.Join(dir, owner), err)
	}
	var records []runproc.Record
	for _, file := range files {
		if filepath.Ext(file.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, owner, file.Name()))
		if err != nil {
			t.Fatalf("read record %s: %v", file.Name(), err)
		}
		var rec runproc.Record
		if err := json.Unmarshal(data, &rec); err != nil {
			t.Fatalf("decode record %s: %v", file.Name(), err)
		}
		records = append(records, rec)
	}
	return records
}

// TestRegistrar pins the registration of a live strand's pane process, the nil seam for an empty owner, and the errors that register nothing.
func TestRegistrar(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const owner = "pair-slug"

	t.Run("LiveStrandRegistersItsPanePidUnderTheOwnerAndRole", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		pid := os.Getpid()
		reed := &shuttlefake.Reed{Strands: []reedengine.StrandStatus{
			{GUID: "other", Live: true, PanePID: pid + 1},
			{GUID: "target", Live: true, PanePID: pid},
		}}

		register := Registrar(reed, dir, owner, RoleGateFixer)
		if register == nil {
			t.Fatal("Registrar() = nil; want a seam")
		}
		if err := register("target"); err != nil {
			t.Fatalf("register() error = %v; want nil", err)
		}
		records := registeredRecords(t, dir, owner)
		if len(records) != 1 || records[0].PID != pid || records[0].Role != RoleGateFixer {
			t.Errorf("records = %+v; want one record of pid %d under role %q", records, pid, RoleGateFixer)
		}
	})

	t.Run("EmptyOwnerReturnsANilSeam", func(t *testing.T) {
		t.Parallel()
		if register := Registrar(&shuttlefake.Reed{}, t.TempDir(), "", RoleGateFixer); register != nil {
			t.Error("Registrar() with an empty owner returned a seam; want nil")
		}
	})

	statusErr := errors.New("reed unreachable")
	errorRows := []struct {
		name    string
		reed    *shuttlefake.Reed
		wantErr error
	}{
		{"AbsentStrandIsAnError", &shuttlefake.Reed{Strands: []reedengine.StrandStatus{{GUID: "other", Live: true, PanePID: 1}}}, nil},
		{"StrandWithNoPanePidIsAnError", &shuttlefake.Reed{Strands: []reedengine.StrandStatus{{GUID: "target", Live: true}}}, nil},
		{"StatusErrorPropagates", &shuttlefake.Reed{StatusErr: statusErr}, statusErr},
	}
	for _, tt := range errorRows {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			err := Registrar(tt.reed, dir, owner, RoleGateFixer)("target")
			if err == nil {
				t.Fatal("register() error = nil; want an error")
			}
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Errorf("register() error = %v; want it to wrap %v", err, tt.wantErr)
			}
			if records := registeredRecords(t, dir, owner); len(records) != 0 {
				t.Errorf("records = %+v; want none registered", records)
			}
		})
	}
}
