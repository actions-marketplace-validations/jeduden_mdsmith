package lsp

import (
	"context"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/internal/rule"

	_ "github.com/jeduden/mdsmith/internal/rules/all"
)

// rootedHarness wires a Server to a real on-disk workspace so the
// symbol-navigation tests can drive lookups against actual files.
// The harness writes the supplied files under a tmp directory, then
// initializes the server with that directory as the workspace root.
func rootedHarness(t *testing.T, files map[string]string) (*testHarness, string, string) {
	t.Helper()
	tmp := t.TempDir()
	for rel, content := range files {
		full := filepath.Join(tmp, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte(content), 0o644))
	}
	h := newHarness(t)
	rootURI := pathToFileURI(t, tmp)
	_, errResp := h.request("initialize", initializeParams{
		RootURI:      &rootURI,
		Capabilities: clientCapabilities{},
	})
	require.Nil(t, errResp)
	return h, tmp, rootURI
}

func pathToFileURI(t *testing.T, p string) string {
	t.Helper()
	abs, err := filepath.Abs(p)
	require.NoError(t, err)
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(abs)}
	return u.String()
}

func TestInitializeAdvertisesNavigationCapabilities(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	resultRaw, errResp := h.request("initialize", initializeParams{})
	require.Nil(t, errResp)
	var res initializeResult
	require.NoError(t, json.Unmarshal(resultRaw, &res))
	assert.True(t, res.Capabilities.DocumentSymbolProvider)
	assert.True(t, res.Capabilities.DefinitionProvider)
	assert.True(t, res.Capabilities.ImplementationProvider)
	assert.True(t, res.Capabilities.ReferencesProvider)
	assert.True(t, res.Capabilities.WorkspaceSymbolProvider)
	assert.True(t, res.Capabilities.CallHierarchyProvider)
}

func TestDocumentSymbolReturnsHeadingTree(t *testing.T) {
	t.Parallel()
	h, _, rootURI := rootedHarness(t, map[string]string{
		"a.md": "# Top\n\n## Sub A\n\ntext\n\n## Sub B\n\nbody\n",
	})
	uri := rootURI + "/a.md"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{
			URI: uri, LanguageID: "markdown", Version: 1,
			Text: "# Top\n\n## Sub A\n\ntext\n\n## Sub B\n\nbody\n",
		},
	})
	// Drain the diagnostics that come from didOpen.
	_ = h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)

	raw, errResp := h.request("textDocument/documentSymbol", documentSymbolParams{
		TextDocument: textDocumentIdentifier{URI: uri},
	})
	require.Nil(t, errResp)
	var syms []documentSymbol
	require.NoError(t, json.Unmarshal(raw, &syms))
	require.Len(t, syms, 1, "expected one root H1: %s", string(raw))
	assert.Equal(t, "Top", syms[0].Name)
	require.Len(t, syms[0].Children, 2)
	assert.Equal(t, "Sub A", syms[0].Children[0].Name)
	assert.Equal(t, "Sub B", syms[0].Children[1].Name)
}

func TestDocumentSymbolIncludesFrontMatter(t *testing.T) {
	t.Parallel()
	h, _, rootURI := rootedHarness(t, map[string]string{
		"a.md": "---\ntitle: Hi\n---\n# Top\n",
	})
	uri := rootURI + "/a.md"
	src := "---\ntitle: Hi\n---\n# Top\n"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: src},
	})
	_ = h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)

	raw, errResp := h.request("textDocument/documentSymbol", documentSymbolParams{
		TextDocument: textDocumentIdentifier{URI: uri},
	})
	require.Nil(t, errResp)
	var syms []documentSymbol
	require.NoError(t, json.Unmarshal(raw, &syms))
	var sawFM bool
	for _, s := range syms {
		if s.Name == "front matter" {
			sawFM = true
			assert.NotEmpty(t, s.Children)
		}
	}
	assert.True(t, sawFM, "expected synthetic front-matter parent: %+v", syms)
}

func TestDefinitionAnchorLink(t *testing.T) {
	t.Parallel()
	src := "# Top\n\nSee [s](#sub).\n\n## Sub\n"
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": src})
	uri := rootURI + "/a.md"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: src},
	})
	_ = h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)

	raw, errResp := h.request("textDocument/definition", textDocumentPositionParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		// Cursor inside `[s](#sub)` — line 3 (0-based: 2), char 8.
		Position: Position{Line: 2, Character: 8},
	})
	require.Nil(t, errResp)
	var loc location
	require.NoError(t, json.Unmarshal(raw, &loc))
	assert.Equal(t, uri, loc.URI)
	// "## Sub" is the 5th line (1-based) → LSP line 4.
	assert.Equal(t, 4, loc.Range.Start.Line)
}

func TestDefinitionFileLink(t *testing.T) {
	t.Parallel()
	srcA := "# A\n\n[next](./b.md)\n"
	srcB := "# B\n"
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": srcA, "b.md": srcB})
	uri := rootURI + "/a.md"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: srcA},
	})
	_ = h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)

	raw, errResp := h.request("textDocument/definition", textDocumentPositionParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Position:     Position{Line: 2, Character: 4},
	})
	require.Nil(t, errResp)
	var loc location
	require.NoError(t, json.Unmarshal(raw, &loc))
	expected := rootURI + "/b.md"
	assert.Equal(t, expected, loc.URI)
	assert.Equal(t, 0, loc.Range.Start.Line)
}

