package syntax

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// scanner.go is the in-house lexer for the CUE subset (plan 240). It turns
// source bytes into a token stream the parser consumes, replacing
// cuelang.org/go/cue/scanner. It tokenizes only the constructs the subset
// reaches: identifiers and keywords, int/float number literals (with the CUE
// `_` digit separators and 0x/0o/0b bases), the three string dialects (plain
// `"…"`, raw `#"…"#`, multiline `"""…"""`) WITH `\(…)` interpolation, the
// operator set, and the structural punctuation. A construct outside the subset
// (an attribute `@x`, a definition `#foo`, a `...` outside a list/struct) is
// either tokenized for the parser to reject or reported as a scan error here.

// tokKind enumerates the lexical categories the parser branches on. It is
// distinct from Token (the operator/literal set the evaluators read): a
// tokKind also covers punctuation (braces, brackets, commas) the parser
// consumes but never stores on a node.
type tokKind int

const (
	tEOF tokKind = iota
	tIdent
	tInt
	tFloat
	tString      // a complete string literal (no interpolation)
	tInterpStart // a string literal opening an interpolation: text up to the first \(
	tColon       // :
	tComma       // ,
	tQuestion    // ?
	tLParen      // (
	tRParen      // )
	tLBrace      // {
	tRBrace      // }
	tLBrack      // [
	tRBrack      // ]
	tDot         // .
	tEllipsis    // ...
	tAssign      // = (only in a let clause, which the subset rejects)
	tOp          // an operator (tok.op carries the Token)
)

// tok is one lexed token. text carries the raw source slice for an ident,
// number, or string (a string keeps its quotes so compileBasicLit can decode
// it); op carries the operator Token for a tOp.
type tok struct {
	kind  tokKind
	text  string
	op    Token
	bytes bool // a string/interpolation using the single-quote (bytes) dialect
}

// scanner walks the source bytes producing tokens on demand. interpStack
// tracks open string-interpolation dialects so a `)` closing an interpolation
// expression resumes the enclosing string fragment.
type scanner struct {
	src string
	pos int
	err error
	// interpStack holds the quote dialect of each open interpolation, innermost
	// last. A non-empty stack means the next `)` at depth 0 resumes a string
	// fragment rather than closing a paren group.
	interpStack []quoteDialect
}

// quoteDialect describes one string literal's delimiters so a resumed
// interpolation fragment is scanned with the same dialect. char is `"` (no
// bytes dialect is supported as a string; `'` is scanned but rejected later),
// numChar is 1 (plain/raw) or 3 (multiline), and hashes is the raw-string `#`
// count.
type quoteDialect struct {
	char    byte
	numChar int
	hashes  int
	// whitespace is the indentation prefix of a multiline string's closing-quote
	// line (CUE's QuoteInfo.whitespace). It is stripped after each interior
	// newline when decoding a multiline body; empty for a single-line dialect.
	whitespace string
}

// newScanner validates the source is UTF-8 with no NUL and returns a scanner
// positioned at the first byte. Invalid UTF-8 or an embedded NUL is a scan
// error up front, matching CUE's lexer, so the per-rune decode in the string
// scanners can assume valid UTF-8.
func newScanner(src string) (*scanner, error) {
	if !utf8.ValidString(src) {
		return nil, fmt.Errorf("cuelite: source is not valid UTF-8")
	}
	if strings.IndexByte(src, 0) >= 0 {
		return nil, fmt.Errorf("cuelite: source contains a NUL byte")
	}
	return &scanner{src: src}, nil
}

// next returns the next token, or a tEOF when the source is exhausted. A scan
// error is recorded on s.err and returned as a tEOF so the parser stops; the
// parser checks s.err after the stream ends.
func (s *scanner) next() tok {
	s.skipTrivia()
	if s.err != nil || s.pos >= len(s.src) {
		return tok{kind: tEOF}
	}
	c := s.src[s.pos]
	switch {
	case isIdentStart(c):
		return s.scanIdent()
	case c >= '0' && c <= '9':
		return s.scanNumber()
	case c == '"' || c == '\'' || c == '#':
		return s.scanString()
	}
	return s.scanPunct()
}

// skipTrivia advances past whitespace and `//` line comments. CUE's other
// comment and attribute forms are outside the subset; a `/*` is left for
// scanPunct to reject as an unexpected character.
func (s *scanner) skipTrivia() {
	for s.pos < len(s.src) {
		c := s.src[s.pos]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			s.pos++
		case c == '/' && s.pos+1 < len(s.src) && s.src[s.pos+1] == '/':
			s.pos += 2
			for s.pos < len(s.src) && s.src[s.pos] != '\n' {
				s.pos++
			}
		default:
			return
		}
	}
}

