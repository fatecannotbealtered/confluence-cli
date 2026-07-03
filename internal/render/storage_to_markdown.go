package render

import (
	"bytes"
	"fmt"
	"html"
	"strconv"
	"strings"

	xhtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// Result is the outcome of a storage-to-Markdown conversion.
type Result struct {
	Markdown          string      `json:"markdown"`
	Fidelity          string      `json:"fidelity"` // "exact" or "lossy"
	UnsupportedMacros []MacroStub `json:"unsupported_macros,omitempty"`
}

// MacroStub records a macro that could not be converted.
type MacroStub struct {
	Name   string `json:"name"`
	RawXML string `json:"raw_xml"`
}

const rawXMLLimit = 500

// StorageToMarkdown converts Confluence storage format to Markdown.
// Unsupported macros become placeholder blockquotes and mark the
// result as lossy. Malformed input never panics.
func StorageToMarkdown(storage string) (Result, error) {
	ctx := &xhtml.Node{Type: xhtml.ElementNode, Data: "body", DataAtom: atom.Body}
	nodes, err := xhtml.ParseFragment(strings.NewReader(preprocessCDATA(storage)), ctx)
	if err != nil {
		return Result{}, err
	}
	c := &converter{}
	md := strings.Join(c.blocks(nodes), "\n\n")
	if md != "" {
		md += "\n"
	}
	fidelity := "exact"
	if c.lossy {
		fidelity = "lossy"
	}
	return Result{Markdown: md, Fidelity: fidelity, UnsupportedMacros: c.stubs}, nil
}

// preprocessCDATA replaces CDATA sections with entity-escaped text so
// the HTML5 parser (which treats CDATA as bogus comments) keeps the
// content intact as text nodes.
func preprocessCDATA(s string) string {
	const open, close = "<![CDATA[", "]]>"
	var b strings.Builder
	for {
		i := strings.Index(s, open)
		if i < 0 {
			b.WriteString(s)
			return b.String()
		}
		b.WriteString(s[:i])
		rest := s[i+len(open):]
		j := strings.Index(rest, close)
		if j < 0 {
			b.WriteString(html.EscapeString(rest))
			return b.String()
		}
		b.WriteString(html.EscapeString(rest[:j]))
		s = rest[j+len(close):]
	}
}

type converter struct {
	lossy bool
	stubs []MacroStub
}

var blockTags = map[string]bool{
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
	"p": true, "ul": true, "ol": true, "table": true, "blockquote": true,
	"hr": true, "pre": true, "div": true,
	"ac:structured-macro": true, "ac:macro": true, "ac:task-list": true,
	"ac:layout": true, "ac:layout-section": true, "ac:layout-cell": true,
	"ac:rich-text-body": true,
}

// blocks renders a sequence of sibling nodes into Markdown blocks.
// Runs of inline content between block elements become paragraphs.
func (c *converter) blocks(nodes []*xhtml.Node) []string {
	var out []string
	var inline strings.Builder
	flush := func() {
		if t := strings.TrimSpace(inline.String()); t != "" {
			out = append(out, t)
		}
		inline.Reset()
	}
	for _, n := range nodes {
		if n.Type == xhtml.ElementNode && blockTags[n.Data] {
			flush()
			if b := c.block(n); b != "" {
				out = append(out, b)
			}
		} else {
			c.inline(&inline, n)
		}
	}
	flush()
	return out
}

func (c *converter) block(n *xhtml.Node) string {
	switch n.Data {
	case "h1", "h2", "h3", "h4", "h5", "h6":
		level := int(n.Data[1] - '0')
		return strings.Repeat("#", level) + " " + oneLine(c.inlineString(children(n)))
	case "p":
		return c.inlineString(children(n))
	case "blockquote":
		return quotePrefix(strings.Join(c.blocks(children(n)), "\n\n"))
	case "ul", "ol":
		return c.list(n)
	case "table":
		return c.table(n)
	case "hr":
		return "---"
	case "pre":
		return fencedBlock("", strings.TrimSuffix(textContent(n), "\n"))
	case "div", "ac:layout", "ac:layout-section", "ac:layout-cell", "ac:rich-text-body":
		return strings.Join(c.blocks(children(n)), "\n\n")
	case "ac:task-list":
		return c.taskList(n)
	case "ac:structured-macro", "ac:macro":
		return c.macro(n)
	}
	return ""
}

func (c *converter) list(n *xhtml.Node) string {
	ordered := n.Data == "ol"
	num := 1
	if s := attrVal(n, "start"); s != "" {
		if v, err := strconv.Atoi(s); err == nil {
			num = v
		}
	}
	var lines []string
	for _, li := range children(n) {
		if li.Type != xhtml.ElementNode || li.Data != "li" {
			continue
		}
		marker := "- "
		if ordered {
			marker = fmt.Sprintf("%d. ", num)
			num++
		}
		// Nested lists join to their parent item with a single newline.
		body := strings.Join(c.blocks(children(li)), "\n")
		itemLines := strings.Split(body, "\n")
		lines = append(lines, marker+itemLines[0])
		pad := strings.Repeat(" ", len(marker))
		for _, l := range itemLines[1:] {
			if l == "" {
				lines = append(lines, "")
			} else {
				lines = append(lines, pad+l)
			}
		}
	}
	return strings.Join(lines, "\n")
}

func (c *converter) taskList(n *xhtml.Node) string {
	var lines []string
	for _, task := range children(n) {
		if task.Type != xhtml.ElementNode || task.Data != "ac:task" {
			continue
		}
		mark := "[ ]"
		var body string
		for _, ch := range children(task) {
			switch ch.Data {
			case "ac:task-status":
				if strings.TrimSpace(textContent(ch)) == "complete" {
					mark = "[x]"
				}
			case "ac:task-body":
				body = oneLine(c.inlineString(children(ch)))
			}
		}
		lines = append(lines, "- "+mark+" "+body)
	}
	return strings.Join(lines, "\n")
}

func (c *converter) table(n *xhtml.Node) string {
	var rows [][]string
	var collectRows func(*xhtml.Node)
	collectRows = func(n *xhtml.Node) {
		for _, ch := range children(n) {
			if ch.Type != xhtml.ElementNode {
				continue
			}
			switch ch.Data {
			case "thead", "tbody", "tfoot":
				collectRows(ch)
			case "tr":
				var cells []string
				for _, cell := range children(ch) {
					if cell.Type == xhtml.ElementNode && (cell.Data == "th" || cell.Data == "td") {
						text := oneLine(c.inlineString(children(cell)))
						cells = append(cells, strings.ReplaceAll(text, "|", "\\|"))
					}
				}
				rows = append(rows, cells)
			}
		}
	}
	collectRows(n)
	if len(rows) == 0 {
		return ""
	}
	width := 0
	for _, r := range rows {
		if len(r) > width {
			width = len(r)
		}
	}
	var b strings.Builder
	writeRow := func(cells []string) {
		b.WriteString("|")
		for i := 0; i < width; i++ {
			cell := ""
			if i < len(cells) {
				cell = cells[i]
			}
			b.WriteString(" " + cell + " |")
		}
		b.WriteString("\n")
	}
	writeRow(rows[0])
	b.WriteString("|" + strings.Repeat(" --- |", width) + "\n")
	for _, r := range rows[1:] {
		writeRow(r)
	}
	return strings.TrimSuffix(b.String(), "\n")
}

var panelTitles = map[string]string{
	"info": "Info", "note": "Note", "warning": "Warning", "tip": "Tip",
}

func (c *converter) macro(n *xhtml.Node) string {
	name := attrVal(n, "ac:name")
	switch {
	case name == "code":
		return c.codeMacro(n)
	case panelTitles[name] != "":
		c.lossy = true
		prefix := "**[" + panelTitles[name] + "]**"
		var inner []string
		for _, ch := range children(n) {
			if ch.Type == xhtml.ElementNode && ch.Data == "ac:rich-text-body" {
				inner = c.blocks(children(ch))
			}
		}
		if len(inner) == 0 {
			inner = []string{prefix}
		} else {
			inner[0] = prefix + " " + inner[0]
		}
		return quotePrefix(strings.Join(inner, "\n\n"))
	default:
		return c.unsupported(name, n)
	}
}

func (c *converter) codeMacro(n *xhtml.Node) string {
	var lang, body string
	for _, ch := range children(n) {
		if ch.Type != xhtml.ElementNode {
			continue
		}
		switch ch.Data {
		case "ac:parameter":
			if attrVal(ch, "ac:name") == "language" {
				lang = oneLine(strings.TrimSpace(textContent(ch)))
			}
		case "ac:plain-text-body":
			body = textContent(ch)
		}
	}
	return fencedBlock(lang, body)
}

func (c *converter) unsupported(name string, n *xhtml.Node) string {
	c.lossy = true
	if name == "" {
		name = n.Data
	}
	var buf bytes.Buffer
	if err := xhtml.Render(&buf, n); err == nil {
		raw := buf.String()
		if r := []rune(raw); len(r) > rawXMLLimit {
			raw = string(r[:rawXMLLimit]) + "..."
		}
		c.stubs = append(c.stubs, MacroStub{Name: name, RawXML: raw})
	} else {
		c.stubs = append(c.stubs, MacroStub{Name: name})
	}
	return "> [unsupported macro: " + name + "]"
}

func (c *converter) inline(b *strings.Builder, n *xhtml.Node) {
	switch n.Type {
	case xhtml.TextNode:
		b.WriteString(escapeMarkdown(collapseSpace(n.Data)))
		return
	case xhtml.ElementNode:
	default:
		return
	}
	switch n.Data {
	case "strong", "b":
		c.wrap(b, n, "**")
	case "em", "i":
		c.wrap(b, n, "*")
	case "del", "s", "strike":
		c.wrap(b, n, "~~")
	case "code":
		b.WriteString(codeSpan(textContent(n)))
	case "a":
		text := c.inlineString(children(n))
		if text == "" {
			text = attrVal(n, "href")
		}
		b.WriteString("[" + text + "](" + attrVal(n, "href") + ")")
	case "br":
		b.WriteString("  \n")
	case "img":
		b.WriteString("![" + escapeMarkdown(attrVal(n, "alt")) + "](" + attrVal(n, "src") + ")")
	case "ac:image":
		b.WriteString(c.acImage(n))
	case "ac:structured-macro", "ac:macro":
		// A macro in inline position: emit an inline placeholder.
		b.WriteString(strings.TrimPrefix(c.macro(n), "> "))
	default:
		for _, ch := range children(n) {
			c.inline(b, ch)
		}
	}
}

func (c *converter) wrap(b *strings.Builder, n *xhtml.Node, delim string) {
	inner := c.inlineString(children(n))
	if inner == "" {
		return
	}
	b.WriteString(delim + inner + delim)
}

func (c *converter) acImage(n *xhtml.Node) string {
	alt := escapeMarkdown(attrVal(n, "ac:alt"))
	for _, ch := range children(n) {
		if ch.Type != xhtml.ElementNode {
			continue
		}
		switch ch.Data {
		case "ri:url":
			return "![" + alt + "](" + attrVal(ch, "ri:value") + ")"
		case "ri:attachment":
			return "![" + alt + "](" + attrVal(ch, "ri:filename") + ")"
		}
	}
	return ""
}

func (c *converter) inlineString(nodes []*xhtml.Node) string {
	var b strings.Builder
	for _, n := range nodes {
		c.inline(&b, n)
	}
	return strings.TrimSpace(b.String())
}

func children(n *xhtml.Node) []*xhtml.Node {
	var out []*xhtml.Node
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		out = append(out, c)
	}
	return out
}

