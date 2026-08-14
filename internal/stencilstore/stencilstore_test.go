// stencilstore_test.go covers NormalizeLF, BodyHash, ParseStamp, ApplyStamp, and Classify against
// hermetic in-memory content -- no t.TempDir() is needed for these, since none of them touch disk.

package stencilstore

import "testing"

func TestNormalizeLF_CRLFHashesLikeLF(t *testing.T) {
	lf := []byte("line one\nline two\n")
	crlf := []byte("line one\r\nline two\r\n")

	if BodyHash(lf) != BodyHash(crlf) {
		t.Errorf("BodyHash(lf) = %q; want it to equal BodyHash(crlf) = %q", BodyHash(lf), BodyHash(crlf))
	}
}

func TestBodyHash_IgnoresBannerChangesOnly(t *testing.T) {
	base := "<!-- lyx-stencil: sha256=aaaa -->\nbody unchanged\n"
	bannerChanged := "<!-- lyx-stencil: sha256=bbbb -->\nbody unchanged\n"
	bodyChanged := "<!-- lyx-stencil: sha256=aaaa -->\nbody changed\n"

	if BodyHash([]byte(base)) != BodyHash([]byte(bannerChanged)) {
		t.Errorf("BodyHash ignored a banner-only change: base=%q changed=%q", BodyHash([]byte(base)), BodyHash([]byte(bannerChanged)))
	}
	if BodyHash([]byte(base)) == BodyHash([]byte(bodyChanged)) {
		t.Errorf("BodyHash did not react to a body change: base=%q changed=%q", BodyHash([]byte(base)), BodyHash([]byte(bodyChanged)))
	}
}

func TestApplyStamp_ReplacesExistingBannerInPlace(t *testing.T) {
	original := []byte("<!-- lyx-stencil: sha256=" + fakeHash('a') + " -->\nbody\n")
	newHash := fakeHash('b')

	got := ApplyStamp(original, newHash)

	stamp, ok := ParseStamp(got)
	if !ok || stamp != newHash {
		t.Fatalf("ParseStamp(ApplyStamp(...)) = (%q, %v); want (%q, true)", stamp, ok, newHash)
	}
	if BodyHash(got) != BodyHash(original) {
		t.Errorf("ApplyStamp changed the body: BodyHash(got) = %q; want %q", BodyHash(got), BodyHash(original))
	}
}

func TestApplyStamp_PrependsNewBannerWhenAbsent(t *testing.T) {
	original := []byte("body with no banner\n")
	hash := fakeHash('c')

	got := ApplyStamp(original, hash)

	stamp, ok := ParseStamp(got)
	if !ok || stamp != hash {
		t.Fatalf("ParseStamp(ApplyStamp(...)) = (%q, %v); want (%q, true)", stamp, ok, hash)
	}
	if BodyHash(got) != BodyHash(original) {
		t.Errorf("ApplyStamp changed the body: BodyHash(got) = %q; want %q", BodyHash(got), BodyHash(original))
	}
}

func TestParseStamp(t *testing.T) {
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
			_, ok := ParseStamp([]byte(tt.content))
			if ok != tt.wantOK {
				t.Errorf("ParseStamp(%q) ok = %v; want %v", tt.content, ok, tt.wantOK)
			}
		})
	}
}

func TestClassify(t *testing.T) {
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Classify(tt.onDisk, tt.exists, shipped)
			if got != tt.want {
				t.Errorf("Classify(...) = %v; want %v", got, tt.want)
			}
		})
	}

	t.Run("ReconciledBeatsStaleStamp", func(t *testing.T) {
		staleHash := BodyHash([]byte("some other old default\n"))
		onDisk := ApplyStamp(shipped, staleHash)

		got := Classify(onDisk, true, shipped)
		if got != StateReconciled {
			t.Errorf("Classify(body==shipped, stale stamp) = %v; want %v", got, StateReconciled)
		}
	})
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
