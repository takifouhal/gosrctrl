package main

import (
	"fmt"
	"go/ast"
	"go/token"
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
	ID          int        // Unique ID for this symbol
	Name        string     // Symbol name
	Kind        SymbolKind // e.g., package, type, var, const, func, method
	PackagePath string     // Full import path of the containing package
	File        string     // Source file where the symbol is declared
	Line        int        // Line number in the file
	Column      int        // Column number in the file
	Receiver    string     // Receiver type (for methods)
}

// Reference captures a usage relationship between two symbols.
type Reference struct {
	FromID  int    // ID of the symbol using another symbol
	ToID    int    // ID of the symbol being used
	File    string // Source file where the usage occurs
	Line    int    // Line number of the usage
	Column  int    // Column number of the usage
	RefType string // e.g. "usage" (could be extended to "call", etc.)
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
// Returns:
//  1. The slice of symbols
//  2. A map from types.Object -> Symbol.ID for quick lookup of definitions
//  3. A map from *packages.Package -> Symbol.ID for the package-level symbol
func ExtractSymbols(pkgs []*packages.Package) (
	[]Symbol,
	map[types.Object]int,
	map[*packages.Package]int,
) {
	var symbols []Symbol
	objectToSymbol := make(map[types.Object]int)
	packageToSymbol := make(map[*packages.Package]int)

	var currentID int
	for _, pkg := range pkgs {
		// Create a symbol entry for the package itself
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
		packageToSymbol[pkg] = currentID

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

			// If it's a function, check if there's a receiver (making it a method)
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

			objectToSymbol[obj] = currentID
		}
	}

	return symbols, objectToSymbol, packageToSymbol
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
	// Fallback for unhandled cases
	return SymbolKindVar
}

// ExtractReferences finds usage relationships between symbols.
//  1. For each identifier in pkg.TypesInfo.Uses, if it's mapped to a known symbol (toID),
//  2. Determine the "from" symbol ID by discovering if that usage is inside a particular function.
//     If none is found, we use the package-level symbol ID.
//  3. Create a Reference with fromID, toID, and usage location.
func ExtractReferences(
	pkgs []*packages.Package,
	objectToSymbol map[types.Object]int,
	packageToSymbol map[*packages.Package]int,
) []Reference {
	var references []Reference

	for _, pkg := range pkgs {
		// Build a map of positions that correspond to a call expression
		callPositions := map[token.Pos]bool{}
		for _, f := range pkg.Syntax {
			if f == nil {
				continue
			}
			ast.Inspect(f, func(n ast.Node) bool {
				if c, ok := n.(*ast.CallExpr); ok {
					switch fn := c.Fun.(type) {
					case *ast.Ident:
						callPositions[fn.Pos()] = true
					case *ast.SelectorExpr:
						callPositions[fn.Sel.Pos()] = true
					}
				}
				return true
			})
		}

		// The package-level symbol ID for references at the top level
		pkgSymbolID := packageToSymbol[pkg]

		// Build a map of filename -> *ast.File
		fileMap := make(map[string]*ast.File)
		for _, f := range pkg.Syntax {
			if f == nil {
				continue
			}
			fileName := pkg.Fset.File(f.Pos()).Name()
			fileMap[fileName] = f
		}

		// For each usage identified in pkg.TypesInfo.Uses
		for id, obj := range pkg.TypesInfo.Uses {
			toID, ok := objectToSymbol[obj]
			if !ok {
				// Usage is referencing an external or unknown symbol; skip it
				continue
			}
			pos := pkg.Fset.Position(id.Pos())
			if !pos.IsValid() {
				continue
			}

			// Determine fromID
			fromID := pkgSymbolID
			fileAst := fileMap[pos.Filename]
			if fileAst != nil {
				funcID := findEnclosingFunctionID(id.Pos(), fileAst, pkg, objectToSymbol)
				if funcID != -1 {
					fromID = funcID
				}
			}

			// Decide reference type
			refType := "usage"
			if callPositions[id.Pos()] {
				refType = "call"
			}

			// Build the reference
			r := Reference{
				FromID:  fromID,
				ToID:    toID,
				File:    pos.Filename,
				Line:    pos.Line,
				Column:  pos.Column,
				RefType: refType,
			}
			references = append(references, r)
		}
	}

	return references
}

// findEnclosingFunctionID checks whether a position belongs to the body of a known function/method.
// If found, returns that function's symbol ID; otherwise returns -1.
func findEnclosingFunctionID(
	pos token.Pos,
	fileAst *ast.File,
	pkg *packages.Package,
	objectToSymbol map[types.Object]int,
) int {
	for _, decl := range fileAst.Decls {
		if funcDecl, ok := decl.(*ast.FuncDecl); ok {
			// Check if pos is inside the function's body
			if funcDecl.Body != nil && funcDecl.Body.Pos() <= pos && pos < funcDecl.Body.End() {
				// This identifier is within funcDecl's body
				fnObj := pkg.TypesInfo.Defs[funcDecl.Name]
				if fnObj != nil {
					if fnID, ok := objectToSymbol[fnObj]; ok {
						return fnID
					}
				}
			}
		}
	}
	return -1
}
