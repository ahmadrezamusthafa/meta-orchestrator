package ast

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Language represents supported source languages for syntactic AST sharding.
type Language string

const (
	LangTypeScript Language = "typescript"
	LangGo         Language = "go"
	LangPHP        Language = "php"
	LangPython     Language = "python"
)

// ASTSymbol represents an extracted syntactic declaration.
type ASTSymbol struct {
	Name      string `json:"name"`
	Kind      string `json:"kind"` // "class", "interface", "function", "method", "route"
	StartLine int    `json:"start_line"`
	Signature string `json:"signature"`
}

// ASTContextSlice encapsulates compact syntactic context with token savings metrics.
type ASTContextSlice struct {
	FilePath          string      `json:"file_path"`
	Language          Language    `json:"language"`
	OriginalLines     int         `json:"original_lines"`
	ShardedLines      int         `json:"sharded_lines"`
	TokenReductionPct float64     `json:"token_reduction_pct"`
	Symbols           []ASTSymbol `json:"symbols"`
	ShardedContent    string      `json:"sharded_content"`
}

// Sharder extracts high-leverage signatures and slices out body implementations.
type Sharder struct{}

// NewSharder creates a new AST sharder instance.
func NewSharder() *Sharder {
	return &Sharder{}
}

// DetectLanguage resolves the language from file extension.
func (s *Sharder) DetectLanguage(filePath string) Language {
	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".ts", ".tsx", ".js", ".vue":
		return LangTypeScript
	case ".go":
		return LangGo
	case ".php":
		return LangPHP
	case ".py":
		return LangPython
	default:
		return LangTypeScript
	}
}

// ShardFile reads a source file and extracts signatures, interfaces, and symbol contracts.
func (s *Sharder) ShardFile(filePath string) (*ASTContextSlice, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open source file: %w", err)
	}
	defer file.Close()

	lang := s.DetectLanguage(filePath)
	scanner := bufio.NewScanner(file)

	var symbols []ASTSymbol
	var shardedBuilder strings.Builder
	lineNum := 0
	originalLines := 0

	// Regex patterns for syntactic definitions
	classRegex := regexp.MustCompile(`(?m)^\s*(export\s+)?(class|interface|type)\s+([A-Za-z0-9_]+)`)
	funcRegex := regexp.MustCompile(`(?m)^\s*(export\s+)?(async\s+)?(function|func|def)\s+([A-Za-z0-9_]+)`)
	methodRegex := regexp.MustCompile(`(?m)^\s*(public|protected|private)?\s*(async\s+)?([A-Za-z0-9_]+)\s*\([^)]*\)\s*([{:]|\s*->)`)
	routeRegex := regexp.MustCompile(`(?m)(Route::|router\.|app\.)(get|post|put|delete|patch)\s*\(`)

	shardedBuilder.WriteString(fmt.Sprintf("// --- Syntactic AST Slice: %s [%s] ---\n", filepath.Base(filePath), lang))

	for scanner.Scan() {
		lineNum++
		originalLines++
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Retain import statements
		if strings.HasPrefix(trimmed, "import ") || strings.HasPrefix(trimmed, "use ") || strings.HasPrefix(trimmed, "from ") || strings.HasPrefix(trimmed, "package ") {
			shardedBuilder.WriteString(line + "\n")
			continue
		}

		// Detect Class/Interface/Type
		if match := classRegex.FindStringSubmatch(line); len(match) > 3 {
			sym := ASTSymbol{
				Name:      match[3],
				Kind:      match[2],
				StartLine: lineNum,
				Signature: trimmed,
			}
			symbols = append(symbols, sym)
			shardedBuilder.WriteString(fmt.Sprintf("\n%s { /* implementation omitted */ }\n", trimmed))
			continue
		}

		// Detect Functions
		if match := funcRegex.FindStringSubmatch(line); len(match) > 4 {
			sym := ASTSymbol{
				Name:      match[4],
				Kind:      "function",
				StartLine: lineNum,
				Signature: trimmed,
			}
			symbols = append(symbols, sym)
			shardedBuilder.WriteString(fmt.Sprintf("%s { ... }\n", trimmed))
			continue
		}

		// Detect Methods
		if match := methodRegex.FindStringSubmatch(line); len(match) > 3 && !strings.HasPrefix(trimmed, "if") && !strings.HasPrefix(trimmed, "for") {
			sym := ASTSymbol{
				Name:      match[3],
				Kind:      "method",
				StartLine: lineNum,
				Signature: trimmed,
			}
			symbols = append(symbols, sym)
			shardedBuilder.WriteString(fmt.Sprintf("  %s { ... }\n", trimmed))
			continue
		}

		// Detect Routes
		if routeRegex.MatchString(line) {
			sym := ASTSymbol{
				Name:      trimmed,
				Kind:      "route",
				StartLine: lineNum,
				Signature: trimmed,
			}
			symbols = append(symbols, sym)
			shardedBuilder.WriteString(trimmed + "\n")
			continue
		}
	}

	shardedContent := shardedBuilder.String()
	shardedLines := strings.Count(shardedContent, "\n")
	reductionPct := 0.0
	if originalLines > 0 {
		reductionPct = float64(originalLines-shardedLines) / float64(originalLines) * 100.0
		if reductionPct < 0 {
			reductionPct = 0
		}
	}

	return &ASTContextSlice{
		FilePath:          filePath,
		Language:          lang,
		OriginalLines:     originalLines,
		ShardedLines:      shardedLines,
		TokenReductionPct: reductionPct,
		Symbols:           symbols,
		ShardedContent:    shardedContent,
	}, nil
}
