package main

import (
	"fmt"
	"go/types"

	"golang.org/x/tools/go/packages"
)

// SymbolKind represents the category/kind of a symbol (package, type, var, func, etc.)
type SymbolKind string

const (
	SymbolKindPackage SymbolKind = "package"
	SymbolKindType    SymbolKind = "type"
	SymbolKindVar     SymbolKind = "var"
	SymbolKindFunc    SymbolKind = "func"
	SymbolKindMethod  SymbolKind = "method"
	SymbolKindConst   SymbolKind = "const"
)

// Symbol stores metadata about a single declared symbol in the codebase
type Symbol struct {
	ID          int         // Unique ID for this symbol
	Name        string      // Symbol name
	Kind        SymbolKind  // e.g., package, type, var, const, func, method
	PackagePath string      // Full import path of the containing package
	File        string      // Source file where the symbol is declared
	Line        int         // Line number in the file
	Column      int         // Column number in the file
	Receiver    string      // Receiver type (for methods)
}

// LoadPackages loads and parses all Go packages beneath the specified path.
// It returns a slice of parsed packages (with syntax and type info) or an error.
func LoadPackages(path string) ([]*packages.Package, error) {
	cfg := &packages.Config{
		Mode: packages.NeedName |
			packages.NeedFiles |
			packages.NeedSyntax |
			packages.NeedTypes |
			packages.NeedTypesInfo,
		Dir: path,
	}

	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		return nil, fmt.Errorf("failed to load packages: %w", err)
	}

	// Check for any loading errors in the loaded packages
	if packages.PrintErrors(pkgs) > 0 {
		return nil, fmt.Errorf("errors encountered while loading packages")
	}

	return pkgs, nil
}

// ExtractSymbols traverses the provided Go packages and collects symbol definitions
// (functions, methods, types, variables, constants, etc.).
func ExtractSymbols(pkgs []*packages.Package) []Symbol {
	var symbols []Symbol
	var currentID int

	for _, pkg := range pkgs {
		// Create a symbol entry for the package itself (optional, useful for indexing context)
		currentID++
		packageSymbol := Symbol{
			ID:          currentID,
			Name:        pkg.Name,
			Kind:        SymbolKindPackage,
			PackagePath: pkg.PkgPath,
			File:        "",
			Line:        0,
			Column:      0,
			Receiver:    "",
		}
		symbols = append(symbols, packageSymbol)

		// For each defined identifier in pkg.TypesInfo.Defs, collect a symbol
		for id, obj := range pkg.TypesInfo.Defs {
			if obj == nil {
				// This identifier is not a definition
				continue
			}

			pos := pkg.Fset.Position(id.Pos())
			if !pos.IsValid() {
				// Ignore definitions with invalid positions
				continue
			}

			kind := classifyObject(obj)
			receiver := ""

			// If it's a function, check if there's a receiver to identify methods
			if fn, ok := obj.(*types.Func); ok {
				sig, _ := fn.Type().(*types.Signature)
				if sig != nil && sig.Recv() != nil {
					receiver = sig.Recv().Type().String()
				}
			}

			currentID++
			symbol := Symbol{
				ID:          currentID,
				Name:        obj.Name(),
				Kind:        kind,
				PackagePath: pkg.PkgPath,
				File:        pos.Filename,
				Line:        pos.Line,
				Column:      pos.Column,
				Receiver:    receiver,
			}
			symbols = append(symbols, symbol)
		}
	}

	return symbols
}

// classifyObject inspects a types.Object to determine its symbol kind.
func classifyObject(obj types.Object) SymbolKind {
	switch o := obj.(type) {
	case *types.PkgName:
		return SymbolKindPackage
	case *types.TypeName:
		return SymbolKindType
	case *types.Const:
		return SymbolKindConst
	case *types.Var:
		return SymbolKindVar
	case *types.Func:
		// Determine if it's a method by checking for a receiver
		if sig, ok := o.Type().(*types.Signature); ok && sig.Recv() != nil {
			return SymbolKindMethod
		}
		return SymbolKindFunc
	}
	// Default fallback for unhandled cases
	return SymbolKindVar
}