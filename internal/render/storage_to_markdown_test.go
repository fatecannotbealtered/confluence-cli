package render

import (
	"fmt"
	"strings"
	"testing"
)

func mustST(t *testing.T, storage string) Result {
	t.Helper()
	r, err := StorageToMarkdown(storage)
	if err != nil {
		t.Fatalf("StorageToMarkdown(%q) error: %v", storage, err)
	}
	return r
}

func exactMD(t *testing.T, storage string) string {
	t.Helper()
	r := mustST(t, storage)
	if r.Fidelity != "exact" {
		t.Errorf("StorageToMarkdown(%q) fidelity = %q, want exact", storage, r.Fidelity)
	}
	return r.Markdown
}

func TestStorageToMarkdownHeadings(t *testing.T) {
	for level := 1; level <= 6; level++ {
		storage := fmt.Sprintf("<h%d>Title</h%d>", level, level)
		want := strings.Repeat("#", level) + " Title\n"
		if got := exactMD(t, storage); got != want {
			t.Errorf("level %d: got %q, want %q", level, got, want)
		}
	}
}

func TestStorageToMarkdownInline(t *testing.T) {
	tests := []struct{ name, storage, want string }{
		{"paragraph", "<p>hello</p>", "hello\n"},
		{"strong", "<p><strong>b</strong></p>", "**b**\n"},
		{"b tag", "<p><b>b</b></p>", "**b**\n"},
		{"em", "<p><em>i</em></p>", "*i*\n"},
		{"del", "<p><del>gone</del></p>", "~~gone~~\n"},
		{"code", "<p><code>x()</code></p>", "`x()`\n"},
		{"code with backtick", "<p><code>a`b</code></p>", "`` a`b ``\n"},
		{"link", `<p><a href="https://e.com">text</a></p>`, "[text](https://e.com)\n"},
		{"link no text", `<p><a href="https://e.com"></a></p>`, "[https://e.com](https://e.com)\n"},
		{"img", `<p><img src="i.png" alt="pic"/></p>`, "![pic](i.png)\n"},
		{"entities", "<p>a &amp; b &lt; c</p>", "a & b < c\n"},
		{"md specials escaped", "<p>2 * 3 [x]</p>", `2 \* 3 \[x\]` + "\n"},
	}
	for _, tt := range tests {
		if got := exactMD(t, tt.storage); got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestStorageToMarkdownLists(t *testing.T) {
	tests := []struct{ name, storage, want string }{
		{"unordered", "<ul><li>a</li><li>b</li></ul>", "- a\n- b\n"},
		{"ordered", "<ol><li>a</li><li>b</li></ol>", "1. a\n2. b\n"},
		{"ordered start", `<ol start="3"><li>a</li></ol>`, "3. a\n"},
		{"nested", "<ul><li>a<ul><li>b</li></ul></li></ul>", "- a\n  - b\n"},
		{"task list",
			"<ac:task-list>" +
				"<ac:task><ac:task-status>incomplete</ac:task-status><ac:task-body>todo</ac:task-body></ac:task>" +
				"<ac:task><ac:task-status>complete</ac:task-status><ac:task-body>done</ac:task-body></ac:task>" +
				"</ac:task-list>",
			"- [ ] todo\n- [x] done\n"},
	}
	for _, tt := range tests {
		if got := exactMD(t, tt.storage); got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestStorageToMarkdownTable(t *testing.T) {
	storage := "<table><tbody><tr><th>a</th><th>b</th></tr><tr><td>1</td><td>x|y</td></tr></tbody></table>"
	want := "| a | b |\n| --- | --- |\n| 1 | x\\|y |\n"
	if got := exactMD(t, storage); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestStorageToMarkdownTableRagged(t *testing.T) {
	storage := "<table><tr><th>a</th><th>b</th></tr><tr><td>1</td></tr></table>"
	want := "| a | b |\n| --- | --- |\n| 1 |  |\n"
	if got := exactMD(t, storage); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestStorageToMarkdownBlockquoteAndHR(t *testing.T) {
	tests := []struct{ name, storage, want string }{
		{"blockquote", "<blockquote><p>q</p></blockquote>", "> q\n"},
		{"multi-paragraph quote", "<blockquote><p>x</p><p>y</p></blockquote>", "> x\n>\n> y\n"},
		{"hr", "<hr/>", "---\n"},
		{"pre", "<pre>raw\ncode</pre>", "```\nraw\ncode\n```\n"},
	}
	for _, tt := range tests {
		if got := exactMD(t, tt.storage); got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestStorageToMarkdownCodeMacro(t *testing.T) {
	tests := []struct{ name, storage, want string }{
		{"with language",
			`<ac:structured-macro ac:name="code"><ac:parameter ac:name="language">go</ac:parameter>` +
				"<ac:plain-text-body><![CDATA[fmt.Println(1)]]></ac:plain-text-body></ac:structured-macro>",
			"```go\nfmt.Println(1)\n```\n"},
		{"no language",
			`<ac:structured-macro ac:name="code">` +
				"<ac:plain-text-body><![CDATA[plain]]></ac:plain-text-body></ac:structured-macro>",
			"```\nplain\n```\n"},
		{"split CDATA terminator",
			`<ac:structured-macro ac:name="code">` +
				"<ac:plain-text-body><![CDATA[a ]]]]><![CDATA[> b]]></ac:plain-text-body></ac:structured-macro>",
			"```\na ]]> b\n```\n"},
		{"body containing fence",
			`<ac:structured-macro ac:name="code">` +
				"<ac:plain-text-body><![CDATA[x\n```\ny]]></ac:plain-text-body></ac:structured-macro>",
			"````\nx\n```\ny\n````\n"},
		{"html specials preserved",
			`<ac:structured-macro ac:name="code">` +
				"<ac:plain-text-body><![CDATA[if a < b && c > d {}]]></ac:plain-text-body></ac:structured-macro>",
			"```\nif a < b && c > d {}\n```\n"},
	}
	for _, tt := range tests {
		if got := exactMD(t, tt.storage); got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestStorageToMarkdownPanels(t *testing.T) {
	for name, title := range panelTitles {
		storage := `<ac:structured-macro ac:name="` + name + `"><ac:rich-text-body><p>watch out</p></ac:rich-text-body></ac:structured-macro>`
		r := mustST(t, storage)
		want := "> **[" + title + "]** watch out\n"
		if r.Markdown != want {
			t.Errorf("%s: got %q, want %q", name, r.Markdown, want)
		}
		if r.Fidelity != "lossy" {
			t.Errorf("%s: fidelity = %q, want lossy", name, r.Fidelity)
		}
		if len(r.UnsupportedMacros) != 0 {
			t.Errorf("%s: panels should not be reported as unsupported, got %v", name, r.UnsupportedMacros)
		}
	}
}

func TestStorageToMarkdownPanelMultiBlock(t *testing.T) {
	storage := `<ac:structured-macro ac:name="warning"><ac:rich-text-body><p>first</p><p>second</p></ac:rich-text-body></ac:structured-macro>`
	r := mustST(t, storage)
	want := "> **[Warning]** first\n>\n> second\n"
	if r.Markdown != want {
		t.Errorf("got %q, want %q", r.Markdown, want)
	}
}

func TestStorageToMarkdownUnsupportedMacro(t *testing.T) {
	storage := `<ac:structured-macro ac:name="toc"><ac:parameter ac:name="maxLevel">2</ac:parameter></ac:structured-macro>`
	r := mustST(t, storage)
	if r.Markdown != "> [unsupported macro: toc]\n" {
		t.Errorf("markdown = %q", r.Markdown)
	}
	if r.Fidelity != "lossy" {
		t.Errorf("fidelity = %q, want lossy", r.Fidelity)
	}
	if len(r.UnsupportedMacros) != 1 || r.UnsupportedMacros[0].Name != "toc" {
		t.Fatalf("stubs = %v", r.UnsupportedMacros)
	}
	if !strings.Contains(r.UnsupportedMacros[0].RawXML, "maxLevel") {
		t.Errorf("RawXML should contain macro body, got %q", r.UnsupportedMacros[0].RawXML)
	}
}

func TestStorageToMarkdownUnsupportedMacroTruncation(t *testing.T) {
	big := strings.Repeat("x", 2*rawXMLLimit)
	storage := `<ac:structured-macro ac:name="huge"><ac:parameter ac:name="p">` + big + `</ac:parameter></ac:structured-macro>`
	r := mustST(t, storage)
	if len(r.UnsupportedMacros) != 1 {
		t.Fatalf("stubs = %v", r.UnsupportedMacros)
	}
	raw := r.UnsupportedMacros[0].RawXML
	if len([]rune(raw)) > rawXMLLimit+3 {
		t.Errorf("RawXML not truncated: len=%d", len(raw))
	}
	if !strings.HasSuffix(raw, "...") {
		t.Errorf("truncated RawXML should end with ..., got suffix %q", raw[len(raw)-10:])
	}
}

func TestStorageToMarkdownUnnamedMacro(t *testing.T) {
	r := mustST(t, `<ac:structured-macro></ac:structured-macro>`)
	if r.Fidelity != "lossy" || len(r.UnsupportedMacros) != 1 {
		t.Fatalf("result = %+v", r)
	}
	if r.UnsupportedMacros[0].Name != "ac:structured-macro" {
		t.Errorf("name = %q", r.UnsupportedMacros[0].Name)
	}
}

func TestStorageToMarkdownAcImage(t *testing.T) {
	tests := []struct{ name, storage, want string }{
		{"url", `<p><ac:image ac:alt="alt"><ri:url ri:value="https://e.com/i.png"/></ac:image></p>`,
			"![alt](https://e.com/i.png)\n"},
		{"attachment", `<p><ac:image><ri:attachment ri:filename="d.png"/></ac:image></p>`,
			"![](d.png)\n"},
	}
	for _, tt := range tests {
		if got := exactMD(t, tt.storage); got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestStorageToMarkdownMalformedInput(t *testing.T) {
	inputs := []string{
		"",
		"<p>unclosed",
		"<p><strong>nested unclosed",
		"</p></div>",
		"<table><tr><td>no closing",
		"<![CDATA[dangling",
		"not html at all & < >",
		"<ac:structured-macro>",
		strings.Repeat("<div>", 100),
	}
	for _, in := range inputs {
		if _, err := StorageToMarkdown(in); err != nil {
			t.Errorf("StorageToMarkdown(%q) error: %v", in, err)
		}
	}
}

func TestStorageToMarkdownBareInlineText(t *testing.T) {
	if got := exactMD(t, "just text with <strong>bold</strong>"); got != "just text with **bold**\n" {
		t.Errorf("got %q", got)
	}
}

func TestStorageToMarkdownEmptyInput(t *testing.T) {
	r := mustST(t, "")
	if r.Markdown != "" || r.Fidelity != "exact" || len(r.UnsupportedMacros) != 0 {
		t.Errorf("empty input: %+v", r)
	}
}
