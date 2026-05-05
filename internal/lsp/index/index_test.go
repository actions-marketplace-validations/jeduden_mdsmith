package index

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateExtractsHeadings(t *testing.T) {
	t.Parallel()
	idx := New("/root")
	src := "# Top\n\n## Sub A\n\ntext\n\n## Sub B\n\ntail\n"
	idx.Update("docs/x.md", []byte(src))
	fe, ok := idx.File("docs/x.md")
	require.True(t, ok)
	require.Len(t, fe.Symbols, 3)

	assert.Equal(t, SymbolHeading, fe.Symbols[0].Kind)
	assert.Equal(t, "Top", fe.Symbols[0].Name)
	assert.Equal(t, "top", fe.Symbols[0].Anchor)
	assert.Equal(t, 1, fe.Symbols[0].Level)
	assert.Equal(t, 1, fe.Symbols[0].StartLine)

	assert.Equal(t, "Sub A", fe.Symbols[1].Name)
	assert.Equal(t, 3, fe.Symbols[1].StartLine)
	// EndLine of Sub A is the line before Sub B.
	assert.Equal(t, 6, fe.Symbols[1].EndLine)
	assert.Equal(t, "Sub B", fe.Symbols[2].Name)
}

func TestUpdateExtractsLinkRefDefs(t *testing.T) {
	t.Parallel()
	idx := New("/root")
	src := "# Title\n\nSee [Foo][foo].\n\n[foo]: https://example.com\n"
	idx.Update("a.md", []byte(src))
	fe, ok := idx.File("a.md")
	require.True(t, ok)

	var found bool
	for _, s := range fe.Symbols {
		if s.Kind == SymbolLinkRef && s.Anchor == "foo" {
			found = true
			assert.Equal(t, 5, s.StartLine)
		}
	}
	assert.True(t, found, "expected link-ref def for 'foo': %+v", fe.Symbols)
}

func TestUpdateExtractsFrontMatterKeys(t *testing.T) {
	t.Parallel()
	idx := New("/root")
	src := "---\ntitle: Hello\nkinds:\n  - guide\n---\n# Body\n"
	idx.Update("p.md", []byte(src))
	fe, ok := idx.File("p.md")
	require.True(t, ok)
	assert.Equal(t, "Hello", fe.Title)
	assert.Equal(t, []string{"guide"}, fe.Kinds)
	var keys []string
	for _, s := range fe.Symbols {
		if s.Kind == SymbolFrontMatter {
			keys = append(keys, s.Name)
		}
	}
	assert.ElementsMatch(t, []string{"title", "kinds"}, keys)
}

func TestUpdateExtractsDirectives(t *testing.T) {
	t.Parallel()
	idx := New("/root")
	src := "# Top\n\n<?include\nfile: \"x.md\"\n?>\nbody\n<?/include?>\n"
	idx.Update("p.md", []byte(src))
	fe, ok := idx.File("p.md")
	require.True(t, ok)
	var dirs []Symbol
	for _, s := range fe.Symbols {
		if s.Kind == SymbolDirective {
			dirs = append(dirs, s)
		}
	}
	require.Len(t, dirs, 1)
	assert.Equal(t, "include", dirs[0].Name)
}

func TestOutgoingEdgesAnchorAndFile(t *testing.T) {
	t.Parallel()
	idx := New("/root")
	idx.Update("a.md", []byte("# A\n\n[here](#section)\n[other](./b.md#sub)\n"))
	fe, ok := idx.File("a.md")
	require.True(t, ok)
	require.Len(t, fe.Outgoing, 2)
	assert.Equal(t, EdgeAnchorLink, fe.Outgoing[0].Kind)
	assert.Equal(t, "section", fe.Outgoing[0].TargetAnchor)
	assert.Equal(t, EdgeFileLink, fe.Outgoing[1].Kind)
	assert.Equal(t, "b.md", fe.Outgoing[1].TargetFile)
	assert.Equal(t, "sub", fe.Outgoing[1].TargetAnchor)
}

func TestOutgoingEdgesIncludeAndBuild(t *testing.T) {
	t.Parallel()
	idx := New("/root")
	src := "# T\n\n<?include\nfile: \"sub/x.md\"\n?>\n<?/include?>\n\n<?build\nsource: \"src.md\"\n?>\n<?/build?>\n"
	idx.Update("p.md", []byte(src))
	fe, ok := idx.File("p.md")
	require.True(t, ok)
	var inc, bld bool
	for _, e := range fe.Outgoing {
		if e.Kind == EdgeInclude && e.TargetFile == "sub/x.md" {
			inc = true
		}
		if e.Kind == EdgeBuild && e.TargetFile == "src.md" {
			bld = true
		}
	}
	assert.True(t, inc, "missing include edge: %+v", fe.Outgoing)
	assert.True(t, bld, "missing build edge: %+v", fe.Outgoing)
}

func TestIncomingEdgesAcrossFiles(t *testing.T) {
	t.Parallel()
	idx := New("/root")
	idx.Update("a.md", []byte("# A\n\n## Sec\n"))
	idx.Update("b.md", []byte("# B\n\n[s](./a.md#sec)\n"))

	in := idx.IncomingEdges("a.md", "sec")
	require.Len(t, in, 1)
	assert.Equal(t, "b.md", in[0].SourceFile)
}

