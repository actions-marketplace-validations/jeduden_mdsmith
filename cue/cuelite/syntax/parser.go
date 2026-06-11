package syntax

import "fmt"

// parser.go is the in-house recursive-descent parser for the CUE subset (plan
// 240). It consumes the scanner's token stream and produces the in-house AST
// (ast.go). It replaces cuelang.org/go/cue/parser for the exact grammar the
// three consumers feed it: a file of field declarations or a single embedded
// expression; struct and list literals with fields, embeds, ellipses, and
// single-clause if/for comprehensions; the bound/disjunction/meet/comparison/
// arithmetic operators at CUE's precedence; selectors, indexing, and calls;
// the `*` default mark and the `?` optional-field marker; and the three string
// dialects with `\(…)` interpolation.

// parseFile parses a complete source string into a File. It is the in-house
// replacement for parser.ParseFile: the source is a sequence of declarations
// (the bare `title: string` form) or a single embedded expression (the
// `{...}` / `close({...})` form the query and schema validators emit).
func parseFile(src string) (*File, error) {
	sc, err := newScanner(src)
	if err != nil {
		return nil, err
	}
	p := &parser{sc: sc}
	p.advance()
	decls, err := p.parseDecls(tEOF)
	if err != nil {
		return nil, err
	}
	if p.sc.err != nil {
		return nil, p.sc.err
	}
	// parseDecls consumes through tEOF (its loop stops only on tEOF for a file),
	// so no trailing token remains; a stray token would have been parsed as
	// another declaration or surfaced as a parse error above.
	return &File{Decls: decls}, nil
}

// parser holds the one-token lookahead over the scanner. cur is the current
// token; advance pulls the next. pending/pendingTok hold a label token that a
// failed field-lookahead (tryFieldLabel) put back for the expression parser:
// when pending is set, parsePrimary consumes pendingTok as the first token of
// an expression and the real cur becomes the following token.
type parser struct {
	sc         *scanner
	cur        tok
	pending    bool
	pendingTok tok
}

// advance pulls the next token from the scanner into cur.
func (p *parser) advance() {
	p.cur = p.sc.next()
}

// take returns the token the parser should treat as current, honoring a
// pending put-back label, and advances past it. It is the single read point
// the expression parser uses so a put-back label is consumed exactly once.
func (p *parser) take() tok {
	if p.pending {
		p.pending = false
		t := p.pendingTok
		// cur already holds the token AFTER the put-back label (tryFieldLabel
		// advanced once), so do not advance again.
		return t
	}
	t := p.cur
	p.advance()
	return t
}

// peekKind returns the kind of the token the expression parser will see next,
// honoring a pending put-back label.
func (p *parser) peekKind() tokKind {
	if p.pending {
		return p.pendingTok.kind
	}
	return p.cur.kind
}

// peekTok returns the token the expression parser will see next, honoring a
// pending put-back label.
func (p *parser) peekTok() tok {
	if p.pending {
		return p.pendingTok
	}
	return p.cur
}

// resume pulls the next interpolation string fragment (the parser calls it
// after consuming the `)` that closes an embedded interpolation expression).
func (p *parser) resume() {
	p.cur = p.sc.resumeInterp()
}

// parseDecls parses declarations until the closing token `end` (tEOF for a
// file, tRBrace for a struct). Declarations are separated by commas or
// newlines; the scanner drops newlines, so a comma is optional between decls
// and the loop simply parses until `end`.
func (p *parser) parseDecls(end tokKind) ([]Decl, error) {
	var decls []Decl
	for p.cur.kind != end && p.cur.kind != tEOF {
		d, err := p.parseDecl()
		if err != nil {
			return nil, err
		}
		decls = append(decls, d)
		// A separating comma is consumed when present; a newline-separated decl
		// has none (the scanner dropped the newline), so the next iteration sees
		// the next decl's first token directly.
		if p.cur.kind == tComma {
			p.advance()
		}
	}
	return decls, nil
}

