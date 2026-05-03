package nospaceinlinktext

import (
	"fmt"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/yuin/goldmark/ast"
)

func init() {
	rule.Register(&Rule{CheckImages: true})
}

// Rule implements MDS049, flagging Markdown links and images whose visible
// text has leading or trailing whitespace inside the brackets.
type Rule struct {
	CheckImages bool
}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS049" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "no-space-in-link-text" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "link" }

// EnabledByDefault implements rule.Defaultable. MDS049 is opt-in.
func (r *Rule) EnabledByDefault() bool { return false }

// ApplySettings implements rule.Configurable.
func (r *Rule) ApplySettings(settings map[string]any) error {
	for k, v := range settings {
		switch k {
		case "check-images":
			b, ok := v.(bool)
			if !ok {
				return fmt.Errorf("no-space-in-link-text: check-images must be a bool, got %T", v)
			}
			r.CheckImages = b
		default:
			return fmt.Errorf("no-space-in-link-text: unknown setting %q", k)
		}
	}
	return nil
}

// DefaultSettings implements rule.Configurable.
func (r *Rule) DefaultSettings() map[string]any {
	return map[string]any{
		"check-images": true,
	}
}

// bracketSpan returns the byte range of text inside the opening `[` and its
// matching `]` for a link or image node. It reads from f.Source starting at
// the node's first child's segment start, scanning backward for `[`.
// Returns (-1, -1) if the span cannot be found.
func bracketSpan(n ast.Node, source []byte) (open, close int) {
	// Find the start of the node's content by looking at the first child's segment.
	// For links/images, walk children to find the first text segment.
	textStart := -1
	for child := n.FirstChild(); child != nil; child = child.NextSibling() {
		if t, ok := child.(*ast.Text); ok {
			textStart = t.Segment.Start
			break
		}
	}

	// If no text child, node may be empty (e.g. ![](url)); search from a
	// position derived from the image/link's known structure. We use the
	// node's first child position if available, or skip.
	if textStart == -1 {
		// Try raw base segment if it exists (Image/Link may have it).
		return -1, -1
	}

	// Scan backward to find the `[` character (skipping `!` for images).
	openBracket := -1
	for i := textStart - 1; i >= 0; i-- {
		if source[i] == '[' {
			openBracket = i
			break
		}
		// Only whitespace or `!` should appear between the start and `[`.
		if source[i] != ' ' && source[i] != '\t' && source[i] != '!' {
			break
		}
	}
	if openBracket == -1 {
		return -1, -1
	}

	// Scan forward from textStart to find the matching `]`.
	// We track bracket depth because link text can itself contain brackets.
	depth := 1
	i := textStart
	for i < len(source) && depth > 0 {
		switch source[i] {
		case '[':
			depth++
		case ']':
			depth--
		}
		i++
	}
	if depth != 0 {
		return -1, -1
	}
	// i now points one past the `]`.
	closeBracket := i - 1
	return openBracket, closeBracket
}

// isImage reports whether n is an *ast.Image.
func isImage(n ast.Node) bool {
	_, ok := n.(*ast.Image)
	return ok
}

type span struct {
	open  int
	close int
	img   bool
}

// collectSpans walks the AST and returns all bracket spans needing inspection.
func (r *Rule) collectSpans(f *lint.File) []span {
	var spans []span
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch n.(type) {
		case *ast.Link, *ast.Image:
		default:
			return ast.WalkContinue, nil
		}
		img := isImage(n)
		if img && !r.CheckImages {
			return ast.WalkContinue, nil
		}
		open, close := bracketSpan(n, f.Source)
		if open == -1 {
			return ast.WalkContinue, nil
		}
		spans = append(spans, span{open: open, close: close, img: img})
		return ast.WalkSkipChildren, nil
	})
	return spans
}

// Check implements rule.Rule.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	var diags []lint.Diagnostic
	for _, s := range r.collectSpans(f) {
		// Text inside brackets is source[s.open+1 : s.close].
		inner := f.Source[s.open+1 : s.close]
		if len(inner) == 0 {
			continue
		}
		role := "link text"
		if s.img {
			role = "image alt text"
		}
		first := inner[0]
		last := inner[len(inner)-1]
		// Only flag space/tab, not newlines.
		if first == ' ' || first == '\t' {
			diags = append(diags, lint.Diagnostic{
				File:     f.Path,
				Line:     f.LineOfOffset(s.open),
				Column:   f.ColumnOfOffset(s.open),
				RuleID:   r.ID(),
				RuleName: r.Name(),
				Severity: lint.Warning,
				Message:  role + " has leading whitespace",
			})
		}
		if last == ' ' || last == '\t' {
			diags = append(diags, lint.Diagnostic{
				File:     f.Path,
				Line:     f.LineOfOffset(s.close - 1),
				Column:   f.ColumnOfOffset(s.close - 1),
				RuleID:   r.ID(),
				RuleName: r.Name(),
				Severity: lint.Warning,
				Message:  role + " has trailing whitespace",
			})
		}
	}
	return diags
}

// Fix implements rule.FixableRule. Trims leading/trailing space/tab inside
// each bracket pair while leaving the surrounding markdown structure intact.
func (r *Rule) Fix(f *lint.File) []byte {
	type replacement struct {
		open  int
		close int
		text  []byte
	}

	var reps []replacement
	for _, s := range r.collectSpans(f) {
		inner := f.Source[s.open+1 : s.close]
		if len(inner) == 0 {
			continue
		}
		trimmed := trimSpaceTab(inner)
		if len(trimmed) == len(inner) {
			continue
		}
		reps = append(reps, replacement{
			open:  s.open + 1,
			close: s.close,
			text:  trimmed,
		})
	}

	if len(reps) == 0 {
		result := make([]byte, len(f.Source))
		copy(result, f.Source)
		return result
	}

	var result []byte
	prev := 0
	for _, rep := range reps {
		result = append(result, f.Source[prev:rep.open]...)
		result = append(result, rep.text...)
		prev = rep.close
	}
	result = append(result, f.Source[prev:]...)
	return result
}

// trimSpaceTab trims leading and trailing space/tab bytes (not newlines).
func trimSpaceTab(b []byte) []byte {
	start := 0
	for start < len(b) && (b[start] == ' ' || b[start] == '\t') {
		start++
	}
	end := len(b)
	for end > start && (b[end-1] == ' ' || b[end-1] == '\t') {
		end--
	}
	return b[start:end]
}

var (
	_ rule.Configurable = (*Rule)(nil)
	_ rule.Defaultable  = (*Rule)(nil)
	_ rule.FixableRule  = (*Rule)(nil)
)
