package render

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
)

func mustMD(t *testing.T, md string) string {
	t.Helper()
	s, err := MarkdownToStorage(md)
	if err != nil {
		t.Fatalf("MarkdownToStorage(%q) error: %v", md, err)
	}
	return s
}

func TestMarkdownToStorageHeadings(t *testing.T) {
	for level := 1; level <= 6; level++ {
		md := strings.Repeat("#", level) + " Title"
		want := fmt.Sprintf("<h%d>Title</h%d>\n", level, level)
		if got := mustMD(t, md); got != want {
			t.Errorf("level %d: got %q, want %q", level, got, want)
		}
	}
}

func TestMarkdownToStorageParagraphAndEmphasis(t *testing.T) {
	tests := []struct{ md, want string }{
		{"hello world", "<p>hello world</p>\n"},
		{"**bold**", "<p><strong>bold</strong></p>\n"},
		{"*italic*", "<p><em>italic</em></p>\n"},
		{"~~gone~~", "<p><del>gone</del></p>\n"},
		{"**bold *nested*** text", "<p><strong>bold <em>nested</em></strong> text</p>\n"},
	}
	for _, tt := range tests {
		if got := mustMD(t, tt.md); got != tt.want {
			t.Errorf("%q: got %q, want %q", tt.md, got, tt.want)
		}
	}
}

