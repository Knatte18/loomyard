// stencilstore_test.go covers BodyHash (and so NormalizeLF), ParseStamp, ApplyStamp, and Classify against hermetic in-memory content -- no t.TempDir() is needed for these, since none of them touch disk.

package stencilstore

import (
	"testing"
	"time"
)

//testtiming:keep pins that CRLF and a changed banner hash like their LF and old-banner twins while a changed body does not, which no covering test asserts
func TestBodyHash(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		a, b      string
		wantEqual bool
	}{
		{
			name:      "CRLFHashesLikeLF",
			a:         "line one\nline two\n",
			b:         "line one\r\nline two\r\n",
			wantEqual: true,
		},
		{
			name:      "IgnoresBannerChange",
			a:         "<!-- lyx-stencil: sha256=aaaa -->\nbody unchanged\n",
			b:         "<!-- lyx-stencil: sha256=bbbb -->\nbody unchanged\n",
			wantEqual: true,
		},
		{
			name:      "ReactsToBodyChange",
			a:         "<!-- lyx-stencil: sha256=aaaa -->\nbody unchanged\n",
			b:         "<!-- lyx-stencil: sha256=aaaa -->\nbody changed\n",
			wantEqual: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			hashA, hashB := BodyHash([]byte(tt.a)), BodyHash([]byte(tt.b))
			if (hashA == hashB) != tt.wantEqual {
				t.Errorf("BodyHash(%q) = %q, BodyHash(%q) = %q; want equal = %v", tt.a, hashA, tt.b, hashB, tt.wantEqual)
			}
		})
	}
}

//testtiming:keep pins that a stamp is replaced in place or prepended with the body hash unchanged, where the restamp test reaches only the replacing case
func TestApplyStamp(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		original string
		hash     string
	}{
		{
			name:     "ReplacesExistingBannerInPlace",
			original: "<!-- lyx-stencil: sha256=" + fakeHash('a') + " -->\nbody\n",
			hash:     fakeHash('b'),
		},
		{
			name:     "PrependsNewBannerWhenAbsent",
			original: "body with no banner\n",
			hash:     fakeHash('c'),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := ApplyStamp([]byte(tt.original), tt.hash)

			stamp, ok := ParseStamp(got)
			if !ok || stamp != tt.hash {
				t.Fatalf("ParseStamp(ApplyStamp(...)) = (%q, %v); want (%q, true)", stamp, ok, tt.hash)
			}
			if BodyHash(got) != BodyHash([]byte(tt.original)) {
				t.Errorf("ApplyStamp changed the body: BodyHash(got) = %q; want %q", BodyHash(got), BodyHash([]byte(tt.original)))
			}
		})
	}
}

func TestParseStamp(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
		wantOK  bool
	}{
		{
			name:    "MissingBanner",
			content: "no banner here\n",
			wantOK:  false,
		},
		{
			name:    "BannerWithNoStencilLine",
			content: "<!-- just a header -->\nbody\n",
			wantOK:  false,
		},
		{
			name:    "MalformedHex",
			content: "<!-- lyx-stencil: sha256=not-hex -->\nbody\n",
			wantOK:  false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, ok := ParseStamp([]byte(tt.content))
			if ok != tt.wantOK {
				t.Errorf("ParseStamp(%q) ok = %v; want %v", tt.content, ok, tt.wantOK)
			}
		})
	}
}

