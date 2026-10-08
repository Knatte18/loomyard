import re
from typing import NamedTuple, Optional


class Version(NamedTuple):
    major: int
    minor: int
    patch: int
    prerelease: tuple[str, ...] = ()
    build: str = ""


class Comparator(NamedTuple):
    op: str
    version: Version
    from_prerelease: bool = False


def parse_version(s: str) -> Optional[Version]:
    """Parse a version string into a Version or None if invalid."""
    pattern = r'^(\d+)\.(\d+)\.(\d+)(?:-([a-zA-Z0-9.-]+))?(?:\+([a-zA-Z0-9.-]+))?$'
    match = re.match(pattern, s)
    if not match:
        return None
    
    major, minor, patch, prerelease, build = match.groups()
    major, minor, patch = int(major), int(minor), int(patch)
    
    if prerelease:
        parts = prerelease.split('.')
        pre_tuple = tuple(parts)
        for part in parts:
            if not part or not re.match(r'^[a-zA-Z0-9-]+$', part):
                return None
            if part.isdigit() and part != '0' and part[0] == '0':
                return None
    else:
        pre_tuple = ()
    
    return Version(major, minor, patch, pre_tuple, build or "")


def compare_prerelease(pre1: tuple[str, ...], pre2: tuple[str, ...]) -> int:
    """Compare prerelease tuples. Returns -1 if pre1 < pre2, 0 if equal, 1 if pre1 > pre2."""
    if not pre1 and not pre2:
        return 0
    if not pre1:
        return 1
    if not pre2:
        return -1
    
    for p1, p2 in zip(pre1, pre2):
        is_num1 = p1.isdigit()
        is_num2 = p2.isdigit()
        
        if is_num1 and is_num2:
            cmp = int(p1) - int(p2)
            if cmp != 0:
                return 1 if cmp > 0 else -1
        elif is_num1:
            return -1
        elif is_num2:
            return 1
        else:
            if p1 < p2:
                return -1
            elif p1 > p2:
                return 1
    
    if len(pre1) < len(pre2):
        return -1
    elif len(pre1) > len(pre2):
        return 1
    return 0


def compare_versions(v1: Version, v2: Version) -> int:
    """Compare two versions. Returns -1 if v1 < v2, 0 if equal, 1 if v1 > v2."""
    if v1.major != v2.major:
        return 1 if v1.major > v2.major else -1
    if v1.minor != v2.minor:
        return 1 if v1.minor > v2.minor else -1
    if v1.patch != v2.patch:
        return 1 if v1.patch > v2.patch else -1
    
    return compare_prerelease(v1.prerelease, v2.prerelease)


def parse_partial_version(s: str) -> tuple[Optional[int], Optional[int], Optional[int]]:
    """Parse a partial version. Returns tuple of (major, minor, patch) where None means wildcard."""
    s = s.strip()
    if not s:
        raise ValueError(f"Empty partial version")
    
    s = s.replace('X', 'x').replace('*', 'x')
    parts = s.split('.')
    
    major, minor, patch = None, None, None
    seen_wildcard = False
    
    for i, part in enumerate(parts):
        if i > 2:
            raise ValueError(f"Too many components in partial version: {s}")
        
        if part == 'x':
            seen_wildcard = True
            if i == 0:
                major = None
            elif i == 1:
                minor = None
            else:
                patch = None
        elif seen_wildcard:
            raise ValueError(f"Non-wildcard after wildcard in partial version: {s}")
        else:
            if not part.isdigit() or (part != '0' and part[0] == '0'):
                raise ValueError(f"Invalid version component: {part}")
            val = int(part)
            if i == 0:
                major = val
            elif i == 1:
                minor = val
            else:
                patch = val
    
    return major, minor, patch


def satisfies_comparator(version: Version, comparator: Comparator) -> bool:
    """Check if a version satisfies a single comparator."""
    op, comp_version, _ = comparator
    
    if op == '*':
        return True
    
    cmp = compare_versions(version, comp_version)
    
    if op == '=':
        return cmp == 0
    elif op == '>=':
        return cmp >= 0
    elif op == '>':
        return cmp > 0
    elif op == '<':
        return cmp < 0
    elif op == '<=':
        return cmp <= 0
    
    return False


def satisfies_set(version: Version, comparators: list[Comparator]) -> bool:
    """Check if a version satisfies all comparators in a set."""
    if not comparators or not any(c.op != '*' for c in comparators):
        return not version.prerelease
    
    for comparator in comparators:
        if not satisfies_comparator(version, comparator):
            return False
    
    if version.prerelease:
        has_matching_prerelease = False
        for comparator in comparators:
            if comparator.from_prerelease:
                v_mmp = Version(version.major, version.minor, version.patch)
                c_mmp = Version(comparator.version.major, comparator.version.minor, comparator.version.patch)
                if compare_versions(v_mmp, c_mmp) == 0:
                    has_matching_prerelease = True
                    break
        if not has_matching_prerelease:
            return False
    
    return True


