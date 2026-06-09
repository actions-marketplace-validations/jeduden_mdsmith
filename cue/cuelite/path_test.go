package cuelite_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/cue/cuelite"
)

// TestParsePath_accepted covers inputs ParsePath accepts and the
// segments it produces.
func TestParsePath_accepted(t *testing.T) {
	t.Run("simple ident", func(t *testing.T) {
		p, err := cuelite.ParsePath("title")
		require.NoError(t, err)
		assert.Equal(t, []string{"title"}, p.Segments())
	})
	t.Run("dotted idents", func(t *testing.T) {
		p, err := cuelite.ParsePath("a.b.c")
		require.NoError(t, err)
		assert.Equal(t, []string{"a", "b", "c"}, p.Segments())
	})
	t.Run("single quoted key", func(t *testing.T) {
		p, err := cuelite.ParsePath(`"my-key"`)
		require.NoError(t, err)
		assert.Equal(t, []string{"my-key"}, p.Segments())
	})
	t.Run("quoted key then ident", func(t *testing.T) {
		p, err := cuelite.ParsePath(`"my-key".sub`)
		require.NoError(t, err)
		assert.Equal(t, []string{"my-key", "sub"}, p.Segments())
	})
	t.Run("ident then quoted key", func(t *testing.T) {
		p, err := cuelite.ParsePath(`params."a.b"`)
		require.NoError(t, err)
		assert.Equal(t, []string{"params", "a.b"}, p.Segments())
	})
	t.Run("quoted key with dot inside", func(t *testing.T) {
		p, err := cuelite.ParsePath(`"a.b"`)
		require.NoError(t, err)
		assert.Equal(t, []string{"a.b"}, p.Segments())
	})
	t.Run("quoted key with escaped quote", func(t *testing.T) {
		p, err := cuelite.ParsePath(`"a\"b"`)
		require.NoError(t, err)
		assert.Equal(t, []string{`a"b`}, p.Segments())
	})
	t.Run("numeric-looking quoted segment", func(t *testing.T) {
		p, err := cuelite.ParsePath(`"123"`)
		require.NoError(t, err)
		assert.Equal(t, []string{"123"}, p.Segments())
	})
	t.Run("quoted key with unicode", func(t *testing.T) {
		p, err := cuelite.ParsePath(`"über"`)
		require.NoError(t, err)
		assert.Equal(t, []string{"über"}, p.Segments())
	})
}

// TestParsePath_rejected covers inputs ParsePath rejects, including each
// *PathError error shape.
func TestParsePath_rejected(t *testing.T) {
	t.Run("empty string is an error", func(t *testing.T) {
		_, err := cuelite.ParsePath("")
		require.Error(t, err)
		var pe *cuelite.PathError
		require.ErrorAs(t, err, &pe)
	})
	t.Run("trailing dot is an error", func(t *testing.T) {
		_, err := cuelite.ParsePath("a.")
		require.Error(t, err)
	})
	t.Run("leading dot is an error", func(t *testing.T) {
		_, err := cuelite.ParsePath(".a")
		require.Error(t, err)
	})
	t.Run("empty quoted segment is an error", func(t *testing.T) {
		// CUE accepts "" but fieldinterp rejects it (empty unquoted).
		_, err := cuelite.ParsePath(`""`)
		require.Error(t, err)
	})
	t.Run("malformed quoted segment is an error", func(t *testing.T) {
		_, err := cuelite.ParsePath(`"a"b`)
		require.Error(t, err)
	})
	t.Run("unterminated quoted segment is an error", func(t *testing.T) {
		// A quoted segment with no closing '"' must be rejected.
		_, err := cuelite.ParsePath(`"unterminated`)
		require.Error(t, err)
		var pe *cuelite.PathError
		require.ErrorAs(t, err, &pe)
	})
	t.Run("invalid escape in quoted segment is an error", func(t *testing.T) {
		// A lone surrogate escape passes the scan but strconv.Unquote rejects it.
		_, err := cuelite.ParsePath(`"\ud800"`)
		require.Error(t, err)
		var pe *cuelite.PathError
		require.ErrorAs(t, err, &pe)
	})
	t.Run("underscore-prefixed ident is an error", func(t *testing.T) {
		// CUE rejects hidden labels (_foo); the in-house parser must too.
		_, err := cuelite.ParsePath("_foo")
		require.Error(t, err)
	})
	t.Run("bare numeric ident is an error", func(t *testing.T) {
		// CUE treats 123 as an index label (non-string); the in-house parser
		// must reject it rather than accepting it as a field name.
		_, err := cuelite.ParsePath("123")
		require.Error(t, err)
	})
}

// TestMakePath covers MakePath and round-trip through Segments.
func TestMakePath(t *testing.T) {
	t.Run("single segment", func(t *testing.T) {
		p := cuelite.MakePath("title")
		assert.Equal(t, []string{"title"}, p.Segments())
	})
	t.Run("multiple segments", func(t *testing.T) {
		p := cuelite.MakePath("a", "b", "c")
		assert.Equal(t, []string{"a", "b", "c"}, p.Segments())
	})
	t.Run("zero segments", func(t *testing.T) {
		p := cuelite.MakePath()
		assert.Nil(t, p.Segments())
	})
	t.Run("segments with hyphens", func(t *testing.T) {
		p := cuelite.MakePath("my-key", "sub")
		assert.Equal(t, []string{"my-key", "sub"}, p.Segments())
	})
}

// TestPath_Segments_returnsCopy ensures Segments returns a fresh copy so
// callers cannot corrupt the Path's internal state.
func TestPath_Segments_returnsCopy(t *testing.T) {
	p := cuelite.MakePath("a", "b", "c")
	got := p.Segments()
	require.Len(t, got, 3)
	got[0] = "MUTATED"
	// A second call must still see the original.
	assert.Equal(t, []string{"a", "b", "c"}, p.Segments())
}