//testtiming:keep pins the state each on-disk shape classifies to, which the reconcile tests observe only through the file outcome
func TestClassify(t *testing.T) {
	t.Parallel()

	shipped := []byte("shipped body\n")
	shippedHash := BodyHash(shipped)
	edited := []byte("edited body\n")

	tests := []struct {
		name   string
		onDisk []byte
		exists bool
		want   State
	}{
		{
			name:   "Absent",
			onDisk: nil,
			exists: false,
			want:   StateAbsent,
		},
		{
			name:   "Untouched",
			onDisk: ApplyStamp(shipped, shippedHash),
			exists: true,
			want:   StateUntouched,
		},
		{
			name:   "Edited",
			onDisk: ApplyStamp(edited, shippedHash),
			exists: true,
			want:   StateEdited,
		},
		{
			name:   "MissingStamp",
			onDisk: edited,
			exists: true,
			want:   StateEdited,
		},
		{
			name:   "ReconciledBeatsStaleStamp",
			onDisk: ApplyStamp(shipped, BodyHash([]byte("some other old default\n"))),
			exists: true,
			want:   StateReconciled,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := Classify(tt.onDisk, tt.exists, shipped)
			if got != tt.want {
				t.Errorf("Classify(...) = %v; want %v", got, tt.want)
			}
		})
	}
}

// TestRelPath pins the family-from-first-token derivation over both stencil and specs names.
// The two loom-plan-* rows are the point: they pin that the registered names contracts/specs
// registers both land under one loom/ family directory, from the side of the package that actually
// performs the derivation, so a future change to the family rule fails here and not only in the
// specs package.
func TestRelPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		want string
	}{
		{"loom-template-plan", "loom/loom-template-plan.md"},
		{"loom-plan-spec", "loom/loom-plan-spec.md"},
		{"solo", "solo.md"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := RelPath(tt.name)
			if got != tt.want {
				t.Errorf("RelPath(%q) = %q; want %q", tt.name, got, tt.want)
			}
		})
	}
}

// TestApplyWriter_RoundTripsThroughParseWriter covers setting, rewriting and removing the writer keys in a stamp line, on a one-line and a multi-line banner, and the body never changing.
func TestApplyWriter_RoundTripsThroughParseWriter(t *testing.T) {
	t.Parallel()

	hash := fakeHash('a')
	first := Writer{Revision: "rev-one", Time: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)}
	second := Writer{Revision: "rev-two", Time: time.Date(2026, 6, 7, 8, 9, 10, 0, time.UTC)}
	oneLine := []byte("<!-- lyx-stencil: sha256=" + hash + " -->\n\nbody\n")
	multiLine := []byte("<!-- note\nlyx-stencil: sha256=" + hash + "\n-->\n\nbody\n")

	tests := []struct {
		name    string
		content []byte
		apply   []Writer
		want    Writer
	}{
		{"set on one-line banner", oneLine, []Writer{first}, first},
		{"set on multi-line banner", multiLine, []Writer{first}, first},
		{"rewrite", oneLine, []Writer{first, second}, second},
		{"remove with zero writer", oneLine, []Writer{first, {}}, Writer{}},
		{"banner with no keys", oneLine, nil, Writer{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.content
			for _, writer := range tt.apply {
				got = ApplyWriter(got, writer)
			}
			parsed := ParseWriter(got)
			if parsed.Revision != tt.want.Revision || !parsed.Time.Equal(tt.want.Time) {
				t.Errorf("ParseWriter(ApplyWriter(...)) = %+v; want %+v", parsed, tt.want)
			}
			if stamp, ok := ParseStamp(got); !ok || stamp != hash {
				t.Errorf("ParseStamp after ApplyWriter = (%q, %v); want the original hash", stamp, ok)
			}
			if BodyHash(got) != BodyHash(tt.content) {
				t.Errorf("ApplyWriter changed the body hash")
			}
			if tt.want == (Writer{}) && string(got) != string(tt.content) {
				t.Errorf("a keyless result = %q; want the original content %q", got, tt.content)
			}
		})
	}

	if got := ApplyWriter([]byte("no banner\n"), first); string(got) != "no banner\n" {
		t.Errorf("ApplyWriter on content with no stamp = %q; want it unchanged", got)
	}
}

// fakeHash returns a syntactically valid 64-lowercase-hex-character stamp value built from a single
// repeated byte, so tests need not compute a real sha256 sum to exercise ApplyStamp/ParseStamp.
func fakeHash(b byte) string {
	hash := make([]byte, 64)
	for i := range hash {
		hash[i] = b
	}
	return string(hash)
}
