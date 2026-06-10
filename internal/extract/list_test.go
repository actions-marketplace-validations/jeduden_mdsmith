package extract

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// listScope builds a single-section schema whose body is one list
// content entry projected with the given mode (empty for the flat
// default).
func listScope(projection string) *schema.Schema {
	return &schema.Schema{
		RootLevel: 2,
		Sections: []schema.Scope{{
			Heading: "Items",
			Matcher: &schema.Matcher{Regex: "Items"},
			Content: []schema.ContentEntry{{
				Kind:       schema.ContentKindList,
				Required:   true,
				Projection: projection,
			}},
		}},
	}
}

// runList projects body through listScope(projection) and returns the
// `items` value plus any diagnostics.
func runList(t *testing.T, body, projection string) (any, []lint.Diagnostic) {
	t.Helper()
	got, diags := run(t, body, listScope(projection), nil)
	if len(diags) > 0 {
		return nil, diags
	}
	items := got.(map[string]any)["items"].(map[string]any)["items"]
	return items, nil
}

// TestExtract_FlatListNoNestedConcatenation is the plan-244 bugfix
// reproduction: a parent item's own text must not absorb a nested
// child's text. The corrupt behaviour emitted
// "open item with boldnested child"; the fix emits only the parent's
// own inline text ("open item with bold"), children excluded.
func TestExtract_FlatListNoNestedConcatenation(t *testing.T) {
	body := "## Items\n\n" +
		"- [x] done item\n" +
		"- [ ] open item with **bold**\n" +
		"  - nested child\n"
	items, diags := runList(t, body, "")
	require.Empty(t, diags)
	assert.Equal(t, []any{
		"[x] done item",
		"[ ] open item with bold",
	}, items)
}

// TestExtract_FlatListItemOnlyNestedList pins the design decision the
// plan calls out: a top-level item whose only content is a nested
// sub-list has no own text, so flat mode projects it as the empty
// string. The item keeps its slot in the array (order preserved); the
// nested child is excluded, since flat mode emits own text only.
func TestExtract_FlatListItemOnlyNestedList(t *testing.T) {
	body := "## Items\n\n" +
		"- parent\n" +
		"-\n" +
		"  - lonely child\n"
	items, diags := runList(t, body, "")
	require.Empty(t, diags)
	assert.Equal(t, []any{"parent", ""}, items)
}
