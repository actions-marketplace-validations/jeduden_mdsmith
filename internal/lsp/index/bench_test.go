package index

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// BenchmarkColdBuild1k measures the cost of indexing 1 000 synthetic
// files. Plan 131 sets a budget of 1 s for this size — anything
// noticeably slower would block the lazy-build path the LSP server
// runs on the first symbol request.
func BenchmarkColdBuild1k(b *testing.B) {
	if testing.Short() {
		b.Skip("benchmark skipped in -short mode")
	}
	files, loader := buildSyntheticCorpus(1000)
	const budget = time.Second

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		idx := New("/root")
		start := time.Now()
		idx.Build(files, loader)
		elapsed := time.Since(start)
		b.ReportMetric(float64(elapsed.Milliseconds()), "build_ms")
		if elapsed > budget {
			b.Fatalf("cold build took %v (> %v) on %d files", elapsed, budget, len(files))
		}
	}
}

// BenchmarkIncrementalUpdate measures one Update on an established
// index. Plan 131 sets a 20 ms budget per `didChange`.
func BenchmarkIncrementalUpdate(b *testing.B) {
	if testing.Short() {
		b.Skip("benchmark skipped in -short mode")
	}
	files, loader := buildSyntheticCorpus(1000)
	idx := New("/root")
	idx.Build(files, loader)
	const budget = 20 * time.Millisecond

	src := []byte(syntheticBody(0))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		start := time.Now()
		idx.Update(files[i%len(files)], src)
		elapsed := time.Since(start)
		if elapsed > budget {
			b.Fatalf("update took %v (> %v)", elapsed, budget)
		}
	}
}

func buildSyntheticCorpus(n int) ([]string, func(string) ([]byte, error)) {
	files := make([]string, 0, n)
	bodies := make(map[string][]byte, n)
	for i := 0; i < n; i++ {
		path := fmt.Sprintf("docs/file_%05d.md", i)
		files = append(files, path)
		bodies[path] = []byte(syntheticBody(i))
	}
	return files, func(p string) ([]byte, error) {
		if b, ok := bodies[p]; ok {
			return b, nil
		}
		return nil, fmt.Errorf("not found: %s", p)
	}
}

func syntheticBody(seed int) string {
	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "title: File %d\nkinds:\n  - reference\n", seed)
	b.WriteString("---\n")
	fmt.Fprintf(&b, "# Top heading %d\n\n", seed)
	for s := 0; s < 5; s++ {
		fmt.Fprintf(&b, "## Section %d-%d\n\n", seed, s)
		next := (seed + 1) % 1000
		fmt.Fprintf(&b,
			"Body for section %d.%d with [a link](./file_%05d.md#top-heading-%d).\n\n",
			seed, s, next, next)
	}
	return b.String()
}
