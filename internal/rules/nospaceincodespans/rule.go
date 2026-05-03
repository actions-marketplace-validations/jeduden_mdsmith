// Package nospaceincodespans implements MDS052, which flags inline code
// spans with leading or trailing whitespace inside the backticks.
package nospaceincodespans

import (
	"bytes"
	"sort"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/yuin/goldmark/ast"
)

func init() {
	rule.Register(&Rule{})
}

// Rule flags inline code spans with leading or trailing whitespace.
type Rule struct{}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS052" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "no-space-in-code-spans" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "whitespace" }

// EnabledByDefault implements rule.Defaultable.
func (r *Rule) EnabledByDefault() bool { return false }

const (
	msgLeading  = "code span has leading whitespace"
	msgTrailing = "code span has trailing whitespace"
)

// Check implements rule.Rule.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	var diags []lint.Diagnostic
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		cs, ok := n.(*ast.CodeSpan)
		if !ok {
			return ast.WalkContinue, nil
		}
		raw, openBt, ok2 := rawContent(cs, f.Source)
		if !ok2 || len(raw) == 0 {
			return ast.WalkContinue, nil
		}
		if isBalancedSingleSpace(raw) {
			return ast.WalkContinue, nil
		}

		line := f.LineOfOffset(openBt)
		col := f.ColumnOfOffset(openBt)

		if isASCIIWhitespace(raw[0]) {
			diags = append(diags, lint.Diagnostic{
				File:     f.Path,
				Line:     line,
				Column:   col,
				RuleID:   r.ID(),
				RuleName: r.Name(),
				Severity: lint.Warning,
				Message:  msgLeading,
			})
		}
		if isASCIIWhitespace(raw[len(raw)-1]) {
			diags = append(diags, lint.Diagnostic{
				File:     f.Path,
				Line:     line,
				Column:   col,
				RuleID:   r.ID(),
				RuleName: r.Name(),
				Severity: lint.Warning,
				Message:  msgTrailing,
			})
		}
		return ast.WalkContinue, nil
	})
	return diags
}

// Fix implements rule.FixableRule. It trims leading and trailing whitespace
// from code span content. Spans that become empty after trimming are left
// unchanged.
func (r *Rule) Fix(f *lint.File) []byte {
	type cut struct {
		start, end int
		repl       []byte
	}
	var cuts []cut

	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		cs, ok := n.(*ast.CodeSpan)
		if !ok {
			return ast.WalkContinue, nil
		}
		raw, _, ok2 := rawContent(cs, f.Source)
		if !ok2 || len(raw) == 0 {
			return ast.WalkContinue, nil
		}
		if isBalancedSingleSpace(raw) {
			return ast.WalkContinue, nil
		}
		if !isASCIIWhitespace(raw[0]) && !isASCIIWhitespace(raw[len(raw)-1]) {
			return ast.WalkContinue, nil
		}
		trimmed := bytes.TrimFunc(raw, func(r rune) bool {
			return isASCIIWhitespace(byte(r))
		})
		if len(trimmed) == 0 {
			return ast.WalkContinue, nil
		}
		// Find the byte offsets of the raw content within f.Source.
		first, last, _ := spanBounds(cs)
		start, end := recoverContentBounds(first, last, f.Source)
		cuts = append(cuts, cut{start: start, end: end, repl: trimmed})
		return ast.WalkContinue, nil
	})

	if len(cuts) == 0 {
		out := make([]byte, len(f.Source))
		copy(out, f.Source)
		return out
	}

	sort.Slice(cuts, func(i, j int) bool { return cuts[i].start < cuts[j].start })
	var out bytes.Buffer
	prev := 0
	for _, c := range cuts {
		if c.start < prev {
			continue
		}
		out.Write(f.Source[prev:c.start])
		out.Write(c.repl)
		prev = c.end
	}
	out.Write(f.Source[prev:])
	return out.Bytes()
}

// rawContent returns the raw bytes between the backtick delimiters of cs
// (before any CommonMark single-space trim) and the byte offset of the
// opening backtick, for position reporting.
//
// Goldmark applies the CommonMark rule "if both sides have exactly one space
// and the content is not all-whitespace, strip one space from each side"
// before recording the text-child segments. To recover the original bytes
// we peek at the adjacent bytes: if a space precedes the first segment and
// a backtick precedes that space, the space was stripped.
func rawContent(cs *ast.CodeSpan, source []byte) (raw []byte, openBt int, ok bool) {
	first, last, ok2 := spanBounds(cs)
	if !ok2 {
		return nil, 0, false
	}
	start, end := recoverContentBounds(first, last, source)
	// Find opening backtick.
	btStart := start
	for btStart > 0 && source[btStart-1] == '`' {
		btStart--
	}
	return source[start:end], btStart, true
}

// recoverContentBounds returns the [start, end) byte range of a code span's
// raw content, undoing the CommonMark single-space trim that goldmark applies
// before recording the text-child segments.
func recoverContentBounds(first, last int, source []byte) (start, end int) {
	start = first
	// If the byte before the segment is a space and the byte before that is
	// a backtick, the leading space was stripped by CommonMark.
	if start > 1 && source[start-1] == ' ' && source[start-2] == '`' {
		start--
	}

	end = last
	// Similarly for the trailing side.
	if end+1 < len(source) && source[end] == ' ' && source[end+1] == '`' {
		end++
	}
	return start, end
}

// spanBounds returns the [start, end) byte range of a CodeSpan's content
// as reported by goldmark (post-trim) by walking text children.
func spanBounds(cs *ast.CodeSpan) (first, last int, ok bool) {
	first = -1
	last = -1
	for c := cs.FirstChild(); c != nil; c = c.NextSibling() {
		t, ok2 := c.(*ast.Text)
		if !ok2 {
			continue
		}
		if first < 0 || t.Segment.Start < first {
			first = t.Segment.Start
		}
		if t.Segment.Stop > last {
			last = t.Segment.Stop
		}
	}
	return first, last, first >= 0 && last >= first
}

// isBalancedSingleSpace reports whether raw has exactly one ASCII space on
// each side with no additional whitespace at either boundary — the CommonMark
// single-space-trim case that renders without leading or trailing space.
func isBalancedSingleSpace(raw []byte) bool {
	n := len(raw)
	if n < 3 {
		return false
	}
	if raw[0] != ' ' || raw[n-1] != ' ' {
		return false
	}
	return !isASCIIWhitespace(raw[1]) && !isASCIIWhitespace(raw[n-2])
}

func isASCIIWhitespace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}

var (
	_ rule.FixableRule = (*Rule)(nil)
	_ rule.Defaultable = (*Rule)(nil)
)
