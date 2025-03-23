package main

import (
    "os"
    "path/filepath"
    "testing"
    "encoding/json"
)

func TestExportToJSON(t *testing.T) {
    // Prepare some dummy symbols and references
    symbols := []Symbol{
        {
            ID:          1,
            Name:        "TestSymbol",
            Kind:        SymbolKindVar,
            PackagePath: "test/package",
            File:        "test.go",
            Line:        10,
            Column:      5,
            Receiver:    "",
        },
    }
    references := []Reference{
        {
            FromID:  1,
            ToID:    1,
            File:    "test.go",
            Line:    12,
            Column:  3,
            RefType: "usage",
        },
    }

    tmpFile := filepath.Join(os.TempDir(), "gosrctrl_test_output.json")
    defer os.Remove(tmpFile)

    err := ExportToJSON(tmpFile, symbols, references)
    if err != nil {
        t.Fatalf("ExportToJSON failed: %v", err)
    }

    // Now read it back and check the contents
    f, err := os.Open(tmpFile)
    if err != nil {
        t.Fatalf("Failed to open exported JSON: %v", err)
    }
    defer f.Close()

    var data DataModel
    dec := json.NewDecoder(f)
    if err := dec.Decode(&data); err != nil {
        t.Fatalf("Failed to decode JSON: %v", err)
    }

    if len(data.Symbols) != 1 {
        t.Errorf("Expected 1 symbol, got %d", len(data.Symbols))
    }
    if len(data.References) != 1 {
        t.Errorf("Expected 1 reference, got %d", len(data.References))
    }
}