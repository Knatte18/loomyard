// report_test.go covers the pure parts of a report:
// the median, the -dir flag, the /proc and /sys parsers and the comparison of two reports.

package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/google/go-cmp/cmp"
)

func TestMedian(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name    string
		samples []time.Duration
		want    time.Duration
	}{
		{"Single", []time.Duration{7}, 7},
		{"OddCountUnsorted", []time.Duration{9, 1, 5}, 5},
		{"EvenCountTakesMeanOfMiddleTwo", []time.Duration{10, 2, 4, 8}, 6},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := median(tt.samples); got != tt.want {
				t.Errorf("median(%v) = %v; want %v", tt.samples, got, tt.want)
			}
		})
	}
}

func TestDirList_Set(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error: %v", err)
	}
	tests := []struct {
		name    string
		values  []string
		want    dirList
		wantErr bool
	}{
		{"NamedPath", []string{"lym=/tmp/lym-probe"}, dirList{{Name: "lym", Path: "/tmp/lym-probe"}}, false},
		{"RelativePathIsResolved", []string{"a=rel"}, dirList{{Name: "a", Path: filepath.Join(cwd, "rel")}}, false},
		{"BarePathIsRefused", []string{"/tmp/zz-probe"}, nil, true},
		{"BarePathHoldingEqualsIsRefused", []string{"/tmp/a=b"}, nil, true},
		{"TwoNames", []string{"a=/x", "b=/y"}, dirList{{Name: "a", Path: "/x"}, {Name: "b", Path: "/y"}}, false},
		{"NameGivenTwiceIsRefused", []string{"a=/x", "a=/y"}, dirList{{Name: "a", Path: "/x"}}, true},
		{"EmptyPathIsRefused", []string{"a="}, nil, true},
		{"EmptyNameIsRefused", []string{"=/x"}, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var got dirList
			var lastErr error
			for _, value := range tt.values {
				lastErr = got.Set(value)
			}
			if (lastErr != nil) != tt.wantErr {
				t.Errorf("Set(%q) error = %v; want error %t", tt.values, lastErr, tt.wantErr)
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("Set(%q) mismatch (-want +got):\n%s", tt.values, diff)
			}
		})
	}
}

func TestParseLoadavg(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name                 string
		text                 string
		want1, want5, want15 *float64
	}{
		{"Kernel", "5.29 8.64 8.34 4/2215 1690393", ptr(5.29), ptr(8.64), ptr(8.34)},
		{"Absent", "", nil, nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got1, got5, got15 := parseLoadavg(tt.text)
			if diff := cmp.Diff([]*float64{tt.want1, tt.want5, tt.want15}, []*float64{got1, got5, got15}); diff != "" {
				t.Errorf("parseLoadavg(%q) mismatch (-want +got):\n%s", tt.text, diff)
			}
		})
	}
}

func TestParseCPUPressure(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name              string
		text              string
		wantAvg10, want60 *float64
	}{
		{
			"ReadsTheSomeLine",
			"some avg10=0.25 avg60=6.62 avg300=12.03 total=4218719109\nfull avg10=1.00 avg60=2.00 avg300=0.00 total=0\n",
			ptr(0.25), ptr(6.62),
		},
		{"Absent", "", nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got10, got60 := parseCPUPressure(tt.text)
			if diff := cmp.Diff([]*float64{tt.wantAvg10, tt.want60}, []*float64{got10, got60}); diff != "" {
				t.Errorf("parseCPUPressure(%q) mismatch (-want +got):\n%s", tt.text, diff)
			}
		})
	}
}

func TestParseCPUInfo(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name      string
		text      string
		wantModel string
		wantMHz   *float64
	}{
		{
			"FirstModelAndMeanFrequency",
			"processor\t: 0\nmodel name\t: Intel(R) Core(TM) Ultra 7\ncpu MHz\t\t: 1000.000\n\n" +
				"processor\t: 1\nmodel name\t: Intel(R) Core(TM) Ultra 7\ncpu MHz\t\t: 3000.000\n",
			"Intel(R) Core(TM) Ultra 7", ptr(2000),
		},
		{"NoFrequency", "model name\t: Neoverse-N1\n", "Neoverse-N1", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotModel, gotMHz := parseCPUInfo(tt.text)
			if gotModel != tt.wantModel {
				t.Errorf("parseCPUInfo() model = %q; want %q", gotModel, tt.wantModel)
			}
			if diff := cmp.Diff(tt.wantMHz, gotMHz); diff != "" {
				t.Errorf("parseCPUInfo() mean MHz mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestSumThrottleCounts(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name  string
		texts []string
		want  *float64
	}{
		{"SumsEveryCore", []string{"3", "4"}, ptr(7)},
		{"UnparsableCount", []string{"3", ""}, nil},
		{"NoCounts", nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if diff := cmp.Diff(tt.want, sumThrottleCounts(tt.texts)); diff != "" {
				t.Errorf("sumThrottleCounts(%q) mismatch (-want +got):\n%s", tt.texts, diff)
			}
		})
	}
}

func TestCompareMeasurements(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	base := []Measurement{
		{Name: "start/true/median_us", Unit: unitMicroseconds, Value: 1300},
		{Name: "start/git_version/median_us", Unit: unitMicroseconds, Value: 0},
		{Name: "newbinary/zz/first_run_ms", Unit: unitMilliseconds, Value: 20},
	}
	other := []Measurement{
		{Name: "newbinary/zz/first_run_ms", Unit: unitMilliseconds, Value: 3000},
		{Name: "start/true/median_us", Unit: unitMicroseconds, Value: 1625},
		{Name: "start/git_version/median_us", Unit: unitMicroseconds, Value: 5200},
		{Name: "start/lym_help/median_us", Unit: unitMicroseconds, Value: 9000},
	}
	want := []ComparisonRow{
		{Name: "start/true/median_us", Unit: unitMicroseconds, Base: ptr(1300), Other: ptr(1625), Ratio: ptr(1.25)},
		{Name: "start/git_version/median_us", Unit: unitMicroseconds, Base: ptr(0), Other: ptr(5200)},
		{Name: "newbinary/zz/first_run_ms", Unit: unitMilliseconds, Base: ptr(20), Other: ptr(3000), Ratio: ptr(150)},
		{Name: "start/lym_help/median_us", Unit: unitMicroseconds, Other: ptr(9000)},
	}
	if diff := cmp.Diff(want, compareMeasurements(base, other)); diff != "" {
		t.Errorf("compareMeasurements() mismatch (-want +got):\n%s", diff)
	}
}

func ptr(value float64) *float64 {
	return &value
}