// parseDecl parses one declaration: an ellipsis tail, a comprehension, a field
// (`label: value` or `label?: value`), or an embedded expression. It decides
// between a field and an embed by lookahead: a label followed by `:` or `?:`
// is a field; anything else is an embedded expression.
func (p *parser) parseDecl() (Decl, error) {
	switch p.cur.kind {
	case tEllipsis:
		return p.parseEllipsisDecl()
	case tIdent:
		if p.cur.text == "if" {
			return p.parseComprehension()
		}
		if p.cur.text == "for" {
			return p.parseComprehension()
		}
	}
	// A field starts with a label (ident or string) followed by `:` or `?:`.
	if lbl, isField := p.tryFieldLabel(); isField {
		return p.parseFieldRest(lbl)
	}
	// Otherwise the declaration is an embedded expression.
	e, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	return &EmbedDecl{Expr: e}, nil
}

// tryFieldLabel peeks for a field label followed by a `:` or `?:`. It returns
// the label and true when the current position is a field; otherwise it
// returns false WITHOUT consuming anything beyond a label it can put back.
// Because the parser has single-token lookahead and labels may be a string or
// ident, it commits to the field form only after seeing the `:`/`?:`, so a
// label-shaped expression (`status` as an embed) is not misread as a field.
func (p *parser) tryFieldLabel() (Label, bool) {
	if p.cur.kind != tIdent && p.cur.kind != tString {
		return nil, false
	}
	lblTok := p.cur
	// Look ahead one token. The scanner is stateless across peeks, so save and
	// scan the next token; if it is not a field separator, the label is the
	// start of an expression and parseExpr re-reads it. To support that, the
	// parser keeps the peeked token in pending.
	p.advance()
	switch p.cur.kind {
	case tColon:
		return labelFromTok(lblTok), true
	case tQuestion:
		// `?:` optional field — confirm the `:` follows.
		p.advance()
		if p.cur.kind == tColon {
			// Encode the optional marker by returning a label and letting
			// parseFieldRest read the constraint; signal optional via a wrapper.
			return optionalLabel{labelFromTok(lblTok)}, true
		}
		// A `?` not followed by `:` is not a field; this shape does not occur in
		// the subset, so report it.
		p.pending = true
		p.pendingTok = lblTok
		return nil, false
	default:
		// Not a field: stash the label token so parseExpr's primary reads it.
		p.pending = true
		p.pendingTok = lblTok
		return nil, false
	}
}

// pending machinery lets tryFieldLabel return a label token to the expression
// parser after a failed field lookahead: parsePrimary consumes pendingTok
// before reading cur.
//
// Fields added to parser via embedding the pending state.

// optionalLabel wraps a Label to carry the `?` optional marker from
// tryFieldLabel to parseFieldRest.
type optionalLabel struct{ Label }

// labelFromTok builds a Label from a label token: a quoted string becomes a
// BasicLit STRING (decoded by fieldLabel), a bare identifier an Ident.
func labelFromTok(t tok) Label {
	if t.kind == tString {
		return &BasicLit{Kind: STRING, Value: t.text}
	}
	return &Ident{Name: t.text}
}

// parseFieldRest parses the value of a field whose label has been consumed,
// with the `:` (and optional `?`) already seen by tryFieldLabel. It consumes
// the `:` and parses the value expression.
func (p *parser) parseFieldRest(lbl Label) (Decl, error) {
	constraint := NoToken
	if ol, ok := lbl.(optionalLabel); ok {
		constraint = OPTION
		lbl = ol.Label
	}
	// cur is the `:`; consume it.
	if p.cur.kind != tColon {
		return nil, fmt.Errorf("cuelite: expected ':' in field declaration")
	}
	p.advance()
	// CUE's nested-field shorthand: `a: b: c` desugars to `a: {b: c}`. When the
	// value position itself starts another `label:` field, build the implicit
	// single-field struct rather than parsing it as an expression.
	if inner, ok := p.tryFieldLabel(); ok {
		nested, err := p.parseFieldRest(inner)
		if err != nil {
			return nil, err
		}
		return &Field{Label: lbl, Value: &StructLit{Elts: []Decl{nested}}, Constraint: constraint}, nil
	}
	val, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	return &Field{Label: lbl, Value: val, Constraint: constraint}, nil
}

