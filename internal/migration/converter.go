package migration

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

type CanonicalNode struct {
	Type     string          `json:"type"`
	Text     string          `json:"text,omitempty"`
	Attrs    map[string]any  `json:"attrs,omitempty"`
	Children []CanonicalNode `json:"children,omitempty"`
}

type MacroUse struct {
	Name        string `json:"name"`
	Supported   bool   `json:"supported"`
	Occurrences int    `json:"occurrences"`
}

type LinkUse struct {
	Type   string `json:"type"`
	Target string `json:"target"`
}

type ConversionResult struct {
	Canonical json.RawMessage `json:"canonical"`
	Editor    json.RawMessage `json:"editor"`
	Text      string          `json:"text"`
	Macros    []MacroUse      `json:"macros"`
	Links     []LinkUse       `json:"links"`
	Warnings  []string        `json:"warnings"`
}

type parseNode struct {
	Type     string
	Text     string
	Attrs    map[string]any
	Children []*parseNode
}

var tagStripper = regexp.MustCompile(`<[^>]+>`)

var supportedMacros = map[string]bool{
	"attachments": true, "children": true, "code": true, "column": true, "content-by-label": true,
	"date": true, "excerpt": true, "excerpt-include": true, "expand": true, "gallery": true,
	"image": true, "include": true, "info": true, "note": true, "page-properties": true,
	"page-properties-report": true, "panel": true, "recently-updated": true, "section": true,
	"status": true, "task-list": true, "tip": true, "toc": true, "user-profile": true, "warning": true,
}

func ConvertStorage(source string) ConversionResult {
	root := &parseNode{Type: "document"}
	result := ConversionResult{}
	wrapped := `<kanvas-root xmlns:ac="urn:kanvas:confluence:ac" xmlns:ri="urn:kanvas:confluence:ri">` + source + `</kanvas-root>`
	decoder := xml.NewDecoder(strings.NewReader(wrapped))
	stack := []*parseNode{root}
	for {
		tok, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			plain := normalizeText(html.UnescapeString(tagStripper.ReplaceAllString(source, " ")))
			result.Warnings = append(result.Warnings, "INVALID_XHTML: "+err.Error())
			root = &parseNode{Type: "document", Children: []*parseNode{{Type: "paragraph", Children: []*parseNode{{Type: "text", Text: plain}}}}}
			break
		}
		switch v := tok.(type) {
		case xml.StartElement:
			if v.Name.Local == "kanvas-root" {
				continue
			}
			n := startNode(v, &result)
			parent := stack[len(stack)-1]
			parent.Children = append(parent.Children, n)
			stack = append(stack, n)
		case xml.EndElement:
			if v.Name.Local != "kanvas-root" && len(stack) > 1 {
				stack = stack[:len(stack)-1]
			}
		case xml.CharData:
			if len(v) == 0 {
				continue
			}
			text := string(v)
			if strings.TrimSpace(text) == "" {
				if strings.ContainsAny(text, "\n\r\t") {
					continue
				}
			}
			parent := stack[len(stack)-1]
			parent.Children = append(parent.Children, &parseNode{Type: "text", Text: text})
		}
	}
	canonical := toCanonical(root)
	result.Canonical = mustJSON(canonical)
	editor := map[string]any{"type": "doc", "content": editorChildren(root.Children, nil)}
	result.Editor = mustJSON(editor)
	result.Text = normalizeText(extractText(root))
	result.Macros = dedupeMacros(result.Macros)
	result.Links = dedupeLinks(result.Links)
	return result
}

