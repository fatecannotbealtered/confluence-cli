// Package render converts between Markdown and the Confluence storage
// format (an XHTML dialect with ac:/ri: extension elements).
package render

import (
	"bytes"
	"fmt"
	"html"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
)

// MarkdownToStorage converts GFM Markdown to Confluence storage format.
func MarkdownToStorage(md string) (string, error) {
	source := []byte(md)
	parser := goldmark.New(goldmark.WithExtensions(extension.GFM)).Parser()
	doc := parser.Parse(text.NewReader(source))
	w := &storageWriter{source: source}
	w.blocks(doc)
	return w.b.String(), nil
}

type storageWriter struct {
	b      strings.Builder
	source []byte
}

func (w *storageWriter) blocks(parent ast.Node) {
	for c := parent.FirstChild(); c != nil; c = c.NextSibling() {
		w.block(c)
	}
}

func (w *storageWriter) block(n ast.Node) {
	switch n := n.(type) {
	case *ast.Heading:
		fmt.Fprintf(&w.b, "<h%d>", n.Level)
		w.inlines(n)
		fmt.Fprintf(&w.b, "</h%d>\n", n.Level)
	case *ast.Paragraph:
		w.b.WriteString("<p>")
		w.inlines(n)
		w.b.WriteString("</p>\n")
	case *ast.TextBlock:
		w.inlines(n)
	case *ast.Blockquote:
		w.b.WriteString("<blockquote>")
		w.blocks(n)
		w.b.WriteString("</blockquote>\n")
	case *ast.List:
		w.list(n)
	case *ast.FencedCodeBlock:
		w.codeMacro(string(n.Language(w.source)), w.rawLines(n))
	case *ast.CodeBlock:
		w.codeMacro("", w.rawLines(n))
	case *ast.ThematicBreak:
		w.b.WriteString("<hr/>\n")
	case *ast.HTMLBlock:
		// Raw HTML in Markdown is conservatively escaped as text.
		raw := w.rawLines(n)
		if n.HasClosure() {
			raw += string(n.ClosureLine.Value(w.source))
		}
		if t := strings.TrimSpace(raw); t != "" {
			w.b.WriteString("<p>" + html.EscapeString(t) + "</p>\n")
		}
	case *extast.Table:
		w.table(n)
	default:
		w.blocks(n)
	}
}

func (w *storageWriter) list(n *ast.List) {
	if isTaskList(n) {
		w.taskList(n)
		return
	}
	tag := "ul"
	attrs := ""
	if n.IsOrdered() {
		tag = "ol"
		if n.Start > 1 {
			attrs = fmt.Sprintf(` start="%d"`, n.Start)
		}
	}
	w.b.WriteString("<" + tag + attrs + ">")
	for li := n.FirstChild(); li != nil; li = li.NextSibling() {
		w.b.WriteString("<li>")
		w.blocks(li)
		w.b.WriteString("</li>")
	}
	w.b.WriteString("</" + tag + ">\n")
}

func (w *storageWriter) taskList(n *ast.List) {
	w.b.WriteString("<ac:task-list>")
	for li := n.FirstChild(); li != nil; li = li.NextSibling() {
		status := "incomplete"
		if taskChecked(li) {
			status = "complete"
		}
		w.b.WriteString("<ac:task><ac:task-status>" + status + "</ac:task-status><ac:task-body>")
		w.blocks(li)
		w.b.WriteString("</ac:task-body></ac:task>")
	}
	w.b.WriteString("</ac:task-list>\n")
}

func isTaskList(n *ast.List) bool {
	li := n.FirstChild()
	if li == nil {
		return false
	}
	tb := li.FirstChild()
	if tb == nil {
		return false
	}
	_, ok := tb.FirstChild().(*extast.TaskCheckBox)
	return ok
}

func taskChecked(li ast.Node) bool {
	tb := li.FirstChild()
	if tb == nil {
		return false
	}
	if cb, ok := tb.FirstChild().(*extast.TaskCheckBox); ok {
		return cb.IsChecked
	}
	return false
}

func (w *storageWriter) table(n *extast.Table) {
	w.b.WriteString("<table><tbody>")
	for row := n.FirstChild(); row != nil; row = row.NextSibling() {
		cellTag := "td"
		if _, ok := row.(*extast.TableHeader); ok {
			cellTag = "th"
		}
		w.b.WriteString("<tr>")
		for cell := row.FirstChild(); cell != nil; cell = cell.NextSibling() {
			w.b.WriteString("<" + cellTag + ">")
			w.inlines(cell)
			w.b.WriteString("</" + cellTag + ">")
		}
		w.b.WriteString("</tr>")
	}
	w.b.WriteString("</tbody></table>\n")
}

