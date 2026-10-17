package ast

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// The sharder is a line-based, regular-expression outline of a source file: declarations are
// recognised per language by their opening line, bodies are left out. It is not a parser, so a
// declaration split over several lines or produced by a macro can be missed.

// Language represents supported source languages.
type Language string

const (
	LangTypeScript Language = "typescript"
	LangGo         Language = "go"
	LangPHP        Language = "php"
	LangPython     Language = "python"
)

// ErrUnsupportedLanguage is returned for files whose language the sharder does not outline.
var ErrUnsupportedLanguage = errors.New("unsupported language: outlines cover .go, .ts/.tsx/.js/.jsx/.mjs/.vue, .php and .py files")

// ASTSymbol represents an extracted declaration.
type ASTSymbol struct {
	Name      string `json:"name"`
	Kind      string `json:"kind"` // "class", "interface", "type", "function", "method", "route"
	StartLine int    `json:"start_line"`
	Signature string `json:"signature"`
}

// ASTContextSlice is a file's outline. LineReductionPct is how many fewer lines the outline has
// than the file; it is a line count, not a token measurement.
type ASTContextSlice struct {
	FilePath         string      `json:"file_path"`
	Language         Language    `json:"language"`
	OriginalLines    int         `json:"original_lines"`
	ShardedLines     int         `json:"sharded_lines"`
	LineReductionPct float64     `json:"line_reduction_pct"`
	Symbols          []ASTSymbol `json:"symbols"`
	ShardedContent   string      `json:"sharded_content"`
}

// Sharder extracts declarations and leaves out their bodies.
type Sharder struct{}

// NewSharder creates a new sharder instance.
func NewSharder() *Sharder {
	return &Sharder{}
}

// DetectLanguage resolves the language from the file extension; "" when unsupported.
func (s *Sharder) DetectLanguage(filePath string) Language {
	switch strings.ToLower(filepath.Ext(filePath)) {
	case ".ts", ".tsx", ".js", ".jsx", ".mjs", ".vue":
		return LangTypeScript
	case ".go":
		return LangGo
	case ".php":
		return LangPHP
	case ".py":
		return LangPython
	}
	return ""
}

type pattern struct {
	re   *regexp.Regexp
	kind string // "" takes the kind from the "kind" group
}

var (
	tsClass   = pattern{regexp.MustCompile(`^\s*(?:export\s+)?(?:default\s+)?(?:abstract\s+)?(?P<kind>class|interface|type|enum)\s+(?P<name>[A-Za-z0-9_$]+)`), ""}
	tsFunc    = pattern{regexp.MustCompile(`^\s*(?:export\s+)?(?:default\s+)?(?:async\s+)?function\s*\*?\s*(?P<name>[A-Za-z0-9_$]+)`), "function"}
	tsArrow   = pattern{regexp.MustCompile(`^\s*(?:export\s+)?const\s+(?P<name>[A-Za-z0-9_$]+)\s*=\s*(?:async\s+)?(?:\([^)]*\)|[A-Za-z0-9_$]+)\s*(?::\s*[^=]+)?=>`), "function"}
	tsMethod  = pattern{regexp.MustCompile(`^\s+(?:(?:public|protected|private|static|readonly|override|abstract|async|get|set)\s+)*(?P<name>[A-Za-z0-9_$]+)\s*(?:<[^>]*>)?\([^)]*\)\s*(?::\s*[^{]+)?\{\s*$`), "method"}
	route     = pattern{regexp.MustCompile(`(?:Route::|router\.|app\.)(?:get|post|put|delete|patch)\s*\(`), "route"}
	goType    = pattern{regexp.MustCompile(`^type\s+(?P<name>[A-Za-z0-9_]+)(?:\[[^\]]*\])?\s+(?P<kind>struct|interface)\b`), ""}
	goTypeAny = pattern{regexp.MustCompile(`^type\s+(?P<name>[A-Za-z0-9_]+)\b`), "type"}
	goMethod  = pattern{regexp.MustCompile(`^func\s*\([^)]*\)\s*(?P<name>[A-Za-z0-9_]+)\s*[\[(]`), "method"}
	goFunc    = pattern{regexp.MustCompile(`^func\s+(?P<name>[A-Za-z0-9_]+)\s*[\[(]`), "function"}
	phpClass  = pattern{regexp.MustCompile(`^\s*(?:(?:abstract|final|readonly)\s+)*(?P<kind>class|interface|trait|enum)\s+(?P<name>[A-Za-z0-9_]+)`), ""}
	phpFunc   = pattern{regexp.MustCompile(`^\s*(?:(?:public|protected|private|static|abstract|final)\s+)*function\s+&?\s*(?P<name>[A-Za-z0-9_]+)\s*\(`), "function"}
	pyClass   = pattern{regexp.MustCompile(`^\s*class\s+(?P<name>[A-Za-z0-9_]+)`), "class"}
	pyFunc    = pattern{regexp.MustCompile(`^(?P<indent>\s*)(?:async\s+)?def\s+(?P<name>[A-Za-z0-9_]+)\s*\(`), "function"}
)

