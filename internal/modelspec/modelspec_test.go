// modelspec_test.go pins the containment invariant between the two closed vocabularies declared in
// modelspec.go: every bracket key spelling in bracketKeys must normalize to a canonical key that
// knownParams also recognizes.

package modelspec

import "testing"

// TestBracketKeys_ContainedInKnownParams asserts the containment over the live vocabularies.
//
//testtiming:keep pins that every bracketKeys spelling normalizes to a knownParams key, which the template load never reaches
func TestBracketKeys_ContainedInKnownParams(t *testing.T) {
	t.Parallel()
	for bracketSpelling, canonicalKey := range bracketKeys {
		if !knownParams[canonicalKey] {
			t.Errorf("bracketKeys[%q] = %q; canonical key not present in knownParams", bracketSpelling, canonicalKey)
		}
	}
}