func attrVal(n *xhtml.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func textContent(n *xhtml.Node) string {
	var b strings.Builder
	var walk func(*xhtml.Node)
	walk = func(n *xhtml.Node) {
		if n.Type == xhtml.TextNode {
			b.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return b.String()
}

var mdEscaper = strings.NewReplacer(
	`\`, `\\`, "`", "\\`", `*`, `\*`, `_`, `\_`, `[`, `\[`, `]`, `\]`, `~`, `\~`,
)

func escapeMarkdown(s string) string {
	return mdEscaper.Replace(s)
}

// collapseSpace collapses whitespace runs to single spaces, keeping a
// single leading/trailing space when present.
func collapseSpace(s string) string {
	joined := strings.Join(strings.Fields(s), " ")
	if joined == "" {
		if s != "" {
			return " "
		}
		return ""
	}
	if s[0] == ' ' || s[0] == '\n' || s[0] == '\t' || s[0] == '\r' {
		joined = " " + joined
	}
	last := s[len(s)-1]
	if last == ' ' || last == '\n' || last == '\t' || last == '\r' {
		joined += " "
	}
	return joined
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func quotePrefix(s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if l == "" {
			lines[i] = ">"
		} else {
			lines[i] = "> " + l
		}
	}
	return strings.Join(lines, "\n")
}

func codeSpan(s string) string {
	if !strings.Contains(s, "`") {
		return "`" + s + "`"
	}
	delim := "`"
	for strings.Contains(s, delim) {
		delim += "`"
	}
	return delim + " " + s + " " + delim
}

func fencedBlock(lang, body string) string {
	fence := "```"
	for strings.Contains(body, fence) {
		fence += "`"
	}
	var b strings.Builder
	b.WriteString(fence + lang + "\n")
	if body != "" {
		b.WriteString(strings.TrimSuffix(body, "\n") + "\n")
	}
	b.WriteString(fence)
	return b.String()
}