func startNode(e xml.StartElement, result *ConversionResult) *parseNode {
	local := strings.ToLower(e.Name.Local)
	attrs := attrsByLocal(e.Attr)
	switch local {
	case "p", "div":
		return &parseNode{Type: "paragraph"}
	case "h1", "h2", "h3", "h4", "h5", "h6":
		return &parseNode{Type: "heading", Attrs: map[string]any{"level": int(local[1] - '0')}}
	case "ul":
		return &parseNode{Type: "bulletList"}
	case "ol":
		return &parseNode{Type: "orderedList"}
	case "li":
		return &parseNode{Type: "listItem"}
	case "strong", "b":
		return &parseNode{Type: "strong"}
	case "em", "i":
		return &parseNode{Type: "emphasis"}
	case "u":
		return &parseNode{Type: "underline"}
	case "s", "strike", "del":
		return &parseNode{Type: "strike"}
	case "code":
		return &parseNode{Type: "inlineCode"}
	case "pre":
		return &parseNode{Type: "codeBlock"}
	case "blockquote":
		return &parseNode{Type: "blockquote"}
	case "br":
		return &parseNode{Type: "hardBreak"}
	case "hr":
		return &parseNode{Type: "horizontalRule"}
	case "table":
		return &parseNode{Type: "table"}
	case "tbody", "thead":
		return &parseNode{Type: "container"}
	case "tr":
		return &parseNode{Type: "tableRow"}
	case "th":
		return &parseNode{Type: "tableHeader"}
	case "td":
		return &parseNode{Type: "tableCell"}
	case "a":
		target := attrs["href"]
		result.Links = append(result.Links, LinkUse{Type: "URL", Target: target})
		return &parseNode{Type: "link", Attrs: map[string]any{"href": target}}
	case "structured-macro", "macro":
		name := strings.ToLower(firstNonEmpty(attrs["name"], attrs["macro-name"]))
		if name == "" {
			name = "unknown"
		}
		result.Macros = append(result.Macros, MacroUse{Name: name, Supported: supportedMacros[name], Occurrences: 1})
		return &parseNode{Type: "macro", Attrs: map[string]any{"name": name, "supported": supportedMacros[name]}}
	case "parameter":
		return &parseNode{Type: "macroParameter", Attrs: map[string]any{"name": attrs["name"]}}
	case "plain-text-body", "rich-text-body":
		return &parseNode{Type: "macroBody"}
	case "page":
		target := firstNonEmpty(attrs["content-title"], attrs["space-key"])
		result.Links = append(result.Links, LinkUse{Type: "PAGE", Target: target})
		return &parseNode{Type: "pageReference", Attrs: map[string]any{"target": target}}
	case "user":
		target := firstNonEmpty(attrs["userkey"], attrs["username"])
		return &parseNode{Type: "userReference", Attrs: map[string]any{"target": target}}
	case "emoticon":
		return &parseNode{Type: "text", Text: ":" + firstNonEmpty(attrs["name"], "emoticon") + ":"}
	default:
		return &parseNode{Type: "container", Attrs: map[string]any{"sourceTag": local}}
	}
}

func attrsByLocal(attrs []xml.Attr) map[string]string {
	out := map[string]string{}
	for _, a := range attrs {
		out[strings.ToLower(a.Name.Local)] = a.Value
	}
	return out
}
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
func toCanonical(n *parseNode) CanonicalNode {
	v := CanonicalNode{Type: n.Type, Text: n.Text, Attrs: n.Attrs}
	for _, c := range n.Children {
		v.Children = append(v.Children, toCanonical(c))
	}
	return v
}
func mustJSON(v any) json.RawMessage { b, _ := json.Marshal(v); return b }

