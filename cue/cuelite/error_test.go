package cuelite

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewPathError(t *testing.T) {
	err := newPathError([]string{"meta", "status"}, "value out of range")
	require.NotNil(t, err)
	assert.Equal(t, []string{"meta", "status"}, err.Path())
	assert.Equal(t, "meta.status: value out of range", err.Error())
}

func TestNewPathError_emptyPath(t *testing.T) {
	err := newPathError(nil, "front matter does not satisfy schema")
	require.NotNil(t, err)
	assert.Nil(t, err.Path())
	assert.Equal(t, "front matter does not satisfy schema", err.Error())
}

func TestPathError_Error(t *testing.T) {
	t.Run("with path", func(t *testing.T) {
		err := &PathError{path: []string{"tags", "1"}, msg: "must be a string"}
		assert.Equal(t, "tags.1: must be a string", err.Error())
	})
	t.Run("without path", func(t *testing.T) {
		err := &PathError{msg: "bare message"}
		assert.Equal(t, "bare message", err.Error())
	})
}

func TestPathError_Path(t *testing.T) {
	err := &PathError{path: []string{"a", "b"}}
	assert.Equal(t, []string{"a", "b"}, err.Path())
}

func TestPathError_errorsAs(t *testing.T) {
	var wrapped error = newPathError([]string{"x"}, "boom")
	var pe *PathError
	require.True(t, errors.As(wrapped, &pe))
	assert.Equal(t, []string{"x"}, pe.Path())
}

func TestErrors(t *testing.T) {
	t.Run("nil error yields nil", func(t *testing.T) {
		assert.Nil(t, Errors(nil))
	})
	t.Run("foreign error yields nil", func(t *testing.T) {
		assert.Nil(t, Errors(errors.New("not a path error")))
	})
	t.Run("single bare PathError yields one", func(t *testing.T) {
		got := Errors(newPathError([]string{"a"}, "boom"))
		require.Len(t, got, 1)
		assert.Equal(t, []string{"a"}, got[0].Path())
	})
	t.Run("joined PathErrors are all collected in order", func(t *testing.T) {
		joined := errors.Join(
			newPathError([]string{"a"}, "x"),
			newPathError([]string{"b"}, "y"),
		)
		got := Errors(joined)
		require.Len(t, got, 2)
		assert.Equal(t, []string{"a"}, got[0].Path())
		assert.Equal(t, []string{"b"}, got[1].Path())
	})
	t.Run("a join mixing a foreign error keeps only the PathErrors", func(t *testing.T) {
		joined := errors.Join(
			newPathError([]string{"a"}, "x"),
			errors.New("foreign"),
		)
		got := Errors(joined)
		require.Len(t, got, 1)
		assert.Equal(t, []string{"a"}, got[0].Path())
	})
}
