package main

import (
	"strings"
	"testing"
)

const testPkgPrefix = "github.com/Knatte18/loomyard/"

func TestParseLineAggregatesPerPackage(t *testing.T) {
	t.Parallel()

	stream := strings.Join([]string{
		`{"Action":"pass","Package":"` + testPkgPrefix + `a","Test":"TestOne","Elapsed":0.5}`,
		`{"Action":"pass","Package":"` + testPkgPrefix + `a","Test":"TestOne/sub","Elapsed":0.4}`,
		`{"Action":"skip","Package":"` + testPkgPrefix + `a","Test":"TestTwo","Elapsed":0}`,
		`{"Action":"fail","Package":"` + testPkgPrefix + `a","Test":"TestThree","Elapsed":1.25}`,
		`{"Action":"output","Package":"` + testPkgPrefix + `a","Test":"TestThree","Output":"boom\n"}`,
		`{"Action":"fail","Package":"` + testPkgPrefix + `a","Elapsed":2}`,
		`{"Action":"output","Package":"` + testPkgPrefix + `b","Output":"?   \tb\t[no test files]\n"}`,
		`{"Action":"skip","Package":"` + testPkgPrefix + `b","Elapsed":0}`,
		`not json`,
	}, "\n")

	pkgs := map[string]*pkgResult{}
	var tests []testResult
	for _, line := range strings.Split(stream, "\n") {
		parseLine([]byte(line), pkgs, &tests)
	}

	rows := []struct {
		pkg     string
		count   int
		serial  float64
		elapsed float64
		action  string
		noTests bool
	}{
		{testPkgPrefix + "a", 3, 1.75, 2, "fail", false},
		{testPkgPrefix + "b", 0, 0, 0, "skip", true},
	}
	for _, want := range rows {
		got := pkgs[want.pkg]
		if got == nil {
			t.Fatalf("package %s missing", want.pkg)
		}
		if got.tests != want.count || got.serial != want.serial || got.elapsed != want.elapsed ||
			got.action != want.action || got.noTests != want.noTests {
			t.Errorf("%s = {tests:%d serial:%v elapsed:%v action:%s noTests:%v}, want %+v",
				want.pkg, got.tests, got.serial, got.elapsed, got.action, got.noTests, want)
		}
	}
	if len(tests) != 3 {
		t.Errorf("top-level tests kept = %d, want 3", len(tests))
	}
}

func TestRedundancyTags(t *testing.T) {
	t.Parallel()

	rows := []struct {
		name    string
		full    bool
		tags    string
		want    string
		wantErr string
	}{
		{name: "default is every tier but llm", want: "integration,tmux"},
		{name: "full", full: true, want: "integration"},
		{name: "explicit", tags: "tmux", want: "tmux"},
		{name: "both", full: true, tags: "tmux", wantErr: "-full and -tags"},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			got, err := redundancyTags(row.full, row.tags)
			if row.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), row.wantErr) {
					t.Fatalf("err = %v, want it to contain %q", err, row.wantErr)
				}
				return
			}
			if err != nil || got != row.want {
				t.Fatalf("redundancyTags = %q, %v; want %q", got, err, row.want)
			}
		})
	}
}

func TestModeConflict(t *testing.T) {
	t.Parallel()

	rows := []struct {
		name       string
		redundancy bool
		resources  bool
		wantErr    string
	}{
		{name: "neither"},
		{name: "redundancy alone", redundancy: true},
		{name: "resources alone", resources: true},
		{name: "both", redundancy: true, resources: true, wantErr: "-resources and -redundancy"},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			err := modeConflict(row.redundancy, row.resources)
			if row.wantErr == "" {
				if err != nil {
					t.Fatalf("modeConflict = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), row.wantErr) {
				t.Fatalf("err = %v, want it to contain %q", err, row.wantErr)
			}
		})
	}
}

func TestResolveTags(t *testing.T) {
	t.Parallel()

	rows := []struct {
		name    string
		full    bool
		tags    string
		want    string
		wantErr string
	}{
		{name: "neither", want: ""},
		{name: "full", full: true, want: "integration"},
		{name: "tags", tags: "integration,tmux", want: "integration,tmux"},
		{name: "both", full: true, tags: "tmux", wantErr: "-full and -tags"},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			got, err := resolveTags(row.full, row.tags)
			if row.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), row.wantErr) {
					t.Fatalf("err = %v, want it to contain %q", err, row.wantErr)
				}
				return
			}
			if err != nil || got != row.want {
				t.Fatalf("resolveTags = %q, %v; want %q", got, err, row.want)
			}
		})
	}
}
