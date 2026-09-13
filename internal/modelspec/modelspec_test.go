// modelspec_test.go pins the containment invariant between the two closed vocabularies declared in
// modelspec.go: every bracket key spelling in bracketKeys must normalize to a canonical key that
// knownParams also recognizes.

package modelspec

import "testing"

func TestBracketKeys_ContainedInKnownParams(t *testing.T) {
	for bracketSpelling, canonicalKey := range bracketKeys {
		if !knownParams[canonicalKey] {
			t.Errorf("bracketKeys[%q] = %q; canonical key not present in knownParams", bracketSpelling, canonicalKey)
		}
	}
}
