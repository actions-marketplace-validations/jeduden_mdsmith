package cuelite

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// parseMultilineSegment reads a CUE multiline string label starting at
// expr[pos]. The opener is a run of `hashes` '#' (already counted by the
// caller) followed by three quotes ("""), so expr[pos : pos+hashes] is the
// '#' run and expr[pos+hashes : pos+hashes+3] is `"""`. It returns the
// decoded label, the number of bytes consumed (opener through the closing
// `"""`+'#'-run), and any error.
//
// CUE multiline-string semantics (cue/literal):
//
//   - The opener `"""` (or '#'×N + `"""`) MUST be followed immediately by a
//     newline ('\n' or '\r\n'); any other byte after it — even a space — is
//     "opening quote of multiline string must be followed by newline".
//   - The closing line is the final line: the bytes between its last newline
//     and the closing `"""`+hashes must be only whitespace, which becomes the
//     INDENTATION prefix. That prefix must start every content line and is
//     stripped from each; a content line lacking it makes the literal decode
//     to the empty string (which the empty-segment check then rejects),
//     matching cue.ParsePath's Unquoted() — which maps the literal's
//     "invalid whitespace" error to "".
//   - The newline that precedes the closing line is excluded from the value.
//   - Escapes follow the same dialect as a single-line string at the same hash
//     level (\n \t \" \uXXXX … at level 0; \#n \#t … at level N), and surrogate
//     escape pairs combine per CUE's UTF-16 rule.
//
// Any malformed multiline literal whose CUE Unquoted() value is "" is reported
// here as an empty segment (decoded == ""), so the empty-segment check in
// consumeQuotedSegment/consumeRawStringSegment rejects it — the same outcome
// as the oracle.
func parseMultilineSegment(expr string, pos, hashes int) (string, int, error) {
	openerLen := hashes + 3 // '#'×N + `"""`
	bodyStart := pos + openerLen
	closeDelim := `"""` + strings.Repeat("#", hashes)
	rel := multilineCloseIndex(expr[bodyStart:], hashes, closeDelim)
	if rel < 0 {
		return "", 0, fmt.Errorf(
			"unterminated multiline string segment starting at position %d", pos)
	}
	// token is the full literal including the opener and closing delimiter, as
	// CUE's ParseQuotes expects (start == end == the whole token).
	tokenEnd := bodyStart + rel + len(closeDelim)
	token := expr[pos:tokenEnd]
	// A malformed multiline literal (bad opener/indent/escape) decodes to "",
	// which the empty-segment check in consumeMultilineSegment rejects — the
	// same outcome as cue.ParsePath's Unquoted(). Only the unterminated case
	// above is a structural error worth its own message.
	return unquoteMultiline(token, hashes), tokenEnd - pos, nil
}

// multilineCloseIndex returns the byte offset within body of the closing
// `"""`+N'#' delimiter of a hash-level-N multiline string, or -1 when none
// is found. It scans left-to-right, skipping each escape sequence
// ('\' + N '#' + one selector byte) so an escaped quote run is never read as
// the close. closeDelim is the precomputed `"""`+N'#' delimiter.
func multilineCloseIndex(body string, hashes int, closeDelim string) int {
	hashRun := strings.Repeat("#", hashes)
	for i := 0; i < len(body); {
		if body[i] == '\\' && strings.HasPrefix(body[i+1:], hashRun) {
			if i+1+hashes >= len(body) {
				return -1
			}
			i += 2 + hashes
			continue
		}
		if strings.HasPrefix(body[i:], closeDelim) {
			return i
		}
		i++
	}
	return -1
}