func (w *storageWriter) codeMacro(lang, code string) {
	w.b.WriteString(`<ac:structured-macro ac:name="code">`)
	if lang != "" {
		w.b.WriteString(`<ac:parameter ac:name="language">` + html.EscapeString(lang) + `</ac:parameter>`)
	}
	w.b.WriteString("<ac:plain-text-body><![CDATA[" + cdataEscape(strings.TrimSuffix(code, "\n")) + "]]></ac:plain-text-body></ac:structured-macro>\n")
}

// cdataEscape splits any "]]>" occurrence across two CDATA sections.
func cdataEscape(s string) string {
	return strings.ReplaceAll(s, "]]>", "]]]]><![CDATA[>")
}

func (w *storageWriter) inlines(parent ast.Node) {
	for c := parent.FirstChild(); c != nil; c = c.NextSibling() {
		w.inline(c)
	}
}

func (w *storageWriter) inline(n ast.Node) {
	switch n := n.(type) {
	case *ast.Text:
		w.b.WriteString(html.EscapeString(string(n.Segment.Value(w.source))))
		if n.HardLineBreak() {
			w.b.WriteString("<br/>")
		} else if n.SoftLineBreak() {
			w.b.WriteString(" ")
		}
	case *ast.String:
		w.b.WriteString(html.EscapeString(string(n.Value)))
	case *ast.CodeSpan:
		w.b.WriteString("<code>" + html.EscapeString(w.plainText(n)) + "</code>")
	case *ast.Emphasis:
		tag := "em"
		if n.Level == 2 {
			tag = "strong"
		}
		w.b.WriteString("<" + tag + ">")
		w.inlines(n)
		w.b.WriteString("</" + tag + ">")
	case *extast.Strikethrough:
		w.b.WriteString("<del>")
		w.inlines(n)
		w.b.WriteString("</del>")
	case *ast.Link:
		w.b.WriteString(`<a href="` + html.EscapeString(safeHref(string(n.Destination))) + `">`)
		w.inlines(n)
		w.b.WriteString("</a>")
	case *ast.AutoLink:
		url := string(n.URL(w.source))
		href := url
		if n.AutoLinkType == ast.AutoLinkEmail && !strings.HasPrefix(href, "mailto:") {
			href = "mailto:" + href
		}
		w.b.WriteString(`<a href="` + html.EscapeString(safeHref(href)) + `">` + html.EscapeString(url) + "</a>")
	case *ast.Image:
		w.image(n)
	case *ast.RawHTML:
		// Raw inline HTML is conservatively escaped as text.
		var buf bytes.Buffer
		for i := 0; i < n.Segments.Len(); i++ {
			seg := n.Segments.At(i)
			buf.Write(seg.Value(w.source))
		}
		w.b.WriteString(html.EscapeString(buf.String()))
	case *extast.TaskCheckBox:
		// Rendered as ac:task-status at the list level.
	default:
		w.inlines(n)
	}
}

// safeHref returns a hyperlink destination safe to embed in a storage-format
// <a href>. An explicit URL scheme is allowed only when it is http, https, or
// mailto; any other scheme (javascript:, data:, vbscript:, file:, ...) is
// neutralized to "#" so a dangerous scheme can never become an active link.
// Relative paths and scheme-relative ("//host") URLs are preserved as-is.
func safeHref(raw string) string {
	s := strings.TrimSpace(raw)
	if i := strings.IndexByte(s, ':'); i >= 0 {
		scheme := s[:i]
		// A '/', '?' or '#' before the first ':' means it is not a scheme but a
		// relative path that merely contains a colon (e.g. "a/b:c").
		if !strings.ContainsAny(scheme, "/?#") {
			switch strings.ToLower(scheme) {
			case "http", "https", "mailto":
			default:
				return "#"
			}
		}
	}
	return s
}

func (w *storageWriter) image(n *ast.Image) {
	dest := string(n.Destination)
	alt := strings.TrimSpace(w.plainText(n))
	w.b.WriteString("<ac:image")
	if alt != "" {
		w.b.WriteString(` ac:alt="` + html.EscapeString(alt) + `"`)
	}
	w.b.WriteString(">")
	lower := strings.ToLower(dest)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		w.b.WriteString(`<ri:url ri:value="` + html.EscapeString(dest) + `"/>`)
	} else {
		w.b.WriteString(`<ri:attachment ri:filename="` + html.EscapeString(dest) + `"/>`)
	}
	w.b.WriteString("</ac:image>")
}

func (w *storageWriter) plainText(n ast.Node) string {
	var b strings.Builder
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		switch c := c.(type) {
		case *ast.Text:
			b.Write(c.Segment.Value(w.source))
		case *ast.String:
			b.Write(c.Value)
		default:
			b.WriteString(w.plainText(c))
		}
	}
	return b.String()
}

func (w *storageWriter) rawLines(n ast.Node) string {
	var buf bytes.Buffer
	lines := n.Lines()
	for i := 0; i < lines.Len(); i++ {
		seg := lines.At(i)
		buf.Write(seg.Value(w.source))
	}
	return buf.String()
}
