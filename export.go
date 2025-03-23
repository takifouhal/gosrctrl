package main

import (
    "encoding/json"
    "fmt"
    "os"
)

// DataModel holds all symbols and references to be serialized to JSON.
type DataModel struct {
    Symbols    []Symbol    `json:"symbols"`
    References []Reference `json:"references"`
    Modules    []GoModule  `json:"modules"`
}
type GoModule struct {
    Path    string `json:"path"`
    Version string `json:"version"`
}


// ExportToJSON serializes the symbols and references to a JSON file at jsonFilePath.
func ExportToJSON(jsonFilePath string, symbols []Symbol, references []Reference, modules []GoModule) error {
    data := DataModel{
        Symbols:    symbols,
        References: references,
        Modules:    modules,
    }

    f, err := os.Create(jsonFilePath)
    if err != nil {
        return fmt.Errorf("failed to create JSON output file: %w", err)
    }
    defer f.Close()

    encoder := json.NewEncoder(f)
    encoder.SetIndent("", "  ")

    if err := encoder.Encode(data); err != nil {
        return fmt.Errorf("failed to encode JSON data: %w", err)
    }

    return nil
}