// unquoteMultiline decodes a full CUE multiline string TOKEN (opener through
// closing delimiter) at hash level `hashes` into its label value. It ports
// CUE's cue/literal multiline algorithm for the string-label subset: the
// quote char is always '"', interpolation ('\(') cannot appear in a path, and
// the \x/octal escapes are rejected (CUE rejects them for double-quoted
// strings). On any malformed-literal condition CUE's path Unquoted() yields
// "", so this returns "" and lets the caller's empty-segment check reject.
func unquoteMultiline(token string, hashes int) string {
	// CUE's scanner makes the opener-newline and escape decisions on the RAW
	// bytes (CR included): the scanner only sets hasCR and runs stripCR on the
	// final literal AFTER scanning (scanner.scanString, stripCR at 429/453),
	// while the opener-newline check (a multiline opener must be followed by a
	// lone '\n' or exactly one '\r\n' — scanner 813-829) and scanEscape (which
	// reads the escape selector and any \u/\U hex digits — scanner 352-413) run
	// on the unstripped source. So a CR run at the opener, a CR between the
	// backslash and the escape selector, or a CR among hex digits is a scan
	// error CUE reports, NOT a stripped-away no-op. Validate the opener and the
	// escapes on the raw token first; only then strip CR for value assembly, so
	// CRLF line endings and bare CRs inside content still decode as if absent —
	// matching cue.ParsePath exactly. (A single-line string keeps its CR, which
	// the single-line decoder rejects — that path never reaches here.)
	if !rawMultilineOK(token, hashes) {
		return ""
	}
	token = stripCR(token)
	// CUE's scanner finds the token END on RAW bytes (consumeStringClose never
	// strips CR), so a CR breaking a `"""`+'#' run let the raw token run on to a
	// LATER close. But the literal CUE then hands to literal.Unquote is the
	// CR-STRIPPED token, and literal.Unquote decodes forward and terminates at the
	// FIRST close it reaches — accepting it ONLY when that close is the whole
	// remaining tail (unquoteChar's `len(s) != ln` → errSyntax otherwise). So a CR
	// that fuses an EARLIER `"""`+'#' run is fatal: literal.Unquote hits that
	// earlier close with bytes still after it and yields "". Re-find the first
	// close on the stripped token; when stripCR moved it earlier than the raw end
	// the value is "", and when it sits at the raw end (no CR fused a close, the
	// common case) decoding proceeds normally.
	closeLen := hashes + 3
	openerLen := hashes + 3
	closeDelim := `"""` + strings.Repeat("#", hashes)
	// The raw scan matched a CR-free `"""`+'#' run (a CR inside the run would have
	// broken the match), and a CR-free run survives stripCR unchanged, so a close
	// always remains here: rel is never negative.
	rel := multilineCloseIndex(token[openerLen:], hashes, closeDelim)
	if openerLen+rel != len(token)-closeLen {
		// A CR fused an earlier `"""`+'#' run: literal.Unquote terminates at it
		// with content still following, so its Unquoted() is "" — rejected.
		return ""
	}
	ws, contentStart, ok := multilineWhitespace(token, hashes)
	if !ok {
		// Opener not followed by a newline, or the closing line carries
		// non-whitespace: CUE's Unquoted() is "".
		return ""
	}
	// The content region is everything from the first content byte up to (but
	// excluding) the closing delimiter; the close sits at the very end of the
	// token (the check above), so trimming its length leaves exactly the content
	// lines plus the final newline and the closing line's indentation.
	content := token[contentStart : len(token)-closeLen]
	decoded, decodeOK := decodeMultilineBody(content, ws, hashes)
	if !decodeOK {
		return ""
	}
	return decoded
}