func TestMarkdownToStorageLists(t *testing.T) {
	tests := []struct{ name, md, want string }{
		{"unordered", "- a\n- b", "<ul><li>a</li><li>b</li></ul>\n"},
		{"ordered", "1. a\n2. b", "<ol><li>a</li><li>b</li></ol>\n"},
		{"ordered start", "3. a\n4. b", `<ol start="3"><li>a</li><li>b</li></ol>` + "\n"},
		{"nested", "- one\n  - nested\n- two", "<ul><li>one<ul><li>nested</li></ul>\n</li><li>two</li></ul>\n"},
		{"task list", "- [ ] todo\n- [x] done",
			"<ac:task-list>" +
				"<ac:task><ac:task-status>incomplete</ac:task-status><ac:task-body>todo</ac:task-body></ac:task>" +
				"<ac:task><ac:task-status>complete</ac:task-status><ac:task-body>done</ac:task-body></ac:task>" +
				"</ac:task-list>\n"},
	}
	for _, tt := range tests {
		if got := mustMD(t, tt.md); got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestMarkdownToStorageTable(t *testing.T) {
	md := "| a | b |\n|---|---|\n| 1 | *2* |"
	want := "<table><tbody><tr><th>a</th><th>b</th></tr><tr><td>1</td><td><em>2</em></td></tr></tbody></table>\n"
	if got := mustMD(t, md); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestMarkdownToStorageCode(t *testing.T) {
	tests := []struct{ name, md, want string }{
		{"inline", "`x < y`", "<p><code>x &lt; y</code></p>\n"},
		{"fenced with lang", "```go\nfmt.Println(1)\n```",
			`<ac:structured-macro ac:name="code"><ac:parameter ac:name="language">go</ac:parameter>` +
				"<ac:plain-text-body><![CDATA[fmt.Println(1)]]></ac:plain-text-body></ac:structured-macro>\n"},
		{"fenced no lang", "```\nplain\n```",
			`<ac:structured-macro ac:name="code">` +
				"<ac:plain-text-body><![CDATA[plain]]></ac:plain-text-body></ac:structured-macro>\n"},
		{"indented code block", "    indented\n",
			`<ac:structured-macro ac:name="code">` +
				"<ac:plain-text-body><![CDATA[indented]]></ac:plain-text-body></ac:structured-macro>\n"},
		{"cdata terminator split", "```\na ]]> b\n```",
			`<ac:structured-macro ac:name="code">` +
				"<ac:plain-text-body><![CDATA[a ]]]]><![CDATA[> b]]></ac:plain-text-body></ac:structured-macro>\n"},
	}
	for _, tt := range tests {
		if got := mustMD(t, tt.md); got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestMarkdownToStorageLinksAndImages(t *testing.T) {
	tests := []struct{ name, md, want string }{
		{"link", "[text](https://example.com)", `<p><a href="https://example.com">text</a></p>` + "\n"},
		{"autolink", "visit https://example.com now",
			`<p>visit <a href="https://example.com">https://example.com</a> now</p>` + "\n"},
		{"image url", "![alt](https://example.com/i.png)",
			`<p><ac:image ac:alt="alt"><ri:url ri:value="https://example.com/i.png"/></ac:image></p>` + "\n"},
		{"image attachment", "![](diagram.png)",
			`<p><ac:image><ri:attachment ri:filename="diagram.png"/></ac:image></p>` + "\n"},
	}
	for _, tt := range tests {
		if got := mustMD(t, tt.md); got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestMarkdownToStorageBlockquoteAndHR(t *testing.T) {
	tests := []struct{ name, md, want string }{
		{"blockquote", "> quoted", "<blockquote><p>quoted</p>\n</blockquote>\n"},
		{"hr", "---", "<hr/>\n"},
	}
	for _, tt := range tests {
		if got := mustMD(t, tt.md); got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestMarkdownToStorageEscaping(t *testing.T) {
	tests := []struct{ name, md, want string }{
		{"special chars", "a < b & c > d", "<p>a &lt; b &amp; c &gt; d</p>\n"},
		{"html block escaped", "<div>hi</div>", "<p>&lt;div&gt;hi&lt;/div&gt;</p>\n"},
		{"inline html escaped", "a <span>b</span> c", "<p>a &lt;span&gt;b&lt;/span&gt; c</p>\n"},
		{"link href escaped", `[x](https://e.com/?a=1&b=2)`, `<p><a href="https://e.com/?a=1&amp;b=2">x</a></p>` + "\n"},
	}
	for _, tt := range tests {
		if got := mustMD(t, tt.md); got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestMarkdownToStorageLineBreaks(t *testing.T) {
	if got := mustMD(t, "a  \nb"); got != "<p>a<br/>b</p>\n" {
		t.Errorf("hard break: got %q", got)
	}
	if got := mustMD(t, "a\nb"); got != "<p>a b</p>\n" {
		t.Errorf("soft break: got %q", got)
	}
}

func TestMarkdownToStorageNoPanicOnWeirdInput(t *testing.T) {
	inputs := []string{
		"",
		"[",
		"```\nunclosed fence",
		strings.Repeat("> ", 50) + "deep",
		"| broken | table\n|---|",
		"\x00\x01\x02",
	}
	for _, in := range inputs {
		if _, err := MarkdownToStorage(in); err != nil {
			t.Errorf("MarkdownToStorage(%q) error: %v", in, err)
		}
	}
}

var hrefRe = regexp.MustCompile(`href="([^"]*)"`)

func TestMarkdownToStorage_NeutralizesDangerousSchemes(t *testing.T) {
	cases := []struct {
		name string
		md   string
	}{
		{"link-javascript", "[click](javascript:alert(document.cookie))"},
		{"link-JavaScript-mixedcase", "[x](JavaScript:alert(1))"},
		{"link-data", "[x](data:text/html,<script>alert(1)</script>)"},
		{"link-vbscript", "[x](vbscript:msgbox(1))"},
		{"autolink-javascript", "<javascript:alert(1)>"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := MarkdownToStorage(tc.md)
			if err != nil {
				t.Fatal(err)
			}
			// Only the href attribute matters: a dangerous scheme surviving as
			// visible link text is inert, but inside href="..." it is active.
			for _, m := range hrefRe.FindAllStringSubmatch(out, -1) {
				h := strings.ToLower(m[1])
				if strings.HasPrefix(h, "javascript:") || strings.HasPrefix(h, "data:") || strings.HasPrefix(h, "vbscript:") {
					t.Fatalf("dangerous scheme survived into href: %s", out)
				}
			}
		})
	}
	// Legitimate schemes must be preserved.
	for _, ok := range []string{"[a](https://x.com/p)", "[a](http://x.com)", "[a](mailto:x@y.com)", "[a](/relative/path)"} {
		out, _ := MarkdownToStorage(ok)
		if strings.Contains(out, `href="#"`) {
			t.Fatalf("legitimate href was neutralized: %s -> %s", ok, out)
		}
	}
}
