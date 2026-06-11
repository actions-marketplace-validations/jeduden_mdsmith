package syntax

import "fmt"

// parse_struct.go parses struct and list literals (plan 240). A struct literal
// `{ … }` holds field, embed, ellipsis, and comprehension declarations; a list
// literal `[ … ]` holds element expressions, comprehensions, and an optional
// `...T` open tail.

// parseStructLit parses a `{ … }` struct literal. The opening `{` is the
// current token.
func (p *parser) parseStructLit() (*StructLit, error) {
	if p.peekKind() != tLBrace {
		return nil, fmt.Errorf("cuelite: expected '{'")
	}
	p.take()
	decls, err := p.parseDecls(tRBrace)
	if err != nil {
		return nil, err
	}
	if p.peekKind() != tRBrace {
		return nil, fmt.Errorf("cuelite: expected '}' to close struct")
	}
	p.take()
	return &StructLit{Elts: decls}, nil
}

// parseListLit parses a `[ … ]` list literal: comma-separated elements, each
// an expression, a comprehension, or the `...T` open tail.
func (p *parser) parseListLit() (*ListLit, error) {
	if p.peekKind() != tLBrack {
		return nil, fmt.Errorf("cuelite: expected '['")
	}
	p.take()
	var elts []Expr
	for p.peekKind() != tRBrack && p.peekKind() != tEOF {
		el, err := p.parseListElem()
		if err != nil {
			return nil, err
		}
		elts = append(elts, el)
		if p.peekKind() == tComma {
			p.take()
			continue
		}
		break
	}
	if p.peekKind() != tRBrack {
		return nil, fmt.Errorf("cuelite: expected ']' to close list")
	}
	p.take()
	return &ListLit{Elts: elts}, nil
}

// parseListElem parses one list element. A `...` is the open tail (carried as
// an Ellipsis expression); an `if`/`for` is a comprehension; anything else is
// an expression. The Ellipsis and Comprehension are Decls but list elements
// are typed Expr in the cuelang tree, so they implement aExpr too via the
// wrappers below.
func (p *parser) parseListElem() (Expr, error) {
	switch p.peekKind() {
	case tEllipsis:
		p.take()
		el := &Ellipsis{}
		if p.startsExpr() {
			t, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			el.Type = t
		}
		return el, nil
	case tIdent:
		if p.peekTok().text == "if" || p.peekTok().text == "for" {
			d, err := p.parseComprehension()
			if err != nil {
				return nil, err
			}
			return d.(Expr), nil
		}
	}
	return p.parseExpr()
}