// rawMultilineOK validates, on the RAW (CR-bearing) token, the two decisions
// CUE's scanner makes before stripCR: the opener-newline rule and escape
// well-formedness. It returns false (so unquoteMultiline yields "" and the
// empty-segment check rejects) when the opener is not followed by a lone '\n'
// or exactly one '\r\n', or when any escape in the body fails a raw scanEscape
// (an unknown selector, or a CR — U+000D — sitting where the selector or a
// \u/\U hex digit must be). The close delimiter is found by the same escape-
// aware multilineCloseIndex scan on the raw token, so the content region here
// matches the scanner's. Precondition: token starts with '#'×hashes + `"""`
// and ends with `"""` + '#'×hashes (parseMultilineSegment guarantees both).
func rawMultilineOK(token string, hashes int) bool {
	openerLen := hashes + 3
	after := token[openerLen:]
	// The opener must be followed by a lone '\n' or exactly one '\r\n'. A CR
	// run ('\r\r\n') or a '\r' not followed by '\n' is "expected newline after
	// multiline quote", which CUE maps to an empty Unquoted().
	switch {
	case strings.HasPrefix(after, "\n"):
	case strings.HasPrefix(after, "\r\n"):
	default:
		return false
	}
	// Scan the raw content region (opener-newline through the byte before the
	// close) for escape validity, exactly as CUE's scanString → scanEscape do.
	closeLen := hashes + 3
	body := token[openerLen : len(token)-closeLen]
	return rawEscapesOK(body, hashes)
}

// rawEscapesOK reports whether every backslash escape in the raw multiline
// body scans cleanly per CUE's scanEscape at hash level `hashes`. It walks the
// raw bytes (CR included): a '\' begins an escape only when followed by exactly
// `hashes` '#'; a '\' whose hash run is broken (e.g. by a CR at level ≥1) is a
// literal backslash CUE accepts, so the walk advances one byte and continues.
// A well-formed introducer is followed by a selector that scanEscape must
// accept, and a \u/\U selector consumes its fixed run of hex digits — a CR
// among them is an "illegal character in escape sequence" CUE rejects. Any
// scanEscape failure returns false, matching the oracle's rejection.
func rawEscapesOK(body string, hashes int) bool {
	hashRun := strings.Repeat("#", hashes)
	for i := 0; i < len(body); {
		if body[i] != '\\' || !strings.HasPrefix(body[i+1:], hashRun) {
			// Not an escape introducer (a literal backslash, or any other byte):
			// CUE copies it verbatim. Advance one byte and keep scanning.
			i++
			continue
		}
		width, ok := rawEscapeScan(body, i, hashes)
		if !ok {
			return false
		}
		i += width
	}
	return true
}

// rawEscapeScan mirrors CUE's scanEscape on the raw body starting at the
// introducer body[i] ('\' followed by `hashes` '#'). It returns the number of
// raw bytes the escape consumes and whether it scans cleanly. The selector
// byte sits at body[i+1+hashes]; a 'u'/'U' selector then consumes 4 or 8 hex
// digits, and a non-hex byte there (including a raw CR) is the rejection CUE
// reports as "illegal character ... in escape sequence". A \u/\U whose hex run
// would extend past the body end (the close delimiter starts mid-run) is a
// truncated escape, also rejected.
//
// Precondition: body[i]=='\\' and body[i+1 : i+1+hashes] is the '#' run, and
// the selector byte body[i+1+hashes] is in range. The caller only enters here
// after strings.HasPrefix(body[i+1:], hashRun), so the '#' run is present, and
// the escape-aware multilineCloseIndex already rejects (as unterminated) a
// '\'+'#'-run that reaches the body end with no selector — so the selector
// index never equals len(body) here, the same invariant decodeMultilineChar
// and rawUnquote rely on.
func rawEscapeScan(body string, i, hashes int) (int, bool) {
	sel := i + escBackslash + hashes
	c := body[sel]
	n := 0
	switch c {
	case 'a', 'b', 'f', 'n', 'r', 't', 'v', '\\', '/', '"':
		return escBackslash + hashes + 1, true
	case 'u':
		n = 4
	case 'U':
		n = 8
	default:
		// An unknown selector — including a raw CR (the level-0 "\<CR>n" case,
		// which CUE reports as "unknown escape sequence") — is rejected.
		return 0, false
	}
	digits := sel + 1
	if digits+n > len(body) {
		return 0, false
	}
	for j := 0; j < n; j++ {
		if _, isHex := hexVal(body[digits+j]); !isHex {
			// A non-hex byte among the \u/\U digits — a raw CR is the
			// "illegal character U+000D in escape sequence" case — is rejected.
			return 0, false
		}
	}
	return digits + n - i, true
}

