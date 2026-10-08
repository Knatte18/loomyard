import unittest
from semver_range import max_satisfying, parse_version, compare_versions, Version


class TestSemverRange(unittest.TestCase):
    
    def test_example_1_caret(self):
        versions = ['1.2.3', '1.9.0', '2.0.0']
        result = max_satisfying(versions, '^1.2.3')
        self.assertEqual(result, '1.9.0')
    
    def test_example_2_caret_zero(self):
        versions = ['0.2.3', '0.2.9', '0.3.0']
        result = max_satisfying(versions, '^0.2.3')
        self.assertEqual(result, '0.2.9')
    
    def test_example_3_prerelease_not_matched(self):
        versions = ['1.2.3', '1.2.4-rc.1']
        result = max_satisfying(versions, '>=1.2.3')
        self.assertEqual(result, '1.2.3')
    
    def test_example_4_prerelease_matched(self):
        versions = ['1.2.4-rc.1', '1.2.4-rc.2']
        result = max_satisfying(versions, '>=1.2.4-rc.1')
        self.assertEqual(result, '1.2.4-rc.2')
    
    def test_example_5_or_range(self):
        versions = ['1.0.0', '2.5.0', '3.1.0']
        result = max_satisfying(versions, '<2.0.0 || >=3.0.0')
        self.assertEqual(result, '3.1.0')
    
    def test_example_6_hyphen_range(self):
        versions = ['1.2.5', '1.3.0', '2.0.0']
        result = max_satisfying(versions, '1.2 - 1.2.9')
        self.assertEqual(result, '1.2.5')
    
    def test_example_7_invalid_range(self):
        with self.assertRaises(ValueError):
            max_satisfying(['1.0.0'], '>=a.b')
    
    def test_exact_match(self):
        versions = ['1.2.3', '1.2.4', '1.2.5']
        result = max_satisfying(versions, '1.2.4')
        self.assertEqual(result, '1.2.4')
    
    def test_no_match(self):
        versions = ['1.2.3', '1.2.4']
        result = max_satisfying(versions, '2.0.0')
        self.assertIsNone(result)
    
    def test_empty_range_means_wildcard(self):
        versions = ['1.2.3', '2.0.0']
        result = max_satisfying(versions, '')
        self.assertEqual(result, '2.0.0')

    def test_whitespace_only_range(self):
        versions = ['1.2.3', '2.0.0']
        result = max_satisfying(versions, '   ')
        self.assertEqual(result, '2.0.0')

    def test_wildcard_range(self):
        versions = ['1.2.3', '2.0.0']
        result = max_satisfying(versions, '*')
        self.assertEqual(result, '2.0.0')
    
    def test_greater_than(self):
        versions = ['1.2.2', '1.2.3', '1.2.4']
        result = max_satisfying(versions, '>1.2.2')
        self.assertEqual(result, '1.2.4')
    
    def test_greater_than_or_equal(self):
        versions = ['1.2.2', '1.2.3', '1.2.4']
        result = max_satisfying(versions, '>=1.2.3')
        self.assertEqual(result, '1.2.4')
    
    def test_less_than(self):
        versions = ['1.2.2', '1.2.3', '1.2.4']
        result = max_satisfying(versions, '<1.2.4')
        self.assertEqual(result, '1.2.3')
    
    def test_less_than_or_equal(self):
        versions = ['1.2.2', '1.2.3', '1.2.4']
        result = max_satisfying(versions, '<=1.2.3')
        self.assertEqual(result, '1.2.3')
    
    def test_partial_version_x(self):
        versions = ['1.2.0', '1.2.5', '1.3.0']
        result = max_satisfying(versions, '1.2.x')
        self.assertEqual(result, '1.2.5')
    
    def test_partial_version_two_components(self):
        versions = ['1.2.0', '1.2.5', '1.3.0']
        result = max_satisfying(versions, '1.2')
        self.assertEqual(result, '1.2.5')
    
    def test_partial_version_one_component(self):
        versions = ['1.0.0', '1.5.0', '2.0.0']
        result = max_satisfying(versions, '1')
        self.assertEqual(result, '1.5.0')
    
    def test_tilde_range_full(self):
        versions = ['1.2.3', '1.2.4', '1.3.0']
        result = max_satisfying(versions, '~1.2.3')
        self.assertEqual(result, '1.2.4')
    
    def test_tilde_range_two_components(self):
        versions = ['1.2.0', '1.2.5', '1.3.0']
        result = max_satisfying(versions, '~1.2')
        self.assertEqual(result, '1.2.5')
    
    def test_tilde_range_one_component(self):
        versions = ['0.9.0', '1.5.0', '2.0.0']
        result = max_satisfying(versions, '~1')
        self.assertEqual(result, '1.5.0')
    
    def test_caret_zero_patch(self):
        versions = ['0.0.3', '0.0.4', '0.1.0']
        result = max_satisfying(versions, '^0.0.3')
        self.assertEqual(result, '0.0.3')
    
    def test_caret_zero_minor(self):
        versions = ['0.0.0', '0.1.0', '1.0.0']
        result = max_satisfying(versions, '^0')
        self.assertEqual(result, '0.1.0')
    
    def test_caret_zero_two_components(self):
        versions = ['0.0.0', '0.0.5', '0.1.0']
        result = max_satisfying(versions, '^0.0')
        self.assertEqual(result, '0.0.5')
    
    def test_hyphen_range_partial_right(self):
        versions = ['1.2.0', '1.2.5', '1.3.0', '1.4.0']
        result = max_satisfying(versions, '1.2 - 1.3')
        self.assertEqual(result, '1.3.0')
    
    def test_or_first_set_matches(self):
        versions = ['1.5.0', '2.5.0', '3.1.0']
        result = max_satisfying(versions, '<2.0.0 || >=3.0.0')
        self.assertEqual(result, '3.1.0')
    
    def test_or_both_sets_match(self):
        versions = ['1.0.0', '1.5.0', '2.0.0']
        result = max_satisfying(versions, '<1.0.0 || >2.0.0')
        self.assertIsNone(result)
    
    def test_invalid_versions_skipped(self):
        versions = ['not-a-version', '1.2.3', '1.2.4']
        result = max_satisfying(versions, '>=1.2.0')
        self.assertEqual(result, '1.2.4')
    
    def test_build_metadata_ignored(self):
        versions = ['1.2.3+build1', '1.2.3+build2', '1.2.4']
        result = max_satisfying(versions, '1.2.3')
        self.assertEqual(result, '1.2.3+build1')
    
    def test_prerelease_lower_than_release(self):
        v1 = parse_version('1.2.3-alpha')
        v2 = parse_version('1.2.3')
        self.assertLess(compare_versions(v1, v2), 0)
    
    def test_prerelease_comparison_numeric(self):
        v1 = parse_version('1.2.3-1')
        v2 = parse_version('1.2.3-2')
        self.assertLess(compare_versions(v1, v2), 0)
    
    def test_prerelease_comparison_alpha(self):
        v1 = parse_version('1.2.3-alpha')
        v2 = parse_version('1.2.3-beta')
        self.assertLess(compare_versions(v1, v2), 0)
    
    def test_prerelease_numeric_lower_than_alpha(self):
        v1 = parse_version('1.2.3-1')
        v2 = parse_version('1.2.3-alpha')
        self.assertLess(compare_versions(v1, v2), 0)
    
    def test_prerelease_longer_higher(self):
        v1 = parse_version('1.2.3-alpha')
        v2 = parse_version('1.2.3-alpha.1')
        self.assertLess(compare_versions(v1, v2), 0)
    
    def test_operator_with_space(self):
        versions = ['1.2.3', '1.2.4']
        result = max_satisfying(versions, '>= 1.2.3')
        self.assertEqual(result, '1.2.4')
    
    def test_multiple_comparators_and(self):
        versions = ['1.1.0', '1.2.0', '1.3.0', '1.4.0']
        result = max_satisfying(versions, '>=1.2.0 <1.4.0')
        self.assertEqual(result, '1.3.0')
    
    def test_greater_partial_major_only(self):
        versions = ['1.9.9', '2.0.0', '3.0.0']
        result = max_satisfying(versions, '>1')
        self.assertEqual(result, '3.0.0')
    
    def test_greater_partial_minor(self):
        versions = ['1.2.9', '1.3.0', '1.4.0']
        result = max_satisfying(versions, '>1.2')
        self.assertEqual(result, '1.4.0')
    
    def test_prerelease_with_prerelease_comparator(self):
        versions = ['1.2.3-alpha.1', '1.2.3-beta.1', '1.2.3']
        result = max_satisfying(versions, '>=1.2.3-alpha.0')
        self.assertEqual(result, '1.2.3')
    
    def test_prerelease_no_match_without_prerelease_comparator(self):
        versions = ['1.2.3-alpha.1', '1.2.3-beta.1']
        result = max_satisfying(versions, '>=1.2.3')
        self.assertIsNone(result)
    
    def test_caret_with_prerelease(self):
        versions = ['1.2.3-alpha.1', '1.2.3', '1.9.0']
        result = max_satisfying(versions, '^1.2.3-alpha.0')
        self.assertEqual(result, '1.9.0')
    
    def test_first_of_equal_builds_returned(self):
        versions = ['1.2.3+build1', '1.2.3+build2', '1.2.3+build3']
        result = max_satisfying(versions, '1.2.3')
        self.assertEqual(result, '1.2.3+build1')
    
    def test_complex_or_range(self):
        versions = ['0.5.0', '1.0.0', '1.5.0', '2.0.0', '2.5.0', '3.0.0']
        result = max_satisfying(versions, '0.x || 2.x || >=3')
        self.assertEqual(result, '3.0.0')
    
    def test_less_than_zero(self):
        versions = ['0.0.0', '0.0.1', '1.0.0']
        result = max_satisfying(versions, '<1.0.0')
        self.assertEqual(result, '0.0.1')
    
    def test_hyphen_range_full_left_partial_right(self):
        versions = ['1.2.3', '2.0.0', '2.5.0', '3.0.0']
        result = max_satisfying(versions, '1.2.3 - 2.5')
        self.assertEqual(result, '2.5.0')


if __name__ == '__main__':
    unittest.main()