func TestSearchSymbolsMatchesHeadings(t *testing.T) {
	t.Parallel()
	idx := New("/root")
	idx.Update("a.md", []byte("# Apple Pie\n\n## Banana Split\n"))
	idx.Update("b.md", []byte("# Cabbage\n"))
	hits := idx.SearchSymbols("apple", 0)
	require.Len(t, hits, 1)
	assert.Equal(t, "Apple Pie", hits[0].Symbol.Name)
}

func TestSearchSymbolsMatchesTitleAndKind(t *testing.T) {
	t.Parallel()
	idx := New("/root")
	idx.Update("a.md", []byte("---\ntitle: Foobar\nkinds:\n  - reference\n---\n# Body\n"))
	hits := idx.SearchSymbols("foobar", 0)
	require.NotEmpty(t, hits)
	assert.Contains(t, namesOf(hits), "Foobar")
	hits = idx.SearchSymbols("reference", 0)
	assert.Contains(t, namesOf(hits), "kind:reference")
}

func TestRemoveDropsFile(t *testing.T) {
	t.Parallel()
	idx := New("/root")
	idx.Update("a.md", []byte("# A\n"))
	idx.Remove("a.md")
	_, ok := idx.File("a.md")
	assert.False(t, ok)
}

func TestBuildReplacesIndex(t *testing.T) {
	t.Parallel()
	idx := New("/root")
	idx.Update("a.md", []byte("# A\n"))
	idx.Build([]string{"b.md"}, func(p string) ([]byte, error) {
		return []byte("# B\n"), nil
	})
	_, gone := idx.File("a.md")
	assert.False(t, gone, "Build should evict files not in the new list")
	_, present := idx.File("b.md")
	assert.True(t, present)
}

func TestFilesByKind(t *testing.T) {
	t.Parallel()
	idx := New("/root")
	idx.Update("a.md", []byte("---\nkinds:\n  - guide\n---\n# A\n"))
	idx.Update("b.md", []byte("---\nkinds:\n  - guide\n  - ref\n---\n# B\n"))
	idx.Update("c.md", []byte("# C\n"))
	got := idx.FilesByKind("guide")
	assert.ElementsMatch(t, []string{"a.md", "b.md"}, got)
}

func TestNormalizePath(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "a/b.md", NormalizePath("a/b.md"))
	assert.Equal(t, "a/b.md", NormalizePath("./a/b.md"))
	assert.Equal(t, "a/b.md", NormalizePath(`a\b.md`))
}

func namesOf(hits []SymbolMatch) []string {
	out := make([]string, 0, len(hits))
	for _, h := range hits {
		out = append(out, h.Symbol.Name)
	}
	return out
}

func TestHeadingDuplicateAnchors(t *testing.T) {
	t.Parallel()
	idx := New("/root")
	src := "# Same\n\n# Same\n\n# Same\n"
	idx.Update("a.md", []byte(src))
	fe, ok := idx.File("a.md")
	require.True(t, ok)
	require.Len(t, fe.Symbols, 3)
	anchors := []string{fe.Symbols[0].Anchor, fe.Symbols[1].Anchor, fe.Symbols[2].Anchor}
	assert.Equal(t, []string{"same", "same-1", "same-2"}, anchors)
}

func TestHeadingsRespectFrontMatterOffset(t *testing.T) {
	t.Parallel()
	idx := New("/root")
	src := "---\ntitle: T\n---\n# Top\n## Sub\n"
	idx.Update("a.md", []byte(src))
	fe, ok := idx.File("a.md")
	require.True(t, ok)
	headings := []Symbol{}
	for _, s := range fe.Symbols {
		if s.Kind == SymbolHeading {
			headings = append(headings, s)
		}
	}
	require.Len(t, headings, 2)
	assert.Equal(t, 4, headings[0].StartLine)
	assert.Equal(t, 5, headings[1].StartLine)
}

func TestUpdateZeroSourceRemoves(t *testing.T) {
	t.Parallel()
	idx := New("/root")
	idx.Update("a.md", []byte("# A\n"))
	idx.Update("a.md", nil)
	_, ok := idx.File("a.md")
	assert.False(t, ok)
}

func TestSearchSymbolsHonorsLimit(t *testing.T) {
	t.Parallel()
	idx := New("/root")
	idx.Update("a.md", []byte("# Foo\n## Foo bar\n## Foo baz\n"))
	hits := idx.SearchSymbols("foo", 2)
	assert.Len(t, hits, 2)
}

func TestRefStyleLinkEdge(t *testing.T) {
	t.Parallel()
	idx := New("/root")
	src := "# T\n\nSee [Foo][bar].\n\n[bar]: ./other.md\n"
	idx.Update("a.md", []byte(src))
	fe, ok := idx.File("a.md")
	require.True(t, ok)
	var saw bool
	for _, e := range fe.Outgoing {
		if e.Kind == EdgeRefLink && strings.EqualFold(e.TargetLabel, "bar") {
			saw = true
		}
	}
	assert.True(t, saw, "edges: %+v", fe.Outgoing)
}