// parseEllipsisDecl parses a `...` tail, optionally followed by an element
// type (`...string`). It appears as a struct/list decl.
func (p *parser) parseEllipsisDecl() (Decl, error) {
	p.advance() // consume `...`
	el := &Ellipsis{}
	if p.startsExpr() {
		t, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		el.Type = t
	}
	return el, nil
}

// parseComprehension parses a comprehension: one or more clauses (if/for/let)
// followed by a struct body. The subset's evaluator accepts only a SINGLE
// clause and rejects a multi-clause form as unsupported; the parser still reads
// every clause so that rejection — not a parse error — fires for a
// `for x in xs if c {…}` or `for x in xs let y = … {…}` form, matching the
// behaviour CUE's parser had (it parsed the multi-clause tree and the evaluator
// rejected it).
func (p *parser) parseComprehension() (Decl, error) {
	var clauses []Clause
	for {
		c, err := p.parseClause()
		if err != nil {
			return nil, err
		}
		clauses = append(clauses, c)
		// Another clause follows when the next token is a clause keyword; the
		// body otherwise.
		if p.cur.kind == tIdent && (p.cur.text == "if" || p.cur.text == "for" || p.cur.text == "let") {
			continue
		}
		break
	}
	if p.cur.kind != tLBrace {
		return nil, fmt.Errorf("cuelite: comprehension body must be a struct")
	}
	body, err := p.parseStructLit()
	if err != nil {
		return nil, err
	}
	return &Comprehension{Clauses: clauses, Value: body}, nil
}

// parseClause parses one comprehension clause: `if cond`, `for x in src`, or
// `let x = expr`.
func (p *parser) parseClause() (Clause, error) {
	switch p.cur.text {
	case "if":
		p.advance()
		cond, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		return &IfClause{Condition: cond}, nil
	case "for":
		p.advance()
		return p.parseForClause()
	case "let":
		p.advance()
		return p.parseLetClause()
	default:
		return nil, fmt.Errorf("cuelite: unsupported comprehension clause %q", p.cur.text)
	}
}

// parseLetClause parses `x = expr` after the `let` keyword.
func (p *parser) parseLetClause() (*LetClause, error) {
	if p.cur.kind != tIdent {
		return nil, fmt.Errorf("cuelite: let clause requires a variable")
	}
	name := &Ident{Name: p.cur.text}
	p.advance()
	if p.cur.kind != tAssign {
		return nil, fmt.Errorf("cuelite: let clause requires '='")
	}
	p.advance()
	expr, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	return &LetClause{Ident: name, Expr: expr}, nil
}

// parseForClause parses `x in src` or `k, x in src` after the `for` keyword.
func (p *parser) parseForClause() (*ForClause, error) {
	if p.cur.kind != tIdent {
		return nil, fmt.Errorf("cuelite: for clause requires a variable")
	}
	first := &Ident{Name: p.cur.text}
	p.advance()
	fc := &ForClause{Value: first}
	if p.cur.kind == tComma {
		p.advance()
		if p.cur.kind != tIdent {
			return nil, fmt.Errorf("cuelite: for clause requires a value variable")
		}
		fc.Key = first
		fc.Value = &Ident{Name: p.cur.text}
		p.advance()
	}
	if p.cur.kind != tIdent || p.cur.text != "in" {
		return nil, fmt.Errorf("cuelite: for clause requires 'in'")
	}
	p.advance()
	src, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	fc.Source = src
	return fc, nil
}
