package render

import "testing"

// Round-trip: MD -> storage -> MD must be stable for supported elements.
func TestRoundTripElements(t *testing.T) {
	cases := []struct{ name, md string }{
		{"heading", "## Section Two\n"},
		{"paragraph", "plain text\n"},
		{"emphasis", "**bold** and *italic* and ~~struck~~\n"},
		{"inline code", "`x + y`\n"},
		{"link", "[home](https://example.com)\n"},
		{"image", "![alt](https://example.com/i.png)\n"},
		{"unordered list", "- a\n- b\n"},
		{"ordered list", "1. a\n2. b\n"},
		{"task list", "- [ ] todo\n- [x] done\n"},
		{"blockquote", "> quoted\n"},
		{"hr", "---\n"},
		{"fenced code", "```go\nfmt.Println(\"hi\")\n```\n"},
		{"table", "| a | b |\n| --- | --- |\n| 1 | 2 |\n"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			storage, err := MarkdownToStorage(tt.md)
			if err != nil {
				t.Fatalf("MarkdownToStorage: %v", err)
			}
			r, err := StorageToMarkdown(storage)
			if err != nil {
				t.Fatalf("StorageToMarkdown: %v", err)
			}
			if r.Fidelity != "exact" {
				t.Errorf("fidelity = %q, want exact (storage: %q)", r.Fidelity, storage)
			}
			if r.Markdown != tt.md {
				t.Errorf("round trip changed content:\n in: %q\nout: %q\nstorage: %q", tt.md, r.Markdown, storage)
			}
		})
	}
}

// A composite document must round-trip to semantically stable Markdown:
// converting the round-tripped Markdown again yields identical storage.
func TestRoundTripCompositeDocument(t *testing.T) {
	md := `# Title

Intro with **bold**, *italic*, ` + "`code`" + `, and a [link](https://example.com).

## List Section

- one
- two

1. first
2. second

- [ ] open task
- [x] closed task

## Code

` + "```python\nprint(\"hello\")\n```" + `

> a quote

| h1 | h2 |
| --- | --- |
| a | b |

---

![diagram](https://example.com/d.png)
`
	storage1, err := MarkdownToStorage(md)
	if err != nil {
		t.Fatalf("first MarkdownToStorage: %v", err)
	}
	r, err := StorageToMarkdown(storage1)
	if err != nil {
		t.Fatalf("StorageToMarkdown: %v", err)
	}
	if r.Fidelity != "exact" {
		t.Errorf("fidelity = %q, want exact", r.Fidelity)
	}
	storage2, err := MarkdownToStorage(r.Markdown)
	if err != nil {
		t.Fatalf("second MarkdownToStorage: %v", err)
	}
	if storage1 != storage2 {
		t.Errorf("storage not stable after round trip:\nfirst:  %q\nsecond: %q\nmd out: %q", storage1, storage2, r.Markdown)
	}
}

// CDATA edge: code containing "]]>" survives a full round trip.
func TestRoundTripCDATABoundary(t *testing.T) {
	md := "```\nend of cdata: ]]> and again ]]>]]>\n```\n"
	storage, err := MarkdownToStorage(md)
	if err != nil {
		t.Fatalf("MarkdownToStorage: %v", err)
	}
	r, err := StorageToMarkdown(storage)
	if err != nil {
		t.Fatalf("StorageToMarkdown: %v", err)
	}
	if r.Markdown != md {
		t.Errorf("CDATA round trip:\n in: %q\nout: %q\nstorage: %q", md, r.Markdown, storage)
	}
}
