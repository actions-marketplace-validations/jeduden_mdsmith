package lint

import (
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// proseContains reports whether any prose range's text contains sub.
func proseContains(f *File, sub string) bool {
	for _, r := range f.ProseRanges() {
		if strings.Contains(string(f.Source[r.Start:r.End]), sub) {
			return true
		}
	}
	return false
}

func TestProseRanges_IncludesProseExcludesCode(t *testing.T) {
	src := []byte(
		"# Heading prose\n" +
			"\n" +
			"A paragraph with `code span text` and a word.\n" +
			"\n" +
			"```\nfenced code body\n```\n" +
			"\n" +
			"    indented code body\n" +
			"\n" +
			"<div>html block body</div>\n" +
			"\n" +
			"> blockquote prose\n" +
			"\n" +
			"- list item prose\n",
	)
	f, err := NewFile("t.md", src)
	require.NoError(t, err)

	// Prose inclusions: heading, paragraph words, blockquote, list item.
	assert.True(t, proseContains(f, "Heading prose"), "heading text is prose")
	assert.True(t, proseContains(f, "A paragraph with"), "paragraph text is prose")
	assert.True(t, proseContains(f, "and a word"), "paragraph tail is prose")
	assert.True(t, proseContains(f, "blockquote prose"), "blockquote text is prose")
	assert.True(t, proseContains(f, "list item prose"), "list item text is prose")

	// Code exclusions: every code shape must be absent from prose ranges.
	assert.False(t, proseContains(f, "code span text"), "code span excluded")
	assert.False(t, proseContains(f, "fenced code body"), "fenced code excluded")
	assert.False(t, proseContains(f, "indented code body"), "indented code excluded")
	assert.False(t, proseContains(f, "html block body"), "HTML block excluded")
}

func TestProseRanges_ExcludesAutolinkAndInlineHTML(t *testing.T) {
	src := []byte(
		"Visit <https://auto.example/link> now and see " +
			"<span>inline html</span> here.\n",
	)
	f, err := NewFile("t.md", src)
	require.NoError(t, err)

	assert.True(t, proseContains(f, "Visit"), "leading prose included")
	assert.True(t, proseContains(f, "now and see"), "mid prose included")
	assert.True(t, proseContains(f, "here"), "trailing prose included")

	// The autolink URL bytes are not prose: a bare-URL or casing rule
	// must not see the link target.
	assert.False(t, proseContains(f, "auto.example"), "autolink URL excluded")

	// The inline raw HTML TAGS are excluded, but the visible text the
	// tags wrap ("inline html") is genuine prose a casing/forbidden-text
	// rule should still inspect, so it stays in the ranges. This mirrors
	// how CommonMark renders <span>inline html</span> as the visible
	// words "inline html".
	assert.False(t, proseContains(f, "<span>"), "inline HTML open tag excluded")
	assert.False(t, proseContains(f, "</span>"), "inline HTML close tag excluded")
	assert.True(t, proseContains(f, "inline html"), "text wrapped by inline HTML is prose")
}

func TestProseRanges_LinkTextIsProseURLIsNot(t *testing.T) {
	src := []byte("See [the link text](https://dest.example/page) here.\n")
	f, err := NewFile("t.md", src)
	require.NoError(t, err)

	assert.True(t, proseContains(f, "the link text"), "link visible text is prose")
	assert.True(t, proseContains(f, "See"), "prose before link included")
	assert.True(t, proseContains(f, "here"), "prose after link included")
	assert.False(t, proseContains(f, "dest.example"), "link destination excluded")
}

func TestProseRanges_RangesAreWithinSourceAndOrdered(t *testing.T) {
	src := []byte("# Title\n\nFirst para.\n\nSecond para.\n")
	f, err := NewFile("t.md", src)
	require.NoError(t, err)

	ranges := f.ProseRanges()
	require.NotEmpty(t, ranges)
	prev := -1
	for _, r := range ranges {
		assert.GreaterOrEqual(t, r.Start, 0)
		assert.LessOrEqual(t, r.End, len(f.Source))
		assert.LessOrEqual(t, r.Start, r.End, "range start before end")
		assert.GreaterOrEqual(t, r.Start, prev, "ranges in document order")
		prev = r.End
	}
}

// TestProseRanges_Memoized pins that repeated calls return the same
// backing slice (computed once per File), matching the codeBlockLines /
// newlineOffsets caching contract.
func TestProseRanges_Memoized(t *testing.T) {
	f, err := NewFile("t.md", []byte("# H\n\nBody text here.\n"))
	require.NoError(t, err)

	a := f.ProseRanges()
	b := f.ProseRanges()
	require.Equal(t, len(a), len(b))
	// Same backing array: cached, not recomputed.
	if len(a) > 0 {
		assert.Equal(t, &a[0], &b[0], "ProseRanges must return the cached slice")
	}
}

// TestProseRanges_NilAST returns nil rather than panicking, matching the
// guard the collect* helpers use for struct-literal Files.
func TestProseRanges_NilAST(t *testing.T) {
	f := &File{Source: []byte("text")}
	assert.Nil(t, f.ProseRanges())
}

// TestProseRanges_ConcurrentMemo exercises the atomic.Bool + mutex memo
// under concurrent readers, the same scope the LSP runs against one
// document.
func TestProseRanges_ConcurrentMemo(t *testing.T) {
	f, err := NewFile("t.md", []byte("# H\n\nBody text here.\n"))
	require.NoError(t, err)

	var wg sync.WaitGroup
	results := make([][]Range, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			results[idx] = f.ProseRanges()
		}(i)
	}
	wg.Wait()
	for i := 1; i < len(results); i++ {
		require.Equal(t, len(results[0]), len(results[i]))
	}
}
