package extract

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// blocksScope builds a single named section that projects its whole
// body via `projection: blocks`.
func blocksScope(heading string) *schema.Schema {
	return &schema.Schema{
		RootLevel: 2,
		Sections: []schema.Scope{{
			Heading:    heading,
			Matcher:    &schema.Matcher{Regex: heading},
			Projection: schema.ProjectionBlocks,
		}},
	}
}

// TestExtract_ScopeBlocksWholeBody is the plan-246 worked example: a
// scope with `projection: blocks` projects its entire body, in
// document order, with containers recursing.
func TestExtract_ScopeBlocksWholeBody(t *testing.T) {
	body := "## Notes\n\n" +
		"First paragraph.\n\n" +
		"```go\nfunc F() {}\n```\n\n" +
		"> A quoted line.\n\n" +
		"- one item\n\n" +
		"| A |\n| - |\n| 1 |\n"
	got, diags := run(t, body, blocksScope("Notes"), nil)
	require.Empty(t, diags)
	notes := got.(map[string]any)["notes"].(map[string]any)
	blocks := notes["blocks"].([]any)
	assert.Equal(t, []any{
		map[string]any{"block": "paragraph", "text": "First paragraph."},
		map[string]any{"block": "code", "lang": "go", "value": "func F() {}\n"},
		map[string]any{"block": "quote", "blocks": []any{
			map[string]any{"block": "paragraph", "text": "A quoted line."},
		}},
		map[string]any{"block": "list", "items": []any{
			map[string]any{"text": "one item"},
		}},
		map[string]any{"block": "table", "columns": []any{"A"}, "rows": []any{[]any{"1"}}},
	}, blocks)
}

// A deeper heading inside a blocks-projected section nests as a
// `section` block, recursive, with the heading text preserved.
func TestExtract_ScopeBlocksNestsDeeperHeading(t *testing.T) {
	body := "## Notes\n\nlead para\n\n### Detail\n\ndetail para\n"
	got, diags := run(t, body, blocksScope("Notes"), nil)
	require.Empty(t, diags)
	blocks := got.(map[string]any)["notes"].(map[string]any)["blocks"].([]any)
	require.Len(t, blocks, 2)
	assert.Equal(t, map[string]any{"block": "paragraph", "text": "lead para"}, blocks[0])
	assert.Equal(t, map[string]any{
		"block":   "section",
		"level":   3,
		"heading": "Detail",
		"blocks": []any{
			map[string]any{"block": "paragraph", "text": "detail para"},
		},
	}, blocks[1])
}

// A scope can declare content entries AND project blocks; the two
// coexist as sibling keys (the declared `text`, plus `blocks`).
func TestExtract_ScopeBlocksAlongsideContent(t *testing.T) {
	sch := &schema.Schema{
		RootLevel: 2,
		Sections: []schema.Scope{{
			Heading:    "Notes",
			Matcher:    &schema.Matcher{Regex: "Notes"},
			Projection: schema.ProjectionBlocks,
			Content: []schema.ContentEntry{
				{Kind: schema.ContentKindParagraph, Required: true},
			},
		}},
	}
	got, diags := run(t, "## Notes\n\nintro\n", sch, nil)
	require.Empty(t, diags)
	notes := got.(map[string]any)["notes"].(map[string]any)
	assert.Equal(t, "intro", notes["text"])
	blocks := notes["blocks"].([]any)
	assert.Equal(t, []any{
		map[string]any{"block": "paragraph", "text": "intro"},
	}, blocks)
}

// A declared content entry that binds to `blocks` collides with the
// whole-body blocks key — reported, not silently overwritten.
func TestExtract_ScopeBlocksKeyCollidesWithBoundEntry(t *testing.T) {
	bindBlocks := "blocks"
	sch := &schema.Schema{
		RootLevel: 2,
		Sections: []schema.Scope{{
			Heading:    "Notes",
			Matcher:    &schema.Matcher{Regex: "Notes"},
			Projection: schema.ProjectionBlocks,
			Content: []schema.ContentEntry{
				{Kind: schema.ContentKindParagraph, Required: true, Bind: &bindBlocks},
			},
		}},
	}
	_, diags := run(t, "## Notes\n\nintro\n", sch, nil)
	require.NotEmpty(t, diags)
	assert.Contains(t, diags[0].Message, "blocks")
}

// schemaBlocksDefault builds a schema with a single declared section
// plus a schema-level `projection: blocks` default.
func schemaBlocksDefault(declared string) *schema.Schema {
	return &schema.Schema{
		RootLevel:  2,
		Projection: schema.ProjectionBlocks,
		Sections: []schema.Scope{
			{Heading: declared, Matcher: &schema.Matcher{Regex: declared}},
		},
	}
}

