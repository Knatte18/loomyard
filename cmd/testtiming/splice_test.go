package main

import (
	"reflect"
	"strings"
	"testing"
)

func spliceFixtureSection(pkg string, tests int) pkgReport {
	return pkgReport{pkg: pkg, tests: tests, wall: 1, serial: 1}
}

func spliceFixtureError(pkg string) pkgReport {
	return pkgReport{pkg: pkg, err: "coverage binary: build failed"}
}

func spliceMeasured(r pkgReport) measuredPackage {
	return measuredPackage{pkg: r.pkg, section: renderPackage(r), failed: r.err != ""}
}

func TestSpliceReport(t *testing.T) {
	t.Parallel()

	const tags = "integration,tmux"
	a, c, e := spliceFixtureSection("internal/a", 1), spliceFixtureSection("internal/c", 2), spliceFixtureSection("internal/e", 3)
	existing := renderReport(tags, []pkgReport{a, c, e})

	newC := spliceFixtureSection("internal/c", 9)
	newA := spliceFixtureSection("internal/a", 7)
	b := spliceFixtureSection("internal/b", 4)
	z := spliceFixtureSection("internal/z", 5)

	rows := []struct {
		name     string
		tags     string
		measured []measuredPackage
		want     []pkgReport
		wantErr  bool
	}{
		{
			name:     "replaces the section of a measured package",
			tags:     tags,
			measured: []measuredPackage{spliceMeasured(newC)},
			want:     []pkgReport{a, newC, e},
		},
		{
			name:     "inserts a package with no section in import-path order",
			tags:     tags,
			measured: []measuredPackage{spliceMeasured(b), spliceMeasured(z)},
			want:     []pkgReport{a, b, c, e, z},
		},
		{
			name:     "removes the section of a package that lists no tests",
			tags:     tags,
			measured: []measuredPackage{{pkg: "internal/c", noTests: true}, {pkg: "internal/x", noTests: true}},
			want:     []pkgReport{a, e},
		},
		{
			name: "keeps the old section of a failed package and writes the others",
			tags: tags,
			measured: []measuredPackage{
				spliceMeasured(newA),
				spliceMeasured(spliceFixtureError("internal/c")),
				spliceMeasured(spliceFixtureError("internal/b")),
			},
			want: []pkgReport{newA, spliceFixtureError("internal/b"), c, e},
		},
		{
			name:     "refuses a different tag set",
			tags:     "integration",
			measured: []measuredPackage{spliceMeasured(newC)},
			wantErr:  true,
		},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()

			got, err := spliceReport(existing, row.tags, row.measured)
			if row.wantErr {
				if err == nil || got != "" {
					t.Fatalf("spliceReport = (%q, %v), want an error and no text", got, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("spliceReport: %v", err)
			}
			if want := renderReport(tags, row.want); got != want {
				t.Errorf("spliced report differs:\n--- got ---\n%s\n--- want ---\n%s", got, want)
			}
		})
	}
}

func TestSpliceReportRejectsReportWithoutTagsHeader(t *testing.T) {
	t.Parallel()

	if got, err := spliceReport("# Test redundancy\n", "integration", nil); err == nil || got != "" {
		t.Fatalf("spliceReport = (%q, %v), want an error and no text", got, err)
	}
}

func TestSplitPatterns(t *testing.T) {
	t.Parallel()

	rows := []struct {
		flag string
		want []string
	}{
		{"./...", []string{"./..."}},
		{"./internal/a,./internal/b", []string{"./internal/a", "./internal/b"}},
		{" ./internal/a , ,./internal/b,", []string{"./internal/a", "./internal/b"}},
		{"", nil},
	}
	for _, row := range rows {
		if got := splitPatterns(row.flag); !reflect.DeepEqual(got, row.want) {
			t.Errorf("splitPatterns(%q) = %q, want %q", row.flag, got, row.want)
		}
	}
}

func TestCoversModule(t *testing.T) {
	t.Parallel()

	layout := moduleLayout{dirs: map[string]string{"m/a": "a", "m/b": "b"}}
	rows := []struct {
		targets []string
		want    bool
	}{
		{[]string{"m/a", "m/b"}, true},
		{[]string{"m/a"}, false},
		{[]string{"m/a", "m/c"}, false},
	}
	for _, row := range rows {
		if got := coversModule(layout, row.targets); got != row.want {
			t.Errorf("coversModule(%s) = %v, want %v", strings.Join(row.targets, ","), got, row.want)
		}
	}
}
