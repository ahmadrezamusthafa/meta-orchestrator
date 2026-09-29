package connectors

import (
	"encoding/json"
	"strings"
	"testing"
)

func adf(t *testing.T, doc string) interface{} {
	t.Helper()
	var v interface{}
	if err := json.Unmarshal([]byte(doc), &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestADFToMarkdownKeepsStructure(t *testing.T) {
	doc := adf(t, `{"type":"doc","version":1,"content":[
	 {"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"Background"}]},
	 {"type":"paragraph","content":[{"type":"text","text":"Users need "},{"type":"text","text":"faster","marks":[{"type":"strong"}]},
	   {"type":"text","text":" checkout. See "},{"type":"text","text":"spec","marks":[{"type":"link","attrs":{"href":"https://x.io/spec"}}]},{"type":"text","text":"."}]},
	 {"type":"heading","attrs":{"level":3},"content":[{"type":"text","text":"Acceptance criteria"}]},
	 {"type":"orderedList","attrs":{"order":1},"content":[
	   {"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"Pay with Apple Pay"}]},
	     {"type":"bulletList","content":[{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"Safari only"}]}]}]}]},
	   {"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"Refund works"}]}]}]},
	 {"type":"codeBlock","attrs":{"language":"go"},"content":[{"type":"text","text":"func Pay() {}"}]},
	 {"type":"table","content":[
	   {"type":"tableRow","content":[{"type":"tableHeader","content":[{"type":"paragraph","content":[{"type":"text","text":"Field"}]}]},{"type":"tableHeader","content":[{"type":"paragraph","content":[{"type":"text","text":"Rule"}]}]}]},
	   {"type":"tableRow","content":[{"type":"tableCell","content":[{"type":"paragraph","content":[{"type":"text","text":"amount"}]}]},{"type":"tableCell","content":[{"type":"paragraph","content":[{"type":"text","text":"> 0"}]}]}]}]},
	 {"type":"taskList","content":[{"type":"taskItem","attrs":{"state":"DONE"},"content":[{"type":"text","text":"Design"}]},{"type":"taskItem","attrs":{"state":"TODO"},"content":[{"type":"text","text":"Build"}]}]}
	]}`)
	got := parseJiraDescription(doc)
	for _, want := range []string{
		"## Background",
		"Users need **faster** checkout. See [spec](https://x.io/spec).",
		"### Acceptance criteria",
		"1. Pay with Apple Pay\n   - Safari only\n2. Refund works",
		"```go\nfunc Pay() {}\n```",
		"| Field | Rule |\n| --- | --- |\n| amount | > 0 |",
		"- [x] Design\n- [ ] Build",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestWikiMarkupToMarkdown(t *testing.T) {
	got := parseJiraDescription("h2. Goal\nMake *checkout* faster, see [spec|https://x.io].\n* one\n** nested\n# first\n{code:java}int x;{code}\n{{monospace}}")
	for _, want := range []string{"## Goal", "Make **checkout** faster, see [spec](https://x.io).", "- one\n  - nested", "1. first", "```java\nint x;\n```", "`monospace`"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}
