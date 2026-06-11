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
	tString        // a complete string literal (no interpolation)
	tInterpStart   // a string literal opening an interpolation: text up to the first \(
	tColon         // :
	tComma         // ,
	tQuestion      // ?
	tLParen        // (
	tRParen        // )
	tLBrace        // {
	tRBrace        // }
	tLBrack        // [
	tRBrack        // ]
	tDot           // .
	tEllipsis      // ...
	tAssign        // = (only in a let clause, which the subset rejects)
	tOp            // an operator (tok.op carries the Token)
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
	src   string
	pos   int
	err   error
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
	isFloat := false
	// A 0x/0o/0b prefix is always an integer; scan hex/base digits straight.
	if s.src[s.pos] == '0' && s.pos+1 < len(s.src) {
		switch s.src[s.pos+1] {
		case 'x', 'X', 'o', 'O', 'b', 'B':
			s.pos += 2
			for s.pos < len(s.src) && (isHexDigit(s.src[s.pos]) || s.src[s.pos] == '_') {
				s.pos++
			}
			return tok{kind: tInt, text: s.src[start:s.pos]}
		}
	}
	for s.pos < len(s.src) && (isDigitByte(s.src[s.pos]) || s.src[s.pos] == '_') {
		s.pos++
	}
	// A fraction `.digits` (a lone trailing `.` is the selector dot, not a
	// fraction, so require a digit after it).
	if s.pos+1 < len(s.src) && s.src[s.pos] == '.' && isDigitByte(s.src[s.pos+1]) {
		isFloat = true
		s.pos++
		for s.pos < len(s.src) && (isDigitByte(s.src[s.pos]) || s.src[s.pos] == '_') {
			s.pos++
		}
	}
	// An exponent `e[+-]digits`.
	if s.pos < len(s.src) && (s.src[s.pos] == 'e' || s.src[s.pos] == 'E') {
		j := s.pos + 1
		if j < len(s.src) && (s.src[j] == '+' || s.src[j] == '-') {
			j++
		}
		if j < len(s.src) && isDigitByte(s.src[j]) {
			isFloat = true
			s.pos = j
			for s.pos < len(s.src) && (isDigitByte(s.src[s.pos]) || s.src[s.pos] == '_') {
				s.pos++
			}
		}
	}
	if isFloat {
		return tok{kind: tFloat, text: s.src[start:s.pos]}
	}
	return tok{kind: tInt, text: s.src[start:s.pos]}
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
	two := ""
	if s.pos+1 < len(s.src) {
		two = s.src[s.pos : s.pos+2]
	}
	switch two {
	case "==":
		s.pos += 2
		return tok{kind: tOp, op: EQL}
	case "!=":
		s.pos += 2
		return tok{kind: tOp, op: NEQ}
	case "<=":
		s.pos += 2
		return tok{kind: tOp, op: LEQ}
	case ">=":
		s.pos += 2
		return tok{kind: tOp, op: GEQ}
	case "=~":
		s.pos += 2
		return tok{kind: tOp, op: MAT}
	case "!~":
		s.pos += 2
		return tok{kind: tOp, op: NMAT}
	}
	if c == '.' && strings.HasPrefix(s.src[s.pos:], "...") {
		s.pos += 3
		return tok{kind: tEllipsis}
	}
	s.pos++
	switch c {
	case ':':
		return tok{kind: tColon}
	case ',':
		return tok{kind: tComma}
	case '?':
		return tok{kind: tQuestion}
	case '(':
		return tok{kind: tLParen}
	case ')':
		return tok{kind: tRParen}
	case '{':
		return tok{kind: tLBrace}
	case '}':
		return tok{kind: tRBrace}
	case '[':
		return tok{kind: tLBrack}
	case ']':
		return tok{kind: tRBrack}
	case '.':
		return tok{kind: tDot}
	case '|':
		return tok{kind: tOp, op: OR}
	case '&':
		return tok{kind: tOp, op: AND}
	case '+':
		return tok{kind: tOp, op: ADD}
	case '-':
		return tok{kind: tOp, op: SUB}
	case '*':
		return tok{kind: tOp, op: MUL}
	case '!':
		return tok{kind: tOp, op: NOT}
	case '<':
		return tok{kind: tOp, op: LSS}
	case '>':
		return tok{kind: tOp, op: GTR}
	case '=':
		// A bare `=` only appears in a `let` clause, which the subset's evaluator
		// rejects as unsupported; tokenizing it (rather than failing the scan)
		// lets the parser build the let clause so that rejection fires.
		return tok{kind: tAssign}
	default:
		s.fail("unexpected character %q", string(c))
		return tok{kind: tEOF}
	}
}

// fail records the first scan error, naming the construct. Subsequent next
// calls return tEOF so the parser stops and surfaces s.err.
func (s *scanner) fail(format string, args ...any) {
	if s.err == nil {
		s.err = fmt.Errorf("cuelite: "+format, args...)
	}
}
