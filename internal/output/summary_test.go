package output_test

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestSummary pins how an envelope line reads as its summary, and that any other line is no envelope.
func TestSummary(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name   string
		line   string
		want   string
		wantOK bool
	}{
		{
			name:   "OkScalars_EmptyLeftOut",
			line:   `{"ok":true,"slug":"x","count":3,"partial":false,"note":""}`,
			want:   "ok\ncount: 3\nslug: x\n",
			wantOK: true,
		},
		{
			name:   "Failed_MultiLineErrorIndented",
			line:   `{"ok":false,"error":"refused: x\nway forward: y"}`,
			want:   "failed: refused: x\n  way forward: y\n",
			wantOK: true,
		},
		{
			name:   "LongPlainList_CollapsesToCount",
			line:   `{"ok":true,"tips":["a","b","c","d","e","f"],"tags":["a","b"]}`,
			want:   "ok\ntags: a, b\ntips: 6 items\nfull envelope: rerun with --envelope\n",
			wantOK: true,
		},
		{
			name:   "FivePlainItems_JoinedWithoutFooter",
			line:   `{"ok":true,"tips":["a","b","c","d","e"]}`,
			want:   "ok\ntips: a, b, c, d, e\n",
			wantOK: true,
		},
		{
			name:   "LongNoteList_OneLinePerWarning",
			line:   `{"ok":true,"warnings":["w1","w2","w3","w4","w5","w6"]}`,
			want:   "ok\nwarnings: w1\nwarnings: w2\nwarnings: w3\nwarnings: w4\nwarnings: w5\nwarnings: w6\n",
			wantOK: true,
		},
		{
			name:   "ShortNoteList_OneLinePerWarning",
			line:   `{"ok":true,"warnings":["a","b"]}`,
			want:   "ok\nwarnings: a\nwarnings: b\n",
			wantOK: true,
		},
		{
			name:   "NotesOutsideCollapsedObject_EachOnOwnLine",
			line:   `{"ok":false,"error":"boom","fetch_error":"nope","detail":{"size":2,"way_forward":"lym fix","x_warning":"careful"}}`,
			want:   "failed: boom\ndetail:\n  size: 2\n  way_forward: lym fix\n  x_warning: careful\nfetch_error: nope\n",
			wantOK: true,
		},
		{
			name: "ObjectList_OneLinePerItemWithNotes",
			line: `{"ok":true,"runs":[{"slug":"a","state":"held","line":"waits for check"},{"slug":"b","line":"running here","way_forward":"lym adopt b"}]}`,
			want: "ok\nruns:\n" +
				"  - a: waits for check\n" +
				"  - b: running here\n" +
				"    way_forward: lym adopt b\n" +
				"full envelope: rerun with --envelope\n",
			wantOK: true,
		},
		{
			name:   "ObjectListWithoutLabel_PlainFields",
			line:   `{"ok":true,"checks":[{"pid":12,"log":"suite.log"}]}`,
			want:   "ok\nchecks:\n  - log=suite.log, pid=12\n",
			wantOK: true,
		},
		{
			name: "DeepObject_CollapsesKeepingLabelledNotes",
			line: `{"ok":true,"reconcile":{"pairs":{"p1":{"branch":"x","warnings":["diverged","ahead"],"tips":[{"slug":"t","error":"boom"}]}}}}`,
			want: "ok\nreconcile:\n  pairs:\n    p1: 3 fields\n" +
				"      tips.t.error: boom\n" +
				"      warnings: diverged\n" +
				"      warnings: ahead\n" +
				"full envelope: rerun with --envelope\n",
			wantOK: true,
		},
		{name: "PlainText", line: "plain text"},
		{name: "ObjectWithoutOk", line: `{"slug":"x"}`},
		{name: "Array", line: `[1]`},
		{name: "TwoEnvelopes", line: `{"ok":true} {"ok":true}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := output.Summary([]byte(tt.line))
			if ok != tt.wantOK {
				t.Fatalf("Summary(%q) ok = %v; want %v", tt.line, ok, tt.wantOK)
			}
			if got != tt.want {
				t.Errorf("Summary(%q) = %q; want %q", tt.line, got, tt.want)
			}
		})
	}
}