var languagePatterns = map[Language][]pattern{
	LangTypeScript: {tsClass, tsFunc, tsArrow, route, tsMethod},
	LangGo:         {goType, goTypeAny, goMethod, goFunc},
	LangPHP:        {phpClass, phpFunc, route},
	LangPython:     {pyClass, pyFunc},
}

var tsKeywords = map[string]bool{"if": true, "for": true, "while": true, "switch": true, "catch": true, "function": true, "return": true, "with": true}

func match(p pattern, line string) (name, kind string, ok bool) {
	m := p.re.FindStringSubmatch(line)
	if m == nil {
		return "", "", false
	}
	kind = p.kind
	for i, g := range p.re.SubexpNames() {
		switch g {
		case "name":
			name = m[i]
		case "kind":
			kind = m[i]
		case "indent":
			if m[i] != "" {
				kind = "method"
			}
		}
	}
	if p.kind == "route" {
		name = strings.TrimSpace(line)
	}
	return name, kind, true
}

// maxLineBytes bounds one source line (minified files have very long ones).
const maxLineBytes = 1 << 20

// ShardFile reads a source file and outlines its imports and declarations.
func (s *Sharder) ShardFile(filePath string) (*ASTContextSlice, error) {
	lang := s.DetectLanguage(filePath)
	if lang == "" {
		return nil, ErrUnsupportedLanguage
	}
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open source file: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), maxLineBytes)
	symbols := []ASTSymbol{}
	var out strings.Builder
	fmt.Fprintf(&out, "// outline: %s [%s]\n", filepath.Base(filePath), lang)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "import ") || strings.HasPrefix(trimmed, "use ") || strings.HasPrefix(trimmed, "from ") || strings.HasPrefix(trimmed, "package ") {
			out.WriteString(line + "\n")
			continue
		}
		for _, p := range languagePatterns[lang] {
			name, kind, ok := match(p, line)
			if !ok || (lang == LangTypeScript && kind == "method" && tsKeywords[name]) {
				continue
			}
			symbols = append(symbols, ASTSymbol{Name: name, Kind: kind, StartLine: lineNum, Signature: trimmed})
			out.WriteString(line + "\n")
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read source file: %w", err)
	}

	content := out.String()
	shardedLines := strings.Count(content, "\n")
	reduction := 0.0
	if lineNum > 0 && shardedLines < lineNum {
		reduction = float64(lineNum-shardedLines) / float64(lineNum) * 100.0
	}
	return &ASTContextSlice{FilePath: filePath, Language: lang, OriginalLines: lineNum, ShardedLines: shardedLines,
		LineReductionPct: reduction, Symbols: symbols, ShardedContent: content}, nil
}
