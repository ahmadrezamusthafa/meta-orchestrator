package ast

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestASTSharderTokenReduction(t *testing.T) {
	tmpDir := t.TempDir()
	sourceFile := filepath.Join(tmpDir, "OrderService.ts")

	// Generate a 500-line simulated TypeScript service with large method bodies
	var sb strings.Builder
	sb.WriteString("import { Database } from './db';\n")
	sb.WriteString("import { PaymentGateway } from './payment';\n\n")
	sb.WriteString("export class OrderService {\n")
	sb.WriteString("  public async processOrder(orderId: string): Promise<boolean> {\n")
	for i := 0; i < 200; i++ {
		sb.WriteString(fmt.Sprintf("    // Heavy computational logic step %d\n    const step%d = await this.db.calculateTax(%d);\n", i, i, i))
	}
	sb.WriteString("    return true;\n  }\n\n")
	sb.WriteString("  public async cancelOrder(orderId: string): Promise<void> {\n")
	for i := 0; i < 150; i++ {
		sb.WriteString(fmt.Sprintf("    // Heavy rollback logic step %d\n    await this.db.rollbackTransaction(%d);\n", i, i))
	}
	sb.WriteString("  }\n}\n")

	if err := os.WriteFile(sourceFile, []byte(sb.String()), 0644); err != nil {
		t.Fatalf("failed to write test source: %v", err)
	}

	sharder := NewSharder()
	slice, err := sharder.ShardFile(sourceFile)
	if err != nil {
		t.Fatalf("failed to shard file: %v", err)
	}

	if slice.OriginalLines < 400 {
		t.Errorf("expected >400 original lines, got %d", slice.OriginalLines)
	}
	if slice.TokenReductionPct < 90.0 {
		t.Errorf("expected >90%% token reduction, got %.2f%%", slice.TokenReductionPct)
	}

	// Verify symbols preserved
	foundProcessOrder := false
	for _, sym := range slice.Symbols {
		if strings.Contains(sym.Signature, "processOrder") {
			foundProcessOrder = true
			break
		}
	}
	if !foundProcessOrder {
		t.Errorf("processOrder signature was not extracted in symbols")
	}

	if !strings.Contains(slice.ShardedContent, "processOrder") {
		t.Errorf("sharded content missing processOrder signature")
	}
}

func TestASTSharderGoFile(t *testing.T) {
	tmpDir := t.TempDir()
	sourceFile := filepath.Join(tmpDir, "server.go")

	content := `package main

import "fmt"

func HandleRequest(id string) error {
	for i := 0; i < 50; i++ {
		fmt.Println("processing", i)
	}
	return nil
}
`
	_ = os.WriteFile(sourceFile, []byte(content), 0644)

	sharder := NewSharder()
	slice, err := sharder.ShardFile(sourceFile)
	if err != nil {
		t.Fatalf("failed to shard go file: %v", err)
	}

	if slice.Language != LangGo {
		t.Errorf("expected LangGo, got %s", slice.Language)
	}
	if len(slice.Symbols) == 0 {
		t.Errorf("expected at least 1 symbol extracted")
	}
}
