package ast

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func outline(t *testing.T, name, src string) *ASTContextSlice {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := NewSharder().ShardFile(path)
	if err != nil {
		t.Fatalf("shard %s: %v", name, err)
	}
	return s
}

func symbols(s *ASTContextSlice) string {
	var out []string
	for _, sym := range s.Symbols {
		out = append(out, fmt.Sprintf("%s:%s@%d", sym.Kind, sym.Name, sym.StartLine))
	}
	return strings.Join(out, " ")
}

func TestTypeScriptOutline(t *testing.T) {
	var sb strings.Builder
	sb.WriteString("import { Database } from './db';\n\nexport class OrderService {\n  public async processOrder(orderId: string): Promise<boolean> {\n")
	for i := 0; i < 200; i++ {
		fmt.Fprintf(&sb, "    if (step%d) {\n      await this.db.calculateTax(%d);\n    }\n", i, i)
	}
	sb.WriteString("    return true;\n  }\n\n  cancelOrder(id: string) {\n    return;\n  }\n}\nexport const total = (a: number) => a * 2;\n")
	s := outline(t, "OrderService.ts", sb.String())
	if got := symbols(s); got != "class:OrderService@3 method:processOrder@4 method:cancelOrder@608 function:total@612" {
		t.Fatalf("symbols = %s", got)
	}
	if s.Language != LangTypeScript || s.OriginalLines != 612 || s.ShardedLines != 6 {
		t.Fatalf("lines %d → %d", s.OriginalLines, s.ShardedLines)
	}
	want := float64(612-6) / 612 * 100
	if s.LineReductionPct != want {
		t.Fatalf("line reduction %.2f, want %.2f", s.LineReductionPct, want)
	}
}

func TestGoOutlineFindsMethodsAndTypes(t *testing.T) {
	s := outline(t, "server.go", `package main

import "fmt"

type Server struct {
	addr string
}

type Handler interface{ Serve() }

type ID string

func (s *Server) Start(port int) error {
	go func() {
		fmt.Println("x")
	}()
	return nil
}

func (Server) name() string { return "" }

func Map[T any](in []T) []T { return in }

func HandleRequest(id string) error {
	return nil
}
`)
	want := "struct:Server@5 interface:Handler@9 type:ID@11 method:Start@13 method:name@20 function:Map@22 function:HandleRequest@24"
	if got := symbols(s); got != want {
		t.Fatalf("symbols = %s\nwant      %s", got, want)
	}
}

func TestPythonAndPHPOutline(t *testing.T) {
	py := outline(t, "svc.py", "class Refund:\n    def apply(self):\n        pass\n\nasync def main():\n    pass\n")
	if got := symbols(py); got != "class:Refund@1 method:apply@2 function:main@5" {
		t.Fatalf("python = %s", got)
	}
	php := outline(t, "Refunds.php", "<?php\nfinal class Refunds {\n  public static function apply($id) {\n  }\n}\nRoute::post('/refunds', [Refunds::class, 'apply']);\n")
	if got := symbols(php); got != "class:Refunds@2 function:apply@3 route:Route::post('/refunds', [Refunds::class, 'apply']);@6" {
		t.Fatalf("php = %s", got)
	}
}

func TestUnknownFilesAreNotOutlined(t *testing.T) {
	sh := NewSharder()
	if l := sh.DetectLanguage("README.md"); l != "" {
		t.Fatalf("README.md detected as %q", l)
	}
	path := filepath.Join(t.TempDir(), "notes.txt")
	_ = os.WriteFile(path, []byte("function x() {}"), 0o644)
	if _, err := sh.ShardFile(path); !errors.Is(err, ErrUnsupportedLanguage) {
		t.Fatalf("err = %v", err)
	}
}