def parse_comparator_set(set_str: str) -> list[Comparator]:
    """Parse a single comparator set (whitespace-separated terms)."""
    tokens = set_str.split()
    comparators = []
    i = 0
    
    while i < len(tokens):
        token = tokens[i]
        
        if token in ('||',):
            i += 1
            continue
        
        if token == '-' and i > 0 and i + 1 < len(tokens):
            left_str = tokens[i - 1]
            right_str = tokens[i + 1]
            
            left_parsed = parse_version(left_str)
            right_parsed = parse_version(right_str)
            left_major, left_minor, left_patch = parse_partial_version(left_str)
            right_major, right_minor, right_patch = parse_partial_version(right_str)
            
            if left_major is None:
                raise ValueError(f"Hyphen range with wildcard on left: {left_str}")
            if right_major is None:
                raise ValueError(f"Hyphen range with wildcard on right: {right_str}")
            
            left_version = Version(left_major, left_minor or 0, left_patch or 0)
            left_has_pre = left_parsed and left_parsed.prerelease
            
            right_has_pre = right_parsed and right_parsed.prerelease
            if right_patch is not None:
                right_comp = Comparator('<=', Version(right_major, right_minor or 0, right_patch), right_has_pre)
            elif right_minor is not None:
                right_comp = Comparator('<', Version(right_major, right_minor + 1, 0), right_has_pre)
            else:
                right_comp = Comparator('<', Version(right_major + 1, 0, 0), right_has_pre)
            
            comparators.append(Comparator('>=', left_version, left_has_pre))
            comparators.append(right_comp)
            
            i += 2
            continue
        
        if token.startswith('^'):
            partial = token[1:]
            parsed = parse_version(partial)
            has_prerelease = parsed and parsed.prerelease
            
            if parsed:
                major, minor, patch = parsed.major, parsed.minor, parsed.patch
            else:
                major, minor, patch = parse_partial_version(partial)
            
            if major is None:
                raise ValueError(f"Caret with wildcard: ^{partial}")
            
            if minor is None:
                upper_major = major + 1
                comparators.append(Comparator('>=', Version(major, 0, 0), has_prerelease))
                comparators.append(Comparator('<', Version(upper_major, 0, 0), has_prerelease))
            elif patch is None:
                upper_minor = minor + 1
                comparators.append(Comparator('>=', Version(major, minor, 0), has_prerelease))
                if major > 0:
                    comparators.append(Comparator('<', Version(major + 1, 0, 0), has_prerelease))
                else:
                    comparators.append(Comparator('<', Version(0, upper_minor, 0), has_prerelease))
            else:
                comparators.append(Comparator('>=', parsed if parsed else Version(major, minor, patch), has_prerelease))
                if major > 0:
                    comparators.append(Comparator('<', Version(major + 1, 0, 0), has_prerelease))
                elif minor > 0:
                    comparators.append(Comparator('<', Version(0, minor + 1, 0), has_prerelease))
                else:
                    comparators.append(Comparator('<', Version(0, 0, patch + 1), has_prerelease))
            i += 1
        
        elif token.startswith('~'):
            partial = token[1:]
            parsed = parse_version(partial)
            has_prerelease = parsed and parsed.prerelease

            if parsed:
                major, minor, patch = parsed.major, parsed.minor, parsed.patch
                minor_specified = True
            else:
                major, minor, patch = parse_partial_version(partial)
                minor_specified = minor is not None

            if major is None:
                raise ValueError(f"Tilde with wildcard: ~{partial}")

            comparators.append(Comparator('>=', parsed if parsed else Version(major, minor or 0, 0), has_prerelease))
            if minor_specified:
                comparators.append(Comparator('<', Version(major, minor + 1, 0), has_prerelease))
            else:
                comparators.append(Comparator('<', Version(major + 1, 0, 0), has_prerelease))
            i += 1
        
        elif token.startswith('>='):
            partial = token[2:].strip()
            if not partial and i + 1 < len(tokens):
                partial = tokens[i + 1]
                i += 1
            parsed = parse_version(partial)
            if parsed:
                comparators.append(Comparator('>=', parsed, bool(parsed.prerelease)))
            else:
                has_prerelease = False
                major, minor, patch = parse_partial_version(partial)
                
                if major is None:
                    comparators.append(Comparator('*', Version(0, 0, 0)))
                elif minor is None:
                    comparators.append(Comparator('>=', Version(major, 0, 0), has_prerelease))
                elif patch is None:
                    comparators.append(Comparator('>=', Version(major, minor, 0), has_prerelease))
                else:
                    comparators.append(Comparator('>=', Version(major, minor, patch), has_prerelease))
            i += 1
        
        elif token.startswith('>'):
            partial = token[1:].strip()
            if not partial and i + 1 < len(tokens):
                partial = tokens[i + 1]
                i += 1
            parsed = parse_version(partial)
            if parsed:
                comparators.append(Comparator('>', parsed, bool(parsed.prerelease)))
            else:
                major, minor, patch = parse_partial_version(partial)
                
                if major is None:
                    raise ValueError(f"Invalid partial version for >: {partial}")
                elif minor is None:
                    comparators.append(Comparator('>=', Version(major + 1, 0, 0)))
                elif patch is None:
                    comparators.append(Comparator('>=', Version(major, minor + 1, 0)))
                else:
                    comparators.append(Comparator('>', Version(major, minor, patch)))
            i += 1
        
        elif token.startswith('<='):
            partial = token[2:].strip()
            if not partial and i + 1 < len(tokens):
                partial = tokens[i + 1]
                i += 1
            parsed = parse_version(partial)
            if parsed:
                comparators.append(Comparator('<=', parsed, bool(parsed.prerelease)))
            else:
                major, minor, patch = parse_partial_version(partial)
                
                if major is None:
                    raise ValueError(f"Invalid partial version for <=: {partial}")
                elif minor is None:
                    comparators.append(Comparator('<', Version(major + 1, 0, 0)))
                elif patch is None:
                    comparators.append(Comparator('<', Version(major, minor + 1, 0)))
                else:
                    comparators.append(Comparator('<=', Version(major, minor, patch)))
            i += 1
        
        elif token.startswith('<'):
            partial = token[1:].strip()
            if not partial and i + 1 < len(tokens):
                partial = tokens[i + 1]
                i += 1
            parsed = parse_version(partial)
            if parsed:
                comparators.append(Comparator('<', parsed, bool(parsed.prerelease)))
            else:
                major, minor, patch = parse_partial_version(partial)
                
                if major is None:
                    raise ValueError(f"Invalid partial version for <: {partial}")
                elif minor is None:
                    comparators.append(Comparator('<', Version(major + 1, 0, 0)))
                elif patch is None:
                    comparators.append(Comparator('<', Version(major, minor + 1, 0)))
                else:
                    comparators.append(Comparator('<', Version(major, minor, patch)))
            i += 1
        
        elif token.startswith('='):
            partial = token[1:].strip()
            if not partial and i + 1 < len(tokens):
                partial = tokens[i + 1]
                i += 1
            parsed = parse_version(partial)
            if parsed:
                comparators.append(Comparator('=', parsed, bool(parsed.prerelease)))
            else:
                has_prerelease = False
                major, minor, patch = parse_partial_version(partial)
                
                if major is None:
                    comparators.append(Comparator('*', Version(0, 0, 0)))
                elif minor is None:
                    comparators.append(Comparator('>=', Version(major, 0, 0), has_prerelease))
                    comparators.append(Comparator('<', Version(major + 1, 0, 0), has_prerelease))
                elif patch is None:
                    comparators.append(Comparator('>=', Version(major, minor, 0), has_prerelease))
                    comparators.append(Comparator('<', Version(major, minor + 1, 0), has_prerelease))
                else:
                    comparators.append(Comparator('=', Version(major, minor, patch), has_prerelease))
            i += 1
        
        else:
            if token in ('*', 'x', 'X'):
                comparators.append(Comparator('*', Version(0, 0, 0)))
                i += 1
                continue

            parsed = parse_version(token)
            if parsed:
                if i + 1 < len(tokens) and tokens[i + 1] == '-':
                    i += 1
                    continue
                comparators.append(Comparator('=', parsed))
                i += 1
                continue

            major, minor, patch = parse_partial_version(token)
            if major is None:
                raise ValueError(f"Invalid version or operator: {token}")

            if i + 1 < len(tokens) and tokens[i + 1] == '-':
                i += 1
                continue

            if minor is None:
                comparators.append(Comparator('>=', Version(major, 0, 0)))
                comparators.append(Comparator('<', Version(major + 1, 0, 0)))
            elif patch is None:
                comparators.append(Comparator('>=', Version(major, minor, 0)))
                comparators.append(Comparator('<', Version(major, minor + 1, 0)))
            else:
                comparators.append(Comparator('=', Version(major, minor, patch)))

            i += 1
    
    return comparators


