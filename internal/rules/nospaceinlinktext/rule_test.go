package nospaceinlinktext

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func check(t *testing.T, src string, checkImages bool) []lint.Diagnostic {
	t.Helper()
	f, err := lint.NewFile("test.md", []byte(src))
	require.NoError(t, err)
	r := &Rule{CheckImages: checkImages}
	return r.Check(f)
}

func fix(t *testing.T, src string, checkImages bool) string {
	t.Helper()
	f, err := lint.NewFile("test.md", []byte(src))
	require.NoError(t, err)
	r := &Rule{CheckImages: checkImages}
	return string(r.Fix(f))
}

func TestCleanLink(t *testing.T) {
	diags := check(t, "# T\n\n[text](url)\n", true)
	assert.Empty(t, diags)
}

func TestLeadingSpace(t *testing.T) {
	diags := check(t, "# T\n\n[ text](url)\n", true)
	require.Len(t, diags, 1)
	assert.Equal(t, "link text has leading whitespace", diags[0].Message)
}

func TestTrailingSpace(t *testing.T) {
	diags := check(t, "# T\n\n[text ](url)\n", true)
	require.Len(t, diags, 1)
	assert.Equal(t, "link text has trailing whitespace", diags[0].Message)
}

func TestLeadingAndTrailingSpace(t *testing.T) {
	diags := check(t, "# T\n\n[ text ](url)\n", true)
	require.Len(t, diags, 2)
	msgs := []string{diags[0].Message, diags[1].Message}
	assert.Contains(t, msgs, "link text has leading whitespace")
	assert.Contains(t, msgs, "link text has trailing whitespace")
}

func TestFixLeadingAndTrailing(t *testing.T) {
	result := fix(t, "# T\n\n[ text ](url)\n", true)
	assert.Equal(t, "# T\n\n[text](url)\n", result)
}

func TestImageLeadingSpace(t *testing.T) {
	diags := check(t, "# T\n\n![ alt](img.png)\n", true)
	require.Len(t, diags, 1)
	assert.Equal(t, "image alt text has leading whitespace", diags[0].Message)
}

func TestImageTrailingSpace(t *testing.T) {
	diags := check(t, "# T\n\n![alt ](img.png)\n", true)
	require.Len(t, diags, 1)
	assert.Equal(t, "image alt text has trailing whitespace", diags[0].Message)
}

func TestImageBothSpaces(t *testing.T) {
	diags := check(t, "# T\n\n![ alt ](img.png)\n", true)
	require.Len(t, diags, 2)
	msgs := []string{diags[0].Message, diags[1].Message}
	assert.Contains(t, msgs, "image alt text has leading whitespace")
	assert.Contains(t, msgs, "image alt text has trailing whitespace")
}

func TestImageFixBothSpaces(t *testing.T) {
	result := fix(t, "# T\n\n![ alt ](img.png)\n", true)
	assert.Equal(t, "# T\n\n![alt](img.png)\n", result)
}

func TestCheckImagesDisabled(t *testing.T) {
	diags := check(t, "# T\n\n![ alt ](img.png)\n", false)
	assert.Empty(t, diags)
}

func TestNewlineInLinkTextNotFlagged(t *testing.T) {
	// Newline between words inside brackets must not be flagged.
	diags := check(t, "# T\n\n[long text that\nwraps](url)\n", true)
	assert.Empty(t, diags)
}

func TestNoChange(t *testing.T) {
	src := "# T\n\n[text](url)\n"
	result := fix(t, src, true)
	assert.Equal(t, src, result)
}

func TestApplySettings_CheckImages(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"check-images": false})
	require.NoError(t, err)
	assert.False(t, r.CheckImages)
}

func TestApplySettings_Unknown(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"unknown": true})
	require.Error(t, err)
}

func TestDefaultSettings(t *testing.T) {
	r := &Rule{}
	s := r.DefaultSettings()
	assert.Equal(t, true, s["check-images"])
}

func TestEnabledByDefault(t *testing.T) {
	r := &Rule{}
	assert.False(t, r.EnabledByDefault())
}