// isIdentStart reports whether c can start an identifier: a letter or
// underscore. CUE also allows a `#`/`_#` definition prefix and a `$`; the
// subset rejects definitions, so a leading `#` is handled by scanString's
// raw-string path or scanPunct, not here.
func isIdentStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// isIdentPart reports whether c can continue an identifier: a letter, digit,
// or underscore.
func isIdentPart(c byte) bool {
	return isIdentStart(c) || (c >= '0' && c <= '9')
}

// scanIdent scans an identifier or keyword. The bool/null keywords become
// their literal tokens via the parser; here every identifier is a tIdent and
// the parser maps the keyword names.
func (s *scanner) scanIdent() tok {
	start := s.pos
	for s.pos < len(s.src) && isIdentPart(s.src[s.pos]) {
		s.pos++
	}
	return tok{kind: tIdent, text: s.src[start:s.pos]}
}

// scanNumber scans an int or float literal, including the CUE `_` digit
// separators and the 0x/0o/0b integer bases. It captures the raw text;
// compileBasicLit parses and range-checks it (an SI suffix or out-of-int64
// literal is rejected there as out-of-subset). A `.` followed by a digit, or
// an `e`/`E` exponent, makes it a float.
func (s *scanner) scanNumber() tok {
	start := s.pos
	// A 0x/0o/0b prefix is always an integer; delegate to the base-prefix path.
	if s.src[s.pos] == '0' && s.pos+1 < len(s.src) {
		if t, ok := s.scanBasePrefixInt(start); ok {
			return t
		}
	}
	for s.pos < len(s.src) && (isDigitByte(s.src[s.pos]) || s.src[s.pos] == '_') {
		s.pos++
	}
	isFloat := s.scanFraction() || s.scanExponent()
	if isFloat {
		return tok{kind: tFloat, text: s.src[start:s.pos]}
	}
	return tok{kind: tInt, text: s.src[start:s.pos]}
}

// scanBasePrefixInt scans a 0x/0o/0b-prefixed integer literal when the current
// position starts with `0` followed by a base letter. It advances past the
// prefix and the base digits (hex digits for all three bases) and returns the
// tInt token and ok=true. It returns ok=false when the byte after `0` is not a
// recognised base letter, leaving s.pos unchanged.
func (s *scanner) scanBasePrefixInt(start int) (tok, bool) {
	switch s.src[s.pos+1] {
	case 'x', 'X', 'o', 'O', 'b', 'B':
		s.pos += 2
		for s.pos < len(s.src) && (isHexDigit(s.src[s.pos]) || s.src[s.pos] == '_') {
			s.pos++
		}
		return tok{kind: tInt, text: s.src[start:s.pos]}, true
	}
	return tok{}, false
}

// scanFraction advances past a `.digits` fractional part when one is present
// (a lone trailing `.` is the selector dot, not a fraction, so a digit after
// the `.` is required). It returns true when a fraction was consumed.
func (s *scanner) scanFraction() bool {
	if s.pos+1 >= len(s.src) || s.src[s.pos] != '.' || !isDigitByte(s.src[s.pos+1]) {
		return false
	}
	s.pos++ // consume '.'
	for s.pos < len(s.src) && (isDigitByte(s.src[s.pos]) || s.src[s.pos] == '_') {
		s.pos++
	}
	return true
}

// scanExponent advances past an `e[+-]digits` exponent part when one is
// present (the `e`/`E` must be followed by an optional sign and at least one
// digit). It returns true when an exponent was consumed.
func (s *scanner) scanExponent() bool {
	if s.pos >= len(s.src) || (s.src[s.pos] != 'e' && s.src[s.pos] != 'E') {
		return false
	}
	j := s.pos + 1
	if j < len(s.src) && (s.src[j] == '+' || s.src[j] == '-') {
		j++
	}
	if j >= len(s.src) || !isDigitByte(s.src[j]) {
		return false
	}
	s.pos = j
	for s.pos < len(s.src) && (isDigitByte(s.src[s.pos]) || s.src[s.pos] == '_') {
		s.pos++
	}
	return true
}

// isDigitByte reports whether c is an ASCII decimal digit.
func isDigitByte(c byte) bool { return c >= '0' && c <= '9' }

