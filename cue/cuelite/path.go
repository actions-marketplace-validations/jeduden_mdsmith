package cuelite

import (
	"fmt"
	"slices"
	"strconv"
	"unicode/utf8"
)

// Path is a parsed CUE field path — an ordered sequence of unquoted
// string segments such as ["a", "b", "c"] for "a.b.c" or ["my-key",
// "sub"] for `"my-key".sub`. It is a value type: methods take and
// return Path by copy. A zero Path (no segments) is valid and its
// Segments() is nil.
//
// The segment representation is the UNQUOTED label string: a quoted
// segment like `"my-key"` yields the segment "my-key", stripping the
// quotes and applying Go string escapes. This is the same shape
// fieldinterp uses for map look-ups, so a Path from ParsePath feeds
// directly into ResolvePath without any extra unquoting step.
type Path struct {
	segments []string
}

// Segments returns the unquoted per-selector strings that make up the
// path, or nil for an empty path. The returned slice is a fresh copy so
// a caller that mutates it cannot corrupt the Path's internal state.
func (p Path) Segments() []string {
	return slices.Clone(p.segments)
}

// MakePath constructs a Path from the given unquoted segments — the
// in-house equivalent of cue.MakePath. It is the constructor consumers
// that build paths programmatically (query.collectPaths) need once they
// migrate off cuelang.org/go.
//
// MakePath does not validate the segment values; each segment is stored
// as-is. A zero-argument call returns a Path with nil segments.
func MakePath(segments ...string) Path {
	if len(segments) == 0 {
		return Path{}
	}
	return Path{segments: slices.Clone(segments)}
}

// ParsePath parses a CUE field-path expression into a Path whose
// Segments() are the unquoted per-selector strings. It is the in-house
// pure-Go implementation of the CUE path grammar mdsmith uses.
//
// The grammar accepted is a dot-separated sequence of selectors:
//
//   - An identifier segment: a letter ([a-zA-Z]) followed by zero or
//     more word characters ([a-zA-Z0-9_]). Digits-only segments (CUE
//     index labels), underscore-prefixed identifiers (CUE hidden
//     labels), and definition identifiers (# prefix) are rejected
//     because they are either not string labels in CUE or are
//     disallowed in CUE paths.
//   - A quoted string segment: a double-quoted string whose value is
//     the unquoted, escape-decoded content. Standard Go/JSON escape
//     sequences are supported (strconv.Unquote). An empty quoted
//     segment is rejected: an empty label yields a surprising key
//     look-up.
//
// ParsePath returns a *PathError (tagged with a nil path) on any input
// that is empty, has a leading or trailing dot, uses a separator after
// a quoted segment where none exists, has a malformed quoted string, or
// produces an empty-string decoded segment.
func ParsePath(expr string) (Path, error) {
	if expr == "" {
		return Path{}, newPathError(nil,
			"path expression must not be empty", nil)
	}
	segments, err := parsePathSegments(expr)
	if err != nil {
		return Path{}, err
	}
	return Path{segments: segments}, nil
}

// parsePathSegments splits expr into unquoted segment strings.
// It is the scanner driving ParsePath. The two segment kinds are
// idents and quoted strings; they alternate with '.' separators.
// A leading dot, a trailing dot, an empty quoted value, a
// malformed quoted string, an unexpected character, or an invalid
// identifier shape each return a *PathError with a nil path.
//
// The invariant "pos < len(expr) on entry to each segment parse" holds
// throughout: the caller (ParsePath) rejects the empty string before
// calling here, and after consuming a '.' separator the trailing-dot
// check guarantees at least one more character before the next segment
// is attempted.
func parsePathSegments(expr string) ([]string, error) {
	segments := make([]string, 0, 4)
	pos := 0
	for {
		// pos < len(expr) is guaranteed by the invariant above.
		r, _ := utf8.DecodeRuneInString(expr[pos:])
		switch {
		case r == '"':
			seg, advance, err := parseQuotedSegment(expr, pos)
			if err != nil {
				return nil, newPathError(nil,
					fmt.Sprintf("invalid path expression %q: %s", expr, err), err)
			}
			if seg == "" {
				return nil, newPathError(nil,
					fmt.Sprintf("invalid path expression %q: empty quoted segment", expr), nil)
			}
			segments = append(segments, seg)
			pos += advance
		case isIdentStart(r):
			seg, advance := parseIdentSegment(expr, pos)
			segments = append(segments, seg)
			pos += advance
		default:
			return nil, newPathError(nil,
				fmt.Sprintf("invalid path expression %q: unexpected character %q at position %d",
					expr, r, pos), nil)
		}
		if pos == len(expr) {
			break
		}
		if expr[pos] != '.' {
			return nil, newPathError(nil,
				fmt.Sprintf("invalid path expression %q: expected '.' or end, got %q at position %d",
					expr, rune(expr[pos]), pos), nil)
		}
		pos++ // consume '.'
		if pos == len(expr) {
			return nil, newPathError(nil,
				fmt.Sprintf("invalid path expression %q: trailing dot", expr), nil)
		}
	}
	return segments, nil
}

// isIdentStart reports whether r is a valid first character for a CUE
// string-label identifier: a letter in [a-zA-Z]. Underscore-prefixed
// identifiers are hidden labels, rejected by CUE paths. Digits-only
// tokens are index labels, also rejected.
func isIdentStart(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

// isIdentCont reports whether r may appear after the first character
// of a CUE string-label identifier: a letter, digit, or underscore.
func isIdentCont(r rune) bool {
	return isIdentStart(r) || (r >= '0' && r <= '9') || r == '_'
}

// parseIdentSegment reads an identifier segment starting at expr[pos]
// and returns the segment text and the number of bytes consumed.
// It assumes pos < len(expr) and expr[pos] satisfies isIdentStart.
func parseIdentSegment(expr string, pos int) (string, int) {
	start := pos
	pos++
	for pos < len(expr) {
		r, size := utf8.DecodeRuneInString(expr[pos:])
		if !isIdentCont(r) {
			break
		}
		pos += size
	}
	return expr[start:pos], pos - start
}

// parseQuotedSegment reads a double-quoted string starting at expr[pos]
// and returns the unquoted string, the number of bytes consumed (including
// both quotes), and any decoding error. It assumes pos < len(expr) and
// expr[pos] == '"'. It locates the closing '"' by scanning for it,
// respecting backslash escapes, then delegates to strconv.Unquote for
// the full escape-sequence decoding so all standard Go/JSON escapes
// (\n, \r, \t, \\, \", \uXXXX, etc.) are handled correctly.
func parseQuotedSegment(expr string, pos int) (string, int, error) {
	end := pos + 1 // skip opening '"'
	for end < len(expr) {
		c := expr[end]
		if c == '\\' {
			end += 2 // skip the escape and the next char
			continue
		}
		if c == '"' {
			end++ // include closing '"'
			raw := expr[pos:end]
			s, err := strconv.Unquote(raw)
			if err != nil {
				return "", 0, fmt.Errorf("malformed quoted segment %s: %w", raw, err)
			}
			return s, end - pos, nil
		}
		end++
	}
	return "", 0, fmt.Errorf("unterminated quoted segment starting at position %d", pos)
}
