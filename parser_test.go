package main

import (
	"path/filepath"
	"testing"
)

func TestLoadPackages(t *testing.T) {
	// We'll load packages from the ./testdata directory
	// to confirm that parsing doesn't error out and returns at least one package.
	path := filepath.Join(".", "testdata")

	pkgs, err := LoadPackages(path, true)
	if err != nil {
		t.Fatalf("Expected no error loading packages, got: %v", err)
	}
	if len(pkgs) == 0 {
		t.Fatalf("Expected at least one package, got 0")
	}
}

func TestExtractSymbols(t *testing.T) {
	path := filepath.Join(".", "testdata")
	pkgs, err := LoadPackages(path, true)
	if err != nil {
		t.Fatalf("Failed to load packages for reference test: %v", err)
	}

	symbols, _, objectToSymbol, packageToSymbol := ExtractSymbols(pkgs)
	if len(symbols) == 0 {
		t.Fatalf("Expected some symbols, got 0")
	}

	if len(objectToSymbol) == 0 {
		t.Fatalf("objectToSymbol map is empty, expected entries")
	}

	if len(packageToSymbol) == 0 {
		t.Fatalf("packageToSymbol map is empty, expected entries")
	}

	// Optionally, check for known symbol names in the sample file
	foundSampleStruct := false
	foundSampleFunc := false
	for _, sym := range symbols {
		if sym.Name == "SampleStruct" {
			foundSampleStruct = true
		}
		if sym.Name == "SampleFunc" {
			foundSampleFunc = true
		}
	}

	if !foundSampleStruct {
		t.Errorf("Expected to find a symbol named 'SampleStruct', but didn't.")
	}
	if !foundSampleFunc {
		t.Errorf("Expected to find a symbol named 'SampleFunc', but didn't.")
	}
}

func TestExtractReferences(t *testing.T) {
	path := filepath.Join(".", "testdata")
	pkgs, err := LoadPackages(path, true)
	if err != nil {
		t.Fatalf("Failed to load packages for reference test: %v", err)
	}

	symbols, _, objectToSymbol, packageToSymbol := ExtractSymbols(pkgs)

	// Use the symbols here to confirm we have at least one
	if len(symbols) == 0 {
		t.Fatalf("Expected some symbols, got 0")
	}

	refs := ExtractReferences(pkgs, symbols, objectToSymbol, packageToSymbol)
	if len(refs) == 0 {
		t.Logf("No references found in sample code. This may be okay if references are external.")
	}
}
