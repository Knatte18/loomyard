package main

import (
	"strings"
	"testing"
)

func TestParseTraceEvents(t *testing.T) {
	t.Parallel()

	version := `{"event":"version","evt":"3","exe":"2.50.0"}` + "\n"
	rows := []struct {
		name        string
		events      string
		wantName    string
		wantFixture bool
		wantOK      bool
	}{
		{
			name:        "fixture git",
			events:      version + `{"event":"def_param","scope":"env","param":"LYX_FIXTURE_GIT","value":"1"}` + "\n" + `{"event":"cmd_name","name":"commit","hierarchy":"commit"}` + "\n",
			wantName:    "commit",
			wantFixture: true,
			wantOK:      true,
		},
		{
			name:     "code git",
			events:   version + `{"event":"cmd_name","name":"rev-parse","hierarchy":"rev-parse"}` + "\n",
			wantName: "rev-parse",
			wantOK:   true,
		},
		{
			name:     "other env param is not the marker",
			events:   version + `{"event":"def_param","scope":"env","param":"HOME","value":"/x"}` + "\n" + `{"event":"cmd_name","name":"status"}` + "\n",
			wantName: "status",
			wantOK:   true,
		},
		{
			name:     "no cmd_name counts under unknown",
			events:   version,
			wantName: "unknown",
			wantOK:   true,
		},
		{name: "empty file", events: "", wantName: "unknown"},
		{name: "garbage lines", events: "not json\n\n", wantName: "unknown"},
	}
	for _, row := range rows {
		gotName, gotFixture, gotOK := parseTraceEvents([]byte(row.events))
		if gotName != row.wantName || gotFixture != row.wantFixture || gotOK != row.wantOK {
			t.Errorf("%s: parseTraceEvents = %q, %v, %v; want %q, %v, %v", row.name, gotName, gotFixture, gotOK, row.wantName, row.wantFixture, row.wantOK)
		}
	}
}

func TestRenderCensus(t *testing.T) {
	t.Parallel()

	var first, second gitCensus
	for range 3 {
		first.add("commit", true)
	}
	first.add("rev-parse", false)
	second.add("rev-parse", false)
	second.add("rev-parse", true)

	got := renderCensus([]resourceRow{{census: first}, {census: second}})

	lines := strings.Split(got, "\n")
	var table []string
	for _, line := range lines {
		if strings.HasPrefix(line, "commit") || strings.HasPrefix(line, "rev-parse") || strings.HasPrefix(line, "TOTAL") {
			table = append(table, strings.Join(strings.Fields(line), " "))
		}
	}
	want := []string{"commit 3 3 0", "rev-parse 3 1 2", "TOTAL 6 4 2"}
	if strings.Join(table, "|") != strings.Join(want, "|") {
		t.Errorf("census rows = %q; want %q (sorted by total, ties by name)\n%s", table, want, got)
	}
	if !strings.Contains(got, "SUBCOMMAND") || !strings.Contains(got, "approximate where a hub build overlaps") {
		t.Errorf("census lacks its header or trailer:\n%s", got)
	}
}

func TestTraceEnv(t *testing.T) {
	t.Parallel()

	got := strings.Join(traceEnv("/tmp/trace"), "\n")
	for _, want := range []string{
		"GIT_TRACE2_EVENT=/tmp/trace",
		"GIT_TRACE2_ENV_VARS=LYX_FIXTURE_GIT",
		"GIT_CONFIG_COUNT=1",
		"GIT_CONFIG_KEY_0=trace2.maxFiles",
		"GIT_CONFIG_VALUE_0=0",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("traceEnv lacks %q:\n%s", want, got)
		}
	}
}
