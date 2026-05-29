package schema

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCompilePattern_FallbackForTestBuiltCrossRef covers the
// fallback path in compilePattern: a CrossRef constructed
// directly (compiled == nil) with a valid Pattern falls through
// to regexp.Compile instead of returning the pre-compiled field.
func TestCompilePattern_FallbackForTestBuiltCrossRef(t *testing.T) {
	cr := CrossRef{Pattern: `\bStep (\d+)\b`, MustMatch: "Step {n}"}
	re, err := cr.compilePattern()
	require.NoError(t, err)
	require.NotNil(t, re)
	assert.True(t, re.MatchString("Step 3"))
}

// TestCompilePattern_ReturnsPreCompiledWhenSet covers the
// pre-compiled fast path: when compiled is set, compilePattern
// returns it directly (pointer identity) without re-compiling.
func TestCompilePattern_ReturnsPreCompiledWhenSet(t *testing.T) {
	precompiled := regexp.MustCompile(`\bStep (\d+)\b`)
	cr := CrossRef{
		Pattern:   `\bStep (\d+)\b`,
		MustMatch: "Step {n}",
		compiled:  precompiled,
	}
	re, err := cr.compilePattern()
	require.NoError(t, err)
	assert.Same(t, precompiled, re)
}

// TestCompileSkip_FallbackForTestBuiltCrossRef covers the
// fallback path in compileSkip: a CrossRef with a non-empty
// SkipLinesMatching but nil compiledSkip falls through to
// regexp.Compile instead of returning the pre-compiled field.
func TestCompileSkip_FallbackForTestBuiltCrossRef(t *testing.T) {
	cr := CrossRef{
		Pattern:           `\bStep (\d+)\b`,
		MustMatch:         "Step {n}",
		SkipLinesMatching: `^>`,
	}
	re, err := cr.compileSkip()
	require.NoError(t, err)
	require.NotNil(t, re)
	assert.True(t, re.MatchString("> blockquote"))
}

// TestCompileSkip_ReturnsPreCompiledWhenSet covers the
// pre-compiled fast path: when compiledSkip is set, compileSkip
// returns it directly (pointer identity) without re-compiling.
func TestCompileSkip_ReturnsPreCompiledWhenSet(t *testing.T) {
	precompiled := regexp.MustCompile(`^>`)
	cr := CrossRef{
		Pattern:           `\bStep (\d+)\b`,
		MustMatch:         "Step {n}",
		SkipLinesMatching: `^>`,
		compiledSkip:      precompiled,
	}
	re, err := cr.compileSkip()
	require.NoError(t, err)
	assert.Same(t, precompiled, re)
}
