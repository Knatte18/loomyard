import unittest

from semver_range import max_satisfying


class MaxSatisfyingTest(unittest.TestCase):
    def check(self, versions, expr, want):
        self.assertEqual(max_satisfying(versions.split(), expr), want)

    def test_caret_full(self):
        self.check("1.2.3 1.9.0 2.0.0", "^1.2.3", "1.9.0")

    def test_caret_zero_minor(self):
        self.check("0.2.3 0.2.9 0.3.0", "^0.2.3", "0.2.9")

    def test_caret_zero_zero(self):
        self.check("0.0.3 0.0.4 0.1.0", "^0.0.3", "0.0.3")

    def test_caret_partials(self):
        self.check("0.0.9 0.1.0 1.0.0", "^0.0", "0.0.9")
        self.check("0.9.0 1.0.0", "^0", "0.9.0")
        self.check("1.2.0 1.9.0 2.0.0", "^1.2", "1.9.0")
        self.check("1.1.9 1.2.0", "^1.2", "1.2.0")

    def test_caret_prerelease_base(self):
        self.check("1.2.3-beta.1 1.2.3-beta.2 1.2.3-beta.3 1.2.4-rc.1", "^1.2.3-beta.2", "1.2.3-beta.3")
        self.check("1.2.3-beta.1 1.2.3", "^1.2.3-beta.2", "1.2.3")

    def test_prerelease_excluded_without_comparator(self):
        self.check("1.2.3 1.2.4-rc.1", ">=1.2.3", "1.2.3")
        self.check("1.3.0-alpha", ">=1.2.3-beta.1 <2.0.0", None)

    def test_prerelease_same_tuple_allowed(self):
        self.check("1.2.3-beta.2", ">=1.2.3-beta.1 <2.0.0", "1.2.3-beta.2")
        self.check("1.2.4-rc.1 1.2.4-rc.2", ">=1.2.4-rc.1", "1.2.4-rc.2")

    def test_star_never_matches_prerelease(self):
        self.check("1.0.0-alpha", "*", None)
        self.check("1.0.0-alpha 0.1.0", "*", "0.1.0")

    def test_prerelease_rule_per_set(self):
        self.check("1.2.3-beta", ">=1.2.3-alpha || <1.0.0", "1.2.3-beta")
        self.check("1.2.3-beta", ">=1.0.0 || <1.2.3-alpha", None)

    def test_or_sets(self):
        self.check("1.0.0 2.5.0 3.1.0", "<2.0.0 || >=3.0.0", "3.1.0")
        self.check("1.0.0 2.5.0", ">=3.0.0 || <2.0.0", "1.0.0")

    def test_hyphen_ranges(self):
        self.check("1.2.5 1.3.0 2.0.0", "1.2 - 1.2.9", "1.2.5")
        self.check("1.1.9 1.2.0 2.3.4 2.3.5", "1.2 - 2.3.4", "2.3.4")
        self.check("2.3.9 2.4.0", "1.2.3 - 2.3", "2.3.9")
        self.check("0.5.0 1.0.0", "1 - 1", "1.0.0")

    def test_tilde(self):
        self.check("1.2.3 1.2.9 1.3.0", "~1.2.3", "1.2.9")
        self.check("1.2.0 1.2.9 1.3.0", "~1.2", "1.2.9")
        self.check("1.0.0 1.9.0 2.0.0", "~1", "1.9.0")

    def test_operators_with_partials(self):
        self.check("1.2.9 1.3.0 1.4.0", ">1.2", "1.4.0")
        self.check("1.2.9 1.3.0", ">1.2", "1.3.0")
        self.check("1.9.0 2.0.0", ">1", "2.0.0")
        self.check("1.9.0 2.0.0", "<=1", "1.9.0")
        self.check("1.2.9 1.3.0", "<=1.2", "1.2.9")
        self.check("1.1.9 1.2.0", "<1.2", "1.1.9")
        self.check("1.1.9 1.2.0", ">=1.2", "1.2.0")

    def test_bare_and_wildcard_partials(self):
        self.check("1.1.0 1.2.0 1.2.7 1.3.0", "1.2", "1.2.7")
        self.check("1.2.7 1.3.0", "=1.2.x", "1.2.7")
        self.check("1.5.0 2.0.0", "=1", "1.5.0")
        self.check("1.5.0 2.0.0", "1.X", "1.5.0")
        self.check("1.5.0 2.0.0", "*", "2.0.0")
        self.check("1.5.0 2.0.0", "x.x", "2.0.0")

    def test_whitespace_after_operator(self):
        self.check("1.2.2 1.2.3", ">= 1.2.3", "1.2.3")
        self.check("1.2.2 1.2.3 1.4.0", ">= 1.2.3 < 1.3", "1.2.3")

    def test_empty_range_means_any(self):
        self.check("1.0.0 2.0.0", "", "2.0.0")
        self.check("1.0.0 2.0.0", "   ", "2.0.0")
        self.check("1.0.0 2.0.0", ">=3.0.0 ||", "2.0.0")

    def test_invalid_versions_skipped(self):
        self.check("v1.0.0 1.0 01.0.0 1.0.0-01 junk 1.2.0", "*", "1.2.0")
        self.check("1.0.0.0 1.0.0", "*", "1.0.0")

    def test_invalid_ranges_raise(self):
        for expr in [">=a.b", "1.2.3.4", ">=", "^", "~x.y", "1.2.x-beta", "01.2.3", "1 -", "- 1", "<>1", "1 - 2 - 3", ">= 1 - 2"]:
            with self.subTest(expr=expr):
                with self.assertRaises(ValueError):
                    max_satisfying(["1.0.0"], expr)

    def test_invalid_range_raises_with_no_versions(self):
        with self.assertRaises(ValueError):
            max_satisfying([], ">=a.b")

    def test_prerelease_ordering(self):
        vs = "1.0.0-alpha 1.0.0-alpha.1 1.0.0-alpha.beta 1.0.0-beta 1.0.0-beta.2 1.0.0-beta.11 1.0.0-rc.1 1.0.0"
        for expr, want in [
            (">=1.0.0-alpha <1.0.0", "1.0.0-rc.1"),
            ("<1.0.0-beta >=1.0.0-alpha", "1.0.0-alpha.beta"),
            ("<1.0.0-beta.11 >=1.0.0-beta", "1.0.0-beta.2"),
            ("<1.0.0-alpha.1 >=1.0.0-alpha", "1.0.0-alpha"),
            ("<1.0.0-alpha.beta >=1.0.0-alpha", "1.0.0-alpha.1"),
        ]:
            with self.subTest(expr=expr):
                self.check(vs, expr, want)

    def test_numeric_ordering_not_lexical(self):
        self.check("1.2.10 1.2.9 1.10.0 1.9.0", "^1.0.0", "1.10.0")

    def test_build_ignored_first_maximum_returned(self):
        self.check("1.0.0+b 1.0.0+a 0.9.0", "*", "1.0.0+b")
        self.check("1.0.0+a 1.0.0+b", "=1.0.0", "1.0.0+a")
        self.check("1.0.0+exp.sha.5114f85", "^1.0.0", "1.0.0+exp.sha.5114f85")

    def test_exact_prerelease_equals(self):
        self.check("1.0.0-rc.1 1.0.0-rc.2 1.0.0", "1.0.0-rc.1", "1.0.0-rc.1")

    def test_no_match_returns_none(self):
        self.check("1.0.0 2.0.0", ">=3.0.0", None)
        self.assertIsNone(max_satisfying([], "*"))

    def test_returns_element_as_given(self):
        versions = ["1.2.3+build.5", "1.2.2"]
        self.assertIs(max_satisfying(versions, "^1.0.0"), versions[0])


if __name__ == "__main__":
    unittest.main()