// multilineWhitespace ports the multiline arm of CUE's ParseQuotes for the
// string-label subset. It returns the indentation prefix `ws` (the whitespace
// the closing line carries before its `"""`+hashes), the byte offset
// contentStart in token where the body begins (just past the opener's newline
// and the leading copy of ws), and ok=false when the closing line is not
// whitespace-prefixed by a real newline. The opener-newline rule is already
// enforced on the raw token by rawMultilineOK before unquoteMultiline strips
// CR and calls this, so by here the opener is followed by a single '\n'.
// Precondition: token starts with '#'×hashes + `"""`, ends with `"""` +
// '#'×hashes, and (rawMultilineOK having passed) token[openerLen]=='\n'.
func multilineWhitespace(token string, hashes int) (ws string, contentStart int, ok bool) {
	openerLen := hashes + 3
	const nlLen = 1 // the opener's single '\n' (CR already stripped)
	// Walk back from just before the closing delimiter over trailing spaces to
	// the newline that ends the last content line; the spaces are the indent.
	closeLen := openerLen // `"""`+hashes has the same length as the opener
	end := len(token) - closeLen
	i := end
	hasNewline := false
	for i > 0 {
		r, size := utf8.DecodeLastRuneInString(token[:i])
		if r == '\n' || !unicode.IsSpace(r) {
			hasNewline = r == '\n'
			break
		}
		i -= size
	}
	if !hasNewline {
		return "", 0, false
	}
	ws = token[i:end]
	contentStart = openerLen + nlLen
	// The first content line must carry the indent prefix (unless it is itself
	// the closing newline, i.e. empty content).
	if contentStart < len(token) && token[contentStart] != '\n' {
		if !strings.HasPrefix(token[contentStart:], ws) {
			return "", 0, false
		}
		contentStart += len(ws)
	}
	return ws, contentStart, true
}

// decodeMultilineBody decodes the content region s of a multiline string (the
// content lines plus the final newline and the closing line's indentation, but
// NOT the closing delimiter itself) given the indentation prefix ws and hash
// level hashes. It ports CUE's literal.Unquote loop for the string-label subset
// and returns ok=false when the literal is malformed (an unknown escape, a bad
// indent on a continuation line, a lone surrogate, or an escaped final newline)
// — CUE's path Unquoted() maps all of these to "". The final newline before the
// closing line is dropped, matching CUE's stripNL rule.
//
// An ESCAPED newline ('\' + N '#' + '\n') is a line continuation that elides
// the newline (CUE's escapedNewline): it appends nothing, skips the next
// line's indentation, and — when it is the FINAL newline before the close —
// rejects (CUE's errEscapedLastNewline). This path is reachable only when the
// raw scanner accepted a literal backslash that stripCR then fused with a '#'
// run into an escape introducer (a '\'+CR+'#' content sequence), exactly the
// asymmetry cue.ParsePath exhibits between its scanner and literal.Unquote.
func decodeMultilineBody(s, ws string, hashes int) (string, bool) {
	var b strings.Builder
	stripNL := false
	wasEscapedNewline := false
	for len(s) > 0 {
		if s[0] == '\n' {
			rest, wsOK := skipIndentAfterNewline(s[1:], ws)
			if !wsOK {
				return "", false
			}
			s = rest
			stripNL = true
			wasEscapedNewline = false
			b.WriteByte('\n')
			continue
		}
		if rest, ok := escapedNewlineTail(s, ws, hashes); ok {
			if rest == nil {
				// The skipped indentation was malformed: CUE's
				// skipWhitespaceAfterNewline errors, mapping to an empty value.
				return "", false
			}
			s = *rest
			wasEscapedNewline = true
			stripNL = false
			continue
		}
		r, width, ok := decodeMultilineChar(s, hashes)
		if !ok {
			return "", false
		}
		b.WriteRune(r)
		s = s[width:]
		stripNL = false
		wasEscapedNewline = false
	}
	if wasEscapedNewline {
		// The last newline before the close was escaped: CUE rejects this as
		// errEscapedLastNewline, an empty Unquoted() the empty-segment check
		// then rejects.
		return "", false
	}
	out := b.String()
	if stripNL && len(out) > 0 {
		// Drop the newline that preceded the closing line.
		out = out[:len(out)-1]
	}
	return out, true
}

