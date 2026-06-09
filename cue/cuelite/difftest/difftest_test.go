package difftest

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recorder is a TestingT that records failures instead of failing, so
// the harness's own failure path can be asserted.
type recorder struct {
	helperCalls int
	failures    []string
}

func (r *recorder) Helper() { r.helperCalls++ }

func (r *recorder) Errorf(format string, args ...any) {
	r.failures = append(r.failures, format)
}

func TestOutcome_Equal(t *testing.T) {
	t.Run("both accept regardless of path", func(t *testing.T) {
		a := Outcome{Accepted: true, Path: []string{"x"}}
		b := Outcome{Accepted: true}
		assert.True(t, a.Equal(b))
	})
	t.Run("accept versus reject differ", func(t *testing.T) {
		assert.False(t, Outcome{Accepted: true}.Equal(Outcome{Accepted: false}))
	})
	t.Run("reject with same path are equal", func(t *testing.T) {
		a := Outcome{Path: []string{"status"}}
		b := Outcome{Path: []string{"status"}}
		assert.True(t, a.Equal(b))
	})
	t.Run("reject with different path differ", func(t *testing.T) {
		a := Outcome{Path: []string{"status"}}
		b := Outcome{Path: []string{"title"}}
		assert.False(t, a.Equal(b))
	})
}

func TestCueLitePath(t *testing.T) {
	t.Run("accepts conforming data", func(t *testing.T) {
		o := CueLitePath(Case{Schema: `{status: string}`, Data: `{"status": "done"}`})
		assert.True(t, o.Accepted)
	})
	t.Run("rejects with field path", func(t *testing.T) {
		o := CueLitePath(Case{Schema: `{status: "✅"}`, Data: `{"status": "🔲"}`})
		require.False(t, o.Accepted)
		assert.Equal(t, []string{"status"}, o.Path)
	})
	t.Run("rejects on schema compile error", func(t *testing.T) {
		o := CueLitePath(Case{Schema: `{status: =}`, Data: `{"status": "x"}`})
		assert.False(t, o.Accepted)
		assert.Nil(t, o.Path)
	})
	t.Run("rejects on data compile error", func(t *testing.T) {
		o := CueLitePath(Case{Schema: `{status: string}`, Data: `{not json`})
		assert.False(t, o.Accepted)
		assert.Nil(t, o.Path)
	})
}

func TestOraclePath(t *testing.T) {
	t.Run("accepts conforming data", func(t *testing.T) {
		o := OraclePath(Case{Schema: `{status: string}`, Data: `{"status": "done"}`})
		assert.True(t, o.Accepted)
	})
	t.Run("rejects with field path", func(t *testing.T) {
		o := OraclePath(Case{Schema: `{status: "✅"}`, Data: `{"status": "🔲"}`})
		require.False(t, o.Accepted)
		assert.Equal(t, []string{"status"}, o.Path)
	})
	t.Run("rejects on schema compile error", func(t *testing.T) {
		o := OraclePath(Case{Schema: `{status: =}`, Data: `{"status": "x"}`})
		assert.False(t, o.Accepted)
		assert.Nil(t, o.Path)
	})
	t.Run("rejects on data compile error", func(t *testing.T) {
		o := OraclePath(Case{Schema: `{status: string}`, Data: `{not json`})
		assert.False(t, o.Accepted)
		assert.Nil(t, o.Path)
	})
}

func TestCompare(t *testing.T) {
	t.Run("agreement records no failure", func(t *testing.T) {
		r := &recorder{}
		ok := Compare(r, CueLitePath, OraclePath, Case{Schema: `{a: string}`, Data: `{"a": "x"}`})
		assert.True(t, ok)
		assert.Empty(t, r.failures)
		assert.Positive(t, r.helperCalls)
	})
	t.Run("disagreement records a failure", func(t *testing.T) {
		r := &recorder{}
		accept := func(Case) Outcome { return Outcome{Accepted: true} }
		reject := func(Case) Outcome { return Outcome{Accepted: false} }
		ok := Compare(r, accept, reject, Case{Name: "mismatch"})
		assert.False(t, ok)
		assert.Len(t, r.failures, 1)
	})
}

func TestRun(t *testing.T) {
	r := &recorder{}
	Run(r, []Case{
		{Name: "ok", Schema: `{a: string}`, Data: `{"a": "x"}`},
		{Name: "bad", Schema: `{a: "✅"}`, Data: `{"a": "🔲"}`},
	})
	assert.Empty(t, r.failures)
	assert.Positive(t, r.helperCalls)
}

// TestRun_corpus is the CI-visible differential run: the in-house path
// and the oracle must agree on every case in the representative corpus.
func TestRun_corpus(t *testing.T) {
	Run(t, corpus())
}

// corpus is a representative set of schema/data cases spanning accept,
// scalar-mismatch reject, and nested-field reject.
func corpus() []Case {
	return []Case{
		{Name: "string ok", Schema: `{status: string}`, Data: `{"status": "done"}`},
		{Name: "int bound ok", Schema: `{n: >=0}`, Data: `{"n": 3}`},
		{Name: "int bound reject", Schema: `{n: >=0}`, Data: `{"n": -1}`},
		{Name: "literal reject", Schema: `{status: "✅"}`, Data: `{"status": "🔲"}`},
		{Name: "regex ok", Schema: `{slug: =~"^[a-z]+$"}`, Data: `{"slug": "abc"}`},
		{Name: "regex reject", Schema: `{slug: =~"^[a-z]+$"}`, Data: `{"slug": "AB1"}`},
		{Name: "nested reject", Schema: `{meta: {status: "✅"}}`, Data: `{"meta": {"status": "x"}}`},
	}
}
