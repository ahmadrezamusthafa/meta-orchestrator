package connectors

import (
	"fmt"
	"regexp"
	"strings"
)

// parseJiraDescription turns a JIRA description into Markdown. JIRA Cloud (REST v3) sends Atlassian
// Document Format; JIRA Server / Data Center (REST v2) sends wiki markup as a string.
func parseJiraDescription(raw interface{}) string {
	switch v := raw.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(wikiToMarkdown(v))
	case map[string]interface{}:
		return strings.TrimSpace(collapseBlankLines(adfBlocks(children(v), "")))
	}
	return ""
}

var multiBlank = regexp.MustCompile(`\n{3,}`)

func collapseBlankLines(s string) string { return multiBlank.ReplaceAllString(s, "\n\n") }

func children(n map[string]interface{}) []map[string]interface{} {
	raw, _ := n["content"].([]interface{})
	out := make([]map[string]interface{}, 0, len(raw))
	for _, c := range raw {
		if m, ok := c.(map[string]interface{}); ok {
			out = append(out, m)
		}
	}
	return out
}

func attr(n map[string]interface{}, key string) interface{} {
	attrs, _ := n["attrs"].(map[string]interface{})
	return attrs[key]
}

func attrString(n map[string]interface{}, key string) string {
	s, _ := attr(n, key).(string)
	return s
}

// adfBlocks renders block nodes, each separated by a blank line. indent prefixes every line (for
// content nested inside list items).
func adfBlocks(nodes []map[string]interface{}, indent string) string {
	var parts []string
	for _, n := range nodes {
		if b := adfBlock(n, indent); strings.TrimSpace(b) != "" {
			parts = append(parts, b)
		}
	}
	return strings.Join(parts, "\n\n")
}

func adfBlock(n map[string]interface{}, indent string) string {
	typ, _ := n["type"].(string)
	switch typ {
	case "paragraph":
		return indentLines(adfInline(children(n)), indent)
	case "heading":
		level := 2
		if f, ok := attr(n, "level").(float64); ok && f >= 1 && f <= 6 {
			level = int(f)
		}
		return indent + strings.Repeat("#", level) + " " + adfInline(children(n))
	case "bulletList":
		return adfList(n, indent, func(int) string { return "- " })
	case "orderedList":
		start := 1
		if f, ok := attr(n, "order").(float64); ok && f > 0 {
			start = int(f)
		}
		return adfList(n, indent, func(i int) string { return fmt.Sprintf("%d. ", start+i) })
	case "taskList":
		return adfList(n, indent, nil)
	case "codeBlock":
		var code strings.Builder
		for _, c := range children(n) {
			t, _ := c["text"].(string)
			code.WriteString(t)
		}
		return indentLines("```"+attrString(n, "language")+"\n"+code.String()+"\n```", indent)
	case "blockquote":
		return prefixLines(adfBlocks(children(n), ""), indent+"> ")
	case "panel":
		kind := attrString(n, "panelType")
		label := map[string]string{"info": "Info", "note": "Note", "warning": "Warning", "error": "Error", "success": "Success"}[kind]
		body := adfBlocks(children(n), "")
		if label != "" {
			body = "**" + label + ":** " + body
		}
		return prefixLines(body, indent+"> ")
	case "rule":
		return indent + "---"
	case "table":
		return indentLines(adfTable(n), indent)
	case "expand", "nestedExpand":
		title := attrString(n, "title")
		body := adfBlocks(children(n), indent)
		if title != "" {
			return indent + "**" + title + "**\n\n" + body
		}
		return body
	case "mediaSingle", "mediaGroup", "media":
		return indent + "_[attachment]_"
	default:
		if len(children(n)) > 0 {
			return adfBlocks(children(n), indent)
		}
		return indent + adfInline([]map[string]interface{}{n})
	}
}

// adfList renders list items; marker returns the bullet for item i (nil for task lists).
func adfList(n map[string]interface{}, indent string, marker func(int) string) string {
	var lines []string
	for i, item := range children(n) {
		m := "- "
		if marker != nil {
			m = marker(i)
		} else if item["type"] == "taskItem" {
			if attrString(item, "state") == "DONE" {
				m = "- [x] "
			} else {
				m = "- [ ] "
			}
		}
		pad := strings.Repeat(" ", len(m))
		var first, rest []string
		if item["type"] == "taskItem" { // task items hold inline content directly
			first = []string{adfInline(children(item))}
		} else {
			for j, c := range children(item) {
				block := adfBlock(c, "")
				if j == 0 && (c["type"] == "paragraph" || c["type"] == nil) {
					first = strings.Split(block, "\n")
					continue
				}
				rest = append(rest, prefixLines(block, pad))
			}
		}
		if len(first) == 0 {
			first = []string{""}
		}
		out := indent + m + first[0]
		for _, l := range first[1:] {
			out += "\n" + indent + pad + l
		}
		for _, r := range rest {
			out += "\n" + prefixLines(r, indent)
		}
		lines = append(lines, out)
	}
	return strings.Join(lines, "\n")
}

