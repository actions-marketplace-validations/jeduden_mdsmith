// Package extract is the published CUE contract for the typed
// output grammar of `mdsmith extract`: the block list a
// `projection: blocks` section emits (#Block) and the inline-span
// list a paragraph emits under `projection: inline` or the
// `block-paragraphs: inline` option (#Span).
//
// mdsmith embeds this file via go:embed; a differential test
// validates every extract fixture's JSON against these definitions,
// so the grammar cannot drift from the implementation. The
// definitions are closed (`close({...})`) so an unexpected key fails
// validation rather than being silently accepted.
//
// See plan/246_block-projection-full-extract.md (block grammar) and
// plan/212_extract-inline-spans.md (span grammar).
package extract

// #Span is one inline span. Leaf spans (text, code, autolink) carry a
// `value`; container spans (emphasis, strong, link, image) carry a
// recursive `children` list. `break` records a wrapped line.
#Span: close({block_span_text}) |
	close({block_span_break}) |
	close({block_span_code}) |
	close({block_span_autolink}) |
	close({block_span_emphasis}) |
	close({block_span_strong}) |
	close({block_span_link}) |
	close({block_span_image})

block_span_text: {span: "text", value: string}
block_span_break: {span: "break", hard: bool}
block_span_code: {span: "code", value: string}
block_span_autolink: {span: "autolink", value: string, url: string}
block_span_emphasis: {span: "emphasis", level: 1, children: [...#Span]}
block_span_strong: {span: "strong", level: 2, children: [...#Span]}
block_span_link: {span: "link", url: string, title?: string, children: [...#Span]}
// `image` appears only under the lenient block-mode inline option.
block_span_image: {span: "image", url: string, title?: string, children: [...#Span]}

// #Item is one list item in the structured `tree` / block-list item
// shape (plan 244): its own `text`, an optional `checked` bool on a
// GFM task item, and an optional recursive `children` sub-list.
#Item: close({
	text: string
	checked?: bool
	children?: [...#Item]
})

// #Block is one block in a `blocks` list. Container blocks (quote,
// section) carry a recursive `blocks` list; leaves carry their own
// payload. A `paragraph` carries either flat `text` or an `inline`
// span list, never both.
#Block: close({block_para_text}) |
	close({block_para_inline}) |
	close({block_code}) |
	close({block_list}) |
	close({block_table}) |
	close({block_quote}) |
	close({block_break}) |
	close({block_html}) |
	close({block_section})

block_para_text: {block: "paragraph", text: string}
block_para_inline: {block: "paragraph", inline: [...#Span]}
block_code: {block: "code", lang?: string, value: string}
block_list: {block: "list", items: [...#Item]}
block_table: {block: "table", columns: [...string], rows: [...[...string]]}
block_quote: {block: "quote", blocks: [...#Block]}
block_break: {block: "break"}
block_html: {block: "html", value: string}
block_section: {block: "section", level: int, heading: string, blocks: [...#Block]}
