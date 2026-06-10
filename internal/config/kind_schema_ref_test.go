package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// decodeSchemaRef decodes a YAML fragment as the `schema:` value of a
// kind body, returning the parsed KindSchemaRef. The fragment is the
// raw value (scalar, mapping, sequence, or null) — not wrapped in a
// key — so the test exercises KindSchemaRef.UnmarshalYAML directly.
func decodeSchemaRef(t *testing.T, frag string) (KindSchemaRef, error) {
	t.Helper()
	var ref KindSchemaRef
	err := yaml.Unmarshal([]byte(frag), &ref)
	return ref, err
}

// TestKindSchemaRef_Scalar covers a bare-string `schema:` value: it is
// a registry reference, so Name is set and Map() is empty (the body is
// resolved later, at load time).
func TestKindSchemaRef_Scalar(t *testing.T) {
	ref, err := decodeSchemaRef(t, `rfc-v1`)
	require.NoError(t, err)
	assert.Equal(t, "rfc-v1", ref.Name)
	assert.Empty(t, ref.Map())
}

// TestKindSchemaRef_Mapping covers an inline `schema:` mapping: Name is
// empty and Map() returns the decoded body.
func TestKindSchemaRef_Mapping(t *testing.T) {
	ref, err := decodeSchemaRef(t, `
frontmatter:
  title: 'string'
sections:
  - heading: "Overview"
`)
	require.NoError(t, err)
	assert.Empty(t, ref.Name)
	require.NotNil(t, ref.Map())
	assert.Contains(t, ref.Map(), "frontmatter")
	assert.Contains(t, ref.Map(), "sections")
}

// TestKindSchemaRef_Null covers a null `schema:` value: it is neither a
// named reference nor an inline body, so both Name and Map() are empty
// and no error fires (the kind simply declares no schema).
func TestKindSchemaRef_Null(t *testing.T) {
	ref, err := decodeSchemaRef(t, `null`)
	require.NoError(t, err)
	assert.Empty(t, ref.Name)
	assert.Empty(t, ref.Map())
}

// TestKindSchemaRef_Sequence rejects a list `schema:` value — a schema
// is a name or a mapping, never a sequence.
func TestKindSchemaRef_Sequence(t *testing.T) {
	_, err := decodeSchemaRef(t, `[a, b]`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "schema")
}

// TestKindSchemaRef_MalformedScalar rejects a scalar that is not a
// valid schema name (it must match the same `[a-z][a-z0-9-]*` rule a
// schema file's basename carries).
func TestKindSchemaRef_MalformedScalar(t *testing.T) {
	_, err := decodeSchemaRef(t, `Not A Name`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "schema")
}

// TestKindSchemaRef_EmptyScalar rejects an empty-string scalar: an
// empty name resolves to nothing and is almost certainly a mistake.
func TestKindSchemaRef_EmptyScalar(t *testing.T) {
	_, err := decodeSchemaRef(t, `""`)
	require.Error(t, err)
}

// TestKindSchemaRef_MapAccessorOnZero pins that the Map() accessor is
// safe to call on a zero-value ref (no inline body, no name).
func TestKindSchemaRef_MapAccessorOnZero(t *testing.T) {
	var ref KindSchemaRef
	assert.Empty(t, ref.Map())
}

// TestKindSchemaRef_NullNodeDirect drives UnmarshalYAML with an
// explicit !!null scalar node. yaml.v3 skips custom unmarshalers for
// null values during a regular decode, so this branch is only
// reachable by direct invocation — the test pins the method's
// contract for callers that feed nodes straight in.
func TestKindSchemaRef_NullNodeDirect(t *testing.T) {
	node := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!null", Value: "null"}
	var ref KindSchemaRef
	require.NoError(t, ref.UnmarshalYAML(node))
	assert.Empty(t, ref.Name)
	assert.Empty(t, ref.Map())
}

// TestKindSchemaRef_ScalarDecodeError covers the scalar arm whose
// Decode fails: a !!binary scalar with invalid base64 cannot decode
// into a string, and the error carries the schema: prefix.
func TestKindSchemaRef_ScalarDecodeError(t *testing.T) {
	_, err := decodeSchemaRef(t, `!!binary "not-base64!"`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "schema")
}

// TestKindSchemaRef_MappingDecodeError covers the mapping arm whose
// Decode fails: yaml.v3 rejects a mapping with duplicate keys.
func TestKindSchemaRef_MappingDecodeError(t *testing.T) {
	_, err := decodeSchemaRef(t, `{a: 1, a: 2}`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "schema")
}

// TestInlineSchema pins the exported constructor for out-of-package
// callers: the body lands in Map(), with no name and no source path.
func TestInlineSchema(t *testing.T) {
	m := map[string]any{"closed": true}
	ref := InlineSchema(m)
	assert.Empty(t, ref.Name)
	assert.Empty(t, ref.SourcePath)
	assert.Equal(t, m, ref.Map())
}

// TestInlineSchemaWithSource pins the exported resolved-ref
// constructor: name, body, and source path are all carried.
func TestInlineSchemaWithSource(t *testing.T) {
	m := map[string]any{"filename": "a.md"}
	ref := InlineSchemaWithSource("rfc-v1", m, ".mdsmith/schemas/rfc-v1.yaml")
	assert.Equal(t, "rfc-v1", ref.Name)
	assert.Equal(t, ".mdsmith/schemas/rfc-v1.yaml", ref.SourcePath)
	assert.Equal(t, m, ref.Map())
}

// TestNodeKindName covers every yaml.Kind the error renderer names,
// plus the zero-kind fallback.
func TestNodeKindName(t *testing.T) {
	cases := map[yaml.Kind]string{
		yaml.SequenceNode: "sequence",
		yaml.MappingNode:  "mapping",
		yaml.ScalarNode:   "scalar",
		yaml.AliasNode:    "alias",
		yaml.DocumentNode: "document",
		yaml.Kind(0):      "value",
	}
	for kind, want := range cases {
		assert.Equal(t, want, nodeKindName(kind))
	}
}
