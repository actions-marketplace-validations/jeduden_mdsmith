package difftest

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// benchCase is the representative schema-plus-data the benchmark and
// its correctness guard both run: a small front-matter-shaped struct
// with a string literal, a bounded int, and a regex-constrained slug —
// the constraint atoms MDS020 and the query surface actually use.
func benchCase() Case {
	return Case{
		Name:   "front matter",
		Schema: `{status: "✅", weight: >=0 & <=100, slug: =~"^[a-z-]+$"}`,
		Data:   `{"status": "✅", "weight": 42, "slug": "release-gating"}`,
	}
}

// TestBenchCaseAccepted guards the benchmark: both paths must accept
// benchCase, so the two BenchmarkValidate sub-benchmarks measure the
// same accepting workload and the benchmark's correctness is checked on
// every ordinary CI test run, not only under -bench.
func TestBenchCaseAccepted(t *testing.T) {
	c := benchCase()
	require.True(t, CueLitePath(c).Accepted, "cuelite path must accept the benchmark case")
	require.True(t, OraclePath(c).Accepted, "oracle path must accept the benchmark case")
}

// BenchmarkValidate measures compile-plus-validate throughput of the
// cuelite façade against the direct CUE oracle on the same case, so a
// later flip to the in-house engine can be compared head to head.
func BenchmarkValidate(b *testing.B) {
	c := benchCase()
	b.Run("cuelite", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if !CueLitePath(c).Accepted {
				b.Fatal("cuelite path rejected the benchmark case")
			}
		}
	})
	b.Run("cue", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if !OraclePath(c).Accepted {
				b.Fatal("oracle path rejected the benchmark case")
			}
		}
	})
}
