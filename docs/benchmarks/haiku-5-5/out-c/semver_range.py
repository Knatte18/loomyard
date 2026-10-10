"""Resolve the highest version satisfying a semver range expression."""

import re

_NUM = r"0|[1-9][0-9]*"
_PART = r"x|X|\*|" + _NUM
_PRE_ID = r"(?:0|[1-9][0-9]*|[0-9]*[A-Za-z-][0-9A-Za-z-]*)"
_PRE = _PRE_ID + r"(?:\." + _PRE_ID + r")*"
_BUILD = r"[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*"

_VERSION_RE = re.compile(
    rf"({_NUM})\.({_NUM})\.({_NUM})(?:-({_PRE}))?(?:\+{_BUILD})?"
)
_PARTIAL_RE = re.compile(
    rf"({_PART})(?:\.({_PART})(?:\.({_PART})(?:-({_PRE}))?(?:\+{_BUILD})?)?)?"
)
_OP_RE = re.compile(r"(>=|<=|>|<|=)?(.*)", re.DOTALL)
_OPERATORS = (">=", "<=", ">", "<", "=")

# A version is (major, minor, patch, prerelease identifiers); () means no prerelease.
# A partial has None for each missing or wildcard component.
_NOTHING = ("<", (0, 0, 0, ()))


def _parse_version(text):
    """Return the version tuple for a strict version string, or None if invalid."""
    if not isinstance(text, str):
        return None
    match = _VERSION_RE.fullmatch(text)
    if not match:
        return None
    major, minor, patch, pre = match.groups()
    return (int(major), int(minor), int(patch), tuple(pre.split(".")) if pre else ())


def _key(version):
    """Return a sort key implementing semver 2.0.0 precedence."""
    major, minor, patch, pre = version
    ids = tuple((0, int(i)) if i.isdigit() else (1, i) for i in pre)
    return (major, minor, patch, not pre, ids)


def _parse_partial(text):
    match = _PARTIAL_RE.fullmatch(text)
    if not match:
        raise ValueError(f"invalid version in range: {text!r}")
    *nums, pre = match.groups()
    parts = []
    wild = False
    for n in nums:
        wild = wild or n is None or n in ("x", "X", "*")
        parts.append(None if wild else int(n))
    if pre and parts[2] is None:
        raise ValueError(f"prerelease on a wildcard version: {text!r}")
    return (*parts, tuple(pre.split(".")) if pre else ())


def _bounds(partial):
    """Return the (lower, upper) bounds of a partial missing its patch or minor."""
    major, minor, _, _ = partial
    if minor is None:
        return (major, 0, 0, ()), (major + 1, 0, 0, ())
    return (major, minor, 0, ()), (major, minor + 1, 0, ())


def _operator(op, partial):
    major, _, patch, _ = partial
    if major is None:
        return [] if op in ("=", ">=", "<=") else [_NOTHING]
    if patch is not None:
        return [(op, partial)]
    lower, upper = _bounds(partial)
    return {
        "=": [(">=", lower), ("<", upper)],
        ">": [(">=", upper)],
        ">=": [(">=", lower)],
        "<": [("<", lower)],
        "<=": [("<", upper)],
    }[op]


def _caret(partial):
    major, minor, patch, _ = partial
    if major is None:
        return []
    lower = (major, minor or 0, patch or 0, partial[3])
    if major > 0 or minor is None:
        upper = (major + 1, 0, 0, ())
    elif minor > 0 or patch is None:
        upper = (0, minor + 1, 0, ())
    else:
        upper = (0, 0, patch + 1, ())
    return [(">=", lower), ("<", upper)]


def _tilde(partial):
    major, minor, patch, pre = partial
    if major is None:
        return []
    lower = (major, minor or 0, patch or 0, pre)
    upper = (major + 1, 0, 0, ()) if minor is None else (major, minor + 1, 0, ())
    return [(">=", lower), ("<", upper)]


def _hyphen(low, high):
    return _operator(">=", _parse_partial(low)) + _operator("<=", _parse_partial(high))


def _parse_term(term):
    if isinstance(term, tuple):
        return _hyphen(*term)
    if term.startswith("^"):
        return _caret(_parse_partial(term[1:]))
    if term.startswith("~"):
        return _tilde(_parse_partial(term[1:]))
    op, rest = _OP_RE.fullmatch(term).groups()
    return _operator(op or "=", _parse_partial(rest))


def _group_terms(tokens):
    """Join split operators to their version and fold hyphen ranges into pairs."""
    terms = []
    i = 0
    while i < len(tokens):
        token = tokens[i]
        if token in _OPERATORS or token == "-":
            if i + 1 >= len(tokens):
                raise ValueError(f"dangling {token!r} in range")
            nxt = tokens[i + 1]
            i += 2
            if token != "-":
                terms.append(token + nxt)
                continue
            if not terms or not isinstance(terms[-1], str) or not _PARTIAL_RE.fullmatch(terms[-1]):
                raise ValueError("hyphen range needs a bare version before it")
            terms.append((terms.pop(), nxt))
        else:
            terms.append(token)
            i += 1
    return terms


def _parse_set(text):
    comparators = []
    for term in _group_terms(text.split()):
        comparators.extend(_parse_term(term))
    return comparators


def _holds(op, version, bound):
    a, b = _key(version), _key(bound)
    return {
        "=": a == b,
        ">": a > b,
        ">=": a >= b,
        "<": a < b,
        "<=": a <= b,
    }[op]


def _satisfies(version, comparators):
    if not all(_holds(op, version, bound) for op, bound in comparators):
        return False
    if version[3]:
        return any(bound[3] and bound[:3] == version[:3] for _, bound in comparators)
    return True


def max_satisfying(versions, range_expr):
    """Return the highest element of `versions` satisfying `range_expr`, or None.

    Invalid entries in `versions` are skipped; an invalid range raises ValueError.
    """
    sets = [_parse_set(part) for part in range_expr.split("||")]
    best = None
    best_key = None
    for text in versions:
        version = _parse_version(text)
        if version is None:
            continue
        if not any(_satisfies(version, comparators) for comparators in sets):
            continue
        if best is None or _key(version) > best_key:
            best, best_key = text, _key(version)
    return best