// escapedNewlineTail reports whether s begins with an escaped-newline line
// continuation ('\' + N '#' + '\n') at hash level `hashes`. When it does, it
// returns a pointer to the remaining body with the elided newline AND the next
// line's indentation consumed (ok=true, rest non-nil); when the indentation
// after the newline is malformed (CUE's skipWhitespaceAfterNewline error) it
// returns ok=true with rest==nil so the caller rejects; when s is not an
// escaped newline it returns ok=false. The body is CR-free here (stripCR ran
// in unquoteMultiline), so only a bare '\n' follows the introducer — CUE's
// '\r\n' arm of escapedNewline cannot occur after stripping.
func escapedNewlineTail(s, ws string, hashes int) (*string, bool) {
	if s[0] != '\\' || !strings.HasPrefix(s[1:], strings.Repeat("#", hashes)) {
		return nil, false
	}
	sel := 1 + hashes
	if sel >= len(s) || s[sel] != '\n' {
		return nil, false
	}
	rest, wsOK := skipIndentAfterNewline(s[sel+1:], ws)
	if !wsOK {
		return nil, true
	}
	return &rest, true
}

// skipIndentAfterNewline consumes the indentation prefix ws at the start of s
// (the bytes just after a body newline). A line that carries the prefix has it
// stripped; a blank line (one that is itself just a newline) carries no prefix
// and is left as-is. Any other content that does not start with the prefix is
// an indentation error, returning ok=false — which CUE maps to an empty
// Unquoted(). The token is CR-free here (unquoteMultiline strips CR), so only
// a bare '\n' marks a blank line.
func skipIndentAfterNewline(s, ws string) (string, bool) {
	switch {
	case strings.HasPrefix(s, ws):
		return s[len(ws):], true
	case strings.HasPrefix(s, "\n"):
		return s, true
	default:
		return "", false
	}
}

// stripCR returns s with every '\r' byte removed, matching CUE's scanner.
// stripCR for multiline string tokens. When s has no CR it is returned
// unchanged with no allocation.
func stripCR(s string) string {
	if !strings.ContainsRune(s, '\r') {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] != '\r' {
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

// decodeMultilineChar decodes one character of a multiline body at hash level
// hashes: a backslash escape (reusing the single-line escape decoders, which
// already handle surrogate pairing) or a verbatim rune. It returns the decoded
// rune, the bytes consumed, and ok=false on a malformed escape. A backslash
// not followed by the '#'×hashes introducer is a literal backslash.
func decodeMultilineChar(s string, hashes int) (rune, int, bool) {
	if s[0] == '\\' && strings.HasPrefix(s[1:], strings.Repeat("#", hashes)) {
		// The introducer's selector byte is always present here: the closing
		// delimiter sits at the end of the content region, and
		// multilineCloseIndex already rejects (as unterminated) a '\'+'#' run
		// with no following byte, so decodeEscapeAt's selector index is in range.
		r, width, err := decodeEscapeAt(s, 0, hashes)
		if err != nil {
			return 0, 0, false
		}
		return r, width, true
	}
	r, size := utf8.DecodeRuneInString(s)
	return r, size, true
}