// isHexDigit reports whether c is an ASCII hex digit.
func isHexDigit(c byte) bool {
	return isDigitByte(c) || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

// scanPunct scans a punctuation token or operator. An unrecognised byte is a
// scan error so a construct outside the subset (an attribute, a `/*` comment)
// fails loudly rather than mis-tokenizing.
func (s *scanner) scanPunct() tok {
	c := s.src[s.pos]
	// Try a two-character operator first (==, !=, <=, >=, =~, !~).
	if s.pos+1 < len(s.src) {
		if t, ok := scanTwoCharOp(s.src[s.pos : s.pos+2]); ok {
			s.pos += 2
			return t
		}
	}
	// `...` ellipsis must be checked before the single `.` dot.
	if c == '.' && strings.HasPrefix(s.src[s.pos:], "...") {
		s.pos += 3
		return tok{kind: tEllipsis}
	}
	s.pos++
	return s.scanOneCharPunct(c)
}

// scanTwoCharOp maps a two-byte string to its operator token and ok=true for
// the six two-character CUE operators (==, !=, <=, >=, =~, !~). It returns
// ok=false for any other two-byte string so the caller falls through to
// single-character scanning.
func scanTwoCharOp(two string) (tok, bool) {
	switch two {
	case "==":
		return tok{kind: tOp, op: EQL}, true
	case "!=":
		return tok{kind: tOp, op: NEQ}, true
	case "<=":
		return tok{kind: tOp, op: LEQ}, true
	case ">=":
		return tok{kind: tOp, op: GEQ}, true
	case "=~":
		return tok{kind: tOp, op: MAT}, true
	case "!~":
		return tok{kind: tOp, op: NMAT}, true
	}
	return tok{}, false
}

// scanOneCharPunct maps a single ASCII byte to its punctuation or operator
// token. It delegates to scanStructuralPunct for the delimiter set and
// scanOperatorChar for the operator set. An unrecognised byte records a scan
// error and returns tEOF.
func (s *scanner) scanOneCharPunct(c byte) tok {
	if t, ok := scanStructuralPunct(c); ok {
		return t
	}
	if t, ok := scanOperatorChar(c); ok {
		return t
	}
	s.fail("unexpected character %q", string(c))
	return tok{kind: tEOF}
}

// scanStructuralPunct maps a single ASCII byte to a structural punctuation
// token (colon, comma, question mark, delimiters, dot) and ok=true. It returns
// ok=false for bytes that are not structural punctuation.
func scanStructuralPunct(c byte) (tok, bool) {
	switch c {
	case ':':
		return tok{kind: tColon}, true
	case ',':
		return tok{kind: tComma}, true
	case '?':
		return tok{kind: tQuestion}, true
	case '(':
		return tok{kind: tLParen}, true
	case ')':
		return tok{kind: tRParen}, true
	case '{':
		return tok{kind: tLBrace}, true
	case '}':
		return tok{kind: tRBrace}, true
	case '[':
		return tok{kind: tLBrack}, true
	case ']':
		return tok{kind: tRBrack}, true
	case '.':
		return tok{kind: tDot}, true
	}
	return tok{}, false
}

// scanOperatorChar maps a single ASCII operator byte to its tOp token and
// ok=true for the single-character operator set (|, &, +, -, *, !, <, >, =).
// A bare `=` maps to tAssign (not tOp) because it only appears in a `let`
// clause, which the evaluator rejects as unsupported; tokenizing it lets the
// parser build the let clause so that rejection fires. It returns ok=false for
// non-operator bytes.
func scanOperatorChar(c byte) (tok, bool) {
	switch c {
	case '|':
		return tok{kind: tOp, op: OR}, true
	case '&':
		return tok{kind: tOp, op: AND}, true
	case '+':
		return tok{kind: tOp, op: ADD}, true
	case '-':
		return tok{kind: tOp, op: SUB}, true
	case '*':
		return tok{kind: tOp, op: MUL}, true
	case '!':
		return tok{kind: tOp, op: NOT}, true
	case '<':
		return tok{kind: tOp, op: LSS}, true
	case '>':
		return tok{kind: tOp, op: GTR}, true
	case '=':
		return tok{kind: tAssign}, true
	}
	return tok{}, false
}

// fail records the first scan error, naming the construct. Subsequent next
// calls return tEOF so the parser stops and surfaces s.err.
func (s *scanner) fail(format string, args ...any) {
	if s.err == nil {
		s.err = fmt.Errorf("cuelite: "+format, args...)
	}
}
