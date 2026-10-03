package envkit

import (
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestFullEnv_FillsEverySeam(t *testing.T) {
	if got := NilSeams(FullEnv(t)); len(got) != 0 {
		t.Errorf("NilSeams(FullEnv) = %v; want none", got)
	}
}

func TestNilSeams_ReportsExactPaths(t *testing.T) {
	env := FullEnv(t)
	env.Shuttle = nil
	env.CommitPlan = nil
	env.Landing.PushBranch = nil
	env.Teardown.Remove = nil
	env.SeedChild.WriteSeed = nil

	got := NilSeams(env)
	slices.Sort(got)
	want := []string{"CommitPlan", "Landing.PushBranch", "SeedChild.WriteSeed", "Shuttle", "Teardown.Remove"}
	if !slices.Equal(got, want) {
		t.Errorf("NilSeams = %v; want %v", got, want)
	}
}

func TestNilSeams_SkipsNilLegalPaths(t *testing.T) {
	env := FullEnv(t)
	env.Now = nil
	env.PrimeLock.Sleep = nil
	if got := NilSeams(env); len(got) != 0 {
		t.Errorf("NilSeams = %v; want none for nil-legal seams", got)
	}
}

func TestFullEnv_PathFieldsAbsoluteUnderOneRoot(t *testing.T) {
	env := FullEnv(t)
	root := filepath.Dir(env.Cwd)

	v := reflect.ValueOf(env)
	checked := 0
	for i := 0; i < v.NumField(); i++ {
		name := v.Type().Field(i).Name
		if v.Field(i).Kind() != reflect.String {
			continue
		}
		if name != "Cwd" && !strings.HasSuffix(name, "Path") && !strings.HasSuffix(name, "Dir") && !strings.HasSuffix(name, "Root") {
			continue
		}
		checked++
		p := v.Field(i).String()
		if !filepath.IsAbs(p) {
			t.Errorf("%s = %q; want an absolute path", name, p)
		}
		if !strings.HasPrefix(p, root+string(filepath.Separator)) {
			t.Errorf("%s = %q; want a path under %q", name, p, root)
		}
	}
	if checked == 0 {
		t.Fatal("no path field checked")
	}
}