// TestExtract_SchemaBlocksProjectsUnlisted is the plan-246 acceptance
// criterion: a schema-level `projection: blocks` projects an unlisted
// section under its slug, with its heading text preserved and its body
// as blocks — no section of a matched document is dropped.
func TestExtract_SchemaBlocksProjectsUnlisted(t *testing.T) {
	body := "## Goal\n\ndeclared body\n\n## Background\n\nbackground body\n"
	got, diags := run(t, body, schemaBlocksDefault("Goal"), nil)
	require.Empty(t, diags)
	root := got.(map[string]any)
	// Declared section: keyed object, now also carrying blocks.
	goal := root["goal"].(map[string]any)
	assert.Equal(t, []any{
		map[string]any{"block": "paragraph", "text": "declared body"},
	}, goal["blocks"])
	// Unlisted section: keyed by slug, heading text preserved.
	bg := root["background"].(map[string]any)
	assert.Equal(t, "Background", bg["heading"])
	assert.Equal(t, []any{
		map[string]any{"block": "paragraph", "text": "background body"},
	}, bg["blocks"])
}

// A declared scope under a schema-level blocks default has no
// `heading` key (its slug already names it); only unlisted scopes
// carry the `heading` text field.
func TestExtract_SchemaBlocksDeclaredHasNoHeadingKey(t *testing.T) {
	body := "## Goal\n\nx\n"
	got, diags := run(t, body, schemaBlocksDefault("Goal"), nil)
	require.Empty(t, diags)
	goal := got.(map[string]any)["goal"].(map[string]any)
	assert.NotContains(t, goal, "heading")
}

// Two unlisted sections whose headings slugify to the same key
// project as an array under that key (repeating matches -> array).
func TestExtract_SchemaBlocksRepeatingUnlistedArray(t *testing.T) {
	body := "## Goal\n\nx\n\n## Note\n\nfirst\n\n## Note\n\nsecond\n"
	got, diags := run(t, body, schemaBlocksDefault("Goal"), nil)
	require.Empty(t, diags)
	arr, ok := got.(map[string]any)["note"].([]any)
	require.True(t, ok, "repeated unlisted slug must project as an array")
	require.Len(t, arr, 2)
	assert.Equal(t, "first", arr[0].(map[string]any)["blocks"].([]any)[0].(map[string]any)["text"])
	assert.Equal(t, "second", arr[1].(map[string]any)["blocks"].([]any)[0].(map[string]any)["text"])
}

// An empty blocks-projected section emits an empty `blocks` array
// (not nil): the body slice is empty but non-nil, so the key appears.
func TestExtract_ScopeBlocksEmptyBody(t *testing.T) {
	body := "## Notes\n\n## Other\n\nx\n"
	sch := &schema.Schema{
		RootLevel: 2,
		Sections: []schema.Scope{
			{Heading: "Notes", Matcher: &schema.Matcher{Regex: "Notes"},
				Projection: schema.ProjectionBlocks},
			litScope("Other"),
		},
	}
	got, diags := run(t, body, sch, nil)
	require.Empty(t, diags)
	notes := got.(map[string]any)["notes"].(map[string]any)
	assert.Equal(t, []any{}, notes["blocks"])
}

// Schema-level blocks + plan 243's H1 title: a single switch yields
// the whole document as data — the H1 under `title`, every section
// (declared and unlisted) under its slug with `blocks`.
func TestExtract_SchemaBlocksWithH1Title(t *testing.T) {
	body := "# Doc Title\n\n## Goal\n\ng\n\n## Extra\n\ne\n"
	got, diags := run(t, body, schemaBlocksDefault("Goal"), nil)
	require.Empty(t, diags)
	root := got.(map[string]any)
	assert.Equal(t, "Doc Title", root["title"])
	assert.Contains(t, root["goal"].(map[string]any), "blocks")
	extra := root["extra"].(map[string]any)
	assert.Equal(t, "Extra", extra["heading"])
	assert.Contains(t, extra, "blocks")
}

// An unlisted heading whose slug collides with a declared scope's key
// is reported (the declared `goal`, then a second unlisted `## Goal`
// that the open schema tolerates but cannot key without colliding).
func TestExtract_SchemaBlocksUnlistedCollidesWithDeclared(t *testing.T) {
	body := "## Goal\n\nfirst\n\n## Goal\n\nsecond\n"
	_, diags := run(t, body, schemaBlocksDefault("Goal"), nil)
	require.NotEmpty(t, diags)
	assert.Contains(t, diags[0].Message, "goal")
}