func adfTable(n map[string]interface{}) string {
	var rows [][]string
	headerRow := false
	for i, row := range children(n) {
		var cells []string
		for _, cell := range children(row) {
			if i == 0 && cell["type"] == "tableHeader" {
				headerRow = true
			}
			text := strings.ReplaceAll(adfBlocks(children(cell), ""), "\n\n", "<br>")
			text = strings.ReplaceAll(strings.ReplaceAll(text, "\n", " "), "|", `\|`)
			cells = append(cells, text)
		}
		rows = append(rows, cells)
	}
	if len(rows) == 0 {
		return ""
	}
	width := 0
	for _, r := range rows {
		if len(r) > width {
			width = len(r)
		}
	}
	if !headerRow { // Markdown tables need a header; use an empty one
		rows = append([][]string{make([]string, width)}, rows...)
	}
	var b strings.Builder
	for i, r := range rows {
		for len(r) < width {
			r = append(r, "")
		}
		b.WriteString("| " + strings.Join(r, " | ") + " |\n")
		if i == 0 {
			b.WriteString("|" + strings.Repeat(" --- |", width) + "\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func adfInline(nodes []map[string]interface{}) string {
	var b strings.Builder
	for _, n := range nodes {
		switch n["type"] {
		case "text":
			b.WriteString(applyMarks(n))
		case "hardBreak":
			b.WriteString("  \n")
		case "mention":
			b.WriteString("@" + strings.TrimPrefix(attrString(n, "text"), "@"))
		case "emoji":
			if t := attrString(n, "text"); t != "" {
				b.WriteString(t)
			} else {
				b.WriteString(attrString(n, "shortName"))
			}
		case "inlineCard", "blockCard":
			if u := attrString(n, "url"); u != "" {
				b.WriteString("<" + u + ">")
			}
		case "date":
			b.WriteString(attrString(n, "timestamp"))
		case "status":
			b.WriteString("`" + attrString(n, "text") + "`")
		default:
			b.WriteString(adfInline(children(n)))
		}
	}
	return b.String()
}

func applyMarks(n map[string]interface{}) string {
	text, _ := n["text"].(string)
	marks, _ := n["marks"].([]interface{})
	link := ""
	for _, raw := range marks {
		m, _ := raw.(map[string]interface{})
		switch m["type"] {
		case "code":
			text = "`" + text + "`"
		case "strong":
			text = "**" + text + "**"
		case "em":
			text = "_" + text + "_"
		case "strike":
			text = "~~" + text + "~~"
		case "link":
			link = attrString(m, "href")
		}
	}
	if link != "" {
		return "[" + text + "](" + link + ")"
	}
	return text
}

func indentLines(s, indent string) string {
	if indent == "" {
		return s
	}
	return prefixLines(s, indent)
}

func prefixLines(s, prefix string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if l == "" && strings.TrimSpace(prefix) == "" {
			continue
		}
		lines[i] = prefix + l
	}
	return strings.Join(lines, "\n")
}

var (
	wikiHeading   = regexp.MustCompile(`(?m)^h([1-6])\.\s+`)
	wikiBullet    = regexp.MustCompile(`(?m)^(\*+|-)\s+`)
	wikiNumbered  = regexp.MustCompile(`(?m)^(#+)\s+`)
	wikiCode      = regexp.MustCompile(`(?s)\{(?:code|noformat)(?::([a-zA-Z0-9+#-]+))?[^}]*\}(.*?)\{(?:code|noformat)\}`)
	wikiQuote     = regexp.MustCompile(`(?s)\{quote\}(.*?)\{quote\}`)
	wikiLink      = regexp.MustCompile(`\[([^|\]]+)\|([^\]]+)\]`)
	wikiBareLink  = regexp.MustCompile(`\[(https?://[^\]\s]+)\]`)
	wikiBold      = regexp.MustCompile(`(^|[\s(])\*([^*\n]+)\*`)
	wikiItalic    = regexp.MustCompile(`(^|[\s(])_([^_\n]+)_`)
	wikiMono      = regexp.MustCompile(`\{\{([^}]+)\}\}`)
	wikiTableHead = regexp.MustCompile(`(?m)^\|\|(.+)\|\|\s*$`)
)

// wikiToMarkdown converts the common JIRA wiki-markup constructs. Anything it does not recognize
// is kept as plain text.
func wikiToMarkdown(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	var blocks []string
	s = wikiCode.ReplaceAllStringFunc(s, func(m string) string {
		p := wikiCode.FindStringSubmatch(m)
		blocks = append(blocks, "```"+p[1]+"\n"+strings.Trim(p[2], "\n")+"\n```")
		return fmt.Sprintf("\x00%d\x00", len(blocks)-1)
	})
	s = wikiQuote.ReplaceAllStringFunc(s, func(m string) string {
		return prefixLines(strings.TrimSpace(wikiQuote.FindStringSubmatch(m)[1]), "> ")
	})
	s = wikiBullet.ReplaceAllStringFunc(s, func(m string) string {
		depth := len(strings.TrimSpace(m))
		if strings.HasPrefix(m, "-") {
			depth = 1
		}
		return strings.Repeat("  ", depth-1) + "- "
	})
	s = wikiNumbered.ReplaceAllStringFunc(s, func(m string) string {
		return strings.Repeat("   ", len(strings.TrimSpace(m))-1) + "1. "
	})
	s = wikiHeading.ReplaceAllStringFunc(s, func(m string) string {
		return strings.Repeat("#", int(m[1]-'0')) + " "
	})
	s = wikiTableHead.ReplaceAllStringFunc(s, func(m string) string {
		cells := strings.Split(strings.Trim(strings.TrimSpace(m), "|"), "||")
		return "| " + strings.Join(cells, " | ") + " |\n|" + strings.Repeat(" --- |", len(cells))
	})
	s = wikiLink.ReplaceAllString(s, "[$1]($2)")
	s = wikiBareLink.ReplaceAllString(s, "<$1>")
	s = wikiMono.ReplaceAllString(s, "`$1`")
	s = wikiBold.ReplaceAllString(s, "$1**$2**")
	s = wikiItalic.ReplaceAllString(s, "$1_${2}_")
	for i, b := range blocks {
		s = strings.Replace(s, fmt.Sprintf("\x00%d\x00", i), b, 1)
	}
	return collapseBlankLines(s)
}