func editorChildren(nodes []*parseNode, marks []map[string]any) []any {
	var out []any
	for _, n := range nodes {
		switch n.Type {
		case "text":
			if n.Text != "" {
				v := map[string]any{"type": "text", "text": n.Text}
				if len(marks) > 0 {
					v["marks"] = marks
				}
				out = append(out, v)
			}
		case "strong", "emphasis", "underline", "strike", "inlineCode", "link":
			markType := map[string]string{"strong": "bold", "emphasis": "italic", "underline": "underline", "strike": "strike", "inlineCode": "code", "link": "link"}[n.Type]
			mark := map[string]any{"type": markType}
			if n.Type == "link" {
				mark["attrs"] = n.Attrs
			}
			out = append(out, editorChildren(n.Children, append(append([]map[string]any{}, marks...), mark))...)
		case "paragraph", "heading", "blockquote", "codeBlock", "bulletList", "orderedList", "listItem", "table", "tableRow", "tableHeader", "tableCell":
			typeName := n.Type
			v := map[string]any{"type": typeName}
			if n.Type == "heading" {
				v["attrs"] = n.Attrs
			}
			children := editorChildren(n.Children, marks)
			if len(children) > 0 {
				v["content"] = children
			}
			out = append(out, v)
		case "hardBreak", "horizontalRule":
			out = append(out, map[string]any{"type": n.Type})
		case "macro":
			out = append(out, editorMacro(n))
		case "pageReference":
			target, _ := n.Attrs["target"].(string)
			out = append(out, map[string]any{"type": "text", "text": firstNonEmpty(target, "[Page]")})
		case "userReference":
			target, _ := n.Attrs["target"].(string)
			out = append(out, map[string]any{"type": "text", "text": "@" + target})
		default:
			out = append(out, editorChildren(n.Children, marks)...)
		}
	}
	return out
}

func editorMacro(n *parseNode) any {
	name, _ := n.Attrs["name"].(string)
	body := normalizeText(extractText(n))
	label := "[" + strings.ToUpper(name) + "]"
	if body != "" {
		label += " " + body
	}
	if name == "code" {
		return map[string]any{"type": "codeBlock", "attrs": map[string]any{"language": macroParameter(n, "language")}, "content": []any{map[string]any{"type": "text", "text": body}}}
	}
	if name == "info" || name == "note" || name == "warning" || name == "tip" || name == "panel" || name == "expand" {
		return map[string]any{"type": "blockquote", "content": []any{map[string]any{"type": "paragraph", "content": []any{map[string]any{"type": "text", "text": label}}}}}
	}
	return map[string]any{"type": "paragraph", "content": []any{map[string]any{"type": "text", "text": label}}}
}

func macroParameter(n *parseNode, name string) string {
	for _, c := range n.Children {
		if c.Type == "macroParameter" {
			if v, _ := c.Attrs["name"].(string); v == name {
				return normalizeText(extractText(c))
			}
		}
	}
	return ""
}
func extractText(n *parseNode) string {
	if n.Type == "text" {
		return n.Text
	}
	var b strings.Builder
	for _, c := range n.Children {
		t := extractText(c)
		if t == "" {
			continue
		}
		if b.Len() > 0 && isBlock(c.Type) {
			b.WriteByte('\n')
		}
		b.WriteString(t)
	}
	return b.String()
}
func isBlock(t string) bool {
	return t == "paragraph" || t == "heading" || t == "listItem" || t == "codeBlock" || t == "blockquote" || t == "tableRow" || t == "macro"
}
func normalizeText(v string) string {
	var b bytes.Buffer
	space := false
	for _, r := range strings.TrimSpace(v) {
		if unicode.IsSpace(r) {
			if !space {
				b.WriteByte(' ')
				space = true
			}
			continue
		}
		space = false
		b.WriteRune(r)
	}
	return b.String()
}
func dedupeMacros(v []MacroUse) []MacroUse {
	m := map[string]MacroUse{}
	for _, x := range v {
		current := m[x.Name]
		current.Name = x.Name
		current.Supported = x.Supported
		current.Occurrences += max(x.Occurrences, 1)
		m[x.Name] = current
	}
	out := make([]MacroUse, 0, len(m))
	for _, x := range m {
		out = append(out, x)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
func dedupeLinks(v []LinkUse) []LinkUse {
	m := map[string]LinkUse{}
	for _, x := range v {
		if x.Target != "" {
			m[fmt.Sprintf("%s:%s", x.Type, x.Target)] = x
		}
	}
	out := make([]LinkUse, 0, len(m))
	for _, x := range m {
		out = append(out, x)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Type+out[i].Target < out[j].Type+out[j].Target })
	return out
}