def parse_range_expr(expr: str) -> list[list[Comparator]]:
    """Parse a range expression into a list of comparator sets (||)."""
    if not expr or expr.isspace():
        return [[Comparator('*', Version(0, 0, 0))]]
    
    sets = []
    for set_str in expr.split('||'):
        set_str = set_str.strip()
        if not set_str:
            sets.append([Comparator('*', Version(0, 0, 0))])
            continue
        
        comparators = parse_comparator_set(set_str)
        if not comparators:
            comparators = [Comparator('*', Version(0, 0, 0))]
        sets.append(comparators)
    
    return sets


def max_satisfying(versions: list[str], range_expr: str) -> Optional[str]:
    """Find the highest version in versions that satisfies range_expr, or None."""
    try:
        comparator_sets = parse_range_expr(range_expr)
    except ValueError:
        raise ValueError(f"Invalid range expression: {range_expr}")
    
    valid_versions = []
    for ver_str in versions:
        parsed = parse_version(ver_str)
        if parsed:
            valid_versions.append((ver_str, parsed))
    
    best = None
    best_version = None
    
    for ver_str, parsed in valid_versions:
        for comparators in comparator_sets:
            if satisfies_set(parsed, comparators):
                if best_version is None or compare_versions(parsed, best_version) > 0:
                    best = ver_str
                    best_version = parsed
                break
    
    return best
