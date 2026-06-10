package cuelite_test

import (
	"fmt"
	"strings"

	"github.com/jeduden/mdsmith/cue/cuelite"
)

// Validate marshalled front matter against a schema: compile both
// sides, unify them, and validate the merged value.
func Example() {
	schema, err := cuelite.Compile("title: string\nstatus: \"draft\" | \"final\"")
	if err != nil {
		panic(err)
	}
	doc, err := cuelite.CompileJSON([]byte(`{"title": "Roadmap", "status": "final"}`))
	if err != nil {
		panic(err)
	}
	fmt.Println(schema.Unify(doc).Validate())
	// Output: <nil>
}

// A rejection decomposes into one PathError per failing field. The
// example prints only the paths: the message text belongs to the
// underlying engine and is not part of the package's contract.
func ExampleErrors() {
	schema, _ := cuelite.Compile("title: string\ncount: int")
	doc, _ := cuelite.CompileJSON([]byte(`{"title": 7, "count": "many"}`))
	err := schema.Unify(doc).Validate()
	for _, pathErr := range cuelite.Errors(err) {
		fmt.Println(strings.Join(pathErr.Path(), "."))
	}
	// Output:
	// title
	// count
}

// The input must be strict JSON: a duplicate object key is rejected
// before the CUE lift, naming the offending key.
func ExampleCompileJSON() {
	_, err := cuelite.CompileJSON([]byte(`{"status": "draft", "status": "final"}`))
	fmt.Println(err)
	// Output: duplicate JSON key "status"
}

// ParsePath parses a CUE field-path expression into its unquoted
// per-selector segments. A quoted label is decoded — the quotes are
// stripped and CUE string escapes applied — so "my-key".sub yields the
// two segments "my-key" and "sub".
func ExampleParsePath() {
	p, err := cuelite.ParsePath(`"my-key".sub`)
	if err != nil {
		panic(err)
	}
	fmt.Printf("%q\n", p.Segments())
	// Output: ["my-key" "sub"]
}

// MakePath constructs a Path directly from segments, and deliberately
// accepts data keys ParsePath would reject. "my-key" needs quoting to
// parse as an expression, but MakePath stores it verbatim — the
// documented asymmetry that lets a path be built from any map key (e.g.
// over Fields() iteration) without round-tripping through the parser.
func ExampleMakePath() {
	p := cuelite.MakePath("my-key", "sub")
	fmt.Printf("%q\n", p.Segments())

	// The same key cannot be PARSED back: bare "my-key" is not a CUE
	// expression-grammar label.
	_, err := cuelite.ParsePath("my-key")
	fmt.Println(err != nil)
	// Output:
	// ["my-key" "sub"]
	// true
}