func TestDefinitionReferenceLink(t *testing.T) {
	t.Parallel()
	src := "# T\n\nSee [foo][bar].\n\n[bar]: https://example.com\n"
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": src})
	uri := rootURI + "/a.md"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: src},
	})
	_ = h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)

	raw, errResp := h.request("textDocument/definition", textDocumentPositionParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		// Cursor inside `[foo][bar]` on line 3.
		Position: Position{Line: 2, Character: 6},
	})
	require.Nil(t, errResp)
	var loc location
	require.NoError(t, json.Unmarshal(raw, &loc))
	assert.Equal(t, uri, loc.URI)
	// `[bar]: …` is on line 5 (1-based) → 4 (0-based).
	assert.Equal(t, 4, loc.Range.Start.Line)
}

func TestReferencesOnHeading(t *testing.T) {
	t.Parallel()
	srcA := "# A\n\n## Sec\n"
	srcB := "# B\n\n[s](./a.md#sec)\n"
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": srcA, "b.md": srcB})
	uri := rootURI + "/a.md"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: srcA},
	})
	_ = h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)

	raw, errResp := h.request("textDocument/references", referencesParams{
		textDocumentPositionParams: textDocumentPositionParams{
			TextDocument: textDocumentIdentifier{URI: uri},
			// Cursor on `## Sec` (line 3, 1-based) → 2.
			Position: Position{Line: 2, Character: 3},
		},
		Context: referencesContext{IncludeDeclaration: false},
	})
	require.Nil(t, errResp)
	var locs []location
	require.NoError(t, json.Unmarshal(raw, &locs))
	require.Len(t, locs, 1)
	assert.Equal(t, rootURI+"/b.md", locs[0].URI)
}

func TestReferencesIncludeDeclaration(t *testing.T) {
	t.Parallel()
	srcA := "# A\n\n## Sec\n"
	srcB := "# B\n\n[s](./a.md#sec)\n"
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": srcA, "b.md": srcB})
	uri := rootURI + "/a.md"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: srcA},
	})
	_ = h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)

	raw, errResp := h.request("textDocument/references", referencesParams{
		textDocumentPositionParams: textDocumentPositionParams{
			TextDocument: textDocumentIdentifier{URI: uri},
			Position:     Position{Line: 2, Character: 3},
		},
		Context: referencesContext{IncludeDeclaration: true},
	})
	require.Nil(t, errResp)
	var locs []location
	require.NoError(t, json.Unmarshal(raw, &locs))
	assert.Len(t, locs, 2, "expected the heading itself plus the link reference")
}

func TestWorkspaceSymbolMatchesHeading(t *testing.T) {
	t.Parallel()
	h, _, rootURI := rootedHarness(t, map[string]string{
		"a.md": "# Apple Pie\n",
		"b.md": "# Banana Split\n",
	})
	// Force the index to build.
	_, _ = h.request("workspace/symbol", workspaceSymbolParams{Query: ""})
	raw, errResp := h.request("workspace/symbol", workspaceSymbolParams{Query: "apple"})
	require.Nil(t, errResp)
	var hits []symbolInformation
	require.NoError(t, json.Unmarshal(raw, &hits))
	require.Len(t, hits, 1)
	assert.Equal(t, "Apple Pie", hits[0].Name)
	assert.Equal(t, rootURI+"/a.md", hits[0].Location.URI)
}

func TestPrepareAndIncomingCalls(t *testing.T) {
	t.Parallel()
	srcA := "# A\n"
	srcB := "# B\n\n[a](./a.md)\n"
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": srcA, "b.md": srcB})
	uriA := rootURI + "/a.md"

	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uriA, LanguageID: "markdown", Version: 1, Text: srcA},
	})
	_ = h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)

	raw, errResp := h.request("textDocument/prepareCallHierarchy", textDocumentPositionParams{
		TextDocument: textDocumentIdentifier{URI: uriA},
		Position:     Position{Line: 0, Character: 0},
	})
	require.Nil(t, errResp)
	var items []callHierarchyItem
	require.NoError(t, json.Unmarshal(raw, &items))
	require.Len(t, items, 1)
	assert.Equal(t, "a.md", items[0].Name)

	raw, errResp = h.request("callHierarchy/incomingCalls", callHierarchyIncomingCallsParams{Item: items[0]})
	require.Nil(t, errResp)
	var calls []callHierarchyIncomingCall
	require.NoError(t, json.Unmarshal(raw, &calls))
	require.Len(t, calls, 1)
	assert.Equal(t, "b.md", calls[0].From.Name)
}

func TestOutgoingCallsForIncludeChain(t *testing.T) {
	t.Parallel()
	srcA := "# A\n\n<?include\nfile: \"b.md\"\n?>\n<?/include?>\n"
	srcB := "# B\n\n<?include\nfile: \"c.md\"\n?>\n<?/include?>\n"
	srcC := "# C\n"
	h, _, rootURI := rootedHarness(t, map[string]string{
		"a.md": srcA, "b.md": srcB, "c.md": srcC,
	})
	uriA := rootURI + "/a.md"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uriA, LanguageID: "markdown", Version: 1, Text: srcA},
	})
	_ = h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)

	raw, errResp := h.request("textDocument/prepareCallHierarchy", textDocumentPositionParams{
		TextDocument: textDocumentIdentifier{URI: uriA},
		Position:     Position{Line: 0, Character: 0},
	})
	require.Nil(t, errResp)
	var items []callHierarchyItem
	require.NoError(t, json.Unmarshal(raw, &items))
	require.Len(t, items, 1)

	raw, errResp = h.request("callHierarchy/outgoingCalls", callHierarchyOutgoingCallsParams{Item: items[0]})
	require.Nil(t, errResp)
	var calls []callHierarchyOutgoingCall
	require.NoError(t, json.Unmarshal(raw, &calls))
	require.Len(t, calls, 1)
	assert.Equal(t, "b.md", calls[0].To.Name)
}

// silence unused warnings in this file
var (
	_ = context.Background
	_ = rule.All
)
